package queueti

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	pb "github.com/Joessst-Dev/queue-ti/pb"
)

// HandlerFunc processes a single message.
// Return nil to Ack the message; return a non-nil error to Nack it with that
// error string.
type HandlerFunc func(ctx context.Context, msg *Message) error

// Consumer subscribes to a topic and dispatches messages to a handler.
type Consumer struct {
	client *Client
	topic  string
	cfg    consumerConfig
}

const (
	backoffStart = 500 * time.Millisecond
	backoffMax   = 30 * time.Second
)

// Consume opens a Subscribe stream to the server and dispatches incoming
// messages to handler, running up to cfg.concurrency handlers concurrently.
//
// Consume blocks until ctx is cancelled. On stream errors it reconnects with
// exponential backoff (500ms → 30s). Panics from handler are recovered and
// treated as a Nack.
//
// The handler's return value controls acknowledgement:
//   - nil   → Ack
//   - error → Nack with error.Error() as the reason
func (c *Consumer) Consume(ctx context.Context, handler HandlerFunc) error {
	sem := make(chan struct{}, c.cfg.concurrency)
	backoff := backoffStart

	for {
		req := &pb.SubscribeRequest{
			Topic: c.topic,
		}
		if c.cfg.visibilityTimeout > 0 {
			vt := c.cfg.visibilityTimeout
			req.VisibilityTimeoutSeconds = &vt
		}

		stream, err := c.client.pb.Subscribe(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("queue-ti consumer: Subscribe error (retrying in %s): %v", backoff, err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		// inner loop: drain the stream until it ends or errors.
		streamHealthy := c.drainStream(ctx, stream, handler, sem, &backoff)
		if !streamHealthy {
			// stream error — reconnect with backoff.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
		}

		if ctx.Err() != nil {
			return nil
		}
	}
}

// drainStream reads messages from stream and dispatches each to handler until
// the stream ends or an error occurs. It returns true when the stream ended
// cleanly (EOF or ctx cancellation) and false on a stream error.
func (c *Consumer) drainStream(
	ctx context.Context,
	stream pb.QueueService_SubscribeClient,
	handler HandlerFunc,
	sem chan struct{},
	backoff *time.Duration,
) (cleanExit bool) {
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				return true
			}
			log.Printf("queue-ti consumer: stream Recv error (will reconnect): %v", err)
			return false
		}

		// First successful Recv resets the reconnect backoff.
		*backoff = backoffStart

		msg := protoToMessage(resp, c.client)

		// Acquire a concurrency slot before spawning.
		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			c.dispatch(ctx, msg, handler)
		}()
	}
}

// dispatch calls handler for msg, recovering any panic as a Nack, and
// acks or nacks based on the returned error.
func (c *Consumer) dispatch(ctx context.Context, msg *Message, handler HandlerFunc) {
	var handlerErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				handlerErr = fmt.Errorf("panic in handler: %v", r)
			}
		}()
		handlerErr = handler(ctx, msg)
	}()

	if handlerErr == nil {
		if err := msg.Ack(ctx); err != nil && ctx.Err() == nil {
			log.Printf("queue-ti consumer: Ack failed for message %s: %v", msg.ID, err)
		}
		return
	}

	if err := msg.Nack(ctx, handlerErr.Error()); err != nil && ctx.Err() == nil {
		log.Printf("queue-ti consumer: Nack failed for message %s: %v", msg.ID, err)
	}
}

// nextBackoff doubles b, capped at backoffMax.
func nextBackoff(b time.Duration) time.Duration {
	b *= 2
	if b > backoffMax {
		return backoffMax
	}
	return b
}

// ConsumeBatch polls the queue in batches, calling handler with each batch.
// Each message in the batch has Ack and Nack closures that call back to the
// server. ConsumeBatch returns when ctx is cancelled or a non-retriable error
// occurs. When the queue is empty it applies the same exponential backoff as
// Consume (500ms → 30s). When messages are returned the backoff resets.
func (c *Consumer) ConsumeBatch(ctx context.Context, topic string, batchSize int, handler func(ctx context.Context, messages []*Message) error) error {
	backoff := backoffStart

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		req := &pb.BatchDequeueRequest{
			Topic: topic,
			Count: uint32(batchSize),
		}
		if c.cfg.visibilityTimeout > 0 {
			vt := c.cfg.visibilityTimeout
			req.VisibilityTimeoutSeconds = &vt
		}

		resp, err := c.client.pb.BatchDequeue(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("queue-ti consumer: BatchDequeue error (retrying in %s): %v", backoff, err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		if len(resp.Messages) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff = nextBackoff(backoff)
			continue
		}

		backoff = backoffStart

		msgs := make([]*Message, len(resp.Messages))
		for i, m := range resp.Messages {
			msgs[i] = dequeueProtoToMessage(m, c.client)
		}

		if err := handler(ctx, msgs); err != nil && ctx.Err() == nil {
			log.Printf("queue-ti consumer: batch handler error: %v", err)
		}
	}
}

func dequeueProtoToMessage(resp *pb.DequeueResponse, c *Client) *Message {
	var createdAt time.Time
	if resp.CreatedAt != nil {
		createdAt = resp.CreatedAt.AsTime()
	}
	return buildMessage(resp.Id, resp.Topic, resp.Payload, resp.Metadata, createdAt, int(resp.RetryCount), c)
}

func protoToMessage(resp *pb.SubscribeResponse, c *Client) *Message {
	var createdAt time.Time
	if resp.CreatedAt != nil {
		createdAt = resp.CreatedAt.AsTime()
	}
	return buildMessage(resp.Id, resp.Topic, resp.Payload, resp.Metadata, createdAt, int(resp.RetryCount), c)
}
