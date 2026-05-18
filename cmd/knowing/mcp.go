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
	"syscall"
	"time"

	"github.com/blackwell-systems/knowing/internal/daemon"
	"github.com/blackwell-systems/knowing/internal/enrichment"
	"github.com/blackwell-systems/knowing/internal/indexer"
	knowingmcp "github.com/blackwell-systems/knowing/internal/mcp"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdMCP runs the MCP server over stdio. This is the mode used by AI agents
// via .mcp.json configuration. Opens the database and serves MCP tool calls
// until stdin closes or SIGINT/SIGTERM is received.
//
// With --watch, also starts an fsnotify file watcher that re-indexes changed
// files on save. This combines MCP serving and live graph updates in one process.
func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	watch := fs.Bool("watch", false, "Watch repo for file changes and re-index on save")
	repoPath := fs.String("repo", "", "Repository path to watch (required with --watch, defaults to cwd)")
	repoURL := fs.String("url", "", "Repository URL (auto-detected if empty)")
	noEnrich := fs.Bool("no-enrich", false, "Skip LSP enrichment after reindex (only with --watch)")
	debounceMs := fs.Int("debounce", 500, "Debounce interval in ms (only with --watch)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found: %s (run 'knowing index' first)", *dbPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	mcpServer := knowingmcp.NewServer(st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Start file watcher if --watch is set.
	if *watch {
		if *repoPath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("resolving cwd: %w", err)
			}
			*repoPath = cwd
		}
		absRepo, err := filepath.Abs(*repoPath)
		if err != nil {
			return fmt.Errorf("resolving repo path: %w", err)
		}
		if *repoURL == "" {
			*repoURL = detectRepoURL(absRepo)
		}

		snapMgr := snapshot.NewSnapshotManager(st)
		idx := indexer.NewIndexer(st, snapMgr)
		registerAllExtractors(idx, false)
		repoHash := types.NewHash([]byte(*repoURL))

		fw, err := newFileWatcher(absRepo, time.Duration(*debounceMs)*time.Millisecond)
		if err != nil {
			return fmt.Errorf("creating watcher: %w", err)
		}
		defer fw.Close()

		// Watch loop in background goroutine.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case files := <-fw.Events():
					if len(files) == 0 {
						continue
					}
					rel := make([]string, 0, len(files))
					for _, f := range files {
						if r, err := filepath.Rel(absRepo, f); err == nil {
							rel = append(rel, r)
						}
					}
					if len(rel) == 0 {
						continue
					}

					log.Printf("Changed: %s", strings.Join(rel, ", "))
					start := time.Now()
					commit := ""
					if resolved, err := daemon.GitHeadCommit(absRepo); err == nil {
						commit = resolved
					}
					snap, err := idx.IndexRepo(ctx, *repoURL, absRepo, commit)
					if err != nil {
						log.Printf("Reindex error: %v", err)
						continue
					}
					log.Printf("Reindexed in %v (nodes: %d, edges: %d)",
						time.Since(start).Round(time.Millisecond),
						snap.NodeCount, snap.EdgeCount)

					if !*noEnrich {
						go func(changed []string) {
							e := enrichment.NewEnricher(st, absRepo)
							if err := e.RunScoped(ctx, repoHash, changed); err != nil {
								log.Printf("Enrichment warning: %v", err)
							}
							e.Close(ctx)
						}(rel)
					}
				}
			}
		}()

		log.Printf("Watching %s for changes (debounce %dms)", absRepo, *debounceMs)
	}

	return mcpServer.ServeStdio(ctx)
}
