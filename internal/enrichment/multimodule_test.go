package enrichment

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestDiscoverModules_NoGoWork(t *testing.T) {
	dir := t.TempDir()

	// Create a go.mod in the workspace root.
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/myproject\n\ngo 1.21\n")

	modules, err := DiscoverModules(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("expected 1 module, got %d", len(modules))
	}
	if modules[0].Dir != dir {
		t.Errorf("expected Dir=%s, got %s", dir, modules[0].Dir)
	}
	if modules[0].Name != "example.com/myproject" {
		t.Errorf("expected Name=example.com/myproject, got %s", modules[0].Name)
	}
}

func TestDiscoverModules_WithGoWork(t *testing.T) {
	dir := t.TempDir()

	// Create 3 sub-modules.
	subModules := []struct {
		path string
		name string
	}{
		{"pkg/api", "example.com/project/api"},
		{"pkg/core", "example.com/project/core"},
		{"tools", "example.com/project/tools"},
	}

	goWorkContent := "go 1.21\n\nuse (\n"
	for _, sm := range subModules {
		modDir := filepath.Join(dir, sm.path)
		if err := os.MkdirAll(modDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(modDir, "go.mod"),
			"module "+sm.name+"\n\ngo 1.21\n")
		goWorkContent += "\t./" + sm.path + "\n"
	}
	goWorkContent += ")\n"

	writeFile(t, filepath.Join(dir, "go.work"), goWorkContent)

	modules, err := DiscoverModules(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(modules) != 3 {
		t.Fatalf("expected 3 modules, got %d", len(modules))
	}

	// Verify each module.
	for i, sm := range subModules {
		expectedDir := filepath.Join(dir, sm.path)
		if modules[i].Dir != expectedDir {
			t.Errorf("module[%d]: expected Dir=%s, got %s", i, expectedDir, modules[i].Dir)
		}
		if modules[i].Name != sm.name {
			t.Errorf("module[%d]: expected Name=%s, got %s", i, sm.name, modules[i].Name)
		}
	}
}

func TestDiscoverModules_MalformedGoWork(t *testing.T) {
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "go.work"), "this is not valid go.work syntax {{{\n")

	_, err := DiscoverModules(dir)
	if err == nil {
		t.Fatal("expected error for malformed go.work, got nil")
	}
}

func TestDiscoverModules_MissingModuleDir(t *testing.T) {
	dir := t.TempDir()

	// go.work references a directory that doesn't exist.
	goWorkContent := "go 1.21\n\nuse (\n\t./nonexistent\n)\n"
	writeFile(t, filepath.Join(dir, "go.work"), goWorkContent)

	_, err := DiscoverModules(dir)
	if err == nil {
		t.Fatal("expected error for missing module directory, got nil")
	}
}

func TestFilesForModule_FiltersCorrectly(t *testing.T) {
	workspaceRoot := "/workspace"

	moduleA := ModuleInfo{Dir: "/workspace/pkg/api", Name: "example.com/api"}
	moduleB := ModuleInfo{Dir: "/workspace/pkg/core", Name: "example.com/core"}
	moduleRoot := ModuleInfo{Dir: "/workspace", Name: "example.com/root"}

	files := []types.File{
		{Path: "pkg/api/handler.go"},
		{Path: "pkg/api/handler_test.go"},
		{Path: "pkg/api/sub/nested.go"},
		{Path: "pkg/core/engine.go"},
		{Path: "pkg/core/engine_test.go"},
		{Path: "pkg/core/internal/util.go"},
		{Path: "main.go"},
		{Path: "cmd/server/main.go"},
		{Path: "pkg/apiv2/compat.go"},
		{Path: "pkg/coreutils/helper.go"},
	}

	// Module A should get only pkg/api/ files (not pkg/apiv2/).
	apiFiles := FilesForModule(files, moduleA, workspaceRoot)
	if len(apiFiles) != 3 {
		t.Fatalf("moduleA: expected 3 files, got %d: %v", len(apiFiles), filePaths(apiFiles))
	}
	for _, f := range apiFiles {
		if !hasPrefix(f.Path, "pkg/api/") {
			t.Errorf("moduleA: unexpected file %s", f.Path)
		}
	}

	// Module B should get only pkg/core/ files (not pkg/coreutils/).
	coreFiles := FilesForModule(files, moduleB, workspaceRoot)
	if len(coreFiles) != 3 {
		t.Fatalf("moduleB: expected 3 files, got %d: %v", len(coreFiles), filePaths(coreFiles))
	}
	for _, f := range coreFiles {
		if !hasPrefix(f.Path, "pkg/core/") {
			t.Errorf("moduleB: unexpected file %s", f.Path)
		}
	}

	// Root module should get ALL files.
	rootFiles := FilesForModule(files, moduleRoot, workspaceRoot)
	if len(rootFiles) != len(files) {
		t.Fatalf("moduleRoot: expected %d files, got %d", len(files), len(rootFiles))
	}
}

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func filePaths(files []types.File) []string {
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	return paths
}

func hasPrefix(path, prefix string) bool {
	return len(path) >= len(prefix) && path[:len(prefix)] == prefix
}
