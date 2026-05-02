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
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}

	var fromTime time.Time
	if req.FromTime != nil && *req.FromTime != "" {
		var err error
		fromTime, err = time.Parse(time.RFC3339, *req.FromTime)
		if err != nil {
			return jsonError(c, fiber.StatusUnprocessableEntity, "from_time must be RFC 3339")
		}
	}

	n, err := s.queueService.ReplayTopic(c.Context(), topic, fromTime)
	if err != nil {
		if errors.Is(err, queue.ErrTopicNotReplayable) || errors.Is(err, queue.ErrReplayWindowTooOld) {
			return jsonError(c, fiber.StatusUnprocessableEntity, err.Error())
		}
		slog.Error("replay topic failed", "topic", topic, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}

	fromStr := ""
	if !fromTime.IsZero() {
		fromStr = fromTime.UTC().Format(time.RFC3339)
	}
	return c.JSON(replayResponse{Topic: topic, Enqueued: n, FromTime: fromStr})
}

func (s *HTTPServer) listMessageLog(c *fiber.Ctx) error {
	topic := c.Params("topic")
	limit, offset, _ := parseLimitOffset(c, 50, 200)

	result, err := s.queueService.ListMessageLog(c.Context(), topic, limit, offset)
	if err != nil {
		slog.Error("list message log failed", "topic", topic, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}

	items := make([]archivedMessageResponse, len(result.Items))
	for i, m := range result.Items {
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
	return c.JSON(messageLogResponse{Items: items, Total: result.Total, Limit: limit, Offset: offset})
}

func (s *HTTPServer) runArchiveReaperOnce(c *fiber.Ctx) error {
	n, err := s.queueService.RunArchiveReaperOnce(c.Context())
	if err != nil {
		slog.Error("archive reaper (manual) failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"deleted": n})
}

func (s *HTTPServer) trimMessageLog(c *fiber.Ctx) error {
	topic := c.Params("topic")
	beforeStr := c.Query("before")
	if beforeStr == "" {
		return jsonError(c, fiber.StatusBadRequest, "before query parameter is required")
	}
	before, err := time.Parse(time.RFC3339, beforeStr)
	if err != nil {
		return jsonError(c, fiber.StatusBadRequest, "before must be RFC 3339")
	}

	n, err := s.queueService.TrimMessageLog(c.Context(), topic, before)
	if err != nil {
		slog.Error("trim message log failed", "topic", topic, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"deleted": n})
}
