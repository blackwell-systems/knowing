package rustresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// builtinFuncs contains Rust standard macros and builtin function-like names.
var builtinFuncs = map[string]bool{
	"println": true, "eprintln": true, "print": true, "eprint": true,
	"format": true, "format_args": true,
	"vec": true,
	"todo": true, "unimplemented": true, "unreachable": true,
	"assert": true, "assert_eq": true, "assert_ne": true,
	"debug_assert": true, "debug_assert_eq": true, "debug_assert_ne": true,
	"dbg": true,
	"panic": true,
	"write": true, "writeln": true,
	"cfg": true, "env": true, "option_env": true,
	"include": true, "include_str": true, "include_bytes": true,
	"concat": true, "stringify": true,
	"file": true, "line": true, "column": true, "module_path": true,
	"compile_error": true,
	"matches": true,
	"thread_local": true,
	"Box":  true,
	"Some": true, "None": true, "Ok": true, "Err": true,
}

// builtinTypes contains Rust primitives, core types, and core traits.
var builtinTypes = map[string]bool{
	// Primitives
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true, "isize": true,
	"u8": true, "u16": true, "u32": true, "u64": true, "u128": true, "usize": true,
	"f32": true, "f64": true, "bool": true, "char": true, "str": true,
	// Core types
	"String": true, "Vec": true, "HashMap": true, "HashSet": true,
	"BTreeMap": true, "BTreeSet": true,
	"Option": true, "Result": true, "Box": true, "Rc": true, "Arc": true,
	"Cell": true, "RefCell": true, "Mutex": true, "RwLock": true,
	"Pin": true, "PhantomData": true, "Cow": true,
	// Core traits
	"Iterator": true, "IntoIterator": true, "Display": true, "Debug": true,
	"Clone": true, "Copy": true, "Default": true, "Drop": true,
	"Fn": true, "FnMut": true, "FnOnce": true,
	"Send": true, "Sync": true, "Sized": true, "Unpin": true,
	"From": true, "Into": true, "TryFrom": true, "TryInto": true,
	"AsRef": true, "AsMut": true, "Deref": true, "DerefMut": true,
	"Borrow": true, "BorrowMut": true, "ToOwned": true, "ToString": true,
	"PartialEq": true, "Eq": true, "PartialOrd": true, "Ord": true, "Hash": true,
	"Add": true, "Sub": true, "Mul": true, "Div": true, "Rem": true,
	"Neg": true, "Not": true,
	"Index": true, "IndexMut": true,
	"Read": true, "Write": true, "Seek": true, "BufRead": true,
	// IO
	"stdin": true, "stdout": true, "stderr": true,
}

// primitives is the set of Rust primitive types that resolve to Builtin.
var primitives = map[string]bool{
	"i8": true, "i16": true, "i32": true, "i64": true, "i128": true, "isize": true,
	"u8": true, "u16": true, "u32": true, "u64": true, "u128": true, "usize": true,
	"f32": true, "f64": true, "bool": true, "char": true, "str": true,
}

// IsBuiltinFunc returns true if name is a Rust builtin macro or function.
func IsBuiltinFunc(name string) bool {
	return builtinFuncs[name]
}

// IsBuiltinType returns true if name is a Rust primitive, core type, or core trait.
func IsBuiltinType(name string) bool {
	return builtinTypes[name]
}

// ResolveBuiltinType returns the typresolve.Type for a Rust builtin type.
// Primitives return Builtin, core types return Named with "std::" prefix.
// Returns nil for non-builtins.
func ResolveBuiltinType(name string) *typresolve.Type {
	if name == "()" {
		return typresolve.Builtin("()")
	}
	if primitives[name] {
		return typresolve.Builtin(name)
	}
	if builtinTypes[name] && !primitives[name] {
		return typresolve.Named("std::" + name)
	}
	return nil
}

// EvalMacroReturnType returns the known return type of common Rust macros.
// Returns Unknown for unrecognized macros or macros whose return type
// cannot be statically determined.
func EvalMacroReturnType(macroName string) *typresolve.Type {
	switch macroName {
	case "vec":
		return typresolve.Named("std::Vec")
	case "format", "format_args":
		return typresolve.Builtin("str")
	case "println", "eprintln", "print", "eprint", "write", "writeln":
		return typresolve.Builtin("()")
	case "todo", "unimplemented", "unreachable", "panic":
		return typresolve.Unknown()
	case "assert", "assert_eq", "assert_ne",
		"debug_assert", "debug_assert_eq", "debug_assert_ne":
		return typresolve.Builtin("()")
	case "dbg":
		return typresolve.Unknown()
	case "include_str":
		return typresolve.Builtin("str")
	case "include_bytes":
		return typresolve.Slice(typresolve.Builtin("u8"))
	case "concat", "stringify":
		return typresolve.Builtin("str")
	case "env", "option_env":
		return typresolve.Builtin("str")
	case "matches", "cfg":
		return typresolve.Builtin("bool")
	case "file", "module_path":
		return typresolve.Builtin("str")
	case "line", "column":
		return typresolve.Builtin("u32")
	default:
		return typresolve.Unknown()
	}
}
