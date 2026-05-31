package javaresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// stdlibRegistry is a pre-built registry of java.lang, java.util, and java.io
// types with their common methods. It is set as the fallback on user registries
// so that stdlib types resolve without requiring explicit definitions.
var stdlibRegistry *typresolve.Registry

// stdlibPackageTypes maps package qualified names to the set of simple class
// names they contain. Used for wildcard import resolution.
var stdlibPackageTypes map[string]map[string]bool

func init() {
	stdlibRegistry = typresolve.NewRegistry()
	stdlibPackageTypes = make(map[string]map[string]bool)

	registerJavaLang()
	registerJavaUtil()
	registerJavaIO()
}

// GetStdlibRegistry returns the shared stdlib registry for use as a fallback.
func GetStdlibRegistry() *typresolve.Registry {
	return stdlibRegistry
}

// StdlibPackageTypes returns the set of simple type names for a given package,
// or nil if the package is not a known stdlib package.
func StdlibPackageTypes(pkg string) map[string]bool {
	return stdlibPackageTypes[pkg]
}

// addStdlibType registers a type and tracks it in the package type map.
func addStdlibType(pkg, simpleName string, isInterface bool, embedded []string) {
	qn := pkg + "." + simpleName
	stdlibRegistry.AddType(typresolve.RegisteredType{
		QualifiedName: qn,
		ShortName:     simpleName,
		IsInterface:   isInterface,
		EmbeddedTypes: embedded,
	})
	if stdlibPackageTypes[pkg] == nil {
		stdlibPackageTypes[pkg] = make(map[string]bool)
	}
	stdlibPackageTypes[pkg][simpleName] = true
}

// addStdlibMethod registers a method on a type in the stdlib registry.
func addStdlibMethod(typeQN, methodName string, sig *typresolve.Type) {
	qn := typeQN + "." + methodName
	stdlibRegistry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: qn,
		ShortName:     methodName,
		ReceiverType:  typeQN,
		Signature:     sig,
		MinParams:     -1,
	})
}

// addStdlibStaticMethod registers a static method (function) in the stdlib registry.
func addStdlibStaticMethod(typeQN, methodName string, sig *typresolve.Type) {
	qn := typeQN + "." + methodName
	stdlibRegistry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: qn,
		ShortName:     methodName,
		ReceiverType:  typeQN,
		Signature:     sig,
		MinParams:     -1,
	})
}

// sig is a helper to build a KindFunc type with the given return type.
func sig(ret *typresolve.Type) *typresolve.Type {
	return typresolve.Func(nil, []*typresolve.Type{ret})
}

// registerJavaLang registers java.lang types: Object, String, Integer, Long,
// Double, Float, Boolean, Byte, Short, Character, Number, System, Math,
// StringBuilder, Comparable, Iterable, Enum, Class, Thread, Throwable,
// Exception, RuntimeException.
func registerJavaLang() {
	pkg := "java.lang"

	// Object
	addStdlibType(pkg, "Object", false, nil)
	addStdlibMethod("java.lang.Object", "getClass", sig(typresolve.Named("java.lang.Class")))
	addStdlibMethod("java.lang.Object", "hashCode", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.Object", "equals", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.Object", "toString", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.Object", "clone", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.lang.Object", "notify", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Object", "notifyAll", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Object", "wait", sig(typresolve.Builtin("void")))

	// String
	addStdlibType(pkg, "String", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.String", "length", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.String", "charAt", sig(typresolve.Builtin("char")))
	addStdlibMethod("java.lang.String", "substring", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "contains", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "startsWith", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "endsWith", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "indexOf", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.String", "lastIndexOf", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.String", "replace", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "replaceAll", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "split", sig(typresolve.Slice(typresolve.Builtin("String"))))
	addStdlibMethod("java.lang.String", "trim", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "strip", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "toLowerCase", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "toUpperCase", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "isBlank", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "toCharArray", sig(typresolve.Slice(typresolve.Builtin("char"))))
	addStdlibMethod("java.lang.String", "concat", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.String", "matches", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.lang.String", "compareTo", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.lang.String", "valueOf", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.String", "format", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.String", "join", sig(typresolve.Builtin("String")))

	// Number (abstract)
	addStdlibType(pkg, "Number", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Number", "intValue", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.Number", "longValue", sig(typresolve.Builtin("long")))
	addStdlibMethod("java.lang.Number", "doubleValue", sig(typresolve.Builtin("double")))
	addStdlibMethod("java.lang.Number", "floatValue", sig(typresolve.Builtin("float")))

	// Integer
	addStdlibType(pkg, "Integer", false, []string{"java.lang.Number"})
	addStdlibMethod("java.lang.Integer", "intValue", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.Integer", "compareTo", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.lang.Integer", "parseInt", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.lang.Integer", "valueOf", sig(typresolve.Named("java.lang.Integer")))
	addStdlibStaticMethod("java.lang.Integer", "toString", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.Integer", "max", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.lang.Integer", "min", sig(typresolve.Builtin("int")))

	// Long
	addStdlibType(pkg, "Long", false, []string{"java.lang.Number"})
	addStdlibMethod("java.lang.Long", "longValue", sig(typresolve.Builtin("long")))
	addStdlibStaticMethod("java.lang.Long", "parseLong", sig(typresolve.Builtin("long")))
	addStdlibStaticMethod("java.lang.Long", "valueOf", sig(typresolve.Named("java.lang.Long")))

	// Double
	addStdlibType(pkg, "Double", false, []string{"java.lang.Number"})
	addStdlibMethod("java.lang.Double", "doubleValue", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Double", "parseDouble", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Double", "valueOf", sig(typresolve.Named("java.lang.Double")))
	addStdlibStaticMethod("java.lang.Double", "isNaN", sig(typresolve.Builtin("boolean")))

	// Float
	addStdlibType(pkg, "Float", false, []string{"java.lang.Number"})
	addStdlibStaticMethod("java.lang.Float", "parseFloat", sig(typresolve.Builtin("float")))
	addStdlibStaticMethod("java.lang.Float", "valueOf", sig(typresolve.Named("java.lang.Float")))

	// Boolean
	addStdlibType(pkg, "Boolean", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Boolean", "booleanValue", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.lang.Boolean", "parseBoolean", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.lang.Boolean", "valueOf", sig(typresolve.Named("java.lang.Boolean")))

	// Byte
	addStdlibType(pkg, "Byte", false, []string{"java.lang.Number"})
	addStdlibStaticMethod("java.lang.Byte", "parseByte", sig(typresolve.Builtin("byte")))

	// Short
	addStdlibType(pkg, "Short", false, []string{"java.lang.Number"})
	addStdlibStaticMethod("java.lang.Short", "parseShort", sig(typresolve.Builtin("short")))

	// Character
	addStdlibType(pkg, "Character", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Character", "charValue", sig(typresolve.Builtin("char")))
	addStdlibStaticMethod("java.lang.Character", "isLetter", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.lang.Character", "isDigit", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.lang.Character", "isWhitespace", sig(typresolve.Builtin("boolean")))

	// System
	addStdlibType(pkg, "System", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.lang.System", "currentTimeMillis", sig(typresolve.Builtin("long")))
	addStdlibStaticMethod("java.lang.System", "nanoTime", sig(typresolve.Builtin("long")))
	addStdlibStaticMethod("java.lang.System", "getenv", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.System", "getProperty", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.System", "exit", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.lang.System", "gc", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.lang.System", "arraycopy", sig(typresolve.Builtin("void")))

	// Math
	addStdlibType(pkg, "Math", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.lang.Math", "abs", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "max", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "min", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "sqrt", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "pow", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "random", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "round", sig(typresolve.Builtin("long")))
	addStdlibStaticMethod("java.lang.Math", "ceil", sig(typresolve.Builtin("double")))
	addStdlibStaticMethod("java.lang.Math", "floor", sig(typresolve.Builtin("double")))

	// StringBuilder
	addStdlibType(pkg, "StringBuilder", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.StringBuilder", "append", sig(typresolve.Named("java.lang.StringBuilder")))
	addStdlibMethod("java.lang.StringBuilder", "insert", sig(typresolve.Named("java.lang.StringBuilder")))
	addStdlibMethod("java.lang.StringBuilder", "delete", sig(typresolve.Named("java.lang.StringBuilder")))
	addStdlibMethod("java.lang.StringBuilder", "reverse", sig(typresolve.Named("java.lang.StringBuilder")))
	addStdlibMethod("java.lang.StringBuilder", "toString", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.StringBuilder", "length", sig(typresolve.Builtin("int")))

	// Comparable (interface)
	addStdlibType(pkg, "Comparable", true, nil)
	addStdlibMethod("java.lang.Comparable", "compareTo", sig(typresolve.Builtin("int")))

	// Iterable (interface)
	addStdlibType(pkg, "Iterable", true, nil)
	addStdlibMethod("java.lang.Iterable", "iterator", sig(typresolve.Named("java.util.Iterator")))

	// Enum (abstract)
	addStdlibType(pkg, "Enum", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Enum", "name", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.Enum", "ordinal", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.lang.Enum", "compareTo", sig(typresolve.Builtin("int")))

	// Class
	addStdlibType(pkg, "Class", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Class", "getName", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.Class", "getSimpleName", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.Class", "newInstance", sig(typresolve.Named("java.lang.Object")))
	addStdlibStaticMethod("java.lang.Class", "forName", sig(typresolve.Named("java.lang.Class")))

	// Thread
	addStdlibType(pkg, "Thread", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Thread", "start", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Thread", "run", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Thread", "join", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Thread", "getName", sig(typresolve.Builtin("String")))
	addStdlibStaticMethod("java.lang.Thread", "currentThread", sig(typresolve.Named("java.lang.Thread")))
	addStdlibStaticMethod("java.lang.Thread", "sleep", sig(typresolve.Builtin("void")))

	// Throwable
	addStdlibType(pkg, "Throwable", false, []string{"java.lang.Object"})
	addStdlibMethod("java.lang.Throwable", "getMessage", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.lang.Throwable", "getCause", sig(typresolve.Named("java.lang.Throwable")))
	addStdlibMethod("java.lang.Throwable", "printStackTrace", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.lang.Throwable", "getStackTrace", sig(typresolve.Slice(typresolve.Named("java.lang.StackTraceElement"))))

	// Exception
	addStdlibType(pkg, "Exception", false, []string{"java.lang.Throwable"})

	// RuntimeException
	addStdlibType(pkg, "RuntimeException", false, []string{"java.lang.Exception"})

	// Runnable (interface)
	addStdlibType(pkg, "Runnable", true, nil)
	addStdlibMethod("java.lang.Runnable", "run", sig(typresolve.Builtin("void")))

	// AutoCloseable (interface)
	addStdlibType(pkg, "AutoCloseable", true, nil)
	addStdlibMethod("java.lang.AutoCloseable", "close", sig(typresolve.Builtin("void")))

	// Void (type)
	addStdlibType(pkg, "Void", false, []string{"java.lang.Object"})
}

// registerJavaUtil registers java.util types: List, ArrayList, LinkedList,
// Map, HashMap, TreeMap, Set, HashSet, TreeSet, Optional, Stream, Iterator,
// Collections, Arrays, Objects.
func registerJavaUtil() {
	pkg := "java.util"

	// Collection (interface)
	addStdlibType(pkg, "Collection", true, []string{"java.lang.Iterable"})
	addStdlibMethod("java.util.Collection", "add", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Collection", "remove", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Collection", "contains", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Collection", "size", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.util.Collection", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Collection", "clear", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.util.Collection", "iterator", sig(typresolve.Named("java.util.Iterator")))
	addStdlibMethod("java.util.Collection", "stream", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.Collection", "toArray", sig(typresolve.Slice(typresolve.Named("java.lang.Object"))))

	// List (interface)
	addStdlibType(pkg, "List", true, []string{"java.util.Collection"})
	addStdlibMethod("java.util.List", "get", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.List", "set", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.List", "add", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.List", "remove", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.List", "indexOf", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.util.List", "size", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.util.List", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.List", "contains", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.List", "subList", sig(typresolve.Named("java.util.List")))
	addStdlibMethod("java.util.List", "sort", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.util.List", "of", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.List", "copyOf", sig(typresolve.Named("java.util.List")))

	// ArrayList
	addStdlibType(pkg, "ArrayList", false, []string{"java.util.List"})
	addStdlibMethod("java.util.ArrayList", "add", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.ArrayList", "get", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.ArrayList", "set", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.ArrayList", "remove", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.ArrayList", "size", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.util.ArrayList", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.ArrayList", "contains", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.ArrayList", "clear", sig(typresolve.Builtin("void")))

	// LinkedList
	addStdlibType(pkg, "LinkedList", false, []string{"java.util.List"})
	addStdlibMethod("java.util.LinkedList", "addFirst", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.util.LinkedList", "addLast", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.util.LinkedList", "getFirst", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.LinkedList", "getLast", sig(typresolve.Named("java.lang.Object")))

	// Set (interface)
	addStdlibType(pkg, "Set", true, []string{"java.util.Collection"})
	addStdlibMethod("java.util.Set", "add", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Set", "remove", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Set", "contains", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Set", "size", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.util.Set", "of", sig(typresolve.Named("java.util.Set")))

	// HashSet
	addStdlibType(pkg, "HashSet", false, []string{"java.util.Set"})

	// TreeSet
	addStdlibType(pkg, "TreeSet", false, []string{"java.util.Set"})

	// Map (interface)
	addStdlibType(pkg, "Map", true, nil)
	addStdlibMethod("java.util.Map", "get", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Map", "put", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Map", "remove", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Map", "containsKey", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Map", "containsValue", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Map", "size", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.util.Map", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Map", "keySet", sig(typresolve.Named("java.util.Set")))
	addStdlibMethod("java.util.Map", "values", sig(typresolve.Named("java.util.Collection")))
	addStdlibMethod("java.util.Map", "entrySet", sig(typresolve.Named("java.util.Set")))
	addStdlibMethod("java.util.Map", "getOrDefault", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Map", "putIfAbsent", sig(typresolve.Named("java.lang.Object")))
	addStdlibStaticMethod("java.util.Map", "of", sig(typresolve.Named("java.util.Map")))
	addStdlibStaticMethod("java.util.Map", "copyOf", sig(typresolve.Named("java.util.Map")))

	// HashMap
	addStdlibType(pkg, "HashMap", false, []string{"java.util.Map"})

	// TreeMap
	addStdlibType(pkg, "TreeMap", false, []string{"java.util.Map"})

	// LinkedHashMap
	addStdlibType(pkg, "LinkedHashMap", false, []string{"java.util.HashMap"})

	// ConcurrentHashMap
	addStdlibType(pkg, "ConcurrentHashMap", false, []string{"java.util.Map"})
	if stdlibPackageTypes["java.util.concurrent"] == nil {
		stdlibPackageTypes["java.util.concurrent"] = make(map[string]bool)
	}
	// Also register in java.util since it's commonly imported from there.

	// Optional
	addStdlibType(pkg, "Optional", false, []string{"java.lang.Object"})
	addStdlibMethod("java.util.Optional", "get", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Optional", "isPresent", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Optional", "isEmpty", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Optional", "orElse", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Optional", "orElseGet", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Optional", "orElseThrow", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Optional", "map", sig(typresolve.Named("java.util.Optional")))
	addStdlibMethod("java.util.Optional", "flatMap", sig(typresolve.Named("java.util.Optional")))
	addStdlibMethod("java.util.Optional", "filter", sig(typresolve.Named("java.util.Optional")))
	addStdlibStaticMethod("java.util.Optional", "of", sig(typresolve.Named("java.util.Optional")))
	addStdlibStaticMethod("java.util.Optional", "ofNullable", sig(typresolve.Named("java.util.Optional")))
	addStdlibStaticMethod("java.util.Optional", "empty", sig(typresolve.Named("java.util.Optional")))

	// Iterator (interface)
	addStdlibType(pkg, "Iterator", true, nil)
	addStdlibMethod("java.util.Iterator", "hasNext", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.util.Iterator", "next", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.Iterator", "remove", sig(typresolve.Builtin("void")))

	// Collections (utility class)
	addStdlibType(pkg, "Collections", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.util.Collections", "sort", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.util.Collections", "unmodifiableList", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.Collections", "unmodifiableMap", sig(typresolve.Named("java.util.Map")))
	addStdlibStaticMethod("java.util.Collections", "unmodifiableSet", sig(typresolve.Named("java.util.Set")))
	addStdlibStaticMethod("java.util.Collections", "emptyList", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.Collections", "emptyMap", sig(typresolve.Named("java.util.Map")))
	addStdlibStaticMethod("java.util.Collections", "emptySet", sig(typresolve.Named("java.util.Set")))
	addStdlibStaticMethod("java.util.Collections", "singletonList", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.Collections", "singleton", sig(typresolve.Named("java.util.Set")))
	addStdlibStaticMethod("java.util.Collections", "reverse", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.util.Collections", "shuffle", sig(typresolve.Builtin("void")))

	// Arrays (utility class)
	addStdlibType(pkg, "Arrays", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.util.Arrays", "asList", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.Arrays", "sort", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.util.Arrays", "stream", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibStaticMethod("java.util.Arrays", "copyOf", sig(typresolve.Slice(typresolve.Named("java.lang.Object"))))
	addStdlibStaticMethod("java.util.Arrays", "fill", sig(typresolve.Builtin("void")))
	addStdlibStaticMethod("java.util.Arrays", "equals", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.util.Arrays", "toString", sig(typresolve.Builtin("String")))

	// Objects (utility class)
	addStdlibType(pkg, "Objects", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.util.Objects", "requireNonNull", sig(typresolve.Named("java.lang.Object")))
	addStdlibStaticMethod("java.util.Objects", "equals", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.util.Objects", "hash", sig(typresolve.Builtin("int")))
	addStdlibStaticMethod("java.util.Objects", "isNull", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.util.Objects", "nonNull", sig(typresolve.Builtin("boolean")))
	addStdlibStaticMethod("java.util.Objects", "toString", sig(typresolve.Builtin("String")))

	// Stream (in java.util.stream, but commonly used)
	streamPkg := "java.util.stream"
	addStdlibType(streamPkg, "Stream", true, nil)
	addStdlibMethod("java.util.stream.Stream", "filter", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "map", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "flatMap", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "collect", sig(typresolve.Named("java.lang.Object")))
	addStdlibMethod("java.util.stream.Stream", "forEach", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.util.stream.Stream", "reduce", sig(typresolve.Named("java.util.Optional")))
	addStdlibMethod("java.util.stream.Stream", "count", sig(typresolve.Builtin("long")))
	addStdlibMethod("java.util.stream.Stream", "findFirst", sig(typresolve.Named("java.util.Optional")))
	addStdlibMethod("java.util.stream.Stream", "findAny", sig(typresolve.Named("java.util.Optional")))
	addStdlibMethod("java.util.stream.Stream", "sorted", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "distinct", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "limit", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibMethod("java.util.stream.Stream", "toList", sig(typresolve.Named("java.util.List")))
	addStdlibStaticMethod("java.util.stream.Stream", "of", sig(typresolve.Named("java.util.stream.Stream")))
	addStdlibStaticMethod("java.util.stream.Stream", "empty", sig(typresolve.Named("java.util.stream.Stream")))

	// Collectors (java.util.stream)
	addStdlibType(streamPkg, "Collectors", false, []string{"java.lang.Object"})
	addStdlibStaticMethod("java.util.stream.Collectors", "toList", sig(typresolve.Named("java.util.stream.Collector")))
	addStdlibStaticMethod("java.util.stream.Collectors", "toSet", sig(typresolve.Named("java.util.stream.Collector")))
	addStdlibStaticMethod("java.util.stream.Collectors", "toMap", sig(typresolve.Named("java.util.stream.Collector")))
	addStdlibStaticMethod("java.util.stream.Collectors", "joining", sig(typresolve.Named("java.util.stream.Collector")))
	addStdlibStaticMethod("java.util.stream.Collectors", "groupingBy", sig(typresolve.Named("java.util.stream.Collector")))
}

// registerJavaIO registers java.io types: InputStream, OutputStream, Reader,
// Writer, File, BufferedReader, BufferedWriter, PrintWriter.
func registerJavaIO() {
	pkg := "java.io"

	// InputStream
	addStdlibType(pkg, "InputStream", false, []string{"java.lang.Object"})
	addStdlibMethod("java.io.InputStream", "read", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.io.InputStream", "close", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.InputStream", "available", sig(typresolve.Builtin("int")))

	// OutputStream
	addStdlibType(pkg, "OutputStream", false, []string{"java.lang.Object"})
	addStdlibMethod("java.io.OutputStream", "write", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.OutputStream", "flush", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.OutputStream", "close", sig(typresolve.Builtin("void")))

	// Reader
	addStdlibType(pkg, "Reader", false, []string{"java.lang.Object"})
	addStdlibMethod("java.io.Reader", "read", sig(typresolve.Builtin("int")))
	addStdlibMethod("java.io.Reader", "close", sig(typresolve.Builtin("void")))

	// Writer
	addStdlibType(pkg, "Writer", false, []string{"java.lang.Object"})
	addStdlibMethod("java.io.Writer", "write", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.Writer", "flush", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.Writer", "close", sig(typresolve.Builtin("void")))

	// File
	addStdlibType(pkg, "File", false, []string{"java.lang.Object"})
	addStdlibMethod("java.io.File", "exists", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.io.File", "getName", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.io.File", "getPath", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.io.File", "getAbsolutePath", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.io.File", "isDirectory", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.io.File", "isFile", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.io.File", "length", sig(typresolve.Builtin("long")))
	addStdlibMethod("java.io.File", "delete", sig(typresolve.Builtin("boolean")))
	addStdlibMethod("java.io.File", "listFiles", sig(typresolve.Slice(typresolve.Named("java.io.File"))))

	// BufferedReader
	addStdlibType(pkg, "BufferedReader", false, []string{"java.io.Reader"})
	addStdlibMethod("java.io.BufferedReader", "readLine", sig(typresolve.Builtin("String")))
	addStdlibMethod("java.io.BufferedReader", "lines", sig(typresolve.Named("java.util.stream.Stream")))

	// BufferedWriter
	addStdlibType(pkg, "BufferedWriter", false, []string{"java.io.Writer"})
	addStdlibMethod("java.io.BufferedWriter", "newLine", sig(typresolve.Builtin("void")))

	// PrintWriter
	addStdlibType(pkg, "PrintWriter", false, []string{"java.io.Writer"})
	addStdlibMethod("java.io.PrintWriter", "println", sig(typresolve.Builtin("void")))
	addStdlibMethod("java.io.PrintWriter", "printf", sig(typresolve.Named("java.io.PrintWriter")))
	addStdlibMethod("java.io.PrintWriter", "print", sig(typresolve.Builtin("void")))

	// Serializable (interface)
	addStdlibType(pkg, "Serializable", true, nil)
}
