package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Ack(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ack begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var topic string
	var key *string
	var payload, metaJSON []byte
	var retryCount, maxRetries int
	var lastError *string
	var originalTopic *string
	var createdAt time.Time

	err = tx.QueryRow(ctx, `
		SELECT topic, key, payload, metadata, retry_count, max_retries,
		       last_error, original_topic, created_at
		FROM messages WHERE id = $1 AND status = 'processing' FOR UPDATE
	`, id).Scan(&topic, &key, &payload, &metaJSON, &retryCount, &maxRetries,
		&lastError, &originalTopic, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ack: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("ack fetch: %w", err)
	}

	var replayable bool
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(replayable, false) FROM topic_config WHERE topic = $1`, topic,
	).Scan(&replayable)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ack check replayable: %w", err)
	}

	if replayable {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_log
			    (id, topic, key, payload, metadata, retry_count, max_retries,
			     last_error, original_topic, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, id, topic, key, payload, metaJSON, retryCount, maxRetries,
			lastError, originalTopic, createdAt)
		if err != nil {
			return fmt.Errorf("ack archive: %w", err)
		}
	}

	if _, err = tx.Exec(ctx, `DELETE FROM messages WHERE id = $1`, id); err != nil {
		return fmt.Errorf("ack delete: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.recorder.RecordAck(topic)
	slog.Debug("message acked", "id", id, "archived", replayable)
	return nil
}

// Nack marks a processing message as failed or retryable. When retry_count + 1
// reaches dlqThreshold the message is promoted to the dead-letter topic
// (<original-topic>.dlq) within the same transaction: its topic is changed,
// original_topic is recorded, status resets to 'pending', retry_count resets to
// 0, and max_retries is set to 0 so the DLQ copy cannot be auto-retried.
//
// When retry_count + 1 is below dlqThreshold (or dlqThreshold is 0) the
// existing retry logic applies: status becomes 'pending' if retries remain,
// 'failed' when max_retries is exhausted.
func (s *Service) Nack(ctx context.Context, id string, processingError string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("nack begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var retryCount, maxRetries int
	var topic string
	var currentStatus string
	err = tx.QueryRow(ctx,
		`SELECT retry_count, max_retries, topic, status FROM messages WHERE id = $1 FOR UPDATE`,
		id,
	).Scan(&retryCount, &maxRetries, &topic, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("nack: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("nack fetch: %w", err)
	}
	if currentStatus != "processing" {
		return fmt.Errorf("nack: %w", ErrNotProcessing)
	}

	nextRetryCount := retryCount + 1

	// Promote to DLQ when the threshold is configured and reached.
	if s.dlqThreshold > 0 && nextRetryCount >= s.dlqThreshold {
		dlqTopic := topic + ".dlq"
		_, err = tx.Exec(ctx, `
			UPDATE messages
			SET topic          = $2,
			    original_topic = $3,
			    status         = 'pending',
			    retry_count    = 0,
			    max_retries    = 0,
			    last_error     = $4,
			    dlq_moved_at   = now(),
			    visibility_timeout = NULL,
			    updated_at     = now()
			WHERE id = $1
		`, id, dlqTopic, topic, processingError)
		if err != nil {
			return fmt.Errorf("nack dlq promotion: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		s.recorder.RecordNack(topic, "dlq")
		slog.Warn("message promoted to DLQ",
			"id", id,
			"original_topic", topic,
			"dlq_topic", dlqTopic,
			"retry_count", nextRetryCount,
			"error", processingError,
		)
		return nil
	}

	// Standard retry / fail path.
	newStatus := "pending"
	if nextRetryCount >= maxRetries {
		newStatus = "failed"
	}

	_, err = tx.Exec(ctx, `
		UPDATE messages
		SET retry_count        = $2,
		    last_error         = $3,
		    visibility_timeout = NULL,
		    updated_at         = now(),
		    status             = $4
		WHERE id = $1
	`, id, nextRetryCount, processingError, newStatus)
	if err != nil {
		return fmt.Errorf("nack update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if newStatus == "failed" {
		s.recorder.RecordNack(topic, "failed")
		slog.Info("message failed: retries exhausted",
			"id", id, "topic", topic,
			"retry_count", nextRetryCount, "max_retries", maxRetries,
		)
	} else {
		s.recorder.RecordNack(topic, "retry")
		slog.Debug("message nacked, will retry",
			"id", id, "topic", topic,
			"retry_count", nextRetryCount, "max_retries", maxRetries,
		)
	}
	return nil
}

// Requeue moves a dead-letter message back to its original topic so it can be
// processed again. It returns ErrNotDLQ when the message has no original_topic
// set (i.e. it was never promoted to a DLQ).
func (s *Service) Requeue(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("requeue begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var originalTopic *string
	err = tx.QueryRow(ctx,
		`SELECT original_topic FROM messages WHERE id = $1 FOR UPDATE`, id,
	).Scan(&originalTopic)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("requeue: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("requeue fetch: %w", err)
	}
	if originalTopic == nil || *originalTopic == "" {
		return fmt.Errorf("requeue: %w", ErrNotDLQ)
	}

	_, err = tx.Exec(ctx, `
		UPDATE messages
		SET topic          = $2,
		    original_topic = NULL,
		    dlq_moved_at   = NULL,
		    status         = 'pending',
		    retry_count    = 0,
		    max_retries    = $3,
		    visibility_timeout = NULL,
		    updated_at     = now()
		WHERE id = $1
	`, id, *originalTopic, s.dlqThreshold)
	if err != nil {
		return fmt.Errorf("requeue update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.recorder.RecordRequeue(*originalTopic)
	slog.Info("message requeued from DLQ", "id", id, "original_topic", *originalTopic)
	return nil
}
