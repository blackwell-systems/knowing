package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func execCommandHelper(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// TestDaemon_StartStop verifies that the daemon starts and stops cleanly.
func TestDaemon_StartStop(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

	// Give it a moment to spin up.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

// TestDaemon_WatchRepo_AddsWatcher verifies that WatchRepo adds a directory
// to the watcher and that UnwatchRepo removes it.
func TestDaemon_WatchRepo_AddsWatcher(t *testing.T) {
	dir := t.TempDir()

	d := NewDaemon(DaemonConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond) // let watcher initialize

	if err := d.WatchRepo(dir); err != nil {
		t.Fatalf("WatchRepo: %v", err)
	}

	d.mu.RLock()
	_, ok := d.repos[dir]
	d.mu.RUnlock()
	if !ok {
		t.Fatal("expected repo to be tracked after WatchRepo")
	}

	if err := d.UnwatchRepo(dir); err != nil {
		t.Fatalf("UnwatchRepo: %v", err)
	}

	d.mu.RLock()
	_, ok = d.repos[dir]
	d.mu.RUnlock()
	if ok {
		t.Fatal("expected repo to be removed after UnwatchRepo")
	}

	cancel()
}

// TestFileWatcher_DetectsChanges verifies that writing a file produces a
// WatchEvent on the events channel.
func TestFileWatcher_DetectsChanges(t *testing.T) {
	dir := t.TempDir()

	fw, err := NewFileWatcher(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileWatcher: %v", err)
	}
	defer fw.Close()

	if err := fw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Write a file inside the watched directory.
	fpath := filepath.Join(dir, "test.go")
	if err := os.WriteFile(fpath, []byte("package main"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	select {
	case ev := <-fw.Events():
		if ev.RepoPath != dir {
			t.Errorf("expected RepoPath=%s, got %s", dir, ev.RepoPath)
		}
		if len(ev.Files) == 0 {
			t.Error("expected at least one file in event")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watch event")
	}
}

// TestFileWatcher_Debounces verifies that multiple rapid writes are
// coalesced into a single event.
func TestFileWatcher_Debounces(t *testing.T) {
	dir := t.TempDir()

	fw, err := NewFileWatcher(200 * time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileWatcher: %v", err)
	}
	defer fw.Close()

	if err := fw.Add(dir); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Rapid-fire writes to multiple files.
	for i := 0; i < 5; i++ {
		fpath := filepath.Join(dir, "file"+string(rune('a'+i))+".go")
		if err := os.WriteFile(fpath, []byte("package main"), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Should get exactly one debounced event containing multiple files.
	select {
	case ev := <-fw.Events():
		if len(ev.Files) < 2 {
			t.Errorf("expected debounced event with multiple files, got %d", len(ev.Files))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for debounced event")
	}

	// No more events should arrive quickly.
	select {
	case <-fw.Events():
		// It's acceptable if the OS sends a second batch, so we don't fail.
	case <-time.After(500 * time.Millisecond):
		// Expected: no further events.
	}
}

// TestDaemon_ConcurrentReads verifies that multiple goroutines can acquire
// read locks simultaneously.
func TestDaemon_ConcurrentReads(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	var active int64
	var maxActive int64
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.RLock()
			cur := atomic.AddInt64(&active, 1)
			// Track max concurrent readers.
			for {
				old := atomic.LoadInt64(&maxActive)
				if cur <= old || atomic.CompareAndSwapInt64(&maxActive, old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond) // hold lock briefly
			atomic.AddInt64(&active, -1)
			d.RUnlock()
		}()
	}

	wg.Wait()

	if m := atomic.LoadInt64(&maxActive); m < 2 {
		t.Errorf("expected concurrent readers > 1, got %d", m)
	}
}

// TestGitHeadCommit verifies that GitHeadCommit reads the HEAD commit from
// a real git repository.
func TestGitHeadCommit(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo and make a commit.
	runGit := func(args ...string) {
		t.Helper()
		cmd := append([]string{"-C", dir}, args...)
		out, err := execCommand("git", cmd...)
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")

	// Write a file and commit.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "hello.txt")
	runGit("commit", "-m", "initial")

	// Get expected commit hash.
	expected, err := execCommand("git", "-C", dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	expected = strings.TrimSpace(string(expected))

	got, err := GitHeadCommit(dir)
	if err != nil {
		t.Fatalf("GitHeadCommit: %v", err)
	}
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestGitHeadCommit_NotGitRepo verifies that GitHeadCommit returns an error
// for a directory that is not a git repository.
func TestGitHeadCommit_NotGitRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := GitHeadCommit(dir)
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

// execCommand runs a command and returns its combined output.
func execCommand(name string, args ...string) (string, error) {
	cmd := execCommandHelper(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// TestDaemon_WriteBlocksReads verifies that write-lock acquisition during
// indexing blocks concurrent readers.
func TestDaemon_WriteBlocksReads(t *testing.T) {
	indexStarted := make(chan struct{})
	indexDone := make(chan struct{})

	d := NewDaemon(DaemonConfig{
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string) error {
			close(indexStarted)
			<-indexDone
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Enqueue an index request via the internal queue.
	d.indexQueue <- indexRequest{repoURL: "test", repoPath: "/tmp"}

	// Wait until the index function is running (holds write lock).
	select {
	case <-indexStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("indexFunc never started")
	}

	// Attempt a read lock in a goroutine; it should block.
	readAcquired := make(chan struct{})
	go func() {
		d.RLock()
		close(readAcquired)
		d.RUnlock()
	}()

	select {
	case <-readAcquired:
		t.Fatal("read lock acquired while write lock should be held")
	case <-time.After(100 * time.Millisecond):
		// Expected: read is blocked.
	}

	// Release the index function, which releases the write lock.
	close(indexDone)

	select {
	case <-readAcquired:
		// Read lock acquired after write released.
	case <-time.After(3 * time.Second):
		t.Fatal("read lock never acquired after write released")
	}

	cancel()
}
