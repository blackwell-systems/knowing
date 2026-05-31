package rubyresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ParseScopeResolution parses a scope_resolution tree-sitter node into its
// fully qualified string representation. It handles nested scope resolution
// (A::B::C) by walking the tree recursively.
//
// Tree-sitter Ruby represents A::B::C as nested scope_resolution:
//
//	(scope_resolution
//	  scope: (scope_resolution
//	    scope: (constant) "A"
//	    name: (constant) "B")
//	  name: (constant) "C")
func ParseScopeResolution(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	scopeNode := node.ChildByFieldName("scope")
	nameNode := node.ChildByFieldName("name")

	if nameNode == nil {
		// Fallback: return the full node content.
		return node.Content(content)
	}

	namePart := nameNode.Content(content)

	if scopeNode == nil {
		// Top-level constant reference (::Foo).
		return "::" + namePart
	}

	if scopeNode.Type() == "scope_resolution" {
		// Recurse on the left-hand scope_resolution.
		leftPart := ParseScopeResolution(scopeNode, content)
		return leftPart + "::" + namePart
	}

	// Scope is a constant or other terminal node.
	scopePart := scopeNode.Content(content)
	return scopePart + "::" + namePart
}

// ResolveConstant resolves a Ruby constant path relative to the current nesting
// context. Ruby constant lookup walks outward from the current nesting.
//
// Rules:
//   - If constPath starts with "::", it is absolute: return constPath[2:].
//   - Otherwise, return the fully qualified name by joining the full nesting
//     with the constant path. This is the innermost candidate; callers that
//     need the outward walk should use ResolveConstantInRegistry instead.
func ResolveConstant(constPath string, nesting []string) string {
	// Absolute constant reference.
	if strings.HasPrefix(constPath, "::") {
		return constPath[2:]
	}

	// No nesting: top-level constant.
	if len(nesting) == 0 {
		return constPath
	}

	// Join the full nesting with the constant path.
	return strings.Join(nesting, "::") + "::" + constPath
}

// ResolveConstantInRegistry resolves a Ruby constant using lexical scoping:
// try A::B::C::X, then A::B::X, then A::X, then X. Returns the first
// qualified name that exists in the registry. Falls back to the innermost
// candidate (full nesting + constPath) if nothing is found.
func ResolveConstantInRegistry(reg *typresolve.Registry, constPath string, nesting []string) string {
	// Absolute reference.
	if strings.HasPrefix(constPath, "::") {
		return constPath[2:]
	}

	// Walk outward: try each nesting prefix from deepest to shallowest.
	for i := len(nesting); i >= 0; i-- {
		var candidate string
		if i == 0 {
			candidate = constPath
		} else {
			candidate = strings.Join(nesting[:i], "::") + "::" + constPath
		}
		if reg.LookupType(candidate) != nil {
			return candidate
		}
	}

	// Nothing found; return innermost candidate for heuristic resolution.
	if len(nesting) == 0 {
		return constPath
	}
	return strings.Join(nesting, "::") + "::" + constPath
}
