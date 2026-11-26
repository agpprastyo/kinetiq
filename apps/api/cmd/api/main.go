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
	defer container.DBRouter.Close()
	defer container.RedisClient.Close()

	port := container.Config.Port
	fiberApp := app.SetupWithContainer(container)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("Kinetiq API listening on port %s", port)
	serveErr := app.Serve(ctx, fiberApp, ":"+port, container, app.ServeOptions{
		DrainDelay: container.Config.ShutdownDrainDelay,
		Timeout:    container.Config.ShutdownTimeout,
	})
	if serveErr != nil {
		log.Printf("Server error: %v", serveErr)
	}

	tctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := container.TracerShutdown(tctx); err != nil {
		log.Printf("Error during tracer shutdown: %v", err)
	}
	cancel()
	container.DBPool.Close()
	container.RedisClient.Close()

	if serveErr != nil {
		os.Exit(1)
	}
	log.Println("Server gracefully stopped.")
}
