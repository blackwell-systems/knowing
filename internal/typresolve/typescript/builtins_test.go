package tsresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

func TestIsBuiltinType(t *testing.T) {
	trueCases := []string{"string", "number", "boolean", "any", "Promise", "Array", "Map", "Set", "Date", "console"}
	for _, name := range trueCases {
		if !IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = false, want true", name)
		}
	}

	falseCases := []string{"MyClass", "express", "React", "foo", ""}
	for _, name := range falseCases {
		if IsBuiltinType(name) {
			t.Errorf("IsBuiltinType(%q) = true, want false", name)
		}
	}
}

func TestResolveBuiltinType(t *testing.T) {
	// Primitives should return Builtin types.
	for _, name := range []string{"string", "number", "boolean", "void", "any", "unknown", "never"} {
		got := ResolveBuiltinType(name)
		if got == nil {
			t.Fatalf("ResolveBuiltinType(%q) = nil, want Builtin", name)
		}
		if got.Kind != typresolve.KindBuiltin || got.Name != name {
			t.Errorf("ResolveBuiltinType(%q) = %v/%v, want Builtin/%v", name, got.Kind, got.Name, name)
		}
	}

	// Non-primitive builtins should return nil.
	for _, name := range []string{"Array", "Promise", "Map", "String", "console"} {
		if got := ResolveBuiltinType(name); got != nil {
			t.Errorf("ResolveBuiltinType(%q) = %v, want nil", name, got)
		}
	}
}

func TestBuiltinWrapperClass(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"string", "String"},
		{"number", "Number"},
		{"boolean", "Boolean"},
		{"bigint", "BigInt"},
		{"symbol", "Symbol"},
		{"foo", ""},
		{"Array", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := BuiltinWrapperClass(tc.in); got != tc.want {
			t.Errorf("BuiltinWrapperClass(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLiteralType(t *testing.T) {
	cases := []struct {
		nodeType string
		wantKind typresolve.TypeKind
		wantName string
	}{
		{"string", typresolve.KindBuiltin, "string"},
		{"template_string", typresolve.KindBuiltin, "string"},
		{"number", typresolve.KindBuiltin, "number"},
		{"true", typresolve.KindBuiltin, "boolean"},
		{"false", typresolve.KindBuiltin, "boolean"},
		{"null", typresolve.KindBuiltin, "null"},
		{"undefined", typresolve.KindBuiltin, "undefined"},
		{"regex", typresolve.KindNamed, "RegExp"},
	}
	for _, tc := range cases {
		got := LiteralType(tc.nodeType)
		if got == nil {
			t.Fatalf("LiteralType(%q) = nil, want %v/%v", tc.nodeType, tc.wantKind, tc.wantName)
		}
		if got.Kind != tc.wantKind || got.Name != tc.wantName {
			t.Errorf("LiteralType(%q) = %v/%v, want %v/%v", tc.nodeType, got.Kind, got.Name, tc.wantKind, tc.wantName)
		}
	}

	// Non-literal should return nil.
	if got := LiteralType("foo"); got != nil {
		t.Errorf("LiteralType(%q) = %v, want nil", "foo", got)
	}
	if got := LiteralType("identifier"); got != nil {
		t.Errorf("LiteralType(%q) = %v, want nil", "identifier", got)
	}
}

func TestUnwrapPromise(t *testing.T) {
	// Promise<string> -> string
	inner := typresolve.Builtin("string")
	promise := &typresolve.Type{Kind: typresolve.KindNamed, Name: "Promise", Elem: inner}
	got := UnwrapPromise(promise)
	if got != inner {
		t.Errorf("UnwrapPromise(Promise<string>) = %v, want Builtin(string)", got)
	}

	// Named("Promise") without Elem -> Unknown
	barePromise := typresolve.Named("Promise")
	got = UnwrapPromise(barePromise)
	if got == nil || got.Kind != typresolve.KindUnknown {
		t.Errorf("UnwrapPromise(bare Promise) = %v, want Unknown", got)
	}

	// Non-Promise type returned unchanged.
	myType := typresolve.Named("MyType")
	got = UnwrapPromise(myType)
	if got != myType {
		t.Errorf("UnwrapPromise(MyType) returned different pointer, want same")
	}

	// nil input.
	got = UnwrapPromise(nil)
	if got != nil {
		t.Errorf("UnwrapPromise(nil) = %v, want nil", got)
	}
}

func TestIsPassthroughUtility(t *testing.T) {
	trueCases := []string{"Partial", "Required", "Readonly", "NonNullable", "Pick", "Omit", "Awaited"}
	for _, name := range trueCases {
		if !IsPassthroughUtility(name) {
			t.Errorf("IsPassthroughUtility(%q) = false, want true", name)
		}
	}

	falseCases := []string{"Promise", "Array", "Map", "string", "Record"}
	for _, name := range falseCases {
		if IsPassthroughUtility(name) {
			t.Errorf("IsPassthroughUtility(%q) = true, want false", name)
		}
	}
}

func TestLookupMember_Direct(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyClass",
		ShortName:     "MyClass",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyClass.doStuff",
		ReceiverType:  "MyClass",
		ShortName:     "doStuff",
		Signature:     typresolve.Func(nil, nil),
	})

	got := LookupMember(reg, "MyClass", "doStuff")
	if got == nil {
		t.Fatal("LookupMember direct: got nil, want doStuff")
	}
	if got.ShortName != "doStuff" {
		t.Errorf("LookupMember direct: got %q, want doStuff", got.ShortName)
	}
}

func TestLookupMember_PrototypeChain1Level(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Base",
		ShortName:     "Base",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "Base.render",
		ReceiverType:  "Base",
		ShortName:     "render",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Child",
		ShortName:     "Child",
		EmbeddedTypes: []string{"Base"},
	})

	got := LookupMember(reg, "Child", "render")
	if got == nil {
		t.Fatal("LookupMember 1-level prototype: got nil, want render")
	}
	if got.ShortName != "render" {
		t.Errorf("LookupMember 1-level prototype: got %q, want render", got.ShortName)
	}
}

func TestLookupMember_PrototypeChain2Levels(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "GrandBase",
		ShortName:     "GrandBase",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "GrandBase.init",
		ReceiverType:  "GrandBase",
		ShortName:     "init",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Parent",
		ShortName:     "Parent",
		EmbeddedTypes: []string{"GrandBase"},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Child",
		ShortName:     "Child",
		EmbeddedTypes: []string{"Parent"},
	})

	got := LookupMember(reg, "Child", "init")
	if got == nil {
		t.Fatal("LookupMember 2-level prototype: got nil, want init")
	}
	if got.ShortName != "init" {
		t.Errorf("LookupMember 2-level prototype: got %q, want init", got.ShortName)
	}
}

func TestLookupMember_BuiltinWrapper(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "String",
		ShortName:     "String",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "String.toUpperCase",
		ReceiverType:  "String",
		ShortName:     "toUpperCase",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})

	got := LookupMember(reg, "string", "toUpperCase")
	if got == nil {
		t.Fatal("LookupMember builtin wrapper: got nil, want toUpperCase")
	}
	if got.ShortName != "toUpperCase" {
		t.Errorf("LookupMember builtin wrapper: got %q, want toUpperCase", got.ShortName)
	}
}

func TestLookupMember_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	got := LookupMember(reg, "Unknown", "method")
	if got != nil {
		t.Errorf("LookupMember not found: got %v, want nil", got)
	}
}

func TestLookupField_Direct(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "User",
		ShortName:     "User",
		Fields: []typresolve.Field{
			{Name: "name", Type: typresolve.Builtin("string")},
			{Name: "age", Type: typresolve.Builtin("number")},
		},
	})

	got := LookupField(reg, "User", "name")
	if got == nil {
		t.Fatal("LookupField direct: got nil, want string")
	}
	if got.Kind != typresolve.KindBuiltin || got.Name != "string" {
		t.Errorf("LookupField direct: got %v/%v, want Builtin/string", got.Kind, got.Name)
	}
}

func TestLookupField_PrototypeChain(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Base",
		ShortName:     "Base",
		Fields: []typresolve.Field{
			{Name: "id", Type: typresolve.Builtin("number")},
		},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Derived",
		ShortName:     "Derived",
		EmbeddedTypes: []string{"Base"},
	})

	got := LookupField(reg, "Derived", "id")
	if got == nil {
		t.Fatal("LookupField prototype: got nil, want number")
	}
	if got.Kind != typresolve.KindBuiltin || got.Name != "number" {
		t.Errorf("LookupField prototype: got %v/%v, want Builtin/number", got.Kind, got.Name)
	}
}

func TestLookupField_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Empty",
		ShortName:     "Empty",
	})
	got := LookupField(reg, "Empty", "missing")
	if got != nil {
		t.Errorf("LookupField not found: got %v, want nil", got)
	}
}

func TestLookupMemberType_FieldFirst(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Config",
		ShortName:     "Config",
		Fields: []typresolve.Field{
			{Name: "port", Type: typresolve.Builtin("number")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "Config.port",
		ReceiverType:  "Config",
		ShortName:     "port",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("number")}),
	})

	// Field should take precedence over method.
	got := LookupMemberType(reg, "Config", "port")
	if got == nil {
		t.Fatal("LookupMemberType: got nil")
	}
	if got.Kind != typresolve.KindBuiltin {
		t.Errorf("LookupMemberType: got kind %v, want Builtin (field takes precedence)", got.Kind)
	}
}

func TestLookupMemberType_MethodFallback(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Service",
		ShortName:     "Service",
	})
	sig := typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("void")})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "Service.start",
		ReceiverType:  "Service",
		ShortName:     "start",
		Signature:     sig,
	})

	got := LookupMemberType(reg, "Service", "start")
	if got == nil {
		t.Fatal("LookupMemberType method fallback: got nil")
	}
	if got.Kind != typresolve.KindFunc {
		t.Errorf("LookupMemberType method fallback: got kind %v, want Func", got.Kind)
	}
}

func TestLookupField_Alias(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Original",
		ShortName:     "Original",
		Fields: []typresolve.Field{
			{Name: "value", Type: typresolve.Builtin("string")},
		},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Alias",
		ShortName:     "Alias",
		AliasOf:       "Original",
	})

	got := LookupField(reg, "Alias", "value")
	if got == nil {
		t.Fatal("LookupField alias: got nil, want string")
	}
	if got.Kind != typresolve.KindBuiltin || got.Name != "string" {
		t.Errorf("LookupField alias: got %v/%v, want Builtin/string", got.Kind, got.Name)
	}
}

func TestLookupMember_Alias(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "Real",
		ShortName:     "Real",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "Real.run",
		ReceiverType:  "Real",
		ShortName:     "run",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "AliasType",
		ShortName:     "AliasType",
		AliasOf:       "Real",
	})

	got := LookupMember(reg, "AliasType", "run")
	if got == nil {
		t.Fatal("LookupMember alias: got nil, want run")
	}
	if got.ShortName != "run" {
		t.Errorf("LookupMember alias: got %q, want run", got.ShortName)
	}
}
