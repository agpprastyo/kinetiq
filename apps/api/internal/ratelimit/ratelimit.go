// Package ratelimit implements Redis-backed fixed-window counters used to
// throttle public ingestion by project and to slow API-key guessing by IP.
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	// BadKeyMax invalid-API-key attempts per BadKeyWindow before an IP is blocked.
	BadKeyMax    = 20
	BadKeyWindow = time.Minute

	defaultProjectLimit = 500
	limitCacheTTL       = 30 * time.Second
)

// hit INCRBYs a counter, starts the window on first hit and returns count + remaining TTL (ms).
var hit = redis.NewScript(`
local c = redis.call('INCRBY', KEYS[1], ARGV[1])
if c == tonumber(ARGV[1]) then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end
return {c, redis.call('PTTL', KEYS[1])}
`)

// LimitLookup returns the per-second event limit of a project.
type LimitLookup func(ctx context.Context, projectID uuid.UUID) (int, error)

type Limiter struct {
	rdb    *redis.Client
	lookup LimitLookup
}

func New(rdb *redis.Client, lookup LimitLookup) *Limiter {
	return &Limiter{rdb: rdb, lookup: lookup}
}

func retry(pttlMs int64) time.Duration {
	if pttlMs < 1 {
		return time.Second
	}
	return time.Duration(pttlMs) * time.Millisecond
}

// AllowProject counts n events against the project's per-second limit.
// Fails open on backend errors so a Redis blip never drops customer telemetry.
func (l *Limiter) AllowProject(ctx context.Context, projectID uuid.UUID, n int) (bool, time.Duration) {
	limit := defaultProjectLimit
	if l.lookup != nil {
		if v, err := l.lookup(ctx, projectID); err == nil && v > 0 {
			limit = v
		}
	}
	res, err := hit.Run(ctx, l.rdb, []string{"rl:proj:" + projectID.String()}, n, 1000).Int64Slice()
	if err != nil || len(res) != 2 {
		return true, 0
	}
	if res[0] > int64(limit) {
		return false, retry(res[1])
	}
	return true, 0
}

// IPBlocked reports whether ip exceeded BadKeyMax invalid-key attempts in the current window.
func (l *Limiter) IPBlocked(ctx context.Context, ip string) (bool, time.Duration) {
	key := "rl:badkey:" + ip
	v, err := l.rdb.Get(ctx, key).Result()
	if err != nil {
		return false, 0
	}
	if c, _ := strconv.Atoi(v); c >= BadKeyMax {
		ttl, _ := l.rdb.PTTL(ctx, key).Result()
		return true, retry(ttl.Milliseconds())
	}
	return false, 0
}

// RecordBadKey registers one failed API-key attempt for ip.
func (l *Limiter) RecordBadKey(ctx context.Context, ip string) {
	_, _ = hit.Run(ctx, l.rdb, []string{"rl:badkey:" + ip}, 1, BadKeyWindow.Milliseconds()).Result()
}

// ProjectLimitLookup reads projects.rate_limit_per_sec, cached briefly in Redis.
func ProjectLimitLookup(pool *pgxpool.Pool, rdb *redis.Client) LimitLookup {
	return func(ctx context.Context, id uuid.UUID) (int, error) {
		key := "rl:cfg:" + id.String()
		if v, err := rdb.Get(ctx, key).Int(); err == nil {
			return v, nil
		}
		var v int
		if err := pool.QueryRow(ctx, `SELECT COALESCE(rate_limit_per_sec, $2) FROM projects WHERE id = $1`, id, defaultProjectLimit).Scan(&v); err != nil {
			return 0, fmt.Errorf("lookup rate limit: %w", err)
		}
		_ = rdb.Set(ctx, key, v, limitCacheTTL).Err()
		return v, nil
	}
}
