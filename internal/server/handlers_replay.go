package server

import (
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type replayRequest struct {
	FromTime *string `json:"from_time,omitempty"`
}

type replayResponse struct {
	Topic    string `json:"topic"`
	Enqueued int64  `json:"enqueued"`
	FromTime string `json:"from_time"`
}

type archivedMessageResponse struct {
	ID            string  `json:"id"`
	Topic         string  `json:"topic"`
	Key           *string `json:"key,omitempty"`
	Payload       string  `json:"payload"`
	RetryCount    int     `json:"retry_count"`
	OriginalTopic string  `json:"original_topic,omitempty"`
	CreatedAt     string  `json:"created_at"`
	AckedAt       string  `json:"acked_at"`
}

type messageLogResponse struct {
	Items  []archivedMessageResponse `json:"items"`
	Total  int                       `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

func (s *HTTPServer) replayTopic(c *fiber.Ctx) error {
	topic := c.Params("topic")

	var req replayRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	var fromTime time.Time
	if req.FromTime != nil && *req.FromTime != "" {
		var err error
		fromTime, err = time.Parse(time.RFC3339, *req.FromTime)
		if err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "from_time must be RFC 3339"})
		}
	}

	n, err := s.queueService.ReplayTopic(c.Context(), topic, fromTime)
	if err != nil {
		if errors.Is(err, queue.ErrTopicNotReplayable) || errors.Is(err, queue.ErrReplayWindowTooOld) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("replay topic failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	fromStr := ""
	if !fromTime.IsZero() {
		fromStr = fromTime.UTC().Format(time.RFC3339)
	}
	return c.JSON(replayResponse{Topic: topic, Enqueued: n, FromTime: fromStr})
}

func (s *HTTPServer) listMessageLog(c *fiber.Ctx) error {
	topic := c.Params("topic")
	limit := c.QueryInt("limit", 50)
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := c.QueryInt("offset", 0)

	msgs, total, err := s.queueService.ListMessageLog(c.Context(), topic, limit, offset)
	if err != nil {
		slog.Error("list message log failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]archivedMessageResponse, len(msgs))
	for i, m := range msgs {
		items[i] = archivedMessageResponse{
			ID:            m.ID,
			Topic:         m.Topic,
			Key:           m.Key,
			Payload:       base64.StdEncoding.EncodeToString(m.Payload),
			RetryCount:    m.RetryCount,
			OriginalTopic: m.OriginalTopic,
			CreatedAt:     m.CreatedAt.UTC().Format(time.RFC3339),
			AckedAt:       m.AckedAt.UTC().Format(time.RFC3339),
		}
	}
	return c.JSON(messageLogResponse{Items: items, Total: total, Limit: limit, Offset: offset})
}

func (s *HTTPServer) runArchiveReaperOnce(c *fiber.Ctx) error {
	n, err := s.queueService.RunArchiveReaperOnce(c.Context())
	if err != nil {
		slog.Error("archive reaper (manual) failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": n})
}

func (s *HTTPServer) trimMessageLog(c *fiber.Ctx) error {
	topic := c.Params("topic")
	beforeStr := c.Query("before")
	if beforeStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "before query parameter is required"})
	}
	before, err := time.Parse(time.RFC3339, beforeStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "before must be RFC 3339"})
	}

	n, err := s.queueService.TrimMessageLog(c.Context(), topic, before)
	if err != nil {
		slog.Error("trim message log failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"deleted": n})
}
