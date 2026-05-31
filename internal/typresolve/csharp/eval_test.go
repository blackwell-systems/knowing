package csresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/csharp"
)

// parseCSharp parses C# source code and returns the root AST node.
// Named parseCSharp (not parseCS) to avoid duplicate function names with
// Agent A's types_test.go. Agent D consolidates in wave 2.
func parseCSharp(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(csharp.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

// newTestCtx creates a ResolveContext for testing with basic setup.
func newTestCtx(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry:         typresolve.NewRegistry(),
		Scope:            typresolve.NewScope(nil),
		Content:          content,
		EnclosingClassQN: "",
		EnclosingBaseQN:  "",
		ModuleQN:         "",
	}
}

func TestEvalExprType_Literals(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantName string
	}{
		{"string_literal", `class C { void M() { var x = "hello"; } }`, "System.String"},
		{"int_literal", `class C { void M() { var x = 42; } }`, "System.Int32"},
		{"real_literal", `class C { void M() { var x = 3.14; } }`, "System.Double"},
		{"bool_literal", `class C { void M() { var x = true; } }`, "System.Boolean"},
		{"char_literal", `class C { void M() { var x = 'a'; } }`, "System.Char"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.src)
			root := parseCSharp(t, tt.src)
			ctx := newTestCtx(content)

			// Find the literal node by walking into the variable_declarator's
			// equals_value_clause.
			literal := findFirstNodeOfType(root, literalTypeForTest(tt.name))
			if literal == nil {
				// Try finding any literal
				literal = findFirstLiteral(root)
			}
			if literal == nil {
				t.Fatalf("could not find literal node in AST")
			}

			result := EvalExprType(ctx, literal)
			if result == nil {
				t.Fatal("EvalExprType returned nil")
			}
			if result.Kind != typresolve.KindNamed {
				t.Fatalf("expected KindNamed, got %v", result.Kind)
			}
			if result.Name != tt.wantName {
				t.Errorf("expected %s, got %s", tt.wantName, result.Name)
			}
		})
	}
}

func TestEvalExprType_ThisMemberAccess(t *testing.T) {
	// In tree-sitter C# grammar, bare `this` is an unnamed token, not a named node.
	// Test this through member access (this.Name), which is the realistic usage.
	src := `class MyService { void M() { var x = this.Name; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyService",
		ShortName:     "MyService",
		Fields:        []typresolve.Field{{Name: "Name", Type: typresolve.Named("System.String")}},
	})

	ctx := &ResolveContext{
		Registry:         reg,
		Scope:            typresolve.NewScope(nil),
		Content:          content,
		EnclosingClassQN: "MyApp.MyService",
	}

	maNode := findFirstNodeOfType(root, "member_access_expression")
	if maNode == nil {
		t.Fatal("could not find member_access_expression node")
	}

	result := EvalExprType(ctx, maNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "System.String" {
		t.Errorf("expected System.String, got %s", result.Name)
	}
}

func TestEvalExprType_ScopeVariable(t *testing.T) {
	src := `class C { void M() { var x = 0; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)
	ctx.Scope.Bind("x", typresolve.Named("System.Int32"))

	// Find an identifier "x" in the AST.
	identNode := findIdentifier(root, content, "x")
	if identNode == nil {
		t.Fatal("could not find identifier 'x'")
	}

	result := EvalExprType(ctx, identNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "System.Int32" {
		t.Errorf("expected System.Int32, got %s", result.Name)
	}
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `class C { void M() { var x = a == b; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	binNode := findFirstNodeOfType(root, "binary_expression")
	if binNode == nil {
		t.Fatal("could not find binary_expression node")
	}

	result := EvalExprType(ctx, binNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "System.Boolean" {
		t.Errorf("expected System.Boolean, got %s", result.Name)
	}
}

func TestEvalExprType_CastExpression(t *testing.T) {
	src := `class C { void M() { var x = (int)y; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	castNode := findFirstNodeOfType(root, "cast_expression")
	if castNode == nil {
		t.Fatal("could not find cast_expression node")
	}

	result := EvalExprType(ctx, castNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "System.Int32" {
		t.Errorf("expected System.Int32, got %s", result.Name)
	}
}

func TestEvalExprType_TupleExpression(t *testing.T) {
	src := `class C { void M() { var x = (1, "hello"); } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	tupleNode := findFirstNodeOfType(root, "tuple_expression")
	if tupleNode == nil {
		t.Fatal("could not find tuple_expression node")
	}

	result := EvalExprType(ctx, tupleNode)
	if result == nil {
		t.Fatal("EvalExprType returned nil")
	}
	if result.Kind != typresolve.KindTuple {
		t.Fatalf("expected KindTuple, got %v", result.Kind)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
	if result.Elements[0].Name != "System.Int32" {
		t.Errorf("element 0: expected System.Int32, got %s", result.Elements[0].Name)
	}
	if result.Elements[1].Name != "System.String" {
		t.Errorf("element 1: expected System.String, got %s", result.Elements[1].Name)
	}
}

func TestEvalExprType_DepthLimit(t *testing.T) {
	// Create a context already near max depth.
	src := `class C { void M() { var x = 1; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)
	ctx.EvalDepth = csEvalMaxDepth // Already at max.

	literal := findFirstLiteral(root)
	if literal == nil {
		t.Fatal("could not find literal")
	}

	result := EvalExprType(ctx, literal)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Kind != typresolve.KindUnknown {
		t.Errorf("expected Unknown at depth limit, got %v", result.Kind)
	}
}

func TestEvalExprType_ObjectCreation(t *testing.T) {
	src := `class C { void M() { var x = new MyService(); } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	objNode := findFirstNodeOfType(root, "object_creation_expression")
	if objNode == nil {
		t.Fatal("could not find object_creation_expression node")
	}

	result := EvalExprType(ctx, objNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "MyService" {
		t.Errorf("expected MyService, got %s", result.Name)
	}
}

func TestEvalExprType_IsExpression(t *testing.T) {
	src := `class C { void M() { var x = obj is string; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	isNode := findFirstNodeOfType(root, "is_expression")
	if isNode == nil {
		t.Fatal("could not find is_expression node")
	}

	result := EvalExprType(ctx, isNode)
	if result == nil || result.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type, got %v", result)
	}
	if result.Name != "System.Boolean" {
		t.Errorf("expected System.Boolean, got %s", result.Name)
	}
}

func TestProcessStatement_VarDeclaration(t *testing.T) {
	src := `class C { void M() { var x = 42; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	// Find the local_declaration_statement.
	declNode := findFirstNodeOfType(root, "local_declaration_statement")
	if declNode == nil {
		t.Fatal("could not find local_declaration_statement node")
	}

	ProcessStatement(ctx, declNode)

	// Check that x is bound in scope.
	xType := ctx.Scope.Lookup("x")
	if xType == nil {
		t.Fatal("expected 'x' to be bound in scope")
	}
	if xType.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type for x, got %v", xType.Kind)
	}
	if xType.Name != "System.Int32" {
		t.Errorf("expected System.Int32, got %s", xType.Name)
	}
}

func TestProcessStatement_ExplicitType(t *testing.T) {
	src := `class C { void M() { string name = "hello"; } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	declNode := findFirstNodeOfType(root, "local_declaration_statement")
	if declNode == nil {
		t.Fatal("could not find local_declaration_statement node")
	}

	ProcessStatement(ctx, declNode)

	nameType := ctx.Scope.Lookup("name")
	if nameType == nil {
		t.Fatal("expected 'name' to be bound in scope")
	}
	if nameType.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type for name, got %v", nameType.Kind)
	}
	if nameType.Name != "System.String" {
		t.Errorf("expected System.String, got %s", nameType.Name)
	}
}

func TestProcessStatement_ForEach(t *testing.T) {
	src := `class C { void M() { foreach (string item in items) { } } }`
	content := []byte(src)
	root := parseCSharp(t, src)

	ctx := newTestCtx(content)

	foreachNode := findFirstNodeOfType(root, "foreach_statement")
	if foreachNode == nil {
		t.Fatal("could not find foreach_statement node")
	}

	ProcessStatement(ctx, foreachNode)

	itemType := ctx.Scope.Lookup("item")
	if itemType == nil {
		t.Fatal("expected 'item' to be bound in scope")
	}
	if itemType.Kind != typresolve.KindNamed {
		t.Fatalf("expected Named type for item, got %v", itemType.Kind)
	}
	if itemType.Name != "System.String" {
		t.Errorf("expected System.String, got %s", itemType.Name)
	}
}

// --- AST helper functions for tests ---

func findFirstNodeOfType(root *sitter.Node, nodeType string) *sitter.Node {
	if root == nil {
		return nil
	}
	if root.Type() == nodeType {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(int(i))
		if found := findFirstNodeOfType(child, nodeType); found != nil {
			return found
		}
	}
	return nil
}

func findFirstLiteral(root *sitter.Node) *sitter.Node {
	types := []string{
		"string_literal", "integer_literal", "real_literal",
		"boolean_literal", "character_literal",
		"interpolated_string_expression", "verbatim_string_literal",
	}
	for _, lt := range types {
		if n := findFirstNodeOfType(root, lt); n != nil {
			return n
		}
	}
	return nil
}

func findIdentifier(root *sitter.Node, content []byte, name string) *sitter.Node {
	if root == nil {
		return nil
	}
	if root.Type() == "identifier" && root.Content(content) == name {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(int(i))
		if found := findIdentifier(child, content, name); found != nil {
			return found
		}
	}
	return nil
}

func literalTypeForTest(name string) string {
	switch name {
	case "string_literal":
		return "string_literal"
	case "int_literal":
		return "integer_literal"
	case "real_literal":
		return "real_literal"
	case "bool_literal":
		return "boolean_literal"
	case "char_literal":
		return "character_literal"
	default:
		return name
	}
}
