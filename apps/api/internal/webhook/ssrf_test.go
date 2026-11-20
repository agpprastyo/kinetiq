package webhook

import (
	"net"
	"testing"
)

func TestIsSafeIPBlocksPrivate(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.1",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.169.254",
		"169.254.0.1",
	}
	for _, ip := range blocked {
		if isSafeIP(net.ParseIP(ip)) {
			t.Errorf("expected blocked: %s", ip)
		}
	}
}

func TestIsSafeIPAllowsPublic(t *testing.T) {
	allowed := []string{"8.8.8.8", "1.1.1.1", "203.0.113.5"}
	for _, ip := range allowed {
		if !isSafeIP(net.ParseIP(ip)) {
			t.Errorf("expected allowed: %s", ip)
		}
	}
}
