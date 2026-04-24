package queue_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	suiteCtx    context.Context
	pgContainer *tcpostgres.PostgresContainer
	containerDSN string
)

func TestQueue(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Queue Suite")
}

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

	containerDSN = fmt.Sprintf(
		"postgres://postgres:postgres@%s:%d/queueti_test?sslmode=disable",
		host, mappedPort.Num(),
	)
})

var _ = AfterSuite(func() {
	if pgContainer != nil {
		_ = pgContainer.Terminate(suiteCtx)
	}
})
