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

func TestGitDiffFiles_Rename(t *testing.T) {
	dir := initGitRepo(t)

	// Create a file and commit.
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit1 := gitCommit(t, dir, "add old.txt")

	// Rename the file.
	cmd := exec.Command("git", "-C", dir, "mv", "old.txt", "new.txt")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git mv: %s: %v", out, err)
	}
	commit2 := gitCommit(t, dir, "rename old to new")

	changed, added, deleted, err := GitDiffFiles(dir, commit1, commit2)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}
	_ = changed
	// Rename should produce a delete of old and add of new.
	if !slices.Contains(deleted, "old.txt") {
		t.Errorf("expected old.txt in deleted for rename, got deleted=%v", deleted)
	}
	if !slices.Contains(added, "new.txt") {
		t.Errorf("expected new.txt in added for rename, got added=%v", added)
	}
}

func TestGitDiffFiles_InvalidCommitHash(t *testing.T) {
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "initial")

	_, _, _, err := GitDiffFiles(dir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "HEAD")
	if err == nil {
		t.Fatal("expected error for invalid old commit hash")
	}
}

func TestGitDiffFiles_SameCommit(t *testing.T) {
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit1 := gitCommit(t, dir, "initial")

	changed, added, deleted, err := GitDiffFiles(dir, commit1, commit1)
	if err != nil {
		t.Fatalf("GitDiffFiles same commit: %v", err)
	}
	if len(changed) != 0 || len(added) != 0 || len(deleted) != 0 {
		t.Errorf("expected no diff for same commit, got changed=%v added=%v deleted=%v", changed, added, deleted)
	}
}

func TestGitDiffFiles_MultipleFileTypes(t *testing.T) {
	dir := initGitRepo(t)

	// Create multiple files and commit.
	for _, name := range []string{"a.go", "b.py", "c.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("init"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit1 := gitCommit(t, dir, "initial")

	// Modify one, delete another, add a new one.
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "b.py")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d.rs"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit2 := gitCommit(t, dir, "mixed changes")

	changed, added, deleted, err := GitDiffFiles(dir, commit1, commit2)
	if err != nil {
		t.Fatalf("GitDiffFiles: %v", err)
	}
	if !slices.Contains(changed, "a.go") {
		t.Errorf("expected a.go in changed, got %v", changed)
	}
	if !slices.Contains(deleted, "b.py") {
		t.Errorf("expected b.py in deleted, got %v", deleted)
	}
	if !slices.Contains(added, "d.rs") {
		t.Errorf("expected d.rs in added, got %v", added)
	}
}

func TestGitDiffFiles_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, _, _, err := GitDiffFiles(dir, "", "HEAD")
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}
