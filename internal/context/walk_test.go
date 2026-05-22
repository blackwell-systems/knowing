package context

import (
	stdctx "context"
	"math"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// walkMockStore implements types.GraphStore for RWR testing.
// Unlike the main mockStore, it returns all edges when edgeType is empty.
// When communities is non-nil, it also implements the communityProvider interface.
type walkMockStore struct {
	mockStore
	communities map[types.Hash]int // optional: if set, enables communityProvider
}

func (m *walkMockStore) CommunitiesForNodes(_ stdctx.Context, hashes []types.Hash) (map[types.Hash]int, error) {
	if m.communities == nil {
		return nil, nil
	}
	result := make(map[types.Hash]int)
	for _, h := range hashes {
		if cid, ok := m.communities[h]; ok {
			result[h] = cid
		}
	}
	return result, nil
}

func (m *walkMockStore) EdgesFrom(_ stdctx.Context, sourceHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.SourceHash == sourceHash && (edgeType == "" || e.EdgeType == edgeType) {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *walkMockStore) EdgesTo(_ stdctx.Context, targetHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.TargetHash == targetHash && (edgeType == "" || e.EdgeType == edgeType) {
			result = append(result, e)
		}
	}
	return result, nil
}

func hashFor(name string) types.Hash {
	return types.NewHash([]byte(name))
}

func TestRWR_EmptySeeds(t *testing.T) {
	store := &walkMockStore{}
	scores, err := RandomWalkWithRestart(stdctx.Background(), store, nil, 0.2, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if scores != nil {
		t.Errorf("expected nil scores for empty seeds, got %v", scores)
	}
}

func TestRWR_SingleSeedNoEdges(t *testing.T) {
	store := &walkMockStore{}
	seed := hashFor("A")

	scores, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{seed}, 0.2, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scores) != 1 {
		t.Fatalf("expected 1 score entry, got %d", len(scores))
	}
	if scores[seed] != 1.0 {
		t.Errorf("expected seed score 1.0, got %f", scores[seed])
	}
}

func TestRWR_LinearChain(t *testing.T) {
	// A -> B -> C (all "calls" edges)
	a := hashFor("A")
	b := hashFor("B")
	c := hashFor("C")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: a, TargetHash: b, EdgeType: "calls"},
				{EdgeHash: hashFor("e2"), SourceHash: b, TargetHash: c, EdgeType: "calls"},
			},
		},
	}

	scores, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{a}, 0.2, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// B should score higher than C (closer to seed A).
	if scores[b] <= scores[c] {
		t.Errorf("expected B (%f) > C (%f): B is closer to seed A", scores[b], scores[c])
	}
	// A should have a positive score (it's the seed and gets restart probability).
	if scores[a] <= 0 {
		t.Errorf("expected positive score for seed A, got %f", scores[a])
	}
}

func TestRWR_StarTopology(t *testing.T) {
	// Hub calls 5 leaves. Seed from leaf0.
	hub := hashFor("hub")
	leaves := make([]types.Hash, 5)
	var edges []types.Edge
	for i := 0; i < 5; i++ {
		leaves[i] = hashFor("leaf" + string(rune('0'+i)))
		edges = append(edges, types.Edge{
			EdgeHash:   hashFor("e-hub-" + string(rune('0'+i))),
			SourceHash: hub,
			TargetHash: leaves[i],
			EdgeType:   "calls",
		})
	}

	store := &walkMockStore{
		mockStore: mockStore{edges: edges},
	}

	// Seed from leaf0.
	scores, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{leaves[0]}, 0.2, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Hub should score highest among non-seed nodes because it connects to the seed
	// via the incoming edge (hub->leaf0 means leaf0's EdgesTo returns hub).
	// Actually, hub is connected to leaf0 via EdgesTo on leaf0 which returns the hub.
	// So from leaf0, probability flows to hub. Hub is the structural center.
	for i := 1; i < 5; i++ {
		if scores[hub] < scores[leaves[i]] {
			t.Errorf("expected hub (%f) >= leaf%d (%f)", scores[hub], i, scores[leaves[i]])
		}
	}
}

func TestRWR_Convergence(t *testing.T) {
	// Triangle: A->B, B->C, C->A
	a := hashFor("A")
	b := hashFor("B")
	c := hashFor("C")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: a, TargetHash: b, EdgeType: "calls"},
				{EdgeHash: hashFor("e2"), SourceHash: b, TargetHash: c, EdgeType: "calls"},
				{EdgeHash: hashFor("e3"), SourceHash: c, TargetHash: a, EdgeType: "calls"},
			},
		},
	}

	scores, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{a}, 0.2, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After normalization, the max is 1.0. But pre-normalization the raw
	// probabilities should sum to ~1.0. Since scores are normalized by dividing
	// by max, we verify that scores are in (0, 1] and the max is exactly 1.0.
	maxScore := 0.0
	for _, v := range scores {
		if v > maxScore {
			maxScore = v
		}
	}
	if math.Abs(maxScore-1.0) > 0.001 {
		t.Errorf("expected max score to be 1.0 after normalization, got %f", maxScore)
	}

	// All three nodes should have positive scores.
	for _, h := range []types.Hash{a, b, c} {
		if scores[h] <= 0 {
			t.Errorf("expected positive score for node, got %f", scores[h])
		}
	}
}

func TestRWR_AlphaEffect(t *testing.T) {
	// Linear chain: A -> B -> C -> D -> E
	nodes := make([]types.Hash, 5)
	var edges []types.Edge
	for i := 0; i < 5; i++ {
		nodes[i] = hashFor("N" + string(rune('0'+i)))
	}
	for i := 0; i < 4; i++ {
		edges = append(edges, types.Edge{
			EdgeHash:   hashFor("e" + string(rune('0'+i))),
			SourceHash: nodes[i],
			TargetHash: nodes[i+1],
			EdgeType:   "calls",
		})
	}

	store := &walkMockStore{mockStore: mockStore{edges: edges}}

	// High alpha (0.5): concentrates near seed.
	highAlpha, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{nodes[0]}, 0.5, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Low alpha (0.1): spreads further.
	lowAlpha, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{nodes[0]}, 0.1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With high alpha, the ratio of distant node (E, index 4) to near node (B, index 1)
	// should be lower than with low alpha (more concentration near seed).
	highRatio := highAlpha[nodes[4]] / highAlpha[nodes[1]]
	lowRatio := lowAlpha[nodes[4]] / lowAlpha[nodes[1]]

	if highRatio >= lowRatio {
		t.Errorf("expected high alpha to concentrate near seeds: "+
			"highAlpha ratio (far/near) = %f, lowAlpha ratio = %f", highRatio, lowRatio)
	}
}

func TestRWR_EdgeWeights(t *testing.T) {
	// From a single seed A, there are two edges:
	// A -> B via "calls" (weight 1.0)
	// A -> C via "imports" (weight 0.5)
	a := hashFor("A")
	b := hashFor("B")
	c := hashFor("C")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: a, TargetHash: b, EdgeType: "calls"},
				{EdgeHash: hashFor("e2"), SourceHash: a, TargetHash: c, EdgeType: "imports"},
			},
		},
	}

	scores, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{a}, 0.2, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// B should receive more probability than C because "calls" has higher weight.
	if scores[b] <= scores[c] {
		t.Errorf("expected B (%f) > C (%f): calls edge weight > imports edge weight",
			scores[b], scores[c])
	}
}

func TestCommunityFilteredRWR_NilFilter(t *testing.T) {
	// With nil communityIDs, CommunityFilteredRWR should produce the same results
	// as RandomWalkWithRestart.
	a := hashFor("A")
	b := hashFor("B")
	c := hashFor("C")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: a, TargetHash: b, EdgeType: "calls"},
				{EdgeHash: hashFor("e2"), SourceHash: b, TargetHash: c, EdgeType: "calls"},
			},
		},
	}

	// Get baseline from RandomWalkWithRestart.
	baseline, err := RandomWalkWithRestart(stdctx.Background(), store, []types.Hash{a}, 0.2, 50)
	if err != nil {
		t.Fatalf("unexpected error from RWR: %v", err)
	}

	// Get result from CommunityFilteredRWR with nil filter.
	filtered, err := CommunityFilteredRWR(stdctx.Background(), store, []types.Hash{a}, 0.2, 50, nil)
	if err != nil {
		t.Fatalf("unexpected error from CommunityFilteredRWR: %v", err)
	}

	// Results should match within tolerance.
	for hash, baseScore := range baseline {
		filtScore, exists := filtered[hash]
		if !exists {
			t.Errorf("node %v present in baseline but missing from filtered result", hash)
			continue
		}
		if math.Abs(baseScore-filtScore) > 0.001 {
			t.Errorf("score mismatch for node %v: baseline=%f, filtered=%f", hash, baseScore, filtScore)
		}
	}
	for hash := range filtered {
		if _, exists := baseline[hash]; !exists {
			t.Errorf("node %v present in filtered but missing from baseline", hash)
		}
	}
}

func TestCommunityFilteredRWR_FilterExcludesNodes(t *testing.T) {
	// Graph topology:
	//   A -> B -> E (community 1: A, B, E)
	//   A -> C -> D (community 2: C, D)
	//
	// Seed from A with filter {1: true}.
	// - B is in community 1 and gets expanded, so E is reachable.
	// - C is in community 2 so it is NOT expanded; D is unreachable.
	// - C still receives some probability from A->C edge (edges to filtered
	//   nodes are still traversed as dead ends).
	// - D should have zero/negligible score (only reachable via C->D which
	//   is never loaded because C is not expanded).
	a := hashFor("A")
	b := hashFor("B")
	c := hashFor("C")
	d := hashFor("D")
	e := hashFor("E")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: a, TargetHash: b, EdgeType: "calls"},
				{EdgeHash: hashFor("e2"), SourceHash: a, TargetHash: c, EdgeType: "calls"},
				{EdgeHash: hashFor("e3"), SourceHash: b, TargetHash: e, EdgeType: "calls"},
				{EdgeHash: hashFor("e4"), SourceHash: c, TargetHash: d, EdgeType: "calls"},
			},
		},
		communities: map[types.Hash]int{
			a: 1,
			b: 1,
			c: 2,
			d: 2,
			e: 1,
		},
	}

	// Filter to community 1 only.
	scores, err := CommunityFilteredRWR(stdctx.Background(), store, []types.Hash{a}, 0.2, 50, map[int]bool{1: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// B should have a non-trivial score (it's in the allowed community).
	if scores[b] < 0.01 {
		t.Errorf("expected B to have non-trivial score, got %f", scores[b])
	}

	// E should have a non-trivial score: B was expanded so B->E is loaded.
	if scores[e] < 0.01 {
		t.Errorf("expected E to have non-trivial score (reachable via expanded B), got %f", scores[e])
	}

	// D should have zero or negligible score: it's only reachable via C's
	// outgoing edges, but C was never expanded (community 2 filtered out),
	// so C->D is never loaded into the adjacency map.
	if scores[d] > 0.01 {
		t.Errorf("expected D to have negligible score (only reachable via filtered-out C), got %f", scores[d])
	}
}

func TestCommunityFilteredRWR_SeedsAlwaysIncluded(t *testing.T) {
	// Seed from X in community 3, with filter {1: true}.
	// X's community is NOT in the filter, but seeds must always be included.
	x := hashFor("X")
	y := hashFor("Y")

	store := &walkMockStore{
		mockStore: mockStore{
			edges: []types.Edge{
				{EdgeHash: hashFor("e1"), SourceHash: x, TargetHash: y, EdgeType: "calls"},
			},
		},
		communities: map[types.Hash]int{
			x: 3,
			y: 1,
		},
	}

	// Filter only allows community 1, but X is in community 3 and is the seed.
	scores, err := CommunityFilteredRWR(stdctx.Background(), store, []types.Hash{x}, 0.2, 50, map[int]bool{1: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// X should still have a score (it's the seed; seeds are never filtered).
	if scores[x] <= 0 {
		t.Errorf("expected seed X to have positive score regardless of community filter, got %f", scores[x])
	}

	// Y should also have a score since it's in community 1 (allowed).
	if scores[y] <= 0 {
		t.Errorf("expected Y (community 1) to have positive score, got %f", scores[y])
	}
}
