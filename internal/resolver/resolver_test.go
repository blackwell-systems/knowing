package resolver

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/testutil"
	"github.com/blackwell-systems/knowing/internal/types"
)

// mockStore embeds *testutil.MockGraphStore and overrides the methods
// required by the resolver.Store interface with custom test logic.
type mockStore struct {
	*testutil.MockGraphStore
	repos []types.Repo
}

func newMockStore() *mockStore {
	return &mockStore{
		MockGraphStore: testutil.NewMockGraphStore(),
	}
}

func (m *mockStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	var dangling []types.Edge
	for _, e := range m.Edges {
		if _, ok := m.Nodes[e.TargetHash]; !ok {
			dangling = append(dangling, *e)
		}
	}
	return dangling, nil
}

func (m *mockStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	return m.repos, nil
}

func (m *mockStore) NodesByName(_ context.Context, _ string) ([]types.Node, error) {
	var nodes []types.Node
	for _, n := range m.Nodes {
		nodes = append(nodes, *n)
	}
	return nodes, nil
}

func (m *mockStore) DeleteEdge(_ context.Context, hash types.Hash) error {
	delete(m.Edges, hash)
	return nil
}

func (m *mockStore) PutEdge(_ context.Context, e types.Edge) error {
	eCopy := e
	m.Edges[e.EdgeHash] = &eCopy
	return nil
}

func (m *mockStore) addNode(n types.Node) {
	m.Nodes[n.NodeHash] = &n
}

func (m *mockStore) addEdge(e types.Edge) {
	m.Edges[e.EdgeHash] = &e
}

func (m *mockStore) addRepo(r types.Repo) {
	m.repos = append(m.repos, r)
}

// TestResolve_CrossRepo verifies that a dangling edge caused by a repo URL
// mismatch is correctly retargeted to the real node.
func TestResolve_CrossRepo(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	// Node in repo B: the correct target.
	targetNodeHash := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "DoThing", "function")
	targetNode := types.Node{
		NodeHash:      targetNodeHash,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.DoThing",
		Kind:          "function",
	}
	ms.addNode(targetNode)

	// Source node in repo A.
	sourceNodeHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	sourceNode := types.Node{
		NodeHash:      sourceNodeHash,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	}
	ms.addNode(sourceNode)

	// Dangling edge: target hash was computed with repoA's URL (the bug).
	wrongTargetHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoB/pkg", types.EmptyHash, "DoThing", "function")
	danglingEdge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceNodeHash, wrongTargetHash, "calls", "ast_resolved"),
		SourceHash: sourceNodeHash,
		TargetHash: wrongTargetHash,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	}
	ms.addEdge(danglingEdge)

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 1 {
		t.Errorf("TotalDangling = %d, want 1", stats.TotalDangling)
	}
	if stats.Retargeted != 1 {
		t.Errorf("Retargeted = %d, want 1", stats.Retargeted)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Action != "retargeted" {
		t.Errorf("Action = %q, want %q", results[0].Action, "retargeted")
	}
	if results[0].ResolvedNode.NodeHash != targetNodeHash {
		t.Errorf("resolved to wrong node: got %s, want %s", results[0].ResolvedNode.NodeHash, targetNodeHash)
	}

	// Verify the old edge was deleted and a new one was created.
	if _, exists := ms.Edges[danglingEdge.EdgeHash]; exists {
		t.Error("old dangling edge should have been deleted")
	}

	// Find the new edge.
	var foundNewEdge bool
	for _, e := range ms.Edges {
		if e.TargetHash == targetNodeHash && e.SourceHash == sourceNodeHash {
			foundNewEdge = true
			if e.EdgeType != "calls" {
				t.Errorf("new edge type = %q, want %q", e.EdgeType, "calls")
			}
			break
		}
	}
	if !foundNewEdge {
		t.Error("expected new edge targeting the correct node")
	}
}

// TestResolve_NoDangling verifies that when all edges have valid targets,
// the resolver reports zero changes.
func TestResolve_NoDangling(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoURL := "https://github.com/org/repo"
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoURL)), RepoURL: repoURL})

	nodeHash := types.ComputeNodeHash(repoURL, "github.com/org/repo/pkg", types.EmptyHash, "Foo", "function")
	ms.addNode(types.Node{
		NodeHash:      nodeHash,
		QualifiedName: repoURL + "://github.com/org/repo/pkg.Foo",
		Kind:          "function",
	})

	sourceHash := types.ComputeNodeHash(repoURL, "github.com/org/repo/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoURL + "://github.com/org/repo/cmd.main",
		Kind:          "function",
	})

	// Edge with a valid target (not dangling).
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, nodeHash, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: nodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if stats.TotalDangling != 0 {
		t.Errorf("TotalDangling = %d, want 0", stats.TotalDangling)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}
}

// TestResolve_NoMatchSkipped verifies that a dangling edge with no matching
// node in any repo is gracefully skipped.
func TestResolve_NoMatchSkipped(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoURL := "https://github.com/org/repo"
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoURL)), RepoURL: repoURL})

	sourceHash := types.ComputeNodeHash(repoURL, "github.com/org/repo/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoURL + "://github.com/org/repo/cmd.main",
		Kind:          "function",
	})

	// Dangling edge pointing to a completely nonexistent node.
	fakeTargetHash := types.NewHash([]byte("nonexistent-target"))
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, fakeTargetHash, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: fakeTargetHash,
		EdgeType:   "calls",
		Confidence: 0.5,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 1 {
		t.Errorf("TotalDangling = %d, want 1", stats.TotalDangling)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Action != "skipped" {
		t.Errorf("Action = %q, want %q", results[0].Action, "skipped")
	}
}

// TestResolve_MethodNode verifies that method nodes (with Type.Method in
// the qualified name) are parsed correctly and can be resolved.
func TestResolve_MethodNode(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	// Method node in repo B.
	targetHash := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "Greet", "method")
	ms.addNode(types.Node{
		NodeHash:      targetHash,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.Greeter.Greet",
		Kind:          "method",
	})

	// Source node in repo A.
	sourceHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	})

	// Dangling edge with wrong repo URL in target hash.
	wrongTarget := types.ComputeNodeHash(repoAURL, "github.com/org/repoB/pkg", types.EmptyHash, "Greet", "method")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongTarget, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongTarget,
		EdgeType:   "calls",
		Confidence: 0.8,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if stats.TotalDangling != 1 {
		t.Errorf("TotalDangling = %d, want 1", stats.TotalDangling)
	}
	if stats.Retargeted != 1 {
		t.Errorf("Retargeted = %d, want 1", stats.Retargeted)
	}
}

// TestResolve_EmptyStore verifies that an empty store (no repos, no nodes,
// no edges) produces zero stats and no error.
func TestResolve_EmptyStore(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if stats.TotalDangling != 0 {
		t.Errorf("TotalDangling = %d, want 0", stats.TotalDangling)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}
}

// TestResolve_SingleRepoNoCrossEdges verifies that a single repo with
// edges only within itself produces no dangling edges and no retargeting.
func TestResolve_SingleRepoNoCrossEdges(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoURL := "https://github.com/org/solo"
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoURL)), RepoURL: repoURL})

	fnA := types.ComputeNodeHash(repoURL, "github.com/org/solo/pkg", types.EmptyHash, "FuncA", "function")
	fnB := types.ComputeNodeHash(repoURL, "github.com/org/solo/pkg", types.EmptyHash, "FuncB", "function")

	ms.addNode(types.Node{
		NodeHash:      fnA,
		QualifiedName: repoURL + "://github.com/org/solo/pkg.FuncA",
		Kind:          "function",
	})
	ms.addNode(types.Node{
		NodeHash:      fnB,
		QualifiedName: repoURL + "://github.com/org/solo/pkg.FuncB",
		Kind:          "function",
	})

	// Valid edge within the same repo; target exists, so not dangling.
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(fnA, fnB, "calls", "ast_resolved"),
		SourceHash: fnA,
		TargetHash: fnB,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if stats.TotalDangling != 0 {
		t.Errorf("TotalDangling = %d, want 0", stats.TotalDangling)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}
}

// TestResolve_DanglingNoMatchInAnyRepo verifies that dangling edges whose
// target hash does not match any node's "wrong hash" in any repo are skipped
// with a clear reason.
func TestResolve_DanglingNoMatchInAnyRepo(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	sourceHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	})

	// Add a node in repoB, but the dangling edge will point to a completely
	// different function that does not exist anywhere.
	repoBNode := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "Exists", "function")
	ms.addNode(types.Node{
		NodeHash:      repoBNode,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.Exists",
		Kind:          "function",
	})

	// Dangling edge targeting a function that does not exist in any repo.
	// This is NOT a repo-URL-mismatch; the function simply does not exist.
	fakeTarget := types.ComputeNodeHash(repoAURL, "github.com/org/repoC/pkg", types.EmptyHash, "Ghost", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, fakeTarget, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: fakeTarget,
		EdgeType:   "calls",
		Confidence: 0.7,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 1 {
		t.Errorf("TotalDangling = %d, want 1", stats.TotalDangling)
	}
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Action != "skipped" {
		t.Errorf("Action = %q, want %q", results[0].Action, "skipped")
	}
	if results[0].Reason != "no matching node found in any repo" {
		t.Errorf("Reason = %q, want %q", results[0].Reason, "no matching node found in any repo")
	}
}

// TestResolve_MultipleReposCrossEdges verifies that dangling edges from
// multiple repos are retargeted correctly when nodes exist in other repos.
func TestResolve_MultipleReposCrossEdges(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"
	repoCURL := "https://github.com/org/repoC"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoCURL)), RepoURL: repoCURL})

	// Source node in repo A.
	sourceA := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceA,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	})

	// Target node in repo B.
	targetB := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "HelperB", "function")
	ms.addNode(types.Node{
		NodeHash:      targetB,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.HelperB",
		Kind:          "function",
	})

	// Target node in repo C.
	targetC := types.ComputeNodeHash(repoCURL, "github.com/org/repoC/lib", types.EmptyHash, "UtilC", "function")
	ms.addNode(types.Node{
		NodeHash:      targetC,
		QualifiedName: repoCURL + "://github.com/org/repoC/lib.UtilC",
		Kind:          "function",
	})

	// Dangling edge #1: repo A calls repo B's function but computed hash with A's URL.
	wrongTargetB := types.ComputeNodeHash(repoAURL, "github.com/org/repoB/pkg", types.EmptyHash, "HelperB", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceA, wrongTargetB, "calls", "ast_resolved"),
		SourceHash: sourceA,
		TargetHash: wrongTargetB,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	})

	// Dangling edge #2: repo A calls repo C's function but computed hash with A's URL.
	wrongTargetC := types.ComputeNodeHash(repoAURL, "github.com/org/repoC/lib", types.EmptyHash, "UtilC", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceA, wrongTargetC, "calls", "ast_resolved"),
		SourceHash: sourceA,
		TargetHash: wrongTargetC,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 2 {
		t.Errorf("TotalDangling = %d, want 2", stats.TotalDangling)
	}
	if stats.Retargeted != 2 {
		t.Errorf("Retargeted = %d, want 2", stats.Retargeted)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}

	// Both results should be retargeted.
	retargetedTargets := make(map[types.Hash]bool)
	for _, res := range results {
		if res.Action != "retargeted" {
			t.Errorf("Action = %q, want %q", res.Action, "retargeted")
		}
		if res.ResolvedNode != nil {
			retargetedTargets[res.ResolvedNode.NodeHash] = true
		}
	}
	if !retargetedTargets[targetB] {
		t.Error("expected HelperB to be a retarget destination")
	}
	if !retargetedTargets[targetC] {
		t.Error("expected UtilC to be a retarget destination")
	}

	// Old edges should be deleted, new edges should target correct nodes.
	if _, exists := ms.Edges[types.ComputeEdgeHash(sourceA, wrongTargetB, "calls", "ast_resolved")]; exists {
		t.Error("old dangling edge for HelperB should have been deleted")
	}
	if _, exists := ms.Edges[types.ComputeEdgeHash(sourceA, wrongTargetC, "calls", "ast_resolved")]; exists {
		t.Error("old dangling edge for UtilC should have been deleted")
	}

	// Verify new edges exist with correct targets.
	foundB, foundC := false, false
	for _, e := range ms.Edges {
		if e.SourceHash == sourceA && e.TargetHash == targetB {
			foundB = true
		}
		if e.SourceHash == sourceA && e.TargetHash == targetC {
			foundC = true
		}
	}
	if !foundB {
		t.Error("expected new edge targeting HelperB in repo B")
	}
	if !foundC {
		t.Error("expected new edge targeting UtilC in repo C")
	}
}

// TestResolve_ThreeReposWithDanglingEdgesToTwo verifies that a single repo
// with dangling edges to two different repos resolves both correctly.
func TestResolve_ThreeReposWithDanglingEdgesToTwo(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/serviceA"
	repoBURL := "https://github.com/org/libB"
	repoCURL := "https://github.com/org/libC"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoCURL)), RepoURL: repoCURL})

	// Source node in repo A.
	sourceHash := types.ComputeNodeHash(repoAURL, "github.com/org/serviceA/cmd", types.EmptyHash, "Run", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoAURL + "://github.com/org/serviceA/cmd.Run",
		Kind:          "function",
	})

	// Target 1 in repo B.
	targetB := types.ComputeNodeHash(repoBURL, "github.com/org/libB/auth", types.EmptyHash, "Authenticate", "function")
	ms.addNode(types.Node{
		NodeHash:      targetB,
		QualifiedName: repoBURL + "://github.com/org/libB/auth.Authenticate",
		Kind:          "function",
	})

	// Target 2 in repo C.
	targetC := types.ComputeNodeHash(repoCURL, "github.com/org/libC/log", types.EmptyHash, "Info", "function")
	ms.addNode(types.Node{
		NodeHash:      targetC,
		QualifiedName: repoCURL + "://github.com/org/libC/log.Info",
		Kind:          "function",
	})

	// Dangling edge 1: repoA -> repoB, but hash computed with repoA URL.
	wrongB := types.ComputeNodeHash(repoAURL, "github.com/org/libB/auth", types.EmptyHash, "Authenticate", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongB, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongB,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	})

	// Dangling edge 2: repoA -> repoC, but hash computed with repoA URL.
	wrongC := types.ComputeNodeHash(repoAURL, "github.com/org/libC/log", types.EmptyHash, "Info", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongC, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongC,
		EdgeType:   "calls",
		Confidence: 0.85,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 2 {
		t.Errorf("TotalDangling = %d, want 2", stats.TotalDangling)
	}
	if stats.Retargeted != 2 {
		t.Errorf("Retargeted = %d, want 2", stats.Retargeted)
	}
	if stats.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", stats.Skipped)
	}

	resolvedTargets := make(map[types.Hash]bool)
	for _, res := range results {
		if res.Action != "retargeted" {
			t.Errorf("expected retargeted, got %q", res.Action)
		}
		if res.ResolvedNode != nil {
			resolvedTargets[res.ResolvedNode.NodeHash] = true
		}
	}
	if !resolvedTargets[targetB] {
		t.Error("expected Authenticate in libB to be resolved")
	}
	if !resolvedTargets[targetC] {
		t.Error("expected Info in libC to be resolved")
	}
}

// TestResolve_DifferentQualifiedNameVariant verifies that the resolver does NOT
// match a node when the target exists under a different naming variant (e.g.,
// different package path). The resolver only matches by recomputing the hash
// with different repo URLs, not by fuzzy name matching.
func TestResolve_DifferentQualifiedNameVariant(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	// Source node in repo A.
	sourceHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	})

	// Node in repo B with a different package path variant.
	// The function "Process" exists but in "github.com/org/repoB/v2/handler" not
	// "github.com/org/repoB/handler" that the dangling edge references.
	targetHash := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/v2/handler", types.EmptyHash, "Process", "function")
	ms.addNode(types.Node{
		NodeHash:      targetHash,
		QualifiedName: repoBURL + "://github.com/org/repoB/v2/handler.Process",
		Kind:          "function",
	})

	// Dangling edge: target computed using repoA's URL AND a different pkg path
	// ("handler" instead of "v2/handler"). This should NOT resolve because
	// recomputing with repoB's URL + "handler" won't match "v2/handler".
	wrongTarget := types.ComputeNodeHash(repoAURL, "github.com/org/repoB/handler", types.EmptyHash, "Process", "function")
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongTarget, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongTarget,
		EdgeType:   "calls",
		Confidence: 0.7,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if stats.TotalDangling != 1 {
		t.Errorf("TotalDangling = %d, want 1", stats.TotalDangling)
	}
	// Should be skipped because the package path doesn't match any existing node.
	if stats.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", stats.Skipped)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}
}

// TestResolve_DifferentEdgeTypes verifies that edges with different types
// (calls vs imports) are all resolved correctly.
func TestResolve_DifferentEdgeTypes(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	// Source node in repo A.
	sourceHash := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/cmd", types.EmptyHash, "main", "function")
	ms.addNode(types.Node{
		NodeHash:      sourceHash,
		QualifiedName: repoAURL + "://github.com/org/repoA/cmd.main",
		Kind:          "function",
	})

	// Target node in repo B.
	targetHash := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "Service", "function")
	ms.addNode(types.Node{
		NodeHash:      targetHash,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.Service",
		Kind:          "function",
	})

	wrongTarget := types.ComputeNodeHash(repoAURL, "github.com/org/repoB/pkg", types.EmptyHash, "Service", "function")

	// Dangling edge with type "calls".
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongTarget, "calls", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongTarget,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	})

	// Dangling edge with type "imports".
	ms.addEdge(types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, wrongTarget, "imports", "ast_resolved"),
		SourceHash: sourceHash,
		TargetHash: wrongTarget,
		EdgeType:   "imports",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	results, stats, err := r.ResolveWithDetails(ctx)
	if err != nil {
		t.Fatalf("ResolveWithDetails: %v", err)
	}

	if stats.TotalDangling != 2 {
		t.Errorf("TotalDangling = %d, want 2", stats.TotalDangling)
	}
	if stats.Retargeted != 2 {
		t.Errorf("Retargeted = %d, want 2", stats.Retargeted)
	}

	// Verify that both edge types are preserved in the new edges.
	edgeTypes := make(map[string]bool)
	for _, e := range ms.Edges {
		if e.TargetHash == targetHash {
			edgeTypes[e.EdgeType] = true
		}
	}
	if !edgeTypes["calls"] {
		t.Error("expected retargeted edge with type 'calls'")
	}
	if !edgeTypes["imports"] {
		t.Error("expected retargeted edge with type 'imports'")
	}

	// Verify each result was retargeted to the same node.
	for _, res := range results {
		if res.Action != "retargeted" {
			t.Errorf("expected retargeted, got %q", res.Action)
		}
		if res.ResolvedNode == nil || res.ResolvedNode.NodeHash != targetHash {
			t.Errorf("expected resolved to targetHash, got %v", res.ResolvedNode)
		}
	}
}

// TestResolve_ValidEdgesNotModified verifies that the resolver does not modify
// edges that already point to valid targets.
func TestResolve_ValidEdgesNotModified(t *testing.T) {
	ctx := context.Background()
	ms := newMockStore()

	repoAURL := "https://github.com/org/repoA"
	repoBURL := "https://github.com/org/repoB"

	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoAURL)), RepoURL: repoAURL})
	ms.addRepo(types.Repo{RepoHash: types.NewHash([]byte(repoBURL)), RepoURL: repoBURL})

	// Nodes in repo A.
	fnA := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/pkg", types.EmptyHash, "FuncA", "function")
	fnB := types.ComputeNodeHash(repoAURL, "github.com/org/repoA/pkg", types.EmptyHash, "FuncB", "function")
	ms.addNode(types.Node{
		NodeHash:      fnA,
		QualifiedName: repoAURL + "://github.com/org/repoA/pkg.FuncA",
		Kind:          "function",
	})
	ms.addNode(types.Node{
		NodeHash:      fnB,
		QualifiedName: repoAURL + "://github.com/org/repoA/pkg.FuncB",
		Kind:          "function",
	})

	// Valid edge within repo A (target exists).
	validEdgeHash := types.ComputeEdgeHash(fnA, fnB, "calls", "ast_resolved")
	ms.addEdge(types.Edge{
		EdgeHash:   validEdgeHash,
		SourceHash: fnA,
		TargetHash: fnB,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	})

	// Cross-repo valid edge: source in A, target in B (target exists).
	targetB := types.ComputeNodeHash(repoBURL, "github.com/org/repoB/pkg", types.EmptyHash, "Helper", "function")
	ms.addNode(types.Node{
		NodeHash:      targetB,
		QualifiedName: repoBURL + "://github.com/org/repoB/pkg.Helper",
		Kind:          "function",
	})
	crossEdgeHash := types.ComputeEdgeHash(fnA, targetB, "calls", "ast_resolved")
	ms.addEdge(types.Edge{
		EdgeHash:   crossEdgeHash,
		SourceHash: fnA,
		TargetHash: targetB,
		EdgeType:   "calls",
		Confidence: 0.95,
		Provenance: "ast_resolved",
	})

	r := NewResolver(ms)
	stats, err := r.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// No dangling edges, so nothing should happen.
	if stats.TotalDangling != 0 {
		t.Errorf("TotalDangling = %d, want 0", stats.TotalDangling)
	}
	if stats.Retargeted != 0 {
		t.Errorf("Retargeted = %d, want 0", stats.Retargeted)
	}

	// Verify the valid edges are still intact and unchanged.
	if _, ok := ms.Edges[validEdgeHash]; !ok {
		t.Error("valid intra-repo edge was unexpectedly deleted")
	}
	if _, ok := ms.Edges[crossEdgeHash]; !ok {
		t.Error("valid cross-repo edge was unexpectedly deleted")
	}

	// Verify edge count hasn't changed (no spurious edges created).
	if len(ms.Edges) != 2 {
		t.Errorf("edge count = %d, want 2", len(ms.Edges))
	}
}

// TestExtractHashInputs verifies the internal parsing of qualified names.
func TestExtractHashInputs(t *testing.T) {
	tests := []struct {
		name       string
		node       types.Node
		wantRepo   string
		wantPkg    string
		wantSymbol string
	}{
		{
			name: "function",
			node: types.Node{
				QualifiedName: "https://github.com/org/repo://github.com/org/repo/pkg.Hello",
				Kind:          "function",
			},
			wantRepo:   "https://github.com/org/repo",
			wantPkg:    "github.com/org/repo/pkg",
			wantSymbol: "Hello",
		},
		{
			name: "method",
			node: types.Node{
				QualifiedName: "https://github.com/org/repo://github.com/org/repo/pkg.Greeter.Greet",
				Kind:          "method",
			},
			wantRepo:   "https://github.com/org/repo",
			wantPkg:    "github.com/org/repo/pkg",
			wantSymbol: "Greet",
		},
		{
			name: "type",
			node: types.Node{
				QualifiedName: "https://github.com/org/repo://github.com/org/repo/pkg.MyType",
				Kind:          "type",
			},
			wantRepo:   "https://github.com/org/repo",
			wantPkg:    "github.com/org/repo/pkg",
			wantSymbol: "MyType",
		},
		{
			name: "invalid no separator",
			node: types.Node{
				QualifiedName: "invalid-name",
				Kind:          "function",
			},
			wantRepo:   "",
			wantPkg:    "",
			wantSymbol: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, pkg, sym := extractHashInputs(tt.node)
			if repo != tt.wantRepo {
				t.Errorf("repoURL = %q, want %q", repo, tt.wantRepo)
			}
			if pkg != tt.wantPkg {
				t.Errorf("pkgPath = %q, want %q", pkg, tt.wantPkg)
			}
			if sym != tt.wantSymbol {
				t.Errorf("symbolName = %q, want %q", sym, tt.wantSymbol)
			}
		})
	}
}
