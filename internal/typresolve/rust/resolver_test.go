package rustresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestRustResolverLanguage(t *testing.T) {
	r := NewRustResolver()
	if r.Language() != "rust" {
		t.Errorf("Language() = %q, want %q", r.Language(), "rust")
	}
}

func TestRustResolverInitWorkspace(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "function", QualifiedName: "crate::config.load"},
		{Kind: "type", QualifiedName: "crate::config.Config"},
		{Kind: "method", QualifiedName: "crate::config.Config.new"},
		{Kind: "interface", QualifiedName: "crate::traits.Serializable"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	if r.registry == nil {
		t.Fatal("registry is nil after InitWorkspace")
	}

	// Verify function is registered.
	if f := r.registry.LookupFunc("crate::config.load"); f == nil {
		t.Error("function crate::config.load not found in registry")
	}

	// Verify type is registered.
	if typ := r.registry.LookupType("crate::config.Config"); typ == nil {
		t.Error("type crate::config.Config not found in registry")
	}

	// Verify method is registered.
	if m := r.registry.LookupMethod("crate::config.Config", "new"); m == nil {
		t.Error("method Config.new not found in registry")
	}

	// Verify interface is registered.
	if typ := r.registry.LookupType("crate::traits.Serializable"); typ == nil {
		t.Error("interface crate::traits.Serializable not found in registry")
	} else if !typ.IsInterface {
		t.Error("Serializable should be marked as interface")
	}
}

func TestRustResolverResolveFile_FunctionCall(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "type", QualifiedName: "crate::config.Config"},
		{Kind: "method", QualifiedName: "crate::config.Config.new"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`use crate::config::Config;

fn process() {
    Config::new();
}
`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/main.rs",
		Content:  src,
		FileHash: types.Hash{},
	})
	if err != nil {
		t.Fatalf("ResolveFile() error = %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge, got 0")
	}

	// Find the call edge.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls &&
			e.Provenance == typresolve.ProvenanceResolverResolved &&
			e.Confidence == typresolve.ResolverConfidence {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no call edge with correct provenance and confidence found; edges = %+v", edges)
	}
}

func TestRustResolverResolveFile_MethodCall(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "type", QualifiedName: "crate::server.Server"},
		{Kind: "method", QualifiedName: "crate::server.Server.new"},
		{Kind: "method", QualifiedName: "crate::server.Server.start"},
		{Kind: "method", QualifiedName: "crate::server.Server.listen"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`struct Server {
    port: u16,
}

impl Server {
    fn new(port: u16) -> Self {
        Server { port }
    }

    fn start(&self) {
        self.listen();
    }

    fn listen(&self) {}
}
`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/server.rs",
		Content:  src,
		FileHash: types.Hash{},
	})
	if err != nil {
		t.Fatalf("ResolveFile() error = %v", err)
	}

	// Verify: start -> listen edge exists.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected call edge from start -> listen, got edges: %+v", edges)
	}
}

func TestRustResolverResolveFile_TraitMethod(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "type", QualifiedName: "crate.Person"},
		{Kind: "interface", QualifiedName: "crate.Greetable"},
		{Kind: "method", QualifiedName: "crate.Greetable.greet"},
		// Implements edge: Person implements Greetable.
		{Kind: "implements", QualifiedName: "crate.Person", Signature: "crate.Greetable"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`trait Greetable {
    fn greet(&self) -> String;
}

struct Person {
    name: String,
}

impl Greetable for Person {
    fn greet(&self) -> String {
        format!("Hello, {}", self.name)
    }
}

fn say_hello(p: &Person) {
    p.greet();
}
`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/greet.rs",
		Content:  src,
		FileHash: types.Hash{},
	})
	if err != nil {
		t.Fatalf("ResolveFile() error = %v", err)
	}

	// Verify: say_hello -> greet edge via trait dispatch.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls &&
			e.Provenance == typresolve.ProvenanceResolverResolved {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected call edge say_hello -> greet via trait dispatch, got edges: %+v", edges)
	}
}

func TestRustResolverResolveFile_ScopeTracking(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "type", QualifiedName: "crate.Config"},
		{Kind: "method", QualifiedName: "crate.Config.new"},
		{Kind: "method", QualifiedName: "crate.Config.port"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`struct Config {
    port: u16,
}

impl Config {
    fn new() -> Self {
        Config { port: 8080 }
    }

    fn port(&self) -> u16 {
        self.port
    }
}

fn run() {
    let config = Config::new();
    config.port();
}
`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/config.rs",
		Content:  src,
		FileHash: types.Hash{},
	})
	if err != nil {
		t.Fatalf("ResolveFile() error = %v", err)
	}

	// Expect at least 2 edges: Config::new() and config.port()
	callEdges := 0
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			callEdges++
		}
	}
	if callEdges < 2 {
		t.Errorf("expected at least 2 call edges (Config::new, config.port), got %d; edges: %+v", callEdges, edges)
	}
}

func TestBuildRegistry_StructsAndMethods(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{Kind: "type", QualifiedName: "crate::server.Server"},
		{Kind: "method", QualifiedName: "crate::server.Server.new"},
		{Kind: "method", QualifiedName: "crate::server.Server.start"},
		{Kind: "method", QualifiedName: "crate::server.Server.listen"},
	}

	reg := BuildRegistry(defs)

	// Verify type is registered.
	typ := reg.LookupType("crate::server.Server")
	if typ == nil {
		t.Fatal("type crate::server.Server not found")
	}

	// Verify methods are registered and findable via LookupMethod.
	methods := []string{"new", "start", "listen"}
	for _, m := range methods {
		if f := LookupMethod(reg, "crate::server.Server", m); f == nil {
			t.Errorf("method %s not found on Server", m)
		}
	}
}

func TestRustResolverResolveFile_MacroInvocation(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`fn process() {
    let items = vec![1, 2, 3];
    println!("items: {:?}", items);
}
`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "src/lib.rs",
		Content:  src,
		FileHash: types.Hash{},
	})
	if err != nil {
		t.Fatalf("ResolveFile() error = %v", err)
	}

	// Standard macros (vec!, println!) should be skipped.
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			t.Errorf("unexpected call edge for std macro: %+v", e)
		}
	}
}

func TestResolveFile_NoParsedTree_FallbackParse(t *testing.T) {
	r := NewRustResolver()
	defs := []typresolve.ResolverDef{
		{Kind: "function", QualifiedName: "crate.hello"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}

	src := []byte(`fn main() {
    hello();
}
`)

	// Pass nil ParsedTree with valid Content.
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath:   "src/main.rs",
		Content:    src,
		FileHash:   types.Hash{},
		ParsedTree: nil,
	})
	if err != nil {
		t.Fatalf("ResolveFile() with nil ParsedTree error = %v", err)
	}

	// Should produce at least one edge for hello() call.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected call edge for hello(), got none; edges: %+v", edges)
	}
}
