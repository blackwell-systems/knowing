package rustresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestIsBuiltinFunc(t *testing.T) {
	trueNames := []string{"println", "vec", "format", "assert_eq", "todo", "panic", "Box", "Some", "None", "Ok", "Err"}
	for _, name := range trueNames {
		if !IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = false, want true", name)
		}
	}

	falseNames := []string{"my_function", "process_data", "calculate", ""}
	for _, name := range falseNames {
		if IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = true, want false", name)
		}
	}
}

func TestIsBuiltinType(t *testing.T) {
	trueNames := []string{"i32", "String", "Vec", "HashMap", "Option", "Result", "Iterator", "Clone", "bool", "str"}
	for _, name := range trueNames {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}

	falseNames := []string{"MyStruct", "Config", "AppState", ""}
	for _, name := range falseNames {
		if IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = true, want false", name)
		}
	}
}

func TestResolveBuiltinType_Primitive(t *testing.T) {
	tests := []struct {
		name string
		want *typresolve.Type
	}{
		{"i32", typresolve.Builtin("i32")},
		{"bool", typresolve.Builtin("bool")},
		{"str", typresolve.Builtin("str")},
		{"f64", typresolve.Builtin("f64")},
		{"char", typresolve.Builtin("char")},
	}
	for _, tt := range tests {
		got := ResolveBuiltinType(tt.name)
		if !typresolve.TypesEqual(got, tt.want) {
			t.Errorf("ResolveBuiltinType(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveBuiltinType_CoreType(t *testing.T) {
	tests := []struct {
		name string
		want *typresolve.Type
	}{
		{"String", typresolve.Named("std::String")},
		{"Vec", typresolve.Named("std::Vec")},
		{"HashMap", typresolve.Named("std::HashMap")},
		{"Option", typresolve.Named("std::Option")},
		{"Result", typresolve.Named("std::Result")},
		{"Box", typresolve.Named("std::Box")},
		{"Arc", typresolve.Named("std::Arc")},
	}
	for _, tt := range tests {
		got := ResolveBuiltinType(tt.name)
		if !typresolve.TypesEqual(got, tt.want) {
			t.Errorf("ResolveBuiltinType(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveBuiltinType_NonBuiltin(t *testing.T) {
	got := ResolveBuiltinType("MyStruct")
	if got != nil {
		t.Errorf("ResolveBuiltinType(\"MyStruct\") = %v, want nil", got)
	}
}

func TestResolveBuiltinType_Unit(t *testing.T) {
	got := ResolveBuiltinType("()")
	want := typresolve.Builtin("()")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("ResolveBuiltinType(\"()\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Vec(t *testing.T) {
	got := EvalMacroReturnType("vec")
	want := typresolve.Named("std::Vec")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"vec\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Format(t *testing.T) {
	got := EvalMacroReturnType("format")
	want := typresolve.Builtin("str")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"format\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Println(t *testing.T) {
	got := EvalMacroReturnType("println")
	want := typresolve.Builtin("()")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"println\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Todo(t *testing.T) {
	got := EvalMacroReturnType("todo")
	if got.Kind != typresolve.KindUnknown {
		t.Errorf("EvalMacroReturnType(\"todo\").Kind = %v, want KindUnknown", got.Kind)
	}
}

func TestEvalMacroReturnType_IncludeStr(t *testing.T) {
	got := EvalMacroReturnType("include_str")
	want := typresolve.Builtin("str")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"include_str\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_IncludeBytes(t *testing.T) {
	got := EvalMacroReturnType("include_bytes")
	want := typresolve.Slice(typresolve.Builtin("u8"))
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"include_bytes\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Matches(t *testing.T) {
	got := EvalMacroReturnType("matches")
	want := typresolve.Builtin("bool")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"matches\") = %v, want %v", got, want)
	}
}

func TestEvalMacroReturnType_Line(t *testing.T) {
	got := EvalMacroReturnType("line")
	want := typresolve.Builtin("u32")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalMacroReturnType(\"line\") = %v, want %v", got, want)
	}
}

func TestLookupMethod_DirectImpl(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::MyStruct",
		ShortName:     "MyStruct",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "crate::MyStruct.new",
		ReceiverType:  "crate::MyStruct",
		ShortName:     "new",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("crate::MyStruct")}),
	})

	got := LookupMethod(reg, "crate::MyStruct", "new")
	if got == nil {
		t.Fatal("LookupMethod returned nil, want non-nil")
	}
	if got.ShortName != "new" {
		t.Errorf("LookupMethod got ShortName=%q, want \"new\"", got.ShortName)
	}
}

func TestLookupMethod_ViaTraitImpl(t *testing.T) {
	reg := typresolve.NewRegistry()
	// Register trait with method
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Display",
		ShortName:     "Display",
		IsInterface:   true,
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "std::Display.fmt",
		ReceiverType:  "std::Display",
		ShortName:     "fmt",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("()")}),
	})
	// Register type implementing trait
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::Point",
		ShortName:     "Point",
		EmbeddedTypes: []string{"std::Display"},
	})

	got := LookupMethod(reg, "crate::Point", "fmt")
	if got == nil {
		t.Fatal("LookupMethod via trait returned nil, want non-nil")
	}
	if got.ShortName != "fmt" {
		t.Errorf("got ShortName=%q, want \"fmt\"", got.ShortName)
	}
}

func TestLookupMethod_DepthLimit(t *testing.T) {
	reg := typresolve.NewRegistry()
	// Create chain of 10 trait impls to exceed depth limit of 8
	for i := 0; i < 10; i++ {
		qn := "trait" + string(rune('A'+i))
		embedded := []string{}
		if i < 9 {
			embedded = []string{"trait" + string(rune('A'+i+1))}
		}
		reg.AddType(typresolve.RegisteredType{
			QualifiedName: qn,
			ShortName:     qn,
			EmbeddedTypes: embedded,
			IsInterface:   true,
		})
	}
	// Method only on the deepest trait (index 9)
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "traitJ.deep_method",
		ReceiverType:  "traitJ",
		ShortName:     "deep_method",
	})

	// Starting from traitA, depth 8 means we can reach traitA..traitH (indices 0-7)
	// but not traitI (index 8) or traitJ (index 9)
	got := LookupMethod(reg, "traitA", "deep_method")
	if got != nil {
		t.Error("LookupMethod should return nil when exceeding depth limit")
	}
}

func TestLookupMethod_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::Foo",
		ShortName:     "Foo",
	})

	got := LookupMethod(reg, "crate::Foo", "nonexistent")
	if got != nil {
		t.Errorf("LookupMethod for missing method = %v, want nil", got)
	}
}

func TestLookupField_Found(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::Config",
		ShortName:     "Config",
		Fields: []typresolve.Field{
			{Name: "host", Type: typresolve.Builtin("str")},
			{Name: "port", Type: typresolve.Builtin("u16")},
		},
	})

	got := LookupField(reg, "crate::Config", "port")
	want := typresolve.Builtin("u16")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("LookupField(\"port\") = %v, want %v", got, want)
	}
}

func TestLookupField_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "crate::Config",
		ShortName:     "Config",
		Fields: []typresolve.Field{
			{Name: "host", Type: typresolve.Builtin("str")},
		},
	})

	got := LookupField(reg, "crate::Config", "missing")
	if got != nil {
		t.Errorf("LookupField for missing field = %v, want nil", got)
	}
}

func TestDerefToBase_Reference(t *testing.T) {
	inner := typresolve.Named("Foo")
	ref := typresolve.Ref(inner)
	got := DerefToBase(ref)
	if !typresolve.TypesEqual(got, inner) {
		t.Errorf("DerefToBase(Ref(Named(\"Foo\"))) = %v, want Named(\"Foo\")", got)
	}
}

func TestDerefToBase_Pointer(t *testing.T) {
	inner := typresolve.Named("Foo")
	ptr := typresolve.Pointer(inner)
	got := DerefToBase(ptr)
	if !typresolve.TypesEqual(got, inner) {
		t.Errorf("DerefToBase(Pointer(Named(\"Foo\"))) = %v, want Named(\"Foo\")", got)
	}
}

func TestDerefToBase_Box(t *testing.T) {
	inner := typresolve.Named("crate::Inner")
	box := &typresolve.Type{Kind: typresolve.KindNamed, Name: "std::Box", Elem: inner}
	got := DerefToBase(box)
	if !typresolve.TypesEqual(got, inner) {
		t.Errorf("DerefToBase(Box<Inner>) = %v, want Named(\"crate::Inner\")", got)
	}
}

func TestDerefToBase_Arc(t *testing.T) {
	inner := typresolve.Named("crate::Data")
	arc := &typresolve.Type{Kind: typresolve.KindNamed, Name: "std::Arc", Elem: inner}
	got := DerefToBase(arc)
	if !typresolve.TypesEqual(got, inner) {
		t.Errorf("DerefToBase(Arc<Data>) = %v, want Named(\"crate::Data\")", got)
	}
}

func TestDerefToBase_Plain(t *testing.T) {
	plain := typresolve.Named("Foo")
	got := DerefToBase(plain)
	if !typresolve.TypesEqual(got, plain) {
		t.Errorf("DerefToBase(Named(\"Foo\")) = %v, want Named(\"Foo\")", got)
	}
}

func TestDerefToBase_NestedRefBox(t *testing.T) {
	inner := typresolve.Named("crate::Value")
	box := &typresolve.Type{Kind: typresolve.KindNamed, Name: "std::Box", Elem: inner}
	ref := typresolve.Ref(box)
	got := DerefToBase(ref)
	// &Box<T> -> Box<T> -> T
	if !typresolve.TypesEqual(got, inner) {
		t.Errorf("DerefToBase(&Box<Value>) = %v, want Named(\"crate::Value\")", got)
	}
}

func TestDerefToBase_Nil(t *testing.T) {
	got := DerefToBase(nil)
	if got != nil {
		t.Errorf("DerefToBase(nil) = %v, want nil", got)
	}
}
