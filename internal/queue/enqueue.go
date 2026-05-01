package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// resolveExpiresAt computes the effective expiry time from a pre-loaded topic
// config, applying the global messageTTL as the fallback. cfg may be nil.
func (s *Service) resolveExpiresAt(cfg *TopicConfig) *time.Time {
	var expiresAt *time.Time
	if s.messageTTL > 0 {
		t := time.Now().Add(s.messageTTL)
		expiresAt = &t
	}
	if cfg != nil && cfg.MessageTTLSeconds != nil {
		if *cfg.MessageTTLSeconds == 0 {
			expiresAt = nil // explicitly no TTL
		} else {
			t := time.Now().Add(time.Duration(*cfg.MessageTTLSeconds) * time.Second)
			expiresAt = &t
		}
	}
	return expiresAt
}

// resolveEnqueueParams merges global service defaults with per-topic overrides
// from topic_config. It returns the effective maxRetries, expiresAt, and
// maxDepth (0 = unlimited) to use when inserting a new message.
func (s *Service) resolveEnqueueParams(ctx context.Context, topic string) (maxRetries int, expiresAt *time.Time, maxDepth int, err error) {
	maxRetries = s.maxRetries
	maxDepth = 0 // 0 = unlimited

	cfg, err := s.GetTopicConfig(ctx, topic)
	if err != nil {
		return 0, nil, 0, err
	}

	expiresAt = s.resolveExpiresAt(cfg)

	if cfg == nil {
		return maxRetries, expiresAt, maxDepth, nil
	}
	if cfg.MaxRetries != nil {
		maxRetries = *cfg.MaxRetries
	}
	if cfg.MaxDepth != nil {
		maxDepth = *cfg.MaxDepth
	}
	return maxRetries, expiresAt, maxDepth, nil
}

// Enqueue inserts a new message onto the given topic. Topics ending in ".dlq"
// are reserved for the dead-letter mechanism and are rejected with ErrReservedTopic.
// Per-topic configuration (max_retries, TTL, max_depth) overrides global defaults
// when a topic_config row exists for the topic.
//
// When key is non-nil and a pending message with the same (topic, key) already
// exists, the INSERT is converted to an upsert: payload, metadata, max_retries,
// and updated_at are overwritten and the existing message ID is returned.
// When key is nil the ON CONFLICT clause never fires (NULLs do not match the
// partial unique index), so regular INSERT semantics are preserved.
func (s *Service) Enqueue(ctx context.Context, topic string, payload []byte, metadata map[string]string, key *string) (string, error) {
	if strings.HasSuffix(topic, ".dlq") {
		return "", fmt.Errorf("enqueue: %w", ErrReservedTopic)
	}

	if s.requireTopicRegistration {
		cfg, err := s.GetTopicConfig(ctx, topic)
		if err != nil {
			return "", fmt.Errorf("enqueue topic check: %w", err)
		}
		if cfg == nil {
			return "", fmt.Errorf("enqueue: %w", ErrTopicNotRegistered)
		}
	}

	if err := s.validatePayload(ctx, topic, payload); err != nil {
		return "", err
	}

	maxRetries, expiresAt, maxDepth, err := s.resolveEnqueueParams(ctx, topic)
	if err != nil {
		return "", fmt.Errorf("enqueue resolve params: %w", err)
	}

	// Depth guard — soft check, race is acceptable for a circuit-breaker use case.
	if maxDepth > 0 {
		var depth int
		err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM messages WHERE topic = $1 AND status IN ('pending', 'processing')`,
			topic,
		).Scan(&depth)
		if err != nil {
			return "", fmt.Errorf("enqueue depth check: %w", err)
		}
		if depth >= maxDepth {
			return "", fmt.Errorf("enqueue: %w", ErrQueueFull)
		}
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO messages (topic, payload, metadata, max_retries, expires_at, key)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (topic, key) WHERE key IS NOT NULL AND status = 'pending'
		 DO UPDATE SET
		     payload     = EXCLUDED.payload,
		     metadata    = EXCLUDED.metadata,
		     max_retries = EXCLUDED.max_retries,
		     updated_at  = now()
		 RETURNING id`,
		topic, payload, metaJSON, maxRetries, expiresAt, key,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}

	s.recorder.RecordEnqueue(topic)
	slog.Debug("message enqueued", "id", id, "topic", topic)
	return id, nil
}
