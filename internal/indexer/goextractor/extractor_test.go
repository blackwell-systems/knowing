package goextractor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// setupTestModule creates a temp directory with a go.mod and a source file.
func setupTestModule(t *testing.T, filename, source string) (dir string, opts types.ExtractOptions) {
	t.Helper()
	dir = t.TempDir()

	modContent := "module testmodule\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	fileHash := types.NewHash([]byte(source))
	repoHash := types.NewHash([]byte("test://repo"))
	opts = types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   filename,
		FileHash:   fileHash,
		Content:    []byte(source),
		ModuleRoot: dir,
	}
	return dir, opts
}

func TestGoExtractor_CanHandle_GoFiles(t *testing.T) {
	ext := NewGoExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"pkg/foo.go", true},
		{"main_test.go", false},
		{"vendor/lib/lib.go", false},
		{"main.py", false},
		{"README.md", false},
		{"pkg/bar_test.go", false},
	}

	for _, tt := range tests {
		got := ext.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestGoExtractor_Extract_ProducesNodes(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Hello() string {
	return "hello"
}

type Greeter struct{}

var Version = "1.0"

const MaxRetries = 3
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(result.Nodes) < 4 {
		t.Fatalf("expected at least 4 nodes (func, type, var, const), got %d", len(result.Nodes))
	}

	// Check we have nodes of the right kinds.
	kindCounts := make(map[string]int)
	for _, n := range result.Nodes {
		kindCounts[n.Kind]++
	}

	if kindCounts["function"] < 1 {
		t.Error("expected at least 1 function node")
	}
	if kindCounts["type"] < 1 {
		t.Error("expected at least 1 type node")
	}
	if kindCounts["var"] < 1 {
		t.Error("expected at least 1 var node")
	}
	if kindCounts["const"] < 1 {
		t.Error("expected at least 1 const node")
	}
}

func TestGoExtractor_Extract_ProducesCallEdges(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Should have at least one call edge for fmt.Println.
	hasCallEdge := false
	for _, e := range result.Edges {
		if e.EdgeType == "calls" {
			hasCallEdge = true
			break
		}
	}
	if !hasCallEdge {
		t.Error("expected at least one 'calls' edge")
	}

	// All edges should have ast_resolved provenance.
	for _, e := range result.Edges {
		if e.Provenance != "ast_resolved" {
			t.Errorf("expected provenance 'ast_resolved', got %q", e.Provenance)
		}
		if e.Confidence != 1.0 {
			t.Errorf("expected confidence 1.0, got %f", e.Confidence)
		}
	}
}

func TestGoExtractor_Extract_QualifiedName(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Hello() {}

type Server struct{}

func (s *Server) Start() {}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Check qualified name format.
	nameSet := make(map[string]bool)
	for _, n := range result.Nodes {
		nameSet[n.QualifiedName] = true
	}

	// Function should be: test://repo://testmodule.Hello
	expectedFunc := "test://repo://testmodule.Hello"
	if !nameSet[expectedFunc] {
		t.Errorf("missing expected qualified name %q, got names: %v", expectedFunc, nameSet)
	}

	// Method should be: test://repo://testmodule.Server.Start
	expectedMethod := "test://repo://testmodule.Server.Start"
	if !nameSet[expectedMethod] {
		t.Errorf("missing expected qualified name %q, got names: %v", expectedMethod, nameSet)
	}
}

func TestGoExtractor_Extract_Deterministic(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Zebra() {}
func Apple() {}
func Mango() {}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	// Run extraction twice and verify same order.
	result1, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("first Extract failed: %v", err)
	}
	result2, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("second Extract failed: %v", err)
	}

	if len(result1.Nodes) != len(result2.Nodes) {
		t.Fatalf("node count mismatch: %d vs %d", len(result1.Nodes), len(result2.Nodes))
	}

	for i := range result1.Nodes {
		if result1.Nodes[i].QualifiedName != result2.Nodes[i].QualifiedName {
			t.Errorf("node %d name mismatch: %q vs %q",
				i, result1.Nodes[i].QualifiedName, result2.Nodes[i].QualifiedName)
		}
	}

	// Verify nodes are sorted.
	for i := 1; i < len(result1.Nodes); i++ {
		if result1.Nodes[i].QualifiedName < result1.Nodes[i-1].QualifiedName {
			t.Errorf("nodes not sorted: %q comes after %q",
				result1.Nodes[i].QualifiedName, result1.Nodes[i-1].QualifiedName)
		}
	}
}

func TestGoExtractor_CanHandle_RejectsTestFiles(t *testing.T) {
	ext := NewGoExtractor()

	testFiles := []string{
		"main_test.go",
		"pkg/handler_test.go",
		"internal/foo/bar_test.go",
	}
	for _, path := range testFiles {
		if ext.CanHandle(path) {
			t.Errorf("CanHandle(%q) should return false for test files", path)
		}
	}
}

func TestGoExtractor_Extract_ImplementsEdges(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

type Speaker interface {
	Speak() string
}

type Dog struct{}

func (d Dog) Speak() string {
	return "woof"
}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	hasImplementsEdge := false
	for _, e := range result.Edges {
		if e.EdgeType == "implements" {
			hasImplementsEdge = true
			if e.Provenance != "ast_resolved" {
				t.Errorf("implements edge should have provenance 'ast_resolved', got %q", e.Provenance)
			}
			if e.Confidence != 1.0 {
				t.Errorf("implements edge should have confidence 1.0, got %f", e.Confidence)
			}
			break
		}
	}
	if !hasImplementsEdge {
		edgeTypes := make(map[string]int)
		for _, e := range result.Edges {
			edgeTypes[e.EdgeType]++
		}
		t.Errorf("expected an 'implements' edge (Dog -> Speaker), got edge types: %v", edgeTypes)
	}
}

func TestGoExtractor_Extract_ReferencesEdges(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

var Version = "1.0"

func PrintVersion() string {
	return Version
}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	hasReferencesEdge := false
	for _, e := range result.Edges {
		if e.EdgeType == "references" {
			hasReferencesEdge = true
			if e.Provenance != "ast_resolved" {
				t.Errorf("references edge should have provenance 'ast_resolved', got %q", e.Provenance)
			}
			if e.Confidence != 1.0 {
				t.Errorf("references edge should have confidence 1.0, got %f", e.Confidence)
			}
			break
		}
	}
	if !hasReferencesEdge {
		edgeTypes := make(map[string]int)
		for _, e := range result.Edges {
			edgeTypes[e.EdgeType]++
		}
		t.Errorf("expected a 'references' edge for Version usage, got edge types: %v", edgeTypes)
	}
}
