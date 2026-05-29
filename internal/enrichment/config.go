package enrichment

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LSPServerConfig describes one language server to use for enrichment.
type LSPServerConfig struct {
	// Command is the server binary and its arguments (e.g., ["gopls"] or ["typescript-language-server", "--stdio"]).
	Command []string `json:"command"`
	// Extensions are file extensions this server handles (without dot, e.g., ["go"] or ["ts", "tsx"]).
	Extensions []string `json:"extensions"`
	// LanguageID is the LSP language identifier (e.g., "go", "typescript", "python").
	LanguageID string `json:"language_id"`
	// WorkDir overrides the LSP initialize rootUri. Use when the project root (Gemfile, package.json)
	// is in a subdirectory of the indexed repo. If empty, the workspace root is used.
	WorkDir string `json:"work_dir,omitempty"`
}

// LSPConfig is the top-level configuration for multi-language enrichment.
type LSPConfig struct {
	Servers []LSPServerConfig `json:"servers"`
}

// matchesFile returns true if this server config handles the given file path.
func (c *LSPServerConfig) matchesFile(path string) bool {
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	for _, e := range c.Extensions {
		if strings.EqualFold(e, ext) {
			return true
		}
	}
	return false
}

// LoadLSPConfig reads a knowing-lsp.json config file.
func LoadLSPConfig(path string) (*LSPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg LSPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DetectLSPServers auto-detects available language servers by checking for
// project marker files and verifying the server binary is on PATH.
// Returns a config with all detected servers.
func DetectLSPServers(workspaceRoot string) *LSPConfig {
	var servers []LSPServerConfig

	// Go: check for go.mod + gopls on PATH.
	// Multi-module repos (go.work) are handled at the enricher level via
	// DiscoverModules, not here. Detection just needs a go.mod somewhere.
	if _, err := os.Stat(filepath.Join(workspaceRoot, "go.mod")); err == nil {
		if _, err := lookPath("gopls"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"gopls"},
				Extensions: []string{"go"},
				LanguageID: "go",
			})
		}
	}

	// TypeScript/JavaScript: check for tsconfig.json or package.json + typescript-language-server
	if hasAny(workspaceRoot, "tsconfig.json", "package.json") {
		if _, err := lookPath("typescript-language-server"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"typescript-language-server", "--stdio"},
				Extensions: []string{"ts", "tsx", "js", "jsx"},
				LanguageID: "typescript",
			})
		}
	}

	// Python: check for pyproject.toml, setup.py, or requirements.txt + pylsp or pyright
	if hasAny(workspaceRoot, "pyproject.toml", "setup.py", "requirements.txt") {
		if _, err := lookPath("pylsp"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"pylsp"},
				Extensions: []string{"py"},
				LanguageID: "python",
			})
		} else if _, err := lookPath("pyright-langserver"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"pyright-langserver", "--stdio"},
				Extensions: []string{"py"},
				LanguageID: "python",
			})
		}
	}

	// Rust: check for Cargo.toml + rust-analyzer
	if _, err := os.Stat(filepath.Join(workspaceRoot, "Cargo.toml")); err == nil {
		if _, err := lookPath("rust-analyzer"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"rust-analyzer"},
				Extensions: []string{"rs"},
				LanguageID: "rust",
			})
		}
	}

	// Java: check for pom.xml or build.gradle + jdtls
	if hasAny(workspaceRoot, "pom.xml", "build.gradle", "build.gradle.kts") {
		if _, err := lookPath("jdtls"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"jdtls"},
				Extensions: []string{"java"},
				LanguageID: "java",
			})
		}
	}

	// C#: check for *.csproj or *.sln + OmniSharp or csharp-ls
	if hasGlob(workspaceRoot, "*.csproj", "*.sln") {
		if _, err := lookPath("OmniSharp"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"OmniSharp", "--languageserver"},
				Extensions: []string{"cs"},
				LanguageID: "csharp",
			})
		} else if csharpLS, err := lookPath("csharp-ls"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{csharpLS},
				Extensions: []string{"cs"},
				LanguageID: "csharp",
			})
		} else if dotnetToolPath := filepath.Join(os.Getenv("HOME"), ".dotnet", "tools", "csharp-ls"); fileExists(dotnetToolPath) {
			servers = append(servers, LSPServerConfig{
				Command:    []string{dotnetToolPath},
				Extensions: []string{"cs"},
				LanguageID: "csharp",
			})
		}
	}

	// Ruby: check for Gemfile + ruby-lsp or solargraph
	if _, err := os.Stat(filepath.Join(workspaceRoot, "Gemfile")); err == nil {
		if _, err := lookPath("ruby-lsp"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"ruby-lsp"},
				Extensions: []string{"rb"},
				LanguageID: "ruby",
			})
		} else if _, err := lookPath("solargraph"); err == nil {
			servers = append(servers, LSPServerConfig{
				Command:    []string{"solargraph", "stdio"},
				Extensions: []string{"rb"},
				LanguageID: "ruby",
			})
		}
	}

	return &LSPConfig{Servers: servers}
}

func hasAny(dir string, names ...string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func hasGlob(dir string, patterns ...string) bool {
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		if len(matches) > 0 {
			return true
		}
	}
	return false
}

// lookPath is a thin wrapper around exec.LookPath for testability.
var lookPath = execLookPath

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}


func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
