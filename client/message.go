package queueti

import (
	"context"
	"time"
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
