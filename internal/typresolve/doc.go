// Package typresolve provides shared infrastructure for in-process
// language-specific type resolution. It includes type representation,
// a type registry, scope chains, and the Resolver interface that
// per-language resolvers implement.
//
// This package replaces external LSP servers for type resolution,
// running inside the same process as tree-sitter extraction and
// sharing the parsed AST directly.
package typresolve
