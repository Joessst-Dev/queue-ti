package queue_test

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
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	suiteCtx     context.Context
	pgContainer  *tcpostgres.PostgresContainer
	containerDSN string
)

func TestMain(m *testing.M) {
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
		"postgres://postgres:postgres@%s:%d/queueti_test?sslmode=disable",
		host, mappedPort.Num(),
	)

	code := m.Run()

	_ = pgContainer.Terminate(suiteCtx)
	os.Exit(code)
}

func TestQueue(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Queue Suite")
}

var _ = BeforeSuite(func() {
	// Container is already running — nothing to do.
	Expect(containerDSN).NotTo(BeEmpty())
})

var _ = AfterSuite(func() {
	// Container is terminated by TestMain.
})
