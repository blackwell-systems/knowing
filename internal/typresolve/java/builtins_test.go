package javaresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestIsBuiltinFunc(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"toString", true},
		{"hashCode", true},
		{"equals", true},
		{"getClass", true},
		{"notify", true},
		{"notifyAll", true},
		{"wait", true},
		{"clone", true},
		{"finalize", true},
		{"println", true},
		{"processData", false},
		{"getData", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsBuiltinFunc(tt.name); got != tt.want {
			t.Errorf("IsBuiltinFunc(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsBuiltinType(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"int", true},
		{"long", true},
		{"String", true},
		{"Object", true},
		{"Integer", true},
		{"Boolean", true},
		{"void", true},
		{"Throwable", true},
		{"Exception", true},
		{"AutoCloseable", true},
		{"MyService", false},
		{"UserRepository", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsBuiltinType(tt.name); got != tt.want {
			t.Errorf("IsBuiltinType(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestResolveBuiltinType(t *testing.T) {
	// Should return Builtin for known types.
	intType := ResolveBuiltinType("int")
	if intType == nil || intType.Kind != typresolve.KindBuiltin || intType.Name != "int" {
		t.Errorf("ResolveBuiltinType(\"int\") = %v, want Builtin(\"int\")", intType)
	}

	strType := ResolveBuiltinType("String")
	if strType == nil || strType.Kind != typresolve.KindBuiltin || strType.Name != "String" {
		t.Errorf("ResolveBuiltinType(\"String\") = %v, want Builtin(\"String\")", strType)
	}

	// Should return nil for unknown types.
	if got := ResolveBuiltinType("MyService"); got != nil {
		t.Errorf("ResolveBuiltinType(\"MyService\") = %v, want nil", got)
	}
}

func TestLiteralType(t *testing.T) {
	tests := []struct {
		nodeType string
		wantKind typresolve.TypeKind
		wantName string
		wantNil  bool
	}{
		{"decimal_integer_literal", typresolve.KindBuiltin, "int", false},
		{"hex_integer_literal", typresolve.KindBuiltin, "int", false},
		{"octal_integer_literal", typresolve.KindBuiltin, "int", false},
		{"binary_integer_literal", typresolve.KindBuiltin, "int", false},
		{"decimal_floating_point_literal", typresolve.KindBuiltin, "double", false},
		{"hex_floating_point_literal", typresolve.KindBuiltin, "double", false},
		{"character_literal", typresolve.KindBuiltin, "char", false},
		{"string_literal", typresolve.KindBuiltin, "String", false},
		{"true", typresolve.KindBuiltin, "boolean", false},
		{"false", typresolve.KindBuiltin, "boolean", false},
		{"null_literal", typresolve.KindUnknown, "", false},
		{"some_other", 0, "", true},
	}
	for _, tt := range tests {
		got := LiteralType(tt.nodeType)
		if tt.wantNil {
			if got != nil {
				t.Errorf("LiteralType(%q) = %v, want nil", tt.nodeType, got)
			}
			continue
		}
		if got == nil {
			t.Errorf("LiteralType(%q) = nil, want non-nil", tt.nodeType)
			continue
		}
		if got.Kind != tt.wantKind {
			t.Errorf("LiteralType(%q).Kind = %v, want %v", tt.nodeType, got.Kind, tt.wantKind)
		}
		if tt.wantName != "" && got.Name != tt.wantName {
			t.Errorf("LiteralType(%q).Name = %q, want %q", tt.nodeType, got.Name, tt.wantName)
		}
	}
}

func TestLookupFieldOrMethod(t *testing.T) {
	reg := typresolve.NewRegistry()

	// Register: Animal (base) -> Dog (subclass) -> GoldenRetriever (sub-subclass)
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Animal",
		ShortName:     "Animal",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "com.example.Animal.speak",
		ShortName:     "speak",
		ReceiverType:  "com.example.Animal",
		MinParams:     -1,
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "com.example.Animal.eat",
		ShortName:     "eat",
		ReceiverType:  "com.example.Animal",
		MinParams:     -1,
	})

	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Dog",
		ShortName:     "Dog",
		EmbeddedTypes: []string{"com.example.Animal"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "com.example.Dog.fetch",
		ShortName:     "fetch",
		ReceiverType:  "com.example.Dog",
		MinParams:     -1,
	})

	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.GoldenRetriever",
		ShortName:     "GoldenRetriever",
		EmbeddedTypes: []string{"com.example.Dog"},
	})

	// Interface: Swimmable with swim() method
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Swimmable",
		ShortName:     "Swimmable",
		IsInterface:   true,
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "com.example.Swimmable.swim",
		ShortName:     "swim",
		ReceiverType:  "com.example.Swimmable",
		MinParams:     -1,
	})

	// Duck implements Swimmable
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Duck",
		ShortName:     "Duck",
		EmbeddedTypes: []string{"com.example.Animal", "com.example.Swimmable"},
	})

	t.Run("direct method found", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.Animal", "speak")
		if f == nil {
			t.Fatal("expected to find speak on Animal")
		}
		if f.QualifiedName != "com.example.Animal.speak" {
			t.Errorf("got %q, want com.example.Animal.speak", f.QualifiedName)
		}
	})

	t.Run("method found via superclass (1 level)", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.Dog", "speak")
		if f == nil {
			t.Fatal("expected to find speak on Dog via Animal")
		}
		if f.QualifiedName != "com.example.Animal.speak" {
			t.Errorf("got %q, want com.example.Animal.speak", f.QualifiedName)
		}
	})

	t.Run("method found via superclass chain (2 levels)", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.GoldenRetriever", "speak")
		if f == nil {
			t.Fatal("expected to find speak on GoldenRetriever via Dog -> Animal")
		}
		if f.QualifiedName != "com.example.Animal.speak" {
			t.Errorf("got %q, want com.example.Animal.speak", f.QualifiedName)
		}
	})

	t.Run("method found via interface", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.Duck", "swim")
		if f == nil {
			t.Fatal("expected to find swim on Duck via Swimmable")
		}
		if f.QualifiedName != "com.example.Swimmable.swim" {
			t.Errorf("got %q, want com.example.Swimmable.swim", f.QualifiedName)
		}
	})

	t.Run("method not found", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.Animal", "nonExistent")
		if f != nil {
			t.Errorf("expected nil for nonExistent method, got %v", f)
		}
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		f := LookupFieldOrMethod(reg, "com.example.Unknown", "speak")
		if f != nil {
			t.Errorf("expected nil for unknown type, got %v", f)
		}
	})
}

func TestLookupField(t *testing.T) {
	reg := typresolve.NewRegistry()

	// Animal with name field
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Animal",
		ShortName:     "Animal",
		Fields: []typresolve.Field{
			{Name: "name", Type: typresolve.Builtin("String")},
			{Name: "age", Type: typresolve.Builtin("int")},
		},
	})

	// Dog extends Animal, adds breed field
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "com.example.Dog",
		ShortName:     "Dog",
		EmbeddedTypes: []string{"com.example.Animal"},
		Fields: []typresolve.Field{
			{Name: "breed", Type: typresolve.Builtin("String")},
		},
	})

	t.Run("direct field found", func(t *testing.T) {
		ft := LookupField(reg, "com.example.Dog", "breed")
		if ft == nil {
			t.Fatal("expected to find breed on Dog")
		}
		if ft.Kind != typresolve.KindBuiltin || ft.Name != "String" {
			t.Errorf("got %v, want Builtin(String)", ft)
		}
	})

	t.Run("field found via superclass", func(t *testing.T) {
		ft := LookupField(reg, "com.example.Dog", "name")
		if ft == nil {
			t.Fatal("expected to find name on Dog via Animal")
		}
		if ft.Kind != typresolve.KindBuiltin || ft.Name != "String" {
			t.Errorf("got %v, want Builtin(String)", ft)
		}
	})

	t.Run("field not found", func(t *testing.T) {
		ft := LookupField(reg, "com.example.Dog", "nonExistent")
		if ft != nil {
			t.Errorf("expected nil for nonExistent field, got %v", ft)
		}
	})

	t.Run("unknown type returns nil", func(t *testing.T) {
		ft := LookupField(reg, "com.example.Unknown", "name")
		if ft != nil {
			t.Errorf("expected nil for unknown type, got %v", ft)
		}
	})
}
