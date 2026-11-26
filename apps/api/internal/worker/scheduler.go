package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"kinetiq/internal/intelligence"
)

// StartIntelligenceSchedulers starts background cron routines for anomaly sweeps and weekly digests
func StartIntelligenceSchedulers(ctx context.Context, intelSvc intelligence.Service, logger *slog.Logger) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	if intelSvc == nil {
		return wg
	}
	wg.Add(2)

	// 1. Hourly anomaly detection sweep across active alert rules
	anomalyTicker := time.NewTicker(30 * time.Minute)
	go func() {
		defer wg.Done()
		defer anomalyTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-anomalyTicker.C:
				anomalies, err := intelSvc.RunAnomalySweep(ctx)
				if err != nil && logger != nil {
					logger.WarnContext(ctx, "failed anomaly detection sweep", slog.String("error", err.Error()))
				} else if len(anomalies) > 0 && logger != nil {
					logger.WarnContext(ctx, "traffic anomaly sweep detected drops", slog.Int("detected_count", len(anomalies)))
				}
			}
		}
	}()

	// 2. Periodic check for weekly digest dispatch (runs daily, checks if Monday UTC)
	digestTicker := time.NewTicker(6 * time.Hour)
	go func() {
		defer wg.Done()
		defer digestTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-digestTicker.C:
				now := time.Now().UTC()
				if now.Weekday() == time.Monday && now.Hour() >= 8 && now.Hour() <= 14 {
					count, err := intelSvc.DispatchWeeklyDigests(ctx)
					if err != nil && logger != nil {
						logger.ErrorContext(ctx, "failed weekly digest dispatch", slog.String("error", err.Error()))
					} else if count > 0 && logger != nil {
						logger.InfoContext(ctx, "weekly performance digests dispatched", slog.Int("count", count))
					}
				}
			}
		}
	}()
	return wg
}
