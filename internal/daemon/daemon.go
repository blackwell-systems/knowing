package daemon

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// The Daemon is the long-running process that ties together file watching,
// incremental reindexing, LSP enrichment, and MCP query serving. It runs
// three concurrent goroutines:
//
//  1. watchLoop: reads CommitEvents from GitWatcher and enqueues index requests
//  2. indexWorker: processes index requests sequentially (holding a write lock)
//  3. MCP server: serves graph queries over HTTP (concurrent reads allowed)
//
// The write lock ensures readers always see a consistent graph state:
// queries block during indexing, and indexing blocks during queries.

// MCPServer abstracts the MCP transport so the daemon does not import the
// MCP package directly.
type MCPServer interface {
	ServeStdio(ctx context.Context) error
	ServeHTTP(ctx context.Context, addr string) error
}

// DaemonConfig holds all dependencies the daemon needs, injected as
// interfaces and callbacks to avoid tight coupling to concrete packages.
type DaemonConfig struct {
	Store     types.GraphStore
	IndexFunc func(ctx context.Context, repoURL, repoPath, commitHash string, changedFiles []string) error
	MCPAddr   string
	MCPServer MCPServer

	// EnrichFunc is called in the background after each successful index run.
	// It receives the repo hash and workspace root path to run LSP enrichment.
	// If nil, enrichment is skipped.
	EnrichFunc func(ctx context.Context, repoHash types.Hash, workspaceRoot string, changedFiles []string) error

	// TraceConfig holds configuration for the runtime trace ingestion pipeline.
	// If nil or Enabled is false, trace ingestion is not started.
	TraceConfig *TraceIngestConfig
}

// TraceIngestConfig holds configuration for the runtime trace ingestion pipeline.
type TraceIngestConfig struct {
	Enabled       bool
	OTLPEndpoint  string
	BatchSize     int
	BatchInterval time.Duration
}

// Daemon is the long-lived process that watches repositories for changes,
// triggers reindexing, and serves MCP queries.
type Daemon struct {
	cfg DaemonConfig

	mu      sync.RWMutex       // protects graph consistency: write lock during indexing, read lock during queries
	watcher *GitWatcher
	repos   map[string]string  // repoPath -> repoURL

	cancel context.CancelFunc // cancels the daemon's root context
	wg     sync.WaitGroup     // tracks all daemon goroutines for graceful shutdown

	indexQueue chan indexRequest // buffered channel of pending index requests
}

type indexRequest struct {
	repoURL      string
	repoPath     string
	changedFiles []string
}

// NewDaemon creates a Daemon with the given configuration. Call Start to
// begin watching and serving.
func NewDaemon(cfg DaemonConfig) *Daemon {
	return &Daemon{
		cfg:        cfg,
		repos:      make(map[string]string),
		indexQueue:  make(chan indexRequest, 128),
	}
}

// Start launches the file watcher, MCP server, and index worker. It blocks
// until the provided context is cancelled or Stop is called.
func (d *Daemon) Start(ctx context.Context) error {
	ctx, d.cancel = context.WithCancel(ctx)

	gw, err := NewGitWatcher(500 * time.Millisecond)
	if err != nil {
		return fmt.Errorf("daemon: create watcher: %w", err)
	}
	d.watcher = gw

	// Re-add any repos that were registered before Start.
	d.mu.RLock()
	for rp := range d.repos {
		_ = gw.Add(rp)
	}
	d.mu.RUnlock()

	// Index worker goroutine.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.indexWorker(ctx)
	}()

	// File change listener goroutine.
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.watchLoop(ctx)
	}()

	// MCP server goroutine (if configured).
	if d.cfg.MCPServer != nil && d.cfg.MCPAddr != "" {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			_ = d.cfg.MCPServer.ServeHTTP(ctx, d.cfg.MCPAddr)
		}()
	}

	// Trace ingestor goroutine (if configured).
	if d.cfg.TraceConfig != nil && d.cfg.TraceConfig.Enabled {
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			d.traceIngestLoop(ctx)
		}()
	}

	// Block until context is cancelled.
	<-ctx.Done()
	return d.shutdown()
}

// Stop triggers graceful shutdown of the daemon.
func (d *Daemon) Stop() error {
	if d.cancel != nil {
		d.cancel()
	}
	return nil
}

// WatchRepo registers a repository directory for file watching and future
// reindexing. If the daemon is already running the directory is added to the
// live watcher immediately.
func (d *Daemon) WatchRepo(repoPath string) error {
	d.mu.Lock()
	d.repos[repoPath] = repoPath // URL defaults to path; callers can set it.
	d.mu.Unlock()

	if d.watcher != nil {
		return d.watcher.Add(repoPath)
	}
	return nil
}

// UnwatchRepo removes a repository from the watch list.
func (d *Daemon) UnwatchRepo(repoPath string) error {
	d.mu.Lock()
	delete(d.repos, repoPath)
	d.mu.Unlock()

	if d.watcher != nil {
		return d.watcher.Remove(repoPath)
	}
	return nil
}

// GitHeadCommit reads the HEAD commit hash from a git repository without
// shelling out to git. It resolves symbolic refs and falls back to packed-refs
// when loose ref files are missing.
//
// Git HEAD resolution works in two stages:
//  1. Read .git/HEAD. If it contains a raw 40-char hex hash, the repo is in
//     "detached HEAD" state and that hash is returned directly.
//  2. If HEAD contains "ref: refs/heads/main" (a symbolic ref), resolve the
//     ref to a commit hash by reading the loose ref file (.git/refs/heads/main).
//     If the loose file is missing (git may pack refs for performance), fall
//     back to scanning .git/packed-refs for the ref.
func GitHeadCommit(repoPath string) (string, error) {
	gitDir := filepath.Join(repoPath, ".git")

	headBytes, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", fmt.Errorf("reading .git/HEAD: %w", err)
	}
	head := strings.TrimSpace(string(headBytes))

	// Detached HEAD: HEAD contains a raw 40-character commit hash.
	if !strings.HasPrefix(head, "ref: ") {
		if len(head) >= 40 {
			return head[:40], nil
		}
		return "", fmt.Errorf("unexpected HEAD content: %s", head)
	}

	// Symbolic ref (e.g., "ref: refs/heads/main"): resolve to commit hash.
	ref := strings.TrimPrefix(head, "ref: ")

	// Try the loose ref file first (most common case after a commit).
	looseRef := filepath.Join(gitDir, ref)
	if data, err := os.ReadFile(looseRef); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	// Fall back to packed-refs. Git packs loose refs into this single file
	// during gc operations. Each line is "<hash> <ref>" with comment lines
	// starting with "#" and peel lines starting with "^".
	packedRefsPath := filepath.Join(gitDir, "packed-refs")
	f, err := os.Open(packedRefsPath)
	if err != nil {
		return "", fmt.Errorf("ref %s not found in loose refs or packed-refs: %w", ref, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && parts[1] == ref {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("ref %s not found in packed-refs", ref)
}

// RLock acquires a read lock, allowing concurrent readers.
func (d *Daemon) RLock() { d.mu.RLock() }

// RUnlock releases the read lock.
func (d *Daemon) RUnlock() { d.mu.RUnlock() }

// traceIngestLoop runs the trace ingestion pipeline. Currently a placeholder
// that blocks until context cancellation. The real implementation will create
// an OTLPReceiver and Ingestor once the trace package is fully integrated.
func (d *Daemon) traceIngestLoop(ctx context.Context) {
	// TODO: Wire OTLPReceiver and Ingestor here.
	// For now, block until context is cancelled.
	<-ctx.Done()
}

// shutdown cleans up watcher and waits for goroutines.
func (d *Daemon) shutdown() error {
	close(d.indexQueue)
	if d.watcher != nil {
		_ = d.watcher.Close()
	}
	d.wg.Wait()
	return nil
}

// watchLoop reads commit events from GitWatcher and enqueues index requests.
func (d *Daemon) watchLoop(ctx context.Context) {
	if d.watcher == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-d.watcher.Events():
			if !ok {
				return
			}
			d.mu.RLock()
			repoURL := d.repos[ev.RepoPath]
			d.mu.RUnlock()
			if repoURL == "" {
				repoURL = ev.RepoPath
			}
			// Combine all changed paths for downstream consumers.
			allChanged := make([]string, 0, len(ev.ChangedFiles)+len(ev.AddedFiles)+len(ev.DeletedFiles))
			allChanged = append(allChanged, ev.ChangedFiles...)
			allChanged = append(allChanged, ev.AddedFiles...)
			allChanged = append(allChanged, ev.DeletedFiles...)
			select {
			case d.indexQueue <- indexRequest{
				repoURL:      repoURL,
				repoPath:     ev.RepoPath,
				changedFiles: allChanged,
			}:
			default:
				// Queue full; skip this event.
			}
		}
	}
}

// indexWorker processes index requests sequentially from the indexQueue channel.
// It holds the daemon's write lock during indexing to ensure readers see a
// consistent graph state. After each successful index, it spawns a background
// goroutine for LSP enrichment (which does not hold the write lock).
func (d *Daemon) indexWorker(ctx context.Context) {
	for req := range d.indexQueue {
		if ctx.Err() != nil {
			return
		}
		if d.cfg.IndexFunc == nil {
			continue
		}
		// Resolve HEAD commit hash; fall back to "unknown" if not a git repo.
		commit, err := GitHeadCommit(req.repoPath)
		if err != nil {
			commit = "unknown"
		}
		// Acquire write lock during indexing so readers wait.
		d.mu.Lock()
		indexErr := d.cfg.IndexFunc(ctx, req.repoURL, req.repoPath, commit, req.changedFiles)
		d.mu.Unlock()

		// Trigger background enrichment after successful index.
		if indexErr == nil && d.cfg.EnrichFunc != nil {
			repoHash := types.NewHash([]byte(req.repoURL))
			d.wg.Add(1)
			go func() {
				defer d.wg.Done()
				_ = d.cfg.EnrichFunc(ctx, repoHash, req.repoPath, req.changedFiles)
			}()
		}
	}
}
