package goresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// BuildImportMap builds a mapping from import alias to full import path
// by walking import_declaration children of the AST root. For imports
// without an explicit alias, the alias is the last path segment.
// Dot imports (".") and blank imports ("_") are skipped.
func BuildImportMap(root *sitter.Node, content []byte) map[string]string {
	imports := make(map[string]string)
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "import_declaration" {
			continue
		}
		for j := 0; j < int(child.ChildCount()); j++ {
			spec := child.Child(j)
			switch spec.Type() {
			case "import_spec":
				addImportSpec(spec, content, imports)
			case "import_spec_list":
				for k := 0; k < int(spec.ChildCount()); k++ {
					item := spec.Child(k)
					if item.Type() == "import_spec" {
						addImportSpec(item, content, imports)
					}
				}
			}
		}
	}
	return imports
}

// addImportSpec extracts one import spec into the imports map.
func addImportSpec(spec *sitter.Node, content []byte, imports map[string]string) {
	pathNode := spec.ChildByFieldName("path")
	if pathNode == nil {
		return
	}
	importPath := strings.Trim(pathNode.Content(content), `"`)

	nameNode := spec.ChildByFieldName("name")
	if nameNode != nil {
		alias := nameNode.Content(content)
		if alias != "." && alias != "_" {
			imports[alias] = importPath
		}
	} else {
		// Default alias is the last path segment.
		parts := strings.Split(importPath, "/")
		alias := parts[len(parts)-1]
		imports[alias] = importPath
	}
}

// ResolveImport resolves an import alias to its full package path.
// Returns the package path and true if the alias is found, or empty
// string and false otherwise.
func ResolveImport(imports map[string]string, alias string) (string, bool) {
	pkgPath, ok := imports[alias]
	return pkgPath, ok
}
