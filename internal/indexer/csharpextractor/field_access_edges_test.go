package csharpextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_CSharpThis(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class Server
{
    private int _port;
    private string _host;

    public void Start()
    {
        var p = this._port;
        this._host = "localhost";
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	if len(fieldEdges) != 2 {
		t.Fatalf("expected 2 accesses_field edges, got %d", len(fieldEdges))
	}

	// Verify provenance and confidence.
	for _, e := range fieldEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("accesses_field edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.8 {
			t.Errorf("accesses_field edge confidence = %f, want 0.8", e.Confidence)
		}
	}

	// Verify source hash is the method hash.
	methodHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "Start", types.KindMethod)
	for _, e := range fieldEdges {
		if e.SourceHash != methodHash {
			t.Errorf("accesses_field edge source should be method hash, got different hash")
		}
	}

	// Verify target hashes correspond to Server._port and Server._host.
	expectedTargets := map[types.Hash]string{
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "Server._port", types.KindField):  "Server._port",
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "Server._host", types.KindField): "Server._host",
	}
	for _, e := range fieldEdges {
		if _, ok := expectedTargets[e.TargetHash]; !ok {
			t.Errorf("unexpected target hash in accesses_field edge")
		}
	}
}

func TestExtractFieldAccessEdges_CSharpSkipsMethodCalls(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class Service
{
    private int _count;

    public void DoWork()
    {
        this.Start();
        this.Process("data");
        var c = this._count;
    }

    private void Start() {}
    private void Process(string s) {}
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	// Should only have 1 accesses_field edge for _count. Start() and Process()
	// are method calls and should be excluded.
	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge (only _count), got %d", len(fieldEdges))
	}

	// Verify the target is the _count field.
	expectedTarget := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "Service._count", types.KindField)
	if fieldEdges[0].TargetHash != expectedTarget {
		t.Errorf("expected target hash for Service._count, got different hash")
	}
}

func TestExtractFieldAccessEdges_CSharpSkipsCommonFields(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class Worker
{
    private object _logger;
    private object _context;
    private bool _disposed;
    private string _name;

    public void Run()
    {
        var l = this._logger;
        var c = this._context;
        var d = this._disposed;
        var n = this._name;
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var fieldEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.AccessesField {
			fieldEdges = append(fieldEdges, e)
		}
	}

	// _logger, _context, _disposed are common fields and should be filtered.
	// Only _name should produce an edge.
	if len(fieldEdges) != 1 {
		t.Fatalf("expected 1 accesses_field edge (only _name), got %d", len(fieldEdges))
	}

	expectedTarget := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "Worker._name", types.KindField)
	if fieldEdges[0].TargetHash != expectedTarget {
		t.Errorf("expected target hash for Worker._name, got different hash")
	}
}

func TestExtractClassFieldNodes_CSharp(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class UserService
{
    private readonly ILogger _logger;
    private string _name;
    private int _count;
    public string DisplayName { get; set; }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var fieldNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == types.KindField {
			fieldNodes = append(fieldNodes, n)
		}
	}

	// Expect 4 field nodes: _logger, _name, _count (from field_declaration)
	// and DisplayName (from property_declaration).
	if len(fieldNodes) != 4 {
		t.Fatalf("expected 4 field nodes, got %d", len(fieldNodes))
	}

	// Verify field node hashes match expected scoped names.
	expectedFields := map[types.Hash]string{
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "UserService._logger", types.KindField):      "UserService._logger",
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "UserService._name", types.KindField):        "UserService._name",
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "UserService._count", types.KindField):       "UserService._count",
		types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "UserService.DisplayName", types.KindField): "UserService.DisplayName",
	}

	for _, n := range fieldNodes {
		if _, ok := expectedFields[n.NodeHash]; !ok {
			t.Errorf("unexpected field node hash for QN %q", n.QualifiedName)
		}
		if n.FileHash != opts.FileHash {
			t.Errorf("field node FileHash mismatch")
		}
		if n.Line == 0 {
			t.Errorf("field node should have non-zero Line")
		}
	}
}
