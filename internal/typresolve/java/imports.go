package javaresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// BuildImportMap extracts all import declarations from a Java AST root and
// builds a map from simple class name to fully-qualified package path.
// Handles regular imports (import com.pkg.Class -> "Class" -> "com.pkg"),
// static imports (import static com.pkg.Class.method -> "Class" -> "com.pkg"),
// and skips wildcard imports (import com.pkg.*).
func BuildImportMap(root *sitter.Node, content []byte) map[string]string {
	imports := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil || child.Type() != "import_declaration" {
			continue
		}

		text := strings.TrimSpace(child.Content(content))
		text = strings.TrimPrefix(text, "import ")
		isStatic := strings.HasPrefix(text, "static ")
		text = strings.TrimPrefix(text, "static ")
		text = strings.TrimSuffix(text, ";")
		text = strings.TrimSpace(text)

		if text == "" {
			continue
		}

		// Skip wildcard imports (import com.example.model.*).
		if strings.HasSuffix(text, ".*") {
			continue
		}

		lastDot := strings.LastIndex(text, ".")
		if lastDot < 0 {
			continue
		}

		if isStatic {
			// For static imports like "com.example.util.MathHelper.calculate",
			// the last segment is the method name; we want the class name
			// (second-to-last segment).
			packagePath := text[:lastDot]
			secondLastDot := strings.LastIndex(packagePath, ".")
			if secondLastDot < 0 {
				// Single-segment package with a class name: e.g., "MathHelper.calculate"
				imports[packagePath] = ""
				continue
			}
			className := packagePath[secondLastDot+1:]
			classPackage := packagePath[:secondLastDot]
			imports[className] = classPackage
		} else {
			// Regular import: "com.example.service.UserService"
			simpleName := text[lastDot+1:]
			packagePath := text[:lastDot]
			imports[simpleName] = packagePath
		}
	}

	return imports
}

// ResolveImport looks up a class name in the import map, returning the
// package path and true if found, or empty string and false otherwise.
func ResolveImport(imports map[string]string, className string) (string, bool) {
	pkg, ok := imports[className]
	return pkg, ok
}

// ExtractPackage extracts the package declaration from the Java AST root.
// Returns the dotted package name (e.g., "com.example.service").
// Returns an empty string if no package declaration is found.
func ExtractPackage(root *sitter.Node, content []byte) string {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "package_declaration" {
			for j := 0; j < int(child.ChildCount()); j++ {
				pkgNode := child.Child(j)
				if pkgNode.Type() == "scoped_identifier" || pkgNode.Type() == "identifier" {
					return pkgNode.Content(content)
				}
			}
		}
	}
	return ""
}
