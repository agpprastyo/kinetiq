package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kinetiq/internal/app"
)

func main() {
	container, err := app.NewContainer(context.Background())
	if err != nil {
		log.Fatalf("failed to initialize dependencies container: %v", err)
	}
	defer container.DBPool.Close()
	defer container.RedisClient.Close()

	port := container.Config.Port
	fiberApp := app.SetupWithContainer(container)

	// Graceful shutdown channel
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Kinetiq API listening on port %s", port)
		if err := fiberApp.Listen(":" + port); err != nil {
			log.Printf("Server shut down error: %v", err)
		}
	}()

	<-shutdown
	log.Println("Shutting down API server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := fiberApp.ShutdownWithContext(ctx); err != nil {
		log.Printf("Error during server shutdown: %v", err)
	}
	if err := container.TracerShutdown(ctx); err != nil {
		log.Printf("Error during tracer shutdown: %v", err)
	}

	log.Println("Server gracefully stopped.")
}
