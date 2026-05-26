package rustextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractProcessExecEdges_RustCommandNew(t *testing.T) {
	source := `
fn build() {
    let cmd = Command::new("cargo");
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	funcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "build", types.KindFunction)

	var procEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.ExecutesProcess && e.SourceHash == funcHash {
			procEdges = append(procEdges, e)
		}
	}

	if len(procEdges) != 1 {
		t.Fatalf("expected 1 executes_process edge, got %d", len(procEdges))
	}

	cargoHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "process://cargo", types.KindProcess)
	if procEdges[0].TargetHash != cargoHash {
		t.Error("executes_process edge target does not match process://cargo")
	}
	if procEdges[0].Confidence != 0.9 {
		t.Errorf("edge confidence = %f, want 0.9", procEdges[0].Confidence)
	}
	if procEdges[0].Provenance != "ast_inferred" {
		t.Errorf("edge provenance = %q, want %q", procEdges[0].Provenance, "ast_inferred")
	}
}

func TestExtractProcessExecEdges_RustDynamicCommand(t *testing.T) {
	source := `
fn run(name: &str) {
    let cmd = Command::new(name);
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	funcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "run", types.KindFunction)

	var procEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.ExecutesProcess && e.SourceHash == funcHash {
			procEdges = append(procEdges, e)
		}
	}

	if len(procEdges) != 1 {
		t.Fatalf("expected 1 executes_process edge, got %d", len(procEdges))
	}

	dynamicHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "process://dynamic", types.KindProcess)
	if procEdges[0].TargetHash != dynamicHash {
		t.Error("executes_process edge target does not match process://dynamic")
	}
}

func TestExtractProcessExecEdges_RustFullPath(t *testing.T) {
	source := `
fn deploy() {
    let cmd = std::process::Command::new("docker");
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	funcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "deploy", types.KindFunction)

	var procEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.ExecutesProcess && e.SourceHash == funcHash {
			procEdges = append(procEdges, e)
		}
	}

	if len(procEdges) != 1 {
		t.Fatalf("expected 1 executes_process edge, got %d", len(procEdges))
	}

	dockerHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "process://docker", types.KindProcess)
	if procEdges[0].TargetHash != dockerHash {
		t.Error("executes_process edge target does not match process://docker")
	}
}
