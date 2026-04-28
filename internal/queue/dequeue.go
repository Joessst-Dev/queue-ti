package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Dequeue(ctx context.Context, topic string, visibilityTimeout time.Duration) (*Message, error) {
	vt := s.visibilityTimeout
	if visibilityTimeout > 0 {
		vt = visibilityTimeout
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Check for a per-topic throughput limit.
	var limit *int
	err = tx.QueryRow(ctx,
		`SELECT throughput_limit FROM topic_config WHERE topic = $1`, topic,
	).Scan(&limit)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("dequeue fetch limit: %w", err)
	}
	if limit != nil && *limit > 0 {
		allowed, err := s.consumeTokens(ctx, tx, topic, *limit, 1)
		if err != nil {
			return nil, err
		}
		if allowed == 0 {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				return nil, fmt.Errorf("dequeue commit (throttled): %w", commitErr)
			}
			return nil, ErrNoMessage
		}
	}

	query := `
		UPDATE messages
		SET status = 'processing',
			visibility_timeout = now() + $1::interval,
			updated_at = now()
		WHERE id = (
			SELECT id FROM messages
			WHERE topic = $2
			  AND status = 'pending'
			  AND (visibility_timeout IS NULL OR visibility_timeout < now())
			  AND (expires_at IS NULL OR expires_at > now())
			  AND (max_retries = 0 OR retry_count < max_retries)
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, metadata, retry_count, max_retries, last_error, expires_at, created_at,
		          COALESCE(original_topic, ''), dlq_moved_at, key
	`

	var msg Message
	var metaJSON []byte
	var lastError *string
	err = tx.QueryRow(ctx, query,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
		topic,
	).Scan(
		&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
		&msg.RetryCount, &msg.MaxRetries, &lastError,
		&msg.ExpiresAt, &msg.CreatedAt,
		&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue commit (empty): %w", commitErr)
		}
		return nil, ErrNoMessage
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue: %w", err)
	}

	if err := hydrateMessage(&msg, metaJSON, lastError); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dequeue commit: %w", err)
	}

	s.recorder.RecordDequeue(msg.Topic)
	slog.Debug("message dequeued", "id", msg.ID, "topic", msg.Topic, "retry_count", msg.RetryCount)
	return &msg, nil
}

// DequeueN atomically claims up to n pending messages from the given topic,
// setting their status to 'processing' and their visibility_timeout to now() +
// visibilityTimeout. It returns an empty (non-nil) slice when no messages are
// available and never blocks. Returns ErrInvalidBatchSize when n < 1 or n > 1000.
func (s *Service) DequeueN(ctx context.Context, topic string, n int, visibilityTimeout time.Duration) ([]*Message, error) {
	if n < 1 || n > 1000 {
		return nil, ErrInvalidBatchSize
	}

	vt := s.visibilityTimeout
	if visibilityTimeout > 0 {
		vt = visibilityTimeout
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue batch begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	allowed := n
	var limit *int
	err = tx.QueryRow(ctx,
		`SELECT throughput_limit FROM topic_config WHERE topic = $1`, topic,
	).Scan(&limit)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("dequeue batch fetch limit: %w", err)
	}
	if limit != nil && *limit > 0 {
		allowed, err = s.consumeTokens(ctx, tx, topic, *limit, n)
		if err != nil {
			return nil, err
		}
	}

	if allowed == 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue batch commit (throttled): %w", commitErr)
		}
		return []*Message{}, nil
	}

	query := `
		UPDATE messages
		SET    status             = 'processing',
		       visibility_timeout = now() + $3::interval,
		       updated_at         = now()
		WHERE  id IN (
		    SELECT id FROM messages
		    WHERE  topic = $1
		      AND  status = 'pending'
		      AND  (visibility_timeout IS NULL OR visibility_timeout < now())
		      AND  (expires_at IS NULL OR expires_at > now())
		      AND  (max_retries = 0 OR retry_count < max_retries)
		      AND  topic NOT LIKE '%.dlq'
		    ORDER BY created_at
		    LIMIT $2
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, metadata, retry_count, max_retries, last_error, expires_at, created_at,
		          COALESCE(original_topic, ''), dlq_moved_at, key
	`

	rows, err := tx.Query(ctx, query,
		topic,
		allowed,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue batch: %w", err)
	}
	defer rows.Close()

	messages := make([]*Message, 0, allowed)
	for rows.Next() {
		var msg Message
		var metaJSON []byte
		var lastError *string
		if err := rows.Scan(
			&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
			&msg.RetryCount, &msg.MaxRetries, &lastError,
			&msg.ExpiresAt, &msg.CreatedAt,
			&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
		); err != nil {
			return nil, fmt.Errorf("dequeue batch scan: %w", err)
		}
		if err := hydrateMessage(&msg, metaJSON, lastError); err != nil {
			return nil, err
		}
		s.recorder.RecordDequeue(msg.Topic)
		slog.Debug("message dequeued (batch)", "id", msg.ID, "topic", msg.Topic, "retry_count", msg.RetryCount)
		messages = append(messages, &msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue batch rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dequeue batch commit: %w", err)
	}

	return messages, nil
}
