package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type migrationFile struct {
	Version int
	Name    string
	Up      string // file name within the migrations FS
	Down    string // empty when no down file exists
}

// upgradeDeps isolates every side effect so the runner is testable offline.
type upgradeDeps struct {
	Migrations fs.FS
	Changelog  string
	Version    func(ctx context.Context) (version int, dirty bool, err error)
	SetVersion func(ctx context.Context, version int, dirty bool) error
	Exec       func(ctx context.Context, sql string) error
	Snapshot   func(ctx context.Context) (dir string, err error)
	Health     func(ctx context.Context) error
	Out        io.Writer
}

type upgradeOpts struct {
	DryRun     bool
	SkipHealth bool
}

var migrationNameRe = regexp.MustCompile(`^(\d+)_.+\.(up|down)\.sql$`)

func loadMigrations(fsys fs.FS) ([]migrationFile, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	byVer := map[int]*migrationFile{}
	for _, e := range entries {
		m := migrationNameRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.Atoi(m[1])
		mf := byVer[v]
		if mf == nil {
			mf = &migrationFile{Version: v, Name: strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".sql"), "."+m[2])}
			byVer[v] = mf
		}
		if m[2] == "up" {
			mf.Up = e.Name()
		} else {
			mf.Down = e.Name()
		}
	}
	var out []migrationFile
	for _, mf := range byVer {
		if mf.Up == "" {
			return nil, fmt.Errorf("migration %d has a down file but no up file", mf.Version)
		}
		out = append(out, *mf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// releaseNotes returns the first changelog section (up to maxLines lines).
func releaseNotes(changelog string, maxLines int) string {
	var out []string
	started := false
	for _, l := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(l, "## [") {
			if started {
				break
			}
			started = true
		}
		if started {
			out = append(out, l)
			if len(out) >= maxLines {
				break
			}
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func runUpgradeFlow(ctx context.Context, d upgradeDeps, o upgradeOpts) error {
	p := func(f string, a ...any) { fmt.Fprintf(d.Out, f+"\n", a...) }

	cur, dirty, err := d.Version(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if dirty {
		return fmt.Errorf("schema_migrations is dirty at version %d: fix the failed migration manually, then run `migrate force <version>` before upgrading", cur)
	}
	all, err := loadMigrations(d.Migrations)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if len(all) == 0 {
		return errors.New("no migrations found")
	}
	target := all[len(all)-1].Version
	if cur > target {
		return fmt.Errorf("database schema version %d is newer than the target %d: refusing to downgrade", cur, target)
	}
	var pending []int // indexes into all
	for i, m := range all {
		if m.Version > cur {
			pending = append(pending, i)
		}
	}

	p("Current schema version: %d", cur)
	p("Target schema version:  %d (%d pending)", target, len(pending))
	if notes := releaseNotes(d.Changelog, 20); notes != "" {
		p("\nRelease notes:\n%s\n", notes)
	}
	if len(pending) == 0 {
		p("Already up to date.")
		return nil
	}
	if o.DryRun {
		p("Dry run: no snapshot taken, no changes made. Would apply:")
		for _, i := range pending {
			p("  %d %s", all[i].Version, all[i].Name)
		}
		return nil
	}

	snap, err := d.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("pre-upgrade snapshot failed, aborting before any change: %w", err)
	}
	p("Pre-upgrade snapshot: %s", snap)

	var applied []int // indexes into all, in apply order
	fail := func(cause error) error {
		p("Upgrade failed: %v", cause)
		p("Rolling back %d applied migration(s)...", len(applied))
		rbErr := rollbackApplied(ctx, d, all, applied, cur)
		if rbErr != nil {
			p("Automatic rollback incomplete: %v", rbErr)
		} else {
			p("Schema rolled back to version %d.", cur)
		}
		p("To restore the full pre-upgrade state: pg_restore --clean --if-exists -d \"$DATABASE_URL\" %s", filepath.Join(snap, snapshotDumpName))
		return cause
	}

	for _, i := range pending {
		m := all[i]
		p("Applying %d %s ...", m.Version, m.Name)
		sql, err := fs.ReadFile(d.Migrations, m.Up)
		if err != nil {
			return fail(err)
		}
		if err := d.SetVersion(ctx, m.Version, true); err != nil {
			return fail(err)
		}
		if err := d.Exec(ctx, string(sql)); err != nil {
			// Multi-statement simple query is one implicit transaction: nothing of this file persisted.
			if rerr := d.SetVersion(ctx, prevVersion(all, i), false); rerr != nil {
				p("Could not reset version marker: %v", rerr)
			}
			return fail(fmt.Errorf("migration %d failed: %w", m.Version, err))
		}
		if err := d.SetVersion(ctx, m.Version, false); err != nil {
			return fail(err)
		}
		applied = append(applied, i)
	}

	if o.SkipHealth {
		p("Health verification skipped.")
	} else {
		p("Verifying /healthz and /readyz ...")
		if err := d.Health(ctx); err != nil {
			return fail(fmt.Errorf("post-upgrade health check failed: %w", err))
		}
	}
	p("Upgrade complete: schema %d -> %d.", cur, target)
	return nil
}

func prevVersion(all []migrationFile, i int) int {
	if i == 0 {
		return 0
	}
	return all[i-1].Version
}

func rollbackApplied(ctx context.Context, d upgradeDeps, all []migrationFile, applied []int, start int) error {
	for n := len(applied) - 1; n >= 0; n-- {
		i := applied[n]
		m := all[i]
		if m.Down == "" {
			return fmt.Errorf("migration %d has no down file; restore from snapshot", m.Version)
		}
		sql, err := fs.ReadFile(d.Migrations, m.Down)
		if err != nil {
			return err
		}
		if err := d.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("down migration %d failed: %w", m.Version, err)
		}
		prev := start
		if n > 0 {
			prev = all[applied[n-1]].Version
		}
		if err := d.SetVersion(ctx, prev, false); err != nil {
			return err
		}
	}
	return nil
}

// --- real implementations (not unit-tested; exercised by running the CLI) ---

const snapshotDumpName = "db.dump"

// ponytail: paths are fixed (run from the repo root); add flags only if validated against a base dir.
const (
	upgradeMigrationsDir = "migrations/app"
	upgradeChangelog     = "CHANGELOG.md"
	upgradeSnapshotDir   = "kinetiq-snapshots"
)

func runUpgrade(args []string) {
	var o upgradeOpts
	apiURL := os.Getenv("API_BASE_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}
	for _, a := range args {
		switch a {
		case "--dry-run":
			o.DryRun = true
		case "--skip-health":
			o.SkipHealth = true
		case "--help", "-h":
			fmt.Println("Usage: kinetiq upgrade [--dry-run] [--skip-health]")
			return
		default:
			fmt.Printf("Unknown flag: %s\nUsage: kinetiq upgrade [--dry-run] [--skip-health]\n", a)
			os.Exit(1)
		}
	}

	dbURL := requireDatabaseURL()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Printf("Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)

	notes, _ := os.ReadFile(upgradeChangelog)
	d := upgradeDeps{
		Migrations: os.DirFS(upgradeMigrationsDir),
		Changelog:  string(notes),
		Out:        os.Stdout,
		Version: func(ctx context.Context) (int, bool, error) {
			if _, err := conn.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)"); err != nil {
				return 0, false, err
			}
			var v int
			var dirty bool
			err := conn.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&v, &dirty)
			if errors.Is(err, pgx.ErrNoRows) {
				return 0, false, nil
			}
			return v, dirty, err
		},
		SetVersion: func(ctx context.Context, v int, dirty bool) error {
			if _, err := conn.Exec(ctx, "DELETE FROM schema_migrations"); err != nil {
				return err
			}
			if v <= 0 {
				return nil
			}
			_, err := conn.Exec(ctx, "INSERT INTO schema_migrations (version, dirty) VALUES ($1, $2)", v, dirty)
			return err
		},
		Exec: func(ctx context.Context, sql string) error {
			_, err := conn.Exec(ctx, sql) // no args => simple protocol, multi-statement
			return err
		},
		Snapshot: func(ctx context.Context) (string, error) { return takeSnapshot(ctx, dbURL, upgradeSnapshotDir) },
		Health:   func(ctx context.Context) error { return waitHealthy(ctx, apiURL, 10, 3*time.Second) },
	}

	if err := runUpgradeFlow(ctx, d, o); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// pgDumpPaths are fixed system locations; PATH is deliberately not consulted (Sonar S4036).
var pgDumpPaths = []string{"/usr/bin/pg_dump", "/usr/local/bin/pg_dump", "/opt/homebrew/bin/pg_dump", "/usr/lib/postgresql/16/bin/pg_dump", "/usr/lib/postgresql/15/bin/pg_dump"}

func findPgDump() (string, error) {
	for _, p := range pgDumpPaths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("pg_dump not found in %s", strings.Join(pgDumpPaths, ", "))
}

// takeSnapshot writes pg_dump (custom format) and a copy of .env into a timestamped directory.
// Credentials go to pg_dump via PG* env vars, never argv.
func takeSnapshot(ctx context.Context, dbURL, root string) (string, error) {
	pgDump, err := findPgDump()
	if err != nil {
		return "", err
	}
	cfg, err := pgx.ParseConfig(dbURL)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "pre-upgrade-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, pgDump, "-Fc", "-f", filepath.Join(dir, snapshotDumpName))
	cmd.Env = append(os.Environ(),
		"PGHOST="+cfg.Host, "PGPORT="+strconv.Itoa(int(cfg.Port)),
		"PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password, "PGDATABASE="+cfg.Database)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if data, err := os.ReadFile(".env"); err == nil {
		if err := os.WriteFile(filepath.Join(dir, ".env"), data, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func waitHealthy(ctx context.Context, base string, attempts int, delay time.Duration) error {
	var last error
	for n := 0; n < attempts; n++ {
		last = nil
		for _, path := range []string{"/healthz", "/readyz"} {
			code, err := probeStatus(ctx, strings.TrimRight(base, "/")+path)
			if err != nil {
				last = err
			} else if code != http.StatusOK {
				last = fmt.Errorf("%s returned HTTP %d", path, code)
			}
			if last != nil {
				break
			}
		}
		if last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return last
}

func probeStatus(ctx context.Context, u string) (int, error) {
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
