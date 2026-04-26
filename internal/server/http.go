package server

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type HTTPServer struct {
	App          *fiber.App
	queueService *queue.Service
	authConfig   config.AuthConfig
}

func NewHTTPServer(qs *queue.Service, authCfg config.AuthConfig, gatherer prometheus.Gatherer) *HTTPServer {
	s := &HTTPServer{
		queueService: qs,
		authConfig:   authCfg,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
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

	promH := fasthttpadaptor.NewFastHTTPHandler(
		promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)
	app.Get("/metrics", func(c *fiber.Ctx) error {
		promH(c.Context())
		return nil
	})

	api := app.Group("/api")
	api.Get("/auth/status", s.authStatus)
	api.Get("/messages", s.withAuth(), s.listMessages)
	api.Post("/messages", s.withAuth(), s.enqueueMessage)
	api.Post("/messages/:id/nack", s.withAuth(), s.nackMessage)
	api.Post("/messages/:id/requeue", s.withAuth(), s.requeueMessage)
	api.Get("/stats", s.withAuth(), s.statsHandler)
	api.Get("/topic-configs", s.withAuth(), s.listTopicConfigs)
	api.Put("/topic-configs/:topic", s.withAuth(), s.upsertTopicConfig)
	api.Delete("/topic-configs/:topic", s.withAuth(), s.deleteTopicConfig)

	s.App = app
	return s
}

// withAuth returns a Fiber middleware that enforces basic auth when enabled.
func (s *HTTPServer) withAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !s.authConfig.Enabled {
			return c.Next()
		}

		authHeader := c.Get("Authorization")
		if authHeader == "" {
			slog.Warn("auth failure: missing authorization header", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authorization header"})
		}

		if !strings.HasPrefix(authHeader, "Basic ") {
			slog.Warn("auth failure: unsupported auth scheme", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unsupported auth scheme"})
		}

		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			slog.Warn("auth failure: invalid authorization format", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid authorization format"})
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 || parts[0] != s.authConfig.Username || parts[1] != s.authConfig.Password {
			slog.Warn("auth failure: invalid credentials", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
		}

		return c.Next()
	}
}

func (s *HTTPServer) authStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"auth_required": s.authConfig.Enabled})
}

type enqueueRequest struct {
	Topic    string            `json:"topic"`
	Payload  string            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

type nackRequest struct {
	Error string `json:"error"`
}

type messageResponse struct {
	ID            string            `json:"id"`
	Topic         string            `json:"topic"`
	Payload       string            `json:"payload"`
	Metadata      map[string]string `json:"metadata"`
	Status        string            `json:"status"`
	RetryCount    int               `json:"retry_count"`
	MaxRetries    int               `json:"max_retries"`
	LastError     string            `json:"last_error,omitempty"`
	ExpiresAt     *string           `json:"expires_at,omitempty"`
	CreatedAt     string            `json:"created_at"`
	OriginalTopic string            `json:"original_topic,omitempty"`
	DLQMovedAt    *string           `json:"dlq_moved_at,omitempty"`
}

type listResponse struct {
	Items  []messageResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

func (s *HTTPServer) listMessages(c *fiber.Ctx) error {
	topic := c.Query("topic")

	limit := c.QueryInt("limit", 50)
	if limit < 1 {
		limit = 1
	} else if limit > 200 {
		limit = 200
	}

	offset := c.QueryInt("offset", 0)
	if offset < 0 {
		offset = 0
	}

	messages, total, err := s.queueService.List(c.Context(), topic, limit, offset)
	if err != nil {
		slog.Error("list messages failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]messageResponse, 0, len(messages))
	for _, m := range messages {
		r := messageResponse{
			ID:            m.ID,
			Topic:         m.Topic,
			Payload:       string(m.Payload),
			Metadata:      m.Metadata,
			Status:        m.Status,
			RetryCount:    m.RetryCount,
			MaxRetries:    m.MaxRetries,
			LastError:     m.LastError,
			CreatedAt:     m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			OriginalTopic: m.OriginalTopic,
		}
		if m.ExpiresAt != nil {
			formatted := m.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
			r.ExpiresAt = &formatted
		}
		if m.DLQMovedAt != nil {
			formatted := m.DLQMovedAt.Format("2006-01-02T15:04:05Z07:00")
			r.DLQMovedAt = &formatted
		}
		items = append(items, r)
	}

	return c.JSON(listResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *HTTPServer) enqueueMessage(c *fiber.Ctx) error {
	var req enqueueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Topic == "" || req.Payload == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "topic and payload are required"})
	}

	id, err := s.queueService.Enqueue(c.Context(), req.Topic, []byte(req.Payload), req.Metadata)
	if err != nil {
		if errors.Is(err, queue.ErrQueueFull) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("enqueue failed", "topic", req.Topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (s *HTTPServer) nackMessage(c *fiber.Ctx) error {
	id := c.Params("id")

	var req nackRequest
	// Body is optional — an empty body is valid (no error message provided).
	_ = c.BodyParser(&req)

	if err := s.queueService.Nack(c.Context(), id, req.Error); err != nil {
		if errors.Is(err, queue.ErrNotFound) || errors.Is(err, queue.ErrNotProcessing) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("nack failed", "id", id, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

type topicStatResponse struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type statsResponse struct {
	Topics []topicStatResponse `json:"topics"`
}

func (s *HTTPServer) statsHandler(c *fiber.Ctx) error {
	stats, err := s.queueService.Stats(c.Context())
	if err != nil {
		slog.Error("stats failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]topicStatResponse, len(stats))
	for i, st := range stats {
		items[i] = topicStatResponse{Topic: st.Topic, Status: st.Status, Count: st.Count}
	}
	return c.JSON(statsResponse{Topics: items})
}

func (s *HTTPServer) requeueMessage(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := s.queueService.Requeue(c.Context(), id); err != nil {
		if errors.Is(err, queue.ErrNotFound) || errors.Is(err, queue.ErrNotDLQ) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("requeue failed", "id", id, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Topic-config types and handlers
// ---------------------------------------------------------------------------

type topicConfigRequest struct {
	MaxRetries        *int `json:"max_retries"`
	MessageTTLSeconds *int `json:"message_ttl_seconds"`
	MaxDepth          *int `json:"max_depth"`
}

type topicConfigResponse struct {
	Topic             string `json:"topic"`
	MaxRetries        *int   `json:"max_retries,omitempty"`
	MessageTTLSeconds *int   `json:"message_ttl_seconds,omitempty"`
	MaxDepth          *int   `json:"max_depth,omitempty"`
}

type listTopicConfigsResponse struct {
	Items []topicConfigResponse `json:"items"`
}

func toTopicConfigResponse(cfg queue.TopicConfig) topicConfigResponse {
	return topicConfigResponse{
		Topic:             cfg.Topic,
		MaxRetries:        cfg.MaxRetries,
		MessageTTLSeconds: cfg.MessageTTLSeconds,
		MaxDepth:          cfg.MaxDepth,
	}
}

func (s *HTTPServer) listTopicConfigs(c *fiber.Ctx) error {
	configs, err := s.queueService.ListTopicConfigs(c.Context())
	if err != nil {
		slog.Error("list topic configs failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]topicConfigResponse, len(configs))
	for i, cfg := range configs {
		items[i] = toTopicConfigResponse(cfg)
	}
	return c.JSON(listTopicConfigsResponse{Items: items})
}

func (s *HTTPServer) upsertTopicConfig(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if strings.HasSuffix(topic, ".dlq") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "topic name may not end in .dlq"})
	}

	var req topicConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	cfg := queue.TopicConfig{
		Topic:             topic,
		MaxRetries:        req.MaxRetries,
		MessageTTLSeconds: req.MessageTTLSeconds,
		MaxDepth:          req.MaxDepth,
	}
	if err := s.queueService.UpsertTopicConfig(c.Context(), cfg); err != nil {
		slog.Error("upsert topic config failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toTopicConfigResponse(cfg))
}

func (s *HTTPServer) deleteTopicConfig(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if err := s.queueService.DeleteTopicConfig(c.Context(), topic); err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("delete topic config failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
