package cppextractor

import (
	"context"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// testOpts creates minimal ExtractOptions for testing.
func testOpts(filePath, content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:    "test-repo",
		RepoHash:   types.EmptyHash,
		CommitHash: "abc123",
		FilePath:   filePath,
		FileHash:   types.EmptyHash,
		Content:    []byte(content),
		ModuleRoot: "/test",
	}
}

// extract runs the extractor on inline C++ source and returns nodes and edges.
func extract(t *testing.T, filePath, content string) ([]types.Node, []types.Edge) {
	t.Helper()
	extractor := NewCppExtractor()
	opts := testOpts(filePath, content)
	result, err := extractor.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	return result.Nodes, result.Edges
}

// findNode returns the first node matching the given kind and name substring.
func findNode(nodes []types.Node, kind, nameSubstring string) *types.Node {
	for i := range nodes {
		if nodes[i].Kind == kind && strings.Contains(nodes[i].QualifiedName, nameSubstring) {
			return &nodes[i]
		}
	}
	return nil
}

// findEdge returns the first edge matching the given type.
func findEdge(edges []types.Edge, edgeType string) *types.Edge {
	for i := range edges {
		if edges[i].EdgeType == edgeType {
			return &edges[i]
		}
	}
	return nil
}

func TestExtractFunction(t *testing.T) {
	src := `int foo(int x) { return x + 1; }`
	nodes, _ := extract(t, "src/main.cpp", src)

	n := findNode(nodes, types.KindFunction, "foo")
	if n == nil {
		t.Fatal("expected function node for foo")
	}
	if n.Kind != types.KindFunction {
		t.Errorf("kind = %q, want %q", n.Kind, types.KindFunction)
	}
	if !strings.Contains(n.QualifiedName, "foo") {
		t.Errorf("QualifiedName = %q, want to contain 'foo'", n.QualifiedName)
	}
	if n.Signature == "" {
		t.Error("expected non-empty signature")
	}
}

func TestExtractFunctionWithBody(t *testing.T) {
	src := `int add(int a, int b) {
    int result = a + b;
    return result;
}`
	nodes, _ := extract(t, "src/math.cpp", src)

	n := findNode(nodes, types.KindFunction, "add")
	if n == nil {
		t.Fatal("expected function node for add")
	}
	if n.Line != 1 {
		t.Errorf("Line = %d, want 1", n.Line)
	}
}

func TestExtractClass(t *testing.T) {
	src := `class Foo {
public:
    void bar();
    int x;
};`
	nodes, edges := extract(t, "include/foo.hpp", src)

	// Class node.
	cls := findNode(nodes, types.KindType, "Foo")
	if cls == nil {
		t.Fatal("expected class node for Foo")
	}
	if cls.Kind != types.KindType {
		t.Errorf("kind = %q, want %q", cls.Kind, types.KindType)
	}

	// Method node.
	method := findNode(nodes, types.KindMethod, "bar")
	if method == nil {
		t.Fatal("expected method node for bar")
	}

	// Field node.
	field := findNode(nodes, types.KindField, "x")
	if field == nil {
		t.Fatal("expected field node for x")
	}

	// Containment edges.
	hasContainEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Contains && e.SourceHash == cls.NodeHash {
			hasContainEdge = true
			break
		}
	}
	if !hasContainEdge {
		t.Error("expected contains edge from Foo to its members")
	}

	// Member_of edges.
	hasMemberOfEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.MemberOf && e.TargetHash == cls.NodeHash {
			hasMemberOfEdge = true
			break
		}
	}
	if !hasMemberOfEdge {
		t.Error("expected member_of edge from member to Foo")
	}
}

func TestExtractClassWithInheritance(t *testing.T) {
	src := `class Derived : public Base {
public:
    void method();
};`
	nodes, edges := extract(t, "src/derived.cpp", src)

	cls := findNode(nodes, types.KindType, "Derived")
	if cls == nil {
		t.Fatal("expected class node for Derived")
	}

	// Extends edge.
	hasExtendsEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Extends && e.SourceHash == cls.NodeHash {
			hasExtendsEdge = true
			break
		}
	}
	if !hasExtendsEdge {
		t.Error("expected extends edge from Derived to Base")
	}
}

func TestExtractStruct(t *testing.T) {
	src := `struct Point {
    float x;
    float y;
};`
	nodes, _ := extract(t, "include/geom.hpp", src)

	s := findNode(nodes, types.KindType, "Point")
	if s == nil {
		t.Fatal("expected struct node for Point")
	}

	xField := findNode(nodes, types.KindField, "x")
	if xField == nil {
		t.Fatal("expected field node for x")
	}

	yField := findNode(nodes, types.KindField, "y")
	if yField == nil {
		t.Fatal("expected field node for y")
	}
}

func TestExtractNamespace(t *testing.T) {
	src := `namespace A {
    void func();
}`
	nodes, edges := extract(t, "src/ns.cpp", src)

	ns := findNode(nodes, types.KindType, "A")
	if ns == nil {
		t.Fatal("expected namespace node for A")
	}
	if !strings.Contains(ns.Signature, "namespace") {
		t.Errorf("signature = %q, want to contain 'namespace'", ns.Signature)
	}

	// Function inside namespace should exist.
	fn := findNode(nodes, types.KindFunction, "func")
	if fn == nil {
		t.Fatal("expected function node for func")
	}

	// Containment edge from namespace to function.
	hasContain := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Contains && e.SourceHash == ns.NodeHash && e.TargetHash == fn.NodeHash {
			hasContain = true
			break
		}
	}
	if !hasContain {
		t.Error("expected contains edge from namespace A to func")
	}
}

func TestExtractEnum(t *testing.T) {
	src := `enum Color {
    RED = 0,
    GREEN = 1,
    BLUE = 2
};`
	nodes, edges := extract(t, "include/types.hpp", src)

	enumNode := findNode(nodes, types.KindType, "Color")
	if enumNode == nil {
		t.Fatal("expected enum node for Color")
	}

	// Enum values.
	redNode := findNode(nodes, types.KindConst, "RED")
	if redNode == nil {
		t.Fatal("expected const node for RED")
	}

	greenNode := findNode(nodes, types.KindConst, "GREEN")
	if greenNode == nil {
		t.Fatal("expected const node for GREEN")
	}

	// Containment edges from enum to values.
	containCount := 0
	for _, e := range edges {
		if e.EdgeType == edgetype.Contains && e.SourceHash == enumNode.NodeHash {
			containCount++
		}
	}
	if containCount < 2 {
		t.Errorf("expected at least 2 contains edges from enum, got %d", containCount)
	}
}

func TestExtractTypedef(t *testing.T) {
	src := `typedef int MyInt;`
	nodes, _ := extract(t, "include/types.hpp", src)

	n := findNode(nodes, types.KindType, "MyInt")
	if n == nil {
		t.Fatal("expected typedef node for MyInt")
	}
}

func TestExtractUsing(t *testing.T) {
	src := `using MyString = std::string;`
	nodes, _ := extract(t, "include/aliases.hpp", src)

	n := findNode(nodes, types.KindType, "MyString")
	if n == nil {
		t.Fatal("expected using node for MyString")
	}
}

func TestExtractMacro(t *testing.T) {
	src := `#define MAX_SIZE 1024`
	nodes, _ := extract(t, "include/config.hpp", src)

	n := findNode(nodes, types.KindConst, "MAX_SIZE")
	if n == nil {
		t.Fatal("expected macro node for MAX_SIZE")
	}
	if !strings.Contains(n.Signature, "#define") {
		t.Errorf("signature = %q, want to contain '#define'", n.Signature)
	}
}

func TestExtractCall(t *testing.T) {
	src := `void caller() {
    callee();
}`
	nodes, edges := extract(t, "src/main.cpp", src)

	caller := findNode(nodes, types.KindFunction, "caller")
	if caller == nil {
		t.Fatal("expected function node for caller")
	}

	// Call edge.
	hasCallEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls && e.SourceHash == caller.NodeHash {
			hasCallEdge = true
			if e.CallSiteLine == 0 {
				t.Error("expected non-zero CallSiteLine")
			}
			if e.CallSiteFile == "" {
				t.Error("expected non-empty CallSiteFile")
			}
			break
		}
	}
	if !hasCallEdge {
		t.Error("expected call edge from caller to callee")
	}
}

func TestExtractCallMethod(t *testing.T) {
	src := `class Foo {
    void bar() {
        baz();
    }
};`
	nodes, edges := extract(t, "src/foo.cpp", src)

	bar := findNode(nodes, types.KindMethod, "bar")
	if bar == nil {
		t.Fatal("expected method node for bar")
	}

	hasCallEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls && e.SourceHash == bar.NodeHash {
			hasCallEdge = true
			break
		}
	}
	if !hasCallEdge {
		t.Error("expected call edge from bar to baz")
	}
}

func TestExtractInclude(t *testing.T) {
	src := `#include "foo.h"`
	_, edges := extract(t, "src/main.cpp", src)

	hasIncludeEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Includes {
			hasIncludeEdge = true
			if e.Confidence != 0.7 {
				t.Errorf("confidence = %f, want 0.7 for unresolved include", e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("provenance = %q, want 'ast_inferred'", e.Provenance)
			}
			break
		}
	}
	if !hasIncludeEdge {
		t.Error("expected includes edge")
	}
}

func TestExtractSystemInclude(t *testing.T) {
	src := `#include <iostream>`
	_, edges := extract(t, "src/main.cpp", src)

	hasIncludeEdge := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Includes {
			hasIncludeEdge = true
			break
		}
	}
	if !hasIncludeEdge {
		t.Error("expected includes edge for system include")
	}
}

func TestExtractMultipleIncludes(t *testing.T) {
	src := `#include "foo.h"
#include "bar.h"
#include <vector>`
	_, edges := extract(t, "src/main.cpp", src)

	includeCount := 0
	for _, e := range edges {
		if e.EdgeType == edgetype.Includes {
			includeCount++
		}
	}
	if includeCount != 3 {
		t.Errorf("expected 3 include edges, got %d", includeCount)
	}
}

func TestCanHandle(t *testing.T) {
	extractor := NewCppExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"src/main.cpp", true},
		{"src/main.cc", true},
		{"src/main.cxx", true},
		{"src/main.c++", true},
		{"include/foo.hpp", true},
		{"include/foo.hh", true},
		{"include/foo.hxx", true},
		{"include/foo.h++", true},
		{"include/foo.h", true},
		{"src/main.c", true},
		{"src/main.py", false},
		{"src/main.go", false},
		{"src/main.rs", false},
		{"src/main.ts", false},
		{"build/src/main.cpp", false},
		{"cmake-build-debug/src/main.cpp", false},
		{".cache/src/main.cpp", false},
		{"third_party/lib/src.cpp", false},
		{"third-party/lib/src.cpp", false},
		{"external/lib/src.cpp", false},
		{"vendor/lib/src.cpp", false},
		{"_deps/lib/src.cpp", false},
	}

	for _, tt := range tests {
		got := extractor.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestExtractorName(t *testing.T) {
	extractor := NewCppExtractor()
	if name := extractor.Name(); name != "treesitter-cpp" {
		t.Errorf("Name() = %q, want %q", name, "treesitter-cpp")
	}
}

func TestExtractDeterministic(t *testing.T) {
	src := `int foo() { return 1; }
int bar() { return 2; }`

	// Run twice and verify same output.
	nodes1, edges1 := extract(t, "src/main.cpp", src)
	nodes2, edges2 := extract(t, "src/main.cpp", src)

	if len(nodes1) != len(nodes2) {
		t.Fatalf("node count mismatch: %d vs %d", len(nodes1), len(nodes2))
	}
	for i := range nodes1 {
		if nodes1[i].QualifiedName != nodes2[i].QualifiedName {
			t.Errorf("node[%d] QN mismatch: %q vs %q", i, nodes1[i].QualifiedName, nodes2[i].QualifiedName)
		}
		if nodes1[i].Kind != nodes2[i].Kind {
			t.Errorf("node[%d] kind mismatch: %q vs %q", i, nodes1[i].Kind, nodes2[i].Kind)
		}
	}

	if len(edges1) != len(edges2) {
		t.Fatalf("edge count mismatch: %d vs %d", len(edges1), len(edges2))
	}
	for i := range edges1 {
		if edges1[i].EdgeHash != edges2[i].EdgeHash {
			t.Errorf("edge[%d] hash mismatch", i)
		}
	}
}

func TestExtractClassWithNestedClass(t *testing.T) {
	src := `class Outer {
    class Inner {
        void method();
    };
};`
	nodes, _ := extract(t, "src/nested.cpp", src)

	outer := findNode(nodes, types.KindType, "Outer")
	if outer == nil {
		t.Fatal("expected class node for Outer")
	}

	inner := findNode(nodes, types.KindType, "Inner")
	if inner == nil {
		t.Fatal("expected class node for Inner")
	}
}

func TestExtractMethodWithParams(t *testing.T) {
	src := `class Foo {
    void process(int x, float y, const std::string& name);
};`
	nodes, _ := extract(t, "src/foo.cpp", src)

	process := findNode(nodes, types.KindMethod, "process")
	if process == nil {
		t.Fatal("expected method node for process")
	}
	if !strings.Contains(process.Signature, "Foo::process") {
		t.Errorf("signature = %q, want to contain 'Foo::process'", process.Signature)
	}
}

func TestExtractExternC(t *testing.T) {
	src := `extern "C" {
    void c_function();
}`
	nodes, _ := extract(t, "src/wrapper.cpp", src)

	fn := findNode(nodes, types.KindFunction, "c_function")
	if fn == nil {
		t.Fatal("expected function node for c_function inside extern C")
	}
}

func TestExtractNamespaceNested(t *testing.T) {
	src := `namespace A {
    namespace B {
        void deep_func();
    }
}`
	nodes, edges := extract(t, "src/nested_ns.cpp", src)

	a := findNode(nodes, types.KindType, "A")
	if a == nil {
		t.Fatal("expected namespace node for A")
	}

	b := findNode(nodes, types.KindType, "B")
	if b == nil {
		t.Fatal("expected namespace node for B")
	}

	deepFunc := findNode(nodes, types.KindFunction, "deep_func")
	if deepFunc == nil {
		t.Fatal("expected function node for deep_func")
	}

	// B should be contained in A.
	hasABContain := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Contains && e.SourceHash == a.NodeHash && e.TargetHash == b.NodeHash {
			hasABContain = true
			break
		}
	}
	if !hasABContain {
		t.Error("expected contains edge from A to B")
	}
}

func TestCompileDBIntegration(t *testing.T) {
	// Test with a compile database entry.
	db := &CompileDB{
		index: make(map[string]*CompileEntry),
	}
	db.index["/test/src/main.cpp"] = &CompileEntry{
		IncludePaths: []string{"/test/include"},
		Defines:      map[string]string{"DEBUG": "1"},
		StdVersion:   "c++17",
		File:         "/test/src/main.cpp",
	}

	extractor := NewCppExtractorWithCompileDB(db)
	if extractor.compileDB == nil {
		t.Fatal("expected non-nil compileDB")
	}

	src := `#include "config.hpp"
int main() { return 0; }`
	opts := testOpts("src/main.cpp", src)
	result, err := extractor.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Should have include edge.
	hasInclude := false
	for _, e := range result.Edges {
		if e.EdgeType == edgetype.Includes {
			hasInclude = true
			break
		}
	}
	if !hasInclude {
		t.Error("expected includes edge with compile database")
	}
}

func TestDocExtraction(t *testing.T) {
	src := `/// Brief description of foo.
/// More details here.
int foo() { return 0; }`
	nodes, _ := extract(t, "src/main.cpp", src)

	foo := findNode(nodes, types.KindFunction, "foo")
	if foo == nil {
		t.Fatal("expected function node for foo")
	}
	if foo.Doc == "" {
		t.Error("expected non-empty doc from preceding comments")
	}
}
