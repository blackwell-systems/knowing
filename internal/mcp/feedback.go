package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// feedbackTool defines the "feedback" MCP tool for recording and querying
// symbol usefulness feedback from agents.
func feedbackTool() mcp.Tool {
	return mcp.NewTool("feedback",
		mcp.WithDescription("Record or query symbol usefulness feedback. Use action='record' with useful=true for relevant symbols or useful=false for noise. Negative feedback (useful=false) penalizes the symbol in future rankings; positive feedback (useful=true) boosts it. The ranking formula centers around 50/50: net-positive feedback boosts, net-negative penalizes, balanced is neutral. Use action='query' to see aggregate stats."),
		mcp.WithString("action", mcp.Required(), mcp.Description("Action to perform: 'record' or 'query'"), Examples("record", "query")),
		mcp.WithString("symbol_hash", mcp.Required(), mcp.Description("Hash of the symbol (64-char hex)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithString("session_id", mcp.Description("Session identifier (required for action='record')"), Examples("session-abc123")),
		mcp.WithBoolean("useful", mcp.Description("true = symbol was relevant (boost future ranking), false = symbol was noise (penalize future ranking). Required for action='record'.")),
	)
}

// handleFeedback handles the "feedback" MCP tool requests.
func (s *Server) handleFeedback(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("feedback: not available (store does not support feedback methods)"), nil
	}

	action, errResult := requireStringArg(req, "action")
	if errResult != nil {
		return errResult, nil
	}

	symbolHashStr, errResult := requireStringArg(req, "symbol_hash")
	if errResult != nil {
		return errResult, nil
	}

	symbolHash, err := parseHash(symbolHashStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid symbol_hash: %v", err)), nil
	}

	switch action {
	case "record":
		sessionID := getStringArg(req, "session_id")
		if sessionID == "" {
			return mcp.NewToolResultError("missing required argument: session_id (required for action='record')"), nil
		}

		// Extract the boolean "useful" argument.
		args := req.GetArguments()
		usefulVal, ok := args["useful"]
		if !ok {
			return mcp.NewToolResultError("missing required argument: useful (required for action='record')"), nil
		}
		useful, ok := usefulVal.(bool)
		if !ok {
			return mcp.NewToolResultError("argument 'useful' must be a boolean"), nil
		}

		// Compute neighborhood_root for merkleized expiration (best-effort).
		neighborhoodRoot := s.computeNeighborhoodRoot(ctx, symbolHash)

		if err := s.sqlStore.RecordFeedback(ctx, symbolHash, sessionID, useful, neighborhoodRoot); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("RecordFeedback failed: %v", err)), nil
		}

		return mcp.NewToolResultText(`{"status":"recorded"}`), nil

	case "query":
		stats, err := s.sqlStore.QueryFeedback(ctx, symbolHash)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("QueryFeedback failed: %v", err)), nil
		}

		result, err := mcp.NewToolResultJSON(stats)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}
		return result, nil

	default:
		return mcp.NewToolResultError(fmt.Sprintf("unknown action %q: must be 'record' or 'query'", action)), nil
	}
}

// computeNeighborhoodRoot computes the SubgraphRoot for the package containing
// the given symbol. Returns EmptyHash if the computation fails (best-effort).
func (s *Server) computeNeighborhoodRoot(ctx context.Context, symbolHash types.Hash) types.Hash {
	if s.snapMgr == nil || s.sqlStore == nil {
		return types.EmptyHash
	}

	// Look up the symbol to get its qualified name.
	node, err := s.sqlStore.GetNode(ctx, symbolHash)
	if err != nil || node == nil {
		return types.EmptyHash
	}

	// Extract the package path from the qualified name.
	pkgPath, err := snapshot.ExtractPackagePath(node.QualifiedName)
	if err != nil {
		return types.EmptyHash
	}

	// Extract repo URL from qualified name (format: "repoURL://pkgPath.Symbol").
	sep := strings.LastIndex(node.QualifiedName, "://")
	if sep < 0 {
		return types.EmptyHash
	}
	repoURL := node.QualifiedName[:sep]
	repoHash := types.NewHash([]byte(repoURL))

	edgeInputs, _, err := s.snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		return types.EmptyHash
	}

	tree := snapshot.BuildHierarchicalTree(edgeInputs)
	return tree.SubgraphRoot([]string{pkgPath})
}
