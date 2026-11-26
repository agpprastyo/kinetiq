package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWaitTimeout(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	if WaitTimeout(&wg, 30*time.Millisecond) {
		t.Fatal("must report timeout while work is still running")
	}
	wg.Done()
	if !WaitTimeout(&wg, time.Second) {
		t.Fatal("must report completion once work finished")
	}
}

func TestSchedulersReturnWaitGroupThatSettlesOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wg := StartRetentionSweeper(ctx, nil, nil, time.Hour, 90)
	cancel()
	if !WaitTimeout(wg, time.Second) {
		t.Fatal("retention sweeper must stop after cancel")
	}
}

func TestHealthHandlerReportsAlive(t *testing.T) {
	rec := httptest.NewRecorder()
	HealthHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 got %d", rec.Code)
	}
}
