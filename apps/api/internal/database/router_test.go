package database

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDBRouter_FallbackWhenReplicaNil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	r := NewRouter(&pgxpool.Pool{}, nil, logger)
	if r.Reader() != r.Writer() {
		t.Fatalf("expected reader to fallback to writer when replica is nil")
	}
}

func TestDBRouter_ReplicaFailover(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	writer := &pgxpool.Pool{}
	reader := &pgxpool.Pool{}
	r := NewRouter(writer, reader, logger)

	// Simulate reader marked unhealthy
	r.readerOK.Store(false)
	if r.Reader() != r.Writer() {
		t.Fatalf("expected failover to writer when replica marked unhealthy")
	}
}

func TestWithSlowQueryLog(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	called := false
	err := WithSlowQueryLog(logger, 0, "test.query", func(ctx context.Context) error {
		called = true
		return errors.New("boom")
	})
	if !called {
		t.Fatalf("expected function to be called")
	}
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error")
	}
}
