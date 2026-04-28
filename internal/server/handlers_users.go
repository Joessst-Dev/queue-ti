package server

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

type userResponse struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	IsAdmin   bool   `json:"is_admin"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type grantResponse struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	Action       string `json:"action"`
	TopicPattern string `json:"topic_pattern"`
	CreatedAt    string `json:"created_at"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IsAdmin  bool   `json:"is_admin"`
}

type updateUserRequest struct {
	Username *string `json:"username"`
	Password *string `json:"password"`
	IsAdmin  *bool   `json:"is_admin"`
}

type addGrantRequest struct {
	Action       string `json:"action"`
	TopicPattern string `json:"topic_pattern"`
}

func toUserResponse(u *users.User) userResponse {
	return userResponse{
		ID:        u.ID,
		Username:  u.Username,
		IsAdmin:   u.IsAdmin,
		CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: u.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func toGrantResponse(g *users.Grant) grantResponse {
	return grantResponse{
		ID:           g.ID,
		UserID:       g.UserID,
		Action:       g.Action,
		TopicPattern: g.TopicPattern,
		CreatedAt:    g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *HTTPServer) listUsers(c *fiber.Ctx) error {
	list, err := s.userStore.List(c.Context())
	if err != nil {
		slog.Error("list users failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]userResponse, len(list))
	for i, u := range list {
		items[i] = toUserResponse(&u)
	}
	return c.JSON(fiber.Map{"items": items})
}

func (s *HTTPServer) createUser(c *fiber.Ctx) error {
	var req createUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Username == "" || req.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "username and password are required"})
	}
	u, err := s.userStore.Create(c.Context(), req.Username, req.Password, req.IsAdmin)
	if errors.Is(err, users.ErrDuplicate) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		slog.Error("create user failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(toUserResponse(u))
}

func (s *HTTPServer) updateUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var req updateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	u, err := s.userStore.Update(c.Context(), id, req.Username, req.Password, req.IsAdmin)
	if errors.Is(err, users.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, users.ErrDuplicate) {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		slog.Error("update user failed", "id", id, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toUserResponse(u))
}

func (s *HTTPServer) deleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var callerID string
	if claims := internalAuth.ClaimsFromCtx(c); claims != nil {
		callerID = claims.UserID
	}
	err := s.userStore.Delete(c.Context(), id, callerID)
	if errors.Is(err, users.ErrCannotDeleteSelf) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if errors.Is(err, users.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		slog.Error("delete user failed", "id", id, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *HTTPServer) listUserGrants(c *fiber.Ctx) error {
	userID := c.Params("id")
	grants, err := s.userStore.ListGrants(c.Context(), userID)
	if err != nil {
		slog.Error("list user grants failed", "user_id", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]grantResponse, len(grants))
	for i, g := range grants {
		items[i] = toGrantResponse(&g)
	}
	return c.JSON(fiber.Map{"items": items})
}

func (s *HTTPServer) addUserGrant(c *fiber.Ctx) error {
	userID := c.Params("id")
	var req addGrantRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if req.Action != "read" && req.Action != "write" && req.Action != "admin" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "action must be one of: read, write, admin"})
	}
	topicPattern := req.TopicPattern
	if topicPattern == "" {
		topicPattern = "*"
	}
	g, err := s.userStore.AddGrant(c.Context(), userID, req.Action, topicPattern)
	if err != nil {
		slog.Error("add user grant failed", "user_id", userID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(toGrantResponse(g))
}

func (s *HTTPServer) deleteUserGrant(c *fiber.Ctx) error {
	userID := c.Params("id")
	grantID := c.Params("grantId")
	err := s.userStore.DeleteGrant(c.Context(), grantID, userID)
	if errors.Is(err, users.ErrNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		slog.Error("delete user grant failed", "user_id", userID, "grant_id", grantID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
