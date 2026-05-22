// Package community_detection benchmarks full vs incremental community detection
// on the live knowing codebase. Measures Louvain and label propagation with:
//   - Full detection (from scratch)
//   - Incremental with 1 changed package (~5% of nodes)
//   - Incremental with no changes (all frozen)
//
// Run: GOWORK=off go test ./bench/community-detection/ -v -count=1 -run TestCommunityBenchmark -timeout 120s
package community_detection

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/community"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

type benchStats struct {
	Min    time.Duration
	Median time.Duration
	P95    time.Duration
	Mean   time.Duration
	Stddev float64
	N      int
}

func (s benchStats) String() string {
	return fmt.Sprintf("min=%v median=%v p95=%v mean=%v stddev=%.2fms n=%d",
		s.Min, s.Median, s.P95, s.Mean, s.Stddev/float64(time.Millisecond), s.N)
}

func measure(n, warmup int, fn func()) benchStats {
	for i := 0; i < warmup; i++ {
		fn()
	}
	durations := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		fn()
		durations[i] = time.Since(start)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	var sum float64
	for _, d := range durations {
		sum += float64(d)
	}
	mean := sum / float64(n)
	var variance float64
	for _, d := range durations {
		diff := float64(d) - mean
		variance += diff * diff
	}
	stddev := math.Sqrt(variance / float64(n))
	p95idx := int(float64(n) * 0.95)
	if p95idx >= n {
		p95idx = n - 1
	}
	return benchStats{
		Min:    durations[0],
		Median: durations[n/2],
		P95:    durations[p95idx],
		Mean:   time.Duration(mean),
		Stddev: stddev,
		N:      n,
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}

// buildGraphFromDB builds a community.Graph from an indexed store.
func buildGraphFromDB(t *testing.T, st *store.SQLiteStore) *community.Graph {
	t.Helper()
	ctx := context.Background()
	repos, err := st.AllRepos(ctx)
	if err != nil {
		t.Fatalf("AllRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("no repos indexed")
	}

	var allNodes []types.Node
	var allEdges []types.Edge
	for _, repo := range repos {
		nodes, err := st.NodesByName(ctx, repo.RepoURL)
		if err != nil {
			t.Fatalf("NodesByName: %v", err)
		}
		allNodes = append(allNodes, nodes...)
		for _, n := range nodes {
			edges, err := st.EdgesFrom(ctx, n.NodeHash, "")
			if err != nil {
				t.Fatalf("EdgesFrom: %v", err)
			}
			allEdges = append(allEdges, edges...)
		}
	}

	nodeSet := make(map[types.Hash]bool, len(allNodes))
	nodeList := make([]types.Hash, 0, len(allNodes))
	for _, n := range allNodes {
		if !nodeSet[n.NodeHash] {
			nodeSet[n.NodeHash] = true
			nodeList = append(nodeList, n.NodeHash)
		}
	}

	adj := make(map[types.Hash][]community.WeightedEdge, len(nodeList))
	edgeCount := 0
	for _, e := range allEdges {
		if nodeSet[e.SourceHash] && nodeSet[e.TargetHash] {
			adj[e.SourceHash] = append(adj[e.SourceHash], community.WeightedEdge{Target: e.TargetHash, Weight: e.Confidence})
			adj[e.TargetHash] = append(adj[e.TargetHash], community.WeightedEdge{Target: e.SourceHash, Weight: e.Confidence})
			edgeCount++
		}
	}

	return &community.Graph{
		Nodes:     nodeList,
		Adj:       adj,
		NodeSet:   nodeSet,
		EdgeCount: edgeCount,
	}
}

// pickChangedNodes selects nodes from one package (simulating a single-package edit).
func pickChangedNodes(nodes []types.Node, targetPkg string) map[types.Hash]bool {
	changed := make(map[types.Hash]bool)
	for _, n := range nodes {
		if strings.Contains(n.QualifiedName, targetPkg) {
			changed[n.NodeHash] = true
		}
	}
	return changed
}

func TestCommunityBenchmark(t *testing.T) {
	repoRoot := findRepoRoot(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "bench.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	repoURL := "github.com/blackwell-systems/knowing"
	repoHash := types.NewHash([]byte(repoURL))
	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())
	if _, err := idx.IndexRepo(ctx, repoURL, repoRoot, "bench"); err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}
	_ = repoHash

	g := buildGraphFromDB(t, st)
	t.Logf("Graph: %d nodes, %d edges", len(g.Nodes), g.EdgeCount)

	// Get all nodes for package selection.
	allNodes, _ := st.NodesByName(ctx, repoURL)
	changedOnePackage := pickChangedNodes(allNodes, "internal/store")
	t.Logf("Changed nodes (internal/store): %d of %d (%.1f%%)",
		len(changedOnePackage), len(g.Nodes), float64(len(changedOnePackage))/float64(len(g.Nodes))*100)

	noChanges := make(map[types.Hash]bool)

	const runs = 10
	const warmup = 2

	// ---- Louvain ----
	louvain := &community.Louvain{Resolution: 1.0, MaxPasses: 20}

	fullResult := louvain.Detect(g)
	communities := make(map[int]bool)
	for _, c := range fullResult {
		communities[c] = true
	}
	t.Logf("Louvain full: %d communities", len(communities))

	statsFull := measure(runs, warmup, func() { louvain.Detect(g) })
	t.Logf("Louvain full detect:           %s", statsFull)

	statsIncOne := measure(runs, warmup, func() {
		louvain.DetectIncremental(g, fullResult, changedOnePackage)
	})
	t.Logf("Louvain incremental (1 pkg):   %s", statsIncOne)

	statsIncNone := measure(runs, warmup, func() {
		louvain.DetectIncremental(g, fullResult, noChanges)
	})
	t.Logf("Louvain incremental (0 changes): %s", statsIncNone)

	// ---- Label Propagation ----
	lp := &community.LabelPropagation{MaxIterations: 50}

	lpFull := lp.Detect(g)
	lpCommunities := make(map[int]bool)
	for _, c := range lpFull {
		lpCommunities[c] = true
	}
	t.Logf("Label propagation full: %d communities", len(lpCommunities))

	statsLPFull := measure(runs, warmup, func() { lp.Detect(g) })
	t.Logf("LP full detect:                %s", statsLPFull)

	statsLPIncOne := measure(runs, warmup, func() {
		lp.DetectIncremental(g, lpFull, changedOnePackage)
	})
	t.Logf("LP incremental (1 pkg):        %s", statsLPIncOne)

	statsLPIncNone := measure(runs, warmup, func() {
		lp.DetectIncremental(g, lpFull, noChanges)
	})
	t.Logf("LP incremental (0 changes):    %s", statsLPIncNone)

	// ---- Speedup calculations ----
	louvainSpeedup1 := float64(statsFull.Median) / float64(statsIncOne.Median)
	louvainSpeedup0 := float64(statsFull.Median) / float64(statsIncNone.Median)
	lpSpeedup1 := float64(statsLPFull.Median) / float64(statsLPIncOne.Median)
	lpSpeedup0 := float64(statsLPFull.Median) / float64(statsLPIncNone.Median)

	t.Logf("")
	t.Logf("=== Speedup Summary ===")
	t.Logf("Louvain: full=%v, 1-pkg=%v (%.1fx), 0-changes=%v (%.1fx)",
		statsFull.Median, statsIncOne.Median, louvainSpeedup1, statsIncNone.Median, louvainSpeedup0)
	t.Logf("LP:      full=%v, 1-pkg=%v (%.1fx), 0-changes=%v (%.1fx)",
		statsLPFull.Median, statsLPIncOne.Median, lpSpeedup1, statsLPIncNone.Median, lpSpeedup0)

	// ---- Correctness: incremental with no changes = same result ----
	incNoChange := louvain.DetectIncremental(g, fullResult, noChanges)
	for _, n := range g.Nodes {
		if incNoChange[n] != fullResult[n] {
			t.Errorf("Louvain incremental (0 changes) diverged: node community changed from %d to %d", fullResult[n], incNoChange[n])
			break
		}
	}

	// ---- Performance contracts ----
	if statsIncNone.Median > 10*time.Millisecond {
		t.Errorf("Louvain incremental (0 changes) median %v exceeds 10ms contract", statsIncNone.Median)
	}
	if statsFull.Median > 5*time.Second {
		t.Errorf("Louvain full detect median %v exceeds 5s contract", statsFull.Median)
	}
}
