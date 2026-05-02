package server

import "github.com/gofiber/fiber/v2"

// jsonError writes a JSON error response with the given HTTP status code.
// It is the single canonical way to return an error from any HTTP handler,
// eliminating the repeated c.Status(N).JSON(fiber.Map{"error": "..."}) pattern.
func jsonError(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// parseLimitOffset extracts and clamps the "limit" and "offset" query parameters.
// limit is clamped to [1, maxLimit] and defaults to defaultLimit when absent or
// zero. offset is clamped to [0, ∞). Invalid values are silently clamped rather
// than rejected, matching the existing handler behaviour.
func parseLimitOffset(c *fiber.Ctx, defaultLimit, maxLimit int) (limit, offset int) {
	limit = c.QueryInt("limit", defaultLimit)
	if limit < 1 {
		limit = 1
	} else if limit > maxLimit {
		limit = maxLimit
	}
	offset = max(c.QueryInt("offset", 0), 0)
	return limit, offset
}
