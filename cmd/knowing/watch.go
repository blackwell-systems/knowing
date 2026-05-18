package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdWatch starts a lightweight file watcher that re-indexes changed files
// on save. Unlike the full daemon (knowing serve), this does not start an
// MCP server or ingest traces. It just keeps the graph up to date.
func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected from go.mod if empty)")
	noEnrich := fs.Bool("no-enrich", false, "Skip LSP enrichment after reindex")
	debounceMs := fs.Int("debounce", 500, "Debounce interval in milliseconds")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		return fmt.Errorf("usage: knowing watch [flags] <repo-path>")
	}
	repoPath, err := filepath.Abs(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Resolve repo URL.
	if *repoURL == "" {
		*repoURL = detectRepoURL(repoPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	registerAllExtractors(idx, false)

	repoHash := types.NewHash([]byte(*repoURL))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nShutting down...")
		cancel()
	}()

	w, err := newFileWatcher(repoPath, time.Duration(*debounceMs)*time.Millisecond)
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer w.Close()

	fmt.Printf("Watching %s for changes (debounce %dms)\n", repoPath, *debounceMs)
	fmt.Println("Press Ctrl+C to stop.")

	for {
		select {
		case <-ctx.Done():
			return nil
		case files := <-w.Events():
			if len(files) == 0 {
				continue
			}
			// Make paths relative to repo root.
			rel := make([]string, 0, len(files))
			for _, f := range files {
				r, err := filepath.Rel(repoPath, f)
				if err == nil {
					rel = append(rel, r)
				}
			}
			if len(rel) == 0 {
				continue
			}

			log.Printf("Changed: %s", strings.Join(rel, ", "))

			start := time.Now()
			commit := ""
			if resolved, err := daemon.GitHeadCommit(repoPath); err == nil {
				commit = resolved
			}
			snap, err := idx.IndexRepo(ctx, *repoURL, repoPath, commit)
			if err != nil {
				log.Printf("Reindex error: %v", err)
				continue
			}
			log.Printf("Reindexed %d files in %v (nodes: %d, edges: %d)",
				len(rel), time.Since(start).Round(time.Millisecond),
				snap.NodeCount, snap.EdgeCount)

			if !*noEnrich {
				go func(changed []string) {
					enricher := enrichment.NewEnricher(st, repoPath)
					if err := enricher.RunScoped(ctx, repoHash, changed); err != nil {
						log.Printf("Enrichment warning: %v", err)
					}
					enricher.Close(ctx)
				}(rel)
			}
		}
	}
}

// sourceExtensions lists file extensions that should trigger reindexing.
var sourceExtensions = map[string]bool{
	".go": true, ".py": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".rs": true, ".java": true, ".cs": true, ".rb": true, ".proto": true,
	".tf": true, ".sql": true, ".css": true, ".scss": true,
	".yaml": true, ".yml": true, ".json": true, ".toml": true,
	".graphql": true, ".gql": true, ".env": true,
	"Makefile": true, "Dockerfile": true, "Jenkinsfile": true,
}

// ignoreDirs lists directories to skip when setting up watchers.
var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true,
	"__pycache__": true, "target": true, "build": true, "dist": true,
	".next": true, ".nuxt": true, ".gradle": true,
}

// fileWatcher watches source files in a repo and emits debounced batches
// of changed file paths.
type fileWatcher struct {
	watcher  *fsnotify.Watcher
	root     string
	debounce time.Duration
	events   chan []string
	done     chan struct{}
}

func newFileWatcher(root string, debounce time.Duration) (*fileWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &fileWatcher{
		watcher:  w,
		root:     root,
		debounce: debounce,
		events:   make(chan []string, 16),
		done:     make(chan struct{}),
	}

	// Walk the repo and add directories (fsnotify watches directories, not files).
	dirs := 0
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if ignoreDirs[info.Name()] {
				return filepath.SkipDir
			}
			if err := w.Add(path); err == nil {
				dirs++
			}
			return nil
		}
		return nil
	})
	if err != nil {
		w.Close()
		return nil, err
	}

	log.Printf("Watching %d directories", dirs)
	go fw.loop()
	return fw, nil
}

func (fw *fileWatcher) Events() <-chan []string {
	return fw.events
}

func (fw *fileWatcher) Close() error {
	close(fw.done)
	return fw.watcher.Close()
}

func (fw *fileWatcher) loop() {
	var mu sync.Mutex
	pending := make(map[string]bool)
	var timer *time.Timer

	for {
		select {
		case <-fw.done:
			return
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			// Only care about writes and creates.
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			// Filter by extension.
			if !isSourceFile(event.Name) {
				continue
			}

			mu.Lock()
			pending[event.Name] = true

			// Reset debounce timer.
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(fw.debounce, func() {
				mu.Lock()
				files := make([]string, 0, len(pending))
				for f := range pending {
					files = append(files, f)
				}
				pending = make(map[string]bool)
				mu.Unlock()

				if len(files) > 0 {
					fw.events <- files
				}
			})
			mu.Unlock()

		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

// isSourceFile returns true if the path has a known source file extension.
func isSourceFile(path string) bool {
	base := filepath.Base(path)
	// Check exact filename matches (Makefile, Dockerfile).
	if sourceExtensions[base] {
		return true
	}
	ext := filepath.Ext(path)
	return sourceExtensions[ext]
}

