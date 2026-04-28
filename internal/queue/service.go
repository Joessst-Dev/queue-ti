package queue

import (
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoMessage          = errors.New("no message available")
	ErrNotFound           = errors.New("message not found")
	ErrNotProcessing      = errors.New("message is not in processing state")
	ErrNotDLQ             = errors.New("message is not a dead-letter message")
	ErrReservedTopic      = errors.New("topic name is reserved: topics may not end with .dlq")
	ErrQueueFull          = errors.New("queue is at maximum depth for this topic")
	ErrTopicNotRegistered = errors.New("topic is not registered; an admin must create it first")
	ErrInvalidBatchSize   = errors.New("batch size must be between 1 and 1000")
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
