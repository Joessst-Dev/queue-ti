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

var _ = Describe("ReplayTopic", Ordered, func() {
	var (
		pool    *pgxpool.Pool
		service *queue.Service
		ctx     context.Context
	)

	BeforeAll(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		err = db.Migrate(ctx, pool)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		if pool != nil {
			pool.Close()
		}
	})

	BeforeEach(func() {
		service = queue.NewService(pool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})

		_, err := pool.Exec(ctx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM message_log")
		Expect(err).NotTo(HaveOccurred())
		_, err = pool.Exec(ctx, "DELETE FROM topic_config")
		Expect(err).NotTo(HaveOccurred())
	})

	// seedArchivedMessage enqueues a message on topic, dequeues it, then acks it
	// so it lands in message_log (ack archives when topic is replayable).
	seedArchivedMessage := func(topic string, payload []byte) {
		id, err := service.Enqueue(ctx, topic, payload, nil, nil)
		Expect(err).NotTo(HaveOccurred())

		_, err = service.Dequeue(ctx, topic, 0)
		Expect(err).NotTo(HaveOccurred())

		err = service.Ack(ctx, id)
		Expect(err).NotTo(HaveOccurred())
	}

	Context("when the topic has no topic_config (not replayable)", func() {
		It("should return ErrTopicNotReplayable", func() {
			_, err := service.ReplayTopic(ctx, "no-config-topic", time.Time{})

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(queue.ErrTopicNotReplayable.Error())))
		})
	})

	Context("when the topic is configured as replayable with no TTL", func() {
		const topic = "replay-nottl-topic"

		BeforeEach(func() {
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:      topic,
				Replayable: true,
			})).To(Succeed())

			seedArchivedMessage(topic, []byte("msg-1"))
			seedArchivedMessage(topic, []byte("msg-2"))
		})

		It("should re-enqueue all archived messages and return the count", func() {
			n, err := service.ReplayTopic(ctx, topic, time.Time{})

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(2)))
		})

		It("should store replayed messages with expires_at = NULL", func() {
			_, err := service.ReplayTopic(ctx, topic, time.Time{})
			Expect(err).NotTo(HaveOccurred())

			rows, err := pool.Query(ctx,
				`SELECT expires_at FROM messages WHERE topic = $1`, topic,
			)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			var count int
			for rows.Next() {
				count++
				var expiresAt *time.Time
				Expect(rows.Scan(&expiresAt)).To(Succeed())
				Expect(expiresAt).To(BeNil(), "replayed message must have NULL expires_at when TTL is not set")
			}
			Expect(count).To(Equal(2))
		})
	})

	Context("when the topic is configured as replayable with MessageTTLSeconds = 300", func() {
		const topic = "replay-ttl-topic"
		const ttlSeconds = 300

		BeforeEach(func() {
			ttl := ttlSeconds
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:             topic,
				Replayable:        true,
				MessageTTLSeconds: &ttl,
			})).To(Succeed())

			seedArchivedMessage(topic, []byte("msg-a"))
		})

		It("should set expires_at to approximately now + TTL on replayed messages", func() {
			before := time.Now()
			n, err := service.ReplayTopic(ctx, topic, time.Time{})
			after := time.Now()

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(1)))

			var expiresAt *time.Time
			Expect(pool.QueryRow(ctx,
				`SELECT expires_at FROM messages WHERE topic = $1`, topic,
			).Scan(&expiresAt)).To(Succeed())

			Expect(expiresAt).NotTo(BeNil())
			Expect(*expiresAt).To(BeTemporally(">=", before.Add(ttlSeconds*time.Second-time.Second)))
			Expect(*expiresAt).To(BeTemporally("<=", after.Add(ttlSeconds*time.Second+time.Second)))
		})
	})

	Context("when the topic is configured with MessageTTLSeconds = 0 (explicit no-expiry) and a global TTL is set", func() {
		const topic = "replay-zerottl-topic"

		BeforeEach(func() {
			// Service has a global TTL of 1 hour; the topic override of 0 must win.
			service = queue.NewService(pool, 30*time.Second, 3, time.Hour, 3, false, queue.NoopRecorder{})

			noTTL := 0
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:             topic,
				Replayable:        true,
				MessageTTLSeconds: &noTTL,
			})).To(Succeed())

			seedArchivedMessage(topic, []byte("msg-b"))
		})

		It("should store replayed messages with expires_at = NULL, overriding the global TTL", func() {
			n, err := service.ReplayTopic(ctx, topic, time.Time{})
			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(1)))

			var expiresAt *time.Time
			Expect(pool.QueryRow(ctx,
				`SELECT expires_at FROM messages WHERE topic = $1`, topic,
			).Scan(&expiresAt)).To(Succeed())
			Expect(expiresAt).To(BeNil())
		})
	})

	Context("when fromTime is set and only messages acked after that time exist", func() {
		const topic = "replay-fromtime-topic"

		var recentMsgTime time.Time

		BeforeEach(func() {
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:      topic,
				Replayable: true,
			})).To(Succeed())

			// Insert an older archive row directly, bypassing ack, so we control acked_at.
			_, err := pool.Exec(ctx, `
				INSERT INTO message_log (id, topic, payload, max_retries, created_at, acked_at)
				VALUES (gen_random_uuid(), $1, 'old-msg', 3, now() - interval '2 hours', now() - interval '2 hours')
			`, topic)
			Expect(err).NotTo(HaveOccurred())

			recentMsgTime = time.Now().Add(-30 * time.Minute)
			seedArchivedMessage(topic, []byte("recent-msg"))
		})

		It("should only replay messages acked at or after fromTime", func() {
			n, err := service.ReplayTopic(ctx, topic, recentMsgTime)

			Expect(err).NotTo(HaveOccurred())
			Expect(n).To(Equal(int64(1)))

			var payload []byte
			Expect(pool.QueryRow(ctx,
				`SELECT payload FROM messages WHERE topic = $1`, topic,
			).Scan(&payload)).To(Succeed())
			Expect(payload).To(Equal([]byte("recent-msg")))
		})
	})

	Context("when fromTime is set and the topic has a TTL", func() {
		const topic = "replay-fromtime-ttl-topic"
		const ttlSeconds = 600

		BeforeEach(func() {
			ttl := ttlSeconds
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:             topic,
				Replayable:        true,
				MessageTTLSeconds: &ttl,
			})).To(Succeed())

			seedArchivedMessage(topic, []byte("msg"))
		})

		It("should set expires_at on replayed messages when using the fromTime branch", func() {
			from := time.Now().Add(-1 * time.Hour)
			before := time.Now()
			_, err := service.ReplayTopic(ctx, topic, from)
			after := time.Now()

			Expect(err).NotTo(HaveOccurred())

			var expiresAt *time.Time
			Expect(pool.QueryRow(ctx,
				`SELECT expires_at FROM messages WHERE topic = $1`, topic,
			).Scan(&expiresAt)).To(Succeed())

			Expect(expiresAt).NotTo(BeNil())
			Expect(*expiresAt).To(BeTemporally(">=", before.Add(ttlSeconds*time.Second-time.Second)))
			Expect(*expiresAt).To(BeTemporally("<=", after.Add(ttlSeconds*time.Second+time.Second)))
		})
	})

	Context("when a replay window is configured and fromTime predates it", func() {
		const topic = "replay-window-topic"

		BeforeEach(func() {
			window := 3600 // 1-hour window
			Expect(service.UpsertTopicConfig(ctx, queue.TopicConfig{
				Topic:               topic,
				Replayable:          true,
				ReplayWindowSeconds: &window,
			})).To(Succeed())
		})

		It("should return ErrReplayWindowTooOld", func() {
			tooOld := time.Now().Add(-2 * time.Hour)
			_, err := service.ReplayTopic(ctx, topic, tooOld)

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring(queue.ErrReplayWindowTooOld.Error())))
		})
	})
})
