package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// ownershipMockStore extends mockGraphStore with ownership-specific behavior.
type ownershipMockStore struct {
	mockGraphStore
	nodesByQualifiedName map[string][]types.Node
}

func newOwnershipMockStore() *ownershipMockStore {
	return &ownershipMockStore{
		mockGraphStore:       *newMockGraphStore(),
		nodesByQualifiedName: make(map[string][]types.Node),
	}
}

func (m *ownershipMockStore) NodesByQualifiedName(_ context.Context, name string) ([]types.Node, error) {
	return m.nodesByQualifiedName[name], nil
}

func TestHandleOwnershipQuery_ByFilePath(t *testing.T) {
	store := newOwnershipMockStore()

	repoHash := testHash("repo1")
	fileHash := testHash("file1")
	ownerHash := testHash("owner-backend")

	// Set up file in store.
	store.filesByRepo[repoHash] = []types.File{
		{FileHash: fileHash, RepoHash: repoHash, Path: "internal/store/sqlite.go"},
	}

	// Set up owner node.
	ownerNode := types.Node{
		NodeHash:      ownerHash,
		QualifiedName: "https://github.com/example/repo://owners/org/backend-team",
		Kind:          "team",
	}
	store.nodes[ownerHash] = &ownerNode

	// Set up owned_by edge from file to owner.
	edgeHash := testHash("edge-owned-by")
	store.edgesFrom[fileHash] = []types.Edge{
		{
			EdgeHash:   edgeHash,
			SourceHash: fileHash,
			TargetHash: ownerHash,
			EdgeType:   "owned_by",
			Confidence: 1.0,
			Provenance: "codeowners",
		},
	}

	srv := NewServer(store)
	req := makeCallToolRequest("ownership_query", map[string]any{
		"repo_hash": hashHex(repoHash),
		"file_path": "internal/store/sqlite.go",
	})

	result, err := srv.handleOwnershipQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp ownershipQueryResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if resp.FilePath != "internal/store/sqlite.go" {
		t.Errorf("file_path = %q, want %q", resp.FilePath, "internal/store/sqlite.go")
	}
	if len(resp.Codeowners) != 1 {
		t.Fatalf("expected 1 codeowner, got %d", len(resp.Codeowners))
	}
	if resp.Codeowners[0].Kind != "team" {
		t.Errorf("codeowner kind = %q, want %q", resp.Codeowners[0].Kind, "team")
	}
	if resp.Codeowners[0].Name != "https://github.com/example/repo://owners/org/backend-team" {
		t.Errorf("codeowner name = %q, unexpected", resp.Codeowners[0].Name)
	}
}

func TestHandleOwnershipQuery_BySymbol(t *testing.T) {
	store := newOwnershipMockStore()

	repoHash := testHash("repo1")
	symbolHash := testHash("symbol-foo")
	fileHash := testHash("file1")
	authorHash := testHash("author-alice")
	ownerHash := testHash("owner-team")

	// Set up symbol node.
	symbolNode := types.Node{
		NodeHash:      symbolHash,
		QualifiedName: "https://github.com/example/repo://pkg.Foo",
		Kind:          "function",
		FileHash:      fileHash,
	}
	store.nodesByQualifiedName["https://github.com/example/repo://pkg.Foo"] = []types.Node{symbolNode}
	store.nodes[symbolHash] = &symbolNode

	// Set up author node.
	authorNode := types.Node{
		NodeHash:      authorHash,
		QualifiedName: "alice",
		Kind:          "author",
	}
	store.nodes[authorHash] = &authorNode

	// Set up owner node (for file's CODEOWNERS).
	ownerNode := types.Node{
		NodeHash:      ownerHash,
		QualifiedName: "https://github.com/example/repo://owners/org/backend-team",
		Kind:          "team",
	}
	store.nodes[ownerHash] = &ownerNode

	// Set up authored_by edge from symbol to author.
	store.edgesFrom[symbolHash] = []types.Edge{
		{
			EdgeHash:   testHash("edge-authored-by"),
			SourceHash: symbolHash,
			TargetHash: authorHash,
			EdgeType:   "authored_by",
			Confidence: 0.9,
			Provenance: "git_blame",
		},
	}

	// Set up owned_by edge from file to owner.
	store.edgesFrom[fileHash] = []types.Edge{
		{
			EdgeHash:   testHash("edge-owned-by"),
			SourceHash: fileHash,
			TargetHash: ownerHash,
			EdgeType:   "owned_by",
			Confidence: 1.0,
			Provenance: "codeowners",
		},
	}

	srv := NewServer(store)
	req := makeCallToolRequest("ownership_query", map[string]any{
		"repo_hash": hashHex(repoHash),
		"symbol":    "https://github.com/example/repo://pkg.Foo",
	})

	result, err := srv.handleOwnershipQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp ownershipQueryResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(resp.Authors) != 1 {
		t.Fatalf("expected 1 author, got %d", len(resp.Authors))
	}
	if resp.Authors[0].Name != "alice" {
		t.Errorf("author name = %q, want %q", resp.Authors[0].Name, "alice")
	}
	if resp.Authors[0].Kind != "author" {
		t.Errorf("author kind = %q, want %q", resp.Authors[0].Kind, "author")
	}
	if resp.Authors[0].Symbol != "https://github.com/example/repo://pkg.Foo" {
		t.Errorf("author symbol = %q, unexpected", resp.Authors[0].Symbol)
	}

	// Should also include CODEOWNERS from the file
	if len(resp.Codeowners) != 1 {
		t.Fatalf("expected 1 codeowner, got %d", len(resp.Codeowners))
	}
	if resp.Codeowners[0].Kind != "team" {
		t.Errorf("codeowner kind = %q, want %q", resp.Codeowners[0].Kind, "team")
	}
}

func TestHandleOwnershipQuery_NoEdges(t *testing.T) {
	store := newOwnershipMockStore()

	repoHash := testHash("repo1")
	fileHash := testHash("file1")

	// Set up file with no ownership edges.
	store.filesByRepo[repoHash] = []types.File{
		{FileHash: fileHash, RepoHash: repoHash, Path: "cmd/main.go"},
	}
	// edgesFrom[fileHash] is empty (no owned_by edges)

	srv := NewServer(store)
	req := makeCallToolRequest("ownership_query", map[string]any{
		"repo_hash": hashHex(repoHash),
		"file_path": "cmd/main.go",
	})

	result, err := srv.handleOwnershipQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp ownershipQueryResult
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(resp.Codeowners) != 0 {
		t.Errorf("expected 0 codeowners, got %d", len(resp.Codeowners))
	}
	if len(resp.Authors) != 0 {
		t.Errorf("expected 0 authors, got %d", len(resp.Authors))
	}
}

func TestHandleOwnershipQuery_MissingArgs(t *testing.T) {
	store := newOwnershipMockStore()
	srv := NewServer(store)

	// Missing repo_hash entirely
	req := makeCallToolRequest("ownership_query", map[string]any{
		"file_path": "internal/store/sqlite.go",
	})

	result, err := srv.handleOwnershipQuery(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return an error result for missing required arg
	if !result.IsError {
		t.Fatalf("expected error result for missing repo_hash, got success")
	}
}
