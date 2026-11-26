package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
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
	defer container.DBRouter.Close()
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

	var bg sync.WaitGroup

	// Liveness endpoint for Kubernetes (the worker serves no other HTTP)
	healthSrv := &http.Server{Addr: ":" + container.Config.WorkerHealthPort, Handler: worker.HealthHandler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			container.Logger.Error("worker health server failed", slog.String("error", err.Error()))
		}
	}()

	// Start background retention sweeper (runs hourly, purges events older than 90 days)
	retentionWG := worker.StartRetentionSweeper(ctx, container.DBPool, container.Logger, 1*time.Hour, 90)
	bg.Go(retentionWG.Wait)

	// Start Phase 10 Automated Intelligence Schedulers (Anomaly sweep & Weekly email digest)
	if container.IntelligenceService != nil {
		schedWG := worker.StartIntelligenceSchedulers(ctx, container.IntelligenceService, container.Logger)
		bg.Go(schedWG.Wait)
	}

	container.Logger.Info("Kinetiq consumer worker running...")
	consumer.Start(ctx)

	// Wait (bounded) for schedulers; the consumer loop already returned after acking its last batch.
	timeout := container.Config.ShutdownTimeout
	if !worker.WaitTimeout(&bg, timeout) {
		container.Logger.Warn("background jobs did not finish before shutdown timeout", slog.Duration("timeout", timeout))
	}
	sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	_ = healthSrv.Shutdown(sctx)
	scancel()
	container.Logger.Info("Kinetiq consumer worker stopped cleanly.")
}
