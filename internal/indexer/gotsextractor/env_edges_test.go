package gotsextractor

import (
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func parseGoSource(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(nil, nil, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return tree.RootNode()
}

func findFirstFunc(root *sitter.Node) *sitter.Node {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" || child.Type() == "method_declaration" {
			return child
		}
	}
	return nil
}

func TestExtractEnvReadEdges_Getenv(t *testing.T) {
	src := `package main

import "os"

func Configure() {
	host := os.Getenv("DB_HOST")
	_ = host
}
`
	root := parseGoSource(t, src)
	funcNode := findFirstFunc(root)
	if funcNode == nil {
		t.Fatal("no function found")
	}

	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		RepoHash: types.NewHash([]byte("test://repo")),
		FileHash: types.NewHash([]byte("testfile")),
		Content:  []byte(src),
	}
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Configure", types.KindFunction)

	nodes, edges := extractEnvReadEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	if nodes[0].QualifiedName != "env://DB_HOST" {
		t.Errorf("expected QN env://DB_HOST, got %s", nodes[0].QualifiedName)
	}
	if nodes[0].Kind != types.KindEnvVar {
		t.Errorf("expected kind %s, got %s", types.KindEnvVar, nodes[0].Kind)
	}
	if edges[0].EdgeType != edgetype.ReadsEnv {
		t.Errorf("expected edge type %s, got %s", edgetype.ReadsEnv, edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", edges[0].Confidence)
	}
}

func TestExtractEnvReadEdges_LookupEnv(t *testing.T) {
	src := `package main

import "os"

func Configure() {
	port, ok := os.LookupEnv("DB_PORT")
	_, _ = port, ok
}
`
	root := parseGoSource(t, src)
	funcNode := findFirstFunc(root)
	if funcNode == nil {
		t.Fatal("no function found")
	}

	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		RepoHash: types.NewHash([]byte("test://repo")),
		FileHash: types.NewHash([]byte("testfile")),
		Content:  []byte(src),
	}
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Configure", types.KindFunction)

	nodes, edges := extractEnvReadEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].QualifiedName != "env://DB_PORT" {
		t.Errorf("expected QN env://DB_PORT, got %s", nodes[0].QualifiedName)
	}
	if edges[0].EdgeType != edgetype.ReadsEnv {
		t.Errorf("expected edge type %s, got %s", edgetype.ReadsEnv, edges[0].EdgeType)
	}
}

func TestExtractEnvReadEdges_VariableArg(t *testing.T) {
	src := `package main

import "os"

func Configure(key string) {
	val := os.Getenv(key)
	_ = val
}
`
	root := parseGoSource(t, src)
	funcNode := findFirstFunc(root)
	if funcNode == nil {
		t.Fatal("no function found")
	}

	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		RepoHash: types.NewHash([]byte("test://repo")),
		FileHash: types.NewHash([]byte("testfile")),
		Content:  []byte(src),
	}
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Configure", types.KindFunction)

	nodes, edges := extractEnvReadEdges(funcNode, opts, "main", sourceHash)

	// Variable argument: no string literal, so nothing extracted.
	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes for variable arg, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for variable arg, got %d", len(edges))
	}
}

func TestExtractEnvReadEdges_MultipleEnvVars(t *testing.T) {
	src := `package main

import "os"

func Configure() {
	host := os.Getenv("DB_HOST")
	port, _ := os.LookupEnv("DB_PORT")
	// Duplicate should be deduplicated
	host2 := os.Getenv("DB_HOST")
	_, _, _ = host, port, host2
}
`
	root := parseGoSource(t, src)
	funcNode := findFirstFunc(root)
	if funcNode == nil {
		t.Fatal("no function found")
	}

	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		RepoHash: types.NewHash([]byte("test://repo")),
		FileHash: types.NewHash([]byte("testfile")),
		Content:  []byte(src),
	}
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Configure", types.KindFunction)

	nodes, edges := extractEnvReadEdges(funcNode, opts, "main", sourceHash)

	// DB_HOST appears twice but should be deduplicated.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes (deduplicated), got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges (deduplicated), got %d", len(edges))
	}
}

func TestExtractEnvReadEdges_NoMatches(t *testing.T) {
	src := `package main

import "fmt"

func Hello() {
	fmt.Println("hello")
}
`
	root := parseGoSource(t, src)
	funcNode := findFirstFunc(root)
	if funcNode == nil {
		t.Fatal("no function found")
	}

	opts := types.ExtractOptions{
		RepoURL:  "test://repo",
		RepoHash: types.NewHash([]byte("test://repo")),
		FileHash: types.NewHash([]byte("testfile")),
		Content:  []byte(src),
	}
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Hello", types.KindFunction)

	nodes, edges := extractEnvReadEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

// NOTE: Integration point: extractEnvReadEdges should be called from the main
// Go extractor's function/method processing loop in gotsextractor.go, alongside
// extractFieldAccessEdges. That file is not in this agent's ownership.
