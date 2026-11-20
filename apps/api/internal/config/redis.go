package config

import (
	"errors"
	"strings"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient returns a Sentinel failover client when REDIS_SENTINEL_ADDRS is set,
// otherwise a standalone client from REDIS_URL.
func (c *Config) NewRedisClient() (*redis.Client, error) {
	opts, err := redis.ParseURL(c.RedisURL)
	if err != nil {
		return nil, err
	}

	var addrs []string
	for _, a := range c.RedisSentinelAddrs {
		if a = strings.TrimSpace(a); a != "" {
			addrs = append(addrs, a)
		}
	}
	if len(addrs) == 0 {
		return redis.NewClient(opts), nil
	}
	if c.RedisSentinelMaster == "" {
		return nil, errors.New("REDIS_SENTINEL_MASTER is required when REDIS_SENTINEL_ADDRS is set")
	}

	return redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    c.RedisSentinelMaster,
		SentinelAddrs: addrs,
		Username:      opts.Username,
		Password:      opts.Password,
		DB:            opts.DB,
	}), nil
}
