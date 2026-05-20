package gotsextractor

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/types"
)

// findFirstFuncBodyNode finds the body of the first function_declaration in the tree.
func findFirstFuncBodyNode(root *sitter.Node) *sitter.Node {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" {
			return child.ChildByFieldName("body")
		}
	}
	return nil
}

func TestExtractGoEndpointEdges_HttpGet(t *testing.T) {
	source := `package mypkg

import "net/http"

func FetchUsers() {
	http.Get("http://service/api/users")
}
`
	_, opts := setupTestModule(t, "client.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	imports := buildImportMap(root, opts.Content)
	pkgPath := "testmodule"
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "FetchUsers", "function")

	body := findFirstFuncBodyNode(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	nodes, edges := ExtractGoEndpointEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	edge := edges[0]
	if edge.EdgeType != "consumes_endpoint" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "consumes_endpoint")
	}
	if edge.Confidence != 0.6 {
		t.Errorf("Confidence = %v, want 0.6", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("Provenance = %q, want %q", edge.Provenance, "ast_inferred")
	}
	if edge.SourceHash != funcNodeHash {
		t.Errorf("SourceHash mismatch")
	}

	node := nodes[0]
	if node.Kind != "endpoint" {
		t.Errorf("node Kind = %q, want %q", node.Kind, "endpoint")
	}
	if node.Signature != "GET /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "GET /api/users")
	}
}

func TestExtractGoEndpointEdges_HttpPost(t *testing.T) {
	source := `package mypkg

import "net/http"

func CreateUser() {
	http.Post("http://service/api/users", "application/json", nil)
}
`
	_, opts := setupTestModule(t, "client.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	imports := buildImportMap(root, opts.Content)
	pkgPath := "testmodule"
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "CreateUser", "function")

	body := findFirstFuncBodyNode(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	nodes, edges := ExtractGoEndpointEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	node := nodes[0]
	if node.Signature != "POST /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "POST /api/users")
	}
}

func TestExtractGoEndpointEdges_NewRequest(t *testing.T) {
	source := `package mypkg

import "net/http"

func MakeRequest() {
	http.NewRequest("GET", "/api/users", nil)
}
`
	_, opts := setupTestModule(t, "client.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	imports := buildImportMap(root, opts.Content)
	pkgPath := "testmodule"
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "MakeRequest", "function")

	body := findFirstFuncBodyNode(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	nodes, edges := ExtractGoEndpointEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	node := nodes[0]
	if node.Signature != "GET /api/users" {
		t.Errorf("node Signature = %q, want %q", node.Signature, "GET /api/users")
	}

	edge := edges[0]
	if edge.EdgeType != "consumes_endpoint" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "consumes_endpoint")
	}
}

func TestExtractGoEndpointEdges_NoHTTPCalls(t *testing.T) {
	source := `package mypkg

import "fmt"

func PrintStuff() {
	fmt.Println("hello")
}
`
	_, opts := setupTestModule(t, "other.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	imports := buildImportMap(root, opts.Content)
	pkgPath := "testmodule"
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "PrintStuff", "function")

	body := findFirstFuncBodyNode(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	nodes, edges := ExtractGoEndpointEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractGoEndpointEdges_NonHTTPPackage(t *testing.T) {
	source := `package mypkg

import "github.com/some/cache"

func GetFromCache() {
	cache.Get("http://example.com/api/data")
}
`
	_, opts := setupTestModule(t, "client.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	imports := buildImportMap(root, opts.Content)
	pkgPath := "testmodule"
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "GetFromCache", "function")

	body := findFirstFuncBodyNode(root)
	if body == nil {
		t.Fatal("no function body found")
	}

	nodes, edges := ExtractGoEndpointEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes for non-HTTP package, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges for non-HTTP package, got %d", len(edges))
	}
}
