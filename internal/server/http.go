package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/valyala/fasthttp/fasthttpadaptor"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/config"
	"github.com/Joessst-Dev/queue-ti/internal/queue"
	"github.com/Joessst-Dev/queue-ti/internal/users"
	"golang.org/x/crypto/bcrypt"
)

type HTTPServer struct {
	App          *fiber.App
	queueService *queue.Service
	authConfig   config.AuthConfig
	userStore    *users.Store
}

func NewHTTPServer(qs *queue.Service, authCfg config.AuthConfig, gatherer prometheus.Gatherer, userStore *users.Store) *HTTPServer {
	s := &HTTPServer{
		queueService: qs,
		authConfig:   authCfg,
		userStore:    userStore,
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Content-Type,Authorization",
	}))

	app.Use(func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		slog.Debug("http request",
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.IP(),
		)
		return err
	})

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	promH := fasthttpadaptor.NewFastHTTPHandler(
		promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}),
	)
	app.Get("/metrics", func(c *fiber.Ctx) error {
		promH(c.Context())
		return nil
	})

	var jwtAuth fiber.Handler
	if authCfg.Enabled {
		jwtAuth = internalAuth.JWTMiddleware([]byte(authCfg.JWTSecret))
	} else {
		jwtAuth = func(c *fiber.Ctx) error { return c.Next() }
	}

	api := app.Group("/api")
	api.Get("/auth/status", s.authStatus)
	api.Post("/auth/login", s.handleLogin)
	api.Post("/auth/refresh", jwtAuth, s.handleRefresh)

	userRoutes := api.Group("/users", jwtAuth, s.requireAdmin())
	userRoutes.Get("", s.listUsers)
	userRoutes.Post("", s.createUser)
	userRoutes.Put("/:id", s.updateUser)
	userRoutes.Delete("/:id", s.deleteUser)
	userRoutes.Get("/:id/grants", s.listUserGrants)
	userRoutes.Post("/:id/grants", s.addUserGrant)
	userRoutes.Delete("/:id/grants/:grantId", s.deleteUserGrant)

	api.Get("/messages", jwtAuth, s.requireGrant("read", func(c *fiber.Ctx) string { return c.Query("topic") }), s.listMessages)
	api.Post("/messages", jwtAuth, s.requireGrant("write", func(c *fiber.Ctx) string {
		var peek struct {
			Topic string `json:"topic"`
		}
		_ = json.Unmarshal(c.Body(), &peek)
		return peek.Topic
	}), s.enqueueMessage)
	api.Post("/messages/:id/nack", jwtAuth, s.requireWriteOnMsgTopic(), s.nackMessage)
	api.Post("/messages/:id/requeue", jwtAuth, s.requireWriteOnMsgTopic(), s.requeueMessage)
	api.Get("/stats", jwtAuth, s.requireAdmin(), s.statsHandler)
	api.Get("/topic-configs", jwtAuth, s.requireAdmin(), s.listTopicConfigs)
	api.Put("/topic-configs/:topic", jwtAuth, s.requireAdmin(), s.upsertTopicConfig)
	api.Delete("/topic-configs/:topic", jwtAuth, s.requireAdmin(), s.deleteTopicConfig)
	api.Get("/topic-schemas", jwtAuth, s.requireAdmin(), s.listTopicSchemas)
	api.Put("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.upsertTopicSchema)
	api.Delete("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.deleteTopicSchema)
	api.Get("/topic-schemas/:topic", jwtAuth, s.requireAdmin(), s.getTopicSchema)

	s.App = app
	return s
}


func (s *HTTPServer) authStatus(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"auth_required": s.authConfig.Enabled})
}

// ---------------------------------------------------------------------------
// Permission middleware
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Auth handlers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// User management response types
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// User management handlers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Queue message types and handlers
// ---------------------------------------------------------------------------

type enqueueRequest struct {
	Topic    string            `json:"topic"`
	Payload  string            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

type nackRequest struct {
	Error string `json:"error"`
}

type messageResponse struct {
	ID            string            `json:"id"`
	Topic         string            `json:"topic"`
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
		r := messageResponse{
			ID:            m.ID,
			Topic:         m.Topic,
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
		items = append(items, r)
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

	id, err := s.queueService.Enqueue(c.Context(), req.Topic, []byte(req.Payload), req.Metadata)
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

type topicStatResponse struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type statsResponse struct {
	Topics []topicStatResponse `json:"topics"`
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

// ---------------------------------------------------------------------------
// Topic-config types and handlers
// ---------------------------------------------------------------------------

type topicConfigRequest struct {
	MaxRetries        *int `json:"max_retries"`
	MessageTTLSeconds *int `json:"message_ttl_seconds"`
	MaxDepth          *int `json:"max_depth"`
}

type topicConfigResponse struct {
	Topic             string `json:"topic"`
	MaxRetries        *int   `json:"max_retries,omitempty"`
	MessageTTLSeconds *int   `json:"message_ttl_seconds,omitempty"`
	MaxDepth          *int   `json:"max_depth,omitempty"`
}

type listTopicConfigsResponse struct {
	Items []topicConfigResponse `json:"items"`
}

func toTopicConfigResponse(cfg queue.TopicConfig) topicConfigResponse {
	return topicConfigResponse{
		Topic:             cfg.Topic,
		MaxRetries:        cfg.MaxRetries,
		MessageTTLSeconds: cfg.MessageTTLSeconds,
		MaxDepth:          cfg.MaxDepth,
	}
}

func (s *HTTPServer) listTopicConfigs(c *fiber.Ctx) error {
	configs, err := s.queueService.ListTopicConfigs(c.Context())
	if err != nil {
		slog.Error("list topic configs failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]topicConfigResponse, len(configs))
	for i, cfg := range configs {
		items[i] = toTopicConfigResponse(cfg)
	}
	return c.JSON(listTopicConfigsResponse{Items: items})
}

func (s *HTTPServer) upsertTopicConfig(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if strings.HasSuffix(topic, ".dlq") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "topic name may not end in .dlq"})
	}

	var req topicConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	cfg := queue.TopicConfig{
		Topic:             topic,
		MaxRetries:        req.MaxRetries,
		MessageTTLSeconds: req.MessageTTLSeconds,
		MaxDepth:          req.MaxDepth,
	}
	if err := s.queueService.UpsertTopicConfig(c.Context(), cfg); err != nil {
		slog.Error("upsert topic config failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toTopicConfigResponse(cfg))
}

func (s *HTTPServer) deleteTopicConfig(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if err := s.queueService.DeleteTopicConfig(c.Context(), topic); err != nil {
		if errors.Is(err, queue.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("delete topic config failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Topic-schema types and handlers
// ---------------------------------------------------------------------------

type topicSchemaRequest struct {
	SchemaJSON string `json:"schema_json"`
}

type topicSchemaResponse struct {
	Topic      string `json:"topic"`
	SchemaJSON string `json:"schema_json"`
	Version    int    `json:"version"`
	UpdatedAt  string `json:"updated_at"`
}

type listTopicSchemasResponse struct {
	Items []topicSchemaResponse `json:"items"`
}

func toTopicSchemaResponse(ts queue.TopicSchema) topicSchemaResponse {
	return topicSchemaResponse{
		Topic:      ts.Topic,
		SchemaJSON: ts.SchemaJSON,
		Version:    ts.Version,
		UpdatedAt:  ts.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (s *HTTPServer) listTopicSchemas(c *fiber.Ctx) error {
	schemas, err := queue.ListTopicSchemas(c.Context(), s.queueService.Pool())
	if err != nil {
		slog.Error("list topic schemas failed", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items := make([]topicSchemaResponse, len(schemas))
	for i, ts := range schemas {
		items[i] = toTopicSchemaResponse(ts)
	}
	return c.JSON(listTopicSchemasResponse{Items: items})
}

func (s *HTTPServer) upsertTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")

	var req topicSchemaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	ts, err := queue.UpsertTopicSchema(c.Context(), s.queueService.Pool(), topic, req.SchemaJSON)
	if err != nil {
		if errors.Is(err, queue.ErrInvalidSchema) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		slog.Error("upsert topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(toTopicSchemaResponse(*ts))
}

func (s *HTTPServer) deleteTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")
	if err := queue.DeleteTopicSchema(c.Context(), s.queueService.Pool(), topic); err != nil {
		slog.Error("delete topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (s *HTTPServer) getTopicSchema(c *fiber.Ctx) error {
	topic := c.Params("topic")
	ts, err := queue.GetTopicSchema(c.Context(), s.queueService.Pool(), topic)
	if err != nil {
		slog.Error("get topic schema failed", "topic", topic, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if ts == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "schema not found"})
	}
	return c.JSON(toTopicSchemaResponse(*ts))
}
