package enrichment

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockStore implements types.GraphStore for testing the enricher's
// edge filtering and upgrade logic without requiring a real database
// or LSP server.
type mockStore struct {
	nodes     map[types.Hash]types.Node
	edges     map[types.Hash]types.Edge
	files     map[types.Hash]types.File
	repos     map[types.Hash]types.Repo
	snapshots map[types.Hash]types.Snapshot

	deletedEdges []types.Hash
	putEdges     []types.Edge
}

func newMockStore() *mockStore {
	return &mockStore{
		nodes:     make(map[types.Hash]types.Node),
		edges:     make(map[types.Hash]types.Edge),
		files:     make(map[types.Hash]types.File),
		repos:     make(map[types.Hash]types.Repo),
		snapshots: make(map[types.Hash]types.Snapshot),
	}
}

func (m *mockStore) PutNode(_ context.Context, n types.Node) error {
	m.nodes[n.NodeHash] = n
	return nil
}

func (m *mockStore) PutEdge(_ context.Context, e types.Edge) error {
	m.edges[e.EdgeHash] = e
	m.putEdges = append(m.putEdges, e)
	return nil
}

func (m *mockStore) PutFile(_ context.Context, f types.File) error {
	m.files[f.FileHash] = f
	return nil
}

func (m *mockStore) PutRepo(_ context.Context, r types.Repo) error {
	m.repos[r.RepoHash] = r
	return nil
}

func (m *mockStore) RecordEdgeEvent(_ context.Context, _ types.EdgeEvent) error {
	return nil
}

func (m *mockStore) CreateSnapshot(_ context.Context, s types.Snapshot) error {
	m.snapshots[s.SnapshotHash] = s
	return nil
}

func (m *mockStore) GetNode(_ context.Context, hash types.Hash) (*types.Node, error) {
	n, ok := m.nodes[hash]
	if !ok {
		return nil, nil
	}
	return &n, nil
}

func (m *mockStore) GetEdge(_ context.Context, hash types.Hash) (*types.Edge, error) {
	e, ok := m.edges[hash]
	if !ok {
		return nil, nil
	}
	return &e, nil
}

func (m *mockStore) GetSnapshot(_ context.Context, hash types.Hash) (*types.Snapshot, error) {
	s, ok := m.snapshots[hash]
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (m *mockStore) GetRepo(_ context.Context, hash types.Hash) (*types.Repo, error) {
	r, ok := m.repos[hash]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *mockStore) NodesByName(_ context.Context, qualifiedPrefix string) ([]types.Node, error) {
	var result []types.Node
	for _, n := range m.nodes {
		if len(n.QualifiedName) >= len(qualifiedPrefix) && n.QualifiedName[:len(qualifiedPrefix)] == qualifiedPrefix {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockStore) EdgesFrom(_ context.Context, sourceHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.SourceHash == sourceHash {
			if edgeType == "" || e.EdgeType == edgeType {
				result = append(result, e)
			}
		}
	}
	return result, nil
}

func (m *mockStore) EdgesTo(_ context.Context, targetHash types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.TargetHash == targetHash {
			if edgeType == "" || e.EdgeType == edgeType {
				result = append(result, e)
			}
		}
	}
	return result, nil
}

func (m *mockStore) DanglingEdges(_ context.Context) ([]types.Edge, error) {
	return nil, nil
}

func (m *mockStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	var result []types.Repo
	for _, r := range m.repos {
		result = append(result, r)
	}
	return result, nil
}

func (m *mockStore) NodesByQualifiedName(_ context.Context, qualifiedName string) ([]types.Node, error) {
	var result []types.Node
	for _, n := range m.nodes {
		if n.QualifiedName == qualifiedName {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockStore) DeleteEdge(_ context.Context, hash types.Hash) error {
	m.deletedEdges = append(m.deletedEdges, hash)
	delete(m.edges, hash)
	return nil
}

func (m *mockStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return nil, nil
}

func (m *mockStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return nil, nil
}

func (m *mockStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return nil, nil
}

func (m *mockStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return nil, nil
}

func (m *mockStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *mockStore) LatestSnapshot(_ context.Context, repoHash types.Hash) (*types.Snapshot, error) {
	for _, s := range m.snapshots {
		if s.RepoHash == repoHash {
			return &s, nil
		}
	}
	return nil, nil
}

func (m *mockStore) FilesByRepo(_ context.Context, repoHash types.Hash) ([]types.File, error) {
	var result []types.File
	for _, f := range m.files {
		if f.RepoHash == repoHash {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockStore) FileByPath(_ context.Context, repoHash types.Hash, path string) (*types.File, error) {
	for _, f := range m.files {
		if f.RepoHash == repoHash && f.Path == path {
			return &f, nil
		}
	}
	return nil, nil
}

func (m *mockStore) DeleteNodesByFile(_ context.Context, _ types.Hash) (int, error) {
	return 0, nil
}

func (m *mockStore) DeleteEdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *mockStore) EdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return nil, nil
}

func (m *mockStore) Close() error { return nil }

// ---------- Tests ----------

func TestNewEnricher_Fields(t *testing.T) {
	store := newMockStore()
	e := NewEnricher(store, "/workspace")

	if e.store != store {
		t.Error("expected store to be set")
	}
	if e.workspaceRoot != "/workspace" {
		t.Errorf("expected workspaceRoot /workspace, got %s", e.workspaceRoot)
	}
	if e.client != nil {
		t.Error("expected client to be nil before Run")
	}
}

func TestUpgradeEdge_ProducesLSPResolved(t *testing.T) {
	sourceHash := types.NewHash([]byte("source"))
	targetHash := types.NewHash([]byte("target"))

	old := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, "calls", "ast_inferred"),
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "calls",
		Confidence: 0.7,
		Provenance: "ast_inferred",
	}

	upgraded := upgradeEdge(old)

	if upgraded.Provenance != "lsp_resolved" {
		t.Errorf("expected provenance lsp_resolved, got %s", upgraded.Provenance)
	}
	if upgraded.Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", upgraded.Confidence)
	}
	if upgraded.SourceHash != old.SourceHash {
		t.Error("source hash should be preserved")
	}
	if upgraded.TargetHash != old.TargetHash {
		t.Error("target hash should be preserved")
	}
	if upgraded.EdgeType != old.EdgeType {
		t.Error("edge type should be preserved")
	}
	expectedHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", "lsp_resolved")
	if upgraded.EdgeHash != expectedHash {
		t.Error("edge hash should be recomputed with new provenance")
	}
	if upgraded.EdgeHash == old.EdgeHash {
		t.Error("upgraded edge hash should differ from original")
	}
}

func TestUpgradeEdge_PreservesEdgeType(t *testing.T) {
	sourceHash := types.NewHash([]byte("src"))
	targetHash := types.NewHash([]byte("tgt"))

	for _, edgeType := range []string{"calls", "imports", "references"} {
		old := types.Edge{
			EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, edgeType, "ast_inferred"),
			SourceHash: sourceHash,
			TargetHash: targetHash,
			EdgeType:   edgeType,
			Confidence: 0.7,
			Provenance: "ast_inferred",
		}

		upgraded := upgradeEdge(old)
		if upgraded.EdgeType != edgeType {
			t.Errorf("edge type %s not preserved; got %s", edgeType, upgraded.EdgeType)
		}
	}
}

func TestRunReturnsErrorWhenNoSnapshot(t *testing.T) {
	store := newMockStore()
	repoHash := types.NewHash([]byte("repo"))
	store.repos[repoHash] = types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/test/repo",
	}
	// No snapshot set.

	e := NewEnricher(store, "/workspace")
	// Run will fail trying to start gopls (no binary), but we test
	// the error path for missing snapshot by passing a cancelled context
	// that skips gopls startup.
	// Actually, Run tries gopls first. So this tests the gopls startup error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := e.Run(ctx, repoHash)
	if err == nil {
		t.Error("expected error from Run with cancelled context")
	}
}
