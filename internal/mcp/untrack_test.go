package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleUntrackRepo_MissingArg(t *testing.T) {
	store := newMockGraphStore()
	srv := NewServer(store)

	req := makeCallToolRequest("untrack_repo", map[string]any{})
	result, err := srv.handleUntrackRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo_url")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "missing required argument") {
		t.Errorf("expected missing argument error, got: %s", text)
	}
}

func TestHandleUntrackRepo_RepoNotFound(t *testing.T) {
	store := newMockGraphStore()
	srv := NewServer(store)

	req := makeCallToolRequest("untrack_repo", map[string]any{
		"repo_url": "github.com/nonexistent/repo",
	})
	result, err := srv.handleUntrackRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "repo not found") {
		t.Errorf("expected 'repo not found' error, got: %s", text)
	}
	// Verify the hash is included in the error message.
	repoHash := types.NewHash([]byte("github.com/nonexistent/repo"))
	if !strings.Contains(text, repoHash.String()) {
		t.Errorf("expected hash %s in error, got: %s", repoHash.String(), text)
	}
}

func TestHandleUntrackRepo_NoSQLiteStore(t *testing.T) {
	store := newMockGraphStore()
	// Register a repo so the "not found" check passes.
	repoURL := "github.com/test/repo"
	repoHash := types.NewHash([]byte(repoURL))
	store.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  repoURL,
	}

	srv := NewServer(store)
	// sqlStore is nil because we used a mock store (not *SQLiteStore).

	req := makeCallToolRequest("untrack_repo", map[string]any{
		"repo_url": repoURL,
	})
	result, err := srv.handleUntrackRepo(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when sqlStore is nil")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "requires SQLite store") {
		t.Errorf("expected 'requires SQLite store' error, got: %s", text)
	}
}

func TestHandleUntrackRepo_WithSQLiteStore(t *testing.T) {
	// This test uses the real SQLiteStore and will only compile after
	// Agent A's DeleteRepoData method is merged. The test verifies the
	// full happy path with actual data insertion and deletion.

	sqlStore, err := newTestSQLiteStore(t)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer sqlStore.Close()

	ctx := context.Background()
	repoURL := "github.com/test/repo"
	repoHash := types.NewHash([]byte(repoURL))

	// Insert test data: repo, file, node, edge, snapshot.
	repo := types.Repo{
		RepoHash: repoHash,
		RepoURL:  repoURL,
	}
	if err := sqlStore.PutRepo(ctx, repo); err != nil {
		t.Fatalf("PutRepo: %v", err)
	}

	fileHash := types.NewHash([]byte("file1"))
	file := types.File{
		FileHash: fileHash,
		RepoHash: repoHash,
		Path:     "pkg/main.go",
	}
	if err := sqlStore.PutFile(ctx, file); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	nodeHash := types.NewHash([]byte("node1"))
	node := types.Node{
		NodeHash:      nodeHash,
		QualifiedName: repoURL + "://pkg.Main",
		Kind:          "function",
		FileHash:      fileHash,
	}
	if err := sqlStore.PutNode(ctx, node); err != nil {
		t.Fatalf("PutNode: %v", err)
	}

	edgeHash := types.NewHash([]byte("edge1"))
	edge := types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: nodeHash,
		TargetHash: types.NewHash([]byte("other-node")),
		EdgeType:   "calls",
	}
	if err := sqlStore.PutEdge(ctx, edge); err != nil {
		t.Fatalf("PutEdge: %v", err)
	}

	snap := types.Snapshot{
		SnapshotHash: types.NewHash([]byte("snap1")),
		RepoHash:     repoHash,
	}
	if err := sqlStore.CreateSnapshot(ctx, snap); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// Create server backed by the SQLiteStore.
	srv := NewServer(sqlStore)

	req := makeCallToolRequest("untrack_repo", map[string]any{
		"repo_url": repoURL,
	})
	result, err := srv.handleUntrackRepo(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		text := result.Content[0].(mcp.TextContent).Text
		t.Fatalf("expected success, got error: %s", text)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Untracked github.com/test/repo") {
		t.Errorf("expected untrack summary, got: %s", text)
	}
	// Verify counts are present.
	if !strings.Contains(text, "deleted") {
		t.Errorf("expected 'deleted' in summary, got: %s", text)
	}

	// Verify data is actually gone.
	files, err := sqlStore.FilesByRepo(ctx, repoHash)
	if err != nil {
		t.Fatalf("FilesByRepo: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files after untrack, got %d", len(files))
	}

	// Verify repo entry still exists (untrack does not remove from roster).
	repoResult, err := sqlStore.GetRepo(ctx, repoHash)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	if repoResult == nil {
		t.Error("expected repo to still exist in roster after untrack")
	}
}

// newTestSQLiteStore creates an in-memory SQLiteStore for testing.
func newTestSQLiteStore(t *testing.T) (*store.SQLiteStore, error) {
	t.Helper()
	return store.NewSQLiteStore(":memory:")
}
