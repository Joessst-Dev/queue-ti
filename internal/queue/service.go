package queue

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Joessst-Dev/queue-ti/internal/broadcast"
)

var (
	ErrNoMessage            = errors.New("no message available")
	ErrNotFound             = errors.New("message not found")
	ErrNotProcessing        = errors.New("message is not in processing state")
	ErrNotDLQ               = errors.New("message is not a dead-letter message")
	ErrReservedTopic        = errors.New("topic name is reserved: topics may not end with .dlq")
	ErrQueueFull            = errors.New("queue is at maximum depth for this topic")
	ErrTopicNotRegistered   = errors.New("topic is not registered; an admin must create it first")
	ErrInvalidBatchSize     = errors.New("batch size must be between 1 and 1000")
	ErrTopicNotReplayable   = errors.New("topic is not configured as replayable")
	ErrReplayWindowTooOld   = errors.New("from_time is before the start of the replay window")
)

// Advisory lock keys for reaper leader election across instances.
// These are stable, application-specific int64 values stored per database.
const (
	expiryReaperLockKey int64 = 7_000_001
	deleteReaperLockKey int64 = 7_000_002
)

// PagedResult wraps a page of items and the total matching count.
// It is the canonical return type for any service method that returns
// a paginated list, keeping those methods within the 2-return-value rule.
type PagedResult[T any] struct {
	Items []T
	Total int
}

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
	broadcaster              broadcast.Broadcaster

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
		broadcaster:              broadcast.Noop{},
	}
}

func (s *Service) SetBroadcaster(b broadcast.Broadcaster) {
	s.broadcaster = b
}

func (s *Service) StartBroadcastListener(ctx context.Context) {
	go s.listenSchemaChanges(ctx)
	go s.listenConfigChanges(ctx)
}

func (s *Service) listenSchemaChanges(ctx context.Context) {
	ch, cancel := s.broadcaster.Subscribe(ctx, broadcast.ChannelSchemaChanged)
	defer cancel()
	for {
		select {
		case topic, ok := <-ch:
			if !ok {
				return
			}
			globalSchemaCache.m.Delete(topic)
			slog.Info("schema cache invalidated via broadcast", "topic", topic)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) listenConfigChanges(ctx context.Context) {
	ch, cancel := s.broadcaster.Subscribe(ctx, broadcast.ChannelConfigChanged)
	defer cancel()
	for {
		select {
		case topic, ok := <-ch:
			if !ok {
				return
			}
			slog.Info("config change received via broadcast", "topic", topic)
		case <-ctx.Done():
			return
		}
	}
}
