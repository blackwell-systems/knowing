// Package goresolve implements the Go language in-process type resolver.
// It implements the typresolve.Resolver interface, resolving Go call
// expressions and type references using tree-sitter AST walking and a
// shared type registry built from extracted definitions.
package goresolve
