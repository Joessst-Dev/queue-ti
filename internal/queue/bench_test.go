package queue_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newBenchService creates an isolated Service backed by a freshly migrated and
// truncated messages table. It registers pool.Close as a b.Cleanup so callers
// do not need to handle teardown themselves.
func newBenchService(b *testing.B) (*queue.Service, *pgxpool.Pool) {
	b.Helper()

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, containerDSN)
	if err != nil {
		b.Fatalf("newBenchService: connect pool: %v", err)
	}

	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		b.Fatalf("newBenchService: migrate: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE messages"); err != nil {
		pool.Close()
		b.Fatalf("newBenchService: truncate messages: %v", err)
	}

	b.Cleanup(pool.Close)

	svc := queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
	return svc, pool
}

// BenchmarkEnqueue measures sequential single-goroutine enqueue throughput.
func BenchmarkEnqueue(b *testing.B) {
	svc, _ := newBenchService(b)
	ctx := context.Background()

	for b.Loop() {
		if _, err := svc.Enqueue(ctx, "bench-enqueue", []byte("payload"), nil); err != nil {
			b.Fatalf("Enqueue: %v", err)
		}
	}
}

// BenchmarkEnqueueParallel measures concurrent enqueue throughput across
// multiple goroutines.
func BenchmarkEnqueueParallel(b *testing.B) {
	svc, _ := newBenchService(b)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := svc.Enqueue(ctx, "bench-enqueue-parallel", []byte("payload"), nil); err != nil {
				b.Errorf("Enqueue: %v", err)
				return
			}
		}
	})
}

// BenchmarkDequeueAck pre-seeds 10 000 messages, then for each iteration
// dequeues one message and immediately acks it. This exercises the
// dequeue+ack hot path without enqueue overhead in the measured loop.
func BenchmarkDequeueAck(b *testing.B) {
	svc, pool := newBenchService(b)
	ctx := context.Background()

	const seed = 10_000
	for i := range seed {
		payload := fmt.Appendf(nil, "seed-%d", i)
		if _, err := svc.Enqueue(ctx, "bench-dequeue-ack", payload, nil); err != nil {
			b.Fatalf("seed Enqueue: %v", err)
		}
	}

	for b.Loop() {
		msg, err := svc.Dequeue(ctx, "bench-dequeue-ack", 0)
		if err != nil {
			// Queue drained before b.N iterations — stop cleanly.
			b.StopTimer()

			// Re-seed so subsequent sub-benchmark runs (if any) can continue.
			// In practice this path is only hit when b.N > seed.
			if _, execErr := pool.Exec(ctx, "TRUNCATE messages"); execErr != nil {
				b.Fatalf("re-seed truncate: %v", execErr)
			}
			for i := range seed {
				payload := fmt.Appendf(nil, "reseed-%d", i)
				if _, enqErr := svc.Enqueue(ctx, "bench-dequeue-ack", payload, nil); enqErr != nil {
					b.Fatalf("re-seed Enqueue: %v", enqErr)
				}
			}
			b.StartTimer()
			continue
		}

		if err := svc.Ack(ctx, msg.ID); err != nil {
			b.Fatalf("Ack: %v", err)
		}
	}
}

// BenchmarkFullPipeline measures the full enqueue → dequeue → ack round-trip
// under parallel load. Each goroutine owns its own topic slice to avoid
// contention on FOR UPDATE SKIP LOCKED.
func BenchmarkFullPipeline(b *testing.B) {
	svc, _ := newBenchService(b)
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		topic := fmt.Sprintf("bench-pipeline-%p", pb)
		for pb.Next() {
			id, err := svc.Enqueue(ctx, topic, []byte("payload"), nil)
			if err != nil {
				b.Errorf("Enqueue: %v", err)
				return
			}

			msg, err := svc.Dequeue(ctx, topic, 0)
			if err != nil {
				b.Errorf("Dequeue: %v", err)
				return
			}

			if err := svc.Ack(ctx, msg.ID); err != nil {
				b.Errorf("Ack %s: %v", id, err)
				return
			}
		}
	})
}
