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

var ErrNoMessage = errors.New("no message available")

type Message struct {
	ID        string
	Topic     string
	Payload   []byte
	Metadata  map[string]string
	Status    string
	CreatedAt time.Time
}

type Service struct {
	pool              *pgxpool.Pool
	visibilityTimeout time.Duration
}

func NewService(pool *pgxpool.Pool, visibilityTimeout time.Duration) *Service {
	return &Service{
		pool:              pool,
		visibilityTimeout: visibilityTimeout,
	}
}

func (s *Service) Enqueue(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("marshal metadata: %w", err)
	}

	var id string
	err = s.pool.QueryRow(ctx,
		`INSERT INTO messages (topic, payload, metadata) VALUES ($1, $2, $3) RETURNING id`,
		topic, payload, metaJSON,
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
			ORDER BY created_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, metadata, created_at
	`

	var msg Message
	var metaJSON []byte
	err := s.pool.QueryRow(ctx, query,
		fmt.Sprintf("%d seconds", int(s.visibilityTimeout.Seconds())),
		topic,
	).Scan(&msg.ID, &msg.Topic, &msg.Payload, &metaJSON, &msg.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMessage
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue: %w", err)
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

// List returns all messages, optionally filtered by topic.
func (s *Service) List(ctx context.Context, topic string) ([]Message, error) {
	var query string
	var args []any

	if topic != "" {
		query = `SELECT id, topic, payload, metadata, status, created_at FROM messages WHERE topic = $1 ORDER BY created_at DESC`
		args = append(args, topic)
	} else {
		query = `SELECT id, topic, payload, metadata, status, created_at FROM messages ORDER BY created_at DESC`
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
		if err := rows.Scan(&msg.ID, &msg.Topic, &msg.Payload, &metaJSON, &msg.Status, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("list scan: %w", err)
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

