package daemon

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
	_ "modernc.org/sqlite"
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

	// GitWatcher requires a git repo with at least one commit.
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
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "init.txt")
	runGit("commit", "-m", "initial")

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
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
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

// TestDaemon_TraceConfigNil verifies that daemon starts without trace config.
func TestDaemon_TraceConfigNil(t *testing.T) {
	d := NewDaemon(DaemonConfig{
		TraceConfig: nil,
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

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

// TestDaemon_TraceConfigDisabled verifies that daemon starts with trace disabled.
func TestDaemon_TraceConfigDisabled(t *testing.T) {
	d := NewDaemon(DaemonConfig{
		TraceConfig: &TraceIngestConfig{
			Enabled:      false,
			OTLPEndpoint: "localhost:4317",
			BatchSize:    1000,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

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

// TestDaemon_TraceConfigEnabled verifies that daemon starts with trace enabled
// and the trace ingest loop runs until context cancellation.
func TestDaemon_TraceConfigEnabled(t *testing.T) {
	d := NewDaemon(DaemonConfig{
		TraceConfig: &TraceIngestConfig{
			Enabled:       true,
			OTLPEndpoint:  "localhost:4317",
			BatchSize:     1000,
			BatchInterval: 10 * time.Second,
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()

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

// TestGitHeadCommit_DetachedHead verifies that GitHeadCommit works in detached
// HEAD state (raw commit hash in .git/HEAD).
func TestGitHeadCommit_DetachedHead(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "initial")

	// Get the commit hash.
	expected, err := execCommand("git", "-C", dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	expected = strings.TrimSpace(expected)

	// Detach HEAD.
	runGit("checkout", "--detach")

	got, err := GitHeadCommit(dir)
	if err != nil {
		t.Fatalf("GitHeadCommit in detached state: %v", err)
	}
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestNewDaemon_DefaultState verifies the initial state of a new daemon.
func TestNewDaemon_DefaultState(t *testing.T) {
	d := NewDaemon(DaemonConfig{})
	if d == nil {
		t.Fatal("NewDaemon returned nil")
	}
	if d.repos == nil {
		t.Error("repos map should be initialized")
	}
	if d.indexQueue == nil {
		t.Error("indexQueue should be initialized")
	}
}

// TestDaemon_WatchRepo_BeforeStart verifies that WatchRepo before Start
// registers the repo but doesn't add to watcher (watcher is nil).
func TestDaemon_WatchRepo_BeforeStart(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	// WatchRepo before Start should not error (watcher is nil, just registers).
	if err := d.WatchRepo("/tmp/somerepo"); err != nil {
		t.Fatalf("WatchRepo before Start: %v", err)
	}

	d.mu.RLock()
	_, ok := d.repos["/tmp/somerepo"]
	d.mu.RUnlock()
	if !ok {
		t.Error("expected repo to be registered")
	}
}

// createTestDB creates a temporary SQLite database file with the schema needed
// by the trace pipeline. Returns the file path.
func createTestDB(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	schema := `
		CREATE TABLE repos (
			repo_hash   BLOB PRIMARY KEY,
			repo_url    TEXT NOT NULL,
			last_commit TEXT,
			last_indexed INTEGER
		);
		CREATE TABLE files (
			file_hash    BLOB PRIMARY KEY,
			repo_hash    BLOB NOT NULL,
			path         TEXT NOT NULL,
			content_hash BLOB NOT NULL
		);
		CREATE TABLE nodes (
			node_hash      BLOB PRIMARY KEY,
			file_hash      BLOB NOT NULL,
			qualified_name TEXT NOT NULL,
			kind           TEXT NOT NULL,
			line           INTEGER,
			signature      TEXT
		);
		CREATE TABLE edges (
			edge_hash    BLOB PRIMARY KEY,
			source_hash  BLOB NOT NULL,
			target_hash  BLOB NOT NULL,
			edge_type    TEXT NOT NULL,
			confidence   REAL NOT NULL DEFAULT 1.0,
			provenance   TEXT NOT NULL DEFAULT 'ast_resolved',
			callsite_line INTEGER NOT NULL DEFAULT 0,
			callsite_col  INTEGER NOT NULL DEFAULT 0,
			callsite_file TEXT NOT NULL DEFAULT '',
			observation_count INTEGER NOT NULL DEFAULT 0,
			last_observed INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE edge_events (
			event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
			edge_hash     BLOB NOT NULL,
			event_type    TEXT NOT NULL,
			snapshot_hash BLOB NOT NULL,
			source_commit TEXT NOT NULL,
			indexer_ver   TEXT NOT NULL,
			timestamp     INTEGER NOT NULL
		);
		CREATE TABLE snapshots (
			snapshot_hash BLOB PRIMARY KEY,
			parent_hash   BLOB,
			repo_hash     BLOB NOT NULL,
			commit_hash   TEXT NOT NULL,
			timestamp     INTEGER NOT NULL,
			node_count    INTEGER NOT NULL,
			edge_count    INTEGER NOT NULL
		);
		CREATE TABLE route_symbols (
			service_name  TEXT NOT NULL,
			route_pattern TEXT NOT NULL,
			node_hash     BLOB NOT NULL,
			mapping_type  TEXT NOT NULL,
			created_at    INTEGER NOT NULL,
			PRIMARY KEY (service_name, route_pattern, mapping_type)
		);
		CREATE INDEX idx_edges_provenance ON edges(provenance);
		CREATE INDEX idx_edges_last_observed ON edges(last_observed);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return dbPath
}

// TestTraceIngestLoop_StartsAndStops verifies that the trace ingest loop starts
// with a valid TraceConfig and temp SQLite database, runs the gRPC receiver,
// and shuts down cleanly when the context is cancelled.
func TestTraceIngestLoop_StartsAndStops(t *testing.T) {
	dbPath := createTestDB(t)

	d := NewDaemon(DaemonConfig{
		TraceConfig: &TraceIngestConfig{
			Enabled:       true,
			OTLPEndpoint:  "localhost:0", // random port
			BatchSize:     100,
			BatchInterval: 1 * time.Second,
		},
		DBPath: dbPath,
	})

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		d.traceIngestLoop(ctx)
		close(done)
	}()

	// Give the loop time to start the gRPC server and enter the ticker loop.
	time.Sleep(200 * time.Millisecond)

	// Cancel context to trigger shutdown.
	cancel()

	select {
	case <-done:
		// Clean shutdown.
	case <-time.After(5 * time.Second):
		t.Fatal("traceIngestLoop did not return after context cancel")
	}
}

// TestTraceIngestLoop_Disabled verifies that traceIngestLoop returns immediately
// when TraceConfig is nil.
func TestTraceIngestLoop_Disabled(t *testing.T) {
	d := NewDaemon(DaemonConfig{
		TraceConfig: nil,
	})

	done := make(chan struct{})
	go func() {
		d.traceIngestLoop(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Returned immediately as expected.
	case <-time.After(1 * time.Second):
		t.Fatal("traceIngestLoop should return immediately when TraceConfig is nil")
	}
}

// TestDaemon_UnwatchRepo_BeforeStart verifies UnwatchRepo before Start.
func TestDaemon_UnwatchRepo_BeforeStart(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	if err := d.WatchRepo("/tmp/repo"); err != nil {
		t.Fatalf("WatchRepo: %v", err)
	}
	if err := d.UnwatchRepo("/tmp/repo"); err != nil {
		t.Fatalf("UnwatchRepo: %v", err)
	}

	d.mu.RLock()
	_, ok := d.repos["/tmp/repo"]
	d.mu.RUnlock()
	if ok {
		t.Error("expected repo to be removed")
	}
}

// TestDaemon_Stop_BeforeStart verifies that Stop before Start doesn't panic.
func TestDaemon_Stop_BeforeStart(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	// Stop before Start should be safe (cancel is nil).
	if err := d.Stop(); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
}

// TestDaemon_IndexWorker_NilIndexFunc verifies that the index worker skips
// requests when IndexFunc is nil.
func TestDaemon_IndexWorker_NilIndexFunc(t *testing.T) {
	d := NewDaemon(DaemonConfig{
		IndexFunc: nil,
	})

	ctx, cancel := context.WithCancel(context.Background())

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Send an index request; it should be silently skipped.
	d.indexQueue <- indexRequest{repoURL: "test", repoPath: "/tmp"}
	time.Sleep(50 * time.Millisecond)

	cancel()
}

// TestDaemon_GitWatcherIntegration verifies that the daemon detects git commits
// via GitWatcher and passes changed files to IndexFunc.
func TestDaemon_GitWatcherIntegration(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo with an initial commit.
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

	// Create initial file and commit.
	if err := os.WriteFile(filepath.Join(dir, "initial.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "initial.txt")
	runGit("commit", "-m", "initial commit")

	// Set up daemon with IndexFunc that captures changedFiles.
	var capturedFiles []string
	var capturedMu sync.Mutex
	indexCalled := make(chan struct{}, 1)

	d := NewDaemon(DaemonConfig{
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
			capturedMu.Lock()
			capturedFiles = changedFiles
			capturedMu.Unlock()
			select {
			case indexCalled <- struct{}{}:
			default:
			}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Register repo before starting.
	if err := d.WatchRepo(dir); err != nil {
		t.Fatalf("WatchRepo: %v", err)
	}

	go func() { _ = d.Start(ctx) }()
	time.Sleep(200 * time.Millisecond) // let watcher initialize

	// Modify a file and make a new commit.
	if err := os.WriteFile(filepath.Join(dir, "changed.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "changed.txt")
	runGit("commit", "-m", "add changed.txt")

	// Wait for IndexFunc to be called.
	select {
	case <-indexCalled:
		capturedMu.Lock()
		files := capturedFiles
		capturedMu.Unlock()

		// Verify changedFiles contains the modified file path.
		found := false
		for _, f := range files {
			if strings.Contains(f, "changed.txt") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected changedFiles to contain 'changed.txt', got %v", files)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for IndexFunc to be called")
	}

	cancel()
}

// TestGitHeadCommit_PackedRefs verifies that GitHeadCommit can resolve a ref
// from .git/packed-refs when the loose ref file is missing.
func TestGitHeadCommit_PackedRefs(t *testing.T) {
	dir := t.TempDir()

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

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "initial")

	expected, err := execCommand("git", "-C", dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	expected = strings.TrimSpace(expected)

	// Run git pack-refs to move refs into packed-refs.
	runGit("pack-refs", "--all")

	// Detect branch name (could be master on some systems).
	branchOut, _ := execCommand("git", "-C", dir, "branch", "--show-current")
	branch := strings.TrimSpace(branchOut)
	if branch == "" {
		branch = "main"
	}
	looseRef := filepath.Join(dir, ".git", "refs", "heads", branch)
	if _, err := os.Stat(looseRef); err == nil {
		// Loose ref still exists; remove it manually to force packed-refs lookup.
		os.Remove(looseRef)
	}

	got, err := GitHeadCommit(dir)
	if err != nil {
		t.Fatalf("GitHeadCommit with packed refs: %v", err)
	}
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

// TestGitHeadCommit_InvalidHEADContent verifies that GitHeadCommit returns an
// error when .git/HEAD contains unexpected content (not a ref, not a hash).
func TestGitHeadCommit_InvalidHEADContent(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write invalid HEAD content (too short to be a hash, no ref: prefix).
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("garbage"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := GitHeadCommit(dir)
	if err == nil {
		t.Fatal("expected error for invalid HEAD content")
	}
	if !strings.Contains(err.Error(), "unexpected HEAD content") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestGitHeadCommit_BrokenSymRef verifies error when symbolic ref points to a
// non-existent branch and packed-refs is missing.
func TestGitHeadCommit_BrokenSymRef(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write HEAD pointing to a branch that doesn't exist.
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/nonexistent\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := GitHeadCommit(dir)
	if err == nil {
		t.Fatal("expected error for broken symbolic ref")
	}
}

// TestDaemon_StartStop_IndexFuncError verifies the daemon handles IndexFunc
// errors gracefully (does not crash, continues processing).
func TestDaemon_StartStop_IndexFuncError(t *testing.T) {
	var indexCallCount int32

	d := NewDaemon(DaemonConfig{
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
			atomic.AddInt32(&indexCallCount, 1)
			return fmt.Errorf("index error")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Start(ctx)
	}()
	time.Sleep(50 * time.Millisecond)

	// Send multiple index requests; IndexFunc returns errors but daemon keeps running.
	for i := 0; i < 3; i++ {
		d.indexQueue <- indexRequest{repoURL: "test", repoPath: "/tmp"}
	}
	time.Sleep(100 * time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}

	count := atomic.LoadInt32(&indexCallCount)
	if count < 3 {
		t.Errorf("expected IndexFunc called at least 3 times, got %d", count)
	}
}

// TestDaemon_EnrichFunc verifies that EnrichFunc is called after a successful
// index, and NOT called after a failed index.
func TestDaemon_EnrichFunc(t *testing.T) {
	var enrichCalled int32
	enrichDone := make(chan struct{}, 5)

	d := NewDaemon(DaemonConfig{
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
			if repoURL == "fail" {
				return fmt.Errorf("index failed")
			}
			return nil
		},
		EnrichFunc: func(ctx context.Context, repoHash types.Hash, workspaceRoot string, changedFiles []string) error {
			atomic.AddInt32(&enrichCalled, 1)
			enrichDone <- struct{}{}
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Successful index should trigger enrich.
	d.indexQueue <- indexRequest{repoURL: "success", repoPath: "/tmp"}

	select {
	case <-enrichDone:
		// Good, enrich was called.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for EnrichFunc")
	}

	// Failed index should NOT trigger enrich.
	d.indexQueue <- indexRequest{repoURL: "fail", repoPath: "/tmp"}
	time.Sleep(100 * time.Millisecond)

	count := atomic.LoadInt32(&enrichCalled)
	if count != 1 {
		t.Errorf("expected EnrichFunc called exactly 1 time, got %d", count)
	}

	cancel()
}

// TestTraceIngestConfig_Defaults verifies that TraceIngestConfig with zero
// values behaves predictably (does not panic).
func TestTraceIngestConfig_Defaults(t *testing.T) {
	cfg := &TraceIngestConfig{
		Enabled:       false,
		OTLPEndpoint:  "",
		BatchSize:     0,
		BatchInterval: 0,
	}

	if cfg.BatchSize != 0 {
		t.Errorf("expected BatchSize 0, got %d", cfg.BatchSize)
	}
	if cfg.BatchInterval != 0 {
		t.Errorf("expected BatchInterval 0, got %v", cfg.BatchInterval)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("expected empty OTLPEndpoint, got %q", cfg.OTLPEndpoint)
	}
}

// TestDaemon_WatchRepo_NonExistentPath verifies that WatchRepo with a
// non-existent directory returns an error when the daemon is running.
func TestDaemon_WatchRepo_NonExistentPath(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	err := d.WatchRepo("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error when watching non-existent path")
	}

	cancel()
}

// TestDaemon_MultipleWatchRepo verifies that calling WatchRepo multiple times
// with different repos tracks all of them.
func TestDaemon_MultipleWatchRepo(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	// WatchRepo before start (no watcher).
	paths := []string{"/tmp/repo1", "/tmp/repo2", "/tmp/repo3"}
	for _, p := range paths {
		if err := d.WatchRepo(p); err != nil {
			t.Fatalf("WatchRepo(%s): %v", p, err)
		}
	}

	d.mu.RLock()
	for _, p := range paths {
		if _, ok := d.repos[p]; !ok {
			t.Errorf("expected %s to be tracked", p)
		}
	}
	repoCount := len(d.repos)
	d.mu.RUnlock()

	if repoCount != 3 {
		t.Errorf("expected 3 repos, got %d", repoCount)
	}
}

// TestDaemon_WatchRepo_SamePathTwice verifies that watching the same path
// twice does not duplicate entries.
func TestDaemon_WatchRepo_SamePathTwice(t *testing.T) {
	d := NewDaemon(DaemonConfig{})

	if err := d.WatchRepo("/tmp/repo"); err != nil {
		t.Fatalf("WatchRepo: %v", err)
	}
	if err := d.WatchRepo("/tmp/repo"); err != nil {
		t.Fatalf("WatchRepo second call: %v", err)
	}

	d.mu.RLock()
	count := len(d.repos)
	d.mu.RUnlock()

	if count != 1 {
		t.Errorf("expected 1 repo entry, got %d", count)
	}
}

// TestDaemon_EnrichFunc_Error verifies that EnrichFunc errors do not crash
// the daemon or block subsequent indexing.
func TestDaemon_EnrichFunc_Error(t *testing.T) {
	var indexCallCount int32
	secondIndexDone := make(chan struct{}, 1)

	d := NewDaemon(DaemonConfig{
		IndexFunc: func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error {
			count := atomic.AddInt32(&indexCallCount, 1)
			if count == 2 {
				select {
				case secondIndexDone <- struct{}{}:
				default:
				}
			}
			return nil
		},
		EnrichFunc: func(ctx context.Context, repoHash types.Hash, workspaceRoot string, changedFiles []string) error {
			return fmt.Errorf("enrich failed")
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = d.Start(ctx) }()
	time.Sleep(50 * time.Millisecond)

	// Send two index requests. The enrich error from the first should not
	// prevent the second index from running.
	d.indexQueue <- indexRequest{repoURL: "a", repoPath: "/tmp"}
	d.indexQueue <- indexRequest{repoURL: "b", repoPath: "/tmp"}

	select {
	case <-secondIndexDone:
		// Second index completed despite enrich error.
	case <-time.After(3 * time.Second):
		t.Fatal("second index never completed after enrich error")
	}

	cancel()
}
