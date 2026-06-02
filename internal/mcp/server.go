// Package mcp exposes the knowing knowledge graph as MCP (Model Context
// Protocol) tools over stdio and HTTP transports.
//
// The server registers 28 tools organized into eight planes:
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
//   - ownership_query: query ownership by file path pattern
//
// Runtime plane (runtime trace queries, requires SQLiteStore):
//   - runtime_traffic: query runtime-observed edges by service and route
//   - dead_routes: find route symbols with no recent observations
//   - trace_stats: aggregate statistics about runtime-derived edges
//
// Context plane (graph-aware context packing):
//   - context_for_task: generate token-budgeted context for a task description
//   - context_for_files: generate blast-radius context for changed files
//   - context_for_pr: generate context for a pull request (changed files between refs)
//   - explain_symbol: explain why a symbol ranked where it did for a task
//
// Feedback plane (agent learning loop):
//   - feedback: record/query symbol usefulness for ranking improvement
//
// Discovery plane (exploration and planning):
//   - test_scope: find affected tests for changed files
//   - flow_between: find all paths between two symbols via BFS
//   - plan_turn: suggest relevant knowing tools for a task description
//   - communities: Louvain modularity-based graph clustering
//
// Audit plane (integrity and proofs):
//   - prove: generate a Merkle proof that a relationship exists
//   - prove_absent: prove a relationship does NOT exist (absence proof)
//   - fsck: verify graph integrity (hashes, references, snapshot chain)
//
// Management plane (data lifecycle):
//   - untrack_repo: remove all graph data for a repository
package mcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/blackwell-systems/knowing/internal/cache"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/embedding"
	"github.com/blackwell-systems/knowing/internal/snapshot"
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
	store       types.GraphStore
	mcpServer   *mcpserver.MCPServer
	sqlStore    *knowingstore.SQLiteStore  // populated via type assertion for runtime queries
	session     *wire.Session             // GCF session state for cross-call deduplication
	ctxSession  *knowingctx.SessionTracker // session-aware retrieval boosts
	vecSearch   *embedding.Searcher        // semantic vector search (nil if model unavailable)
	taskMemory  *knowingctx.TaskMemory     // passive task-symbol learning (nil if no SQLite)
	implicit    *knowingctx.ImplicitFeedback // implicit feedback detection from tool usage
	snapMgr     *snapshot.SnapshotManager  // nil if no snapshot manager is wired in
	resultCache *cache.SubgraphCache       // nil if caching is disabled
	startTime   time.Time                  // server creation time for uptime tracking

	// Session counters for the knowing://session resource.
	contextCalls  atomic.Int64 // incremented on each context_for_task / context_for_files call
	symbolsServed atomic.Int64 // incremented by the number of symbols returned per context call
}

// NewServer creates a new MCP server backed by the given GraphStore.
// It registers all tools, prompts, and resources.
func NewServer(store types.GraphStore) *Server {
	s := &Server{
		store:      store,
		session:    wire.NewSession(),
		ctxSession: knowingctx.NewSessionTracker(),
		implicit:   knowingctx.NewImplicitFeedback(),
		startTime:  time.Now(),
	}
	// Initialize embedding-based gap-fill seeds (off by default).
	// Enable with: knowing mcp --embeddings (or KNOWING_EMBEDDINGS=1)
	// Downloads ~30MB model on first use. Gap-fill seeds use embeddings to
	// bridge vocabulary gaps when BM25 returns < 5 candidates.
	// Session 23: confirmed neutral on cold-start benchmarks (P@10 identical
	// with and without, 3 runs). Kept as opt-in for future model improvements.
	if os.Getenv("KNOWING_EMBEDDINGS") == "1" {
		model := os.Getenv("KNOWING_EMBED_MODEL")
		if model == "" {
			model = "nomic-code"
		}
		log.Printf("[knowing] Embedding gap-fill: ON (model: %s, local inference, no API calls)", model)
		if embedder, err := embedding.New(); err == nil {
			s.vecSearch = embedding.NewSearcher(embedder)
			// Attach persistent vector cache if SQLite store is available.
			if ss, ok := store.(*knowingstore.SQLiteStore); ok {
				s.vecSearch.SetStore(ss)
				// Eagerly load vectors into memory so gap-fill never blocks on SQLite.
				if n := s.vecSearch.PreloadVectors(context.Background()); n > 0 {
					log.Printf("[knowing] Pre-loaded %d embedding vectors into memory", n)
				}
			}
			go s.buildVectorIndex()
		} else {
			log.Printf("[knowing] Embedding init: FAILED (%v)", err)
		}
	} else {
		log.Printf("[knowing] Embeddings: OFF (use --embeddings to enable, currently neutral on benchmarks)")
	}
	// Try to get SQLiteStore for runtime queries.
	if ss, ok := store.(*knowingstore.SQLiteStore); ok {
		s.sqlStore = ss
		// Task memory disabled (session 24, confirmed neutral on honest measurement).
		// Implicit feedback (noise demotion) is the active learning mechanism.
		// s.taskMemory = knowingctx.NewTaskMemory(ss.DB())
	}

	// Startup summary: show the user what features are active.
	s.logStartupSummary(store)

	s.mcpServer = mcpserver.NewMCPServer(
		"knowing",
		"0.1.0",
	)
	s.registerTools()
	s.registerPrompts()
	s.registerResources()
	return s
}

// logStartupSummary prints a concise summary of active features and graph stats.
func (s *Server) logStartupSummary(store types.GraphStore) {
	ctx := context.Background()

	// Graph stats from SQLite.
	var nodes, edges, vectors int
	if ss, ok := store.(*knowingstore.SQLiteStore); ok {
		if ec, err := ss.EdgeCount(ctx); err == nil {
			edges = ec
		}
		if rc, err := ss.RealNodeCount(ctx); err == nil {
			nodes = rc
		}
		// Count pre-embedded vectors.
		var count int
		_ = ss.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM embeddings`).Scan(&count)
		vectors = count
	}

	embeddingStatus := "OFF"
	if s.vecSearch != nil {
		embeddingStatus = "ON"
	}
	gapFillStatus := "OFF"
	if s.vecSearch != nil && vectors > 0 {
		gapFillStatus = fmt.Sprintf("ON (%d vectors cached)", vectors)
	} else if s.vecSearch != nil {
		gapFillStatus = "ON (will embed on first query)"
	}

	log.Printf("[knowing] Graph: %d nodes, %d edges", nodes, edges)
	log.Printf("[knowing] Embeddings: %s | Gap-fill seeds: %s | Equivalence classes: 263", embeddingStatus, gapFillStatus)
}

// SetSnapshotManager attaches a SnapshotManager so MCP handlers can access
// the most recently computed HierarchicalTree for subgraph cache keying.
// Call this before Start if you want cache-aware blast_radius and test_scope.
func (s *Server) SetSnapshotManager(sm *snapshot.SnapshotManager) {
	s.snapMgr = sm
}

// DisableImplicitFeedback turns off the implicit feedback tracker (noise demotion).
// Use --no-feedback flag or KNOWING_NO_FEEDBACK=1 for A/B testing.
func (s *Server) DisableImplicitFeedback() {
	s.implicit = nil
}

// SetResultCache attaches a SubgraphCache for memoizing blast_radius and
// test_scope results. When set, handlers check the cache before computing
// and store results after a miss. Pass nil to disable caching.
func (s *Server) SetResultCache(c *cache.SubgraphCache) {
	s.resultCache = c
}

// registerTools registers all 28 MCP tools on the server.
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
	s.mcpServer.AddTool(ownershipQueryTool(), s.handleOwnershipQuery)

	// Runtime trace query tools
	s.mcpServer.AddTool(runtimeTrafficTool(), s.handleRuntimeTraffic)
	s.mcpServer.AddTool(deadRoutesTool(), s.handleDeadRoutes)
	s.mcpServer.AddTool(traceStatsTool(), s.handleTraceStats)

	// Context packing tools
	s.mcpServer.AddTool(contextForTaskTool(), s.handleContextForTask)
	s.mcpServer.AddTool(contextForFilesTool(), s.handleContextForFiles)
	s.mcpServer.AddTool(contextForPRTool(), s.handleContextForPR)
	s.mcpServer.AddTool(explainSymbolTool(), s.handleExplainSymbol)

	// Feedback plane
	s.mcpServer.AddTool(feedbackTool(), s.handleFeedback)

	// Discovery plane
	s.mcpServer.AddTool(testScopeTool(), s.handleTestScope)
	s.mcpServer.AddTool(flowBetweenTool(), s.handleFlowBetween)
	s.mcpServer.AddTool(planTurnTool(), s.handlePlanTurn)
	s.mcpServer.AddTool(communitiesTool(), s.handleCommunities)

	// Audit plane
	s.mcpServer.AddTool(proveTool(), s.handleProve)
	s.mcpServer.AddTool(proveAbsentTool(), s.handleProveAbsent)
	s.mcpServer.AddTool(fsckTool(), s.handleFsck)

	// Management plane
	s.mcpServer.AddTool(untrackRepoTool(), s.handleUntrackRepo)
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
}

// ObserveToolUse checks whether content from a tool call (e.g., Edit old_string,
// file path, Agent prompt) references symbols that were recently returned by
// context_for_task. If matches are found, positive feedback is auto-recorded.
//
// This implements implicit feedback: the agent doesn't need to call the feedback
// tool explicitly. Using a symbol after receiving it in context is sufficient
// signal that it was useful.
//
// Returns the number of symbols attributed.
func (s *Server) ObserveToolUse(ctx context.Context, content string) int {
	if s.implicit == nil || content == "" {
		return 0
	}

	used := s.implicit.DetectUsed(content)
	if len(used) == 0 {
		return 0
	}

	// Record positive feedback for each implicitly used symbol.
	if s.sqlStore != nil {
		for _, h := range used {
			_ = s.sqlStore.RecordFeedback(ctx, h, "implicit", true, types.EmptyHash, types.EmptyHash)
		}
	}

	return len(used)
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

// buildVectorIndex embeds all existing nodes and builds the HNSW index.
// Runs in a background goroutine at server startup. Processes in batches
// to avoid memory pressure from large embedding batches.
func (s *Server) buildVectorIndex() {
	ctx := context.Background()
	nodes, err := s.store.NodesByName(ctx, "%")
	if err != nil || len(nodes) == 0 {
		return
	}

	// Get file paths for each node (needed for embedding text).
	type nodeWithPath struct {
		node types.Node
		path string
	}
	var items []nodeWithPath
	for _, n := range nodes {
		// Skip noise (mocks, dist/, vendor/).
		qn := n.QualifiedName
		skip := false
		for _, noise := range []string{"/dist/", "/vendor/", "/node_modules/", "mock", "fake"} {
			if strings.Contains(qn, noise) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		items = append(items, nodeWithPath{node: n, path: ""})
	}

	// Embed in batches of 64.
	const batchSize = 64
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[i:end]
		batchNodes := make([]types.Node, len(batch))
		batchPaths := make([]string, len(batch))
		for j, item := range batch {
			batchNodes[j] = item.node
			batchPaths[j] = item.path
		}
		if err := s.vecSearch.IndexBatch(ctx, batchNodes, batchPaths); err != nil {
			log.Printf("[warn] vector index batch %d: %v", i/batchSize, err)
			return
		}
	}
	log.Printf("[info] vector index built: %d symbols embedded", s.vecSearch.Count())
	// Notify connected clients that semantic search is now available.
	s.mcpServer.SendNotificationToAllClients("notifications/message", map[string]any{
		"level":  "info",
		"logger": "knowing",
		"data":   fmt.Sprintf("Semantic search ready: %d symbols embedded", s.vecSearch.Count()),
	})
}

