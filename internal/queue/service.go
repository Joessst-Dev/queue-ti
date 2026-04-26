package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoMessage    = errors.New("no message available")
	ErrNotFound     = errors.New("message not found")
	ErrNotProcessing = errors.New("message is not in processing state")
)

type Message struct {
	ID         string
	Topic      string
	Payload    []byte
	Metadata   map[string]string
	Status     string
	RetryCount int
	MaxRetries int
	LastError  string
	ExpiresAt  *time.Time
	CreatedAt  time.Time
}

type Service struct {
	pool              *pgxpool.Pool
	visibilityTimeout time.Duration
	maxRetries        int
	messageTTL        time.Duration
}

func NewService(pool *pgxpool.Pool, visibilityTimeout time.Duration, maxRetries int, messageTTL time.Duration) *Service {
	return &Service{
		pool:              pool,
		visibilityTimeout: visibilityTimeout,
		maxRetries:        maxRetries,
		messageTTL:        messageTTL,
	}
}

func (s *Service) Enqueue(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	var expiresAt *time.Time
	if s.messageTTL > 0 {
		t := time.Now().Add(s.messageTTL)
		expiresAt = &t
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO messages (topic, payload, metadata, max_retries, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		topic, payload, metaJSON, s.maxRetries, expiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}

	return id, nil
}

func (s *Service) Dequeue(ctx context.Context, topic string) (*Message, error) {
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
			  AND retry_count < max_retries
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, metadata, retry_count, max_retries, last_error, expires_at, created_at
	`

	var msg Message
	var metaJSON []byte
	var lastError *string
	err := s.pool.QueryRow(ctx, query,
		fmt.Sprintf("%d seconds", int(s.visibilityTimeout.Seconds())),
		topic,
	).Scan(&msg.ID, &msg.Topic, &msg.Payload, &metaJSON, &msg.RetryCount, &msg.MaxRetries, &lastError, &msg.ExpiresAt, &msg.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMessage
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue: %w", err)
	}

	if lastError != nil {
		msg.LastError = *lastError
	}

	if metaJSON != nil {
		if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &msg, nil
}

func (s *Service) Ack(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx,
		`DELETE FROM messages WHERE id = $1 AND status = 'processing'`, id)
	if err != nil {
		return fmt.Errorf("ack: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("ack: message not found or not in processing state")
	}
	return nil
}

// Nack marks a processing message as failed or retryable. If the message has
// exhausted its retry allowance (retry_count + 1 >= max_retries) its status
// becomes 'failed'; otherwise it is reset to 'pending' so it can be dequeued
// again. In both cases retry_count is incremented and last_error is recorded.
func (s *Service) Nack(ctx context.Context, id string, processingError string) error {
	query := `
		UPDATE messages
		SET retry_count        = retry_count + 1,
		    last_error         = $2,
		    visibility_timeout = NULL,
		    updated_at         = now(),
		    status             = CASE
		                           WHEN retry_count + 1 < max_retries THEN 'pending'
		                           ELSE 'failed'
		                         END
		WHERE id = $1
		  AND status = 'processing'
	`

	result, err := s.pool.Exec(ctx, query, id, processingError)
	if err != nil {
		return fmt.Errorf("nack: %w", err)
	}

	if result.RowsAffected() == 0 {
		// Distinguish between "not found at all" and "wrong state".
		var exists bool
		err = s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM messages WHERE id = $1)`, id,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("nack existence check: %w", err)
		}
		if !exists {
			return fmt.Errorf("nack: %w", ErrNotFound)
		}
		return fmt.Errorf("nack: %w", ErrNotProcessing)
	}

	return nil
}

// List returns all messages, optionally filtered by topic.
func (s *Service) List(ctx context.Context, topic string) ([]Message, error) {
	var query string
	var args []any

	if topic != "" {
		query = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error, expires_at, created_at
		         FROM messages WHERE topic = $1 ORDER BY created_at DESC`
		args = append(args, topic)
	} else {
		query = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error, expires_at, created_at
		         FROM messages ORDER BY created_at DESC`
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var metaJSON []byte
		var lastError *string
		if err := rows.Scan(&msg.ID, &msg.Topic, &msg.Payload, &metaJSON, &msg.Status,
			&msg.RetryCount, &msg.MaxRetries, &lastError, &msg.ExpiresAt, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
		}
		if lastError != nil {
			msg.LastError = *lastError
		}
		if metaJSON != nil {
			if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
				return nil, fmt.Errorf("list unmarshal metadata: %w", err)
			}
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// StartExpiryReaper launches a background goroutine that periodically marks
// expired messages (expires_at < now()) as 'expired'. It runs until ctx is
// cancelled. The first tick fires immediately (after interval).
func (s *Service) StartExpiryReaper(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.pool.Exec(ctx, `
					UPDATE messages
					SET    status     = 'expired',
					       updated_at = now()
					WHERE  expires_at IS NOT NULL
					  AND  expires_at < now()
					  AND  status IN ('pending', 'processing')
				`)
			}
		}
	}()
}
