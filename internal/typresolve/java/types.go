package javaresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// javaBuiltinTypes is the set of Java primitives and common builtin types.
var javaBuiltinTypes = map[string]bool{
	"int":       true,
	"long":      true,
	"short":     true,
	"byte":      true,
	"char":      true,
	"boolean":   true,
	"float":     true,
	"double":    true,
	"void":      true,
	"String":    true,
	"Object":    true,
	"Integer":   true,
	"Long":      true,
	"Boolean":   true,
	"Double":    true,
	"Float":     true,
	"Class":     true,
	"Void":      true,
	"Byte":      true,
	"Short":     true,
	"Character": true,
}

// genericSliceTypes maps Java collection type names that should be treated
// as slice-like (single type parameter collections).
var genericSliceTypes = map[string]bool{
	"List":       true,
	"ArrayList":  true,
	"LinkedList": true,
	"Collection": true,
	"Iterable":   true,
	"Set":        true,
	"HashSet":    true,
	"TreeSet":    true,
	"Queue":      true,
	"Deque":      true,
}

// genericMapTypes maps Java map type names that should be treated as
// map-like (two type parameter collections).
var genericMapTypes = map[string]bool{
	"Map":               true,
	"HashMap":           true,
	"TreeMap":           true,
	"LinkedHashMap":     true,
	"ConcurrentHashMap": true,
}

// IsBuiltinType returns true if the given name is a Java primitive or
// common builtin type.
func IsBuiltinType(name string) bool {
	return javaBuiltinTypes[name]
}

// ResolveBuiltinType returns a typresolve.Builtin type if the name is a
// Java builtin type, nil otherwise.
func ResolveBuiltinType(name string) *typresolve.Type {
	if javaBuiltinTypes[name] {
		return typresolve.Builtin(name)
	}
	return nil
}

// ParseTypeNode converts a tree-sitter Java type AST node to a
// typresolve.Type. It handles type_identifier, generic_type, array_type,
// scoped_type_identifier, void_type, integral_type, floating_point_type,
// boolean_type, and wildcard patterns.
func ParseTypeNode(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	switch node.Type() {
	case "type_identifier":
		name := node.Content(content)
		if javaBuiltinTypes[name] {
			return typresolve.Builtin(name)
		}
		qn := qualifyTypeName(name, pkgQN, imports)
		return typresolve.Named(qn)

	case "integral_type":
		// int, long, short, byte, char
		return typresolve.Builtin(node.Content(content))

	case "floating_point_type":
		// float, double
		return typresolve.Builtin(node.Content(content))

	case "boolean_type":
		return typresolve.Builtin("boolean")

	case "void_type":
		return typresolve.Builtin("void")

	case "generic_type":
		return parseGenericType(node, content, pkgQN, imports)

	case "array_type":
		element := node.ChildByFieldName("element")
		if element != nil {
			return typresolve.Slice(ParseTypeNode(element, content, pkgQN, imports))
		}
		return typresolve.Slice(typresolve.Unknown())

	case "scoped_type_identifier":
		return parseScopedType(node, content, pkgQN, imports)

	case "wildcard":
		return parseWildcard(node, content, pkgQN, imports)

	default:
		return typresolve.Unknown()
	}
}

// parseGenericType handles generic_type nodes like List<String>, Map<K, V>.
func parseGenericType(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) *typresolve.Type {
	// Get the base type name from the first type_identifier or scoped_type_identifier child.
	var baseName string
	var baseNode *sitter.Node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" || child.Type() == "scoped_type_identifier" {
			baseName = extractBaseTypeName(child, content)
			baseNode = child
			break
		}
	}
	if baseName == "" {
		return typresolve.Unknown()
	}

	// Collect type arguments.
	var typeArgs []*typresolve.Type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_arguments" {
			for j := 0; j < int(child.ChildCount()); j++ {
				arg := child.Child(j)
				if arg == nil {
					continue
				}
				// Skip punctuation: <, >, ,
				if arg.Type() == "<" || arg.Type() == ">" || arg.Type() == "," {
					continue
				}
				typeArgs = append(typeArgs, ParseTypeNode(arg, content, pkgQN, imports))
			}
			break
		}
	}

	// Map known generic containers.
	if genericSliceTypes[baseName] && len(typeArgs) > 0 {
		return typresolve.Slice(typeArgs[0])
	}
	if genericMapTypes[baseName] && len(typeArgs) >= 2 {
		return typresolve.Map(typeArgs[0], typeArgs[1])
	}
	if baseName == "Optional" && len(typeArgs) > 0 {
		return typresolve.Optional(typeArgs[0])
	}

	// For unknown generic types, return Named with TypeParams set.
	var qualifiedBase string
	if baseNode != nil && baseNode.Type() == "scoped_type_identifier" {
		qualifiedBase = parseScopedTypeName(baseNode, content, pkgQN, imports)
	} else {
		qualifiedBase = qualifyTypeName(baseName, pkgQN, imports)
	}
	result := typresolve.Named(qualifiedBase)
	for _, arg := range typeArgs {
		result.TypeParams = append(result.TypeParams, typresolve.TypeParam{
			Name:       arg.Name,
			Constraint: arg,
		})
	}
	return result
}

// parseScopedType handles scoped_type_identifier nodes like pkg.ClassName
// or Outer.Inner.
func parseScopedType(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) *typresolve.Type {
	qn := parseScopedTypeName(node, content, pkgQN, imports)
	return typresolve.Named(qn)
}

// parseScopedTypeName resolves a scoped_type_identifier to a qualified name string.
func parseScopedTypeName(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) string {
	nameNode := node.ChildByFieldName("name")
	scopeNode := node.ChildByFieldName("scope")

	if nameNode == nil {
		return node.Content(content)
	}

	name := nameNode.Content(content)

	if scopeNode == nil {
		return qualifyTypeName(name, pkgQN, imports)
	}

	scopeText := scopeNode.Content(content)

	// Check if the scope matches an import.
	if pkg, ok := imports[scopeText]; ok {
		return pkg + "." + scopeText + "." + name
	}

	// Otherwise concatenate scope + "." + name.
	return scopeText + "." + name
}

// parseWildcard handles wildcard nodes: ?, ? extends T, ? super T.
func parseWildcard(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) *typresolve.Type {
	// Look for "extends" or "super" bounds.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if child.Type() == "extends" || child.Content(content) == "extends" {
			// ? extends T: find the bound type (next sibling).
			for j := i + 1; j < int(node.ChildCount()); j++ {
				bound := node.Child(j)
				if bound != nil && bound.Type() != "extends" && bound.Type() != "super" {
					return ParseTypeNode(bound, content, pkgQN, imports)
				}
			}
		}
		if child.Type() == "super" || child.Content(content) == "super" {
			// ? super T: return Unknown (lower bound is not useful for resolution).
			return typresolve.Unknown()
		}
	}
	// Bare ?: return Unknown.
	return typresolve.Unknown()
}

// extractBaseTypeName extracts the base class name from a type node,
// stripping generics and array suffixes. Used for import qualification.
func extractBaseTypeName(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	switch node.Type() {
	case "type_identifier":
		return node.Content(content)
	case "scoped_type_identifier":
		name := node.ChildByFieldName("name")
		if name != nil {
			return name.Content(content)
		}
		return node.Content(content)
	case "generic_type":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "type_identifier" || child.Type() == "scoped_type_identifier" {
				return extractBaseTypeName(child, content)
			}
		}
	case "array_type":
		element := node.ChildByFieldName("element")
		if element != nil {
			return extractBaseTypeName(element, content)
		}
	}
	return ""
}

// qualifyTypeName qualifies a simple class name using the import map or
// the current package qualified name.
func qualifyTypeName(name string, pkgQN string, imports map[string]string) string {
	if pkg, ok := imports[name]; ok {
		return pkg + "." + name
	}
	if pkgQN != "" {
		return pkgQN + "." + name
	}
	return name
}
