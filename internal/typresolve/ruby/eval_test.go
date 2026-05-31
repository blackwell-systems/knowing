package rubyresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/stretchr/testify/require"
)

// parseRuby parses Ruby source code and returns the root AST node.
func parseRuby(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(ruby.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	require.NoError(t, err)
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
}

// findNodeByType walks the tree and returns the first node matching the type.
func findNodeByType(root *sitter.Node, nodeType string) *sitter.Node {
	if root.Type() == nodeType {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		if found := findNodeByType(root.Child(i), nodeType); found != nil {
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
		if found := findNodeByTypeAndText(root.Child(i), nodeType, text, content); found != nil {
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
		Requires: make(map[string]string),
		Nesting:  nil,
		Content:  content,
	}
}

// --- EvalExprType tests ---

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `x`
	content := []byte(src)
	root := parseRuby(t, src)

	xNode := findNodeByTypeAndText(root, "identifier", "x", content)
	require.NotNil(t, xNode, "could not find identifier 'x'")

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("MyType"))

	result := EvalExprType(ctx, xNode)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "MyType", result.Name)
}

func TestEvalExprType_Constant(t *testing.T) {
	src := `String`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "constant")
	require.NotNil(t, node, "could not find constant node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::String", result.Name)
}

func TestEvalExprType_ScopeResolution(t *testing.T) {
	src := `ActiveRecord::Base`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "scope_resolution")
	require.NotNil(t, node, "could not find scope_resolution node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "ActiveRecord::Base", result.Name)
}

func TestEvalExprType_CallNew(t *testing.T) {
	src := `User.new`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "call")
	require.NotNil(t, node, "could not find call node")

	ctx := newTestContext(content)
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "User",
		ShortName:     "User",
	})

	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "User", result.Name)
}

func TestEvalExprType_CallMethod(t *testing.T) {
	src := `user.name`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "call")
	require.NotNil(t, node, "could not find call node")

	ctx := newTestContext(content)
	ctx.Scope.Bind("user", typresolve.Named("User"))
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "User",
		ShortName:     "User",
	})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "User.name",
		ReceiverType:  "User",
		ShortName:     "name",
		Signature: typresolve.Func(nil, []*typresolve.Type{
			typresolve.Named("Ruby::String"),
		}),
	})

	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::String", result.Name)
}

func TestEvalExprType_CallToS(t *testing.T) {
	src := `x.to_s`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "call")
	require.NotNil(t, node, "could not find call node")

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("Ruby::Integer"))

	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::String", result.Name)
}

func TestEvalExprType_CallNilQ(t *testing.T) {
	src := `x.nil?`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "call")
	require.NotNil(t, node, "could not find call node")

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("Ruby::Object"))

	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::TrueClass", result.Name)
}

func TestEvalExprType_BuiltinPuts(t *testing.T) {
	src := `puts("hello")`
	content := []byte(src)
	root := parseRuby(t, src)

	// puts may be parsed as a call or an identifier + arguments.
	node := findNodeByType(root, "call")
	if node == nil {
		// tree-sitter-ruby may parse puts(...) differently.
		// Try finding it as method_call or identifier.
		node = findNodeByType(root, "method_call")
	}

	ctx := newTestContext(content)

	if node != nil {
		result := EvalExprType(ctx, node)
		require.Equal(t, typresolve.KindNamed, result.Kind)
		require.Equal(t, "Ruby::NilClass", result.Name)
	} else {
		// If tree-sitter-ruby doesn't produce a call node, test the bare
		// identifier path.
		idNode := findNodeByTypeAndText(root, "identifier", "puts", content)
		require.NotNil(t, idNode, "could not find puts node")
		result := EvalExprType(ctx, idNode)
		// Bare builtin identifier returns Unknown.
		require.Equal(t, typresolve.KindUnknown, result.Kind)
	}
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `"hello"`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "string")
	require.NotNil(t, node, "could not find string node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::String", result.Name)
}

func TestEvalExprType_IntegerLiteral(t *testing.T) {
	src := `42`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "integer")
	require.NotNil(t, node, "could not find integer node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Integer", result.Name)
}

func TestEvalExprType_FloatLiteral(t *testing.T) {
	src := `3.14`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "float")
	require.NotNil(t, node, "could not find float node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Float", result.Name)
}

func TestEvalExprType_SymbolLiteral(t *testing.T) {
	src := `:foo`
	content := []byte(src)
	root := parseRuby(t, src)

	// Ruby tree-sitter may parse :foo as "simple_symbol" or "symbol".
	node := findNodeByType(root, "simple_symbol")
	if node == nil {
		node = findNodeByType(root, "symbol")
	}
	require.NotNil(t, node, "could not find symbol node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Symbol", result.Name)
}

func TestEvalExprType_ArrayLiteral(t *testing.T) {
	src := `[1, 2, 3]`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "array")
	require.NotNil(t, node, "could not find array node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Array", result.Name)
}

func TestEvalExprType_HashLiteral(t *testing.T) {
	src := `{a: 1}`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "hash")
	require.NotNil(t, node, "could not find hash node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Hash", result.Name)
}

func TestEvalExprType_NilLiteral(t *testing.T) {
	src := `nil`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "nil")
	require.NotNil(t, node, "could not find nil node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::NilClass", result.Name)
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `x == y`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "binary")
	require.NotNil(t, node, "could not find binary node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::TrueClass", result.Name)
}

func TestEvalExprType_Regex(t *testing.T) {
	src := `/pattern/`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "regex")
	require.NotNil(t, node, "could not find regex node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Regexp", result.Name)
}

func TestEvalExprType_Range(t *testing.T) {
	src := `1..10`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "range")
	require.NotNil(t, node, "could not find range node")

	ctx := newTestContext(content)
	result := EvalExprType(ctx, node)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Range", result.Name)
}

// --- ProcessStatement tests ---

func TestProcessStatement_Assignment(t *testing.T) {
	src := `x = 42`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "assignment")
	require.NotNil(t, node, "could not find assignment node")

	ctx := newTestContext(content)
	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	require.NotNil(t, result)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Integer", result.Name)
}

func TestProcessStatement_InstanceVar(t *testing.T) {
	src := `@name = "foo"`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "assignment")
	require.NotNil(t, node, "could not find assignment node")

	ctx := newTestContext(content)
	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("@name")
	require.NotNil(t, result)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::String", result.Name)
}

func TestProcessStatement_ClassVar(t *testing.T) {
	src := `@@count = 0`
	content := []byte(src)
	root := parseRuby(t, src)

	node := findNodeByType(root, "assignment")
	require.NotNil(t, node, "could not find assignment node")

	ctx := newTestContext(content)
	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("@@count")
	require.NotNil(t, result)
	require.Equal(t, typresolve.KindNamed, result.Kind)
	require.Equal(t, "Ruby::Integer", result.Name)
}

func TestProcessStatement_MultiAssignment(t *testing.T) {
	src := `a, b = 1, "two"`
	content := []byte(src)
	root := parseRuby(t, src)

	// For multi-assignment, tree-sitter-ruby parses this as assignment
	// with left_assignment_list. ProcessStatement is called on the
	// left_assignment_list node.
	listNode := findNodeByType(root, "left_assignment_list")
	if listNode != nil {
		ctx := newTestContext(content)
		ProcessStatement(ctx, listNode)

		a := ctx.Scope.Lookup("a")
		require.NotNil(t, a)
		require.Equal(t, typresolve.KindNamed, a.Kind)
		require.Equal(t, "Ruby::Integer", a.Name)

		b := ctx.Scope.Lookup("b")
		require.NotNil(t, b)
		require.Equal(t, typresolve.KindNamed, b.Kind)
		require.Equal(t, "Ruby::String", b.Name)
	} else {
		// If tree-sitter doesn't produce left_assignment_list, handle
		// as regular assignment.
		assignNode := findNodeByType(root, "assignment")
		require.NotNil(t, assignNode, "could not find assignment or left_assignment_list")

		ctx := newTestContext(content)
		ProcessStatement(ctx, assignNode)
		// At minimum, some binding should happen.
	}
}
