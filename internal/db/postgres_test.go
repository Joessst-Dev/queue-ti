package db_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	suiteCtx    context.Context
	pgContainer *tcpostgres.PostgresContainer
	containerDB config.DBConfig
)

var _ = BeforeSuite(func() {
	suiteCtx = context.Background()

	var err error
	pgContainer, err = tcpostgres.Run(suiteCtx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("queueti_test"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	Expect(err).NotTo(HaveOccurred())

	host, err := pgContainer.Host(suiteCtx)
	Expect(err).NotTo(HaveOccurred())

	mappedPort, err := pgContainer.MappedPort(suiteCtx, "5432/tcp")
	Expect(err).NotTo(HaveOccurred())

	containerDB = config.DBConfig{
		Host:     host,
		Port:     int(mappedPort.Num()),
		User:     "postgres",
		Password: "postgres",
		Name:     "queueti_test",
		SSLMode:  "disable",
	}
})

var _ = AfterSuite(func() {
	if pgContainer != nil {
		_ = pgContainer.Terminate(suiteCtx)
	}
})

var _ = Describe("Connect", func() {
	Context("Given a valid database config", func() {
		It("should return a usable connection pool", func() {
			// When we connect with correct credentials
			pool, err := db.Connect(suiteCtx, containerDB)

			// Then no error occurs and the pool is ready to use
			Expect(err).NotTo(HaveOccurred())
			Expect(pool).NotTo(BeNil())
			pool.Close()
		})

		It("should return a pool that can execute queries", func() {
			// When we connect and run a trivial query
			pool, err := db.Connect(suiteCtx, containerDB)
			Expect(err).NotTo(HaveOccurred())
			defer pool.Close()

			var result int
			err = pool.QueryRow(suiteCtx, "SELECT 1").Scan(&result)

			// Then the query succeeds
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(1))
		})
	})

	Context("Given an unreachable database host", func() {
		It("should return an error wrapping the ping failure", func() {
			// Given a config pointing to a port that nothing is listening on
			badCfg := config.DBConfig{
				Host:     "127.0.0.1",
				Port:     1, // nothing listens here
				User:     "postgres",
				Password: "postgres",
				Name:     "queueti_test",
				SSLMode:  "disable",
			}

			// When we attempt to connect
			ctx, cancel := context.WithTimeout(suiteCtx, 5*time.Second)
			defer cancel()

			_, err := db.Connect(ctx, badCfg)

			// Then an error is returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to ping database"))
		})
	})

	Context("Given incorrect database credentials", func() {
		It("should return an error wrapping the ping failure", func() {
			// Given a config with the right host but wrong password
			badCfg := containerDB
			badCfg.Password = "wrongpassword"

			// When we attempt to connect
			ctx, cancel := context.WithTimeout(suiteCtx, 5*time.Second)
			defer cancel()

			_, err := db.Connect(ctx, badCfg)

			// Then an error is returned
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to ping database"))
		})
	})
})

var _ = Describe("Migrate", func() {
	var pool *pgxpool.Pool

	BeforeEach(func() {
		var err error
		pool, err = db.Connect(suiteCtx, containerDB)
		Expect(err).NotTo(HaveOccurred())

		// Start from a clean schema so each spec is independent
		_, err = pool.Exec(suiteCtx, `DROP TABLE IF EXISTS messages, schema_migrations`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
		}
	})

	Context("Given a fresh database with no migrations applied", func() {
		It("should apply migrations without error", func() {
			// When we run Migrate
			err := db.Migrate(suiteCtx, pool)

			// Then no error occurs
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the messages table", func() {
			// When we run Migrate
			Expect(db.Migrate(suiteCtx, pool)).To(Succeed())

			// Then the messages table exists
			var exists bool
			err := pool.QueryRow(suiteCtx, `
				SELECT EXISTS (
					SELECT FROM information_schema.tables
					WHERE table_schema = 'public' AND table_name = 'messages'
				)
			`).Scan(&exists)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("should create the messages table with the expected columns", func() {
			// When we run Migrate
			Expect(db.Migrate(suiteCtx, pool)).To(Succeed())

			// Then the expected columns are present
			rows, err := pool.Query(suiteCtx, `
				SELECT column_name
				FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = 'messages'
				ORDER BY column_name
			`)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()

			var columns []string
			for rows.Next() {
				var col string
				Expect(rows.Scan(&col)).To(Succeed())
				columns = append(columns, col)
			}

			Expect(columns).To(ConsistOf(
				"created_at",
				"id",
				"metadata",
				"payload",
				"status",
				"topic",
				"updated_at",
				"visibility_timeout",
			))
		})

		It("should create the dequeue index on messages", func() {
			// When we run Migrate
			Expect(db.Migrate(suiteCtx, pool)).To(Succeed())

			// Then the dequeue index exists
			var exists bool
			err := pool.QueryRow(suiteCtx, `
				SELECT EXISTS (
					SELECT FROM pg_indexes
					WHERE schemaname = 'public'
					  AND tablename  = 'messages'
					  AND indexname  = 'idx_messages_dequeue'
				)
			`).Scan(&exists)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue())
		})
	})

	Context("Given migrations have already been applied", func() {
		It("should be idempotent and return no error", func() {
			// Given Migrate has already run once
			Expect(db.Migrate(suiteCtx, pool)).To(Succeed())

			// When we run Migrate a second time
			err := db.Migrate(suiteCtx, pool)

			// Then no error is returned (ErrNoChange is swallowed internally)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
