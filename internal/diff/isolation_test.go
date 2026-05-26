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

func TestIsolation_EmptyChangedFiles(t *testing.T) {
	store := &mockIsolationStore{
		nodes:    map[types.Hash][]types.Node{},
		edges:    nil,
		allNodes: map[types.Hash]*types.Node{},
	}

	results, err := ComputeIsolation(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for nil changedFiles, got %v", results)
	}

	results, err = ComputeIsolation(context.Background(), store, []types.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if results != nil {
		t.Errorf("expected nil results for empty changedFiles, got %v", results)
	}
}

func TestIsolation_ZeroInboundZeroOutbound(t *testing.T) {
	// A file with no inbound and no dangerous outbound edges.
	// inbound_factor = 1.0, outbound_factor = 0.0, score = 0.0.
	// This is an isolated file that does nothing dangerous: low risk.
	fileHash := hash(1)
	nodeHash := hash(10)

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/benign.go.Helper"}},
		},
		edges:    nil,
		allNodes: map[types.Hash]*types.Node{},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 0.0 {
		t.Errorf("expected score 0.0 for isolated benign file, got %f", results[0].Score)
	}
	if results[0].InboundEdges != 0 {
		t.Errorf("expected 0 inbound edges, got %d", results[0].InboundEdges)
	}
	if results[0].OutboundEdges != 0 {
		t.Errorf("expected 0 outbound edges, got %d", results[0].OutboundEdges)
	}
}

func TestIsolation_ManyInboundReducesScore(t *testing.T) {
	// 10+ inbound edges should reduce inbound_factor to its minimum (0.3).
	// With 2 dangerous outbound edges: score = 0.3 * (2/5) * 1.0 = 0.12.
	fileHash := hash(1)
	nodeHash := hash(10)
	envNodeHash := hash(30)

	var edges []types.Edge
	// 10 inbound edges from external nodes.
	for i := byte(0); i < 10; i++ {
		edges = append(edges, types.Edge{
			SourceHash: hash(100 + i),
			TargetHash: nodeHash,
			EdgeType:   edgetype.Calls,
		})
	}
	// 2 reads_env outbound edges.
	for i := 0; i < 2; i++ {
		edges = append(edges, types.Edge{
			SourceHash: nodeHash,
			TargetHash: envNodeHash,
			EdgeType:   edgetype.ReadsEnv,
		})
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/popular.go.ReadConfig"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash: {NodeHash: envNodeHash, QualifiedName: "DATABASE_URL"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// inbound_factor = 1.0 - (10/10 * 0.7) = 0.3
	// outbound_factor = 2/5 = 0.4
	// score = 0.3 * 0.4 * 1.0 = 0.12
	expected := 0.12
	if results[0].Score < expected-0.02 || results[0].Score > expected+0.02 {
		t.Errorf("expected score ~%.2f, got %f", expected, results[0].Score)
	}
	if results[0].InboundEdges != 10 {
		t.Errorf("expected 10 inbound edges, got %d", results[0].InboundEdges)
	}
}

func TestIsolation_ConsumesEndpointNotDangerous(t *testing.T) {
	// consumes_endpoint edges are not counted as dangerous outbound edges
	// by the current implementation (only reads_env and executes_process are).
	fileHash := hash(1)
	nodeHash := hash(10)
	endpointHash := hash(50)

	edges := []types.Edge{
		{
			SourceHash: nodeHash,
			TargetHash: endpointHash,
			EdgeType:   edgetype.ConsumesEndpoint,
		},
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/client.go.FetchData"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			endpointHash: {NodeHash: endpointHash, QualifiedName: "https://api.example.com"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// outbound_factor = 0 because consumes_endpoint is not in the dangerous set.
	if results[0].Score != 0.0 {
		t.Errorf("expected score 0.0 (consumes_endpoint not dangerous), got %f", results[0].Score)
	}
	if results[0].OutboundEdges != 0 {
		t.Errorf("expected 0 dangerous outbound edges, got %d", results[0].OutboundEdges)
	}
}

func TestIsolation_HookFactorBoostsScore(t *testing.T) {
	// Verify the hook factor (1.5x) amplifies scores.
	// Compare two identical setups: one with reads_env (no hook), one with executes_process (hook).
	fileHash1 := hash(1)
	nodeHash1 := hash(10)
	envNodeHash := hash(30)

	fileHash2 := hash(2)
	nodeHash2 := hash(11)
	procNodeHash := hash(40)

	edges := []types.Edge{
		// File 1: 1 reads_env edge (not a hook).
		{SourceHash: nodeHash1, TargetHash: envNodeHash, EdgeType: edgetype.ReadsEnv},
		// File 2: 1 executes_process edge (is a hook).
		{SourceHash: nodeHash2, TargetHash: procNodeHash, EdgeType: edgetype.ExecutesProcess},
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash1: {{NodeHash: nodeHash1, QualifiedName: "pkg/env_reader.go.Read"}},
			fileHash2: {{NodeHash: nodeHash2, QualifiedName: "pkg/installer.go.PostInstall"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash:  {NodeHash: envNodeHash, QualifiedName: "API_KEY"},
			procNodeHash: {NodeHash: procNodeHash, QualifiedName: "npm"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash1, fileHash2})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	envScore := results[0].Score
	hookScore := results[1].Score

	// Both have inbound_factor=1.0, outbound_factor=1/5=0.2.
	// File 1 (env): score = 1.0 * 0.2 * 1.0 = 0.2
	// File 2 (hook): score = 1.0 * 0.2 * 1.5 = 0.3
	if envScore < 0.15 || envScore > 0.25 {
		t.Errorf("expected env score ~0.2, got %f", envScore)
	}
	if hookScore < 0.25 || hookScore > 0.35 {
		t.Errorf("expected hook score ~0.3, got %f", hookScore)
	}
	if !results[1].HookExecuted {
		t.Error("expected HookExecuted=true for executes_process file")
	}
	if results[0].HookExecuted {
		t.Error("expected HookExecuted=false for reads_env-only file")
	}
}

func TestIsolation_ScoreNeverNegative(t *testing.T) {
	// Even with extreme inbound edges, score should not go below 0.0.
	fileHash := hash(1)
	nodeHash := hash(10)
	envNodeHash := hash(30)

	var edges []types.Edge
	// 50 inbound edges (well over the cap of 10).
	for i := byte(0); i < 50; i++ {
		edges = append(edges, types.Edge{
			SourceHash: hash(100 + i),
			TargetHash: nodeHash,
			EdgeType:   edgetype.Calls,
		})
	}
	// 1 dangerous outbound edge.
	edges = append(edges, types.Edge{
		SourceHash: nodeHash,
		TargetHash: envNodeHash,
		EdgeType:   edgetype.ReadsEnv,
	})

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/core.go.Init"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash: {NodeHash: envNodeHash, QualifiedName: "HOME"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score < 0.0 {
		t.Errorf("score must not be negative, got %f", results[0].Score)
	}
	// inbound capped at 10, so inbound_factor = 0.3; outbound_factor = 0.2
	// score = 0.3 * 0.2 = 0.06
	if results[0].Score > 0.1 {
		t.Errorf("expected low score for extremely well-connected file, got %f", results[0].Score)
	}
}

func TestIsolation_FileWithNoNodes(t *testing.T) {
	// A changed file hash that has no nodes in the store.
	fileHash := hash(1)

	store := &mockIsolationStore{
		nodes:    map[types.Hash][]types.Node{fileHash: {}},
		edges:    nil,
		allNodes: map[types.Hash]*types.Node{},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Score != 0.0 {
		t.Errorf("expected score 0.0 for file with no nodes, got %f", results[0].Score)
	}
}

func TestIsolation_MixedDangerousEdges(t *testing.T) {
	// A file with both reads_env and executes_process edges.
	fileHash := hash(1)
	nodeHash := hash(10)
	envNodeHash := hash(30)
	procNodeHash := hash(40)

	edges := []types.Edge{
		{SourceHash: nodeHash, TargetHash: envNodeHash, EdgeType: edgetype.ReadsEnv},
		{SourceHash: nodeHash, TargetHash: envNodeHash, EdgeType: edgetype.ReadsEnv},
		{SourceHash: nodeHash, TargetHash: procNodeHash, EdgeType: edgetype.ExecutesProcess},
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash: {{NodeHash: nodeHash, QualifiedName: "pkg/mixed.go.Run"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash:  {NodeHash: envNodeHash, QualifiedName: "TOKEN"},
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

	// 3 dangerous outbound edges, 0 inbound.
	// inbound_factor = 1.0, outbound_factor = 3/5 = 0.6, hook_factor = 1.5 (executes_process).
	// score = 1.0 * 0.6 * 1.5 = 0.9
	if results[0].Score < 0.8 || results[0].Score > 1.0 {
		t.Errorf("expected score ~0.9, got %f", results[0].Score)
	}
	if len(results[0].ReadsEnv) != 2 {
		t.Errorf("expected 2 ReadsEnv entries, got %d", len(results[0].ReadsEnv))
	}
	if len(results[0].ExecutesProc) != 1 {
		t.Errorf("expected 1 ExecutesProc entry, got %d", len(results[0].ExecutesProc))
	}
	if !results[0].HookExecuted {
		t.Error("expected HookExecuted=true when executes_process is present")
	}
}

func TestIsolation_InboundFromChangedFilesIgnored(t *testing.T) {
	// Inbound edges from OTHER changed files should not count as inbound.
	fileHash1 := hash(1)
	nodeHash1 := hash(10)
	fileHash2 := hash(2)
	nodeHash2 := hash(11)
	envNodeHash := hash(30)

	edges := []types.Edge{
		// nodeHash2 (in changed file 2) calls nodeHash1 (in changed file 1).
		{SourceHash: nodeHash2, TargetHash: nodeHash1, EdgeType: edgetype.Calls},
		// nodeHash1 reads env.
		{SourceHash: nodeHash1, TargetHash: envNodeHash, EdgeType: edgetype.ReadsEnv},
	}

	store := &mockIsolationStore{
		nodes: map[types.Hash][]types.Node{
			fileHash1: {{NodeHash: nodeHash1, QualifiedName: "pkg/a.go.Func"}},
			fileHash2: {{NodeHash: nodeHash2, QualifiedName: "pkg/b.go.Caller"}},
		},
		edges: edges,
		allNodes: map[types.Hash]*types.Node{
			envNodeHash: {NodeHash: envNodeHash, QualifiedName: "SECRET"},
		},
	}

	results, err := ComputeIsolation(context.Background(), store, []types.Hash{fileHash1, fileHash2})
	if err != nil {
		t.Fatal(err)
	}

	// Find fileHash1's result.
	var file1Result IsolationResult
	for _, r := range results {
		if r.File == "pkg/a.go.Func" {
			file1Result = r
			break
		}
	}

	// The inbound edge from nodeHash2 should be ignored since it's in changedFiles.
	if file1Result.InboundEdges != 0 {
		t.Errorf("expected 0 inbound edges (edge from changed file ignored), got %d", file1Result.InboundEdges)
	}
	// inbound_factor=1.0, outbound_factor=1/5=0.2, score=0.2
	if file1Result.Score < 0.15 || file1Result.Score > 0.25 {
		t.Errorf("expected score ~0.2, got %f", file1Result.Score)
	}
}
