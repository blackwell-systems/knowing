// Package rubyresolve implements the Ruby language in-process type resolver.
// It implements the typresolve.Resolver interface, resolving Ruby method
// calls and constant references using tree-sitter AST walking and a
// shared type registry built from extracted definitions.
package rubyresolve
