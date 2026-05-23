package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// cmdStale reports stale nodes and edges from changed files since the last
// indexed snapshot. It compares the current HEAD to the last snapshot's commit
// hash and identifies graph elements that may be outdated.
//
// Usage:
//
//	knowing stale -db knowing.db -repo .
//	knowing stale -db knowing.db -repo /path/to/repo
func cmdStale(args []string) error {
	fs := flag.NewFlagSet("stale", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to SQLite database (env: KNOWING_DB)")
	repoPath := fs.String("repo", ".", "Repository path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found: %s (run 'knowing index' first)", *dbPath)
	}

	// Resolve repo path to absolute.
	absRepo, err := filepath.Abs(*repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}

	// Determine repo URL: try go.mod module path, then fall back to absolute path.
	repoURL := ""
	if modData, err := os.ReadFile(filepath.Join(absRepo, "go.mod")); err == nil {
		for _, line := range strings.Split(string(modData), "\n") {
			if strings.HasPrefix(line, "module ") {
				repoURL = strings.TrimSpace(strings.TrimPrefix(line, "module "))
				break
			}
		}
	}
	if repoURL == "" {
		repoURL = absRepo
	}

	repoHash := types.NewHash([]byte(repoURL))

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Get the latest snapshot for this repo.
	snap, err := st.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return fmt.Errorf("querying latest snapshot: %w", err)
	}

	var changedPaths []string
	if snap != nil && snap.CommitHash != "" {
		// Run git diff to find changed files since the snapshot commit.
		cmd := exec.Command("git", "-C", absRepo, "diff", "--name-only", snap.CommitHash, "HEAD")
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("running git diff: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				changedPaths = append(changedPaths, line)
			}
		}
	} else {
		// No snapshot exists: cannot determine staleness.
		fmt.Fprintln(os.Stderr, "No snapshot found; unable to determine changed files.")
		fmt.Fprintln(os.Stderr, "Run 'knowing index' first.")
		return nil
	}

	if len(changedPaths) == 0 {
		fmt.Println("No changed files detected. Graph is up to date.")
		return nil
	}

	// Find stale nodes in the changed files.
	staleNodes, err := st.StaleNodesByFiles(ctx, repoHash, changedPaths)
	if err != nil {
		return fmt.Errorf("querying stale nodes: %w", err)
	}

	// Build a file_hash -> path mapping for counting nodes per file.
	type fileStaleness struct {
		nodeCount int
		edgeCount int
	}
	fileStats := make(map[string]*fileStaleness)
	for _, p := range changedPaths {
		fileStats[p] = &fileStaleness{}
	}

	// Map file hashes to paths for node attribution.
	fileHashToPath := make(map[types.Hash]string)
	for _, p := range changedPaths {
		f, err := st.FileByPath(ctx, repoHash, p)
		if err != nil || f == nil {
			continue
		}
		fileHashToPath[f.FileHash] = p
	}

	// Count nodes per file.
	for _, n := range staleNodes {
		if p, ok := fileHashToPath[n.FileHash]; ok {
			fileStats[p].nodeCount++
		}
	}

	// Count stale edges per changed file.
	for fileHash, p := range fileHashToPath {
		edges, err := st.EdgesBySourceFile(ctx, fileHash)
		if err != nil {
			continue
		}
		fileStats[p].edgeCount = len(edges)
	}

	// Output summary table.
	fmt.Printf("%-60s %6s %6s\n", "FILE", "NODES", "EDGES")
	fmt.Printf("%-60s %6s %6s\n", strings.Repeat("-", 60), "------", "------")
	hasStale := false
	for _, p := range changedPaths {
		stats := fileStats[p]
		if stats.nodeCount > 0 || stats.edgeCount > 0 {
			hasStale = true
			fmt.Printf("%-60s %6d %6d\n", p, stats.nodeCount, stats.edgeCount)
		}
	}

	if !hasStale {
		// Changed files exist but have no indexed symbols.
		fmt.Println("Changed files have no indexed symbols.")
		return nil
	}

	// Exit code 1 if stale files found (useful for CI).
	os.Exit(1)
	return nil
}
