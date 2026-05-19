package mcp

// resources.go registers the 8 MCP resources that provide lightweight
// orientation context at zero exchange cost. Resources are read directly by
// the MCP host without a tool call, making them ideal for session-opener data
// and schema references.
//
// Registered resources:
//   - knowing://report        (P1) session opener: node/edge counts, top kinds, hotspots
//   - knowing://schema        (P1) graph schema reference: node kinds, edge types, provenance
//   - knowing://stats         (P1) detailed node/edge counts by repo, kind, and edge type
//   - knowing://repos         (P2) all tracked repos with node/edge counts per repo
//   - knowing://session       (P2) current session state: context calls, cache stats, uptime
//   - knowing://index-health  (P2) health check with integrity status and snapshot age
//   - knowing://communities   (P3) Louvain community list with metadata
//   - knowing://community/{id}(P3) single community detail (parameterized template)

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	knowingstore "github.com/blackwell-systems/knowing/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

// registerResources adds all 8 MCP resources to the server.
// Called from NewServer after tools and prompts are registered.
func (s *Server) registerResources() {
	// P1 resources
	s.mcpServer.AddResource(
		mcp.NewResource("knowing://report", "Graph Report",
			mcp.WithResourceDescription("Session opener: node/edge counts, top kinds, hotspots, and snapshot age."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceReport,
	)

	s.mcpServer.AddResource(
		mcp.NewResource("knowing://schema", "Graph Schema",
			mcp.WithResourceDescription("Graph schema reference: node kinds, edge types, provenance tiers, qualified name format, and hash format."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceSchema,
	)

	s.mcpServer.AddResource(
		mcp.NewResource("knowing://stats", "Graph Stats",
			mcp.WithResourceDescription("Detailed node/edge counts by repo, kind, and edge type."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceStats,
	)

	// P2 resources
	s.mcpServer.AddResource(
		mcp.NewResource("knowing://repos", "Tracked Repos",
			mcp.WithResourceDescription("All tracked repositories with node/edge counts and last-indexed timestamp."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceRepos,
	)

	s.mcpServer.AddResource(
		mcp.NewResource("knowing://session", "Session State",
			mcp.WithResourceDescription("Current session state: context calls, symbols served, cache hits/misses, uptime."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceSession,
	)

	s.mcpServer.AddResource(
		mcp.NewResource("knowing://index-health", "Index Health",
			mcp.WithResourceDescription("Health check: integrity status, latest snapshot age, node/edge counts."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceIndexHealth,
	)

	// P3 resources
	s.mcpServer.AddResource(
		mcp.NewResource("knowing://communities", "Community List",
			mcp.WithResourceDescription("Louvain community list with size, dominant package, cohesion, and top symbols."),
			mcp.WithMIMEType("application/json"),
		),
		s.handleResourceCommunities,
	)

	s.mcpServer.AddResourceTemplate(
		mcp.NewResourceTemplate("knowing://community/{id}", "Community Detail",
			mcp.WithTemplateDescription("Detail for a single community: members, dominant package, cohesion, packages, and cross-community connection count."),
			mcp.WithTemplateMIMEType("application/json"),
		),
		s.handleResourceCommunityByID,
	)
}

// ----- resource response types -----

// reportResource is the knowing://report payload.
type reportResource struct {
	Nodes              int64       `json:"nodes"`
	Edges              int64       `json:"edges"`
	Repos              int         `json:"repos"`
	TopKinds           []kindCount `json:"top_kinds"`
	TopLanguages       []string    `json:"top_languages"`
	Hotspots           int         `json:"hotspots"`
	SnapshotAgeSeconds int64       `json:"snapshot_age_seconds"`
}

type kindCount struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

// statsResource is the knowing://stats payload.
type statsResource struct {
	TotalNodes int64            `json:"total_nodes"`
	TotalEdges int64            `json:"total_edges"`
	ByRepo     []repoStats      `json:"by_repo"`
	ByKind     map[string]int64 `json:"by_kind"`
	ByEdgeType map[string]int64 `json:"by_edge_type"`
}

type repoStats struct {
	URL   string `json:"url"`
	Nodes int64  `json:"nodes"`
	Edges int64  `json:"edges"`
}

// reposResource is the knowing://repos payload.
type reposResource struct {
	Repos []repoEntry `json:"repos"`
}

type repoEntry struct {
	URL         string `json:"url"`
	NodeCount   int64  `json:"node_count"`
	EdgeCount   int64  `json:"edge_count"`
	LastIndexed string `json:"last_indexed"`
	LastCommit  string `json:"last_commit"`
}

// sessionResource is the knowing://session payload.
type sessionResource struct {
	ContextCalls  int64 `json:"context_calls"`
	SymbolsServed int64 `json:"symbols_served"`
	FeedbackGiven int64 `json:"feedback_given"`
	CacheHits     int64 `json:"cache_hits"`
	CacheMisses   int64 `json:"cache_misses"`
	UptimeSeconds int64 `json:"uptime_seconds"`
}

// indexHealthResource is the knowing://index-health payload.
type indexHealthResource struct {
	Status                   string `json:"status"`
	LatestSnapshotAgeSeconds int64  `json:"latest_snapshot_age_seconds"`
	NodeCount                int64  `json:"node_count"`
	EdgeCount                int64  `json:"edge_count"`
	IntegrityCheck           string `json:"integrity_check"`
}

// schemaResource is the knowing://schema payload.
type schemaResource struct {
	NodeKinds           []string         `json:"node_kinds"`
	EdgeTypes           []string         `json:"edge_types"`
	ProvenanceTiers     []provenanceTier `json:"provenance_tiers"`
	QualifiedNameFormat string           `json:"qualified_name_format"`
	HashFormat          string           `json:"hash_format"`
}

type provenanceTier struct {
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
}

// communitiesResource is the knowing://communities payload.
type communitiesResource struct {
	Communities []communityInfo `json:"communities"`
	Algorithm   string          `json:"algorithm"`
}

// communityDetailResource is the knowing://community/{id} payload.
type communityDetailResource struct {
	ID                      int      `json:"id"`
	Size                    int      `json:"size"`
	TopSymbols              []string `json:"top_symbols"`
	Cohesion                float64  `json:"cohesion"`
	DominantPackage         string   `json:"dominant_package"`
	MerkleRoot              string   `json:"merkle_root,omitempty"`
	Packages                []string `json:"packages,omitempty"`
	CrossCommunityEdgeCount int      `json:"cross_community_edge_count"`
}

// ----- helpers -----

// textResource encodes v to JSON and wraps it in a TextResourceContents.
func textResource(uri string, v any) ([]mcp.ResourceContents, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// countNodes executes a COUNT(*) query on the nodes table.
// Falls back to zero when sqlStore is unavailable.
func (s *Server) countNodes(ctx context.Context) (int64, error) {
	if s.sqlStore == nil {
		return 0, nil
	}
	var n int64
	if err := s.sqlStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// countEdges executes a COUNT(*) query on the edges table.
func (s *Server) countEdges(ctx context.Context) (int64, error) {
	if s.sqlStore == nil {
		return 0, nil
	}
	var n int64
	if err := s.sqlStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// kindCounts returns node counts grouped by kind, sorted by count descending.
func (s *Server) kindCounts(ctx context.Context) ([]kindCount, error) {
	if s.sqlStore == nil {
		return nil, nil
	}
	rows, err := s.sqlStore.DB().QueryContext(ctx,
		`SELECT kind, COUNT(*) AS cnt FROM nodes GROUP BY kind ORDER BY cnt DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []kindCount
	for rows.Next() {
		var kc kindCount
		if err := rows.Scan(&kc.Kind, &kc.Count); err != nil {
			return nil, err
		}
		out = append(out, kc)
	}
	return out, rows.Err()
}

// edgeTypeCounts returns edge counts grouped by edge_type.
func (s *Server) edgeTypeCounts(ctx context.Context) (map[string]int64, error) {
	if s.sqlStore == nil {
		return map[string]int64{}, nil
	}
	rows, err := s.sqlStore.DB().QueryContext(ctx,
		`SELECT edge_type, COUNT(*) AS cnt FROM edges GROUP BY edge_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]int64)
	for rows.Next() {
		var edgeType string
		var cnt int64
		if err := rows.Scan(&edgeType, &cnt); err != nil {
			return nil, err
		}
		out[edgeType] = cnt
	}
	return out, rows.Err()
}

// latestSnapshotAge returns the age in seconds of the most recent snapshot
// across all repos. Returns -1 when no snapshot exists.
func (s *Server) latestSnapshotAge(ctx context.Context) int64 {
	if s.sqlStore == nil {
		return -1
	}
	var ts int64
	err := s.sqlStore.DB().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(timestamp), 0) FROM snapshots`).Scan(&ts)
	if err != nil || ts == 0 {
		return -1
	}
	age := time.Now().Unix() - ts
	if age < 0 {
		return 0
	}
	return age
}

// repoNodeEdgeCounts returns per-repo node and edge counts keyed by repo URL.
// Nodes are joined via: nodes.file_hash -> files.file_hash -> files.repo_hash -> repos.
// Edges are joined via their source node.
func (s *Server) repoNodeEdgeCounts(ctx context.Context) (map[string]repoStats, error) {
	result := make(map[string]repoStats)
	if s.sqlStore == nil {
		return result, nil
	}

	// Nodes per repo via files join.
	nodeRows, err := s.sqlStore.DB().QueryContext(ctx, `
		SELECT r.repo_url, COUNT(DISTINCT n.node_hash) AS cnt
		FROM nodes n
		JOIN files f ON n.file_hash = f.file_hash
		JOIN repos r ON f.repo_hash = r.repo_hash
		GROUP BY r.repo_url
	`)
	if err != nil {
		return nil, err
	}
	defer nodeRows.Close()

	for nodeRows.Next() {
		var url string
		var cnt int64
		if err := nodeRows.Scan(&url, &cnt); err != nil {
			return nil, err
		}
		rs := result[url]
		rs.URL = url
		rs.Nodes = cnt
		result[url] = rs
	}
	if err := nodeRows.Err(); err != nil {
		return nil, err
	}

	// Edges per repo via source node -> files join.
	edgeRows, err := s.sqlStore.DB().QueryContext(ctx, `
		SELECT r.repo_url, COUNT(DISTINCT e.edge_hash) AS cnt
		FROM edges e
		JOIN nodes n ON e.source_hash = n.node_hash
		JOIN files f ON n.file_hash = f.file_hash
		JOIN repos r ON f.repo_hash = r.repo_hash
		GROUP BY r.repo_url
	`)
	if err != nil {
		return nil, err
	}
	defer edgeRows.Close()

	for edgeRows.Next() {
		var url string
		var cnt int64
		if err := edgeRows.Scan(&url, &cnt); err != nil {
			return nil, err
		}
		rs := result[url]
		rs.URL = url
		rs.Edges = cnt
		result[url] = rs
	}
	return result, edgeRows.Err()
}

// hotspotCount returns the number of nodes with above-average in-degree,
// a rough proxy for architectural hotspots.
func (s *Server) hotspotCount(ctx context.Context) int {
	if s.sqlStore == nil {
		return 0
	}
	var count int
	err := s.sqlStore.DB().QueryRowContext(ctx, `
		WITH in_degrees AS (
			SELECT target_hash, COUNT(*) AS deg FROM edges GROUP BY target_hash
		),
		avg_deg AS (SELECT AVG(deg) AS avg FROM in_degrees)
		SELECT COUNT(*) FROM in_degrees, avg_deg WHERE deg > avg
	`).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

// ----- resource handlers -----

// handleResourceReport serves knowing://report.
func (s *Server) handleResourceReport(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	nodes, err := s.countNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("count nodes: %w", err)
	}
	edges, err := s.countEdges(ctx)
	if err != nil {
		return nil, fmt.Errorf("count edges: %w", err)
	}

	repos, err := s.store.AllRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("all repos: %w", err)
	}

	kinds, err := s.kindCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("kind counts: %w", err)
	}

	// Cap top_kinds at 5.
	topKinds := kinds
	if len(topKinds) > 5 {
		topKinds = topKinds[:5]
	}
	if topKinds == nil {
		topKinds = []kindCount{}
	}

	age := s.latestSnapshotAge(ctx)
	hotspots := s.hotspotCount(ctx)

	resp := reportResource{
		Nodes:              nodes,
		Edges:              edges,
		Repos:              len(repos),
		TopKinds:           topKinds,
		TopLanguages:       []string{},
		Hotspots:           hotspots,
		SnapshotAgeSeconds: age,
	}
	return textResource("knowing://report", resp)
}

// staticSchema is the hardcoded graph schema. Values match types.go and the
// edge types used throughout the codebase.
var staticSchema = schemaResource{
	NodeKinds: []string{
		"function", "type", "method", "interface", "const", "var", "service", "route",
	},
	EdgeTypes: []string{
		"calls", "imports", "implements", "references",
		"throws", "handles_route", "runtime_calls", "runtime_rpc",
	},
	ProvenanceTiers: []provenanceTier{
		{Name: "ast_resolved", Confidence: 1.0},
		{Name: "scip_resolved", Confidence: 0.95},
		{Name: "lsp_resolved", Confidence: 0.9},
		{Name: "runtime_observed", Confidence: 0.8},
		{Name: "ast_inferred", Confidence: 0.7},
	},
	QualifiedNameFormat: "{repoURL}://{pkgPath}.{SymbolName}",
	HashFormat:          `SHA-256 with domain prefix (node\0, edge\0, snapshot\0, merkle\0)`,
}

// handleResourceSchema serves knowing://schema.
func (s *Server) handleResourceSchema(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return textResource("knowing://schema", staticSchema)
}

// handleResourceStats serves knowing://stats.
func (s *Server) handleResourceStats(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	nodes, err := s.countNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("count nodes: %w", err)
	}
	edges, err := s.countEdges(ctx)
	if err != nil {
		return nil, fmt.Errorf("count edges: %w", err)
	}

	kinds, err := s.kindCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("kind counts: %w", err)
	}
	byKind := make(map[string]int64, len(kinds))
	for _, kc := range kinds {
		byKind[kc.Kind] = kc.Count
	}

	byEdgeType, err := s.edgeTypeCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("edge type counts: %w", err)
	}

	repoCounts, err := s.repoNodeEdgeCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo counts: %w", err)
	}

	byRepo := make([]repoStats, 0, len(repoCounts))
	for _, rs := range repoCounts {
		byRepo = append(byRepo, rs)
	}
	sort.Slice(byRepo, func(i, j int) bool { return byRepo[i].URL < byRepo[j].URL })

	resp := statsResource{
		TotalNodes: nodes,
		TotalEdges: edges,
		ByRepo:     byRepo,
		ByKind:     byKind,
		ByEdgeType: byEdgeType,
	}
	return textResource("knowing://stats", resp)
}

// handleResourceRepos serves knowing://repos.
func (s *Server) handleResourceRepos(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	repos, err := s.store.AllRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("all repos: %w", err)
	}

	repoCounts, err := s.repoNodeEdgeCounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo counts: %w", err)
	}

	entries := make([]repoEntry, 0, len(repos))
	for _, r := range repos {
		rs := repoCounts[r.RepoURL]
		var lastIndexed string
		if r.LastIndexed > 0 {
			lastIndexed = time.Unix(r.LastIndexed, 0).UTC().Format(time.RFC3339)
		}
		entries = append(entries, repoEntry{
			URL:         r.RepoURL,
			NodeCount:   rs.Nodes,
			EdgeCount:   rs.Edges,
			LastIndexed: lastIndexed,
			LastCommit:  r.LastCommit,
		})
	}

	return textResource("knowing://repos", reposResource{Repos: entries})
}

// handleResourceSession serves knowing://session.
func (s *Server) handleResourceSession(_ context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	resp := sessionResource{
		ContextCalls:  s.contextCalls.Load(),
		SymbolsServed: s.symbolsServed.Load(),
	}

	if s.resultCache != nil {
		stats := s.resultCache.Stats()
		resp.CacheHits = stats.Hits
		resp.CacheMisses = stats.Misses
	}

	if !s.startTime.IsZero() {
		resp.UptimeSeconds = int64(time.Since(s.startTime).Seconds())
	}

	return textResource("knowing://session", resp)
}

// handleResourceIndexHealth serves knowing://index-health.
func (s *Server) handleResourceIndexHealth(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	nodes, err := s.countNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("count nodes: %w", err)
	}
	edges, err := s.countEdges(ctx)
	if err != nil {
		return nil, fmt.Errorf("count edges: %w", err)
	}

	age := s.latestSnapshotAge(ctx)

	// Determine status based on snapshot age.
	status := "healthy"
	switch {
	case age < 0:
		status = "no_snapshot"
	case age > 3600:
		status = "stale"
	}

	// Run integrity check if SQLiteStore is available.
	integrityResult := "ok"
	if ss, ok := s.store.(*knowingstore.SQLiteStore); ok {
		if err := ss.IntegrityCheck(ctx); err != nil {
			integrityResult = fmt.Sprintf("error: %v", err)
			status = "corrupted"
		}
	}

	resp := indexHealthResource{
		Status:                   status,
		LatestSnapshotAgeSeconds: age,
		NodeCount:                nodes,
		EdgeCount:                edges,
		IntegrityCheck:           integrityResult,
	}
	return textResource("knowing://index-health", resp)
}

// newCommunitiesReq builds a CallToolRequest for the communities tool with action=list.
func newCommunitiesListReq() mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "communities",
			Arguments: map[string]any{
				"action":    "list",
				"algorithm": "louvain",
			},
		},
	}
}

// handleResourceCommunities serves knowing://communities.
// Reuses the communities tool handler and re-wraps the result as a resource.
func (s *Server) handleResourceCommunities(ctx context.Context, _ mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	req := newCommunitiesListReq()
	result, err := s.handleCommunities(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("communities detection: %w", err)
	}
	if result.IsError {
		return nil, fmt.Errorf("communities detection returned error")
	}

	var toolResult communityResult
	if len(result.Content) > 0 {
		if tc, ok := result.Content[0].(mcp.TextContent); ok {
			if err := json.Unmarshal([]byte(tc.Text), &toolResult); err != nil {
				return nil, fmt.Errorf("parse communities result: %w", err)
			}
		}
	}

	if toolResult.Communities == nil {
		toolResult.Communities = []communityInfo{}
	}

	return textResource("knowing://communities", communitiesResource{
		Communities: toolResult.Communities,
		Algorithm:   "louvain",
	})
}

// handleResourceCommunityByID serves knowing://community/{id}.
func (s *Server) handleResourceCommunityByID(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// Extract the {id} variable from the URI.
	// URI format: knowing://community/{id}
	uri := req.Params.URI
	const prefix = "knowing://community/"
	idStr := ""
	if len(uri) > len(prefix) {
		idStr = uri[len(prefix):]
	}
	if idStr == "" {
		return nil, fmt.Errorf("missing community id in URI: %s", uri)
	}
	targetID, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid community id %q: %w", idStr, err)
	}

	// Get full community list via the tool handler.
	listReq := newCommunitiesListReq()
	listResult, err := s.handleCommunities(ctx, listReq)
	if err != nil {
		return nil, fmt.Errorf("communities detection: %w", err)
	}
	if listResult.IsError {
		return nil, fmt.Errorf("communities detection returned error")
	}

	var toolResult communityResult
	if len(listResult.Content) > 0 {
		if tc, ok := listResult.Content[0].(mcp.TextContent); ok {
			if err := json.Unmarshal([]byte(tc.Text), &toolResult); err != nil {
				return nil, fmt.Errorf("parse communities result: %w", err)
			}
		}
	}

	// Find the target community.
	var found *communityInfo
	for i := range toolResult.Communities {
		if toolResult.Communities[i].ID == targetID {
			found = &toolResult.Communities[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("community %d not found", targetID)
	}

	detail := communityDetailResource{
		ID:              found.ID,
		Size:            found.Size,
		TopSymbols:      found.TopSymbols,
		Cohesion:        found.Cohesion,
		DominantPackage: found.DominantPackage,
		MerkleRoot:      found.MerkleRoot,
		Packages:        found.Packages,
	}
	if detail.TopSymbols == nil {
		detail.TopSymbols = []string{}
	}
	if detail.Packages == nil {
		detail.Packages = []string{}
	}

	return textResource(uri, detail)
}
