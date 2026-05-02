package broadcast

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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
	safeChannel := pgx.Identifier{channel}.Sanitize()

	go func() {
		defer close(ch)
		for subCtx.Err() == nil {
			p.runListenLoop(subCtx, safeChannel, ch)
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

func (p *PG) runListenLoop(ctx context.Context, safeChannel string, ch chan<- string) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("broadcast: failed to acquire connection", "channel", safeChannel, "error", err)
		}
		return
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+safeChannel); err != nil {
		if ctx.Err() == nil {
			slog.Error("broadcast: LISTEN failed", "channel", safeChannel, "error", err)
		}
		return
	}

	for {
		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("broadcast: notification wait failed — will reconnect", "channel", safeChannel, "error", err)
			}
			return
		}
		select {
		case ch <- notification.Payload:
		case <-ctx.Done():
			return
		}
	}
}

func (p *PG) Close() error {
	return nil
}
