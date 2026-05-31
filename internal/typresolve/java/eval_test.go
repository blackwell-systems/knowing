package javaresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/stretchr/testify/require"
)

// parseJava parses Java source and returns the root node.
func parseJava(t *testing.T, src string) (*sitter.Node, []byte) {
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode(), content
}

// findNode walks the tree and returns the first node matching the given type.
func findNode(root *sitter.Node, nodeType string) *sitter.Node {
	if root.Type() == nodeType {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		if found := findNode(root.Child(int(i)), nodeType); found != nil {
			return found
		}
	}
	return nil
}

// findNodeByTypeAndText walks the tree and returns the first node matching
// the given type whose text content matches.
func findNodeByTypeAndText(root *sitter.Node, nodeType, text string, content []byte) *sitter.Node {
	if root.Type() == nodeType && root.Content(content) == text {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		if found := findNodeByTypeAndText(root.Child(int(i)), nodeType, text, content); found != nil {
			return found
		}
	}
	return nil
}

// newTestContext creates a ResolveContext for testing.
func newTestContext(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]string),
		PkgQN:    "com.example",
		Content:  content,
	}
}

// --- EvalExprType tests ---

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `class Test { void m() { x.toString(); } }`
	root, content := parseJava(t, src)

	xNode := findNodeByTypeAndText(root, "identifier", "x", content)
	require.NotNil(t, xNode, "could not find identifier 'x'")

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("com.example.MyType"))

	result := EvalExprType(ctx, xNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "com.example.MyType", result.Name)
}

func TestEvalExprType_This(t *testing.T) {
	src := `class Test { void m() { this.field = 1; } }`
	root, content := parseJava(t, src)

	thisNode := findNode(root, "this")
	require.NotNil(t, thisNode, "could not find 'this' node")

	ctx := newTestContext(content)
	ctx.EnclosingClassQN = "com.example.Test"

	result := EvalExprType(ctx, thisNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "com.example.Test", result.Name)
}

func TestEvalExprType_ThisNoClass(t *testing.T) {
	src := `class Test { void m() { this.field = 1; } }`
	root, content := parseJava(t, src)

	thisNode := findNode(root, "this")
	require.NotNil(t, thisNode)

	ctx := newTestContext(content)
	// No EnclosingClassQN set.

	result := EvalExprType(ctx, thisNode)
	require.Equal(t, typresolve.KindUnknown, result.Kind)
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `class Test { void m() { String s = "hello"; } }`
	root, content := parseJava(t, src)

	strNode := findNode(root, "string_literal")
	require.NotNil(t, strNode, "could not find string_literal")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, strNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "String", result.Name)
}

func TestEvalExprType_IntegerLiteral(t *testing.T) {
	src := `class Test { void m() { int x = 42; } }`
	root, content := parseJava(t, src)

	intNode := findNode(root, "decimal_integer_literal")
	require.NotNil(t, intNode, "could not find decimal_integer_literal")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, intNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestEvalExprType_BooleanLiteral(t *testing.T) {
	src := `class Test { void m() { boolean b = true; } }`
	root, content := parseJava(t, src)

	boolNode := findNode(root, "true")
	require.NotNil(t, boolNode, "could not find 'true' node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, boolNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `class Test { void m() { boolean b = (x == y); } }`
	root, content := parseJava(t, src)

	binNode := findNode(root, "binary_expression")
	require.NotNil(t, binNode, "could not find binary_expression")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, binNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_ObjectCreation(t *testing.T) {
	src := `class Test { void m() { MyClass obj = new MyClass(); } }`
	root, content := parseJava(t, src)

	newNode := findNode(root, "object_creation_expression")
	require.NotNil(t, newNode, "could not find object_creation_expression")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, newNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "com.example.MyClass", result.Name)
}

func TestEvalExprType_CastExpression(t *testing.T) {
	src := `class Test { void m() { String s = (String) obj; } }`
	root, content := parseJava(t, src)

	castNode := findNode(root, "cast_expression")
	require.NotNil(t, castNode, "could not find cast_expression")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, castNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "String", result.Name)
}

func TestEvalExprType_ArrayAccess(t *testing.T) {
	src := `class Test { void m() { int x = arr[0]; } }`
	root, content := parseJava(t, src)

	accessNode := findNode(root, "array_access")
	require.NotNil(t, accessNode, "could not find array_access")

	ctx := newTestContext(content)
	ctx.Scope.Bind("arr", typresolve.Slice(typresolve.Builtin("int")))

	result := EvalExprType(ctx, accessNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestEvalExprType_InstanceOf(t *testing.T) {
	src := `class Test { void m() { boolean b = (obj instanceof String); } }`
	root, content := parseJava(t, src)

	instNode := findNode(root, "instanceof_expression")
	require.NotNil(t, instNode, "could not find instanceof_expression")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, instNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_Nil(t *testing.T) {
	result := EvalExprType(nil, nil)
	require.Equal(t, typresolve.KindUnknown, result.Kind)
}

// --- ProcessStatement tests ---

func TestProcessStatement_LocalVarDecl(t *testing.T) {
	src := `class Test { void m() { int x = 42; } }`
	root, content := parseJava(t, src)

	declNode := findNode(root, "local_variable_declaration")
	require.NotNil(t, declNode, "could not find local_variable_declaration")

	ctx := newTestContext(content)
	ProcessStatement(ctx, declNode)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result, "x should be bound in scope")
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestProcessStatement_VarInference(t *testing.T) {
	src := `class Test { void m() { var x = "hello"; } }`
	root, content := parseJava(t, src)

	declNode := findNode(root, "local_variable_declaration")
	require.NotNil(t, declNode, "could not find local_variable_declaration")

	ctx := newTestContext(content)
	ProcessStatement(ctx, declNode)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result, "x should be bound in scope")
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "String", result.Name)
}

func TestProcessStatement_EnhancedFor(t *testing.T) {
	src := `class Test { void m() { for (String s : list) { } } }`
	root, content := parseJava(t, src)

	forNode := findNode(root, "enhanced_for_statement")
	require.NotNil(t, forNode, "could not find enhanced_for_statement")

	ctx := newTestContext(content)
	ProcessStatement(ctx, forNode)

	result := ctx.Scope.Lookup("s")
	require.NotNil(t, result, "s should be bound in scope")
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "String", result.Name)
}

func TestProcessStatement_CatchClause(t *testing.T) {
	src := `class Test { void m() { try { } catch (IOException e) { } } }`
	root, content := parseJava(t, src)

	catchNode := findNode(root, "catch_clause")
	require.NotNil(t, catchNode, "could not find catch_clause")

	ctx := newTestContext(content)
	ProcessStatement(ctx, catchNode)

	result := ctx.Scope.Lookup("e")
	require.NotNil(t, result, "e should be bound in scope")
	// IOException is not a builtin, so it resolves as Named via package.
	require.Equal(t, typresolve.KindNamed, result.Kind)
}

func TestProcessStatement_ExpressionAssignment(t *testing.T) {
	src := `class Test { void m() { x = 42; } }`
	root, content := parseJava(t, src)

	exprStmt := findNode(root, "expression_statement")
	require.NotNil(t, exprStmt, "could not find expression_statement")

	ctx := newTestContext(content)
	ProcessStatement(ctx, exprStmt)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result, "x should be bound in scope")
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestProcessStatement_Nil(t *testing.T) {
	// Should not panic.
	ProcessStatement(nil, nil)
}
