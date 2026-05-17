package goextractor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"

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
		{"main_test.go", true},
		{"vendor/lib/lib.go", false},
		{"main.py", false},
		{"README.md", false},
		{"pkg/bar_test.go", true},
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

func TestGoExtractor_CanHandle_AcceptsTestFiles(t *testing.T) {
	ext := NewGoExtractor()

	testFiles := []string{
		"main_test.go",
		"pkg/handler_test.go",
		"internal/foo/bar_test.go",
	}
	for _, path := range testFiles {
		if !ext.CanHandle(path) {
			t.Errorf("CanHandle(%q) should return true for test files (needed for test-scope)", path)
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

func TestGoExtractor_Extract_CrossFunctionCallEdges(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Helper() string {
	return "help"
}

func Caller() string {
	return Helper()
}
`
	_, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	result, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Find the Helper node hash.
	var helperNodeHash types.Hash
	for _, n := range result.Nodes {
		if n.Kind == "function" && n.QualifiedName == "test://repo://testmodule.Helper" {
			helperNodeHash = n.NodeHash
			break
		}
	}
	if helperNodeHash.IsZero() {
		t.Fatal("Helper node not found")
	}

	// Find the call edge from Caller to Helper and verify its target matches
	// the Helper node hash exactly.
	found := false
	for _, e := range result.Edges {
		if e.EdgeType == "calls" && e.TargetHash == helperNodeHash {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a 'calls' edge whose target matches Helper node hash; " +
			"this verifies node hashes and call edge target hashes are computed consistently")
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

func TestCrossRepoCallEdgeHash(t *testing.T) {
	// Set up two modules: repoB provides a library, repoA calls it.
	tmpDir := t.TempDir()

	// repoB: module github.com/test/repoB with an exported function.
	repoBDir := filepath.Join(tmpDir, "repoB")
	repoBPkgDir := filepath.Join(repoBDir, "pkg")
	if err := os.MkdirAll(repoBPkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoBDir, "go.mod"),
		[]byte("module github.com/test/repoB\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoBPkgDir, "lib.go"),
		[]byte("package pkg\n\nfunc DoThing() string { return \"done\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// repoA: module github.com/test/repoA that imports repoB.
	repoADir := filepath.Join(tmpDir, "repoA")
	if err := os.MkdirAll(repoADir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoAMod := fmt.Sprintf(`module github.com/test/repoA

go 1.23

require github.com/test/repoB v0.0.0

replace github.com/test/repoB => %s
`, repoBDir)
	if err := os.WriteFile(filepath.Join(repoADir, "go.mod"), []byte(repoAMod), 0o644); err != nil {
		t.Fatal(err)
	}
	repoAMain := `package main

import "github.com/test/repoB/pkg"

func main() {
	pkg.DoThing()
}
`
	if err := os.WriteFile(filepath.Join(repoADir, "main.go"), []byte(repoAMain), 0o644); err != nil {
		t.Fatal(err)
	}

	// Extract repoA's main.go.
	ext := NewGoExtractor()
	content, err := os.ReadFile(filepath.Join(repoADir, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	fileHash := types.NewHash(content)
	repoHash := types.NewHash([]byte("github.com/test/repoA"))
	opts := types.ExtractOptions{
		RepoURL:    "github.com/test/repoA",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   "main.go",
		FileHash:   fileHash,
		Content:    content,
		ModuleRoot: repoADir,
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// The expected target hash should use repoB's URL, not repoA's.
	expectedTargetHash := types.ComputeNodeHash(
		"github.com/test/repoB",
		"github.com/test/repoB/pkg",
		types.EmptyHash,
		"DoThing",
		"function",
	)

	// Find the call edge to DoThing.
	found := false
	for _, e := range result.Edges {
		if e.EdgeType == "calls" && e.TargetHash == expectedTargetHash {
			found = true
			break
		}
	}
	if !found {
		// Show what we got for debugging.
		wrongHash := types.ComputeNodeHash(
			"github.com/test/repoA",
			"github.com/test/repoB/pkg",
			types.EmptyHash,
			"DoThing",
			"function",
		)
		for _, e := range result.Edges {
			if e.EdgeType == "calls" {
				if e.TargetHash == wrongHash {
					t.Fatalf("call edge to DoThing uses repoA's URL (wrong); expected repoB's URL")
				}
				t.Logf("call edge target hash: %s", e.TargetHash)
			}
		}
		t.Fatal("no call edge found targeting DoThing with repoB's URL")
	}
}

func TestGoExtractor_ExtractWithPackage_MatchesExtract(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Hello() string {
	return "hello"
}

type Greeter struct{}

var Version = "1.0"

const MaxRetries = 3
`
	dir, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	// Get the result from the standard Extract method.
	extractResult, err := ext.Extract(ctx, opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Manually load the package the same way Extract does internally.
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule,
		Dir:     dir,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load failed: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("packages.Load returned no packages")
	}

	// Get the result from ExtractWithPackage.
	ewpResult, err := ext.ExtractWithPackage(ctx, opts, pkgs[0])
	if err != nil {
		t.Fatalf("ExtractWithPackage failed: %v", err)
	}

	// Compare nodes.
	if len(extractResult.Nodes) != len(ewpResult.Nodes) {
		t.Fatalf("node count mismatch: Extract=%d, ExtractWithPackage=%d",
			len(extractResult.Nodes), len(ewpResult.Nodes))
	}
	for i, n := range extractResult.Nodes {
		ewpN := ewpResult.Nodes[i]
		if n.NodeHash != ewpN.NodeHash {
			t.Errorf("node %d hash mismatch: %s vs %s", i, n.NodeHash, ewpN.NodeHash)
		}
		if n.QualifiedName != ewpN.QualifiedName {
			t.Errorf("node %d qualified name mismatch: %q vs %q", i, n.QualifiedName, ewpN.QualifiedName)
		}
		if n.Kind != ewpN.Kind {
			t.Errorf("node %d kind mismatch: %q vs %q", i, n.Kind, ewpN.Kind)
		}
	}

	// Compare edges.
	if len(extractResult.Edges) != len(ewpResult.Edges) {
		t.Fatalf("edge count mismatch: Extract=%d, ExtractWithPackage=%d",
			len(extractResult.Edges), len(ewpResult.Edges))
	}
	for i, e := range extractResult.Edges {
		ewpE := ewpResult.Edges[i]
		if e.EdgeHash != ewpE.EdgeHash {
			t.Errorf("edge %d hash mismatch: %s vs %s", i, e.EdgeHash, ewpE.EdgeHash)
		}
		if e.EdgeType != ewpE.EdgeType {
			t.Errorf("edge %d type mismatch: %q vs %q", i, e.EdgeType, ewpE.EdgeType)
		}
	}
}

func TestGoExtractor_ExtractWithPackage_NilSafety(t *testing.T) {
	ext := NewGoExtractor()
	source := `package main

func Hello() string {
	return "hello"
}
`
	dir, opts := setupTestModule(t, "main.go", source)
	ctx := context.Background()

	// Load the package, then nil out TypesInfo to test nil safety.
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports |
			packages.NeedModule,
		Dir:     dir,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load failed: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("packages.Load returned no packages")
	}

	pkg := pkgs[0]
	pkg.TypesInfo = nil

	// Should not panic; may return fewer edges due to nil TypesInfo.
	result, err := ext.ExtractWithPackage(ctx, opts, pkg)
	if err != nil {
		t.Fatalf("ExtractWithPackage with nil TypesInfo failed: %v", err)
	}

	// Should still produce nodes from AST declarations.
	if len(result.Nodes) == 0 {
		t.Error("expected at least one node even with nil TypesInfo")
	}
}
