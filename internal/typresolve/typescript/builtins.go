package tsresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// builtinTypes is the set of all TypeScript/JavaScript builtin type names,
// including primitive types, wrapper classes, and common global objects.
var builtinTypes = map[string]bool{
	// Primitive types
	"string": true, "number": true, "boolean": true, "bigint": true,
	"any": true, "unknown": true, "void": true, "never": true,
	"null": true, "undefined": true, "object": true, "symbol": true,
	// Wrapper classes and common globals
	"String": true, "Number": true, "Boolean": true, "BigInt": true,
	"Symbol": true, "Object": true, "Array": true, "Promise": true,
	"Map": true, "Set": true, "WeakMap": true, "WeakSet": true,
	"Date": true, "RegExp": true, "Error": true, "Function": true,
	"Math": true, "JSON": true, "console": true,
}

// primitiveTypes is the subset of builtins that are true primitives
// (lowercase types that resolve to typresolve.Builtin).
var primitiveTypes = map[string]bool{
	"string": true, "number": true, "boolean": true, "bigint": true,
	"any": true, "unknown": true, "void": true, "never": true,
	"null": true, "undefined": true, "object": true, "symbol": true,
}

// IsBuiltinType returns true if name is a TypeScript/JavaScript builtin type.
func IsBuiltinType(name string) bool {
	return builtinTypes[name]
}

// ResolveBuiltinType returns a typresolve.Builtin type for primitive types
// (string, number, boolean, etc.). Returns nil for non-primitive builtins
// like Array or Promise, which are Named types with methods.
func ResolveBuiltinType(name string) *typresolve.Type {
	if primitiveTypes[name] {
		return typresolve.Builtin(name)
	}
	return nil
}

// BuiltinWrapperClass maps builtin primitive type names to their wrapper
// class for method dispatch. For example, "string" maps to "String" so
// that "hello".toUpperCase() can find methods on the String prototype.
// Returns empty string if the builtin has no wrapper class.
func BuiltinWrapperClass(builtinName string) string {
	switch builtinName {
	case "string":
		return "String"
	case "number":
		return "Number"
	case "boolean":
		return "Boolean"
	case "bigint":
		return "BigInt"
	case "symbol":
		return "Symbol"
	default:
		return ""
	}
}

// LiteralType maps tree-sitter literal node types to their corresponding
// builtin types. Returns nil if the node type is not a recognized literal.
func LiteralType(nodeType string) *typresolve.Type {
	switch nodeType {
	case "string", "template_string":
		return typresolve.Builtin("string")
	case "number":
		return typresolve.Builtin("number")
	case "true", "false":
		return typresolve.Builtin("boolean")
	case "null":
		return typresolve.Builtin("null")
	case "undefined":
		return typresolve.Builtin("undefined")
	case "regex":
		return typresolve.Named("RegExp")
	default:
		return nil
	}
}

// UnwrapPromise extracts the inner type T from a Promise<T> type.
// Promise<T> is represented as Named("Promise") with Elem = T.
// If the type is Named("Promise") without an Elem, returns Unknown.
// For non-Promise types, returns the input unchanged.
func UnwrapPromise(t *typresolve.Type) *typresolve.Type {
	if t != nil && t.Kind == typresolve.KindNamed && t.Name == "Promise" {
		if t.Elem != nil {
			return t.Elem
		}
		return typresolve.Unknown()
	}
	return t
}

// passthroughUtilities contains TypeScript utility types that pass through
// to their inner type for method dispatch purposes.
var passthroughUtilities = map[string]bool{
	"Partial":     true,
	"Required":    true,
	"Readonly":    true,
	"NonNullable": true,
	"Pick":        true,
	"Omit":        true,
	"Awaited":     true,
}

// IsPassthroughUtility returns true for TypeScript utility types that pass
// through to their inner type for method dispatch: Partial, Required,
// Readonly, NonNullable, Pick, Omit, Awaited.
func IsPassthroughUtility(name string) bool {
	return passthroughUtilities[name]
}
