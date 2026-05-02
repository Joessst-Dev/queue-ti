package server

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"
)

var validPurgeStatuses = map[string]bool{
	"pending":    true,
	"processing": true,
	"expired":    true,
}

type purgeTopicRequest struct {
	Statuses []string `json:"statuses"`
}

func (s *HTTPServer) purgeTopicMessages(c *fiber.Ctx) error {
	topic := c.Params("topic")

	var req purgeTopicRequest
	if err := c.BodyParser(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}

	statuses := req.Statuses
	if len(statuses) == 0 {
		statuses = []string{"pending", "processing", "expired"}
	}

	for _, st := range statuses {
		if !validPurgeStatuses[st] {
			return jsonError(c, fiber.StatusBadRequest,
				"invalid status: "+st+"; allowed values are pending, processing, expired")
		}
	}

	n, err := s.queueService.PurgeTopic(c.Context(), topic, statuses)
	if err != nil {
		slog.Error("purge topic failed", "topic", topic, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"deleted": n})
}

func (s *HTTPServer) purgeByKeyMessages(c *fiber.Ctx) error {
	topic := c.Params("topic")
	key := c.Params("key")
	if key == "" {
		return jsonError(c, fiber.StatusBadRequest, "key is required")
	}
	n, err := s.queueService.PurgeByKey(c.Context(), topic, key)
	if err != nil {
		slog.Error("purge by key failed", "topic", topic, "key", key, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"deleted": n})
}

func (s *HTTPServer) runExpiryReaperOnce(c *fiber.Ctx) error {
	n, err := s.queueService.RunExpiryReaperOnce(c.Context())
	if err != nil {
		slog.Error("expiry reaper (manual) failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"expired": n})
}

func (s *HTTPServer) runDeleteReaperOnce(c *fiber.Ctx) error {
	n, err := s.queueService.RunDeleteReaperOnce(c.Context())
	if err != nil {
		slog.Error("delete reaper (manual) failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"deleted": n})
}

func (s *HTTPServer) getDeleteReaperSchedule(c *fiber.Ctx) error {
	schedule, err := s.queueService.GetDeleteReaperSchedule(c.Context())
	if err != nil {
		slog.Error("get delete reaper schedule failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.JSON(fiber.Map{"schedule": schedule, "active": schedule != ""})
}

func (s *HTTPServer) updateDeleteReaperSchedule(c *fiber.Ctx) error {
	var req struct {
		Schedule string `json:"schedule"`
	}
	if err := c.BodyParser(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if err := s.queueService.UpdateDeleteReaperSchedule(c.Context(), req.Schedule); err != nil {
		return jsonError(c, fiber.StatusUnprocessableEntity, err.Error())
	}
	return c.JSON(fiber.Map{"schedule": req.Schedule, "active": req.Schedule != ""})
}
