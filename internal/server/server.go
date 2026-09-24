package server

import (
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func NewFiberServer() *fiber.App {
	// Templates live on disk next to the binary; Reload picks up edits without a
	// restart while developing.
	engine := html.New("./views", ".html")
	engine.Reload(os.Getenv("APP_ENV") == "dev")

	app := fiber.New(fiber.Config{
		AppName:      "URL Shortener",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Views:        engine,
		ViewsLayout:  "layouts/main",
	})
	return app
}
