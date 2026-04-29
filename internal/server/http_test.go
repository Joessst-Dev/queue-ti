package server_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/db"
	"github.com/Joessst-Dev/queue-ti/internal/metrics"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/server"
	"github.com/Joessst-Dev/queue-ti/internal/users"
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

// testJWTSecret is a fixed secret used across all JWT-related HTTP tests.
const testJWTSecret = "http-test-jwt-secret"

var _ = Describe("HTTP Server", func() {
	var (
		queueService *queue.Service
		userStore    *users.Store
	)

	BeforeEach(func() {
		_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM user_grants")
		Expect(err).NotTo(HaveOccurred())
		_, err = httpTestPool.Exec(httpTestCtx, "DELETE FROM users")
		Expect(err).NotTo(HaveOccurred())
		_, err = httpTestPool.Exec(httpTestCtx, "DELETE FROM messages")
		Expect(err).NotTo(HaveOccurred())

		queueService = queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, false, queue.NoopRecorder{})
		userStore = users.NewStore(httpTestPool)
	})

	// ---------------------------------------------------------------------------
	// GET /healthz
	// ---------------------------------------------------------------------------

	Describe("GET /healthz", func() {
		Context("Given the server is running", func() {
			It("should return HTTP 200", func() {
				// Given the HTTP server with auth disabled
				httpServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

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
	// GET /api/version
	// ---------------------------------------------------------------------------

	Describe("GET /api/version", func() {
		Context("Given the server is running", func() {
			It("should return the version string injected at construction", func() {
				httpServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "v1.2.3")

				req := httptest.NewRequest("GET", "/api/version", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]string
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["version"]).To(Equal("v1.2.3"))
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
				httpServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

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
				httpServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
					Enabled:   true,
					JWTSecret: testJWTSecret,
				}, prometheus.NewRegistry(), userStore, "dev")

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
	// JWT middleware (replaces old Basic-auth withAuth tests)
	// ---------------------------------------------------------------------------

	Describe("JWT middleware", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("Given auth is disabled", func() {
			It("should pass the request through without an Authorization header", func() {
				// Given the HTTP server with auth disabled
				noAuthServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

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

		Context("Given the Authorization header uses a non-Bearer scheme", func() {
			It("should return 401 with an unsupported scheme error", func() {
				// When we send a Basic header instead of Bearer
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", basicAuthHeader("admin", "secret"))
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the unsupported scheme error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("unsupported auth scheme"))
			})
		})

		Context("Given the Bearer token is invalid", func() {
			It("should return 401 with an invalid token error", func() {
				// When we send a garbage Bearer token
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", "Bearer not.a.real.token")
				resp, err := httpServer.App.Test(req)

				// Then a 401 with the invalid token error is returned
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid or expired token"))
			})
		})

		Context("Given a valid Bearer token is provided", func() {
			It("should pass the request through to the handler", func() {
				// Seed an admin user and get a valid token.
				admin, err := userStore.Create(httpTestCtx, "admin", "secret", true)
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				// When we send the valid Bearer token
				req := httptest.NewRequest("GET", "/api/messages", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := httpServer.App.Test(req)

				// Then the request passes through and is not rejected with 401
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).NotTo(Equal(401))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/auth/login
	// ---------------------------------------------------------------------------

	Describe("POST /api/auth/login", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when valid credentials are provided", func() {
			It("should return 200 with a non-empty token", func() {
				_, err := userStore.Create(httpTestCtx, "loginuser", "correctpass", false)
				Expect(err).NotTo(HaveOccurred())

				body := `{"username":"loginuser","password":"correctpass"}`
				req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["token"]).NotTo(BeEmpty())
			})
		})

		Context("when the password is wrong", func() {
			It("should return 401", func() {
				_, err := userStore.Create(httpTestCtx, "loginuser2", "realpass", false)
				Expect(err).NotTo(HaveOccurred())

				body := `{"username":"loginuser2","password":"wrongpass"}`
				req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid credentials"))
			})
		})

		Context("when the username is unknown", func() {
			It("should return 401", func() {
				body := `{"username":"nobody","password":"pass"}`
				req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("invalid credentials"))
			})
		})

		Context("when required fields are missing", func() {
			It("should return 400 when the body is empty", func() {
				req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("username and password are required"))
			})

			It("should return 400 when only username is provided", func() {
				req := httptest.NewRequest("POST", "/api/auth/login", strings.NewReader(`{"username":"u"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/auth/refresh
	// ---------------------------------------------------------------------------

	Describe("POST /api/auth/refresh", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when a valid token is provided", func() {
			It("should return 200 with a new token", func() {
				u, err := userStore.Create(httpTestCtx, "refresh-user", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), u.ID, u.Username, u.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["token"]).NotTo(BeEmpty())
			})
		})

		Context("when no token is provided", func() {
			It("should return 401", func() {
				req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("when an invalid token is provided", func() {
			It("should return 401", func() {
				req := httptest.NewRequest("POST", "/api/auth/refresh", nil)
				req.Header.Set("Authorization", "Bearer invalid.token.here")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// Permission enforcement
	// ---------------------------------------------------------------------------

	Describe("Permission enforcement", func() {
		var httpServer *server.HTTPServer
		authCfg := config.AuthConfig{
			Enabled:   true,
			JWTSecret: testJWTSecret,
		}

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, authCfg, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when an admin user makes requests", func() {
			It("should be able to access all protected routes", func() {
				admin, err := userStore.Create(httpTestCtx, "superadmin", "pass", true)
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				// GET /api/stats — admin-only
				req := httptest.NewRequest("GET", "/api/stats", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// GET /api/messages (any topic)
				req = httptest.NewRequest("GET", "/api/messages?topic=orders", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})

		Context("when a user has a read grant on orders.*", func() {
			It("should allow GET /api/messages for orders.x but deny for payments", func() {
				u, err := userStore.Create(httpTestCtx, "reader", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				_, err = userStore.AddGrant(httpTestCtx, u.ID, "read", "orders.*")
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), u.ID, u.Username, u.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				// Should be allowed for orders.new (matches orders.*)
				req := httptest.NewRequest("GET", "/api/messages?topic=orders.new", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// Should be denied for payments (does not match orders.*)
				req = httptest.NewRequest("GET", "/api/messages?topic=payments", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
				Expect(decodeErrorBody(resp.Body)).To(Equal("insufficient permissions"))
			})
		})

		Context("when a user has no grants", func() {
			It("should receive 403 on all protected routes", func() {
				u, err := userStore.Create(httpTestCtx, "no-grants-user", "pass", false)
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), u.ID, u.Username, u.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				authHeader := "Bearer " + token

				// GET /api/messages
				req := httptest.NewRequest("GET", "/api/messages?topic=orders", nil)
				req.Header.Set("Authorization", authHeader)
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))

				// POST /api/messages
				req = httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"orders","payload":"hi"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", authHeader)
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))

				// GET /api/stats (admin-only)
				req = httptest.NewRequest("GET", "/api/stats", nil)
				req.Header.Set("Authorization", authHeader)
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})

		Context("when auth is enabled and no Authorization header is sent", func() {
			It("should return 401 on requireGrant-protected routes", func() {
				// GET /api/messages — protected by requireGrant("read")
				req := httptest.NewRequest("GET", "/api/messages?topic=orders", nil)
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))

				// POST /api/messages — protected by requireGrant("write")
				req = httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"orders","payload":"hi"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})

			It("should return 401 on requireAdmin-protected routes", func() {
				// GET /api/stats — protected by requireAdmin
				req := httptest.NewRequest("GET", "/api/stats", nil)
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))

				// GET /api/users — protected by requireAdmin
				req = httptest.NewRequest("GET", "/api/users", nil)
				resp, err = httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /api/messages
	// ---------------------------------------------------------------------------

	Describe("GET /api/messages", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
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
				_, err := queueService.Enqueue(httpTestCtx, "topic-alpha", []byte("msg-alpha"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = queueService.Enqueue(httpTestCtx, "topic-beta", []byte("msg-beta"), nil, nil)
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
				id, err := queueService.Enqueue(httpTestCtx, "topic-fields", []byte("hello"), map[string]string{"key": "value"}, nil)
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

		Context("Given auth is enabled and a valid Bearer token is provided", func() {
			It("should return messages for authenticated admin requests", func() {
				// Given auth is enabled, an admin user, and a seeded message
				us := users.NewStore(httpTestPool)
				admin, err := us.Create(httpTestCtx, "msg-admin", "secret", true)
				Expect(err).NotTo(HaveOccurred())

				authServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
					Enabled:   true,
					JWTSecret: testJWTSecret,
				}, prometheus.NewRegistry(), us, "dev")
				_, err = queueService.Enqueue(httpTestCtx, "auth-topic", []byte("secure-msg"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				token, err := users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				// When we call the list endpoint with a valid JWT
				req := httptest.NewRequest("GET", "/api/messages?topic=auth-topic", nil)
				req.Header.Set("Authorization", "Bearer "+token)
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
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
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
			var adminToken string

			BeforeEach(func() {
				us := users.NewStore(httpTestPool)
				admin, err := us.Create(httpTestCtx, "post-admin", "secret", true)
				Expect(err).NotTo(HaveOccurred())

				adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
				Expect(err).NotTo(HaveOccurred())

				authServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
					Enabled:   true,
					JWTSecret: testJWTSecret,
				}, prometheus.NewRegistry(), us, "dev")
			})

			It("should enqueue successfully when a valid admin token is provided", func() {
				// When we POST with a valid Bearer token
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"auth-post-topic","payload":"secure"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
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

		Context("when require_topic_registration is enabled and the topic is not registered", func() {
			It("should return 422", func() {
				// Given a service with topic registration enforcement and no topic_config row.
				strictService := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, true, queue.NoopRecorder{})
				strictServer := server.NewHTTPServer(strictService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"unknown-topic","payload":"hello"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := strictServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(422))
			})
		})

		Context("when require_topic_registration is enabled and the topic is registered", func() {
			It("should return 201", func() {
				// Given a service with topic registration enforcement and a registered topic.
				strictService := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, true, queue.NoopRecorder{})
				Expect(strictService.UpsertTopicConfig(httpTestCtx, queue.TopicConfig{
					Topic: "known-topic",
				})).To(Succeed())

				strictServer := server.NewHTTPServer(strictService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"known-topic","payload":"hello"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := strictServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				Expect(body["id"]).NotTo(BeEmpty())
			})
		})

		Context("when a key is provided in the request body", func() {
			It("should return 201 and expose the key when listing the message", func() {
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"key-http-topic","payload":"hello","key":"k1"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				listReq := httptest.NewRequest("GET", "/api/messages?topic=key-http-topic", nil)
				listResp, err := httpServer.App.Test(listReq)
				Expect(err).NotTo(HaveOccurred())

				var result listResult
				Expect(json.NewDecoder(listResp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
				Expect(result.Items[0]["key"]).To(Equal("k1"))
			})
		})

		Context("when key is omitted from the request body", func() {
			It("should return 201 and the message should have no key field in the list response", func() {
				req := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"keyless-http-topic","payload":"no-key"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))

				listReq := httptest.NewRequest("GET", "/api/messages?topic=keyless-http-topic", nil)
				listResp, err := httpServer.App.Test(listReq)
				Expect(err).NotTo(HaveOccurred())

				var result listResult
				Expect(json.NewDecoder(listResp.Body).Decode(&result)).To(Succeed())
				Expect(result.Items).To(HaveLen(1))
				_, hasKey := result.Items[0]["key"]
				Expect(hasKey).To(BeFalse())
			})
		})

		Context("when the same key+topic is posted twice", func() {
			It("should return the same message ID on the second post", func() {
				body := `{"topic":"upsert-http-topic","payload":"first","key":"dup-key"}`

				req1 := httptest.NewRequest("POST", "/api/messages", strings.NewReader(body))
				req1.Header.Set("Content-Type", "application/json")
				resp1, err := httpServer.App.Test(req1)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp1.StatusCode).To(Equal(201))

				var body1 map[string]any
				Expect(json.NewDecoder(resp1.Body).Decode(&body1)).To(Succeed())
				id1 := body1["id"].(string)

				req2 := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"upsert-http-topic","payload":"second","key":"dup-key"}`))
				req2.Header.Set("Content-Type", "application/json")
				resp2, err := httpServer.App.Test(req2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp2.StatusCode).To(Equal(201))

				var body2 map[string]any
				Expect(json.NewDecoder(resp2.Body).Decode(&body2)).To(Succeed())
				Expect(body2["id"].(string)).To(Equal(id1))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// DELETE /api/topics/:topic/messages/by-key/:key
	// ---------------------------------------------------------------------------

	Describe("DELETE /api/topics/:topic/messages/by-key/:key", func() {
		var (
			httpServer *server.HTTPServer
			adminToken string
			userToken  string
		)

		BeforeEach(func() {
			admin, err := userStore.Create(httpTestCtx, "bykey-admin", "secret", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			nonAdmin, err := userStore.Create(httpTestCtx, "bykey-user", "secret", false)
			Expect(err).NotTo(HaveOccurred())
			userToken, err = users.IssueToken([]byte(testJWTSecret), nonAdmin.ID, nonAdmin.Username, nonAdmin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when a message with the given key exists on the topic", func() {
			It("should return 200 with deleted = 1", func() {
				k := "target-key"
				_, err := queueService.Enqueue(httpTestCtx, "bykey-topic", []byte("hello"), nil, &k)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest("DELETE", "/api/topics/bykey-topic/messages/by-key/target-key", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["deleted"]).To(BeEquivalentTo(1))
			})
		})

		Context("when no messages exist for the given key", func() {
			It("should return 200 with deleted = 0", func() {
				req := httptest.NewRequest("DELETE", "/api/topics/bykey-topic/messages/by-key/ghost-key", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["deleted"]).To(BeEquivalentTo(0))
			})
		})

		Context("when the caller is not an admin", func() {
			It("should return 403", func() {
				req := httptest.NewRequest("DELETE", "/api/topics/bykey-topic/messages/by-key/k1", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/messages/:id/requeue
	// ---------------------------------------------------------------------------

	Describe("POST /api/messages/:id/requeue", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("Given a message that has been promoted to the DLQ", func() {
			var dlqMessageID string

			BeforeEach(func() {
				// Use dlqThreshold = 1 so a single nack promotes the message.
				dlqService := queue.NewService(httpTestPool, 30*time.Second, 5, 0, 1, false, queue.NoopRecorder{})
				httpServer = server.NewHTTPServer(dlqService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

				var err error
				dlqMessageID, err = dlqService.Enqueue(httpTestCtx, "payments", []byte("charge"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				_, err = dlqService.Dequeue(httpTestCtx, "payments", 0)
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
				regularMessageID, err = queueService.Enqueue(httpTestCtx, "regular-http-topic", []byte("normal"), nil, nil)
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
			svc := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, false, rec)
			httpServer = server.NewHTTPServer(svc, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, reg, userStore, "dev")
		})

		Context("when called without an Authorization header and auth is enabled", func() {
			It("should return 401 — metrics are protected by JWT middleware", func() {
				req := httptest.NewRequest("GET", "/metrics", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})

		Context("after a message has been enqueued", func() {
			BeforeEach(func() {
				// Enqueue via the service that shares the same recorder.
				reg2 := prometheus.NewRegistry()
				rec2 := metrics.New(httpTestPool, reg2)
				svc2 := queue.NewService(httpTestPool, 30*time.Second, 3, 0, 3, false, rec2)
				httpServer = server.NewHTTPServer(svc2, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, reg2, nil, "dev")

				_, err := svc2.Enqueue(httpTestCtx, "metrics-topic", []byte("hello"), nil, nil)
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
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")

			// Given 5 messages are enqueued on the same topic
			for i := range 5 {
				payload := fmt.Sprintf("msg-%d", i)
				_, err := queueService.Enqueue(httpTestCtx, paginationTopic, []byte(payload), nil, nil)
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
				_, err := queueService.Enqueue(httpTestCtx, "other-topic", []byte("noise"), nil, nil)
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

	// ---------------------------------------------------------------------------
	// GET /api/topic-configs
	// ---------------------------------------------------------------------------

	Describe("GET /api/topic-configs", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("when no topic configs exist", func() {
			It("should return 200 with an empty items array", func() {
				req := httptest.NewRequest("GET", "/api/topic-configs", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				items, ok := body["items"].([]any)
				Expect(ok).To(BeTrue())
				Expect(items).To(BeEmpty())
			})
		})

		Context("after a topic config has been upserted", func() {
			It("should return the config in the items list", func() {
				maxRetries := 7
				Expect(queueService.UpsertTopicConfig(httpTestCtx, queue.TopicConfig{
					Topic:      "listed-topic",
					MaxRetries: &maxRetries,
				})).To(Succeed())

				req := httptest.NewRequest("GET", "/api/topic-configs", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				items, ok := body["items"].([]any)
				Expect(ok).To(BeTrue())
				Expect(items).To(HaveLen(1))
				item := items[0].(map[string]any)
				Expect(item["topic"]).To(Equal("listed-topic"))
				Expect(item["max_retries"]).To(BeEquivalentTo(7))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// PUT /api/topic-configs/:topic
	// ---------------------------------------------------------------------------

	Describe("PUT /api/topic-configs/:topic", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("when a valid body is provided", func() {
			It("should return 200 with the stored config", func() {
				body := `{"max_retries":5,"max_depth":100}`
				req := httptest.NewRequest("PUT", "/api/topic-configs/my-topic", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["topic"]).To(Equal("my-topic"))
				Expect(result["max_retries"]).To(BeEquivalentTo(5))
				Expect(result["max_depth"]).To(BeEquivalentTo(100))
			})
		})

		Context("when the topic name ends with .dlq", func() {
			It("should return 400", func() {
				req := httptest.NewRequest("PUT", "/api/topic-configs/payments.dlq",
					strings.NewReader(`{"max_retries":1}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("topic name may not end in .dlq"))
			})
		})

		Context("when no Authorization header is provided and auth is enabled", func() {
			It("should return 401", func() {
				authServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
					Enabled:   true,
					JWTSecret: testJWTSecret,
				}, prometheus.NewRegistry(), userStore, "dev")

				req := httptest.NewRequest("PUT", "/api/topic-configs/some-topic",
					strings.NewReader(`{"max_retries":1}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := authServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// DELETE /api/topic-configs/:topic
	// ---------------------------------------------------------------------------

	Describe("DELETE /api/topic-configs/:topic", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("when the topic config exists", func() {
			It("should return 204", func() {
				maxRetries := 3
				Expect(queueService.UpsertTopicConfig(httpTestCtx, queue.TopicConfig{
					Topic:      "delete-http-topic",
					MaxRetries: &maxRetries,
				})).To(Succeed())

				req := httptest.NewRequest("DELETE", "/api/topic-configs/delete-http-topic", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(204))
			})
		})

		Context("when the topic config does not exist", func() {
			It("should return 404", func() {
				req := httptest.NewRequest("DELETE", "/api/topic-configs/ghost-topic", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(404))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/messages with max_depth enforced
	// ---------------------------------------------------------------------------

	Describe("POST /api/messages with max_depth enforced", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			_, err := httpTestPool.Exec(httpTestCtx, "DELETE FROM topic_config")
			Expect(err).NotTo(HaveOccurred())
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("when a topic config with max_depth=1 is set and one message is already enqueued", func() {
			It("should return 429 on the second enqueue attempt", func() {
				maxDepth := 1
				Expect(queueService.UpsertTopicConfig(httpTestCtx, queue.TopicConfig{
					Topic:    "depth-http-topic",
					MaxDepth: &maxDepth,
				})).To(Succeed())

				// First enqueue — must succeed.
				req1 := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"depth-http-topic","payload":"first"}`))
				req1.Header.Set("Content-Type", "application/json")
				resp1, err := httpServer.App.Test(req1)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp1.StatusCode).To(Equal(201))

				// Second enqueue — must be rejected with 429.
				req2 := httptest.NewRequest("POST", "/api/messages",
					strings.NewReader(`{"topic":"depth-http-topic","payload":"second"}`))
				req2.Header.Set("Content-Type", "application/json")
				resp2, err := httpServer.App.Test(req2)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp2.StatusCode).To(Equal(429))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/topics/:topic/purge
	// ---------------------------------------------------------------------------

	Describe("POST /api/topics/:topic/purge", func() {
		var (
			httpServer *server.HTTPServer
			adminToken string
			userToken  string
		)

		BeforeEach(func() {
			admin, err := userStore.Create(httpTestCtx, "purge-admin", "secret", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			nonAdmin, err := userStore.Create(httpTestCtx, "purge-user", "secret", false)
			Expect(err).NotTo(HaveOccurred())
			userToken, err = users.IssueToken([]byte(testJWTSecret), nonAdmin.ID, nonAdmin.Username, nonAdmin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")

			// Seed 2 pending messages on "purge-http-topic"
			_, err = queueService.Enqueue(httpTestCtx, "purge-http-topic", []byte("msg-1"), nil, nil)
			Expect(err).NotTo(HaveOccurred())
			_, err = queueService.Enqueue(httpTestCtx, "purge-http-topic", []byte("msg-2"), nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when an admin sends a valid purge request", func() {
			It("should return 200 with the number of deleted messages", func() {
				body := `{"statuses":["pending"]}`
				req := httptest.NewRequest("POST", "/api/topics/purge-http-topic/purge", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				Expect(result["deleted"]).To(BeEquivalentTo(2))
			})
		})

		Context("when the request body omits statuses", func() {
			It("should default to all statuses and return 200 with deleted >= 0", func() {
				req := httptest.NewRequest("POST", "/api/topics/purge-http-topic/purge", strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				deleted, ok := result["deleted"].(float64)
				Expect(ok).To(BeTrue())
				Expect(deleted).To(BeNumerically(">=", 0))
			})
		})

		Context("when the request contains an invalid status value", func() {
			It("should return 400", func() {
				body := `{"statuses":["invalid"]}`
				req := httptest.NewRequest("POST", "/api/topics/purge-http-topic/purge", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
			})
		})

		Context("when the caller is not an admin", func() {
			It("should return 403", func() {
				body := `{"statuses":["pending"]}`
				req := httptest.NewRequest("POST", "/api/topics/purge-http-topic/purge", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+userToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/admin/expiry-reaper/run
	// ---------------------------------------------------------------------------

	Describe("POST /api/admin/expiry-reaper/run", func() {
		var (
			httpServer *server.HTTPServer
			adminToken string
			userToken  string
		)

		BeforeEach(func() {
			admin, err := userStore.Create(httpTestCtx, "expiry-admin", "secret", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			nonAdmin, err := userStore.Create(httpTestCtx, "expiry-user", "secret", false)
			Expect(err).NotTo(HaveOccurred())
			userToken, err = users.IssueToken([]byte(testJWTSecret), nonAdmin.ID, nonAdmin.Username, nonAdmin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when an admin triggers the expiry reaper", func() {
			It("should return 200 with an expired count >= 0", func() {
				req := httptest.NewRequest("POST", "/api/admin/expiry-reaper/run", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				expired, ok := result["expired"].(float64)
				Expect(ok).To(BeTrue())
				Expect(expired).To(BeNumerically(">=", 0))
			})
		})

		Context("when the caller is not an admin", func() {
			It("should return 403", func() {
				req := httptest.NewRequest("POST", "/api/admin/expiry-reaper/run", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/admin/delete-reaper/run
	// ---------------------------------------------------------------------------

	Describe("POST /api/admin/delete-reaper/run", func() {
		var (
			httpServer *server.HTTPServer
			adminToken string
			userToken  string
		)

		BeforeEach(func() {
			admin, err := userStore.Create(httpTestCtx, "delete-admin", "secret", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			nonAdmin, err := userStore.Create(httpTestCtx, "delete-user", "secret", false)
			Expect(err).NotTo(HaveOccurred())
			userToken, err = users.IssueToken([]byte(testJWTSecret), nonAdmin.ID, nonAdmin.Username, nonAdmin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")
		})

		Context("when an admin triggers the delete reaper", func() {
			It("should return 200 with a deleted count >= 0", func() {
				req := httptest.NewRequest("POST", "/api/admin/delete-reaper/run", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var result map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&result)).To(Succeed())
				deleted, ok := result["deleted"].(float64)
				Expect(ok).To(BeTrue())
				Expect(deleted).To(BeNumerically(">=", 0))
			})
		})

		Context("when the caller is not an admin", func() {
			It("should return 403", func() {
				req := httptest.NewRequest("POST", "/api/admin/delete-reaper/run", nil)
				req.Header.Set("Authorization", "Bearer "+userToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(403))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// GET /api/stats
	// ---------------------------------------------------------------------------

	Describe("GET /api/stats", func() {
		var httpServer *server.HTTPServer

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{Enabled: false}, prometheus.NewRegistry(), nil, "dev")
		})

		Context("with no messages", func() {
			It("should return status 200 with an empty topics array", func() {
				req := httptest.NewRequest("GET", "/api/stats", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
				topics, ok := body["topics"].([]any)
				Expect(ok).To(BeTrue())
				Expect(topics).To(BeEmpty())
			})
		})

		Context("with messages across two topics", func() {
			BeforeEach(func() {
				// Enqueue 2 pending on "orders"
				_, err := queueService.Enqueue(httpTestCtx, "orders", []byte("order-1"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
				_, err = queueService.Enqueue(httpTestCtx, "orders", []byte("order-2"), nil, nil)
				Expect(err).NotTo(HaveOccurred())

				// Dequeue 1 from "orders" to put it into processing state
				_, err = queueService.Dequeue(httpTestCtx, "orders", 0)
				Expect(err).NotTo(HaveOccurred())

				// Enqueue 1 pending on "payments"
				_, err = queueService.Enqueue(httpTestCtx, "payments", []byte("payment-1"), nil, nil)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return the correct count entries for each topic/status pair", func() {
				req := httptest.NewRequest("GET", "/api/stats", nil)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				var body map[string]any
				Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())

				topics, ok := body["topics"].([]any)
				Expect(ok).To(BeTrue())
				Expect(topics).To(HaveLen(3))

				toStatMap := func(v any) map[string]any {
					m, _ := v.(map[string]any)
					return m
				}

				Expect(topics).To(ContainElements(
					WithTransform(toStatMap, And(
						HaveKeyWithValue("topic", "orders"),
						HaveKeyWithValue("status", "pending"),
						HaveKeyWithValue("count", float64(1)),
					)),
					WithTransform(toStatMap, And(
						HaveKeyWithValue("topic", "orders"),
						HaveKeyWithValue("status", "processing"),
						HaveKeyWithValue("count", float64(1)),
					)),
					WithTransform(toStatMap, And(
						HaveKeyWithValue("topic", "payments"),
						HaveKeyWithValue("status", "pending"),
						HaveKeyWithValue("count", float64(1)),
					)),
				))
			})
		})

		Context("when auth is enabled and no Authorization header is sent", func() {
			It("should return 401", func() {
				authServer := server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
					Enabled:   true,
					JWTSecret: testJWTSecret,
				}, prometheus.NewRegistry(), userStore, "dev")

				req := httptest.NewRequest("GET", "/api/stats", nil)
				resp, err := authServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(401))
				Expect(decodeErrorBody(resp.Body)).To(Equal("missing authorization header"))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// POST /api/users
	// ---------------------------------------------------------------------------

	Describe("POST /api/users", func() {
		var httpServer *server.HTTPServer
		var adminToken string

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")

			admin, err := userStore.Create(httpTestCtx, "post-users-admin", "strongpassword123", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when the password is shorter than 12 characters", func() {
			It("should return 400 with a descriptive error", func() {
				body := `{"username":"newuser","password":"short"}`
				req := httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("password must be at least 12 characters"))
			})
		})

		Context("when the password meets the minimum length", func() {
			It("should return 201 and create the user", func() {
				body := `{"username":"newuser","password":"strongpassword123"}`
				req := httptest.NewRequest("POST", "/api/users", strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(201))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// PUT /api/users/:id
	// ---------------------------------------------------------------------------

	Describe("PUT /api/users/:id", func() {
		var httpServer *server.HTTPServer
		var adminToken string
		var targetUserID string

		BeforeEach(func() {
			httpServer = server.NewHTTPServer(queueService, config.ServerConfig{CORSOrigins: "*"}, config.RedisConfig{}, config.AuthConfig{
				Enabled:   true,
				JWTSecret: testJWTSecret,
			}, prometheus.NewRegistry(), userStore, "dev")

			admin, err := userStore.Create(httpTestCtx, "put-users-admin", "strongpassword123", true)
			Expect(err).NotTo(HaveOccurred())
			adminToken, err = users.IssueToken([]byte(testJWTSecret), admin.ID, admin.Username, admin.IsAdmin)
			Expect(err).NotTo(HaveOccurred())

			target, err := userStore.Create(httpTestCtx, "put-users-target", "strongpassword123", false)
			Expect(err).NotTo(HaveOccurred())
			targetUserID = target.ID
		})

		Context("when the new password is shorter than 12 characters", func() {
			It("should return 400 with a descriptive error", func() {
				body := `{"password":"short"}`
				req := httptest.NewRequest("PUT", "/api/users/"+targetUserID, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(400))
				Expect(decodeErrorBody(resp.Body)).To(Equal("password must be at least 12 characters"))
			})
		})

		Context("when the new password meets the minimum length", func() {
			It("should return 200 and update the user", func() {
				body := `{"password":"newstrongpassword123"}`
				req := httptest.NewRequest("PUT", "/api/users/"+targetUserID, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				resp, err := httpServer.App.Test(req)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
			})
		})
	})

	// ---------------------------------------------------------------------------
	// Redis-backed rate limiter integration test
	// ---------------------------------------------------------------------------

	Describe("POST /api/auth/login rate limiting with Redis storage", func() {
		var redisContainer *tcredis.RedisContainer

		BeforeEach(func() {
			var err error
			redisContainer, err = tcredis.Run(httpTestCtx, "redis:7-alpine")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if redisContainer != nil {
				_ = redisContainer.Terminate(httpTestCtx)
			}
		})

		It("enforces the rate limit after 10 requests and returns 429 on the 11th", func() {
			connStr, err := redisContainer.ConnectionString(httpTestCtx)
			Expect(err).NotTo(HaveOccurred())

			u, err := url.Parse(connStr)
			Expect(err).NotTo(HaveOccurred())
			host, portStr, err := net.SplitHostPort(u.Host)
			Expect(err).NotTo(HaveOccurred())
			port, err := strconv.Atoi(portStr)
			Expect(err).NotTo(HaveOccurred())

			httpServer := server.NewHTTPServer(
				queueService,
				config.ServerConfig{CORSOrigins: "*"},
				config.RedisConfig{Host: host, Port: port},
				config.AuthConfig{Enabled: false},
				prometheus.NewRegistry(),
				userStore,
				"dev",
			)

			for i := range 10 {
				req := httptest.NewRequest("POST", "/api/auth/login",
					strings.NewReader(`{"username":"x","password":"y"}`))
				req.Header.Set("Content-Type", "application/json")
				resp, err := httpServer.App.Test(req)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).NotTo(Equal(429), "request %d should not be rate-limited yet", i+1)
			}

			req := httptest.NewRequest("POST", "/api/auth/login",
				strings.NewReader(`{"username":"x","password":"y"}`))
			req.Header.Set("Content-Type", "application/json")
			resp, err := httpServer.App.Test(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(429))
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
