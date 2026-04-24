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

		service = queue.NewService(pool, 30*time.Second)
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
})

