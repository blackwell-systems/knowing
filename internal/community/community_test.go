package community

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func hash(s string) types.Hash {
	return types.NewHash([]byte(s))
}

// buildTwoClusterGraph creates a graph with two dense clusters connected by one bridge edge.
//
//	Cluster A: a1 <-> a2 <-> a3
//	Cluster B: b1 <-> b2 <-> b3
//	Bridge:    a3 <-> b1
func buildTwoClusterGraph() *Graph {
	a1, a2, a3 := hash("a1"), hash("a2"), hash("a3")
	b1, b2, b3 := hash("b1"), hash("b2"), hash("b3")
	nodes := []types.Hash{a1, a2, a3, b1, b2, b3}
	adj := map[types.Hash][]WeightedEdge{
		a1: {{a2, 1}, {a3, 1}},
		a2: {{a1, 1}, {a3, 1}},
		a3: {{a1, 1}, {a2, 1}, {b1, 0.1}},
		b1: {{b2, 1}, {b3, 1}, {a3, 0.1}},
		b2: {{b1, 1}, {b3, 1}},
		b3: {{b1, 1}, {b2, 1}},
	}
	nodeSet := make(map[types.Hash]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n] = true
	}
	return &Graph{Nodes: nodes, Adj: adj, NodeSet: nodeSet, EdgeCount: 7}
}

func TestLouvain_Detect_TwoClusters(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &Louvain{Resolution: 1.0, MaxPasses: 20}
	comm := algo.Detect(g)

	if len(comm) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(comm))
	}

	// a1, a2, a3 should be in same community; b1, b2, b3 in another.
	a1, a2, a3 := hash("a1"), hash("a2"), hash("a3")
	b1, b2, b3 := hash("b1"), hash("b2"), hash("b3")

	if comm[a1] != comm[a2] || comm[a2] != comm[a3] {
		t.Error("cluster A nodes should be in the same community")
	}
	if comm[b1] != comm[b2] || comm[b2] != comm[b3] {
		t.Error("cluster B nodes should be in the same community")
	}
	if comm[a1] == comm[b1] {
		t.Error("clusters A and B should be in different communities")
	}
}

func TestLouvain_DetectIncremental_FrozenNodes(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &Louvain{Resolution: 1.0, MaxPasses: 20}

	// Full detection first.
	full := algo.Detect(g)

	a1, a2, a3 := hash("a1"), hash("a2"), hash("a3")
	b1, b2, b3 := hash("b1"), hash("b2"), hash("b3")

	// Incremental with no changed nodes: all frozen, assignments unchanged.
	noChanges := make(map[types.Hash]bool)
	inc := algo.DetectIncremental(g, full, noChanges)

	for _, n := range g.Nodes {
		if inc[n] != full[n] {
			t.Errorf("node %s changed from %d to %d with no changes", n, full[n], inc[n])
		}
	}

	// Incremental with only cluster A changed: B stays frozen.
	changedA := map[types.Hash]bool{a1: true, a2: true, a3: true}
	incA := algo.DetectIncremental(g, full, changedA)

	// B nodes must keep their original community.
	if incA[b1] != incA[b2] || incA[b2] != incA[b3] {
		t.Error("frozen cluster B nodes should stay in the same community")
	}
	// A nodes should still form a cluster (may have different ID but same community).
	if incA[a1] != incA[a2] || incA[a2] != incA[a3] {
		t.Error("changed cluster A nodes should still form a community")
	}
}

func TestLouvain_DetectIncremental_NilPrevious(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &Louvain{Resolution: 1.0, MaxPasses: 20}

	// nil previous should fall back to full detection.
	comm := algo.DetectIncremental(g, nil, nil)
	if len(comm) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(comm))
	}
}

func TestLouvain_DetectIncremental_NewNode(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &Louvain{Resolution: 1.0, MaxPasses: 20}
	full := algo.Detect(g)

	// Add a new node c1 connected to b2 and b3.
	c1 := hash("c1")
	g.Nodes = append(g.Nodes, c1)
	g.NodeSet[c1] = true
	g.Adj[c1] = []WeightedEdge{{hash("b2"), 1}, {hash("b3"), 1}}
	g.Adj[hash("b2")] = append(g.Adj[hash("b2")], WeightedEdge{c1, 1})
	g.Adj[hash("b3")] = append(g.Adj[hash("b3")], WeightedEdge{c1, 1})

	changed := map[types.Hash]bool{c1: true}
	inc := algo.DetectIncremental(g, full, changed)

	// c1 should join cluster B since it's connected to b2 and b3.
	b1 := hash("b1")
	if inc[c1] != inc[b1] {
		t.Errorf("new node c1 (comm %d) should join cluster B (comm %d)", inc[c1], inc[b1])
	}
}

func TestLabelPropagation_Detect_TwoClusters(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &LabelPropagation{MaxIterations: 50}
	comm := algo.Detect(g)

	if len(comm) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(comm))
	}

	a1, a2, a3 := hash("a1"), hash("a2"), hash("a3")
	b1, b2, b3 := hash("b1"), hash("b2"), hash("b3")

	if comm[a1] != comm[a2] || comm[a2] != comm[a3] {
		t.Error("cluster A nodes should be in the same community")
	}
	if comm[b1] != comm[b2] || comm[b2] != comm[b3] {
		t.Error("cluster B nodes should be in the same community")
	}
}

func TestLabelPropagation_DetectIncremental_FrozenNodes(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &LabelPropagation{MaxIterations: 50}
	full := algo.Detect(g)

	b1, b2, b3 := hash("b1"), hash("b2"), hash("b3")

	// Incremental with no changes: frozen.
	noChanges := make(map[types.Hash]bool)
	inc := algo.DetectIncremental(g, full, noChanges)
	for _, n := range g.Nodes {
		if inc[n] != full[n] {
			t.Errorf("node %s changed from %d to %d with no changes", n, full[n], inc[n])
		}
	}

	// Only change cluster A: B stays frozen.
	changedA := map[types.Hash]bool{hash("a1"): true, hash("a2"): true, hash("a3"): true}
	incA := algo.DetectIncremental(g, full, changedA)
	if incA[b1] != incA[b2] || incA[b2] != incA[b3] {
		t.Error("frozen cluster B nodes should stay in the same community")
	}
}

func TestLabelPropagation_DetectIncremental_NilPrevious(t *testing.T) {
	g := buildTwoClusterGraph()
	algo := &LabelPropagation{MaxIterations: 50}
	comm := algo.DetectIncremental(g, nil, nil)
	if len(comm) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(comm))
	}
}

// Verify both algorithms satisfy IncrementalAlgorithm at compile time.
var _ IncrementalAlgorithm = (*Louvain)(nil)
var _ IncrementalAlgorithm = (*LabelPropagation)(nil)

func TestRegistry(t *testing.T) {
	names := Names()
	if len(names) < 3 {
		t.Errorf("expected at least 3 registered algorithms, got %d", len(names))
	}
	if Get("louvain") == nil {
		t.Error("louvain not registered")
	}
	if Get("label-propagation") == nil {
		t.Error("label-propagation not registered")
	}
	if Get("nonexistent") != nil {
		t.Error("nonexistent algorithm should return nil")
	}
}
