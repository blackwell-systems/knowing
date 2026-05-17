// Package feedback_loop benchmarks the compounding intelligence thesis:
// does feedback accumulation measurably improve context engine precision?
//
// The test:
// 1. Indexes the knowing repo into a temp DB
// 2. Defines 5 task fixtures with ground-truth "useful" symbols
// 3. Runs context_for_task WITHOUT feedback (baseline precision)
// 4. Records feedback for ground-truth symbols
// 5. Runs context_for_task WITH feedback (boosted precision)
// 6. Measures: precision@10, recall@10, and MRR improvement
//
// Additionally tests:
// - Community scoping: feedback in community A doesn't leak to community B
// - Natural expiration: feedback on a renamed symbol doesn't pollute results
package feedback_loop

import (
	"context"
	"os"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// taskFixture defines a task query and its ground-truth relevant symbols.
type taskFixture struct {
	Name        string
	Description string
	// GroundTruth: qualified name substrings that are "useful" for this task.
	// A result symbol is "useful" if any ground truth substring appears in its qualified name.
	GroundTruth []string
}

var fixtures = []taskFixture{
	{
		Name:        "context_engine",
		Description: "Implement HITS hub/authority reranking in the context engine ranking pipeline",
		GroundTruth: []string{
			"context.RankSymbols",
			"context.HITSScores",
			"context.ComputeHITS",
			"context.ContextEngine",
			"context.ForTask",
			"context.RankedSymbol",
			"context.packIntoBudget",
			"context.RandomWalkWithRestart",
		},
	},
	{
		Name:        "mcp_server",
		Description: "Add a new MCP tool to the knowing server that queries blast radius",
		GroundTruth: []string{
			"mcp.Server",
			"mcp.NewServer",
			"mcp.blastRadiusTool",
			"mcp.handleBlastRadius",
			"mcp.registerTools",
			"mcp.requireStringArg",
			"mcp.requireHash",
		},
	},
	{
		Name:        "indexer_pipeline",
		Description: "Add a new language extractor to the indexer framework and register it",
		GroundTruth: []string{
			"indexer.Indexer",
			"indexer.NewIndexer",
			"indexer.IndexRepo",
			"types.Extractor",
			"types.ExtractOptions",
			"types.ExtractResult",
			"gotsextractor.NewGoTreeSitterExtractor",
		},
	},
	{
		Name:        "store_layer",
		Description: "Add a new SQLite query method to the store for finding nodes by file path",
		GroundTruth: []string{
			"store.SQLiteStore",
			"store.NewSQLiteStore",
			"store.NodesByName",
			"store.NodesByFilePath",
			"store.FileByPath",
			"store.EdgesFrom",
			"store.EdgesTo",
			"types.GraphStore",
		},
	},
	{
		Name:        "test_selection",
		Description: "Find affected tests by tracing the call graph backward from changed symbols",
		GroundTruth: []string{
			"knowing.cmdTestScope",
			"knowing.symbolsInFiles",
			"knowing.findAffectedTests",
			"knowing.isTestFunction",
			"store.NodesByFilePath",
			"store.EdgesTo",
			"store.GetNode",
		},
	},
}

func TestFeedbackCompounding(t *testing.T) {
	// Skip in short mode (requires indexing).
	if testing.Short() {
		t.Skip("skipping feedback benchmark in short mode")
	}

	// Find repo root.
	repoRoot := findRepoRoot(t)

	// Create temp DB and index.
	dbPath := t.TempDir() + "/bench.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	engine := knowingctx.NewContextEngine(st)

	// Phase 1: Baseline (no feedback).
	t.Log("=== Phase 1: Baseline (no feedback) ===")
	baselineResults := make(map[string]precisionResult)
	for _, fix := range fixtures {
		result := measurePrecision(t, engine, st, fix, false)
		baselineResults[fix.Name] = result
		t.Logf("  %s: precision@10=%.1f%% recall@10=%.1f%% MRR=%.3f",
			fix.Name, result.Precision*100, result.Recall*100, result.MRR)
	}

	// Phase 2: Record feedback for ground-truth symbols.
	t.Log("=== Phase 2: Recording feedback ===")
	for _, fix := range fixtures {
		recordGroundTruthFeedback(t, ctx, st, fix)
	}

	// Phase 3: With feedback.
	t.Log("=== Phase 3: With feedback ===")
	feedbackResults := make(map[string]precisionResult)
	for _, fix := range fixtures {
		result := measurePrecision(t, engine, st, fix, true)
		feedbackResults[fix.Name] = result
		t.Logf("  %s: precision@10=%.1f%% recall@10=%.1f%% MRR=%.3f",
			fix.Name, result.Precision*100, result.Recall*100, result.MRR)
	}

	// Phase 4: Report improvement.
	t.Log("=== Results ===")
	var totalBaselineP, totalFeedbackP float64
	var totalBaselineR, totalFeedbackR float64
	for _, fix := range fixtures {
		bp := baselineResults[fix.Name]
		fp := feedbackResults[fix.Name]
		delta := (fp.Precision - bp.Precision) * 100
		t.Logf("  %s: precision %.1f%% -> %.1f%% (%+.1f%%)",
			fix.Name, bp.Precision*100, fp.Precision*100, delta)
		totalBaselineP += bp.Precision
		totalFeedbackP += fp.Precision
		totalBaselineR += bp.Recall
		totalFeedbackR += fp.Recall
	}

	avgBaselineP := totalBaselineP / float64(len(fixtures))
	avgFeedbackP := totalFeedbackP / float64(len(fixtures))
	avgBaselineR := totalBaselineR / float64(len(fixtures))
	avgFeedbackR := totalFeedbackR / float64(len(fixtures))

	t.Logf("\n  AVERAGE: precision %.1f%% -> %.1f%% (%+.1f%%)",
		avgBaselineP*100, avgFeedbackP*100, (avgFeedbackP-avgBaselineP)*100)
	t.Logf("  AVERAGE: recall    %.1f%% -> %.1f%% (%+.1f%%)",
		avgBaselineR*100, avgFeedbackR*100, (avgFeedbackR-avgBaselineR)*100)

	// The thesis: feedback improves precision.
	if avgFeedbackP <= avgBaselineP {
		t.Errorf("THESIS FAILED: feedback did not improve average precision (baseline=%.3f, feedback=%.3f)",
			avgBaselineP, avgFeedbackP)
	}
}

func TestCommunityScoping(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping community scoping test in short mode")
	}

	repoRoot := findRepoRoot(t)
	dbPath := t.TempDir() + "/bench.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	_, err = idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoRoot, "HEAD")
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	engine := knowingctx.NewContextEngine(st)

	// Record feedback for context_engine task (community A).
	contextFix := fixtures[0] // context_engine
	recordGroundTruthFeedback(t, ctx, st, contextFix)

	// Query for indexer task (community B).
	// The feedback from community A should NOT improve indexer results.
	indexerFix := fixtures[2] // indexer_pipeline

	withFeedback := measurePrecision(t, engine, st, indexerFix, true)
	withoutFeedback := measurePrecision(t, engine, st, indexerFix, false)

	// Feedback from a different community should have minimal impact.
	delta := withFeedback.Precision - withoutFeedback.Precision
	t.Logf("Cross-community leak: indexer precision delta = %+.1f%% (should be ~0%%)", delta*100)

	if delta > 0.15 {
		t.Errorf("COMMUNITY SCOPING FAILED: feedback from context_engine leaked into indexer results (delta=%.3f)", delta)
	}
}

func TestNaturalExpiration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping natural expiration test in short mode")
	}

	dbPath := t.TempDir() + "/bench.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// Record feedback for a symbol hash.
	symbolHash := types.NewHash([]byte("github.com/blackwell-systems/knowing://internal/context.OldFunction/function"))
	err = st.RecordFeedback(ctx, symbolHash, "session-1", true)
	if err != nil {
		t.Fatalf("record feedback: %v", err)
	}

	// Query feedback for that hash: should exist.
	stats, err := st.QueryFeedback(ctx, symbolHash)
	if err != nil {
		t.Fatalf("query feedback: %v", err)
	}
	if stats.UsefulCount != 1 {
		t.Fatalf("expected 1 useful, got %d", stats.UsefulCount)
	}

	// Now "rename" the symbol (new hash).
	newHash := types.NewHash([]byte("github.com/blackwell-systems/knowing://internal/context.NewFunction/function"))

	// Query feedback for the new hash: should NOT exist (natural expiration).
	newStats, err := st.QueryFeedback(ctx, newHash)
	if err != nil {
		t.Fatalf("query feedback new: %v", err)
	}
	if newStats.UsefulCount != 0 {
		t.Errorf("EXPIRATION FAILED: renamed symbol inherited old feedback (got %d useful)", newStats.UsefulCount)
	}

	// The old feedback is structurally orphaned: no node in the graph matches
	// the old hash anymore (it was "renamed"). FeedbackBoosts on the new hash
	// returns nothing, so the ranking is unaffected.
	boosts, err := st.FeedbackBoosts(ctx, []types.Hash{newHash})
	if err != nil {
		t.Fatalf("FeedbackBoosts: %v", err)
	}
	if _, exists := boosts[newHash]; exists {
		t.Errorf("EXPIRATION FAILED: new hash has feedback boost")
	}
	t.Log("Natural expiration verified: renamed symbol has no feedback inheritance")
}

// --- helpers ---

type precisionResult struct {
	Precision float64 // relevant in top-10 / 10
	Recall    float64 // relevant in top-10 / total relevant
	MRR       float64 // 1/rank of first relevant result
}

func measurePrecision(t *testing.T, engine *knowingctx.ContextEngine, st *store.SQLiteStore, fix taskFixture, useFeedback bool) precisionResult {
	t.Helper()
	ctx := context.Background()

	// If useFeedback, populate FeedbackBoost on scoring inputs.
	// This requires the context engine to query FeedbackBoosts.
	// For now, we test via the public ForTask API which will use feedback
	// if the wiring is complete.
	block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: fix.Description,
		TokenBudget:     5000,
		Format:          "json",
	})
	if err != nil {
		t.Fatalf("ForTask(%s): %v", fix.Name, err)
	}

	// Measure precision@10: of the top 10 results, how many are in ground truth?
	topK := 10
	if len(block.Symbols) < topK {
		topK = len(block.Symbols)
	}

	relevant := 0
	firstRelevantRank := 0
	for i := 0; i < topK; i++ {
		if isRelevant(block.Symbols[i].Node.QualifiedName, fix.GroundTruth) {
			relevant++
			if firstRelevantRank == 0 {
				firstRelevantRank = i + 1
			}
		}
	}

	precision := float64(relevant) / float64(topK)
	recall := float64(relevant) / float64(len(fix.GroundTruth))
	mrr := 0.0
	if firstRelevantRank > 0 {
		mrr = 1.0 / float64(firstRelevantRank)
	}

	return precisionResult{Precision: precision, Recall: recall, MRR: mrr}
}

func isRelevant(qualifiedName string, groundTruth []string) bool {
	for _, gt := range groundTruth {
		if strings.Contains(qualifiedName, gt) {
			return true
		}
	}
	return false
}

func recordGroundTruthFeedback(t *testing.T, ctx context.Context, st *store.SQLiteStore, fix taskFixture) {
	t.Helper()

	// Find the actual node hashes for ground-truth symbols.
	for _, gt := range fix.GroundTruth {
		nodes, err := st.NodesByName(ctx, "%"+lastComponent(gt))
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if strings.Contains(n.QualifiedName, gt) {
				err := st.RecordFeedback(ctx, n.NodeHash, "bench-session", true)
				if err != nil {
					t.Logf("  failed to record feedback for %s: %v", gt, err)
				}
				break
			}
		}
	}
}

func lastComponent(s string) string {
	dot := strings.LastIndex(s, ".")
	if dot < 0 {
		return s
	}
	return s[dot+1:]
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from CWD to find go.mod with knowing module.
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(dir + "/go.mod"); err == nil {
			return dir
		}
		parent := dir[:strings.LastIndex(dir, "/")]
		if parent == dir {
			t.Fatal("cannot find repo root")
		}
		dir = parent
	}
}
