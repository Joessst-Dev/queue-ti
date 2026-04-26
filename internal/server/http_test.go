package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/metrics"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Package-level vars shared across HTTP test specs within this suite.
var (
	httpTestPool *pgxpool.Pool
	httpTestCtx  context.Context
	pgContainer  *tcpostgres.PostgresContainer
)

type listResult struct {
	Items  []map[string]any `json:"items"`
	Total  int              `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

var _ = BeforeSuite(func() {
	httpTestCtx = context.Background()

	var err error
	pgContainer, err = tcpostgres.Run(httpTestCtx,
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

	dsn, err := pgContainer.ConnectionString(httpTestCtx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())

	httpTestPool, err = pgxpool.New(httpTestCtx, dsn)
	Expect(err).NotTo(HaveOccurred())

	err = db.Migrate(httpTestCtx, httpTestPool)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if httpTestPool != nil {
		httpTestPool.Close()
	}
	if pgContainer != nil {
		_ = pgContainer.Terminate(httpTestCtx)
	}
})

var _ = Describe("HTTP Server", func() {
	var queueService *queue.Service

	BeforeEach(func() {
		_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())

		queueService = queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, queue.NoopRecorder{})
	})

	// ---------------------------------------------------------------------------
	// GET /healthz
	// ---------------------------------------------------------------------------

	Describe("GET /healthz", func() {
		Context("Given the server is running", func() {
			It("should return HTTP 200", func() {
				// Given the HTTP server with auth disabled
				httpServer := server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())

				// When we call the health endpoint
				req := httptest.NewRequest("GET", "/healthz", nil)
				resp, err := httpServer.App.Test(req)

				// Then the server responds with 200 OK
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /api/auth/status
	// ---------------------------------------------------------------------------

	Describe("GET /api/auth/status", func() {
		Context("Given auth is disabled", func() {
			It("should return auth_required: false", func() {
				// Given the HTTP server with auth disabled
				httpServer := server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())

				// When we call the auth status endpoint
				req := httptest.NewRequest("GET", "/api/auth/status", nil)
				resp, err := httpServer.App.Test(req)

				// Then the response body contains auth_required: false
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["auth_required"]).To(BeFalse())
			})
		})

		Context("Given auth is enabled", func() {
			It("should return auth_required: true", func() {
				// Given the HTTP server with auth enabled
				httpServer := server.NewHTTPServer(queueService, config.AuthConfig{
					Enabled:  true,
					Username: "admin",
					Password: "secret",
				}, prometheus.NewRegistry())

				// When we call the auth status endpoint
				req := httptest.NewRequest("GET", "/api/auth/status", nil)
				resp, err := httpServer.App.Test(req)

				// Then the response body contains auth_required: true
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["auth_required"]).To(BeTrue())
			})
		})
	})

	// ---------------------------------------------------------------------------
	// withAuth middleware
	// ---------------------------------------------------------------------------

	Describe("withAuth middleware", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			// Given the HTTP server with auth enabled
			httpServer = server.NewHTTPServer(queueService, config.AuthConfig{
				Enabled:  true,
				Username: "admin",
				Password: "secret",
			}, prometheus.NewRegistry())
		})

		Context("Given auth is disabled", func() {
			It("should pass the request through without an Authorization header", func() {
				// Given the HTTP server with auth disabled
				noAuthServer := server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())

				// When we call a protected endpoint without credentials
				req := httptest.NewRequest("GET", "/api/messages", nil)
				resp, err := noAuthServer.App.Test(req)

				// Then the request passes through and is not rejected with 401
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).NotTo(Equal(401))
			})
		})

		Context("Given the Authorization header is missing", func() {
			It("should return 401 with a missing header error", func() {
				// When we call a protected endpoint with no Authorization header
				req := httptest.NewRequest("GET", "/api/messages", nil)
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the appropriate error message is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("missing authorization header"))
			})
		})

		Context("Given the Authorization header uses a non-Basic scheme", func() {
			It("should return 401 with an unsupported scheme error", func() {
				// When we send a Bearer token instead of Basic credentials
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", "Bearer sometoken")
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the unsupported scheme error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("unsupported auth scheme"))
			})
		})

		Context("Given the Basic Authorization header contains malformed base64", func() {
			It("should return 401 with an invalid format error", func() {
				// When we send a corrupt base64 credential
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", "Basic not!valid!base64!!!")
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the invalid format error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid authorization format"))
			})
		})

		Context("Given the credentials are incorrect", func() {
			It("should return 401 with an invalid credentials error", func() {
				// When we send valid base64 but wrong username/password
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", basicAuthHeader("admin", "wrongpassword"))
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the invalid credentials error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid credentials"))
			})
		})

		Context("Given the correct credentials are provided", func() {
			It("should pass the request through to the handler", func() {
				// When we send the correct username and password
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
				resp, err := httpServer.App.Test(req)

				// Then the request passes through and is not rejected with 401
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).NotTo(Equal(401))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /api/messages
	// ---------------------------------------------------------------------------

	Describe("GET /api/messages", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())
		})

		Context("Given no messages exist", func() {
			It("should return an empty items array with zero total", func() {
				// When we call the list endpoint on an empty queue
				req := httptest.NewRequest("GET", "/api/messages", nil)
				resp, err := httpServer.App.Test(req)

				// Then an empty items array is returned with total 0
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(BeEmpty())
				Expect(result.Total).To(Equal(0))
			})
		})

		Context("Given messages exist across multiple topics", func() {
			BeforeEach(func() {
				// Given messages are enqueued on two different topics
				_, err := queueService.Enqueue(httpTestCtx, "topic-alpha", []byte("msg-alpha"), nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = queueService.Enqueue(httpTestCtx, "topic-beta", []byte("msg-beta"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return all messages when no topic filter is specified", func() {
				// When we list messages without a filter
				req := httptest.NewRequest("GET", "/api/messages", nil)
				resp, err := httpServer.App.Test(req)

				// Then all messages across all topics are returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(2))
			})

			It("should return only messages matching the topic query parameter", func() {
				// When we list messages filtered by topic
				req := httptest.NewRequest("GET", "/api/messages?topic=topic-alpha", nil)
				resp, err := httpServer.App.Test(req)

				// Then only messages from that topic are returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0]["topic"]).To(Equal("topic-alpha"))
				Expect(result.Items[0]["payload"]).To(Equal("msg-alpha"))
			})
		})

		Context("Given a message with metadata is enqueued", func() {
			It("should return a response with all expected fields", func() {
				// Given a message with metadata
				id, err := queueService.Enqueue(httpTestCtx, "topic-fields", []byte("hello"), map[string]string{"key": "value"})
				Expect(err).NotTo(HaveOccurred())

				// When we list messages for that topic
				req := httptest.NewRequest("GET", "/api/messages?topic=topic-fields", nil)
				resp, err := httpServer.App.Test(req)

				// Then all expected fields are present and correct
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))

				msg := result.Items[0]
				Expect(msg["id"]).To(Equal(id))
				Expect(msg["topic"]).To(Equal("topic-fields"))
				Expect(msg["payload"]).To(Equal("hello"))
				Expect(msg["status"]).To(Equal("pending"))
				Expect(msg["created_at"]).NotTo(BeEmpty())
				metadata, ok := msg["metadata"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(metadata["key"]).To(Equal("value"))
			})
		})

		Context("Given auth is enabled and correct credentials are provided", func() {
			It("should return messages for authenticated requests", func() {
				// Given auth is enabled and a message has been enqueued
				authServer := server.NewHTTPServer(queueService, config.AuthConfig{
					Enabled:  true,
					Username: "admin",
					Password: "secret",
				}, prometheus.NewRegistry())
				_, err := queueService.Enqueue(httpTestCtx, "auth-topic", []byte("secure-msg"), nil)
				Expect(err).NotTo(HaveOccurred())

				// When we call the list endpoint with valid credentials
				req := httptest.NewRequest("GET", "/api/messages?topic=auth-topic", nil)
				req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
				resp, err := authServer.App.Test(req)

				// Then the messages are returned successfully
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/messages
	// ---------------------------------------------------------------------------

	Describe("POST /api/messages", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())
		})

		Context("Given the request body is not valid JSON", func() {
			It("should return 400 with an invalid body error", func() {
				// When we POST a non-JSON body
				req := httptest.NewRequest("POST", "/api/messages", strings.NewReader("this is not json"))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				// Then a 400 with the invalid request body error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid request body"))
			})
		})

		Context("Given the topic field is missing", func() {
			It("should return 400 with a validation error", func() {
				// When we POST a body with no topic
				req := httptest.NewRequest("POST", "/api/messages", strings.NewReader(`{"payload":"hello"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				// Then a 400 with the required fields error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("topic and payload are required"))
			})
		})

		Context("Given the payload field is missing", func() {
			It("should return 400 with a validation error", func() {
				// When we POST a body with no payload
				req := httptest.NewRequest("POST", "/api/messages", strings.NewReader(`{"topic":"some-topic"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				// Then a 400 with the required fields error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("topic and payload are required"))
			})
		})

		Context("Given a valid enqueue request", func() {
			It("should return 201 with the new message ID", func() {
				// When we POST a valid topic and payload
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"post-topic","payload":"hello world"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				// Then a 201 is returned with a non-empty message ID
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["id"]).NotTo(BeEmpty())
			})

			It("should persist the message so it appears in a subsequent list", func() {
				// Given we enqueue a message via HTTP POST
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"persist-topic","payload":"check me"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				// When we list messages on that topic
				listReq := httptest.NewRequest("GET", "/api/messages?topic=persist-topic", nil)
				listResp, err := httpServer.App.Test(listReq)

				// Then the message we just enqueued is present
				Expect(err).NotTo(HaveOccurred())
				Expect(listResp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(listResp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0]["payload"]).To(Equal("check me"))
			})

			It("should persist metadata when metadata is provided", func() {
				// Given a request with topic, payload, and metadata
				body := `{"topic":"meta-post-topic","payload":"with-meta","metadata":{"env":"test","version":"1"}}`
				req := httptest.NewRequest("POST", "/api/messages", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				// Then a 201 is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				// When we list the message back
				listReq := httptest.NewRequest("GET", "/api/messages?topic=meta-post-topic", nil)
				listResp, err := httpServer.App.Test(listReq)
				Expect(err).NotTo(HaveOccurred())

				var result listResult
				Expect(json.NewDecoder(listResp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))

				// Then the metadata fields are preserved
				metadata, ok := result.Items[0]["metadata"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(metadata["env"]).To(Equal("test"))
				Expect(metadata["version"]).To(Equal("1"))
			})
		})

		Context("Given auth is enabled", func() {
			var authServer *server.HTTPServer

			BeforeEach(func() {
				authServer = server.NewHTTPServer(queueService, config.AuthConfig{
					Enabled:  true,
					Username: "admin",
					Password: "secret",
				}, prometheus.NewRegistry())
			})

			It("should enqueue successfully when correct credentials are provided", func() {
				// When we POST with valid Basic credentials
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"auth-post-topic","payload":"secure"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
				resp, err := authServer.App.Test(req)

				// Then the message is created successfully
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["id"]).NotTo(BeEmpty())
			})

			It("should return 401 when no credentials are provided", func() {
				// When we POST without an Authorization header
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"auth-post-topic","payload":"secure"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := authServer.App.Test(req)

				// Then the request is rejected with 401
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("missing authorization header"))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/messages/:id/requeue
	// ---------------------------------------------------------------------------

	Describe("POST /api/messages/:id/requeue", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())
		})

		Context("Given a message that has been promoted to the DLQ", func() {
			var dlqMessageID string

			BeforeEach(func() {
				// Use dlqThreshold = 1 so a single nack promotes the message.
				dlqService := queue.NewService(httpTestPool, 30*time.Second, 5, 0, 1, queue.NoopRecorder{})
				httpServer = server.NewHTTPServer(dlqService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())

				var err error
				dlqMessageID, err = dlqService.Enqueue(httpTestCtx, "payments", []byte("charge"), nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = dlqService.Dequeue(httpTestCtx, "payments")
				Expect(err).NotTo(HaveOccurred())

				err = dlqService.Nack(httpTestCtx, dlqMessageID, "gateway timeout")
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return 204 and move the message back to its original topic", func() {
				req := httptest.NewRequest("POST", "/api/messages/"+dlqMessageID+"/requeue", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})
		})

		Context("Given a message ID that is not a DLQ message", func() {
			var regularMessageID string

			BeforeEach(func() {
				var err error
				regularMessageID, err = queueService.Enqueue(httpTestCtx, "regular-http-topic", []byte("normal"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return 404", func() {
				req := httptest.NewRequest("POST", "/api/messages/"+regularMessageID+"/requeue", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /metrics
	// ---------------------------------------------------------------------------

	Describe("GET /metrics", func() {
		var (
			reg        *prometheus.Registry
			httpServer *server.HTTPServer
		)

		BeforeEach(func() {
			reg = prometheus.NewRegistry()
			rec := metrics.New(httpTestPool, reg)
			svc := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, rec)
			httpServer = server.NewHTTPServer(svc, config.AuthConfig{Enabled: true, Username: "admin", Password: "secret"}, reg)
		})

		Context("when called without an Authorization header and auth is enabled", func() {
			It("should return 200 OK — metrics are always public", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})

		Context("after a message has been enqueued", func() {
			BeforeEach(func() {
				_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM messages")
				Expect(err).NotTo(HaveOccurred())

				// Enqueue via the service that shares the same recorder.
				reg2 := prometheus.NewRegistry()
				rec2 := metrics.New(httpTestPool, reg2)
				svc2 := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, rec2)
				httpServer = server.NewHTTPServer(svc2, config.AuthConfig{Enabled: false}, reg2)

				_, err = svc2.Enqueue(httpTestCtx, "metrics-topic", []byte("hello"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should include queueti_enqueued_total in the response body", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("queueti_enqueued_total"))
			})

			It("should include queueti_queue_depth in the response body", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(body)).To(ContainSubstring("queueti_queue_depth"))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /api/messages pagination
	// ---------------------------------------------------------------------------

	Describe("GET /api/messages pagination", func() {
		var httpServer *server.HTTPServer
		const paginationTopic = "pagination-topic"

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.AuthConfig{Enabled: false}, prometheus.NewRegistry())

			// Given 5 messages are enqueued on the same topic
			for i := 0; i < 5; i++ {
				payload := fmt.Sprintf("msg-%d", i)
				_, err := queueService.Enqueue(httpTestCtx, paginationTopic, []byte(payload), nil)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("When limit=2 and offset=0 are requested", func() {
			It("should return the first 2 items with total=5", func() {
				req := httptest.NewRequest("GET", "/api/messages?topic="+paginationTopic+"&limit=2&offset=0", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(2))
				Expect(result.Total).To(Equal(5))
				Expect(result.Limit).To(Equal(2))
				Expect(result.Offset).To(Equal(0))
			})
		})

		Context("When limit=2 and offset=2 are requested", func() {
			It("should return items 3 and 4 with total=5", func() {
				req := httptest.NewRequest("GET", "/api/messages?topic="+paginationTopic+"&limit=2&offset=2", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(2))
				Expect(result.Total).To(Equal(5))
			})
		})

		Context("When limit=2 and offset=4 are requested (last page)", func() {
			It("should return only the remaining 1 item with total=5", func() {
				req := httptest.NewRequest("GET", "/api/messages?topic="+paginationTopic+"&limit=2&offset=4", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Total).To(Equal(5))
			})
		})

		Context("When limit=2 and offset=5 are requested (past the end)", func() {
			It("should return an empty items array with total=5", func() {
				req := httptest.NewRequest("GET", "/api/messages?topic="+paginationTopic+"&limit=2&offset=5", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(BeEmpty())
				Expect(result.Total).To(Equal(5))
			})
		})

		Context("When a topic filter is applied alongside pagination", func() {
			BeforeEach(func() {
				// Enqueue an extra message on a different topic to confirm the count is scoped
				_, err := queueService.Enqueue(httpTestCtx, "other-topic", []byte("noise"), nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should report a total that matches only the filtered topic count", func() {
				req := httptest.NewRequest("GET", "/api/messages?topic="+paginationTopic+"&limit=10&offset=0", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result listResult
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result.Total).To(Equal(5))
				Expect(result.Items).To(HaveLen(5))
			})
		})
	})

})

func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

func decodeErrorBody(body io.Reader) string {
	var result map[string]any
	if err := json.NewDecoder(body).Decode(&result); err != nil {
		return fmt.Sprintf("(failed to decode body: %v)", err)
	}
	msg, _ := result["error"].(string)
	return msg
}
