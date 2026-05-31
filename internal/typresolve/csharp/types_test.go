package csresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// parseCS parses C# source code and returns the root AST node.
func parseCS(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(csharp.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

// findFirstNodeOfType is defined in eval_test.go (same package).

// findTypeNodeInVarDecl finds the type node from a variable declaration.
func findTypeNodeInVarDecl(t *testing.T, root *sitter.Node) *sitter.Node {
	t.Helper()
	// Look for variable_declaration -> type child.
	varDecl := findFirstNodeOfType(root, "variable_declaration")
	if varDecl == nil {
		t.Fatal("no variable_declaration found")
	}
	// First named child is typically the type.
	if varDecl.NamedChildCount() > 0 {
		return varDecl.NamedChild(0)
	}
	t.Fatal("no type node in variable_declaration")
	return nil
}

func TestParseTypeNode_PredefinedTypes(t *testing.T) {
	tests := []struct {
		src      string
		wantName string
	}{
		{"class C { int x; }", "System.Int32"},
		{"class C { string x; }", "System.String"},
		{"class C { bool x; }", "System.Boolean"},
		{"class C { double x; }", "System.Double"},
		{"class C { float x; }", "System.Single"},
		{"class C { decimal x; }", "System.Decimal"},
		{"class C { object x; }", "System.Object"},
		{"class C { char x; }", "System.Char"},
		{"class C { long x; }", "System.Int64"},
		{"class C { byte x; }", "System.Byte"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			root := parseCS(t, tt.src)
			content := []byte(tt.src)

			// Find the predefined_type or type node in the field declaration.
			typeNode := findFirstNodeOfType(root, "predefined_type")
			if typeNode == nil {
				t.Fatal("no predefined_type node found")
			}

			got := ParseTypeNode(typeNode, content, "", nil, nil)
			if got == nil {
				t.Fatal("ParseTypeNode returned nil")
			}
			if got.Kind != typresolve.KindNamed {
				t.Fatalf("want KindNamed, got %v", got.Kind)
			}
			if got.Name != tt.wantName {
				t.Errorf("want %q, got %q", tt.wantName, got.Name)
			}
		})
	}
}

func TestParseTypeNode_NullableType(t *testing.T) {
	src := "class C { int? x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "nullable_type")
	if typeNode == nil {
		t.Fatal("no nullable_type node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	// Nullable unwraps to inner type.
	if got.Kind != typresolve.KindNamed {
		t.Fatalf("want KindNamed, got %v", got.Kind)
	}
	if got.Name != "System.Int32" {
		t.Errorf("want System.Int32, got %q", got.Name)
	}
}

func TestParseTypeNode_ArrayType(t *testing.T) {
	src := "class C { int[] x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "array_type")
	if typeNode == nil {
		t.Fatal("no array_type node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	if got.Kind != typresolve.KindSlice {
		t.Fatalf("want KindSlice, got %v", got.Kind)
	}
	if got.Elem == nil || got.Elem.Name != "System.Int32" {
		t.Errorf("want elem System.Int32, got %v", got.Elem)
	}
}

func TestParseTypeNode_TupleType(t *testing.T) {
	src := "class C { (int, string) x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "tuple_type")
	if typeNode == nil {
		t.Fatal("no tuple_type node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	if got.Kind != typresolve.KindTuple {
		t.Fatalf("want KindTuple, got %v", got.Kind)
	}
	if len(got.Elements) != 2 {
		t.Fatalf("want 2 elements, got %d", len(got.Elements))
	}
	if got.Elements[0].Name != "System.Int32" {
		t.Errorf("elem[0] want System.Int32, got %q", got.Elements[0].Name)
	}
	if got.Elements[1].Name != "System.String" {
		t.Errorf("elem[1] want System.String, got %q", got.Elements[1].Name)
	}
}

func TestParseTypeNode_GenericName(t *testing.T) {
	src := "class C { List<int> x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "generic_name")
	if typeNode == nil {
		t.Fatal("no generic_name node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	// List<int> should map to Slice(System.Int32).
	if got.Kind != typresolve.KindSlice {
		t.Fatalf("want KindSlice for List<int>, got %v", got.Kind)
	}
	if got.Elem == nil || got.Elem.Name != "System.Int32" {
		t.Errorf("want elem System.Int32, got %v", got.Elem)
	}
}

func TestParseTypeNode_GenericDictionary(t *testing.T) {
	src := "class C { Dictionary<string, int> x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "generic_name")
	if typeNode == nil {
		t.Fatal("no generic_name node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	// Dictionary<string, int> -> Map(String, Int32).
	if got.Kind != typresolve.KindMap {
		t.Fatalf("want KindMap for Dictionary<string, int>, got %v", got.Kind)
	}
	if got.Key == nil || got.Key.Name != "System.String" {
		t.Errorf("want key System.String, got %v", got.Key)
	}
	if got.Value == nil || got.Value.Name != "System.Int32" {
		t.Errorf("want value System.Int32, got %v", got.Value)
	}
}

func TestParseTypeNode_QualifiedName(t *testing.T) {
	src := "class C { System.IO.Stream x; }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "qualified_name")
	if typeNode == nil {
		// Try identifier approach: the grammar may parse this differently.
		t.Skip("no qualified_name node found; grammar may use member_access_expression for type references")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	if got.Kind != typresolve.KindNamed {
		t.Fatalf("want KindNamed, got %v", got.Kind)
	}
	if got.Name != "System.IO.Stream" {
		t.Errorf("want System.IO.Stream, got %q", got.Name)
	}
}

func TestParseTypeNode_ImplicitType(t *testing.T) {
	src := "class C { void M() { var x = 1; } }"
	root := parseCS(t, src)
	content := []byte(src)

	typeNode := findFirstNodeOfType(root, "implicit_type")
	if typeNode == nil {
		t.Skip("no implicit_type node found")
	}

	got := ParseTypeNode(typeNode, content, "", nil, nil)
	if got == nil {
		t.Fatal("ParseTypeNode returned nil")
	}
	if got.Kind != typresolve.KindUnknown {
		t.Errorf("want KindUnknown for var, got %v", got.Kind)
	}
}

func TestBuildUsingMap(t *testing.T) {
	src := `using System;
using System.Collections.Generic;
using static System.Console;
using MyAlias = System.Text.StringBuilder;
`
	root := parseCS(t, src)
	content := []byte(src)

	usings := BuildUsingMap(root, content)

	// Should have implicit System + the 4 explicit directives.
	// But "using System" is explicit too, so implicit + explicit System = duplicate.
	// At minimum we expect 5 entries (1 implicit + 4 explicit).
	if len(usings) < 4 {
		t.Fatalf("want at least 4 usings, got %d", len(usings))
	}

	// Check that implicit System is first.
	if usings[0].Kind != UsingNamespace || usings[0].TargetQN != "System" {
		t.Errorf("first using should be implicit System, got %+v", usings[0])
	}

	// Find the static using.
	foundStatic := false
	for _, u := range usings {
		if u.Kind == UsingStatic && u.TargetQN == "System.Console" {
			foundStatic = true
		}
	}
	if !foundStatic {
		t.Error("missing using static System.Console")
	}

	// Find the alias using.
	foundAlias := false
	for _, u := range usings {
		if u.Kind == UsingAlias && u.LocalName == "MyAlias" && u.TargetQN == "System.Text.StringBuilder" {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Error("missing using MyAlias = System.Text.StringBuilder")
	}
}

func TestResolveTypeName_PredefinedAlias(t *testing.T) {
	got := ResolveTypeName("int", "", nil, nil, "", "")
	if got != "System.Int32" {
		t.Errorf("want System.Int32, got %q", got)
	}
}

func TestResolveTypeName_ExactRegistryHit(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyService",
		ShortName:     "MyService",
	})

	got := ResolveTypeName("MyApp.MyService", "", nil, reg, "", "")
	if got != "MyApp.MyService" {
		t.Errorf("want MyApp.MyService, got %q", got)
	}
}

func TestResolveTypeName_NamespaceResolution(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.Models.User",
		ShortName:     "User",
	})

	got := ResolveTypeName("User", "MyApp.Models", nil, reg, "", "")
	if got != "MyApp.Models.User" {
		t.Errorf("want MyApp.Models.User, got %q", got)
	}
}

func TestResolveTypeName_UsingNamespace(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.IO.File",
		ShortName:     "File",
	})

	usings := []UsingInfo{
		{Kind: UsingNamespace, TargetQN: "System.IO"},
	}

	got := ResolveTypeName("File", "", usings, reg, "", "")
	if got != "System.IO.File" {
		t.Errorf("want System.IO.File, got %q", got)
	}
}

func TestResolveTypeName_UsingAlias(t *testing.T) {
	usings := []UsingInfo{
		{Kind: UsingAlias, LocalName: "SB", TargetQN: "System.Text.StringBuilder"},
	}

	got := ResolveTypeName("SB", "", usings, nil, "", "")
	if got != "System.Text.StringBuilder" {
		t.Errorf("want System.Text.StringBuilder, got %q", got)
	}
}

func TestResolveTypeName_GlobalPrefix(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.String",
		ShortName:     "String",
	})

	got := ResolveTypeName("global::System.String", "", nil, reg, "", "")
	if got != "System.String" {
		t.Errorf("want System.String, got %q", got)
	}
}

func TestResolveTypeName_NestedType(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.Outer.Inner",
		ShortName:     "Inner",
	})

	got := ResolveTypeName("Inner", "", nil, reg, "MyApp.Outer", "")
	if got != "MyApp.Outer.Inner" {
		t.Errorf("want MyApp.Outer.Inner, got %q", got)
	}
}

func TestStripGenericArgs(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"List<int>", "List"},
		{"Dictionary<string, int>", "Dictionary"},
		{"MyType", "MyType"},
		{"Task<List<int>>", "Task"},
	}
	for _, tt := range tests {
		got := stripGenericArgs(tt.input)
		if got != tt.want {
			t.Errorf("stripGenericArgs(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeCSName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"global::System.String", "global.System.String"},
		{"A::B::C", "A.B.C"},
		{"Normal.Name", "Normal.Name"},
	}
	for _, tt := range tests {
		got := normalizeCSName(tt.input)
		if got != tt.want {
			t.Errorf("normalizeCSName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
