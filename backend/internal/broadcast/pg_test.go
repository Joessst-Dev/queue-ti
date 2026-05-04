package broadcast_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Joessst-Dev/queue-ti/internal/broadcast"
)

var (
	suiteCtx       context.Context
	pgContainer    *tcpostgres.PostgresContainer
	containerDSN   string
	redisContainer *tcredis.RedisContainer
	redisAddr      string
)

func TestMain(m *testing.M) {
	suiteCtx = context.Background()

	var err error
	pgContainer, err = tcpostgres.Run(suiteCtx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("broadcast_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	host, err := pgContainer.Host(suiteCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container host: %v\n", err)
		os.Exit(1)
	}
	mappedPort, err := pgContainer.MappedPort(suiteCtx, "5432/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container port: %v\n", err)
		os.Exit(1)
	}
	containerDSN = fmt.Sprintf(
		"postgres://postgres:postgres@%s:%d/broadcast_test?sslmode=disable",
		host, mappedPort.Num(),
	)

	redisContainer, err = tcredis.Run(suiteCtx, "redis:7-alpine")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start redis container: %v\n", err)
		os.Exit(1)
	}
	redisAddr, err = redisContainer.ConnectionString(suiteCtx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get redis connection string: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	_ = pgContainer.Terminate(suiteCtx)
	_ = redisContainer.Terminate(suiteCtx)
	os.Exit(code)
}

func TestBroadcast(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Broadcast Suite")
}

var _ = BeforeSuite(func() {
	Expect(containerDSN).NotTo(BeEmpty())
})

var _ = AfterSuite(func() {
	// Container is terminated by TestMain.
})

var _ = Describe("PG broadcaster", func() {
	var (
		pool *pgxpool.Pool
		pg   *broadcast.PG
		ctx  context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		var err error
		pool, err = pgxpool.New(ctx, containerDSN)
		Expect(err).NotTo(HaveOccurred())

		pg = broadcast.NewPG(pool)
	})

	AfterEach(func() {
		Expect(pg.Close()).To(Succeed())
		pool.Close()
	})

	Describe("Publish then Subscribe", func() {
		Context("when a payload is published to a channel", func() {
			It("should be received by a subscriber on that channel", func() {
				ch, cancel := pg.Subscribe(ctx, broadcast.ChannelSchemaChanged)
				defer cancel()

				// Give the subscriber goroutine time to execute LISTEN before publishing.
				time.Sleep(50 * time.Millisecond)

				Expect(pg.Publish(ctx, broadcast.ChannelSchemaChanged, "orders")).To(Succeed())

				Eventually(ch, 3*time.Second).Should(Receive(Equal("orders")))
			})
		})
	})

	Describe("Subscribe receives multiple payloads", func() {
		Context("when three payloads are published in sequence", func() {
			It("should deliver all three in order", func() {
				ch, cancel := pg.Subscribe(ctx, broadcast.ChannelSchemaChanged)
				defer cancel()

				time.Sleep(50 * time.Millisecond)

				for _, topic := range []string{"alpha", "beta", "gamma"} {
					Expect(pg.Publish(ctx, broadcast.ChannelSchemaChanged, topic)).To(Succeed())
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
				ch, cancel := pg.Subscribe(ctx, broadcast.ChannelSchemaChanged)

				cancel()

				Eventually(ch, 3*time.Second).Should(BeClosed())
			})
		})
	})
})

var _ = Describe("Noop broadcaster", func() {
	var noop broadcast.Noop

	Describe("Publish", func() {
		Context("always", func() {
			It("should return nil", func() {
				err := noop.Publish(context.Background(), broadcast.ChannelSchemaChanged, "any")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Subscribe", func() {
		Context("when cancel is called", func() {
			It("should close the returned channel", func() {
				ch, cancel := noop.Subscribe(context.Background(), broadcast.ChannelSchemaChanged)

				cancel()

				Eventually(ch, 3*time.Second).Should(BeClosed())
			})
		})
	})
})
