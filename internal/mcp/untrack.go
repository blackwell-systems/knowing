package mcp

import (
	"context"
	"fmt"

	knowingstore "github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func untrackRepoTool() mcp.Tool {
	return mcp.NewTool("untrack_repo",
		mcp.WithDescription("Remove all graph data for a repository. Evicts nodes, edges, files, snapshots, feedback, and notes. Does not remove the repository from the roster."),
		mcp.WithString("repo_url", mcp.Required(), mcp.Description("Repository URL to untrack (used to compute repo hash)"), Examples("github.com/org/repo")),
	)
}

func (s *Server) handleUntrackRepo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoURL, errResult := requireStringArg(req, "repo_url")
	if errResult != nil {
		return errResult, nil
	}

	repoHash := types.NewHash([]byte(repoURL))

	// Verify the repo exists before attempting deletion.
	repo, err := s.store.GetRepo(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error looking up repo: %v", err)), nil
	}
	if repo == nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo not found: %s (hash: %s)", repoURL, repoHash.String())), nil
	}

	// Need SQLiteStore for DeleteRepoData.
	if s.sqlStore == nil {
		return mcp.NewToolResultError("untrack_repo requires SQLite store"), nil
	}

	result, err := s.sqlStore.DeleteRepoData(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("delete failed: %v", err)), nil
	}

	summary := fmt.Sprintf(
		"Untracked %s: deleted %d nodes, %d edges, %d files, %d snapshots, %d feedback entries, %d notes",
		repoURL, result.Nodes, result.Edges, result.Files, result.Snapshots, result.Feedback, result.Notes,
	)
	return mcp.NewToolResultText(summary), nil
}

// Ensure knowingstore is used (compilation guard for parallel wave development).
var _ = (*knowingstore.SQLiteStore)(nil)
