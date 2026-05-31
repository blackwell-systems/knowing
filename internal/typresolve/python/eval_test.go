package pyresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/stretchr/testify/require"
)

// parsePython parses Python source and returns the root node.
func parsePython(t *testing.T, src string) *sitter.Node {
	t.Helper()
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
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

// newPyTestContext creates a ResolveContext for testing.
func newPyTestContext(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]ImportInfo),
		ModuleQN: "mymod",
		Content:  content,
	}
}

// --- EvalExprType tests ---

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `x = something`
	content := []byte(src)
	root := parsePython(t, src)

	// Find the identifier "x" on the left side. We want to test a usage,
	// so let's set up a scenario where x is looked up.
	xNode := findNodeByTypeAndText(root, "identifier", "x", content)
	require.NotNil(t, xNode, "could not find identifier 'x'")

	ctx := newPyTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("mymod.MyType"))

	result := EvalExprType(ctx, xNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "mymod.MyType", result.Name)
}

func TestEvalExprType_IdentifierTrue(t *testing.T) {
	src := `x = True`
	content := []byte(src)
	root := parsePython(t, src)

	trueNode := findNodeByTypeAndText(root, "true", "True", content)
	if trueNode == nil {
		// tree-sitter-python may parse True as identifier.
		trueNode = findNodeByTypeAndText(root, "identifier", "True", content)
	}
	require.NotNil(t, trueNode, "could not find True node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, trueNode)
	// True may be parsed as a literal "true" node (handled by LiteralType)
	// or as an identifier. Either way should resolve to bool.
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "bool", result.Name)
}

func TestEvalExprType_IdentifierFalse(t *testing.T) {
	src := `x = False`
	content := []byte(src)
	root := parsePython(t, src)

	falseNode := findNodeByTypeAndText(root, "false", "False", content)
	if falseNode == nil {
		falseNode = findNodeByTypeAndText(root, "identifier", "False", content)
	}
	require.NotNil(t, falseNode, "could not find False node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, falseNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "bool", result.Name)
}

func TestEvalExprType_IdentifierNone(t *testing.T) {
	src := `x = None`
	content := []byte(src)
	root := parsePython(t, src)

	noneNode := findNodeByTypeAndText(root, "none", "None", content)
	if noneNode == nil {
		noneNode = findNodeByTypeAndText(root, "identifier", "None", content)
	}
	require.NotNil(t, noneNode, "could not find None node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, noneNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "None", result.Name)
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `x = "hello"`
	content := []byte(src)
	root := parsePython(t, src)

	strNode := findNode(root, "string")
	require.NotNil(t, strNode, "could not find string node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, strNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "str", result.Name)
}

func TestEvalExprType_IntegerLiteral(t *testing.T) {
	src := `x = 42`
	content := []byte(src)
	root := parsePython(t, src)

	intNode := findNode(root, "integer")
	require.NotNil(t, intNode, "could not find integer node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, intNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestEvalExprType_BooleanOperator(t *testing.T) {
	src := `x = a and b`
	content := []byte(src)
	root := parsePython(t, src)

	boolOpNode := findNode(root, "boolean_operator")
	require.NotNil(t, boolOpNode, "could not find boolean_operator node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, boolOpNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "bool", result.Name)
}

func TestEvalExprType_ListLiteral(t *testing.T) {
	src := `x = [1, 2, 3]`
	content := []byte(src)
	root := parsePython(t, src)

	listNode := findNode(root, "list")
	require.NotNil(t, listNode, "could not find list node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, listNode)
	require.Equal(t, typresolve.KindSlice, result.Kind)
	require.NotNil(t, result.Elem)
	require.Equal(t, typresolve.KindBuiltin, result.Elem.Kind)
	require.Equal(t, "int", result.Elem.Name)
}

func TestEvalExprType_DictLiteral(t *testing.T) {
	src := `x = {"a": 1}`
	content := []byte(src)
	root := parsePython(t, src)

	dictNode := findNode(root, "dictionary")
	require.NotNil(t, dictNode, "could not find dictionary node")

	ctx := newPyTestContext(content)
	result := EvalExprType(ctx, dictNode)
	require.Equal(t, typresolve.KindMap, result.Kind)
	require.NotNil(t, result.Key)
	require.Equal(t, typresolve.KindBuiltin, result.Key.Kind)
	require.Equal(t, "str", result.Key.Name)
	require.NotNil(t, result.Value)
	require.Equal(t, typresolve.KindBuiltin, result.Value.Kind)
	require.Equal(t, "int", result.Value.Name)
}

func TestEvalExprType_AttributeImport(t *testing.T) {
	src := `x = os.path`
	content := []byte(src)
	root := parsePython(t, src)

	attrNode := findNode(root, "attribute")
	require.NotNil(t, attrNode, "could not find attribute node")

	ctx := newPyTestContext(content)
	ctx.Imports["os"] = ImportInfo{ModulePath: "os", IsFromStyle: false}

	// Register os.path as a type.
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "os.path",
		ShortName:     "path",
	})

	result := EvalExprType(ctx, attrNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "os.path", result.Name)
}

func TestEvalExprType_CallReturnsType(t *testing.T) {
	src := `x = foo()`
	content := []byte(src)
	root := parsePython(t, src)

	callNode := findNode(root, "call")
	require.NotNil(t, callNode, "could not find call node")

	ctx := newPyTestContext(content)
	sig := typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "mymod.foo",
		ShortName:     "foo",
		Signature:     sig,
	})

	result := EvalExprType(ctx, callNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "int", result.Name)
}

func TestEvalExprType_ConstructorCall(t *testing.T) {
	src := `x = MyClass()`
	content := []byte(src)
	root := parsePython(t, src)

	callNode := findNode(root, "call")
	require.NotNil(t, callNode, "could not find call node")

	ctx := newPyTestContext(content)
	// Bind MyClass as a Named type in scope (simulating from-import).
	ctx.Scope.Bind("MyClass", typresolve.Named("mymod.MyClass"))

	result := EvalExprType(ctx, callNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "mymod.MyClass", result.Name)
}

func TestEvalExprType_SubscriptOnList(t *testing.T) {
	src := `x = items[0]`
	content := []byte(src)
	root := parsePython(t, src)

	subNode := findNode(root, "subscript")
	require.NotNil(t, subNode, "could not find subscript node")

	ctx := newPyTestContext(content)
	ctx.Scope.Bind("items", typresolve.Slice(typresolve.Builtin("str")))

	result := EvalExprType(ctx, subNode)
	require.Equal(t, typresolve.KindBuiltin, result.Kind)
	require.Equal(t, "str", result.Name)
}

// --- ProcessStatement tests ---

func TestProcessStatement_SimpleAssignment(t *testing.T) {
	src := `x = 42`
	content := []byte(src)
	root := parsePython(t, src)

	assignNode := findNode(root, "assignment")
	require.NotNil(t, assignNode, "could not find assignment node")

	ctx := newPyTestContext(content)
	ProcessStatement(ctx, assignNode)

	bound := ctx.Scope.Lookup("x")
	require.NotNil(t, bound, "x not bound in scope")
	require.Equal(t, typresolve.KindBuiltin, bound.Kind)
	require.Equal(t, "int", bound.Name)
}

func TestProcessStatement_AnnotatedAssignment(t *testing.T) {
	src := `x: int = 42`
	content := []byte(src)
	root := parsePython(t, src)

	// tree-sitter-python may parse annotated assignment differently.
	// It could be "assignment" with a "type" field.
	assignNode := findNode(root, "assignment")
	if assignNode == nil {
		// Try "type" or other node types.
		t.Skip("annotated assignment node not found; may need different tree-sitter parse")
	}

	ctx := newPyTestContext(content)
	ProcessStatement(ctx, assignNode)

	bound := ctx.Scope.Lookup("x")
	require.NotNil(t, bound, "x not bound in scope")
	require.Equal(t, typresolve.KindBuiltin, bound.Kind)
	require.Equal(t, "int", bound.Name)
}

func TestProcessStatement_TupleUnpacking(t *testing.T) {
	src := `a, b = (1, "s")`
	content := []byte(src)
	root := parsePython(t, src)

	assignNode := findNode(root, "assignment")
	require.NotNil(t, assignNode, "could not find assignment node")

	ctx := newPyTestContext(content)
	ProcessStatement(ctx, assignNode)

	a := ctx.Scope.Lookup("a")
	require.NotNil(t, a, "a not bound in scope")
	require.Equal(t, typresolve.KindBuiltin, a.Kind)
	require.Equal(t, "int", a.Name)

	b := ctx.Scope.Lookup("b")
	require.NotNil(t, b, "b not bound in scope")
	require.Equal(t, typresolve.KindBuiltin, b.Kind)
	require.Equal(t, "str", b.Name)
}

func TestProcessStatement_ForLoop(t *testing.T) {
	src := `for x in [1, 2]:
    pass`
	content := []byte(src)
	root := parsePython(t, src)

	forNode := findNode(root, "for_statement")
	require.NotNil(t, forNode, "could not find for_statement node")

	ctx := newPyTestContext(content)
	ProcessStatement(ctx, forNode)

	x := ctx.Scope.Lookup("x")
	require.NotNil(t, x, "x not bound in scope")
	require.Equal(t, typresolve.KindBuiltin, x.Kind)
	require.Equal(t, "int", x.Name)
}
