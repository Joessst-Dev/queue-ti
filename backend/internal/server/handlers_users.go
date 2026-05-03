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
	ID            string `json:"id"`
	UserID        string `json:"user_id"`
	Action        string `json:"action"`
	TopicPattern  string `json:"topic_pattern"`
	ConsumerGroup string `json:"consumer_group,omitempty"`
	CreatedAt     string `json:"created_at"`
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

type addConsumerGroupGrantRequest struct {
	TopicPattern  string `json:"topic_pattern"`
	ConsumerGroup string `json:"consumer_group"`
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
		ID:            g.ID,
		UserID:        g.UserID,
		Action:        g.Action,
		TopicPattern:  g.TopicPattern,
		ConsumerGroup: g.ConsumerGroup,
		CreatedAt:     g.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *HTTPServer) listUsers(c *fiber.Ctx) error {
	list, err := s.userStore.List(c.Context())
	if err != nil {
		slog.Error("list users failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
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
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.Username == "" || req.Password == "" {
		return jsonError(c, fiber.StatusBadRequest, "username and password are required")
	}
	if len(req.Password) < 12 {
		return jsonError(c, fiber.StatusBadRequest, "password must be at least 12 characters")
	}
	u, err := s.userStore.Create(c.Context(), req.Username, req.Password, req.IsAdmin)
	if errors.Is(err, users.ErrDuplicate) {
		return jsonError(c, fiber.StatusConflict, err.Error())
	}
	if err != nil {
		slog.Error("create user failed", "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.Status(fiber.StatusCreated).JSON(toUserResponse(u))
}

func (s *HTTPServer) updateUser(c *fiber.Ctx) error {
	id := c.Params("id")
	var req updateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.Password != nil && len(*req.Password) < 12 {
		return jsonError(c, fiber.StatusBadRequest, "password must be at least 12 characters")
	}
	u, err := s.userStore.Update(c.Context(), id, req.Username, req.Password, req.IsAdmin)
	if errors.Is(err, users.ErrNotFound) {
		return jsonError(c, fiber.StatusNotFound, err.Error())
	}
	if errors.Is(err, users.ErrDuplicate) {
		return jsonError(c, fiber.StatusConflict, err.Error())
	}
	if err != nil {
		slog.Error("update user failed", "id", id, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
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
		return jsonError(c, fiber.StatusBadRequest, err.Error())
	}
	if errors.Is(err, users.ErrNotFound) {
		return jsonError(c, fiber.StatusNotFound, err.Error())
	}
	if err != nil {
		slog.Error("delete user failed", "id", id, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *HTTPServer) listUserGrants(c *fiber.Ctx) error {
	userID := c.Params("id")
	grants, err := s.userStore.ListGrants(c.Context(), userID)
	if err != nil {
		slog.Error("list user grants failed", "user_id", userID, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
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
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.Action != "read" && req.Action != "write" && req.Action != "admin" {
		return jsonError(c, fiber.StatusBadRequest, "action must be one of: read, write, admin")
	}
	topicPattern := req.TopicPattern
	if topicPattern == "" {
		topicPattern = "*"
	}
	g, err := s.userStore.AddGrant(c.Context(), userID, req.Action, topicPattern)
	if err != nil {
		slog.Error("add user grant failed", "user_id", userID, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.Status(fiber.StatusCreated).JSON(toGrantResponse(g))
}

func (s *HTTPServer) addConsumerGroupGrant(c *fiber.Ctx) error {
	userID := c.Params("id")
	var req addConsumerGroupGrantRequest
	if err := c.BodyParser(&req); err != nil {
		return jsonError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if req.ConsumerGroup == "" {
		return jsonError(c, fiber.StatusBadRequest, "consumer_group is required")
	}
	topicPattern := req.TopicPattern
	if topicPattern == "" {
		topicPattern = "*"
	}
	g, err := s.userStore.AddConsumerGroupGrant(c.Context(), userID, topicPattern, req.ConsumerGroup)
	if errors.Is(err, users.ErrDuplicate) {
		return jsonError(c, fiber.StatusConflict, err.Error())
	}
	if err != nil {
		slog.Error("add consumer group grant failed", "user_id", userID, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.Status(fiber.StatusCreated).JSON(toGrantResponse(g))
}

func (s *HTTPServer) deleteUserGrant(c *fiber.Ctx) error {
	userID := c.Params("id")
	grantID := c.Params("grantId")
	err := s.userStore.DeleteGrant(c.Context(), grantID, userID)
	if errors.Is(err, users.ErrNotFound) {
		return jsonError(c, fiber.StatusNotFound, err.Error())
	}
	if err != nil {
		slog.Error("delete user grant failed", "user_id", userID, "grant_id", grantID, "error", err)
		return jsonError(c, fiber.StatusInternalServerError, "internal server error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
