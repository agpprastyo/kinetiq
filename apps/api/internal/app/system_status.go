package app

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"kinetiq/internal/ingestion"
)

type PostgresStatus struct {
	Status          string `json:"status"`
	LatencyMs       int64  `json:"latency_ms"`
	OpenConnections int32  `json:"open_connections"`
}

type RedisStatus struct {
	Status          string `json:"status"`
	LatencyMs       int64  `json:"latency_ms"`
	UsedMemoryHuman string `json:"used_memory_human"`
}

type QueueStatus struct {
	StreamLength    int64 `json:"stream_length"`
	PendingPelCount int64 `json:"pending_pel_count"`
	ConsumerLag     int64 `json:"consumer_lag"`
}

type IngestionStatus struct {
	EventsPerSec1m float64 `json:"events_per_sec_1m"`
	EventsTotal24h int64   `json:"events_total_24h"`
}

type SystemStatusResponse struct {
	Postgres  PostgresStatus  `json:"postgres"`
	Redis     RedisStatus     `json:"redis"`
	Queue     QueueStatus     `json:"queue"`
	Ingestion IngestionStatus `json:"ingestion"`
}

// collectSystemStatus gathers infrastructure health, Redis stream queue metrics, and ingestion rates.
// Partial failures are reported per-component as "degraded" without panicking.
func collectSystemStatus(ctx context.Context, dbPool *pgxpool.Pool, redisClient *redis.Client) SystemStatusResponse {
	res := SystemStatusResponse{
		Postgres: PostgresStatus{Status: "degraded"},
		Redis:    RedisStatus{Status: "degraded", UsedMemoryHuman: "unknown"},
	}

	if dbPool != nil {
		start := time.Now()
		if err := dbPool.Ping(ctx); err == nil {
			res.Postgres.Status = "healthy"
			res.Postgres.LatencyMs = time.Since(start).Milliseconds()
			if stat := dbPool.Stat(); stat != nil {
				res.Postgres.OpenConnections = stat.TotalConns()
			}

			var count1m, count24h int64
			err := dbPool.QueryRow(ctx, `
				SELECT
					COUNT(*) FILTER (WHERE timestamp >= NOW() - INTERVAL '1 minute'),
					COUNT(*) FILTER (WHERE timestamp >= NOW() - INTERVAL '24 hours')
				FROM events
			`).Scan(&count1m, &count24h)
			if err == nil {
				res.Ingestion.EventsTotal24h = count24h
				res.Ingestion.EventsPerSec1m = math.Round((float64(count1m)/60.0)*100) / 100
			}
		} else {
			res.Postgres.LatencyMs = time.Since(start).Milliseconds()
		}
	}

	if redisClient != nil {
		start := time.Now()
		if err := redisClient.Ping(ctx).Err(); err == nil {
			res.Redis.Status = "healthy"
			res.Redis.LatencyMs = time.Since(start).Milliseconds()

			if info, err := redisClient.Info(ctx, "memory").Result(); err == nil {
				if mem := parseRedisUsedMemoryHuman(info); mem != "" {
					res.Redis.UsedMemoryHuman = mem
				}
			}

			if xlen, err := redisClient.XLen(ctx, ingestion.StreamKey).Result(); err == nil {
				res.Queue.StreamLength = xlen
			}
			if groups, err := redisClient.XInfoGroups(ctx, ingestion.StreamKey).Result(); err == nil {
				for _, g := range groups {
					res.Queue.PendingPelCount += g.Pending
					res.Queue.ConsumerLag += g.Lag
				}
			}
		} else {
			res.Redis.LatencyMs = time.Since(start).Milliseconds()
		}
	}

	return res
}

func parseRedisUsedMemoryHuman(info string) string {
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "used_memory_human:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "used_memory_human:"))
		}
	}
	return ""
}

func systemStatusHandler(c *Container) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		cctx, cancel := context.WithTimeout(ctx.UserContext(), 3*time.Second)
		defer cancel()
		status := collectSystemStatus(cctx, c.DBPool, c.RedisClient)
		return ctx.Status(fiber.StatusOK).JSON(status)
	}
}
