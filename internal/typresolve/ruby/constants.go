package rubyresolve

import (
	"strings"

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
//     with the constant path. The resolver will check registry existence and
//     fall back to progressively shorter prefixes.
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
