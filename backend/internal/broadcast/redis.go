package broadcast

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisBroadcaster implements Broadcaster using Redis pub/sub.
// It does not own the underlying client — Close is a no-op.
type RedisBroadcaster struct {
	client redis.UniversalClient
}

func NewRedis(client redis.UniversalClient) *RedisBroadcaster {
	return &RedisBroadcaster{client: client}
}

func (r *RedisBroadcaster) Publish(ctx context.Context, channel, payload string) error {
	return r.client.Publish(ctx, channel, payload).Err()
}

func (r *RedisBroadcaster) Subscribe(ctx context.Context, channel string) (<-chan string, context.CancelFunc) {
	subCtx, cancel := context.WithCancel(ctx)
	ch := make(chan string, 16)

	go func() {
		defer close(ch)
		for subCtx.Err() == nil {
			r.runSubscribeLoop(subCtx, channel, ch)
			if subCtx.Err() != nil {
				return
			}
			select {
			case <-time.After(2 * time.Second):
			case <-subCtx.Done():
				return
			}
		}
	}()

	return ch, cancel
}

func (r *RedisBroadcaster) runSubscribeLoop(ctx context.Context, channel string, ch chan<- string) {
	sub := r.client.Subscribe(ctx, channel)
	defer func() {
		if err := sub.Close(); err != nil && ctx.Err() == nil {
			slog.Error("broadcast: redis subscription close failed", "channel", channel, "error", err)
		}
	}()

	msgCh := sub.Channel()
	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				return
			}
			select {
			case ch <- msg.Payload:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *RedisBroadcaster) Close() error {
	return nil
}
