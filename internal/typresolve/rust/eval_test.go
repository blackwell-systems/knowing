package rustresolve

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/rust"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func parseRust(t *testing.T, src string) *sitter.Node {
	t.Helper()
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())
	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
}

func findNode(root *sitter.Node, nodeType string) *sitter.Node {
	if root.Type() == nodeType {
		return root
	}
	for i := 0; i < int(root.ChildCount()); i++ {
		if found := findNode(root.Child(i), nodeType); found != nil {
			return found
		}
	}
	return nil
}

func newTestContext(content []byte) *ResolveContext {
	return &ResolveContext{
		Registry: typresolve.NewRegistry(),
		Scope:    typresolve.NewScope(nil),
		Uses:     make(map[string]string),
		ModuleQN: "crate::test",
		Content:  content,
	}
}

// --- EvalExprType tests ---

func TestEvalExprType_IdentifierFromScope(t *testing.T) {
	src := `fn main() { let x = foo(); x }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("MyType"))

	// Find the last identifier "x" in the block
	block := findNode(root, "block")
	if block == nil {
		t.Fatal("no block found")
	}
	// Find an identifier node that is "x" (not in let)
	var identNode *sitter.Node
	for i := int(block.ChildCount()) - 1; i >= 0; i-- {
		child := block.Child(i)
		if child.Type() == "identifier" && child.Content(content) == "x" {
			identNode = child
			break
		}
	}
	if identNode == nil {
		// Try expression_statement wrapper
		for i := int(block.ChildCount()) - 1; i >= 0; i-- {
			child := block.Child(i)
			inner := findNode(child, "identifier")
			if inner != nil && inner.Content(content) == "x" {
				identNode = inner
				break
			}
		}
	}
	if identNode == nil {
		t.Fatal("could not find identifier 'x'")
	}

	result := EvalExprType(ctx, identNode)
	if result.Kind != typresolve.KindNamed || result.Name != "MyType" {
		t.Errorf("expected Named(MyType), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_IntegerLiteral(t *testing.T) {
	src := `fn main() { 42 }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "integer_literal")
	if node == nil {
		t.Fatal("no integer_literal found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "i32" {
		t.Errorf("expected Builtin(i32), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_FloatLiteral(t *testing.T) {
	src := `fn main() { 3.14 }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "float_literal")
	if node == nil {
		t.Fatal("no float_literal found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "f64" {
		t.Errorf("expected Builtin(f64), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_StringLiteral(t *testing.T) {
	src := `fn main() { "hello" }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "string_literal")
	if node == nil {
		t.Fatal("no string_literal found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "str" {
		t.Errorf("expected Builtin(str), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_BoolLiteral(t *testing.T) {
	src := `fn main() { true }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	// tree-sitter may parse "true" as "true" node type or boolean_literal
	node := findNode(root, "boolean_literal")
	if node == nil {
		node = findNode(root, "true")
	}
	if node == nil {
		t.Fatal("no boolean literal found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "bool" {
		t.Errorf("expected Builtin(bool), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_CharLiteral(t *testing.T) {
	src := `fn main() { 'a' }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "char_literal")
	if node == nil {
		t.Fatal("no char_literal found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "char" {
		t.Errorf("expected Builtin(char), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_ArrayLiteral(t *testing.T) {
	src := `fn main() { [1, 2, 3] }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "array_expression")
	if node == nil {
		t.Fatal("no array_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindSlice {
		t.Errorf("expected Slice, got %v", result.Kind)
	}
}

func TestEvalExprType_Reference(t *testing.T) {
	src := `fn main() { &x }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Named("Foo"))

	node := findNode(root, "reference_expression")
	if node == nil {
		t.Fatal("no reference_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindReference {
		t.Errorf("expected Reference, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindNamed || result.Elem.Name != "Foo" {
		t.Errorf("expected Ref(Named(Foo)), got %v", result)
	}
}

func TestEvalExprType_BinaryComparison(t *testing.T) {
	src := `fn main() { x == y }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "binary_expression")
	if node == nil {
		t.Fatal("no binary_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "bool" {
		t.Errorf("expected Builtin(bool), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_BinaryArithmetic(t *testing.T) {
	src := `fn main() { x + y }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)
	ctx.Scope.Bind("x", typresolve.Builtin("i32"))
	ctx.Scope.Bind("y", typresolve.Builtin("i32"))

	node := findNode(root, "binary_expression")
	if node == nil {
		t.Fatal("no binary_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "i32" {
		t.Errorf("expected Builtin(i32), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_MacroInvocation(t *testing.T) {
	src := `fn main() { vec![1, 2] }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "macro_invocation")
	if node == nil {
		t.Fatal("no macro_invocation found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindNamed || result.Name != "std::Vec" {
		t.Errorf("expected Named(std::Vec), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_StructExpression(t *testing.T) {
	src := `fn main() { Config { port: 8080 } }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)
	ctx.Registry.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::test::Config",
		ShortName:     "Config",
	})

	node := findNode(root, "struct_expression")
	if node == nil {
		t.Fatal("no struct_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindNamed || result.Name != "crate::test::Config" {
		t.Errorf("expected Named(crate::test::Config), got %v %q", result.Kind, result.Name)
	}
}

func TestEvalExprType_TryExpression(t *testing.T) {
	src := `fn main() { foo()? }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	// Register foo() as returning Result with Elem=i32
	resultType := typresolve.Named("std::Result")
	resultType.Elem = typresolve.Builtin("i32")
	ctx.Registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "crate::test::foo",
		ShortName:     "foo",
		Signature:     typresolve.Func(nil, []*typresolve.Type{resultType}),
	})
	ctx.Scope.Bind("foo", typresolve.Func(nil, []*typresolve.Type{resultType}))

	node := findNode(root, "try_expression")
	if node == nil {
		t.Fatal("no try_expression found")
	}

	result := EvalExprType(ctx, node)
	if result.Kind != typresolve.KindBuiltin || result.Name != "i32" {
		t.Errorf("expected Builtin(i32), got %v %q", result.Kind, result.Name)
	}
}

// --- ParseTypeNode tests ---

func TestParseTypeNode_Primitive(t *testing.T) {
	src := `fn foo() -> i32 { 0 }`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "primitive_type")
	if node == nil {
		t.Fatal("no primitive_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindBuiltin || result.Name != "i32" {
		t.Errorf("expected Builtin(i32), got %v %q", result.Kind, result.Name)
	}
}

func TestParseTypeNode_Reference(t *testing.T) {
	src := `fn foo(x: &str) {}`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "reference_type")
	if node == nil {
		t.Fatal("no reference_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindReference {
		t.Errorf("expected Reference, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindBuiltin || result.Elem.Name != "str" {
		t.Errorf("expected Ref(Builtin(str)), got elem=%v", result.Elem)
	}
}

func TestParseTypeNode_MutReference(t *testing.T) {
	src := `fn foo(x: &mut String) {}`
	content := []byte(src)
	root := parseRust(t, src)
	uses := map[string]string{"String": "std::String"}

	node := findNode(root, "reference_type")
	if node == nil {
		t.Fatal("no reference_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", uses)
	if result.Kind != typresolve.KindReference {
		t.Errorf("expected Reference, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindNamed || result.Elem.Name != "std::String" {
		t.Errorf("expected Ref(Named(std::String)), got elem=%v", result.Elem)
	}
}

func TestParseTypeNode_Option(t *testing.T) {
	src := `fn foo() -> Option<i32> { None }`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "generic_type")
	if node == nil {
		t.Fatal("no generic_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindOptional {
		t.Errorf("expected Optional, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindBuiltin || result.Elem.Name != "i32" {
		t.Errorf("expected Optional(Builtin(i32)), got elem=%v", result.Elem)
	}
}

func TestParseTypeNode_Vec(t *testing.T) {
	src := `fn foo() -> Vec<String> { vec![] }`
	content := []byte(src)
	root := parseRust(t, src)
	uses := map[string]string{"String": "std::String"}

	node := findNode(root, "generic_type")
	if node == nil {
		t.Fatal("no generic_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", uses)
	if result.Kind != typresolve.KindSlice {
		t.Errorf("expected Slice, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindNamed || result.Elem.Name != "std::String" {
		t.Errorf("expected Slice(Named(std::String)), got elem=%v", result.Elem)
	}
}

func TestParseTypeNode_HashMap(t *testing.T) {
	src := `fn foo() -> HashMap<String, i32> { todo!() }`
	content := []byte(src)
	root := parseRust(t, src)
	uses := map[string]string{"String": "std::String"}

	node := findNode(root, "generic_type")
	if node == nil {
		t.Fatal("no generic_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", uses)
	if result.Kind != typresolve.KindMap {
		t.Errorf("expected Map, got %v", result.Kind)
	}
	if result.Key == nil || result.Key.Name != "std::String" {
		t.Errorf("expected Map key Named(std::String), got %v", result.Key)
	}
	if result.Value == nil || result.Value.Kind != typresolve.KindBuiltin || result.Value.Name != "i32" {
		t.Errorf("expected Map value Builtin(i32), got %v", result.Value)
	}
}

func TestParseTypeNode_Tuple(t *testing.T) {
	src := `fn foo() -> (i32, String) { todo!() }`
	content := []byte(src)
	root := parseRust(t, src)
	uses := map[string]string{"String": "std::String"}

	node := findNode(root, "tuple_type")
	if node == nil {
		t.Fatal("no tuple_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", uses)
	if result.Kind != typresolve.KindTuple {
		t.Errorf("expected Tuple, got %v", result.Kind)
	}
	if len(result.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result.Elements))
	}
	if result.Elements[0].Kind != typresolve.KindBuiltin || result.Elements[0].Name != "i32" {
		t.Errorf("expected elem[0] Builtin(i32), got %v", result.Elements[0])
	}
	if result.Elements[1].Kind != typresolve.KindNamed || result.Elements[1].Name != "std::String" {
		t.Errorf("expected elem[1] Named(std::String), got %v", result.Elements[1])
	}
}

func TestParseTypeNode_Unit(t *testing.T) {
	src := `fn foo() -> () {}`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "unit_type")
	if node == nil {
		t.Fatal("no unit_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindBuiltin || result.Name != "()" {
		t.Errorf("expected Builtin(()), got %v %q", result.Kind, result.Name)
	}
}

func TestParseTypeNode_Array(t *testing.T) {
	src := `fn foo() -> [u8; 32] { todo!() }`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "array_type")
	if node == nil {
		t.Fatal("no array_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindArray {
		t.Errorf("expected Array, got %v", result.Kind)
	}
	if result.Elem == nil || result.Elem.Kind != typresolve.KindBuiltin || result.Elem.Name != "u8" {
		t.Errorf("expected Array(Builtin(u8)), got elem=%v", result.Elem)
	}
}

func TestParseTypeNode_Box(t *testing.T) {
	src := `fn foo() -> Box<dyn Error> { todo!() }`
	content := []byte(src)
	root := parseRust(t, src)

	node := findNode(root, "generic_type")
	if node == nil {
		t.Fatal("no generic_type found")
	}

	result := ParseTypeNode(node, content, "crate::test", nil)
	if result.Kind != typresolve.KindNamed || result.Name != "std::Box" {
		t.Errorf("expected Named(std::Box), got %v %q", result.Kind, result.Name)
	}
	if result.Elem == nil {
		t.Error("expected Box to have Elem set")
	}
}

// --- ProcessStatement tests ---

func TestProcessStatement_LetBinding(t *testing.T) {
	src := `fn main() { let x = 42; }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "let_declaration")
	if node == nil {
		t.Fatal("no let_declaration found")
	}

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	if result == nil {
		t.Fatal("x not bound")
	}
	if result.Kind != typresolve.KindBuiltin || result.Name != "i32" {
		t.Errorf("expected Builtin(i32), got %v %q", result.Kind, result.Name)
	}
}

func TestProcessStatement_LetWithType(t *testing.T) {
	src := `fn main() { let x: String = String::new(); }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)
	ctx.Uses["String"] = "std::String"

	node := findNode(root, "let_declaration")
	if node == nil {
		t.Fatal("no let_declaration found")
	}

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	if result == nil {
		t.Fatal("x not bound")
	}
	if result.Kind != typresolve.KindNamed || result.Name != "std::String" {
		t.Errorf("expected Named(std::String), got %v %q", result.Kind, result.Name)
	}
}

func TestProcessStatement_LetMut(t *testing.T) {
	src := `fn main() { let mut x = "hello"; }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "let_declaration")
	if node == nil {
		t.Fatal("no let_declaration found")
	}

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("x")
	if result == nil {
		t.Fatal("x not bound")
	}
	if result.Kind != typresolve.KindBuiltin || result.Name != "str" {
		t.Errorf("expected Builtin(str), got %v %q", result.Kind, result.Name)
	}
}

func TestProcessStatement_LetTuple(t *testing.T) {
	src := `fn main() { let (a, b) = (1, "two"); }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "let_declaration")
	if node == nil {
		t.Fatal("no let_declaration found")
	}

	ProcessStatement(ctx, node)

	a := ctx.Scope.Lookup("a")
	if a == nil {
		t.Fatal("a not bound")
	}
	b := ctx.Scope.Lookup("b")
	if b == nil {
		t.Fatal("b not bound")
	}
	// a should be i32, b should be str
	if a.Kind != typresolve.KindBuiltin || a.Name != "i32" {
		t.Errorf("expected a=Builtin(i32), got %v %q", a.Kind, a.Name)
	}
	if b.Kind != typresolve.KindBuiltin || b.Name != "str" {
		t.Errorf("expected b=Builtin(str), got %v %q", b.Kind, b.Name)
	}
}

func TestProcessStatement_ForLoop(t *testing.T) {
	src := `fn main() { for item in items {} }`
	content := []byte(src)
	root := parseRust(t, src)
	ctx := newTestContext(content)

	node := findNode(root, "for_expression")
	if node == nil {
		t.Fatal("no for_expression found")
	}

	ProcessStatement(ctx, node)

	result := ctx.Scope.Lookup("item")
	if result == nil {
		t.Fatal("item not bound")
	}
	if result.Kind != typresolve.KindUnknown {
		t.Errorf("expected Unknown, got %v", result.Kind)
	}
}
