package csresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// predefinedAliases maps C# keyword type names to their System.* fully
// qualified equivalents.
var predefinedAliases = map[string]string{
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
	"nint":    "System.IntPtr",
	"nuint":   "System.UIntPtr",
	"void":    "System.Void",
	"dynamic": "System.Object",
}

// PredefinedAlias returns the System.* fully qualified name for C# predefined
// type aliases (int -> System.Int32, string -> System.String, etc.).
// Returns empty string if name is not a predefined alias.
func PredefinedAlias(name string) string {
	return predefinedAliases[name]
}

// builtinFuncSet contains C# expression keywords that should not be resolved
// as user-defined calls.
var builtinFuncSet = map[string]bool{
	"typeof":     true,
	"nameof":     true,
	"sizeof":     true,
	"default":    true,
	"checked":    true,
	"unchecked":  true,
	"stackalloc": true,
}

// IsBuiltinFunc returns true if the given name is a C# expression keyword
// that should not be resolved as a user-defined call.
func IsBuiltinFunc(name string) bool {
	return builtinFuncSet[name]
}

// IsKeywordSelf returns true for C# self-referencing keywords "this" and "base".
func IsKeywordSelf(name string) bool {
	return name == "this" || name == "base"
}

// taskPrefixes lists the fully qualified names of C# async task types
// whose generic type argument should be unwrapped.
var taskPrefixes = []string{
	"System.Threading.Tasks.Task",
	"System.Threading.Tasks.ValueTask",
}

// UnwrapTask unwraps async Task<T>/ValueTask<T> types. Since typresolve.Type
// does not store generic type arguments at the Named level, this uses a
// name-based heuristic: if the type name matches a Task/ValueTask pattern,
// return Unknown (the caller resolves the concrete return type via the
// registry's function signature). For bare Task/ValueTask (void-returning
// async), also returns Unknown.
func UnwrapTask(t *typresolve.Type) *typresolve.Type {
	if t == nil || t.Kind != typresolve.KindNamed {
		return t
	}
	for _, prefix := range taskPrefixes {
		if t.Name == prefix || strings.HasPrefix(t.Name, prefix+"<") || strings.HasPrefix(t.Name, prefix+"`") {
			return typresolve.Unknown()
		}
	}
	return t
}

// UnwrapNullable unwraps Nullable<T> types. Same heuristic as UnwrapTask:
// if the type name matches System.Nullable, return Unknown (the caller
// resolves the inner type from context).
func UnwrapNullable(t *typresolve.Type) *typresolve.Type {
	if t == nil || t.Kind != typresolve.KindNamed {
		return t
	}
	if t.Name == "System.Nullable" || strings.HasPrefix(t.Name, "System.Nullable<") || strings.HasPrefix(t.Name, "System.Nullable`") {
		return typresolve.Unknown()
	}
	return t
}
