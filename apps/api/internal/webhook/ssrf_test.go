package webhook

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestIsSafeTargetURLRejectsUnsafe(t *testing.T) {
	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1:8080/",
		"http://[::1]/",
		"http://10.0.0.5/hook",
		"http://0.0.0.0/",
		"http://100.64.0.1/",
		"file:///etc/passwd",
		"gopher://8.8.8.8/",
		"not a url",
	} {
		if err := IsSafeTargetURL(u); err == nil {
			t.Errorf("expected %q rejected", u)
		}
	}
	if err := IsSafeTargetURL("https://8.8.8.8/hook"); err != nil {
		t.Errorf("public ip should pass: %v", err)
	}
}

func TestSafeHTTPClientBlocksLoopbackAtDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	if _, err := NewSafeHTTPClient(2 * time.Second).Get(srv.URL); err == nil {
		t.Fatal("expected dial to loopback to be blocked")
	}
}

func TestSafeHTTPClientDoesNotFollowRedirects(t *testing.T) {
	c := NewSafeHTTPClient(time.Second)
	req, _ := http.NewRequest(http.MethodGet, "http://8.8.8.8/", nil)
	if err := c.CheckRedirect(req, nil); err != http.ErrUseLastResponse {
		t.Fatalf("expected ErrUseLastResponse, got %v", err)
	}
}
