package auth

import (
	"log/slog"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Joessst-Dev/queue-ti/internal/users"
)

func JWTMiddleware(jwtSecret []byte) fiber.Handler {
	return func(c *fiber.Ctx) error {
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			slog.Warn("jwt auth: missing authorization header", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "missing authorization header"})
		}
		if !strings.HasPrefix(authHeader, "Bearer ") {
			slog.Warn("jwt auth: unsupported auth scheme", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unsupported auth scheme"})
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := users.ParseToken(jwtSecret, tokenStr)
		if err != nil {
			slog.Warn("jwt auth: invalid token", "ip", c.IP(), "path", c.Path(), "error", err)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired token"})
		}
		c.Locals("claims", claims)
		return c.Next()
	}
}

func ClaimsFromCtx(c *fiber.Ctx) *users.Claims {
	v := c.Locals("claims")
	if v == nil {
		return nil
	}
	claims, _ := v.(*users.Claims)
	return claims
}
