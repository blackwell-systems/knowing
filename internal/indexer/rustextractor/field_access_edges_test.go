package rustextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_RustSelf(t *testing.T) {
	source := `
struct Server {
    port: u16,
    running: bool,
}

impl Server {
    fn start(&mut self) {
        let p = self.port;
        self.running = true;
    }
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// The method "start" should have accesses_field edges to Server.port and Server.running.
	basePath := computeBasePath(opts)
	methodHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "start", types.KindMethod)

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField && e.SourceHash == methodHash {
			fieldEdges = append(fieldEdges, e)
		}
	}

	if len(fieldEdges) != 2 {
		t.Fatalf("expected 2 accesses_field edges, got %d", len(fieldEdges))
	}

	// Verify target hashes match expected field nodes.
	portHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "Server.port", types.KindField)
	runningHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "Server.running", types.KindField)

	foundPort, foundRunning := false, false
	for _, e := range fieldEdges {
		if e.TargetHash == portHash {
			foundPort = true
			if e.Confidence != 0.8 {
				t.Errorf("port edge confidence = %f, want 0.8", e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("port edge provenance = %q, want %q", e.Provenance, "ast_inferred")
			}
		}
		if e.TargetHash == runningHash {
			foundRunning = true
		}
	}
	if !foundPort {
		t.Error("missing accesses_field edge for Server.port")
	}
	if !foundRunning {
		t.Error("missing accesses_field edge for Server.running")
	}
}

func TestExtractFieldAccessEdges_RustSkipsMethodCalls(t *testing.T) {
	source := `
struct Client {
    name: String,
}

impl Client {
    fn process(&self) {
        self.connect();
        let n = self.name;
    }

    fn connect(&self) {}
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	methodHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "process", types.KindMethod)

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField && e.SourceHash == methodHash {
			fieldEdges = append(fieldEdges, e)
		}
	}

	// Should have edge to "name" but NOT "connect" (method call).
	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge, got %d", len(fieldEdges))
	}

	nameHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "Client.name", types.KindField)
	if fieldEdges[0].TargetHash != nameHash {
		t.Errorf("expected edge target to be Client.name, got different hash")
	}

	// Verify "connect" was not treated as a field access.
	connectHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "Client.connect", types.KindField)
	for _, e := range fieldEdges {
		if e.TargetHash == connectHash {
			t.Error("self.connect() method call was incorrectly treated as field access")
		}
	}
}

func TestExtractFieldAccessEdges_RustSkipsCommonFields(t *testing.T) {
	source := `
struct Handler {
    ctx: Context,
    mu: Mutex,
    logger: Logger,
    name: String,
}

impl Handler {
    fn handle(&self) {
        let c = self.ctx;
        let m = self.mu;
        let l = self.logger;
        let n = self.name;
    }
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	methodHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "handle", types.KindMethod)

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField && e.SourceHash == methodHash {
			fieldEdges = append(fieldEdges, e)
		}
	}

	// Only "name" should survive; ctx, mu, logger are all common fields.
	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge (only name), got %d", len(fieldEdges))
	}

	nameHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "Handler.name", types.KindField)
	if fieldEdges[0].TargetHash != nameHash {
		t.Errorf("expected edge target to be Handler.name, got different hash")
	}
}

func TestExtractStructFieldNodes_Rust(t *testing.T) {
	source := `
struct Config {
    host: String,
    port: u16,
    debug: bool,
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Count field nodes.
	var fieldNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == types.KindField {
			fieldNodes = append(fieldNodes, n)
		}
	}

	if len(fieldNodes) != 3 {
		t.Fatalf("expected 3 field nodes, got %d", len(fieldNodes))
	}

	// Verify expected field nodes exist.
	basePath := computeBasePath(opts)
	expectedFields := []struct {
		name      string
		signature string
	}{
		{"Config.host", "host: String"},
		{"Config.port", "port: u16"},
		{"Config.debug", "debug: bool"},
	}

	for _, ef := range expectedFields {
		expectedHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, ef.name, types.KindField)
		found := false
		for _, fn := range fieldNodes {
			if fn.NodeHash == expectedHash {
				found = true
				if fn.Signature != ef.signature {
					t.Errorf("field %s: signature = %q, want %q", ef.name, fn.Signature, ef.signature)
				}
				if fn.Kind != types.KindField {
					t.Errorf("field %s: kind = %q, want %q", ef.name, fn.Kind, types.KindField)
				}
				break
			}
		}
		if !found {
			t.Errorf("missing field node for %s", ef.name)
		}
	}
}
