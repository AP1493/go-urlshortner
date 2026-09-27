package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AP1493/go-urlshortner/internal/handlers"
	"github.com/AP1493/go-urlshortner/internal/postgres"
	"github.com/AP1493/go-urlshortner/internal/redis"
	"github.com/AP1493/go-urlshortner/internal/routes"
	"github.com/AP1493/go-urlshortner/internal/server"
	"github.com/joho/godotenv"
)

// port is the address the server listens on inside its container or host.
func port() string {
	if v := strings.TrimSpace(os.Getenv("PORT")); v != "" {
		return v
	}
	return "8080"
}

func main() {
	// .env is a local-development convenience. In Docker and on EC2 the values
	// come from the real environment, so a missing file is not an error.
	if err := godotenv.Load(".env"); err != nil {
		fmt.Println("No .env file loaded, using environment variables")
	}

	db, err := postgres.InitPostgres()
	if err != nil {
		fmt.Println("Error initializing Postgres:", err)
		return
	}
	defer db.Close()

	rdb, err := redis.InitRedis()
	if err != nil {
		fmt.Println("Error initializing Redis:", err)
		return
	}
	defer rdb.Close()

	handler := handlers.NewHandler(db.DB(), rdb.Client())

	app := server.NewFiberServer()

	routes.SetupRoutes(app, handler)

	addr := ":" + port()
	errChn := make(chan error)
	go func() {
		errChn <- app.Listen(addr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	select {
	case err := <-errChn:
		if err != nil {
			fmt.Println("Error starting server:", err)
		}
	case sig := <-quit:
		fmt.Println("Received signal:", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(ctx); err != nil {
			fmt.Println("Error shutting down server:", err)
		}
	}
}
