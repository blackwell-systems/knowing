package gotsextractor

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractDocumentsEdges_FunctionWithDoc(t *testing.T) {
	source := `package mypkg

// Foo does something important.
func Foo() {}
`
	_, opts := setupTestModule(t, "foo.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"

	// Simulate what Extract produces: a function node with Doc populated.
	declNodes := []types.Node{
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Foo", "function"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Foo",
			Kind:          "function",
			Line:          4,
			Doc:           "Foo does something important.",
		},
	}

	nodes, edges := ExtractDocumentsEdges(root, opts, pkgPath, declNodes)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 comment node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	// Verify comment node.
	cn := nodes[0]
	if cn.Kind != "doc_comment" {
		t.Errorf("comment node Kind = %q, want %q", cn.Kind, "doc_comment")
	}
	if cn.Line != 3 {
		t.Errorf("comment node Line = %d, want 3 (one before declaration)", cn.Line)
	}
	if cn.Doc != "Foo does something important." {
		t.Errorf("comment node Doc = %q, want %q", cn.Doc, "Foo does something important.")
	}

	// Verify edge.
	edge := edges[0]
	if edge.EdgeType != "documents" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "documents")
	}
	if edge.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("Provenance = %q, want %q", edge.Provenance, "ast_inferred")
	}
	if edge.SourceHash != cn.NodeHash {
		t.Errorf("edge SourceHash should match comment node hash")
	}
	if edge.TargetHash != declNodes[0].NodeHash {
		t.Errorf("edge TargetHash should match function node hash")
	}
}

func TestExtractDocumentsEdges_NoDoc(t *testing.T) {
	source := `package mypkg

func Bar() {}
`
	_, opts := setupTestModule(t, "bar.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"

	// Function without doc comment.
	declNodes := []types.Node{
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Bar", "function"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Bar",
			Kind:          "function",
			Line:          3,
			Doc:           "", // no doc
		},
	}

	nodes, edges := ExtractDocumentsEdges(root, opts, pkgPath, declNodes)

	if len(nodes) != 0 {
		t.Fatalf("expected 0 comment nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractDocumentsEdges_TypeWithDoc(t *testing.T) {
	source := `package mypkg

// Server handles HTTP requests.
type Server struct{}
`
	_, opts := setupTestModule(t, "server.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"

	declNodes := []types.Node{
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Server", "type"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Server",
			Kind:          "type",
			Line:          4,
			Doc:           "Server handles HTTP requests.",
		},
	}

	nodes, edges := ExtractDocumentsEdges(root, opts, pkgPath, declNodes)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 comment node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	cn := nodes[0]
	if cn.Kind != "doc_comment" {
		t.Errorf("comment node Kind = %q, want %q", cn.Kind, "doc_comment")
	}

	edge := edges[0]
	if edge.EdgeType != "documents" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "documents")
	}
	if edge.TargetHash != declNodes[0].NodeHash {
		t.Errorf("edge target should be Server type node")
	}
}

func TestExtractDocumentsEdges_MethodWithDoc(t *testing.T) {
	source := `package mypkg

// Handle processes a request.
func (s *Server) Handle() {}
`
	_, opts := setupTestModule(t, "handle.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"

	declNodes := []types.Node{
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Handle", "method"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Server.Handle",
			Kind:          "method",
			Line:          4,
			Doc:           "Handle processes a request.",
		},
	}

	nodes, edges := ExtractDocumentsEdges(root, opts, pkgPath, declNodes)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 comment node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	cn := nodes[0]
	if cn.Kind != "doc_comment" {
		t.Errorf("comment node Kind = %q, want %q", cn.Kind, "doc_comment")
	}
	if cn.Doc != "Handle processes a request." {
		t.Errorf("comment node Doc = %q, want %q", cn.Doc, "Handle processes a request.")
	}

	edge := edges[0]
	if edge.EdgeType != "documents" {
		t.Errorf("EdgeType = %q, want %q", edge.EdgeType, "documents")
	}
	if edge.Confidence != 0.9 {
		t.Errorf("Confidence = %v, want 0.9", edge.Confidence)
	}
}

func TestExtractDocumentsEdges_MultipleDecls(t *testing.T) {
	source := `package mypkg

// Alpha is the first function.
func Alpha() {}

// Beta is the second function.
func Beta() {}

func Gamma() {}
`
	_, opts := setupTestModule(t, "multi.go", source)
	root, tree := parseTestSource(t, source)
	defer tree.Close()

	pkgPath := "testmodule"

	declNodes := []types.Node{
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Alpha", "function"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Alpha",
			Kind:          "function",
			Line:          4,
			Doc:           "Alpha is the first function.",
		},
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Beta", "function"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Beta",
			Kind:          "function",
			Line:          7,
			Doc:           "Beta is the second function.",
		},
		{
			NodeHash:      types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Gamma", "function"),
			FileHash:      opts.FileHash,
			QualifiedName: opts.RepoURL + "://" + pkgPath + ".Gamma",
			Kind:          "function",
			Line:          9,
			Doc:           "", // no doc
		},
	}

	nodes, edges := ExtractDocumentsEdges(root, opts, pkgPath, declNodes)

	// Only Alpha and Beta have docs.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 comment nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Verify all edges are 'documents' type.
	for i, edge := range edges {
		if edge.EdgeType != "documents" {
			t.Errorf("edges[%d].EdgeType = %q, want %q", i, edge.EdgeType, "documents")
		}
		if edge.Confidence != 0.9 {
			t.Errorf("edges[%d].Confidence = %v, want 0.9", i, edge.Confidence)
		}
	}

	// Verify the two edges target Alpha and Beta respectively.
	alphaHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Alpha", "function")
	betaHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, "Beta", "function")
	targets := map[types.Hash]bool{
		edges[0].TargetHash: true,
		edges[1].TargetHash: true,
	}
	if !targets[alphaHash] {
		t.Errorf("expected an edge targeting Alpha")
	}
	if !targets[betaHash] {
		t.Errorf("expected an edge targeting Beta")
	}
}
