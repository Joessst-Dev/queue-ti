package queueti

import (
	"context"
	"fmt"
	"time"

	pb "github.com/Joessst-Dev/queue-ti/pb"
)

// Message is a single message received from the queue.
type Message struct {
	ID         string
	Topic      string
	Payload    []byte
	Metadata   map[string]string
	CreatedAt  time.Time
	RetryCount int

	ack  func(ctx context.Context) error
	nack func(ctx context.Context, reason string) error
}

// Ack acknowledges the message, removing it from the queue permanently.
func (m *Message) Ack(ctx context.Context) error {
	return m.ack(ctx)
}

// Nack negatively acknowledges the message. reason is stored as the error
// string. The message becomes visible again after the visibility timeout.
func (m *Message) Nack(ctx context.Context, reason string) error {
	return m.nack(ctx, reason)
}

// buildMessage constructs a Message with ack/nack closures wired to the server.
func buildMessage(id, topic string, payload []byte, metadata map[string]string, createdAt time.Time, retryCount int, c *Client) *Message {
	msg := &Message{
		ID:         id,
		Topic:      topic,
		Payload:    payload,
		Metadata:   metadata,
		CreatedAt:  createdAt,
		RetryCount: retryCount,
	}
	msg.ack = func(ctx context.Context) error {
		_, err := c.pb.Ack(ctx, &pb.AckRequest{Id: id})
		if err != nil {
			return fmt.Errorf("ack message %s: %w", id, err)
		}
		return nil
	}
	msg.nack = func(ctx context.Context, reason string) error {
		_, err := c.pb.Nack(ctx, &pb.NackRequest{Id: id, Error: reason})
		if err != nil {
			return fmt.Errorf("nack message %s: %w", id, err)
		}
		return nil
	}
	return msg
}
