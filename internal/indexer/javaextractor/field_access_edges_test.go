package javaextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestExtractFieldAccessEdges_JavaThis(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class UserService {
    private String name;
    private int count;

    public void process() {
        String n = this.name;
        this.count = this.count + 1;
    }
}
`
	opts := makeOpts("src/UserService.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	fieldEdges := filterEdgesByType(result.Edges, edgetype.AccessesField)
	if len(fieldEdges) < 2 {
		t.Fatalf("expected at least 2 accesses_field edges, got %d", len(fieldEdges))
	}

	// Verify edge properties.
	for _, e := range fieldEdges {
		if e.Confidence != 0.8 {
			t.Errorf("expected confidence 0.8, got %v", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("expected provenance ast_inferred, got %q", e.Provenance)
		}
	}

	// Verify that both "name" and "count" field accesses are found.
	// The source hash should be the method node hash for UserService.process.
	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "UserService.process", types.KindMethod)
	nameFieldHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "UserService.name", types.KindField)
	countFieldHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "UserService.count", types.KindField)

	foundName := false
	foundCount := false
	for _, e := range fieldEdges {
		if e.SourceHash == methodHash && e.TargetHash == nameFieldHash {
			foundName = true
		}
		if e.SourceHash == methodHash && e.TargetHash == countFieldHash {
			foundCount = true
		}
	}
	if !foundName {
		t.Error("missing accesses_field edge for this.name")
	}
	if !foundCount {
		t.Error("missing accesses_field edge for this.count")
	}
}

func TestExtractFieldAccessEdges_JavaSkipsMethodCalls(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Worker {
    private String data;

    public void run() {
        this.process();
        this.cleanup();
        String d = this.data;
    }

    private void process() {}
    private void cleanup() {}
}
`
	opts := makeOpts("src/Worker.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	fieldEdges := filterEdgesByType(result.Edges, edgetype.AccessesField)

	// Only this.data should produce a field access edge.
	// this.process() and this.cleanup() are method_invocation nodes, not field_access.
	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Worker.run", types.KindMethod)
	dataFieldHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Worker.data", types.KindField)

	foundData := false
	for _, e := range fieldEdges {
		if e.SourceHash == methodHash && e.TargetHash == dataFieldHash {
			foundData = true
		}
	}
	if !foundData {
		t.Error("missing accesses_field edge for this.data")
	}

	// Should be exactly 1 field access edge from Worker.run (only this.data).
	runEdges := 0
	for _, e := range fieldEdges {
		if e.SourceHash == methodHash {
			runEdges++
		}
	}
	if runEdges != 1 {
		t.Errorf("expected 1 field access edge from Worker.run, got %d", runEdges)
	}
}

func TestExtractFieldAccessEdges_JavaSkipsCommonFields(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Service {
    private Object logger;
    private Object LOG;
    private Object ctx;
    private long serialVersionUID;
    private String importantField;

    public void doWork() {
        this.logger.info("test");
        Object l = this.LOG;
        Object c = this.ctx;
        long uid = this.serialVersionUID;
        String val = this.importantField;
    }
}
`
	opts := makeOpts("src/Service.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	fieldEdges := filterEdgesByType(result.Edges, edgetype.AccessesField)
	methodHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.doWork", types.KindMethod)

	// Only importantField should produce an edge; logger, LOG, ctx, serialVersionUID are filtered.
	importantHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.importantField", types.KindField)
	foundImportant := false
	for _, e := range fieldEdges {
		if e.SourceHash == methodHash && e.TargetHash == importantHash {
			foundImportant = true
		}
	}
	if !foundImportant {
		t.Error("missing accesses_field edge for this.importantField")
	}

	// Verify filtered fields are NOT present.
	loggerHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.logger", types.KindField)
	logHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.LOG", types.KindField)
	ctxHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.ctx", types.KindField)
	suidHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Service.serialVersionUID", types.KindField)

	for _, e := range fieldEdges {
		if e.SourceHash == methodHash {
			if e.TargetHash == loggerHash {
				t.Error("should NOT emit accesses_field for this.logger (common field)")
			}
			if e.TargetHash == logHash {
				t.Error("should NOT emit accesses_field for this.LOG (common field)")
			}
			if e.TargetHash == ctxHash {
				t.Error("should NOT emit accesses_field for this.ctx (common field)")
			}
			if e.TargetHash == suidHash {
				t.Error("should NOT emit accesses_field for this.serialVersionUID (common field)")
			}
		}
	}
}

func TestExtractClassFieldNodes_Java(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example;

public class Account {
    private String name;
    private int balance;
    private static final long serialVersionUID = 1L;

    public String getName() {
        return this.name;
    }
}
`
	opts := makeOpts("src/Account.java", source)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	fieldNodes := filterNodesByKind(result.Nodes, types.KindField)
	if len(fieldNodes) < 3 {
		t.Fatalf("expected at least 3 field nodes, got %d: %v", len(fieldNodes), nodeNames(result.Nodes))
	}

	// Verify field nodes have correct properties.
	foundName := false
	foundBalance := false
	foundSerialVersionUID := false
	for _, n := range fieldNodes {
		if containsName(n.QualifiedName, "Account.name") {
			foundName = true
			if n.Kind != types.KindField {
				t.Errorf("Account.name: expected kind %q, got %q", types.KindField, n.Kind)
			}
			expectedQN := "test://repo://com.example.Account.name"
			if n.QualifiedName != expectedQN {
				t.Errorf("Account.name QN: got %q, want %q", n.QualifiedName, expectedQN)
			}
		}
		if containsName(n.QualifiedName, "Account.balance") {
			foundBalance = true
		}
		if containsName(n.QualifiedName, "Account.serialVersionUID") {
			foundSerialVersionUID = true
		}
	}
	if !foundName {
		t.Error("missing field node for Account.name")
	}
	if !foundBalance {
		t.Error("missing field node for Account.balance")
	}
	if !foundSerialVersionUID {
		t.Error("missing field node for Account.serialVersionUID")
	}

	// Verify node hashes are computed correctly.
	expectedHash := types.ComputeNodeHash("test://repo", "com.example", types.EmptyHash, "Account.name", types.KindField)
	for _, n := range fieldNodes {
		if containsName(n.QualifiedName, "Account.name") {
			if n.NodeHash != expectedHash {
				t.Errorf("Account.name hash mismatch: got %v, want %v", n.NodeHash, expectedHash)
			}
		}
	}
}
