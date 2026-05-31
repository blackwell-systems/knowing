package rustresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

var rustPrimitives = map[string]bool{
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true,
	"isize": true, "u8": true, "u16": true, "u32": true, "u64": true,
	"u128": true, "usize": true, "f32": true, "f64": true,
	"bool": true, "char": true, "str": true,
}

func isRustPrimitive(name string) bool {
	return rustPrimitives[name]
}

// ParseTypeNode converts a tree-sitter Rust type AST node to a typresolve.Type.
func ParseTypeNode(node *sitter.Node, content []byte, moduleQN string, uses map[string]string) *typresolve.Type {
	if node == nil {
		return typresolve.Unknown()
	}

	switch node.Type() {
	case "type_identifier":
		name := node.Content(content)
		if isRustPrimitive(name) {
			return typresolve.Builtin(name)
		}
		if path, ok := uses[name]; ok {
			return typresolve.Named(path)
		}
		return typresolve.Named(moduleQN + "::" + name)

	case "primitive_type":
		return typresolve.Builtin(node.Content(content))

	case "scoped_type_identifier":
		// Qualified type like std::io::Result
		fullPath := node.Content(content)
		parts := strings.Split(fullPath, "::")
		if len(parts) > 0 {
			if resolved, ok := uses[parts[0]]; ok {
				parts[0] = resolved
				return typresolve.Named(strings.Join(parts, "::"))
			}
		}
		return typresolve.Named(fullPath)

	case "reference_type":
		// &T or &mut T - find the inner type child
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			childType := child.Type()
			if childType != "&" && childType != "mutable_specifier" {
				inner := ParseTypeNode(child, content, moduleQN, uses)
				return typresolve.Ref(inner)
			}
		}
		return typresolve.Ref(typresolve.Unknown())

	case "generic_type":
		// Vec<T>, HashMap<K,V>, Option<T>, Result<T,E>, Box<T>, etc.
		baseNode := node.ChildByFieldName("type")
		if baseNode == nil && node.ChildCount() > 0 {
			baseNode = node.Child(0)
		}
		baseName := ""
		if baseNode != nil {
			baseName = baseNode.Content(content)
		}

		// Collect type arguments
		var typeArgs []*typresolve.Type
		argsNode := node.ChildByFieldName("type_arguments")
		if argsNode == nil {
			// Look for type_arguments child
			for i := 0; i < int(node.ChildCount()); i++ {
				if node.Child(i).Type() == "type_arguments" {
					argsNode = node.Child(i)
					break
				}
			}
		}
		if argsNode != nil {
			for i := 0; i < int(argsNode.ChildCount()); i++ {
				child := argsNode.Child(i)
				if child.Type() != "<" && child.Type() != ">" && child.Type() != "," {
					typeArgs = append(typeArgs, ParseTypeNode(child, content, moduleQN, uses))
				}
			}
		}

		switch baseName {
		case "Option":
			if len(typeArgs) > 0 {
				return typresolve.Optional(typeArgs[0])
			}
			return typresolve.Optional(typresolve.Unknown())
		case "Vec":
			if len(typeArgs) > 0 {
				return typresolve.Slice(typeArgs[0])
			}
			return typresolve.Slice(typresolve.Unknown())
		case "Box", "Arc", "Rc":
			t := typresolve.Named("std::" + baseName)
			if len(typeArgs) > 0 {
				t.Elem = typeArgs[0]
			}
			return t
		case "Result":
			t := typresolve.Named("std::Result")
			if len(typeArgs) > 0 {
				t.Elem = typeArgs[0]
			}
			return t
		case "HashMap":
			if len(typeArgs) >= 2 {
				return typresolve.Map(typeArgs[0], typeArgs[1])
			}
			return typresolve.Map(typresolve.Unknown(), typresolve.Unknown())
		default:
			// Resolve through uses or keep as-is
			if path, ok := uses[baseName]; ok {
				return typresolve.Named(path)
			}
			return typresolve.Named(baseName)
		}

	case "array_type":
		// [T; N]
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "[" && child.Type() != "]" && child.Type() != ";" && child.Type() != "integer_literal" {
				elem := ParseTypeNode(child, content, moduleQN, uses)
				return typresolve.Array(elem)
			}
		}
		return typresolve.Array(typresolve.Unknown())

	case "slice_type":
		// [T]
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "[" && child.Type() != "]" {
				elem := ParseTypeNode(child, content, moduleQN, uses)
				return typresolve.Slice(elem)
			}
		}
		return typresolve.Slice(typresolve.Unknown())

	case "tuple_type":
		// (T1, T2, T3)
		var elems []*typresolve.Type
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "(" && child.Type() != ")" && child.Type() != "," {
				elems = append(elems, ParseTypeNode(child, content, moduleQN, uses))
			}
		}
		return typresolve.Tuple(elems)

	case "function_type":
		// fn(A) -> B
		var params []typresolve.Param
		var returns []*typresolve.Type
		paramsNode := node.ChildByFieldName("parameters")
		if paramsNode != nil {
			for i := 0; i < int(paramsNode.ChildCount()); i++ {
				child := paramsNode.Child(i)
				if child.Type() != "(" && child.Type() != ")" && child.Type() != "," {
					params = append(params, typresolve.Param{
						Type: ParseTypeNode(child, content, moduleQN, uses),
					})
				}
			}
		}
		retNode := node.ChildByFieldName("return_type")
		if retNode != nil {
			returns = append(returns, ParseTypeNode(retNode, content, moduleQN, uses))
		}
		return typresolve.Func(params, returns)

	case "unit_type":
		return typresolve.Builtin("()")

	case "pointer_type":
		// *const T or *mut T
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "*" && child.Type() != "mutable_specifier" && child.Type() != "const" {
				inner := ParseTypeNode(child, content, moduleQN, uses)
				return typresolve.Pointer(inner)
			}
		}
		return typresolve.Pointer(typresolve.Unknown())

	case "never_type":
		return typresolve.Unknown()

	case "dynamic_type":
		// dyn Trait
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "dyn" {
				traitName := child.Content(content)
				t := typresolve.Named(traitName)
				t.Kind = typresolve.KindInterface
				return t
			}
		}
		return typresolve.Unknown()

	case "bounded_type":
		// T + Trait - parse first type
		if node.ChildCount() > 0 {
			return ParseTypeNode(node.Child(0), content, moduleQN, uses)
		}
		return typresolve.Unknown()

	case "macro_invocation":
		return typresolve.Unknown()

	default:
		return typresolve.Unknown()
	}
}
