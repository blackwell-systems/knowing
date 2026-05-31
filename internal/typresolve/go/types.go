package goresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// goBuiltinTypes is the set of Go builtin type names used by
// isBuiltinType. This is a local copy to avoid depending on Agent B's
// builtins.go (which is built in the same wave). Agent D will wire the
// final integration in wave 2.
var goBuiltinTypes = map[string]bool{
	"int":        true,
	"int8":       true,
	"int16":      true,
	"int32":      true,
	"int64":      true,
	"uint":       true,
	"uint8":      true,
	"uint16":     true,
	"uint32":     true,
	"uint64":     true,
	"float32":    true,
	"float64":    true,
	"complex64":  true,
	"complex128": true,
	"string":     true,
	"bool":       true,
	"byte":       true,
	"rune":       true,
	"error":      true,
	"any":        true,
	"uintptr":    true,
}

// isBuiltinType reports whether name is a Go builtin type.
func isBuiltinType(name string) bool {
	return goBuiltinTypes[name]
}

// ParseTypeNode converts a tree-sitter Go type AST node to a
// typresolve.Type. It handles all major Go type expression forms:
// type_identifier, qualified_type, pointer_type, slice_type, array_type,
// map_type, channel_type, function_type, interface_type, struct_type,
// parenthesized_type, type_elem, generic_type, and parameter_list
// (multi-return).
//
// pkgQN is the fully qualified package name (e.g. "net/http") used to
// qualify non-builtin named types. imports maps import aliases to their
// full package paths for qualified_type resolution.
func ParseTypeNode(node *sitter.Node, content []byte, pkgQN string, imports map[string]string) *typresolve.Type {
	if node == nil {
		return nil
	}

	switch node.Type() {
	case "type_identifier":
		name := node.Content(content)
		if isBuiltinType(name) {
			return typresolve.Builtin(name)
		}
		return typresolve.Named(pkgQN + "." + name)

	case "qualified_type":
		pkgNode := node.ChildByFieldName("package")
		nameNode := node.ChildByFieldName("name")
		if pkgNode == nil || nameNode == nil {
			return nil
		}
		alias := pkgNode.Content(content)
		typeName := nameNode.Content(content)
		if resolved, ok := imports[alias]; ok {
			return typresolve.Named(resolved + "." + typeName)
		}
		// Fallback: use alias as package path.
		return typresolve.Named(alias + "." + typeName)

	case "pointer_type":
		inner := ParseTypeNode(node.NamedChild(0), content, pkgQN, imports)
		if inner == nil {
			return nil
		}
		return typresolve.Pointer(inner)

	case "slice_type":
		elem := node.ChildByFieldName("element")
		if elem == nil && node.NamedChildCount() > 0 {
			elem = node.NamedChild(0)
		}
		inner := ParseTypeNode(elem, content, pkgQN, imports)
		if inner == nil {
			return nil
		}
		return typresolve.Slice(inner)

	case "array_type":
		elem := node.ChildByFieldName("element")
		if elem == nil && node.NamedChildCount() > 0 {
			// The element is typically the last named child (after the length).
			elem = node.NamedChild(int(node.NamedChildCount()) - 1)
		}
		inner := ParseTypeNode(elem, content, pkgQN, imports)
		if inner == nil {
			return nil
		}
		return typresolve.Slice(inner)

	case "map_type":
		keyNode := node.ChildByFieldName("key")
		valNode := node.ChildByFieldName("value")
		key := ParseTypeNode(keyNode, content, pkgQN, imports)
		val := ParseTypeNode(valNode, content, pkgQN, imports)
		if key == nil {
			key = typresolve.Unknown()
		}
		if val == nil {
			val = typresolve.Unknown()
		}
		return typresolve.Map(key, val)

	case "channel_type":
		// Determine direction from the text.
		text := node.Content(content)
		var dir typresolve.ChanDir
		switch {
		case strings.HasPrefix(text, "<-chan"):
			dir = typresolve.ChanRecv
		case strings.HasPrefix(text, "chan<-"):
			dir = typresolve.ChanSend
		default:
			dir = typresolve.ChanBidi
		}
		// The value/element is the last named child.
		var elemNode *sitter.Node
		if nc := node.NamedChildCount(); nc > 0 {
			elemNode = node.NamedChild(int(nc) - 1)
		}
		elem := ParseTypeNode(elemNode, content, pkgQN, imports)
		if elem == nil {
			elem = typresolve.Unknown()
		}
		return typresolve.Channel(elem, dir)

	case "function_type":
		return typresolve.Func(nil, nil)

	case "interface_type":
		return &typresolve.Type{Kind: typresolve.KindInterface}

	case "struct_type":
		return &typresolve.Type{Kind: typresolve.KindStruct}

	case "parenthesized_type", "type_elem":
		if node.NamedChildCount() > 0 {
			return ParseTypeNode(node.NamedChild(0), content, pkgQN, imports)
		}
		return nil

	case "generic_type":
		typeChild := node.ChildByFieldName("type")
		if typeChild != nil {
			return ParseTypeNode(typeChild, content, pkgQN, imports)
		}
		if node.NamedChildCount() > 0 {
			return ParseTypeNode(node.NamedChild(0), content, pkgQN, imports)
		}
		return nil

	case "parameter_list":
		// Multi-return: collect parameter_declaration types.
		var elems []*typresolve.Type
		for i := 0; i < int(node.NamedChildCount()); i++ {
			param := node.NamedChild(i)
			if param.Type() != "parameter_declaration" {
				continue
			}
			typeNode := param.ChildByFieldName("type")
			if typeNode == nil {
				continue
			}
			t := ParseTypeNode(typeNode, content, pkgQN, imports)
			if t != nil {
				elems = append(elems, t)
			}
		}
		if len(elems) == 1 {
			return elems[0]
		}
		if len(elems) > 1 {
			return typresolve.Tuple(elems)
		}
		return nil

	default:
		return nil
	}
}
