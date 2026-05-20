package gotsextractor

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFeatureFlagEdges_LaunchDarkly(t *testing.T) {
	source := `package mypkg

import "github.com/launchdarkly/go-server-sdk/v6"

func HandleRequest() {
	client := ldclient.LDClient{}
	if client.BoolVariation("my-flag", nil, false) {
		doWork()
	}
}
`
	_, opts := setupTestModule(t, "handler.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "HandleRequest", "function")

	// Find the function body.
	body := findFuncBody(root, "HandleRequest", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for HandleRequest")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 flag node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	// Verify flag node.
	fn := nodes[0]
	if fn.Kind != "feature_flag" {
		t.Errorf("flag node Kind = %q, want %q", fn.Kind, "feature_flag")
	}
	if fn.Signature != "flag my-flag" {
		t.Errorf("flag node Signature = %q, want %q", fn.Signature, "flag my-flag")
	}
	if fn.QualifiedName != opts.RepoURL+"://flags.my-flag" {
		t.Errorf("flag node QualifiedName = %q, want %q", fn.QualifiedName, opts.RepoURL+"://flags.my-flag")
	}
	if !fn.FileHash.IsZero() {
		t.Errorf("flag node FileHash should be EmptyHash (synthetic)")
	}

	// Verify edge.
	edge := edges[0]
	if edge.EdgeType != "gated_by_flag" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "gated_by_flag")
	}
	if edge.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("Provenance = %q, want %q", edge.Provenance, "ast_inferred")
	}
	if edge.SourceHash != funcNodeHash {
		t.Errorf("edge source should be the function hash")
	}
	if edge.TargetHash != fn.NodeHash {
		t.Errorf("edge target should be the flag node hash")
	}
}

func TestExtractFeatureFlagEdges_Unleash(t *testing.T) {
	source := `package mypkg

func ServeHTTP() {
	if unleash.IsEnabled("dark-mode") {
		renderDarkTheme()
	}
}
`
	_, opts := setupTestModule(t, "serve.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "ServeHTTP", "function")

	body := findFuncBody(root, "ServeHTTP", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for ServeHTTP")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 flag node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	fn := nodes[0]
	if fn.Signature != "flag dark-mode" {
		t.Errorf("flag node Signature = %q, want %q", fn.Signature, "flag dark-mode")
	}

	edge := edges[0]
	if edge.EdgeType != "gated_by_flag" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "gated_by_flag")
	}
}

func TestExtractFeatureFlagEdges_CustomFunction(t *testing.T) {
	source := `package mypkg

func ProcessOrder() {
	if IsFeatureEnabled("new-checkout") {
		newCheckout()
	}
}
`
	_, opts := setupTestModule(t, "order.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "ProcessOrder", "function")

	body := findFuncBody(root, "ProcessOrder", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for ProcessOrder")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 flag node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	fn := nodes[0]
	if fn.Signature != "flag new-checkout" {
		t.Errorf("flag node Signature = %q, want %q", fn.Signature, "flag new-checkout")
	}

	edge := edges[0]
	if edge.EdgeType != "gated_by_flag" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "gated_by_flag")
	}
}

func TestExtractFeatureFlagEdges_NoFlags(t *testing.T) {
	source := `package mypkg

func SimpleFunc() {
	x := computeValue()
	println(x)
}
`
	_, opts := setupTestModule(t, "simple.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "SimpleFunc", "function")

	body := findFuncBody(root, "SimpleFunc", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for SimpleFunc")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 0 {
		t.Fatalf("expected 0 flag nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractFeatureFlagEdges_MultipleFlagsSameFunc(t *testing.T) {
	source := `package mypkg

func MultiFlag() {
	if client.BoolVariation("flag-a", nil, false) {
		doA()
	}
	if client.StringVariation("flag-b", nil, "") {
		doB()
	}
}
`
	_, opts := setupTestModule(t, "multi.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "MultiFlag", "function")

	body := findFuncBody(root, "MultiFlag", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for MultiFlag")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 flag nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Verify both edges are gated_by_flag and from the same source.
	for i, edge := range edges {
		if edge.EdgeType != "gated_by_flag" {
			t.Errorf("edges[%d].EdgeType = %q, want %q", i, edge.EdgeType, "gated_by_flag")
		}
		if edge.SourceHash != funcNodeHash {
			t.Errorf("edges[%d].SourceHash should be MultiFlag", i)
		}
	}

	// Verify we have two distinct flag nodes.
	if nodes[0].NodeHash == nodes[1].NodeHash {
		t.Errorf("expected two distinct flag nodes, got same hash")
	}
}

func TestExtractFeatureFlagEdges_DynamicString(t *testing.T) {
	source := `package mypkg

func DynamicFlag() {
	flagName := getFlagName()
	if client.BoolVariation(flagName, nil, false) {
		doWork()
	}
}
`
	_, opts := setupTestModule(t, "dynamic.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"
	imports := buildImportMap(root, opts.Content)
	funcNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "DynamicFlag", "function")

	body := findFuncBody(root, "DynamicFlag", opts.Content)
	if body == nil {
		t.Fatal("could not find function body for DynamicFlag")
	}

	nodes, edges := ExtractFeatureFlagEdges(body, opts, pkgPath, funcNodeHash, imports)

	// Dynamic string argument (variable, not literal) should be skipped.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 flag nodes for dynamic string, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for dynamic string, got %d", len(edges))
	}
}

// findFuncBody is a test helper that locates the body of a named function
// in the tree-sitter root node.
func findFuncBody(root *sitter.Node, funcName string, content []byte) *sitter.Node {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "function_declaration" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		if nameNode.Content(content) == funcName {
			return child.ChildByFieldName("body")
		}
	}
	return nil
}
