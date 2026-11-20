package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

type fakeDB struct {
	version  int
	dirty    bool
	executed []string
	failOn   string // substring of SQL that fails
	snaps    int
	snapErr  error
	health   error
	out      bytes.Buffer
}

func (f *fakeDB) deps() upgradeDeps {
	return upgradeDeps{
		Migrations: fstest.MapFS{
			"000001_a.up.sql":   {Data: []byte("UP1")},
			"000001_a.down.sql": {Data: []byte("DOWN1")},
			"000002_b.up.sql":   {Data: []byte("UP2")},
			"000002_b.down.sql": {Data: []byte("DOWN2")},
			"000003_c.up.sql":   {Data: []byte("UP3")},
			"000003_c.down.sql": {Data: []byte("DOWN3")},
			"README.md":         {Data: []byte("ignored")},
		},
		Changelog: "# Changelog\n\n## [v2.0.0]\n- shiny\n\n## [v1.0.0]\n- old\n",
		Version:   func(context.Context) (int, bool, error) { return f.version, f.dirty, nil },
		SetVersion: func(_ context.Context, v int, d bool) error {
			f.version, f.dirty = v, d
			return nil
		},
		Exec: func(_ context.Context, sql string) error {
			f.executed = append(f.executed, sql)
			if f.failOn != "" && sql == f.failOn {
				return errors.New("boom")
			}
			return nil
		},
		Snapshot: func(context.Context) (string, error) {
			f.snaps++
			return "/snap/x", f.snapErr
		},
		Health: func(context.Context) error { return f.health },
		Out:    &f.out,
	}
}

func TestUpgradeAppliesPendingAfterSnapshot(t *testing.T) {
	f := &fakeDB{version: 1}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err != nil {
		t.Fatal(err)
	}
	if f.snaps != 1 || f.version != 3 || f.dirty {
		t.Fatalf("snaps=%d version=%d dirty=%v", f.snaps, f.version, f.dirty)
	}
	if strings.Join(f.executed, ",") != "UP2,UP3" {
		t.Fatalf("executed %v", f.executed)
	}
	out := f.out.String()
	for _, w := range []string{"Current schema version: 1", "Target schema version:  3 (2 pending)", "v2.0.0", "shiny", "Upgrade complete"} {
		if !strings.Contains(out, w) {
			t.Fatalf("missing %q in\n%s", w, out)
		}
	}
	if strings.Contains(out, "v1.0.0") {
		t.Fatal("release notes should only show the latest section")
	}
}

func TestUpgradeDryRunChangesNothing(t *testing.T) {
	f := &fakeDB{version: 1}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if f.snaps != 0 || len(f.executed) != 0 || f.version != 1 {
		t.Fatalf("dry run mutated state: %+v", f)
	}
	if !strings.Contains(f.out.String(), "000002_b") {
		t.Fatalf("dry run should list pending:\n%s", f.out.String())
	}
}

func TestUpgradeUpToDate(t *testing.T) {
	f := &fakeDB{version: 3}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err != nil {
		t.Fatal(err)
	}
	if f.snaps != 0 || len(f.executed) != 0 || !strings.Contains(f.out.String(), "Already up to date") {
		t.Fatalf("unexpected: %s", f.out.String())
	}
}

func TestUpgradeSnapshotFailureAbortsBeforeMigrating(t *testing.T) {
	f := &fakeDB{version: 1, snapErr: errors.New("disk full")}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err == nil {
		t.Fatal("expected error")
	}
	if len(f.executed) != 0 || f.version != 1 {
		t.Fatalf("migrated despite snapshot failure: %v", f.executed)
	}
}

func TestUpgradeMigrationFailureRollsBack(t *testing.T) {
	f := &fakeDB{version: 1, failOn: "UP3"}
	err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{})
	if err == nil || !strings.Contains(err.Error(), "migration 3 failed") {
		t.Fatalf("err=%v", err)
	}
	// UP2 applied, UP3 failed, then DOWN2 reverts UP2.
	if strings.Join(f.executed, ",") != "UP2,UP3,DOWN2" {
		t.Fatalf("executed %v", f.executed)
	}
	if f.version != 1 || f.dirty {
		t.Fatalf("version=%d dirty=%v", f.version, f.dirty)
	}
	if !strings.Contains(f.out.String(), "pg_restore") || !strings.Contains(f.out.String(), "/snap/x/db.dump") {
		t.Fatalf("missing restore guidance:\n%s", f.out.String())
	}
}

func TestUpgradeFirstMigrationFailureResetsMarker(t *testing.T) {
	f := &fakeDB{version: 1, failOn: "UP2"}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err == nil {
		t.Fatal("expected error")
	}
	if strings.Join(f.executed, ",") != "UP2" || f.version != 1 || f.dirty {
		t.Fatalf("executed=%v version=%d dirty=%v", f.executed, f.version, f.dirty)
	}
}

func TestUpgradeHealthFailureRollsBackAll(t *testing.T) {
	f := &fakeDB{version: 1, health: errors.New("503")}
	err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{})
	if err == nil || !strings.Contains(err.Error(), "health check failed") {
		t.Fatalf("err=%v", err)
	}
	if strings.Join(f.executed, ",") != "UP2,UP3,DOWN3,DOWN2" || f.version != 1 {
		t.Fatalf("executed=%v version=%d", f.executed, f.version)
	}
}

func TestUpgradeSkipHealth(t *testing.T) {
	f := &fakeDB{version: 1, health: errors.New("down")}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{SkipHealth: true}); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeRollbackWithoutDownFileGuidesRestore(t *testing.T) {
	f := &fakeDB{version: 0, health: errors.New("down")}
	d := f.deps()
	d.Migrations = fstest.MapFS{"000001_a.up.sql": {Data: []byte("UP1")}}
	if err := runUpgradeFlow(context.Background(), d, upgradeOpts{}); err == nil {
		t.Fatal("expected error")
	}
	out := f.out.String()
	if !strings.Contains(out, "no down file") || !strings.Contains(out, "pg_restore") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestUpgradeRefusesDirtyAndDowngrade(t *testing.T) {
	f := &fakeDB{version: 2, dirty: true}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err == nil || f.snaps != 0 {
		t.Fatalf("dirty must be refused, err=%v", err)
	}
	f = &fakeDB{version: 9}
	if err := runUpgradeFlow(context.Background(), f.deps(), upgradeOpts{}); err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("downgrade must be refused, err=%v", err)
	}
}

func TestLoadMigrationsRejectsOrphanDown(t *testing.T) {
	if _, err := loadMigrations(fstest.MapFS{"000001_a.down.sql": {Data: []byte("x")}}); err == nil {
		t.Fatal("expected error")
	}
}

func TestReleaseNotesTruncates(t *testing.T) {
	got := releaseNotes("## [a]\n1\n2\n3\n## [b]\n4", 2)
	if got != "## [a]\n1" {
		t.Fatalf("got %q", got)
	}
}

func TestWaitHealthy(t *testing.T) {
	ready := 503
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/readyz" {
			w.WriteHeader(ready)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := waitHealthy(context.Background(), srv.URL, 2, time.Millisecond); err == nil || !strings.Contains(err.Error(), "/readyz") {
		t.Fatalf("err=%v", err)
	}
	ready = 200
	if err := waitHealthy(context.Background(), srv.URL, 2, time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
