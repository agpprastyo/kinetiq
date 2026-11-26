package database

import (
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig parses url and applies pool size and a server-side statement_timeout.
// Zero values keep pgx defaults.
func PoolConfig(url string, maxConns int, statementTimeout time.Duration) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if maxConns > 0 {
		cfg.MaxConns = int32(maxConns)
	}
	if statementTimeout > 0 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(statementTimeout.Milliseconds(), 10)
	}
	return cfg, nil
}
