package typresolve

import "testing"

func TestRegistryAddFuncLookupFunc(t *testing.T) {
	r := NewRegistry()
	f := RegisteredFunc{
		QualifiedName: "fmt.Println",
		ShortName:     "Println",
		Signature:     &Type{Kind: KindFunc},
		MinParams:     1,
	}
	r.AddFunc(f)

	got := r.LookupFunc("fmt.Println")
	if got == nil {
		t.Fatal("expected non-nil result for LookupFunc")
	}
	if got.QualifiedName != "fmt.Println" {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, "fmt.Println")
	}
	if got.ShortName != "Println" {
		t.Errorf("ShortName = %q, want %q", got.ShortName, "Println")
	}
	if got.MinParams != 1 {
		t.Errorf("MinParams = %d, want 1", got.MinParams)
	}
}

func TestRegistryAddTypeLookupType(t *testing.T) {
	r := NewRegistry()
	typ := RegisteredType{
		QualifiedName: "http.Request",
		ShortName:     "Request",
		Fields: []Field{
			{Name: "Method", Type: &Type{Kind: KindBuiltin, Name: "string"}},
		},
		IsInterface: false,
	}
	r.AddType(typ)

	got := r.LookupType("http.Request")
	if got == nil {
		t.Fatal("expected non-nil result for LookupType")
	}
	if got.QualifiedName != "http.Request" {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, "http.Request")
	}
	if len(got.Fields) != 1 {
		t.Fatalf("len(Fields) = %d, want 1", len(got.Fields))
	}
	if got.Fields[0].Name != "Method" {
		t.Errorf("Fields[0].Name = %q, want %q", got.Fields[0].Name, "Method")
	}
}

func TestRegistryLookupMethod(t *testing.T) {
	r := NewRegistry()
	m := RegisteredFunc{
		QualifiedName: "http.Request.URL",
		ReceiverType:  "http.Request",
		ShortName:     "URL",
		Signature:     &Type{Kind: KindFunc},
		MinParams:     0,
	}
	r.AddFunc(m)

	// Should be findable via LookupMethod.
	got := r.LookupMethod("http.Request", "URL")
	if got == nil {
		t.Fatal("expected non-nil result for LookupMethod")
	}
	if got.QualifiedName != "http.Request.URL" {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, "http.Request.URL")
	}

	// Should also be findable via LookupFunc by qualified name.
	got2 := r.LookupFunc("http.Request.URL")
	if got2 == nil {
		t.Fatal("expected non-nil result for LookupFunc on method")
	}
}

func TestRegistryLookupSymbol(t *testing.T) {
	r := NewRegistry()
	f := RegisteredFunc{
		QualifiedName: "fmt.Sprintf",
		ShortName:     "Sprintf",
		Signature:     &Type{Kind: KindFunc},
		MinParams:     1,
	}
	r.AddFunc(f)

	got := r.LookupSymbol("fmt", "Sprintf")
	if got == nil {
		t.Fatal("expected non-nil result for LookupSymbol")
	}
	if got.QualifiedName != "fmt.Sprintf" {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, "fmt.Sprintf")
	}
}

func TestRegistryFallbackChaining(t *testing.T) {
	parent := NewRegistry()
	parent.AddFunc(RegisteredFunc{
		QualifiedName: "builtin.len",
		ShortName:     "len",
		MinParams:     1,
	})
	parent.AddType(RegisteredType{
		QualifiedName: "builtin.error",
		ShortName:     "error",
		IsInterface:   true,
	})

	child := NewRegistry()
	child.SetFallback(parent)

	// Item in fallback found.
	if got := child.LookupFunc("builtin.len"); got == nil {
		t.Error("expected fallback lookup to find builtin.len")
	}
	if got := child.LookupType("builtin.error"); got == nil {
		t.Error("expected fallback lookup to find builtin.error")
	}

	// Local overrides fallback.
	child.AddFunc(RegisteredFunc{
		QualifiedName: "builtin.len",
		ShortName:     "len",
		MinParams:     42, // different from parent
	})
	got := child.LookupFunc("builtin.len")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.MinParams != 42 {
		t.Errorf("MinParams = %d, want 42 (local should override fallback)", got.MinParams)
	}
}

func TestRegistryFallbackMethod(t *testing.T) {
	parent := NewRegistry()
	parent.AddFunc(RegisteredFunc{
		QualifiedName: "io.Reader.Read",
		ReceiverType:  "io.Reader",
		ShortName:     "Read",
		MinParams:     1,
	})

	child := NewRegistry()
	child.SetFallback(parent)

	got := child.LookupMethod("io.Reader", "Read")
	if got == nil {
		t.Fatal("expected fallback to find method")
	}
	if got.QualifiedName != "io.Reader.Read" {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, "io.Reader.Read")
	}
}

func TestRegistryFallbackSymbol(t *testing.T) {
	parent := NewRegistry()
	parent.AddFunc(RegisteredFunc{
		QualifiedName: "os.Exit",
		ShortName:     "Exit",
		MinParams:     1,
	})

	child := NewRegistry()
	child.SetFallback(parent)

	got := child.LookupSymbol("os", "Exit")
	if got == nil {
		t.Fatal("expected fallback to find symbol")
	}
}

func TestRegistryResolveAlias(t *testing.T) {
	r := NewRegistry()

	// Single alias: MyString -> string
	r.AddType(RegisteredType{
		QualifiedName: "pkg.MyString",
		ShortName:     "MyString",
		AliasOf:       "builtin.string",
	})
	r.AddType(RegisteredType{
		QualifiedName: "builtin.string",
		ShortName:     "string",
	})

	got := r.ResolveAlias("pkg.MyString")
	if got == nil {
		t.Fatal("expected non-nil for single alias resolution")
	}
	if got.QualifiedName != "builtin.string" {
		t.Errorf("resolved to %q, want %q", got.QualifiedName, "builtin.string")
	}

	// Chain of 2: A -> B -> C
	r.AddType(RegisteredType{
		QualifiedName: "pkg.A",
		ShortName:     "A",
		AliasOf:       "pkg.B",
	})
	r.AddType(RegisteredType{
		QualifiedName: "pkg.B",
		ShortName:     "B",
		AliasOf:       "pkg.C",
	})
	r.AddType(RegisteredType{
		QualifiedName: "pkg.C",
		ShortName:     "C",
	})

	got = r.ResolveAlias("pkg.A")
	if got == nil {
		t.Fatal("expected non-nil for chain resolution")
	}
	if got.QualifiedName != "pkg.C" {
		t.Errorf("resolved to %q, want %q", got.QualifiedName, "pkg.C")
	}
}

func TestRegistryResolveAliasCycleDetection(t *testing.T) {
	r := NewRegistry()

	// Create a cycle: X -> Y -> X
	r.AddType(RegisteredType{
		QualifiedName: "pkg.X",
		ShortName:     "X",
		AliasOf:       "pkg.Y",
	})
	r.AddType(RegisteredType{
		QualifiedName: "pkg.Y",
		ShortName:     "Y",
		AliasOf:       "pkg.X",
	})

	got := r.ResolveAlias("pkg.X")
	if got != nil {
		t.Errorf("expected nil for cyclic alias, got %q", got.QualifiedName)
	}
}

func TestRegistryResolveAliasNotFound(t *testing.T) {
	r := NewRegistry()

	// Alias pointing to nonexistent type.
	r.AddType(RegisteredType{
		QualifiedName: "pkg.Missing",
		ShortName:     "Missing",
		AliasOf:       "pkg.DoesNotExist",
	})

	got := r.ResolveAlias("pkg.Missing")
	if got != nil {
		t.Errorf("expected nil for missing alias target, got %v", got)
	}

	// Completely unknown type.
	got = r.ResolveAlias("pkg.Unknown")
	if got != nil {
		t.Errorf("expected nil for unknown type, got %v", got)
	}
}

func TestRegistryEmptyReturnsNil(t *testing.T) {
	r := NewRegistry()

	if got := r.LookupFunc("anything"); got != nil {
		t.Error("expected nil from empty registry LookupFunc")
	}
	if got := r.LookupType("anything"); got != nil {
		t.Error("expected nil from empty registry LookupType")
	}
	if got := r.LookupMethod("recv", "method"); got != nil {
		t.Error("expected nil from empty registry LookupMethod")
	}
	if got := r.LookupSymbol("pkg", "name"); got != nil {
		t.Error("expected nil from empty registry LookupSymbol")
	}
	if got := r.ResolveAlias("anything"); got != nil {
		t.Error("expected nil from empty registry ResolveAlias")
	}
}

func TestRegistryFuncCountTypeCount(t *testing.T) {
	r := NewRegistry()

	if r.FuncCount() != 0 {
		t.Errorf("FuncCount() = %d, want 0", r.FuncCount())
	}
	if r.TypeCount() != 0 {
		t.Errorf("TypeCount() = %d, want 0", r.TypeCount())
	}

	r.AddFunc(RegisteredFunc{QualifiedName: "a.F1", ShortName: "F1"})
	r.AddFunc(RegisteredFunc{QualifiedName: "a.F2", ShortName: "F2"})
	r.AddType(RegisteredType{QualifiedName: "a.T1", ShortName: "T1"})

	if r.FuncCount() != 2 {
		t.Errorf("FuncCount() = %d, want 2", r.FuncCount())
	}
	if r.TypeCount() != 1 {
		t.Errorf("TypeCount() = %d, want 1", r.TypeCount())
	}

	// Counts are local only, not including fallback.
	parent := NewRegistry()
	parent.AddFunc(RegisteredFunc{QualifiedName: "b.F3", ShortName: "F3"})
	parent.AddType(RegisteredType{QualifiedName: "b.T2", ShortName: "T2"})
	r.SetFallback(parent)

	if r.FuncCount() != 2 {
		t.Errorf("FuncCount() with fallback = %d, want 2 (local only)", r.FuncCount())
	}
	if r.TypeCount() != 1 {
		t.Errorf("TypeCount() with fallback = %d, want 1 (local only)", r.TypeCount())
	}
}

func TestRegistryResolveNonAlias(t *testing.T) {
	r := NewRegistry()
	r.AddType(RegisteredType{
		QualifiedName: "pkg.Concrete",
		ShortName:     "Concrete",
	})

	got := r.ResolveAlias("pkg.Concrete")
	if got == nil {
		t.Fatal("expected non-nil for non-alias type")
	}
	if got.QualifiedName != "pkg.Concrete" {
		t.Errorf("resolved to %q, want %q", got.QualifiedName, "pkg.Concrete")
	}
}
