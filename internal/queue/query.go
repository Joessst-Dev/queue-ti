package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
)

// TopicStat holds the message count for a single (topic, status) pair.
type TopicStat struct {
	Topic  string
	Status string
	Count  int
}

// List returns a paginated page of messages, optionally filtered by topic.
// The PagedResult carries both the page and the total matching count.
func (s *Service) List(ctx context.Context, topic string, limit, offset int) (PagedResult[Message], error) {
	var countQuery, selectQuery string
	var args []any

	if topic != "" {
		countQuery = `SELECT COUNT(*) FROM messages WHERE topic = $1`
		selectQuery = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error,
		                      expires_at, created_at, COALESCE(original_topic, ''), dlq_moved_at, key
		               FROM messages WHERE topic = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []any{topic, limit, offset}
	} else {
		countQuery = `SELECT COUNT(*) FROM messages`
		selectQuery = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error,
		                      expires_at, created_at, COALESCE(original_topic, ''), dlq_moved_at, key
		               FROM messages ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	var total int
	var countArgs []any
	if topic != "" {
		countArgs = []any{topic}
	}
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return PagedResult[Message]{}, fmt.Errorf("list count: %w", err)
	}

	rows, err := s.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return PagedResult[Message]{}, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var metaJSON []byte
		var lastError *string
		if err := rows.Scan(
			&msg.ID, &msg.Topic, &msg.Payload, &metaJSON, &msg.Status,
			&msg.RetryCount, &msg.MaxRetries, &lastError,
			&msg.ExpiresAt, &msg.CreatedAt,
			&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
		); err != nil {
			return PagedResult[Message]{}, fmt.Errorf("list scan: %w", err)
		}
		if err := hydrateMessage(&msg, metaJSON, lastError); err != nil {
			return PagedResult[Message]{}, err
		}
		messages = append(messages, msg)
	}

	slog.Debug("list messages", "topic", topic, "limit", limit, "offset", offset, "total", total, "returned", len(messages))
	return PagedResult[Message]{Items: messages, Total: total}, nil
}

// Stats returns the message count grouped by topic and status, ordered by
// topic ASC, status ASC. It returns an empty (non-nil) slice when no messages
// exist.
func (s *Service) Stats(ctx context.Context) ([]TopicStat, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT topic, status, COUNT(*) FROM messages GROUP BY topic, status ORDER BY topic, status`)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	defer rows.Close()

	stats := []TopicStat{}
	for rows.Next() {
		var ts TopicStat
		if err := rows.Scan(&ts.Topic, &ts.Status, &ts.Count); err != nil {
			return nil, fmt.Errorf("stats scan: %w", err)
		}
		stats = append(stats, ts)
	}
	return stats, nil
}

// TopicForMessage returns the topic of the message with the given ID.
// It returns ErrNotFound if no such message exists.
func (s *Service) TopicForMessage(ctx context.Context, id string) (string, error) {
	var topic string
	err := s.pool.QueryRow(ctx, `SELECT topic FROM messages WHERE id = $1`, id).Scan(&topic)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return topic, err
}
