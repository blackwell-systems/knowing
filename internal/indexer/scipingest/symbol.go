// Package scipingest parses SCIP (Source Code Intelligence Protocol) index
// files and imports their symbol definitions and references into the knowing
// knowledge graph.
package scipingest

import (
	"errors"
	"strings"
)

// ParseSCIPSymbol parses a SCIP symbol string (e.g.,
// "scip-go gomod github.com/org/repo v1.0.0 pkg/Name.") into
// its components: repo (package-name), pkgPath (descriptor path before symbol),
// symbolName (final descriptor component without suffix), and symbolKind.
func ParseSCIPSymbol(symbol string) (repo, pkgPath, symbolName, symbolKind string, err error) {
	if symbol == "" {
		return "", "", "", "", errors.New("empty symbol string")
	}

	// Handle local symbols (prefix "local ")
	if strings.HasPrefix(symbol, "local ") {
		name := strings.TrimPrefix(symbol, "local ")
		return "", "", name, "function", nil
	}

	// Standard format: scheme manager package-name version descriptor...
	// Fields are separated by spaces. The first 4 are fixed; remaining is the descriptor.
	parts := strings.SplitN(symbol, " ", 5)
	if len(parts) < 5 {
		return "", "", "", "", errors.New("invalid SCIP symbol: not enough fields")
	}

	repo = parts[2]      // package-name field (e.g., "github.com/org/repo")
	descriptor := parts[4] // everything after version

	// Parse the descriptor to extract pkgPath, symbolName, and kind.
	// Descriptors use suffixes: "." = type, "#" = field/var, "()" = method/function
	// Example descriptors: "pkg/MyType.", "pkg/MyType.Method().", "pkg/DoThing()."

	// Split descriptor into segments by analyzing suffixes
	symbolName, symbolKind, pkgPath = parseDescriptor(descriptor)

	return repo, pkgPath, symbolName, symbolKind, nil
}

// parseDescriptor extracts the symbol name, kind, and package path from a
// SCIP descriptor string. Descriptors are composed of segments, each ending
// with a suffix indicating kind.
func parseDescriptor(descriptor string) (name, kind, pkgPath string) {
	// Trim trailing whitespace
	descriptor = strings.TrimSpace(descriptor)
	if descriptor == "" {
		return "", "function", ""
	}

	// Find the last segment and its suffix to determine kind and name.
	// We work backwards from the end of the descriptor.

	// Check for method pattern: ends with "()." (method call on a type)
	if strings.HasSuffix(descriptor, "().") {
		// Method: strip "()." from end, then find the method name
		trimmed := strings.TrimSuffix(descriptor, "().")
		lastDot := strings.LastIndex(trimmed, ".")
		if lastDot >= 0 {
			name = trimmed[lastDot+1:]
			pkgPath = extractPkgPath(trimmed[:lastDot])
		} else {
			// Check if there's a "/" separating package from name
			lastSlash := strings.LastIndex(trimmed, "/")
			if lastSlash >= 0 {
				name = trimmed[lastSlash+1:]
				pkgPath = trimmed[:lastSlash]
			} else {
				name = trimmed
			}
		}
		return name, "method", pkgPath
	}

	// Check for function pattern: ends with "()"
	if strings.HasSuffix(descriptor, "()") {
		trimmed := strings.TrimSuffix(descriptor, "()")
		lastDot := strings.LastIndex(trimmed, ".")
		if lastDot >= 0 {
			name = trimmed[lastDot+1:]
			pkgPath = extractPkgPath(trimmed[:lastDot])
		} else {
			lastSlash := strings.LastIndex(trimmed, "/")
			if lastSlash >= 0 {
				name = trimmed[lastSlash+1:]
				pkgPath = trimmed[:lastSlash]
			} else {
				name = trimmed
			}
		}
		return name, "function", pkgPath
	}

	// Check for field pattern: ends with "#"
	if strings.HasSuffix(descriptor, "#") {
		trimmed := strings.TrimSuffix(descriptor, "#")
		lastDot := strings.LastIndex(trimmed, ".")
		if lastDot >= 0 {
			name = trimmed[lastDot+1:]
			pkgPath = extractPkgPath(trimmed[:lastDot])
		} else {
			lastSlash := strings.LastIndex(trimmed, "/")
			if lastSlash >= 0 {
				name = trimmed[lastSlash+1:]
				pkgPath = trimmed[:lastSlash]
			} else {
				name = trimmed
			}
		}
		return name, "var", pkgPath
	}

	// Check for type/namespace pattern: ends with "."
	if strings.HasSuffix(descriptor, ".") {
		trimmed := strings.TrimSuffix(descriptor, ".")
		lastDot := strings.LastIndex(trimmed, ".")
		if lastDot >= 0 {
			name = trimmed[lastDot+1:]
			pkgPath = extractPkgPath(trimmed[:lastDot])
		} else {
			lastSlash := strings.LastIndex(trimmed, "/")
			if lastSlash >= 0 {
				name = trimmed[lastSlash+1:]
				pkgPath = trimmed[:lastSlash]
			} else {
				name = trimmed
			}
		}
		return name, "type", pkgPath
	}

	// No recognized suffix: treat as function (fallback)
	lastSlash := strings.LastIndex(descriptor, "/")
	if lastSlash >= 0 {
		name = descriptor[lastSlash+1:]
		pkgPath = descriptor[:lastSlash]
	} else {
		name = descriptor
	}
	return name, "function", pkgPath
}

// extractPkgPath extracts the package path from a descriptor prefix.
// It handles cases like "pkg/MyType" where "pkg" is the package path
// and "MyType" is a type in the path.
func extractPkgPath(prefix string) string {
	// If prefix contains a "/" it has a package path component
	lastSlash := strings.LastIndex(prefix, "/")
	if lastSlash >= 0 {
		return prefix[:lastSlash]
	}
	// Single segment; treat as package path
	return prefix
}
