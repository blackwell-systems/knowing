package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// flowStep represents one hop in a path between two symbols.
type flowStep struct {
	Symbol   string `json:"symbol"`
	EdgeType string `json:"edge_type"`
}

// flowPath represents a complete path from source to target.
type flowPath struct {
	Steps []flowStep `json:"steps"`
}

// flowResult is the JSON response for the flow_between tool.
type flowResult struct {
	Source    string     `json:"source"`
	Target   string     `json:"target"`
	Paths    []flowPath `json:"paths"`
	PathCount int       `json:"path_count"`
	Truncated bool      `json:"truncated"`
}

const maxFlowPaths = 10

// flowBetweenTool defines the "flow_between" MCP tool.
func flowBetweenTool() mcp.Tool {
	return mcp.NewTool("flow_between",
		mcp.WithDescription("Find paths between two symbols in the knowledge graph using BFS traversal."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("source_symbol", mcp.Required(), mcp.Description("Qualified name of the source symbol (e.g. github.com/org/repo://pkg.FuncA)")),
		mcp.WithString("target_symbol", mcp.Required(), mcp.Description("Qualified name of the target symbol (e.g. github.com/org/repo://pkg.FuncB)")),
		mcp.WithInteger("max_depth", mcp.Description("Maximum BFS depth (default 5)")),
	)
}

// bfsState tracks the current position and path taken during BFS.
type bfsState struct {
	hash types.Hash
	path []flowStep
}

// handleFlowBetween implements BFS path finding between two symbols.
func (s *Server) handleFlowBetween(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceSymbol, errResult := requireStringArg(req, "source_symbol")
	if errResult != nil {
		return errResult, nil
	}
	targetSymbol, errResult := requireStringArg(req, "target_symbol")
	if errResult != nil {
		return errResult, nil
	}
	maxDepth := getIntArg(req, "max_depth", 5)

	// Resolve source symbol to nodes.
	sourceNodes, err := s.store.NodesByQualifiedName(ctx, sourceSymbol)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to resolve source: %v", err)), nil
	}
	if len(sourceNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("symbol not found: %s", sourceSymbol)), nil
	}

	// Resolve target symbol to nodes.
	targetNodes, err := s.store.NodesByQualifiedName(ctx, targetSymbol)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to resolve target: %v", err)), nil
	}
	if len(targetNodes) == 0 {
		return mcp.NewToolResultError(fmt.Sprintf("symbol not found: %s", targetSymbol)), nil
	}

	// Build target hash set for fast lookup.
	targetSet := make(map[types.Hash]bool, len(targetNodes))
	for _, n := range targetNodes {
		targetSet[n.NodeHash] = true
	}

	// BFS from all source nodes.
	var paths []flowPath
	truncated := false

	queue := make([]bfsState, 0, len(sourceNodes))
	for _, n := range sourceNodes {
		queue = append(queue, bfsState{
			hash: n.NodeHash,
			path: []flowStep{{Symbol: n.QualifiedName, EdgeType: ""}},
		})
	}

	for len(queue) > 0 && len(paths) < maxFlowPaths {
		current := queue[0]
		queue = queue[1:]

		// Check depth limit (path length includes source, so depth is len-1).
		if len(current.path) > maxDepth {
			continue
		}

		// Get all outgoing edges from current node.
		edges, err := s.store.EdgesFrom(ctx, current.hash, "")
		if err != nil {
			continue
		}

		for _, edge := range edges {
			// Build the step for this edge.
			targetNode, err := s.store.GetNode(ctx, edge.TargetHash)
			if err != nil || targetNode == nil {
				continue
			}

			step := flowStep{
				Symbol:   targetNode.QualifiedName,
				EdgeType: edge.EdgeType,
			}

			newPath := make([]flowStep, len(current.path), len(current.path)+1)
			copy(newPath, current.path)
			newPath = append(newPath, step)

			// Check if we reached a target.
			if targetSet[edge.TargetHash] {
				paths = append(paths, flowPath{Steps: newPath})
				if len(paths) >= maxFlowPaths {
					truncated = true
					break
				}
				continue
			}

			// Only continue BFS if we haven't exceeded depth.
			if len(newPath) <= maxDepth {
				queue = append(queue, bfsState{
					hash: edge.TargetHash,
					path: newPath,
				})
			}
		}
	}

	// If we stopped early because queue still has items, mark truncated.
	if len(queue) > 0 && len(paths) >= maxFlowPaths {
		truncated = true
	}

	result := flowResult{
		Source:    sourceSymbol,
		Target:    targetSymbol,
		Paths:    paths,
		PathCount: len(paths),
		Truncated: truncated,
	}

	data, err := json.Marshal(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(data)), nil
}
