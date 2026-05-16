package gotsextractor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// setupTestModule creates a temp directory with a go.mod and source file,
// then returns ExtractOptions ready for testing.
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

func TestGoTreeSitterExtractor_Name(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	if got := ext.Name(); got != "go-treesitter" {
		t.Errorf("Name() = %q, want %q", got, "go-treesitter")
	}
}

func TestGoTreeSitterExtractor_CanHandle(t *testing.T) {
	ext := NewGoTreeSitterExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"pkg/foo.go", true},
		{"internal/bar.go", true},
		{"main_test.go", false},
		{"pkg/bar_test.go", false},
		{"vendor/lib/lib.go", false},
		{"some/vendor/pkg.go", false},
		{"main.py", false},
		{"README.md", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ext.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestGoTreeSitterExtractor_ExtractFunctions(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

func Hello() string {
	return "hello"
}

func Goodbye() {
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}

	// Nodes are sorted by QualifiedName.
	goodbye := result.Nodes[0]
	hello := result.Nodes[1]

	if goodbye.Kind != "function" {
		t.Errorf("Goodbye kind = %q, want %q", goodbye.Kind, "function")
	}
	if !strings.HasSuffix(goodbye.QualifiedName, "testmodule.Goodbye") {
		t.Errorf("Goodbye QualifiedName = %q, want suffix testmodule.Goodbye", goodbye.QualifiedName)
	}

	if hello.Kind != "function" {
		t.Errorf("Hello kind = %q, want %q", hello.Kind, "function")
	}
	if hello.Signature != "func Hello()" {
		t.Errorf("Hello Signature = %q, want %q", hello.Signature, "func Hello()")
	}
}

func TestGoTreeSitterExtractor_ExtractMethods(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

type Server struct{}

func (s *Server) Start() {}
func (s Server) Stop() {}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Expect: Server (type), Start (method), Stop (method)
	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}

	var methods []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "method" {
			methods = append(methods, n)
		}
	}
	if len(methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(methods))
	}

	// Both methods should have Server in the qualified name.
	for _, m := range methods {
		if !strings.Contains(m.QualifiedName, ".Server.") {
			t.Errorf("method QualifiedName %q should contain .Server.", m.QualifiedName)
		}
	}

	// Check signature format.
	for _, m := range methods {
		if !strings.HasPrefix(m.Signature, "func (Server) ") {
			t.Errorf("method Signature %q should start with 'func (Server) '", m.Signature)
		}
	}
}

func TestGoTreeSitterExtractor_ExtractTypes(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

type MyStruct struct {
	Name string
}

type MyInterface interface {
	DoThing()
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}

	kindMap := make(map[string]string)
	for _, n := range result.Nodes {
		parts := strings.Split(n.QualifiedName, ".")
		name := parts[len(parts)-1]
		kindMap[name] = n.Kind
	}

	if kindMap["MyStruct"] != "type" {
		t.Errorf("MyStruct kind = %q, want %q", kindMap["MyStruct"], "type")
	}
	if kindMap["MyInterface"] != "interface" {
		t.Errorf("MyInterface kind = %q, want %q", kindMap["MyInterface"], "interface")
	}
}

func TestGoTreeSitterExtractor_ExtractConstsAndVars(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

const MaxRetries = 3

var Version = "1.0"
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}

	kindMap := make(map[string]string)
	for _, n := range result.Nodes {
		parts := strings.Split(n.QualifiedName, ".")
		name := parts[len(parts)-1]
		kindMap[name] = n.Kind
	}

	if kindMap["MaxRetries"] != "const" {
		t.Errorf("MaxRetries kind = %q, want %q", kindMap["MaxRetries"], "const")
	}
	if kindMap["Version"] != "var" {
		t.Errorf("Version kind = %q, want %q", kindMap["Version"], "var")
	}
}

func TestGoTreeSitterExtractor_ExtractCallEdges(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import "fmt"

func Hello() {
	fmt.Println("hello")
	localFunc()
}

func localFunc() {}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Filter call edges.
	var callEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "calls" {
			callEdges = append(callEdges, e)
		}
	}

	if len(callEdges) != 2 {
		t.Fatalf("expected 2 call edges, got %d", len(callEdges))
	}

	// All call edges should have confidence 0.7 and provenance "ast_inferred".
	for _, e := range callEdges {
		if e.Confidence != 0.7 {
			t.Errorf("call edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("call edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestGoTreeSitterExtractor_ExtractImportEdges(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Args)
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Filter import edges.
	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}

	if len(importEdges) != 2 {
		t.Fatalf("expected 2 import edges, got %d", len(importEdges))
	}

	// All import edges should have confidence 0.7 and provenance "ast_inferred".
	for _, e := range importEdges {
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestGoTreeSitterExtractor_EdgeProvenanceAndConfidence(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import "fmt"

func Hello() {
	fmt.Println("hi")
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, e := range result.Edges {
		if e.Confidence != 0.7 {
			t.Errorf("edge %s confidence = %v, want 0.7", e.EdgeType, e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge %s provenance = %q, want %q", e.EdgeType, e.Provenance, "ast_inferred")
		}
	}
}

func TestGoTreeSitterExtractor_SubdirectoryPackagePath(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package sub

func Foo() {}
`
	dir := t.TempDir()
	subDir := filepath.Join(dir, "pkg", "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	modContent := "module testmodule\n\ngo 1.23\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(modContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "sub.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   types.NewHash([]byte("test://repo")),
		CommitHash: "abc123",
		FilePath:   "pkg/sub/sub.go",
		FileHash:   types.NewHash([]byte(source)),
		Content:    []byte(source),
		ModuleRoot: dir,
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}

	want := "test://repo://testmodule/pkg/sub.Foo"
	if result.Nodes[0].QualifiedName != want {
		t.Errorf("QualifiedName = %q, want %q", result.Nodes[0].QualifiedName, want)
	}
}

func TestGoTreeSitterExtractor_ImportAliasResolution(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import (
	myfmt "fmt"
)

func Hello() {
	myfmt.Println("hello")
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// The call edge should resolve "myfmt" to "fmt" via the import alias.
	var callEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "calls" {
			callEdges = append(callEdges, e)
		}
	}
	if len(callEdges) != 1 {
		t.Fatalf("expected 1 call edge, got %d", len(callEdges))
	}

	// Compute the expected target hash using "fmt" as the package path.
	expectedTarget := types.ComputeNodeHash("stdlib", "fmt", types.EmptyHash, "Println", "function")
	if callEdges[0].TargetHash != expectedTarget {
		t.Errorf("call target hash mismatch: alias was not resolved to 'fmt'")
	}
}

func TestGoTreeSitterExtractor_EmptyFile(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

// testReadCloser wraps a string reader to implement io.ReadCloser.
type testReadCloser struct {
	io.Reader
}

func (t testReadCloser) Close() error { return nil }

func TestExtractRouteSymbols_HttpHandleFunc(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import "net/http"

func setupRoutes() {
	http.HandleFunc("/health", healthHandler)
	http.Handle("/api/v1", apiHandler)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {}
func apiHandler(w http.ResponseWriter, r *http.Request) {}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Find route_handler nodes.
	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes, got %d", len(routeNodes))
	}

	// Check that route patterns are present.
	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["ANY /health"] {
		t.Errorf("missing route pattern 'ANY /health', got %v", patterns)
	}
	if !patterns["ANY /api/v1"] {
		t.Errorf("missing route pattern 'ANY /api/v1', got %v", patterns)
	}

	// Check handles_route edges exist.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}
	if len(routeEdges) != 2 {
		t.Fatalf("expected 2 handles_route edges, got %d", len(routeEdges))
	}

	// Verify edge properties.
	for _, e := range routeEdges {
		if e.Confidence != 0.7 {
			t.Errorf("handles_route edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("handles_route edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestExtractRouteSymbols_ChiRouter(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import "github.com/go-chi/chi/v5"

func setupRoutes() {
	r := chi.NewRouter()
	r.Get("/users", listUsers)
	r.Post("/users", createUser)
	r.Delete("/users/{id}", deleteUser)
}

func listUsers() {}
func createUser() {}
func deleteUser() {}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 3 {
		t.Fatalf("expected 3 route_handler nodes, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /users"] {
		t.Errorf("missing route pattern 'GET /users', got %v", patterns)
	}
	if !patterns["POST /users"] {
		t.Errorf("missing route pattern 'POST /users', got %v", patterns)
	}
	if !patterns["DELETE /users/{id}"] {
		t.Errorf("missing route pattern 'DELETE /users/{id}', got %v", patterns)
	}

	// Verify handles_route edges.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}
	if len(routeEdges) != 3 {
		t.Fatalf("expected 3 handles_route edges, got %d", len(routeEdges))
	}
}

func TestExtractRouteSymbols_NoRoutes(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import "fmt"

func main() {
	fmt.Println("no routes here")
}
`
	_, opts := setupTestModule(t, "main.go", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// No route_handler nodes should be present.
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			t.Errorf("unexpected route_handler node: %+v", n)
		}
	}

	// No handles_route edges should be present.
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			t.Errorf("unexpected handles_route edge: %+v", e)
		}
	}
}

func TestBuildImportMap(t *testing.T) {
	ext := NewGoTreeSitterExtractor()
	source := `package main

import (
	"fmt"
	"os"
	myfmt "github.com/custom/fmt"
)

func main() {}
`
	_, opts := setupTestModule(t, "main.go", source)

	// Use the extractor to parse and then inspect the import map indirectly
	// through the call edge resolution.
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// We should have import edges for all 3 imports.
	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}
	if len(importEdges) != 3 {
		t.Fatalf("expected 3 import edges, got %d", len(importEdges))
	}
}
