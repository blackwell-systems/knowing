package treesitter

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_PythonSelf(t *testing.T) {
	src := `class Server:
    def __init__(self):
        self.port = 8080
        self.running = False

    def start(self):
        self.running = True
        p = self.port
`
	ext := mustExtractor(t)
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// The start() method accesses self.running and self.port.
	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	// start() accesses running and port. __init__ also accesses running and port.
	// Total: 2 from start + 2 from __init__ = 4 edges.
	// But we also need to check that the start method specifically produces edges.
	// Let's find the start method's hash and check its edges.
	var startHash types.Hash
	for _, n := range result.Nodes {
		if n.Kind == types.KindMethod && n.QualifiedName == "github.com/example/repo://src/src/main.py.Server.start" {
			startHash = n.NodeHash
			break
		}
	}
	if startHash == types.EmptyHash {
		t.Fatal("start method node not found")
	}

	var startFieldEdges []types.Edge
	for _, e := range fieldEdges {
		if e.SourceHash == startHash {
			startFieldEdges = append(startFieldEdges, e)
		}
	}

	if len(startFieldEdges) != 2 {
		t.Fatalf("expected 2 accesses_field edges from start(), got %d", len(startFieldEdges))
	}

	// Verify provenance and confidence.
	for _, e := range startFieldEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("expected provenance 'ast_inferred', got %q", e.Provenance)
		}
		if e.Confidence != 0.8 {
			t.Errorf("expected confidence 0.8, got %f", e.Confidence)
		}
	}
}

func TestExtractFieldAccessEdges_PythonSkipsMethodCalls(t *testing.T) {
	src := `class Server:
    def start(self):
        self.stop()
        self.configure()
`
	ext := mustExtractor(t)
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// self.stop() and self.configure() are method calls, not field accesses.
	// No accesses_field edges should be created.
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			t.Errorf("unexpected accesses_field edge (method calls should be excluded)")
		}
	}
}

func TestExtractFieldAccessEdges_PythonSkipsCommonFields(t *testing.T) {
	src := `class Server:
    def start(self):
        self.logger.info("starting")
        self.ctx = new_context()
        p = self.port
`
	ext := mustExtractor(t)
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// logger and ctx are common fields and should be filtered.
	// Only "port" should produce an accesses_field edge.
	var startHash types.Hash
	for _, n := range result.Nodes {
		if n.Kind == types.KindMethod && n.QualifiedName == "github.com/example/repo://src/src/main.py.Server.start" {
			startHash = n.NodeHash
			break
		}
	}
	if startHash == types.EmptyHash {
		t.Fatal("start method node not found")
	}

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField && e.SourceHash == startHash {
			fieldEdges = append(fieldEdges, e)
		}
	}

	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge (port only), got %d", len(fieldEdges))
	}

	// Verify it points to Server.port.
	expectedTarget := types.ComputeNodeHash("github.com/example/repo", "src", types.EmptyHash, "Server.port", types.KindField)
	if fieldEdges[0].TargetHash != expectedTarget {
		t.Errorf("expected target hash for Server.port, got different hash")
	}
}

func TestExtractClassFieldNodes_Python(t *testing.T) {
	src := `class Server:
    host: str
    port: int

    def __init__(self):
        self.running = False
        self.config = {}
`
	ext := mustExtractor(t)
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// Should have field nodes for: running, config (from __init__),
	// and host, port (from class-level annotations).
	var fieldNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == types.KindField {
			fieldNodes = append(fieldNodes, n)
		}
	}

	// We expect at least running and config from __init__.
	// Class-level annotations depend on how tree-sitter parses them.
	if len(fieldNodes) < 2 {
		t.Fatalf("expected at least 2 field nodes, got %d: %+v", len(fieldNodes), fieldNodes)
	}

	// Check that __init__ fields are present.
	found := map[string]bool{}
	for _, n := range fieldNodes {
		found[n.QualifiedName] = true
	}

	expectedFromInit := []string{
		"github.com/example/repo://src/src/main.py.Server.running",
		"github.com/example/repo://src/src/main.py.Server.config",
	}
	for _, qn := range expectedFromInit {
		if !found[qn] {
			t.Errorf("missing field node with QN: %s (got: %v)", qn, found)
		}
	}

	// Verify field node hashes follow the expected pattern.
	for _, n := range fieldNodes {
		if n.FileHash != opts.FileHash {
			t.Errorf("field node %s has wrong FileHash", n.QualifiedName)
		}
	}
}
