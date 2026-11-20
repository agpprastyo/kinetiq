package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

type Status string

const (
	StatusPass Status = "PASS"
	StatusWarn Status = "WARN"
	StatusFail Status = "FAIL"
)

const (
	minRAMBytes  = 2 << 30
	minDiskBytes = 20 << 30
	minPGMajor   = 15
	certWarnDays = 14
	streamKey    = "events:stream"
	streamMaxLen = 500000
)

var doctorPorts = []int{80, 443, 3000, 8080}

type CheckResult struct {
	Category    string `json:"category"`
	Name        string `json:"name"`
	Status      Status `json:"status"`
	Detail      string `json:"detail,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

type DoctorReport struct {
	Results []CheckResult `json:"results"`
	Pass    int           `json:"pass"`
	Warn    int           `json:"warn"`
	Fail    int           `json:"fail"`
}

// doctorDeps isolates every side effect so checks run offline in tests.
type doctorDeps struct {
	Getenv     func(string) string
	TotalRAM   func() (uint64, error)
	FreeDisk   func(path string) (uint64, error)
	EngineVer  func(ctx context.Context) (string, error)
	PortFree   func(port int) bool
	PGQuery    func(ctx context.Context, dsn, query string) (string, error)
	RedisPing  func(ctx context.Context, url string) error
	RedisXLen  func(ctx context.Context, url, key string) (int64, error)
	HTTPGet    func(ctx context.Context, url string) (int, error)
	CertExpiry func(ctx context.Context, host string) (time.Time, error)
	Now        func() time.Time
}

func defaultDoctorDeps() doctorDeps {
	return doctorDeps{
		Getenv:     os.Getenv,
		TotalRAM:   readTotalRAM,
		FreeDisk:   freeDisk,
		EngineVer:  containerEngineVersion,
		PortFree:   portFree,
		PGQuery:    pgScalar,
		RedisPing:  redisPing,
		RedisXLen:  redisXLen,
		HTTPGet:    httpStatus,
		CertExpiry: certExpiry,
		Now:        time.Now,
	}
}

func runDoctor(args []string) {
	asJSON := false
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	report := collectDoctor(ctx, defaultDoctorDeps())
	if err := renderDoctor(os.Stdout, report, asJSON); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(doctorExitCode(report))
}

func doctorExitCode(r DoctorReport) int {
	if r.Fail > 0 {
		return 1
	}
	return 0
}

func collectDoctor(ctx context.Context, d doctorDeps) DoctorReport {
	var all []CheckResult
	all = append(all, checkEnv(d)...)
	all = append(all, checkSystem(ctx, d)...)
	all = append(all, checkPorts(d)...)
	all = append(all, checkDatabase(ctx, d)...)
	all = append(all, checkRedis(ctx, d)...)
	all = append(all, checkAuth(ctx, d)...)
	all = append(all, checkPublicURL(ctx, d)...)

	r := DoctorReport{Results: all}
	for _, c := range all {
		switch c.Status {
		case StatusPass:
			r.Pass++
		case StatusWarn:
			r.Warn++
		case StatusFail:
			r.Fail++
		}
	}
	return r
}

func res(cat, name string, s Status, detail, fix string) CheckResult {
	if s == StatusPass {
		fix = ""
	}
	return CheckResult{Category: cat, Name: name, Status: s, Detail: detail, Remediation: fix}
}

func requireEnv(d doctorDeps, key, cat, name string) (string, []CheckResult) {
	v := d.Getenv(key)
	if v == "" {
		return "", []CheckResult{res(cat, name, StatusFail, key+" not set", "Set "+key)}
	}
	return v, nil
}

func checkEnv(d doctorDeps) []CheckResult {
	var out []CheckResult
	for _, k := range []string{"DATABASE_URL", "REDIS_URL", "AUTH_BASE_URL", "AUTH_INTERNAL_SECRET", "PUBLIC_APP_URL"} {
		if d.Getenv(k) == "" {
			out = append(out, res("env", k, StatusFail, "not set", "Set "+k+" in .env or the container environment"))
		} else {
			out = append(out, res("env", k, StatusPass, "set", ""))
		}
	}
	return out
}

// minCheck: err -> WARN, value < min -> FAIL, else PASS.
func minCheck(name string, val, min uint64, err error, unit, warnFix, failFix string) CheckResult {
	switch {
	case err != nil:
		return res("system", name, StatusWarn, err.Error(), warnFix)
	case val < min:
		return res("system", name, StatusFail, fmt.Sprintf("%.1f GB%s (need >= %.0f GB)", gb(val), unit, gb(min)), failFix)
	}
	return res("system", name, StatusPass, fmt.Sprintf("%.1f GB%s", gb(val), unit), "")
}

func checkSystem(ctx context.Context, d doctorDeps) []CheckResult {
	ram, ramErr := d.TotalRAM()
	free, diskErr := d.FreeDisk(".")
	out := []CheckResult{
		minCheck("RAM", ram, minRAMBytes, ramErr, "", "Verify manually: at least 2GB RAM recommended", "Upgrade host to at least 2GB RAM or add swap"),
		minCheck("Disk", free, minDiskBytes, diskErr, " free", "Verify manually: at least 20GB free recommended", "Free disk space or expand the volume (docker system prune, retention run)"),
	}
	v, err := d.EngineVer(ctx)
	if err != nil {
		return append(out, res("system", "Container engine", StatusFail, err.Error(), "Install Docker (https://docs.docker.com/engine/install/) or Podman"))
	}
	return append(out, res("system", "Container engine", StatusPass, v, ""))
}

func checkPorts(d doctorDeps) []CheckResult {
	var out []CheckResult
	for _, p := range doctorPorts {
		name := fmt.Sprintf("Port %d", p)
		if d.PortFree(p) {
			out = append(out, res("ports", name, StatusPass, "free", ""))
		} else {
			out = append(out, res("ports", name, StatusWarn, "in use", fmt.Sprintf("Stop the process bound to port %d (ss -ltnp | grep :%d) unless it is Kinetiq itself", p, p)))
		}
	}
	return out
}

func checkDatabase(ctx context.Context, d doctorDeps) []CheckResult {
	dsn, missing := requireEnv(d, "DATABASE_URL", "database", "Connection")
	if missing != nil {
		return missing
	}
	var out []CheckResult

	start := d.Now()
	ver, err := d.PGQuery(ctx, dsn, "SHOW server_version")
	lat := d.Now().Sub(start)
	if err != nil {
		return []CheckResult{res("database", "Connection", StatusFail, err.Error(), "Check Postgres is running and DATABASE_URL credentials/host are correct")}
	}
	out = append(out, res("database", "Connection", StatusPass, "latency "+lat.Round(time.Millisecond).String(), ""))

	if major, ok := parsePGMajor(ver); !ok {
		out = append(out, res("database", "Version", StatusWarn, "unparseable: "+ver, "Ensure PostgreSQL 15+"))
	} else if major < minPGMajor {
		out = append(out, res("database", "Version", StatusFail, "PostgreSQL "+ver, "Upgrade to PostgreSQL 15 or newer"))
	} else {
		out = append(out, res("database", "Version", StatusPass, "PostgreSQL "+ver, ""))
	}

	if tv, err := d.PGQuery(ctx, dsn, "SELECT extversion FROM pg_extension WHERE extname='timescaledb'"); err != nil {
		out = append(out, res("database", "TimescaleDB", StatusWarn, err.Error(), "Run CREATE EXTENSION timescaledb;"))
	} else if tv == "" {
		out = append(out, res("database", "TimescaleDB", StatusWarn, "extension not installed", "Use a TimescaleDB image and run CREATE EXTENSION timescaledb;"))
	} else {
		out = append(out, res("database", "TimescaleDB", StatusPass, "v"+tv, ""))
	}

	// golang-migrate bookkeeping table: (version, dirty).
	if mv, err := d.PGQuery(ctx, dsn, "SELECT version||':'||dirty FROM schema_migrations LIMIT 1"); err != nil {
		out = append(out, res("database", "Migrations", StatusFail, err.Error(), "Run: migrate -path migrations/app -database $DATABASE_URL up"))
	} else if mv == "" {
		out = append(out, res("database", "Migrations", StatusFail, "no migrations applied", "Run: migrate -path migrations/app -database $DATABASE_URL up"))
	} else if strings.HasSuffix(mv, ":true") {
		out = append(out, res("database", "Migrations", StatusFail, "dirty at version "+strings.TrimSuffix(mv, ":true"), "Fix the failed migration, then: migrate force <version>"))
	} else {
		out = append(out, res("database", "Migrations", StatusPass, "version "+strings.TrimSuffix(mv, ":false"), ""))
	}
	return out
}

func checkRedis(ctx context.Context, d doctorDeps) []CheckResult {
	u, missing := requireEnv(d, "REDIS_URL", "redis", "Connection")
	if missing != nil {
		return missing
	}
	start := d.Now()
	if err := d.RedisPing(ctx, u); err != nil {
		return []CheckResult{res("redis", "Connection", StatusFail, err.Error(), "Check Redis is running and REDIS_URL is correct")}
	}
	out := []CheckResult{res("redis", "Connection", StatusPass, "PONG, latency "+d.Now().Sub(start).Round(time.Millisecond).String(), "")}

	n, err := d.RedisXLen(ctx, u, streamKey)
	switch {
	case err != nil:
		out = append(out, res("redis", "Stream trimming", StatusWarn, err.Error(), "Verify the worker is running"))
	case n > streamMaxLen*2:
		out = append(out, res("redis", "Stream trimming", StatusWarn, fmt.Sprintf("%s length %d exceeds 2x cap %d", streamKey, n, streamMaxLen), "Worker is not consuming/trimming; check worker logs or XTRIM manually"))
	default:
		out = append(out, res("redis", "Stream trimming", StatusPass, fmt.Sprintf("%s length %d", streamKey, n), ""))
	}
	return out
}

func checkAuth(ctx context.Context, d doctorDeps) []CheckResult {
	base, missing := requireEnv(d, "AUTH_BASE_URL", "auth", "Health")
	if missing != nil {
		return missing
	}
	code, err := d.HTTPGet(ctx, strings.TrimRight(base, "/")+"/health")
	if err != nil {
		return []CheckResult{res("auth", "Health", StatusFail, err.Error(), "Start the auth service and verify AUTH_BASE_URL")}
	}
	if code != http.StatusOK {
		return []CheckResult{res("auth", "Health", StatusFail, "HTTP "+strconv.Itoa(code), "Inspect auth service logs")}
	}
	return []CheckResult{res("auth", "Health", StatusPass, "HTTP 200", "")}
}

func checkPublicURL(ctx context.Context, d doctorDeps) []CheckResult {
	raw, missing := requireEnv(d, "PUBLIC_APP_URL", "network", "Public URL")
	if missing != nil {
		return missing
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return []CheckResult{res("network", "Public URL", StatusFail, "invalid URL: "+raw, "Set PUBLIC_APP_URL to e.g. https://analytics.example.com")}
	}
	var out []CheckResult
	if code, err := d.HTTPGet(ctx, raw); err != nil {
		out = append(out, res("network", "Public URL", StatusWarn, err.Error(), "Check DNS, firewall and reverse proxy for "+u.Host))
	} else if code >= 500 {
		out = append(out, res("network", "Public URL", StatusFail, "HTTP "+strconv.Itoa(code), "Check reverse proxy and app logs"))
	} else {
		out = append(out, res("network", "Public URL", StatusPass, "HTTP "+strconv.Itoa(code), ""))
	}

	if u.Scheme != "https" {
		return append(out, res("network", "TLS certificate", StatusWarn, "PUBLIC_APP_URL is not https", "Serve over HTTPS in production"))
	}
	host := u.Host
	if u.Port() == "" {
		host += ":443"
	}
	exp, err := d.CertExpiry(ctx, host)
	if err != nil {
		return append(out, res("network", "TLS certificate", StatusFail, err.Error(), "Install a valid certificate (e.g. Let's Encrypt)"))
	}
	days := int(exp.Sub(d.Now()).Hours() / 24)
	st, detail, fix := StatusPass, fmt.Sprintf("expires in %d days (%s)", days, exp.Format("2006-01-02")), "Renew the certificate soon"
	if exp.Before(d.Now()) {
		st, detail, fix = StatusFail, "expired "+exp.Format("2006-01-02"), "Renew the certificate"
	} else if days < certWarnDays {
		st = StatusWarn
	}
	out = append(out, res("network", "TLS certificate", st, detail, fix))
	return out
}

func renderDoctor(w io.Writer, r DoctorReport, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	color := map[Status]string{StatusPass: "\033[32m", StatusWarn: "\033[33m", StatusFail: "\033[31m"}
	noColor := os.Getenv("NO_COLOR") != ""
	for _, c := range r.Results {
		badge := string(c.Status)
		if !noColor {
			badge = color[c.Status] + badge + "\033[0m"
		}
		fmt.Fprintf(w, "[%s] %-9s %-18s %s\n", badge, c.Category, c.Name, c.Detail)
		if c.Remediation != "" {
			fmt.Fprintf(w, "       fix: %s\n", c.Remediation)
		}
	}
	_, err := fmt.Fprintf(w, "\n%d passed, %d warnings, %d failed\n", r.Pass, r.Warn, r.Fail)
	return err
}

func parsePGMajor(v string) (int, bool) {
	f := strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' })
	if len(f) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(f[0])
	return n, err == nil
}

func gb(b uint64) float64 { return float64(b) / (1 << 30) }

// --- real implementations (not unit-tested; exercised by running the CLI) ---

func readTotalRAM() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			fs := strings.Fields(rest)
			if len(fs) == 0 {
				break
			}
			kb, err := strconv.ParseUint(fs[0], 10, 64)
			return kb * 1024, err
		}
	}
	return 0, errors.New("MemTotal not found in /proc/meminfo")
}

func freeDisk(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}

func containerEngineVersion(ctx context.Context) (string, error) {
	for _, bin := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(bin); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, bin, "--version").Output()
		if err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}
	return "", errors.New("neither docker nor podman found")
}

func portFree(port int) bool {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// pgScalar returns the first column of the first row as text, or "" when no rows.
func pgScalar(ctx context.Context, dsn, query string) (string, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return "", err
	}
	defer conn.Close(ctx)
	var v string
	if err := conn.QueryRow(ctx, query).Scan(&v); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return v, nil
}

func redisClient(u string) (*redis.Client, error) {
	opt, err := redis.ParseURL(u)
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opt), nil
}

func withRedis[T any](u string, fn func(*redis.Client) (T, error)) (T, error) {
	c, err := redisClient(u)
	if err != nil {
		var zero T
		return zero, err
	}
	defer c.Close()
	return fn(c)
}

func redisPing(ctx context.Context, u string) error {
	_, err := withRedis(u, func(c *redis.Client) (struct{}, error) { return struct{}{}, c.Ping(ctx).Err() })
	return err
}

func redisXLen(ctx context.Context, u, key string) (int64, error) {
	return withRedis(u, func(c *redis.Client) (int64, error) { return c.XLen(ctx, key).Result() })
}

func httpStatus(ctx context.Context, u string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func certExpiry(ctx context.Context, hostport string) (time.Time, error) {
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12}}
	conn, err := dialer.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return time.Time{}, errors.New("no peer certificate")
	}
	return certs[0].NotAfter, nil
}
