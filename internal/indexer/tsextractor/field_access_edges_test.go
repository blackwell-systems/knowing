package tsextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_TSThis(t *testing.T) {
	source := `class Server {
  port: number;
  running: boolean;

  start() {
    this.running = true;
    const p = this.port;
  }
}
`
	opts := makeOpts(t, "server.ts", source)
	ext := NewTypeScriptExtractor()

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

func TestExtractFieldAccessEdges_TSSkipsMethodCalls(t *testing.T) {
	source := `class Server {
  port: number;

  start() {
    this.stop();
    this.configure();
  }

  stop() {}
  configure() {}
}
`
	opts := makeOpts(t, "server.ts", source)
	ext := NewTypeScriptExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// this.stop() and this.configure() are method calls, not field accesses.
	// No accesses_field edges expected.
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			t.Errorf("unexpected accesses_field edge (method call should be excluded)")
		}
	}
}

func TestExtractFieldAccessEdges_TSSkipsCommonFields(t *testing.T) {
	source := `class Server {
  logger: Logger;
  port: number;
  ctx: Context;

  start() {
    this.logger.info("starting");
    const p = this.port;
    this.ctx.cancel();
  }
}
`
	opts := makeOpts(t, "server.ts", source)
	ext := NewTypeScriptExtractor()

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// logger and ctx are in isCommonFieldName, should be filtered.
	// Only "port" should produce an accesses_field edge.
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

func TestExtractClassFieldNodes_TypeScript(t *testing.T) {
	source := `class Server {
  port: number;
  config: Config;
  running: boolean;

  start() {
    this.running = true;
  }
}
`
	opts := makeOpts(t, "server.ts", source)
	ext := NewTypeScriptExtractor()

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
		"test://repo://server.Server.port",
		"test://repo://server.Server.config",
		"test://repo://server.Server.running",
	}
	for _, qn := range expected {
		if !found[qn] {
			t.Errorf("missing field node with QN: %s (got: %v)", qn, found)
		}
	}
}
