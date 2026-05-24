package enrichment

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ModuleInfo describes a Go module discovered from go.work.
type ModuleInfo struct {
	Dir  string // absolute path to module directory
	Name string // module path from go.mod (e.g., "k8s.io/api")
}

// DiscoverModules parses go.work in workspaceRoot and returns all
// module directories. If go.work does not exist, returns a single
// ModuleInfo for the workspace root itself (using go.mod's module path).
// Returns an error only if go.work exists but cannot be parsed.
func DiscoverModules(workspaceRoot string) ([]ModuleInfo, error) {
	goWorkPath := filepath.Join(workspaceRoot, "go.work")

	data, err := os.ReadFile(goWorkPath)
	if err != nil {
		if os.IsNotExist(err) {
			// No go.work: fall back to single module from go.mod.
			return discoverSingleModule(workspaceRoot)
		}
		return nil, fmt.Errorf("reading go.work: %w", err)
	}

	workFile, err := modfile.ParseWork(goWorkPath, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing go.work: %w", err)
	}

	var modules []ModuleInfo
	for _, use := range workFile.Use {
		moduleDir := use.Path
		if !filepath.IsAbs(moduleDir) {
			moduleDir = filepath.Join(workspaceRoot, moduleDir)
		}
		moduleDir = filepath.Clean(moduleDir)

		moduleName, err := readModuleName(moduleDir)
		if err != nil {
			return nil, fmt.Errorf("reading go.mod in %s: %w", moduleDir, err)
		}

		modules = append(modules, ModuleInfo{
			Dir:  moduleDir,
			Name: moduleName,
		})
	}

	return modules, nil
}

// FilesForModule filters files to those whose paths are within
// the module's directory (relative to workspace root).
func FilesForModule(files []types.File, module ModuleInfo, workspaceRoot string) []types.File {
	// If the module is the workspace root itself, all files belong to it.
	cleanModDir := filepath.Clean(module.Dir)
	cleanRoot := filepath.Clean(workspaceRoot)
	if cleanModDir == cleanRoot {
		return files
	}

	// Compute relative prefix for the module directory.
	relPrefix, err := filepath.Rel(cleanRoot, cleanModDir)
	if err != nil {
		// Should not happen with valid paths, but return empty if it does.
		return nil
	}
	// Ensure the prefix ends with a separator for correct prefix matching.
	prefix := relPrefix + string(filepath.Separator)

	var result []types.File
	for _, f := range files {
		// Normalize file path separators for comparison.
		normalized := filepath.FromSlash(f.Path)
		if strings.HasPrefix(normalized, prefix) || normalized == relPrefix {
			result = append(result, f)
		}
	}
	return result
}

// discoverSingleModule reads go.mod from workspaceRoot and returns a single ModuleInfo.
func discoverSingleModule(workspaceRoot string) ([]ModuleInfo, error) {
	moduleName, err := readModuleName(workspaceRoot)
	if err != nil {
		// No go.mod either: return empty, not an error (workspace may not be a Go project).
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return []ModuleInfo{{Dir: workspaceRoot, Name: moduleName}}, nil
}

// readModuleName reads the module path from the go.mod in the given directory.
func readModuleName(dir string) (string, error) {
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", err
	}
	modFile, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return "", fmt.Errorf("parsing go.mod: %w", err)
	}
	if modFile.Module == nil {
		return "", fmt.Errorf("go.mod in %s has no module directive", dir)
	}
	return modFile.Module.Mod.Path, nil
}
