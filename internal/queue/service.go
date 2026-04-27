package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

var (
	ErrNoMessage           = errors.New("no message available")
	ErrNotFound            = errors.New("message not found")
	ErrNotProcessing       = errors.New("message is not in processing state")
	ErrNotDLQ              = errors.New("message is not a dead-letter message")
	ErrReservedTopic       = errors.New("topic name is reserved: topics may not end with .dlq")
	ErrQueueFull           = errors.New("queue is at maximum depth for this topic")
	ErrTopicNotRegistered  = errors.New("topic is not registered; an admin must create it first")
	ErrInvalidBatchSize    = errors.New("batch size must be between 1 and 1000")
)

// Advisory lock keys for reaper leader election across instances.
// These are stable, application-specific int64 values stored per database.
const (
	expiryReaperLockKey int64 = 7_000_001
	deleteReaperLockKey int64 = 7_000_002
)

type Message struct {
	ID            string
	Topic         string
	Key           *string
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
	pool                     *pgxpool.Pool
	visibilityTimeout        time.Duration
	maxRetries               int
	messageTTL               time.Duration
	dlqThreshold             int
	requireTopicRegistration bool
	recorder                 MetricsRecorder

	deleteReaperMu   sync.Mutex
	deleteReaperStop func()
}

// Pool returns the underlying connection pool. It is intentionally narrow —
// callers should prefer the Service methods for all queue operations, but the
// HTTP layer needs the pool to call package-level schema functions directly.
func (s *Service) Pool() *pgxpool.Pool {
	return s.pool
}

func NewService(pool *pgxpool.Pool, visibilityTimeout time.Duration, maxRetries int, messageTTL time.Duration, dlqThreshold int, requireTopicRegistration bool, recorder MetricsRecorder) *Service {
	if recorder == nil {
		recorder = NoopRecorder{}
	}
	return &Service{
		pool:                     pool,
		visibilityTimeout:        visibilityTimeout,
		maxRetries:               maxRetries,
		messageTTL:               messageTTL,
		dlqThreshold:             dlqThreshold,
		requireTopicRegistration: requireTopicRegistration,
		recorder:                 recorder,
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
		          COALESCE(original_topic, ''), dlq_moved_at, key
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
		&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
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

// DequeueN atomically claims up to n pending messages from the given topic,
// setting their status to 'processing' and their visibility_timeout to now() +
// visibilityTimeout. It returns an empty (non-nil) slice when no messages are
// available and never blocks. Returns ErrInvalidBatchSize when n < 1 or n > 1000.
func (s *Service) DequeueN(ctx context.Context, topic string, n int, visibilityTimeout time.Duration) ([]*Message, error) {
	if n < 1 || n > 1000 {
		return nil, ErrInvalidBatchSize
	}

	vt := s.visibilityTimeout
	if visibilityTimeout > 0 {
		vt = visibilityTimeout
	}

	query := `
		UPDATE messages
		SET    status             = 'processing',
		       visibility_timeout = now() + $3::interval,
		       updated_at         = now()
		WHERE  id IN (
		    SELECT id FROM messages
		    WHERE  topic = $1
		      AND  status = 'pending'
		      AND  (visibility_timeout IS NULL OR visibility_timeout < now())
		      AND  (expires_at IS NULL OR expires_at > now())
		      AND  (max_retries = 0 OR retry_count < max_retries)
		      AND  topic NOT LIKE '%.dlq'
		    ORDER BY created_at
		    LIMIT $2
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, topic, payload, metadata, retry_count, max_retries, last_error, expires_at, created_at,
		          COALESCE(original_topic, ''), dlq_moved_at, key
	`

	rows, err := s.pool.Query(ctx, query,
		topic,
		n,
		fmt.Sprintf("%d seconds", int(vt.Seconds())),
	)
	if err != nil {
		return nil, fmt.Errorf("dequeue batch: %w", err)
	}
	defer rows.Close()

	messages := make([]*Message, 0, n)
	for rows.Next() {
		var msg Message
		var metaJSON []byte
		var lastError *string
		if err := rows.Scan(
			&msg.ID, &msg.Topic, &msg.Payload, &metaJSON,
			&msg.RetryCount, &msg.MaxRetries, &lastError,
			&msg.ExpiresAt, &msg.CreatedAt,
			&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
		); err != nil {
			return nil, fmt.Errorf("dequeue batch scan: %w", err)
		}
		if lastError != nil {
			msg.LastError = *lastError
		}
		if metaJSON != nil {
			if err := json.Unmarshal(metaJSON, &msg.Metadata); err != nil {
				return nil, fmt.Errorf("dequeue batch unmarshal metadata: %w", err)
			}
		}
		s.recorder.RecordDequeue(msg.Topic)
		slog.Debug("message dequeued (batch)", "id", msg.ID, "topic", msg.Topic, "retry_count", msg.RetryCount)
		messages = append(messages, &msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dequeue batch rows: %w", err)
	}

	return messages, nil
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
			&msg.OriginalTopic, &msg.DLQMovedAt, &msg.Key,
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

// tryWithReaperLock acquires a transaction-level PostgreSQL advisory lock for
// lockKey, then calls fn inside that transaction and commits. Returns nil when
// fn succeeds. Returns nil (without calling fn) when another instance already
// holds the lock — the caller should silently skip that tick.
func (s *Service) tryWithReaperLock(ctx context.Context, lockKey int64, fn func(pgx.Tx) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("reaper lock begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var acquired bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockKey).Scan(&acquired); err != nil {
		return fmt.Errorf("reaper advisory lock: %w", err)
	}
	if !acquired {
		slog.Debug("reaper tick skipped: lock held by another instance", "lock_key", lockKey)
		return nil
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
				err := s.tryWithReaperLock(ctx, expiryReaperLockKey, func(tx pgx.Tx) error {
					tag, err := tx.Exec(ctx, `
						UPDATE messages
						SET    status     = 'expired',
						       updated_at = now()
						WHERE  expires_at IS NOT NULL
						  AND  expires_at < now()
						  AND  status IN ('pending', 'processing')
					`)
					if err != nil {
						return err
					}
					if n := tag.RowsAffected(); n > 0 {
						s.recorder.RecordExpired(n)
						slog.Info("expiry reaper expired messages", "count", n)
					}
					return nil
				})
				if err != nil {
					slog.Error("expiry reaper failed", "error", err)
				}
			}
		}
	}()
}

// RunExpiryReaperOnce marks all eligible expired messages as 'expired' in a
// single pass and returns the number of rows affected. It is the one-shot
// equivalent of the background StartExpiryReaper ticker.
func (s *Service) RunExpiryReaperOnce(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE messages
		SET    status     = 'expired',
		       updated_at = now()
		WHERE  expires_at IS NOT NULL
		  AND  expires_at < now()
		  AND  status IN ('pending', 'processing')
	`)
	if err != nil {
		return 0, fmt.Errorf("run expiry reaper: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		s.recorder.RecordExpired(n)
		slog.Info("expiry reaper (manual) expired messages", "count", n)
	}
	return n, nil
}

// RunDeleteReaperOnce permanently deletes all messages with status 'expired'
// and returns the number of rows deleted.
func (s *Service) RunDeleteReaperOnce(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM messages WHERE status = 'expired'`)
	if err != nil {
		return 0, fmt.Errorf("run delete reaper: %w", err)
	}
	n := tag.RowsAffected()
	if n > 0 {
		s.recorder.RecordDeleted(n)
		slog.Info("delete reaper removed expired messages", "count", n)
	}
	return n, nil
}

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

// StartDeleteReaper schedules the delete reaper using the given cron
// expression. If schedule is empty it is a no-op and returns a no-op stop
// function. The stop function stops the cron scheduler gracefully.
// The stop func is also stored internally so UpdateDeleteReaperSchedule can
// swap the cron at runtime.
func (s *Service) StartDeleteReaper(ctx context.Context, schedule string) (stop func(), err error) {
	if schedule == "" {
		noop := func() {}
		s.deleteReaperMu.Lock()
		s.deleteReaperStop = noop
		s.deleteReaperMu.Unlock()
		return noop, nil
	}

	c := cron.New()
	_, err = c.AddFunc(schedule, func() {
		runErr := s.tryWithReaperLock(ctx, deleteReaperLockKey, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `DELETE FROM messages WHERE status = 'expired'`)
			if err != nil {
				return err
			}
			if n := tag.RowsAffected(); n > 0 {
				s.recorder.RecordDeleted(n)
				slog.Info("delete reaper (scheduled) removed expired messages", "count", n)
			}
			return nil
		})
		if runErr != nil {
			slog.Error("delete reaper failed", "error", runErr)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("start delete reaper: invalid schedule %q: %w", schedule, err)
	}

	c.Start()
	stopFn := func() { c.Stop() }
	s.deleteReaperMu.Lock()
	s.deleteReaperStop = stopFn
	s.deleteReaperMu.Unlock()
	return stopFn, nil
}

// GetDeleteReaperSchedule returns the cron schedule stored in system_settings.
// Returns "" if no schedule has been persisted.
func (s *Service) GetDeleteReaperSchedule(ctx context.Context) (string, error) {
	var schedule string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM system_settings WHERE key = 'delete_reaper_schedule'`,
	).Scan(&schedule)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get delete reaper schedule: %w", err)
	}
	return schedule, nil
}

// UpdateDeleteReaperSchedule validates schedule, persists it to system_settings,
// stops the current cron, and starts a new one. An empty schedule disables the cron.
func (s *Service) UpdateDeleteReaperSchedule(ctx context.Context, schedule string) error {
	if schedule != "" {
		if _, parseErr := cron.ParseStandard(schedule); parseErr != nil {
			return fmt.Errorf("invalid cron schedule: %w", parseErr)
		}
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO system_settings (key, value) VALUES ('delete_reaper_schedule', $1)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value
	`, schedule)
	if err != nil {
		return fmt.Errorf("persist delete reaper schedule: %w", err)
	}

	s.deleteReaperMu.Lock()
	if s.deleteReaperStop != nil {
		s.deleteReaperStop()
		s.deleteReaperStop = nil
	}
	s.deleteReaperMu.Unlock()

	if schedule == "" {
		return nil
	}
	_, err = s.StartDeleteReaper(ctx, schedule)
	return err
}
