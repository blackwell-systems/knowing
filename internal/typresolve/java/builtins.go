package javaresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// builtinFuncs is the set of Java Object methods that should be skipped
// during resolution to avoid noise edges.
var builtinFuncs = map[string]bool{
	"toString":    true,
	"hashCode":    true,
	"equals":      true,
	"getClass":    true,
	"notify":      true,
	"notifyAll":   true,
	"wait":        true,
	"clone":       true,
	"finalize":    true,
	"println":     true, // System.out.println / System.err.println
}

// IsBuiltinFunc returns true if the given name is a Java method that should be
// skipped during resolution (Object methods: toString, hashCode, equals,
// getClass, notify, notifyAll, wait, clone, finalize; also println for
// System.out/err).
func IsBuiltinFunc(name string) bool {
	return builtinFuncs[name]
}

// builtinTypes is the set of Java primitives and common builtin types.
var builtinTypes = map[string]bool{
	// Primitives
	"int":     true,
	"long":    true,
	"short":   true,
	"byte":    true,
	"char":    true,
	"boolean": true,
	"float":   true,
	"double":  true,
	"void":    true,
	// Common reference types
	"String":           true,
	"Object":           true,
	"Integer":          true,
	"Long":             true,
	"Boolean":          true,
	"Double":           true,
	"Float":            true,
	"Class":            true,
	"Void":             true,
	"Byte":             true,
	"Short":            true,
	"Character":        true,
	"Number":           true,
	"Comparable":       true,
	"Serializable":     true,
	"Iterable":         true,
	"AutoCloseable":    true,
	"Throwable":        true,
	"Exception":        true,
	"RuntimeException": true,
	"Error":            true,
}

// IsBuiltinType returns true if the given name is a Java primitive or common
// builtin type (int, long, String, Object, Integer, etc.).
func IsBuiltinType(name string) bool {
	return builtinTypes[name]
}

// ResolveBuiltinType returns a typresolve.Builtin type if the name is a Java
// builtin type, nil otherwise.
func ResolveBuiltinType(name string) *typresolve.Type {
	if builtinTypes[name] {
		return typresolve.Builtin(name)
	}
	return nil
}

// LiteralType maps Java tree-sitter literal node types to builtin types.
// Returns nil if the node type is not a recognized literal.
func LiteralType(nodeType string) *typresolve.Type {
	switch nodeType {
	case "decimal_integer_literal", "hex_integer_literal",
		"octal_integer_literal", "binary_integer_literal":
		return typresolve.Builtin("int")
	case "decimal_floating_point_literal", "hex_floating_point_literal":
		return typresolve.Builtin("double")
	case "character_literal":
		return typresolve.Builtin("char")
	case "string_literal":
		return typresolve.Builtin("String")
	case "true", "false":
		return typresolve.Builtin("boolean")
	case "null_literal":
		return typresolve.Unknown()
	default:
		return nil
	}
}
