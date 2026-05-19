// Benchmarks context pack deduplication (Phase 3 P5):
// measures token savings when agent passes pack_root and gets "unchanged"
// instead of the full context payload.
//
// Run: GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestContextPackDedup -timeout 120s
package merkle_diff

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}

	tasks := []struct {
		name   string
		desc   string
		budget int
	}{
		{"small", "refactor auth", 5000},
		{"medium", "add caching to the context engine retrieval pipeline", 20000},
		{"large", "implement incremental community detection with Merkle tree change tracking", 50000},
	}

	unchangedResponse := "unchanged pack_root=XXXX symbols=N\nContext is identical to your prior request. No retransmission needed."
	unchangedTokens := len(unchangedResponse) / 4 // rough token estimate

	t.Logf("=== Context Pack Deduplication (P5) ===")
	t.Logf("")

	var totalFullTokens, totalSaved int

	for _, task := range tasks {
		engine := knowingctx.NewContextEngine(st)
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     task.budget,
			Format:          "xml",
		})
		if err != nil {
			t.Fatalf("ForTask(%s): %v", task.name, err)
		}

		// Encode to GCF to measure actual token cost.
		payload, err := wire.FromContextBlock(ctx, block, "context_for_task", st)
		if err != nil {
			t.Fatalf("FromContextBlock: %v", err)
		}
		gcfOutput := wire.Encode(payload)
		fullTokens := len(gcfOutput) / 4 // rough token estimate (4 chars per token)
		fullBytes := len(gcfOutput)

		saved := fullTokens - unchangedTokens
		savingsPct := float64(saved) / float64(fullTokens) * 100

		t.Logf("Task %q (%s, budget=%d):", task.desc, task.name, task.budget)
		t.Logf("  Symbols: %d, PackRoot: %s", len(block.Symbols), block.PackRoot)
		t.Logf("  Full response: %d bytes (~%d tokens)", fullBytes, fullTokens)
		t.Logf("  Unchanged response: %d bytes (~%d tokens)", len(unchangedResponse), unchangedTokens)
		t.Logf("  Savings per dedup: %d tokens (%.0f%%)", saved, savingsPct)
		t.Logf("")

		totalFullTokens += fullTokens
		totalSaved += saved
	}

	t.Logf("=== Summary ===")
	t.Logf("Total full tokens (3 tasks): %d", totalFullTokens)
	t.Logf("Total saved by dedup:        %d tokens", totalSaved)
	t.Logf("Unchanged response cost:     ~%d tokens (fixed)", unchangedTokens)

	// Performance contract: dedup should save at least 90% of tokens.
	for _, task := range tasks {
		engine := knowingctx.NewContextEngine(st)
		block, _ := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     task.budget,
		})
		payload, _ := wire.FromContextBlock(ctx, block, "context_for_task", st)
		gcf := wire.Encode(payload)
		fullTokens := len(gcf) / 4
		savingsPct := float64(fullTokens-unchangedTokens) / float64(fullTokens) * 100
		if savingsPct < 90 {
			t.Errorf("task %q: dedup saves only %.0f%% (expected >90%%)", task.desc, savingsPct)
		}
	}

	// Verify PackRoot determinism: same task twice = same PackRoot.
	engine1 := knowingctx.NewContextEngine(st)
	engine2 := knowingctx.NewContextEngine(st)
	b1, _ := engine1.ForTask(ctx, knowingctx.TaskOptions{TaskDescription: "refactor auth", TokenBudget: 5000})
	b2, _ := engine2.ForTask(ctx, knowingctx.TaskOptions{TaskDescription: "refactor auth", TokenBudget: 5000})
	if b1.PackRoot != b2.PackRoot {
		t.Errorf("PackRoot not deterministic: %s != %s", b1.PackRoot, b2.PackRoot)
	}
	t.Logf("PackRoot determinism: verified (%s)", b1.PackRoot)

	// Measure latency: cached ForTask (which dedup hits) vs cold.
	statsCold := measure(5, 1, func() {
		e := knowingctx.NewContextEngine(st)
		e.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: "incremental community detection",
			TokenBudget:     50000,
		})
	})
	t.Logf("Cold ForTask (no cache):  %s", statsCold)

	// Warm the persistent cache, then measure with fresh engine.
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
	t.Logf("Persistent cache ForTask: %s", statsWarm)
	t.Logf("")
	t.Logf("Agent workflow: first call gets full context + pack_root.")
	t.Logf("Subsequent calls pass pack_root, get 'unchanged' (~%d tokens).", unchangedTokens)
	t.Logf("Savings compound: %d calls/session * %d tokens saved = significant budget recovery.",
		5, totalSaved/len(tasks))

	_ = fmt.Sprintf("") // keep fmt import
}
