package goresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	golang "github.com/smacker/go-tree-sitter/golang"
)

// parseGo parses Go source code and returns the root node.
func parseGo(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

func TestGoResolverLanguage(t *testing.T) {
	r := NewGoResolver()
	if got := r.Language(); got != "go" {
		t.Errorf("Language() = %q, want %q", got, "go")
	}
}

func TestGoResolverInitWorkspace(t *testing.T) {
	r := NewGoResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "fmt.Println", Kind: "function", PackagePath: "fmt"},
		{QualifiedName: "main.MyType", Kind: "type", PackagePath: "main"},
		{QualifiedName: "main.MyInterface", Kind: "interface", PackagePath: "main"},
		{QualifiedName: "main.MyType.DoWork", Kind: "method", PackagePath: "main"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	if r.registry == nil {
		t.Fatal("registry is nil after InitWorkspace")
	}

	// Verify functions are registered.
	if f := r.registry.LookupSymbol("fmt", "Println"); f == nil {
		t.Error("expected fmt.Println in registry")
	}

	// Verify types are registered.
	if tp := r.registry.LookupType("main.MyType"); tp == nil {
		t.Error("expected main.MyType in registry")
	}

	// Verify interfaces are registered as types with IsInterface=true.
	if tp := r.registry.LookupType("main.MyInterface"); tp == nil {
		t.Error("expected main.MyInterface in registry")
	} else if !tp.IsInterface {
		t.Error("expected main.MyInterface.IsInterface=true")
	}

	// Verify methods are registered.
	if m := r.registry.LookupMethod("main.MyType", "DoWork"); m == nil {
		t.Error("expected main.MyType.DoWork in registry")
	}
}

func TestGoResolverResolveFile_SimpleCalls(t *testing.T) {
	src := `package main

import "fmt"

func hello() {
	fmt.Println("hello")
}
`
	r := NewGoResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "fmt.Println", Kind: "function", PackagePath: "fmt"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseGo(t, src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "main.go",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge from hello -> fmt.Println")
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls &&
			e.Provenance == typresolve.ProvenanceResolverResolved &&
			e.Confidence == typresolve.ResolverConfidence {
			found = true
			if e.CallSiteFile != "main.go" {
				t.Errorf("CallSiteFile = %q, want %q", e.CallSiteFile, "main.go")
			}
			if e.CallSiteLine < 1 {
				t.Errorf("CallSiteLine = %d, want >= 1", e.CallSiteLine)
			}
		}
	}
	if !found {
		t.Error("no edge with correct provenance and confidence found")
	}
}

func TestGoResolverResolveFile_LocalCalls(t *testing.T) {
	src := `package main

func greet() {
	helper()
}

func helper() {}
`
	r := NewGoResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "main.helper", Kind: "function", PackagePath: "main"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseGo(t, src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "main.go",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge from greet -> helper")
	}

	for _, e := range edges {
		if e.EdgeType != edgetype.Calls {
			t.Errorf("unexpected edge type %q", e.EdgeType)
		}
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
		if e.Confidence != typresolve.ResolverConfidence {
			t.Errorf("confidence = %f, want %f", e.Confidence, typresolve.ResolverConfidence)
		}
	}
}

func TestGoResolverResolveFile_MethodCalls(t *testing.T) {
	src := `package main

type Server struct{}

func (s *Server) Start() {}

func run() {
	var s Server
	s.Start()
}
`
	r := NewGoResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "main.Server", Kind: "type", PackagePath: "main"},
		{QualifiedName: "main.Server.Start", Kind: "method", PackagePath: "main"},
	}
	if err := r.InitWorkspace(context.Background(), defs); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseGo(t, src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "main.go",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge from run -> Server.Start")
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls &&
			e.Provenance == typresolve.ProvenanceResolverResolved {
			found = true
		}
	}
	if !found {
		t.Error("no method dispatch edge found")
	}
}

func TestResolveCallsInFile_ScopeTracking(t *testing.T) {
	src := `package main

type Client struct{}

func (c *Client) Do() {}

func process() {
	c := Client{}
	c.Do()
}
`
	root := parseGo(t, src)
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "main.Client",
		ShortName:     "Client",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "main.Client.Do",
		ShortName:     "Do",
		ReceiverType:  "main.Client",
		MinParams:     -1,
	})

	imports := BuildImportMap(root, []byte(src))
	rctx := &ResolveContext{
		Registry: reg,
		Scope:    typresolve.NewScope(nil),
		Imports:  imports,
		PkgQN:    "main",
		Content:  []byte(src),
	}

	edges := ResolveCallsInFile(rctx, root, types.Hash{}, "", "main.go")

	if len(edges) == 0 {
		t.Fatal("expected at least one edge from process -> Client.Do")
	}

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Error("no scope-tracked method call edge found")
	}
}

func TestResolveCallsInFile_RangeLoop(t *testing.T) {
	src := `package main

type Item struct{}

func (it *Item) Process() {}

func handleItems() {
	items := make([]Item, 0)
	for _, v := range items {
		v.Process()
	}
}
`
	root := parseGo(t, src)
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "main.Item",
		ShortName:     "Item",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "main.Item.Process",
		ShortName:     "Process",
		ReceiverType:  "main.Item",
		MinParams:     -1,
	})

	imports := BuildImportMap(root, []byte(src))
	rctx := &ResolveContext{
		Registry: reg,
		Scope:    typresolve.NewScope(nil),
		Imports:  imports,
		PkgQN:    "main",
		Content:  []byte(src),
	}

	edges := ResolveCallsInFile(rctx, root, types.Hash{}, "", "main.go")

	// We expect at least one edge for v.Process() call.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	if !found {
		t.Error("no range loop method call edge found; expected v.Process() to resolve")
	}
}

func TestBuildRegistry(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "pkg.FuncA", Kind: "function"},
		{QualifiedName: "pkg.TypeB", Kind: "type"},
		{QualifiedName: "pkg.IfaceC", Kind: "interface"},
		{QualifiedName: "pkg.TypeB.MethodD", Kind: "method"},
	}

	reg := BuildRegistry(defs, nil, nil)

	if reg.FuncCount() != 2 { // FuncA + MethodD
		t.Errorf("FuncCount = %d, want 2", reg.FuncCount())
	}
	if reg.TypeCount() != 2 { // TypeB + IfaceC
		t.Errorf("TypeCount = %d, want 2", reg.TypeCount())
	}

	if f := reg.LookupFunc("pkg.FuncA"); f == nil {
		t.Error("expected pkg.FuncA in registry")
	} else if f.ShortName != "FuncA" {
		t.Errorf("ShortName = %q, want %q", f.ShortName, "FuncA")
	}

	if tp := reg.LookupType("pkg.IfaceC"); tp == nil {
		t.Error("expected pkg.IfaceC in registry")
	} else if !tp.IsInterface {
		t.Error("expected pkg.IfaceC.IsInterface=true")
	}

	if m := reg.LookupMethod("pkg.TypeB", "MethodD"); m == nil {
		t.Error("expected method TypeB.MethodD in registry")
	}
}

func TestResolveFile_NoParsedTree_FallbackParse(t *testing.T) {
	src := `package main

func foo() {}
`
	r := NewGoResolver()
	if err := r.InitWorkspace(context.Background(), nil); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	// Pass nil ParsedTree; should fall back to re-parsing from Content.
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "main.go",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: nil,
	})
	if err != nil {
		t.Fatalf("ResolveFile with nil ParsedTree: %v", err)
	}
	// No calls in this file, so no edges expected, but no error.
	_ = edges
}
