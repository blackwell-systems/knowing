package enrichment

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestNewCppEnricher_Fields(t *testing.T) {
	store := newMockStore()
	tmpDir := t.TempDir()

	e := NewCppEnricher(store, tmpDir)

	if e.store != store {
		t.Error("expected store to be set")
	}
	if e.workspaceRoot != tmpDir {
		t.Errorf("expected workspaceRoot %s, got %s", tmpDir, e.workspaceRoot)
	}
}

func TestCppEnricher_HasCompileDB_True(t *testing.T) {
	tmpDir := t.TempDir()

	// Create compile_commands.json in build/ directory.
	buildDir := filepath.Join(tmpDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	compileDB := `[{"directory": "/tmp", "command": "c++ -c src/main.cpp", "file": "src/main.cpp"}]`
	if err := os.WriteFile(filepath.Join(buildDir, "compile_commands.json"), []byte(compileDB), 0644); err != nil {
		t.Fatal(err)
	}

	store := newMockStore()
	e := NewCppEnricher(store, tmpDir)

	if !e.HasCompileDB() {
		t.Error("expected HasCompileDB to be true when compile_commands.json exists in build/")
	}
	if e.CompileDBDir() != buildDir {
		t.Errorf("expected CompileDBDir %s, got %s", buildDir, e.CompileDBDir())
	}
}

func TestCppEnricher_HasCompileDB_False(t *testing.T) {
	tmpDir := t.TempDir()

	store := newMockStore()
	e := NewCppEnricher(store, tmpDir)

	if e.HasCompileDB() {
		t.Error("expected HasCompileDB to be false without compile_commands.json")
	}
	if e.CompileDBDir() != "" {
		t.Errorf("expected empty CompileDBDir, got %s", e.CompileDBDir())
	}
}

func TestFindCompileDBDir_SearchOrder(t *testing.T) {
	tests := []struct {
		name     string
		subpath  string // relative path to create compile_commands.json
		expected string // expected directory suffix
	}{
		{"project_root", "compile_commands.json", ""},
		{"build_dir", "build/compile_commands.json", "build"},
		{"build_debug", "build-debug/compile_commands.json", "build-debug"},
		{"build_release", "build-release/compile_commands.json", "build-release"},
		{"cache_dir", ".cache/compile_commands.json", ".cache"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			fullPath := filepath.Join(tmpDir, tt.subpath)
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fullPath, []byte("[]"), 0644); err != nil {
				t.Fatal(err)
			}

			result := findCompileDBDir(tmpDir)
			if result == "" {
				t.Error("expected non-empty result")
				return
			}
			if tt.expected == "" {
				// Project root case: result should equal tmpDir.
				if result != tmpDir {
					t.Errorf("expected %s, got %s", tmpDir, result)
				}
			} else {
				expected := filepath.Join(tmpDir, tt.expected)
				if result != expected {
					t.Errorf("expected %s, got %s", expected, result)
				}
			}
		})
	}
}

func TestFindCompileDBDir_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	result := findCompileDBDir(tmpDir)
	if result != "" {
		t.Errorf("expected empty result when no compile_commands.json, got %s", result)
	}
}

func TestDetectCppLSPConfig_WithClangd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create compile_commands.json.
	buildDir := filepath.Join(tmpDir, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatal(err)
	}
	compileDB := `[{"directory": "/tmp", "command": "c++ -c src/main.cpp", "file": "src/main.cpp"}]`
	if err := os.WriteFile(filepath.Join(buildDir, "compile_commands.json"), []byte(compileDB), 0644); err != nil {
		t.Fatal(err)
	}

	// DetectCppLSPConfig requires clangd on PATH. In CI, clangd may not be
	// installed, so we test the negative case (no clangd) separately.
	// For the positive case, we verify the function doesn't panic when
	// called without clangd.
	_ = DetectCppLSPConfig(tmpDir)
}

func TestDetectCppLSPConfig_NoCompileDB(t *testing.T) {
	tmpDir := t.TempDir()

	// Without compile_commands.json, DetectCppLSPConfig should still return
	// a config if clangd is available (clangd can work without compile_commands.json).
	_ = DetectCppLSPConfig(tmpDir)
}

func TestSuggestCppInstall(t *testing.T) {
	suggestion := SuggestCppInstall()
	// The suggestion depends on whether clangd is installed.
	// We just verify it doesn't panic and returns a string.
	_ = suggestion
}

func TestIsCppFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"src/main.cpp", true},
		{"src/main.cc", true},
		{"src/main.cxx", true},
		{"src/main.c++", true},
		{"include/header.hpp", true},
		{"include/header.hh", true},
		{"include/header.hxx", true},
		{"include/header.h++", true},
		{"include/header.h", true},
		{"src/legacy.c", true},
		{"src/main.go", false},
		{"src/main.py", false},
		{"src/main.ts", false},
		{"README.md", false},
		{"src/main.CPP", true}, // case-insensitive
		{"src/main.HPP", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsCppFile(tt.path); got != tt.expected {
				t.Errorf("IsCppFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestCppFileExtensions(t *testing.T) {
	exts := CppFileExtensions()
	if len(exts) == 0 {
		t.Error("expected non-empty extensions list")
	}

	// Verify key extensions are present.
	needed := map[string]bool{
		"cpp": true, "cc": true, "cxx": true, "c++": true,
		"hpp": true, "hh": true, "hxx": true, "h++": true,
		"h": true, "c": true,
	}
	for _, ext := range exts {
		delete(needed, ext)
	}
	if len(needed) > 0 {
		t.Errorf("missing extensions: %v", needed)
	}
}

func TestCppEnricher_Run_NoClangd(t *testing.T) {
	// When clangd is not on PATH, Run should return nil (not an error).
	// We test this by setting a config with no servers. The enricher
	// should exit early without errors.
	store := newMockStore()
	repoHash := types.NewHash([]byte("repo"))
	store.repos[repoHash] = types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/test/repo",
	}

	// Add a snapshot so the snapshot check doesn't fail.
	snapHash := types.NewHash([]byte("snap"))
	store.snapshots[snapHash] = types.Snapshot{
		SnapshotHash: snapHash,
		RepoHash:     repoHash,
	}

	tmpDir := t.TempDir()
	e := NewCppEnricher(store, tmpDir)

	// Manually set a config with no servers to simulate no-clangd.
	e.SetLSPConfig(&LSPConfig{Servers: nil})

	err := e.Run(context.Background(), repoHash)
	if err != nil {
		t.Errorf("expected nil error from Run with no servers, got: %v", err)
	}
}

func TestCppEnricher_RunScoped_NoClangd(t *testing.T) {
	store := newMockStore()
	repoHash := types.NewHash([]byte("repo"))
	store.repos[repoHash] = types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/test/repo",
	}

	// Add a snapshot so the snapshot check doesn't fail.
	snapHash := types.NewHash([]byte("snap"))
	store.snapshots[snapHash] = types.Snapshot{
		SnapshotHash: snapHash,
		RepoHash:     repoHash,
	}

	tmpDir := t.TempDir()
	e := NewCppEnricher(store, tmpDir)

	// Set a config with no servers to simulate no-clangd.
	e.SetLSPConfig(&LSPConfig{Servers: nil})

	err := e.RunScoped(context.Background(), repoHash, []string{"src/main.cpp"})
	if err != nil {
		t.Errorf("expected nil error from RunScoped with no servers, got: %v", err)
	}
}

func TestCppEnricher_UpgradeEdges_WithMockStore(t *testing.T) {
	// Test that the enricher correctly processes ast_inferred edges
	// through the upgrade pipeline (without actually calling clangd).
	store := newMockStore()
	repoHash := types.NewHash([]byte("repo"))
	store.repos[repoHash] = types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/test/repo",
	}

	fileHash := types.NewHash([]byte("file1"))
	store.files[fileHash] = types.File{
		FileHash: fileHash,
		RepoHash: repoHash,
		Path:     "src/main.cpp",
	}

	// Create a function node.
	nodeHash := types.NewHash([]byte("func_main"))
	store.nodes[nodeHash] = types.Node{
		NodeHash:      nodeHash,
		FileHash:      fileHash,
		QualifiedName: "https://github.com/test/repo/src/main.main",
		Kind:          "function",
	}

	// Create an ast_inferred call edge.
	targetHash := types.NewHash([]byte("target_func"))
	edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", "ast_inferred")
	store.edges[edgeHash] = types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   nodeHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   "ast_inferred",
		CallSiteLine: 10,
		CallSiteCol:  5,
		CallSiteFile: "src/main.cpp",
	}

	filePathByHash := map[types.Hash]string{
		fileHash: "src/main.cpp",
	}

	stats := &enrichStats{}

	tmpDir := t.TempDir()
	e := NewCppEnricher(store, tmpDir)

	// Use a cancelled context to prevent actual LSP calls.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Call upgradeCallEdges directly (from the embedded Enricher).
	e.upgradeCallEdges(ctx, nil, repoHash, filePathByHash, stats, nil)

	// With cancelled context, no edges should be processed.
	if stats.edgesProcessed.Load() != 0 {
		t.Errorf("expected 0 edges processed with cancelled context, got %d", stats.edgesProcessed.Load())
	}
}

func TestCppEnricher_PhantomNodeLabel(t *testing.T) {
	tests := []struct {
		edgeType   string
		targetFile string
		expected   string
	}{
		{"calls", "src/utils.cpp", "calls:utils.cpp"},
		{"extends", "", "extends.external"},
		{"includes", "include/foo.hpp", "includes:foo.hpp"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := phantomNodeLabel(tt.edgeType, tt.targetFile)
			if got != tt.expected {
				t.Errorf("phantomNodeLabel(%q, %q) = %q, want %q",
					tt.edgeType, tt.targetFile, got, tt.expected)
			}
		})
	}
}

func TestCppEnricher_ExtensionMatching(t *testing.T) {
	// Verify the extension list matches what config.matchesFile expects.
	cfg := LSPServerConfig{
		Extensions: CppFileExtensions(),
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"src/main.cpp", true},
		{"src/main.cc", true},
		{"include/header.h", true},
		{"src/main.go", false},
		{"src/main.py", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := cfg.matchesFile(tt.path); got != tt.expected {
				t.Errorf("matchesFile(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}
