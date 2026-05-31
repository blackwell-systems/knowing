package rubyresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestIsBuiltinFunc(t *testing.T) {
	trueNames := []string{"puts", "lambda", "attr_reader", "include", "proc", "require", "extend"}
	for _, name := range trueNames {
		if !IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = false, want true", name)
		}
	}

	falseNames := []string{"my_method", "calculate", "foo", "bar_baz"}
	for _, name := range falseNames {
		if IsBuiltinFunc(name) {
			t.Errorf("IsBuiltinFunc(%q) = true, want false", name)
		}
	}
}

func TestIsBuiltinType(t *testing.T) {
	trueNames := []string{"String", "Array", "Hash", "Enumerable", "Kernel", "Object", "Proc"}
	for _, name := range trueNames {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}

	falseNames := []string{"User", "ApplicationController", "MyClass", "FooBar"}
	for _, name := range falseNames {
		if IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = true, want false", name)
		}
	}
}

func TestResolveBuiltinType(t *testing.T) {
	// Known builtin
	got := ResolveBuiltinType("String")
	if got == nil {
		t.Fatal("ResolveBuiltinType(\"String\") = nil, want Named(\"Ruby::String\")")
	}
	want := typresolve.Named("Ruby::String")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("ResolveBuiltinType(\"String\") = %v, want %v", got, want)
	}

	// Unknown type
	got = ResolveBuiltinType("User")
	if got != nil {
		t.Errorf("ResolveBuiltinType(\"User\") = %v, want nil", got)
	}
}

func TestEvalBuiltinCall_Puts(t *testing.T) {
	got := EvalBuiltinCall("puts", nil, nil, nil)
	want := typresolve.Named("Ruby::NilClass")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalBuiltinCall(\"puts\") = %v, want %v", got, want)
	}
}

func TestEvalBuiltinCall_Lambda(t *testing.T) {
	got := EvalBuiltinCall("lambda", nil, nil, nil)
	want := typresolve.Named("Ruby::Proc")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalBuiltinCall(\"lambda\") = %v, want %v", got, want)
	}
}

func TestEvalBuiltinCall_ArrayConversion(t *testing.T) {
	got := EvalBuiltinCall("Array", nil, nil, nil)
	want := typresolve.Named("Ruby::Array")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalBuiltinCall(\"Array\") = %v, want %v", got, want)
	}
}

func TestEvalBuiltinCall_IsA(t *testing.T) {
	got := EvalBuiltinCall("is_a?", nil, nil, nil)
	want := typresolve.Named("Ruby::TrueClass")
	if !typresolve.TypesEqual(got, want) {
		t.Errorf("EvalBuiltinCall(\"is_a?\") = %v, want %v", got, want)
	}
}

func TestEvalBuiltinCall_Unknown(t *testing.T) {
	got := EvalBuiltinCall("some_random_func", nil, nil, nil)
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("EvalBuiltinCall(\"some_random_func\") should return Unknown, got %v", got)
	}
}

// --- Method lookup tests ---

func TestLookupAttribute_DirectMethod(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp::User.name",
		ReceiverType:  "MyApp::User",
		ShortName:     "name",
	})

	got := LookupAttribute(reg, "MyApp::User", "name")
	if got == nil {
		t.Fatal("LookupAttribute should find direct method 'name'")
	}
	if got.ShortName != "name" {
		t.Errorf("got ShortName=%q, want 'name'", got.ShortName)
	}
}

func TestLookupAttribute_ViaInclude(t *testing.T) {
	reg := typresolve.NewRegistry()

	// Module with a method
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Printable",
		ShortName:     "Printable",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "Printable.to_s",
		ReceiverType:  "Printable",
		ShortName:     "to_s",
	})

	// Class including the module
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
		EmbeddedTypes: []string{"Printable"},
	})

	got := LookupAttribute(reg, "MyApp::User", "to_s")
	if got == nil {
		t.Fatal("LookupAttribute should find method via include")
	}
	if got.ShortName != "to_s" {
		t.Errorf("got ShortName=%q, want 'to_s'", got.ShortName)
	}
}

func TestLookupAttribute_ViaInheritance(t *testing.T) {
	reg := typresolve.NewRegistry()

	// Superclass with a method
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "ActiveRecord::Base",
		ShortName:     "Base",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "ActiveRecord::Base.save",
		ReceiverType:  "ActiveRecord::Base",
		ShortName:     "save",
	})

	// Subclass with superclass in EmbeddedTypes[0]
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
		EmbeddedTypes: []string{"ActiveRecord::Base"},
	})

	got := LookupAttribute(reg, "MyApp::User", "save")
	if got == nil {
		t.Fatal("LookupAttribute should find method via inheritance")
	}
	if got.ShortName != "save" {
		t.Errorf("got ShortName=%q, want 'save'", got.ShortName)
	}
}

func TestLookupAttribute_DepthLimit(t *testing.T) {
	reg := typresolve.NewRegistry()

	// Create a chain of 10 modules, each including the next
	for i := 0; i < 10; i++ {
		name := "M" + string(rune('A'+i))
		var embedded []string
		if i < 9 {
			next := "M" + string(rune('A'+i+1))
			embedded = []string{next}
		}
		reg.AddType(typresolve.RegisteredType{
			QualifiedName: name,
			ShortName:     name,
			EmbeddedTypes: embedded,
		})
	}

	// Add method on the last module (MJ, index 9)
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MJ.deep_method",
		ReceiverType:  "MJ",
		ShortName:     "deep_method",
	})

	// Starting from MA, chain is MA->MB->MC->MD->ME->MF->MG->MH->MI->MJ
	// That's 9 hops; depth limit is 8, so it should NOT be found
	got := LookupAttribute(reg, "MA", "deep_method")
	if got != nil {
		t.Error("LookupAttribute should return nil when depth limit exceeded")
	}

	// But from MB (8 hops) it should also fail (MB=depth0, MC=depth1, ..., MJ=depth8 >= limit)
	got = LookupAttribute(reg, "MB", "deep_method")
	if got != nil {
		t.Error("LookupAttribute from MB should return nil (depth 8 = limit)")
	}

	// From MC (7 hops) it should succeed
	got = LookupAttribute(reg, "MC", "deep_method")
	if got == nil {
		t.Error("LookupAttribute from MC should find deep_method (7 hops within limit)")
	}
}

func TestLookupAttribute_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
	})

	got := LookupAttribute(reg, "MyApp::User", "nonexistent")
	if got != nil {
		t.Error("LookupAttribute should return nil for missing method")
	}

	// Also test with completely unknown type
	got = LookupAttribute(reg, "Unknown::Type", "method")
	if got != nil {
		t.Error("LookupAttribute should return nil for unknown type")
	}
}

func TestLookupAttrAccessor_Reader(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
	})
	// attr_reader :name generates a getter method named "name"
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp::User.name",
		ReceiverType:  "MyApp::User",
		ShortName:     "name",
	})

	got := LookupAttrAccessor(reg, "MyApp::User", "name")
	if got == nil {
		t.Fatal("LookupAttrAccessor should find reader method")
	}
	if got.ShortName != "name" {
		t.Errorf("got ShortName=%q, want 'name'", got.ShortName)
	}
}

func TestLookupAttrAccessor_Writer(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
	})
	// attr_writer :name generates a setter method named "name="
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp::User.name=",
		ReceiverType:  "MyApp::User",
		ShortName:     "name=",
	})

	got := LookupAttrAccessor(reg, "MyApp::User", "name")
	if got == nil {
		t.Fatal("LookupAttrAccessor should find writer method via name=")
	}
	if got.ShortName != "name=" {
		t.Errorf("got ShortName=%q, want 'name='", got.ShortName)
	}
}

func TestResolveNew(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp::User",
		ShortName:     "User",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp::User.initialize",
		ReceiverType:  "MyApp::User",
		ShortName:     "initialize",
		Signature: typresolve.Func(
			[]typresolve.Param{{Name: "name", Type: typresolve.Named("Ruby::String")}},
			nil,
		),
	})

	got := ResolveNew(reg, "MyApp::User")
	if got == nil {
		t.Fatal("ResolveNew should find initialize method")
	}
	if got.ShortName != "initialize" {
		t.Errorf("got ShortName=%q, want 'initialize'", got.ShortName)
	}

	// No initialize method
	got = ResolveNew(reg, "Unknown::Type")
	if got != nil {
		t.Error("ResolveNew should return nil for type without initialize")
	}
}
