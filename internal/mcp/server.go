// Package mcp exposes the knowing graph as MCP tools over stdio and HTTP.
package mcp

import (
	"context"
	"net/http"
	"os"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps an MCP server that exposes graph queries as tools.
type Server struct {
	store     types.GraphStore
	mcpServer *mcpserver.MCPServer
}

// NewServer creates a new MCP server backed by the given GraphStore.
// It registers all 11 tools (execution and intelligence planes).
func NewServer(store types.GraphStore) *Server {
	s := &Server{store: store}
	s.mcpServer = mcpserver.NewMCPServer(
		"knowing",
		"0.1.0",
	)
	s.registerTools()
	return s
}

// registerTools registers all 11 MCP tools on the server.
func (s *Server) registerTools() {
	// Execution plane tools
	s.mcpServer.AddTool(indexRepoTool(), s.handleIndexRepo)
	s.mcpServer.AddTool(crossRepoCallersTool(), s.handleCrossRepoCallers)
	s.mcpServer.AddTool(graphQueryTool(), s.handleGraphQuery)
	s.mcpServer.AddTool(repoGraphTool(), s.handleRepoGraph)

	// Intelligence plane tools (read-only)
	s.mcpServer.AddTool(blastRadiusTool(), s.handleBlastRadius)
	s.mcpServer.AddTool(traceDataflowTool(), s.handleTraceDataflow)
	s.mcpServer.AddTool(staleEdgesTool(), s.handleStaleEdges)
	s.mcpServer.AddTool(snapshotDiffTool(), s.handleSnapshotDiff)
	s.mcpServer.AddTool(semanticDiffTool(), s.handleSemanticDiff)
	s.mcpServer.AddTool(prImpactTool(), s.handlePRImpact)
	s.mcpServer.AddTool(ownershipTool(), s.handleOwnership)
}

// ToolNames returns the names of all registered tools, useful for testing.
func (s *Server) ToolNames() []string {
	return []string{
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
}

// ServeStdio runs the MCP server over stdin/stdout until ctx is cancelled.
func (s *Server) ServeStdio(ctx context.Context) error {
	stdio := mcpserver.NewStdioServer(s.mcpServer)
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

// ServeHTTP runs the MCP server over HTTP at the given address until ctx is cancelled.
func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	httpServer := mcpserver.NewStreamableHTTPServer(s.mcpServer)

	srv := &http.Server{
		Addr:    addr,
		Handler: httpServer,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return srv.Close()
	case err := <-errCh:
		return err
	}
}

// --- Tool definitions ---

func indexRepoTool() mcp.Tool {
	return mcp.NewTool("index_repo",
		mcp.WithDescription("Index a repository to build the knowledge graph. Records the request for the daemon to process."),
		mcp.WithString("repo_url", mcp.Required(), mcp.Description("URL of the repository to index")),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Local filesystem path to the repository")),
		mcp.WithString("commit_hash", mcp.Description("Git commit hash to index (defaults to HEAD)")),
	)
}

func crossRepoCallersTool() mcp.Tool {
	return mcp.NewTool("cross_repo_callers",
		mcp.WithDescription("Find all transitive callers of a symbol across repositories."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("target_hash", mcp.Required(), mcp.Description("Hash of the target node")),
		mcp.WithInteger("max_depth", mcp.Description("Maximum traversal depth (default 5)")),
	)
}

func graphQueryTool() mcp.Tool {
	return mcp.NewTool("graph_query",
		mcp.WithDescription("Query graph nodes by qualified name prefix."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("prefix", mcp.Required(), mcp.Description("Qualified name prefix to search for")),
	)
}

func repoGraphTool() mcp.Tool {
	return mcp.NewTool("repo_graph",
		mcp.WithDescription("Get all files and their nodes for a repository."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("repo_hash", mcp.Required(), mcp.Description("Hash of the repository")),
	)
}

func blastRadiusTool() mcp.Tool {
	return mcp.NewTool("blast_radius",
		mcp.WithDescription("Compute the blast radius of a symbol: all transitive callers grouped by repository."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("target_hash", mcp.Required(), mcp.Description("Hash of the target node")),
		mcp.WithString("snapshot_hash", mcp.Description("Snapshot hash for point-in-time query")),
	)
}

func traceDataflowTool() mcp.Tool {
	return mcp.NewTool("trace_dataflow",
		mcp.WithDescription("Trace data flow from a symbol: all transitive callees."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("source_hash", mcp.Required(), mcp.Description("Hash of the source node")),
		mcp.WithInteger("max_depth", mcp.Description("Maximum traversal depth (default 5)")),
	)
}

func staleEdgesTool() mcp.Tool {
	return mcp.NewTool("stale_edges",
		mcp.WithDescription("Find edges in the graph that are stale (no longer valid in the latest snapshot)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("snapshot_hash", mcp.Required(), mcp.Description("Snapshot hash to check for staleness")),
	)
}

func snapshotDiffTool() mcp.Tool {
	return mcp.NewTool("snapshot_diff",
		mcp.WithDescription("Compute the structural diff between two snapshots."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old snapshot")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new snapshot")),
	)
}

func semanticDiffTool() mcp.Tool {
	return mcp.NewTool("semantic_diff",
		mcp.WithDescription("Compute semantic diff between two snapshots, including added/removed nodes and edges with context."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old snapshot")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new snapshot")),
	)
}

func prImpactTool() mcp.Tool {
	return mcp.NewTool("pr_impact",
		mcp.WithDescription("Analyze the impact of changes between two snapshots, including blast radius of all changed symbols."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old (base) snapshot")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new (head) snapshot")),
	)
}

func ownershipTool() mcp.Tool {
	return mcp.NewTool("ownership",
		mcp.WithDescription("List all files and top-level symbols in a repository, useful for understanding code ownership."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("repo_hash", mcp.Required(), mcp.Description("Hash of the repository")),
	)
}
