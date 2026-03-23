package gitgit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Project struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	SSHURLToRepo  string `json:"ssh_url_to_repo"`
	HTTPURLToRepo string `json:"http_url_to_repo"`
	PathWithNS    string `json:"path_with_namespace"`
	Archived      bool   `json:"archived"`
}

type Config struct {
	GroupID int
	URL     string
	Token   string
	DataDir string
	Regex   string
	Workers int
	UseHTTP bool
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}

func FetchProjects(ctx context.Context, cfg Config) ([]Project, error) {
	var all []Project

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/groups/%d/projects?per_page=100&page=%d&include_subgroups=true",
			cfg.URL, cfg.GroupID, page)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("PRIVATE-TOKEN", cfg.Token)

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			return nil, fmt.Errorf("page %d: closing body: %w", page, cerr)
		}
		if err != nil {
			return nil, fmt.Errorf("page %d: reading body: %w", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("page %d: HTTP %d: %s", page, resp.StatusCode, string(body))
		}

		var projects []Project
		if err := json.Unmarshal(body, &projects); err != nil {
			return nil, fmt.Errorf("page %d: parsing JSON: %w", page, err)
		}

		if len(projects) == 0 {
			break
		}

		all = append(all, projects...)
		log.Printf("[fetch] page %d: got %d projects (%d total)", page, len(projects), len(all))
	}

	return all, nil
}

func FilterProjects(projects []Project, regex string) ([]Project, error) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return nil, fmt.Errorf("bad regex %q: %w", regex, err)
	}

	var filtered []Project
	for _, p := range projects {
		if p.Archived {
			continue
		}
		if re.MatchString(p.PathWithNS) {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

type FailedProject struct {
	Path string
	Err  string
}

func ProcessProjects(ctx context.Context, cfg Config, projects []Project) []FailedProject {
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var failures []FailedProject

	for _, p := range projects {
		wg.Add(1)

		go func(proj Project) {
			sem <- struct{}{}
			defer func() { <-sem }()
			defer wg.Done()

			cloneURL := proj.SSHURLToRepo
			if cfg.UseHTTP {
				cloneURL = proj.HTTPURLToRepo
			}

			nsDir := filepath.Join(cfg.DataDir, filepath.Dir(proj.PathWithNS))
			repoDir := filepath.Join(cfg.DataDir, proj.PathWithNS)

			if err := os.MkdirAll(nsDir, 0o750); err != nil {
				log.Printf("[%s] error creating dir: %v", proj.PathWithNS, err)
				mu.Lock()
				failures = append(failures, FailedProject{Path: proj.PathWithNS, Err: fmt.Sprintf("creating dir: %v", err)})
				mu.Unlock()
				return
			}

			var err error
			if IsGitRepo(repoDir) {
				err = UpdateRepo(ctx, proj, repoDir, cloneURL)
			} else {
				err = CloneRepo(ctx, proj, cloneURL, nsDir)
			}
			if err != nil {
				mu.Lock()
				failures = append(failures, FailedProject{Path: proj.PathWithNS, Err: err.Error()})
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	return failures
}

func IsGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func CloneRepo(ctx context.Context, proj Project, url, parentDir string) error {
	log.Printf("[clone] %s -> %s", proj.PathWithNS, parentDir)

	cmd := exec.CommandContext(ctx, "git", "clone", "--quiet", "--", url) // #nosec G204 -- url comes from GitLab API response, -- guards against option injection
	cmd.Dir = parentDir
	cmd.Env = gitEnv()

	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[%s] clone failed: %v\n%s", proj.PathWithNS, err, out)
		return err
	}
	log.Printf("[clone] done %s", proj.PathWithNS)
	return nil
}

func UpdateRepo(ctx context.Context, proj Project, repoDir, expectedURL string) error {
	log.Printf("[update] %s -> %s", proj.PathWithNS, repoDir)

	// Sync remote URL if it changed (e.g. HTTPS→SSH switch).
	getURL := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	getURL.Dir = repoDir
	getURL.Env = gitEnv()
	if urlOut, err := getURL.Output(); err == nil {
		current := strings.TrimSpace(string(urlOut))
		if current != expectedURL {
			log.Printf("[update] %s: remote origin %s -> %s", proj.PathWithNS, current, expectedURL)
			setURL := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", expectedURL)
			setURL.Dir = repoDir
			setURL.Env = gitEnv()
			if out, err := setURL.CombinedOutput(); err != nil {
				log.Printf("[%s] set-url failed: %v\n%s", proj.PathWithNS, err, out)
				return err
			}
		}
	}

	cmd := exec.CommandContext(ctx, "git", "pull", "--all", "--quiet")
	cmd.Dir = repoDir
	cmd.Env = gitEnv()

	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[%s] pull failed: %v\n%s", proj.PathWithNS, err, out)
		return err
	}
	log.Printf("[update] done %s", proj.PathWithNS)
	return nil
}
