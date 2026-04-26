package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoMessage     = errors.New("no message available")
	ErrNotFound      = errors.New("message not found")
	ErrNotProcessing = errors.New("message is not in processing state")
	ErrNotDLQ        = errors.New("message is not a dead-letter message")
	ErrReservedTopic = errors.New("topic name is reserved: topics may not end with .dlq")
	ErrQueueFull     = errors.New("queue is at maximum depth for this topic")
)

type Message struct {
	ID            string
	Topic         string
	Payload       []byte
	Metadata      map[string]string
	Status        string
	RetryCount    int
	MaxRetries    int
	LastError     string
	ExpiresAt     *time.Time
	CreatedAt     time.Time
	OriginalTopic string
	DLQMovedAt    *time.Time
}

type Service struct {
	pool              *pgxpool.Pool
	visibilityTimeout time.Duration
	maxRetries        int
	messageTTL        time.Duration
	dlqThreshold      int
	recorder          MetricsRecorder
}

func NewService(pool *pgxpool.Pool, visibilityTimeout time.Duration, maxRetries int, messageTTL time.Duration, dlqThreshold int, recorder MetricsRecorder) *Service {
	if recorder == nil {
		recorder = NoopRecorder{}
	}
	return &Service{
		pool:              pool,
		visibilityTimeout: visibilityTimeout,
		maxRetries:        maxRetries,
		messageTTL:        messageTTL,
		dlqThreshold:      dlqThreshold,
		recorder:          recorder,
	}
}

// resolveEnqueueParams merges global service defaults with per-topic overrides
// from topic_config. It returns the effective maxRetries, expiresAt, and
// maxDepth (0 = unlimited) to use when inserting a new message.
func (s *Service) resolveEnqueueParams(ctx context.Context, topic string) (maxRetries int, expiresAt *time.Time, maxDepth int, err error) {
	maxRetries = s.maxRetries
	maxDepth = 0 // 0 = unlimited

	if s.messageTTL > 0 {
		t := time.Now().Add(s.messageTTL)
		expiresAt = &t
	}

	cfg, err := s.GetTopicConfig(ctx, topic)
	if err != nil {
		return 0, nil, 0, err
	}
	if cfg == nil {
		return maxRetries, expiresAt, maxDepth, nil
	}

	if cfg.MaxRetries != nil {
		maxRetries = *cfg.MaxRetries
	}
	if cfg.MessageTTLSeconds != nil {
		if *cfg.MessageTTLSeconds == 0 {
			expiresAt = nil // explicitly no TTL
		} else {
			t := time.Now().Add(time.Duration(*cfg.MessageTTLSeconds) * time.Second)
			expiresAt = &t
		}
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
func (s *Service) Enqueue(ctx context.Context, topic string, payload []byte, metadata map[string]string) (string, error) {
	if strings.HasSuffix(topic, ".dlq") {
		return "", fmt.Errorf("enqueue: %w", ErrReservedTopic)
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
		`INSERT INTO messages (topic, payload, metadata, max_retries, expires_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		topic, payload, metaJSON, maxRetries, expiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("enqueue: %w", err)
	}

	s.recorder.RecordEnqueue(topic)
	slog.Debug("message enqueued", "id", id, "topic", topic)
	return id, nil
}

func (s *Service) Dequeue(ctx context.Context, topic string, visibilityTimeout time.Duration) (*Message, error) {
	vt := s.visibilityTimeout
	if visibilityTimeout > 0 {
		vt = visibilityTimeout
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
		          COALESCE(original_topic, ''), dlq_moved_at
	`

	var msg Message
	var metaJSON []byte
	var lastError *string
	err := s.pool.QueryRow(ctx, query,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
		topic,
	).Scan(
		&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
		&msg.RetryCount, &msg.MaxRetries, &lastError,
		&msg.ExpiresAt, &msg.CreatedAt,
		&msg.OriginalTopic, &msg.DLQMovedAt,
	)

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

	s.recorder.RecordDequeue(msg.Topic)
	slog.Debug("message dequeued", "id", msg.ID, "topic", msg.Topic, "retry_count", msg.RetryCount)
	return &msg, nil
}

func (s *Service) Ack(ctx context.Context, id string) error {
	var topic string
	err := s.pool.QueryRow(ctx,
		`DELETE FROM messages WHERE id = $1 AND status = 'processing' RETURNING topic`, id,
	).Scan(&topic)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("ack: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("ack: %w", err)
	}
	s.recorder.RecordAck(topic)
	slog.Debug("message acked", "id", id)
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

	// Fetch the current message state inside the transaction so we can
	// decide the promotion path atomically.
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
	var newStatus string
	if nextRetryCount < maxRetries {
		newStatus = "pending"
	} else {
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
			"id", id,
			"topic", topic,
			"retry_count", nextRetryCount,
			"max_retries", maxRetries,
		)
	} else {
		s.recorder.RecordNack(topic, "retry")
		slog.Debug("message nacked, will retry",
			"id", id,
			"topic", topic,
			"retry_count", nextRetryCount,
			"max_retries", maxRetries,
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

// List returns paginated messages, optionally filtered by topic.
// Returns the page of messages, the total matching count, and any error.
func (s *Service) List(ctx context.Context, topic string, limit, offset int) ([]Message, int, error) {
	var countQuery, selectQuery string
	var args []any

	if topic != "" {
		countQuery = `SELECT COUNT(*) FROM messages WHERE topic = $1`
		selectQuery = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error,
		                      expires_at, created_at, COALESCE(original_topic, ''), dlq_moved_at
		               FROM messages WHERE topic = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		args = []any{topic, limit, offset}
	} else {
		countQuery = `SELECT COUNT(*) FROM messages`
		selectQuery = `SELECT id, topic, payload, metadata, status, retry_count, max_retries, last_error,
		                      expires_at, created_at, COALESCE(original_topic, ''), dlq_moved_at
		               FROM messages ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		args = []any{limit, offset}
	}

	var total int
	var countArgs []any
	if topic != "" {
		countArgs = []any{topic}
	}
	if err := s.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("list count: %w", err)
	}

	rows, err := s.pool.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list: %w", err)
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
			&msg.OriginalTopic, &msg.DLQMovedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("list scan: %w", err)
		}
		if lastError != nil {
			msg.LastError = *lastError
		}
		if metaJSON != nil {
			if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
				return nil, 0, fmt.Errorf("list unmarshal metadata: %w", err)
			}
		}
		messages = append(messages, msg)
	}

	slog.Debug("list messages", "topic", topic, "limit", limit, "offset", offset, "total", total, "returned", len(messages))
	return messages, total, nil
}

// TopicStat holds the message count for a single (topic, status) pair.
type TopicStat struct {
	Topic  string
	Status string
	Count  int
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
				tag, err := s.pool.Exec(ctx, `
					UPDATE messages
					SET    status     = 'expired',
					       updated_at = now()
					WHERE  expires_at IS NOT NULL
					  AND  expires_at < now()
					  AND  status IN ('pending', 'processing')
				`)
				if err != nil {
					slog.Error("expiry reaper failed", "error", err)
					continue
				}
				if n := tag.RowsAffected(); n > 0 {
					s.recorder.RecordExpired(n)
					slog.Info("expiry reaper expired messages", "count", n)
				}
			}
		}
	}()
}
