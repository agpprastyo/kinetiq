package webhook

import (
	"testing"
	"time"
)

func TestSignAndVerifyHMAC(t *testing.T) {
	secret := "test-secret"
	payload := `{"event":"pageview","data":{}}`
	sig, ts := SignPayload(secret, payload, time.Now())
	if err := VerifySignature(secret, payload, sig, ts, 5*time.Minute); err != nil {
		t.Fatalf("verify failed: %v", err)
	}
}

func TestVerifyFailsOnTamper(t *testing.T) {
	secret := "test-secret"
	payload := `{"event":"pageview"}`
	sig, ts := SignPayload(secret, payload, time.Now())
	if err := VerifySignature(secret, payload+"x", sig, ts, 5*time.Minute); err == nil {
		t.Fatal("expected failure")
	}
}

func TestVerifyFailsOnOldTimestamp(t *testing.T) {
	secret := "test-secret"
	payload := `{"event":"pageview"}`
	old := time.Now().Add(-10 * time.Minute)
	sig, ts := SignPayload(secret, payload, old)
	if err := VerifySignature(secret, payload, sig, ts, 5*time.Minute); err == nil {
		t.Fatal("expected failure")
	}
}
