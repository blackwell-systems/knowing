package rustresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveUsePath resolves a Rust use path to a module path and external flag.
// It handles crate::, self::, super::, and external crate prefixes.
// The modulePath is the path without the final name segment (the imported symbol).
func ResolveUsePath(usePath string, currentFile string) (modulePath string, isExternal bool) {
	segments := strings.Split(usePath, "::")
	if len(segments) == 0 {
		return "", false
	}

	switch segments[0] {
	case "crate":
		// crate::module::Type -> modulePath = "crate::module"
		if len(segments) <= 1 {
			return "crate", false
		}
		// Module path is everything except the last segment (the symbol name).
		modulePath = strings.Join(segments[:len(segments)-1], "::")
		return modulePath, false

	case "self":
		// self::helpers::resolve -> resolve relative to current module.
		prefix := strings.Join(segments[:len(segments)-1], "::")
		resolved := ResolveModulePath(prefix, currentFile)
		if resolved == "" {
			resolved = prefix
		}
		return resolved, false

	case "super":
		// super::utils -> resolve relative to parent module.
		prefix := strings.Join(segments[:len(segments)-1], "::")
		resolved := ResolveModulePath(prefix, currentFile)
		if resolved == "" {
			resolved = prefix
		}
		return resolved, false

	default:
		// External crate (std, tokio, serde, etc.).
		if len(segments) <= 1 {
			return usePath, true
		}
		modulePath = strings.Join(segments[:len(segments)-1], "::")
		return modulePath, true
	}
}

// BuildUseMap walks top-level use_declaration nodes in the AST and builds
// a map from imported short name to the resolved module path.
// For example, `use crate::core::Config;` yields map["Config"] = "crate::core".
func BuildUseMap(root *sitter.Node, content []byte, filePath string) map[string]string {
	uses := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "use_declaration" {
			continue
		}
		processUseDeclaration(child, content, filePath, uses)
	}

	return uses
}

// processUseDeclaration handles a single use_declaration node.
func processUseDeclaration(node *sitter.Node, content []byte, filePath string, uses map[string]string) {
	// The argument field contains the use path/tree.
	argNode := node.ChildByFieldName("argument")
	if argNode == nil {
		// Fallback: try first meaningful child.
		for i := 0; i < int(node.ChildCount()); i++ {
			c := node.Child(i)
			t := c.Type()
			if t == "scoped_identifier" || t == "scoped_use_list" || t == "use_as_clause" || t == "identifier" {
				argNode = c
				break
			}
		}
		if argNode == nil {
			return
		}
	}

	argText := argNode.Content(content)

	// Handle use_as_clause: `use std::io::Read as IoRead`
	if argNode.Type() == "use_as_clause" {
		processAsClause(argText, filePath, uses)
		return
	}

	// Handle glob: `use crate::prelude::*`
	if strings.HasSuffix(argText, "::*") {
		return
	}

	// Handle group imports: `use crate::module::{A, B, C}`
	if braceIdx := strings.Index(argText, "::{"); braceIdx != -1 {
		processGroupImport(argText, braceIdx, filePath, uses)
		return
	}

	// Simple import: `use crate::module::Type`
	processSimpleImport(argText, filePath, uses)
}

// processAsClause handles `use path::Symbol as Alias`.
func processAsClause(text string, filePath string, uses map[string]string) {
	// Format: "path::Symbol as Alias"
	parts := strings.SplitN(text, " as ", 2)
	if len(parts) != 2 {
		return
	}
	fullPath := strings.TrimSpace(parts[0])
	alias := strings.TrimSpace(parts[1])

	modulePath, _ := ResolveUsePath(fullPath, filePath)
	uses[alias] = modulePath
}

// processGroupImport handles `use prefix::{A, B, C}`.
func processGroupImport(text string, braceIdx int, filePath string, uses map[string]string) {
	prefix := text[:braceIdx]

	// Extract names from within braces.
	braceContent := text[braceIdx+3:] // skip "::{"
	endBrace := strings.Index(braceContent, "}")
	if endBrace != -1 {
		braceContent = braceContent[:endBrace]
	}

	names := strings.Split(braceContent, ",")
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || name == "*" {
			continue
		}

		// Handle alias within group: `{Read as IoRead}`
		if asIdx := strings.Index(name, " as "); asIdx != -1 {
			alias := strings.TrimSpace(name[asIdx+4:])
			fullPath := prefix + "::" + strings.TrimSpace(name[:asIdx])
			modulePath, _ := ResolveUsePath(fullPath, filePath)
			uses[alias] = modulePath
			continue
		}

		// Build full path for resolution.
		fullPath := prefix + "::" + name
		modulePath, _ := ResolveUsePath(fullPath, filePath)
		uses[name] = modulePath
	}
}

// processSimpleImport handles `use path::Symbol`.
func processSimpleImport(text string, filePath string, uses map[string]string) {
	// The last segment is the imported name.
	lastSep := strings.LastIndex(text, "::")
	if lastSep == -1 {
		// Single segment: just a crate name.
		uses[text] = text
		return
	}

	name := text[lastSep+2:]
	modulePath, _ := ResolveUsePath(text, filePath)
	uses[name] = modulePath
}
