package queueti

import (
	"context"
	"fmt"

	pb "github.com/Joessst-Dev/queue-ti/pb"
)

// Producer publishes messages to the queue.
type Producer struct {
	client *Client
}

// PublishOption configures a Publish call.
type PublishOption func(*publishConfig)

type publishConfig struct {
	metadata map[string]string
	key      *string
}

// WithMetadata attaches key-value metadata to the published message.
func WithMetadata(m map[string]string) PublishOption {
	return func(cfg *publishConfig) {
		cfg.metadata = m
	}
}

// WithKey sets a deduplication key on the published message. When a pending
// message with the same (topic, key) pair already exists it is upserted rather
// than creating a new row.
func WithKey(key string) PublishOption {
	return func(cfg *publishConfig) {
		cfg.key = &key
	}
}

// Publish enqueues a message on topic with the given payload.
// Returns the assigned message ID.
func (p *Producer) Publish(ctx context.Context, topic string, payload []byte, opts ...PublishOption) (string, error) {
	cfg := &publishConfig{}
	for _, o := range opts {
		o(cfg)
	}

	resp, err := p.client.pb.Enqueue(ctx, &pb.EnqueueRequest{
		Topic:    topic,
		Payload:  payload,
		Metadata: cfg.metadata,
		Key:      cfg.key,
	})
	if err != nil {
		return "", fmt.Errorf("publish to topic %q: %w", topic, err)
	}
	return resp.Id, nil
}
