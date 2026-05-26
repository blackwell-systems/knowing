package javaextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractProcessExecEdges_RuntimeExec(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Deployer {
    public void deploy() {
        Runtime.getRuntime().exec("ls");
    }
}
`
	opts := makeOpts("src/Deployer.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	procEdges := filterEdgesByType(result.Edges, edgetype.ExecutesProcess)
	if len(procEdges) < 1 {
		t.Fatalf("expected at least 1 executes_process edge, got %d", len(procEdges))
	}

	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Deployer.deploy", types.KindMethod)
	lsHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "process://ls", types.KindProcess)

	found := false
	for _, e := range procEdges {
		if e.SourceHash == methodHash && e.TargetHash == lsHash {
			found = true
			if e.Confidence != 0.9 {
				t.Errorf("expected confidence 0.9, got %v", e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("expected provenance ast_inferred, got %q", e.Provenance)
			}
		}
	}
	if !found {
		t.Error("missing executes_process edge for process://ls")
	}
}

func TestExtractProcessExecEdges_ProcessBuilder(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Runner {
    public void run() {
        new ProcessBuilder("docker").start();
    }
}
`
	opts := makeOpts("src/Runner.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	procEdges := filterEdgesByType(result.Edges, edgetype.ExecutesProcess)
	if len(procEdges) < 1 {
		t.Fatalf("expected at least 1 executes_process edge, got %d", len(procEdges))
	}

	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Runner.run", types.KindMethod)
	dockerHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "process://docker", types.KindProcess)

	found := false
	for _, e := range procEdges {
		if e.SourceHash == methodHash && e.TargetHash == dockerHash {
			found = true
		}
	}
	if !found {
		t.Error("missing executes_process edge for process://docker")
	}

	// Verify process nodes are emitted.
	procNodes := filterNodesByKind(result.Nodes, types.KindProcess)
	if len(procNodes) < 1 {
		t.Fatalf("expected at least 1 process node, got %d", len(procNodes))
	}
}

func TestExtractProcessExecEdges_BothPatterns(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Builder {
    public void build() {
        Runtime.getRuntime().exec("ls");
        new ProcessBuilder("docker").start();
    }
}
`
	opts := makeOpts("src/Builder.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	procEdges := filterEdgesByType(result.Edges, edgetype.ExecutesProcess)
	if len(procEdges) < 2 {
		t.Fatalf("expected at least 2 executes_process edges, got %d", len(procEdges))
	}

	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Builder.build", types.KindMethod)
	lsHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "process://ls", types.KindProcess)
	dockerHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "process://docker", types.KindProcess)

	foundLs := false
	foundDocker := false
	for _, e := range procEdges {
		if e.SourceHash == methodHash && e.TargetHash == lsHash {
			foundLs = true
		}
		if e.SourceHash == methodHash && e.TargetHash == dockerHash {
			foundDocker = true
		}
	}
	if !foundLs {
		t.Error("missing executes_process edge for process://ls")
	}
	if !foundDocker {
		t.Error("missing executes_process edge for process://docker")
	}
}

func TestExtractProcessExecEdges_DynamicArg(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Runner {
    public void run(String cmd) {
        Runtime.getRuntime().exec(cmd);
    }
}
`
	opts := makeOpts("src/Runner.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	procEdges := filterEdgesByType(result.Edges, edgetype.ExecutesProcess)
	if len(procEdges) < 1 {
		t.Fatalf("expected at least 1 executes_process edge for dynamic arg, got %d", len(procEdges))
	}

	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Runner.run", types.KindMethod)
	dynamicHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "process://dynamic", types.KindProcess)

	found := false
	for _, e := range procEdges {
		if e.SourceHash == methodHash && e.TargetHash == dynamicHash {
			found = true
		}
	}
	if !found {
		t.Error("missing executes_process edge for process://dynamic")
	}
}
