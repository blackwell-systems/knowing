package typresolve

import "testing"

func TestScopeNewWithNilParent(t *testing.T) {
	s := NewScope(nil)
	if s == nil {
		t.Fatal("NewScope(nil) returned nil")
	}
	if s.Parent() != nil {
		t.Fatal("root scope parent should be nil")
	}
}

func TestScopeBindAndLookup(t *testing.T) {
	s := NewScope(nil)
	typ := &Type{Kind: KindNamed, Name: "int"}
	s.Bind("x", typ)

	got := s.Lookup("x")
	if got != typ {
		t.Fatalf("Lookup(x) = %v, want %v", got, typ)
	}
}

func TestScopeLookupUnbound(t *testing.T) {
	s := NewScope(nil)
	if s.Lookup("missing") != nil {
		t.Fatal("Lookup of unbound name should return nil")
	}
}

func TestScopeInnerShadowsOuter(t *testing.T) {
	outer := NewScope(nil)
	outerType := &Type{Kind: KindNamed, Name: "string"}
	outer.Bind("x", outerType)

	inner := NewScope(outer)
	innerType := &Type{Kind: KindNamed, Name: "int"}
	inner.Bind("x", innerType)

	got := inner.Lookup("x")
	if got != innerType {
		t.Fatalf("inner.Lookup(x) should return inner binding, got %v", got)
	}

	// outer is unchanged
	got = outer.Lookup("x")
	if got != outerType {
		t.Fatalf("outer.Lookup(x) should still return outer binding, got %v", got)
	}
}

func TestScopeLookupWalksToParent(t *testing.T) {
	outer := NewScope(nil)
	typ := &Type{Kind: KindBuiltin, Name: "bool"}
	outer.Bind("flag", typ)

	inner := NewScope(outer)
	// inner does not bind "flag"

	got := inner.Lookup("flag")
	if got != typ {
		t.Fatalf("inner.Lookup(flag) should find parent binding, got %v", got)
	}
}

func TestScopeRebindOverwrites(t *testing.T) {
	s := NewScope(nil)
	first := &Type{Kind: KindNamed, Name: "A"}
	second := &Type{Kind: KindNamed, Name: "B"}

	s.Bind("v", first)
	s.Bind("v", second)

	got := s.Lookup("v")
	if got != second {
		t.Fatalf("rebind should overwrite: got %v, want %v", got, second)
	}
}

func TestScopeDeepNesting(t *testing.T) {
	// Build a chain of 7 levels, bind at root, lookup from deepest
	root := NewScope(nil)
	rootType := &Type{Kind: KindBuiltin, Name: "error"}
	root.Bind("err", rootType)

	current := root
	for i := 0; i < 6; i++ {
		current = NewScope(current)
	}

	got := current.Lookup("err")
	if got != rootType {
		t.Fatalf("deep Lookup should find root binding, got %v", got)
	}

	// Verify parent chain length
	depth := 0
	for s := current; s != nil; s = s.Parent() {
		depth++
	}
	if depth != 7 {
		t.Fatalf("expected depth 7, got %d", depth)
	}
}

func TestScopeDeepNestingShadowing(t *testing.T) {
	// Bind "x" at multiple levels, verify closest wins
	s0 := NewScope(nil)
	s0.Bind("x", &Type{Kind: KindNamed, Name: "L0"})

	s1 := NewScope(s0)
	s2 := NewScope(s1)
	midType := &Type{Kind: KindNamed, Name: "L2"}
	s2.Bind("x", midType)

	s3 := NewScope(s2)
	s4 := NewScope(s3)

	got := s4.Lookup("x")
	if got != midType {
		t.Fatalf("should find L2 binding, got %v", got)
	}
}

func TestScopeMultipleBindings(t *testing.T) {
	s := NewScope(nil)
	s.Bind("a", &Type{Kind: KindBuiltin, Name: "int"})
	s.Bind("b", &Type{Kind: KindBuiltin, Name: "string"})
	s.Bind("c", &Type{Kind: KindBuiltin, Name: "bool"})

	if s.Lookup("a") == nil || s.Lookup("a").Name != "int" {
		t.Fatal("binding a missing or wrong")
	}
	if s.Lookup("b") == nil || s.Lookup("b").Name != "string" {
		t.Fatal("binding b missing or wrong")
	}
	if s.Lookup("c") == nil || s.Lookup("c").Name != "bool" {
		t.Fatal("binding c missing or wrong")
	}
}
