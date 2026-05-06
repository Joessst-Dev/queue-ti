package queue_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ = Describe("Consumer Groups", func() {
	var (
		pool    *pgxpool.Pool
		svc     *queue.Service
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		_, err = pool.Exec(ctx, "DELETE FROM consumer_groups")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM topic_config")
		Expect(err).NotTo(HaveOccurred())

		svc = queue.NewService(pool, 30*time.Second, 3, 0, 0, false, queue.NoopRecorder{})
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	// -------------------------------------------------------------------------
	// RegisterConsumerGroup
	// -------------------------------------------------------------------------

	Describe("RegisterConsumerGroup", func() {
		Context("when the group does not yet exist", func() {
			It("should register the group and return no error", func() {
				err := svc.RegisterConsumerGroup(ctx, "orders", "groupA")

				Expect(err).NotTo(HaveOccurred())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM consumer_groups WHERE topic = 'orders' AND consumer_group = 'groupA'`,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(1))
			})
		})

		Context("when requireTopicRegistration is enabled and the topic has no config row", func() {
			It("should return ErrTopicNotRegistered", func() {
				strictSvc := queue.NewService(pool, 30*time.Second, 3, 0, 0, true, queue.NoopRecorder{})

				err := strictSvc.RegisterConsumerGroup(ctx, "unknown-topic", "groupA")

				Expect(err).To(MatchError(queue.ErrTopicNotRegistered))
			})
		})

		Context("when the same group is registered a second time", func() {
			BeforeEach(func() {
				Expect(svc.RegisterConsumerGroup(ctx, "orders", "groupA")).To(Succeed())
			})

			It("should return ErrConsumerGroupExists", func() {
				err := svc.RegisterConsumerGroup(ctx, "orders", "groupA")

				Expect(err).To(MatchError(queue.ErrConsumerGroupExists))
			})
		})

		Context("when pending messages already exist on the topic at registration time", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, "orders", []byte("backfill-me"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should backfill a delivery row for each pending message", func() {
				Expect(svc.RegisterConsumerGroup(ctx, "orders", "groupA")).To(Succeed())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`,
					msgID,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(1))
			})
		})
	})

	// -------------------------------------------------------------------------
	// UnregisterConsumerGroup
	// -------------------------------------------------------------------------

	Describe("UnregisterConsumerGroup", func() {
		Context("when the group is registered", func() {
			BeforeEach(func() {
				Expect(svc.RegisterConsumerGroup(ctx, "orders", "groupA")).To(Succeed())
			})

			It("should remove the group and return no error", func() {
				err := svc.UnregisterConsumerGroup(ctx, "orders", "groupA")

				Expect(err).NotTo(HaveOccurred())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM consumer_groups WHERE topic = 'orders' AND consumer_group = 'groupA'`,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(0))
			})
		})

		Context("when the group does not exist", func() {
			It("should return ErrConsumerGroupNotFound", func() {
				err := svc.UnregisterConsumerGroup(ctx, "orders", "ghost")

				Expect(err).To(MatchError(queue.ErrConsumerGroupNotFound))
			})
		})
	})

	// -------------------------------------------------------------------------
	// ListConsumerGroups
	// -------------------------------------------------------------------------

	Describe("ListConsumerGroups", func() {
		Context("when no groups are registered for a topic", func() {
			It("should return an empty (non-nil) slice", func() {
				groups, err := svc.ListConsumerGroups(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				Expect(groups).NotTo(BeNil())
				Expect(groups).To(BeEmpty())
			})
		})

		Context("when multiple groups are registered", func() {
			BeforeEach(func() {
				// Register in a controlled order and verify they come back oldest-first.
				Expect(svc.RegisterConsumerGroup(ctx, "orders", "alpha")).To(Succeed())
				time.Sleep(5 * time.Millisecond) // ensure distinct created_at values
				Expect(svc.RegisterConsumerGroup(ctx, "orders", "beta")).To(Succeed())
			})

			It("should return the groups ordered by registration time", func() {
				groups, err := svc.ListConsumerGroups(ctx, "orders")

				Expect(err).NotTo(HaveOccurred())
				Expect(groups).To(Equal([]string{"alpha", "beta"}))
			})
		})
	})

	// -------------------------------------------------------------------------
	// DequeueForGroup
	// -------------------------------------------------------------------------

	Describe("DequeueForGroup", func() {
		const topic = "cg-dequeue-topic"
		const timeout = 30 * time.Second

		BeforeEach(func() {
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupA")).To(Succeed())
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupB")).To(Succeed())
		})

		Context("when no messages exist on the topic", func() {
			It("should return ErrNoMessage", func() {
				_, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)

				Expect(err).To(MatchError(queue.ErrNoMessage))
			})
		})

		Context("when a message is enqueued after two groups are registered", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("hello"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should deliver the message independently to each group", func() {
				msgA, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgA.ID).To(Equal(msgID))
				Expect(msgA.Payload).To(Equal([]byte("hello")))

				// groupB must also receive the same message — not "stolen".
				msgB, err := svc.DequeueForGroup(ctx, topic, "groupB", timeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgB.ID).To(Equal(msgID))
			})

			It("should set the delivery status to 'processing' for the consuming group only", func() {
				_, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())

				var statusA, statusB string
				Expect(pool.QueryRow(ctx,
					`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
				).Scan(&statusA)).To(Succeed())
				Expect(pool.QueryRow(ctx,
					`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupB'`, msgID,
				).Scan(&statusB)).To(Succeed())

				Expect(statusA).To(Equal("processing"))
				Expect(statusB).To(Equal("pending"))
			})
		})

		Context("when a delivery's visibility_timeout is in the past", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("redeliver-me"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				// Dequeue with a 1-second timeout then expire it immediately.
				_, err = svc.DequeueForGroup(ctx, topic, "groupA", 1*time.Second)
				Expect(err).NotTo(HaveOccurred())

				// Artificially expire the visibility_timeout.
				_, err = pool.Exec(ctx,
					`UPDATE message_deliveries SET status = 'pending', visibility_timeout = now() - interval '1 second'
					 WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should redeliver the message to groupA", func() {
				msg, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)

				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(msgID))
			})
		})
	})

	// -------------------------------------------------------------------------
	// DequeueNForGroup
	// -------------------------------------------------------------------------

	Describe("DequeueNForGroup", func() {
		const topic = "cg-dequeueN-topic"
		const timeout = 30 * time.Second

		BeforeEach(func() {
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupA")).To(Succeed())
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupB")).To(Succeed())

			for range 3 {
				_, err := svc.Enqueue(ctx, topic, []byte("msg"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("when n is invalid", func() {
			It("should return ErrInvalidBatchSize for n = 0", func() {
				_, err := svc.DequeueNForGroup(ctx, topic, "groupA", 0, timeout)
				Expect(err).To(MatchError(queue.ErrInvalidBatchSize))
			})

			It("should return ErrInvalidBatchSize for n = 1001", func() {
				_, err := svc.DequeueNForGroup(ctx, topic, "groupA", 1001, timeout)
				Expect(err).To(MatchError(queue.ErrInvalidBatchSize))
			})
		})

		Context("when no messages are available", func() {
			It("should return an empty (non-nil) slice", func() {
				// Drain first.
				_, err := svc.DequeueNForGroup(ctx, topic, "groupA", 10, timeout)
				Expect(err).NotTo(HaveOccurred())

				msgs, err := svc.DequeueNForGroup(ctx, topic, "groupA", 10, timeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgs).NotTo(BeNil())
				Expect(msgs).To(BeEmpty())
			})
		})

		Context("when messages exist", func() {
			It("should return up to N messages for the calling group", func() {
				msgs, err := svc.DequeueNForGroup(ctx, topic, "groupA", 2, timeout)

				Expect(err).NotTo(HaveOccurred())
				Expect(msgs).To(HaveLen(2))
			})

			It("should isolate batches: groupB still sees all 3 messages after groupA dequeued 2", func() {
				_, err := svc.DequeueNForGroup(ctx, topic, "groupA", 2, timeout)
				Expect(err).NotTo(HaveOccurred())

				msgsB, err := svc.DequeueNForGroup(ctx, topic, "groupB", 10, timeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgsB).To(HaveLen(3))
			})
		})
	})

	// -------------------------------------------------------------------------
	// AckForGroup
	// -------------------------------------------------------------------------

	Describe("AckForGroup", func() {
		const topic = "cg-ack-topic"
		const timeout = 30 * time.Second

		BeforeEach(func() {
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupA")).To(Succeed())
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupB")).To(Succeed())
		})

		Context("when a delivery is in 'processing' state", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("ack-me"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should delete the delivery row for the acking group", func() {
				Expect(svc.AckForGroup(ctx, msgID, "groupA")).To(Succeed())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(0))
			})

			It("should NOT delete the parent message when groupB's delivery still exists", func() {
				Expect(svc.AckForGroup(ctx, msgID, "groupA")).To(Succeed())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE id = $1`, msgID,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(1))
			})
		})

		Context("when it is the last group to ack", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("last-ack"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
				_, err = svc.DequeueForGroup(ctx, topic, "groupB", timeout)
				Expect(err).NotTo(HaveOccurred())

				// groupA acks first — message must survive.
				Expect(svc.AckForGroup(ctx, msgID, "groupA")).To(Succeed())
			})

			It("should delete the parent messages row after the final ack", func() {
				Expect(svc.AckForGroup(ctx, msgID, "groupB")).To(Succeed())

				var count int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE id = $1`, msgID,
				).Scan(&count)).To(Succeed())
				Expect(count).To(Equal(0))
			})
		})

		Context("when the delivery does not exist or is not processing", func() {
			It("should return ErrNotFound for an unknown message+group pair", func() {
				err := svc.AckForGroup(ctx, "00000000-0000-0000-0000-000000000000", "groupA")

				Expect(err).To(MatchError(ContainSubstring(queue.ErrNotFound.Error())))
			})
		})

		Context("when the topic is configured as replayable and both groups ack", func() {
			var msgID string

			BeforeEach(func() {
				_, err := pool.Exec(ctx,
					`INSERT INTO topic_config (topic, replayable) VALUES ($1, true)
					 ON CONFLICT (topic) DO UPDATE SET replayable = true`, topic)
				Expect(err).NotTo(HaveOccurred())

				msgID, err = svc.Enqueue(ctx, topic, []byte("replayable-msg"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
				_, err = svc.DequeueForGroup(ctx, topic, "groupB", timeout)
				Expect(err).NotTo(HaveOccurred())

				// groupA acks first — message_log must NOT yet have the row.
				Expect(svc.AckForGroup(ctx, msgID, "groupA")).To(Succeed())
			})

			It("should archive the message to message_log only after the last group acks", func() {
				var countBefore int
				Expect(pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM message_log WHERE id = $1`, msgID,
				).Scan(&countBefore)).To(Succeed())
				Expect(countBefore).To(Equal(0))

				Expect(svc.AckForGroup(ctx, msgID, "groupB")).To(Succeed())

				var countAfter int
				Expect(pool.QueryRow(ctx,
					`SELECT COUNT(*) FROM message_log WHERE id = $1`, msgID,
				).Scan(&countAfter)).To(Succeed())
				Expect(countAfter).To(Equal(1))
			})
		})
	})

	// -------------------------------------------------------------------------
	// NackForGroup
	// -------------------------------------------------------------------------

	Describe("NackForGroup", func() {
		const topic = "cg-nack-topic"
		const timeout = 30 * time.Second

		BeforeEach(func() {
			Expect(svc.RegisterConsumerGroup(ctx, topic, "groupA")).To(Succeed())
		})

		Context("when the delivery is processing", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("nack-me"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should increment retry_count, record last_error, reset visibility_timeout, and set status to pending", func() {
				Expect(svc.NackForGroup(ctx, msgID, "groupA", "transient error")).To(Succeed())

				var retryCount int
				var lastError string
				var vt *time.Time
				var status string
				Expect(pool.QueryRow(ctx, `
					SELECT retry_count, last_error, visibility_timeout, status
					FROM   message_deliveries
					WHERE  message_id = $1 AND consumer_group = 'groupA'
				`, msgID).Scan(&retryCount, &lastError, &vt, &status)).To(Succeed())

				Expect(retryCount).To(Equal(1))
				Expect(lastError).To(Equal("transient error"))
				Expect(vt).To(BeNil())
				Expect(status).To(Equal("pending"))
			})

			It("should allow the message to be redelivered after a nack", func() {
				Expect(svc.NackForGroup(ctx, msgID, "groupA", "retry me")).To(Succeed())

				msg, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(msgID))
				Expect(msg.RetryCount).To(Equal(1))
			})
		})

		Context("when max_retries is exhausted", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("final"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				// The trigger creates the delivery with max_retries = 0 (unlimited).
				// Set max_retries = 1 directly so the first nack exhausts retries.
				_, err = pool.Exec(ctx,
					`UPDATE message_deliveries SET max_retries = 1 WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should set status to 'failed' and make the delivery unavailable", func() {
				Expect(svc.NackForGroup(ctx, msgID, "groupA", "fatal")).To(Succeed())

				var status string
				Expect(pool.QueryRow(ctx,
					`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
				).Scan(&status)).To(Succeed())
				Expect(status).To(Equal("failed"))

				_, err := svc.DequeueForGroup(ctx, topic, "groupA", timeout)
				Expect(err).To(MatchError(queue.ErrNoMessage))
			})
		})

		Context("when the delivery does not exist", func() {
			It("should return ErrNotFound", func() {
				err := svc.NackForGroup(ctx, "00000000-0000-0000-0000-000000000000", "groupA", "irrelevant")

				Expect(err).To(MatchError(ContainSubstring(queue.ErrNotFound.Error())))
			})
		})

		Context("when the delivery is not in processing state", func() {
			var msgID string

			BeforeEach(func() {
				var err error
				msgID, err = svc.Enqueue(ctx, topic, []byte("pending"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
				// Deliberately do NOT dequeue — delivery stays 'pending'.
			})

			It("should return ErrNotProcessing", func() {
				err := svc.NackForGroup(ctx, msgID, "groupA", "should fail")

				Expect(err).To(MatchError(ContainSubstring(queue.ErrNotProcessing.Error())))
			})
		})

		Context("when a per-topic MaxRetries is lower than the global dlqThreshold", func() {
			const perTopicTopic = "cg-per-topic-low"
			var msgID string

			BeforeEach(func() {
				// Global dlqThreshold = 5 — without the fix one nack would leave
				// the delivery as 'pending'. Per-topic MaxRetries = 1 should take
				// precedence and mark the delivery 'failed' immediately.
				svc = queue.NewService(pool, 30*time.Second, 3, 0, 5, false, queue.NoopRecorder{})

				perTopicRetries := 1
				err := svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      perTopicTopic,
					MaxRetries: &perTopicRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(svc.RegisterConsumerGroup(ctx, perTopicTopic, "groupA")).To(Succeed())

				msgID, err = svc.Enqueue(ctx, perTopicTopic, []byte("work"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = svc.DequeueForGroup(ctx, perTopicTopic, "groupA", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should mark the delivery failed after the first nack", func() {
				Expect(svc.NackForGroup(ctx, msgID, "groupA", "threshold hit")).To(Succeed())

				var status string
				Expect(pool.QueryRow(ctx,
					`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
				).Scan(&status)).To(Succeed())
				Expect(status).To(Equal("failed"))
			})
		})

		Context("when a per-topic MaxRetries is higher than the global dlqThreshold", func() {
			const perTopicTopic = "cg-per-topic-high"
			var msgID string

			BeforeEach(func() {
				// Global dlqThreshold = 1 — without the fix one nack would mark
				// the delivery 'failed'. Per-topic MaxRetries = 3 should override
				// and keep the delivery 'pending' so it can be retried.
				svc = queue.NewService(pool, 30*time.Second, 3, 0, 1, false, queue.NoopRecorder{})

				perTopicRetries := 3
				err := svc.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      perTopicTopic,
					MaxRetries: &perTopicRetries,
				})
				Expect(err).NotTo(HaveOccurred())

				Expect(svc.RegisterConsumerGroup(ctx, perTopicTopic, "groupA")).To(Succeed())

				msgID, err = svc.Enqueue(ctx, perTopicTopic, []byte("work"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should keep the delivery pending after the first nack", func() {
				_, err := svc.DequeueForGroup(ctx, perTopicTopic, "groupA", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
				Expect(svc.NackForGroup(ctx, msgID, "groupA", "transient")).To(Succeed())

				var status string
				Expect(pool.QueryRow(ctx,
					`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
				).Scan(&status)).To(Succeed())
				Expect(status).To(Equal("pending"))
			})

			It("should mark the delivery failed on the third nack", func() {
				for i := range 3 {
					_, err := svc.DequeueForGroup(ctx, perTopicTopic, "groupA", 30*time.Second)
					Expect(err).NotTo(HaveOccurred())
					Expect(svc.NackForGroup(ctx, msgID, "groupA", "transient")).To(Succeed())

					var status string
					Expect(pool.QueryRow(ctx,
						`SELECT status FROM message_deliveries WHERE message_id = $1 AND consumer_group = 'groupA'`, msgID,
					).Scan(&status)).To(Succeed())

					if i < 2 {
						Expect(status).To(Equal("pending"), "expected pending after nack %d", i+1)
					} else {
						Expect(status).To(Equal("failed"), "expected failed after nack %d", i+1)
					}
				}
			})
		})
	})
})
