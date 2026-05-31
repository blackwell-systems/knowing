package goresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
)

// parseAndFindExpr parses Go source and returns the first node matching exprType.
func parseAndFindExpr(t *testing.T, src string, exprType string) (*sitter.Node, []byte) {
	t.Helper()
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	t.Cleanup(func() { tree.Close() })

	var found *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if found != nil {
			return
		}
		if n.Type() == exprType {
			found = n
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(tree.RootNode())
	if found == nil {
		t.Fatalf("no %s found in source", exprType)
	}
	return found, content
}

// parseGoRoot parses Go source and returns the root node.
func parseGoRoot(t *testing.T, src string) (*sitter.Node, []byte) {
	t.Helper()
	content := []byte(src)
	parser := sitter.NewParser()
	parser.SetLanguage(golang.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode(), content
}

// newTestContext creates a ResolveContext for testing.
func newTestContext(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]string),
		PkgQN:    "pkg",
		Content:  content,
	}
}

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `package main
func f() {
	x := 1
	_ = x
}
`
	node, content := parseAndFindExpr(t, src, "identifier")
	// Find the "x" identifier in the `_ = x` assignment (last identifier).
	// We need to find a standalone x usage. Let's search more carefully.
	var xNode *sitter.Node
	var walkAll func(n *sitter.Node)
	walkAll = func(n *sitter.Node) {
		if n.Type() == "identifier" && n.Content(content) == "x" {
			xNode = n // keep last one found
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walkAll(n.Child(int(i)))
		}
	}
	_ = node // suppress unused
	root, _ := parseGoRoot(t, src)
	walkAll(root)

	if xNode == nil {
		t.Fatal("could not find identifier 'x'")
	}

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("pkg.MyType"))

	result := EvalExprType(ctx, xNode)
	if result.Kind != typresolve.KindNamed {
		t.Errorf("expected KindNamed, got %v", result.Kind)
	}
	if result.Name != "pkg.MyType" {
		t.Errorf("expected name pkg.MyType, got %s", result.Name)
	}
}

func TestEvalExprType_IdentifierPackageFunc(t *testing.T) {
	src := `package main
func f() {
	_ = myFunc
}
`
	node, content := parseAndFindExpr(t, src, "identifier")
	// Find "myFunc" identifier.
	var target *sitter.Node
	root, _ := parseGoRoot(t, src)
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if n.Type() == "identifier" && n.Content(content) == "myFunc" {
			target = n
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(root)
	_ = node

	if target == nil {
		t.Fatal("could not find 'myFunc' identifier")
	}

	ctx := newTestContext(content)
	sig := typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.myFunc",
		ShortName:     "myFunc",
		Signature:     sig,
	})

	result := EvalExprType(ctx, target)
	if result.Kind != typresolve.KindFunc {
		t.Errorf("expected KindFunc, got %v", result.Kind)
	}
}

func TestEvalExprType_SelectorImport(t *testing.T) {
	src := `package main
import "net/http"
func f() {
	_ = http.Get
}
`
	node, content := parseAndFindExpr(t, src, "selector_expression")

	ctx := newTestContext(content)
	ctx.Imports["http"] = "net/http"
	sig := typresolve.Func(
		[]typresolve.Param{{Name: "url", Type: typresolve.Builtin("string")}},
		[]*typresolve.Type{typresolve.Named("net/http.Response"), typresolve.Builtin("error")},
	)
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Get",
		ShortName:     "Get",
		Signature:     sig,
	})

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindFunc {
		t.Errorf("expected KindFunc, got %v", result.Kind)
	}
	if len(result.Returns) != 2 {
		t.Errorf("expected 2 returns, got %d", len(result.Returns))
	}
}

func TestEvalExprType_SelectorMethod(t *testing.T) {
	src := `package main
func f(s MyType) {
	_ = s.Method
}
`
	node, content := parseAndFindExpr(t, src, "selector_expression")

	ctx := newTestContext(content)
	ctx.Scope.Bind("s", typresolve.Named("pkg.MyType"))
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyType",
		ShortName:     "MyType",
		MethodNames:   []string{"Method"},
		MethodQNs:     []string{"pkg.MyType.Method"},
	})
	sig := typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.MyType.Method",
		ShortName:     "Method",
		ReceiverType:  "pkg.MyType",
		Signature:     sig,
	})

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindFunc {
		t.Errorf("expected KindFunc, got %v", result.Kind)
	}
}

func TestEvalExprType_CallReturnsType(t *testing.T) {
	src := `package main
func f() {
	_ = myFunc()
}
`
	node, content := parseAndFindExpr(t, src, "call_expression")

	ctx := newTestContext(content)
	sig := typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.myFunc",
		ShortName:     "myFunc",
		Signature:     sig,
	})

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "string" {
		t.Errorf("expected Builtin(string), got %v(%s)", result.Kind, result.Name)
	}
}

func TestEvalExprType_BuiltinLen(t *testing.T) {
	src := `package main
func f() {
	_ = len("hello")
}
`
	node, content := parseAndFindExpr(t, src, "call_expression")

	ctx := newTestContext(content)

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "int" {
		t.Errorf("expected Builtin(int), got %v(%s)", result.Kind, result.Name)
	}
}

func TestEvalExprType_CompositeLiteral(t *testing.T) {
	src := `package main
func f() {
	_ = MyType{}
}
`
	node, content := parseAndFindExpr(t, src, "composite_literal")

	ctx := newTestContext(content)
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyType",
		ShortName:     "MyType",
	})

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindNamed {
		t.Errorf("expected KindNamed, got %v", result.Kind)
	}
}

func TestEvalExprType_UnaryAddress(t *testing.T) {
	src := `package main
func f() {
	x := 1
	_ = &x
}
`
	node, content := parseAndFindExpr(t, src, "unary_expression")

	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Builtin("int"))

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindPointer {
		t.Errorf("expected KindPointer, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindBuiltin || result.Elem.Name != "int" {
		t.Errorf("expected Pointer(int), got unexpected elem")
	}
}

func TestEvalExprType_IndexSlice(t *testing.T) {
	src := `package main
func f() {
	_ = xs[0]
}
`
	node, content := parseAndFindExpr(t, src, "index_expression")

	ctx := newTestContext(content)
	ctx.Scope.Bind("xs", typresolve.Slice(typresolve.Builtin("string")))

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "string" {
		t.Errorf("expected Builtin(string), got %v(%s)", result.Kind, result.Name)
	}
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `package main
func f() {
	_ = "hello"
}
`
	node, content := parseAndFindExpr(t, src, "interpreted_string_literal")

	ctx := newTestContext(content)

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "string" {
		t.Errorf("expected Builtin(string), got %v(%s)", result.Kind, result.Name)
	}
}

func TestEvalExprType_IntLiteral(t *testing.T) {
	src := `package main
func f() {
	_ = 42
}
`
	node, content := parseAndFindExpr(t, src, "int_literal")

	ctx := newTestContext(content)

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "int" {
		t.Errorf("expected Builtin(int), got %v(%s)", result.Kind, result.Name)
	}
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `package main
func f() {
	_ = 1 == 2
}
`
	node, content := parseAndFindExpr(t, src, "binary_expression")

	ctx := newTestContext(content)

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "bool" {
		t.Errorf("expected Builtin(bool), got %v(%s)", result.Kind, result.Name)
	}
}

// --- ProcessStatement tests ---

func TestProcessStatement_ShortVarDecl(t *testing.T) {
	src := `package main
func f() {
	x := 42
}
`
	root, content := parseGoRoot(t, src)

	ctx := newTestContext(content)

	// Find the short_var_declaration node.
	var svd *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if svd != nil {
			return
		}
		if n.Type() == "short_var_declaration" {
			svd = n
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(root)

	if svd == nil {
		t.Fatal("no short_var_declaration found")
	}

	ProcessStatement(ctx, svd)

	bound := ctx.Scope.Lookup("x")
	if bound == nil {
		t.Fatal("x not bound in scope")
	}
	if bound.Kind != typresolve.KindBuiltin || bound.Name != "int" {
		t.Errorf("expected Builtin(int), got %v(%s)", bound.Kind, bound.Name)
	}
}

func TestProcessStatement_ShortVarDeclMultiReturn(t *testing.T) {
	src := `package main
func f() {
	a, err := foo()
}
`
	root, content := parseGoRoot(t, src)

	ctx := newTestContext(content)
	// Register foo as returning (string, error).
	sig := typresolve.Func(nil, []*typresolve.Type{
		typresolve.Builtin("string"),
		typresolve.Builtin("error"),
	})
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.foo",
		ShortName:     "foo",
		Signature:     sig,
	})

	var svd *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if svd != nil {
			return
		}
		if n.Type() == "short_var_declaration" {
			svd = n
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(root)

	if svd == nil {
		t.Fatal("no short_var_declaration found")
	}

	ProcessStatement(ctx, svd)

	a := ctx.Scope.Lookup("a")
	if a == nil {
		t.Fatal("a not bound")
	}
	if a.Kind != typresolve.KindBuiltin || a.Name != "string" {
		t.Errorf("expected Builtin(string) for a, got %v(%s)", a.Kind, a.Name)
	}

	errVar := ctx.Scope.Lookup("err")
	if errVar == nil {
		t.Fatal("err not bound")
	}
	if errVar.Kind != typresolve.KindBuiltin || errVar.Name != "error" {
		t.Errorf("expected Builtin(error) for err, got %v(%s)", errVar.Kind, errVar.Name)
	}
}

func TestProcessStatement_VarSpecWithType(t *testing.T) {
	src := `package main
func f() {
	var x int
}
`
	root, content := parseGoRoot(t, src)

	ctx := newTestContext(content)

	var vs *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if vs != nil {
			return
		}
		if n.Type() == "var_declaration" {
			vs = n
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(root)

	if vs == nil {
		t.Fatal("no var_declaration found")
	}

	ProcessStatement(ctx, vs)

	bound := ctx.Scope.Lookup("x")
	if bound == nil {
		t.Fatal("x not bound in scope")
	}
	if bound.Kind != typresolve.KindBuiltin || bound.Name != "int" {
		t.Errorf("expected Builtin(int), got %v(%s)", bound.Kind, bound.Name)
	}
}

func TestProcessStatement_RangeOverSlice(t *testing.T) {
	src := `package main
func f() {
	for i, v := range xs {
		_ = i
		_ = v
	}
}
`
	root, content := parseGoRoot(t, src)

	ctx := newTestContext(content)
	ctx.Scope.Bind("xs", typresolve.Slice(typresolve.Builtin("string")))

	// Find the for_statement (which contains the range_clause).
	var forStmt *sitter.Node
	var walk func(n *sitter.Node)
	walk = func(n *sitter.Node) {
		if forStmt != nil {
			return
		}
		if n.Type() == "for_statement" {
			forStmt = n
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(int(i)))
		}
	}
	walk(root)

	if forStmt == nil {
		t.Fatal("no for_statement found")
	}

	ProcessStatement(ctx, forStmt)

	i := ctx.Scope.Lookup("i")
	if i == nil {
		t.Fatal("i not bound")
	}
	if i.Kind != typresolve.KindBuiltin || i.Name != "int" {
		t.Errorf("expected Builtin(int) for i, got %v(%s)", i.Kind, i.Name)
	}

	v := ctx.Scope.Lookup("v")
	if v == nil {
		t.Fatal("v not bound")
	}
	if v.Kind != typresolve.KindBuiltin || v.Name != "string" {
		t.Errorf("expected Builtin(string) for v, got %v(%s)", v.Kind, v.Name)
	}
}
