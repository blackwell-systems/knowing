package typresolve

import "testing"

func TestTypeKindString(t *testing.T) {
	tests := []struct {
		kind TypeKind
		want string
	}{
		{KindUnknown, "Unknown"},
		{KindNamed, "Named"},
		{KindPointer, "Pointer"},
		{KindSlice, "Slice"},
		{KindMap, "Map"},
		{KindChannel, "Channel"},
		{KindFunc, "Func"},
		{KindInterface, "Interface"},
		{KindStruct, "Struct"},
		{KindBuiltin, "Builtin"},
		{KindTuple, "Tuple"},
		{KindTypeParam, "TypeParam"},
		{KindArray, "Array"},
		{KindOptional, "Optional"},
		{KindReference, "Reference"},
		{KindAlias, "Alias"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("TypeKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestConstructors(t *testing.T) {
	t.Run("Unknown", func(t *testing.T) {
		ty := Unknown()
		if ty.Kind != KindUnknown {
			t.Errorf("Unknown().Kind = %v, want KindUnknown", ty.Kind)
		}
	})

	t.Run("Named", func(t *testing.T) {
		ty := Named("http.Request")
		if ty.Kind != KindNamed || ty.Name != "http.Request" {
			t.Errorf("Named() = {Kind: %v, Name: %q}, want {KindNamed, \"http.Request\"}", ty.Kind, ty.Name)
		}
	})

	t.Run("Pointer", func(t *testing.T) {
		elem := Named("Foo")
		ty := Pointer(elem)
		if ty.Kind != KindPointer || ty.Elem != elem {
			t.Errorf("Pointer() incorrect kind or elem")
		}
	})

	t.Run("Slice", func(t *testing.T) {
		elem := Builtin("int")
		ty := Slice(elem)
		if ty.Kind != KindSlice || ty.Elem != elem {
			t.Errorf("Slice() incorrect kind or elem")
		}
	})

	t.Run("Map", func(t *testing.T) {
		k := Builtin("string")
		v := Named("Foo")
		ty := Map(k, v)
		if ty.Kind != KindMap || ty.Key != k || ty.Value != v {
			t.Errorf("Map() incorrect kind, key, or value")
		}
	})

	t.Run("Channel", func(t *testing.T) {
		elem := Builtin("int")
		ty := Channel(elem, ChanSend)
		if ty.Kind != KindChannel || ty.Elem != elem || ty.ChanDir != ChanSend {
			t.Errorf("Channel() incorrect kind, elem, or dir")
		}
	})

	t.Run("Func", func(t *testing.T) {
		params := []Param{{Name: "x", Type: Builtin("int")}}
		returns := []*Type{Builtin("error")}
		ty := Func(params, returns)
		if ty.Kind != KindFunc || len(ty.Params) != 1 || len(ty.Returns) != 1 {
			t.Errorf("Func() incorrect kind, params, or returns")
		}
	})

	t.Run("Builtin", func(t *testing.T) {
		ty := Builtin("string")
		if ty.Kind != KindBuiltin || ty.Name != "string" {
			t.Errorf("Builtin() incorrect kind or name")
		}
	})

	t.Run("Tuple", func(t *testing.T) {
		elems := []*Type{Builtin("int"), Builtin("error")}
		ty := Tuple(elems)
		if ty.Kind != KindTuple || len(ty.Elements) != 2 {
			t.Errorf("Tuple() incorrect kind or elements")
		}
	})

	t.Run("TypeParamType", func(t *testing.T) {
		ty := TypeParamType("T")
		if ty.Kind != KindTypeParam || ty.Name != "T" {
			t.Errorf("TypeParamType() incorrect kind or name")
		}
	})

	t.Run("Array", func(t *testing.T) {
		elem := Builtin("byte")
		ty := Array(elem)
		if ty.Kind != KindArray || ty.Elem != elem {
			t.Errorf("Array() incorrect kind or elem")
		}
	})

	t.Run("Optional", func(t *testing.T) {
		elem := Named("Foo")
		ty := Optional(elem)
		if ty.Kind != KindOptional || ty.Elem != elem {
			t.Errorf("Optional() incorrect kind or elem")
		}
	})

	t.Run("Ref", func(t *testing.T) {
		elem := Named("Bar")
		ty := Ref(elem)
		if ty.Kind != KindReference || ty.Elem != elem {
			t.Errorf("Ref() incorrect kind or elem")
		}
	})

	t.Run("Alias", func(t *testing.T) {
		underlying := Builtin("int")
		ty := Alias("MyInt", underlying)
		if ty.Kind != KindAlias || ty.Name != "MyInt" || ty.Underlying != underlying {
			t.Errorf("Alias() incorrect kind, name, or underlying")
		}
	})

	t.Run("Interface", func(t *testing.T) {
		methods := []Method{
			{Name: "Read", Signature: Func([]Param{{Name: "p", Type: Slice(Builtin("byte"))}}, []*Type{Builtin("int"), Builtin("error")})},
		}
		ty := Interface(methods)
		if ty.Kind != KindInterface || len(ty.Methods) != 1 {
			t.Errorf("Interface() incorrect kind or methods")
		}
	})

	t.Run("Struct", func(t *testing.T) {
		fields := []Field{
			{Name: "ID", Type: Builtin("int")},
			{Name: "Name", Type: Builtin("string")},
		}
		ty := Struct(fields)
		if ty.Kind != KindStruct || len(ty.Fields) != 2 {
			t.Errorf("Struct() incorrect kind or fields")
		}
	})
}

func TestTypesEqual(t *testing.T) {
	t.Run("both nil", func(t *testing.T) {
		if !TypesEqual(nil, nil) {
			t.Error("TypesEqual(nil, nil) should be true")
		}
	})

	t.Run("one nil", func(t *testing.T) {
		if TypesEqual(Named("Foo"), nil) {
			t.Error("TypesEqual(Named, nil) should be false")
		}
		if TypesEqual(nil, Named("Foo")) {
			t.Error("TypesEqual(nil, Named) should be false")
		}
	})

	t.Run("same simple type", func(t *testing.T) {
		a := Named("http.Request")
		b := Named("http.Request")
		if !TypesEqual(a, b) {
			t.Error("equal Named types should be equal")
		}
	})

	t.Run("different kinds", func(t *testing.T) {
		a := Named("Foo")
		b := Builtin("Foo")
		if TypesEqual(a, b) {
			t.Error("different kinds should not be equal")
		}
	})

	t.Run("nested pointer to named", func(t *testing.T) {
		a := Pointer(Named("Foo"))
		b := Pointer(Named("Foo"))
		if !TypesEqual(a, b) {
			t.Error("equal nested types should be equal")
		}
	})

	t.Run("different nested", func(t *testing.T) {
		a := Pointer(Named("Foo"))
		b := Pointer(Named("Bar"))
		if TypesEqual(a, b) {
			t.Error("different nested types should not be equal")
		}
	})

	t.Run("func types equal", func(t *testing.T) {
		a := Func([]Param{{Name: "x", Type: Builtin("int")}}, []*Type{Builtin("error")})
		b := Func([]Param{{Name: "x", Type: Builtin("int")}}, []*Type{Builtin("error")})
		if !TypesEqual(a, b) {
			t.Error("equal func types should be equal")
		}
	})

	t.Run("func types different params", func(t *testing.T) {
		a := Func([]Param{{Name: "x", Type: Builtin("int")}}, nil)
		b := Func([]Param{{Name: "y", Type: Builtin("int")}}, nil)
		if TypesEqual(a, b) {
			t.Error("func types with different param names should not be equal")
		}
	})

	t.Run("map types", func(t *testing.T) {
		a := Map(Builtin("string"), Named("Foo"))
		b := Map(Builtin("string"), Named("Foo"))
		if !TypesEqual(a, b) {
			t.Error("equal map types should be equal")
		}
	})

	t.Run("struct types", func(t *testing.T) {
		a := Struct([]Field{{Name: "ID", Type: Builtin("int")}})
		b := Struct([]Field{{Name: "ID", Type: Builtin("int")}})
		if !TypesEqual(a, b) {
			t.Error("equal struct types should be equal")
		}
	})

	t.Run("interface types", func(t *testing.T) {
		sig := Func(nil, []*Type{Builtin("string")})
		a := Interface([]Method{{Name: "String", Signature: sig}})
		b := Interface([]Method{{Name: "String", Signature: Func(nil, []*Type{Builtin("string")})}})
		if !TypesEqual(a, b) {
			t.Error("equal interface types should be equal")
		}
	})
}

func TestTypeConvenienceMethods(t *testing.T) {
	t.Run("IsUnknown", func(t *testing.T) {
		if !Unknown().IsUnknown() {
			t.Error("Unknown().IsUnknown() should be true")
		}
		if Named("X").IsUnknown() {
			t.Error("Named().IsUnknown() should be false")
		}
	})

	t.Run("IsInterface", func(t *testing.T) {
		if !Interface(nil).IsInterface() {
			t.Error("Interface().IsInterface() should be true")
		}
		if Named("X").IsInterface() {
			t.Error("Named().IsInterface() should be false")
		}
	})

	t.Run("IsPointer", func(t *testing.T) {
		if !Pointer(Builtin("int")).IsPointer() {
			t.Error("Pointer().IsPointer() should be true")
		}
		if Builtin("int").IsPointer() {
			t.Error("Builtin().IsPointer() should be false")
		}
	})
}

func TestTypeDeref(t *testing.T) {
	t.Run("pointer deref", func(t *testing.T) {
		inner := Named("Foo")
		ptr := Pointer(inner)
		if got := ptr.Deref(); got != inner {
			t.Error("Deref on pointer should return elem")
		}
	})

	t.Run("non-pointer passthrough", func(t *testing.T) {
		ty := Named("Foo")
		if got := ty.Deref(); got != ty {
			t.Error("Deref on non-pointer should return self")
		}
	})

	t.Run("nil passthrough", func(t *testing.T) {
		var ty *Type
		if got := ty.Deref(); got != nil {
			t.Error("Deref on nil should return nil")
		}
	})
}

func TestTypeGetElem(t *testing.T) {
	t.Run("pointer elem", func(t *testing.T) {
		inner := Builtin("int")
		ptr := Pointer(inner)
		if got := ptr.GetElem(); got != inner {
			t.Error("GetElem on pointer should return elem")
		}
	})

	t.Run("slice elem", func(t *testing.T) {
		inner := Builtin("string")
		sl := Slice(inner)
		if got := sl.GetElem(); got != inner {
			t.Error("GetElem on slice should return elem")
		}
	})

	t.Run("array elem", func(t *testing.T) {
		inner := Builtin("byte")
		arr := Array(inner)
		if got := arr.GetElem(); got != inner {
			t.Error("GetElem on array should return elem")
		}
	})

	t.Run("channel elem", func(t *testing.T) {
		inner := Builtin("int")
		ch := Channel(inner, ChanRecv)
		if got := ch.GetElem(); got != inner {
			t.Error("GetElem on channel should return elem")
		}
	})

	t.Run("optional elem", func(t *testing.T) {
		inner := Named("Foo")
		opt := Optional(inner)
		if got := opt.GetElem(); got != inner {
			t.Error("GetElem on optional should return elem")
		}
	})

	t.Run("reference elem", func(t *testing.T) {
		inner := Named("Bar")
		ref := Ref(inner)
		if got := ref.GetElem(); got != inner {
			t.Error("GetElem on reference should return elem")
		}
	})

	t.Run("named returns nil", func(t *testing.T) {
		ty := Named("Foo")
		if got := ty.GetElem(); got != nil {
			t.Error("GetElem on Named should return nil")
		}
	})

	t.Run("nil type returns nil", func(t *testing.T) {
		var ty *Type
		if got := ty.GetElem(); got != nil {
			t.Error("GetElem on nil should return nil")
		}
	})
}
