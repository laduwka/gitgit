package gitgit

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
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

func FetchProjects(cfg Config) ([]Project, error) {
	var all []Project

	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/groups/%d/projects?per_page=100&page=%d&include_subgroups=true",
			cfg.URL, cfg.GroupID, page)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("PRIVATE-TOKEN", cfg.Token)

		resp, err := http.DefaultClient.Do(req)
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

func ProcessProjects(cfg Config, projects []Project) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, cfg.Workers)

	for _, p := range projects {
		wg.Add(1)
		sem <- struct{}{}

		go func(proj Project) {
			defer wg.Done()
			defer func() { <-sem }()

			cloneURL := proj.SSHURLToRepo
			if cfg.UseHTTP {
				cloneURL = proj.HTTPURLToRepo
			}

			nsDir := filepath.Join(cfg.DataDir, filepath.Dir(proj.PathWithNS))
			repoDir := filepath.Join(cfg.DataDir, proj.PathWithNS)

			if err := os.MkdirAll(nsDir, 0o750); err != nil {
				log.Printf("[%s] error creating dir: %v", proj.PathWithNS, err)
				return
			}

			if IsGitRepo(repoDir) {
				UpdateRepo(proj, repoDir)
			} else {
				CloneRepo(proj, cloneURL, nsDir)
			}
		}(p)
	}

	wg.Wait()
}

func IsGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func CloneRepo(proj Project, url, parentDir string) {
	fmt.Printf("[clone] %s\n", proj.PathWithNS)

	cmd := exec.Command("git", "clone", "--", url) // #nosec G204 -- url comes from GitLab API response, -- guards against option injection
	cmd.Dir = parentDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[%s] clone failed: %v", proj.PathWithNS, err)
	}
}

func UpdateRepo(proj Project, repoDir string) {
	fmt.Printf("[update] %s\n", proj.PathWithNS)

	cmd := exec.Command("git", "pull", "--all")
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[%s] pull failed: %v", proj.PathWithNS, err)
	}
}
