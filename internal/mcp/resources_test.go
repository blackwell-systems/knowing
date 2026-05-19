package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// ----- extended mock for resources tests -----

// resourceMockStore extends mockGraphStore with AllRepos support.
type resourceMockStore struct {
	mockGraphStore
	allRepos []types.Repo
}

func (m *resourceMockStore) AllRepos(_ context.Context) ([]types.Repo, error) {
	return m.allRepos, nil
}

func newResourceMockStore() *resourceMockStore {
	return &resourceMockStore{
		mockGraphStore: *newMockGraphStore(),
	}
}

// ----- helpers -----

// openTestDB opens an in-memory SQLite store for integration tests.
func openTestDB(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	ss, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	return ss
}

// readResource calls the resource handler at the given URI and returns the
// decoded TextResourceContents text.
func readResource(t *testing.T, srv *Server, uri string) string {
	t.Helper()
	// Route to the correct handler by URI prefix.
	ctx := context.Background()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	var contents []mcp.ResourceContents
	var err error

	switch {
	case uri == "knowing://report":
		contents, err = srv.handleResourceReport(ctx, req)
	case uri == "knowing://schema":
		contents, err = srv.handleResourceSchema(ctx, req)
	case uri == "knowing://stats":
		contents, err = srv.handleResourceStats(ctx, req)
	case uri == "knowing://repos":
		contents, err = srv.handleResourceRepos(ctx, req)
	case uri == "knowing://session":
		contents, err = srv.handleResourceSession(ctx, req)
	case uri == "knowing://index-health":
		contents, err = srv.handleResourceIndexHealth(ctx, req)
	case uri == "knowing://communities":
		contents, err = srv.handleResourceCommunities(ctx, req)
	default:
		contents, err = srv.handleResourceCommunityByID(ctx, req)
	}
	if err != nil {
		t.Fatalf("resource %s error: %v", uri, err)
	}
	if len(contents) == 0 {
		t.Fatalf("resource %s returned no contents", uri)
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("resource %s content is not TextResourceContents", uri)
	}
	return tc.Text
}

// ----- tests -----

// TestResourceReport_ValidJSON checks that knowing://report returns valid JSON
// with all required fields.
func TestResourceReport_ValidJSON(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	text := readResource(t, srv, "knowing://report")

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v\ntext: %s", err, text)
	}

	required := []string{"nodes", "edges", "repos", "top_kinds", "top_languages", "hotspots", "snapshot_age_seconds"}
	for _, field := range required {
		if _, ok := result[field]; !ok {
			t.Errorf("missing field %q in report", field)
		}
	}
}

// TestResourceReport_WithSQLite checks the report counts from a real SQLite store.
func TestResourceReport_WithSQLite(t *testing.T) {
	if os.Getenv("KNOWING_SLOW_TESTS") == "" && testing.Short() {
		t.Skip("skipping SQLite resource test in short mode; set KNOWING_SLOW_TESTS=1 to run")
	}
	ss := openTestDB(t)
	srv := NewServer(ss)

	ctx := context.Background()

	// Insert 2 nodes and 1 edge.
	repo := types.Repo{
		RepoHash:    types.NewHash([]byte("testrepo")),
		RepoURL:     "github.com/test/repo",
		LastCommit:  "abc123",
		LastIndexed: time.Now().Unix(),
	}
	if err := ss.PutRepo(ctx, repo); err != nil {
		t.Fatalf("PutRepo: %v", err)
	}

	file := types.File{
		FileHash:    types.NewHash([]byte("testfile")),
		RepoHash:    repo.RepoHash,
		Path:        "main.go",
		ContentHash: types.NewHash([]byte("content")),
	}
	if err := ss.PutFile(ctx, file); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	n1 := types.Node{
		NodeHash:      types.NewHash([]byte("node1")),
		FileHash:      file.FileHash,
		QualifiedName: "github.com/test/repo://main.Foo",
		Kind:          "function",
	}
	n2 := types.Node{
		NodeHash:      types.NewHash([]byte("node2")),
		FileHash:      file.FileHash,
		QualifiedName: "github.com/test/repo://main.Bar",
		Kind:          "function",
	}
	if err := ss.PutNode(ctx, n1); err != nil {
		t.Fatalf("PutNode n1: %v", err)
	}
	if err := ss.PutNode(ctx, n2); err != nil {
		t.Fatalf("PutNode n2: %v", err)
	}

	e := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge1")),
		SourceHash: n1.NodeHash,
		TargetHash: n2.NodeHash,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	}
	if err := ss.PutEdge(ctx, e); err != nil {
		t.Fatalf("PutEdge: %v", err)
	}

	text := readResource(t, srv, "knowing://report")
	var result reportResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.Nodes != 2 {
		t.Errorf("expected 2 nodes, got %d", result.Nodes)
	}
	if result.Edges != 1 {
		t.Errorf("expected 1 edge, got %d", result.Edges)
	}
	if result.Repos != 1 {
		t.Errorf("expected 1 repo, got %d", result.Repos)
	}
}

// TestResourceSchema_WellFormed checks that knowing://schema is well-formed
// and contains the expected schema fields.
func TestResourceSchema_WellFormed(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	text := readResource(t, srv, "knowing://schema")

	var result schemaResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v\ntext: %s", err, text)
	}

	// Check node kinds.
	expectedKinds := []string{"function", "type", "method", "interface", "const", "var", "service", "route"}
	if len(result.NodeKinds) != len(expectedKinds) {
		t.Errorf("expected %d node kinds, got %d", len(expectedKinds), len(result.NodeKinds))
	}
	kindSet := make(map[string]bool)
	for _, k := range result.NodeKinds {
		kindSet[k] = true
	}
	for _, k := range expectedKinds {
		if !kindSet[k] {
			t.Errorf("missing node kind %q", k)
		}
	}

	// Check edge types.
	if len(result.EdgeTypes) == 0 {
		t.Error("schema has no edge types")
	}

	// Check provenance tiers.
	if len(result.ProvenanceTiers) == 0 {
		t.Error("schema has no provenance tiers")
	}
	for _, pt := range result.ProvenanceTiers {
		if pt.Confidence <= 0 || pt.Confidence > 1.0 {
			t.Errorf("provenance tier %q has invalid confidence %f", pt.Name, pt.Confidence)
		}
	}

	// Check required string fields.
	if result.QualifiedNameFormat == "" {
		t.Error("schema missing qualified_name_format")
	}
	if result.HashFormat == "" {
		t.Error("schema missing hash_format")
	}
}

// TestResourceStats_MatchesSQLQueries checks that knowing://stats counts match
// direct SQL counts using a real SQLite store.
func TestResourceStats_MatchesSQLQueries(t *testing.T) {
	if os.Getenv("KNOWING_SLOW_TESTS") == "" && testing.Short() {
		t.Skip("skipping SQLite resource test in short mode; set KNOWING_SLOW_TESTS=1 to run")
	}
	ss := openTestDB(t)
	srv := NewServer(ss)
	ctx := context.Background()

	// Insert test data.
	repo := types.Repo{
		RepoHash:    types.NewHash([]byte("repo1")),
		RepoURL:     "github.com/test/repo",
		LastCommit:  "deadbeef",
		LastIndexed: time.Now().Unix(),
	}
	if err := ss.PutRepo(ctx, repo); err != nil {
		t.Fatalf("PutRepo: %v", err)
	}

	file := types.File{
		FileHash:    types.NewHash([]byte("file1")),
		RepoHash:    repo.RepoHash,
		Path:        "pkg/pkg.go",
		ContentHash: types.NewHash([]byte("content1")),
	}
	if err := ss.PutFile(ctx, file); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	nodes := []types.Node{
		{NodeHash: types.NewHash([]byte("fn1")), FileHash: file.FileHash, QualifiedName: "github.com/test/repo://pkg.Func1", Kind: "function"},
		{NodeHash: types.NewHash([]byte("fn2")), FileHash: file.FileHash, QualifiedName: "github.com/test/repo://pkg.Func2", Kind: "function"},
		{NodeHash: types.NewHash([]byte("tp1")), FileHash: file.FileHash, QualifiedName: "github.com/test/repo://pkg.Type1", Kind: "type"},
	}
	for _, n := range nodes {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatalf("PutNode: %v", err)
		}
	}

	edges := []types.Edge{
		{EdgeHash: types.NewHash([]byte("e1")), SourceHash: nodes[0].NodeHash, TargetHash: nodes[1].NodeHash, EdgeType: "calls", Confidence: 0.9, Provenance: "ast_resolved"},
		{EdgeHash: types.NewHash([]byte("e2")), SourceHash: nodes[1].NodeHash, TargetHash: nodes[2].NodeHash, EdgeType: "references", Confidence: 0.7, Provenance: "ast_inferred"},
	}
	for _, e := range edges {
		if err := ss.PutEdge(ctx, e); err != nil {
			t.Fatalf("PutEdge: %v", err)
		}
	}

	text := readResource(t, srv, "knowing://stats")
	var result statsResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.TotalNodes != 3 {
		t.Errorf("expected 3 total nodes, got %d", result.TotalNodes)
	}
	if result.TotalEdges != 2 {
		t.Errorf("expected 2 total edges, got %d", result.TotalEdges)
	}
	if result.ByKind["function"] != 2 {
		t.Errorf("expected 2 function nodes, got %d", result.ByKind["function"])
	}
	if result.ByKind["type"] != 1 {
		t.Errorf("expected 1 type node, got %d", result.ByKind["type"])
	}
	if result.ByEdgeType["calls"] != 1 {
		t.Errorf("expected 1 calls edge, got %d", result.ByEdgeType["calls"])
	}
	if result.ByEdgeType["references"] != 1 {
		t.Errorf("expected 1 references edge, got %d", result.ByEdgeType["references"])
	}
}

// TestResourceRepos_ListsAllRepos checks that knowing://repos includes all
// registered repositories.
func TestResourceRepos_ListsAllRepos(t *testing.T) {
	if os.Getenv("KNOWING_SLOW_TESTS") == "" && testing.Short() {
		t.Skip("skipping SQLite resource test in short mode; set KNOWING_SLOW_TESTS=1 to run")
	}
	ss := openTestDB(t)
	srv := NewServer(ss)
	ctx := context.Background()

	repoURLs := []string{"github.com/org/alpha", "github.com/org/beta"}
	for _, url := range repoURLs {
		r := types.Repo{
			RepoHash:    types.NewHash([]byte(url)),
			RepoURL:     url,
			LastCommit:  "abc",
			LastIndexed: time.Now().Unix(),
		}
		if err := ss.PutRepo(ctx, r); err != nil {
			t.Fatalf("PutRepo %s: %v", url, err)
		}
	}

	text := readResource(t, srv, "knowing://repos")
	var result reposResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(result.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(result.Repos))
	}

	urlSet := make(map[string]bool)
	for _, r := range result.Repos {
		urlSet[r.URL] = true
	}
	for _, url := range repoURLs {
		if !urlSet[url] {
			t.Errorf("missing repo %q in repos resource", url)
		}
	}
}

// TestResourceRepos_WithMockStore checks repos resource with a mock store.
func TestResourceRepos_WithMockStore(t *testing.T) {
	ms := newResourceMockStore()
	ms.allRepos = []types.Repo{
		{RepoHash: testHash("r1"), RepoURL: "github.com/x/a", LastCommit: "abc", LastIndexed: time.Now().Unix()},
		{RepoHash: testHash("r2"), RepoURL: "github.com/x/b", LastCommit: "def", LastIndexed: time.Now().Unix()},
	}
	srv := NewServer(ms)

	text := readResource(t, srv, "knowing://repos")
	var result reposResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(result.Repos))
	}
}

// TestResourceSession_ZeroCounters checks that session resource returns zeros
// before any context calls.
func TestResourceSession_ZeroCounters(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	text := readResource(t, srv, "knowing://session")

	var result sessionResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v\ntext: %s", err, text)
	}
	if result.ContextCalls != 0 {
		t.Errorf("expected 0 context_calls, got %d", result.ContextCalls)
	}
	if result.SymbolsServed != 0 {
		t.Errorf("expected 0 symbols_served, got %d", result.SymbolsServed)
	}
	if result.UptimeSeconds < 0 {
		t.Errorf("expected non-negative uptime, got %d", result.UptimeSeconds)
	}
}

// TestResourceSession_CountersIncrement checks that context call counters
// increment atomically.
func TestResourceSession_CountersIncrement(t *testing.T) {
	srv := NewServer(newResourceMockStore())

	// Manually increment counters (simulating context calls).
	srv.contextCalls.Add(3)
	srv.symbolsServed.Add(42)

	text := readResource(t, srv, "knowing://session")
	var result sessionResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if result.ContextCalls != 3 {
		t.Errorf("expected 3 context_calls, got %d", result.ContextCalls)
	}
	if result.SymbolsServed != 42 {
		t.Errorf("expected 42 symbols_served, got %d", result.SymbolsServed)
	}
}

// TestResourceIndexHealth_ValidJSON checks that knowing://index-health returns
// valid JSON with all required fields.
func TestResourceIndexHealth_ValidJSON(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	text := readResource(t, srv, "knowing://index-health")

	var result map[string]any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v\ntext: %s", err, text)
	}

	required := []string{"status", "latest_snapshot_age_seconds", "node_count", "edge_count", "integrity_check"}
	for _, field := range required {
		if _, ok := result[field]; !ok {
			t.Errorf("missing field %q in index-health", field)
		}
	}
}

// TestResourceIndexHealth_WithSQLite checks health with a real SQLite store
// (integrity_check should pass on a fresh DB).
func TestResourceIndexHealth_WithSQLite(t *testing.T) {
	if os.Getenv("KNOWING_SLOW_TESTS") == "" && testing.Short() {
		t.Skip("skipping SQLite resource test in short mode; set KNOWING_SLOW_TESTS=1 to run")
	}
	ss := openTestDB(t)
	srv := NewServer(ss)

	text := readResource(t, srv, "knowing://index-health")
	var result indexHealthResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result.IntegrityCheck != "ok" {
		t.Errorf("expected integrity_check=ok, got %q", result.IntegrityCheck)
	}
	// Fresh DB: no snapshot, so status should be "no_snapshot".
	if result.Status != "no_snapshot" && result.Status != "healthy" {
		t.Errorf("unexpected status %q for empty DB", result.Status)
	}
}

// TestResourceCommunities_ValidJSON checks that knowing://communities returns
// valid JSON with required fields.
func TestResourceCommunities_ValidJSON(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	text := readResource(t, srv, "knowing://communities")

	var result communitiesResource
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatalf("invalid JSON: %v\ntext: %s", err, text)
	}

	if result.Algorithm != "louvain" {
		t.Errorf("expected algorithm=louvain, got %q", result.Algorithm)
	}
	if result.Communities == nil {
		t.Error("communities field is nil")
	}
}

// TestResourceTextResourceContents_MIME checks that resource contents have
// the correct MIME type.
func TestResourceTextResourceContents_MIME(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	ctx := context.Background()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "knowing://schema"

	contents, err := srv.handleResourceSchema(ctx, req)
	if err != nil {
		t.Fatalf("handleResourceSchema: %v", err)
	}
	if len(contents) == 0 {
		t.Fatal("no contents returned")
	}
	tc, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatal("content is not TextResourceContents")
	}
	if tc.MIMEType != "application/json" {
		t.Errorf("expected MIMEType=application/json, got %q", tc.MIMEType)
	}
	if tc.URI != "knowing://schema" {
		t.Errorf("expected URI=knowing://schema, got %q", tc.URI)
	}
}

// TestResourceCommunityByID_InvalidID checks that an invalid community ID
// returns an error.
func TestResourceCommunityByID_InvalidID(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	ctx := context.Background()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "knowing://community/notanumber"

	_, err := srv.handleResourceCommunityByID(ctx, req)
	if err == nil {
		t.Error("expected error for invalid community id, got nil")
	}
}

// TestResourceCommunityByID_MissingID checks that a missing ID returns an error.
func TestResourceCommunityByID_MissingID(t *testing.T) {
	srv := NewServer(newResourceMockStore())
	ctx := context.Background()
	req := mcp.ReadResourceRequest{}
	req.Params.URI = "knowing://community/"

	_, err := srv.handleResourceCommunityByID(ctx, req)
	if err == nil {
		t.Error("expected error for missing community id, got nil")
	}
}

// TestRegisterResources_AddedToServer checks that registerResources wires
// resources into the MCP server (NewServer calls it).
func TestRegisterResources_AddedToServer(t *testing.T) {
	// NewServer calls registerResources; if any resource registration panics
	// (e.g., due to API mismatch), this test will fail.
	srv := NewServer(newResourceMockStore())
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}
