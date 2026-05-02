package queue_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = Describe("throughput throttle", func() {
	var (
		pool    *pgxpool.Pool
		service *queue.Service
		ctx     context.Context
	)

	const throttleTopic = "throttle-topic"

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM topic_config")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM topic_throughput")
		Expect(err).NotTo(HaveOccurred())

		service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	// seed n pending messages on throttleTopic
	seedMessages := func(n int) {
		GinkgoHelper()
		for i := range n {
			_, err := service.Enqueue(ctx, throttleTopic, fmt.Appendf(nil, "msg-%d", i), nil, nil)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	// setLimit upserts a topic_config row with the given throughput_limit.
	setLimit := func(limit int) {
		GinkgoHelper()
		Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
			Topic:           throttleTopic,
			ThroughputLimit: &limit,
		})).To(Succeed())
	}

	Describe("DequeueN", func() {
		Context("when the budget fully covers the requested batch", func() {
			BeforeEach(func() {
				// limit = 10 msg/s; request 3 — budget is ample on a fresh bucket.
				setLimit(10)
				seedMessages(5)
			})

			It("should return all requested messages with no error", func() {
				msgs, err := service.DequeueN(ctx, throttleTopic, 3, 30*time.Second)

				Expect(err).NotTo(HaveOccurred())
				Expect(msgs).To(HaveLen(3))
			})
		})

		Context("when the budget partially covers the requested batch (soft limit)", func() {
			BeforeEach(func() {
				// Seed 5 messages and set limit = 10. Then drain the bucket to 2 tokens
				// by directly updating topic_throughput.
				setLimit(10)
				seedMessages(5)

				// Pre-insert the throughput row with only 2 tokens remaining.
				_, err := pool.Exec(ctx, `
					INSERT INTO topic_throughput (topic, tokens, last_refill)
					VALUES ($1, 2, now())
					ON CONFLICT (topic) DO UPDATE
					SET tokens = 2, last_refill = now()
				`, throttleTopic)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return only as many messages as the remaining budget allows, with no error", func() {
				msgs, err := service.DequeueN(ctx, throttleTopic, 5, 30*time.Second)

				Expect(err).NotTo(HaveOccurred())
				// Only 2 tokens were available, so at most 2 messages are returned.
				Expect(len(msgs)).To(BeNumerically("<=", 2))
				Expect(len(msgs)).To(BeNumerically(">=", 1))
			})
		})

		Context("when the budget is exhausted", func() {
			BeforeEach(func() {
				setLimit(10)
				seedMessages(3)

				// Set tokens to 0 — bucket is empty.
				_, err := pool.Exec(ctx, `
					INSERT INTO topic_throughput (topic, tokens, last_refill)
					VALUES ($1, 0, now())
					ON CONFLICT (topic) DO UPDATE
					SET tokens = 0, last_refill = now()
				`, throttleTopic)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return an empty non-nil slice with no error", func() {
				msgs, err := service.DequeueN(ctx, throttleTopic, 3, 30*time.Second)

				Expect(err).NotTo(HaveOccurred())
				Expect(msgs).NotTo(BeNil())
				Expect(msgs).To(BeEmpty())
			})

			It("should leave all messages in pending status", func() {
				_, err := service.DequeueN(ctx, throttleTopic, 3, 30*time.Second)
				Expect(err).NotTo(HaveOccurred())

				var pendingCount int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE topic = $1 AND status = 'pending'`, throttleTopic,
				).Scan(&pendingCount)).To(Succeed())
				Expect(pendingCount).To(Equal(3))
			})
		})

		Context("when throughput_limit is nil (no topic_config row)", func() {
			BeforeEach(func() {
				// No topic_config at all — unlimited.
				seedMessages(5)
			})

			It("should return all requested messages without creating a topic_throughput row", func() {
				msgs, err := service.DequeueN(ctx, throttleTopic, 5, 30*time.Second)

				Expect(err).NotTo(HaveOccurred())
				Expect(msgs).To(HaveLen(5))

				var throttleRowCount int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM topic_throughput WHERE topic = $1`, throttleTopic,
				).Scan(&throttleRowCount)).To(Succeed())
				Expect(throttleRowCount).To(Equal(0))
			})
		})

		Context("when tokens refill over time", func() {
			BeforeEach(func() {
				// limit = 2 msg/s; set last_refill to 5 seconds ago with 0 tokens.
				// After refill: tokens = min(2, 0 + 2*5) = 2.
				setLimit(2)
				seedMessages(3)

				_, err := pool.Exec(ctx, `
					INSERT INTO topic_throughput (topic, tokens, last_refill)
					VALUES ($1, 0, now() - interval '5 seconds')
					ON CONFLICT (topic) DO UPDATE
					SET tokens = 0, last_refill = now() - interval '5 seconds'
				`, throttleTopic)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should refill the bucket and return up to the refilled amount", func() {
				msgs, err := service.DequeueN(ctx, throttleTopic, 3, 30*time.Second)

				Expect(err).NotTo(HaveOccurred())
				// 5 seconds × 2 msg/s = 10, capped at limit = 2; so 2 messages returned.
				Expect(msgs).To(HaveLen(2))
			})
		})
	})

	Describe("Dequeue (singular)", func() {
		Context("when the budget is exhausted", func() {
			BeforeEach(func() {
				setLimit(5)
				seedMessages(2)

				// Drain the bucket to 0.
				_, err := pool.Exec(ctx, `
					INSERT INTO topic_throughput (topic, tokens, last_refill)
					VALUES ($1, 0, now())
					ON CONFLICT (topic) DO UPDATE
					SET tokens = 0, last_refill = now()
				`, throttleTopic)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return ErrNoMessage so the subscriber backoff loop still works", func() {
				_, err := service.Dequeue(ctx, throttleTopic, 0)

				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("when the budget has at least one token", func() {
			BeforeEach(func() {
				setLimit(5)
				seedMessages(1)
				// Fresh bucket — tokens start at limit (5) on first call.
			})

			It("should return the message normally", func() {
				msg, err := service.Dequeue(ctx, throttleTopic, 0)

				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())
			})
		})
	})
})
