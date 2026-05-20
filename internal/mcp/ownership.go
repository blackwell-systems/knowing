package mcp

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/mark3labs/mcp-go/mcp"
)

// ownershipQueryTool defines the enhanced "ownership_query" MCP tool.
// Unlike the existing "ownership" tool (which lists files and symbols),
// this tool queries owned_by and authored_by edges to answer "who owns
// this code?" queries.
func ownershipQueryTool() mcp.Tool {
	return mcp.NewTool("ownership_query",
		mcp.WithDescription("Query code ownership: find owners (from CODEOWNERS) and authors (from git blame) for a file or symbol."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("file_path", mcp.Description("File path relative to repo root"), Examples("internal/store/sqlite.go")),
		mcp.WithString("symbol", mcp.Description("Qualified symbol name to query authorship for"), Examples("github.com/org/repo://pkg.FunctionName")),
		mcp.WithString("repo_hash", mcp.Required(), mcp.Description("Hash of the repository (64-char hex)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

// ownershipQueryResult is the JSON response for the ownership_query tool.
type ownershipQueryResult struct {
	FilePath   string          `json:"file_path,omitempty"`
	Symbol     string          `json:"symbol,omitempty"`
	Codeowners []ownerEntry    `json:"codeowners"`
	Authors    []authorEntry   `json:"authors"`
}

// ownerEntry represents a CODEOWNERS-derived owner.
type ownerEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "team" or "user"
}

// authorEntry represents a git blame-derived author.
type authorEntry struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`   // always "author"
	Symbol string `json:"symbol,omitempty"`
}

// handleOwnershipQuery implements the ownership_query tool handler.
// Accepts: file_path (string) or symbol (string), repo_hash (string, required)
// Returns: JSON with owners (from owned_by edges) and authors (from authored_by edges)
func (s *Server) handleOwnershipQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoHash, errResult := requireHash(req, "repo_hash")
	if errResult != nil {
		return errResult, nil
	}

	filePath := getStringArg(req, "file_path")
	symbol := getStringArg(req, "symbol")

	if filePath == "" && symbol == "" {
		return mcp.NewToolResultError("at least one of file_path or symbol must be provided"), nil
	}

	result := ownershipQueryResult{
		FilePath:   filePath,
		Symbol:     symbol,
		Codeowners: []ownerEntry{},
		Authors:    []authorEntry{},
	}

	// If file_path is provided, look up owned_by edges for that file.
	if filePath != "" {
		file, err := s.store.FileByPath(ctx, repoHash, filePath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("FileByPath failed: %v", err)), nil
		}
		if file != nil {
			edges, err := s.store.EdgesFrom(ctx, file.FileHash, edgetype.OwnedBy)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("EdgesFrom failed: %v", err)), nil
			}
			for _, edge := range edges {
				node, err := s.store.GetNode(ctx, edge.TargetHash)
				if err != nil || node == nil {
					continue
				}
				result.Codeowners = append(result.Codeowners, ownerEntry{
					Name: node.QualifiedName,
					Kind: node.Kind,
				})
			}
		}
	}

	// If symbol is provided, look up authored_by edges for that symbol.
	if symbol != "" {
		nodes, err := s.store.NodesByQualifiedName(ctx, symbol)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("NodesByQualifiedName failed: %v", err)), nil
		}
		for _, node := range nodes {
			edges, err := s.store.EdgesFrom(ctx, node.NodeHash, edgetype.AuthoredBy)
			if err != nil {
				continue
			}
			for _, edge := range edges {
				authorNode, err := s.store.GetNode(ctx, edge.TargetHash)
				if err != nil || authorNode == nil {
					continue
				}
				result.Authors = append(result.Authors, authorEntry{
					Name:   authorNode.QualifiedName,
					Kind:   "author",
					Symbol: node.QualifiedName,
				})
			}

			// Also look up the file's owned_by edges for CODEOWNERS data.
			if !node.FileHash.IsZero() && filePath == "" {
				fileEdges, err := s.store.EdgesFrom(ctx, node.FileHash, edgetype.OwnedBy)
				if err == nil {
					for _, edge := range fileEdges {
						ownerNode, err := s.store.GetNode(ctx, edge.TargetHash)
						if err != nil || ownerNode == nil {
							continue
						}
						// Avoid duplicate entries.
						found := false
						for _, existing := range result.Codeowners {
							if existing.Name == ownerNode.QualifiedName {
								found = true
								break
							}
						}
						if !found {
							result.Codeowners = append(result.Codeowners, ownerEntry{
								Name: ownerNode.QualifiedName,
								Kind: ownerNode.Kind,
							})
						}
					}
				}
			}
		}
	}

	toolResult, err := mcp.NewToolResultJSON(result)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return toolResult, nil
}
