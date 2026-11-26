package app

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	db "kinetiq/db/sqlc"
	"kinetiq/internal/alerting"
	"kinetiq/internal/analytics"
	"kinetiq/internal/auth"
	"kinetiq/internal/compliance"
	"kinetiq/internal/config"
	"kinetiq/internal/database"
	"kinetiq/internal/dlq"
	"kinetiq/internal/export"
	"kinetiq/internal/ingestion"
	"kinetiq/internal/insight"
	"kinetiq/internal/intelligence"
	"kinetiq/internal/live"
	"kinetiq/internal/mailer"
	"kinetiq/internal/observability"
	"kinetiq/internal/person"
	"kinetiq/internal/project"
	"kinetiq/internal/ratelimit"
	"kinetiq/internal/retention"
	"kinetiq/internal/validator"
)

type Container struct {
	Config              *config.Config
	Logger              *slog.Logger
	DBPool              *pgxpool.Pool
	DBRouter            database.DBRouting
	RedisClient         *redis.Client
	AuthClient          auth.Client
	APIKeyValidator     auth.APIKeyValidator
	Validator           *validator.CustomValidator
	ProjectRepo         project.Repository
	ProjectService      project.Service
	ProjectHandler      *project.Handler
	IngestionService    ingestion.Service
	IngestionHandler    *ingestion.Handler
	PersonHandler       *person.Handler
	AnalyticsService    analytics.Service
	AnalyticsHandler    *analytics.Handler
	LiveHandler         *live.Handler
	ExportHandler       *export.Handler
	AlertingRepo        alerting.Repository
	AlertingService     alerting.Service
	AlertingHandler     *alerting.Handler
	InsightRepo         insight.Repository
	InsightService      insight.Service
	InsightHandler      *insight.Handler
	IntelligenceService intelligence.Service
	IntelligenceHandler *intelligence.Handler
	DLQService          dlq.Service
	DLQHandler          *dlq.Handler
	MailerDispatcher    *mailer.Dispatcher
	MailerCleaner       *mailer.Cleaner
	MailerHandler       *mailer.Handler
	AuditService        compliance.Service
	AuditHandler        *compliance.Handler
	RetentionService    retention.Service
	RetentionHandler    *retention.Handler
	TracerShutdown      func(context.Context) error

	draining   atomic.Bool
	streamOnce sync.Once
	streamCtx  context.Context
	stopStream context.CancelFunc
}

func NewContainer(ctx context.Context) (*Container, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	logger := observability.NewLogger()
	cfg.LogSafeConfig(logger)

	tracerShutdown, err := observability.InitTracer(ctx, cfg.OtelServiceName, cfg.OtelExporterOtlpEndpoint, cfg.OtelTracesSamplerArg, cfg.OtelExporterOtlpEndpoint != "")
	if err != nil {
		return nil, err
	}

	poolCfg, err := database.PoolConfig(cfg.DatabaseURL, cfg.DBMaxOpenConns, 0)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	// Reader pool (replica, or a second pool on the primary) carries statement_timeout so
	// analytics/export queries are bounded server-side; writer pool stays unrestricted.
	readerURL := cfg.DatabaseReadReplicaURL
	if readerURL == "" {
		readerURL = cfg.DatabaseURL
	}
	var readerPool *pgxpool.Pool
	if readerCfg, rerr := database.PoolConfig(readerURL, cfg.DBMaxOpenConns, cfg.DBStatementTimeout); rerr == nil {
		readerPool, err = pgxpool.NewWithConfig(ctx, readerCfg)
	} else {
		err = rerr
	}
	if err != nil {
		logger.Warn("failed to create reader pool, falling back to primary", slog.String("error", err.Error()))
		readerPool = nil
	}

	dbRouter := database.NewRouter(pool, readerPool, logger)

	rdb, err := cfg.NewRedisClient()
	if err != nil {
		return nil, err
	}

	v := validator.New()
	authClient := auth.NewHTTPClient(cfg.AuthBaseURL, cfg.AuthInternalSecret)
	queries := db.New(pool)
	keyValidator := auth.NewProjectKeyValidator(queries, rdb, logger)

	projectRepo := project.NewSQLCRepository(dbRouter.Writer(), logger)
	projectService := project.NewProjectService(projectRepo, logger)
	projectHandler := project.NewHandler(projectService, v, logger)

	alertingRepo := alerting.NewSQLRepository(dbRouter.Writer())
	alertingDispatcher := alerting.NewDispatcher()
	alertingService := alerting.NewService(alertingRepo, alertingDispatcher, rdb, logger)
	alertingHandler := alerting.NewHandler(alertingService, projectService, logger)

	insightRepo := insight.NewRepository(dbRouter.Reader())
	insightService := insight.NewService(insightRepo, logger)
	insightHandler := insight.NewHandler(insightService, projectService, v, logger)

	intelligenceService := intelligence.NewService(dbRouter.Reader(), rdb, alertingService, logger)
	intelligenceHandler := intelligence.NewHandler(intelligenceService, projectService, logger)

	ingestionService := ingestion.NewIngestionService(rdb, logger)
	ingestionHandler := ingestion.NewHandler(ingestionService, keyValidator, v, logger, cfg.IngestionMaxBatchSize, cfg.IngestionMaxPropertyBytes)

	personHandler := person.NewHandler(person.NewService(dbRouter.Writer()), keyValidator)

	limiter := ratelimit.New(rdb, ratelimit.ProjectLimitLookup(dbRouter.Writer(), rdb))
	ingestionHandler.SetLimiter(limiter)
	personHandler.SetLimiter(limiter)

	baseAnalyticsService := analytics.NewAnalyticsService(dbRouter.Reader(), queries, logger)
	cachedAnalyticsService := analytics.NewCachedAnalyticsService(baseAnalyticsService, rdb, logger)
	analyticsHandler := analytics.NewHandler(cachedAnalyticsService, projectService, logger)
	liveHandler := live.NewHandler(rdb, projectService, logger)
	exportHandler := export.NewHandler(dbRouter.Reader(), projectService, logger)

	dlqService := dlq.NewService(dbRouter.Writer(), rdb, logger)
	dlqHandler := dlq.NewHandler(dlqService, logger)

	mailerSender := mailer.NewConsoleSender(logger)
	mailerDispatcher := mailer.NewDispatcher(dbRouter.Writer(), mailerSender, logger, 5*time.Second)
	mailerCleaner := mailer.NewCleaner(dbRouter.Writer(), logger, 30*24*time.Hour, 12*time.Hour)
	mailerHandler := mailer.NewHandler(mailerDispatcher, mailerCleaner, logger)

	auditService := compliance.NewAuditService(dbRouter.Writer(), logger)
	projectAccess := func(ctx context.Context, userID string, projectID uuid.UUID) (string, error) {
		p, err := projectService.Get(ctx, userID, projectID)
		if err != nil || p == nil {
			return "", project.ErrProjectNotFound
		}
		return p.Role, nil
	}
	auditHandler := compliance.NewHandler(auditService, projectAccess, logger)
	projectHandler.SetAuditService(auditService)

	retentionService := retention.NewRetentionService(dbRouter.Writer(), logger)
	retentionHandler := retention.NewHandler(retentionService, projectAccess, logger)

	c := &Container{
		TracerShutdown:      tracerShutdown,
		Config:              cfg,
		Logger:              logger,
		DBPool:              pool,
		DBRouter:            dbRouter,
		RedisClient:         rdb,
		AuthClient:          authClient,
		APIKeyValidator:     keyValidator,
		Validator:           v,
		ProjectRepo:         projectRepo,
		ProjectService:      projectService,
		ProjectHandler:      projectHandler,
		IngestionService:    ingestionService,
		IngestionHandler:    ingestionHandler,
		PersonHandler:       personHandler,
		AnalyticsService:    cachedAnalyticsService,
		AnalyticsHandler:    analyticsHandler,
		LiveHandler:         liveHandler,
		ExportHandler:       exportHandler,
		AlertingRepo:        alertingRepo,
		AlertingService:     alertingService,
		AlertingHandler:     alertingHandler,
		InsightRepo:         insightRepo,
		InsightService:      insightService,
		InsightHandler:      insightHandler,
		IntelligenceService: intelligenceService,
		IntelligenceHandler: intelligenceHandler,
		DLQService:          dlqService,
		DLQHandler:          dlqHandler,
		MailerDispatcher:    mailerDispatcher,
		MailerCleaner:       mailerCleaner,
		MailerHandler:       mailerHandler,
		AuditService:        auditService,
		AuditHandler:        auditHandler,
		RetentionService:    retentionService,
		RetentionHandler:    retentionHandler,
	}
	liveHandler.SetShutdown(c.StreamContext().Done())
	return c, nil
}

// BeginDrain makes /readyz answer 503 so the load balancer stops routing here.
func (c *Container) BeginDrain() { c.draining.Store(true) }

// Draining reports whether shutdown has started.
func (c *Container) Draining() bool { return c.draining.Load() }

// StreamContext is cancelled by StopStreams; long-lived responses (SSE) watch it.
func (c *Container) StreamContext() context.Context {
	c.streamOnce.Do(func() { c.streamCtx, c.stopStream = context.WithCancel(context.Background()) })
	return c.streamCtx
}

// StopStreams ends open SSE streams so server shutdown is not held by them.
func (c *Container) StopStreams() {
	c.StreamContext()
	c.stopStream()
}
