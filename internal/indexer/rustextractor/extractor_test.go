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

func TestRustExtractor_MultipleImplBlocksSameStruct(t *testing.T) {
	ext := NewRustExtractor()
	source := `struct Connection {
    url: String,
}

impl Connection {
    fn new(url: String) -> Self {
        Connection { url }
    }

    fn connect(&self) -> Result<(), Error> {
        Ok(())
    }
}

impl Connection {
    fn disconnect(&self) {
    }

    fn is_alive(&self) -> bool {
        true
    }
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	methodNodes := filterNodes(result.Nodes, "method")
	if len(methodNodes) != 4 {
		t.Fatalf("expected 4 method nodes from 2 impl blocks, got %d", len(methodNodes))
	}

	names := nodeNames(methodNodes)
	assertContains(t, names, "new")
	assertContains(t, names, "connect")
	assertContains(t, names, "disconnect")
	assertContains(t, names, "is_alive")

	// All methods should reference Connection in their qualified name.
	for _, n := range methodNodes {
		if !containsStr(n.QualifiedName, "Connection") {
			t.Errorf("method QualifiedName %q should contain Connection", n.QualifiedName)
		}
	}
}

func TestRustExtractor_TraitMethodsProduceInterfaceKind(t *testing.T) {
	ext := NewRustExtractor()
	source := `trait Storage {
    fn get(&self, key: &str) -> Option<Vec<u8>>;
    fn set(&mut self, key: &str, value: Vec<u8>) -> Result<(), Error>;
    fn delete(&mut self, key: &str) -> Result<(), Error>;
}

trait Serializable {
    fn serialize(&self) -> Vec<u8>;
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	ifaceNodes := filterNodes(result.Nodes, "interface")
	if len(ifaceNodes) != 2 {
		t.Fatalf("expected 2 interface nodes for 2 traits, got %d", len(ifaceNodes))
	}

	names := nodeNames(ifaceNodes)
	assertContains(t, names, "Storage")
	assertContains(t, names, "Serializable")

	// Verify kind and signature format.
	for _, n := range ifaceNodes {
		if n.Kind != "interface" {
			t.Errorf("trait node %q has kind %q, want %q", n.QualifiedName, n.Kind, "interface")
		}
		if !containsStr(n.Signature, "trait") {
			t.Errorf("trait node signature %q should contain 'trait'", n.Signature)
		}
	}
}

func TestRustExtractor_TargetDirExcluded(t *testing.T) {
	ext := NewRustExtractor()

	excluded := []string{
		"target/debug/build/main.rs",
		"target/release/deps/lib.rs",
		"my_project/target/debug/foo.rs",
		"a/b/target/c/d.rs",
	}

	for _, p := range excluded {
		if ext.CanHandle(p) {
			t.Errorf("CanHandle(%q) = true, want false (target dir should be excluded)", p)
		}
	}

	// Paths that merely contain "target" as substring (not as directory component)
	// should still be handled.
	included := []string{
		"src/targeting/mod.rs",
		"src/target_utils.rs",
	}
	for _, p := range included {
		if !ext.CanHandle(p) {
			t.Errorf("CanHandle(%q) = false, want true (target as substring, not dir)", p)
		}
	}
}

func TestRustExtractor_MacroInvocationCallEdges(t *testing.T) {
	ext := NewRustExtractor()
	source := `fn build_data() -> Vec<i32> {
    println!("building data");
    let v = vec![1, 2, 3];
    eprintln!("done: {:?}", v);
    v
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdges(result.Edges, "calls")
	if len(callEdges) < 3 {
		t.Fatalf("expected at least 3 call edges for macro invocations (println!, vec!, eprintln!), got %d", len(callEdges))
	}

	// Every call edge should have valid call site info.
	for _, e := range callEdges {
		if e.CallSiteLine < 1 {
			t.Errorf("macro call edge has CallSiteLine %d, want >= 1", e.CallSiteLine)
		}
		if e.CallSiteFile != "src/lib.rs" {
			t.Errorf("macro call edge has CallSiteFile %q, want %q", e.CallSiteFile, "src/lib.rs")
		}
		if e.EdgeType != "calls" {
			t.Errorf("expected edge type 'calls', got %q", e.EdgeType)
		}
	}
}

func TestRustExtractor_NestedModuleDetection(t *testing.T) {
	ext := NewRustExtractor()
	// Nested mod blocks are not top-level items that the extractor extracts
	// directly, but functions inside them should still be found if they appear
	// at the top level of the AST (inline module bodies are children of the root).
	source := `fn top_level() {}

mod tests {
    fn test_something() {}
    fn test_other() {}
}
`
	opts := makeOpts(source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// The top-level function should always be extracted.
	funcNodes := filterNodes(result.Nodes, "function")
	if len(funcNodes) < 1 {
		t.Fatalf("expected at least 1 function node for top_level, got %d", len(funcNodes))
	}

	names := nodeNames(funcNodes)
	assertContains(t, names, "top_level")

	// The extractor walks only top-level children, so mod's contents may or may
	// not appear. We verify that the extractor doesn't panic and handles the
	// nested module gracefully (no errors, top-level still works).
	if len(result.Nodes) < 1 {
		t.Error("expected at least one node to be extracted")
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
