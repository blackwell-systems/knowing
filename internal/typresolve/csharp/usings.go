package csresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// UsingKind classifies the kind of a C# using directive.
type UsingKind int

const (
	// UsingNamespace is a plain "using Namespace;" directive.
	UsingNamespace UsingKind = iota
	// UsingStatic is a "using static Class;" directive.
	UsingStatic
	// UsingAlias is a "using Alias = Target;" directive.
	UsingAlias
)

// UsingInfo represents a single C# using directive with its kind, local name
// (for aliases), target qualified name, and global flag.
type UsingInfo struct {
	Kind      UsingKind
	LocalName string // for aliases: the local alias name
	TargetQN  string // the target namespace/class/type
	IsGlobal  bool
}

// BuildUsingMap extracts all using directives from the C# AST root and returns
// a slice of UsingInfo. Always includes implicit "using System".
func BuildUsingMap(root *sitter.Node, content []byte) []UsingInfo {
	// Start with implicit System using.
	usings := []UsingInfo{
		{Kind: UsingNamespace, TargetQN: "System"},
	}

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "using_directive" {
			continue
		}
		if u, ok := parseUsingDirective(child, content); ok {
			usings = append(usings, u)
		}
	}
	return usings
}

// parseUsingDirective parses a single using_directive AST node into a UsingInfo.
func parseUsingDirective(node *sitter.Node, content []byte) (UsingInfo, bool) {
	text := node.Content(content)

	var u UsingInfo

	// Detect global using.
	if strings.HasPrefix(text, "global ") {
		u.IsGlobal = true
		text = strings.TrimPrefix(text, "global ")
	}

	// Strip "using " prefix.
	text = strings.TrimPrefix(text, "using ")

	// Detect static using.
	if strings.HasPrefix(text, "static ") {
		u.Kind = UsingStatic
		text = strings.TrimPrefix(text, "static ")
		text = strings.TrimSuffix(text, ";")
		text = strings.TrimSpace(text)
		u.TargetQN = text
		return u, text != ""
	}

	// Detect alias using (contains "=").
	if idx := strings.Index(text, "="); idx >= 0 {
		u.Kind = UsingAlias
		u.LocalName = strings.TrimSpace(text[:idx])
		target := strings.TrimSpace(text[idx+1:])
		target = strings.TrimSuffix(target, ";")
		target = strings.TrimSpace(target)
		u.TargetQN = target
		return u, u.LocalName != "" && u.TargetQN != ""
	}

	// Plain namespace using.
	u.Kind = UsingNamespace
	text = strings.TrimSuffix(text, ";")
	text = strings.TrimSpace(text)
	u.TargetQN = text
	return u, text != ""
}

// ResolveTypeName resolves a bare type name through the C# resolution chain.
// It follows the 10-step algorithm ported from cs_lsp.c lines 317-442:
//  1. Strip global:: prefix, normalize :: to .
//  2. Strip generic args for lookup
//  3. Check predefined alias (int -> System.Int32)
//  4. Exact registry hit
//  5. Nested type under enclosing class
//  6. Each namespace prefix from innermost outward
//  7. Module-prefixed (file-local QN)
//  8. using namespace X -> try X.bare
//  9. using A = X alias substitution
//  10. Short-name fallback
//  11. Return bare name as last resort
func ResolveTypeName(raw string, namespaceQN string, usings []UsingInfo,
	registry *typresolve.Registry, enclosingClassQN string, moduleQN string) string {

	if raw == "" {
		return ""
	}

	// Step 1: Strip global:: prefix, normalize :: to .
	name := stripGlobalPrefix(raw)
	name = normalizeCSName(name)

	// Step 2: Strip generic args for lookup.
	lookup := stripGenericArgs(name)

	// Step 3: Check predefined alias.
	if alias := csPredefTypes[lookup]; alias != "" {
		return alias
	}

	// Step 4: Exact registry hit.
	if registry != nil && registry.LookupType(lookup) != nil {
		return lookup
	}

	// Step 5: Nested type under enclosing class.
	if enclosingClassQN != "" {
		nested := enclosingClassQN + "." + lookup
		if registry != nil && registry.LookupType(nested) != nil {
			return nested
		}
	}

	// Step 6: Each namespace prefix from innermost outward.
	if namespaceQN != "" {
		parts := strings.Split(namespaceQN, ".")
		for depth := len(parts); depth > 0; depth-- {
			prefix := strings.Join(parts[:depth], ".")
			candidate := prefix + "." + lookup
			if registry != nil && registry.LookupType(candidate) != nil {
				return candidate
			}
		}
	}

	// Step 7: Module-prefixed (file-local QN).
	if moduleQN != "" {
		candidate := moduleQN + "." + lookup
		if registry != nil && registry.LookupType(candidate) != nil {
			return candidate
		}
	}

	// Step 8: using namespace X -> try X.bare.
	for _, u := range usings {
		if u.Kind == UsingNamespace {
			candidate := u.TargetQN + "." + lookup
			if registry != nil && registry.LookupType(candidate) != nil {
				return candidate
			}
		}
	}

	// Step 9: using A = X alias substitution.
	for _, u := range usings {
		if u.Kind == UsingAlias && u.LocalName == lookup {
			return u.TargetQN
		}
	}

	// Step 10: Short-name fallback (best match by namespace prefix).
	if registry != nil {
		bestMatch := shortNameFallback(registry, lookup)
		if bestMatch != "" {
			return bestMatch
		}
	}

	// Step 11: Return bare name as last resort.
	return name
}

// shortNameFallback searches the registry for types whose short name matches
// the lookup. If found, returns the first match. This is a linear scan, but
// acceptable for resolution fallback.
func shortNameFallback(registry *typresolve.Registry, shortName string) string {
	// The registry doesn't expose iteration, so we can't do a linear scan.
	// Instead, try common System namespace prefixes as a heuristic.
	commonPrefixes := []string{
		"System", "System.Collections.Generic", "System.Linq",
		"System.IO", "System.Text", "System.Threading",
		"System.Threading.Tasks", "System.Net", "System.Net.Http",
	}
	for _, prefix := range commonPrefixes {
		candidate := prefix + "." + shortName
		if registry.LookupType(candidate) != nil {
			return candidate
		}
	}
	return ""
}

// stripGlobalPrefix removes the "global::" prefix from a C# type name.
func stripGlobalPrefix(name string) string {
	return strings.TrimPrefix(name, "global::")
}

// stripGenericArgs removes generic type arguments from a name.
// "List<int>" -> "List", "Dictionary<string, int>" -> "Dictionary".
func stripGenericArgs(name string) string {
	if idx := strings.IndexByte(name, '<'); idx >= 0 {
		return name[:idx]
	}
	return name
}

// normalizeCSName normalizes C# type name separators: replaces "::" with ".".
func normalizeCSName(name string) string {
	return strings.ReplaceAll(name, "::", ".")
}
