package queue

import (
	"context"
	"fmt"
	"log/slog"
)

// RegisterConsumerGroup registers a new consumer group for the given topic and
// backfills delivery rows for all currently pending messages. Registering the
// same group twice returns ErrConsumerGroupExists. When requireTopicRegistration
// is enabled, the topic must have an existing topic_config row.
func (s *Service) RegisterConsumerGroup(ctx context.Context, topic, group string) error {
	if s.requireTopicRegistration {
		cfg, err := s.GetTopicConfig(ctx, topic)
		if err != nil {
			return fmt.Errorf("register consumer group check topic: %w", err)
		}
		if cfg == nil {
			return ErrTopicNotRegistered
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("register consumer group begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `
		INSERT INTO consumer_groups (topic, consumer_group)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, topic, group)
	if err != nil {
		return fmt.Errorf("register consumer group insert: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConsumerGroupExists
	}

	// Backfill delivery rows for all pending messages that predate this
	// registration. The trigger handles future inserts automatically.
	_, err = tx.Exec(ctx, `
		INSERT INTO message_deliveries (message_id, consumer_group, max_retries)
		SELECT m.id, $1, m.max_retries
		FROM   messages m
		WHERE  m.topic  = $2
		  AND  m.status = 'pending'
		ON CONFLICT DO NOTHING
	`, group, topic)
	if err != nil {
		return fmt.Errorf("register consumer group backfill: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("register consumer group commit: %w", err)
	}

	slog.Info("consumer group registered", "topic", topic, "group", group)
	return nil
}

// UnregisterConsumerGroup removes a consumer group from the registry and
// deletes all its delivery rows for messages on that topic.
// Returns ErrConsumerGroupNotFound when the group does not exist.
func (s *Service) UnregisterConsumerGroup(ctx context.Context, topic, group string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("unregister consumer group begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, `
		DELETE FROM consumer_groups
		WHERE topic = $1 AND consumer_group = $2
	`, topic, group)
	if err != nil {
		return fmt.Errorf("unregister consumer group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConsumerGroupNotFound
	}

	// Remove delivery rows for this group scoped to the topic's messages.
	// ON DELETE CASCADE would only fire if the consumer_groups FK existed on
	// message_deliveries; since it does not, we clean up explicitly.
	_, err = tx.Exec(ctx, `
		DELETE FROM message_deliveries md
		USING messages m
		WHERE md.message_id = m.id
		  AND m.topic = $1
		  AND md.consumer_group = $2
	`, topic, group)
	if err != nil {
		return fmt.Errorf("unregister consumer group delete deliveries: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("unregister consumer group commit: %w", err)
	}

	slog.Info("consumer group unregistered", "topic", topic, "group", group)
	return nil
}

// ListConsumerGroups returns all registered consumer group names for the given
// topic ordered by registration time (oldest first).
func (s *Service) ListConsumerGroups(ctx context.Context, topic string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT consumer_group
		FROM   consumer_groups
		WHERE  topic = $1
		ORDER  BY created_at
	`, topic)
	if err != nil {
		return nil, fmt.Errorf("list consumer groups: %w", err)
	}
	defer rows.Close()

	var groups []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, fmt.Errorf("list consumer groups scan: %w", err)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list consumer groups rows: %w", err)
	}

	// Prefer an explicit empty slice over nil for consistent API behaviour.
	if groups == nil {
		groups = []string{}
	}

	return groups, nil
}

