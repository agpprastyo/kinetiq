package config_test

import (
	"os"
	"testing"

	"kinetiq/internal/config"
)

func TestConfig_Load(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("APP_ENV", "testing")
	os.Setenv("INGESTION_MAX_BATCH_SIZE", "250")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("expected config to load successfully: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.AppEnv != "testing" {
		t.Errorf("expected app_env 'testing', got %s", cfg.AppEnv)
	}
	if cfg.IngestionMaxBatchSize != 250 {
		t.Errorf("expected batch size 250, got %d", cfg.IngestionMaxBatchSize)
	}
	// Verify defaults
	if cfg.LogLevel == "" {
		t.Errorf("expected default log_level, got empty")
	}
}
