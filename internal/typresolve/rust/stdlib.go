package rustresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// RegisterStdlib populates a registry with Rust standard library types and their
// common methods. This provides method resolution for std types (Vec, HashMap,
// String, Option, Result, Box, Arc, Rc, etc.) without requiring LSP enrichment.
func RegisterStdlib(reg *typresolve.Registry) {
	// --- Core types ---

	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Vec",
		ShortName:     "Vec",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::HashMap",
		ShortName:     "HashMap",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::HashSet",
		ShortName:     "HashSet",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::BTreeMap",
		ShortName:     "BTreeMap",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::BTreeSet",
		ShortName:     "BTreeSet",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::String",
		ShortName:     "String",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Option",
		ShortName:     "Option",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Result",
		ShortName:     "Result",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Box",
		ShortName:     "Box",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Arc",
		ShortName:     "Arc",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Rc",
		ShortName:     "Rc",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Mutex",
		ShortName:     "Mutex",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::RwLock",
		ShortName:     "RwLock",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Cell",
		ShortName:     "Cell",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::RefCell",
		ShortName:     "RefCell",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Cow",
		ShortName:     "Cow",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Pin",
		ShortName:     "Pin",
		IsInterface:   false,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::PhantomData",
		ShortName:     "PhantomData",
		IsInterface:   false,
	})

	// --- Core traits (registered as interfaces) ---

	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Iterator",
		ShortName:     "Iterator",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::IntoIterator",
		ShortName:     "IntoIterator",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Display",
		ShortName:     "Display",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Debug",
		ShortName:     "Debug",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Clone",
		ShortName:     "Clone",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Default",
		ShortName:     "Default",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::From",
		ShortName:     "From",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Into",
		ShortName:     "Into",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::AsRef",
		ShortName:     "AsRef",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Deref",
		ShortName:     "Deref",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Drop",
		ShortName:     "Drop",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::PartialEq",
		ShortName:     "PartialEq",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Eq",
		ShortName:     "Eq",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::PartialOrd",
		ShortName:     "PartialOrd",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Ord",
		ShortName:     "Ord",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Hash",
		ShortName:     "Hash",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Send",
		ShortName:     "Send",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Sync",
		ShortName:     "Sync",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Serialize",
		ShortName:     "Serialize",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Deserialize",
		ShortName:     "Deserialize",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "std::Future",
		ShortName:     "Future",
		IsInterface:   true,
	})

	// --- Vec methods ---

	addStdMethod(reg, "std::Vec", "new", nil, []*typresolve.Type{typresolve.Named("std::Vec")})
	addStdMethod(reg, "std::Vec", "with_capacity", nil, []*typresolve.Type{typresolve.Named("std::Vec")})
	addStdMethod(reg, "std::Vec", "push", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "pop", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Vec", "len", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::Vec", "is_empty", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Vec", "contains", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Vec", "iter", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Vec", "iter_mut", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Vec", "into_iter", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Vec", "get", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Vec", "first", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Vec", "last", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Vec", "remove", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Vec", "retain", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "clear", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "sort", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "sort_by", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "dedup", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "extend", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "truncate", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Vec", "drain", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Vec", "split_off", nil, []*typresolve.Type{typresolve.Named("std::Vec")})
	addStdMethod(reg, "std::Vec", "as_slice", nil, []*typresolve.Type{typresolve.Slice(typresolve.Unknown())})

	// --- HashMap methods ---

	addStdMethod(reg, "std::HashMap", "new", nil, []*typresolve.Type{typresolve.Named("std::HashMap")})
	addStdMethod(reg, "std::HashMap", "with_capacity", nil, []*typresolve.Type{typresolve.Named("std::HashMap")})
	addStdMethod(reg, "std::HashMap", "insert", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::HashMap", "get", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::HashMap", "get_mut", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::HashMap", "remove", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::HashMap", "contains_key", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashMap", "len", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::HashMap", "is_empty", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashMap", "keys", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::HashMap", "values", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::HashMap", "iter", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::HashMap", "entry", nil, []*typresolve.Type{typresolve.Named("std::HashMap::Entry")})
	addStdMethod(reg, "std::HashMap", "clear", nil, []*typresolve.Type{typresolve.Builtin("()")})

	// --- HashSet methods ---

	addStdMethod(reg, "std::HashSet", "new", nil, []*typresolve.Type{typresolve.Named("std::HashSet")})
	addStdMethod(reg, "std::HashSet", "insert", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashSet", "contains", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashSet", "remove", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashSet", "len", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::HashSet", "is_empty", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::HashSet", "iter", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})

	// --- String methods ---

	addStdMethod(reg, "std::String", "new", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "from", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "len", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::String", "is_empty", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::String", "push", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::String", "push_str", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::String", "contains", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::String", "starts_with", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::String", "ends_with", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::String", "trim", nil, []*typresolve.Type{typresolve.Builtin("str")})
	addStdMethod(reg, "std::String", "to_lowercase", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "to_uppercase", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "replace", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "split", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::String", "chars", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::String", "bytes", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::String", "as_str", nil, []*typresolve.Type{typresolve.Builtin("str")})
	addStdMethod(reg, "std::String", "as_bytes", nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("u8"))})
	addStdMethod(reg, "std::String", "clone", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "to_string", nil, []*typresolve.Type{typresolve.Named("std::String")})
	addStdMethod(reg, "std::String", "parse", nil, []*typresolve.Type{typresolve.Named("std::Result")})

	// --- Option methods ---

	addStdMethod(reg, "std::Option", "unwrap", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Option", "unwrap_or", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Option", "unwrap_or_default", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Option", "unwrap_or_else", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Option", "is_some", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Option", "is_none", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Option", "map", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "and_then", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "or", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "or_else", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "ok_or", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Option", "ok_or_else", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Option", "as_ref", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "take", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Option", "expect", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Result methods ---

	addStdMethod(reg, "std::Result", "unwrap", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Result", "unwrap_err", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Result", "unwrap_or", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Result", "unwrap_or_default", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Result", "unwrap_or_else", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Result", "is_ok", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Result", "is_err", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Result", "ok", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Result", "err", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Result", "map", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Result", "map_err", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Result", "and_then", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Result", "or_else", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Result", "as_ref", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Result", "expect", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Box methods ---

	addStdMethod(reg, "std::Box", "new", nil, []*typresolve.Type{typresolve.Named("std::Box")})
	addStdMethod(reg, "std::Box", "into_inner", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Arc methods ---

	addStdMethod(reg, "std::Arc", "new", nil, []*typresolve.Type{typresolve.Named("std::Arc")})
	addStdMethod(reg, "std::Arc", "clone", nil, []*typresolve.Type{typresolve.Named("std::Arc")})
	addStdMethod(reg, "std::Arc", "strong_count", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::Arc", "weak_count", nil, []*typresolve.Type{typresolve.Builtin("usize")})

	// --- Rc methods ---

	addStdMethod(reg, "std::Rc", "new", nil, []*typresolve.Type{typresolve.Named("std::Rc")})
	addStdMethod(reg, "std::Rc", "clone", nil, []*typresolve.Type{typresolve.Named("std::Rc")})
	addStdMethod(reg, "std::Rc", "strong_count", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::Rc", "weak_count", nil, []*typresolve.Type{typresolve.Builtin("usize")})

	// --- Mutex methods ---

	addStdMethod(reg, "std::Mutex", "new", nil, []*typresolve.Type{typresolve.Named("std::Mutex")})
	addStdMethod(reg, "std::Mutex", "lock", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::Mutex", "try_lock", nil, []*typresolve.Type{typresolve.Named("std::Result")})

	// --- RwLock methods ---

	addStdMethod(reg, "std::RwLock", "new", nil, []*typresolve.Type{typresolve.Named("std::RwLock")})
	addStdMethod(reg, "std::RwLock", "read", nil, []*typresolve.Type{typresolve.Named("std::Result")})
	addStdMethod(reg, "std::RwLock", "write", nil, []*typresolve.Type{typresolve.Named("std::Result")})

	// --- Iterator methods (trait methods) ---

	addStdMethod(reg, "std::Iterator", "next", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "map", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "filter", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "filter_map", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "flat_map", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "flatten", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "collect", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Iterator", "for_each", nil, []*typresolve.Type{typresolve.Builtin("()")})
	addStdMethod(reg, "std::Iterator", "count", nil, []*typresolve.Type{typresolve.Builtin("usize")})
	addStdMethod(reg, "std::Iterator", "any", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Iterator", "all", nil, []*typresolve.Type{typresolve.Builtin("bool")})
	addStdMethod(reg, "std::Iterator", "find", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "fold", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Iterator", "reduce", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "enumerate", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "zip", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "chain", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "take", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "skip", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "peekable", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "cloned", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "copied", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})
	addStdMethod(reg, "std::Iterator", "sum", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Iterator", "product", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Iterator", "min", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "max", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "last", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "nth", nil, []*typresolve.Type{typresolve.Optional(typresolve.Unknown())})
	addStdMethod(reg, "std::Iterator", "position", nil, []*typresolve.Type{typresolve.Optional(typresolve.Builtin("usize"))})
	addStdMethod(reg, "std::Iterator", "inspect", nil, []*typresolve.Type{typresolve.Named("std::Iterator")})

	// --- Clone trait ---
	addStdMethod(reg, "std::Clone", "clone", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Default trait ---
	addStdMethod(reg, "std::Default", "default", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Display trait ---
	addStdMethod(reg, "std::Display", "fmt", nil, []*typresolve.Type{typresolve.Named("std::Result")})

	// --- Debug trait ---
	addStdMethod(reg, "std::Debug", "fmt", nil, []*typresolve.Type{typresolve.Named("std::Result")})

	// --- From/Into traits ---
	addStdMethod(reg, "std::From", "from", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Into", "into", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- AsRef/Deref ---
	addStdMethod(reg, "std::AsRef", "as_ref", nil, []*typresolve.Type{typresolve.Unknown()})
	addStdMethod(reg, "std::Deref", "deref", nil, []*typresolve.Type{typresolve.Unknown()})

	// --- Future trait ---
	addStdMethod(reg, "std::Future", "poll", nil, []*typresolve.Type{typresolve.Named("std::Poll")})
}

// addStdMethod registers a method on a std type in the registry.
func addStdMethod(reg *typresolve.Registry, receiverQN, methodName string, params []typresolve.Param, returns []*typresolve.Type) {
	qn := receiverQN + "." + methodName
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: qn,
		ShortName:     methodName,
		ReceiverType:  receiverQN,
		MinParams:     -1,
		Signature:     typresolve.Func(params, returns),
	})
}

// knownDerives maps derive macro names to the traits they implement.
var knownDerives = map[string]string{
	"Debug":       "std::Debug",
	"Clone":       "std::Clone",
	"Copy":        "std::Clone", // Copy implies Clone
	"Default":     "std::Default",
	"PartialEq":   "std::PartialEq",
	"Eq":          "std::Eq",
	"PartialOrd":  "std::PartialOrd",
	"Ord":         "std::Ord",
	"Hash":        "std::Hash",
	"Serialize":   "std::Serialize",
	"Deserialize": "std::Deserialize",
}

// KnownDeriveTraitQN returns the trait QN for a known derive macro name,
// or empty string if not recognized.
func KnownDeriveTraitQN(deriveName string) string {
	return knownDerives[deriveName]
}
