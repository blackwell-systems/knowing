package gotsextractor

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/types"
)

// parseTestSource parses Go source and returns the tree-sitter root node.
// Caller must call tree.Close() when done.
func parseTestSource(t *testing.T, source string) (*sitter.Node, *sitter.Tree) {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(source))
	if err != nil {
		t.Fatalf("tree-sitter parse error: %v", err)
	}
	return tree.RootNode(), tree
}

func TestExtractTestsEdges_BasicTest(t *testing.T) {
	source := `package mypkg

func Bar() {}

func TestFoo(t *testing.T) {
	Bar()
}
`
	_, opts := setupTestModule(t, "foo_test.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "tests" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "tests")
	}
	if edge.Confidence != 0.7 {
		t.Errorf("Confidence = %v, want 0.7", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("Provenance = %q, want %q", edge.Provenance, "ast_inferred")
	}

	// Verify source is TestFoo and target is Bar.
	expectedSource := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "TestFoo", "function")
	expectedTarget := types.ComputeNodeHash("stdlib", pkgPath, types.EmptyHash, "Bar", "function")
	if edge.SourceHash != expectedSource {
		t.Errorf("SourceHash mismatch: got %v, want TestFoo hash", edge.SourceHash)
	}
	if edge.TargetHash != expectedTarget {
		t.Errorf("TargetHash mismatch: got %v, want Bar hash", edge.TargetHash)
	}
}

func TestExtractTestsEdges_MultipleCalls(t *testing.T) {
	source := `package mypkg

func Alpha() {}
func Beta() {}
func Gamma() {}

func TestMultiple(t *testing.T) {
	Alpha()
	Beta()
	Gamma()
}
`
	_, opts := setupTestModule(t, "multi_test.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	if len(edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(edges))
	}

	// All edges should be from TestMultiple.
	testHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "TestMultiple", "function")
	for _, e := range edges {
		if e.SourceHash != testHash {
			t.Errorf("edge source is not TestMultiple")
		}
		if e.EdgeType != "tests" {
			t.Errorf("EdgeType = %q, want %q", e.EdgeType, "tests")
		}
	}
}

func TestExtractTestsEdges_NoTestFunctions(t *testing.T) {
	source := `package mypkg

func Foo() {}
func Bar() {
	Foo()
}
`
	// Use a non-test filename so the function detects it's not a test file.
	_, opts := setupTestModule(t, "regular.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for non-test file, got %d", len(edges))
	}
}

func TestExtractTestsEdges_BenchmarkFunction(t *testing.T) {
	source := `package mypkg

func Compute() int {
	return 42
}

func BenchmarkCompute(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Compute()
	}
}
`
	_, opts := setupTestModule(t, "bench_test.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "tests" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "tests")
	}

	// Verify source is BenchmarkCompute.
	expectedSource := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "BenchmarkCompute", "function")
	if edge.SourceHash != expectedSource {
		t.Errorf("SourceHash mismatch: expected BenchmarkCompute hash")
	}
}

func TestExtractTestsEdges_TestCallingTest(t *testing.T) {
	source := `package mypkg

func Prod() {}

func TestHelper(t *testing.T) {
	Prod()
}

func TestMain(t *testing.T) {
	TestHelper(t)
	Prod()
}
`
	_, opts := setupTestModule(t, "helper_test.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	// TestHelper calls Prod -> 1 edge
	// TestMain calls TestHelper (excluded) and Prod -> 1 edge
	// Total: 2 edges (no edge for TestHelper call from TestMain)
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges (TestHelper excluded), got %d", len(edges))
	}

	// Verify no edge has a target matching "TestHelper".
	testHelperHash := types.ComputeNodeHash("stdlib", pkgPath, types.EmptyHash, "TestHelper", "function")
	for _, e := range edges {
		if e.TargetHash == testHelperHash {
			t.Errorf("found edge targeting TestHelper, which should be excluded")
		}
	}
}

func TestExtractTestsEdges_CrossPackageCall(t *testing.T) {
	source := `package mypkg

import "github.com/org/lib/pkg"

func TestCross(t *testing.T) {
	pkg.DoWork()
}
`
	_, opts := setupTestModule(t, "cross_test.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)

	edges := ExtractTestsEdges(root, opts, pkgPath, imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "tests" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "tests")
	}

	// Target should resolve via the import map to the external package.
	expectedTarget := types.ComputeNodeHash("github.com/org/lib", "github.com/org/lib/pkg", types.EmptyHash, "DoWork", "function")
	if edge.TargetHash != expectedTarget {
		t.Errorf("TargetHash mismatch: expected cross-package DoWork hash, got %v", edge.TargetHash)
	}

	// Source should be TestCross.
	expectedSource := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "TestCross", "function")
	if edge.SourceHash != expectedSource {
		t.Errorf("SourceHash mismatch: expected TestCross hash")
	}
}
