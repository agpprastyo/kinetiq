package analytics_test

import (
	"testing"

	"kinetiq/internal/analytics"
)

func TestParseTZ(t *testing.T) {
	for in, want := range map[string]string{"": "UTC", "UTC": "UTC", "Asia/Jakarta": "Asia/Jakarta", "America/Argentina/Buenos_Aires": "America/Argentina/Buenos_Aires"} {
		got, err := analytics.ParseTZ(in)
		if err != nil || got != want {
			t.Errorf("ParseTZ(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"Mars/Phobos", "Local", "UTC'; DROP TABLE events;--", "../etc/passwd", "+07:00"} {
		if _, err := analytics.ParseTZ(bad); err == nil {
			t.Errorf("ParseTZ(%q) must fail", bad)
		}
	}
}

func TestTZPartOfCacheKey(t *testing.T) {
	a := analytics.QueryOptions{TZ: "UTC"}.CacheKey()
	b := analytics.QueryOptions{TZ: "Asia/Jakarta"}.CacheKey()
	if a == b {
		t.Fatal("tz must change the cache key")
	}
}
