package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/swagger"
	"github.com/valyala/fasthttp"

	_ "kinetiq/docs/swagger"
	"kinetiq/internal/analytics"
	"kinetiq/internal/auth"
	"kinetiq/internal/config"
	"kinetiq/internal/observability"
	"kinetiq/internal/response"
)

const corsHeaders = "Origin, Content-Type, Accept, Authorization, Cookie, X-Request-ID, traceparent"

// publicIngestPaths are authenticated by project API key (Bearer), never by cookie,
// so they accept any origin without credentials (browser SDK on customer sites).
// Decision: per-project allowed_origins is advisory metadata, not enforced here,
// because an Origin header is trivially forged by non-browser clients.
var publicIngestPaths = map[string]bool{
	"/api/v1/ingest":   true,
	"/api/v1/identify": true,
	"/api/v1/alias":    true,
}

// newFiberConfig derives client IP strictly from the configured trusted proxies:
// c.IP() ignores forwarded headers unless the socket peer is in TRUSTED_PROXIES.
func newFiberConfig(cfg *config.Config) fiber.Config {
	fc := fiber.Config{}
	if cfg == nil {
		return fc
	}
	fc.EnableTrustedProxyCheck = true
	fc.TrustedProxies = cfg.TrustedProxies
	fc.ProxyHeader = cfg.ProxyHeader
	fc.EnableIPValidation = true
	return fc
}

// registerCORS: public ingestion = any origin, no credentials; everything else =
// credentialed CORS restricted to WEB_ORIGIN.
func registerCORS(app *fiber.App, cfg *config.Config) {
	origins := []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	if cfg != nil {
		origins = cfg.AllowedWebOrigins()
	}
	app.Use(cors.New(cors.Config{
		Next:         func(c *fiber.Ctx) bool { return !publicIngestPaths[c.Path()] },
		AllowOrigins: "*",
		AllowHeaders: corsHeaders,
		AllowMethods: "POST, OPTIONS",
	}))
	dashboard := cors.Config{
		Next:             func(c *fiber.Ctx) bool { return publicIngestPaths[c.Path()] },
		AllowHeaders:     corsHeaders,
		AllowCredentials: true,
		AllowMethods:     "GET, POST, HEAD, PUT, DELETE, PATCH, OPTIONS",
	}
	if len(origins) > 0 {
		dashboard.AllowOrigins = strings.Join(origins, ",")
	} else {
		dashboard.AllowOriginsFunc = func(string) bool { return false }
	}
	app.Use(cors.New(dashboard))
}

var errCodes = map[int]string{
	fiber.StatusNotFound:              "NOT_FOUND",
	fiber.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
	fiber.StatusRequestEntityTooLarge: "PAYLOAD_TOO_LARGE",
	fiber.StatusRequestTimeout:        "REQUEST_TIMEOUT",
}

// errorHandler renders every framework/unhandled error in the standard envelope.
func errorHandler(ctx *fiber.Ctx, err error) error {
	var fe *fiber.Error
	if !errors.As(err, &fe) {
		return response.Error(ctx, fiber.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
	if fe.Code >= fiber.StatusInternalServerError {
		return response.Error(ctx, fe.Code, "INTERNAL_ERROR", "Internal server error")
	}
	code, ok := errCodes[fe.Code]
	if !ok {
		code = "BAD_REQUEST"
	}
	return response.Error(ctx, fe.Code, code, fe.Message)
}

// sseWriteTimeout lifts the server write deadline for long-lived SSE streams.
const sseWriteTimeout = 24 * time.Hour

func headerReceived(h *fasthttp.RequestHeader) fasthttp.RequestConfig {
	uri := string(h.RequestURI())
	if strings.HasSuffix(uri, "/live") || strings.Contains(uri, "/live?") {
		return fasthttp.RequestConfig{WriteTimeout: sseWriteTimeout}
	}
	return fasthttp.RequestConfig{}
}

// @title Kinetiq Analytics API
// @version 1.0
// @description High-performance Real-Time Product Analytics API
// @host localhost:8080
// @BasePath /
func SetupWithContainer(c *Container) *fiber.App {
	observability.InitLogger()
	fc := newFiberConfig(c.Config)
	fc.ErrorHandler = errorHandler
	if c.Config != nil {
		fc.BodyLimit = int(c.Config.IngestionMaxBodyBytes)
		fc.ReadTimeout = c.Config.HTTPReadTimeout
		fc.WriteTimeout = c.Config.HTTPWriteTimeout
		fc.IdleTimeout = c.Config.HTTPIdleTimeout
	}
	app := fiber.New(fc)
	app.Server().HeaderReceived = headerReceived
	app.Use(recover.New())
	registerCORS(app, c.Config)

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

	registerReadiness(app, c)

	token := ""
	if c.Config != nil {
		token = c.Config.MetricsAuthToken
	}
	app.Get("/metrics", observability.MetricsHandler(token))

	api := app.Group("/api/v1")
	// Public ingestion endpoint authenticated by Project API Key
	api.Post("/ingest", c.IngestionHandler.IngestEvents)
	api.Post("/identify", c.PersonHandler.Identify)
	api.Post("/alias", c.PersonHandler.Alias)

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

	// Analytics query endpoints (bounded by a per-request deadline -> 504 QUERY_TIMEOUT)
	qt := analytics.QueryTimeout(20 * time.Second)
	if c.Config != nil && c.Config.AnalyticsQueryTimeout > 0 {
		qt = analytics.QueryTimeout(c.Config.AnalyticsQueryTimeout)
	}
	protected.Get("/projects/:projectID/analytics/summary", qt, c.AnalyticsHandler.GetSummary)
	protected.Get("/projects/:projectID/analytics/top-events", qt, c.AnalyticsHandler.GetTopEvents)
	protected.Get("/projects/:projectID/analytics/volume", qt, c.AnalyticsHandler.GetVolume)
	protected.Get("/projects/:projectID/analytics/funnel", qt, c.AnalyticsHandler.GetFunnel)
	protected.Get("/projects/:projectID/analytics/funnel/dropoff", qt, c.AnalyticsHandler.GetFunnelDropoff)
	protected.Get("/projects/:projectID/analytics/breakdown", qt, c.AnalyticsHandler.GetPropertyBreakdown)
	protected.Get("/projects/:projectID/analytics/breakdown/devices", qt, c.AnalyticsHandler.GetDeviceBreakdown)
	protected.Get("/projects/:projectID/analytics/breakdown/geo", qt, c.AnalyticsHandler.GetGeoBreakdown)
	protected.Get("/projects/:projectID/analytics/retention", qt, c.AnalyticsHandler.GetRetention)
	protected.Get("/projects/:projectID/analytics/sessions", qt, c.AnalyticsHandler.GetSessions)
	protected.Get("/projects/:projectID/catalog/events", qt, c.AnalyticsHandler.ListEvents)
	protected.Get("/projects/:projectID/catalog/properties", qt, c.AnalyticsHandler.ListProperties)
	protected.Get("/projects/:projectID/catalog/property-values", qt, c.AnalyticsHandler.ListPropertyValues)
	protected.Get("/projects/:projectID/analytics/users", qt, c.AnalyticsHandler.ListUsers)
	protected.Get("/projects/:projectID/analytics/users/:distinctId/timeline", qt, c.AnalyticsHandler.GetUserTimeline)

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

	if c.IntelligenceHandler != nil {
		opsGroup.Post("/intelligence/dispatch-digests", auth.RequirePlatformStaff("ops"), c.IntelligenceHandler.TriggerDigestDispatch)
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

// registerReadiness serves /readyz and /api/v1/readyz: 503 while draining, else DB+Redis ping.
func registerReadiness(app *fiber.App, c *Container) {
	ready := func(ctx *fiber.Ctx) error {
		if c.Draining() {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "draining"})
		}
		cctx, cancel := context.WithTimeout(ctx.UserContext(), 2*time.Second)
		defer cancel()
		if err := c.DBPool.Ping(cctx); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable", "error": "database unavailable: " + err.Error()})
		}
		if err := c.RedisClient.Ping(cctx).Err(); err != nil {
			return ctx.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "unavailable", "error": "redis unavailable: " + err.Error()})
		}
		return ctx.JSON(fiber.Map{"status": "ready", "services": fiber.Map{"postgres": "connected", "redis": "connected"}})
	}
	app.Get("/readyz", ready)
}

// Drainer is the shutdown hook surface of Container.
type Drainer interface {
	BeginDrain()
	StopStreams()
}

type ServeOptions struct {
	DrainDelay time.Duration
	Timeout    time.Duration
}

// Serve listens on addr until ctx is cancelled, then: readiness 503, wait DrainDelay,
// close SSE streams, shut down within Timeout. Returns listen errors (bind failure)
// so callers exit non-zero.
func Serve(ctx context.Context, app *fiber.App, addr string, d Drainer, opt ServeOptions) error {
	listenErr := make(chan error, 1)
	go func() { listenErr <- app.Listen(addr) }()

	select {
	case err := <-listenErr:
		return err
	case <-ctx.Done():
	}

	d.BeginDrain()
	select {
	case <-time.After(opt.DrainDelay):
	case err := <-listenErr:
		return err
	}
	d.StopStreams()

	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := app.ShutdownWithContext(sctx); err != nil {
		return err
	}
	return <-listenErr
}
