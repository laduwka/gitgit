package gitgit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit runs git in dir and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "init.defaultBranch=main"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// newSourceRepo creates a repo with one commit that can be cloned by path.
func newSourceRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	src := t.TempDir()
	runGit(t, src, "init", "-q")
	runGit(t, src, "commit", "-q", "--allow-empty", "-m", "init")
	return src
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected %s to not exist, stat err: %v", path, err)
	}
}

func TestCloneRepoSuccess(t *testing.T) {
	src := newSourceRepo(t)
	parent := t.TempDir()
	proj := Project{PathWithNS: "group/repo"}

	if err := CloneRepo(context.Background(), proj, src, parent); err != nil {
		t.Fatalf("CloneRepo: %v", err)
	}
	if !IsGitRepo(filepath.Join(parent, "repo")) {
		t.Error("expected cloned repo")
	}
	assertNotExist(t, filepath.Join(parent, ".repo.gitgit-tmp"))
}

func TestCloneRepoFailureLeavesNothing(t *testing.T) {
	newSourceRepo(t)
	parent := t.TempDir()
	proj := Project{PathWithNS: "group/repo"}

	if err := CloneRepo(context.Background(), proj, filepath.Join(t.TempDir(), "missing"), parent); err == nil {
		t.Fatal("expected error for missing source")
	}
	assertNotExist(t, filepath.Join(parent, "repo"))
	assertNotExist(t, filepath.Join(parent, ".repo.gitgit-tmp"))
}

func TestCloneRepoCanceledLeavesNothing(t *testing.T) {
	src := newSourceRepo(t)
	parent := t.TempDir()
	proj := Project{PathWithNS: "group/repo"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CloneRepo(ctx, proj, src, parent); err == nil {
		t.Fatal("expected error for canceled context")
	}
	assertNotExist(t, filepath.Join(parent, "repo"))
	assertNotExist(t, filepath.Join(parent, ".repo.gitgit-tmp"))
}

func TestCloneRepoRemovesStaleTmp(t *testing.T) {
	src := newSourceRepo(t)
	parent := t.TempDir()
	proj := Project{PathWithNS: "group/repo"}

	stale := filepath.Join(parent, ".repo.gitgit-tmp")
	if err := os.MkdirAll(filepath.Join(stale, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := CloneRepo(context.Background(), proj, src, parent); err != nil {
		t.Fatalf("CloneRepo: %v", err)
	}
	if !IsGitRepo(filepath.Join(parent, "repo")) {
		t.Error("expected cloned repo")
	}
	assertNotExist(t, stale)
}

func TestCloneRepoExistingNonGitDir(t *testing.T) {
	src := newSourceRepo(t)
	parent := t.TempDir()
	proj := Project{PathWithNS: "group/repo"}

	keep := filepath.Join(parent, "repo", "keep.txt")
	if err := os.MkdirAll(filepath.Dir(keep), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CloneRepo(context.Background(), proj, src, parent); err == nil {
		t.Fatal("expected error for existing non-git dir")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("existing file must be preserved: %v", err)
	}
	assertNotExist(t, filepath.Join(parent, ".repo.gitgit-tmp"))
}

func TestProcessProjectsCanceled(t *testing.T) {
	projects := []Project{
		{ID: 1, PathWithNS: "group/repo", SSHURLToRepo: "git@fake:g/r.git"},
		{ID: 2, PathWithNS: "group/repo2", SSHURLToRepo: "git@fake:g/r2.git"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := ProcessProjects(ctx, Config{Workers: 1, DataDir: t.TempDir()}, projects)
	if len(res.Failures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(res.Failures))
	}
	if res.Interrupted != 2 {
		t.Errorf("expected 2 interrupted, got %d", res.Interrupted)
	}
}
