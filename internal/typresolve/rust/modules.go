package rustresolve

import (
	"path/filepath"
	"strings"
)

// ResolveModulePath converts a Rust module path prefix to a resolved module path.
// It handles crate::, self::, super::, and external crate resolution.
func ResolveModulePath(prefix string, currentFile string) string {
	segments := strings.Split(prefix, "::")

	if len(segments) == 0 {
		return ""
	}

	switch segments[0] {
	case "crate":
		// Resolve from crate root: return remaining segments joined.
		if len(segments) == 1 {
			return "crate"
		}
		return strings.Join(segments, "::")

	case "self":
		// Resolve from current module.
		currentModule := InferModuleQN(currentFile)
		if len(segments) == 1 {
			return currentModule
		}
		remaining := strings.Join(segments[1:], "::")
		if currentModule == "crate" {
			return "crate::" + remaining
		}
		return currentModule + "::" + remaining

	case "super":
		// Resolve from parent module.
		currentModule := InferModuleQN(currentFile)
		// Go up one level: strip last :: segment
		parts := strings.Split(currentModule, "::")
		if len(parts) > 1 {
			parts = parts[:len(parts)-1]
		}
		parentModule := strings.Join(parts, "::")
		if len(segments) == 1 {
			return parentModule
		}
		remaining := strings.Join(segments[1:], "::")
		if parentModule == "crate" {
			return "crate::" + remaining
		}
		return parentModule + "::" + remaining

	default:
		// External crate: cannot resolve to local path.
		return ""
	}
}

// InferModuleQN converts a file path to a Rust module qualified name.
// Examples:
//   - src/foo/bar.rs -> crate::foo::bar
//   - src/foo/mod.rs -> crate::foo
//   - src/lib.rs -> crate
//   - src/main.rs -> crate
//   - src/resolver/types.rs -> crate::resolver::types
func InferModuleQN(filePath string) string {
	// Normalize path separators.
	p := filepath.ToSlash(filePath)

	// Strip src/ prefix if present.
	if idx := strings.Index(p, "src/"); idx != -1 {
		p = p[idx+4:]
	} else if p == "src" {
		return "crate"
	}

	// Strip .rs extension.
	p = strings.TrimSuffix(p, ".rs")

	// Handle special cases.
	base := filepath.Base(p)
	switch base {
	case "mod":
		// mod.rs: use the directory name.
		p = filepath.Dir(p)
		if p == "." {
			return "crate"
		}
	case "lib", "main":
		return "crate"
	}

	// Replace / with ::
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, "/", "::")

	if p == "" || p == "." {
		return "crate"
	}

	return "crate::" + p
}
