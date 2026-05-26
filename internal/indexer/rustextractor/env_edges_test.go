package rustextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractEnvReadEdges_RustEnvVar(t *testing.T) {
	source := `
fn configure() {
    let host = env::var("DB_HOST").unwrap();
    let path = std::env::var_os("PATH");
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	funcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "configure", types.KindFunction)

	var envEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.ReadsEnv && e.SourceHash == funcHash {
			envEdges = append(envEdges, e)
		}
	}

	if len(envEdges) != 2 {
		t.Fatalf("expected 2 reads_env edges, got %d", len(envEdges))
	}

	dbHostHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "env://DB_HOST", types.KindEnvVar)
	pathHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "env://PATH", types.KindEnvVar)

	foundDBHost, foundPath := false, false
	for _, e := range envEdges {
		if e.TargetHash == dbHostHash {
			foundDBHost = true
			if e.Confidence != 0.9 {
				t.Errorf("DB_HOST edge confidence = %f, want 0.9", e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("DB_HOST edge provenance = %q, want %q", e.Provenance, "ast_inferred")
			}
		}
		if e.TargetHash == pathHash {
			foundPath = true
		}
	}
	if !foundDBHost {
		t.Error("missing reads_env edge for env://DB_HOST")
	}
	if !foundPath {
		t.Error("missing reads_env edge for env://PATH")
	}
}

func TestExtractEnvReadEdges_RustDeduplicates(t *testing.T) {
	source := `
fn load_config() {
    let a = env::var("HOME").unwrap();
    let b = env::var("HOME").unwrap();
}
`
	opts := makeOpts(source)
	ext := NewRustExtractor()
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	basePath := computeBasePath(opts)
	funcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, "load_config", types.KindFunction)

	var envEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.ReadsEnv && e.SourceHash == funcHash {
			envEdges = append(envEdges, e)
		}
	}

	if len(envEdges) != 1 {
		t.Fatalf("expected 1 reads_env edge (deduplicated), got %d", len(envEdges))
	}
}
