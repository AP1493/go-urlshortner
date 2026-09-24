package routes

import (
	"github.com/AP1493/go-urlshortner/internal/handlers"
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App, handler *handlers.Handler) {
	app.Get("/healthz", handler.Health)

	// Server-rendered pages.
	app.Get("/", handler.Home)
	app.Post("/", handler.ShortenForm)

	shorten := app.Group("/shorten")
	shorten.Post("/", handler.Create)
	shorten.Get("/:shorten_code", handler.Get)
	shorten.Get("/:shorten_code/stats", handler.Stats)
	shorten.Put("/:shorten_code", handler.Update)
	shorten.Delete("/:shorten_code", handler.Delete)
}
