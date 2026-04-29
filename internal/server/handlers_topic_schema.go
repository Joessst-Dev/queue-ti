package server

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type topicSchemaRequest struct {
	SchemaJSON string `json:"schema_json"`
}

type topicSchemaResponse struct {
	Topic      string `json:"topic"`
	SchemaJSON string `json:"schema_json"`
	Version    int    `json:"version"`
	UpdatedAt  string `json:"updated_at"`
}

type listTopicSchemasResponse struct {
	Items []topicSchemaResponse `json:"items"`
}

func toTopicSchemaResponse(ts queue.TopicSchema) topicSchemaResponse {
	return topicSchemaResponse{
		Topic:      ts.Topic,
		SchemaJSON: ts.SchemaJSON,
		Version:    ts.Version,
		UpdatedAt:  ts.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *HTTPServer) listTopicSchemas(c *fiber.Ctx) error {
	schemas, err := queue.ListTopicSchemas(c.Context(), s.queueService.Pool())
	if err != nil {
		slog.Error("list topic schemas failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	items := make([]topicSchemaResponse, len(schemas))
	for i, ts := range schemas {
		items[i] = toTopicSchemaResponse(ts)
	}
	return c.JSON(listTopicSchemasResponse{Items: items})
}

func (s *HTTPServer) upsertTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")

	var req topicSchemaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	ts, err := queue.UpsertTopicSchema(c.Context(), s.queueService.Pool(), topic, req.SchemaJSON)
	if err != nil {
		if errors.Is(err, queue.ErrInvalidSchema) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("upsert topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.JSON(toTopicSchemaResponse(*ts))
}

func (s *HTTPServer) deleteTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if err := queue.DeleteTopicSchema(c.Context(), s.queueService.Pool(), topic); err != nil {
		slog.Error("delete topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *HTTPServer) getTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")
	ts, err := queue.GetTopicSchema(c.Context(), s.queueService.Pool(), topic)
	if err != nil {
		slog.Error("get topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
	}
	if ts == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schema not found"})
	}
	return c.JSON(toTopicSchemaResponse(*ts))
}
