package merkle_diff

import (
	"context"
	"os"
	"testing"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestGrafanaScaleBenchmark runs mechanical benchmarks against the Grafana
// graph (714K edges, 338K nodes) to validate performance at production scale.
// Skip if the Grafana DB doesn't exist (CI won't have it).
func TestGrafanaScaleBenchmark(t *testing.T) {
	dbPath := os.Getenv("HOME") + "/.knowing/knowing.db"
	if _, err := os.Stat(dbPath); err != nil {
		t.Skip("Grafana DB not found at ~/.knowing/knowing.db")
	}

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	repoHash := types.NewHash([]byte("github.com/grafana/grafana"))

	t.Log("=== Grafana Scale Benchmarks ===")
	t.Log("")

	// Benchmark 1: Collect edges and build hierarchical tree.
	sm := snapshot.NewSnapshotManager(st)
	start := time.Now()
	edges, _, err := sm.CollectEdgeInputs(ctx, repoHash)
	collectDur := time.Since(start)
	if err != nil {
		t.Fatalf("CollectEdgeInputs: %v", err)
	}
	t.Logf("CollectEdgeInputs: %v (%d edges)", collectDur, len(edges))

	start = time.Now()
	tree := snapshot.BuildHierarchicalTree(edges)
	buildDur := time.Since(start)
	t.Logf("BuildHierarchicalTree: %v (%d packages, %d edge-type roots)",
		buildDur, len(tree.PackageRoots), len(tree.EdgeTypeRoots))

	// Benchmark 2: SubgraphRoot for a subset of packages.
	pkgs := make([]string, 0, 10)
	for pkg := range tree.PackageRoots {
		if len(pkgs) >= 10 {
			break
		}
		pkgs = append(pkgs, pkg)
	}

	start = time.Now()
	iterations := 10000
	for i := 0; i < iterations; i++ {
		_ = tree.SubgraphRoot(pkgs)
	}
	subDur := time.Since(start)
	t.Logf("SubgraphRoot (10 pkgs, %d iters): %v (%v per call)", iterations, subDur, subDur/time.Duration(iterations))

	// Benchmark 3: DiffHierarchicalTrees (identical = fast path).
	start = time.Now()
	for i := 0; i < iterations; i++ {
		snapshot.DiffHierarchicalTrees(tree, tree)
	}
	diffSameDur := time.Since(start)
	t.Logf("DiffHierarchicalTrees (identical, %d iters): %v (%v per call)", iterations, diffSameDur, diffSameDur/time.Duration(iterations))

	// Benchmark 4: Context retrieval on Grafana tasks.
	engine := knowingctx.NewContextEngine(st)
	tasks := []string{
		"add rate limiting to the API server",
		"fix alerting notification channel configuration",
		"implement dashboard provisioning from git repository",
		"add RBAC permissions check to datasource proxy",
		"refactor the unified storage layer for blob handling",
	}

	t.Log("")
	t.Log("=== Context Retrieval (5 Grafana tasks) ===")
	var totalCtxDur time.Duration
	for _, task := range tasks {
		start = time.Now()
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task,
			TokenBudget:     5000,
			Format:          "json",
		})
		dur := time.Since(start)
		totalCtxDur += dur
		if err != nil {
			t.Logf("  %.40s: ERROR %v", task, err)
			continue
		}
		t.Logf("  %.40s: %d symbols, %v", task, len(block.Symbols), dur)
	}
	avgCtx := totalCtxDur / time.Duration(len(tasks))
	t.Logf("\nAvg context retrieval: %v", avgCtx)

	// Summary.
	t.Log("")
	t.Log("=== Summary ===")
	t.Logf("Graph: %d edges, %d packages, %d edge-type roots", len(edges), len(tree.PackageRoots), len(tree.EdgeTypeRoots))
	t.Logf("Build hierarchical tree: %v", buildDur)
	t.Logf("SubgraphRoot: %v/call", subDur/time.Duration(iterations))
	t.Logf("Diff (identical): %v/call", diffSameDur/time.Duration(iterations))
	t.Logf("Context retrieval avg: %v", avgCtx)

	// Assertions: the system should work at this scale.
	if len(edges) < 100000 {
		t.Errorf("expected 100K+ edges from Grafana, got %d", len(edges))
	}
	if buildDur > 30*time.Second {
		t.Errorf("hierarchical tree build took too long: %v (expected <30s)", buildDur)
	}
	if avgCtx > 10*time.Second {
		t.Errorf("context retrieval too slow: %v (expected <10s)", avgCtx)
	}
}
