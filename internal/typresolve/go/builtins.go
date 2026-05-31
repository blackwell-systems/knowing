package goresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ExprEvaluator is a callback for recursive expression evaluation.
type ExprEvaluator func(node *sitter.Node) *typresolve.Type

// builtinFuncs is the set of Go builtin function names.
var builtinFuncs = map[string]bool{
	"make":    true,
	"new":     true,
	"append":  true,
	"len":     true,
	"cap":     true,
	"delete":  true,
	"close":   true,
	"copy":    true,
	"panic":   true,
	"recover": true,
	"print":   true,
	"println": true,
	"complex": true,
	"real":    true,
	"imag":    true,
	"min":     true,
	"max":     true,
	"clear":   true,
}

// IsBuiltinFunc returns true if the given name is a Go builtin function.
func IsBuiltinFunc(name string) bool {
	return builtinFuncs[name]
}

// builtinTypes is the set of Go builtin type names.
var builtinTypes = map[string]bool{
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
	"uintptr":    true,
	"any":        true,
}

// ResolveBuiltinType returns a typresolve.Builtin type if the name is a Go
// builtin type (int, string, bool, error, etc.), nil otherwise.
func ResolveBuiltinType(name string) *typresolve.Type {
	if builtinTypes[name] {
		return typresolve.Builtin(name)
	}
	return nil
}

// EvalBuiltinCall evaluates the return type of a Go builtin function call.
// Handles make (returns type arg), new (returns *type arg), append (returns
// slice arg type), len/cap (returns int), etc.
func EvalBuiltinCall(name string, args *sitter.Node, content []byte, pkgQN string, imports map[string]string, evalExpr ExprEvaluator) *typresolve.Type {
	switch name {
	case "make":
		// make(Type, ...): parse first arg as type node, return that type.
		if args != nil && args.NamedChildCount() > 0 {
			firstArg := args.NamedChild(0)
			return ParseTypeNode(firstArg, content, pkgQN, imports)
		}
		return typresolve.Unknown()

	case "new":
		// new(Type): parse first arg as type, return *Type.
		if args != nil && args.NamedChildCount() > 0 {
			firstArg := args.NamedChild(0)
			inner := ParseTypeNode(firstArg, content, pkgQN, imports)
			if inner != nil {
				return typresolve.Pointer(inner)
			}
		}
		return typresolve.Pointer(typresolve.Unknown())

	case "append":
		// append(slice, ...): evaluate first arg type, return it.
		if args != nil && args.NamedChildCount() > 0 && evalExpr != nil {
			firstArg := args.NamedChild(0)
			return evalExpr(firstArg)
		}
		return typresolve.Unknown()

	case "len", "cap":
		return typresolve.Builtin("int")

	case "delete", "close", "copy":
		return typresolve.Unknown()

	case "complex":
		return typresolve.Builtin("complex128")

	case "real", "imag":
		return typresolve.Builtin("float64")

	case "min", "max":
		// min/max: evaluate first arg type, return it.
		if args != nil && args.NamedChildCount() > 0 && evalExpr != nil {
			firstArg := args.NamedChild(0)
			return evalExpr(firstArg)
		}
		return typresolve.Unknown()

	default:
		return typresolve.Unknown()
	}
}
