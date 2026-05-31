package rubyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// maxMRODepth is the maximum depth for method resolution order traversal.
// Ruby's MRO can be deeper than Go's embedding, so we allow 8 levels.
const maxMRODepth = 8

// LookupAttribute looks up a method or attribute on a named type, following
// the Ruby ancestor chain (include/extend/prepend stored as EmbeddedTypes)
// up to maxMRODepth levels deep. Returns the registered function if found.
func LookupAttribute(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.RegisteredFunc {
	return lookupAttributeDepth(reg, typeQN, memberName, 0)
}

// lookupAttributeDepth is the internal recursive helper with depth tracking.
func lookupAttributeDepth(reg *typresolve.Registry, typeQN string, memberName string, depth int) *typresolve.RegisteredFunc {
	if depth >= maxMRODepth {
		return nil
	}

	// 1. Direct method lookup
	if f := reg.LookupMethod(typeQN, memberName); f != nil {
		return f
	}

	// 2. Get type info
	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// 3. Follow alias
	if t.AliasOf != "" {
		return lookupAttributeDepth(reg, t.AliasOf, memberName, depth+1)
	}

	// 4. Check embedded types (included modules + superclass) following MRO
	for _, embedded := range t.EmbeddedTypes {
		if f := lookupAttributeDepth(reg, embedded, memberName, depth+1); f != nil {
			return f
		}
	}

	return nil
}

// LookupAttrAccessor checks if a member name corresponds to an
// attr_reader/attr_writer/attr_accessor-generated method on the type.
// These are stored as methods with synthetic signatures in the registry.
func LookupAttrAccessor(reg *typresolve.Registry, typeQN string, memberName string) *typresolve.RegisteredFunc {
	// Try reader (exact name)
	if f := reg.LookupMethod(typeQN, memberName); f != nil {
		return f
	}

	// Try writer (name=)
	if f := reg.LookupMethod(typeQN, memberName+"="); f != nil {
		return f
	}

	// Delegate to MRO traversal
	if f := LookupAttribute(reg, typeQN, memberName); f != nil {
		return f
	}

	// Try writer via MRO
	if f := LookupAttribute(reg, typeQN, memberName+"="); f != nil {
		return f
	}

	return nil
}

// ResolveNew resolves ClassName.new to the initialize method.
// When ClassName.new is called, Ruby dispatches to initialize.
func ResolveNew(reg *typresolve.Registry, typeQN string) *typresolve.RegisteredFunc {
	return LookupAttribute(reg, typeQN, "initialize")
}
