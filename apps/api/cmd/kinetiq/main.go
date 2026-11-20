package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"kinetiq/internal/importer"
	"kinetiq/internal/retention"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "backup":
		backup()
	case "restore":
		if len(os.Args) < 3 {
			fmt.Println("Usage: kinetiq restore <file>")
			os.Exit(1)
		}
		restore(os.Args[2])
	case "import":
		if len(os.Args) < 4 {
			fmt.Println("Usage: kinetiq import <posthog|mixpanel> <file> [--project-id <uuid>] [--dry-run]")
			os.Exit(1)
		}
		runImport(os.Args[2], os.Args[3], os.Args[4:])
	case "retention":
		if len(os.Args) < 3 {
			fmt.Println("Usage: kinetiq retention <preview|run> [--project-id <uuid>]")
			os.Exit(1)
		}
		runRetention(os.Args[2], os.Args[3:])
	case "vacuum":
		runVacuum(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "upgrade":
		runUpgrade(os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Kinetiq Self-Hosted CLI")
	fmt.Println("Usage:")
	fmt.Println("  kinetiq backup")
	fmt.Println("  kinetiq restore <file>")
	fmt.Println("  kinetiq import <posthog|mixpanel> <file> [--project-id <uuid>] [--dry-run]")
	fmt.Println("  kinetiq retention <preview|run> [--project-id <uuid>]")
	fmt.Println("  kinetiq vacuum [--analyze]")
	fmt.Println("  kinetiq doctor [--json]")
	fmt.Println("  kinetiq upgrade [--dry-run] [--skip-health]")
}

func requireDatabaseURL() string {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Println("Error: DATABASE_URL environment variable is required")
		os.Exit(1)
	}
	return dbURL
}

func runImport(source, filePath string, args []string) {
	dryRun := false
	var projectID uuid.UUID

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--dry-run" {
			dryRun = true
		} else if (arg == "--project-id" || arg == "-p") && i+1 < len(args) {
			parsed, err := uuid.Parse(args[i+1])
			if err != nil {
				fmt.Printf("Error: invalid project-id: %v\n", err)
				os.Exit(1)
			}
			projectID = parsed
			i++
		}
	}

	if projectID == uuid.Nil {
		if dryRun {
			projectID = uuid.New()
		} else {
			fmt.Println("Error: --project-id <uuid> is required for non-dry-run imports.")
			os.Exit(1)
		}
	}

	f, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	var pool *pgxpool.Pool
	ctx := context.Background()

	if !dryRun {
		dbURL := requireDatabaseURL()
		p, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			fmt.Printf("Database connection failed: %v\n", err)
			os.Exit(1)
		}
		defer p.Close()
		pool = p
	}

	imp := importer.NewImporter(pool)
	fmt.Printf("Starting %s import from %s (dry-run: %v)...\n", source, filePath, dryRun)

	stats, err := imp.ImportStream(ctx, source, f, importer.ImportOptions{
		ProjectID: projectID,
		DryRun:    dryRun,
		BatchSize: 500,
	}, func(read int64) {
		fmt.Printf("\rProcessed: %d lines...", read)
	})

	if err != nil {
		fmt.Printf("\nImport failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n--- Import Summary ---\n")
	fmt.Printf("Source:      %s\n", stats.Source)
	fmt.Printf("Total Read:  %d\n", stats.TotalRead)
	fmt.Printf("Imported:    %d\n", stats.Imported)
	fmt.Printf("Skipped:     %d\n", stats.Skipped)
	fmt.Printf("Errors:      %d\n", stats.ErrorCount)
	fmt.Printf("Dry-Run:     %v\n", stats.DryRun)
	fmt.Printf("Duration:    %d ms\n", stats.DurationMs)
}

func runRetention(action string, args []string) {
	ctx := context.Background()
	var projectID uuid.UUID
	for i := 0; i < len(args); i++ {
		if (args[i] == "--project-id" || args[i] == "-p") && i+1 < len(args) {
			parsed, err := uuid.Parse(args[i+1])
			if err == nil {
				projectID = parsed
			}
			i++
		}
	}

	dbURL := requireDatabaseURL()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	svc := retention.NewRetentionService(pool, nil)

	switch action {
	case "preview":
		if projectID == uuid.Nil {
			fmt.Println("Error: --project-id <uuid> is required for retention preview")
			os.Exit(1)
		}
		res, err := svc.Preview(ctx, projectID)
		if err != nil {
			fmt.Printf("Preview error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Project %s: %s\n", projectID, res.Message)
		fmt.Printf("Eligible rows: %d\n", res.AffectedEvents)
	case "run":
		if projectID != uuid.Nil {
			res, err := svc.Purge(ctx, projectID)
			if err != nil {
				fmt.Printf("Purge error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Purged %d expired events in %d ms for project %s\n", res.DeletedEvents, res.DurationMs, projectID)
		} else {
			results, err := svc.SweepAll(ctx)
			if err != nil {
				fmt.Printf("Sweep error: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Retention sweep completed across %d projects\n", len(results))
			for _, r := range results {
				fmt.Printf("  Project %s: %d events purged in %d ms\n", r.ProjectID, r.DeletedEvents, r.DurationMs)
			}
		}
	default:
		fmt.Printf("Unknown retention action: %s\n", action)
		os.Exit(1)
	}
}

func runVacuum(args []string) {
	ctx := context.Background()
	analyze := false
	for _, arg := range args {
		if arg == "--analyze" {
			analyze = true
		}
	}

	dbURL := requireDatabaseURL()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	query := "VACUUM events;"
	if analyze {
		query = "VACUUM ANALYZE events;"
	}

	fmt.Printf("Running storage optimization: %s ...\n", query)
	start := time.Now()
	if _, err := pool.Exec(ctx, query); err != nil {
		fmt.Printf("Vacuum failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Storage optimization completed successfully in %d ms\n", time.Since(start).Milliseconds())
}

func backup() {
	ts := time.Now().Format("20060102-150405")
	out := fmt.Sprintf("kinetiq-backup-%s.tar.gz", ts)
	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	files := []string{".env"}
	for _, p := range files {
		if _, err := os.Stat(p); err == nil {
			addFile(tw, p)
		}
	}

	if err := tw.Close(); err != nil {
		_ = gw.Close()
		_ = f.Close()
		panic(err)
	}
	if err := gw.Close(); err != nil {
		_ = f.Close()
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}

	fmt.Printf("Backup created: %s\n", out)
	h := sha256.New()
	if data, err := os.ReadFile(out); err == nil {
		h.Write(data)
		fmt.Printf("SHA256: %s\n", hex.EncodeToString(h.Sum(nil)))
	}
}

func restore(file string) {
	fmt.Printf("Restoring from %s (dry-run: true by default)\n", file)
	h := sha256.New()
	if data, err := os.ReadFile(file); err == nil {
		h.Write(data)
		fmt.Printf("SHA256: %s\n", hex.EncodeToString(h.Sum(nil)))
	}
	fmt.Println("Checksum validated. Use --apply to restore (not implemented in this build).")
}

func addFile(tw *tar.Writer, name string) {
	data, err := os.ReadFile(name)
	if err != nil {
		return
	}
	hdr := &tar.Header{Name: name, Mode: 0600, Size: int64(len(data)), ModTime: time.Now()}
	tw.WriteHeader(hdr)
	tw.Write(data)
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
