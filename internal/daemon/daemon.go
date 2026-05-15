package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

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
	IndexFunc func(ctx context.Context, repoURL, repoPath, commitHash string) error
	MCPAddr   string
	MCPServer MCPServer
}

// Daemon is the long-lived process that watches repositories for changes,
// triggers reindexing, and serves MCP queries.
type Daemon struct {
	cfg DaemonConfig

	mu      sync.RWMutex
	watcher *FileWatcher
	repos   map[string]string // repoPath -> repoURL

	cancel context.CancelFunc
	wg     sync.WaitGroup

	indexQueue chan indexRequest
}

type indexRequest struct {
	repoURL  string
	repoPath string
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

	fw, err := NewFileWatcher(500 * time.Millisecond)
	if err != nil {
		return fmt.Errorf("daemon: create watcher: %w", err)
	}
	d.watcher = fw

	// Re-add any repos that were registered before Start.
	d.mu.RLock()
	for rp := range d.repos {
		_ = fw.Add(rp)
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

// RLock acquires a read lock, allowing concurrent readers.
func (d *Daemon) RLock() { d.mu.RLock() }

// RUnlock releases the read lock.
func (d *Daemon) RUnlock() { d.mu.RUnlock() }

// shutdown cleans up watcher and waits for goroutines.
func (d *Daemon) shutdown() error {
	close(d.indexQueue)
	if d.watcher != nil {
		_ = d.watcher.Close()
	}
	d.wg.Wait()
	return nil
}

// watchLoop reads debounced file events and enqueues index requests.
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
			select {
			case d.indexQueue <- indexRequest{repoURL: repoURL, repoPath: ev.RepoPath}:
			default:
				// Queue full; skip this event.
			}
		}
	}
}

// indexWorker processes index requests sequentially, holding a write lock
// during indexing to block concurrent reads.
func (d *Daemon) indexWorker(ctx context.Context) {
	for req := range d.indexQueue {
		if ctx.Err() != nil {
			return
		}
		if d.cfg.IndexFunc == nil {
			continue
		}
		// Acquire write lock during indexing so readers wait.
		d.mu.Lock()
		_ = d.cfg.IndexFunc(ctx, req.repoURL, req.repoPath, "HEAD")
		d.mu.Unlock()
	}
}
