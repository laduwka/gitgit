package gitgit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// newClonePair returns an upstream repo with one commit and a clone of it
// at <dataDir>/g/cl, so it can also be used with ProcessProjects.
func newClonePair(t *testing.T) (up, cl, dataDir string) {
	t.Helper()
	up = newSourceRepo(t)
	dataDir = t.TempDir()
	cl = filepath.Join(dataDir, "g", "cl")
	if err := os.MkdirAll(filepath.Dir(cl), 0o750); err != nil {
		t.Fatal(err)
	}
	runGit(t, filepath.Dir(cl), "clone", "-q", up, cl)
	return up, cl, dataDir
}

func commitIn(t *testing.T, dir, msg string) {
	t.Helper()
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
}

func revParse(t *testing.T, dir, rev string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "rev-parse", rev))
}

func updateClone(up, cl string) error {
	return UpdateRepo(context.Background(), Project{PathWithNS: "g/cl"}, cl, up)
}

func requireSkipped(t *testing.T, err error, want string) {
	t.Helper()
	var skip *SkippedError
	if !errors.As(err, &skip) {
		t.Fatalf("expected *SkippedError, got %v", err)
	}
	if !strings.Contains(skip.Reason, want) {
		t.Errorf("reason %q does not contain %q", skip.Reason, want)
	}
}

func TestUpdateRepoFastForward(t *testing.T) {
	up, cl, _ := newClonePair(t)
	commitIn(t, up, "next")

	if err := updateClone(up, cl); err != nil {
		t.Fatalf("UpdateRepo: %v", err)
	}
	if got, want := revParse(t, cl, "HEAD"), revParse(t, up, "HEAD"); got != want {
		t.Errorf("HEAD = %s, want %s", got, want)
	}
}

func TestUpdateRepoInSyncAndAhead(t *testing.T) {
	up, cl, _ := newClonePair(t)
	if err := updateClone(up, cl); err != nil {
		t.Fatalf("in sync: %v", err)
	}
	commitIn(t, cl, "local")
	if err := updateClone(up, cl); err != nil {
		t.Fatalf("ahead: %v", err)
	}
}

func TestUpdateRepoDiverged(t *testing.T) {
	up, cl, _ := newClonePair(t)
	commitIn(t, up, "remote")
	commitIn(t, cl, "local")
	before := revParse(t, cl, "HEAD")

	requireSkipped(t, updateClone(up, cl), "diverged")
	if after := revParse(t, cl, "HEAD"); after != before {
		t.Errorf("HEAD moved from %s to %s", before, after)
	}
}

func TestUpdateRepoUpstreamGone(t *testing.T) {
	up, cl, _ := newClonePair(t)
	runGit(t, up, "branch", "feat")
	runGit(t, cl, "fetch", "-q")
	runGit(t, cl, "switch", "-q", "-c", "feat", "--track", "origin/feat")
	runGit(t, up, "branch", "-q", "-D", "feat")

	requireSkipped(t, updateClone(up, cl), "is gone")
}

func TestUpdateRepoNoUpstream(t *testing.T) {
	up, cl, _ := newClonePair(t)
	runGit(t, cl, "switch", "-q", "-c", "local")

	requireSkipped(t, updateClone(up, cl), "no upstream")
}

func TestUpdateRepoDetachedHead(t *testing.T) {
	up, cl, _ := newClonePair(t)
	runGit(t, cl, "checkout", "-q", "--detach")

	requireSkipped(t, updateClone(up, cl), "detached HEAD")
}

func TestUpdateRepoUntrackedFileInTheWay(t *testing.T) {
	up, cl, _ := newClonePair(t)
	if err := os.WriteFile(filepath.Join(up, "f.txt"), []byte("remote"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, up, "add", "f.txt")
	commitIn(t, up, "add f")
	local := filepath.Join(cl, "f.txt")
	if err := os.WriteFile(local, []byte("local"), 0o600); err != nil {
		t.Fatal(err)
	}

	requireSkipped(t, updateClone(up, cl), "fast-forward of main")
	if b, err := os.ReadFile(local); err != nil || string(b) != "local" {
		t.Errorf("local file changed: %q, %v", b, err)
	}
}

func TestUpdateRepoFetchFailureIsError(t *testing.T) {
	_, cl, _ := newClonePair(t)
	missing := filepath.Join(t.TempDir(), "missing")

	err := UpdateRepo(context.Background(), Project{PathWithNS: "g/cl"}, cl, missing)
	if err == nil {
		t.Fatal("expected error")
	}
	var skip *SkippedError
	if errors.As(err, &skip) {
		t.Fatalf("fetch failure must not be a skip: %v", err)
	}
}

func TestProcessProjectsCollectsSkipped(t *testing.T) {
	up, cl, dataDir := newClonePair(t)
	commitIn(t, up, "remote")
	commitIn(t, cl, "local")

	projects := []Project{{ID: 1, PathWithNS: "g/cl", SSHURLToRepo: up}}
	res := ProcessProjects(context.Background(), Config{Workers: 1, DataDir: dataDir}, projects)
	if len(res.Failures) != 0 {
		t.Errorf("expected 0 failures, got %v", res.Failures)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Path != "g/cl" {
		t.Errorf("expected g/cl skipped, got %v", res.Skipped)
	}
}
