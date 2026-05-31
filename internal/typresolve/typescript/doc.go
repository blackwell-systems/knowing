// Package tsresolve implements the TypeScript/JavaScript in-process type
// resolver. It implements the typresolve.Resolver interface, resolving
// TypeScript and JavaScript call expressions, property access, JSX elements,
// and type references using tree-sitter AST walking and a shared type
// registry built from extracted definitions.
//
// Supported dialects: .ts, .tsx, .js, .jsx
package tsresolve
