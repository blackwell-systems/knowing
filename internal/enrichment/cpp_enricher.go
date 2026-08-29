package enrichment

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// CppEnricher wraps the generic Enricher with C++/clangd-specific logic.
// It locates compile_commands.json, constructs the clangd command with the
// appropriate flags, and falls back gracefully when clangd is not installed.
//
// The core enrichment (edge upgrade, phantom node creation, new edge discovery)
// is delegated to the generic Enricher. This wrapper only handles:
//   - clangd availability detection
//   - compile_commands.json discovery
//   - clangd command construction with --compile-commands-dir
type CppEnricher struct {
	*Enricher
	workspaceRoot string
	compileDBDir  string // directory containing compile_commands.json (empty if not found)
}

// NewCppEnricher creates a CppEnricher for the given workspace.
// It locates compile_commands.json using the standard search order.
// Returns nil if the workspace has no C++ files (determined later at Run time).
func NewCppEnricher(store types.GraphStore, workspaceRoot string) *CppEnricher {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absRoot = workspaceRoot
	}

	compileDBDir := findCompileDBDir(absRoot)

	return &CppEnricher{
		Enricher:      NewEnricher(store, absRoot),
		workspaceRoot: absRoot,
		compileDBDir:  compileDBDir,
	}
}

// DetectClangd checks whether clangd is available on PATH.
// Returns the path to the clangd binary and nil error if found.
func DetectClangd() (string, error) {
	return exec.LookPath("clangd")
}

// HasCompileDB returns true if compile_commands.json was found.
func (c *CppEnricher) HasCompileDB() bool {
	return c.compileDBDir != ""
}

// CompileDBDir returns the directory containing compile_commands.json, or empty.
func (c *CppEnricher) CompileDBDir() string {
	return c.compileDBDir
}

// DetectCppLSPConfig detects clangd availability and returns an LSPConfig
// suitable for C++ enrichment. Returns nil if clangd is not installed.
func DetectCppLSPConfig(workspaceRoot string) *LSPConfig {
	clangdPath, err := DetectClangd()
	if err != nil {
		return nil
	}

	compileDBDir := findCompileDBDir(workspaceRoot)
	command := []string{clangdPath}

	// Pass --compile-commands-dir so clangd finds the compilation database.
	if compileDBDir != "" {
		command = append(command, "--compile-commands-dir="+compileDBDir)
	}

	// Tell clangd to use header insertion from newly indexed files.
	command = append(command, "--header-insertion=never")

	return &LSPConfig{
		Servers: []LSPServerConfig{
			{
				Command:    command,
				Extensions: []string{"cpp", "cc", "cxx", "c++", "hpp", "hh", "hxx", "h++", "h", "c"},
				LanguageID: "cpp",
			},
		},
	}
}

// Run starts clangd and runs the full enrichment pipeline for C++ files.
// Returns nil if clangd is not available (graceful degradation).
func (c *CppEnricher) Run(ctx context.Context, repoHash types.Hash) error {
	clangdPath, err := DetectClangd()
	if err != nil {
		log.Printf("enrichment: clangd not found; C++ LSP enrichment skipped")
		log.Printf("enrichment: install clangd: https://clangd.llvm.org/installation")
		return nil
	}

	// Build the clangd command.
	command := []string{clangdPath}
	if c.compileDBDir != "" {
		command = append(command, "--compile-commands-dir="+c.compileDBDir)
	}
	command = append(command, "--header-insertion=never")

	c.SetLSPConfig(&LSPConfig{
		Servers: []LSPServerConfig{
			{
				Command:    command,
				Extensions: []string{"cpp", "cc", "cxx", "c++", "hpp", "hh", "hxx", "h++", "h", "c"},
				LanguageID: "cpp",
			},
		},
	})

	return c.Enricher.Run(ctx, repoHash)
}

// RunScoped runs enrichment only for edges originating from the given files.
// Returns nil if clangd is not available (graceful degradation).
func (c *CppEnricher) RunScoped(ctx context.Context, repoHash types.Hash, changedFiles []string) error {
	clangdPath, err := DetectClangd()
	if err != nil {
		log.Printf("enrichment: clangd not found; C++ LSP enrichment skipped")
		return nil
	}

	command := []string{clangdPath}
	if c.compileDBDir != "" {
		command = append(command, "--compile-commands-dir="+c.compileDBDir)
	}
	command = append(command, "--header-insertion=never")

	c.SetLSPConfig(&LSPConfig{
		Servers: []LSPServerConfig{
			{
				Command:    command,
				Extensions: []string{"cpp", "cc", "cxx", "c++", "hpp", "hh", "hxx", "h++", "h", "c"},
				LanguageID: "cpp",
			},
		},
	})

	return c.Enricher.RunScoped(ctx, repoHash, changedFiles)
}

// SuggestCppInstall returns an install suggestion for clangd, or empty string.
func SuggestCppInstall() string {
	if _, err := DetectClangd(); err == nil {
		return ""
	}
	return "C++: install clangd (https://clangd.llvm.org/installation)"
}

// findCompileDBDir searches for compile_commands.json using the standard
// search order from ADR-003. Returns the directory containing it, or empty.
func findCompileDBDir(projectRoot string) string {
	candidates := []string{
		projectRoot,
		filepath.Join(projectRoot, "build"),
		filepath.Join(projectRoot, "build-debug"),
		filepath.Join(projectRoot, "build-release"),
		filepath.Join(projectRoot, ".cache"),
	}

	// Add cmake-build-* directories.
	if matches, err := filepath.Glob(filepath.Join(projectRoot, "cmake-build-*")); err == nil {
		candidates = append(candidates, matches...)
	}

	for _, dir := range candidates {
		path := filepath.Join(dir, "compile_commands.json")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return dir
		}
	}

	return ""
}

// CppFileExtensions returns the file extensions that clangd handles.
func CppFileExtensions() []string {
	return []string{"cpp", "cc", "cxx", "c++", "hpp", "hh", "hxx", "h++", "h", "c"}
}

// IsCppFile returns true if the file path has a C/C++ extension.
func IsCppFile(path string) bool {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	ext = strings.ToLower(ext)
	for _, e := range CppFileExtensions() {
		if ext == e {
			return true
		}
	}
	return false
}

// phantomNodeLabel returns a human-readable label for a phantom node
// created for an unresolved clangd definition target.
func phantomNodeLabel(edgeType, targetFile string) string {
	if targetFile != "" {
		return fmt.Sprintf("%s:%s", edgeType, filepath.Base(targetFile))
	}
	return fmt.Sprintf("%s.external", edgeType)
}
