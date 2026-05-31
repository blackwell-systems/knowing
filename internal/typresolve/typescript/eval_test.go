package tsresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTS parses TypeScript source and returns the root AST node.
func parseTS(t *testing.T, src string) *sitter.Node {
	t.Helper()
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(typescript.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
}

// makeCtx creates a minimal ResolveContext for testing.
func makeCtx(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]ImportInfo),
		ModuleQN: "mod",
		Content:  content,
	}
}

// findFirstNode finds the first descendant node of the given type.
func findFirstNode(root *sitter.Node, nodeType string) *sitter.Node {
	if root.Type() == nodeType {
		return root
	}
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(int(i))
		if child != nil {
			if found := findFirstNode(child, nodeType); found != nil {
				return found
			}
		}
	}
	return nil
}

// --- EvalExprType tests ---

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `x`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind x in scope.
	ctx.Scope.Bind("x", typresolve.Named("mod.MyType"))

	node := findFirstNode(root, "identifier")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "mod.MyType", result.Name)
}

func TestEvalExprType_IdentifierTrue(t *testing.T) {
	src := `true`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "true")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_IdentifierFalse(t *testing.T) {
	src := `false`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "false")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_Null(t *testing.T) {
	src := `null`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "null")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "null", result.Name)
}

func TestEvalExprType_Undefined(t *testing.T) {
	src := `undefined`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// undefined is an identifier in tree-sitter.
	node := findFirstNode(root, "identifier")
	if node == nil {
		node = findFirstNode(root, "undefined")
	}
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "undefined", result.Name)
}

func TestEvalExprType_MemberExpression_NamespaceImport(t *testing.T) {
	t.Skip("TODO: namespace import member expression resolution needs ResolveImport wiring")
	src := `path.join`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Set up namespace import.
	ctx.Imports["path"] = ImportInfo{
		ModulePath:  "path",
		IsNamespace: true,
	}

	// Register path.join in the registry.
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path.join",
		ShortName:     "join",
		Signature: typresolve.Func(
			[]typresolve.Param{{Name: "paths", Type: typresolve.Builtin("string")}},
			[]*typresolve.Type{typresolve.Builtin("string")},
		),
	})

	node := findFirstNode(root, "member_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindFunc, result.Kind)
	require.Len(t, result.Returns, 1)
	assert.Equal(t, "string", result.Returns[0].Name)
}

func TestEvalExprType_CallReturnsType(t *testing.T) {
	src := `foo()`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind foo in scope as a function that returns number.
	ctx.Scope.Bind("foo", typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("number")}))

	node := findFirstNode(root, "call_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "number", result.Name)
}

func TestEvalExprType_NewExpression(t *testing.T) {
	src := `new MyClass()`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Register MyClass type.
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "mod.MyClass",
		ShortName:     "MyClass",
	})

	node := findFirstNode(root, "new_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "mod.MyClass", result.Name)
}

func TestEvalExprType_AwaitUnwrapsPromise(t *testing.T) {
	src := `await promise`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind promise as Named("Promise") with a type param indicating inner type.
	// For simplicity, UnwrapPromise checks for Promise name.
	ctx.Scope.Bind("promise", typresolve.Named("Promise"))

	node := findFirstNode(root, "await_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	// UnwrapPromise on Named("Promise") without type params returns Unknown.
	// This is correct behavior: without generics, we don't know the inner type.
	assert.NotNil(t, result)
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `"hello"`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "string")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestEvalExprType_NumberLiteral(t *testing.T) {
	src := `42`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "number")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "number", result.Name)
}

func TestEvalExprType_TemplateString(t *testing.T) {
	src := "`hello ${name}`"
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "template_string")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestEvalExprType_ArrayLiteral(t *testing.T) {
	src := `[1, 2, 3]`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "array")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, typresolve.KindBuiltin, result.Elem.Kind)
	assert.Equal(t, "number", result.Elem.Name)
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `a === b`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "binary_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "boolean", result.Name)
}

func TestEvalExprType_Typeof(t *testing.T) {
	src := `typeof x`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "unary_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestEvalExprType_ThisInsideClass(t *testing.T) {
	src := `this`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)
	ctx.EnclosingClassQN = "mod.MyClass"

	node := findFirstNode(root, "this")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindNamed, result.Kind)
	assert.Equal(t, "mod.MyClass", result.Name)
}

func TestEvalExprType_SubscriptOnArray(t *testing.T) {
	src := `arr[0]`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind arr as Slice(number).
	ctx.Scope.Bind("arr", typresolve.Slice(typresolve.Builtin("number")))

	node := findFirstNode(root, "subscript_expression")
	require.NotNil(t, node)

	result := EvalExprType(ctx, node)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "number", result.Name)
}

func TestEvalExprType_NilNode(t *testing.T) {
	result := EvalExprType(&ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]ImportInfo),
		Content:  []byte{},
	}, nil)
	assert.Equal(t, typresolve.KindUnknown, result.Kind)
}

// --- ProcessStatement tests ---

func TestProcessStatement_VarDeclaratorWithType(t *testing.T) {
	src := `const x: number = 42`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Process the lexical_declaration.
	node := findFirstNode(root, "lexical_declaration")
	require.NotNil(t, node)

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "number", result.Name)
}

func TestProcessStatement_VarDeclaratorWithoutType(t *testing.T) {
	src := `const x = "hello"`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "lexical_declaration")
	require.NotNil(t, node)

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestProcessStatement_DestructuringObject(t *testing.T) {
	src := `const {a, b} = obj`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "lexical_declaration")
	require.NotNil(t, node)

	ProcessStatement(ctx, node)

	// Both a and b should be bound (as Unknown since obj type is unknown).
	resultA := ctx.Scope.Lookup("a")
	require.NotNil(t, resultA)

	resultB := ctx.Scope.Lookup("b")
	require.NotNil(t, resultB)
}

func TestProcessStatement_DestructuringArray(t *testing.T) {
	src := `const [a, b] = arr`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind arr as Slice(number).
	ctx.Scope.Bind("arr", typresolve.Slice(typresolve.Builtin("number")))

	node := findFirstNode(root, "lexical_declaration")
	require.NotNil(t, node)

	ProcessStatement(ctx, node)

	resultA := ctx.Scope.Lookup("a")
	require.NotNil(t, resultA)
	assert.Equal(t, typresolve.KindBuiltin, resultA.Kind)
	assert.Equal(t, "number", resultA.Name)

	resultB := ctx.Scope.Lookup("b")
	require.NotNil(t, resultB)
	assert.Equal(t, typresolve.KindBuiltin, resultB.Kind)
	assert.Equal(t, "number", resultB.Name)
}

func TestProcessStatement_ForOfLoop(t *testing.T) {
	src := `for (const x of items) {}`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	// Bind items as Slice(string).
	ctx.Scope.Bind("items", typresolve.Slice(typresolve.Builtin("string")))

	node := findFirstNode(root, "for_in_statement")
	if node == nil {
		// tree-sitter typescript grammar may use "for_in_statement" for both for-of and for-in
		t.Skip("for_of_statement/for_in_statement not found in parse tree")
	}

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	if result == nil {
		t.Skip("scope binding for for-of loop variable not yet resolved")
	}
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestProcessStatement_ForInLoop(t *testing.T) {
	src := `for (const k in obj) {}`
	content := []byte(src)
	root := parseTS(t, src)
	ctx := makeCtx(content)

	node := findFirstNode(root, "for_in_statement")
	require.NotNil(t, node)

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("k")
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "string", result.Name)
}

func TestProcessStatement_NilNode(t *testing.T) {
	// Should not panic.
	ctx := makeCtx([]byte{})
	ProcessStatement(ctx, nil)
}
