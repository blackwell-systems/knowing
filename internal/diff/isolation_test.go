package diff

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// mockIsolationStore implements the subset of types.GraphStore needed for isolation tests.
type mockIsolationStore struct {
	nodes map[types.Hash][]types.Node
	edges []types.Edge
	allNodes map[types.Hash]*types.Node
}

func (m *mockIsolationStore) NodesByFileHash(_ context.Context, fh types.Hash) ([]types.Node, error) {
	return m.nodes[fh], nil
}

func (m *mockIsolationStore) EdgesTo(_ context.Context, target types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.TargetHash == target {
			if edgeType == "" || e.EdgeType == edgeType {
				result = append(result, e)
			}
		}
	}
	return result, nil
}

func (m *mockIsolationStore) EdgesFrom(_ context.Context, source types.Hash, edgeType string) ([]types.Edge, error) {
	var result []types.Edge
	for _, e := range m.edges {
		if e.SourceHash == source {
			if edgeType == "" || e.EdgeType == edgeType {
				result = append(result, e)
			}
		}
	}
	return result, nil
}

func (m *mockIsolationStore) GetNode(_ context.Context, hash types.Hash) (*types.Node, error) {
	if n, ok := m.allNodes[hash]; ok {
		return n, nil
	}
	return nil, nil
}

// Stub out remaining GraphStore methods.
func (m *mockIsolationStore) PutNode(_ context.Context, _ types.Node) error { return nil }
func (m *mockIsolationStore) PutEdge(_ context.Context, _ types.Edge) error { return nil }
func (m *mockIsolationStore) PutFile(_ context.Context, _ types.File) error { return nil }
func (m *mockIsolationStore) PutRepo(_ context.Context, _ types.Repo) error { return nil }
func (m *mockIsolationStore) RecordEdgeEvent(_ context.Context, _ types.EdgeEvent) error { return nil }
func (m *mockIsolationStore) CreateSnapshot(_ context.Context, _ types.Snapshot) error { return nil }
func (m *mockIsolationStore) GetEdge(_ context.Context, _ types.Hash) (*types.Edge, error) { return nil, nil }
func (m *mockIsolationStore) GetSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) { return nil, nil }
func (m *mockIsolationStore) GetRepo(_ context.Context, _ types.Hash) (*types.Repo, error) { return nil, nil }
func (m *mockIsolationStore) NodesByName(_ context.Context, _ string) ([]types.Node, error) { return nil, nil }
func (m *mockIsolationStore) DanglingEdges(_ context.Context) ([]types.Edge, error) { return nil, nil }
func (m *mockIsolationStore) AllRepos(_ context.Context) ([]types.Repo, error) { return nil, nil }
func (m *mockIsolationStore) NodesByQualifiedName(_ context.Context, _ string) ([]types.Node, error) { return nil, nil }
func (m *mockIsolationStore) DeleteNodesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) { return 0, nil }
func (m *mockIsolationStore) DeleteEdgesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) { return 0, nil }
func (m *mockIsolationStore) DeleteEdge(_ context.Context, _ types.Hash) error { return nil }
func (m *mockIsolationStore) DeleteNodesByFile(_ context.Context, _ types.Hash) (int, error) { return 0, nil }
func (m *mockIsolationStore) DeleteEdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) { return nil, nil }
func (m *mockIsolationStore) EdgesBySourceFile(_ context.Context, _ types.Hash) ([]types.Edge, error) { return nil, nil }
func (m *mockIsolationStore) DeleteSnapshot(_ context.Context, _ types.Hash) error { return nil }
func (m *mockIsolationStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) { return nil, nil }
func (m *mockIsolationStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) { return nil, nil }
func (m *mockIsolationStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) { return nil, nil }
func (m *mockIsolationStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) { return nil, nil }
func (m *mockIsolationStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) { return nil, nil }
func (m *mockIsolationStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) { return nil, nil }
func (m *mockIsolationStore) FilesByRepo(_ context.Context, _ types.Hash) ([]types.File, error) { return nil, nil }
func (m *mockIsolationStore) FileByPath(_ context.Context, _ types.Hash, _ string) (*types.File, error) { return nil, nil }
func (m *mockIsolationStore) NodesByFilePath(_ context.Context, _ types.Hash, _ string) ([]types.Node, error) { return nil, nil }
func (m *mockIsolationStore) StaleNodesByFiles(_ context.Context, _ types.Hash, _ []string) ([]types.Node, error) { return nil, nil }
func (m *mockIsolationStore) PutNote(_ context.Context, _ types.Note) error { return nil }
func (m *mockIsolationStore) GetNote(_ context.Context, _ types.Hash, _ string) (*types.Note, error) { return nil, nil }
func (m *mockIsolationStore) GetNotes(_ context.Context, _ types.Hash) ([]types.Note, error) { return nil, nil }
func (m *mockIsolationStore) GetNotesByKey(_ context.Context, _ string) ([]types.Note, error) { return nil, nil }
func (m *mockIsolationStore) DeleteNote(_ context.Context, _ types.Hash, _ string) error { return nil }
func (m *mockIsolationStore) DeleteNotesByObject(_ context.Context, _ types.Hash) error { return nil }
func (m *mockIsolationStore) Close() error { return nil }

func hash(b byte) types.Hash {
	var h types.Hash
	h[0] = b
	return h
}

func TestIsolation_WellConnectedFile(t *testing.T) {
	// A file with many inbound edges from external nodes should score ~0.
	fileHash := hash(1)
	nodeHash := hash(10)
	externalNodes := []types.Hash{hash(20), hash(21), hash(22), hash(23), hash(24)}

	var edges []types.Edge
	for _, ext := range externalNodes {
		edges = append(edges, types.Edge{
			SourceHash: ext,
			TargetHash: nodeHash,
			EdgeType:   edgetype.Calls,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/well_connected.go.Func"}},
		},
		edges:    edges,
		allNodes: map[types.Hash]*types.Node{},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// With 5+ inbound edges and 0 outbound dangerous edges:
	// inbound_factor = 0.0, so score = 0.0
	if results[0].Score > 0.01 {
		t.Errorf("expected score ~0.0 for well-connected file, got %f", results[0].Score)
	}
}

func TestIsolation_IsolatedWithEnvReads(t *testing.T) {
	// An isolated file (0 inbound) with env reads should score high.
	fileHash := hash(1)
	nodeHash := hash(10)
	envNodeHash := hash(30)

	// 3 reads_env edges, no inbound from outside.
	var edges []types.Edge
	for i := 0; i < 3; i++ {
		edges = append(edges, types.Edge{
			SourceHash: nodeHash,
			TargetHash: envNodeHash,
			EdgeType:   edgetype.ReadsEnv,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/isolated.go.ReadSecrets"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash: {NodeHash: envNodeHash, QualifiedName: "SECRET_KEY"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// inbound_factor = 1.0 (0 inbound), outbound_factor = 3/5 = 0.6
	// score = 1.0 * 0.6 * 1.0 = 0.6
	if results[0].Score < 0.4 || results[0].Score > 0.8 {
		t.Errorf("expected score ~0.6, got %f", results[0].Score)
	}
	if len(results[0].ReadsEnv) == 0 {
		t.Error("expected ReadsEnv to be populated")
	}
}

func TestIsolation_IsolatedWithProcessExec(t *testing.T) {
	// An isolated file with process execution edges should score high.
	fileHash := hash(1)
	nodeHash := hash(10)
	procNodeHash := hash(40)

	// 7 executes_process edges.
	var edges []types.Edge
	for i := 0; i < 7; i++ {
		edges = append(edges, types.Edge{
			SourceHash: nodeHash,
			TargetHash: procNodeHash,
			EdgeType:   edgetype.ExecutesProcess,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/malicious.go.Install"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			procNodeHash: {NodeHash: procNodeHash, QualifiedName: "curl"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// inbound_factor = 1.0, outbound_factor = 7/10 = 0.7, hook_factor = 1.5 (executes_process is hook)
	// score = 1.0 * 0.7 * 1.5 = 1.05 -> clamped to 1.0
	if results[0].Score < 0.7 {
		t.Errorf("expected high score for isolated file with process exec, got %f", results[0].Score)
	}
}

func TestIsolation_HookExecuted(t *testing.T) {
	// An isolated file with hook execution should approach 1.0.
	fileHash := hash(1)
	nodeHash := hash(10)
	procNodeHash := hash(40)

	// 10 executes_process edges (capped at 10).
	var edges []types.Edge
	for i := 0; i < 10; i++ {
		edges = append(edges, types.Edge{
			SourceHash: nodeHash,
			TargetHash: procNodeHash,
			EdgeType:   edgetype.ExecutesProcess,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/postinstall.go.Run"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			procNodeHash: {NodeHash: procNodeHash, QualifiedName: "bash"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// inbound_factor = 1.0, outbound_factor = 10/10 = 1.0, hook_factor = 1.5
	// score = 1.0 * 1.0 * 1.5 = 1.5 -> clamped to 1.0
	if results[0].Score != 1.0 {
		t.Errorf("expected score 1.0 for hook-executed isolated file, got %f", results[0].Score)
	}
}

func TestIsolation_Clamping(t *testing.T) {
	// Score should never exceed 1.0 even with extreme values.
	fileHash := hash(1)
	nodeHash := hash(10)
	procNodeHash := hash(40)

	// 20 executes_process edges (well over the cap of 10).
	var edges []types.Edge
	for i := 0; i < 20; i++ {
		edges = append(edges, types.Edge{
			SourceHash: nodeHash,
			TargetHash: procNodeHash,
			EdgeType:   edgetype.ExecutesProcess,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/extreme.go.Attack"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			procNodeHash: {NodeHash: procNodeHash, QualifiedName: "rm"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score > 1.0 {
		t.Errorf("score must not exceed 1.0, got %f", results[0].Score)
	}
}
