package handlers

import "github.com/gofiber/fiber/v2"

// Health reports whether the process can still reach its database. Container
// healthchecks and load balancers use it, so it stays cheap and returns 503
// rather than an error page when the db is gone.
func (h *Handler) Health(c *fiber.Ctx) error {
	sqlDB, err := h.db.DB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "database": "unavailable"})
	}

	if err := sqlDB.PingContext(c.Context()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "database": "unreachable"})
	}

	return c.JSON(fiber.Map{"status": "ok", "database": "ok"})
}

// RedisHealth pings Redis and sends back its reply, "PONG", as plain text. Like
// Health it answers 503 rather than an error page when Redis is unreachable.
func (h *Handler) RedisHealth(c *fiber.Ctx) error {
	pong, err := h.rdb.Ping(c.Context()).Result()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "redis": "unreachable"})
	}

	return c.SendString(pong)
}
