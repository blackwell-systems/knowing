package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
	mcp "github.com/mark3labs/mcp-go/mcp"
)

// flowMockStore wraps mockGraphStore and adds NodesByQualifiedName support.
type flowMockStore struct {
	*mockGraphStore
	nodesByQualified map[string][]types.Node
}

func (m *flowMockStore) NodesByQualifiedName(_ context.Context, qualifiedName string) ([]types.Node, error) {
	return m.nodesByQualified[qualifiedName], nil
}

// makeHash creates a deterministic hash from a byte for testing.
func makeHash(b byte) types.Hash {
	var h types.Hash
	h[0] = b
	return h
}

// buildTestGraph creates a small graph: A->B->C->D, A->C (shortcut).
func buildTestGraph() (*flowMockStore, types.Hash, types.Hash, types.Hash, types.Hash) {
	hashA := makeHash(1)
	hashB := makeHash(2)
	hashC := makeHash(3)
	hashD := makeHash(4)

	nodeA := &types.Node{NodeHash: hashA, QualifiedName: "pkg.A"}
	nodeB := &types.Node{NodeHash: hashB, QualifiedName: "pkg.B"}
	nodeC := &types.Node{NodeHash: hashC, QualifiedName: "pkg.C"}
	nodeD := &types.Node{NodeHash: hashD, QualifiedName: "pkg.D"}

	mock := &flowMockStore{
		mockGraphStore: &mockGraphStore{
			nodes: map[types.Hash]*types.Node{
				hashA: nodeA,
				hashB: nodeB,
				hashC: nodeC,
				hashD: nodeD,
			},
			edges:       make(map[types.Hash]*types.Edge),
			snapshots:   make(map[types.Hash]*types.Snapshot),
			repos:       make(map[types.Hash]*types.Repo),
			files:       make(map[types.Hash]*types.File),
			nodesByName: make(map[string][]types.Node),
			edgesFrom: map[types.Hash][]types.Edge{
				hashA: {
					{SourceHash: hashA, TargetHash: hashB, EdgeType: "calls"},
					{SourceHash: hashA, TargetHash: hashC, EdgeType: "calls"},
				},
				hashB: {
					{SourceHash: hashB, TargetHash: hashC, EdgeType: "calls"},
				},
				hashC: {
					{SourceHash: hashC, TargetHash: hashD, EdgeType: "calls"},
				},
			},
			edgesTo:    make(map[types.Hash][]types.Edge),
			filesByRepo: make(map[types.Hash][]types.File),
		},
		nodesByQualified: map[string][]types.Node{
			"pkg.A": {*nodeA},
			"pkg.B": {*nodeB},
			"pkg.C": {*nodeC},
			"pkg.D": {*nodeD},
		},
	}

	return mock, hashA, hashB, hashC, hashD
}

func makeFlowRequest(source, target string, maxDepth int) mcp.CallToolRequest {
	args := map[string]any{
		"source_symbol": source,
		"target_symbol": target,
	}
	if maxDepth > 0 {
		args["max_depth"] = float64(maxDepth)
	}
	return makeCallToolRequest("flow_between", args)
}

func TestFlowBetween_FindsPaths(t *testing.T) {
	mock, _, _, _, _ := buildTestGraph()
	s := &Server{store: mock}

	req := makeFlowRequest("pkg.A", "pkg.C", 0)
	result, err := s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fr flowResult
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &fr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if fr.PathCount < 2 {
		t.Errorf("expected at least 2 paths, got %d", fr.PathCount)
	}

	// Verify both direct A->C and indirect A->B->C paths exist.
	foundDirect := false
	foundIndirect := false
	for _, p := range fr.Paths {
		if len(p.Steps) == 2 && p.Steps[0].Symbol == "pkg.A" && p.Steps[1].Symbol == "pkg.C" {
			foundDirect = true
		}
		if len(p.Steps) == 3 && p.Steps[0].Symbol == "pkg.A" && p.Steps[1].Symbol == "pkg.B" && p.Steps[2].Symbol == "pkg.C" {
			foundIndirect = true
		}
	}
	if !foundDirect {
		t.Error("expected direct path A->C not found")
	}
	if !foundIndirect {
		t.Error("expected indirect path A->B->C not found")
	}
}

func TestFlowBetween_DepthLimiting(t *testing.T) {
	mock, _, _, _, _ := buildTestGraph()
	s := &Server{store: mock}

	// With max_depth=1, only direct edges are followed.
	// Path A->C is depth 1 (1 hop), path A->B->C is depth 2 (2 hops).
	// So only A->C should be found.
	req := makeFlowRequest("pkg.A", "pkg.C", 1)
	result, err := s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fr flowResult
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &fr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Only the direct path should be found with depth 1.
	if fr.PathCount != 1 {
		t.Errorf("expected 1 path with depth limit 1, got %d", fr.PathCount)
	}
}

func TestFlowBetween_SymbolNotFound(t *testing.T) {
	mock, _, _, _, _ := buildTestGraph()
	s := &Server{store: mock}

	// Source not found.
	req := makeFlowRequest("pkg.NonExistent", "pkg.C", 0)
	result, err := s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing source symbol")
	}

	// Target not found.
	req = makeFlowRequest("pkg.A", "pkg.NonExistent", 0)
	result, err = s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for missing target symbol")
	}
}

func TestFlowBetween_NoPath(t *testing.T) {
	mock, _, _, _, _ := buildTestGraph()
	s := &Server{store: mock}

	// D has no outgoing edges so B cannot reach D through C via depth limit of 1.
	// Actually D->nothing, so from D to A there's no path.
	req := makeFlowRequest("pkg.D", "pkg.A", 0)
	result, err := s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fr flowResult
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &fr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if fr.PathCount != 0 {
		t.Errorf("expected 0 paths, got %d", fr.PathCount)
	}
	if len(fr.Paths) != 0 {
		t.Errorf("expected empty paths array, got %d paths", len(fr.Paths))
	}
}

func TestFlowBetween_Truncation(t *testing.T) {
	// Build a graph with many parallel paths from source to target.
	hashSrc := makeHash(10)
	hashTgt := makeHash(20)

	nodeSrc := &types.Node{NodeHash: hashSrc, QualifiedName: "pkg.Src"}
	nodeTgt := &types.Node{NodeHash: hashTgt, QualifiedName: "pkg.Tgt"}

	nodes := map[types.Hash]*types.Node{
		hashSrc: nodeSrc,
		hashTgt: nodeTgt,
	}
	edgesFrom := map[types.Hash][]types.Edge{}

	// Create 15 intermediate nodes, each with a direct edge from src and to tgt.
	var srcEdges []types.Edge
	for i := byte(100); i < 115; i++ {
		h := makeHash(i)
		nodes[h] = &types.Node{NodeHash: h, QualifiedName: "pkg.Mid" + string(rune('A'+i-100))}
		srcEdges = append(srcEdges, types.Edge{SourceHash: hashSrc, TargetHash: h, EdgeType: "calls"})
		edgesFrom[h] = []types.Edge{{SourceHash: h, TargetHash: hashTgt, EdgeType: "calls"}}
	}
	edgesFrom[hashSrc] = srcEdges

	mock := &flowMockStore{
		mockGraphStore: &mockGraphStore{
			nodes:       nodes,
			edges:       make(map[types.Hash]*types.Edge),
			snapshots:   make(map[types.Hash]*types.Snapshot),
			repos:       make(map[types.Hash]*types.Repo),
			files:       make(map[types.Hash]*types.File),
			nodesByName: make(map[string][]types.Node),
			edgesFrom:   edgesFrom,
			edgesTo:     make(map[types.Hash][]types.Edge),
			filesByRepo: make(map[types.Hash][]types.File),
		},
		nodesByQualified: map[string][]types.Node{
			"pkg.Src": {*nodeSrc},
			"pkg.Tgt": {*nodeTgt},
		},
	}

	s := &Server{store: mock}
	req := makeFlowRequest("pkg.Src", "pkg.Tgt", 0)
	result, err := s.handleFlowBetween(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fr flowResult
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &fr); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if fr.PathCount != maxFlowPaths {
		t.Errorf("expected %d paths (capped), got %d", maxFlowPaths, fr.PathCount)
	}
	if !fr.Truncated {
		t.Error("expected truncated=true when paths exceed limit")
	}
}
