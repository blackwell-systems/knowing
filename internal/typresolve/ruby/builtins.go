package rubyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// builtinFuncs is the set of Ruby builtin methods and Kernel methods.
var builtinFuncs = map[string]bool{
	// I/O
	"puts": true, "print": true, "p": true, "pp": true, "printf": true,
	"sprintf": true, "gets": true, "warn": true,
	// Control flow
	"raise": true, "fail": true, "throw": true, "catch": true,
	// Loading
	"require": true, "require_relative": true, "load": true, "autoload": true,
	// Blocks/procs
	"lambda": true, "proc": true, "block_given?": true, "iterator?": true,
	// Miscellaneous Kernel
	"loop": true, "sleep": true, "exit": true, "abort": true, "at_exit": true,
	"rand": true, "srand": true,
	"open": true, "select": true,
	// Object methods
	"freeze": true, "frozen?": true, "dup": true, "clone": true,
	"taint": true, "untaint": true, "tainted?": true,
	"respond_to?": true, "send": true, "public_send": true,
	"method": true, "__send__": true, "__method__": true,
	"is_a?": true, "kind_of?": true, "instance_of?": true,
	"nil?": true, "class": true, "object_id": true, "hash": true, "equal?": true,
	// Metaprogramming
	"define_method": true, "alias_method": true, "method_defined?": true,
	// Attribute accessors
	"attr_reader": true, "attr_writer": true, "attr_accessor": true,
	// Module inclusion
	"include": true, "extend": true, "prepend": true,
	// Visibility
	"public": true, "private": true, "protected": true, "module_function": true,
	// Conversion functions (Kernel)
	"Array": true, "Hash": true, "String": true,
	"Integer": true, "Float": true, "Rational": true, "Complex": true,
}

// IsBuiltinFunc returns true if the given name is a Ruby builtin method/function.
func IsBuiltinFunc(name string) bool {
	return builtinFuncs[name]
}

// builtinTypes is the set of Ruby core classes and modules.
var builtinTypes = map[string]bool{
	// Core hierarchy
	"Object": true, "BasicObject": true, "Kernel": true, "Module": true, "Class": true,
	// Strings and numbers
	"String": true, "Integer": true, "Float": true, "Numeric": true,
	"Complex": true, "Rational": true,
	// Collections
	"Array": true, "Hash": true, "Set": true, "Range": true,
	// Pattern matching
	"Regexp": true, "MatchData": true,
	// Primitives
	"Symbol": true, "NilClass": true, "TrueClass": true, "FalseClass": true,
	// I/O
	"IO": true, "File": true, "Dir": true, "Pathname": true,
	// Callables
	"Proc": true, "Method": true, "UnboundMethod": true,
	// Concurrency
	"Thread": true, "Mutex": true, "Fiber": true, "Ractor": true,
	// Mixins
	"Enumerable": true, "Comparable": true, "Enumerator": true,
	// Exceptions
	"Exception": true, "StandardError": true, "RuntimeError": true,
	"TypeError": true, "ArgumentError": true, "NameError": true,
	"NoMethodError": true, "ZeroDivisionError": true, "IOError": true, "Errno": true,
	// Data structures
	"Struct": true, "OpenStruct": true, "Data": true,
	// Date/time
	"Time": true, "Date": true, "DateTime": true,
	// Utilities
	"Encoding": true, "Marshal": true, "ObjectSpace": true, "GC": true,
}

// IsBuiltinType returns true if the given name is a Ruby builtin class/module.
func IsBuiltinType(name string) bool {
	return builtinTypes[name]
}

// ResolveBuiltinType returns a typresolve.Named type if the name is a Ruby builtin
// class, nil otherwise. Uses "Ruby::" prefix for builtin qualified names.
func ResolveBuiltinType(name string) *typresolve.Type {
	if !builtinTypes[name] {
		return nil
	}
	return typresolve.Named("Ruby::" + name)
}

// EvalBuiltinCall evaluates return types of common Ruby builtin methods.
// The ctx parameter is unused for now but allows future extension.
func EvalBuiltinCall(name string, args *sitter.Node, content []byte, ctx interface{}) *typresolve.Type {
	switch name {
	// I/O methods return nil
	case "puts", "print", "p", "warn":
		return typresolve.Named("Ruby::NilClass")

	// Input
	case "gets":
		return typresolve.Named("Ruby::String")

	// Random
	case "rand":
		return typresolve.Named("Ruby::Float")

	// Blocks/procs
	case "lambda", "proc":
		return typresolve.Named("Ruby::Proc")

	// Kernel conversion functions
	case "Array":
		return typresolve.Named("Ruby::Array")
	case "Hash":
		return typresolve.Named("Ruby::Hash")
	case "String":
		return typresolve.Named("Ruby::String")
	case "Integer":
		return typresolve.Named("Ruby::Integer")
	case "Float":
		return typresolve.Named("Ruby::Float")

	// Object identity (returns self, unknown receiver type)
	case "freeze", "dup", "clone":
		return typresolve.Unknown()

	// Boolean predicates (approximated as TrueClass)
	case "is_a?", "kind_of?", "instance_of?", "nil?", "respond_to?",
		"frozen?", "tainted?", "equal?", "block_given?", "iterator?":
		return typresolve.Named("Ruby::TrueClass")

	// Reflection
	case "class":
		return typresolve.Named("Ruby::Class")
	case "object_id", "hash":
		return typresolve.Named("Ruby::Integer")
	case "method":
		return typresolve.Named("Ruby::Method")
	case "__method__":
		return typresolve.Named("Ruby::Symbol")

	default:
		return typresolve.Unknown()
	}
}
