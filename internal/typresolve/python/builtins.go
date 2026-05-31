package pyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// builtinFuncs is the set of Python builtin function names.
var builtinFuncs = map[string]bool{
	"print": true, "len": true, "range": true, "type": true,
	"isinstance": true, "issubclass": true, "hasattr": true,
	"getattr": true, "setattr": true, "delattr": true, "super": true,
	"iter": true, "next": true, "enumerate": true, "zip": true,
	"map": true, "filter": true, "sorted": true, "reversed": true,
	"min": true, "max": true, "sum": true, "abs": true,
	"round": true, "hash": true, "id": true, "repr": true,
	"str": true, "int": true, "float": true, "bool": true,
	"list": true, "dict": true, "set": true, "tuple": true,
	"frozenset": true, "bytes": true, "bytearray": true,
	"memoryview": true, "object": true, "chr": true, "ord": true,
	"hex": true, "oct": true, "bin": true, "pow": true,
	"divmod": true, "all": true, "any": true, "input": true,
	"open": true, "vars": true, "dir": true, "globals": true,
	"locals": true, "callable": true, "classmethod": true,
	"staticmethod": true, "property": true, "breakpoint": true,
	"compile": true, "eval": true, "exec": true,
}

// builtinTypes is the set of Python builtin type names.
var builtinTypes = map[string]bool{
	"int": true, "str": true, "float": true, "bool": true,
	"bytes": true, "list": true, "dict": true, "set": true,
	"tuple": true, "frozenset": true, "object": true, "type": true,
	"None": true, "complex": true, "bytearray": true,
	"memoryview": true, "range": true, "slice": true,
	"property": true, "classmethod": true, "staticmethod": true,
	"super": true, "enumerate": true, "filter": true,
	"map": true, "reversed": true, "zip": true,
}

// IsBuiltinFunc returns true if name is a Python builtin function.
func IsBuiltinFunc(name string) bool {
	return builtinFuncs[name]
}

// IsBuiltinType returns true if name is a Python builtin type.
func IsBuiltinType(name string) bool {
	return builtinTypes[name]
}

// ResolveBuiltinType returns a typresolve.Builtin type if name is a Python
// builtin type, nil otherwise.
func ResolveBuiltinType(name string) *typresolve.Type {
	if builtinTypes[name] {
		return typresolve.Builtin(name)
	}
	return nil
}

// LiteralType maps Python tree-sitter literal node types to builtin types.
// Returns nil if nodeType is not a recognized literal.
func LiteralType(nodeType string) *typresolve.Type {
	switch nodeType {
	case "integer":
		return typresolve.Builtin("int")
	case "float":
		return typresolve.Builtin("float")
	case "string", "concatenated_string":
		return typresolve.Builtin("str")
	case "true", "false":
		return typresolve.Builtin("bool")
	case "none":
		return typresolve.Builtin("None")
	case "list_comprehension":
		return typresolve.Builtin("list")
	case "dictionary_comprehension":
		return typresolve.Builtin("dict")
	case "set_comprehension":
		return typresolve.Builtin("set")
	case "generator_expression":
		return typresolve.Builtin("generator")
	default:
		return nil
	}
}

// IterableElementType returns the element type when iterating over a container
// type. For slices, returns the element type. For maps, returns the key type
// (iterating a dict yields keys). For single-element tuples, returns that
// element. Returns Unknown for other types.
func IterableElementType(iterType *typresolve.Type) *typresolve.Type {
	if iterType == nil {
		return typresolve.Unknown()
	}
	switch iterType.Kind {
	case typresolve.KindSlice:
		if iterType.Elem != nil {
			return iterType.Elem
		}
		return typresolve.Unknown()
	case typresolve.KindMap:
		if iterType.Key != nil {
			return iterType.Key
		}
		return typresolve.Unknown()
	case typresolve.KindTuple:
		if len(iterType.Elements) == 1 {
			return iterType.Elements[0]
		}
		return typresolve.Unknown()
	default:
		return typresolve.Unknown()
	}
}
