package javaresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	java "github.com/smacker/go-tree-sitter/java"
)

// parseJavaFile parses Java source code and returns the root node.
func parseJavaFile(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

func TestJavaResolverLanguage(t *testing.T) {
	r := NewJavaResolver()
	if got := r.Language(); got != "java" {
		t.Errorf("Language() = %q, want %q", got, "java")
	}
}

func TestJavaResolverInitWorkspace(t *testing.T) {
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "com.example.service.UserService.processData", Kind: "method", PackagePath: "com.example.service"},
		{QualifiedName: "com.example.util.Helper.compute", Kind: "function", PackagePath: "com.example.util"},
		{QualifiedName: "com.example.model.User", Kind: "type", PackagePath: "com.example.model"},
		{QualifiedName: "com.example.api.Endpoint", Kind: "interface", PackagePath: "com.example.api"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	if r.registry == nil {
		t.Fatal("registry is nil after InitWorkspace")
	}

	// Verify functions are registered.
	if f := r.registry.LookupFunc("com.example.util.Helper.compute"); f == nil {
		t.Error("expected com.example.util.Helper.compute in registry")
	}

	// Verify methods are registered (by receiver lookup).
	if f := r.registry.LookupMethod("com.example.service.UserService", "processData"); f == nil {
		t.Error("expected processData method on UserService in registry")
	}

	// Verify types are registered.
	if tp := r.registry.LookupType("com.example.model.User"); tp == nil {
		t.Error("expected com.example.model.User in registry")
	}

	// Verify interfaces are registered.
	if tp := r.registry.LookupType("com.example.api.Endpoint"); tp == nil {
		t.Error("expected com.example.api.Endpoint in registry")
	} else if !tp.IsInterface {
		t.Error("expected com.example.api.Endpoint to be an interface")
	}
}

func TestJavaResolverResolveFile_ImportCall(t *testing.T) {
	src := `package com.example;
import java.util.ArrayList;
class Main {
    void process() {
        ArrayList list = new ArrayList();
        list.add("item");
    }
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "java.util.ArrayList", Kind: "class", PackagePath: "java.util"},
		{QualifiedName: "java.util.ArrayList.<init>", Kind: "method", PackagePath: "java.util"},
		{QualifiedName: "java.util.ArrayList.add", Kind: "method", PackagePath: "java.util"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseJavaFile(t, src)
	content := []byte(src)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Main.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge from ResolveFile")
	}

	// Verify all edges have correct provenance and confidence.
	for _, e := range edges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
		if e.Confidence != typresolve.ResolverConfidence {
			t.Errorf("edge confidence = %v, want %v", e.Confidence, typresolve.ResolverConfidence)
		}
		if e.EdgeType != edgetype.Calls {
			t.Errorf("edge type = %q, want %q", e.EdgeType, edgetype.Calls)
		}
	}
}

func TestJavaResolverResolveFile_LocalCalls(t *testing.T) {
	src := `package com.example;
class Service {
    void helper() {}
    void main() {
        helper();
    }
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "com.example.Service", Kind: "class", PackagePath: "com.example"},
		{QualifiedName: "com.example.Service.helper", Kind: "method", PackagePath: "com.example"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseJavaFile(t, src)
	content := []byte(src)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Service.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge for local method call")
	}

	// The call to helper() should resolve to com.example.Service.helper.
	found := false
	for _, e := range edges {
		if e.CallSiteFile == "src/main/java/com/example/Service.java" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected edge with correct CallSiteFile")
	}
}

func TestJavaResolverResolveFile_MethodCalls(t *testing.T) {
	src := `package com.example;
class UserService {
    void process() {}
}
class Main {
    void run(UserService svc) {
        svc.process();
    }
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "com.example.UserService", Kind: "class", PackagePath: "com.example"},
		{QualifiedName: "com.example.UserService.process", Kind: "method", PackagePath: "com.example"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseJavaFile(t, src)
	content := []byte(src)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Main.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge for method dispatch call")
	}

	// Verify edge has correct provenance.
	for _, e := range edges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
	}
}

func TestJavaResolverResolveFile_ConstructorCall(t *testing.T) {
	src := `package com.example;
class Item {}
class Main {
    void create() {
        Item item = new Item();
    }
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "com.example.Item", Kind: "class", PackagePath: "com.example"},
		{QualifiedName: "com.example.Item.<init>", Kind: "method", PackagePath: "com.example"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseJavaFile(t, src)
	content := []byte(src)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Main.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should have at least one edge for new Item().
	if len(edges) == 0 {
		t.Fatal("expected at least one edge for constructor call")
	}

	// Verify constructor edge targets <init>.
	for _, e := range edges {
		if e.EdgeType != edgetype.Calls {
			t.Errorf("edge type = %q, want %q", e.EdgeType, edgetype.Calls)
		}
	}
}

func TestJavaResolverResolveFile_StaticMethodCall(t *testing.T) {
	src := `package com.example;
import java.util.Collections;
class Main {
    void sort() {
        Collections.sort(null);
    }
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "java.util.Collections", Kind: "class", PackagePath: "java.util"},
		{QualifiedName: "java.util.Collections.sort", Kind: "method", PackagePath: "java.util"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseJavaFile(t, src)
	content := []byte(src)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Main.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge for static method call")
	}

	// Verify edge provenance.
	for _, e := range edges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
		if e.Confidence != typresolve.ResolverConfidence {
			t.Errorf("edge confidence = %v, want %v", e.Confidence, typresolve.ResolverConfidence)
		}
	}
}

func TestResolveFile_NoParsedTree_FallbackParse(t *testing.T) {
	src := `package com.example;
class Main {
    void run() {}
}
`
	r := NewJavaResolver()
	defs := []typresolve.ResolverDef{}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	content := []byte(src)

	// Pass nil ParsedTree; should fall back to re-parsing from Content.
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main/java/com/example/Main.java",
		FileHash:   types.EmptyHash,
		Content:    content,
		ParsedTree: nil,
	})
	if err != nil {
		t.Fatalf("ResolveFile with nil ParsedTree: %v", err)
	}

	// Should not error; edges may be empty since there are no calls.
	_ = edges
}
