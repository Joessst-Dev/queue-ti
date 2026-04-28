package queue

import (
	"context"
	"fmt"
	"log/slog"
)

// PurgeTopic permanently deletes messages belonging to the given topic whose
// status is one of the provided statuses. It returns the number of deleted
// rows. Validation of status values is the caller's responsibility.
func (s *Service) PurgeTopic(ctx context.Context, topic string, statuses []string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM messages WHERE topic = $1 AND status = ANY($2)`,
		topic, statuses,
	)
	if err != nil {
		return 0, fmt.Errorf("purge topic: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		slog.Info("purged messages from topic", "topic", topic, "statuses", statuses, "count", n)
	}
	return n, nil
}

// PurgeByKey hard-deletes all messages for the given topic with the given key,
// regardless of status. Returns the number of rows deleted.
func (s *Service) PurgeByKey(ctx context.Context, topic, key string) (int64, error) {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM messages WHERE topic = $1 AND key = $2`,
		topic, key,
	)
	if err != nil {
		return 0, fmt.Errorf("purge by key: %w", err)
	}
	return tag.RowsAffected(), nil
}
