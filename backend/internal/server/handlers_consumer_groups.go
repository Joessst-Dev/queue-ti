package server

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/queue"
)

type registerConsumerGroupRequest struct {
	ConsumerGroup string `json:"consumer_group"`
}

type listConsumerGroupsResponse struct {
	Items []string `json:"items"`
}

func (s *HTTPServer) listConsumerGroups(c *fiber.Ctx) error {
	topic := c.Params("topic")

	groups, err := s.queueService.ListConsumerGroups(c.Context(), topic)
	if err != nil {
		slog.Error("list consumer groups failed", "topic", topic, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}

	items := groups
	if items == nil {
		items = []string{}
	}

	return c.JSON(listConsumerGroupsResponse{Items: items})
}

func (s *HTTPServer) registerConsumerGroup(c *fiber.Ctx) error {
	topic := c.Params("topic")

	var req registerConsumerGroupRequest
	if err := c.BodyParser(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}

	if req.ConsumerGroup == "" {
		return jsonError(c, fiber.StatusBadRequest, "consumer_group is required")
	}

	if err := s.queueService.RegisterConsumerGroup(c.Context(), topic, req.ConsumerGroup); err != nil {
		if errors.Is(err, queue.ErrConsumerGroupExists) {
			return jsonError(c, fiber.StatusConflict, err.Error())
		}
		slog.Error("register consumer group failed", "topic", topic, "group", req.ConsumerGroup, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}

	return c.SendStatus(fiber.StatusCreated)
}

func (s *HTTPServer) unregisterConsumerGroup(c *fiber.Ctx) error {
	topic := c.Params("topic")
	group := c.Params("group")

	if err := s.queueService.UnregisterConsumerGroup(c.Context(), topic, group); err != nil {
		if errors.Is(err, queue.ErrConsumerGroupNotFound) {
			return jsonError(c, fiber.StatusNotFound, err.Error())
		}
		slog.Error("unregister consumer group failed", "topic", topic, "group", group, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}

	return c.SendStatus(fiber.StatusNoContent)
}
