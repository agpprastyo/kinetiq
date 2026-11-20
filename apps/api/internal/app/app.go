package app

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"

	_ "kinetiq/docs/swagger"
	"kinetiq/internal/auth"
	"kinetiq/internal/observability"
)

// @title Kinetiq Analytics API
// @version 1.0
// @description High-performance Real-Time Product Analytics API
// @host localhost:8080
// @BasePath /
func SetupWithContainer(c *Container) *fiber.App {
	observability.InitLogger()
	app := fiber.New()

	// CORS configuration for web dashboard
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:5173, http://127.0.0.1:5173",
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, Cookie, X-Request-ID, traceparent",
		AllowCredentials: true,
		AllowMethods:     "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
	}))

	// Global tracing middleware (Request ID & W3C traceparent)
	app.Use(observability.TraceMiddleware())

	// Swagger UI route
	app.Get("/swagger/*", swagger.HandlerDefault)

	// Health & Metrics Endpoints
	app.Get("/healthz", func(ctx *fiber.Ctx) error {
		return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "alive",
		})
	})

	app.Get("/readyz", func(ctx *fiber.Ctx) error {
		cctx, cancel := context.WithTimeout(ctx.UserContext(), 2*time.Second)
		defer cancel()

		// 1. Check PostgreSQL connection
		if err := c.DBPool.Ping(cctx); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "unavailable",
				"error":  "database unavailable: " + err.Error(),
			})
		}

		// 2. Check Redis connection
		if err := c.RedisClient.Ping(cctx).Err(); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "unavailable",
				"error":  "redis unavailable: " + err.Error(),
			})
		}

		return ctx.Status(fiber.StatusOK).JSON(fiber.Map{
			"status": "ready",
			"services": fiber.Map{
				"postgres": "connected",
				"redis":    "connected",
			},
		})
	})

	token := ""
	if c.Config != nil {
		token = c.Config.MetricsAuthToken
	}
	app.Get("/metrics", observability.MetricsHandler(token))

	api := app.Group("/api/v1")
	api.Get("/health", func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{"status": "ok"})
	})
	api.Get("/healthz", func(ctx *fiber.Ctx) error {
		return ctx.JSON(fiber.Map{"status": "alive"})
	})
	api.Get("/readyz", func(ctx *fiber.Ctx) error {
		cctx, cancel := context.WithTimeout(ctx.UserContext(), 2*time.Second)
		defer cancel()
		if err := c.DBPool.Ping(cctx); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable", "error": err.Error()})
		}
		if err := c.RedisClient.Ping(cctx).Err(); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable", "error": err.Error()})
		}
		return ctx.JSON(fiber.Map{"status": "ready"})
	})

	// Public ingestion endpoint authenticated by Project API Key
	api.Post("/ingest", c.IngestionHandler.IngestEvents)

	// Protected browser session endpoints
	protected := api.Group("", auth.SessionMiddleware(c.AuthClient))
	protected.Post("/projects", c.ProjectHandler.CreateProject)
	protected.Get("/projects", c.ProjectHandler.ListProjects)
	protected.Get("/projects/:projectID", c.ProjectHandler.GetProject)
	protected.Patch("/projects/:projectID", c.ProjectHandler.UpdateProject)
	protected.Delete("/projects/:projectID", c.ProjectHandler.DeleteProject)
	protected.Post("/projects/:projectID/purge-events", c.ProjectHandler.PurgeEvents)
	protected.Post("/projects/:projectID/rotate-key", c.ProjectHandler.RotateKey)
	protected.Get("/projects/:projectID/keys", c.ProjectHandler.ListAPIKeys)
	protected.Post("/projects/:projectID/keys", c.ProjectHandler.CreateAPIKey)
	protected.Delete("/projects/:projectID/keys/:keyID", c.ProjectHandler.RevokeAPIKey)

	// Analytics query endpoints
	protected.Get("/projects/:projectID/analytics/summary", c.AnalyticsHandler.GetSummary)
	protected.Get("/projects/:projectID/analytics/top-events", c.AnalyticsHandler.GetTopEvents)
	protected.Get("/projects/:projectID/analytics/volume", c.AnalyticsHandler.GetVolume)
	protected.Get("/projects/:projectID/analytics/funnel", c.AnalyticsHandler.GetFunnel)
	protected.Get("/projects/:projectID/analytics/breakdown", c.AnalyticsHandler.GetPropertyBreakdown)
	protected.Get("/projects/:projectID/analytics/breakdown/devices", c.AnalyticsHandler.GetDeviceBreakdown)
	protected.Get("/projects/:projectID/analytics/breakdown/geo", c.AnalyticsHandler.GetGeoBreakdown)
	protected.Get("/projects/:projectID/analytics/retention", c.AnalyticsHandler.GetRetention)
	protected.Get("/projects/:projectID/analytics/users", c.AnalyticsHandler.ListUsers)
	protected.Get("/projects/:projectID/analytics/users/:distinctId/timeline", c.AnalyticsHandler.GetUserTimeline)

	// Real-time live SSE stream
	protected.Get("/projects/:projectID/live", c.LiveHandler.StreamLiveEvents)

	// Data export & Automated Cloud Storage Sync endpoints
	protected.Get("/projects/:projectID/export", c.ExportHandler.ExportEvents)
	protected.Get("/projects/:projectID/storage-sync", c.ExportHandler.GetStorageSync)
	protected.Put("/projects/:projectID/storage-sync", c.ExportHandler.PutStorageSync)
	protected.Post("/projects/:projectID/storage-sync/test", c.ExportHandler.TestStorageSync)

	// Custom insight widget builder endpoints
	if c.InsightHandler != nil {
		protected.Get("/projects/:projectID/insights", c.InsightHandler.ListInsights)
		protected.Post("/projects/:projectID/insights", c.InsightHandler.CreateInsight)
		protected.Put("/projects/:projectID/insights/:insightId", c.InsightHandler.UpdateInsight)
		protected.Delete("/projects/:projectID/insights/:insightId", c.InsightHandler.DeleteInsight)
	}

	// Automated Intelligence & Digest endpoints
	if c.IntelligenceHandler != nil {
		protected.Get("/projects/:projectID/intelligence/digest", c.IntelligenceHandler.GetWeeklyDigest)
		protected.Get("/projects/:projectID/intelligence/anomaly", c.IntelligenceHandler.CheckAnomaly)
		protected.Post("/intelligence/dispatch-digests", c.IntelligenceHandler.TriggerDigestDispatch)
	}

	// Alert rules endpoints
	if c.AlertingHandler != nil {
		protected.Get("/projects/:projectID/alerts", c.AlertingHandler.ListRules)
		protected.Post("/projects/:projectID/alerts", c.AlertingHandler.CreateRule)
		protected.Put("/projects/:projectID/alerts/:ruleID", c.AlertingHandler.UpdateRule)
		protected.Delete("/projects/:projectID/alerts/:ruleID", c.AlertingHandler.DeleteRule)
		protected.Post("/projects/:projectID/alerts/:ruleID/test", c.AlertingHandler.TestRule)
		protected.Post("/projects/:projectID/alerts/:ruleID/snooze", c.AlertingHandler.SnoozeRule)
	}

	// Workspace compliance & audit trail endpoints
	if c.AuditHandler != nil {
		protected.Get("/workspace/audit-logs", c.AuditHandler.ListWorkspaceAuditLogs)
		protected.Delete("/projects/:projectID/compliance/users/:distinctID", c.AuditHandler.DeleteUserData)
	}

	// Project retention policy & lifecycle endpoints
	if c.RetentionHandler != nil {
		protected.Get("/projects/:projectID/retention", c.RetentionHandler.GetPolicy)
		protected.Put("/projects/:projectID/retention", c.RetentionHandler.UpdatePolicy)
		protected.Get("/projects/:projectID/retention/preview", c.RetentionHandler.Preview)
		protected.Post("/projects/:projectID/retention/run", c.RetentionHandler.RunPurge)
	}

	// Protected billing & quota endpoints

	// Protected Ops Mission Control & DLQ endpoints (Air-gapped platform staff only)
	opsGroup := protected.Group("/ops", auth.RequirePlatformStaff())

	if c.DLQHandler != nil {
		opsGroup.Get("/dlq", auth.RequirePlatformStaff("ops"), c.DLQHandler.ListDeadLetters)
		opsGroup.Post("/dlq/replay/:id", auth.RequirePlatformStaff("ops"), c.DLQHandler.ReplayMessage)
		opsGroup.Post("/dlq/replay-all", auth.RequirePlatformStaff("ops"), c.DLQHandler.ReplayAllMessages)
		opsGroup.Delete("/dlq/:id", auth.RequirePlatformStaff("ops"), c.DLQHandler.PurgeMessage)
		opsGroup.Delete("/dlq", auth.RequirePlatformStaff("ops"), c.DLQHandler.PurgeAllMessages)
	}

	if c.MailerHandler != nil {
		opsGroup.Get("/mailer/metrics", auth.RequirePlatformStaff("ops", "support"), c.MailerHandler.GetMetrics)
	}

	return app
}

func Setup() *fiber.App {
	container, err := NewContainer(context.Background())
	if err != nil {
		panic(err)
	}
	return SetupWithContainer(container)
}
