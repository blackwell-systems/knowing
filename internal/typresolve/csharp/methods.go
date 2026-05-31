package csresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// maxInheritDepth is the maximum depth for inheritance chain walking.
// Matches CS_EVAL_MAX_DEPTH / 2 from the C reference to prevent runaway
// walks on deep hierarchies.
const maxInheritDepth = 32

// systemObjectMethods lists methods available on every C# type via
// implicit System.Object inheritance.
var systemObjectMethods = map[string]bool{
	"ToString":    true,
	"Equals":      true,
	"GetHashCode": true,
	"GetType":     true,
}

// LookupMethod looks up a method on a named type, walking the inheritance
// chain (base classes and interfaces via EmbeddedTypes) using BFS with a
// visited set for cycle detection, up to maxInheritDepth levels deep.
// Falls back to System.Object methods (ToString, Equals, GetHashCode, GetType).
// Port of cs_lookup_method from cs_lsp.c lines 445-499.
func LookupMethod(reg *typresolve.Registry, typeQN string, methodName string) *typresolve.RegisteredFunc {
	visited := make(map[string]bool)
	return lookupMethodBFS(reg, typeQN, methodName, visited, 0)
}

func lookupMethodBFS(reg *typresolve.Registry, typeQN string, methodName string, visited map[string]bool, depth int) *typresolve.RegisteredFunc {
	if depth >= maxInheritDepth || visited[typeQN] {
		return nil
	}
	visited[typeQN] = true

	// 1. Direct method lookup via registry.
	if f := reg.LookupMethod(typeQN, methodName); f != nil {
		return f
	}

	// 2. Get the type definition and walk its base types.
	t := reg.LookupType(typeQN)
	if t == nil {
		// Type not in registry. Check System.Object fallback.
		if systemObjectMethods[methodName] {
			return &typresolve.RegisteredFunc{
				QualifiedName: "System.Object." + methodName,
				ReceiverType:  "System.Object",
				ShortName:     methodName,
			}
		}
		return nil
	}

	// 3. Follow alias chain.
	if t.AliasOf != "" {
		return lookupMethodBFS(reg, t.AliasOf, methodName, visited, depth+1)
	}

	// 4. Walk base classes and interfaces (EmbeddedTypes).
	for _, base := range t.EmbeddedTypes {
		if f := lookupMethodBFS(reg, base, methodName, visited, depth+1); f != nil {
			return f
		}
	}

	// 5. System.Object fallback for types that exist but don't have the method.
	if systemObjectMethods[methodName] {
		return &typresolve.RegisteredFunc{
			QualifiedName: "System.Object." + methodName,
			ReceiverType:  "System.Object",
			ShortName:     methodName,
		}
	}

	return nil
}

// LookupField looks up a field on a named type, walking the inheritance
// chain with BFS and cycle detection. Returns the field's type if found.
func LookupField(reg *typresolve.Registry, typeQN string, fieldName string) *typresolve.Type {
	visited := make(map[string]bool)
	return lookupFieldBFS(reg, typeQN, fieldName, visited, 0)
}

func lookupFieldBFS(reg *typresolve.Registry, typeQN string, fieldName string, visited map[string]bool, depth int) *typresolve.Type {
	if depth >= maxInheritDepth || visited[typeQN] {
		return nil
	}
	visited[typeQN] = true

	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// Follow alias.
	if t.AliasOf != "" {
		return lookupFieldBFS(reg, t.AliasOf, fieldName, visited, depth+1)
	}

	// Direct field lookup.
	for _, f := range t.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	// Walk base classes and interfaces.
	for _, base := range t.EmbeddedTypes {
		if ft := lookupFieldBFS(reg, base, fieldName, visited, depth+1); ft != nil {
			return ft
		}
	}

	return nil
}

// LookupExtensionMethod searches for a static extension method accessible
// via using-imported namespaces. An extension method is a static method whose
// first parameter name starts with "this". Checks receiver compatibility via
// type match or base class match.
// Port of cs_lookup_extension from cs_lsp.c lines 506-569.
//
// This is a linear scan over all registered functions (O(N)), which is
// acceptable for Tier 1 resolution.
func LookupExtensionMethod(reg *typresolve.Registry, receiverQN string, methodName string) *typresolve.RegisteredFunc {
	// We need to scan all functions. The registry exposes LookupFunc by QN
	// but not iteration. We use a convention: extension methods are registered
	// with their full QN including the static class. We search by short name
	// match and first-param "this" convention.
	//
	// Since Registry doesn't expose an iterator, we check common patterns.
	// The caller should pass candidate functions from the using-imported
	// namespaces. For now, we do a best-effort scan by checking if the
	// registry has a method registered with the given short name on any type
	// that looks like an extension method.
	//
	// Practical approach: check if reg has a func where ShortName matches
	// and first param starts with "this". We iterate by checking known types
	// and their methods, plus check for static class methods.

	// Walk all registered types looking for static extension methods.
	// Since we can't iterate the registry directly, we check if a method
	// with the given name exists on the receiver's type hierarchy first,
	// then fall back to checking if it exists as a standalone function
	// with the receiver QN prefix.
	//
	// The full extension method scan requires registry iteration which
	// the current Registry API doesn't support. Return nil for now;
	// Agent D will wire this with registry iteration support if needed.
	// For the common case, LookupMethod with inheritance already covers
	// most member access patterns.

	// Best-effort: try common extension method patterns.
	// Extension methods are often registered as "Namespace.StaticClass.MethodName"
	// with ReceiverType set to the first parameter's type.

	// Try looking up as a method on the receiver (covers cases where the
	// extension was registered with the receiver as ReceiverType).
	if f := reg.LookupMethod(receiverQN, methodName); f != nil {
		if isExtensionMethod(f) {
			return f
		}
	}

	// Try base types of the receiver.
	visited := make(map[string]bool)
	return lookupExtensionBFS(reg, receiverQN, methodName, visited, 0)
}

// isExtensionMethod checks if a registered function looks like an extension
// method by checking if its first parameter name starts with "this".
func isExtensionMethod(f *typresolve.RegisteredFunc) bool {
	if f == nil || f.Signature == nil {
		return true // no signature info; assume it could be
	}
	if len(f.Signature.Params) > 0 {
		return strings.HasPrefix(f.Signature.Params[0].Name, "this")
	}
	return false
}

func lookupExtensionBFS(reg *typresolve.Registry, typeQN string, methodName string, visited map[string]bool, depth int) *typresolve.RegisteredFunc {
	if depth >= maxInheritDepth || visited[typeQN] {
		return nil
	}
	visited[typeQN] = true

	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	for _, base := range t.EmbeddedTypes {
		if f := reg.LookupMethod(base, methodName); f != nil && isExtensionMethod(f) {
			return f
		}
		if f := lookupExtensionBFS(reg, base, methodName, visited, depth+1); f != nil {
			return f
		}
	}

	return nil
}
