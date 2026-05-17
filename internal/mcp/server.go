// Package mcp exposes the knowing knowledge graph as MCP (Model Context
// Protocol) tools over stdio and HTTP transports.
//
// The server registers 16 tools organized into four planes:
//
// Execution plane (write operations):
//   - index_repo: trigger indexing of a repository
//   - cross_repo_callers: find transitive callers across all repos
//   - graph_query: search nodes by qualified name prefix
//   - repo_graph: list all files in a repository
//
// Intelligence plane (read-only analytics):
//   - blast_radius: compute all transitive callers of a symbol, grouped by repo
//   - trace_dataflow: follow all transitive callees from a symbol
//   - stale_edges: find edges invalidated by file changes
//   - snapshot_diff: raw structural diff between two snapshots
//   - semantic_diff: enriched diff with summary statistics
//   - pr_impact: blast radius analysis of all symbols changed between snapshots
//   - ownership: list files and their symbols for code ownership analysis
//
// Runtime plane (runtime trace queries, requires SQLiteStore):
//   - runtime_traffic: query runtime-observed edges by service and route
//   - dead_routes: find route symbols with no recent observations
//   - trace_stats: aggregate statistics about runtime-derived edges
//
// Context plane (graph-aware context packing):
//   - context_for_task: generate token-budgeted context for a task description
//   - context_for_files: generate blast-radius context for changed files
package mcp

import (
	"context"
	"net/http"
	"os"

	knowingstore "github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/blackwell-systems/knowing/internal/wire"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// Server wraps an MCP server that exposes graph queries as tools. It holds
// a reference to the GraphStore for executing queries and delegates transport
// handling to the mcp-go library.
type Server struct {
	store     types.GraphStore
	mcpServer *mcpserver.MCPServer
	sqlStore  *knowingstore.SQLiteStore // populated via type assertion for runtime queries
	session   *wire.Session            // GCF session state for cross-call deduplication
}

// NewServer creates a new MCP server backed by the given GraphStore.
// It registers all 17 tools (execution, intelligence, runtime, and context planes).
func NewServer(store types.GraphStore) *Server {
	s := &Server{
		store:   store,
		session: wire.NewSession(),
	}
	// Try to get SQLiteStore for runtime queries.
	if ss, ok := store.(*knowingstore.SQLiteStore); ok {
		s.sqlStore = ss
	}
	s.mcpServer = mcpserver.NewMCPServer(
		"knowing",
		"0.1.0",
	)
	s.registerTools()
	s.registerPrompts()
	return s
}

// registerTools registers all 17 MCP tools on the server.
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

	// Runtime trace query tools
	s.mcpServer.AddTool(runtimeTrafficTool(), s.handleRuntimeTraffic)
	s.mcpServer.AddTool(deadRoutesTool(), s.handleDeadRoutes)
	s.mcpServer.AddTool(traceStatsTool(), s.handleTraceStats)

	// Context packing tools
	s.mcpServer.AddTool(contextForTaskTool(), s.handleContextForTask)
	s.mcpServer.AddTool(contextForFilesTool(), s.handleContextForFiles)
	s.mcpServer.AddTool(contextForPRTool(), s.handleContextForPR)
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
		"runtime_traffic",
		"dead_routes",
		"trace_stats",
		"context_for_task",
		"context_for_files",
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

// Examples returns a PropertyOption that sets JSON Schema "examples" on the property.
// This satisfies mcp-assert W103 which requires enum, pattern, example, or default.
func Examples(values ...string) mcp.PropertyOption {
	return func(m map[string]any) {
		m["examples"] = values
	}
}

// --- Tool definitions ---

func indexRepoTool() mcp.Tool {
	return mcp.NewTool("index_repo",
		mcp.WithDescription("Index a repository to build the knowledge graph. Records the request for the daemon to process."),
		mcp.WithString("repo_url", mcp.Required(), mcp.Description("URL of the repository to index (e.g. https://github.com/org/repo)"), Examples("https://github.com/org/repo")),
		mcp.WithString("repo_path", mcp.Required(), mcp.Description("Local filesystem path to the repository (e.g. /home/user/code/my-repo)"), Examples("/home/user/code/my-repo")),
		mcp.WithString("commit_hash", mcp.Description("Git commit hash to index, defaults to HEAD (e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

func crossRepoCallersTool() mcp.Tool {
	return mcp.NewTool("cross_repo_callers",
		mcp.WithDescription("Find all transitive callers of a symbol across repositories."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("target_hash", mcp.Required(), mcp.Description("Hash of the target node (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithInteger("max_depth", mcp.Description("Maximum traversal depth (default 5)")),
	)
}

func graphQueryTool() mcp.Tool {
	return mcp.NewTool("graph_query",
		mcp.WithDescription("Query graph nodes by qualified name prefix."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("prefix", mcp.Required(), mcp.Description("Qualified name prefix to search for (e.g. github.com/org/repo://pkg.FunctionName)"), Examples("github.com/org/repo://pkg.FunctionName")),
	)
}

func repoGraphTool() mcp.Tool {
	return mcp.NewTool("repo_graph",
		mcp.WithDescription("Get all files and their nodes for a repository."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("repo_hash", mcp.Required(), mcp.Description("Hash of the repository (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

func blastRadiusTool() mcp.Tool {
	return mcp.NewTool("blast_radius",
		mcp.WithDescription("Compute the blast radius of a symbol: all transitive callers grouped by repository."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("target_hash", mcp.Required(), mcp.Description("Hash of the target node (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithString("snapshot_hash", mcp.Description("Snapshot hash for point-in-time query (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

func traceDataflowTool() mcp.Tool {
	return mcp.NewTool("trace_dataflow",
		mcp.WithDescription("Trace data flow from a symbol: all transitive callees."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("source_hash", mcp.Required(), mcp.Description("Hash of the source node (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithInteger("max_depth", mcp.Description("Maximum traversal depth (default 5)")),
	)
}

func staleEdgesTool() mcp.Tool {
	return mcp.NewTool("stale_edges",
		mcp.WithDescription("Find edges in the graph that are stale (no longer valid in the latest snapshot)."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("snapshot_hash", mcp.Required(), mcp.Description("Snapshot hash to check for staleness (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

func snapshotDiffTool() mcp.Tool {
	return mcp.NewTool("snapshot_diff",
		mcp.WithDescription("Compute the structural diff between two snapshots."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4")),
	)
}

func semanticDiffTool() mcp.Tool {
	return mcp.NewTool("semantic_diff",
		mcp.WithDescription("Compute semantic diff between two snapshots, including added/removed nodes and edges with context."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4")),
	)
}

func prImpactTool() mcp.Tool {
	return mcp.NewTool("pr_impact",
		mcp.WithDescription("Analyze the impact of changes between two snapshots, including blast radius of all changed symbols."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("old_snapshot", mcp.Required(), mcp.Description("Hash of the old (base) snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
		mcp.WithString("new_snapshot", mcp.Required(), mcp.Description("Hash of the new (head) snapshot (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4")),
	)
}

func ownershipTool() mcp.Tool {
	return mcp.NewTool("ownership",
		mcp.WithDescription("List all files and top-level symbols in a repository, useful for understanding code ownership."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("repo_hash", mcp.Required(), mcp.Description("Hash of the repository (64-char hex, e.g. a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2)"), Examples("a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2")),
	)
}

func runtimeTrafficTool() mcp.Tool {
	return mcp.NewTool("runtime_traffic",
		mcp.WithDescription("Query runtime-observed edges filtered by service name and optional route pattern. Returns edges with observation counts from OTLP trace data."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithString("service_name", mcp.Required(), mcp.Description("Service name to filter edges by (e.g. user-service)"), Examples("user-service")),
		mcp.WithString("route_pattern", mcp.Description("Optional route pattern filter, SQL LIKE syntax (e.g. /api/v1/%)"), Examples("/api/v1/%")),
		mcp.WithInteger("limit", mcp.Description("Maximum number of edges to return (default 100)")),
	)
}

func deadRoutesTool() mcp.Tool {
	return mcp.NewTool("dead_routes",
		mcp.WithDescription("Find route symbols that have no runtime observations in the specified number of days, indicating potentially dead routes."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithInteger("stale_days", mcp.Description("Number of days without observations to consider a route dead (default 30)")),
	)
}

func traceStatsTool() mcp.Tool {
	return mcp.NewTool("trace_stats",
		mcp.WithDescription("Get aggregate statistics about runtime-derived edges, including counts of active, stale, and GC-eligible edges by type."),
		mcp.WithReadOnlyHintAnnotation(true),
	)
}
