package server

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type enqueueRequest struct {
	Topic    string            `json:"topic"`
	Payload  string            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
	Key      *string           `json:"key,omitempty"`
}

type nackRequest struct {
	Error string `json:"error"`
}

type messageResponse struct {
	ID            string            `json:"id"`
	Topic         string            `json:"topic"`
	Key           *string           `json:"key,omitempty"`
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

type batchDequeueRequest struct {
	Topic                 string `json:"topic"`
	Count                 int    `json:"count"`
	VisibilityTimeoutSecs *int   `json:"visibility_timeout_seconds,omitempty"`
}

type batchDequeueResponse struct {
	Messages []messageResponse `json:"messages"`
}

type topicStatResponse struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type statsResponse struct {
	Topics []topicStatResponse `json:"topics"`
}

func toMessageResponse(m queue.Message) messageResponse {
	r := messageResponse{
		ID:            m.ID,
		Topic:         m.Topic,
		Key:           m.Key,
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
	return r
}

func (s *HTTPServer) listMessages(c *fiber.Ctx) error {
	topic := c.Query("topic")

	limit := c.QueryInt("limit", 50)
	if limit < 1 {
		limit = 1
	} else if limit > 200 {
		limit = 200
	}

	offset := max(c.QueryInt("offset", 0), 0)

	messages, total, err := s.queueService.List(c.Context(), topic, limit, offset)
	if err != nil {
		slog.Error("list messages failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]messageResponse, 0, len(messages))
	for _, m := range messages {
		items = append(items, toMessageResponse(m))
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

	id, err := s.queueService.Enqueue(c.Context(), req.Topic, []byte(req.Payload), req.Metadata, req.Key)
	if err != nil {
		if errors.Is(err, queue.ErrSchemaValidation) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, queue.ErrTopicNotRegistered) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		if errors.Is(err, queue.ErrQueueFull) {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("enqueue failed", "topic", req.Topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (s *HTTPServer) batchDequeueMessages(c *fiber.Ctx) error {
	var req batchDequeueRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	if req.Topic == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "topic is required"})
	}

	if req.Count == 0 {
		req.Count = 1
	}

	if req.Count < 1 || req.Count > 1000 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "count must be between 1 and 1000"})
	}

	var vt time.Duration
	if req.VisibilityTimeoutSecs != nil {
		vt = time.Duration(*req.VisibilityTimeoutSecs) * time.Second
	}

	batch, err := s.queueService.DequeueN(c.Context(), req.Topic, req.Count, vt)
	if err != nil {
		if errors.Is(err, queue.ErrInvalidBatchSize) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("batch dequeue failed", "topic", req.Topic, "count", req.Count, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	items := make([]messageResponse, 0, len(batch))
	for _, m := range batch {
		items = append(items, toMessageResponse(*m))
	}

	return c.JSON(batchDequeueResponse{Messages: items})
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
