// Package pyresolve implements the Python language in-process type resolver.
// It implements the typresolve.Resolver interface, resolving Python call
// expressions and type references using tree-sitter AST walking and a
// shared type registry built from extracted definitions.
package pyresolve
