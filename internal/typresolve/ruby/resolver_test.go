package rubyresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"
)

// parseRubyResolver is a test helper that parses Ruby source and returns the root node.
func parseRubyResolver(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(ruby.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

func TestRubyResolverLanguage(t *testing.T) {
	r := NewRubyResolver()
	if got := r.Language(); got != "ruby" {
		t.Errorf("Language() = %q, want %q", got, "ruby")
	}
}

func TestRubyResolverInitWorkspace(t *testing.T) {
	r := NewRubyResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app/models/user.User", Kind: "type", Signature: "class User"},
		{QualifiedName: "app/models/user.User.initialize", Kind: "method"},
		{QualifiedName: "app/helpers.helper_func", Kind: "function"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace() error = %v", err)
	}
}

func TestRubyResolverResolveFile_SimpleMethodCall(t *testing.T) {
	src := `require "json"

def process
  JSON.parse('{}')
end
`
	r := NewRubyResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "json.JSON", Kind: "type", Signature: "module JSON"},
		{QualifiedName: "json.JSON.parse", Kind: "method"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseRubyResolver(t, src)
	opts := typresolve.ResolveFileOpts{
		FilePath:   "app/processor.rb",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	}

	edges, err := r.ResolveFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	if len(edges) == 0 {
		t.Fatal("expected at least one edge, got 0")
	}

	// Find the call edge.
	var found bool
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			if e.Provenance != typresolve.ProvenanceResolverResolved {
				t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
			}
			if e.Confidence != typresolve.ResolverConfidence {
				t.Errorf("edge confidence = %v, want %v", e.Confidence, typresolve.ResolverConfidence)
			}
			if e.CallSiteFile != "app/processor.rb" {
				t.Errorf("edge CallSiteFile = %q, want %q", e.CallSiteFile, "app/processor.rb")
			}
			found = true
		}
	}

	if !found {
		t.Error("no calls edge found in resolved edges")
	}
}

func TestRubyResolverResolveFile_ClassMethod(t *testing.T) {
	src := `class User
  def initialize(name)
    @name = name
  end

  def greet
    puts @name
  end
end

user = User.new("Alice")
user.greet
`
	r := NewRubyResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app/user.User", Kind: "type", Signature: "class User"},
		{QualifiedName: "app/user.User.initialize", Kind: "method"},
		{QualifiedName: "app/user.User.greet", Kind: "method"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseRubyResolver(t, src)
	opts := typresolve.ResolveFileOpts{
		FilePath:   "app/user.rb",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	}

	edges, err := r.ResolveFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should have at least two call edges: User.new -> initialize, user.greet.
	callEdges := filterCallEdges(edges)
	if len(callEdges) < 2 {
		t.Errorf("expected at least 2 call edges, got %d", len(callEdges))
		for i, e := range edges {
			t.Logf("  edge[%d]: type=%s, file=%s, line=%d", i, e.EdgeType, e.CallSiteFile, e.CallSiteLine)
		}
	}

	// All edges must have correct provenance and confidence.
	for _, e := range callEdges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
		if e.Confidence != typresolve.ResolverConfidence {
			t.Errorf("edge confidence = %v, want %v", e.Confidence, typresolve.ResolverConfidence)
		}
	}
}

func TestRubyResolverResolveFile_ModuleInclude(t *testing.T) {
	src := `module Greetable
  def greet
    "hello"
  end
end

class Person
  include Greetable

  def say_hi
    greet
  end
end
`
	r := NewRubyResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app/person.Greetable", Kind: "type", Signature: "module Greetable"},
		{QualifiedName: "app/person.Greetable.greet", Kind: "method"},
		{QualifiedName: "app/person.Person", Kind: "type", Signature: "class Person"},
		// Register include relationship.
		{QualifiedName: "app/person.Person", Kind: "implements", Signature: "app/person.Greetable"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseRubyResolver(t, src)
	opts := typresolve.ResolveFileOpts{
		FilePath:   "app/person.rb",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	}

	edges, err := r.ResolveFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	callEdges := filterCallEdges(edges)
	if len(callEdges) == 0 {
		t.Fatal("expected at least one call edge for greet, got 0")
	}

	// Verify provenance on all edges.
	for _, e := range callEdges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
	}
}

func TestRubyResolverResolveFile_ScopeTracking(t *testing.T) {
	src := `class Example
  def run
    result = compute
    result.to_s
  end

  def compute
    42
  end
end
`
	r := NewRubyResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app/example.Example", Kind: "type", Signature: "class Example"},
		{QualifiedName: "app/example.Example.run", Kind: "method"},
		{QualifiedName: "app/example.Example.compute", Kind: "method"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	root := parseRubyResolver(t, src)
	opts := typresolve.ResolveFileOpts{
		FilePath:   "app/example.rb",
		FileHash:   types.Hash{},
		Content:    []byte(src),
		ParsedTree: root,
	}

	edges, err := r.ResolveFile(context.Background(), opts)
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	callEdges := filterCallEdges(edges)
	if len(callEdges) == 0 {
		t.Fatal("expected call edges from scope tracking, got 0")
	}

	// Verify all edges have correct provenance.
	for _, e := range callEdges {
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, typresolve.ProvenanceResolverResolved)
		}
	}
}

func TestBuildRegistry_ClassesAndMethods(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "app/models/user.User", Kind: "type", Signature: "class User"},
		{QualifiedName: "app/models/user.User.initialize", Kind: "method"},
		{QualifiedName: "app/models/user.User.name", Kind: "method"},
		{QualifiedName: "app/helpers.format_name", Kind: "function"},
	}

	reg := BuildRegistry(defs)

	// Verify type is registered.
	userType := reg.LookupType("app/models/user.User")
	if userType == nil {
		t.Fatal("expected User type to be registered")
	}
	if userType.ShortName != "User" {
		t.Errorf("User ShortName = %q, want %q", userType.ShortName, "User")
	}

	// Verify methods are registered and accessible via LookupMethod.
	initMethod := reg.LookupMethod("app/models/user.User", "initialize")
	if initMethod == nil {
		t.Fatal("expected User.initialize method to be registered")
	}
	if initMethod.ReceiverType != "app/models/user.User" {
		t.Errorf("initialize ReceiverType = %q, want %q", initMethod.ReceiverType, "app/models/user.User")
	}

	nameMethod := reg.LookupMethod("app/models/user.User", "name")
	if nameMethod == nil {
		t.Fatal("expected User.name method to be registered")
	}

	// Verify function is registered.
	fn := reg.LookupFunc("app/helpers.format_name")
	if fn == nil {
		t.Fatal("expected format_name function to be registered")
	}
	if fn.ShortName != "format_name" {
		t.Errorf("format_name ShortName = %q, want %q", fn.ShortName, "format_name")
	}

	// Verify LookupAttribute follows MRO.
	f := LookupAttribute(reg, "app/models/user.User", "initialize")
	if f == nil {
		t.Fatal("expected LookupAttribute to find initialize")
	}
}

func TestBuildRegistry_ModuleInclusion(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "lib/greetable.Greetable", Kind: "type", Signature: "module Greetable"},
		{QualifiedName: "lib/greetable.Greetable.greet", Kind: "method"},
		{QualifiedName: "app/person.Person", Kind: "type", Signature: "class Person"},
		{QualifiedName: "app/person.Person", Kind: "implements", Signature: "lib/greetable.Greetable"},
	}

	reg := BuildRegistry(defs)

	// Verify the Greetable module is registered as interface.
	greetable := reg.LookupType("lib/greetable.Greetable")
	if greetable == nil {
		t.Fatal("expected Greetable to be registered")
	}
	if !greetable.IsInterface {
		t.Error("expected Greetable to have IsInterface=true (module)")
	}

	// Verify Person has Greetable in EmbeddedTypes.
	person := reg.LookupType("app/person.Person")
	if person == nil {
		t.Fatal("expected Person to be registered")
	}
	if len(person.EmbeddedTypes) == 0 {
		t.Fatal("expected Person to have EmbeddedTypes with Greetable")
	}
	if person.EmbeddedTypes[0] != "lib/greetable.Greetable" {
		t.Errorf("Person EmbeddedTypes[0] = %q, want %q", person.EmbeddedTypes[0], "lib/greetable.Greetable")
	}

	// Verify LookupAttribute can find greet via MRO.
	f := LookupAttribute(reg, "app/person.Person", "greet")
	if f == nil {
		t.Fatal("expected LookupAttribute to find greet on Person via MRO")
	}
	if f.QualifiedName != "lib/greetable.Greetable.greet" {
		t.Errorf("found method QN = %q, want %q", f.QualifiedName, "lib/greetable.Greetable.greet")
	}
}

// filterCallEdges returns only edges with EdgeType == "calls".
func filterCallEdges(edges []types.Edge) []types.Edge {
	var result []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			result = append(result, e)
		}
	}
	return result
}
