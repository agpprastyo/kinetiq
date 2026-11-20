package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"kinetiq/internal/config"
	"kinetiq/internal/project"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect db: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	nullLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	projectRepo := project.NewSQLCRepository(pool, nullLogger)
	projectSvc := project.NewProjectService(projectRepo, nullLogger)

	userID := "user-1-id"
	proj, err := projectSvc.Create(ctx, userID, "LoadTest Benchmark Project")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create project: %v\n", err)
		os.Exit(1)
	}

	cookie := "user1_session_seed_k6"

	// Output clean key=value format directly
	fmt.Printf("PROJECT_ID=%s\n", proj.ID.String())
	fmt.Printf("API_KEY=%s\n", proj.APIKey)
	fmt.Printf("COOKIE=%s\n", cookie)
}
