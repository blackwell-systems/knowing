package goextractor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// createTempModule creates a temporary Go module with the given files.
// Files is a map of relative path to file content.
func createTempModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	// Write go.mod
	goMod := "module example.com/testmod\n\ngo 1.21\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	for relPath, content := range files {
		absPath := filepath.Join(dir, relPath)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func TestBulkLoad_SinglePackage(t *testing.T) {
	dir := createTempModule(t, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	})

	result, err := BulkLoad(context.Background(), dir)
	if err != nil {
		t.Fatalf("BulkLoad failed: %v", err)
	}

	if result.FilePackages == nil {
		t.Fatal("FilePackages is nil")
	}

	mainPath := filepath.Join(dir, "main.go")
	pkg, ok := result.FilePackages[mainPath]
	if !ok {
		t.Fatalf("expected FilePackages to contain %s, got keys: %v", mainPath, keys(result.FilePackages))
	}
	if pkg.Name != "main" {
		t.Errorf("expected package name 'main', got %q", pkg.Name)
	}

	if result.Fset == nil {
		t.Error("Fset should not be nil")
	}
}

func TestBulkLoad_MultiPackage(t *testing.T) {
	dir := createTempModule(t, map[string]string{
		"pkg/a/a.go": "package a\n\nfunc Hello() string { return \"hello\" }\n",
		"pkg/b/b.go": "package b\n\nfunc World() string { return \"world\" }\n",
	})

	result, err := BulkLoad(context.Background(), dir)
	if err != nil {
		t.Fatalf("BulkLoad failed: %v", err)
	}

	aPath := filepath.Join(dir, "pkg/a/a.go")
	bPath := filepath.Join(dir, "pkg/b/b.go")

	if _, ok := result.FilePackages[aPath]; !ok {
		t.Errorf("expected FilePackages to contain %s, got keys: %v", aPath, keys(result.FilePackages))
	}
	if _, ok := result.FilePackages[bPath]; !ok {
		t.Errorf("expected FilePackages to contain %s, got keys: %v", bPath, keys(result.FilePackages))
	}

	if pkgA, ok := result.FilePackages[aPath]; ok {
		if pkgA.Name != "a" {
			t.Errorf("expected package name 'a', got %q", pkgA.Name)
		}
	}
	if pkgB, ok := result.FilePackages[bPath]; ok {
		if pkgB.Name != "b" {
			t.Errorf("expected package name 'b', got %q", pkgB.Name)
		}
	}
}

func TestBulkLoad_IncludesTestFiles(t *testing.T) {
	// Verify that _test.go files ARE included in FilePackages
	// (needed for test-scope to trace call edges from tests to production code).
	dir := createTempModule(t, map[string]string{
		"pkg/a/a.go":      "package a\n\nfunc Hello() string { return \"hello\" }\n",
		"pkg/a/a_test.go": "package a\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) { Hello() }\n",
	})

	result, err := BulkLoad(context.Background(), dir)
	if err != nil {
		t.Fatalf("BulkLoad failed: %v", err)
	}

	testPath := filepath.Join(dir, "pkg/a/a_test.go")
	if _, ok := result.FilePackages[testPath]; !ok {
		t.Errorf("expected FilePackages to contain test file %s (needed for test-scope)", testPath)
	}

	srcPath := filepath.Join(dir, "pkg/a/a.go")
	if _, ok := result.FilePackages[srcPath]; !ok {
		t.Errorf("expected FilePackages to contain %s, got keys: %v", srcPath, keys(result.FilePackages))
	}
}

func TestBulkLoad_EmptyModule(t *testing.T) {
	// A module with no .go files should return an empty map, not an error.
	dir := createTempModule(t, map[string]string{})

	result, err := BulkLoad(context.Background(), dir)
	if err != nil {
		t.Fatalf("BulkLoad failed: %v", err)
	}

	if result.FilePackages == nil {
		t.Fatal("FilePackages should not be nil")
	}

	if len(result.FilePackages) != 0 {
		t.Errorf("expected empty FilePackages, got %d entries", len(result.FilePackages))
	}
}

// keys returns the string keys of the FilePackages map for diagnostic output.
func keys[V any](m map[string]V) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
