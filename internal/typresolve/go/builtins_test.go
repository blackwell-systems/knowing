package goresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseGoSource is defined in types_test.go (same package).

// --- IsBuiltinFunc tests ---

func TestIsBuiltinFunc(t *testing.T) {
	trueNames := []string{"make", "len", "append", "new", "cap", "delete", "close",
		"copy", "panic", "recover", "print", "println", "complex", "real", "imag",
		"min", "max", "clear"}
	for _, name := range trueNames {
		assert.True(t, IsBuiltinFunc(name), "expected %q to be a builtin func", name)
	}

	falseNames := []string{"fmt", "MyFunc", "Println", ""}
	for _, name := range falseNames {
		assert.False(t, IsBuiltinFunc(name), "expected %q to NOT be a builtin func", name)
	}
}

// --- ResolveBuiltinType tests ---

func TestResolveBuiltinType(t *testing.T) {
	for _, name := range []string{"int", "string", "error", "bool", "byte", "rune", "any"} {
		typ := ResolveBuiltinType(name)
		require.NotNil(t, typ, "expected %q to resolve to a builtin type", name)
		assert.Equal(t, typresolve.KindBuiltin, typ.Kind)
		assert.Equal(t, name, typ.Name)
	}

	assert.Nil(t, ResolveBuiltinType("MyType"))
	assert.Nil(t, ResolveBuiltinType(""))
	assert.Nil(t, ResolveBuiltinType("fmt"))
}

// --- EvalBuiltinCall tests ---

func TestEvalBuiltinCall_Make(t *testing.T) {
	src := `package main
func f() { make([]int, 0) }
`
	content := []byte(src)
	root := parseGoSource(t, src)

	// Navigate to the argument_list of make([]int, 0).
	// source_file -> function_declaration -> block -> expression_statement -> call_expression -> argument_list
	callExpr := root.NamedChild(1).ChildByFieldName("body").NamedChild(0).NamedChild(0)
	require.Equal(t, "call_expression", callExpr.Type())
	args := callExpr.ChildByFieldName("arguments")
	require.NotNil(t, args)

	result := EvalBuiltinCall("make", args, content, "main", nil, nil)
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindSlice, result.Kind)
}

func TestEvalBuiltinCall_New(t *testing.T) {
	src := `package main
func f() { new(int) }
`
	content := []byte(src)
	root := parseGoSource(t, src)

	callExpr := root.NamedChild(1).ChildByFieldName("body").NamedChild(0).NamedChild(0)
	require.Equal(t, "call_expression", callExpr.Type())
	args := callExpr.ChildByFieldName("arguments")
	require.NotNil(t, args)

	result := EvalBuiltinCall("new", args, content, "main", nil, nil)
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindPointer, result.Kind)
	require.NotNil(t, result.Elem)
	assert.Equal(t, typresolve.KindBuiltin, result.Elem.Kind)
	assert.Equal(t, "int", result.Elem.Name)
}

func TestEvalBuiltinCall_Len(t *testing.T) {
	result := EvalBuiltinCall("len", nil, nil, "", nil, nil)
	require.NotNil(t, result)
	assert.Equal(t, typresolve.KindBuiltin, result.Kind)
	assert.Equal(t, "int", result.Name)
}

func TestEvalBuiltinCall_Append(t *testing.T) {
	sliceType := typresolve.Slice(typresolve.Builtin("string"))

	evalExpr := func(node *sitter.Node) *typresolve.Type {
		return sliceType
	}

	src := `package main
func f() { append(s, "x") }
`
	content := []byte(src)
	root := parseGoSource(t, src)

	callExpr := root.NamedChild(1).ChildByFieldName("body").NamedChild(0).NamedChild(0)
	require.Equal(t, "call_expression", callExpr.Type())
	args := callExpr.ChildByFieldName("arguments")

	result := EvalBuiltinCall("append", args, content, "main", nil, evalExpr)
	require.NotNil(t, result)
	assert.True(t, typresolve.TypesEqual(sliceType, result))
}

// --- LookupFieldOrMethod tests ---

func TestLookupFieldOrMethod_DirectMethod(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyStruct",
		ShortName:     "MyStruct",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.MyStruct.DoStuff",
		ReceiverType:  "pkg.MyStruct",
		ShortName:     "DoStuff",
	})

	f := LookupFieldOrMethod(reg, "pkg.MyStruct", "DoStuff")
	require.NotNil(t, f)
	assert.Equal(t, "pkg.MyStruct.DoStuff", f.QualifiedName)
}

func TestLookupFieldOrMethod_ViaEmbedding1Level(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Inner",
		ShortName:     "Inner",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.Inner.Method",
		ReceiverType:  "pkg.Inner",
		ShortName:     "Method",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Outer",
		ShortName:     "Outer",
		EmbeddedTypes: []string{"pkg.Inner"},
	})

	f := LookupFieldOrMethod(reg, "pkg.Outer", "Method")
	require.NotNil(t, f)
	assert.Equal(t, "pkg.Inner.Method", f.QualifiedName)
}

func TestLookupFieldOrMethod_ViaEmbedding2Levels(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Base",
		ShortName:     "Base",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "pkg.Base.DeepMethod",
		ReceiverType:  "pkg.Base",
		ShortName:     "DeepMethod",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Mid",
		ShortName:     "Mid",
		EmbeddedTypes: []string{"pkg.Base"},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Top",
		ShortName:     "Top",
		EmbeddedTypes: []string{"pkg.Mid"},
	})

	f := LookupFieldOrMethod(reg, "pkg.Top", "DeepMethod")
	require.NotNil(t, f)
	assert.Equal(t, "pkg.Base.DeepMethod", f.QualifiedName)
}

func TestLookupFieldOrMethod_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyStruct",
		ShortName:     "MyStruct",
	})

	f := LookupFieldOrMethod(reg, "pkg.MyStruct", "NonExistent")
	assert.Nil(t, f)
}

// --- LookupField tests ---

func TestLookupField_DirectField(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyStruct",
		ShortName:     "MyStruct",
		Fields: []typresolve.Field{
			{Name: "Name", Type: typresolve.Builtin("string")},
			{Name: "Age", Type: typresolve.Builtin("int")},
		},
	})

	ft := LookupField(reg, "pkg.MyStruct", "Name")
	require.NotNil(t, ft)
	assert.Equal(t, typresolve.KindBuiltin, ft.Kind)
	assert.Equal(t, "string", ft.Name)
}

func TestLookupField_ViaEmbedding(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Base",
		ShortName:     "Base",
		Fields: []typresolve.Field{
			{Name: "ID", Type: typresolve.Builtin("int")},
		},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.Child",
		ShortName:     "Child",
		EmbeddedTypes: []string{"pkg.Base"},
	})

	ft := LookupField(reg, "pkg.Child", "ID")
	require.NotNil(t, ft)
	assert.Equal(t, typresolve.KindBuiltin, ft.Kind)
	assert.Equal(t, "int", ft.Name)
}

func TestLookupField_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "pkg.MyStruct",
		ShortName:     "MyStruct",
	})

	ft := LookupField(reg, "pkg.MyStruct", "NonExistent")
	assert.Nil(t, ft)
}
