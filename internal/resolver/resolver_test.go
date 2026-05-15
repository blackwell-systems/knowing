package resolver

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockStore implements the Store interface for testing.
type mockStore struct {
	nodes map[types.Hash]*types.Node
	edges map[types.Hash]*types.Edge
	repos []types.Repo
}

func newMockStore() *mockStore {
	return &mockStore{
		nodes: make(map[types.Hash]*types.Node),
		edges: make(map[types.Hash]*types.Edge),
	}
}

func (m *mockStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	var dangling []types.Edge
	for _, e := range m.edges {
		if _, ok := m.nodes[e.TargetHash]; !ok {
			dangling = append(dangling, *e)
		}
	}
	return dangling, nil
}

func (m *mockStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	return m.repos, nil
}

func (m *mockStore) GetNode(_ context.Context, hash types.Hash) (*types.Node, error) {
	if n, ok := m.nodes[hash]; ok {
		return n, nil
	}
	return nil, nil
}

func (m *mockStore) NodesByName(_ context.Context, _ string) ([]types.Node, error) {
	var nodes []types.Node
	for _, n := range m.nodes {
		nodes = append(nodes, *n)
	}
	return nodes, nil
}

func (m *mockStore) DeleteEdge(_ context.Context, hash types.Hash) error {
	delete(m.edges, hash)
	return nil
}

func (m *mockStore) PutEdge(_ context.Context, e types.Edge) error {
	eCopy := e
	m.edges[e.EdgeHash] = &eCopy
	return nil
}

func (m *mockStore) addNode(n types.Node) {
	m.nodes[n.NodeHash] = &n
}

func (m *mockStore) addEdge(e types.Edge) {
	m.edges[e.EdgeHash] = &e
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
	if _, exists := ms.edges[danglingEdge.EdgeHash]; exists {
		t.Error("old dangling edge should have been deleted")
	}

	// Find the new edge.
	var foundNewEdge bool
	for _, e := range ms.edges {
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
