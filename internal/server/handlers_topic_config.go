package server

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

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
