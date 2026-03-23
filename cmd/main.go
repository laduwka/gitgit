package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/laduwka/gitgit/internal/gitgit"
)

func main() {
	cfg := gitgit.Config{}

	flag.IntVar(&cfg.GroupID, "id", 0, "GitLab group ID")
	flag.StringVar(&cfg.URL, "url", "", "GitLab API URL (default: https://gitlab.com/api/v4)")
	flag.StringVar(&cfg.Token, "token", "", "GitLab private token (or use TOKEN env var)")
	flag.StringVar(&cfg.DataDir, "data", "", "Work directory (default: ./<group_id>)")
	flag.StringVar(&cfg.Regex, "regex", ".", "Regex to filter projects by path")
	flag.IntVar(&cfg.Workers, "workers", 4, "Number of parallel workers")
	flag.BoolVar(&cfg.UseHTTP, "http", false, "Clone via HTTPS instead of SSH")
	flag.Parse()

	if cfg.Workers < 1 {
		log.Fatal("error: -workers must be >= 1")
	}

	if cfg.GroupID == 0 {
		fmt.Fprintln(os.Stderr, "error: -id is required")
		flag.Usage()
		os.Exit(1)
	}

	if cfg.Token == "" {
		cfg.Token = os.Getenv("TOKEN")
	}
	if cfg.Token == "" {
		log.Fatal("error: token is required, use -token flag or TOKEN env var")
	}

	if cfg.URL == "" {
		cfg.URL = os.Getenv("URL")
	}
	if cfg.URL == "" {
		cfg.URL = "https://gitlab.com/api/v4"
	}

	if cfg.DataDir == "" {
		cfg.DataDir = fmt.Sprintf("%d", cfg.GroupID)
	}
	cfg.DataDir, _ = filepath.Abs(cfg.DataDir)

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		log.Fatalf("error: cannot create data dir: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	projects, err := gitgit.FetchProjects(ctx, cfg)
	if err != nil {
		log.Fatalf("error: fetching projects: %v", err)
	}

	filtered, err := gitgit.FilterProjects(projects, cfg.Regex)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Printf("found %d projects (%d after filter)", len(projects), len(filtered))

	failures := gitgit.ProcessProjects(ctx, cfg, filtered)
	if len(failures) > 0 {
		fmt.Fprintf(os.Stderr, "\n=== FAILED PROJECTS ===\n")
		for _, f := range failures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.Path, f.Err)
		}
		fmt.Fprintln(os.Stderr)
		log.Fatalf("completed with %d errors", len(failures))
	}
}
