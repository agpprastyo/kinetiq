package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// SignPayload generates HMAC SHA-256 signature for a payload with timestamp.
func SignPayload(secret, payload string, ts time.Time) (string, string) {
	timestamp := strconv.FormatInt(ts.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + sig, timestamp
}

// VerifySignature verifies a webhook signature against secret, payload, timestamp.
func VerifySignature(secret, payload, signature, timestamp string, maxAge time.Duration) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if time.Since(time.Unix(ts, 0)) > maxAge {
		return fmt.Errorf("timestamp too old")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + payload))
	exp := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(exp), []byte(signature)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}
