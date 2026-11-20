package database

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DBRouting interface {
	Writer() *pgxpool.Pool
	Reader() *pgxpool.Pool
	Close()
	Ping(ctx context.Context) error
}

type Router struct {
	writerPool *pgxpool.Pool
	readerPool *pgxpool.Pool
	logger     *slog.Logger
	readerOK   atomic.Bool
}

func NewRouter(writerPool, readerPool *pgxpool.Pool, logger *slog.Logger) *Router {
	r := &Router{
		writerPool: writerPool,
		readerPool: readerPool,
		logger:     logger,
	}
	r.readerOK.Store(readerPool != nil)
	return r
}

func (r *Router) Writer() *pgxpool.Pool {
	if r.writerPool == nil {
		return r.readerPool
	}
	return r.writerPool
}

func (r *Router) Reader() *pgxpool.Pool {
	if r.readerPool != nil && r.readerOK.Load() {
		return r.readerPool
	}
	if r.writerPool != nil {
		if r.readerPool != nil && r.logger != nil {
			r.logger.Warn("read replica unavailable, falling back to primary writer pool")
		}
		return r.writerPool
	}
	return r.readerPool
}

func (r *Router) Close() {
	if r.readerPool != nil {
		r.readerPool.Close()
	}
	if r.writerPool != nil {
		r.writerPool.Close()
	}
}

func (r *Router) Ping(ctx context.Context) error {
	var errs []error
	if r.writerPool != nil {
		if err := r.writerPool.Ping(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if r.readerPool != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := r.readerPool.Ping(pingCtx)
		cancel()
		if err != nil {
			r.readerOK.Store(false)
			if r.logger != nil {
				r.logger.Warn("read replica ping failed", slog.String("error", err.Error()))
			}
			errs = append(errs, err)
		} else {
			r.readerOK.Store(true)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	if r.writerPool != nil {
		return nil
	}
	return errors.Join(errs...)
}

func WithSlowQueryLog(logger *slog.Logger, threshold time.Duration, queryName string, fn func(context.Context) error) error {
	if logger == nil {
		return fn(context.Background())
	}
	start := time.Now()
	err := fn(context.Background())
	d := time.Since(start)
	if d > threshold {
		logger.Warn("slow database query detected",
			slog.Duration("duration", d),
			slog.String("query", queryName),
		)
	}
	return err
}
