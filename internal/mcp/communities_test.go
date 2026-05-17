package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// communityMockStore extends mockGraphStore with NodesByQualifiedName support.
type communityMockStore struct {
	mockGraphStore
	qualifiedNodes map[string][]types.Node
}

func (m *communityMockStore) NodesByQualifiedName(_ context.Context, name string) ([]types.Node, error) {
	if nodes, ok := m.qualifiedNodes[name]; ok {
		return nodes, nil
	}
	return nil, nil
}

func newCommunityMockStore() *communityMockStore {
	return &communityMockStore{
		mockGraphStore: *newMockGraphStore(),
		qualifiedNodes: make(map[string][]types.Node),
	}
}

// buildTwoClusterGraph creates a graph with two clearly separable clusters:
// Cluster 1: A-B-C (fully connected)
// Cluster 2: D-E-F (fully connected)
// One cross-edge: C->D
func buildTwoClusterGraph() (*communityMockStore, []types.Node) {
	store := newCommunityMockStore()

	// Create nodes.
	nodeA := types.Node{NodeHash: testHash("A"), QualifiedName: "https://github.com/org/repo://pkg1.FuncA", Kind: "function"}
	nodeB := types.Node{NodeHash: testHash("B"), QualifiedName: "https://github.com/org/repo://pkg1.FuncB", Kind: "function"}
	nodeC := types.Node{NodeHash: testHash("C"), QualifiedName: "https://github.com/org/repo://pkg1.FuncC", Kind: "function"}
	nodeD := types.Node{NodeHash: testHash("D"), QualifiedName: "https://github.com/org/repo://pkg2.FuncD", Kind: "function"}
	nodeE := types.Node{NodeHash: testHash("E"), QualifiedName: "https://github.com/org/repo://pkg2.FuncE", Kind: "function"}
	nodeF := types.Node{NodeHash: testHash("F"), QualifiedName: "https://github.com/org/repo://pkg2.FuncF", Kind: "function"}

	allNodes := []types.Node{nodeA, nodeB, nodeC, nodeD, nodeE, nodeF}

	// Store nodes.
	for _, n := range allNodes {
		store.nodes[n.NodeHash] = &n
	}

	// Set up NodesByName to return all nodes for empty prefix.
	store.nodesByName[""] = allNodes
	store.nodesByName["https://github.com/org/repo"] = allNodes

	// Set up qualified name lookups.
	for _, n := range allNodes {
		store.qualifiedNodes[n.QualifiedName] = []types.Node{n}
	}

	// Build edges: Cluster 1 (A-B, A-C, B-C).
	store.edgesFrom[nodeA.NodeHash] = []types.Edge{
		{SourceHash: nodeA.NodeHash, TargetHash: nodeB.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeA.NodeHash, TargetHash: nodeC.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}
	store.edgesFrom[nodeB.NodeHash] = []types.Edge{
		{SourceHash: nodeB.NodeHash, TargetHash: nodeA.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeB.NodeHash, TargetHash: nodeC.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}
	store.edgesFrom[nodeC.NodeHash] = []types.Edge{
		{SourceHash: nodeC.NodeHash, TargetHash: nodeA.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeC.NodeHash, TargetHash: nodeB.NodeHash, EdgeType: "calls", Confidence: 1.0},
		// Cross-edge to cluster 2.
		{SourceHash: nodeC.NodeHash, TargetHash: nodeD.NodeHash, EdgeType: "calls", Confidence: 0.5},
	}

	// Cluster 2 (D-E, D-F, E-F).
	store.edgesFrom[nodeD.NodeHash] = []types.Edge{
		{SourceHash: nodeD.NodeHash, TargetHash: nodeE.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeD.NodeHash, TargetHash: nodeF.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}
	store.edgesFrom[nodeE.NodeHash] = []types.Edge{
		{SourceHash: nodeE.NodeHash, TargetHash: nodeD.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeE.NodeHash, TargetHash: nodeF.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}
	store.edgesFrom[nodeF.NodeHash] = []types.Edge{
		{SourceHash: nodeF.NodeHash, TargetHash: nodeD.NodeHash, EdgeType: "calls", Confidence: 1.0},
		{SourceHash: nodeF.NodeHash, TargetHash: nodeE.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}

	return store, allNodes
}

func TestCommunities_List_TwoClusters(t *testing.T) {
	store, _ := buildTwoClusterGraph()

	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "list",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	var cr communityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Should produce 2 communities.
	if len(cr.Communities) != 2 {
		t.Errorf("expected 2 communities, got %d", len(cr.Communities))
	}

	// Each community should have 3 nodes.
	for _, c := range cr.Communities {
		if c.Size != 3 {
			t.Errorf("community %d: expected size 3, got %d", c.ID, c.Size)
		}
	}

	// Node count should be 6.
	if cr.NodeCount != 6 {
		t.Errorf("expected node_count=6, got %d", cr.NodeCount)
	}
}

func TestCommunities_Cohesion(t *testing.T) {
	store, _ := buildTwoClusterGraph()

	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "list",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}

	var cr communityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Cohesion should be high for tight clusters (> 0.8).
	for _, c := range cr.Communities {
		if c.Cohesion < 0.8 {
			t.Errorf("community %d: expected high cohesion, got %.2f", c.ID, c.Cohesion)
		}
	}
}

func TestCommunities_TopSymbols(t *testing.T) {
	store, _ := buildTwoClusterGraph()

	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "list",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}

	var cr communityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	for _, c := range cr.Communities {
		if len(c.TopSymbols) == 0 {
			t.Errorf("community %d: expected top_symbols to be populated", c.ID)
		}
		if len(c.TopSymbols) > 5 {
			t.Errorf("community %d: expected at most 5 top_symbols, got %d", c.ID, len(c.TopSymbols))
		}
	}
}

func TestCommunities_ForSymbol(t *testing.T) {
	store, _ := buildTwoClusterGraph()

	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "for_symbol",
		"symbol": "https://github.com/org/repo://pkg1.FuncA",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content)
	}

	var scr symbolCommunityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &scr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if scr.Symbol != "https://github.com/org/repo://pkg1.FuncA" {
		t.Errorf("expected symbol in result, got %q", scr.Symbol)
	}

	// Community should contain FuncA.
	if scr.Community.Size != 3 {
		t.Errorf("expected community size 3, got %d", scr.Community.Size)
	}

	// Should have at least one neighbor community (the other cluster).
	if len(scr.Neighbors) == 0 {
		t.Errorf("expected at least one neighbor community")
	}
}

func TestCommunities_RepoURLFilter(t *testing.T) {
	store := newCommunityMockStore()

	// Create nodes from two repos.
	nodeA := types.Node{NodeHash: testHash("A"), QualifiedName: "https://github.com/org/repo1://pkg.FuncA", Kind: "function"}
	nodeB := types.Node{NodeHash: testHash("B"), QualifiedName: "https://github.com/org/repo1://pkg.FuncB", Kind: "function"}
	nodeC := types.Node{NodeHash: testHash("C"), QualifiedName: "https://github.com/org/repo2://pkg.FuncC", Kind: "function"}

	// Only repo1 nodes returned when filtering.
	store.nodesByName["https://github.com/org/repo1"] = []types.Node{nodeA, nodeB}
	store.nodesByName[""] = []types.Node{nodeA, nodeB, nodeC}

	store.nodes[nodeA.NodeHash] = &nodeA
	store.nodes[nodeB.NodeHash] = &nodeB
	store.nodes[nodeC.NodeHash] = &nodeC

	store.edgesFrom[nodeA.NodeHash] = []types.Edge{
		{SourceHash: nodeA.NodeHash, TargetHash: nodeB.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}
	store.edgesFrom[nodeB.NodeHash] = []types.Edge{
		{SourceHash: nodeB.NodeHash, TargetHash: nodeA.NodeHash, EdgeType: "calls", Confidence: 1.0},
	}

	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action":   "list",
		"repo_url": "https://github.com/org/repo1",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}

	var cr communityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Should only see repo1 nodes (2 nodes).
	if cr.NodeCount != 2 {
		t.Errorf("expected node_count=2 with repo filter, got %d", cr.NodeCount)
	}
}

func TestCommunities_ForSymbol_MissingSymbol(t *testing.T) {
	store := newCommunityMockStore()
	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "for_symbol",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when symbol is missing")
	}
}

func TestCommunities_EmptyGraph(t *testing.T) {
	store := newCommunityMockStore()
	srv := &Server{store: store}
	ctx := context.Background()

	req := makeCallToolRequest("communities", map[string]any{
		"action": "list",
	})

	result, err := srv.handleCommunities(ctx, req)
	if err != nil {
		t.Fatalf("handleCommunities error: %v", err)
	}

	var cr communityResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &cr); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if cr.NodeCount != 0 {
		t.Errorf("expected 0 nodes for empty graph, got %d", cr.NodeCount)
	}
}
