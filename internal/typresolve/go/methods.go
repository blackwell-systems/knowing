package goresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// maxEmbedDepth is the maximum depth for embedded type promotion lookups.
const maxEmbedDepth = 5

// LookupFieldOrMethod looks up a method on a named type, following embedded
// type promotion up to maxEmbedDepth levels deep. Returns the registered
// function if found, nil otherwise.
func LookupFieldOrMethod(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.RegisteredFunc {
	return lookupFieldOrMethodDepth(reg, typeQN, memberName, 0)
}

func lookupFieldOrMethodDepth(reg *typresolve.Registry, typeQN string, memberName string, depth int) *typresolve.RegisteredFunc {
	if depth >= maxEmbedDepth {
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
		return lookupFieldOrMethodDepth(reg, t.AliasOf, memberName, depth+1)
	}

	// 4. Check embedded types.
	for _, embedded := range t.EmbeddedTypes {
		if f := lookupFieldOrMethodDepth(reg, embedded, memberName, depth+1); f != nil {
			return f
		}
	}

	return nil
}

// LookupField looks up a struct field on a named type, following embedded type
// promotion up to maxEmbedDepth levels deep. Returns the field's type if found,
// nil otherwise.
func LookupField(reg *typresolve.Registry, typeQN string, fieldName string) *typresolve.Type {
	return lookupFieldDepth(reg, typeQN, fieldName, 0)
}

func lookupFieldDepth(reg *typresolve.Registry, typeQN string, fieldName string, depth int) *typresolve.Type {
	if depth >= maxEmbedDepth {
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

	// 4. Check embedded types.
	for _, embedded := range t.EmbeddedTypes {
		if ft := lookupFieldDepth(reg, embedded, fieldName, depth+1); ft != nil {
			return ft
		}
	}

	return nil
}
