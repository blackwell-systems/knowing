package gotsextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_BasicReceiver(t *testing.T) {
	src := `package server

type Server struct {
	port    int
	config  *Config
	running bool
}

func (s *Server) Start() {
	s.running = true
	p := s.port
	_ = p
}
`
	_, opts := setupTestModule(t, "server.go", src)
	ext := NewGoTreeSitterExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// Should have accesses_field edges for "running" and "port".
	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	if len(fieldEdges) != 2 {
		t.Fatalf("expected 2 accesses_field edges, got %d", len(fieldEdges))
	}
}

func TestExtractFieldAccessEdges_SkipsMethodCalls(t *testing.T) {
	src := `package server

type Server struct {
	port int
}

func (s *Server) Start() {
	s.Stop()
}

func (s *Server) Stop() {}
`
	_, opts := setupTestModule(t, "server.go", src)
	ext := NewGoTreeSitterExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// s.Stop() is a method call, not a field access. No accesses_field edges expected.
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			t.Errorf("unexpected accesses_field edge (method call should be excluded)")
		}
	}
}

func TestExtractFieldAccessEdges_SkipsCommonFields(t *testing.T) {
	src := `package server

import "sync"

type Server struct {
	mu     sync.Mutex
	port   int
}

func (s *Server) Start() {
	s.mu.Lock()
	p := s.port
	_ = p
}
`
	_, opts := setupTestModule(t, "server.go", src)
	ext := NewGoTreeSitterExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// mu is in isCommonFieldName, should be filtered. Only "port" should produce an edge.
	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge (port only), got %d", len(fieldEdges))
	}
}

func TestExtractStructFields_CreatesFieldNodes(t *testing.T) {
	src := `package server

type Server struct {
	port    int
	config  *Config
	running bool
}
`
	_, opts := setupTestModule(t, "server.go", src)
	ext := NewGoTreeSitterExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// Should have field nodes for port, config, running.
	var fieldNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == types.KindField {
			fieldNodes = append(fieldNodes, n)
		}
	}

	if len(fieldNodes) != 3 {
		t.Fatalf("expected 3 field nodes, got %d", len(fieldNodes))
	}

	// Verify QN pattern.
	found := map[string]bool{}
	for _, n := range fieldNodes {
		found[n.QualifiedName] = true
	}
	expected := []string{
		"test://repo://testmodule.Server.port",
		"test://repo://testmodule.Server.config",
		"test://repo://testmodule.Server.running",
	}
	for _, qn := range expected {
		if !found[qn] {
			t.Errorf("missing field node with QN: %s (got: %v)", qn, found)
		}
	}
}
