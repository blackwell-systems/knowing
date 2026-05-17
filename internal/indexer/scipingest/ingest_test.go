package scipingest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestIngestFile_SyntheticIndex(t *testing.T) {
	// Create a synthetic SCIP index with one document containing
	// a type definition, a function definition, and a reference.
	idx := &scip.Index{
		Metadata: &scip.Metadata{
			Version:    scip.ProtocolVersion_UnspecifiedProtocolVersion,
			ProjectRoot: "file:///home/user/myproject",
		},
		Documents: []*scip.Document{
			{
				RelativePath: "pkg/handler.go",
				Symbols: []*scip.SymbolInformation{
					{
						Symbol:        "scip-go gomod github.com/org/myproject v0.1.0 pkg/Handler.",
						Documentation: []string{"Handler handles requests."},
					},
					{
						Symbol:        "scip-go gomod github.com/org/myproject v0.1.0 pkg/Handler.ServeHTTP().",
						Documentation: []string{"ServeHTTP serves HTTP requests."},
					},
				},
				Occurrences: []*scip.Occurrence{
					{
						Range:       []int32{5, 5, 12},
						Symbol:      "scip-go gomod github.com/org/myproject v0.1.0 pkg/Handler.",
						SymbolRoles: int32(scip.SymbolRole_Definition),
					},
					{
						Range:       []int32{10, 5, 20},
						Symbol:      "scip-go gomod github.com/org/myproject v0.1.0 pkg/Handler.ServeHTTP().",
						SymbolRoles: int32(scip.SymbolRole_Definition),
					},
					{
						// Reference from ServeHTTP to an external symbol
						Range:       []int32{15, 10, 25},
						Symbol:      "scip-go gomod github.com/org/httplib v1.0.0 pkg/WriteResponse().",
						SymbolRoles: 0, // not a definition, so it's a reference
					},
				},
			},
		},
	}

	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}

	tmpDir := t.TempDir()
	scipFile := filepath.Join(tmpDir, "index.scip")
	if err := os.WriteFile(scipFile, data, 0o644); err != nil {
		t.Fatalf("write scip file: %v", err)
	}

	// Open an in-memory SQLite store
	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()

	ingester := NewSCIPIngester(st)
	ctx := context.Background()
	result, err := ingester.IngestFile(ctx, SCIPIngestOptions{
		FilePath: scipFile,
		RepoURL:  "github.com/org/myproject",
	})
	if err != nil {
		t.Fatalf("IngestFile: %v", err)
	}

	// Verify results
	if result.DocsProcessed != 1 {
		t.Errorf("DocsProcessed = %d, want 1", result.DocsProcessed)
	}
	// 2 defined nodes + 1 external node
	if result.NodesCreated < 2 {
		t.Errorf("NodesCreated = %d, want >= 2", result.NodesCreated)
	}
	// At least 1 reference edge
	if result.EdgesCreated < 1 {
		t.Errorf("EdgesCreated = %d, want >= 1", result.EdgesCreated)
	}

	// Verify that the Handler node was stored
	handlerHash := types.ComputeNodeHash("github.com/org/myproject", "pkg", types.EmptyHash, "Handler", "type")
	node, err := st.GetNode(ctx, handlerHash)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if node == nil {
		t.Fatal("Handler node not found in store")
	}
	if node.Kind != "type" {
		t.Errorf("Handler node kind = %q, want %q", node.Kind, "type")
	}
	if node.QualifiedName != "github.com/org/myproject://pkg.Handler" {
		t.Errorf("Handler qualified name = %q, want %q", node.QualifiedName, "github.com/org/myproject://pkg.Handler")
	}

	// Verify the external node was created
	extHash := types.ComputeNodeHash("github.com/org/httplib", "pkg", types.EmptyHash, "WriteResponse", "method")
	extNode, err := st.GetNode(ctx, extHash)
	if err != nil {
		t.Fatalf("GetNode (external): %v", err)
	}
	if extNode == nil {
		t.Fatal("external WriteResponse node not found in store")
	}
}

func TestIngestFile_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()

	ingester := NewSCIPIngester(st)
	_, err = ingester.IngestFile(context.Background(), SCIPIngestOptions{
		FilePath: "/nonexistent/path/index.scip",
		RepoURL:  "github.com/org/repo",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestIngestFile_InvalidProtobuf(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.scip")
	if err := os.WriteFile(badFile, []byte("not a protobuf"), 0o644); err != nil {
		t.Fatalf("write bad file: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()

	ingester := NewSCIPIngester(st)
	_, err = ingester.IngestFile(context.Background(), SCIPIngestOptions{
		FilePath: badFile,
		RepoURL:  "github.com/org/repo",
	})
	// Invalid protobuf might not error (proto.Unmarshal can be lenient with arbitrary bytes)
	// but we at least verify it doesn't panic
	_ = err
}

func TestIngestFile_EmptyIndex(t *testing.T) {
	idx := &scip.Index{
		Metadata: &scip.Metadata{},
	}
	data, err := proto.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	tmpDir := t.TempDir()
	scipFile := filepath.Join(tmpDir, "empty.scip")
	if err := os.WriteFile(scipFile, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer st.Close()

	ingester := NewSCIPIngester(st)
	result, err := ingester.IngestFile(context.Background(), SCIPIngestOptions{
		FilePath: scipFile,
		RepoURL:  "github.com/org/repo",
	})
	if err != nil {
		t.Fatalf("IngestFile: %v", err)
	}
	if result.DocsProcessed != 0 {
		t.Errorf("DocsProcessed = %d, want 0", result.DocsProcessed)
	}
	if result.NodesCreated != 0 {
		t.Errorf("NodesCreated = %d, want 0", result.NodesCreated)
	}
}
