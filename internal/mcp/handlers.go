package mcp

// handlers.go implements the MCP tool handler functions. Each handler
// extracts arguments from the MCP request, calls the appropriate GraphStore
// method, and returns the result as JSON. Error handling follows the MCP
// convention: tool-level errors are returned as mcp.CallToolResult with
// IsError=true, while transport-level errors are returned as Go errors.

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/blackwell-systems/knowing/internal/diff"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// parseHash decodes a hex-encoded hash string into a types.Hash.
func parseHash(s string) (types.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.EmptyHash, fmt.Errorf("invalid hash %q: %w", s, err)
	}
	if len(b) != 32 {
		return types.EmptyHash, fmt.Errorf("invalid hash %q: expected 32 bytes, got %d", s, len(b))
	}
	var h types.Hash
	copy(h[:], b)
	return h, nil
}

// getStringArg extracts a string argument from the request, returning empty string if absent.
func getStringArg(req mcp.CallToolRequest, name string) string {
	args := req.GetArguments()
	if args == nil {
		return ""
	}
	v, ok := args[name].(string)
	if !ok {
		return ""
	}
	return v
}

// getIntArg extracts an integer argument from the request, returning defaultVal if absent.
func getIntArg(req mcp.CallToolRequest, name string, defaultVal int) int {
	args := req.GetArguments()
	if args == nil {
		return defaultVal
	}
	v, ok := args[name].(float64) // JSON numbers are float64
	if !ok {
		return defaultVal
	}
	return int(v)
}

// requireStringArg extracts a required string argument, returning an error result if missing.
func requireStringArg(req mcp.CallToolRequest, name string) (string, *mcp.CallToolResult) {
	v := getStringArg(req, name)
	if v == "" {
		return "", mcp.NewToolResultError(fmt.Sprintf("missing required argument: %s", name))
	}
	return v, nil
}

// requireHash extracts and parses a required hash argument.
func requireHash(req mcp.CallToolRequest, name string) (types.Hash, *mcp.CallToolResult) {
	s, errResult := requireStringArg(req, name)
	if errResult != nil {
		return types.EmptyHash, errResult
	}
	h, err := parseHash(s)
	if err != nil {
		return types.EmptyHash, mcp.NewToolResultError(err.Error())
	}
	return h, nil
}

// optionalHash extracts and parses an optional hash argument, returning EmptyHash if absent.
func optionalHash(req mcp.CallToolRequest, name string) (types.Hash, *mcp.CallToolResult) {
	s := getStringArg(req, name)
	if s == "" {
		return types.EmptyHash, nil
	}
	h, err := parseHash(s)
	if err != nil {
		return types.EmptyHash, mcp.NewToolResultError(err.Error())
	}
	return h, nil
}

// indexFunc is the package-level function that performs actual indexing.
// When nil, handleIndexRepo returns an error indicating no indexer is configured.
var indexFunc func(ctx context.Context, repoURL, repoPath, commitHash string) error

// SetIndexFunc sets the package-level indexing function used by the index_repo handler.
func SetIndexFunc(fn func(ctx context.Context, repoURL, repoPath, commitHash string) error) {
	indexFunc = fn
}

// --- Execution plane handlers ---

// handleIndexRepo indexes a repository using the configured indexFunc.
// Returns an error if no indexing function has been configured via SetIndexFunc.
func (s *Server) handleIndexRepo(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoURL, errResult := requireStringArg(req, "repo_url")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, errResult := requireStringArg(req, "repo_path")
	if errResult != nil {
		return errResult, nil
	}
	commitHash := getStringArg(req, "commit_hash")

	if indexFunc == nil {
		return mcp.NewToolResultError("index_repo: no indexing function configured; call SetIndexFunc first"), nil
	}

	if err := indexFunc(ctx, repoURL, repoPath, commitHash); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("index_repo failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"index_repo complete: repo_url=%s repo_path=%s commit=%s",
		repoURL, repoPath, commitHash,
	)), nil
}

// handleCrossRepoCallers finds all transitive callers of a symbol.
func (s *Server) handleCrossRepoCallers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	targetHash, errResult := requireHash(req, "target_hash")
	if errResult != nil {
		return errResult, nil
	}
	maxDepth := getIntArg(req, "max_depth", 5)

	// Use the latest snapshot from any repo for cross-repo queries.
	callers, err := s.store.TransitiveCallers(ctx, targetHash, maxDepth, types.EmptyHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("TransitiveCallers failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(callers)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleGraphQuery searches for nodes by qualified name prefix.
func (s *Server) handleGraphQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prefix, errResult := requireStringArg(req, "prefix")
	if errResult != nil {
		return errResult, nil
	}

	// Implicit feedback: querying a symbol by name is a signal the agent is
	// actively investigating it (likely received from context_for_task).
	s.ObserveToolUse(ctx, prefix)

	nodes, err := s.store.NodesByName(ctx, prefix)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("NodesByName failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(nodes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleRepoGraph returns all files and their relationships for a repository.
func (s *Server) handleRepoGraph(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoHash, errResult := requireHash(req, "repo_hash")
	if errResult != nil {
		return errResult, nil
	}

	files, err := s.store.FilesByRepo(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("FilesByRepo failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(files)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// --- Intelligence plane handlers (read-only) ---

// handleBlastRadius computes the blast radius of a symbol.
// When a SubgraphCache and SnapshotManager are wired in, results are cached
// by a key derived from hash(targetHash bytes + SubgraphRoot of the symbol's
// package). A snapshot_hash argument bypasses the cache to ensure correctness
// for point-in-time queries.
func (s *Server) handleBlastRadius(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	targetHash, errResult := requireHash(req, "target_hash")
	if errResult != nil {
		return errResult, nil
	}
	snapshotHash, errResult := optionalHash(req, "snapshot_hash")
	if errResult != nil {
		return errResult, nil
	}

	// Cache lookup: only when caching is enabled and no snapshot_hash is given
	// (point-in-time queries must bypass cache to stay correct).
	var cacheKey types.Hash
	if s.resultCache != nil && s.snapMgr != nil && snapshotHash.IsZero() {
		cacheKey = blastRadiusCacheKey(ctx, s, targetHash)
		if !cacheKey.IsZero() {
			if data, ok := s.resultCache.Get(cacheKey); ok {
				return &mcp.CallToolResult{
					Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
				}, nil
			}
		}
	}

	br, err := s.store.BlastRadius(ctx, targetHash, snapshotHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("BlastRadius failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(br)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}

	// Cache store.
	if s.resultCache != nil && !cacheKey.IsZero() && len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			s.resultCache.Put(cacheKey, []byte(tc.Text))
		}
	}

	return result, nil
}

// blastRadiusCacheKey computes a cache key for a blast_radius call by combining
// the target node hash with the subgraph root of the node's package from the
// most recently computed HierarchicalTree. Returns EmptyHash when the node
// cannot be looked up or no tree is available.
func blastRadiusCacheKey(ctx context.Context, s *Server, targetHash types.Hash) types.Hash {
	tree := s.snapMgr.LastHierarchicalTree()
	if tree == nil {
		return types.EmptyHash
	}

	// Look up the node to find its package path.
	node, err := s.store.GetNode(ctx, targetHash)
	if err != nil || node == nil {
		return types.EmptyHash
	}

	pkg := extractQualifiedPackage(node.QualifiedName)
	if pkg == "" {
		return types.EmptyHash
	}

	subgraphRoot := tree.SubgraphRoot([]string{pkg})
	// Key: sha256("blast_radius\x00" + targetHash + subgraphRoot)
	// "blast_radius" is 12 bytes, NUL is 1 byte, two hashes are 32 bytes each = 77 total.
	var buf [77]byte
	copy(buf[:12], "blast_radius")
	buf[12] = 0
	copy(buf[13:45], targetHash[:])
	copy(buf[45:77], subgraphRoot[:])
	return types.NewHash(buf[:])
}

// extractQualifiedPackage extracts the package path from a qualified node name.
// Format: "repoURL://pkg/path.SymbolName" -> "pkg/path"
func extractQualifiedPackage(qualifiedName string) string {
	parts := strings.SplitN(qualifiedName, "://", 2)
	if len(parts) < 2 {
		return ""
	}
	pkgAndSymbol := parts[1]
	lastDot := strings.LastIndex(pkgAndSymbol, ".")
	if lastDot < 0 {
		return pkgAndSymbol
	}
	pkg := pkgAndSymbol[:lastDot]
	// Strip method receiver if present: "pkg/path.Type.Method" -> "pkg/path"
	if idx := strings.LastIndex(pkg, "."); idx >= 0 {
		after := pkg[idx+1:]
		if len(after) > 0 && after[0] >= 'A' && after[0] <= 'Z' {
			pkg = pkg[:idx]
		}
	}
	return pkg
}

// handleTraceDataflow traces all transitive callees from a symbol.
func (s *Server) handleTraceDataflow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceHash, errResult := requireHash(req, "source_hash")
	if errResult != nil {
		return errResult, nil
	}
	maxDepth := getIntArg(req, "max_depth", 5)

	callees, err := s.store.TransitiveCallees(ctx, sourceHash, maxDepth, types.EmptyHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("TransitiveCallees failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(callees)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleStaleEdges finds stale edges in a snapshot.
func (s *Server) handleStaleEdges(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	snapshotHash, errResult := requireHash(req, "snapshot_hash")
	if errResult != nil {
		return errResult, nil
	}

	edges, err := s.store.StaleEdges(ctx, snapshotHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("StaleEdges failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(edges)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleSnapshotDiff computes the structural diff between two snapshots.
func (s *Server) handleSnapshotDiff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	oldHash, errResult := requireHash(req, "old_snapshot")
	if errResult != nil {
		return errResult, nil
	}
	newHash, errResult := requireHash(req, "new_snapshot")
	if errResult != nil {
		return errResult, nil
	}

	diff, err := s.store.SnapshotDiff(ctx, oldHash, newHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("SnapshotDiff failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(diff)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleSemanticDiff computes a semantic diff between two snapshots using
// the diff package for enriched output with qualified names and context.
func (s *Server) handleSemanticDiff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	oldHash, errResult := requireHash(req, "old_snapshot")
	if errResult != nil {
		return errResult, nil
	}
	newHash, errResult := requireHash(req, "new_snapshot")
	if errResult != nil {
		return errResult, nil
	}

	semDiff, err := diff.SemanticDiff(ctx, s.store, oldHash, newHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("SemanticDiff failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(semDiff)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handlePRImpact analyzes the impact of changes between two snapshots,
// computing blast radius for all changed symbols using the diff package.
func (s *Server) handlePRImpact(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	oldHash, errResult := requireHash(req, "old_snapshot")
	if errResult != nil {
		return errResult, nil
	}
	newHash, errResult := requireHash(req, "new_snapshot")
	if errResult != nil {
		return errResult, nil
	}

	impact, err := diff.PRImpact(ctx, s.store, oldHash, newHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("PRImpact failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(impact)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleOwnership lists all files and top-level symbols for a repository.
// It retrieves the repo, queries all nodes via NodesByName using the repo URL,
// groups nodes by FileHash, and pairs them with files from FilesByRepo.
func (s *Server) handleOwnership(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoHash, errResult := requireHash(req, "repo_hash")
	if errResult != nil {
		return errResult, nil
	}

	repo, err := s.store.GetRepo(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("GetRepo failed: %v", err)), nil
	}
	if repo == nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo not found: %s", repoHash)), nil
	}

	// Get all nodes for this repo using the repo URL as prefix.
	allNodes, err := s.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("NodesByName failed: %v", err)), nil
	}

	// Group nodes by their FileHash.
	nodesByFile := make(map[types.Hash][]types.Node)
	for _, n := range allNodes {
		nodesByFile[n.FileHash] = append(nodesByFile[n.FileHash], n)
	}

	// Get all files for the repo.
	files, err := s.store.FilesByRepo(ctx, repoHash)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("FilesByRepo failed: %v", err)), nil
	}

	type fileOwnership struct {
		File  types.File   `json:"file"`
		Nodes []types.Node `json:"nodes,omitempty"`
	}

	var ownership []fileOwnership
	for _, f := range files {
		ownership = append(ownership, fileOwnership{
			File:  f,
			Nodes: nodesByFile[f.FileHash],
		})
	}

	result, err := mcp.NewToolResultJSON(ownership)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// --- Runtime trace query handlers ---

// handleRuntimeTraffic returns runtime-observed edges filtered by service name
// and optional route pattern. Requires the underlying store to be a SQLiteStore.
func (s *Server) handleRuntimeTraffic(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("runtime queries not available: store does not support runtime methods"), nil
	}

	serviceName, errResult := requireStringArg(req, "service_name")
	if errResult != nil {
		return errResult, nil
	}
	routePattern := getStringArg(req, "route_pattern")
	limit := getIntArg(req, "limit", 100)

	edges, err := s.sqlStore.RuntimeEdgesByService(ctx, serviceName, routePattern, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("RuntimeEdgesByService failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(edges)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleDeadRoutes returns route symbols with no recent runtime observations.
// Requires the underlying store to be a SQLiteStore.
func (s *Server) handleDeadRoutes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("runtime queries not available: store does not support runtime methods"), nil
	}

	staleDays := getIntArg(req, "stale_days", 30)

	routes, err := s.sqlStore.DeadRoutes(ctx, staleDays)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("DeadRoutes failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(routes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}

// handleTraceStats returns aggregate statistics about runtime-derived edges.
// Requires the underlying store to be a SQLiteStore.
func (s *Server) handleTraceStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.sqlStore == nil {
		return mcp.NewToolResultError("runtime queries not available: store does not support runtime methods"), nil
	}

	stats, err := s.sqlStore.RuntimeEdgeStatsAggregate(ctx)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("RuntimeEdgeStatsAggregate failed: %v", err)), nil
	}

	result, err := mcp.NewToolResultJSON(stats)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal result: %v", err)), nil
	}
	return result, nil
}
