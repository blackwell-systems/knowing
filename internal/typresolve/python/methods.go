package pyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// maxMRODepth is the maximum depth for MRO-based attribute lookups,
// preventing infinite recursion in cyclic inheritance chains.
const maxMRODepth = 5

// LookupAttribute looks up a method on a named type, following MRO
// (base classes stored as EmbeddedTypes) up to maxMRODepth levels deep.
// Returns the registered function if found, nil otherwise.
func LookupAttribute(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.RegisteredFunc {
	return lookupAttributeDepth(reg, typeQN, memberName, 0)
}

func lookupAttributeDepth(reg *typresolve.Registry, typeQN string, memberName string, depth int) *typresolve.RegisteredFunc {
	if depth >= maxMRODepth {
		return nil
	}

	// 1. Direct method lookup.
	if f := reg.LookupMethod(typeQN, memberName); f != nil {
		return f
	}

	// 2. Get the type definition.
	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// 3. Follow alias.
	if t.AliasOf != "" {
		return lookupAttributeDepth(reg, t.AliasOf, memberName, depth+1)
	}

	// 4. Check base classes (stored as EmbeddedTypes).
	for _, base := range t.EmbeddedTypes {
		if f := lookupAttributeDepth(reg, base, memberName, depth+1); f != nil {
			return f
		}
	}

	return nil
}

// LookupField looks up an instance field on a named type, following the MRO
// chain up to maxMRODepth levels deep. Returns the field's type if found,
// nil otherwise.
func LookupField(reg *typresolve.Registry, typeQN string, fieldName string) *typresolve.Type {
	return lookupFieldDepth(reg, typeQN, fieldName, 0)
}

func lookupFieldDepth(reg *typresolve.Registry, typeQN string, fieldName string, depth int) *typresolve.Type {
	if depth >= maxMRODepth {
		return nil
	}

	// 1. Get the type definition.
	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// 2. Follow alias.
	if t.AliasOf != "" {
		return lookupFieldDepth(reg, t.AliasOf, fieldName, depth+1)
	}

	// 3. Direct field lookup.
	for _, f := range t.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	// 4. Check base classes (stored as EmbeddedTypes).
	for _, base := range t.EmbeddedTypes {
		if ft := lookupFieldDepth(reg, base, fieldName, depth+1); ft != nil {
			return ft
		}
	}

	return nil
}
