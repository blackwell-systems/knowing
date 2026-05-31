// Package rustresolve implements the Rust language in-process type resolver.
// It implements the typresolve.Resolver interface, resolving Rust call
// expressions, method calls, and type references using tree-sitter AST
// walking and a shared type registry built from extracted definitions.
package rustresolve
