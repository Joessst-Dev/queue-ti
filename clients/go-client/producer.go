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
