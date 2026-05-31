package typresolve

import "fmt"

// TypeKind enumerates the language-agnostic type representation kinds.
type TypeKind int

const (
	KindUnknown   TypeKind = iota
	KindNamed              // named type: "Database", "http.Request"
	KindPointer            // *T
	KindSlice              // []T
	KindMap                // map[K]V
	KindChannel            // chan T (with direction)
	KindFunc               // func(params) returns
	KindInterface          // interface{...}
	KindStruct             // struct{...}
	KindBuiltin            // int, string, bool, error, etc.
	KindTuple              // multi-return (T1, T2)
	KindTypeParam          // generic type parameter: T, K, V
	KindArray              // [N]T
	KindOptional           // *T or Option<T> language-agnostic optional
	KindReference          // &T (Rust/C++ reference)
	KindAlias              // type alias
)

var kindNames = [...]string{
	KindUnknown:   "Unknown",
	KindNamed:     "Named",
	KindPointer:   "Pointer",
	KindSlice:     "Slice",
	KindMap:       "Map",
	KindChannel:   "Channel",
	KindFunc:      "Func",
	KindInterface: "Interface",
	KindStruct:    "Struct",
	KindBuiltin:   "Builtin",
	KindTuple:     "Tuple",
	KindTypeParam: "TypeParam",
	KindArray:     "Array",
	KindOptional:  "Optional",
	KindReference: "Reference",
	KindAlias:     "Alias",
}

// String returns a human-readable name for the TypeKind.
func (k TypeKind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("TypeKind(%d)", int(k))
}

// ChanDir represents the direction of a channel type.
type ChanDir int

const (
	ChanBidi ChanDir = iota // chan T
	ChanSend                // chan<- T
	ChanRecv                // <-chan T
)

// Param represents a function parameter.
type Param struct {
	Name string
	Type *Type
}

// Field represents a struct field.
type Field struct {
	Name string
	Type *Type
}

// Method represents an interface method.
type Method struct {
	Name      string
	Signature *Type // KindFunc
}

// TypeParam represents a generic type parameter.
type TypeParam struct {
	Name       string
	Constraint *Type // nil means "any"
}

// Type is the core type representation struct with kind discrimination.
// Different fields are populated based on Kind, analogous to the tagged union
// approach in the C reference implementation (CBMType).
type Type struct {
	Kind       TypeKind
	Name       string      // qualified name for Named/Builtin/TypeParam/Alias
	Elem       *Type       // element for Pointer/Slice/Array/Channel/Optional/Reference
	Key        *Type       // key for Map
	Value      *Type       // value for Map
	Params     []Param     // parameters for Func
	Returns    []*Type     // return types for Func
	Fields     []Field     // fields for Struct
	Methods    []Method    // methods for Interface
	Elements   []*Type     // elements for Tuple
	TypeParams []TypeParam // generic type parameters
	Underlying *Type       // underlying type for Alias
	ChanDir    ChanDir     // direction for Channel
}

// IsUnknown returns true if the type has KindUnknown.
func (t *Type) IsUnknown() bool {
	return t != nil && t.Kind == KindUnknown
}

// IsInterface returns true if the type has KindInterface.
func (t *Type) IsInterface() bool {
	return t != nil && t.Kind == KindInterface
}

// IsPointer returns true if the type has KindPointer.
func (t *Type) IsPointer() bool {
	return t != nil && t.Kind == KindPointer
}

// GetElem returns the element type for Pointer, Slice, Array, Channel,
// Optional, and Reference kinds. Returns nil for other kinds.
func (t *Type) GetElem() *Type {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case KindPointer, KindSlice, KindArray, KindChannel, KindOptional, KindReference:
		return t.Elem
	default:
		return nil
	}
}

// Deref removes one pointer level: returns the element type if Pointer,
// or the type itself otherwise.
func (t *Type) Deref() *Type {
	if t != nil && t.Kind == KindPointer && t.Elem != nil {
		return t.Elem
	}
	return t
}

// --- Constructor functions ---

// Unknown returns a new type with KindUnknown.
func Unknown() *Type {
	return &Type{Kind: KindUnknown}
}

// Named returns a new named type with the given qualified name.
func Named(qualifiedName string) *Type {
	return &Type{Kind: KindNamed, Name: qualifiedName}
}

// Pointer returns a new pointer type wrapping elem.
func Pointer(elem *Type) *Type {
	return &Type{Kind: KindPointer, Elem: elem}
}

// Slice returns a new slice type with the given element type.
func Slice(elem *Type) *Type {
	return &Type{Kind: KindSlice, Elem: elem}
}

// Map returns a new map type with the given key and value types.
func Map(key, value *Type) *Type {
	return &Type{Kind: KindMap, Key: key, Value: value}
}

// Channel returns a new channel type with the given element type and direction.
func Channel(elem *Type, dir ChanDir) *Type {
	return &Type{Kind: KindChannel, Elem: elem, ChanDir: dir}
}

// Func returns a new function type with the given parameters and return types.
func Func(params []Param, returns []*Type) *Type {
	return &Type{Kind: KindFunc, Params: params, Returns: returns}
}

// Builtin returns a new builtin type (int, string, bool, error, etc.).
func Builtin(name string) *Type {
	return &Type{Kind: KindBuiltin, Name: name}
}

// Tuple returns a new tuple type (multi-return) with the given element types.
func Tuple(elems []*Type) *Type {
	return &Type{Kind: KindTuple, Elements: elems}
}

// TypeParamType returns a new generic type parameter.
func TypeParamType(name string) *Type {
	return &Type{Kind: KindTypeParam, Name: name}
}

// Array returns a new array type with the given element type.
func Array(elem *Type) *Type {
	return &Type{Kind: KindArray, Elem: elem}
}

// Optional returns a new optional type wrapping elem.
func Optional(elem *Type) *Type {
	return &Type{Kind: KindOptional, Elem: elem}
}

// Ref returns a new reference type wrapping elem.
func Ref(elem *Type) *Type {
	return &Type{Kind: KindReference, Elem: elem}
}

// Alias returns a new alias type with the given name and underlying type.
func Alias(name string, underlying *Type) *Type {
	return &Type{Kind: KindAlias, Name: name, Underlying: underlying}
}

// Interface returns a new interface type with the given methods.
func Interface(methods []Method) *Type {
	return &Type{Kind: KindInterface, Methods: methods}
}

// Struct returns a new struct type with the given fields.
func Struct(fields []Field) *Type {
	return &Type{Kind: KindStruct, Fields: fields}
}

// TypesEqual performs structural equality comparison of two types.
// Two types are equal if their kinds match and all structural members
// match recursively. Both nil returns true; one nil returns false.
func TypesEqual(a, b *Type) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Name != b.Name {
		return false
	}
	if a.ChanDir != b.ChanDir {
		return false
	}
	if !TypesEqual(a.Elem, b.Elem) {
		return false
	}
	if !TypesEqual(a.Key, b.Key) {
		return false
	}
	if !TypesEqual(a.Value, b.Value) {
		return false
	}
	if !TypesEqual(a.Underlying, b.Underlying) {
		return false
	}

	// Compare Params
	if len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if a.Params[i].Name != b.Params[i].Name {
			return false
		}
		if !TypesEqual(a.Params[i].Type, b.Params[i].Type) {
			return false
		}
	}

	// Compare Returns
	if len(a.Returns) != len(b.Returns) {
		return false
	}
	for i := range a.Returns {
		if !TypesEqual(a.Returns[i], b.Returns[i]) {
			return false
		}
	}

	// Compare Fields
	if len(a.Fields) != len(b.Fields) {
		return false
	}
	for i := range a.Fields {
		if a.Fields[i].Name != b.Fields[i].Name {
			return false
		}
		if !TypesEqual(a.Fields[i].Type, b.Fields[i].Type) {
			return false
		}
	}

	// Compare Methods
	if len(a.Methods) != len(b.Methods) {
		return false
	}
	for i := range a.Methods {
		if a.Methods[i].Name != b.Methods[i].Name {
			return false
		}
		if !TypesEqual(a.Methods[i].Signature, b.Methods[i].Signature) {
			return false
		}
	}

	// Compare Elements (Tuple)
	if len(a.Elements) != len(b.Elements) {
		return false
	}
	for i := range a.Elements {
		if !TypesEqual(a.Elements[i], b.Elements[i]) {
			return false
		}
	}

	// Compare TypeParams
	if len(a.TypeParams) != len(b.TypeParams) {
		return false
	}
	for i := range a.TypeParams {
		if a.TypeParams[i].Name != b.TypeParams[i].Name {
			return false
		}
		if !TypesEqual(a.TypeParams[i].Constraint, b.TypeParams[i].Constraint) {
			return false
		}
	}

	return true
}
