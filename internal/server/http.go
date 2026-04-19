package server

import (
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"

	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type HTTPServer struct {
	App          *fiber.App
	queueService *queue.Service
	authConfig   config.AuthConfig
}

func NewHTTPServer(qs *queue.Service, authCfg config.AuthConfig) *HTTPServer {
	s := &HTTPServer{
		queueService: qs,
		authConfig:   authCfg,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	api := app.Group("/api")
	api.Get("/auth/status", s.authStatus)
	api.Get("/messages", s.withAuth(), s.listMessages)
	api.Post("/messages", s.withAuth(), s.enqueueMessage)

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
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authorization header"})
		}

		if !strings.HasPrefix(authHeader, "Basic ") {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unsupported auth scheme"})
		}

		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid authorization format"})
		}

		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 || parts[0] != s.authConfig.Username || parts[1] != s.authConfig.Password {
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

type messageResponse struct {
	ID        string            `json:"id"`
	Topic     string            `json:"topic"`
	Payload   string            `json:"payload"`
	Metadata  map[string]string `json:"metadata"`
	Status    string            `json:"status"`
	CreatedAt string            `json:"created_at"`
}

func (s *HTTPServer) listMessages(c *fiber.Ctx) error {
	topic := c.Query("topic")

	messages, err := s.queueService.List(c.Context(), topic)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	resp := make([]messageResponse, 0, len(messages))
	for _, m := range messages {
		resp = append(resp, messageResponse{
			ID:        m.ID,
			Topic:     m.Topic,
			Payload:   string(m.Payload),
			Metadata:  m.Metadata,
			Status:    m.Status,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return c.JSON(resp)
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

