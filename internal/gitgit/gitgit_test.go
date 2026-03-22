package gitgit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFetchProjects(t *testing.T) {
	page1 := []Project{
		{ID: 1, Name: "repo-a", PathWithNS: "group/repo-a", SSHURLToRepo: "git@gitlab.com:group/repo-a.git"},
		{ID: 2, Name: "repo-b", PathWithNS: "group/repo-b", SSHURLToRepo: "git@gitlab.com:group/repo-b.git"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		page := r.URL.Query().Get("page")
		switch page {
		case "1", "":
			_ = json.NewEncoder(w).Encode(page1)
		default:
			_ = json.NewEncoder(w).Encode([]Project{})
		}
	}))
	defer srv.Close()

	cfg := Config{
		GroupID: 42,
		URL:     srv.URL,
		Token:   "test-token",
	}

	projects, err := FetchProjects(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	if projects[0].Name != "repo-a" {
		t.Errorf("expected repo-a, got %s", projects[0].Name)
	}
}

func TestFetchProjectsUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := Config{
		GroupID: 42,
		URL:     srv.URL,
		Token:   "bad-token",
	}

	_, err := FetchProjects(cfg)
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestFetchProjectsPagination(t *testing.T) {
	page1 := make([]Project, 100)
	for i := range page1 {
		page1[i] = Project{ID: i, Name: "repo", PathWithNS: "group/repo"}
	}
	page2 := []Project{
		{ID: 100, Name: "last", PathWithNS: "group/last"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "1", "":
			_ = json.NewEncoder(w).Encode(page1)
		case "2":
			_ = json.NewEncoder(w).Encode(page2)
		default:
			_ = json.NewEncoder(w).Encode([]Project{})
		}
	}))
	defer srv.Close()

	cfg := Config{GroupID: 1, URL: srv.URL, Token: "t"}
	projects, err := FetchProjects(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 101 {
		t.Fatalf("expected 101 projects, got %d", len(projects))
	}
}

func TestFilterProjects(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "group/backend/api", Archived: false},
		{ID: 2, PathWithNS: "group/frontend/web", Archived: false},
		{ID: 3, PathWithNS: "group/backend/worker", Archived: false},
		{ID: 4, PathWithNS: "group/backend/old", Archived: true},
	}

	filtered, err := FilterProjects(projects, "backend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(filtered))
	}

	for _, p := range filtered {
		if p.Archived {
			t.Error("archived project should be excluded")
		}
	}
}

func TestFilterProjectsMatchAll(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "a/b"},
		{ID: 2, PathWithNS: "c/d"},
	}

	filtered, err := FilterProjects(projects, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2, got %d", len(filtered))
	}
}

func TestFilterProjectsBadRegex(t *testing.T) {
	_, err := FilterProjects(nil, "[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestIsGitRepo(t *testing.T) {
	dir := t.TempDir()

	if IsGitRepo(dir) {
		t.Error("empty dir should not be a git repo")
	}

	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if !IsGitRepo(dir) {
		t.Error("dir with .git should be detected as git repo")
	}
}

func TestIsGitRepoFileNotDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("ref: ..."), 0o644); err != nil {
		t.Fatal(err)
	}

	if IsGitRepo(dir) {
		t.Error(".git as file should not count as git repo dir")
	}
}

func TestIsGitRepoNonexistentDir(t *testing.T) {
	if IsGitRepo(filepath.Join(t.TempDir(), "does-not-exist")) {
		t.Error("nonexistent dir should not be a git repo")
	}
}

func TestFetchProjectsEmptyGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Project{})
	}))
	defer srv.Close()

	cfg := Config{GroupID: 1, URL: srv.URL, Token: "t"}
	projects, err := FetchProjects(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestFetchProjectsInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer srv.Close()

	cfg := Config{GroupID: 1, URL: srv.URL, Token: "t"}
	_, err := FetchProjects(cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestFetchProjectsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	cfg := Config{GroupID: 1, URL: srv.URL, Token: "t"}
	_, err := FetchProjects(cfg)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestFilterProjectsEmptyRegex(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "a/b"},
		{ID: 2, PathWithNS: "c/d"},
	}

	filtered, err := FilterProjects(projects, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(filtered))
	}
}

func TestFilterProjectsAllArchived(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "a/b", Archived: true},
		{ID: 2, PathWithNS: "c/d", Archived: true},
	}

	filtered, err := FilterProjects(projects, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(filtered))
	}
}

func TestProcessProjectsEmpty(t *testing.T) {
	cfg := Config{Workers: 2, DataDir: t.TempDir()}
	ProcessProjects(cfg, nil)
}

func TestProcessProjectsCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	projects := []Project{
		{ID: 1, PathWithNS: "group/sub/repo", SSHURLToRepo: "git@fake:g/s/r.git"},
	}

	cfg := Config{Workers: 1, DataDir: dir}
	ProcessProjects(cfg, projects)

	nsDir := filepath.Join(dir, "group", "sub")
	if info, err := os.Stat(nsDir); err != nil || !info.IsDir() {
		t.Errorf("expected namespace directory %s to be created", nsDir)
	}
}

func TestProcessProjectsBadDataDir(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "group/repo", SSHURLToRepo: "git@fake:g/r.git"},
		{ID: 2, PathWithNS: "group/repo2", SSHURLToRepo: "git@fake:g/r2.git"},
	}

	badDir := filepath.Join(t.TempDir(), "file-not-dir")
	if err := os.WriteFile(badDir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Workers: 1, DataDir: badDir}
	ProcessProjects(cfg, projects)
}
