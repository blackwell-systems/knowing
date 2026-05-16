package main

import (
	"bytes"
	"encoding/json"
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

// TestCmdIndex_FullFlagParses verifies that the --full flag is accepted by cmdIndex.
func TestCmdIndex_FullFlagParses(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = cmdIndex([]string{"-db", dbPath, "-url", "example.com/myrepo", "-commit", "abc123", "-full", repoDir})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdIndex with --full: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	output := buf.String()

	if !strings.Contains(output, "Indexed") {
		t.Errorf("expected 'Indexed' in output, got %q", output)
	}
	// With --full, enrichment should NOT run.
	if strings.Contains(output, "LSP enrichment") {
		t.Errorf("--full mode should not trigger LSP enrichment, got %q", output)
	}
}

// TestCmdIndex_DefaultUsesTreeSitter verifies that without --full, the default
// tree-sitter path is used and enrichment is triggered.
func TestCmdIndex_DefaultUsesTreeSitter(t *testing.T) {
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
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout and stderr.
	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	err = cmdIndex([]string{"-db", dbPath, "-url", "example.com/myrepo", "-commit", "abc123", repoDir})

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	if err != nil {
		t.Fatalf("cmdIndex without --full: %v", err)
	}

	var bufOut bytes.Buffer
	if _, err := bufOut.ReadFrom(rOut); err != nil {
		t.Fatal(err)
	}
	var bufErr bytes.Buffer
	if _, err := bufErr.ReadFrom(rErr); err != nil {
		t.Fatal(err)
	}
	output := bufOut.String()

	if !strings.Contains(output, "Indexed") {
		t.Errorf("expected 'Indexed' in output, got %q", output)
	}
	// Default mode should print enrichment message.
	if !strings.Contains(output, "Running LSP enrichment") {
		t.Errorf("expected 'Running LSP enrichment' in output, got %q", output)
	}
}

// TestUnknownSubcommand verifies that an unknown subcommand returns an error.
func TestUnknownSubcommand(t *testing.T) {
	// Capture stderr where usage is printed.
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	err = run([]string{"nonexistent"})

	w.Close()
	os.Stderr = old

	// Drain the pipe to avoid deadlock.
	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Errorf("expected 'unknown subcommand' in error, got %q", err.Error())
	}
}

// TestCmdQuery_NoArgs verifies that query without arguments returns an error.
func TestCmdQuery_NoArgs(t *testing.T) {
	err := cmdQuery([]string{})
	if err == nil {
		t.Fatal("expected error for query without arguments")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected 'usage' in error, got %q", err.Error())
	}
}

// TestCmdIndex_NoArgs verifies that index without arguments returns an error.
func TestCmdIndex_NoArgs(t *testing.T) {
	err := cmdIndex([]string{})
	if err == nil {
		t.Fatal("expected error for index without arguments")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected 'usage' in error, got %q", err.Error())
	}
}

// TestCmdQuery_NoResults verifies that query with no matches prints "No nodes found."
func TestCmdQuery_NoResults(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = cmdQuery([]string{"-db", dbPath, "nonexistent.Symbol"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdQuery: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No nodes found") {
		t.Errorf("expected 'No nodes found' in output, got %q", output)
	}
}

// TestCmdExport_EmptyDB verifies that export from an empty database produces valid JSON.
func TestCmdExport_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = cmdExport([]string{"-db", dbPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdExport: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify it's valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\noutput: %s", err, output)
	}

	// Check metadata.
	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected metadata in output")
	}
	if metadata["node_count"].(float64) != 0 {
		t.Errorf("expected 0 nodes, got %v", metadata["node_count"])
	}
	if metadata["edge_count"].(float64) != 0 {
		t.Errorf("expected 0 edges, got %v", metadata["edge_count"])
	}

	// Check nodes and edges arrays exist.
	nodes, ok := result["nodes"].([]interface{})
	if !ok {
		t.Fatal("expected nodes array in output")
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
}

// TestCmdExport_UnsupportedFormat verifies that an unsupported format returns an error.
func TestCmdExport_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	err := cmdExport([]string{"-db", dbPath, "-format", "yaml"})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format' in error, got %q", err.Error())
	}
}

// TestCmdExport_NoArgs verifies that export works with just default args (no filters).
func TestCmdExport_NoArgs(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = cmdExport([]string{"-db", dbPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdExport with no filters: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	// Verify valid JSON.
	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	metadata := result["metadata"].(map[string]interface{})
	if metadata["repo"].(string) != "all" {
		t.Errorf("expected repo=all, got %q", metadata["repo"])
	}
}

// TestCmdExport_InUsageOutput verifies that the export command appears in usage.
func TestCmdExport_InUsageOutput(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	_ = run([]string{})

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "export") {
		t.Errorf("expected 'export' in usage output, got %q", output)
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
