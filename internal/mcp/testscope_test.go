package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// seedTestScopeData inserts a repo, files, nodes (including test functions), and
// call edges into the given SQLiteStore for test_scope handler testing.
//
// Graph structure:
//
//	file: "pkg/core/engine.go" contains [Engine.Run, Engine.process]
//	file: "pkg/core/engine_test.go" contains [TestEngineRun, BenchmarkProcess]
//	file: "pkg/api/handler.go" contains [Handler.ServeHTTP]
//
//	TestEngineRun -> Engine.Run -> Engine.process
//	BenchmarkProcess -> Engine.process
//	Handler.ServeHTTP -> Engine.Run
func seedTestScopeData(t *testing.T, ss *store.SQLiteStore) {
	t.Helper()
	ctx := context.Background()

	repoHash := testHash("test-repo")
	if err := ss.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  "github.com/example/project",
	}); err != nil {
		t.Fatal(err)
	}

	// Files
	fileEngine := testHash("file-engine")
	fileEngineTest := testHash("file-engine-test")
	fileHandler := testHash("file-handler")

	if err := ss.PutFile(ctx, types.File{
		FileHash: fileEngine,
		RepoHash: repoHash,
		Path:     "pkg/core/engine.go",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutFile(ctx, types.File{
		FileHash: fileEngineTest,
		RepoHash: repoHash,
		Path:     "pkg/core/engine_test.go",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ss.PutFile(ctx, types.File{
		FileHash: fileHandler,
		RepoHash: repoHash,
		Path:     "pkg/api/handler.go",
	}); err != nil {
		t.Fatal(err)
	}

	// Nodes
	engineRun := testHash("Engine.Run")
	engineProcess := testHash("Engine.process")
	testEngineRun := testHash("TestEngineRun")
	benchProcess := testHash("BenchmarkProcess")
	handlerServe := testHash("Handler.ServeHTTP")

	nodes := []types.Node{
		{NodeHash: engineRun, FileHash: fileEngine, QualifiedName: "github.com/example/project://pkg/core.Engine.Run", Kind: "method", Line: 10},
		{NodeHash: engineProcess, FileHash: fileEngine, QualifiedName: "github.com/example/project://pkg/core.Engine.process", Kind: "method", Line: 20},
		{NodeHash: testEngineRun, FileHash: fileEngineTest, QualifiedName: "github.com/example/project://pkg/core.TestEngineRun", Kind: "function", Line: 5},
		{NodeHash: benchProcess, FileHash: fileEngineTest, QualifiedName: "github.com/example/project://pkg/core.BenchmarkProcess", Kind: "function", Line: 30},
		{NodeHash: handlerServe, FileHash: fileHandler, QualifiedName: "github.com/example/project://pkg/api.Handler.ServeHTTP", Kind: "method", Line: 15},
	}
	for _, n := range nodes {
		if err := ss.PutNode(ctx, n); err != nil {
			t.Fatal(err)
		}
	}

	// Call edges: source calls target
	// TestEngineRun -> Engine.Run
	// Engine.Run -> Engine.process
	// BenchmarkProcess -> Engine.process
	// Handler.ServeHTTP -> Engine.Run
	edges := []types.Edge{
		{EdgeHash: testHash("edge-1"), SourceHash: testEngineRun, TargetHash: engineRun, EdgeType: "calls", Confidence: 1.0, Provenance: "ast_resolved"},
		{EdgeHash: testHash("edge-2"), SourceHash: engineRun, TargetHash: engineProcess, EdgeType: "calls", Confidence: 1.0, Provenance: "ast_resolved"},
		{EdgeHash: testHash("edge-3"), SourceHash: benchProcess, TargetHash: engineProcess, EdgeType: "calls", Confidence: 1.0, Provenance: "ast_resolved"},
		{EdgeHash: testHash("edge-4"), SourceHash: handlerServe, TargetHash: engineRun, EdgeType: "calls", Confidence: 1.0, Provenance: "ast_resolved"},
	}
	for _, e := range edges {
		if err := ss.PutEdge(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTestScope_AffectedTests(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	seedTestScopeData(t, ss)
	ctx := context.Background()

	// Changing engine.go should find both TestEngineRun and BenchmarkProcess.
	req := makeCallToolRequest("test_scope", map[string]any{
		"files":  "pkg/core/engine.go",
		"output": "functions",
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	var out testScopeResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Mode != "functions" {
		t.Errorf("expected mode=functions, got %s", out.Mode)
	}

	// Should find TestEngineRun (calls Engine.Run which is in the file)
	// and BenchmarkProcess (calls Engine.process which is in the file).
	found := make(map[string]bool)
	for _, r := range out.Results {
		found[r] = true
	}
	if !found["github.com/example/project://pkg/core.TestEngineRun"] {
		t.Errorf("expected TestEngineRun in results, got %v", out.Results)
	}
	if !found["github.com/example/project://pkg/core.BenchmarkProcess"] {
		t.Errorf("expected BenchmarkProcess in results, got %v", out.Results)
	}
}

func TestTestScope_PackagesMode(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	seedTestScopeData(t, ss)
	ctx := context.Background()

	req := makeCallToolRequest("test_scope", map[string]any{
		"files":  "pkg/core/engine.go",
		"output": "packages",
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out testScopeResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Mode != "packages" {
		t.Errorf("expected mode=packages, got %s", out.Mode)
	}
	if out.Count < 1 {
		t.Errorf("expected at least 1 package, got %d", out.Count)
	}

	// Should contain pkg/core since both tests are in that package.
	found := false
	for _, r := range out.Results {
		if r == "pkg/core" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'pkg/core' in packages, got %v", out.Results)
	}
}

func TestTestScope_RunMode(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	seedTestScopeData(t, ss)
	ctx := context.Background()

	req := makeCallToolRequest("test_scope", map[string]any{
		"files":  "pkg/core/engine.go",
		"output": "run",
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out testScopeResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if out.Mode != "run" {
		t.Errorf("expected mode=run, got %s", out.Mode)
	}
	if out.Count != 1 {
		t.Errorf("expected 1 result (the regex), got %d", out.Count)
	}
	if out.Count > 0 {
		regex := out.Results[0]
		if !containsSubstring(regex, "TestEngineRun") {
			t.Errorf("expected regex to contain TestEngineRun, got %s", regex)
		}
		if !containsSubstring(regex, "BenchmarkProcess") {
			t.Errorf("expected regex to contain BenchmarkProcess, got %s", regex)
		}
	}
}

func TestTestScope_DepthLimiting(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	seedTestScopeData(t, ss)
	ctx := context.Background()

	// With depth=1, changing engine.go should find direct callers of symbols
	// in engine.go: TestEngineRun calls Engine.Run, BenchmarkProcess calls Engine.process,
	// Handler.ServeHTTP calls Engine.Run. But Handler.ServeHTTP is not a test.
	// So we should still find TestEngineRun and BenchmarkProcess at depth 1.
	req := makeCallToolRequest("test_scope", map[string]any{
		"files":  "pkg/core/engine.go",
		"output": "functions",
		"depth":  float64(1),
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out testScopeResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// At depth 1, we find direct callers of Engine.Run and Engine.process.
	// TestEngineRun -> Engine.Run (depth 1, found)
	// BenchmarkProcess -> Engine.process (depth 1, found)
	// Handler.ServeHTTP -> Engine.Run (depth 1, not a test, not included in results)
	found := make(map[string]bool)
	for _, r := range out.Results {
		found[r] = true
	}
	if !found["github.com/example/project://pkg/core.TestEngineRun"] {
		t.Errorf("expected TestEngineRun at depth 1, got %v", out.Results)
	}
	if !found["github.com/example/project://pkg/core.BenchmarkProcess"] {
		t.Errorf("expected BenchmarkProcess at depth 1, got %v", out.Results)
	}
}

func TestTestScope_MissingSQLStore(t *testing.T) {
	// Create server without SQLiteStore (using mock).
	srv := &Server{store: newMockGraphStore()}
	ctx := context.Background()

	req := makeCallToolRequest("test_scope", map[string]any{
		"files": "some/file.go",
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when sqlStore is nil")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !containsSubstring(text, "SQLiteStore") {
		t.Errorf("expected error mentioning SQLiteStore, got: %s", text)
	}
}

func TestTestScope_MultipleFiles(t *testing.T) {
	srv, ss := newTestSQLiteServer(t)
	seedTestScopeData(t, ss)
	ctx := context.Background()

	// Query both files at once.
	req := makeCallToolRequest("test_scope", map[string]any{
		"files":  "pkg/core/engine.go, pkg/api/handler.go",
		"output": "functions",
	})

	result, err := srv.handleTestScope(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var out testScopeResult
	text := result.Content[0].(mcp.TextContent).Text
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Handler.ServeHTTP is in handler.go. Its caller would need to be a test.
	// TestEngineRun and BenchmarkProcess call things in engine.go.
	// Since Handler.ServeHTTP is not a test, its callers (if any) at depth would be tests.
	// In our graph, nothing calls Handler.ServeHTTP, so we only get tests from engine.go.
	if out.Count < 2 {
		t.Errorf("expected at least 2 test functions, got %d: %v", out.Count, out.Results)
	}
}

// containsSubstring checks if s contains substr.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
