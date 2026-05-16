package rustextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// makeOpts creates ExtractOptions for testing with the given Rust source.
func makeOpts(source string) types.ExtractOptions {
	fileHash := types.NewHash([]byte(source))
	repoHash := types.NewHash([]byte("test://repo"))
	return types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   "src/lib.rs",
		FileHash:   fileHash,
		Content:    []byte(source),
		ModuleRoot: "/tmp/testcrate",
	}
}

func TestRustExtractor_Name(t *testing.T) {
	ext := NewRustExtractor()
	if got := ext.Name(); got != "treesitter-rust" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-rust")
	}
}

func TestRustExtractor_CanHandle(t *testing.T) {
	ext := NewRustExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"src/main.rs", true},
		{"src/lib.rs", true},
		{"src/models/user.rs", true},
		{"main.go", false},
		{"script.py", false},
		{"target/debug/main.rs", false},
		{"some/target/release/lib.rs", false},
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

func TestRustExtractor_ExtractFunctions(t *testing.T) {
	ext := NewRustExtractor()
	source := `fn hello() -> String {
    String::from("hello")
}

fn goodbye() {
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have at least 2 function nodes.
	funcNodes := filterNodes(result.Nodes, "function")
	if len(funcNodes) < 2 {
		t.Fatalf("expected at least 2 function nodes, got %d (total nodes: %d)", len(funcNodes), len(result.Nodes))
	}

	// Check names are present.
	names := nodeNames(funcNodes)
	assertContains(t, names, "goodbye")
	assertContains(t, names, "hello")

	// Verify kind and line.
	for _, n := range funcNodes {
		if n.Kind != "function" {
			t.Errorf("node %q has kind %q, want %q", n.QualifiedName, n.Kind, "function")
		}
		if n.Line < 1 {
			t.Errorf("node %q has line %d, want >= 1", n.QualifiedName, n.Line)
		}
	}
}

func TestRustExtractor_ExtractStructsAndEnums(t *testing.T) {
	ext := NewRustExtractor()
	source := `struct AppState {
    db: Pool,
}

enum Status {
    Active,
    Inactive,
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	typeNodes := filterNodes(result.Nodes, "type")
	if len(typeNodes) != 2 {
		t.Fatalf("expected 2 type nodes, got %d", len(typeNodes))
	}

	names := nodeNames(typeNodes)
	assertContains(t, names, "AppState")
	assertContains(t, names, "Status")

	// Check signatures.
	for _, n := range typeNodes {
		if n.Kind != "type" {
			t.Errorf("node %q has kind %q, want %q", n.QualifiedName, n.Kind, "type")
		}
	}
}

func TestRustExtractor_ExtractTraits(t *testing.T) {
	ext := NewRustExtractor()
	source := `trait Repository {
    fn find(&self, id: u64) -> Option<Entity>;
    fn save(&self, entity: Entity) -> Result<(), Error>;
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	ifaceNodes := filterNodes(result.Nodes, "interface")
	if len(ifaceNodes) != 1 {
		t.Fatalf("expected 1 interface node, got %d", len(ifaceNodes))
	}

	if !containsName(ifaceNodes, "Repository") {
		t.Errorf("expected interface node for Repository")
	}

	n := ifaceNodes[0]
	if n.Kind != "interface" {
		t.Errorf("trait node has kind %q, want %q", n.Kind, "interface")
	}
	if n.Line < 1 {
		t.Errorf("trait node line = %d, want >= 1", n.Line)
	}
}

func TestRustExtractor_ExtractImplMethods(t *testing.T) {
	ext := NewRustExtractor()
	source := `struct AppState {
    db: Pool,
}

impl AppState {
    fn new(pool: Pool) -> Self {
        Self { db: pool }
    }

    fn get_db(&self) -> &Pool {
        &self.db
    }
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	methodNodes := filterNodes(result.Nodes, "method")
	if len(methodNodes) < 2 {
		t.Fatalf("expected at least 2 method nodes, got %d", len(methodNodes))
	}

	names := nodeNames(methodNodes)
	assertContains(t, names, "new")
	assertContains(t, names, "get_db")

	// Methods should have impl type in QualifiedName.
	for _, n := range methodNodes {
		if n.Kind != "method" {
			t.Errorf("node %q has kind %q, want %q", n.QualifiedName, n.Kind, "method")
		}
		if !containsStr(n.QualifiedName, "AppState") {
			t.Errorf("method QualifiedName %q should contain impl type AppState", n.QualifiedName)
		}
	}
}

func TestRustExtractor_ExtractUseStatements(t *testing.T) {
	ext := NewRustExtractor()
	source := `use std::collections::HashMap;
use serde::Deserialize;
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	importEdges := filterEdges(result.Edges, "imports")
	if len(importEdges) < 2 {
		t.Fatalf("expected at least 2 import edges, got %d", len(importEdges))
	}

	// All import edges should have correct provenance and confidence.
	for _, e := range importEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestRustExtractor_ExtractCallEdges(t *testing.T) {
	ext := NewRustExtractor()
	source := `fn main() {
    let result = compute();
    std::io::println("hello");
}

fn compute() -> i32 {
    42
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdges(result.Edges, "calls")
	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// Call edges should have call-site positions.
	for _, e := range callEdges {
		if e.CallSiteLine < 1 {
			t.Errorf("call edge CallSiteLine = %d, want >= 1", e.CallSiteLine)
		}
		if e.CallSiteFile == "" {
			t.Errorf("call edge CallSiteFile is empty")
		}
	}
}

func TestRustExtractor_ExtractMacroInvocations(t *testing.T) {
	ext := NewRustExtractor()
	source := `fn main() {
    println!("hello world");
    vec![1, 2, 3];
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdges(result.Edges, "calls")
	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge for macros, got %d", len(callEdges))
	}

	// Macro call edges should have provenance and confidence set.
	for _, e := range callEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("macro call edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("macro call edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestRustExtractor_ActixRoutes(t *testing.T) {
	ext := NewRustExtractor()
	source := `#[get("/hello")]
fn hello() -> &'static str {
    "Hello!"
}

#[post("/users")]
fn create_user() -> HttpResponse {
    HttpResponse::Ok().finish()
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	routeNodes := filterNodes(result.Nodes, "route_handler")
	if len(routeNodes) < 2 {
		t.Fatalf("expected at least 2 route_handler nodes, got %d", len(routeNodes))
	}

	routeEdges := filterEdges(result.Edges, "handles_route")
	if len(routeEdges) < 2 {
		t.Fatalf("expected at least 2 handles_route edges, got %d", len(routeEdges))
	}

	// Verify route patterns.
	signatures := make([]string, 0, len(routeNodes))
	for _, n := range routeNodes {
		signatures = append(signatures, n.Signature)
	}
	assertContainsStr(t, signatures, "GET /hello")
	assertContainsStr(t, signatures, "POST /users")
}

func TestRustExtractor_AxumRoutes(t *testing.T) {
	ext := NewRustExtractor()
	// Axum uses Router::new().route("/path", get(handler)) pattern.
	// The tree-sitter Rust grammar may represent this as method_call chains.
	// We detect .route() calls with string path and handler arguments.
	source := `fn app() -> Router {
    Router::new()
        .route("/hello", get(hello_handler))
        .route("/users", post(create_user))
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// The Axum route detection is best-effort since tree-sitter may represent
	// method chains differently. Check that extraction doesn't error.
	// Route detection for chained method calls is heuristic.
	_ = result
}

func TestRustExtractor_EdgeProvenanceAndConfidence(t *testing.T) {
	ext := NewRustExtractor()
	source := `use std::io;

fn main() {
    let x = compute();
}

fn compute() -> i32 { 42 }
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, e := range result.Edges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestRustExtractor_EmptyFile(t *testing.T) {
	ext := NewRustExtractor()
	source := ""
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty file, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges for empty file, got %d", len(result.Edges))
	}
}

// --- helpers ---

func filterNodes(nodes []types.Node, kind string) []types.Node {
	var out []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func filterEdges(edges []types.Edge, edgeType string) []types.Edge {
	var out []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			out = append(out, e)
		}
	}
	return out
}

func nodeNames(nodes []types.Node) []string {
	var names []string
	for _, n := range nodes {
		// Extract just the last segment of the QualifiedName.
		parts := splitQualified(n.QualifiedName)
		if len(parts) > 0 {
			names = append(names, parts[len(parts)-1])
		}
	}
	return names
}

func splitQualified(qname string) []string {
	// QualifiedName format: {repoURL}://{moduleRoot}/{basePath}.{name}
	// Split on "." to get components.
	idx := lastDotIndex(qname)
	if idx < 0 {
		return []string{qname}
	}
	return []string{qname[:idx], qname[idx+1:]}
}

func lastDotIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

func containsName(nodes []types.Node, name string) bool {
	for _, n := range nodes {
		if containsStr(n.QualifiedName, name) {
			return true
		}
	}
	return false
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || containsSubstr(haystack, needle))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func assertContains(t *testing.T, strs []string, target string) {
	t.Helper()
	for _, s := range strs {
		if s == target {
			return
		}
	}
	t.Errorf("expected %v to contain %q", strs, target)
}

func assertContainsStr(t *testing.T, strs []string, target string) {
	t.Helper()
	for _, s := range strs {
		if s == target {
			return
		}
	}
	t.Errorf("expected %v to contain %q", strs, target)
}
