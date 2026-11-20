package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kinetiq/internal/app"
	"kinetiq/internal/worker"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	container, err := app.NewContainer(ctx)
	if err != nil {
		log.Fatalf("failed to initialize dependencies container: %v", err)
	}
	defer container.DBPool.Close()
	defer container.RedisClient.Close()

	workerCfg := worker.DefaultConfig()
	workerCfg.MaxRetries = int32(container.Config.QueueMaxRetries)

	consumer, err := worker.NewConsumerWorker(
		container.DBPool,
		container.RedisClient,
		container.Logger,
		workerCfg,
	)
	if err != nil {
		container.Logger.Error("failed to create consumer worker", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if container.AlertingService != nil {
		consumer.SetAlertingService(container.AlertingService)
	}

	// Handle graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdown
		container.Logger.Info("received shutdown signal, stopping worker loop...")
		cancel()
	}()

	// Start background retention sweeper (runs hourly, purges events older than 90 days)
	worker.StartRetentionSweeper(ctx, container.DBPool, container.Logger, 1*time.Hour, 90)

	// Start Phase 10 Automated Intelligence Schedulers (Anomaly sweep & Weekly email digest)
	if container.IntelligenceService != nil {
		worker.StartIntelligenceSchedulers(ctx, container.IntelligenceService, container.Logger)
	}

	container.Logger.Info("Kinetiq consumer worker running...")
	consumer.Start(ctx)

	// Allow pending operations time to finish safely
	time.Sleep(500 * time.Millisecond)
	container.Logger.Info("Kinetiq consumer worker stopped cleanly.")
}
