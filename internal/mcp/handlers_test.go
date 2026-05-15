package mcp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockGraphStore implements types.GraphStore for testing.
type mockGraphStore struct {
	nodes     map[types.Hash]*types.Node
	edges     map[types.Hash]*types.Edge
	snapshots map[types.Hash]*types.Snapshot
	repos     map[types.Hash]*types.Repo
	files     map[types.Hash]*types.File

	// Indexed data for query methods.
	nodesByName map[string][]types.Node
	edgesFrom   map[types.Hash][]types.Edge
	edgesTo     map[types.Hash][]types.Edge
	filesByRepo map[types.Hash][]types.File

	// Results for traversal methods.
	transitiveCallers []types.CallerResult
	transitiveCallees []types.CalleeResult
	blastRadiusResult *types.BlastRadiusResult
	snapshotDiffResult *types.DiffResult
	staleEdgesResult  []types.Edge
	latestSnapshot    *types.Snapshot
}

func newMockGraphStore() *mockGraphStore {
	return &mockGraphStore{
		nodes:       make(map[types.Hash]*types.Node),
		edges:       make(map[types.Hash]*types.Edge),
		snapshots:   make(map[types.Hash]*types.Snapshot),
		repos:       make(map[types.Hash]*types.Repo),
		files:       make(map[types.Hash]*types.File),
		nodesByName: make(map[string][]types.Node),
		edgesFrom:   make(map[types.Hash][]types.Edge),
		edgesTo:     make(map[types.Hash][]types.Edge),
		filesByRepo: make(map[types.Hash][]types.File),
	}
}

func (m *mockGraphStore) PutNode(_ context.Context, n types.Node) error {
	m.nodes[n.NodeHash] = &n
	return nil
}

func (m *mockGraphStore) PutEdge(_ context.Context, e types.Edge) error {
	m.edges[e.EdgeHash] = &e
	return nil
}

func (m *mockGraphStore) PutFile(_ context.Context, f types.File) error {
	m.files[f.FileHash] = &f
	return nil
}

func (m *mockGraphStore) PutRepo(_ context.Context, r types.Repo) error {
	m.repos[r.RepoHash] = &r
	return nil
}

func (m *mockGraphStore) RecordEdgeEvent(_ context.Context, _ types.EdgeEvent) error {
	return nil
}

func (m *mockGraphStore) CreateSnapshot(_ context.Context, s types.Snapshot) error {
	m.snapshots[s.SnapshotHash] = &s
	return nil
}

func (m *mockGraphStore) GetNode(_ context.Context, hash types.Hash) (*types.Node, error) {
	return m.nodes[hash], nil
}

func (m *mockGraphStore) GetEdge(_ context.Context, hash types.Hash) (*types.Edge, error) {
	return m.edges[hash], nil
}

func (m *mockGraphStore) GetSnapshot(_ context.Context, hash types.Hash) (*types.Snapshot, error) {
	return m.snapshots[hash], nil
}

func (m *mockGraphStore) GetRepo(_ context.Context, hash types.Hash) (*types.Repo, error) {
	return m.repos[hash], nil
}

func (m *mockGraphStore) NodesByName(_ context.Context, prefix string) ([]types.Node, error) {
	if nodes, ok := m.nodesByName[prefix]; ok {
		return nodes, nil
	}
	return nil, nil
}

func (m *mockGraphStore) EdgesFrom(_ context.Context, sourceHash types.Hash, _ string) ([]types.Edge, error) {
	return m.edgesFrom[sourceHash], nil
}

func (m *mockGraphStore) EdgesTo(_ context.Context, targetHash types.Hash, _ string) ([]types.Edge, error) {
	return m.edgesTo[targetHash], nil
}

func (m *mockGraphStore) TransitiveCallers(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CallerResult, error) {
	return m.transitiveCallers, nil
}

func (m *mockGraphStore) TransitiveCallees(_ context.Context, _ types.Hash, _ int, _ types.Hash) ([]types.CalleeResult, error) {
	return m.transitiveCallees, nil
}

func (m *mockGraphStore) BlastRadius(_ context.Context, _ types.Hash, _ types.Hash) (*types.BlastRadiusResult, error) {
	return m.blastRadiusResult, nil
}

func (m *mockGraphStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return m.snapshotDiffResult, nil
}

func (m *mockGraphStore) StaleEdges(_ context.Context, _ types.Hash) ([]types.Edge, error) {
	return m.staleEdgesResult, nil
}

func (m *mockGraphStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return m.latestSnapshot, nil
}

func (m *mockGraphStore) FilesByRepo(_ context.Context, repoHash types.Hash) ([]types.File, error) {
	return m.filesByRepo[repoHash], nil
}

func (m *mockGraphStore) FileByPath(_ context.Context, repoHash types.Hash, path string) (*types.File, error) {
	for _, f := range m.filesByRepo[repoHash] {
		if f.Path == path {
			return &f, nil
		}
	}
	return nil, nil
}

func (m *mockGraphStore) Close() error {
	return nil
}

// testHash creates a deterministic hash from a string for testing.
func testHash(s string) types.Hash {
	return types.NewHash([]byte(s))
}

// hashHex returns the hex string of a hash.
func hashHex(h types.Hash) string {
	return hex.EncodeToString(h[:])
}

// makeCallToolRequest creates a CallToolRequest with the given arguments.
func makeCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestNewServer_RegistersAllTools(t *testing.T) {
	store := newMockGraphStore()
	srv := NewServer(store)

	expected := []string{
		"index_repo",
		"cross_repo_callers",
		"graph_query",
		"repo_graph",
		"blast_radius",
		"trace_dataflow",
		"stale_edges",
		"snapshot_diff",
		"semantic_diff",
		"pr_impact",
		"ownership",
	}

	names := srv.ToolNames()
	if len(names) != len(expected) {
		t.Fatalf("expected %d tools, got %d", len(expected), len(names))
	}

	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, e := range expected {
		if !nameSet[e] {
			t.Errorf("missing tool: %s", e)
		}
	}
}

func TestHandleCrossRepoCallers_ReturnsCallers(t *testing.T) {
	store := newMockGraphStore()
	targetNode := types.Node{
		NodeHash:      testHash("target"),
		QualifiedName: "github.com/example/pkg.Foo",
		Kind:          "function",
	}
	callerNode := types.Node{
		NodeHash:      testHash("caller1"),
		QualifiedName: "github.com/example/other.Bar",
		Kind:          "function",
	}
	store.transitiveCallers = []types.CallerResult{
		{Node: callerNode, Depth: 1},
	}
	_ = targetNode // used for context

	srv := NewServer(store)
	req := makeCallToolRequest("cross_repo_callers", map[string]any{
		"target_hash": hashHex(testHash("target")),
		"max_depth":   float64(3),
	})

	result, err := srv.handleCrossRepoCallers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	// Verify the result contains caller data.
	text := result.Content[0].(mcp.TextContent).Text
	var callers []types.CallerResult
	if err := json.Unmarshal([]byte(text), &callers); err != nil {
		t.Fatalf("failed to unmarshal callers: %v", err)
	}
	if len(callers) != 1 {
		t.Fatalf("expected 1 caller, got %d", len(callers))
	}
	if callers[0].Depth != 1 {
		t.Errorf("expected depth 1, got %d", callers[0].Depth)
	}
}

func TestHandleBlastRadius_ReturnsGroupedResults(t *testing.T) {
	store := newMockGraphStore()
	store.blastRadiusResult = &types.BlastRadiusResult{
		Target: types.Node{
			NodeHash:      testHash("target"),
			QualifiedName: "github.com/example/pkg.Foo",
			Kind:          "function",
		},
		ByRepo: map[string][]types.CallerWithProvenance{
			"github.com/example/repo1": {
				{
					Caller: types.Node{
						NodeHash:      testHash("caller1"),
						QualifiedName: "github.com/example/repo1.Bar",
						Kind:          "function",
					},
					Depth:      1,
					Confidence: 1.0,
				},
			},
			"github.com/example/repo2": {
				{
					Caller: types.Node{
						NodeHash:      testHash("caller2"),
						QualifiedName: "github.com/example/repo2.Baz",
						Kind:          "function",
					},
					Depth:      2,
					Confidence: 0.9,
				},
			},
		},
		TotalCount: 2,
	}

	srv := NewServer(store)
	req := makeCallToolRequest("blast_radius", map[string]any{
		"target_hash": hashHex(testHash("target")),
	})

	result, err := srv.handleBlastRadius(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var br types.BlastRadiusResult
	if err := json.Unmarshal([]byte(text), &br); err != nil {
		t.Fatalf("failed to unmarshal blast radius: %v", err)
	}
	if br.TotalCount != 2 {
		t.Errorf("expected total count 2, got %d", br.TotalCount)
	}
	if len(br.ByRepo) != 2 {
		t.Errorf("expected 2 repos, got %d", len(br.ByRepo))
	}
}

func TestHandleGraphQuery_NodesByPrefix(t *testing.T) {
	store := newMockGraphStore()
	store.nodesByName["github.com/example"] = []types.Node{
		{
			NodeHash:      testHash("node1"),
			QualifiedName: "github.com/example/pkg.Foo",
			Kind:          "function",
		},
		{
			NodeHash:      testHash("node2"),
			QualifiedName: "github.com/example/pkg.Bar",
			Kind:          "type",
		},
	}

	srv := NewServer(store)
	req := makeCallToolRequest("graph_query", map[string]any{
		"prefix": "github.com/example",
	})

	result, err := srv.handleGraphQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var nodes []types.Node
	if err := json.Unmarshal([]byte(text), &nodes); err != nil {
		t.Fatalf("failed to unmarshal nodes: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestHandleSnapshotDiff_ReturnsChanges(t *testing.T) {
	store := newMockGraphStore()
	store.snapshotDiffResult = &types.DiffResult{
		OldSnapshot: testHash("old"),
		NewSnapshot: testHash("new"),
		NodesAdded: []types.Node{
			{
				NodeHash:      testHash("added-node"),
				QualifiedName: "github.com/example/pkg.NewFunc",
				Kind:          "function",
			},
		},
		NodesRemoved: []types.Node{
			{
				NodeHash:      testHash("removed-node"),
				QualifiedName: "github.com/example/pkg.OldFunc",
				Kind:          "function",
			},
		},
		EdgesAdded:   []types.Edge{{EdgeHash: testHash("added-edge")}},
		EdgesRemoved: []types.Edge{{EdgeHash: testHash("removed-edge")}},
	}

	srv := NewServer(store)
	req := makeCallToolRequest("snapshot_diff", map[string]any{
		"old_snapshot": hashHex(testHash("old")),
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handleSnapshotDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var diff types.DiffResult
	if err := json.Unmarshal([]byte(text), &diff); err != nil {
		t.Fatalf("failed to unmarshal diff: %v", err)
	}
	if len(diff.NodesAdded) != 1 {
		t.Errorf("expected 1 node added, got %d", len(diff.NodesAdded))
	}
	if len(diff.NodesRemoved) != 1 {
		t.Errorf("expected 1 node removed, got %d", len(diff.NodesRemoved))
	}
	if len(diff.EdgesAdded) != 1 {
		t.Errorf("expected 1 edge added, got %d", len(diff.EdgesAdded))
	}
	if len(diff.EdgesRemoved) != 1 {
		t.Errorf("expected 1 edge removed, got %d", len(diff.EdgesRemoved))
	}
}

func TestHandleStaleEdges_FindsStale(t *testing.T) {
	store := newMockGraphStore()
	store.staleEdgesResult = []types.Edge{
		{
			EdgeHash:   testHash("stale1"),
			SourceHash: testHash("src1"),
			TargetHash: testHash("tgt1"),
			EdgeType:   "calls",
			Confidence: 0.3,
		},
		{
			EdgeHash:   testHash("stale2"),
			SourceHash: testHash("src2"),
			TargetHash: testHash("tgt2"),
			EdgeType:   "imports",
			Confidence: 0.1,
		},
	}

	srv := NewServer(store)
	req := makeCallToolRequest("stale_edges", map[string]any{
		"snapshot_hash": hashHex(testHash("snapshot")),
	})

	result, err := srv.handleStaleEdges(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var edges []types.Edge
	if err := json.Unmarshal([]byte(text), &edges); err != nil {
		t.Fatalf("failed to unmarshal edges: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 stale edges, got %d", len(edges))
	}
	if edges[0].EdgeType != "calls" {
		t.Errorf("expected edge type 'calls', got %q", edges[0].EdgeType)
	}
}
