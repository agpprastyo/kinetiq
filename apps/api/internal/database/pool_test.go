package database

import (
	"testing"
	"time"
)

func TestPoolConfig(t *testing.T) {
	cfg, err := PoolConfig("postgres://u@localhost:5432/d?sslmode=disable", 25, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 25 {
		t.Errorf("max conns=%d", cfg.MaxConns)
	}
	if got := cfg.ConnConfig.RuntimeParams["statement_timeout"]; got != "15000" {
		t.Errorf("statement_timeout=%q", got)
	}
	cfg, _ = PoolConfig("postgres://u@localhost:5432/d", 0, 0)
	if _, ok := cfg.ConnConfig.RuntimeParams["statement_timeout"]; ok {
		t.Errorf("zero timeout must not set statement_timeout")
	}
	if _, err := PoolConfig("::bad::", 1, 0); err == nil {
		t.Errorf("want parse error")
	}
}
