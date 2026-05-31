package tsresolve

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// ImportInfo describes a single TypeScript import binding. Captures the
// source module path and the style of import (named, default, namespace,
// or require).
type ImportInfo struct {
	ModulePath   string // source module path (e.g. "./utils", "express")
	OriginalName string // original name before aliasing
	IsNamespace  bool   // true for "import * as X from ..."
	IsDefault    bool   // true for "import X from ..."
}

// BuildImportMap builds per-file import map from the TypeScript AST root.
// Returns a map from local binding name to ImportInfo. Handles ES6 imports
// (named, default, namespace) and CommonJS require().
func BuildImportMap(root *sitter.Node, content []byte) map[string]ImportInfo {
	imports := make(map[string]ImportInfo)

	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		switch node.Type() {
		case "import_statement":
			processImportStatement(node, content, imports)
		case "lexical_declaration", "variable_declaration":
			processRequireDeclaration(node, content, imports)
		}
	}

	return imports
}

// processImportStatement extracts bindings from an ES6 import statement.
func processImportStatement(node *sitter.Node, content []byte, imports map[string]ImportInfo) {
	// Get the source module path.
	src := node.ChildByFieldName("source")
	if src == nil {
		return
	}
	modPath := strings.Trim(src.Content(content), `"'`)
	if modPath == "" {
		return
	}

	// Find and process the import clause.
	for j := 0; j < int(node.ChildCount()); j++ {
		child := node.Child(j)
		if child == nil || child.Type() != "import_clause" {
			continue
		}

		for k := 0; k < int(child.ChildCount()); k++ {
			clause := child.Child(k)
			if clause == nil {
				continue
			}

			switch clause.Type() {
			case "identifier":
				// Default import: import X from './module'
				name := clause.Content(content)
				imports[name] = ImportInfo{
					ModulePath:   modPath,
					OriginalName: name,
					IsDefault:    true,
				}

			case "named_imports":
				// Named imports: import { X, Y as Z } from './module'
				for m := 0; m < int(clause.ChildCount()); m++ {
					spec := clause.Child(m)
					if spec == nil || spec.Type() != "import_specifier" {
						continue
					}
					nameNode := spec.ChildByFieldName("name")
					aliasNode := spec.ChildByFieldName("alias")

					if nameNode == nil {
						continue
					}
					originalName := nameNode.Content(content)
					bindName := originalName
					if aliasNode != nil {
						bindName = aliasNode.Content(content)
					}
					imports[bindName] = ImportInfo{
						ModulePath:   modPath,
						OriginalName: originalName,
					}
				}

			case "namespace_import":
				// Namespace import: import * as X from './module'
				for m := 0; m < int(clause.ChildCount()); m++ {
					id := clause.Child(m)
					if id != nil && id.Type() == "identifier" {
						name := id.Content(content)
						imports[name] = ImportInfo{
							ModulePath:   modPath,
							OriginalName: name,
							IsNamespace:  true,
						}
						break
					}
				}
			}
		}
	}
}

// processRequireDeclaration handles CommonJS require() declarations:
//
//	const X = require('module')
//	const { X } = require('module')
func processRequireDeclaration(node *sitter.Node, content []byte, imports map[string]ImportInfo) {
	for i := 0; i < int(node.ChildCount()); i++ {
		decl := node.Child(i)
		if decl == nil || decl.Type() != "variable_declarator" {
			continue
		}

		// Check if value is require() call.
		valueNode := decl.ChildByFieldName("value")
		if valueNode == nil || valueNode.Type() != "call_expression" {
			continue
		}

		funcNode := valueNode.ChildByFieldName("function")
		if funcNode == nil || funcNode.Content(content) != "require" {
			continue
		}

		// Extract module path from arguments.
		argsNode := valueNode.ChildByFieldName("arguments")
		if argsNode == nil || argsNode.NamedChildCount() == 0 {
			continue
		}
		argNode := argsNode.NamedChild(0)
		if argNode == nil {
			continue
		}
		modPath := strings.Trim(argNode.Content(content), `"'`)
		if modPath == "" {
			continue
		}

		// Extract binding name.
		nameNode := decl.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}

		switch nameNode.Type() {
		case "identifier":
			name := nameNode.Content(content)
			imports[name] = ImportInfo{
				ModulePath:   modPath,
				OriginalName: name,
				IsDefault:    true,
			}
		case "object_pattern":
			// Destructured require: const { X, Y } = require('module')
			for j := 0; j < int(nameNode.NamedChildCount()); j++ {
				prop := nameNode.NamedChild(j)
				if prop == nil {
					continue
				}
				if prop.Type() == "shorthand_property_identifier_pattern" {
					name := prop.Content(content)
					imports[name] = ImportInfo{
						ModulePath:   modPath,
						OriginalName: name,
					}
				} else if prop.Type() == "pair_pattern" {
					keyNode := prop.ChildByFieldName("key")
					valNode := prop.ChildByFieldName("value")
					if keyNode != nil && valNode != nil {
						imports[valNode.Content(content)] = ImportInfo{
							ModulePath:   modPath,
							OriginalName: keyNode.Content(content),
						}
					}
				}
			}
		}
	}
}

// ResolveImport resolves an import local name to its ImportInfo.
func ResolveImport(imports map[string]ImportInfo, name string) (ImportInfo, bool) {
	info, ok := imports[name]
	return info, ok
}

// ResolveModulePath resolves a TypeScript import source path to a qualified
// name prefix. Only resolves relative imports (starting with "." or "..").
// Returns "" for bare module specifiers (non-relative).
func ResolveModulePath(importSource string, currentFile string) string {
	if !strings.HasPrefix(importSource, ".") {
		return ""
	}

	dir := filepath.Dir(currentFile)
	resolved := filepath.Join(dir, importSource)
	resolved = filepath.ToSlash(resolved)

	// Strip known extensions.
	resolved = strings.TrimSuffix(resolved, ".ts")
	resolved = strings.TrimSuffix(resolved, ".tsx")
	resolved = strings.TrimSuffix(resolved, ".js")
	resolved = strings.TrimSuffix(resolved, ".jsx")

	return resolved
}
