package time_to_consistency

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

func TestK8sLatency_CacheComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping k8s latency bench in short mode")
	}

	dbPath := filepath.Join("..", "cross-system", "corpus", "repos", "kubernetes", ".knowing", "graph.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Skipf("k8s DB not found: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	tasks := []string{
		"refactor the scheduler to use a priority queue for pod binding",
		"add rate limiting to the API server request handler",
		"fix the kubelet node status reporting when network is flaky",
	}

	// Phase 1: Queries WITHOUT adjacency cache.
	t.Log("=== Phase 1: k8s Latency WITHOUT adjacency cache ===")
	var uncachedTimes []time.Duration
	for _, task := range tasks {
		engine := knowingctx.NewContextEngine(st)
		start := time.Now()
		res, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task,
			TokenBudget:     5000,
			Format:          "json",
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Logf("  ERROR: %v", err)
			continue
		}
		uncachedTimes = append(uncachedTimes, elapsed)
		t.Logf("  [uncached] %s: %v (%d symbols)", task[:50], elapsed, len(res.Symbols))
	}

	// Phase 2: Build adjacency cache.
	t.Log("")
	t.Log("=== Building adjacency cache (782K edges) ===")
	cacheStart := time.Now()
	if err := knowingctx.BuildAdjacencyCache(ctx, st); err != nil {
		t.Fatalf("cache build failed: %v", err)
	}
	cacheDuration := time.Since(cacheStart)
	t.Logf("  Cache built in %v", cacheDuration)

	// Phase 3: Queries WITH adjacency cache.
	t.Log("")
	t.Log("=== Phase 3: k8s Latency WITH adjacency cache ===")
	var cachedTimes []time.Duration
	for _, task := range tasks {
		engine := knowingctx.NewContextEngine(st)
		start := time.Now()
		res, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task,
			TokenBudget:     5000,
			Format:          "json",
		})
		elapsed := time.Since(start)
		if err != nil {
			t.Logf("  ERROR: %v", err)
			continue
		}
		cachedTimes = append(cachedTimes, elapsed)
		t.Logf("  [cached]   %s: %v (%d symbols)", task[:50], elapsed, len(res.Symbols))
	}

	// Phase 4: Report.
	t.Log("")
	t.Log("=== Results ===")
	t.Logf("  | Task | Uncached | Cached | Speedup |")
	t.Logf("  |------|----------|--------|---------|")
	for i := range tasks {
		if i < len(uncachedTimes) && i < len(cachedTimes) {
			speedup := float64(uncachedTimes[i]) / float64(cachedTimes[i])
			t.Logf("  | %s | %v | %v | %.1fx |",
				tasks[i][:40], uncachedTimes[i], cachedTimes[i], speedup)
		}
	}
	t.Logf("  Cache build time: %v", cacheDuration)

	// Compute averages.
	if len(uncachedTimes) > 0 && len(cachedTimes) > 0 {
		var avgUncached, avgCached time.Duration
		for _, d := range uncachedTimes {
			avgUncached += d
		}
		for _, d := range cachedTimes {
			avgCached += d
		}
		avgUncached /= time.Duration(len(uncachedTimes))
		avgCached /= time.Duration(len(cachedTimes))
		t.Logf("")
		t.Logf("  Average uncached: %v", avgUncached)
		t.Logf("  Average cached:   %v", avgCached)
		t.Logf("  Overall speedup:  %.1fx", float64(avgUncached)/float64(avgCached))
	}

	fmt.Println()
}
