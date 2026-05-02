package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const throttleLockNamespace int64 = 8_000_000_000

// consumeTokens enforces a token-bucket rate limit for topic inside tx.
// limit is the configured messages/second ceiling (must be > 0).
// requested is the number of messages the caller wants to dequeue.
// Returns the number of messages the caller is allowed to dequeue (0…requested).
// Returns an error only on DB failure.
func (s *Service) consumeTokens(ctx context.Context, tx pgx.Tx, topic string, limit, requested int) (int, error) {
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock($1::bigint + hashtext($2)::int::bigint)`,
		throttleLockNamespace, topic,
	); err != nil {
		return 0, fmt.Errorf("throttle advisory lock: %w", err)
	}

	// Upsert token-bucket row, refilling based on elapsed time, capped at limit.
	var available float64
	if err := tx.QueryRow(ctx, `
		INSERT INTO topic_throughput (topic, tokens, last_refill)
		VALUES ($1, $2::float, now())
		ON CONFLICT (topic) DO UPDATE
		SET tokens      = LEAST(
		                      $2::float,
		                      topic_throughput.tokens
		                      + $2::float * EXTRACT(EPOCH FROM (now() - topic_throughput.last_refill))
		                  ),
		    last_refill = now()
		RETURNING tokens
	`, topic, limit).Scan(&available); err != nil {
		return 0, fmt.Errorf("throttle refill: %w", err)
	}

	allowed := min(requested, int(available))
	if allowed <= 0 {
		return 0, nil
	}

	if _, err := tx.Exec(ctx,
		`UPDATE topic_throughput SET tokens = tokens - $2 WHERE topic = $1`,
		topic, float64(allowed),
	); err != nil {
		return 0, fmt.Errorf("throttle deduct: %w", err)
	}

	return allowed, nil
}
