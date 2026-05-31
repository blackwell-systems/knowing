package javaresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// ImportInfo holds the full import context for a Java file, including
// regular imports, wildcard imports, and static imports.
type ImportInfo struct {
	// Regular: className -> packagePath (e.g., "UserService" -> "com.example.service")
	Regular map[string]string
	// WildcardPackages: list of package paths imported with .* (e.g., ["java.util", "com.example.model"])
	WildcardPackages []string
	// StaticMethods: methodName -> classQN (e.g., "assertEquals" -> "org.junit.Assert")
	StaticMethods map[string]string
	// StaticWildcardClasses: list of classQNs imported with static .* (e.g., ["java.lang.Math"])
	StaticWildcardClasses []string
}

// BuildImportMap extracts all import declarations from a Java AST root and
// builds a map from simple class name to fully-qualified package path.
// Handles regular imports (import com.pkg.Class -> "Class" -> "com.pkg"),
// static imports (import static com.pkg.Class.method -> "Class" -> "com.pkg"),
// and skips wildcard imports (import com.pkg.*).
func BuildImportMap(root *sitter.Node, content []byte) map[string]string {
	info := BuildImportInfo(root, content)
	return info.Regular
}

// BuildImportInfo extracts all import declarations from a Java AST root and
// returns full import context including wildcards and static imports.
func BuildImportInfo(root *sitter.Node, content []byte) *ImportInfo {
	info := &ImportInfo{
		Regular:       make(map[string]string),
		StaticMethods: make(map[string]string),
	}

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

		if isStatic {
			if strings.HasSuffix(text, ".*") {
				// Static wildcard import: import static com.pkg.Class.*
				classQN := strings.TrimSuffix(text, ".*")
				info.StaticWildcardClasses = append(info.StaticWildcardClasses, classQN)
				continue
			}
			// Static single method import: import static com.pkg.Class.method
			lastDot := strings.LastIndex(text, ".")
			if lastDot < 0 {
				continue
			}
			methodName := text[lastDot+1:]
			classQN := text[:lastDot]
			info.StaticMethods[methodName] = classQN

			// Also register the class itself for Class.method() patterns.
			secondLastDot := strings.LastIndex(classQN, ".")
			if secondLastDot >= 0 {
				className := classQN[secondLastDot+1:]
				classPackage := classQN[:secondLastDot]
				info.Regular[className] = classPackage
			} else {
				info.Regular[classQN] = ""
			}
			continue
		}

		// Wildcard imports (import com.example.model.*).
		if strings.HasSuffix(text, ".*") {
			pkg := strings.TrimSuffix(text, ".*")
			info.WildcardPackages = append(info.WildcardPackages, pkg)
			continue
		}

		lastDot := strings.LastIndex(text, ".")
		if lastDot < 0 {
			continue
		}

		// Regular import: "com.example.service.UserService"
		simpleName := text[lastDot+1:]
		packagePath := text[:lastDot]
		info.Regular[simpleName] = packagePath
	}

	return info
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
