package gotsextractor

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractProcessExecEdges_Command(t *testing.T) {
	src := `package main

import "os/exec"

func Build() {
	cmd := exec.Command("go", "build", "./...")
	_ = cmd
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
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Build", types.KindFunction)

	nodes, edges := extractProcessExecEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	if nodes[0].QualifiedName != "process://go" {
		t.Errorf("expected QN process://go, got %s", nodes[0].QualifiedName)
	}
	if nodes[0].Kind != types.KindProcess {
		t.Errorf("expected kind %s, got %s", types.KindProcess, nodes[0].Kind)
	}
	if edges[0].EdgeType != edgetype.ExecutesProcess {
		t.Errorf("expected edge type %s, got %s", edgetype.ExecutesProcess, edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", edges[0].Confidence)
	}
}

func TestExtractProcessExecEdges_CommandContext(t *testing.T) {
	src := `package main

import (
	"context"
	"os/exec"
)

func Deploy(ctx context.Context) {
	cmd := exec.CommandContext(ctx, "docker", "run", "app")
	_ = cmd
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
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Deploy", types.KindFunction)

	nodes, edges := extractProcessExecEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].QualifiedName != "process://docker" {
		t.Errorf("expected QN process://docker, got %s", nodes[0].QualifiedName)
	}
	if edges[0].EdgeType != edgetype.ExecutesProcess {
		t.Errorf("expected edge type %s, got %s", edgetype.ExecutesProcess, edges[0].EdgeType)
	}
}

func TestExtractProcessExecEdges_DynamicArg(t *testing.T) {
	src := `package main

import "os/exec"

func Run(binary string) {
	cmd := exec.Command(binary, "arg1")
	_ = cmd
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
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Run", types.KindFunction)

	nodes, edges := extractProcessExecEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 node for dynamic arg, got %d", len(nodes))
	}
	if nodes[0].QualifiedName != "process://dynamic" {
		t.Errorf("expected QN process://dynamic, got %s", nodes[0].QualifiedName)
	}
	if edges[0].EdgeType != edgetype.ExecutesProcess {
		t.Errorf("expected edge type %s, got %s", edgetype.ExecutesProcess, edges[0].EdgeType)
	}
}

func TestExtractProcessExecEdges_NoMatches(t *testing.T) {
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

	nodes, edges := extractProcessExecEdges(funcNode, opts, "main", sourceHash)

	if len(nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractProcessExecEdges_MultipleCommands(t *testing.T) {
	src := `package main

import "os/exec"

func Setup() {
	exec.Command("git", "clone", "repo")
	exec.Command("make", "build")
	// Duplicate should be deduplicated
	exec.Command("git", "pull")
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
	sourceHash := types.ComputeNodeHash("test://repo", "main", types.EmptyHash, "Setup", types.KindFunction)

	nodes, edges := extractProcessExecEdges(funcNode, opts, "main", sourceHash)

	// "git" appears twice but should be deduplicated. So: git + make = 2.
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes (deduplicated), got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges (deduplicated), got %d", len(edges))
	}
}

// NOTE: Integration point: extractProcessExecEdges should be called from the main
// Go extractor's function/method processing loop in gotsextractor.go, alongside
// extractFieldAccessEdges. That file is not in this agent's ownership.
