package pyresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestIsBuiltinFunc(t *testing.T) {
	for _, name := range []string{"print", "len", "range", "isinstance"} {
		if !IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"MyFunc", "flask", "django", ""} {
		if IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = true, want false", name)
		}
	}
}

func TestIsBuiltinType(t *testing.T) {
	for _, name := range []string{"int", "str", "list", "dict", "None"} {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"MyClass", "Django", ""} {
		if IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = true, want false", name)
		}
	}
}

func TestResolveBuiltinType(t *testing.T) {
	got := ResolveBuiltinType("int")
	if got == nil || got.Kind != typresolve.KindBuiltin || got.Name != "int" {
		t.Errorf("ResolveBuiltinType(\"int\") = %v, want Builtin(\"int\")", got)
	}

	got = ResolveBuiltinType("str")
	if got == nil || got.Kind != typresolve.KindBuiltin || got.Name != "str" {
		t.Errorf("ResolveBuiltinType(\"str\") = %v, want Builtin(\"str\")", got)
	}

	got = ResolveBuiltinType("MyType")
	if got != nil {
		t.Errorf("ResolveBuiltinType(\"MyType\") = %v, want nil", got)
	}
}

func TestLiteralType(t *testing.T) {
	tests := []struct {
		nodeType string
		wantName string
	}{
		{"integer", "int"},
		{"float", "float"},
		{"string", "str"},
		{"concatenated_string", "str"},
		{"true", "bool"},
		{"false", "bool"},
		{"none", "None"},
		{"list_comprehension", "list"},
		{"dictionary_comprehension", "dict"},
		{"set_comprehension", "set"},
		{"generator_expression", "generator"},
	}
	for _, tt := range tests {
		got := LiteralType(tt.nodeType)
		if got == nil || got.Kind != typresolve.KindBuiltin || got.Name != tt.wantName {
			t.Errorf("LiteralType(%q) = %v, want Builtin(%q)", tt.nodeType, got, tt.wantName)
		}
	}

	if got := LiteralType("some_other"); got != nil {
		t.Errorf("LiteralType(\"some_other\") = %v, want nil", got)
	}
}

func TestIterableElementType(t *testing.T) {
	// Slice -> element type
	intType := typresolve.Builtin("int")
	sliceType := typresolve.Slice(intType)
	got := IterableElementType(sliceType)
	if !typresolve.TypesEqual(got, intType) {
		t.Errorf("IterableElementType(Slice(int)) = %v, want int", got)
	}

	// Map -> key type
	strType := typresolve.Builtin("str")
	mapType := typresolve.Map(strType, intType)
	got = IterableElementType(mapType)
	if !typresolve.TypesEqual(got, strType) {
		t.Errorf("IterableElementType(Map(str,int)) = %v, want str", got)
	}

	// Single-element tuple -> that element
	tupleType := typresolve.Tuple([]*typresolve.Type{intType})
	got = IterableElementType(tupleType)
	if !typresolve.TypesEqual(got, intType) {
		t.Errorf("IterableElementType(Tuple(int)) = %v, want int", got)
	}

	// Multi-element tuple -> Unknown
	tupleType2 := typresolve.Tuple([]*typresolve.Type{intType, strType})
	got = IterableElementType(tupleType2)
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("IterableElementType(Tuple(int,str)) = %v, want Unknown", got)
	}

	// Unknown -> Unknown
	got = IterableElementType(typresolve.Unknown())
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("IterableElementType(Unknown) = %v, want Unknown", got)
	}

	// nil -> Unknown
	got = IterableElementType(nil)
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("IterableElementType(nil) = %v, want Unknown", got)
	}
}

// buildTestRegistry creates a registry simulating a Python class hierarchy:
//
//	class Base:
//	    name: str
//	    def greet(self): ...
//
//	class Middle(Base):
//	    age: int
//	    def hello(self): ...
//
//	class Child(Middle):
//	    def wave(self): ...
func buildTestRegistry() *typresolve.Registry {
	reg := typresolve.NewRegistry()

	// Base class with field "name" and method "greet"
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "myapp.Base",
		ShortName:     "Base",
		Fields: []typresolve.Field{
			{Name: "name", Type: typresolve.Builtin("str")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "myapp.Base.greet",
		ReceiverType:  "myapp.Base",
		ShortName:     "greet",
	})

	// Middle class inherits from Base, adds field "age" and method "hello"
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "myapp.Middle",
		ShortName:     "Middle",
		EmbeddedTypes: []string{"myapp.Base"},
		Fields: []typresolve.Field{
			{Name: "age", Type: typresolve.Builtin("int")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "myapp.Middle.hello",
		ReceiverType:  "myapp.Middle",
		ShortName:     "hello",
	})

	// Child class inherits from Middle, adds method "wave"
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "myapp.Child",
		ShortName:     "Child",
		EmbeddedTypes: []string{"myapp.Middle"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "myapp.Child.wave",
		ReceiverType:  "myapp.Child",
		ShortName:     "wave",
	})

	return reg
}

func TestLookupAttribute_Direct(t *testing.T) {
	reg := buildTestRegistry()

	f := LookupAttribute(reg, "myapp.Base", "greet")
	if f == nil {
		t.Fatal("LookupAttribute(Base, greet) = nil, want non-nil")
	}
	if f.QualifiedName != "myapp.Base.greet" {
		t.Errorf("got QN %q, want myapp.Base.greet", f.QualifiedName)
	}
}

func TestLookupAttribute_ViaBaseClass(t *testing.T) {
	reg := buildTestRegistry()

	// Middle inherits greet from Base
	f := LookupAttribute(reg, "myapp.Middle", "greet")
	if f == nil {
		t.Fatal("LookupAttribute(Middle, greet) = nil, want non-nil")
	}
	if f.QualifiedName != "myapp.Base.greet" {
		t.Errorf("got QN %q, want myapp.Base.greet", f.QualifiedName)
	}
}

func TestLookupAttribute_ViaBaseClassChain(t *testing.T) {
	reg := buildTestRegistry()

	// Child -> Middle -> Base: greet should be found 2 levels up
	f := LookupAttribute(reg, "myapp.Child", "greet")
	if f == nil {
		t.Fatal("LookupAttribute(Child, greet) = nil, want non-nil")
	}
	if f.QualifiedName != "myapp.Base.greet" {
		t.Errorf("got QN %q, want myapp.Base.greet", f.QualifiedName)
	}
}

func TestLookupAttribute_NotFound(t *testing.T) {
	reg := buildTestRegistry()

	f := LookupAttribute(reg, "myapp.Child", "nonexistent")
	if f != nil {
		t.Errorf("LookupAttribute(Child, nonexistent) = %v, want nil", f)
	}
}

func TestLookupField_Direct(t *testing.T) {
	reg := buildTestRegistry()

	ft := LookupField(reg, "myapp.Base", "name")
	if ft == nil {
		t.Fatal("LookupField(Base, name) = nil, want non-nil")
	}
	if !typresolve.TypesEqual(ft, typresolve.Builtin("str")) {
		t.Errorf("LookupField(Base, name) = %v, want Builtin(str)", ft)
	}
}

func TestLookupField_ViaBaseClass(t *testing.T) {
	reg := buildTestRegistry()

	// Middle inherits field "name" from Base
	ft := LookupField(reg, "myapp.Middle", "name")
	if ft == nil {
		t.Fatal("LookupField(Middle, name) = nil, want non-nil")
	}
	if !typresolve.TypesEqual(ft, typresolve.Builtin("str")) {
		t.Errorf("LookupField(Middle, name) = %v, want Builtin(str)", ft)
	}
}

func TestLookupField_ViaBaseClassChain(t *testing.T) {
	reg := buildTestRegistry()

	// Child -> Middle -> Base: field "name" found 2 levels up
	ft := LookupField(reg, "myapp.Child", "name")
	if ft == nil {
		t.Fatal("LookupField(Child, name) = nil, want non-nil")
	}
	if !typresolve.TypesEqual(ft, typresolve.Builtin("str")) {
		t.Errorf("LookupField(Child, name) = %v, want Builtin(str)", ft)
	}
}

func TestLookupField_NotFound(t *testing.T) {
	reg := buildTestRegistry()

	ft := LookupField(reg, "myapp.Child", "nonexistent")
	if ft != nil {
		t.Errorf("LookupField(Child, nonexistent) = %v, want nil", ft)
	}
}
