package rubyresolve

import (
	"path"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// ResolveRequire resolves a require or require_relative path to a module-level
// qualified name. For require_relative, the path is resolved relative to the
// directory of currentFile. For require, the path is returned as-is (gem paths
// are already qualified). The ".rb" extension is stripped if present.
// Returns (resolvedModulePath, true) on success, ("", false) on failure.
func ResolveRequire(requirePath string, currentFile string, isRelative bool) (string, bool) {
	if requirePath == "" {
		return "", false
	}

	if isRelative {
		dir := filepath.Dir(currentFile)
		resolved := filepath.Join(dir, requirePath)
		// Normalize to forward slashes for consistency.
		resolved = filepath.ToSlash(resolved)
		// Clean the path to resolve ".." and "." components.
		resolved = path.Clean(resolved)
		// Strip .rb extension if present.
		resolved = strings.TrimSuffix(resolved, ".rb")
		return resolved, true
	}

	// Standard require: return as-is, stripping .rb if present.
	resolved := strings.TrimSuffix(requirePath, ".rb")
	return resolved, true
}

// extractStringContent extracts the string value from a tree-sitter string node.
// It looks for a string_content child first, then falls back to trimming quotes.
func extractStringContent(strNode *sitter.Node, content []byte) string {
	for i := 0; i < int(strNode.ChildCount()); i++ {
		child := strNode.Child(i)
		if child.Type() == "string_content" {
			return child.Content(content)
		}
	}
	// Fallback: trim quotes from full node content.
	val := strNode.Content(content)
	return strings.Trim(val, `"'`)
}

// BuildRequireMap builds a per-file require map from the Ruby AST root.
// It walks top-level call nodes for require/require_relative, extracts string
// arguments, and resolves paths. Returns map[localBinding]modulePath.
func BuildRequireMap(root *sitter.Node, content []byte, currentFile string) map[string]string {
	result := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "call" {
			continue
		}

		// Get the method field child.
		methodNode := child.ChildByFieldName("method")
		if methodNode == nil {
			continue
		}
		methodName := methodNode.Content(content)
		if methodName != "require" && methodName != "require_relative" {
			continue
		}

		// Get the arguments field child.
		argsNode := child.ChildByFieldName("arguments")
		if argsNode == nil {
			continue
		}

		// Find the first string argument.
		var strValue string
		for j := 0; j < int(argsNode.ChildCount()); j++ {
			argChild := argsNode.Child(j)
			if argChild.Type() == "string" {
				strValue = extractStringContent(argChild, content)
				break
			}
		}
		if strValue == "" {
			continue
		}

		isRelative := methodName == "require_relative"
		resolved, ok := ResolveRequire(strValue, currentFile, isRelative)
		if !ok {
			continue
		}

		// Map the last path segment as local binding.
		lastSeg := path.Base(resolved)
		result[lastSeg] = resolved

		// Also map the full path for direct lookups.
		if lastSeg != resolved {
			result[resolved] = resolved
		}
	}

	return result
}
