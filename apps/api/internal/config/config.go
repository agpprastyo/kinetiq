package config

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	AppEnv string `env:"APP_ENV" envDefault:"development"`
	Port   string `env:"PORT" envDefault:"8080"`

	// Shutdown: readiness flips to 503 on SIGTERM, then DrainDelay lets the load balancer
	// deregister the pod, then in-flight work gets ShutdownTimeout. Keep the sum below the
	// pod terminationGracePeriodSeconds (30s in Helm).
	ShutdownDrainDelay time.Duration `env:"SHUTDOWN_DRAIN_DELAY" envDefault:"5s"`
	ShutdownTimeout    time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"20s"`
	WorkerHealthPort   string        `env:"WORKER_HEALTH_PORT" envDefault:"8081"`

	DatabaseURL            string `env:"DATABASE_URL" envDefault:"postgres://kinetiq:kinetiq@localhost:5432/kinetiq?sslmode=disable"`
	DatabaseReadReplicaURL string `env:"DATABASE_READ_REPLICA_URL" envDefault:""`
	DBMaxOpenConns         int    `env:"DB_MAX_OPEN_CONNS" envDefault:"25"`
	DBMaxIdleConns         int    `env:"DB_MAX_IDLE_CONNS" envDefault:"10"`
	DBSlowQueryThresholdMs int64  `env:"DB_SLOW_QUERY_THRESHOLD_MS" envDefault:"200"`
	// DBStatementTimeout applies to the analytics/reader pool; must fire before AnalyticsQueryTimeout.
	DBStatementTimeout    time.Duration `env:"DB_STATEMENT_TIMEOUT" envDefault:"15s"`
	AnalyticsQueryTimeout time.Duration `env:"ANALYTICS_QUERY_TIMEOUT" envDefault:"20s"`

	HTTPReadTimeout  time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"15s"`
	HTTPWriteTimeout time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"60s"`
	HTTPIdleTimeout  time.Duration `env:"HTTP_IDLE_TIMEOUT" envDefault:"120s"`

	RedisURL string `env:"REDIS_URL" envDefault:"redis://localhost:6379/0"`
	// Redis Sentinel (HA). When RedisSentinelAddrs is set, the client connects via
	// Sentinel and auto-follows failover; db/username/password come from REDIS_URL.
	RedisSentinelAddrs  []string `env:"REDIS_SENTINEL_ADDRS" envSeparator:","`
	RedisSentinelMaster string   `env:"REDIS_SENTINEL_MASTER" envDefault:""`

	AuthBaseURL        string `env:"AUTH_BASE_URL" envDefault:"http://localhost:3001"`
	AuthInternalSecret string `env:"AUTH_INTERNAL_SECRET" envDefault:"test-internal-secret"`

	PublicAppURL string `env:"PUBLIC_APP_URL" envDefault:"http://localhost:5173"`
	// WebOrigin is a comma-separated list of dashboard origins allowed (with credentials) by CORS.
	WebOrigin string `env:"WEB_ORIGIN" envDefault:"http://localhost:5173"`

	// TrustedProxies lists proxy IPs/CIDRs whose ProxyHeader is honoured for client IP.
	// Empty (default) = never trust forwarded headers; c.IP() is the socket peer.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`
	ProxyHeader    string   `env:"PROXY_HEADER" envDefault:"X-Forwarded-For"`

	IngestionMaxBodyBytes     int64 `env:"INGESTION_MAX_BODY_BYTES" envDefault:"262144"`
	IngestionMaxBatchSize     int   `env:"INGESTION_MAX_BATCH_SIZE" envDefault:"100"`
	IngestionMaxPropertyBytes int64 `env:"INGESTION_MAX_PROPERTY_BYTES" envDefault:"16384"`
	QueueMaxRetries           int   `env:"QUEUE_MAX_RETRIES" envDefault:"5"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"debug"`

	// Observability / OpenTelemetry
	OtelExporterOtlpEndpoint string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:""`
	OtelServiceName          string  `env:"OTEL_SERVICE_NAME" envDefault:"kinetiq-api"`
	OtelTracesSamplerArg     float64 `env:"OTEL_TRACES_SAMPLER_ARG" envDefault:"0.1"`
	MetricsAuthToken         string  `env:"METRICS_AUTH_TOKEN" envDefault:""`
}

func Load() (*Config, error) {
	// Attempt to load .env from repository root or current dir (ignore error if not present in prod/docker)
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")

	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// AllowedWebOrigins returns the parsed WEB_ORIGIN list.
func (c *Config) AllowedWebOrigins() []string {
	var out []string
	for _, o := range strings.Split(c.WebOrigin, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// Validate refuses wildcard credentialed CORS origins and, in production, default secrets.
func (c *Config) Validate() error {
	for _, o := range c.AllowedWebOrigins() {
		if strings.Contains(o, "*") {
			return errors.New("WEB_ORIGIN must list explicit origins; wildcards are not allowed with credentials")
		}
	}
	if c.AppEnv != "production" {
		return nil
	}
	switch c.AuthInternalSecret {
	case "", "test-internal-secret":
		return errors.New("AUTH_INTERNAL_SECRET must be set to a non-default value in production")
	}
	return nil
}

// LogSafeConfig prints non-sensitive configuration keys for diagnostics on startup
func (c *Config) LogSafeConfig(logger *slog.Logger) {
	if logger == nil {
		return
	}
	logger.Info("application configuration loaded",
		slog.String("app_env", c.AppEnv),
		slog.String("port", c.Port),
		slog.String("auth_base_url", c.AuthBaseURL),
		slog.Int64("max_body_bytes", c.IngestionMaxBodyBytes),
		slog.Int("max_batch_size", c.IngestionMaxBatchSize),
		slog.Int("db_max_open_conns", c.DBMaxOpenConns),
		slog.Int("db_max_idle_conns", c.DBMaxIdleConns),
		slog.Int64("db_slow_query_threshold_ms", c.DBSlowQueryThresholdMs),
		slog.Bool("redis_sentinel_enabled", len(c.RedisSentinelAddrs) > 0),
		slog.Bool("db_read_replica_enabled", c.DatabaseReadReplicaURL != ""),
		slog.Bool("otel_enabled", c.OtelExporterOtlpEndpoint != ""),
		slog.Bool("metrics_auth_enabled", c.MetricsAuthToken != ""),
	)
}
