// Package resolve provides shared utilities for determining whether an import
// path refers to an external dependency, a standard library module, or a
// local/relative import.
package resolve

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
	panic("not implemented")
}

// Pre-built language configs.
var (
	TypeScriptConfig LangConfig
	PythonConfig     LangConfig
	RustConfig       LangConfig
	JavaConfig       LangConfig
	CSharpConfig     LangConfig
)
