package tsresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
)


func TestTypeScriptResolverLanguage(t *testing.T) {
	r := NewTypeScriptResolver()
	if got := r.Language(); got != "typescript" {
		t.Fatalf("Language() = %q, want %q", got, "typescript")
	}
}

func TestTypeScriptResolverInitWorkspace(t *testing.T) {
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "src/utils.helper", Kind: "function"},
		{QualifiedName: "src/models.User", Kind: "class"},
		{QualifiedName: "src/models.User.getName", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace failed: %v", err)
	}
	if r.registry == nil {
		t.Fatal("registry is nil after InitWorkspace")
	}
	if r.registry.LookupFunc("src/utils.helper") == nil {
		t.Error("expected helper function in registry")
	}
	if r.registry.LookupType("src/models.User") == nil {
		t.Error("expected User type in registry")
	}
	if r.registry.LookupMethod("src/models.User", "getName") == nil {
		t.Error("expected User.getName method in registry")
	}
}

func TestTypeScriptResolverResolveFile_ImportCall(t *testing.T) {
	src := `import { readFileSync } from 'fs';

function loadConfig() {
    const data = readFileSync('config.json');
}
`
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "fs.readFileSync", Kind: "function"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/config.ts",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls && e.Provenance == typresolve.ProvenanceResolverResolved {
			if e.Confidence != typresolve.ResolverConfidence {
				t.Errorf("edge confidence = %v, want %v", e.Confidence, typresolve.ResolverConfidence)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("expected a calls edge for readFileSync, got %d edges total", len(edges))
	}
}

func TestTypeScriptResolverResolveFile_LocalCalls(t *testing.T) {
	src := `function helper() {}
function main() { helper(); }
`
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app.helper", Kind: "function"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "app.ts",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a calls edge for helper(), got %d edges total", len(edges))
	}
}

func TestTypeScriptResolverResolveFile_MethodCalls(t *testing.T) {
	src := `class MyService {
    getData(): string { return ""; }
}
function use(svc: MyService) {
    svc.getData();
}
`
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app.MyService", Kind: "class"},
		{QualifiedName: "app.MyService.getData", Kind: "method"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "app.ts",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a calls edge for svc.getData(), got %d edges total", len(edges))
	}
}

func TestTypeScriptResolverResolveFile_NewExpression(t *testing.T) {
	src := `class MyClass {}
function create() {
    const obj = new MyClass();
}
`
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app.MyClass", Kind: "class"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "app.ts",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a calls edge for new MyClass(), got %d edges total", len(edges))
	}
}

func TestTypeScriptResolverResolveFile_JSXElement(t *testing.T) {
	// The TypeScript grammar may not parse JSX; use TSX if available.
	// Try parsing with the standard TS grammar first.
	src := `import React from 'react';
function Header() { return null; }
function App() { return <Header />; }
`
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Skip("TSX grammar not available")
	}
	root := tree.RootNode()
	// Check if JSX was parsed (look for jsx nodes).
	hasJSX := containsNodeType(root, "jsx_self_closing_element")
	tree.Close()
	if !hasJSX {
		t.Skip("TypeScript grammar does not parse JSX; TSX variant needed")
	}

	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app.Header", Kind: "function"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "app.tsx",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a calls edge for <Header />, got %d edges total", len(edges))
	}
}

func TestResolveCallsInFile_AwaitUnwrap(t *testing.T) {
	src := `async function fetch() {
    const resp = await getResponse();
    resp.json();
}
`
	r := NewTypeScriptResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app.getResponse", Kind: "function"},
		{QualifiedName: "Response.json", Kind: "method"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	// Register Response type and getResponse function with a Promise<Response> return.
	r.registry.AddType(typresolve.RegisteredType{
		QualifiedName: "Response",
		ShortName:     "Response",
	})
	// Update getResponse to return Promise<Response>.
	promiseResponse := typresolve.Named("Promise")
	promiseResponse.Elem = typresolve.Named("Response")
	r.registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "app.getResponse",
		ShortName:     "getResponse",
		Signature: typresolve.Func(
			nil,
			[]*typresolve.Type{promiseResponse},
		),
		MinParams: -1,
	})

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "app.ts",
		FileHash: types.EmptyHash,
		Content:  []byte(src),
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// We expect at least one call edge (for getResponse).
	if len(edges) == 0 {
		t.Error("expected at least one call edge")
	}
	for _, e := range edges {
		if e.EdgeType != edgetype.Calls {
			t.Errorf("unexpected edge type %q", e.EdgeType)
		}
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
	}
}

// containsNodeType recursively checks if the AST contains a node of the given type.
func containsNodeType(node *sitter.Node, nodeType string) bool {
	if node == nil {
		return false
	}
	if node.Type() == nodeType {
		return true
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		if containsNodeType(node.Child(i), nodeType) {
			return true
		}
	}
	return false
}

// TestBuildRegistry verifies the registry builder.
func TestBuildRegistry(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "src/utils.helper", Kind: "function"},
		{QualifiedName: "src/models.User", Kind: "class"},
		{QualifiedName: "src/models.User.getName", Kind: "method"},
		{QualifiedName: "src/api.Handler", Kind: "interface"},
	}

	reg := BuildRegistry(defs)

	if f := reg.LookupFunc("src/utils.helper"); f == nil {
		t.Error("expected helper function")
	} else if f.ShortName != "helper" {
		t.Errorf("ShortName = %q, want %q", f.ShortName, "helper")
	}

	if tp := reg.LookupType("src/models.User"); tp == nil {
		t.Error("expected User type")
	} else if tp.ShortName != "User" {
		t.Errorf("ShortName = %q, want %q", tp.ShortName, "User")
	}

	if m := reg.LookupMethod("src/models.User", "getName"); m == nil {
		t.Error("expected User.getName method")
	} else {
		if m.ReceiverType != "src/models.User" {
			t.Errorf("ReceiverType = %q, want %q", m.ReceiverType, "src/models.User")
		}
		if m.ShortName != "getName" {
			t.Errorf("ShortName = %q, want %q", m.ShortName, "getName")
		}
	}

	if tp := reg.LookupType("src/api.Handler"); tp == nil {
		t.Error("expected Handler interface")
	} else if !tp.IsInterface {
		t.Error("expected IsInterface = true")
	}
}

// TestInferModuleQN verifies module QN derivation from file paths.
func TestInferModuleQN(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"src/compiler/checker.ts", "src/compiler/checker"},
		{"app.ts", "app"},
		{"utils/index.tsx", "utils/index"},
		{"main.js", "main"},
	}
	for _, tt := range tests {
		got := inferModuleQN(tt.path)
		if got != tt.want {
			t.Errorf("inferModuleQN(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
