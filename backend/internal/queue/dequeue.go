package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// applyThroughputLimit checks the per-topic throughput limit and consumes want
// tokens from the bucket. Returns the number of tokens actually granted (≤ want).
// Returns 0 (with nil error) when the bucket is exhausted — the caller must
// commit the transaction and return early. Returns want when no limit is set.
func (s *Service) applyThroughputLimit(ctx context.Context, tx pgx.Tx, topic string, want int) (int, error) {
	var limit *int
	err := tx.QueryRow(ctx, `SELECT throughput_limit FROM topic_config WHERE topic = $1`, topic).Scan(&limit)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("throughput limit fetch: %w", err)
	}
	if limit == nil || *limit == 0 {
		return want, nil
	}
	return s.consumeTokens(ctx, tx, topic, *limit, want)
}

const deliveryJoinQuery = `
	SELECT m.id, m.topic, m.payload, m.metadata, md.retry_count, md.max_retries,
	       md.last_error, m.expires_at, m.created_at,
	       COALESCE(m.original_topic, ''), m.dlq_moved_at, m.key
	FROM   messages m
	JOIN   message_deliveries md ON md.message_id = m.id
	WHERE  m.id = $1 AND md.consumer_group = $2
`

// scanMessageWithDelivery scans a single row from the delivery JOIN query into
// a Message, hydrating its metadata and last_error fields.
func scanMessageWithDelivery(row pgx.Row) (*Message, error) {
	var msg Message
	var metaJSON []byte
	var lastError *string
	err := row.Scan(
		&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
		&msg.RetryCount, &msg.MaxRetries, &lastError,
		&msg.ExpiresAt, &msg.CreatedAt,
		&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
	)
	if err != nil {
		return nil, err
	}
	if err := hydrateMessage(&msg, metaJSON, lastError); err != nil {
		return nil, err
	}
	return &msg, nil
}

// scanMessageWithDeliveryRows scans the next row from a multi-row delivery JOIN
// result set. The caller is responsible for iterating rows and checking rows.Err().
func scanMessageWithDeliveryRows(rows pgx.Rows) (*Message, error) {
	var msg Message
	var metaJSON []byte
	var lastError *string
	if err := rows.Scan(
		&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
		&msg.RetryCount, &msg.MaxRetries, &lastError,
		&msg.ExpiresAt, &msg.CreatedAt,
		&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
	); err != nil {
		return nil, err
	}
	if err := hydrateMessage(&msg, metaJSON, lastError); err != nil {
		return nil, err
	}
	return &msg, nil
}

// resolveVisibilityTimeout returns the effective visibility timeout to use for
// a dequeue operation. When requested is positive it takes precedence; otherwise
// the service default is used.
func (s *Service) resolveVisibilityTimeout(requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return s.visibilityTimeout
}

func (s *Service) Dequeue(ctx context.Context, topic string, visibilityTimeout time.Duration) (*Message, error) {
	vt := s.resolveVisibilityTimeout(visibilityTimeout)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Check for a per-topic throughput limit.
	allowed, err := s.applyThroughputLimit(ctx, tx, topic, 1)
	if err != nil {
		return nil, err
	}
	if allowed == 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue commit (throttled): %w", commitErr)
		}
		return nil, ErrNoMessage
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

// DequeueForGroup atomically claims the next pending message for the given
// consumer group. It operates on message_deliveries rather than messages so
// each group has its own independent delivery cursor. Returns ErrNoMessage when
// no eligible delivery exists.
func (s *Service) DequeueForGroup(ctx context.Context, topic, group string, visibilityTimeout time.Duration) (*Message, error) {
	vt := s.resolveVisibilityTimeout(visibilityTimeout)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue group begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	allowed, err := s.applyThroughputLimit(ctx, tx, topic, 1)
	if err != nil {
		return nil, err
	}
	if allowed == 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue group commit (throttled): %w", commitErr)
		}
		return nil, ErrNoMessage
	}

	var messageID string
	err = tx.QueryRow(ctx, `
		UPDATE message_deliveries
		SET    status             = 'processing',
		       visibility_timeout = now() + $1::interval,
		       updated_at         = now()
		WHERE  (message_id, consumer_group) = (
		    SELECT md.message_id, md.consumer_group
		    FROM   message_deliveries md
		    JOIN   messages m ON m.id = md.message_id
		    WHERE  md.consumer_group = $2
		      AND  m.topic            = $3
		      AND  md.status          = 'pending'
		      AND  (md.visibility_timeout IS NULL OR md.visibility_timeout < now())
		      AND  (m.expires_at IS NULL OR m.expires_at > now())
		      AND  (md.max_retries = 0 OR md.retry_count < md.max_retries)
		    ORDER BY m.created_at
		    LIMIT 1
		    FOR UPDATE OF md SKIP LOCKED
		)
		RETURNING message_id
	`,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
		group,
		topic,
	).Scan(&messageID)

	if errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue group commit (empty): %w", commitErr)
		}
		return nil, ErrNoMessage
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue group: %w", err)
	}

	msg, err := scanMessageWithDelivery(tx.QueryRow(ctx, deliveryJoinQuery, messageID, group))
	if err != nil {
		return nil, fmt.Errorf("dequeue group fetch message: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dequeue group commit: %w", err)
	}

	s.recorder.RecordDequeue(msg.Topic)
	slog.Debug("message dequeued for group", "id", msg.ID, "topic", msg.Topic, "group", group, "retry_count", msg.RetryCount)
	return msg, nil
}

// DequeueNForGroup atomically claims up to n pending messages for the given
// consumer group. Returns ErrInvalidBatchSize when n < 1 or n > 1000. Returns
// an empty (non-nil) slice when no messages are available.
func (s *Service) DequeueNForGroup(ctx context.Context, topic, group string, n int, visibilityTimeout time.Duration) ([]*Message, error) {
	if n < 1 || n > 1000 {
		return nil, ErrInvalidBatchSize
	}

	vt := s.resolveVisibilityTimeout(visibilityTimeout)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue group batch begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	allowed, err := s.applyThroughputLimit(ctx, tx, topic, n)
	if err != nil {
		return nil, err
	}
	if allowed == 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue group batch commit (throttled): %w", commitErr)
		}
		return []*Message{}, nil
	}

	rows, err := tx.Query(ctx, `
		UPDATE message_deliveries
		SET    status             = 'processing',
		       visibility_timeout = now() + $4::interval,
		       updated_at         = now()
		WHERE  (message_id, consumer_group) IN (
		    SELECT md.message_id, md.consumer_group
		    FROM   message_deliveries md
		    JOIN   messages m ON m.id = md.message_id
		    WHERE  md.consumer_group = $1
		      AND  m.topic            = $2
		      AND  md.status          = 'pending'
		      AND  (md.visibility_timeout IS NULL OR md.visibility_timeout < now())
		      AND  (m.expires_at IS NULL OR m.expires_at > now())
		      AND  (md.max_retries = 0 OR md.retry_count < md.max_retries)
		    ORDER BY m.created_at
		    LIMIT $3
		    FOR UPDATE OF md SKIP LOCKED
		)
		RETURNING message_id
	`,
		group,
		topic,
		allowed,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue group batch: %w", err)
	}

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("dequeue group batch scan id: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue group batch rows: %w", err)
	}

	if len(ids) == 0 {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, fmt.Errorf("dequeue group batch commit (empty): %w", commitErr)
		}
		return []*Message{}, nil
	}

	msgRows, err := tx.Query(ctx, `
		SELECT m.id, m.topic, m.payload, m.metadata, md.retry_count, md.max_retries,
		       md.last_error, m.expires_at, m.created_at,
		       COALESCE(m.original_topic, ''), m.dlq_moved_at, m.key
		FROM   messages m
		JOIN   message_deliveries md ON md.message_id = m.id
		WHERE  m.id = ANY($1) AND md.consumer_group = $2
		ORDER  BY m.created_at
	`, ids, group)
	if err != nil {
		return nil, fmt.Errorf("dequeue group batch fetch: %w", err)
	}
	defer msgRows.Close()

	messages := make([]*Message, 0, len(ids))
	for msgRows.Next() {
		msg, err := scanMessageWithDeliveryRows(msgRows)
		if err != nil {
			return nil, fmt.Errorf("dequeue group batch scan message: %w", err)
		}
		s.recorder.RecordDequeue(msg.Topic)
		slog.Debug("message dequeued for group (batch)", "id", msg.ID, "topic", msg.Topic, "group", group, "retry_count", msg.RetryCount)
		messages = append(messages, msg)
	}
	if err := msgRows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue group batch message rows: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("dequeue group batch commit: %w", err)
	}

	return messages, nil
}

// DequeueN atomically claims up to n pending messages from the given topic,
// setting their status to 'processing' and their visibility_timeout to now() +
// visibilityTimeout. It returns an empty (non-nil) slice when no messages are
// available and never blocks. Returns ErrInvalidBatchSize when n < 1 or n > 1000.
func (s *Service) DequeueN(ctx context.Context, topic string, n int, visibilityTimeout time.Duration) ([]*Message, error) {
	if n < 1 || n > 1000 {
		return nil, ErrInvalidBatchSize
	}

	vt := s.resolveVisibilityTimeout(visibilityTimeout)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("dequeue batch begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	allowed, err := s.applyThroughputLimit(ctx, tx, topic, n)
	if err != nil {
		return nil, err
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
