package config_test

import (
	"testing"

	"kinetiq/internal/config"
)

func TestNewRedisClient_Standalone(t *testing.T) {
	cfg := &config.Config{RedisURL: "redis://:pw@cache:6380/2"}
	rdb, err := cfg.NewRedisClient()
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()
	o := rdb.Options()
	if o.Addr != "cache:6380" || o.DB != 2 || o.Password != "pw" {
		t.Errorf("unexpected standalone opts: addr=%s db=%d", o.Addr, o.DB)
	}
}

func TestNewRedisClient_Sentinel(t *testing.T) {
	t.Setenv("REDIS_SENTINEL_ADDRS", "s1:26379, s2:26379,s3:26379")
	t.Setenv("REDIS_SENTINEL_MASTER", "kinetiq")
	t.Setenv("REDIS_URL", "redis://:pw@ignored:6379/3")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RedisSentinelAddrs) != 3 || cfg.RedisSentinelMaster != "kinetiq" {
		t.Fatalf("sentinel env not parsed: %v %q", cfg.RedisSentinelAddrs, cfg.RedisSentinelMaster)
	}
	rdb, err := cfg.NewRedisClient()
	if err != nil {
		t.Fatal(err)
	}
	defer rdb.Close()
	o := rdb.Options()
	if o.Addr != "FailoverClient" || o.DB != 3 || o.Password != "pw" {
		t.Errorf("expected failover client inheriting db/password from REDIS_URL, got addr=%s db=%d", o.Addr, o.DB)
	}
}

func TestNewRedisClient_SentinelMissingMaster(t *testing.T) {
	cfg := &config.Config{RedisURL: "redis://localhost:6379/0", RedisSentinelAddrs: []string{"s1:26379"}}
	if _, err := cfg.NewRedisClient(); err == nil {
		t.Fatal("expected error when REDIS_SENTINEL_MASTER is empty")
	}
}
