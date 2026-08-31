package cppextractor

import (
	"os"
	"path/filepath"
	"testing"
)

func sampleCompileDB() string {
	return `[
  {
    "directory": "/home/user/project/build",
    "command": "g++ -I../include -I/usr/include -DDEBUG -DVERSION=2 -std=c++17 -c ../src/main.cpp",
    "file": "../src/main.cpp"
  },
  {
    "directory": "/home/user/project/build",
    "arguments": ["g++", "-I", "../lib/include", "-DNDEBUG", "-std=c++20", "-c", "../src/util.cpp"],
    "file": "../src/util.cpp"
  }
]`
}

func TestLoadCompileDBFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compile_commands.json")
	if err := os.WriteFile(path, []byte(sampleCompileDB()), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := LoadCompileDBFromPath(path)
	if err != nil {
		t.Fatalf("LoadCompileDBFromPath: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil CompileDB")
	}
	if len(db.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(db.entries))
	}
}

func TestLoadCompileDBFromPath_Missing(t *testing.T) {
	db, err := LoadCompileDBFromPath("/nonexistent/compile_commands.json")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if db != nil {
		t.Fatal("expected nil CompileDB for missing file")
	}
}

func TestLoadCompileDBFromPath_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compile_commands.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCompileDBFromPath(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLookup_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compile_commands.json")
	if err := os.WriteFile(path, []byte(sampleCompileDB()), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := LoadCompileDBFromPath(path)
	if err != nil {
		t.Fatalf("LoadCompileDBFromPath: %v", err)
	}

	// Look up by absolute path (resolveFilePath resolves relative paths against directory).
	absFile := filepath.Join("/home/user/project/build", "..", "src", "main.cpp")
	absFile = filepath.Clean(absFile)
	entry := db.Lookup(absFile)
	if entry == nil {
		t.Fatalf("expected non-nil entry for %s", absFile)
	}

	// Should have 2 include paths from the command string.
	if len(entry.IncludePaths) != 2 {
		t.Fatalf("expected 2 include paths, got %d: %v", len(entry.IncludePaths), entry.IncludePaths)
	}
	if entry.IncludePaths[0] != filepath.Join("/home/user/project/build", "..", "include") {
		t.Errorf("include path[0] = %q, want project/build/../include", entry.IncludePaths[0])
	}

	// Should have 2 defines.
	if len(entry.Defines) != 2 {
		t.Fatalf("expected 2 defines, got %d: %v", len(entry.Defines), entry.Defines)
	}
	if entry.Defines["DEBUG"] != "1" {
		t.Errorf("Defines[DEBUG] = %q, want %q", entry.Defines["DEBUG"], "1")
	}
	if entry.Defines["VERSION"] != "2" {
		t.Errorf("Defines[VERSION] = %q, want %q", entry.Defines["VERSION"], "2")
	}

	if entry.StdVersion != "c++17" {
		t.Errorf("StdVersion = %q, want %q", entry.StdVersion, "c++17")
	}
}

func TestLookup_SecondEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compile_commands.json")
	if err := os.WriteFile(path, []byte(sampleCompileDB()), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := LoadCompileDBFromPath(path)
	if err != nil {
		t.Fatalf("LoadCompileDBFromPath: %v", err)
	}

	absFile := filepath.Clean(filepath.Join("/home/user/project/build", "..", "src", "util.cpp"))
	entry := db.Lookup(absFile)
	if entry == nil {
		t.Fatalf("expected non-nil entry for util.cpp")
	}

	if entry.StdVersion != "c++20" {
		t.Errorf("StdVersion = %q, want %q", entry.StdVersion, "c++20")
	}
	if len(entry.IncludePaths) != 1 {
		t.Fatalf("expected 1 include path, got %d", len(entry.IncludePaths))
	}
	if entry.Defines["NDEBUG"] != "1" {
		t.Errorf("Defines[NDEBUG] = %q, want %q", entry.Defines["NDEBUG"], "1")
	}
}

func TestLookup_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "compile_commands.json")
	if err := os.WriteFile(path, []byte(sampleCompileDB()), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := LoadCompileDBFromPath(path)
	if err != nil {
		t.Fatalf("LoadCompileDBFromPath: %v", err)
	}

	entry := db.Lookup("/nonexistent/file.cpp")
	if entry != nil {
		t.Fatal("expected nil for missing file")
	}
}

func TestLookup_NilDB(t *testing.T) {
	var db *CompileDB
	if db.Lookup("anything") != nil {
		t.Fatal("expected nil from nil CompileDB")
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"g++ -I include -DFOO -std=c++17 -c file.cpp", []string{"g++", "-I", "include", "-DFOO", "-std=c++17", "-c", "file.cpp"}},
		{`g++ -I"some dir" -c file.cpp`, []string{"g++", "-Isome dir", "-c", "file.cpp"}},
		{"", nil},
	}
	for _, tt := range tests {
		got := splitCommand(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitCommand(%q) = %v, want %v (len %d vs %d)", tt.input, got, tt.want, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCommand(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSplitDefine(t *testing.T) {
	tests := []struct {
		input string
		key   string
		value string
	}{
		{"FOO=bar", "FOO", "bar"},
		{"FOO=", "FOO", ""},
		{"NDEBUG", "NDEBUG", "1"},
		{"=value", "", "value"},
	}
	for _, tt := range tests {
		k, v := splitDefine(tt.input)
		if k != tt.key || v != tt.value {
			t.Errorf("splitDefine(%q) = (%q, %q), want (%q, %q)", tt.input, k, v, tt.key, tt.value)
		}
	}
}

func TestResolveIncludePath(t *testing.T) {
	tests := []struct {
		include string
		dir     string
		want    string
	}{
		{"/usr/include", "/tmp/build", "/usr/include"},
		{"../include", "/tmp/build", filepath.Join("/tmp/build", "..", "include")},
		{"include", "/tmp/project", filepath.Join("/tmp/project", "include")},
	}
	for _, tt := range tests {
		got := resolveIncludePath(tt.include, tt.dir)
		if got != tt.want {
			t.Errorf("resolveIncludePath(%q, %q) = %q, want %q", tt.include, tt.dir, got, tt.want)
		}
	}
}

func TestLoadCompileDB_AutoDiscover(t *testing.T) {
	projectRoot := t.TempDir()
	buildDir := filepath.Join(projectRoot, "build")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(buildDir, "compile_commands.json")
	if err := os.WriteFile(path, []byte(sampleCompileDB()), 0o644); err != nil {
		t.Fatal(err)
	}

	db, err := LoadCompileDB(projectRoot)
	if err != nil {
		t.Fatalf("LoadCompileDB: %v", err)
	}
	if db == nil {
		t.Fatal("expected non-nil CompileDB from auto-discovery")
	}
}

func TestLoadCompileDB_NotFound(t *testing.T) {
	projectRoot := t.TempDir()
	db, err := LoadCompileDB(projectRoot)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if db != nil {
		t.Fatal("expected nil CompileDB when no compile_commands.json exists")
	}
}
