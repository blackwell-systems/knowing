package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// initGitRepo creates a temporary git repo with an initial commit and returns
// the repo path and the initial commit hash.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "-C", dir, "init"},
		{"git", "-C", dir, "config", "user.email", "test@test.com"},
		{"git", "-C", dir, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("init git repo %v: %s: %v", args, out, err)
		}
	}
	return dir
}

func gitCommit(t *testing.T, dir, msg string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "add", "-A")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %s: %v", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "commit", "-m", msg, "--allow-empty")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s: %v", out, err)
	}
	cmd = exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	return string(out[:40])
}

func TestGitDiffFiles_ModifyAddDelete(t *testing.T) {
	dir := initGitRepo(t)

	// Initial file and commit.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit1 := gitCommit(t, dir, "initial")

	// Modify a.txt and add b.txt.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit2 := gitCommit(t, dir, "modify and add")

	changed, added, deleted, err := GitDiffFiles(dir, commit1, commit2)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}
	if !slices.Contains(changed, "a.txt") {
		t.Errorf("expected a.txt in changed, got %v", changed)
	}
	if !slices.Contains(added, "b.txt") {
		t.Errorf("expected b.txt in added, got %v", added)
	}

	// Delete b.txt.
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	commit3 := gitCommit(t, dir, "delete b")

	_, _, deleted, err = GitDiffFiles(dir, commit2, commit3)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}
	if !slices.Contains(deleted, "b.txt") {
		t.Errorf("expected b.txt in deleted, got %v", deleted)
	}
}

func TestGitDiffFiles_EmptyOldCommit(t *testing.T) {
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit1 := gitCommit(t, dir, "initial")
	_ = commit1

	_, added, _, err := GitDiffFiles(dir, "", "HEAD")
	if err != nil {
		t.Fatalf("GitDiffFiles with empty oldCommit: %v", err)
	}
	if !slices.Contains(added, "a.txt") || !slices.Contains(added, "b.txt") {
		t.Errorf("expected a.txt and b.txt in added, got %v", added)
	}
}
