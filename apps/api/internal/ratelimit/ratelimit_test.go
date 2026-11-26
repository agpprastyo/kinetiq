package ratelimit

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func testRedis(t *testing.T) *redis.Client {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/0"
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unavailable: %v", err)
	}
	return rdb
}

func TestAllowProjectEnforcesPerSecondLimit(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	l := New(rdb, func(context.Context, uuid.UUID) (int, error) { return 5, nil })
	pid := uuid.New()

	if ok, _ := l.AllowProject(ctx, pid, 5); !ok {
		t.Fatal("5 events within limit 5 must pass")
	}
	ok, retry := l.AllowProject(ctx, pid, 1)
	if ok {
		t.Fatal("6th event must be rejected")
	}
	if retry <= 0 || retry > time.Second {
		t.Fatalf("retry must be within the 1s window, got %v", retry)
	}
	if ok, _ := l.AllowProject(ctx, uuid.New(), 1); !ok {
		t.Fatal("limits must be per project")
	}
}

func TestBadKeyThrottlePerIP(t *testing.T) {
	rdb := testRedis(t)
	ctx := context.Background()
	l := New(rdb, nil)
	ip := "203.0.113." + uuid.NewString()[:4]
	for i := 0; i < BadKeyMax-1; i++ {
		l.RecordBadKey(ctx, ip)
	}
	if blocked, _ := l.IPBlocked(ctx, ip); blocked {
		t.Fatal("must not block below threshold")
	}
	l.RecordBadKey(ctx, ip)
	blocked, retry := l.IPBlocked(ctx, ip)
	if !blocked || retry <= 0 {
		t.Fatalf("must block at threshold, got %v %v", blocked, retry)
	}
	if blocked, _ := l.IPBlocked(ctx, ip+"x"); blocked {
		t.Fatal("throttle must be per IP")
	}
}
