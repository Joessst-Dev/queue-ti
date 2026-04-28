package server

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

func (s *HTTPServer) handleLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil || req.Username == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "username and password are required"})
	}
	user, hash, err := s.userStore.GetByUsername(c.Context(), req.Username)
	if errors.Is(err, users.ErrNotFound) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid credentials"})
	}
	token, err := users.IssueToken([]byte(s.authConfig.JWTSecret), user.ID, user.Username, user.IsAdmin)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to issue token"})
	}
	return c.JSON(tokenResponse{Token: token})
}

func (s *HTTPServer) handleRefresh(c *fiber.Ctx) error {
	claims := internalAuth.ClaimsFromCtx(c)
	if claims == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "not authenticated"})
	}
	user, err := s.userStore.GetByID(c.Context(), claims.UserID)
	if errors.Is(err, users.ErrNotFound) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "user no longer exists"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "internal error"})
	}
	token, err := users.IssueToken([]byte(s.authConfig.JWTSecret), user.ID, user.Username, user.IsAdmin)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to issue token"})
	}
	return c.JSON(tokenResponse{Token: token})
}
