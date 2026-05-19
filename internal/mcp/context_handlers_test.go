package mcp

import (
	"context"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleContextForTask_MissingArg(t *testing.T) {
	ms := newMockGraphStore()
	srv := NewServer(ms)

	req := makeCallToolRequest("context_for_task", map[string]any{})

	result, err := srv.handleContextForTask(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing task_description")
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "missing required argument") {
		t.Errorf("expected 'missing required argument' in error, got: %s", text)
	}
}

func TestHandleContextForTask_Valid(t *testing.T) {
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	ctx := context.Background()

	// Seed a node that matches the keyword "refactor".
	nodeHash := testHash("refactor-func")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      nodeHash,
		QualifiedName: "refactor.ExtractMethod",
		Kind:          "function",
		Signature:     "func ExtractMethod(src string) string",
	}); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ss)

	req := makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "refactor the extraction logic",
		"token_budget":     float64(100000),
	})

	result, err := srv.handleContextForTask(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("expected success, got error: %s", text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "<context") {
		t.Errorf("expected XML context output, got: %s", text)
	}
}

func TestHandleContextForFiles_MissingArg(t *testing.T) {
	ms := newMockGraphStore()
	srv := NewServer(ms)

	req := makeCallToolRequest("context_for_files", map[string]any{})

	result, err := srv.handleContextForFiles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing files argument")
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "missing required argument") {
		t.Errorf("expected 'missing required argument' in error, got: %s", text)
	}
}

func TestHandleContextForFiles_Valid(t *testing.T) {
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	ctx := context.Background()

	// Seed a repo and file.
	repoURL := "https://github.com/example/repo"
	repoHash := types.NewHash([]byte(repoURL))
	if err := ss.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  repoURL,
	}); err != nil {
		t.Fatal(err)
	}

	fileHash := testHash("main-go-file")
	if err := ss.PutFile(ctx, types.File{
		FileHash: fileHash,
		RepoHash: repoHash,
		Path:     "cmd/main.go",
	}); err != nil {
		t.Fatal(err)
	}

	// Seed a node in that file.
	nodeHash := testHash("main-func")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      nodeHash,
		QualifiedName: "main.main",
		Kind:          "function",
		FileHash:      fileHash,
		Signature:     "func main()",
	}); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ss)

	req := makeCallToolRequest("context_for_files", map[string]any{
		"files":    "cmd/main.go",
		"repo_url": repoURL,
	})

	result, err := srv.handleContextForFiles(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("expected success, got error: %s", text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "<context") {
		t.Errorf("expected XML context output, got: %s", text)
	}
}

func TestHandleContextForTask_PackRootDedup(t *testing.T) {
	ss, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create SQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })

	ctx := context.Background()

	// Seed a node.
	nodeHash := testHash("dedup-func")
	if err := ss.PutNode(ctx, types.Node{
		NodeHash:      nodeHash,
		QualifiedName: "dedup.Handler",
		Kind:          "function",
		Signature:     "func Handler()",
	}); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(ss)

	// First call: no pack_root, should return full context.
	req1 := makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "dedup handler logic",
		"token_budget":     float64(50000),
	})
	result1, err := srv.handleContextForTask(ctx, req1)
	if err != nil {
		t.Fatal(err)
	}
	text1 := result1.Content[0].(mcp.TextContent).Text
	if strings.Contains(text1, "unchanged") {
		t.Fatal("first call should not return unchanged")
	}

	// Extract PackRoot by calling ForTask directly.
	engine := knowingctx.NewContextEngine(ss)
	block, _ := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "dedup handler logic",
		TokenBudget:     50000,
	})
	packRoot := block.PackRoot.String()

	// Second call: pass pack_root, should return "unchanged".
	req2 := makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "dedup handler logic",
		"token_budget":     float64(50000),
		"pack_root":        packRoot,
	})
	result2, err := srv.handleContextForTask(ctx, req2)
	if err != nil {
		t.Fatal(err)
	}
	text2 := result2.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text2, "unchanged") {
		t.Errorf("second call with same pack_root should return unchanged, got: %s", text2)
	}

	// Third call: pass a different pack_root, should return full context.
	req3 := makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "dedup handler logic",
		"token_budget":     float64(50000),
		"pack_root":        "0000000000000000000000000000000000000000000000000000000000000000",
	})
	result3, err := srv.handleContextForTask(ctx, req3)
	if err != nil {
		t.Fatal(err)
	}
	text3 := result3.Content[0].(mcp.TextContent).Text
	if strings.Contains(text3, "unchanged") {
		t.Error("call with different pack_root should return full context, not unchanged")
	}
}

func TestContextToolRegistration(t *testing.T) {
	ms := newMockGraphStore()
	srv := NewServer(ms)

	names := srv.ToolNames()
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	if !nameSet["context_for_task"] {
		t.Error("context_for_task not found in ToolNames()")
	}
	if !nameSet["context_for_files"] {
		t.Error("context_for_files not found in ToolNames()")
	}
}
