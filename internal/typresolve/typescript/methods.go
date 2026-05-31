package tsresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// maxLookupDepth is the maximum prototype chain traversal depth to prevent
// infinite loops from circular inheritance.
const maxLookupDepth = 5

// LookupMember looks up a method on a named type, following the prototype
// chain (stored as EmbeddedTypes) up to maxLookupDepth levels deep. For
// builtin primitive types (string, number), delegates to the wrapper class
// (String, Number). Returns the registered function if found, nil otherwise.
func LookupMember(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.RegisteredFunc {
	return lookupMember(reg, typeQN, memberName, 0)
}

func lookupMember(reg *typresolve.Registry, typeQN string, memberName string, depth int) *typresolve.RegisteredFunc {
	if depth >= maxLookupDepth {
		return nil
	}

	// 1. Direct method lookup.
	if f := reg.LookupMethod(typeQN, memberName); f != nil {
		return f
	}

	// 2. Get type info.
	t := reg.LookupType(typeQN)
	if t == nil {
		// 5. If typeQN is a builtin primitive, try via wrapper class.
		if wrapper := BuiltinWrapperClass(typeQN); wrapper != "" {
			return lookupMember(reg, wrapper, memberName, depth+1)
		}
		return nil
	}

	// 3. Follow alias.
	if t.AliasOf != "" {
		return lookupMember(reg, t.AliasOf, memberName, depth+1)
	}

	// 4. Check prototype chain (EmbeddedTypes).
	for _, parent := range t.EmbeddedTypes {
		if f := lookupMember(reg, parent, memberName, depth+1); f != nil {
			return f
		}
	}

	// 5. If typeQN is a builtin primitive, try via wrapper class.
	if wrapper := BuiltinWrapperClass(typeQN); wrapper != "" {
		return lookupMember(reg, wrapper, memberName, depth+1)
	}

	return nil
}

// LookupField looks up a field/property on a named type, following the
// prototype chain (EmbeddedTypes) up to maxLookupDepth levels deep.
// Returns the field's type if found, nil otherwise.
func LookupField(reg *typresolve.Registry, typeQN string, fieldName string) *typresolve.Type {
	return lookupField(reg, typeQN, fieldName, 0)
}

func lookupField(reg *typresolve.Registry, typeQN string, fieldName string, depth int) *typresolve.Type {
	if depth >= maxLookupDepth {
		return nil
	}

	// 1. Get type info.
	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// 2. Follow alias.
	if t.AliasOf != "" {
		return lookupField(reg, t.AliasOf, fieldName, depth+1)
	}

	// 3. Direct field lookup.
	for _, f := range t.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	// 4. Check prototype chain.
	for _, parent := range t.EmbeddedTypes {
		if ft := lookupField(reg, parent, fieldName, depth+1); ft != nil {
			return ft
		}
	}

	return nil
}

// LookupMemberType combines field and method lookup. First checks fields
// (returns the field type directly), then checks methods (returns the
// method's signature type). This is the Go equivalent of lookup_member_type
// in the C reference.
//
// Fix #7: Resolves type aliases before member lookup.
// Fix #8: For intersection types (stored as Named with TypeParams containing
// multiple members), dispatches lookup across all intersection members.
func LookupMemberType(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.Type {
	// Try fields first.
	if ft := LookupField(reg, typeQN, memberName); ft != nil {
		return ft
	}
	// Then try methods.
	if f := LookupMember(reg, typeQN, memberName); f != nil {
		return f.Signature
	}
	return nil
}

// LookupMemberTypeOnType performs member lookup on an arbitrary Type value,
// handling intersection types (Struct merging), struct fields, Named dispatch,
// and builtin wrapper classes. Used by evalMemberExpression for compound types.
func LookupMemberTypeOnType(reg *typresolve.Registry, t *typresolve.Type, memberName string) *typresolve.Type {
	if t == nil || reg == nil {
		return nil
	}

	switch t.Kind {
	case typresolve.KindNamed:
		// Fix #8: Check if this is an intersection type (TypeParams hold members).
		if len(t.TypeParams) > 1 {
			for _, tp := range t.TypeParams {
				if tp.Constraint != nil {
					if result := LookupMemberTypeOnType(reg, tp.Constraint, memberName); result != nil {
						return result
					}
				}
			}
		}
		return LookupMemberType(reg, t.Name, memberName)

	case typresolve.KindStruct:
		for _, f := range t.Fields {
			if f.Name == memberName {
				return f.Type
			}
		}

	case typresolve.KindBuiltin:
		wrapper := BuiltinWrapperClass(t.Name)
		if wrapper != "" {
			return LookupMemberType(reg, wrapper, memberName)
		}

	case typresolve.KindSlice, typresolve.KindArray:
		return LookupMemberType(reg, "Array", memberName)
	}

	return nil
}
