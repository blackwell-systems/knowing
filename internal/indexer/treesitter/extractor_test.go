package treesitter

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeOpts(content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:    "github.com/example/repo",
		RepoHash:   types.NewHash([]byte("github.com/example/repo")),
		CommitHash: "abc123",
		FilePath:   "src/main.py",
		FileHash:   types.NewHash([]byte(content)),
		Content:    []byte(content),
		ModuleRoot: "src",
	}
}

func mustExtractor(t *testing.T) *TreeSitterExtractor {
	t.Helper()
	ext, err := NewTreeSitterExtractor("python")
	if err != nil {
		t.Fatalf("NewTreeSitterExtractor: %v", err)
	}
	return ext
}

func TestTreeSitterExtractor_CanHandle_PythonFiles(t *testing.T) {
	ext := mustExtractor(t)

	tests := []struct {
		path string
		want bool
	}{
		{"main.py", true},
		{"src/utils.py", true},
		{"main.go", false},
		{"main.js", false},
		{"", false},
		{"pyfile", false},
		{"file.pyx", false},
	}

	for _, tt := range tests {
		if got := ext.CanHandle(tt.path); got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestTreeSitterExtractor_Extract_PythonFunction(t *testing.T) {
	ext := mustExtractor(t)
	opts := makeOpts("def hello():\n    pass\n")

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}

	node := result.Nodes[0]
	if node.Kind != "function" {
		t.Errorf("expected kind 'function', got %q", node.Kind)
	}
	if node.QualifiedName != "github.com/example/repo://src/src/main.py.hello" {
		t.Errorf("unexpected qualified name: %q", node.QualifiedName)
	}
	if node.Line != 1 {
		t.Errorf("expected line 1, got %d", node.Line)
	}
}

func TestTreeSitterExtractor_Extract_PythonClass(t *testing.T) {
	ext := mustExtractor(t)
	src := `class MyClass:
    def method_one(self):
        pass

    def method_two(self):
        pass
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Expect: 1 class (type) + 2 methods = 3 nodes.
	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}

	// Check we have the class node.
	var classFound, m1Found, m2Found bool
	for _, n := range result.Nodes {
		switch {
		case n.Kind == "type" && n.QualifiedName == "github.com/example/repo://src/src/main.py.MyClass":
			classFound = true
		case n.Kind == "method" && n.QualifiedName == "github.com/example/repo://src/src/main.py.MyClass.method_one":
			m1Found = true
		case n.Kind == "method" && n.QualifiedName == "github.com/example/repo://src/src/main.py.MyClass.method_two":
			m2Found = true
		}
	}

	if !classFound {
		t.Error("class node MyClass not found")
	}
	if !m1Found {
		t.Error("method node method_one not found")
	}
	if !m2Found {
		t.Error("method node method_two not found")
	}
}

func TestTreeSitterExtractor_Extract_PythonImport(t *testing.T) {
	ext := mustExtractor(t)
	src := "import os\nfrom pathlib import Path\n"
	opts := makeOpts(src)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(result.Edges) < 2 {
		t.Fatalf("expected at least 2 import edges, got %d", len(result.Edges))
	}

	var osImport, pathlibImport bool
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			// We can verify provenance and confidence.
			if e.Confidence != 1.0 {
				t.Errorf("expected confidence 1.0, got %f", e.Confidence)
			}
			if e.Provenance != "ast_resolved" {
				t.Errorf("expected provenance 'ast_resolved', got %q", e.Provenance)
			}
			// Check target hashes correspond to os and pathlib.
			osHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, "os", "module")
			pathlibHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, "pathlib", "module")
			if e.TargetHash == osHash {
				osImport = true
			}
			if e.TargetHash == pathlibHash {
				pathlibImport = true
			}
		}
	}

	if !osImport {
		t.Error("import edge for 'os' not found")
	}
	if !pathlibImport {
		t.Error("import edge for 'pathlib' not found")
	}
}

func TestTreeSitterExtractor_Extract_CallEdges(t *testing.T) {
	ext := mustExtractor(t)
	src := "def foo():\n    bar()\n    baz(42)\n"
	opts := makeOpts(src)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Should have call edges for bar() and baz().
	callEdges := 0
	for _, e := range result.Edges {
		if e.EdgeType == "calls" {
			callEdges++
			if e.Confidence != 1.0 {
				t.Errorf("expected confidence 1.0, got %f", e.Confidence)
			}
			if e.Provenance != "ast_resolved" {
				t.Errorf("expected provenance 'ast_resolved', got %q", e.Provenance)
			}
		}
	}

	if callEdges < 2 {
		t.Errorf("expected at least 2 call edges, got %d", callEdges)
	}
}

func TestTreeSitterExtractor_Extract_Deterministic(t *testing.T) {
	ext := mustExtractor(t)
	src := `def zebra():
    pass

def alpha():
    pass

class Middle:
    def beta(self):
        pass
`
	opts := makeOpts(src)

	// Run extraction twice and verify results are identical.
	r1, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract 1: %v", err)
	}

	r2, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract 2: %v", err)
	}

	if len(r1.Nodes) != len(r2.Nodes) {
		t.Fatalf("node count mismatch: %d vs %d", len(r1.Nodes), len(r2.Nodes))
	}

	for i := range r1.Nodes {
		if r1.Nodes[i].QualifiedName != r2.Nodes[i].QualifiedName {
			t.Errorf("node %d: %q != %q", i, r1.Nodes[i].QualifiedName, r2.Nodes[i].QualifiedName)
		}
		if r1.Nodes[i].Kind != r2.Nodes[i].Kind {
			t.Errorf("node %d kind: %q != %q", i, r1.Nodes[i].Kind, r2.Nodes[i].Kind)
		}
	}

	// Verify sorted order: alpha < Middle < Middle.beta < zebra
	if len(r1.Nodes) >= 2 {
		for i := 1; i < len(r1.Nodes); i++ {
			prev := r1.Nodes[i-1]
			curr := r1.Nodes[i]
			if prev.QualifiedName > curr.QualifiedName {
				t.Errorf("nodes not sorted: %q > %q", prev.QualifiedName, curr.QualifiedName)
			}
		}
	}
}

func TestTreeSitterExtractor_QualifiedName_Format(t *testing.T) {
	ext := mustExtractor(t)
	src := `class Animal:
    def speak(self):
        pass

def standalone():
    pass
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	expectedNames := map[string]string{
		"github.com/example/repo://src/src/main.py.Animal":       "type",
		"github.com/example/repo://src/src/main.py.Animal.speak": "method",
		"github.com/example/repo://src/src/main.py.standalone":   "function",
	}

	if len(result.Nodes) != len(expectedNames) {
		t.Fatalf("expected %d nodes, got %d", len(expectedNames), len(result.Nodes))
	}

	for _, n := range result.Nodes {
		expectedKind, ok := expectedNames[n.QualifiedName]
		if !ok {
			t.Errorf("unexpected node: %q (kind=%s)", n.QualifiedName, n.Kind)
			continue
		}
		if n.Kind != expectedKind {
			t.Errorf("node %q: expected kind %q, got %q", n.QualifiedName, expectedKind, n.Kind)
		}
	}
}

func TestNewTreeSitterExtractor_UnsupportedLanguage(t *testing.T) {
	_, err := NewTreeSitterExtractor("ruby")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestTreeSitterExtractor_Name(t *testing.T) {
	ext := mustExtractor(t)
	if got := ext.Name(); got != "treesitter-python" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-python")
	}
}

func makeOptsWithPath(content, filePath string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:    "github.com/example/repo",
		RepoHash:   types.NewHash([]byte("github.com/example/repo")),
		CommitHash: "abc123",
		FilePath:   filePath,
		FileHash:   types.NewHash([]byte(content)),
		Content:    []byte(content),
		ModuleRoot: "src",
	}
}

func TestTreeSitterExtractor_FlaskRoutes(t *testing.T) {
	ext := mustExtractor(t)
	source := `from flask import Flask
app = Flask(__name__)

@app.get("/users")
def get_users():
    return []

@app.post("/users")
def create_user():
    return {"id": 1}
`
	opts := makeOptsWithPath(source, "app.py")
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

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /users"] {
		t.Errorf("missing 'GET /users', got %v", patterns)
	}
	if !patterns["POST /users"] {
		t.Errorf("missing 'POST /users', got %v", patterns)
	}

	// Check handles_route edges.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}
	if len(routeEdges) != 2 {
		t.Fatalf("expected 2 handles_route edges, got %d", len(routeEdges))
	}
}

func TestTreeSitterExtractor_FastAPIRoutes(t *testing.T) {
	ext := mustExtractor(t)
	source := `from fastapi import FastAPI
app = FastAPI()

@app.get("/items/{item_id}")
async def read_item(item_id: int):
    return {"item_id": item_id}

@app.post("/items")
async def create_item(item: dict):
    return item

@app.delete("/items/{item_id}")
async def delete_item(item_id: int):
    return {"deleted": True}
`
	opts := makeOptsWithPath(source, "main.py")
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
		t.Fatalf("expected 3 route_handler nodes for FastAPI, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /items/{item_id}"] {
		t.Errorf("missing 'GET /items/{item_id}', got %v", patterns)
	}
	if !patterns["POST /items"] {
		t.Errorf("missing 'POST /items', got %v", patterns)
	}
	if !patterns["DELETE /items/{item_id}"] {
		t.Errorf("missing 'DELETE /items/{item_id}', got %v", patterns)
	}
}

func TestTreeSitterExtractor_DjangoURLPatterns(t *testing.T) {
	ext := mustExtractor(t)
	source := `from django.urls import path
from . import views

urlpatterns = [
    path('users/', views.user_list),
    path('users/<int:pk>/', views.user_detail),
]
`
	opts := makeOptsWithPath(source, "myapp/urls.py")
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

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes for Django, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["ANY /users/"] {
		t.Errorf("missing 'ANY /users/', got %v", patterns)
	}
	if !patterns["ANY /users/<int:pk>/"] {
		t.Errorf("missing 'ANY /users/<int:pk>/', got %v", patterns)
	}
}

func TestTreeSitterExtractor_NoRoutesInPlainPython(t *testing.T) {
	ext := mustExtractor(t)
	source := `def process_data(data):
    return data.strip()

class DataProcessor:
    def run(self):
        return self.process()
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			t.Errorf("unexpected route_handler node in plain Python: %+v", n)
		}
	}
}
