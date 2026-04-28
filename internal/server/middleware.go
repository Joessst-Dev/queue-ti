package server

import (
	"github.com/gofiber/fiber/v2"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

func (s *HTTPServer) requireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := internalAuth.ClaimsFromCtx(c)
		if claims == nil {
			return c.Next()
		}
		if claims.IsAdmin {
			return c.Next()
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "admin access required"})
	}
}

func (s *HTTPServer) requireGrant(action string, topicFn func(*fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := internalAuth.ClaimsFromCtx(c)
		if claims == nil {
			return c.Next()
		}
		if claims.IsAdmin {
			return c.Next()
		}
		topic := topicFn(c)
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check permissions"})
		}
		if !users.HasGrant(grants, action, topic) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
		}
		return c.Next()
	}
}

func (s *HTTPServer) requireWriteOnMsgTopic() fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims := internalAuth.ClaimsFromCtx(c)
		if claims == nil {
			return c.Next()
		}
		if claims.IsAdmin {
			return c.Next()
		}
		id := c.Params("id")
		topic, err := s.queueService.TopicForMessage(c.Context(), id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "message not found"})
		}
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to check permissions"})
		}
		if !users.HasGrant(grants, "write", topic) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "insufficient permissions"})
		}
		return c.Next()
	}
}
