package pyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// RegisterStdlib pre-registers Python stdlib modules, functions, classes, and
// methods into the provided registry. This enables call resolution and type
// evaluation for code that uses the standard library without requiring a full
// typeshed parse.
//
// Coverage: os, sys, collections, pathlib, json, re, typing, io, builtins
// (methods on str, list, dict, set, int, float, bytes, tuple, bool, object).
func RegisterStdlib(reg *typresolve.Registry) {
	// --- builtins module functions ---
	builtinFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"print", typresolve.Builtin("None")},
		{"len", typresolve.Builtin("int")},
		{"range", typresolve.Named("builtins.range")},
		{"type", typresolve.Named("builtins.type")},
		{"isinstance", typresolve.Builtin("bool")},
		{"issubclass", typresolve.Builtin("bool")},
		{"hasattr", typresolve.Builtin("bool")},
		{"getattr", nil},
		{"setattr", typresolve.Builtin("None")},
		{"delattr", typresolve.Builtin("None")},
		{"iter", nil},
		{"next", nil},
		{"enumerate", nil},
		{"zip", nil},
		{"map", nil},
		{"filter", nil},
		{"sorted", typresolve.Builtin("list")},
		{"reversed", nil},
		{"min", nil},
		{"max", nil},
		{"sum", typresolve.Builtin("int")},
		{"abs", nil},
		{"round", typresolve.Builtin("int")},
		{"hash", typresolve.Builtin("int")},
		{"id", typresolve.Builtin("int")},
		{"repr", typresolve.Builtin("str")},
		{"str", typresolve.Builtin("str")},
		{"int", typresolve.Builtin("int")},
		{"float", typresolve.Builtin("float")},
		{"bool", typresolve.Builtin("bool")},
		{"list", typresolve.Builtin("list")},
		{"dict", typresolve.Builtin("dict")},
		{"set", typresolve.Builtin("set")},
		{"tuple", typresolve.Builtin("tuple")},
		{"frozenset", typresolve.Named("builtins.frozenset")},
		{"bytes", typresolve.Builtin("bytes")},
		{"bytearray", typresolve.Named("builtins.bytearray")},
		{"memoryview", typresolve.Named("builtins.memoryview")},
		{"object", typresolve.Named("builtins.object")},
		{"chr", typresolve.Builtin("str")},
		{"ord", typresolve.Builtin("int")},
		{"hex", typresolve.Builtin("str")},
		{"oct", typresolve.Builtin("str")},
		{"bin", typresolve.Builtin("str")},
		{"pow", typresolve.Builtin("int")},
		{"divmod", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("int")})},
		{"all", typresolve.Builtin("bool")},
		{"any", typresolve.Builtin("bool")},
		{"input", typresolve.Builtin("str")},
		{"open", typresolve.Named("io.TextIOWrapper")},
		{"vars", typresolve.Builtin("dict")},
		{"dir", typresolve.Builtin("list")},
		{"globals", typresolve.Builtin("dict")},
		{"locals", typresolve.Builtin("dict")},
		{"callable", typresolve.Builtin("bool")},
		{"format", typresolve.Builtin("str")},
		{"ascii", typresolve.Builtin("str")},
		{"super", nil}, // handled specially
		{"compile", typresolve.Named("builtins.code")},
		{"eval", nil},
		{"exec", typresolve.Builtin("None")},
		{"breakpoint", typresolve.Builtin("None")},
	}

	for _, f := range builtinFunctions {
		var sig *typresolve.Type
		if f.ret != nil {
			sig = typresolve.Func(nil, []*typresolve.Type{f.ret})
		}
		reg.AddFunc(typresolve.RegisteredFunc{
			QualifiedName: "builtins." + f.name,
			ShortName:     f.name,
			Signature:     sig,
			MinParams:     -1,
		})
	}

	// --- builtins types ---
	builtinTypes := []string{
		"int", "str", "float", "bool", "bytes", "list", "dict", "set",
		"tuple", "frozenset", "object", "type", "None", "complex",
		"bytearray", "memoryview", "range", "slice", "property",
		"classmethod", "staticmethod", "super", "enumerate", "filter",
		"map", "reversed", "zip",
	}
	for _, name := range builtinTypes {
		reg.AddType(typresolve.RegisteredType{
			QualifiedName: "builtins." + name,
			ShortName:     name,
		})
	}

	// --- str methods ---
	strMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"upper", typresolve.Builtin("str")},
		{"lower", typresolve.Builtin("str")},
		{"strip", typresolve.Builtin("str")},
		{"lstrip", typresolve.Builtin("str")},
		{"rstrip", typresolve.Builtin("str")},
		{"split", typresolve.Slice(typresolve.Builtin("str"))},
		{"rsplit", typresolve.Slice(typresolve.Builtin("str"))},
		{"splitlines", typresolve.Slice(typresolve.Builtin("str"))},
		{"join", typresolve.Builtin("str")},
		{"replace", typresolve.Builtin("str")},
		{"find", typresolve.Builtin("int")},
		{"rfind", typresolve.Builtin("int")},
		{"index", typresolve.Builtin("int")},
		{"rindex", typresolve.Builtin("int")},
		{"count", typresolve.Builtin("int")},
		{"startswith", typresolve.Builtin("bool")},
		{"endswith", typresolve.Builtin("bool")},
		{"isdigit", typresolve.Builtin("bool")},
		{"isalpha", typresolve.Builtin("bool")},
		{"isalnum", typresolve.Builtin("bool")},
		{"isspace", typresolve.Builtin("bool")},
		{"isupper", typresolve.Builtin("bool")},
		{"islower", typresolve.Builtin("bool")},
		{"istitle", typresolve.Builtin("bool")},
		{"isascii", typresolve.Builtin("bool")},
		{"isdecimal", typresolve.Builtin("bool")},
		{"isnumeric", typresolve.Builtin("bool")},
		{"isidentifier", typresolve.Builtin("bool")},
		{"isprintable", typresolve.Builtin("bool")},
		{"title", typresolve.Builtin("str")},
		{"capitalize", typresolve.Builtin("str")},
		{"casefold", typresolve.Builtin("str")},
		{"swapcase", typresolve.Builtin("str")},
		{"center", typresolve.Builtin("str")},
		{"ljust", typresolve.Builtin("str")},
		{"rjust", typresolve.Builtin("str")},
		{"zfill", typresolve.Builtin("str")},
		{"format", typresolve.Builtin("str")},
		{"format_map", typresolve.Builtin("str")},
		{"encode", typresolve.Builtin("bytes")},
		{"expandtabs", typresolve.Builtin("str")},
		{"maketrans", typresolve.Builtin("dict")},
		{"translate", typresolve.Builtin("str")},
		{"partition", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("str"), typresolve.Builtin("str")})},
		{"rpartition", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("str"), typresolve.Builtin("str")})},
		{"removeprefix", typresolve.Builtin("str")},
		{"removesuffix", typresolve.Builtin("str")},
	}
	registerMethods(reg, "builtins.str", strMethods)

	// --- list methods ---
	listMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"append", typresolve.Builtin("None")},
		{"extend", typresolve.Builtin("None")},
		{"insert", typresolve.Builtin("None")},
		{"remove", typresolve.Builtin("None")},
		{"pop", nil},
		{"clear", typresolve.Builtin("None")},
		{"index", typresolve.Builtin("int")},
		{"count", typresolve.Builtin("int")},
		{"sort", typresolve.Builtin("None")},
		{"reverse", typresolve.Builtin("None")},
		{"copy", typresolve.Builtin("list")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
		{"__iter__", nil},
		{"__getitem__", nil},
		{"__setitem__", typresolve.Builtin("None")},
	}
	registerMethods(reg, "builtins.list", listMethods)

	// --- dict methods ---
	dictMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"keys", nil},
		{"values", nil},
		{"items", nil},
		{"get", nil},
		{"pop", nil},
		{"setdefault", nil},
		{"update", typresolve.Builtin("None")},
		{"clear", typresolve.Builtin("None")},
		{"copy", typresolve.Builtin("dict")},
		{"fromkeys", typresolve.Builtin("dict")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
		{"__iter__", nil},
		{"__getitem__", nil},
		{"__setitem__", typresolve.Builtin("None")},
		{"__delitem__", typresolve.Builtin("None")},
	}
	registerMethods(reg, "builtins.dict", dictMethods)

	// --- set methods ---
	setMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"add", typresolve.Builtin("None")},
		{"remove", typresolve.Builtin("None")},
		{"discard", typresolve.Builtin("None")},
		{"pop", nil},
		{"clear", typresolve.Builtin("None")},
		{"copy", typresolve.Named("builtins.set")},
		{"union", typresolve.Named("builtins.set")},
		{"intersection", typresolve.Named("builtins.set")},
		{"difference", typresolve.Named("builtins.set")},
		{"symmetric_difference", typresolve.Named("builtins.set")},
		{"issubset", typresolve.Builtin("bool")},
		{"issuperset", typresolve.Builtin("bool")},
		{"isdisjoint", typresolve.Builtin("bool")},
		{"update", typresolve.Builtin("None")},
		{"intersection_update", typresolve.Builtin("None")},
		{"difference_update", typresolve.Builtin("None")},
		{"symmetric_difference_update", typresolve.Builtin("None")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
		{"__iter__", nil},
	}
	registerMethods(reg, "builtins.set", setMethods)

	// --- int methods ---
	intMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"bit_length", typresolve.Builtin("int")},
		{"bit_count", typresolve.Builtin("int")},
		{"to_bytes", typresolve.Builtin("bytes")},
		{"from_bytes", typresolve.Builtin("int")},
		{"conjugate", typresolve.Builtin("int")},
		{"__abs__", typresolve.Builtin("int")},
		{"__add__", typresolve.Builtin("int")},
		{"__sub__", typresolve.Builtin("int")},
		{"__mul__", typresolve.Builtin("int")},
		{"__truediv__", typresolve.Builtin("float")},
		{"__floordiv__", typresolve.Builtin("int")},
		{"__mod__", typresolve.Builtin("int")},
		{"__pow__", typresolve.Builtin("int")},
		{"__neg__", typresolve.Builtin("int")},
		{"__pos__", typresolve.Builtin("int")},
		{"__invert__", typresolve.Builtin("int")},
		{"__and__", typresolve.Builtin("int")},
		{"__or__", typresolve.Builtin("int")},
		{"__xor__", typresolve.Builtin("int")},
		{"__lshift__", typresolve.Builtin("int")},
		{"__rshift__", typresolve.Builtin("int")},
		{"__bool__", typresolve.Builtin("bool")},
		{"__str__", typresolve.Builtin("str")},
		{"__repr__", typresolve.Builtin("str")},
		{"__hash__", typresolve.Builtin("int")},
		{"__eq__", typresolve.Builtin("bool")},
		{"__ne__", typresolve.Builtin("bool")},
		{"__lt__", typresolve.Builtin("bool")},
		{"__le__", typresolve.Builtin("bool")},
		{"__gt__", typresolve.Builtin("bool")},
		{"__ge__", typresolve.Builtin("bool")},
	}
	registerMethods(reg, "builtins.int", intMethods)

	// --- float methods ---
	floatMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"is_integer", typresolve.Builtin("bool")},
		{"hex", typresolve.Builtin("str")},
		{"fromhex", typresolve.Builtin("float")},
		{"as_integer_ratio", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("int")})},
		{"conjugate", typresolve.Builtin("float")},
		{"__abs__", typresolve.Builtin("float")},
		{"__add__", typresolve.Builtin("float")},
		{"__sub__", typresolve.Builtin("float")},
		{"__mul__", typresolve.Builtin("float")},
		{"__truediv__", typresolve.Builtin("float")},
		{"__floordiv__", typresolve.Builtin("float")},
		{"__mod__", typresolve.Builtin("float")},
		{"__pow__", typresolve.Builtin("float")},
		{"__neg__", typresolve.Builtin("float")},
		{"__bool__", typresolve.Builtin("bool")},
		{"__str__", typresolve.Builtin("str")},
		{"__repr__", typresolve.Builtin("str")},
		{"__hash__", typresolve.Builtin("int")},
		{"__int__", typresolve.Builtin("int")},
		{"__round__", typresolve.Builtin("int")},
	}
	registerMethods(reg, "builtins.float", floatMethods)

	// --- bytes methods ---
	bytesMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"decode", typresolve.Builtin("str")},
		{"hex", typresolve.Builtin("str")},
		{"count", typresolve.Builtin("int")},
		{"find", typresolve.Builtin("int")},
		{"rfind", typresolve.Builtin("int")},
		{"index", typresolve.Builtin("int")},
		{"rindex", typresolve.Builtin("int")},
		{"startswith", typresolve.Builtin("bool")},
		{"endswith", typresolve.Builtin("bool")},
		{"isdigit", typresolve.Builtin("bool")},
		{"isalpha", typresolve.Builtin("bool")},
		{"isalnum", typresolve.Builtin("bool")},
		{"isspace", typresolve.Builtin("bool")},
		{"isupper", typresolve.Builtin("bool")},
		{"islower", typresolve.Builtin("bool")},
		{"isascii", typresolve.Builtin("bool")},
		{"upper", typresolve.Builtin("bytes")},
		{"lower", typresolve.Builtin("bytes")},
		{"strip", typresolve.Builtin("bytes")},
		{"lstrip", typresolve.Builtin("bytes")},
		{"rstrip", typresolve.Builtin("bytes")},
		{"split", typresolve.Slice(typresolve.Builtin("bytes"))},
		{"rsplit", typresolve.Slice(typresolve.Builtin("bytes"))},
		{"splitlines", typresolve.Slice(typresolve.Builtin("bytes"))},
		{"join", typresolve.Builtin("bytes")},
		{"replace", typresolve.Builtin("bytes")},
		{"title", typresolve.Builtin("bytes")},
		{"capitalize", typresolve.Builtin("bytes")},
		{"center", typresolve.Builtin("bytes")},
		{"ljust", typresolve.Builtin("bytes")},
		{"rjust", typresolve.Builtin("bytes")},
		{"zfill", typresolve.Builtin("bytes")},
		{"expandtabs", typresolve.Builtin("bytes")},
		{"translate", typresolve.Builtin("bytes")},
		{"partition", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("bytes"), typresolve.Builtin("bytes"), typresolve.Builtin("bytes")})},
		{"rpartition", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("bytes"), typresolve.Builtin("bytes"), typresolve.Builtin("bytes")})},
		{"removeprefix", typresolve.Builtin("bytes")},
		{"removesuffix", typresolve.Builtin("bytes")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
		{"__iter__", nil},
	}
	registerMethods(reg, "builtins.bytes", bytesMethods)

	// --- tuple methods ---
	tupleMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"count", typresolve.Builtin("int")},
		{"index", typresolve.Builtin("int")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
		{"__iter__", nil},
		{"__getitem__", nil},
		{"__hash__", typresolve.Builtin("int")},
	}
	registerMethods(reg, "builtins.tuple", tupleMethods)

	// --- bool methods (inherits int) ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "builtins.bool",
		ShortName:     "bool",
		EmbeddedTypes: []string{"builtins.int"},
	})

	// --- object methods ---
	objectMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"__init__", typresolve.Builtin("None")},
		{"__str__", typresolve.Builtin("str")},
		{"__repr__", typresolve.Builtin("str")},
		{"__hash__", typresolve.Builtin("int")},
		{"__eq__", typresolve.Builtin("bool")},
		{"__ne__", typresolve.Builtin("bool")},
		{"__class__", nil},
		{"__sizeof__", typresolve.Builtin("int")},
		{"__dir__", typresolve.Builtin("list")},
		{"__format__", typresolve.Builtin("str")},
		{"__init_subclass__", typresolve.Builtin("None")},
		{"__subclasshook__", typresolve.Builtin("bool")},
	}
	registerMethods(reg, "builtins.object", objectMethods)

	// --- os module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "os", ShortName: "os"})
	osFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"getcwd", typresolve.Builtin("str")},
		{"listdir", typresolve.Slice(typresolve.Builtin("str"))},
		{"makedirs", typresolve.Builtin("None")},
		{"mkdir", typresolve.Builtin("None")},
		{"remove", typresolve.Builtin("None")},
		{"rmdir", typresolve.Builtin("None")},
		{"rename", typresolve.Builtin("None")},
		{"stat", typresolve.Named("os.stat_result")},
		{"walk", nil},
		{"getenv", typresolve.Optional(typresolve.Builtin("str"))},
		{"environ", typresolve.Builtin("dict")},
		{"path", nil},
		{"sep", typresolve.Builtin("str")},
		{"linesep", typresolve.Builtin("str")},
		{"devnull", typresolve.Builtin("str")},
		{"urandom", typresolve.Builtin("bytes")},
		{"cpu_count", typresolve.Optional(typresolve.Builtin("int"))},
		{"getpid", typresolve.Builtin("int")},
		{"getppid", typresolve.Builtin("int")},
		{"kill", typresolve.Builtin("None")},
		{"system", typresolve.Builtin("int")},
		{"chdir", typresolve.Builtin("None")},
		{"chmod", typresolve.Builtin("None")},
		{"link", typresolve.Builtin("None")},
		{"symlink", typresolve.Builtin("None")},
		{"readlink", typresolve.Builtin("str")},
		{"isatty", typresolve.Builtin("bool")},
	}
	registerFunctions(reg, "os", osFunctions)
	reg.AddType(typresolve.RegisteredType{QualifiedName: "os.stat_result", ShortName: "stat_result"})

	// --- os.path module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "os.path", ShortName: "path"})
	osPathFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"join", typresolve.Builtin("str")},
		{"exists", typresolve.Builtin("bool")},
		{"isfile", typresolve.Builtin("bool")},
		{"isdir", typresolve.Builtin("bool")},
		{"isabs", typresolve.Builtin("bool")},
		{"islink", typresolve.Builtin("bool")},
		{"basename", typresolve.Builtin("str")},
		{"dirname", typresolve.Builtin("str")},
		{"abspath", typresolve.Builtin("str")},
		{"realpath", typresolve.Builtin("str")},
		{"relpath", typresolve.Builtin("str")},
		{"normpath", typresolve.Builtin("str")},
		{"expanduser", typresolve.Builtin("str")},
		{"split", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("str")})},
		{"splitext", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("str")})},
		{"getsize", typresolve.Builtin("int")},
		{"getmtime", typresolve.Builtin("float")},
		{"getatime", typresolve.Builtin("float")},
		{"getctime", typresolve.Builtin("float")},
	}
	registerFunctions(reg, "os.path", osPathFunctions)

	// --- sys module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "sys", ShortName: "sys"})
	sysFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"exit", typresolve.Builtin("None")},
		{"getrecursionlimit", typresolve.Builtin("int")},
		{"setrecursionlimit", typresolve.Builtin("None")},
		{"getsizeof", typresolve.Builtin("int")},
		{"getdefaultencoding", typresolve.Builtin("str")},
		{"getfilesystemencoding", typresolve.Builtin("str")},
		{"intern", typresolve.Builtin("str")},
		{"exc_info", typresolve.Tuple([]*typresolve.Type{nil, nil, nil})},
	}
	registerFunctions(reg, "sys", sysFunctions)

	// --- collections module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections", ShortName: "collections"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections.OrderedDict", ShortName: "OrderedDict", EmbeddedTypes: []string{"builtins.dict"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections.defaultdict", ShortName: "defaultdict", EmbeddedTypes: []string{"builtins.dict"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections.Counter", ShortName: "Counter", EmbeddedTypes: []string{"builtins.dict"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections.deque", ShortName: "deque"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "collections.namedtuple", ShortName: "namedtuple"})
	dequeMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"append", typresolve.Builtin("None")},
		{"appendleft", typresolve.Builtin("None")},
		{"pop", nil},
		{"popleft", nil},
		{"extend", typresolve.Builtin("None")},
		{"extendleft", typresolve.Builtin("None")},
		{"rotate", typresolve.Builtin("None")},
		{"clear", typresolve.Builtin("None")},
		{"copy", typresolve.Named("collections.deque")},
		{"count", typresolve.Builtin("int")},
		{"index", typresolve.Builtin("int")},
		{"remove", typresolve.Builtin("None")},
		{"reverse", typresolve.Builtin("None")},
		{"__len__", typresolve.Builtin("int")},
		{"__contains__", typresolve.Builtin("bool")},
	}
	registerMethods(reg, "collections.deque", dequeMethods)

	// --- pathlib module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib", ShortName: "pathlib"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.Path", ShortName: "Path"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.PurePath", ShortName: "PurePath"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.PurePosixPath", ShortName: "PurePosixPath", EmbeddedTypes: []string{"pathlib.PurePath"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.PureWindowsPath", ShortName: "PureWindowsPath", EmbeddedTypes: []string{"pathlib.PurePath"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.PosixPath", ShortName: "PosixPath", EmbeddedTypes: []string{"pathlib.Path"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "pathlib.WindowsPath", ShortName: "WindowsPath", EmbeddedTypes: []string{"pathlib.Path"}})
	pathMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"exists", typresolve.Builtin("bool")},
		{"is_file", typresolve.Builtin("bool")},
		{"is_dir", typresolve.Builtin("bool")},
		{"is_symlink", typresolve.Builtin("bool")},
		{"is_absolute", typresolve.Builtin("bool")},
		{"is_relative_to", typresolve.Builtin("bool")},
		{"resolve", typresolve.Named("pathlib.Path")},
		{"absolute", typresolve.Named("pathlib.Path")},
		{"parent", typresolve.Named("pathlib.Path")},
		{"name", typresolve.Builtin("str")},
		{"stem", typresolve.Builtin("str")},
		{"suffix", typresolve.Builtin("str")},
		{"suffixes", typresolve.Slice(typresolve.Builtin("str"))},
		{"parts", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str")})},
		{"as_posix", typresolve.Builtin("str")},
		{"as_uri", typresolve.Builtin("str")},
		{"stat", typresolve.Named("os.stat_result")},
		{"read_text", typresolve.Builtin("str")},
		{"read_bytes", typresolve.Builtin("bytes")},
		{"write_text", typresolve.Builtin("int")},
		{"write_bytes", typresolve.Builtin("int")},
		{"open", typresolve.Named("io.TextIOWrapper")},
		{"mkdir", typresolve.Builtin("None")},
		{"rmdir", typresolve.Builtin("None")},
		{"unlink", typresolve.Builtin("None")},
		{"rename", typresolve.Named("pathlib.Path")},
		{"replace", typresolve.Named("pathlib.Path")},
		{"glob", typresolve.Slice(typresolve.Named("pathlib.Path"))},
		{"rglob", typresolve.Slice(typresolve.Named("pathlib.Path"))},
		{"iterdir", typresolve.Slice(typresolve.Named("pathlib.Path"))},
		{"joinpath", typresolve.Named("pathlib.Path")},
		{"with_name", typresolve.Named("pathlib.Path")},
		{"with_stem", typresolve.Named("pathlib.Path")},
		{"with_suffix", typresolve.Named("pathlib.Path")},
		{"__truediv__", typresolve.Named("pathlib.Path")},
		{"__str__", typresolve.Builtin("str")},
		{"__repr__", typresolve.Builtin("str")},
	}
	registerMethods(reg, "pathlib.Path", pathMethods)

	// --- json module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "json", ShortName: "json"})
	jsonFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"dumps", typresolve.Builtin("str")},
		{"loads", nil},
		{"dump", typresolve.Builtin("None")},
		{"load", nil},
		{"JSONDecodeError", typresolve.Named("json.JSONDecodeError")},
	}
	registerFunctions(reg, "json", jsonFunctions)
	reg.AddType(typresolve.RegisteredType{QualifiedName: "json.JSONDecodeError", ShortName: "JSONDecodeError"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "json.JSONEncoder", ShortName: "JSONEncoder"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "json.JSONDecoder", ShortName: "JSONDecoder"})

	// --- re module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "re", ShortName: "re"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "re.Pattern", ShortName: "Pattern"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "re.Match", ShortName: "Match"})
	reFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"compile", typresolve.Named("re.Pattern")},
		{"match", typresolve.Optional(typresolve.Named("re.Match"))},
		{"search", typresolve.Optional(typresolve.Named("re.Match"))},
		{"fullmatch", typresolve.Optional(typresolve.Named("re.Match"))},
		{"findall", typresolve.Slice(typresolve.Builtin("str"))},
		{"finditer", nil},
		{"sub", typresolve.Builtin("str")},
		{"subn", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("int")})},
		{"split", typresolve.Slice(typresolve.Builtin("str"))},
		{"escape", typresolve.Builtin("str")},
		{"purge", typresolve.Builtin("None")},
	}
	registerFunctions(reg, "re", reFunctions)
	rePatternMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"match", typresolve.Optional(typresolve.Named("re.Match"))},
		{"search", typresolve.Optional(typresolve.Named("re.Match"))},
		{"fullmatch", typresolve.Optional(typresolve.Named("re.Match"))},
		{"findall", typresolve.Slice(typresolve.Builtin("str"))},
		{"finditer", nil},
		{"sub", typresolve.Builtin("str")},
		{"subn", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str"), typresolve.Builtin("int")})},
		{"split", typresolve.Slice(typresolve.Builtin("str"))},
		{"pattern", typresolve.Builtin("str")},
		{"flags", typresolve.Builtin("int")},
		{"groups", typresolve.Builtin("int")},
	}
	registerMethods(reg, "re.Pattern", rePatternMethods)
	reMatchMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"group", typresolve.Builtin("str")},
		{"groups", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("str")})},
		{"groupdict", typresolve.Builtin("dict")},
		{"start", typresolve.Builtin("int")},
		{"end", typresolve.Builtin("int")},
		{"span", typresolve.Tuple([]*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("int")})},
		{"expand", typresolve.Builtin("str")},
		{"string", typresolve.Builtin("str")},
		{"re", typresolve.Named("re.Pattern")},
		{"pos", typresolve.Builtin("int")},
		{"endpos", typresolve.Builtin("int")},
		{"lastindex", typresolve.Optional(typresolve.Builtin("int"))},
		{"lastgroup", typresolve.Optional(typresolve.Builtin("str"))},
	}
	registerMethods(reg, "re.Match", reMatchMethods)

	// --- io module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io", ShortName: "io"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.IOBase", ShortName: "IOBase"})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.RawIOBase", ShortName: "RawIOBase", EmbeddedTypes: []string{"io.IOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.BufferedIOBase", ShortName: "BufferedIOBase", EmbeddedTypes: []string{"io.IOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.TextIOBase", ShortName: "TextIOBase", EmbeddedTypes: []string{"io.IOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.TextIOWrapper", ShortName: "TextIOWrapper", EmbeddedTypes: []string{"io.TextIOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.BufferedReader", ShortName: "BufferedReader", EmbeddedTypes: []string{"io.BufferedIOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.BufferedWriter", ShortName: "BufferedWriter", EmbeddedTypes: []string{"io.BufferedIOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.BytesIO", ShortName: "BytesIO", EmbeddedTypes: []string{"io.BufferedIOBase"}})
	reg.AddType(typresolve.RegisteredType{QualifiedName: "io.StringIO", ShortName: "StringIO", EmbeddedTypes: []string{"io.TextIOBase"}})
	ioBaseMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"close", typresolve.Builtin("None")},
		{"closed", typresolve.Builtin("bool")},
		{"fileno", typresolve.Builtin("int")},
		{"flush", typresolve.Builtin("None")},
		{"isatty", typresolve.Builtin("bool")},
		{"readable", typresolve.Builtin("bool")},
		{"writable", typresolve.Builtin("bool")},
		{"seekable", typresolve.Builtin("bool")},
		{"seek", typresolve.Builtin("int")},
		{"tell", typresolve.Builtin("int")},
		{"truncate", typresolve.Builtin("int")},
	}
	registerMethods(reg, "io.IOBase", ioBaseMethods)
	textIOMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"read", typresolve.Builtin("str")},
		{"readline", typresolve.Builtin("str")},
		{"readlines", typresolve.Slice(typresolve.Builtin("str"))},
		{"write", typresolve.Builtin("int")},
		{"writelines", typresolve.Builtin("None")},
		{"__enter__", typresolve.Named("io.TextIOWrapper")},
		{"__exit__", typresolve.Builtin("None")},
		{"__iter__", nil},
	}
	registerMethods(reg, "io.TextIOBase", textIOMethods)
	registerMethods(reg, "io.TextIOWrapper", textIOMethods)
	bufIOMethods := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"read", typresolve.Builtin("bytes")},
		{"readline", typresolve.Builtin("bytes")},
		{"readlines", typresolve.Slice(typresolve.Builtin("bytes"))},
		{"write", typresolve.Builtin("int")},
		{"__enter__", typresolve.Named("io.BufferedIOBase")},
		{"__exit__", typresolve.Builtin("None")},
	}
	registerMethods(reg, "io.BufferedIOBase", bufIOMethods)
	registerMethods(reg, "io.BytesIO", bufIOMethods)
	ioFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"open", typresolve.Named("io.TextIOWrapper")},
		{"StringIO", typresolve.Named("io.StringIO")},
		{"BytesIO", typresolve.Named("io.BytesIO")},
	}
	registerFunctions(reg, "io", ioFunctions)

	// --- typing module ---
	reg.AddType(typresolve.RegisteredType{QualifiedName: "typing", ShortName: "typing"})
	typingFunctions := []struct {
		name string
		ret  *typresolve.Type
	}{
		{"cast", nil},
		{"get_type_hints", typresolve.Builtin("dict")},
		{"overload", nil},
		{"no_type_check", nil},
		{"runtime_checkable", nil},
		{"final", nil},
	}
	registerFunctions(reg, "typing", typingFunctions)
}

// registerMethods is a helper to bulk-register methods on a receiver type.
func registerMethods(reg *typresolve.Registry, receiverQN string, methods []struct {
	name string
	ret  *typresolve.Type
}) {
	for _, m := range methods {
		var sig *typresolve.Type
		if m.ret != nil {
			sig = typresolve.Func(nil, []*typresolve.Type{m.ret})
		}
		reg.AddFunc(typresolve.RegisteredFunc{
			QualifiedName: receiverQN + "." + m.name,
			ShortName:     m.name,
			ReceiverType:  receiverQN,
			Signature:     sig,
			MinParams:     -1,
		})
	}
}

// registerFunctions is a helper to bulk-register functions in a module.
func registerFunctions(reg *typresolve.Registry, moduleQN string, functions []struct {
	name string
	ret  *typresolve.Type
}) {
	for _, f := range functions {
		var sig *typresolve.Type
		if f.ret != nil {
			sig = typresolve.Func(nil, []*typresolve.Type{f.ret})
		}
		reg.AddFunc(typresolve.RegisteredFunc{
			QualifiedName: moduleQN + "." + f.name,
			ShortName:     f.name,
			Signature:     sig,
			MinParams:     -1,
		})
	}
}
