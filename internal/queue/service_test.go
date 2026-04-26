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

var _ = Describe("Queue Service", func() {
	var (
		pool    *pgxpool.Pool
		service *queue.Service
		ctx     context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())

		// Ensure a clean state before each test
		_, err = pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())

		service = queue.NewService(pool, 30*time.Second, 3, 0)
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	// Tests for enqueueing messages into the queue
	Describe("Enqueue", func() {
		Context("Given a valid topic and payload", func() {
			It("should persist the message and return a unique ID", func() {
				// When we enqueue a message with metadata
				id, err := service.Enqueue(ctx, "test-topic", []byte("hello"), map[string]string{"key": "value"})

				// Then no error occurs and a non-empty ID is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeEmpty())
			})
		})

		Context("Given a service configured with max_retries = 5", func() {
			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 5, 0)
			})

			It("should store the message with retry_count = 0 and max_retries = 5", func() {
				id, err := service.Enqueue(ctx, "retry-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				var retryCount, maxRetries int
				err = pool.QueryRow(ctx,
					`SELECT retry_count, max_retries FROM messages WHERE id = $1`, id,
				).Scan(&retryCount, &maxRetries)
				Expect(err).NotTo(HaveOccurred())
				Expect(retryCount).To(Equal(0))
				Expect(maxRetries).To(Equal(5))
			})
		})

		Context("Given a service with a TTL of 1 hour", func() {
			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 3, time.Hour)
			})

			It("should set expires_at to approximately now + TTL", func() {
				before := time.Now()
				id, err := service.Enqueue(ctx, "ttl-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())
				after := time.Now()

				var expiresAt *time.Time
				err = pool.QueryRow(ctx,
					`SELECT expires_at FROM messages WHERE id = $1`, id,
				).Scan(&expiresAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(expiresAt).NotTo(BeNil())
				Expect(*expiresAt).To(BeTemporally(">=", before.Add(time.Hour-time.Second)))
				Expect(*expiresAt).To(BeTemporally("<=", after.Add(time.Hour+time.Second)))
			})
		})

		Context("Given a service with TTL = 0 (no expiry)", func() {
			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 3, 0)
			})

			It("should store the message with expires_at = NULL", func() {
				id, err := service.Enqueue(ctx, "no-ttl-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				var expiresAt *time.Time
				err = pool.QueryRow(ctx,
					`SELECT expires_at FROM messages WHERE id = $1`, id,
				).Scan(&expiresAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(expiresAt).To(BeNil())
			})
		})
	})

	// Tests for dequeueing messages from the queue
	Describe("Dequeue", func() {
		Context("Given multiple messages on the same topic", func() {
			var firstID string

			BeforeEach(func() {
				// Given two messages are enqueued in order
				var err error
				firstID, err = service.Enqueue(ctx, "test-topic", []byte("first"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Enqueue(ctx, "test-topic", []byte("second"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the oldest pending message first (FIFO)", func() {
				// When we dequeue from the topic
				msg, err := service.Dequeue(ctx, "test-topic")

				// Then the first enqueued message is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(firstID))
				Expect(msg.Payload).To(Equal([]byte("first")))
			})
		})

		Context("Given an empty topic with no pending messages", func() {
			It("should return ErrNoMessage", func() {
				// When we attempt to dequeue from a topic with no messages
				_, err := service.Dequeue(ctx, "empty-topic")

				// Then the specific no-message error is returned
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a single message that has already been dequeued", func() {
			BeforeEach(func() {
				// Given one message is enqueued and then dequeued
				_, err := service.Enqueue(ctx, "test-topic", []byte("only-one"), nil)
				Expect(err).NotTo(HaveOccurred())

				msg, err := service.Dequeue(ctx, "test-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())
			})

			It("should not deliver the same message twice", func() {
				// When we attempt to dequeue again
				_, err := service.Dequeue(ctx, "test-topic")

				// Then no message is available
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a message whose expires_at is in the past", func() {
			BeforeEach(func() {
				// Insert an already-expired message directly so we can control the timestamp.
				_, err := pool.Exec(ctx, `
					INSERT INTO messages (topic, payload, max_retries, expires_at)
					VALUES ('exp-topic', 'stale', 3, now() - interval '1 second')
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not return the expired message", func() {
				_, err := service.Dequeue(ctx, "exp-topic")
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a message whose retry_count has reached max_retries", func() {
			BeforeEach(func() {
				// Insert a message that has already exhausted its retries.
				_, err := pool.Exec(ctx, `
					INSERT INTO messages (topic, payload, retry_count, max_retries)
					VALUES ('exhausted-topic', 'done', 3, 3)
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should not return the retry-exhausted message", func() {
				_, err := service.Dequeue(ctx, "exhausted-topic")
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a message with retries remaining and no expiry", func() {
			BeforeEach(func() {
				_, err := service.Enqueue(ctx, "available-topic", []byte("work"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the message normally", func() {
				msg, err := service.Dequeue(ctx, "available-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())
				Expect(msg.Payload).To(Equal([]byte("work")))
			})
		})
	})

	// Tests for acknowledging (completing) messages
	Describe("Ack", func() {
		Context("Given a message that is currently being processed", func() {
			var messageID string

			BeforeEach(func() {
				// Given a message is enqueued and then dequeued (status = processing)
				var err error
				messageID, err = service.Enqueue(ctx, "test-topic", []byte("ack-me"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "test-topic")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should permanently remove the message from the queue", func() {
				// When we acknowledge the message
				err := service.Ack(ctx, messageID)

				// Then no error occurs
				Expect(err).NotTo(HaveOccurred())

				// And the message no longer exists in the database
				var count int
				err = pool.QueryRow(ctx, "SELECT count(*) FROM messages WHERE id = $1", messageID).Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0))
			})
		})

		Context("Given a message ID that does not exist", func() {
			It("should return an error", func() {
				// When we try to ack a non-existent message
				err := service.Ack(ctx, "00000000-0000-0000-0000-000000000000")

				// Then an error is returned
				Expect(err).To(HaveOccurred())
			})
		})
	})

	// Tests for negative acknowledgement (Nack)
	Describe("Nack", func() {
		Context("Given a message that is currently being processed", func() {
			var messageID string

			BeforeEach(func() {
				var err error
				messageID, err = service.Enqueue(ctx, "nack-topic", []byte("work"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "nack-topic")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should increment retry_count, set last_error, and reset visibility_timeout to NULL", func() {
				err := service.Nack(ctx, messageID, "something went wrong")
				Expect(err).NotTo(HaveOccurred())

				var retryCount int
				var lastError *string
				var visibilityTimeout *time.Time
				err = pool.QueryRow(ctx,
					`SELECT retry_count, last_error, visibility_timeout FROM messages WHERE id = $1`, messageID,
				).Scan(&retryCount, &lastError, &visibilityTimeout)
				Expect(err).NotTo(HaveOccurred())
				Expect(retryCount).To(Equal(1))
				Expect(lastError).NotTo(BeNil())
				Expect(*lastError).To(Equal("something went wrong"))
				Expect(visibilityTimeout).To(BeNil())
			})
		})

		Context("Given a service with max_retries = 3 and a message on its first nack (retry_count 0 → 1)", func() {
			var messageID string

			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 3, 0)
				var err error
				messageID, err = service.Enqueue(ctx, "retry-nack-topic", []byte("retry"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "retry-nack-topic")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should set status back to 'pending' so the message can be dequeued again", func() {
				err := service.Nack(ctx, messageID, "transient error")
				Expect(err).NotTo(HaveOccurred())

				var msgStatus string
				err = pool.QueryRow(ctx,
					`SELECT status FROM messages WHERE id = $1`, messageID,
				).Scan(&msgStatus)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgStatus).To(Equal("pending"))

				// Confirm it can actually be dequeued again.
				msg, err := service.Dequeue(ctx, "retry-nack-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(messageID))
			})
		})

		Context("Given a service with max_retries = 1 and a message on its first (and last) nack", func() {
			var messageID string

			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 1, 0)
				var err error
				messageID, err = service.Enqueue(ctx, "final-nack-topic", []byte("one-shot"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "final-nack-topic")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should transition the message to 'failed' and make it unavailable for dequeue", func() {
				err := service.Nack(ctx, messageID, "fatal error")
				Expect(err).NotTo(HaveOccurred())

				var msgStatus string
				err = pool.QueryRow(ctx,
					`SELECT status FROM messages WHERE id = $1`, messageID,
				).Scan(&msgStatus)
				Expect(err).NotTo(HaveOccurred())
				Expect(msgStatus).To(Equal("failed"))

				// Confirm it is no longer dequeue-able.
				_, err = service.Dequeue(ctx, "final-nack-topic")
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a message ID that does not exist", func() {
			It("should return ErrNotFound", func() {
				err := service.Nack(ctx, "00000000-0000-0000-0000-000000000000", "irrelevant")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(queue.ErrNotFound.Error())))
			})
		})

		Context("Given a message that is in 'pending' state (not processing)", func() {
			var messageID string

			BeforeEach(func() {
				var err error
				messageID, err = service.Enqueue(ctx, "pending-nack-topic", []byte("pending"), nil)
				Expect(err).NotTo(HaveOccurred())
				// Deliberately do NOT dequeue it — it stays 'pending'.
			})

			It("should return ErrNotProcessing", func() {
				err := service.Nack(ctx, messageID, "should fail")
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(ContainSubstring(queue.ErrNotProcessing.Error())))
			})
		})
	})

	// Tests for metadata preservation through the queue lifecycle
	Describe("Metadata", func() {
		Context("Given a message with metadata key-value pairs", func() {
			It("should preserve all metadata through enqueue and dequeue", func() {
				// Given metadata is attached to a message
				meta := map[string]string{"env": "test", "priority": "high"}

				// When the message is enqueued and then dequeued
				_, err := service.Enqueue(ctx, "meta-topic", []byte("data"), meta)
				Expect(err).NotTo(HaveOccurred())

				msg, err := service.Dequeue(ctx, "meta-topic")
				Expect(err).NotTo(HaveOccurred())

				// Then the metadata is identical to what was enqueued
				Expect(msg.Metadata).To(Equal(meta))
			})
		})
	})

	// Tests for the background expiry reaper
	Describe("StartExpiryReaper", func() {
		Context("Given a message that is already expired", func() {
			var messageID string

			BeforeEach(func() {
				// Insert an expired pending message directly.
				err := pool.QueryRow(ctx, `
					INSERT INTO messages (topic, payload, max_retries, expires_at)
					VALUES ('reaper-topic', 'stale', 3, now() - interval '1 second')
					RETURNING id
				`).Scan(&messageID)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should mark the expired message as 'expired' after the reaper fires", func() {
				reaperCtx, cancel := context.WithCancel(ctx)
				defer cancel()

				// Use a very short interval so the reaper fires quickly in the test.
				service.StartExpiryReaper(reaperCtx, 50*time.Millisecond)

				// Wait long enough for at least one reaper tick.
				Eventually(func() string {
					var msgStatus string
					_ = pool.QueryRow(ctx,
						`SELECT status FROM messages WHERE id = $1`, messageID,
					).Scan(&msgStatus)
					return msgStatus
				}, 2*time.Second, 100*time.Millisecond).Should(Equal("expired"))
			})
		})
	})
})
