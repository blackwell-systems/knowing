// Package normalize provides symbol name canonicalization for cross-system comparison.
// Different systems produce different formats for the same symbol. This package
// normalizes them to a common form for ground-truth matching.
package normalize

import (
	"strings"
)

// Symbol canonicalizes a qualified symbol name for comparison.
//
// Input formats handled:
//   - knowing: "github.com/org/repo/pkg.FuncName"
//   - GitNexus: "pkg/FuncName" or "pkg.FuncName"
//   - Aider: "pkg/file.go:FuncName"
//   - SCIP: "github.com/org/repo/pkg.FuncName."
//   - grep: "file.go:42:func FuncName("
//
// Output: "package.SymbolName" or "package.Type.Method"
func Symbol(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// Strip trailing dots (SCIP format)
	s = strings.TrimRight(s, ".")

	// Strip trailing parens (grep captures "func Foo(")
	s = strings.TrimRight(s, "(")

	// Handle grep format: "file.go:42:func FuncName"
	if parts := strings.SplitN(s, ":", 3); len(parts) == 3 && looksLikeFile(parts[0]) {
		content := strings.TrimSpace(parts[2])
		content = stripDeclKeyword(content)
		if idx := strings.IndexAny(content, "( {[<"); idx > 0 {
			content = content[:idx]
		}
		return content
	}

	// Handle "file.go:SymbolName" format (2-part, file:symbol)
	if parts := strings.SplitN(s, ":", 2); len(parts) == 2 && looksLikeFile(parts[0]) {
		if isNumeric(parts[1]) {
			return "" // file:line with no symbol info
		}
		return strings.TrimSpace(parts[1])
	}

	// Strip repository URL prefix
	s = stripRepoURL(s)

	// Keep only the last path component + symbol
	// "internal/auth/handler.Handler" -> "handler.Handler"
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		s = s[idx+1:]
	}

	return s
}

// MatchesGroundTruth checks if a retrieved symbol matches a ground truth entry.
// Uses normalized comparison with substring and suffix matching to handle
// different qualification levels across systems.
func MatchesGroundTruth(retrieved, groundTruth string) bool {
	r := Symbol(retrieved)
	g := Symbol(groundTruth)

	if r == "" || g == "" {
		return false
	}

	// Exact match
	if r == g {
		return true
	}

	// Case-insensitive exact match
	if strings.EqualFold(r, g) {
		return true
	}

	// Substring match (handles different qualification levels)
	if strings.Contains(r, g) || strings.Contains(g, r) {
		return true
	}

	// Suffix match: "pkg.Type.Method" matches "Type.Method"
	if strings.HasSuffix(r, "."+g) || strings.HasSuffix(g, "."+r) {
		return true
	}

	return false
}

func stripRepoURL(s string) string {
	// Remove "https://..." or "http://..." prefix
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}

	// Check if it starts with a domain (github.com, gitlab.com, etc.)
	parts := strings.Split(s, "/")
	if len(parts) >= 3 && strings.Contains(parts[0], ".") && !containsUpper(parts[0]) {
		// domain/org/repo/rest -> rest
		return strings.Join(parts[3:], "/")
	}
	return s
}

func stripDeclKeyword(s string) string {
	for _, prefix := range []string{"func ", "type ", "var ", "const ", "def ", "class ", "interface ", "struct "} {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimPrefix(s, prefix)
		}
	}
	return s
}

func looksLikeFile(s string) bool {
	exts := []string{".go", ".py", ".ts", ".tsx", ".js", ".jsx", ".rs", ".java", ".cs", ".rb"}
	for _, ext := range exts {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

func containsUpper(s string) bool {
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
