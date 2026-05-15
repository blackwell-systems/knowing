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
