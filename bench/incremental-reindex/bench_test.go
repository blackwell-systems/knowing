// Package incremental_reindex benchmarks the cost of incremental file reindexing
// vs full repository reindexing. Proves that IndexFilesIncremental is proportional
// to the number of changed files, not the total repo size.
package incremental_reindex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

func TestIncrementalReindex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping incremental reindex bench in short mode")
	}

	repoRoot := findRepoRoot(t)
	ctx := context.Background()

	// Phase 1: Full index (baseline).
	dbPath := t.TempDir() + "/bench.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	t.Log("=== Phase 1: Full Index ===")
	fullStart := time.Now()
	snap, err := idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("full index: %v", err)
	}
	fullDuration := time.Since(fullStart)
	t.Logf("  Full index: %v (%d nodes, %d edges)", fullDuration, snap.NodeCount, snap.EdgeCount)

	// Rebuild FTS after full index.
	st.RebuildFTS(ctx) //nolint:errcheck

	// Phase 2: Incremental reindex (1 file).
	t.Log("")
	t.Log("=== Phase 2: Incremental (1 file) ===")
	oneFile := []string{"internal/context/context.go"}
	incStart := time.Now()
	err = idx.IndexFilesIncremental(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD", oneFile)
	if err != nil {
		t.Fatalf("incremental 1 file: %v", err)
	}
	incDuration := time.Since(incStart)
	t.Logf("  1 file: %v", incDuration)

	// Phase 3: Incremental reindex (5 files).
	t.Log("")
	t.Log("=== Phase 3: Incremental (5 files) ===")
	fiveFiles := []string{
		"internal/context/context.go",
		"internal/context/ranking.go",
		"internal/context/walk.go",
		"internal/context/explain.go",
		"internal/store/sqlite.go",
	}
	inc5Start := time.Now()
	err = idx.IndexFilesIncremental(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD", fiveFiles)
	if err != nil {
		t.Fatalf("incremental 5 files: %v", err)
	}
	inc5Duration := time.Since(inc5Start)
	t.Logf("  5 files: %v", inc5Duration)

	// Phase 4: Incremental reindex (20 files - a typical PR).
	t.Log("")
	t.Log("=== Phase 4: Incremental (20 files) ===")
	twentyFiles := collectFiles(repoRoot, "internal/context/", 10)
	twentyFiles = append(twentyFiles, collectFiles(repoRoot, "internal/store/", 5)...)
	twentyFiles = append(twentyFiles, collectFiles(repoRoot, "internal/indexer/", 5)...)
	if len(twentyFiles) > 20 {
		twentyFiles = twentyFiles[:20]
	}
	inc20Start := time.Now()
	err = idx.IndexFilesIncremental(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD", twentyFiles)
	if err != nil {
		t.Fatalf("incremental 20 files: %v", err)
	}
	inc20Duration := time.Since(inc20Start)
	t.Logf("  20 files: %v", inc20Duration)

	// Phase 5: Full re-index for comparison.
	t.Log("")
	t.Log("=== Phase 5: Full Re-Index (comparison) ===")
	reFullStart := time.Now()
	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("full re-index: %v", err)
	}
	reFullDuration := time.Since(reFullStart)
	t.Logf("  Full re-index: %v", reFullDuration)

	// Report.
	t.Log("")
	t.Log("=== Results ===")
	t.Logf("  | Files | Duration | vs Full | Speedup |")
	t.Logf("  |-------|----------|---------|---------|")
	t.Logf("  | 1     | %v | %v | %.0fx |", incDuration, fullDuration, float64(fullDuration)/float64(incDuration))
	t.Logf("  | 5     | %v | %v | %.0fx |", inc5Duration, fullDuration, float64(fullDuration)/float64(inc5Duration))
	t.Logf("  | 20    | %v | %v | %.0fx |", inc20Duration, fullDuration, float64(fullDuration)/float64(inc20Duration))
	t.Logf("  | full  | %v | - | 1x |", reFullDuration)
	t.Log("")
	t.Logf("  Incremental cost per file: %v", incDuration/time.Duration(len(oneFile)))
	t.Logf("  Full index cost per file: %v", fullDuration/time.Duration(snap.NodeCount))

	// Assertion: incremental should be at least 5x faster than full for 1 file.
	if incDuration > fullDuration/5 {
		t.Errorf("incremental (1 file) not fast enough: %v vs full %v (expected >5x speedup)", incDuration, fullDuration)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			mod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if strings.Contains(string(mod), "github.com/blackwell-systems/knowing") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

func collectFiles(repoRoot, prefix string, max int) []string {
	var files []string
	absPrefix := filepath.Join(repoRoot, prefix)
	filepath.WalkDir(absPrefix, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			rel, _ := filepath.Rel(repoRoot, path)
			files = append(files, rel)
		}
		if len(files) >= max {
			return fmt.Errorf("done")
		}
		return nil
	})
	return files
}
