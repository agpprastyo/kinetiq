package app

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

type fakeDrainer struct {
	draining, streamsStopped atomic.Bool
	order                    []string
}

func (f *fakeDrainer) BeginDrain() { f.draining.Store(true); f.order = append(f.order, "drain") }
func (f *fakeDrainer) StopStreams() {
	f.streamsStopped.Store(true)
	f.order = append(f.order, "streams")
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func TestServeReturnsListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	err = Serve(context.Background(), fiber.New(), l.Addr().String(), &fakeDrainer{}, ServeOptions{})
	if err == nil {
		t.Fatal("bind failure must be returned so the process exits non-zero")
	}
}

func TestServeDrainsBeforeShutdown(t *testing.T) {
	d := &fakeDrainer{}
	addr := freeAddr(t)
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, fiber.New(), addr, d, ServeOptions{DrainDelay: 150 * time.Millisecond, Timeout: 2 * time.Second})
	}()
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	stop()
	time.Sleep(50 * time.Millisecond)
	if !d.draining.Load() {
		t.Fatal("readiness must flip immediately on signal")
	}
	if d.streamsStopped.Load() {
		t.Fatal("streams must stay open during the drain delay")
	}
	if err := <-done; err != nil {
		t.Fatalf("clean shutdown expected, got %v", err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("must wait the drain delay before shutting down")
	}
	if !d.streamsStopped.Load() || len(d.order) != 2 || d.order[0] != "drain" {
		t.Fatalf("expected drain then streams, got %v", d.order)
	}
}

func TestReadyzReturns503WhileDraining(t *testing.T) {
	c := &Container{}
	a := fiber.New()
	registerReadiness(a, c)
	for _, p := range []string{"/readyz"} {
		c.draining.Store(false)
		c.BeginDrain()
		resp, err := a.Test(httptest.NewRequest(http.MethodGet, p, nil))
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("%s: want 503 when draining, got %d", p, resp.StatusCode)
		}
	}
}

func TestContainerStopStreamsCancelsStreamContext(t *testing.T) {
	c := &Container{}
	ctx := c.StreamContext()
	select {
	case <-ctx.Done():
		t.Fatal("must not be cancelled yet")
	default:
	}
	c.StopStreams()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("StopStreams must cancel the stream context")
	}
}
