package queue_test

import (
	"context"
	"errors"
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
		_, err = pool.Exec(ctx, "DELETE FROM topic_config")
		Expect(err).NotTo(HaveOccurred())

		service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
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
				service = queue.NewService(pool, 30*time.Second, 5, 0, 3, false, queue.NoopRecorder{})
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
				service = queue.NewService(pool, 30*time.Second, 3, time.Hour, 3, false, queue.NoopRecorder{})
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
				service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
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
				msg, err := service.Dequeue(ctx, "test-topic", 0)

				// Then the first enqueued message is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(firstID))
				Expect(msg.Payload).To(Equal([]byte("first")))
			})
		})

		Context("Given an empty topic with no pending messages", func() {
			It("should return ErrNoMessage", func() {
				// When we attempt to dequeue from a topic with no messages
				_, err := service.Dequeue(ctx, "empty-topic", 0)

				// Then the specific no-message error is returned
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a single message that has already been dequeued", func() {
			BeforeEach(func() {
				// Given one message is enqueued and then dequeued
				_, err := service.Enqueue(ctx, "test-topic", []byte("only-one"), nil)
				Expect(err).NotTo(HaveOccurred())

				msg, err := service.Dequeue(ctx, "test-topic", 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())
			})

			It("should not deliver the same message twice", func() {
				// When we attempt to dequeue again
				_, err := service.Dequeue(ctx, "test-topic", 0)

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
				_, err := service.Dequeue(ctx, "exp-topic", 0)
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
				_, err := service.Dequeue(ctx, "exhausted-topic", 0)
				Expect(err).To(Equal(queue.ErrNoMessage))
			})
		})

		Context("Given a message with retries remaining and no expiry", func() {
			BeforeEach(func() {
				_, err := service.Enqueue(ctx, "available-topic", []byte("work"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the message normally", func() {
				msg, err := service.Dequeue(ctx, "available-topic", 0)
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

				_, err = service.Dequeue(ctx, "test-topic", 0)
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

				_, err = service.Dequeue(ctx, "nack-topic", 0)
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
				service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
				var err error
				messageID, err = service.Enqueue(ctx, "retry-nack-topic", []byte("retry"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "retry-nack-topic", 0)
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
				msg, err := service.Dequeue(ctx, "retry-nack-topic", 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(msg.ID).To(Equal(messageID))
			})
		})

		Context("Given a service with max_retries = 1 and a message on its first (and last) nack", func() {
			var messageID string

			BeforeEach(func() {
				service = queue.NewService(pool, 30*time.Second, 1, 0, 0, false, queue.NoopRecorder{})
				var err error
				messageID, err = service.Enqueue(ctx, "final-nack-topic", []byte("one-shot"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = service.Dequeue(ctx, "final-nack-topic", 0)
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
				_, err = service.Dequeue(ctx, "final-nack-topic", 0)
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

				msg, err := service.Dequeue(ctx, "meta-topic", 0)
				Expect(err).NotTo(HaveOccurred())

				// Then the metadata is identical to what was enqueued
				Expect(msg.Metadata).To(Equal(meta))
			})
		})
	})

	// Tests for the dead-letter queue feature
	Describe("DLQ", func() {

		// --- Enqueue guard ---

		Describe("Enqueue", func() {
			Context("when the topic ends with .dlq", func() {
				It("should return ErrReservedTopic", func() {
					_, err := service.Enqueue(ctx, "payments.dlq", []byte("payload"), nil)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring(queue.ErrReservedTopic.Error())))
				})
			})
		})

		// --- Nack → DLQ promotion ---

		Describe("Nack", func() {
			Context("when a service is configured with dlqThreshold = 2 and a message has been nacked enough times to exhaust the threshold", func() {
				var messageID string

				BeforeEach(func() {
					// dlqThreshold = 2, maxRetries = 5 so normal retries would
					// still be available — but DLQ fires first at count 2.
					service = queue.NewService(pool, 30*time.Second, 5, 0, 2, false, queue.NoopRecorder{})

					var err error
					messageID, err = service.Enqueue(ctx, "orders", []byte("process me"), nil)
					Expect(err).NotTo(HaveOccurred())

					// First nack: retry_count becomes 1, still below threshold (2).
					_, err = service.Dequeue(ctx, "orders", 0)
					Expect(err).NotTo(HaveOccurred())
					err = service.Nack(ctx, messageID, "transient")
					Expect(err).NotTo(HaveOccurred())

					// Second nack: retry_count becomes 2, equals threshold → DLQ promotion.
					_, err = service.Dequeue(ctx, "orders", 0)
					Expect(err).NotTo(HaveOccurred())
					err = service.Nack(ctx, messageID, "still failing")
					Expect(err).NotTo(HaveOccurred())
				})

				It("should move the message to <topic>.dlq with status pending and retry_count 0", func() {
					var topic, status, originalTopic string
					var retryCount, maxRetries int
					var dlqMovedAt *time.Time

					err := pool.QueryRow(ctx, `
						SELECT topic, status, retry_count, max_retries,
						       COALESCE(original_topic, ''), dlq_moved_at
						FROM messages WHERE id = $1
					`, messageID).Scan(&topic, &status, &retryCount, &maxRetries, &originalTopic, &dlqMovedAt)

					Expect(err).NotTo(HaveOccurred())
					Expect(topic).To(Equal("orders.dlq"))
					Expect(originalTopic).To(Equal("orders"))
					Expect(status).To(Equal("pending"))
					Expect(retryCount).To(Equal(0))
					Expect(maxRetries).To(Equal(0))
					Expect(dlqMovedAt).NotTo(BeNil())
				})

				It("should make the message dequeue-able on the DLQ topic", func() {
					msg, err := service.Dequeue(ctx, "orders.dlq", 0)

					Expect(err).NotTo(HaveOccurred())
					Expect(msg.ID).To(Equal(messageID))
					Expect(msg.Topic).To(Equal("orders.dlq"))
					Expect(msg.OriginalTopic).To(Equal("orders"))
				})

				It("should remove the message from the original topic", func() {
					_, err := service.Dequeue(ctx, "orders", 0)

					Expect(err).To(Equal(queue.ErrNoMessage))
				})
			})

			Context("when retry_count + 1 is still below dlqThreshold", func() {
				var messageID string

				BeforeEach(func() {
					// dlqThreshold = 3; after one nack retry_count = 1 < 3, no promotion.
					service = queue.NewService(pool, 30*time.Second, 5, 0, 3, false, queue.NoopRecorder{})

					var err error
					messageID, err = service.Enqueue(ctx, "invoices", []byte("work"), nil)
					Expect(err).NotTo(HaveOccurred())

					_, err = service.Dequeue(ctx, "invoices", 0)
					Expect(err).NotTo(HaveOccurred())

					err = service.Nack(ctx, messageID, "one failure")
					Expect(err).NotTo(HaveOccurred())
				})

				It("should keep the message on the original topic and not promote it to the DLQ", func() {
					var topic, originalTopic string
					err := pool.QueryRow(ctx,
						`SELECT topic, COALESCE(original_topic, '') FROM messages WHERE id = $1`, messageID,
					).Scan(&topic, &originalTopic)

					Expect(err).NotTo(HaveOccurred())
					Expect(topic).To(Equal("invoices"))
					Expect(originalTopic).To(BeEmpty())
				})
			})
		})

		// --- Requeue ---

		Describe("Requeue", func() {
			Context("when a DLQ message exists with an original_topic", func() {
				var messageID string

				BeforeEach(func() {
					// Promote a message to DLQ by exhausting the threshold.
					service = queue.NewService(pool, 30*time.Second, 5, 0, 1, false, queue.NoopRecorder{})

					var err error
					messageID, err = service.Enqueue(ctx, "shipments", []byte("ship it"), nil)
					Expect(err).NotTo(HaveOccurred())

					_, err = service.Dequeue(ctx, "shipments", 0)
					Expect(err).NotTo(HaveOccurred())

					err = service.Nack(ctx, messageID, "carrier down")
					Expect(err).NotTo(HaveOccurred())

					// Confirm it landed in the DLQ.
					var topic string
					err = pool.QueryRow(ctx, `SELECT topic FROM messages WHERE id = $1`, messageID).Scan(&topic)
					Expect(err).NotTo(HaveOccurred())
					Expect(topic).To(Equal("shipments.dlq"))
				})

				It("should move the message back to its original topic with status pending and cleared DLQ fields", func() {
					err := service.Requeue(ctx, messageID)
					Expect(err).NotTo(HaveOccurred())

					var topic, status string
					var originalTopic *string
					var dlqMovedAt *time.Time
					var retryCount int

					err = pool.QueryRow(ctx, `
						SELECT topic, status, original_topic, dlq_moved_at, retry_count
						FROM messages WHERE id = $1
					`, messageID).Scan(&topic, &status, &originalTopic, &dlqMovedAt, &retryCount)

					Expect(err).NotTo(HaveOccurred())
					Expect(topic).To(Equal("shipments"))
					Expect(status).To(Equal("pending"))
					Expect(originalTopic).To(BeNil())
					Expect(dlqMovedAt).To(BeNil())
					Expect(retryCount).To(Equal(0))
				})

				It("should make the requeued message dequeue-able on the original topic", func() {
					err := service.Requeue(ctx, messageID)
					Expect(err).NotTo(HaveOccurred())

					msg, err := service.Dequeue(ctx, "shipments", 0)
					Expect(err).NotTo(HaveOccurred())
					Expect(msg.ID).To(Equal(messageID))
				})
			})

			Context("when called on a message that has no original_topic (not a DLQ message)", func() {
				var messageID string

				BeforeEach(func() {
					var err error
					messageID, err = service.Enqueue(ctx, "regular-topic", []byte("normal"), nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return ErrNotDLQ", func() {
					err := service.Requeue(ctx, messageID)

					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(ContainSubstring(queue.ErrNotDLQ.Error())))
				})
			})
		})
	})

	// Tests for per-dequeue configurable visibility timeout
	Describe("Dequeue with custom visibility timeout", func() {
		Context("when visibility_timeout_seconds is 0 (use server default)", func() {
			It("should store visibility_timeout close to now() + server default", func() {
				// Use a 30-second server default (the suite default).
				id, err := service.Enqueue(ctx, "vt-default-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				before := time.Now()
				// When we dequeue with 0 (use server default of 30s)
				msg, err := service.Dequeue(ctx, "vt-default-topic", 0)
				after := time.Now()

				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())

				// Then visibility_timeout is approximately now + 30s
				var vt time.Time
				err = pool.QueryRow(ctx,
					`SELECT visibility_timeout FROM messages WHERE id = $1`, id,
				).Scan(&vt)
				Expect(err).NotTo(HaveOccurred())
				Expect(vt).To(BeTemporally(">=", before.Add(30*time.Second-time.Second)))
				Expect(vt).To(BeTemporally("<=", after.Add(30*time.Second+time.Second)))
			})
		})

		Context("when a custom visibility_timeout_seconds of 2 is provided", func() {
			It("should store visibility_timeout close to now() + 2s", func() {
				id, err := service.Enqueue(ctx, "vt-custom-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				before := time.Now()
				// When we dequeue with a 2-second custom timeout
				msg, err := service.Dequeue(ctx, "vt-custom-topic", 2*time.Second)
				after := time.Now()

				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())

				// Then visibility_timeout is approximately now + 2s, not now + 30s
				var vt time.Time
				err = pool.QueryRow(ctx,
					`SELECT visibility_timeout FROM messages WHERE id = $1`, id,
				).Scan(&vt)
				Expect(err).NotTo(HaveOccurred())
				Expect(vt).To(BeTemporally(">=", before.Add(2*time.Second-time.Second)))
				Expect(vt).To(BeTemporally("<=", after.Add(2*time.Second+time.Second)))

				// And crucially the custom timeout is much shorter than the server default (30s)
				Expect(vt).To(BeTemporally("<", time.Now().Add(30*time.Second)))
			})
		})

		Context("when visibility_timeout_seconds is negative", func() {
			It("should fall back to the server default and dequeue successfully", func() {
				id, err := service.Enqueue(ctx, "vt-negative-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				before := time.Now()
				// When we pass a negative duration (treated as "not set", falls back to server default)
				msg, err := service.Dequeue(ctx, "vt-negative-topic", -1*time.Second)
				after := time.Now()

				// Then the dequeue succeeds — the negative value is ignored
				Expect(err).NotTo(HaveOccurred())
				Expect(msg).NotTo(BeNil())

				// And visibility_timeout reflects the server default (30s), not -1s
				var vt time.Time
				err = pool.QueryRow(ctx,
					`SELECT visibility_timeout FROM messages WHERE id = $1`, id,
				).Scan(&vt)
				Expect(err).NotTo(HaveOccurred())
				Expect(vt).To(BeTemporally(">=", before.Add(30*time.Second-time.Second)))
				Expect(vt).To(BeTemporally("<=", after.Add(30*time.Second+time.Second)))
			})
		})
	})

	// Tests for per-topic configuration CRUD
	Describe("TopicConfig CRUD", func() {
		BeforeEach(func() {
			_, err := pool.Exec(ctx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when no row exists for a topic", func() {
			It("should return nil, nil from GetTopicConfig", func() {
				cfg, err := service.GetTopicConfig(ctx, "nonexistent-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeNil())
			})
		})

		Context("when a config is upserted", func() {
			It("should be retrievable via GetTopicConfig with the stored values", func() {
				maxRetries := 7
				ttl := 120
				depth := 50
				err := service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:             "cfg-topic",
					MaxRetries:        &maxRetries,
					MessageTTLSeconds: &ttl,
					MaxDepth:          &depth,
				})
				Expect(err).NotTo(HaveOccurred())

				cfg, err := service.GetTopicConfig(ctx, "cfg-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).NotTo(BeNil())
				Expect(cfg.Topic).To(Equal("cfg-topic"))
				Expect(*cfg.MaxRetries).To(Equal(7))
				Expect(*cfg.MessageTTLSeconds).To(Equal(120))
				Expect(*cfg.MaxDepth).To(Equal(50))
			})
		})

		Context("when UpsertTopicConfig is called twice for the same topic", func() {
			It("should overwrite the first values with the second", func() {
				first := 3
				second := 9
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "overwrite-topic",
					MaxRetries: &first,
				})).To(Succeed())

				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "overwrite-topic",
					MaxRetries: &second,
				})).To(Succeed())

				cfg, err := service.GetTopicConfig(ctx, "overwrite-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(*cfg.MaxRetries).To(Equal(9))
			})
		})

		Context("when DeleteTopicConfig is called on an existing topic", func() {
			It("should return nil and a subsequent Get should return nil", func() {
				maxRetries := 1
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "delete-topic",
					MaxRetries: &maxRetries,
				})).To(Succeed())

				Expect(service.DeleteTopicConfig(ctx, "delete-topic")).To(Succeed())

				cfg, err := service.GetTopicConfig(ctx, "delete-topic")
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg).To(BeNil())
			})
		})

		Context("when DeleteTopicConfig is called for a topic that does not exist", func() {
			It("should return ErrNotFound", func() {
				err := service.DeleteTopicConfig(ctx, "ghost-topic")
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, queue.ErrNotFound)).To(BeTrue())
			})
		})

		Context("when multiple configs exist", func() {
			It("should return all rows ordered by topic ASC from ListTopicConfigs", func() {
				r1 := 1
				r2 := 2
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{Topic: "zebra", MaxRetries: &r2})).To(Succeed())
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{Topic: "alpha", MaxRetries: &r1})).To(Succeed())

				configs, err := service.ListTopicConfigs(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(configs).To(HaveLen(2))
				Expect(configs[0].Topic).To(Equal("alpha"))
				Expect(configs[1].Topic).To(Equal("zebra"))
			})

			It("should return an empty slice (not nil) when no configs exist", func() {
				configs, err := service.ListTopicConfigs(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(configs).NotTo(BeNil())
				Expect(configs).To(BeEmpty())
			})
		})
	})

	// Tests for Enqueue respecting per-topic configuration
	Describe("Enqueue respects topic config", func() {
		BeforeEach(func() {
			_, err := pool.Exec(ctx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the topic has MaxRetries = 7 configured", func() {
			It("should store the message with max_retries = 7", func() {
				maxRetries := 7
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:      "topic-retries",
					MaxRetries: &maxRetries,
				})).To(Succeed())

				id, err := service.Enqueue(ctx, "topic-retries", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				var stored int
				Expect(pool.QueryRow(ctx, `SELECT max_retries FROM messages WHERE id = $1`, id).Scan(&stored)).To(Succeed())
				Expect(stored).To(Equal(7))
			})
		})

		Context("when the topic has MessageTTLSeconds = 60 configured", func() {
			It("should set expires_at to approximately now + 60s", func() {
				ttl := 60
				Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:             "topic-ttl",
					MessageTTLSeconds: &ttl,
				})).To(Succeed())

				before := time.Now()
				id, err := service.Enqueue(ctx, "topic-ttl", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())
				after := time.Now()

				var expiresAt *time.Time
				Expect(pool.QueryRow(ctx, `SELECT expires_at FROM messages WHERE id = $1`, id).Scan(&expiresAt)).To(Succeed())
				Expect(expiresAt).NotTo(BeNil())
				Expect(*expiresAt).To(BeTemporally(">=", before.Add(59*time.Second)))
				Expect(*expiresAt).To(BeTemporally("<=", after.Add(61*time.Second)))
			})
		})

		Context("when the topic has MessageTTLSeconds = 0 configured and the service has a global TTL", func() {
			It("should set expires_at = NULL overriding the global TTL", func() {
				ttlService := queue.NewService(pool, 30*time.Second, 3, time.Hour, 3, false, queue.NoopRecorder{})
				noTTL := 0
				Expect(ttlService.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:             "topic-nottl",
					MessageTTLSeconds: &noTTL,
				})).To(Succeed())

				id, err := ttlService.Enqueue(ctx, "topic-nottl", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				var expiresAt *time.Time
				Expect(pool.QueryRow(ctx, `SELECT expires_at FROM messages WHERE id = $1`, id).Scan(&expiresAt)).To(Succeed())
				Expect(expiresAt).To(BeNil())
			})
		})

		Context("when no topic_config row exists", func() {
			It("should fall back to the global service defaults", func() {
				globalService := queue.NewService(pool, 30*time.Second, 5, 0, 3, false, queue.NoopRecorder{})
				id, err := globalService.Enqueue(ctx, "topic-defaults", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())

				var maxRetries int
				var expiresAt *time.Time
				Expect(pool.QueryRow(ctx,
					`SELECT max_retries, expires_at FROM messages WHERE id = $1`, id,
				).Scan(&maxRetries, &expiresAt)).To(Succeed())
				Expect(maxRetries).To(Equal(5))
				Expect(expiresAt).To(BeNil())
			})
		})

		Context("when MaxDepth = 2 is configured for a topic", func() {
			var depthService *queue.Service

			BeforeEach(func() {
				depthService = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
				maxDepth := 2
				Expect(depthService.UpsertTopicConfig(ctx, queue.TopicConfig{
					Topic:    "topic-depth",
					MaxDepth: &maxDepth,
				})).To(Succeed())
			})

			It("should allow the first two enqueues but reject the third with ErrQueueFull", func() {
				_, err := depthService.Enqueue(ctx, "topic-depth", []byte("one"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("two"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("three"), nil)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, queue.ErrQueueFull)).To(BeTrue())
			})

			It("should allow a new enqueue after an ack reduces the depth below max", func() {
				id1, err := depthService.Enqueue(ctx, "topic-depth", []byte("one"), nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("two"), nil)
				Expect(err).NotTo(HaveOccurred())

				// Dequeue and ack the first message so depth drops to 1.
				_, err = depthService.Dequeue(ctx, "topic-depth", 0)
				Expect(err).NotTo(HaveOccurred())
				Expect(depthService.Ack(ctx, id1)).To(Succeed())

				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("three"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should count both pending AND processing messages toward the depth limit", func() {
				_, err := depthService.Enqueue(ctx, "topic-depth", []byte("one"), nil)
				Expect(err).NotTo(HaveOccurred())

				// Dequeue to move the first message to 'processing' — it still counts.
				_, err = depthService.Dequeue(ctx, "topic-depth", 0)
				Expect(err).NotTo(HaveOccurred())

				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("two"), nil)
				Expect(err).NotTo(HaveOccurred())

				// Depth is now 2 (1 processing + 1 pending); third enqueue must be rejected.
				_, err = depthService.Enqueue(ctx, "topic-depth", []byte("three"), nil)
				Expect(err).To(HaveOccurred())
				Expect(errors.Is(err, queue.ErrQueueFull)).To(BeTrue())
			})
		})
	})

	// Tests for topic registration enforcement
	Describe("topic registration enforcement", func() {
		BeforeEach(func() {
			_, err := pool.Exec(ctx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when require_topic_registration is false (default)", func() {
			It("should enqueue to any topic regardless of topic_config presence", func() {
				// Service with registration enforcement off — no topic_config row exists.
				id, err := service.Enqueue(ctx, "unregistered-topic", []byte("payload"), nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(id).NotTo(BeEmpty())
			})
		})

		Context("when require_topic_registration is true", func() {
			var strictService *queue.Service

			BeforeEach(func() {
				strictService = queue.NewService(pool, 30*time.Second, 3, 0, 3, true, queue.NoopRecorder{})
			})

			Context("and the topic has no topic_config row", func() {
				It("should return ErrTopicNotRegistered", func() {
					_, err := strictService.Enqueue(ctx, "unregistered-topic", []byte("payload"), nil)
					Expect(err).To(HaveOccurred())
					Expect(errors.Is(err, queue.ErrTopicNotRegistered)).To(BeTrue())
				})
			})

			Context("and the topic has a topic_config row", func() {
				BeforeEach(func() {
					// Register the topic by upserting a config row.
					Expect(strictService.UpsertTopicConfig(ctx, queue.TopicConfig{
						Topic: "registered-topic",
					})).To(Succeed())
				})

				It("should enqueue successfully", func() {
					id, err := strictService.Enqueue(ctx, "registered-topic", []byte("payload"), nil)
					Expect(err).NotTo(HaveOccurred())
					Expect(id).NotTo(BeEmpty())
				})
			})
		})
	})

	// Tests for purging messages from a topic by status
	Describe("PurgeTopic", func() {
		BeforeEach(func() {
			// Insert 3 pending + 2 expired messages on "purge-topic", and 1 pending on "other-topic"
			_, err := pool.Exec(ctx, `
				INSERT INTO messages (topic, payload, status, max_retries)
				VALUES
					('purge-topic', 'p1', 'pending', 3),
					('purge-topic', 'p2', 'pending', 3),
					('purge-topic', 'p3', 'pending', 3),
					('purge-topic', 'e1', 'expired', 3),
					('purge-topic', 'e2', 'expired', 3),
					('other-topic', 'o1', 'pending', 3)
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when purging only the pending status from the topic", func() {
			It("should delete 3 rows and leave the expired and other-topic rows intact", func() {
				n, err := service.PurgeTopic(ctx, "purge-topic", []string{"pending"})

				Expect(err).NotTo(HaveOccurred())
				Expect(n).To(Equal(int64(3)))

				// 2 expired rows on purge-topic must still exist
				var expiredCount int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE topic = 'purge-topic' AND status = 'expired'`,
				).Scan(&expiredCount)).To(Succeed())
				Expect(expiredCount).To(Equal(2))

				// The other-topic row must still exist
				var otherCount int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE topic = 'other-topic'`,
				).Scan(&otherCount)).To(Succeed())
				Expect(otherCount).To(Equal(1))
			})
		})

		Context("when purging both pending and expired statuses from the topic", func() {
			It("should delete all 5 rows belonging to that topic", func() {
				n, err := service.PurgeTopic(ctx, "purge-topic", []string{"pending", "expired"})

				Expect(err).NotTo(HaveOccurred())
				Expect(n).To(Equal(int64(5)))

				var remaining int
				Expect(pool.QueryRow(ctx,
					`SELECT count(*) FROM messages WHERE topic = 'purge-topic'`,
				).Scan(&remaining)).To(Succeed())
				Expect(remaining).To(Equal(0))
			})
		})

		Context("when the topic does not exist", func() {
			It("should return 0 without an error", func() {
				n, err := service.PurgeTopic(ctx, "ghost", []string{"pending"})

				Expect(err).NotTo(HaveOccurred())
				Expect(n).To(Equal(int64(0)))
			})
		})
	})

	// Tests for the manual expiry reaper
	Describe("RunExpiryReaperOnce", func() {
		var pastExpiryID string

		BeforeEach(func() {
			// A pending message whose expires_at is already in the past — must be expired
			err := pool.QueryRow(ctx, `
				INSERT INTO messages (topic, payload, max_retries, expires_at)
				VALUES ('reaper-expiry-topic', 'past', 3, now() - interval '1 minute')
				RETURNING id
			`).Scan(&pastExpiryID)
			Expect(err).NotTo(HaveOccurred())

			// A pending message whose expires_at is well in the future — must be left alone
			_, err = pool.Exec(ctx, `
				INSERT INTO messages (topic, payload, max_retries, expires_at)
				VALUES ('reaper-expiry-topic', 'future', 3, now() + interval '1 hour')
			`)
			Expect(err).NotTo(HaveOccurred())

			// A pending message with no expires_at — must never be touched by the reaper
			_, err = pool.Exec(ctx, `
				INSERT INTO messages (topic, payload, max_retries)
				VALUES ('reaper-expiry-topic', 'no-expiry', 3)
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return 1 — only the already-past message is transitioned", func() {
			n, err := service.RunExpiryReaperOnce(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(1)))
		})

		It("should mark only the past-expiry message as expired", func() {
			_, err := service.RunExpiryReaperOnce(ctx)
			Expect(err).NotTo(HaveOccurred())

			var status string
			Expect(pool.QueryRow(ctx,
				`SELECT status FROM messages WHERE id = $1`, pastExpiryID,
			).Scan(&status)).To(Succeed())
			Expect(status).To(Equal("expired"))
		})

		It("should leave the future-expiry and no-expiry messages as pending", func() {
			_, err := service.RunExpiryReaperOnce(ctx)
			Expect(err).NotTo(HaveOccurred())

			var pendingCount int
			Expect(pool.QueryRow(ctx, `
				SELECT count(*) FROM messages
				WHERE topic = 'reaper-expiry-topic' AND status = 'pending'
			`).Scan(&pendingCount)).To(Succeed())
			Expect(pendingCount).To(Equal(2))
		})
	})

	// Tests for the manual delete reaper
	Describe("RunDeleteReaperOnce", func() {
		var expiredID, pendingID string

		BeforeEach(func() {
			err := pool.QueryRow(ctx, `
				INSERT INTO messages (topic, payload, status, max_retries)
				VALUES ('reaper-delete-topic', 'expired-msg', 'expired', 3)
				RETURNING id
			`).Scan(&expiredID)
			Expect(err).NotTo(HaveOccurred())

			err = pool.QueryRow(ctx, `
				INSERT INTO messages (topic, payload, max_retries)
				VALUES ('reaper-delete-topic', 'pending-msg', 3)
				RETURNING id
			`).Scan(&pendingID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return 1 and permanently remove only the expired row", func() {
			n, err := service.RunDeleteReaperOnce(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(1)))

			var count int
			Expect(pool.QueryRow(ctx,
				`SELECT count(*) FROM messages WHERE id = $1`, expiredID,
			).Scan(&count)).To(Succeed())
			Expect(count).To(Equal(0))
		})

		It("should leave the pending row intact", func() {
			_, err := service.RunDeleteReaperOnce(ctx)
			Expect(err).NotTo(HaveOccurred())

			var status string
			Expect(pool.QueryRow(ctx,
				`SELECT status FROM messages WHERE id = $1`, pendingID,
			).Scan(&status)).To(Succeed())
			Expect(status).To(Equal("pending"))
		})

		It("should return 0 when called again — the operation is idempotent", func() {
			_, err := service.RunDeleteReaperOnce(ctx)
			Expect(err).NotTo(HaveOccurred())

			n, err := service.RunDeleteReaperOnce(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(0)))
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
