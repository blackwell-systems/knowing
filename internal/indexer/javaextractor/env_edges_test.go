package javaextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractEnvReadEdges_SystemGetenv(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Config {
    public void load() {
        String apiKey = System.getenv("API_KEY");
        String dbHost = System.getenv("DB_HOST");
    }
}
`
	opts := makeOpts("src/Config.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	envEdges := filterEdgesByType(result.Edges, edgetype.ReadsEnv)
	if len(envEdges) < 2 {
		t.Fatalf("expected at least 2 reads_env edges, got %d", len(envEdges))
	}

	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Config.load", types.KindMethod)
	apiKeyHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "env://API_KEY", types.KindEnvVar)
	dbHostHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "env://DB_HOST", types.KindEnvVar)

	foundAPIKey := false
	foundDBHost := false
	for _, e := range envEdges {
		if e.SourceHash == methodHash && e.TargetHash == apiKeyHash {
			foundAPIKey = true
			if e.Confidence != 0.9 {
				t.Errorf("expected confidence 0.9, got %v", e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("expected provenance ast_inferred, got %q", e.Provenance)
			}
		}
		if e.SourceHash == methodHash && e.TargetHash == dbHostHash {
			foundDBHost = true
		}
	}
	if !foundAPIKey {
		t.Error("missing reads_env edge for env://API_KEY")
	}
	if !foundDBHost {
		t.Error("missing reads_env edge for env://DB_HOST")
	}

	// Verify env_var nodes are emitted.
	envNodes := filterNodesByKind(result.Nodes, types.KindEnvVar)
	if len(envNodes) < 2 {
		t.Fatalf("expected at least 2 env_var nodes, got %d", len(envNodes))
	}
}

func TestExtractEnvReadEdges_NoStringLiteral(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Config {
    public void load(String key) {
        String val = System.getenv(key);
    }
}
`
	opts := makeOpts("src/Config.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Non-string-literal args should not produce edges.
	envEdges := filterEdgesByType(result.Edges, edgetype.ReadsEnv)
	if len(envEdges) != 0 {
		t.Errorf("expected 0 reads_env edges for non-literal arg, got %d", len(envEdges))
	}
}

func TestExtractEnvReadEdges_Deduplicates(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Config {
    public void load() {
        String v1 = System.getenv("API_KEY");
        String v2 = System.getenv("API_KEY");
    }
}
`
	opts := makeOpts("src/Config.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	envEdges := filterEdgesByType(result.Edges, edgetype.ReadsEnv)
	if len(envEdges) != 1 {
		t.Errorf("expected 1 reads_env edge (deduplicated), got %d", len(envEdges))
	}
}
