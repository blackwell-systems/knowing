package mcp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/testutil"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockGraphStore embeds *testutil.MockGraphStore for no-op defaults on methods
// that tests don't override. Local fields provide the data maps that tests
// populate directly. Methods that read from local fields override the embedded
// implementations.
type mockGraphStore struct {
	*testutil.MockGraphStore

	// Data maps (same as before, for backward compat with other test files in this package).
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
	transitiveCallers  []types.CallerResult
	transitiveCallees  []types.CalleeResult
	blastRadiusResult  *types.BlastRadiusResult
	snapshotDiffResult *types.DiffResult
	staleEdgesResult   []types.Edge
	latestSnapshot     *types.Snapshot
}

func newMockGraphStore() *mockGraphStore {
	return &mockGraphStore{
		MockGraphStore: testutil.NewMockGraphStore(),
		nodes:          make(map[types.Hash]*types.Node),
		edges:          make(map[types.Hash]*types.Edge),
		snapshots:      make(map[types.Hash]*types.Snapshot),
		repos:          make(map[types.Hash]*types.Repo),
		files:          make(map[types.Hash]*types.File),
		nodesByName:    make(map[string][]types.Node),
		edgesFrom:      make(map[types.Hash][]types.Edge),
		edgesTo:        make(map[types.Hash][]types.Edge),
		filesByRepo:    make(map[types.Hash][]types.File),
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
		"ownership_query",
		"runtime_traffic",
		"dead_routes",
		"trace_stats",
		"context_for_task",
		"context_for_files",
		"context_for_pr",
		"explain_symbol",
		"feedback",
		"test_scope",
		"flow_between",
		"plan_turn",
		"communities",
		"prove",
		"prove_absent",
		"fsck",
		"untrack_repo",
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

func TestHandleIndexRepo_WithIndexFunc(t *testing.T) {
	store := newMockGraphStore()
	srv := NewServer(store)

	var calledURL, calledPath, calledCommit string
	SetIndexFunc(func(_ context.Context, repoURL, repoPath, commitHash string) error {
		calledURL = repoURL
		calledPath = repoPath
		calledCommit = commitHash
		return nil
	})
	defer SetIndexFunc(nil) // clean up global state

	req := makeCallToolRequest("index_repo", map[string]any{
		"repo_url":    "https://github.com/example/repo",
		"repo_path":   "/tmp/repo",
		"commit_hash": "abc123",
	})

	result, err := srv.handleIndexRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
	if calledURL != "https://github.com/example/repo" {
		t.Errorf("expected repo URL to be passed, got %q", calledURL)
	}
	if calledPath != "/tmp/repo" {
		t.Errorf("expected repo path to be passed, got %q", calledPath)
	}
	if calledCommit != "abc123" {
		t.Errorf("expected commit hash to be passed, got %q", calledCommit)
	}
}

func TestHandleIndexRepo_NoIndexFunc(t *testing.T) {
	store := newMockGraphStore()
	srv := NewServer(store)

	// Ensure indexFunc is nil.
	SetIndexFunc(nil)

	req := makeCallToolRequest("index_repo", map[string]any{
		"repo_url":  "https://github.com/example/repo",
		"repo_path": "/tmp/repo",
	})

	result, err := srv.handleIndexRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when indexFunc is nil")
	}
}

func TestHandleOwnership_WithData(t *testing.T) {
	store := newMockGraphStore()

	repoHash := testHash("repo1")
	store.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	fileHash := testHash("file1")
	store.filesByRepo[repoHash] = []types.File{
		{FileHash: fileHash, RepoHash: repoHash, Path: "pkg/main.go"},
	}

	node1 := types.Node{
		NodeHash:      testHash("node1"),
		FileHash:      fileHash,
		QualifiedName: "https://github.com/example/repo://pkg.Foo",
		Kind:          "function",
	}
	node2 := types.Node{
		NodeHash:      testHash("node2"),
		FileHash:      fileHash,
		QualifiedName: "https://github.com/example/repo://pkg.Bar",
		Kind:          "type",
	}
	store.nodesByName["https://github.com/example/repo"] = []types.Node{node1, node2}

	srv := NewServer(store)
	req := makeCallToolRequest("ownership", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleOwnership(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var ownership []struct {
		File  types.File   `json:"file"`
		Nodes []types.Node `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(text), &ownership); err != nil {
		t.Fatalf("failed to unmarshal ownership: %v", err)
	}
	if len(ownership) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(ownership))
	}
	if ownership[0].File.Path != "pkg/main.go" {
		t.Errorf("expected file path pkg/main.go, got %q", ownership[0].File.Path)
	}
	if len(ownership[0].Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(ownership[0].Nodes))
	}
}

func TestHandleStaleEdges_FindsStale(t *testing.T) {
	store := newMockGraphStore()
	store.staleEdgesResult =[]types.Edge{
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

// --- Runtime trace query handler tests ---

// newTestSQLiteServer creates an MCP server backed by a real in-memory SQLiteStore.
func newTestSQLiteServer(t *testing.T) (*Server, *store.SQLiteStore) {
	t.Helper()
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	srv := NewServer(ss)
	return srv, ss
}

func TestHandleRuntimeTraffic(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Insert a node as a target.
	targetHash := testHash("handler-node")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      targetHash,
		QualifiedName: "github.com/example/api.GetUser",
		Kind:          "function",
	}); err != nil {
		t.Fatal(err)
	}

	// Insert a route symbol mapping.
	if err := ss.PutRouteSymbol(ctx, "user-service", "GET /users/:id", targetHash, "http_route"); err != nil {
		t.Fatal(err)
	}

	// Insert a runtime edge targeting that node.
	sourceHash := testHash("caller-node")
	edgeHash := testHash("runtime-edge-1")
	if err := ss.PutEdge(ctx, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "otel_trace",
	}); err != nil {
		t.Fatal(err)
	}
	// Update observation count so it looks like a runtime edge.
	now := time.Now().Unix()
	if err := ss.UpdateObservation(ctx, edgeHash, 42, now, 0.9); err != nil {
		t.Fatal(err)
	}

	req := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name": "user-service",
	})

	result, err := srv.handleRuntimeTraffic(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var edges []types.Edge
	if err := json.Unmarshal([]byte(text), &edges); err != nil {
		t.Fatalf("failed to unmarshal edges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].ObservationCount != 42 {
		t.Errorf("expected observation_count 42, got %d", edges[0].ObservationCount)
	}
}

func TestHandleDeadRoutes(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Insert a route symbol with no matching runtime edges (dead route).
	deadTarget := testHash("dead-handler")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      deadTarget,
		QualifiedName: "github.com/example/api.OldEndpoint",
		Kind:          "function",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "api-service", "DELETE /legacy", deadTarget, "http_route"); err != nil {
		t.Fatal(err)
	}

	req := makeCallToolRequest("dead_routes", map[string]any{
		"stale_days": float64(30),
	})

	result, err := srv.handleDeadRoutes(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var routes []store.RouteSymbolRow
	if err := json.Unmarshal([]byte(text), &routes); err != nil {
		t.Fatalf("failed to unmarshal routes: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 dead route, got %d", len(routes))
	}
	if routes[0].ServiceName != "api-service" {
		t.Errorf("expected service_name 'api-service', got %q", routes[0].ServiceName)
	}
	if routes[0].RoutePattern != "DELETE /legacy" {
		t.Errorf("expected route_pattern 'DELETE /legacy', got %q", routes[0].RoutePattern)
	}
}

func TestHandleTraceStats(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Insert runtime edges with different observation ages.
	now := time.Now().Unix()
	for i, lastObs := range []int64{
		now,                  // active (within 7 days)
		now - 40*86400,       // stale (30+ days)
		now - 100*86400,      // GC eligible (90+ days)
	} {
		edgeHash := testHash(fmt.Sprintf("stats-edge-%d", i))
		if err := ss.PutEdge(ctx, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: testHash(fmt.Sprintf("src-%d", i)),
			TargetHash: testHash(fmt.Sprintf("tgt-%d", i)),
			EdgeType:   "calls",
			Confidence: 0.8,
			Provenance: "otel_trace",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.UpdateObservation(ctx, edgeHash, i+1, lastObs, 0.8); err != nil {
			t.Fatal(err)
		}
	}

	req := makeCallToolRequest("trace_stats", map[string]any{})

	result, err := srv.handleTraceStats(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var stats store.RuntimeStatsRow
	if err := json.Unmarshal([]byte(text), &stats); err != nil {
		t.Fatalf("failed to unmarshal stats: %v", err)
	}
	if stats.TotalEdges != 3 {
		t.Errorf("expected 3 total edges, got %d", stats.TotalEdges)
	}
	if stats.ActiveEdges != 1 {
		t.Errorf("expected 1 active edge, got %d", stats.ActiveEdges)
	}
	if stats.StaleEdges != 2 {
		t.Errorf("expected 2 stale edges, got %d", stats.StaleEdges)
	}
	if stats.GCEligible != 1 {
		t.Errorf("expected 1 GC eligible edge, got %d", stats.GCEligible)
	}
}

func TestHandleSemanticDiff_ReturnsEnrichedResult(t *testing.T) {
	s := newMockGraphStore()

	addedNode := types.Node{
		NodeHash:      testHash("added-node"),
		QualifiedName: "github.com/example/pkg.NewFunc",
		Kind:          "function",
		Line:          42,
		Signature:     "func NewFunc()",
	}
	removedNode := types.Node{
		NodeHash:      testHash("removed-node"),
		QualifiedName: "github.com/example/pkg.OldFunc",
		Kind:          "function",
		Line:          10,
	}
	addedEdge := types.Edge{
		EdgeHash:   testHash("added-edge"),
		SourceHash: testHash("added-node"),
		TargetHash: testHash("some-target"),
		EdgeType:   "calls",
		Confidence: 0.95,
	}

	s.snapshotDiffResult = &types.DiffResult{
		OldSnapshot:  testHash("old"),
		NewSnapshot:  testHash("new"),
		NodesAdded:   []types.Node{addedNode},
		NodesRemoved: []types.Node{removedNode},
		EdgesAdded:   []types.Edge{addedEdge},
		EdgesRemoved: []types.Edge{},
	}

	// Populate nodes so GetNode resolves them for edge enrichment.
	s.nodes[addedNode.NodeHash] = &addedNode
	s.nodes[removedNode.NodeHash] = &removedNode

	srv := NewServer(s)
	req := makeCallToolRequest("semantic_diff", map[string]any{
		"old_snapshot": hashHex(testHash("old")),
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handleSemanticDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text

	// Unmarshal into a generic map to verify enriched fields.
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Verify key fields exist.
	for _, key := range []string{"nodes_added", "edges_added", "summary"} {
		if _, ok := out[key]; !ok {
			t.Errorf("missing expected field %q in result", key)
		}
	}

	// Verify summary has structured counts.
	var summary map[string]interface{}
	if err := json.Unmarshal(out["summary"], &summary); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}
	if v, ok := summary["nodes_added"].(float64); !ok || int(v) != 1 {
		t.Errorf("expected summary.nodes_added=1, got %v", summary["nodes_added"])
	}

	// Verify nodes_added has enriched data.
	var nodesAdded []map[string]interface{}
	if err := json.Unmarshal(out["nodes_added"], &nodesAdded); err != nil {
		t.Fatalf("failed to unmarshal nodes_added: %v", err)
	}
	if len(nodesAdded) != 1 {
		t.Fatalf("expected 1 node added, got %d", len(nodesAdded))
	}
	if nodesAdded[0]["qualified_name"] != "github.com/example/pkg.NewFunc" {
		t.Errorf("expected qualified_name, got %v", nodesAdded[0]["qualified_name"])
	}
}

func TestHandlePRImpact_ReturnsImpactAnalysis(t *testing.T) {
	s := newMockGraphStore()

	removedNode := types.Node{
		NodeHash:      testHash("removed-node"),
		QualifiedName: "github.com/example/pkg.OldFunc",
		Kind:          "function",
		Line:          10,
	}
	callerNode := types.Node{
		NodeHash:      testHash("caller-node"),
		QualifiedName: "github.com/example/pkg.Caller",
		Kind:          "function",
	}

	s.snapshotDiffResult = &types.DiffResult{
		OldSnapshot:  testHash("old"),
		NewSnapshot:  testHash("new"),
		NodesAdded:   []types.Node{},
		NodesRemoved: []types.Node{removedNode},
		EdgesAdded:   []types.Edge{},
		EdgesRemoved: []types.Edge{},
	}

	// Populate nodes for GetNode calls.
	s.nodes[removedNode.NodeHash] = &removedNode
	s.nodes[callerNode.NodeHash] = &callerNode

	// Set blast radius result for the removed node.
	s.blastRadiusResult = &types.BlastRadiusResult{
		Target: removedNode,
		ByRepo: map[string][]types.CallerWithProvenance{
			"github.com/example/repo": {
				{
					Caller:     callerNode,
					Depth:      1,
					Confidence: 1.0,
				},
			},
		},
		TotalCount: 1,
	}

	srv := NewServer(s)
	req := makeCallToolRequest("pr_impact", map[string]any{
		"old_snapshot": hashHex(testHash("old")),
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handlePRImpact(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text

	// Unmarshal into a generic map to verify key fields.
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	for _, key := range []string{"changed_symbols", "summary"} {
		if _, ok := out[key]; !ok {
			t.Errorf("missing expected field %q in result", key)
		}
	}

	// Verify summary has risk_level and counts.
	var summary map[string]interface{}
	if err := json.Unmarshal(out["summary"], &summary); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}
	if v, ok := summary["total_symbols_changed"].(float64); !ok || int(v) != 1 {
		t.Errorf("expected total_symbols_changed=1, got %v", summary["total_symbols_changed"])
	}
	if _, ok := summary["risk_level"]; !ok {
		t.Error("expected risk_level in summary")
	}

	// Verify changed_symbols has the removed node with callers.
	var symbols []map[string]interface{}
	if err := json.Unmarshal(out["changed_symbols"], &symbols); err != nil {
		t.Fatalf("failed to unmarshal changed_symbols: %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("expected 1 changed symbol, got %d", len(symbols))
	}
	if symbols[0]["change_type"] != "removed" {
		t.Errorf("expected change_type=removed, got %v", symbols[0]["change_type"])
	}
}

// --- Integration tests using real SQLite ---

// TestHandleSemanticDiff_Integration creates a real SQLite store with edge events
// and two snapshots, calls the semantic_diff handler, and verifies the response
// contains enriched node names (not just hashes).
func TestHandleSemanticDiff_Integration(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Set up a repo, file, and two nodes.
	repoHash := testHash("integ-repo")
	if err := ss.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://example.com/integ",
	}); err != nil {
		t.Fatal(err)
	}

	fileHash := testHash("integ-file")
	if err := ss.PutFile(ctx, types.File{
		FileHash:    fileHash,
		RepoHash:    repoHash,
		Path:        "service.go",
		ContentHash: testHash("content-1"),
	}); err != nil {
		t.Fatal(err)
	}

	nodeA := types.Node{
		NodeHash:      testHash("node-alpha"),
		FileHash:      fileHash,
		QualifiedName: "pkg.Alpha",
		Kind:          "function",
		Line:          10,
		Signature:     "func Alpha()",
	}
	nodeB := types.Node{
		NodeHash:      testHash("node-beta"),
		FileHash:      fileHash,
		QualifiedName: "pkg.Beta",
		Kind:          "function",
		Line:          20,
		Signature:     "func Beta()",
	}
	for _, n := range []types.Node{nodeA, nodeB} {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatalf("PutNode %s: %v", n.QualifiedName, err)
		}
	}

	// Create an edge: Alpha calls Beta.
	edgeAB := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(nodeA.NodeHash, nodeB.NodeHash, "calls", "ast_resolved"),
		SourceHash: nodeA.NodeHash,
		TargetHash: nodeB.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := ss.PutEdge(ctx, edgeAB); err != nil {
		t.Fatal(err)
	}

	// Create snapshots and edge events.
	oldSnap := testHash("integ-old-snap")
	newSnap := testHash("integ-new-snap")

	for _, snap := range []types.Snapshot{
		{SnapshotHash: oldSnap, RepoHash: repoHash, CommitHash: "c1", Timestamp: time.Now().Unix()},
		{SnapshotHash: newSnap, RepoHash: repoHash, CommitHash: "c2", Timestamp: time.Now().Unix()},
	} {
		if err := ss.CreateSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}

	// Record the edge as added in the new snapshot.
	if err := ss.RecordEdgeEvent(ctx, types.EdgeEvent{
		EdgeHash:     edgeAB.EdgeHash,
		EventType:    "added",
		SnapshotHash: newSnap,
		SourceCommit: "c2",
		IndexerVer:   "v1",
		Timestamp:    time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	// Call the semantic_diff handler.
	req := makeCallToolRequest("semantic_diff", map[string]any{
		"old_snapshot": hashHex(oldSnap),
		"new_snapshot": hashHex(newSnap),
	})

	result, err := srv.handleSemanticDiff(ctx, req)
	if err != nil {
		t.Fatalf("handleSemanticDiff: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text

	// Parse the response and verify enriched names.
	var diffResult map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &diffResult); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var edgesAdded []map[string]interface{}
	if err := json.Unmarshal(diffResult["edges_added"], &edgesAdded); err != nil {
		t.Fatalf("unmarshal edges_added: %v", err)
	}
	if len(edgesAdded) != 1 {
		t.Fatalf("expected 1 edge added, got %d", len(edgesAdded))
	}

	// The enriched edge should contain the qualified names, not raw hashes.
	if edgesAdded[0]["source_name"] != "pkg.Alpha" {
		t.Errorf("expected source_name=pkg.Alpha, got %v", edgesAdded[0]["source_name"])
	}
	if edgesAdded[0]["target_name"] != "pkg.Beta" {
		t.Errorf("expected target_name=pkg.Beta, got %v", edgesAdded[0]["target_name"])
	}

	// Verify the modified node appears (Alpha had edges change).
	var nodesModified []map[string]interface{}
	if err := json.Unmarshal(diffResult["nodes_modified"], &nodesModified); err != nil {
		t.Fatalf("unmarshal nodes_modified: %v", err)
	}
	if len(nodesModified) != 1 {
		t.Fatalf("expected 1 modified node, got %d", len(nodesModified))
	}
	if nodesModified[0]["qualified_name"] != "pkg.Alpha" {
		t.Errorf("expected modified node pkg.Alpha, got %v", nodesModified[0]["qualified_name"])
	}
}

// TestHandlePRImpact_Integration creates a real SQLite store with a call chain,
// simulates a change, and verifies the PR impact handler returns blast radius data.
func TestHandlePRImpact_Integration(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	repoHash := testHash("impact-repo")
	if err := ss.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://example.com/impact",
	}); err != nil {
		t.Fatal(err)
	}

	fileHash := testHash("impact-file")
	if err := ss.PutFile(ctx, types.File{
		FileHash:    fileHash,
		RepoHash:    repoHash,
		Path:        "handler.go",
		ContentHash: testHash("content-impact"),
	}); err != nil {
		t.Fatal(err)
	}

	// Build a call chain: Caller -> Target -> Helper.
	caller := types.Node{
		NodeHash:      testHash("impact-caller"),
		FileHash:      fileHash,
		QualifiedName: "pkg.Caller",
		Kind:          "function",
		Line:          5,
	}
	target := types.Node{
		NodeHash:      testHash("impact-target"),
		FileHash:      fileHash,
		QualifiedName: "pkg.Target",
		Kind:          "function",
		Line:          15,
	}
	helper := types.Node{
		NodeHash:      testHash("impact-helper"),
		FileHash:      fileHash,
		QualifiedName: "pkg.Helper",
		Kind:          "function",
		Line:          25,
	}
	for _, n := range []types.Node{caller, target, helper} {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Caller -> Target.
	edgeCT := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(caller.NodeHash, target.NodeHash, "calls", "ast_resolved"),
		SourceHash: caller.NodeHash,
		TargetHash: target.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	// Target -> Helper.
	edgeTH := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(target.NodeHash, helper.NodeHash, "calls", "ast_resolved"),
		SourceHash: target.NodeHash,
		TargetHash: helper.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	for _, e := range []types.Edge{edgeCT, edgeTH} {
		if err := ss.PutEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	// Create snapshots.
	oldSnap := testHash("impact-old")
	newSnap := testHash("impact-new")
	for _, snap := range []types.Snapshot{
		{SnapshotHash: oldSnap, RepoHash: repoHash, CommitHash: "c1", Timestamp: time.Now().Unix()},
		{SnapshotHash: newSnap, RepoHash: repoHash, CommitHash: "c2", Timestamp: time.Now().Unix()},
	} {
		if err := ss.CreateSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate: Target -> Helper edge was added in new snapshot.
	if err := ss.RecordEdgeEvent(ctx, types.EdgeEvent{
		EdgeHash:     edgeTH.EdgeHash,
		EventType:    "added",
		SnapshotHash: newSnap,
		SourceCommit: "c2",
		IndexerVer:   "v1",
		Timestamp:    time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	// Call the pr_impact handler.
	req := makeCallToolRequest("pr_impact", map[string]any{
		"old_snapshot": hashHex(oldSnap),
		"new_snapshot": hashHex(newSnap),
	})

	result, err := srv.handlePRImpact(ctx, req)
	if err != nil {
		t.Fatalf("handlePRImpact: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text

	var impact map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &impact); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify changed_symbols exists and has at least one entry.
	var symbols []map[string]interface{}
	if err := json.Unmarshal(impact["changed_symbols"], &symbols); err != nil {
		t.Fatalf("unmarshal changed_symbols: %v", err)
	}
	if len(symbols) < 1 {
		t.Fatal("expected at least 1 changed symbol")
	}

	// Target should be modified (its outgoing edge was added).
	foundTarget := false
	for _, sym := range symbols {
		symData := sym["symbol"].(map[string]interface{})
		if symData["qualified_name"] == "pkg.Target" && sym["change_type"] == "modified" {
			foundTarget = true
			// Target has a caller (Caller), so caller_count should be >= 1.
			callerCount := int(sym["caller_count"].(float64))
			if callerCount < 1 {
				t.Errorf("expected caller_count >= 1 for Target, got %d", callerCount)
			}
			break
		}
	}
	if !foundTarget {
		t.Errorf("expected pkg.Target as modified symbol, got: %v", symbols)
	}

	// Verify summary has risk_level.
	var summary map[string]interface{}
	if err := json.Unmarshal(impact["summary"], &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if _, ok := summary["risk_level"]; !ok {
		t.Error("expected risk_level in summary")
	}
	if summary["total_symbols_changed"].(float64) < 1 {
		t.Errorf("expected total_symbols_changed >= 1, got %v", summary["total_symbols_changed"])
	}

	// Verify affected_edges contains the added edge.
	var affectedEdges []map[string]interface{}
	if err := json.Unmarshal(impact["affected_edges"], &affectedEdges); err != nil {
		t.Fatalf("unmarshal affected_edges: %v", err)
	}
	if len(affectedEdges) != 1 {
		t.Errorf("expected 1 affected edge, got %d", len(affectedEdges))
	}
}

func TestRuntimeToolsUnavailable(t *testing.T) {
	// Use mock store which is not a SQLiteStore; sqlStore should be nil.
	mockStore := newMockGraphStore()
	srv := NewServer(mockStore)
	ctx := context.Background()

	tests := []struct {
		name    string
		handler func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error)
		args    map[string]any
	}{
		{
			name:    "runtime_traffic",
			handler: srv.handleRuntimeTraffic,
			args:    map[string]any{"service_name": "test"},
		},
		{
			name:    "dead_routes",
			handler: srv.handleDeadRoutes,
			args:    map[string]any{},
		},
		{
			name:    "trace_stats",
			handler: srv.handleTraceStats,
			args:    map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeCallToolRequest(tt.name, tt.args)
			result, err := tt.handler(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result when sqlStore is nil")
			}
			text := result.Content[0].(mcp.TextContent).Text
			if text != "runtime queries not available: store does not support runtime methods" {
				t.Errorf("unexpected error message: %q", text)
			}
		})
	}
}

// --- Missing handler coverage ---

func TestHandleTraceDataflow_ReturnsCallees(t *testing.T) {
	s := newMockGraphStore()
	calleeNode := types.Node{
		NodeHash:      testHash("callee1"),
		QualifiedName: "github.com/example/pkg.Helper",
		Kind:          "function",
	}
	s.transitiveCallees = []types.CalleeResult{
		{Node: calleeNode, Depth: 1},
	}

	srv := NewServer(s)
	req := makeCallToolRequest("trace_dataflow", map[string]any{
		"source_hash": hashHex(testHash("source")),
		"max_depth":   float64(3),
	})

	result, err := srv.handleTraceDataflow(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var callees []types.CalleeResult
	if err := json.Unmarshal([]byte(text), &callees); err != nil {
		t.Fatalf("failed to unmarshal callees: %v", err)
	}
	if len(callees) != 1 {
		t.Fatalf("expected 1 callee, got %d", len(callees))
	}
	if callees[0].Depth != 1 {
		t.Errorf("expected depth 1, got %d", callees[0].Depth)
	}
	if callees[0].Node.QualifiedName != "github.com/example/pkg.Helper" {
		t.Errorf("unexpected callee name: %s", callees[0].Node.QualifiedName)
	}
}

func TestHandleTraceDataflow_EmptyResult(t *testing.T) {
	s := newMockGraphStore()
	s.transitiveCallees = nil

	srv := NewServer(s)
	req := makeCallToolRequest("trace_dataflow", map[string]any{
		"source_hash": hashHex(testHash("isolated-node")),
	})

	result, err := srv.handleTraceDataflow(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	if text != "null" {
		var callees []types.CalleeResult
		if err := json.Unmarshal([]byte(text), &callees); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(callees) != 0 {
			t.Errorf("expected 0 callees, got %d", len(callees))
		}
	}
}

func TestHandleTraceDataflow_MissingSourceHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("trace_dataflow", map[string]any{})

	result, err := srv.handleTraceDataflow(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing source_hash")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "missing required argument: source_hash" {
		t.Errorf("unexpected error text: %q", text)
	}
}

func TestHandleRepoGraph_ReturnsFiles(t *testing.T) {
	s := newMockGraphStore()
	repoHash := testHash("repo1")
	s.filesByRepo[repoHash] = []types.File{
		{FileHash: testHash("f1"), RepoHash: repoHash, Path: "main.go"},
		{FileHash: testHash("f2"), RepoHash: repoHash, Path: "util.go"},
	}

	srv := NewServer(s)
	req := makeCallToolRequest("repo_graph", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleRepoGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var files []types.File
	if err := json.Unmarshal([]byte(text), &files); err != nil {
		t.Fatalf("failed to unmarshal files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Path != "main.go" {
		t.Errorf("expected main.go, got %q", files[0].Path)
	}
}

func TestHandleRepoGraph_EmptyRepo(t *testing.T) {
	s := newMockGraphStore()
	repoHash := testHash("empty-repo")

	srv := NewServer(s)
	req := makeCallToolRequest("repo_graph", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleRepoGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	if text != "null" {
		var files []types.File
		if err := json.Unmarshal([]byte(text), &files); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(files) != 0 {
			t.Errorf("expected 0 files, got %d", len(files))
		}
	}
}

func TestHandleRepoGraph_MissingRepoHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("repo_graph", map[string]any{})

	result, err := srv.handleRepoGraph(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo_hash")
	}
}

// --- Error paths: invalid hashes ---

func TestHandleCrossRepoCallers_InvalidHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("cross_repo_callers", map[string]any{
		"target_hash": "not-a-valid-hex",
	})

	result, err := srv.handleCrossRepoCallers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid hash")
	}
}

func TestHandleCrossRepoCallers_ShortHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	// Valid hex but only 16 bytes instead of 32.
	req := makeCallToolRequest("cross_repo_callers", map[string]any{
		"target_hash": "aabbccddaabbccddaabbccddaabbccdd",
	})

	result, err := srv.handleCrossRepoCallers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for short hash (16 bytes)")
	}
}

func TestHandleCrossRepoCallers_MissingTargetHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("cross_repo_callers", map[string]any{})

	result, err := srv.handleCrossRepoCallers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing target_hash")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "missing required argument: target_hash" {
		t.Errorf("unexpected error text: %q", text)
	}
}

func TestHandleBlastRadius_MissingTargetHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("blast_radius", map[string]any{})

	result, err := srv.handleBlastRadius(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing target_hash")
	}
}

func TestHandleBlastRadius_InvalidSnapshotHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("blast_radius", map[string]any{
		"target_hash":   hashHex(testHash("target")),
		"snapshot_hash": "zzz-not-hex",
	})

	result, err := srv.handleBlastRadius(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid snapshot_hash")
	}
}

func TestHandleGraphQuery_MissingPrefix(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("graph_query", map[string]any{})

	result, err := srv.handleGraphQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing prefix")
	}
}

func TestHandleGraphQuery_EmptyResults(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("graph_query", map[string]any{
		"prefix": "nonexistent.prefix",
	})

	result, err := srv.handleGraphQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	if text != "null" {
		var nodes []types.Node
		if err := json.Unmarshal([]byte(text), &nodes); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(nodes) != 0 {
			t.Errorf("expected 0 nodes, got %d", len(nodes))
		}
	}
}

func TestHandleStaleEdges_MissingSnapshotHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("stale_edges", map[string]any{})

	result, err := srv.handleStaleEdges(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing snapshot_hash")
	}
}

func TestHandleStaleEdges_EmptyResult(t *testing.T) {
	s := newMockGraphStore()
	s.staleEdgesResult = nil

	srv := NewServer(s)
	req := makeCallToolRequest("stale_edges", map[string]any{
		"snapshot_hash": hashHex(testHash("clean-snap")),
	})

	result, err := srv.handleStaleEdges(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}
}

func TestHandleSnapshotDiff_MissingOldSnapshot(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("snapshot_diff", map[string]any{
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handleSnapshotDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing old_snapshot")
	}
}

func TestHandleSnapshotDiff_MissingNewSnapshot(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("snapshot_diff", map[string]any{
		"old_snapshot": hashHex(testHash("old")),
	})

	result, err := srv.handleSnapshotDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing new_snapshot")
	}
}

func TestHandleSemanticDiff_MissingOldSnapshot(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("semantic_diff", map[string]any{
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handleSemanticDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing old_snapshot")
	}
}

func TestHandleSemanticDiff_InvalidHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("semantic_diff", map[string]any{
		"old_snapshot": "bad-hex",
		"new_snapshot": hashHex(testHash("new")),
	})

	result, err := srv.handleSemanticDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid hash")
	}
}

func TestHandlePRImpact_MissingSnapshots(t *testing.T) {
	srv := NewServer(newMockGraphStore())

	t.Run("missing_old", func(t *testing.T) {
		req := makeCallToolRequest("pr_impact", map[string]any{
			"new_snapshot": hashHex(testHash("new")),
		})
		result, err := srv.handlePRImpact(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing old_snapshot")
		}
	})

	t.Run("missing_new", func(t *testing.T) {
		req := makeCallToolRequest("pr_impact", map[string]any{
			"old_snapshot": hashHex(testHash("old")),
		})
		result, err := srv.handlePRImpact(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing new_snapshot")
		}
	})
}

func TestHandleOwnership_RepoNotFound(t *testing.T) {
	s := newMockGraphStore()
	srv := NewServer(s)
	req := makeCallToolRequest("ownership", map[string]any{
		"repo_hash": hashHex(testHash("nonexistent-repo")),
	})

	result, err := srv.handleOwnership(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when repo not found")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Error("expected non-empty error message")
	}
}

func TestHandleOwnership_MissingRepoHash(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	req := makeCallToolRequest("ownership", map[string]any{})

	result, err := srv.handleOwnership(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo_hash")
	}
}

func TestHandleOwnership_EmptyRepo(t *testing.T) {
	s := newMockGraphStore()
	repoHash := testHash("empty-repo")
	s.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/empty",
	}
	// No files, no nodes.

	srv := NewServer(s)
	req := makeCallToolRequest("ownership", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleOwnership(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}
}

func TestHandleIndexRepo_MissingRequiredArgs(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	SetIndexFunc(func(_ context.Context, _, _, _ string) error { return nil })
	defer SetIndexFunc(nil)

	t.Run("missing_repo_url", func(t *testing.T) {
		req := makeCallToolRequest("index_repo", map[string]any{
			"repo_path": "/tmp/repo",
		})
		result, err := srv.handleIndexRepo(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing repo_url")
		}
	})

	t.Run("missing_repo_path", func(t *testing.T) {
		req := makeCallToolRequest("index_repo", map[string]any{
			"repo_url": "https://github.com/example/repo",
		})
		result, err := srv.handleIndexRepo(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing repo_path")
		}
	})
}

func TestHandleIndexRepo_IndexFuncError(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	SetIndexFunc(func(_ context.Context, _, _, _ string) error {
		return fmt.Errorf("disk full")
	})
	defer SetIndexFunc(nil)

	req := makeCallToolRequest("index_repo", map[string]any{
		"repo_url":  "https://github.com/example/repo",
		"repo_path": "/tmp/repo",
	})

	result, err := srv.handleIndexRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when indexFunc returns error")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if text != "index_repo failed: disk full" {
		t.Errorf("unexpected error text: %q", text)
	}
}

func TestHandleRuntimeTraffic_MissingServiceName(t *testing.T) {
	srv, _ := newTestSQLiteServer(t)
	req := makeCallToolRequest("runtime_traffic", map[string]any{})

	result, err := srv.handleRuntimeTraffic(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing service_name")
	}
}

func TestHandleRuntimeTraffic_EmptyResults(t *testing.T) {
	srv, _ := newTestSQLiteServer(t)
	req := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name": "nonexistent-service",
	})

	result, err := srv.handleRuntimeTraffic(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

func TestHandleRuntimeTraffic_WithRoutePattern(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Insert two route symbols for same service but different routes.
	target1 := testHash("handler-get")
	target2 := testHash("handler-post")
	for _, n := range []types.Node{
		{NodeHash: target1, QualifiedName: "api.GetHandler", Kind: "function"},
		{NodeHash: target2, QualifiedName: "api.PostHandler", Kind: "function"},
	} {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := ss.PutRouteSymbol(ctx, "api-svc", "GET /items", target1, "http_route"); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "api-svc", "POST /items", target2, "http_route"); err != nil {
		t.Fatal(err)
	}

	// Insert runtime edges targeting both.
	now := time.Now().Unix()
	for i, tgt := range []types.Hash{target1, target2} {
		eh := testHash(fmt.Sprintf("rt-edge-%d", i))
		if err := ss.PutEdge(ctx, types.Edge{
			EdgeHash: eh, SourceHash: testHash(fmt.Sprintf("rt-src-%d", i)),
			TargetHash: tgt, EdgeType: "calls", Confidence: 0.9, Provenance: "otel_trace",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.UpdateObservation(ctx, eh, 10, now, 0.9); err != nil {
			t.Fatal(err)
		}
	}

	// Query with route_pattern filter.
	req := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name":  "api-svc",
		"route_pattern": "GET%",
	})

	result, err := srv.handleRuntimeTraffic(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var edges []types.Edge
	if err := json.Unmarshal([]byte(text), &edges); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge with GET route filter, got %d", len(edges))
	}
}

func TestHandleRuntimeTraffic_WithLimit(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	// Insert multiple runtime edges for the same service.
	now := time.Now().Unix()
	target := testHash("limit-handler")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash: target, QualifiedName: "api.LimitHandler", Kind: "function",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "limit-svc", "GET /limit", target, "http_route"); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		eh := testHash(fmt.Sprintf("limit-edge-%d", i))
		if err := ss.PutEdge(ctx, types.Edge{
			EdgeHash: eh, SourceHash: testHash(fmt.Sprintf("limit-src-%d", i)),
			TargetHash: target, EdgeType: "calls", Confidence: 0.9, Provenance: "otel_trace",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.UpdateObservation(ctx, eh, i+1, now, 0.9); err != nil {
			t.Fatal(err)
		}
	}

	req := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name": "limit-svc",
		"limit":        float64(2),
	})

	result, err := srv.handleRuntimeTraffic(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var edges []types.Edge
	if err := json.Unmarshal([]byte(text), &edges); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(edges) > 2 {
		t.Errorf("expected at most 2 edges with limit=2, got %d", len(edges))
	}
}

func TestHandleDeadRoutes_DefaultStaleDays(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	target := testHash("dead-default")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash: target, QualifiedName: "api.DeadDefault", Kind: "function",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "svc", "GET /dead", target, "http_route"); err != nil {
		t.Fatal(err)
	}

	// No stale_days argument; should use default of 30.
	req := makeCallToolRequest("dead_routes", map[string]any{})

	result, err := srv.handleDeadRoutes(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var routes []store.RouteSymbolRow
	if err := json.Unmarshal([]byte(text), &routes); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(routes) < 1 {
		t.Errorf("expected at least 1 dead route with default stale_days, got %d", len(routes))
	}
}

func TestHandleTraceStats_EmptyStore(t *testing.T) {
	srv, _ := newTestSQLiteServer(t)
	req := makeCallToolRequest("trace_stats", map[string]any{})

	result, err := srv.handleTraceStats(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var stats store.RuntimeStatsRow
	if err := json.Unmarshal([]byte(text), &stats); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if stats.TotalEdges != 0 {
		t.Errorf("expected 0 total edges in empty store, got %d", stats.TotalEdges)
	}
}

// --- Large result set tests ---

func TestHandleGraphQuery_LargeResultSet(t *testing.T) {
	s := newMockGraphStore()
	nodes := make([]types.Node, 100)
	for i := range nodes {
		nodes[i] = types.Node{
			NodeHash:      testHash(fmt.Sprintf("large-node-%d", i)),
			QualifiedName: fmt.Sprintf("github.com/example/pkg.Func%d", i),
			Kind:          "function",
		}
	}
	s.nodesByName["github.com/example/pkg"] = nodes

	srv := NewServer(s)
	req := makeCallToolRequest("graph_query", map[string]any{
		"prefix": "github.com/example/pkg",
	})

	result, err := srv.handleGraphQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var returnedNodes []types.Node
	if err := json.Unmarshal([]byte(text), &returnedNodes); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(returnedNodes) != 100 {
		t.Errorf("expected 100 nodes, got %d", len(returnedNodes))
	}
}

func TestHandleCrossRepoCallers_LargeResultSet(t *testing.T) {
	s := newMockGraphStore()
	callers := make([]types.CallerResult, 50)
	for i := range callers {
		callers[i] = types.CallerResult{
			Node: types.Node{
				NodeHash:      testHash(fmt.Sprintf("large-caller-%d", i)),
				QualifiedName: fmt.Sprintf("github.com/repo%d/pkg.Func", i),
				Kind:          "function",
			},
			Depth: (i % 5) + 1,
		}
	}
	s.transitiveCallers = callers

	srv := NewServer(s)
	req := makeCallToolRequest("cross_repo_callers", map[string]any{
		"target_hash": hashHex(testHash("popular-target")),
	})

	result, err := srv.handleCrossRepoCallers(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var returnedCallers []types.CallerResult
	if err := json.Unmarshal([]byte(text), &returnedCallers); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(returnedCallers) != 50 {
		t.Errorf("expected 50 callers, got %d", len(returnedCallers))
	}
}

// --- SemanticDiff and PRImpact with edge events via real SQLite ---

func TestHandleSemanticDiff_EmptyDiff(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	repoHash := testHash("empty-diff-repo")
	if err := ss.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: "https://example.com/empty-diff"}); err != nil {
		t.Fatal(err)
	}

	oldSnap := testHash("empty-old")
	newSnap := testHash("empty-new")
	for _, snap := range []types.Snapshot{
		{SnapshotHash: oldSnap, RepoHash: repoHash, CommitHash: "c1", Timestamp: time.Now().Unix()},
		{SnapshotHash: newSnap, RepoHash: repoHash, CommitHash: "c2", Timestamp: time.Now().Unix()},
	} {
		if err := ss.CreateSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}

	// No edge events between snapshots: diff should be empty.
	req := makeCallToolRequest("semantic_diff", map[string]any{
		"old_snapshot": hashHex(oldSnap),
		"new_snapshot": hashHex(newSnap),
	})

	result, err := srv.handleSemanticDiff(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Summary should show zero changes.
	var summary map[string]interface{}
	if err := json.Unmarshal(out["summary"], &summary); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}
	if v, ok := summary["nodes_added"].(float64); !ok || int(v) != 0 {
		t.Errorf("expected nodes_added=0, got %v", summary["nodes_added"])
	}
}

func TestHandlePRImpact_EmptyDiff(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	repoHash := testHash("empty-impact-repo")
	if err := ss.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: "https://example.com/empty-impact"}); err != nil {
		t.Fatal(err)
	}

	oldSnap := testHash("empty-impact-old")
	newSnap := testHash("empty-impact-new")
	for _, snap := range []types.Snapshot{
		{SnapshotHash: oldSnap, RepoHash: repoHash, CommitHash: "c1", Timestamp: time.Now().Unix()},
		{SnapshotHash: newSnap, RepoHash: repoHash, CommitHash: "c2", Timestamp: time.Now().Unix()},
	} {
		if err := ss.CreateSnapshot(ctx, snap); err != nil {
			t.Fatal(err)
		}
	}

	req := makeCallToolRequest("pr_impact", map[string]any{
		"old_snapshot": hashHex(oldSnap),
		"new_snapshot": hashHex(newSnap),
	})

	result, err := srv.handlePRImpact(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var summary map[string]interface{}
	if err := json.Unmarshal(out["summary"], &summary); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}
	if v := summary["total_symbols_changed"].(float64); int(v) != 0 {
		t.Errorf("expected 0 changed symbols, got %v", v)
	}
}

// --- Helper function unit tests ---

func TestParseHash_Valid(t *testing.T) {
	h := testHash("test")
	hexStr := hashHex(h)
	parsed, err := parseHash(hexStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed != h {
		t.Errorf("hash mismatch")
	}
}

func TestParseHash_InvalidHex(t *testing.T) {
	_, err := parseHash("not-valid-hex-string!")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestParseHash_WrongLength(t *testing.T) {
	_, err := parseHash("aabb") // 2 bytes, not 32
	if err == nil {
		t.Fatal("expected error for wrong length")
	}
}

func TestGetStringArg(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{
		"name": "value",
	})
	if v := getStringArg(req, "name"); v != "value" {
		t.Errorf("expected 'value', got %q", v)
	}
	if v := getStringArg(req, "missing"); v != "" {
		t.Errorf("expected empty string for missing arg, got %q", v)
	}
}

func TestGetStringArg_NilArgs(t *testing.T) {
	req := makeCallToolRequest("test", nil)
	if v := getStringArg(req, "name"); v != "" {
		t.Errorf("expected empty string for nil args, got %q", v)
	}
}

func TestGetIntArg(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{
		"count": float64(42),
	})
	if v := getIntArg(req, "count", 10); v != 42 {
		t.Errorf("expected 42, got %d", v)
	}
	if v := getIntArg(req, "missing", 10); v != 10 {
		t.Errorf("expected default 10, got %d", v)
	}
}

func TestGetIntArg_NilArgs(t *testing.T) {
	req := makeCallToolRequest("test", nil)
	if v := getIntArg(req, "count", 99); v != 99 {
		t.Errorf("expected default 99, got %d", v)
	}
}

func TestGetIntArg_WrongType(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{
		"count": "not-a-number",
	})
	if v := getIntArg(req, "count", 5); v != 5 {
		t.Errorf("expected default 5 for wrong type, got %d", v)
	}
}

func TestRequireStringArg_Present(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{"key": "val"})
	v, errResult := requireStringArg(req, "key")
	if errResult != nil {
		t.Fatalf("unexpected error result")
	}
	if v != "val" {
		t.Errorf("expected 'val', got %q", v)
	}
}

func TestRequireStringArg_Missing(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{})
	_, errResult := requireStringArg(req, "key")
	if errResult == nil {
		t.Fatal("expected error result for missing arg")
	}
	if !errResult.IsError {
		t.Error("expected IsError=true")
	}
}

func TestRequireHash_Valid(t *testing.T) {
	h := testHash("hash-test")
	req := makeCallToolRequest("test", map[string]any{"h": hashHex(h)})
	parsed, errResult := requireHash(req, "h")
	if errResult != nil {
		t.Fatalf("unexpected error result")
	}
	if parsed != h {
		t.Error("hash mismatch")
	}
}

func TestRequireHash_Missing(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{})
	_, errResult := requireHash(req, "h")
	if errResult == nil {
		t.Fatal("expected error result for missing hash")
	}
}

func TestRequireHash_Invalid(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{"h": "zzzz"})
	_, errResult := requireHash(req, "h")
	if errResult == nil {
		t.Fatal("expected error result for invalid hash")
	}
}

func TestOptionalHash_Present(t *testing.T) {
	h := testHash("opt-hash")
	req := makeCallToolRequest("test", map[string]any{"h": hashHex(h)})
	parsed, errResult := optionalHash(req, "h")
	if errResult != nil {
		t.Fatalf("unexpected error result")
	}
	if parsed != h {
		t.Error("hash mismatch")
	}
}

func TestOptionalHash_Absent(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{})
	h, errResult := optionalHash(req, "h")
	if errResult != nil {
		t.Fatalf("unexpected error result")
	}
	if h != types.EmptyHash {
		t.Error("expected EmptyHash when absent")
	}
}

func TestOptionalHash_Invalid(t *testing.T) {
	req := makeCallToolRequest("test", map[string]any{"h": "bad!"})
	_, errResult := optionalHash(req, "h")
	if errResult == nil {
		t.Fatal("expected error result for invalid hash")
	}
}

// --- Integration: RepoGraph with real SQLite ---

func TestHandleRepoGraph_Integration(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	repoHash := testHash("rg-repo")
	if err := ss.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: "https://example.com/rg"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := ss.PutFile(ctx, types.File{
			FileHash:    testHash(fmt.Sprintf("rg-file-%d", i)),
			RepoHash:    repoHash,
			Path:        fmt.Sprintf("pkg/file%d.go", i),
			ContentHash: testHash(fmt.Sprintf("rg-content-%d", i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	req := makeCallToolRequest("repo_graph", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleRepoGraph(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var files []types.File
	if err := json.Unmarshal([]byte(text), &files); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}
}

// --- Integration: Ownership with real SQLite ---

func TestHandleOwnership_Integration(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	repoHash := testHash("own-repo")
	if err := ss.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: "https://example.com/own"}); err != nil {
		t.Fatal(err)
	}

	fileHash := testHash("own-file")
	if err := ss.PutFile(ctx, types.File{
		FileHash: fileHash, RepoHash: repoHash, Path: "main.go",
		ContentHash: testHash("own-content"),
	}); err != nil {
		t.Fatal(err)
	}

	if err := ss.PutNode(ctx, types.Node{
		NodeHash: testHash("own-node"), FileHash: fileHash,
		QualifiedName: "https://example.com/own://main.Handler",
		Kind: "function", Line: 10,
	}); err != nil {
		t.Fatal(err)
	}

	req := makeCallToolRequest("ownership", map[string]any{
		"repo_hash": hashHex(repoHash),
	})

	result, err := srv.handleOwnership(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var ownership []struct {
		File  types.File   `json:"file"`
		Nodes []types.Node `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(text), &ownership); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(ownership) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(ownership))
	}
	if len(ownership[0].Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(ownership[0].Nodes))
	}
}

// --- Additional coverage tests ---

func TestHandleContextForTask_WithSQLiteStore_MultipleNodes(t *testing.T) {
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	ctx := context.Background()

	// Seed multiple nodes matching different keywords.
	nodes := []types.Node{
		{NodeHash: testHash("auth-handler"), QualifiedName: "api.AuthenticateUser", Kind: "function", Signature: "func AuthenticateUser(token string) error"},
		{NodeHash: testHash("auth-middleware"), QualifiedName: "middleware.AuthCheck", Kind: "function", Signature: "func AuthCheck(next http.Handler) http.Handler"},
		{NodeHash: testHash("db-connect"), QualifiedName: "db.Connect", Kind: "function", Signature: "func Connect(dsn string) (*DB, error)"},
	}
	for _, n := range nodes {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Add edges between them so graph ranking has something to work with.
	edge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(nodes[1].NodeHash, nodes[0].NodeHash, "calls", "ast_resolved"),
		SourceHash: nodes[1].NodeHash,
		TargetHash: nodes[0].NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := ss.PutEdge(ctx, edge); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ss)

	req := makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "fix authentication middleware",
		"token_budget":     float64(80000),
	})

	result, err := srv.handleContextForTask(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("expected success, got error: %s", text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if text == "" {
		t.Fatal("expected non-empty context output")
	}
	// Should be GCF format (default since session 27).
	if !strings.Contains(text, "GCF profile=graph") {
		t.Errorf("expected GCF context block, got: %.100s", text)
	}
}

func TestHandleContextForFiles_WithSQLiteStore_MultipleFiles(t *testing.T) {
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	ctx := context.Background()

	repoURL := "https://github.com/example/myapp"
	repoHash := types.NewHash([]byte(repoURL))
	if err := ss.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: repoURL}); err != nil {
		t.Fatal(err)
	}

	// Seed two files with nodes.
	file1Hash := testHash("file-handler")
	file2Hash := testHash("file-service")
	if err := ss.PutFile(ctx, types.File{FileHash: file1Hash, RepoHash: repoHash, Path: "handler.go"}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutFile(ctx, types.File{FileHash: file2Hash, RepoHash: repoHash, Path: "service.go"}); err != nil {
		t.Fatal(err)
	}

	node1 := types.Node{NodeHash: testHash("handler-func"), FileHash: file1Hash, QualifiedName: "pkg.HandleRequest", Kind: "function", Signature: "func HandleRequest(w http.ResponseWriter, r *http.Request)"}
	node2 := types.Node{NodeHash: testHash("service-func"), FileHash: file2Hash, QualifiedName: "pkg.ProcessData", Kind: "function", Signature: "func ProcessData(data []byte) error"}
	for _, n := range []types.Node{node1, node2} {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Edge: handler calls service.
	edge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(node1.NodeHash, node2.NodeHash, "calls", "ast_resolved"),
		SourceHash: node1.NodeHash,
		TargetHash: node2.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := ss.PutEdge(ctx, edge); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ss)

	req := makeCallToolRequest("context_for_files", map[string]any{
		"files":        "handler.go, service.go",
		"repo_url":     repoURL,
		"token_budget": float64(60000),
	})

	result, err := srv.handleContextForFiles(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("expected success, got error: %s", text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// GCF is default format since session 27.
	if !strings.Contains(text, "GCF profile=graph") {
		t.Errorf("expected GCF context block, got: %.100s", text)
	}
}

func TestHandleTraceStats_WithPopulatedRuntimeEdges(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert runtime edges with varying ages and types.
	type edgeSpec struct {
		id       string
		edgeType string
		lastObs  int64
		count    int
	}
	specs := []edgeSpec{
		{"active-call-1", "calls", now - 3600, 100},          // active (1 hour ago)
		{"active-call-2", "calls", now - 86400, 50},          // active (1 day ago)
		{"stale-call-1", "calls", now - 45*86400, 20},        // stale (45 days)
		{"gc-eligible-1", "calls", now - 120*86400, 5},       // GC eligible (120 days)
		{"gc-eligible-2", "imports", now - 100*86400, 2},     // GC eligible (100 days)
	}

	for i, spec := range specs {
		edgeHash := testHash(spec.id)
		if err := ss.PutEdge(ctx, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: testHash(fmt.Sprintf("src-ts-%d", i)),
			TargetHash: testHash(fmt.Sprintf("tgt-ts-%d", i)),
			EdgeType:   spec.edgeType,
			Confidence: 0.85,
			Provenance: "otel_trace",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.UpdateObservation(ctx, edgeHash, spec.count, spec.lastObs, 0.85); err != nil {
			t.Fatal(err)
		}
	}

	req := makeCallToolRequest("trace_stats", map[string]any{})

	result, err := srv.handleTraceStats(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var stats store.RuntimeStatsRow
	if err := json.Unmarshal([]byte(text), &stats); err != nil {
		t.Fatalf("failed to unmarshal stats: %v", err)
	}
	if stats.TotalEdges != 5 {
		t.Errorf("expected 5 total edges, got %d", stats.TotalEdges)
	}
	if stats.ActiveEdges != 2 {
		t.Errorf("expected 2 active edges, got %d", stats.ActiveEdges)
	}
	if stats.GCEligible < 2 {
		t.Errorf("expected at least 2 GC eligible edges, got %d", stats.GCEligible)
	}
}

func TestHandleDeadRoutes_MixActiveAndDead(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Create an active route (has recent runtime observations).
	activeTarget := testHash("active-handler")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      activeTarget,
		QualifiedName: "api.GetUsers",
		Kind:          "function",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "user-svc", "GET /users", activeTarget, "http_route"); err != nil {
		t.Fatal(err)
	}
	// Add a runtime edge with recent observation.
	activeEdge := testHash("active-edge")
	if err := ss.PutEdge(ctx, types.Edge{
		EdgeHash:   activeEdge,
		SourceHash: testHash("gateway"),
		TargetHash: activeTarget,
		EdgeType:   "calls",
		Confidence: 0.95,
		Provenance: "otel_trace",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.UpdateObservation(ctx, activeEdge, 500, now-3600, 0.95); err != nil {
		t.Fatal(err)
	}

	// Create two dead routes (no recent runtime observations).
	dead1 := testHash("dead-handler-1")
	dead2 := testHash("dead-handler-2")
	if err := ss.PutNode(ctx, types.Node{NodeHash: dead1, QualifiedName: "api.OldEndpoint", Kind: "function"}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutNode(ctx, types.Node{NodeHash: dead2, QualifiedName: "api.DeprecatedAction", Kind: "function"}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "user-svc", "DELETE /legacy", dead1, "http_route"); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutRouteSymbol(ctx, "admin-svc", "POST /admin/reset", dead2, "http_route"); err != nil {
		t.Fatal(err)
	}

	req := makeCallToolRequest("dead_routes", map[string]any{
		"stale_days": float64(7),
	})

	result, err := srv.handleDeadRoutes(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var routes []store.RouteSymbolRow
	if err := json.Unmarshal([]byte(text), &routes); err != nil {
		t.Fatalf("failed to unmarshal routes: %v", err)
	}

	// Should have 2 dead routes; the active one should be excluded.
	if len(routes) != 2 {
		t.Fatalf("expected 2 dead routes (active excluded), got %d", len(routes))
	}

	// Collect route patterns.
	patterns := make(map[string]bool)
	for _, r := range routes {
		patterns[r.RoutePattern] = true
	}
	if !patterns["DELETE /legacy"] {
		t.Error("expected dead route 'DELETE /legacy' to be present")
	}
	if !patterns["POST /admin/reset"] {
		t.Error("expected dead route 'POST /admin/reset' to be present")
	}
	if patterns["GET /users"] {
		t.Error("active route 'GET /users' should NOT be in dead routes")
	}
}

func TestHandleRuntimeTraffic_WithRoutePatternFilter(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Set up multiple route symbols for the same service.
	targets := []struct {
		id      string
		name    string
		route   string
		service string
	}{
		{"target-users", "api.GetUsers", "GET /users", "api-svc"},
		{"target-user-id", "api.GetUserByID", "GET /users/:id", "api-svc"},
		{"target-orders", "api.GetOrders", "GET /orders", "api-svc"},
		{"target-other", "api.OtherService", "GET /other", "other-svc"},
	}

	for _, tgt := range targets {
		nodeHash := testHash(tgt.id)
		if err := ss.PutNode(ctx, types.Node{
			NodeHash:      nodeHash,
			QualifiedName: tgt.name,
			Kind:          "function",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.PutRouteSymbol(ctx, tgt.service, tgt.route, nodeHash, "http_route"); err != nil {
			t.Fatal(err)
		}

		// Add a runtime edge for each.
		edgeHash := testHash("edge-" + tgt.id)
		if err := ss.PutEdge(ctx, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: testHash("caller-" + tgt.id),
			TargetHash: nodeHash,
			EdgeType:   "calls",
			Confidence: 0.9,
			Provenance: "otel_trace",
		}); err != nil {
			t.Fatal(err)
		}
		if err := ss.UpdateObservation(ctx, edgeHash, 10, now, 0.9); err != nil {
			t.Fatal(err)
		}
	}

	// Query by service_name only.
	req := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name": "api-svc",
	})

	result, err := srv.handleRuntimeTraffic(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var edges []types.Edge
	if err := json.Unmarshal([]byte(text), &edges); err != nil {
		t.Fatalf("failed to unmarshal edges: %v", err)
	}
	// Should return 3 edges for api-svc (not the other-svc one).
	if len(edges) != 3 {
		t.Errorf("expected 3 edges for api-svc, got %d", len(edges))
	}

	// Query with route_pattern filter.
	req2 := makeCallToolRequest("runtime_traffic", map[string]any{
		"service_name":  "api-svc",
		"route_pattern": "%users%",
	})

	result2, err := srv.handleRuntimeTraffic(ctx, req2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("expected success, got error: %v", result2.Content)
	}

	text2 := result2.Content[0].(mcp.TextContent).Text
	var edges2 []types.Edge
	if err := json.Unmarshal([]byte(text2), &edges2); err != nil {
		t.Fatalf("failed to unmarshal edges: %v", err)
	}
	// Should return only the users-related edges (2 of them).
	if len(edges2) != 2 {
		t.Errorf("expected 2 edges matching '%%users%%' pattern, got %d", len(edges2))
	}
}
