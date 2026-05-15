package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = run([]string{"version"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != Version {
		t.Errorf("got %q, want %q", got, Version)
	}
}

func TestMainFunction_NoArgs_PrintsUsage(t *testing.T) {
	// Capture stderr where usage is printed.
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	err = run([]string{})

	w.Close()
	os.Stderr = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output, got %q", output)
	}
	if !strings.Contains(output, "serve") {
		t.Errorf("expected 'serve' in usage, got %q", output)
	}
	if !strings.Contains(output, "index") {
		t.Errorf("expected 'index' in usage, got %q", output)
	}
	if !strings.Contains(output, "query") {
		t.Errorf("expected 'query' in usage, got %q", output)
	}
	if !strings.Contains(output, "version") {
		t.Errorf("expected 'version' in usage, got %q", output)
	}
}

// TestCmdIndex_SimpleGoModule verifies that cmdIndex can index a simple Go module.
func TestCmdIndex_SimpleGoModule(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create a minimal Go module.
	repoDir := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "go.mod"), []byte("module example.com/myrepo\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n\nfunc Hello() string { return \"hello\" }\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = cmdIndex([]string{"-db", dbPath, "-url", "example.com/myrepo", "-commit", "abc123", repoDir})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdIndex: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if !strings.Contains(output, "Indexed") {
		t.Errorf("expected 'Indexed' in output, got %q", output)
	}
	if !strings.Contains(output, "Repo:") {
		t.Errorf("expected 'Repo:' in output, got %q", output)
	}
	if !strings.Contains(output, "Snapshot:") {
		t.Errorf("expected 'Snapshot:' in output, got %q", output)
	}
	if !strings.Contains(output, "Nodes:") {
		t.Errorf("expected 'Nodes:' in output, got %q", output)
	}
}
