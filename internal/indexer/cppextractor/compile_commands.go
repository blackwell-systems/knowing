// Package cppextractor provides C++ source file extraction for the knowing
// code graph. It parses compile_commands.json (CMake compilation databases)
// to provide include paths, macro definitions, and language standard version
// for cross-file resolution.
package cppextractor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// CompileCommandEntry represents a single entry in compile_commands.json.
// The CMake format allows either a single Command string or a pre-split
// Arguments array; at most one of these will be set.
type CompileCommandEntry struct {
	Directory string   `json:"directory"`
	Command   string   `json:"command,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
	File      string   `json:"file"`
}

// CompileEntry holds parsed information for a single source file extracted
// from a compile_commands.json entry. Include paths are resolved to absolute
// paths relative to the entry's working directory.
type CompileEntry struct {
	Directory    string
	IncludePaths []string          // resolved to absolute paths
	Defines      map[string]string // -DFOO=bar -> Defines["FOO"] = "bar"
	StdVersion   string            // -std=c++17 -> "c++17"
	File         string
	WorkingDir   string
}

// CompileDB is the parsed compilation database. It is safe to call Lookup
// from multiple goroutines after LoadCompileDB returns.
type CompileDB struct {
	entries []CompileCommandEntry
	index   map[string]*CompileEntry // file path -> parsed entry
}

// searchCandidates returns the ordered list of directories to search for
// compile_commands.json under projectRoot. The search order covers common
// CMake build directory conventions.
func searchCandidates(projectRoot string) []string {
	return []string{
		projectRoot,
		filepath.Join(projectRoot, "build"),
		filepath.Join(projectRoot, "build-debug"),
		filepath.Join(projectRoot, "build-release"),
		filepath.Join(projectRoot, ".cache"),
	}
}

// globBuildDirs returns cmake-build-* subdirectories under projectRoot.
func globBuildDirs(projectRoot string) []string {
	matches, _ := filepath.Glob(filepath.Join(projectRoot, "cmake-build-*"))
	return matches
}

// LoadCompileDB auto-discovers compile_commands.json under projectRoot,
// searching well-known build directories. It returns (nil, nil) if no
// compile_commands.json is found anywhere in the search path.
func LoadCompileDB(projectRoot string) (*CompileDB, error) {
	candidates := searchCandidates(projectRoot)
	candidates = append(candidates, globBuildDirs(projectRoot)...)

	for _, dir := range candidates {
		path := filepath.Join(dir, "compile_commands.json")
		db, err := LoadCompileDBFromPath(path)
		if err != nil {
			return nil, err
		}
		if db != nil {
			return db, nil
		}
	}
	return nil, nil
}

// LoadCompileDBFromPath loads and parses a compile_commands.json file at the
// given path. It returns (nil, nil) if the file does not exist.
func LoadCompileDBFromPath(path string) (*CompileDB, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var raw []CompileCommandEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	db := &CompileDB{
		entries: raw,
		index:  make(map[string]*CompileEntry, len(raw)),
	}

	for i := range raw {
		entry := parseCommandArgs(raw[i].Command, raw[i].Arguments, raw[i].Directory)
		absFile := resolveFilePath(raw[i].File, raw[i].Directory)
		entry.File = absFile
		db.index[absFile] = entry
	}

	return db, nil
}

// Lookup returns the parsed compilation info for the given file path, or nil
// if the file is not present in the database.
func (db *CompileDB) Lookup(filePath string) *CompileEntry {
	if db == nil {
		return nil
	}
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	return db.index[abs]
}

// parseCommandArgs extracts -I, -D, and -std= flags from either a single
// command string or a pre-split arguments array. The directory parameter is
// the working directory from the compile_commands.json entry, used to resolve
// relative include paths.
func parseCommandArgs(command string, args []string, directory string) *CompileEntry {
	entry := &CompileEntry{
		Directory:  directory,
		WorkingDir: directory,
		Defines:    make(map[string]string),
	}

	// Normalize: if we have a command string, split it into args.
	if command != "" && len(args) == 0 {
		args = splitCommand(command)
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case strings.HasPrefix(arg, "-I"):
			inc := strings.TrimPrefix(arg, "-I")
			if inc == "" && i+1 < len(args) {
				i++
				inc = args[i]
			}
			entry.IncludePaths = append(entry.IncludePaths, resolveIncludePath(inc, directory))

		case strings.HasPrefix(arg, "-D"):
			def := strings.TrimPrefix(arg, "-D")
			if def == "" && i+1 < len(args) {
				i++
				def = args[i]
			}
			k, v := splitDefine(def)
			entry.Defines[k] = v

		case strings.HasPrefix(arg, "-std="):
			entry.StdVersion = strings.TrimPrefix(arg, "-std=")
		}
	}

	return entry
}

// splitCommand splits a compiler command string into arguments, respecting
// double-quoted strings. This is a simplified splitter that handles the
// common case; it does not handle escaped quotes or shell expansions.
func splitCommand(command string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(command); i++ {
		ch := command[i]

		switch {
		case ch == '"' :
			inQuotes = !inQuotes
		case ch == ' ' && !inQuotes:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// splitDefine splits a -D definition like "FOO=bar" into key and value.
// If no = sign is present, the value defaults to "1".
func splitDefine(def string) (string, string) {
	if idx := strings.IndexByte(def, '='); idx >= 0 {
		return def[:idx], def[idx+1:]
	}
	return def, "1"
}

// resolveIncludePath resolves a potentially relative include path to an
// absolute path using the entry's working directory as the base.
func resolveIncludePath(includePath, directory string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}
	return filepath.Join(directory, includePath)
}

// resolveFilePath resolves a source file path to an absolute path.
func resolveFilePath(filePath, directory string) string {
	if filepath.IsAbs(filePath) {
		return filePath
	}
	abs, err := filepath.Abs(filepath.Join(directory, filePath))
	if err != nil {
		return filePath
	}
	return abs
}
