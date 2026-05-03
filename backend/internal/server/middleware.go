package server

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	internalAuth "github.com/Joessst-Dev/queue-ti/internal/auth"
	"github.com/Joessst-Dev/queue-ti/internal/users"
)

// Middleware error message constants — centralised here so that any change to
// the wording is applied consistently across all middleware functions and tests
// that assert on these strings.
const (
	errMsgAuthRequired        = "authentication required"
	errMsgAdminRequired       = "admin access required"
	errMsgInsufficientPerms   = "insufficient permissions"
	errMsgFailedCheckPerms    = "failed to check permissions"
	errMsgMessageNotFound     = "message not found"
)

// checkAuthAndAdmin is the shared preamble for every middleware in this file.
// It returns (done=true, err) when the middleware should stop processing:
//   - done=true, err=401 response  — auth is enabled but caller has no claims
//   - done=true, err=nil           — caller is an admin; skip further grant checks
//
// When done=false the caller must continue with its own grant logic.
func (s *HTTPServer) checkAuthAndAdmin(c *fiber.Ctx) (done bool, err error) {
	claims := internalAuth.ClaimsFromCtx(c)
	if s.authConfig.Enabled && claims == nil {
		return true, jsonError(c, fiber.StatusUnauthorized, errMsgAuthRequired)
	}
	if claims == nil {
		// Auth disabled — pass through unconditionally.
		return true, c.Next()
	}
	if claims.IsAdmin {
		return true, c.Next()
	}
	return false, nil
}

func (s *HTTPServer) requireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := s.checkAuthAndAdmin(c); done {
			return err
		}
		return jsonError(c, fiber.StatusForbidden, errMsgAdminRequired)
	}
}

func (s *HTTPServer) requireGrant(action string, topicFn func(*fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := s.checkAuthAndAdmin(c); done {
			return err
		}
		claims := internalAuth.ClaimsFromCtx(c)
		topic := topicFn(c)
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, errMsgFailedCheckPerms)
		}
		if !users.HasGrant(grants, action, topic) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		return c.Next()
	}
}

func (s *HTTPServer) requireWriteOnMsgTopic() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := s.checkAuthAndAdmin(c); done {
			return err
		}
		claims := internalAuth.ClaimsFromCtx(c)
		id := c.Params("id")
		topic, err := s.queueService.TopicForMessage(c.Context(), id)
		if err != nil {
			return jsonError(c, fiber.StatusNotFound, errMsgMessageNotFound)
		}
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, errMsgFailedCheckPerms)
		}
		if !users.HasGrant(grants, "write", topic) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		return c.Next()
	}
}

// requireDequeueGrant checks dequeue access (write or consume) on a topic,
// and optionally consumer-group access when a group is present. extractFn is
// called once to parse both values, avoiding redundant body unmarshalling.
func (s *HTTPServer) requireDequeueGrant(extractFn func(*fiber.Ctx) (string, string)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := s.checkAuthAndAdmin(c); done {
			return err
		}
		claims := internalAuth.ClaimsFromCtx(c)
		topic, group := extractFn(c)
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, errMsgFailedCheckPerms)
		}
		if !users.HasDequeueAccess(grants, topic) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		if group != "" && !users.HasConsumerGroupAccess(grants, topic, group) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		return c.Next()
	}
}

// requireDequeueOnMsgTopic is like requireWriteOnMsgTopic but checks dequeue
// access (write or consume) and additionally validates consumer-group access
// when "consumer_group" is present in the request body.
func (s *HTTPServer) requireDequeueOnMsgTopic() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if done, err := s.checkAuthAndAdmin(c); done {
			return err
		}
		claims := internalAuth.ClaimsFromCtx(c)
		id := c.Params("id")
		topic, err := s.queueService.TopicForMessage(c.Context(), id)
		if err != nil {
			return jsonError(c, fiber.StatusNotFound, errMsgMessageNotFound)
		}
		grants, err := s.userStore.GetUserGrants(c.Context(), claims.UserID)
		if err != nil {
			return jsonError(c, fiber.StatusInternalServerError, errMsgFailedCheckPerms)
		}
		if !users.HasDequeueAccess(grants, topic) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		var peek struct {
			ConsumerGroup string `json:"consumer_group"`
		}
		// c.Body() is idempotent in Fiber; the handler will re-parse safely.
		_ = json.Unmarshal(c.Body(), &peek)
		if peek.ConsumerGroup != "" && !users.HasConsumerGroupAccess(grants, topic, peek.ConsumerGroup) {
			return jsonError(c, fiber.StatusForbidden, errMsgInsufficientPerms)
		}
		return c.Next()
	}
}
