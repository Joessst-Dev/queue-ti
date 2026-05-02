package broadcast

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PG struct {
	pool *pgxpool.Pool
}

func NewPG(pool *pgxpool.Pool) *PG {
	return &PG{pool: pool}
}

func (p *PG) Publish(ctx context.Context, channel, payload string) error {
	_, err := p.pool.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

func (p *PG) Subscribe(ctx context.Context, channel string) (<-chan string, context.CancelFunc) {
	subCtx, cancel := context.WithCancel(ctx)
	ch := make(chan string, 16)

	go func() {
		defer close(ch)

		conn, err := p.pool.Acquire(subCtx)
		if err != nil {
			if subCtx.Err() == nil {
				slog.Error("broadcast: failed to acquire connection", "channel", channel, "error", err)
			}
			return
		}
		defer conn.Release()

		if _, err := conn.Exec(subCtx, fmt.Sprintf("LISTEN %s", channel)); err != nil {
			if subCtx.Err() == nil {
				slog.Error("broadcast: LISTEN failed", "channel", channel, "error", err)
			}
			return
		}

		for {
			notification, err := conn.Conn().WaitForNotification(subCtx)
			if err != nil {
				if subCtx.Err() == nil {
					slog.Error("broadcast: wait for notification failed", "channel", channel, "error", err)
				}
				return
			}
			select {
			case ch <- notification.Payload:
			case <-subCtx.Done():
				return
			}
		}
	}()

	return ch, cancel
}

func (p *PG) Close() error {
	return nil
}
