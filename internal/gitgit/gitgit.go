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
	EmptyRepo     bool   `json:"empty_repo"`
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

// gitWaitDelay is how long a git process gets to exit after SIGINT on
// cancellation before it is killed.
var gitWaitDelay = 10 * time.Second

func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
}

// gitCommand builds a git command that, on ctx cancellation, receives SIGINT
// instead of SIGKILL, so git can remove lock files and partial clones itself.
// It is killed only if it does not exit within gitWaitDelay.
func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- fixed git subcommands; values come from GitLab API response and callers put "--" before them
	cmd.Dir = dir
	cmd.Env = gitEnv()
	cmd.Cancel = func() error {
		if err := cmd.Process.Signal(os.Interrupt); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
	cmd.WaitDelay = gitWaitDelay
	return cmd
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

// FilterProjects отбирает проекты, подлежащие синхронизации, и дополнительно
// возвращает число пропущенных пустых репозиториев.
//
// Пустой репозиторий (ни одного коммита на удалённой стороне) отсеивается по той
// же причине, что и архивный: синхронизировать в нём нечего. Без этого git clone
// заводит локальную ветку по умолчанию без коммитов, а последующий git pull
// возвращает ненулевой код, и весь прогон выглядит упавшим.
func FilterProjects(projects []Project, regex string) ([]Project, int, error) {
	re, err := regexp.Compile(regex)
	if err != nil {
		return nil, 0, fmt.Errorf("bad regex %q: %w", regex, err)
	}

	var filtered []Project
	skippedEmpty := 0
	for _, p := range projects {
		if p.Archived {
			continue
		}
		if !re.MatchString(p.PathWithNS) {
			continue
		}
		if p.EmptyRepo {
			skippedEmpty++
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, skippedEmpty, nil
}

type FailedProject struct {
	Path string
	Err  string
}

// Result is the outcome of ProcessProjects.
type Result struct {
	Failures []FailedProject
	// Interrupted counts projects that were not started or were aborted
	// because ctx was canceled. They are not reported as failures.
	Interrupted int
}

func ProcessProjects(ctx context.Context, cfg Config, projects []Project) Result {
	workers := cfg.Workers
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var res Result

	for _, p := range projects {
		wg.Add(1)

		go func(proj Project) {
			sem <- struct{}{}
			defer func() { <-sem }()
			defer wg.Done()

			if ctx.Err() != nil {
				mu.Lock()
				res.Interrupted++
				mu.Unlock()
				return
			}

			cloneURL := proj.SSHURLToRepo
			if cfg.UseHTTP {
				cloneURL = proj.HTTPURLToRepo
			}

			nsDir := filepath.Join(cfg.DataDir, filepath.Dir(proj.PathWithNS))
			repoDir := filepath.Join(cfg.DataDir, proj.PathWithNS)

			if err := os.MkdirAll(nsDir, 0o750); err != nil {
				log.Printf("[%s] error creating dir: %v", proj.PathWithNS, err)
				mu.Lock()
				res.Failures = append(res.Failures, FailedProject{Path: proj.PathWithNS, Err: fmt.Sprintf("creating dir: %v", err)})
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
				if ctx.Err() != nil {
					res.Interrupted++
				} else {
					res.Failures = append(res.Failures, FailedProject{Path: proj.PathWithNS, Err: err.Error()})
				}
				mu.Unlock()
			}
		}(p)
	}

	wg.Wait()
	return res
}

func IsGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// CloneRepo clones into a hidden temp dir next to the target and renames it
// into place only after a successful clone, so an interrupted clone never
// leaves a half-populated repo that the next run would treat as existing.
func CloneRepo(ctx context.Context, proj Project, url, parentDir string) error {
	log.Printf("[clone] %s -> %s", proj.PathWithNS, parentDir)

	name := filepath.Base(proj.PathWithNS)
	repoDir := filepath.Join(parentDir, name)
	tmpDir := filepath.Join(parentDir, "."+name+".gitgit-tmp")

	if entries, err := os.ReadDir(repoDir); err == nil && len(entries) > 0 {
		err := fmt.Errorf("%s exists and is not a git repository", repoDir)
		log.Printf("[%s] clone failed: %v", proj.PathWithNS, err)
		return err
	}

	// Leftover from a run that was killed before it could clean up.
	if err := os.RemoveAll(tmpDir); err != nil {
		log.Printf("[%s] clone failed: removing stale %s: %v", proj.PathWithNS, tmpDir, err)
		return err
	}

	cmd := gitCommand(ctx, parentDir, "clone", "--quiet", "--", url, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmpDir)
		log.Printf("[%s] clone failed: %v\n%s", proj.PathWithNS, err, out)
		return err
	}

	if err := os.Rename(tmpDir, repoDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		log.Printf("[%s] clone failed: %v", proj.PathWithNS, err)
		return err
	}
	log.Printf("[clone] done %s", proj.PathWithNS)
	return nil
}

func UpdateRepo(ctx context.Context, proj Project, repoDir, expectedURL string) error {
	log.Printf("[update] %s -> %s", proj.PathWithNS, repoDir)

	// Sync remote URL if it changed (e.g. HTTPS→SSH switch).
	getURL := gitCommand(ctx, repoDir, "remote", "get-url", "origin")
	if urlOut, err := getURL.Output(); err == nil {
		current := strings.TrimSpace(string(urlOut))
		if current != expectedURL {
			log.Printf("[update] %s: remote origin %s -> %s", proj.PathWithNS, current, expectedURL)
			setURL := gitCommand(ctx, repoDir, "remote", "set-url", "--", "origin", expectedURL)
			if out, err := setURL.CombinedOutput(); err != nil {
				log.Printf("[%s] set-url failed: %v\n%s", proj.PathWithNS, err, out)
				return err
			}
		}
	}

	cmd := gitCommand(ctx, repoDir, "pull", "--all", "--quiet")
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("[%s] pull failed: %v\n%s", proj.PathWithNS, err, out)
		return err
	}
	log.Printf("[update] done %s", proj.PathWithNS)
	return nil
}
