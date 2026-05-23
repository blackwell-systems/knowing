// Package resolve provides shared utilities for determining whether an import
// path refers to an external dependency, a standard library module, or a
// local/relative import.
package resolve

import "strings"

// LangConfig provides language-specific rules for external URL inference.
type LangConfig struct {
	// StdlibCheck returns true if the top-level segment indicates stdlib.
	StdlibCheck func(topLevel string) bool
	// IsRelative returns true if the path is a relative/local import.
	IsRelative func(path string) bool
	// IsLocal returns true if the path is local to the project (e.g., same Java package prefix).
	// May be nil if not applicable.
	IsLocal func(path, localContext string) bool
	// ExtractPkgName extracts the package identifier from the full import path.
	// Returns the portion to use after "external://".
	ExtractPkgName func(path string) string
	// Separator is the path component separator ("/" for TS, "." for Java/C#, "::" for Rust).
	Separator string
}

// InferExternalRepoURL returns "external://{pkgName}" for external packages,
// "stdlib" for standard library imports, or "" for relative/local imports.
// The localContext parameter is optional (used by Java for same-package detection).
func InferExternalRepoURL(path string, localContext string, cfg LangConfig) string {
	if path == "" {
		return ""
	}

	// Check relative/local imports first.
	if cfg.IsRelative != nil && cfg.IsRelative(path) {
		return ""
	}

	// Check project-local imports (e.g., Java same-package).
	if cfg.IsLocal != nil && cfg.IsLocal(path, localContext) {
		return ""
	}

	// Extract top-level segment for stdlib check.
	topLevel := path
	if cfg.Separator != "" {
		if idx := strings.Index(path, cfg.Separator); idx > 0 {
			topLevel = path[:idx]
		}
	}

	// Check stdlib.
	if cfg.StdlibCheck != nil && cfg.StdlibCheck(topLevel) {
		return "stdlib"
	}

	// External: extract package name.
	if cfg.ExtractPkgName != nil {
		return "external://" + cfg.ExtractPkgName(path)
	}

	return "external://" + topLevel
}

// PythonStdlibSet is the set of known Python stdlib top-level modules.
var PythonStdlibSet = map[string]bool{
	"os": true, "sys": true, "re": true, "io": true, "json": true,
	"math": true, "time": true, "datetime": true, "collections": true,
	"itertools": true, "functools": true, "pathlib": true, "typing": true,
	"abc": true, "ast": true, "asyncio": true, "base64": true,
	"contextlib": true, "copy": true, "csv": true, "dataclasses": true,
	"enum": true, "errno": true, "glob": true, "hashlib": true,
	"http": true, "importlib": true, "inspect": true, "logging": true,
	"multiprocessing": true, "operator": true, "pickle": true,
	"platform": true, "pprint": true, "queue": true, "random": true,
	"shutil": true, "signal": true, "socket": true, "sqlite3": true,
	"string": true, "struct": true, "subprocess": true, "tempfile": true,
	"textwrap": true, "threading": true, "traceback": true, "unittest": true,
	"urllib": true, "uuid": true, "warnings": true, "weakref": true,
	"xml": true, "zipfile": true, "argparse": true, "configparser": true,
	"email": true, "html": true, "ssl": true, "secrets": true,
	"statistics": true, "types": true,
}

// TypeScriptConfig provides language-specific rules for TypeScript/JavaScript imports.
var TypeScriptConfig = LangConfig{
	StdlibCheck: nil, // TypeScript has no stdlib concept for imports.
	IsRelative: func(path string) bool {
		return strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/")
	},
	IsLocal: nil,
	ExtractPkgName: func(path string) string {
		// Scoped packages: @scope/name/subpath -> @scope/name
		if strings.HasPrefix(path, "@") {
			parts := strings.SplitN(path, "/", 3)
			if len(parts) >= 2 {
				return parts[0] + "/" + parts[1]
			}
			return path
		}
		// Unscoped: name/subpath -> name
		if idx := strings.Index(path, "/"); idx > 0 {
			return path[:idx]
		}
		return path
	},
	Separator: "/",
}

// PythonConfig provides language-specific rules for Python imports.
var PythonConfig = LangConfig{
	StdlibCheck: func(topLevel string) bool {
		return PythonStdlibSet[topLevel]
	},
	IsRelative: func(path string) bool {
		return strings.HasPrefix(path, ".")
	},
	IsLocal: nil,
	ExtractPkgName: func(path string) string {
		// Python: top-level module name only (before first ".").
		if idx := strings.Index(path, "."); idx > 0 {
			return path[:idx]
		}
		return path
	},
	Separator: ".",
}

// RustConfig provides language-specific rules for Rust imports.
var RustConfig = LangConfig{
	StdlibCheck: func(topLevel string) bool {
		switch topLevel {
		case "std", "core", "alloc":
			return true
		}
		return false
	},
	IsRelative: func(path string) bool {
		parts := strings.SplitN(path, "::", 2)
		if len(parts) == 0 {
			return false
		}
		switch parts[0] {
		case "crate", "super", "self":
			return true
		}
		return false
	},
	IsLocal: nil,
	ExtractPkgName: func(path string) string {
		// Rust: first segment before "::" is the crate name.
		if idx := strings.Index(path, "::"); idx > 0 {
			return path[:idx]
		}
		return path
	},
	Separator: "::",
}

// JavaConfig provides language-specific rules for Java imports.
var JavaConfig = LangConfig{
	StdlibCheck: func(topLevel string) bool {
		return topLevel == "java" || topLevel == "javax"
	},
	IsRelative: nil, // Java has no relative imports.
	IsLocal: func(path, localContext string) bool {
		if localContext == "" {
			return false
		}
		parts := strings.Split(path, ".")
		localParts := strings.Split(localContext, ".")
		if len(localParts) >= 2 && len(parts) >= 2 &&
			localParts[0] == parts[0] && localParts[1] == parts[1] {
			return true
		}
		return false
	},
	ExtractPkgName: func(path string) string {
		// Java: use first 2 dot-separated segments as group identifier.
		parts := strings.Split(path, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return path
	},
	Separator: ".",
}

// CSharpConfig provides language-specific rules for C# imports.
var CSharpConfig = LangConfig{
	StdlibCheck: func(topLevel string) bool {
		return topLevel == "System" || topLevel == "Microsoft"
	},
	IsRelative: nil, // C# has no relative imports.
	IsLocal:    nil,
	ExtractPkgName: func(path string) string {
		// C#: use first 2 dot-separated segments as identifier.
		parts := strings.Split(path, ".")
		if len(parts) >= 2 {
			return parts[0] + "." + parts[1]
		}
		return path
	},
	Separator: ".",
}
