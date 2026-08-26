package server

import (
	"time"

	"github.com/gofiber/fiber/v2"
)

func NewFiberServer() *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "URL Shortener",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	})
	return app
}
