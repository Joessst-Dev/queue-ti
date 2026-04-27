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
}

// WithMetadata attaches key-value metadata to the published message.
func WithMetadata(m map[string]string) PublishOption {
	return func(cfg *publishConfig) {
		cfg.metadata = m
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
	})
	if err != nil {
		return "", fmt.Errorf("publish to topic %q: %w", topic, err)
	}
	return resp.Id, nil
}
