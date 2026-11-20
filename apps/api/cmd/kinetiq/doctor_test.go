package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func healthyDeps() doctorDeps {
	env := map[string]string{
		"DATABASE_URL":         "postgres://u@db/x",
		"REDIS_URL":            "redis://r",
		"AUTH_BASE_URL":        "http://auth:3001",
		"AUTH_INTERNAL_SECRET": "s",
		"PUBLIC_APP_URL":       "https://example.com",
	}
	d := doctorDeps{
		Getenv:    func(k string) string { return env[k] },
		TotalRAM:  func() (uint64, error) { return 4 << 30, nil },
		FreeDisk:  func(string) (uint64, error) { return 50 << 30, nil },
		EngineVer: func(context.Context) (string, error) { return "Docker version 27", nil },
		PortFree:  func(int) bool { return true },
		PGQuery: func(_ context.Context, _, q string) (string, error) {
			switch {
			case strings.Contains(q, "server_version"):
				return "16.2", nil
			case strings.Contains(q, "timescaledb"):
				return "2.14.0", nil
			default:
				return "12:false", nil
			}
		},
		RedisPing: func(context.Context, string) error { return nil },
		RedisXLen: func(context.Context, string, string) (int64, error) { return 10, nil },
		HTTPGet:   func(context.Context, string) (int, error) { return 200, nil },
		Now:       func() time.Time { return fixedNow },
	}
	certIn(90 * 24 * time.Hour)(&d)
	return d
}

func find(r DoctorReport, cat, name string) CheckResult {
	for _, c := range r.Results {
		if c.Category == cat && c.Name == name {
			return c
		}
	}
	return CheckResult{}
}

func TestDoctorAllHealthy(t *testing.T) {
	r := collectDoctor(context.Background(), healthyDeps())
	if r.Fail != 0 || r.Warn != 0 || doctorExitCode(r) != 0 {
		t.Fatalf("expected clean report, got %+v", r)
	}
}

// pgReturns overrides the result of the PG query containing match.
func pgReturns(match, val string) func(*doctorDeps) {
	return func(d *doctorDeps) {
		base := d.PGQuery
		d.PGQuery = func(c context.Context, dsn, q string) (string, error) {
			if strings.Contains(q, match) {
				return val, nil
			}
			return base(c, dsn, q)
		}
	}
}

// httpReturns makes URLs containing match return code; others return 200.
func httpReturns(match string, code int) func(*doctorDeps) {
	return func(d *doctorDeps) {
		d.HTTPGet = func(_ context.Context, u string) (int, error) {
			if strings.Contains(u, match) {
				return code, nil
			}
			return 200, nil
		}
	}
}

func certIn(dur time.Duration) func(*doctorDeps) {
	return func(d *doctorDeps) {
		d.CertExpiry = func(context.Context, string) (time.Time, error) { return fixedNow.Add(dur), nil }
	}
}

func TestDoctorFailures(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*doctorDeps)
		cat, chk string
		want     Status
	}{
		{"low ram", func(d *doctorDeps) { d.TotalRAM = func() (uint64, error) { return 1 << 30, nil } }, "system", "RAM", StatusFail},
		{"ram unreadable", func(d *doctorDeps) { d.TotalRAM = func() (uint64, error) { return 0, errors.New("x") } }, "system", "RAM", StatusWarn},
		{"low disk", func(d *doctorDeps) { d.FreeDisk = func(string) (uint64, error) { return 1 << 30, nil } }, "system", "Disk", StatusFail},
		{"no engine", func(d *doctorDeps) {
			d.EngineVer = func(context.Context) (string, error) { return "", errors.New("none") }
		}, "system", "Container engine", StatusFail},
		{"port busy", func(d *doctorDeps) { d.PortFree = func(p int) bool { return p != 443 } }, "ports", "Port 443", StatusWarn},
		{"missing env", func(d *doctorDeps) { d.Getenv = func(string) string { return "" } }, "env", "DATABASE_URL", StatusFail},
		{"db down", func(d *doctorDeps) {
			d.PGQuery = func(context.Context, string, string) (string, error) { return "", errors.New("refused") }
		}, "database", "Connection", StatusFail},
		{"old pg", pgReturns("server_version", "14.9 (Debian)"), "database", "Version", StatusFail},
		{"no timescale", pgReturns("timescaledb", ""), "database", "TimescaleDB", StatusWarn},
		{"dirty migration", pgReturns("schema_migrations", "12:true"), "database", "Migrations", StatusFail},
		{"no migrations", pgReturns("schema_migrations", ""), "database", "Migrations", StatusFail},
		{"redis down", func(d *doctorDeps) {
			d.RedisPing = func(context.Context, string) error { return errors.New("refused") }
		}, "redis", "Connection", StatusFail},
		{"stream huge", func(d *doctorDeps) {
			d.RedisXLen = func(context.Context, string, string) (int64, error) { return streamMaxLen * 3, nil }
		}, "redis", "Stream trimming", StatusWarn},
		{"auth down", func(d *doctorDeps) {
			d.HTTPGet = func(context.Context, string) (int, error) { return 0, errors.New("refused") }
		}, "auth", "Health", StatusFail},
		{"auth 503", httpReturns("auth", 503), "auth", "Health", StatusFail},
		{"cert expiring", certIn(3 * 24 * time.Hour), "network", "TLS certificate", StatusWarn},
		{"cert expired", certIn(-time.Hour), "network", "TLS certificate", StatusFail},
		{"cert error", func(d *doctorDeps) {
			d.CertExpiry = func(context.Context, string) (time.Time, error) { return time.Time{}, errors.New("bad") }
		}, "network", "TLS certificate", StatusFail},
		{"public 500", httpReturns("example.com", 502), "network", "Public URL", StatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := healthyDeps()
			tc.mutate(&d)
			r := collectDoctor(context.Background(), d)
			got := find(r, tc.cat, tc.chk)
			if got.Status != tc.want {
				t.Fatalf("%s/%s = %q (%s), want %q", tc.cat, tc.chk, got.Status, got.Detail, tc.want)
			}
			if got.Remediation == "" {
				t.Fatalf("non-pass result must carry remediation")
			}
			if tc.want == StatusFail && doctorExitCode(r) != 1 {
				t.Fatalf("FAIL must exit 1")
			}
		})
	}
}

func TestDoctorWarnOnlyExitsZero(t *testing.T) {
	d := healthyDeps()
	d.PortFree = func(int) bool { return false }
	r := collectDoctor(context.Background(), d)
	if r.Warn != len(doctorPorts) || r.Fail != 0 || doctorExitCode(r) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestDoctorHTTPPublicURLSkipsTLS(t *testing.T) {
	d := healthyDeps()
	base := d.Getenv
	d.Getenv = func(k string) string {
		if k == "PUBLIC_APP_URL" {
			return "http://example.com"
		}
		return base(k)
	}
	d.CertExpiry = func(context.Context, string) (time.Time, error) {
		t.Fatal("must not dial TLS")
		return time.Time{}, nil
	}
	if got := find(collectDoctor(context.Background(), d), "network", "TLS certificate"); got.Status != StatusWarn {
		t.Fatalf("got %q", got.Status)
	}
}

func TestRenderJSON(t *testing.T) {
	r := collectDoctor(context.Background(), healthyDeps())
	var buf bytes.Buffer
	if err := renderDoctor(&buf, r, true); err != nil {
		t.Fatal(err)
	}
	var back DoctorReport
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatal(err)
	}
	if back.Pass != r.Pass || len(back.Results) != len(r.Results) {
		t.Fatalf("roundtrip mismatch")
	}
}

func TestRenderHuman(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	d := healthyDeps()
	d.RedisPing = func(context.Context, string) error { return errors.New("refused") }
	var buf bytes.Buffer
	if err := renderDoctor(&buf, collectDoctor(context.Background(), d), false); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"[PASS]", "[FAIL]", "fix: Check Redis", "1 failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestParsePGMajor(t *testing.T) {
	for in, want := range map[string]int{"16.2": 16, "14.9 (Debian)": 14, "15": 15} {
		if got, ok := parsePGMajor(in); !ok || got != want {
			t.Fatalf("%q => %d,%v", in, got, ok)
		}
	}
	if _, ok := parsePGMajor("abc"); ok {
		t.Fatal("expected failure")
	}
}
