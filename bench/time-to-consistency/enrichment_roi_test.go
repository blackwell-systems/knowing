package time_to_consistency

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

// TestEnrichmentROI compares retrieval quality on Flask with and without LSP
// enrichment. The enriched DB (from the corpus) has 4091 external nodes (59%
// of all nodes) and 4267 LSP-resolved edges. The question: does enrichment
// help or hurt after the phantom external node filter?
func TestEnrichmentROI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping enrichment ROI in short mode")
	}

	flaskPath := filepath.Join("..", "cross-system", "corpus", "repos", "flask")
	if _, err := os.Stat(flaskPath); err != nil {
		t.Skipf("flask repo not found: %v", err)
	}

	enrichedDB := filepath.Join("..", "cross-system", "corpus", "repos", "flask", ".knowing", "graph.db")
	if _, err := os.Stat(enrichedDB); err != nil {
		t.Skipf("enriched DB not found: %v", err)
	}

	tasks := []struct {
		id   string
		desc string
	}{
		{"flask-easy-001", "add a before_request hook that validates API keys"},
		{"flask-easy-002", "register a new Flask blueprint for user profiles"},
		{"flask-easy-003", "add a custom error handler for 404 responses"},
		{"flask-medium-001", "implement session-based authentication with login/logout"},
		{"flask-medium-002", "add rate limiting middleware to the Flask application"},
		{"flask-medium-003", "implement a file upload endpoint with validation"},
		{"flask-hard-001", "add WebSocket support to the Flask application"},
		{"flask-hard-002", "implement a plugin system for Flask extensions"},
	}

	ctx := context.Background()

	// Phase 1: Index Flask WITHOUT enrichment (tree-sitter only).
	t.Log("=== Phase 1: Index Flask without enrichment ===")
	noEnrichDB := filepath.Join(t.TempDir(), "flask-no-enrich.db")
	stNoEnrich, err := store.NewSQLiteStore(noEnrichDB)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer stNoEnrich.Close()

	snapMgr := snapshot.NewSnapshotManager(stNoEnrich)
	idx := indexer.NewIndexer(stNoEnrich, snapMgr)
	pyExt, err := treesitter.NewTreeSitterExtractor("python")
	if err != nil {
		t.Fatalf("python extractor: %v", err)
	}
	idx.Register(pyExt)

	snap, err := idx.IndexRepo(ctx, "github.com/pallets/flask", flaskPath, "HEAD")
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	stNoEnrich.RebuildFTS(ctx) //nolint:errcheck
	t.Logf("  No-enrich: %d nodes, %d edges", snap.NodeCount, snap.EdgeCount)

	// Phase 2: Open the enriched DB.
	t.Log("")
	t.Log("=== Phase 2: Open enriched Flask DB ===")
	stEnriched, err := store.NewSQLiteStore(enrichedDB)
	if err != nil {
		t.Fatalf("enriched store: %v", err)
	}
	defer stEnriched.Close()
	t.Logf("  Enriched: opened (has LSP-resolved edges + external nodes)")

	// Phase 3: Run same tasks on both, compare results.
	t.Log("")
	t.Log("=== Phase 3: Query comparison ===")

	type taskResult struct {
		id         string
		noEnrich   int // symbols from flask in top-10
		enriched   int
		noEnrichMs int64
		enrichedMs int64
	}
	var results []taskResult

	for _, task := range tasks {
		// No-enrich query.
		engineNoEnrich := knowingctx.NewContextEngine(stNoEnrich)
		startNE := time.Now()
		resNE, _ := engineNoEnrich.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     5000,
			Format:          "json",
		})
		neMs := time.Since(startNE).Milliseconds()

		// Enriched query.
		engineEnriched := knowingctx.NewContextEngine(stEnriched)
		startE := time.Now()
		resE, _ := engineEnriched.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     5000,
			Format:          "json",
		})
		eMs := time.Since(startE).Milliseconds()

		// Count non-external symbols in top-10.
		countReal := func(symbols []knowingctx.RankedSymbol) int {
			count := 0
			top := 10
			if len(symbols) < top {
				top = len(symbols)
			}
			for i := 0; i < top; i++ {
				qn := symbols[i].Node.QualifiedName
				if !strings.Contains(qn, "external://") && symbols[i].Node.Kind != "external" {
					count++
				}
			}
			return count
		}

		r := taskResult{
			id:         task.id,
			noEnrich:   countReal(resNE.Symbols),
			enriched:   countReal(resE.Symbols),
			noEnrichMs: neMs,
			enrichedMs: eMs,
		}
		results = append(results, r)
	}

	// Phase 4: Report.
	t.Log("")
	t.Log("=== Results ===")
	t.Logf("  | Task               | No-Enrich (real/10) | Enriched (real/10) | Delta | Latency NE | Latency E |")
	t.Logf("  |--------------------|--------------------|--------------------|-------|------------|-----------|")

	totalNE := 0
	totalE := 0
	for _, r := range results {
		delta := r.enriched - r.noEnrich
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		t.Logf("  | %-18s | %d                  | %d                  | %s%d    | %dms        | %dms       |",
			r.id, r.noEnrich, r.enriched, sign, delta, r.noEnrichMs, r.enrichedMs)
		totalNE += r.noEnrich
		totalE += r.enriched
	}

	t.Log("")
	t.Logf("  Total real symbols in top-10: no-enrich=%d, enriched=%d", totalNE, totalE)
	if totalE > totalNE {
		t.Logf("  Enrichment HELPS: +%d real symbols across %d tasks (%.1f%% improvement)",
			totalE-totalNE, len(tasks), float64(totalE-totalNE)/float64(totalNE)*100)
	} else if totalE < totalNE {
		t.Logf("  Enrichment HURTS: %d fewer real symbols across %d tasks (%.1f%% degradation)",
			totalNE-totalE, len(tasks), float64(totalNE-totalE)/float64(totalNE)*100)
	} else {
		t.Log("  Enrichment is NEUTRAL: same number of real symbols")
	}

	fmt.Println()
}
