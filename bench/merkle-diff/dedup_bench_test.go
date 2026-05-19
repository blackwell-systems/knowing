// Benchmarks context pack deduplication (Phase 3 P5):
// measures byte savings when agent passes pack_root and gets "unchanged"
// instead of the full context payload.
//
// Uses the real wire encoder to measure actual GCF output size (what agents
// receive), and compares against the known "unchanged" response.
//
// Run: GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestContextPackDedup -timeout 120s
package merkle_diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/wire"
)

func TestContextPackDedup(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("not in knowing repo root")
	}

	tmpDB := filepath.Join(t.TempDir(), "bench.db")
	st, err := store.NewSQLiteStore(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Indexed: %d nodes, %d edges", snap.NodeCount, snap.EdgeCount)

	tasks := []struct {
		name   string
		desc   string
		budget int
	}{
		{"small", "refactor auth", 5000},
		{"medium", "add caching to the context engine retrieval pipeline", 20000},
		{"large", "implement incremental community detection with Merkle tree change tracking", 50000},
	}

	t.Logf("=== Context Pack Deduplication (P5) ===")
	t.Logf("")

	type dedupResult struct {
		name           string
		symbols        int
		fullBytes      int
		unchangedBytes int
		savingsPct     float64
		packRoot       string
	}
	var results []dedupResult

	for _, task := range tasks {
		engine := knowingctx.NewContextEngine(st)
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     task.budget,
		})
		if err != nil {
			t.Fatalf("ForTask(%s): %v", task.name, err)
		}

		// Encode with the real wire encoder (what agents receive).
		payload, err := wire.FromContextBlock(ctx, block, "context_for_task", st)
		if err != nil {
			t.Fatalf("FromContextBlock(%s): %v", task.name, err)
		}
		gcfOutput := wire.Encode(payload)
		fullBytes := len(gcfOutput)

		// The "unchanged" response is what the MCP handler returns on dedup hit.
		unchangedResponse := fmt.Sprintf(
			"unchanged pack_root=%s symbols=%d\nContext is identical to your prior request. No retransmission needed.",
			block.PackRoot, len(block.Symbols))
		unchangedBytes := len(unchangedResponse)

		savingsPct := float64(fullBytes-unchangedBytes) / float64(fullBytes) * 100

		results = append(results, dedupResult{
			name:           task.name,
			symbols:        len(block.Symbols),
			fullBytes:      fullBytes,
			unchangedBytes: unchangedBytes,
			savingsPct:     savingsPct,
			packRoot:       block.PackRoot.String(),
		})

		t.Logf("Task %q (%s, budget=%d):", task.desc, task.name, task.budget)
		t.Logf("  Symbols: %d, PackRoot: %s", len(block.Symbols), block.PackRoot)
		t.Logf("  Full GCF response:  %d bytes", fullBytes)
		t.Logf("  Unchanged response: %d bytes", unchangedBytes)
		t.Logf("  Savings: %.1f%%", savingsPct)
		t.Logf("")
	}

	// --- Performance contracts ---
	for _, r := range results {
		if r.savingsPct < 90 {
			t.Errorf("task %q: dedup saves only %.1f%% (contract: >90%%)", r.name, r.savingsPct)
		}
	}

	// --- PackRoot determinism: same task, two independent engines ---
	engine1 := knowingctx.NewContextEngine(st)
	engine2 := knowingctx.NewContextEngine(st)
	b1, _ := engine1.ForTask(ctx, knowingctx.TaskOptions{TaskDescription: "refactor auth", TokenBudget: 5000})
	b2, _ := engine2.ForTask(ctx, knowingctx.TaskOptions{TaskDescription: "refactor auth", TokenBudget: 5000})
	if b1.PackRoot != b2.PackRoot {
		t.Errorf("PackRoot not deterministic: %s != %s", b1.PackRoot, b2.PackRoot)
	}
	t.Logf("PackRoot determinism: verified (%s)", b1.PackRoot)

	// --- Wire format: PackRoot in GCF header ---
	payload, _ := wire.FromContextBlock(ctx, b1, "context_for_task", st)
	gcf := wire.Encode(payload)
	if !strings.Contains(gcf, "pack_root="+b1.PackRoot.String()) {
		t.Error("PackRoot missing from GCF header")
	}
	t.Logf("GCF header pack_root: verified")

	// --- Wire format: PackRoot in JSON ---
	jsonOutput, err := wire.EncodeWith("json", payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jsonOutput, `"pack_root"`) {
		t.Errorf("PackRoot missing from JSON output (first 200 chars): %s", jsonOutput[:min(200, len(jsonOutput))])
	} else {
		t.Logf("JSON pack_root field: verified")
	}

	// --- Latency: cold vs persistent cache ---
	statsCold := measure(5, 1, func() {
		e := knowingctx.NewContextEngine(st)
		e.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: "incremental community detection",
			TokenBudget:     50000,
		})
	})
	t.Logf("Cold ForTask:         %s", statsCold)

	// Warm persistent cache.
	warmEngine := knowingctx.NewContextEngine(st)
	warmEngine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "incremental community detection",
		TokenBudget:     50000,
	})
	statsWarm := measure(10, 2, func() {
		e := knowingctx.NewContextEngine(st)
		e.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: "incremental community detection",
			TokenBudget:     50000,
		})
	})
	t.Logf("Persistent cache hit: %s", statsWarm)

	// --- Summary ---
	t.Logf("")
	t.Logf("=== Summary ===")
	for _, r := range results {
		t.Logf("  %s: %d -> %d bytes (%.0f%% saved)", r.name, r.fullBytes, r.unchangedBytes, r.savingsPct)
	}
	t.Logf("Agent gets full context + pack_root on first call.")
	t.Logf("Subsequent calls with same pack_root: ~%d bytes instead of full payload.", results[0].unchangedBytes)
}
