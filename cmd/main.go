package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AP1493/go-urlshortner/internal/handlers"
	"github.com/AP1493/go-urlshortner/internal/postgres"
	"github.com/AP1493/go-urlshortner/internal/routes"
	"github.com/AP1493/go-urlshortner/internal/server"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println("Error loading .env file")
	}

	db, err := postgres.InitPostgres()
	if err != nil {
		fmt.Println("Error initializing Postgres:", err)
		return
	}
	defer db.Close()

	handler := handlers.NewHandler(db.DB())

	app := server.NewFiberServer()

	routes.SetupRoutes(app, handler)

	errChn := make(chan error)
	go func() {
		errChn <- app.Listen(":8080")
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
