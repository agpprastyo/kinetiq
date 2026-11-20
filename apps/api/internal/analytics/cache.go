package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"kinetiq/internal/observability"
)

type CachedAnalyticsService struct {
	underlying Service
	redis      *redis.Client
	logger     *slog.Logger
	ttl        time.Duration
}

func NewCachedAnalyticsService(underlying Service, redisClient *redis.Client, logger *slog.Logger) *CachedAnalyticsService {
	return &CachedAnalyticsService{
		underlying: underlying,
		redis:      redisClient,
		logger:     logger,
		ttl:        45 * time.Second, // short TTL between 30-60s
	}
}

func (c *CachedAnalyticsService) cacheKey(projectID uuid.UUID, endpoint, rawParams string) string {
	h := sha256.Sum256([]byte(rawParams))
	hashStr := hex.EncodeToString(h[:8]) // first 16 hex chars
	return fmt.Sprintf("cache:analytics:%s:%s:%s", projectID.String(), endpoint, hashStr)
}

func (c *CachedAnalyticsService) GetSummary(ctx context.Context, projectID uuid.UUID, from, to time.Time) (*SummaryResponse, error) {
	key := c.cacheKey(projectID, "summary", fmt.Sprintf("from=%s&to=%s", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp SummaryResponse
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return &resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetSummary(ctx, projectID, from, to)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetTopEvents(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) (*TopEventsResponse, error) {
	key := c.cacheKey(projectID, "top-events", fmt.Sprintf("from=%s&to=%s&limit=%d", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), limit))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp TopEventsResponse
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return &resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetTopEvents(ctx, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetVolume(ctx context.Context, projectID uuid.UUID, from, to time.Time, interval string) (*VolumeResponse, error) {
	key := c.cacheKey(projectID, "volume", fmt.Sprintf("from=%s&to=%s&interval=%s", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), interval))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp VolumeResponse
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return &resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetVolume(ctx, projectID, from, to, interval)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetFunnel(ctx context.Context, projectID uuid.UUID, from, to time.Time, steps []string) (*FunnelResponse, error) {
	key := c.cacheKey(projectID, "funnel", fmt.Sprintf("from=%s&to=%s&steps=%v", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), steps))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp FunnelResponse
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return &resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetFunnel(ctx, projectID, from, to, steps)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetPropertyBreakdown(ctx context.Context, projectID uuid.UUID, eventName, property string, from, to time.Time, limit int) ([]PropertyBreakdownItem, error) {
	key := c.cacheKey(projectID, "breakdown", fmt.Sprintf("event=%s&prop=%s&from=%s&to=%s&limit=%d", eventName, property, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), limit))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp []PropertyBreakdownItem
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetPropertyBreakdown(ctx, projectID, eventName, property, from, to, limit)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetRetention(ctx context.Context, projectID uuid.UUID, from, to time.Time, firstEvent, returnEvent, granularity string) (*RetentionResponse, error) {
	key := c.cacheKey(projectID, "retention", fmt.Sprintf("first=%s&return=%s&granularity=%s&from=%s&to=%s", firstEvent, returnEvent, granularity, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp RetentionResponse
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return &resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetRetention(ctx, projectID, from, to, firstEvent, returnEvent, granularity)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) ListUsers(ctx context.Context, projectID uuid.UUID, search string, limit, offset int) ([]UserExplorerItem, error) {
	return c.underlying.ListUsers(ctx, projectID, search, limit, offset)
}

func (c *CachedAnalyticsService) GetUserTimeline(ctx context.Context, projectID uuid.UUID, distinctID string, limit int, before *time.Time) ([]UserTimelineItem, error) {
	return c.underlying.GetUserTimeline(ctx, projectID, distinctID, limit, before)
}

func (c *CachedAnalyticsService) GetDeviceBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time) ([]DeviceBreakdownItem, error) {
	key := c.cacheKey(projectID, "breakdown_devices", fmt.Sprintf("from=%s&to=%s", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339)))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp []DeviceBreakdownItem
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetDeviceBreakdown(ctx, projectID, from, to)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}

func (c *CachedAnalyticsService) GetGeoBreakdown(ctx context.Context, projectID uuid.UUID, from, to time.Time, limit int) ([]GeoBreakdownItem, error) {
	key := c.cacheKey(projectID, "breakdown_geo", fmt.Sprintf("from=%s&to=%s&limit=%d", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339), limit))

	if c.redis != nil {
		if val, err := c.redis.Get(ctx, key).Result(); err == nil {
			var resp []GeoBreakdownItem
			if err := json.Unmarshal([]byte(val), &resp); err == nil {
				observability.DefaultMetrics.CacheHit()
				return resp, nil
			}
		}
	}
	observability.DefaultMetrics.CacheMiss()

	resp, err := c.underlying.GetGeoBreakdown(ctx, projectID, from, to, limit)
	if err != nil {
		return nil, err
	}

	if c.redis != nil {
		if b, err := json.Marshal(resp); err == nil {
			_ = c.redis.Set(ctx, key, string(b), c.ttl).Err()
		}
	}

	return resp, nil
}
