package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// TopicConfig holds per-topic overrides. A nil pointer field means "use global default".
type TopicConfig struct {
	Topic               string
	MaxRetries          *int
	MessageTTLSeconds   *int // 0 = no TTL (never expires), nil = use global default
	MaxDepth            *int // 0 or nil = unlimited
	Replayable          bool
	ReplayWindowSeconds *int // nil = no window (always replayable when Replayable is true)
}

func (s *Service) GetTopicConfig(ctx context.Context, topic string) (*TopicConfig, error) {
	var cfg TopicConfig
	err := s.pool.QueryRow(ctx,
		`SELECT topic, max_retries, message_ttl_seconds, max_depth, replayable, replay_window_seconds
		 FROM topic_config WHERE topic = $1`,
		topic,
	).Scan(&cfg.Topic, &cfg.MaxRetries, &cfg.MessageTTLSeconds, &cfg.MaxDepth, &cfg.Replayable, &cfg.ReplayWindowSeconds)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get topic config: %w", err)
	}
	return &cfg, nil
}

func (s *Service) UpsertTopicConfig(ctx context.Context, cfg TopicConfig) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO topic_config (topic, max_retries, message_ttl_seconds, max_depth, replayable, replay_window_seconds, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (topic) DO UPDATE
		SET max_retries          = EXCLUDED.max_retries,
		    message_ttl_seconds  = EXCLUDED.message_ttl_seconds,
		    max_depth            = EXCLUDED.max_depth,
		    replayable           = EXCLUDED.replayable,
		    replay_window_seconds = EXCLUDED.replay_window_seconds,
		    updated_at           = now()
	`, cfg.Topic, cfg.MaxRetries, cfg.MessageTTLSeconds, cfg.MaxDepth, cfg.Replayable, cfg.ReplayWindowSeconds)
	if err != nil {
		return fmt.Errorf("upsert topic config: %w", err)
	}
	return nil
}

func (s *Service) DeleteTopicConfig(ctx context.Context, topic string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM topic_config WHERE topic = $1`, topic)
	if err != nil {
		return fmt.Errorf("delete topic config: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("delete topic config: %w", ErrNotFound)
	}
	return nil
}

func (s *Service) ListTopicConfigs(ctx context.Context) ([]TopicConfig, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT topic, max_retries, message_ttl_seconds, max_depth, replayable, replay_window_seconds
		 FROM topic_config ORDER BY topic ASC`)
	if err != nil {
		return nil, fmt.Errorf("list topic configs: %w", err)
	}
	defer rows.Close()

	configs := []TopicConfig{}
	for rows.Next() {
		var cfg TopicConfig
		if err := rows.Scan(&cfg.Topic, &cfg.MaxRetries, &cfg.MessageTTLSeconds, &cfg.MaxDepth, &cfg.Replayable, &cfg.ReplayWindowSeconds); err != nil {
			return nil, fmt.Errorf("list topic configs scan: %w", err)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}
