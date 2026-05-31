package pyresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// ImportInfo describes a single Python import binding.
type ImportInfo struct {
	ModulePath  string // full dotted module path (e.g. "django.db.models")
	IsFromStyle bool   // true for "from X import Y", false for "import X"
}

// BuildImportMap builds a per-file import map from the Python AST root.
// It returns a map from the local binding name to its ImportInfo. Handles:
//   - import X (binds X as module)
//   - import X as Y (binds Y as module X)
//   - from X import Y (binds Y as symbol from X)
//   - from X import Y as Z (binds Z as symbol Y from X)
//   - import X.Y.Z (binds X, X.Y, X.Y.Z as modules)
//
// Wildcard imports (from X import *) are skipped.
func BuildImportMap(root *sitter.Node, content []byte) map[string]ImportInfo {
	imports := make(map[string]ImportInfo)
	if root == nil {
		return imports
	}

	for i := 0; i < int(root.ChildCount()); i++ {
		node := root.Child(i)
		if node == nil {
			continue
		}

		switch node.Type() {
		case "import_statement":
			processImportStatement(node, content, imports)
		case "import_from_statement":
			processImportFromStatement(node, content, imports)
		}
	}

	return imports
}

// processImportStatement handles "import X", "import X as Y",
// and "import X.Y.Z" (binding intermediate prefixes).
func processImportStatement(node *sitter.Node, content []byte, imports map[string]ImportInfo) {
	for j := 0; j < int(node.ChildCount()); j++ {
		child := node.Child(j)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "dotted_name":
			fullName := child.Content(content)
			// Bind the full dotted name
			imports[fullName] = ImportInfo{
				ModulePath:  fullName,
				IsFromStyle: false,
			}
			// Bind intermediate prefixes: for "import os.path",
			// bind "os" -> "os" as well.
			bindIntermediatePrefixes(fullName, imports)

		case "aliased_import":
			alias := child.ChildByFieldName("alias")
			name := child.ChildByFieldName("name")
			if alias != nil && name != nil {
				imports[alias.Content(content)] = ImportInfo{
					ModulePath:  name.Content(content),
					IsFromStyle: false,
				}
			} else if name != nil {
				fullName := name.Content(content)
				imports[fullName] = ImportInfo{
					ModulePath:  fullName,
					IsFromStyle: false,
				}
				bindIntermediatePrefixes(fullName, imports)
			}
		}
	}
}

// processImportFromStatement handles "from X import Y" and
// "from X import Y as Z". Skips wildcard imports.
func processImportFromStatement(node *sitter.Node, content []byte, imports map[string]ImportInfo) {
	moduleNode := node.ChildByFieldName("module_name")
	if moduleNode == nil {
		return
	}
	moduleName := moduleNode.Content(content)

	for j := 0; j < int(node.ChildCount()); j++ {
		child := node.Child(j)
		if child == nil {
			continue
		}

		switch child.Type() {
		case "dotted_name":
			// Skip the module_name node itself
			if child == moduleNode {
				continue
			}
			importedName := child.Content(content)
			// Skip wildcard
			if importedName == "*" {
				continue
			}
			imports[importedName] = ImportInfo{
				ModulePath:  moduleName + "." + importedName,
				IsFromStyle: true,
			}

		case "aliased_import":
			alias := child.ChildByFieldName("alias")
			name := child.ChildByFieldName("name")
			if name == nil {
				continue
			}
			importedName := name.Content(content)
			fullPath := moduleName + "." + importedName

			if alias != nil {
				imports[alias.Content(content)] = ImportInfo{
					ModulePath:  fullPath,
					IsFromStyle: true,
				}
			} else {
				imports[importedName] = ImportInfo{
					ModulePath:  fullPath,
					IsFromStyle: true,
				}
			}
		}
	}
}

// bindIntermediatePrefixes binds each prefix of a dotted module path.
// For "os.path.join", it binds "os" and "os.path".
func bindIntermediatePrefixes(fullName string, imports map[string]ImportInfo) {
	parts := strings.Split(fullName, ".")
	for k := 1; k < len(parts); k++ {
		prefix := strings.Join(parts[:k], ".")
		if _, exists := imports[prefix]; !exists {
			imports[prefix] = ImportInfo{
				ModulePath:  prefix,
				IsFromStyle: false,
			}
		}
	}
}

// ResolveImport resolves an import local name to its ImportInfo.
// Returns the ImportInfo and true if found, or zero value and false otherwise.
func ResolveImport(imports map[string]ImportInfo, name string) (ImportInfo, bool) {
	info, ok := imports[name]
	return info, ok
}

// WildcardImport is the sentinel name used when "from X import *" is
// encountered. Currently skipped by BuildImportMap.
const WildcardImport = "*"
