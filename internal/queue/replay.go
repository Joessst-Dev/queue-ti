package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// ArchivedMessage is a row from message_log returned by ListMessageLog.
type ArchivedMessage struct {
	ID            string
	Topic         string
	Key           *string
	Payload       []byte
	Metadata      map[string]string
	RetryCount    int
	CreatedAt     time.Time
	AckedAt       time.Time
	OriginalTopic string
}

// ReplayTopic re-enqueues as pending copies of all messages archived for the
// given topic. fromTime sets the lower bound on acked_at; a zero value means
// "from the beginning of the window" (or all messages when no window is set).
// Returns ErrTopicNotReplayable when the topic has replayable = false.
// Returns ErrReplayWindowTooOld when fromTime predates the configured window.
func (s *Service) ReplayTopic(ctx context.Context, topic string, fromTime time.Time) (int64, error) {
	cfg, err := s.GetTopicConfig(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("replay: get config: %w", err)
	}
	if cfg == nil || !cfg.Replayable {
		return 0, fmt.Errorf("replay: %w", ErrTopicNotReplayable)
	}

	var windowStart time.Time
	if cfg.ReplayWindowSeconds != nil && *cfg.ReplayWindowSeconds > 0 {
		windowStart = time.Now().Add(-time.Duration(*cfg.ReplayWindowSeconds) * time.Second)
		if !fromTime.IsZero() && fromTime.Before(windowStart) {
			return 0, fmt.Errorf("replay: %w", ErrReplayWindowTooOld)
		}
	}

	effectiveFrom := fromTime
	if effectiveFrom.IsZero() {
		effectiveFrom = windowStart
	}

	_, expiresAt, _, err := s.resolveEnqueueParams(ctx, topic)
	if err != nil {
		return 0, fmt.Errorf("replay resolve params: %w", err)
	}

	var tag pgconn.CommandTag
	if effectiveFrom.IsZero() {
		tag, err = s.pool.Exec(ctx, `
			INSERT INTO messages
			    (id, topic, key, payload, metadata, retry_count, max_retries,
			     last_error, original_topic, created_at, status, expires_at)
			SELECT gen_random_uuid(), topic, key, payload, metadata,
			       0, max_retries, NULL, original_topic, now(), 'pending', $2
			FROM message_log
			WHERE topic = $1
			ON CONFLICT DO NOTHING
		`, topic, expiresAt)
	} else {
		tag, err = s.pool.Exec(ctx, `
			INSERT INTO messages
			    (id, topic, key, payload, metadata, retry_count, max_retries,
			     last_error, original_topic, created_at, status, expires_at)
			SELECT gen_random_uuid(), topic, key, payload, metadata,
			       0, max_retries, NULL, original_topic, now(), 'pending', $3
			FROM message_log
			WHERE topic = $1
			  AND acked_at >= $2
			ON CONFLICT DO NOTHING
		`, topic, effectiveFrom, expiresAt)
	}
	if err != nil {
		return 0, fmt.Errorf("replay insert: %w", err)
	}

	n := tag.RowsAffected()
	if n > 0 {
		s.recorder.RecordReplay(topic, n)
		slog.Info("topic replayed", "topic", topic, "from", effectiveFrom, "count", n)
	}
	return n, nil
}

// ListMessageLog returns paginated archived messages for topic ordered by
// acked_at DESC. It returns the page, total count, and any error.
func (s *Service) ListMessageLog(ctx context.Context, topic string, limit, offset int) ([]ArchivedMessage, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM message_log WHERE topic = $1`, topic,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list message log count: %w", err)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, topic, key, payload, metadata, retry_count,
		       created_at, acked_at, COALESCE(original_topic, '')
		FROM message_log
		WHERE topic = $1
		ORDER BY acked_at DESC
		LIMIT $2 OFFSET $3
	`, topic, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list message log: %w", err)
	}
	defer rows.Close()

	var msgs []ArchivedMessage
	for rows.Next() {
		var msg ArchivedMessage
		var metaJSON []byte
		if err := rows.Scan(
			&msg.ID, &msg.Topic, &msg.Key, &msg.Payload, &metaJSON,
			&msg.RetryCount, &msg.CreatedAt, &msg.AckedAt, &msg.OriginalTopic,
		); err != nil {
			return nil, 0, fmt.Errorf("list message log scan: %w", err)
		}
		if metaJSON != nil {
			if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
				return nil, 0, fmt.Errorf("list message log unmarshal metadata: %w", err)
			}
		}
		msgs = append(msgs, msg)
	}
	return msgs, total, nil
}

// TrimMessageLog permanently deletes message_log rows for topic where
// acked_at < before. Returns the number of rows deleted.
func (s *Service) TrimMessageLog(ctx context.Context, topic string, before time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM message_log WHERE topic = $1 AND acked_at < $2`, topic, before,
	)
	if err != nil {
		return 0, fmt.Errorf("trim message log: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		slog.Info("message log trimmed", "topic", topic, "before", before, "count", n)
	}
	return n, nil
}
