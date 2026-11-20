package webhook

import (
	"testing"
	"time"
)

func TestComputeBackoff(t *testing.T) {
	d := NewDispatcher(DefaultConfig())
	b1 := d.ComputeBackoff(1)
	b2 := d.ComputeBackoff(2)
	if b2 <= b1 {
		t.Fatalf("backoff should grow: %v vs %v", b1, b2)
	}
}

func TestCircuitBreakerOpens(t *testing.T) {
	cb := NewCircuitBreaker(2, 2)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.GetState() != StateOpen {
		t.Fatalf("expected open, got %v", cb.GetState())
	}
}

func TestCircuitBreakerClosesAfterSuccess(t *testing.T) {
	cb := NewCircuitBreaker(1, 1)
	cb.RecordFailure()
	cb.openUntil = time.Now().UnixNano() - 1
	if cb.GetState() != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", cb.GetState())
	}
	cb.RecordSuccess()
	if cb.GetState() != StateClosed {
		t.Fatalf("expected closed, got %v", cb.GetState())
	}
}
