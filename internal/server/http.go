package server

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

type HTTPServer struct {
	App          *fiber.App
	queueService *queue.Service
	authConfig   config.AuthConfig
	serverConfig config.ServerConfig
	userStore    *users.Store
	version      string
}

// NewHTTPServer creates the HTTP server. Pass a non-nil rateLimitStore (e.g. Redis) to
// share rate-limit state across replicas; nil falls back to in-memory storage.
func NewHTTPServer(qs *queue.Service, serverCfg config.ServerConfig, rateLimitStore fiber.Storage, authCfg config.AuthConfig, gatherer prometheus.Gatherer, userStore *users.Store, version string) *HTTPServer {
	s := &HTTPServer{
		queueService: qs,
		authConfig:   authCfg,
		serverConfig: serverCfg,
		userStore:    userStore,
		version:      version,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		// Trust X-Real-IP set by a reverse proxy (nginx) so that the rate limiter
		// keys on the actual client IP rather than the proxy's IP.
		ProxyHeader: "X-Real-IP",
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: serverCfg.CORSOrigins,
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))

	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		slog.Debug("http request",
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.IP(),
		)
		return err
	})

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	var jwtAuth fiber.Handler
	if authCfg.Enabled {
		jwtAuth = internalAuth.JWTMiddleware([]byte(authCfg.JWTSecret))
	} else {
		jwtAuth = func(c *fiber.Ctx) error { return c.Next() }
	}

	promH := fasthttpadaptor.NewFastHTTPHandler(
		promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)
	app.Get("/metrics", jwtAuth, func(c *fiber.Ctx) error {
		promH(c.Context())
		return nil
	})

	var loginLimiter fiber.Handler
	if rateLimitStore != nil {
		loginLimiter = limiter.New(limiter.Config{
			Max:        10,
			Expiration: 60 * time.Second,
			Storage:    rateLimitStore,
		})
		slog.Info("login rate limiter using Redis storage")
	} else {
		loginLimiter = limiter.New(limiter.Config{
			Max:        10,
			Expiration: 60 * time.Second,
		})
		slog.Info("login rate limiter using in-memory storage (single-instance only)")
	}

	api := app.Group("/api")
	api.Get("/version", s.versionHandler)
	api.Get("/auth/status", s.authStatus)
	api.Post("/auth/login", loginLimiter, s.handleLogin)
	api.Post("/auth/refresh", jwtAuth, s.handleRefresh)

	userRoutes := api.Group("/users", jwtAuth, s.requireAdmin())
	userRoutes.Get("", s.listUsers)
	userRoutes.Post("", s.createUser)
	userRoutes.Put("/:id", s.updateUser)
	userRoutes.Delete("/:id", s.deleteUser)
	userRoutes.Get("/:id/grants", s.listUserGrants)
	userRoutes.Post("/:id/grants", s.addUserGrant)
	userRoutes.Delete("/:id/grants/:grantId", s.deleteUserGrant)

	api.Get("/messages", jwtAuth, s.requireGrant("read", func(c *fiber.Ctx) string { return c.Query("topic") }), s.listMessages)
	api.Post("/messages", jwtAuth, s.requireGrant("write", func(c *fiber.Ctx) string {
		var peek struct {
			Topic string `json:"topic"`
		}
		_ = json.Unmarshal(c.Body(), &peek)
		return peek.Topic
	}), s.enqueueMessage)
	api.Post("/messages/dequeue", jwtAuth, s.requireGrant("write", func(c *fiber.Ctx) string {
		var peek struct {
			Topic string `json:"topic"`
		}
		_ = json.Unmarshal(c.Body(), &peek)
		return peek.Topic
	}), s.batchDequeueMessages)
	api.Post("/messages/:id/nack", jwtAuth, s.requireWriteOnMsgTopic(), s.nackMessage)
	api.Post("/messages/:id/requeue", jwtAuth, s.requireWriteOnMsgTopic(), s.requeueMessage)
	api.Get("/stats", jwtAuth, s.requireAdmin(), s.statsHandler)
	api.Get("/topic-configs", jwtAuth, s.requireAdmin(), s.listTopicConfigs)
	api.Put("/topic-configs/:topic", jwtAuth, s.requireAdmin(), s.upsertTopicConfig)
	api.Delete("/topic-configs/:topic", jwtAuth, s.requireAdmin(), s.deleteTopicConfig)
	api.Get("/topic-schemas", jwtAuth, s.requireAdmin(), s.listTopicSchemas)
	api.Put("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.upsertTopicSchema)
	api.Delete("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.deleteTopicSchema)
	api.Get("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.getTopicSchema)
	api.Post("/topics/:topic/purge", jwtAuth, s.requireAdmin(), s.purgeTopicMessages)
	api.Delete("/topics/:topic/messages/by-key/:key", jwtAuth, s.requireAdmin(), s.purgeByKeyMessages)
	api.Post("/admin/expiry-reaper/run", jwtAuth, s.requireAdmin(), s.runExpiryReaperOnce)
	api.Post("/admin/delete-reaper/run", jwtAuth, s.requireAdmin(), s.runDeleteReaperOnce)
	api.Get("/admin/delete-reaper/schedule", jwtAuth, s.requireAdmin(), s.getDeleteReaperSchedule)
	api.Put("/admin/delete-reaper/schedule", jwtAuth, s.requireAdmin(), s.updateDeleteReaperSchedule)
	api.Post("/topics/:topic/replay", jwtAuth, s.requireAdmin(), s.replayTopic)
	api.Get("/topics/:topic/message-log", jwtAuth, s.requireAdmin(), s.listMessageLog)
	api.Delete("/topics/:topic/message-log", jwtAuth, s.requireAdmin(), s.trimMessageLog)
	api.Post("/admin/archive-reaper/run", jwtAuth, s.requireAdmin(), s.runArchiveReaperOnce)

	s.App = app
	return s
}

func (s *HTTPServer) versionHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"version": s.version})
}

func (s *HTTPServer) authStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"auth_required": s.authConfig.Enabled})
}
