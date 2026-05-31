package csresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// csPredefTypes maps C# predefined type keyword aliases to their System.*
// fully qualified names. This is a local copy for compile-time independence
// from builtins.go (Agent B).
var csPredefTypes = map[string]string{
	"int":     "System.Int32",
	"uint":    "System.UInt32",
	"long":    "System.Int64",
	"ulong":   "System.UInt64",
	"short":   "System.Int16",
	"ushort":  "System.UInt16",
	"byte":    "System.Byte",
	"sbyte":   "System.SByte",
	"float":   "System.Single",
	"double":  "System.Double",
	"decimal": "System.Decimal",
	"bool":    "System.Boolean",
	"char":    "System.Char",
	"string":  "System.String",
	"object":  "System.Object",
	"void":    "System.Void",
	"nint":    "System.IntPtr",
	"nuint":   "System.UIntPtr",
	"dynamic": "System.Object",
}

// csGenericCollections maps well-known generic collection base names to
// their simplified type representations.
var csGenericCollections = map[string]string{
	"List":      "slice",
	"IList":     "slice",
	"IEnumerable": "slice",
	"ICollection": "slice",
	"HashSet":   "slice",
	"Queue":     "slice",
	"Stack":     "slice",
	"Dictionary": "map",
	"IDictionary": "map",
	"SortedDictionary": "map",
	"ConcurrentDictionary": "map",
}

// csTaskTypes lists type names that should be async-unwrapped.
var csTaskTypes = map[string]bool{
	"Task":      true,
	"ValueTask": true,
	"System.Threading.Tasks.Task":      true,
	"System.Threading.Tasks.ValueTask": true,
}

// ParseTypeNode converts a tree-sitter C# type AST node to a typresolve.Type.
// It handles predefined_type, nullable_type, array_type, pointer_type,
// tuple_type, generic_name, qualified_name, identifier, ref_type, and
// implicit_type node kinds.
func ParseTypeNode(node *sitter.Node, content []byte, namespaceQN string,
	usings []UsingInfo, registry *typresolve.Registry) *typresolve.Type {

	if node == nil {
		return nil
	}

	switch node.Type() {
	case "predefined_type":
		name := node.Content(content)
		if alias, ok := csPredefTypes[name]; ok {
			return typresolve.Named(alias)
		}
		return typresolve.Named(name)

	case "nullable_type":
		// Nullable<T>: unwrap to inner type (T? -> T for resolution).
		if node.NamedChildCount() > 0 {
			inner := ParseTypeNode(node.NamedChild(0), content, namespaceQN, usings, registry)
			if inner != nil {
				return inner
			}
		}
		return typresolve.Unknown()

	case "array_type":
		// Array type: T[] -> Slice(T).
		if node.NamedChildCount() > 0 {
			elem := ParseTypeNode(node.NamedChild(0), content, namespaceQN, usings, registry)
			if elem != nil {
				return typresolve.Slice(elem)
			}
		}
		return typresolve.Slice(typresolve.Unknown())

	case "pointer_type":
		// Pointer type (unsafe): T* -> Pointer(T).
		if node.NamedChildCount() > 0 {
			inner := ParseTypeNode(node.NamedChild(0), content, namespaceQN, usings, registry)
			if inner != nil {
				return typresolve.Pointer(inner)
			}
		}
		return typresolve.Pointer(typresolve.Unknown())

	case "tuple_type":
		// Tuple: (T1, T2, ...) -> Tuple([T1, T2, ...]).
		var elems []*typresolve.Type
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "tuple_element" {
				// tuple_element has a type child.
				if child.NamedChildCount() > 0 {
					elemType := ParseTypeNode(child.NamedChild(0), content, namespaceQN, usings, registry)
					if elemType != nil {
						elems = append(elems, elemType)
						continue
					}
				}
				elems = append(elems, typresolve.Unknown())
			}
		}
		if len(elems) > 0 {
			return typresolve.Tuple(elems)
		}
		return typresolve.Unknown()

	case "generic_name":
		// Generic type: Name<T1, T2>.
		// Extract base name and type arguments.
		nameNode := node.ChildByFieldName("name")
		var baseName string
		if nameNode != nil {
			baseName = nameNode.Content(content)
		} else {
			// Fallback: first child is often the identifier.
			for i := 0; i < int(node.ChildCount()); i++ {
				c := node.Child(i)
				if c.Type() == "identifier" || c.Type() == "identifier_name" {
					baseName = c.Content(content)
					break
				}
			}
		}
		if baseName == "" {
			// Last resort: strip generic args from full content.
			baseName = stripGenericArgs(node.Content(content))
		}

		// Collect type arguments.
		var typeArgs []*typresolve.Type
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "type_argument_list" {
				for j := 0; j < int(child.NamedChildCount()); j++ {
					arg := ParseTypeNode(child.NamedChild(j), content, namespaceQN, usings, registry)
					if arg != nil {
						typeArgs = append(typeArgs, arg)
					}
				}
			}
		}

		// Handle well-known generic collections.
		if kind, ok := csGenericCollections[baseName]; ok {
			switch kind {
			case "slice":
				if len(typeArgs) > 0 {
					return typresolve.Slice(typeArgs[0])
				}
				return typresolve.Slice(typresolve.Unknown())
			case "map":
				key := typresolve.Unknown()
				val := typresolve.Unknown()
				if len(typeArgs) > 0 {
					key = typeArgs[0]
				}
				if len(typeArgs) > 1 {
					val = typeArgs[1]
				}
				return typresolve.Map(key, val)
			}
		}

		// Handle Task/ValueTask (async unwrap).
		if csTaskTypes[baseName] {
			if len(typeArgs) > 0 {
				return typeArgs[0]
			}
			return typresolve.Unknown()
		}

		// Resolve the base name through namespace/using chain.
		resolved := ResolveTypeName(baseName, namespaceQN, usings, registry, "", "")
		return typresolve.Named(resolved)

	case "qualified_name":
		// Fully or partially qualified name: A.B.C.
		name := node.Content(content)
		name = normalizeCSName(name)
		resolved := ResolveTypeName(name, namespaceQN, usings, registry, "", "")
		return typresolve.Named(resolved)

	case "identifier", "identifier_name":
		// Simple identifier type name.
		name := node.Content(content)
		// Check if it's a predefined alias.
		if alias, ok := csPredefTypes[name]; ok {
			return typresolve.Named(alias)
		}
		resolved := ResolveTypeName(name, namespaceQN, usings, registry, "", "")
		return typresolve.Named(resolved)

	case "alias_qualified_name":
		// alias::Type -> resolve through using aliases.
		name := node.Content(content)
		name = normalizeCSName(name)
		resolved := ResolveTypeName(name, namespaceQN, usings, registry, "", "")
		return typresolve.Named(resolved)

	case "ref_type":
		// ref T -> Reference(T).
		if node.NamedChildCount() > 0 {
			inner := ParseTypeNode(node.NamedChild(0), content, namespaceQN, usings, registry)
			if inner != nil {
				return typresolve.Ref(inner)
			}
		}
		return typresolve.Ref(typresolve.Unknown())

	case "implicit_type":
		// var / implicit type: inferred at usage.
		return typresolve.Unknown()

	case "type_parameter":
		// Generic type parameter: T.
		name := node.Content(content)
		return typresolve.TypeParamType(name)

	case "array_rank_specifier":
		// Part of array type, not a standalone type.
		return nil

	default:
		// For unrecognized node types, try to resolve the content as a name.
		text := strings.TrimSpace(node.Content(content))
		if text != "" {
			resolved := ResolveTypeName(text, namespaceQN, usings, registry, "", "")
			return typresolve.Named(resolved)
		}
		return nil
	}
}
