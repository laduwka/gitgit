package main

import (
	"flag"
	"fmt"
	"log"
	"os"
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

	projects, err := gitgit.FetchProjects(cfg)
	if err != nil {
		log.Fatalf("error: fetching projects: %v", err)
	}

	filtered, err := gitgit.FilterProjects(projects, cfg.Regex)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	fmt.Printf("found %d projects (%d after filter)\n", len(projects), len(filtered))

	gitgit.ProcessProjects(cfg, filtered)
}
