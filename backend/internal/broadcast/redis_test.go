package broadcast_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"

	"github.com/Joessst-Dev/queue-ti/internal/broadcast"
)

var _ = Describe("Redis broadcaster", func() {
	var (
		client *redis.Client
		rb     *broadcast.RedisBroadcaster
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		opts, err := redis.ParseURL(redisAddr)
		Expect(err).NotTo(HaveOccurred())
		client = redis.NewClient(opts)

		rb = broadcast.NewRedis(client)
	})

	AfterEach(func() {
		Expect(rb.Close()).To(Succeed())
		Expect(client.Close()).To(Succeed())
	})

	Describe("Publish then Subscribe", func() {
		Context("when a payload is published to a channel", func() {
			It("should be received by a subscriber on that channel", func() {
				ch, cancel := rb.Subscribe(ctx, broadcast.ChannelSchemaChanged)
				defer cancel()

				// Allow the subscriber goroutine to send SUBSCRIBE before publishing.
				time.Sleep(50 * time.Millisecond)

				Expect(rb.Publish(ctx, broadcast.ChannelSchemaChanged, "orders")).To(Succeed())

				Eventually(ch, 3*time.Second).Should(Receive(Equal("orders")))
			})
		})
	})

	Describe("Subscribe receives multiple payloads", func() {
		Context("when three payloads are published in sequence", func() {
			It("should deliver all three in order", func() {
				ch, cancel := rb.Subscribe(ctx, broadcast.ChannelSchemaChanged)
				defer cancel()

				time.Sleep(50 * time.Millisecond)

				for _, topic := range []string{"alpha", "beta", "gamma"} {
					Expect(rb.Publish(ctx, broadcast.ChannelSchemaChanged, topic)).To(Succeed())
				}

				Eventually(ch, 3*time.Second).Should(Receive(Equal("alpha")))
				Eventually(ch, 3*time.Second).Should(Receive(Equal("beta")))
				Eventually(ch, 3*time.Second).Should(Receive(Equal("gamma")))
			})
		})
	})

	Describe("Cancel stops the subscriber", func() {
		Context("when cancel is called", func() {
			It("should close the channel", func() {
				ch, cancel := rb.Subscribe(ctx, broadcast.ChannelSchemaChanged)

				cancel()

				Eventually(ch, 3*time.Second).Should(BeClosed())
			})
		})
	})

	Describe("Cross-instance delivery", func() {
		Context("when two separate broadcaster instances share the same Redis", func() {
			It("should deliver a publish from one to a subscriber on the other", func() {
				opts, err := redis.ParseURL(redisAddr)
				Expect(err).NotTo(HaveOccurred())
				client2 := redis.NewClient(opts)
				defer client2.Close()

				rb2 := broadcast.NewRedis(client2)
				defer rb2.Close()

				ch, cancel := rb2.Subscribe(ctx, broadcast.ChannelConfigChanged)
				defer cancel()

				time.Sleep(50 * time.Millisecond)

				Expect(rb.Publish(ctx, broadcast.ChannelConfigChanged, "my-topic")).To(Succeed())

				Eventually(ch, 3*time.Second).Should(Receive(Equal("my-topic")))
			})
		})
	})
})
