package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGitWatcher_DetectsCommit(t *testing.T) {
	dir := initGitRepo(t)

	// Create initial commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "initial")

	gw, err := NewGitWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewGitWatcher: %v", err)
	}
	defer gw.Close()

	if err := gw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Make a new commit that modifies init.txt.
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "second commit")

	// Wait for the commit event.
	select {
	case ev := <-gw.Events():
		if ev.OldCommit == ev.NewCommit {
			t.Error("expected different old and new commits")
		}
		if ev.RepoPath == "" {
			t.Error("expected non-empty RepoPath")
		}
		// The changed files should include init.txt.
		found := false
		for _, f := range ev.ChangedFiles {
			if f == "init.txt" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected init.txt in ChangedFiles, got %v", ev.ChangedFiles)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for CommitEvent")
	}
}

func TestGitWatcher_CloseShutdown(t *testing.T) {
	gw, err := NewGitWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewGitWatcher: %v", err)
	}

	// Close should return without hanging.
	done := make(chan struct{})
	go func() {
		if err := gw.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not return in time")
	}
}

func TestGitWatcher_StartStop_Lifecycle(t *testing.T) {
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir, "initial")

	gw, err := NewGitWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewGitWatcher: %v", err)
	}

	if err := gw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Remove and re-add to test lifecycle.
	if err := gw.Remove(dir); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// After remove, repos map should not contain dir.
	gw.mu.Lock()
	_, tracked := gw.repos[dir]
	gw.mu.Unlock()
	if tracked {
		t.Error("expected repo to be untracked after Remove")
	}

	// Re-add should work.
	if err := gw.Add(dir); err != nil {
		t.Fatalf("Re-Add: %v", err)
	}

	gw.mu.Lock()
	_, tracked = gw.repos[dir]
	gw.mu.Unlock()

	// The path stored is the absolute path; resolve dir to compare.
	absDir, _ := filepath.Abs(dir)
	gw.mu.Lock()
	_, tracked = gw.repos[absDir]
	gw.mu.Unlock()
	if !tracked {
		t.Error("expected repo to be tracked after re-Add")
	}

	if err := gw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Events channel should be closed after Close.
	_, ok := <-gw.Events()
	if ok {
		t.Error("expected Events channel to be closed after Close")
	}
}

func TestGitWatcher_AddNonGitRepo(t *testing.T) {
	dir := t.TempDir()

	gw, err := NewGitWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewGitWatcher: %v", err)
	}
	defer gw.Close()

	// Add should fail because there is no .git/HEAD to read.
	if err := gw.Add(dir); err == nil {
		t.Fatal("expected error when adding non-git repo")
	}
}

func TestGitWatcher_MultipleRepos(t *testing.T) {
	// Create two independent repos.
	dir1 := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir1, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir1, "initial")

	dir2 := initGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir2, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommit(t, dir2, "initial")

	gw, err := NewGitWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewGitWatcher: %v", err)
	}
	defer gw.Close()

	if err := gw.Add(dir1); err != nil {
		t.Fatalf("Add dir1: %v", err)
	}
	if err := gw.Add(dir2); err != nil {
		t.Fatalf("Add dir2: %v", err)
	}

	absDir1, _ := filepath.Abs(dir1)
	absDir2, _ := filepath.Abs(dir2)

	gw.mu.Lock()
	_, has1 := gw.repos[absDir1]
	_, has2 := gw.repos[absDir2]
	gw.mu.Unlock()

	if !has1 {
		t.Error("expected dir1 to be tracked")
	}
	if !has2 {
		t.Error("expected dir2 to be tracked")
	}
}
