package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
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

// TestExportIntegration creates a temp DB with known nodes and edges, calls
// cmdExport, and verifies the JSON output contains the expected data. It also
// tests the --repo filter to verify only matching nodes appear.
func TestExportIntegration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "export-test.db")

	// Open a store and insert some nodes and edges.
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	ctx := context.Background()
	repoURL := "https://example.com/exportrepo"
	repoHash := types.NewHash([]byte(repoURL))

	// Register repo.
	if err := st.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  repoURL,
	}); err != nil {
		t.Fatalf("PutRepo: %v", err)
	}

	// Create a file.
	fileHash := types.NewHash(append(repoHash[:], []byte("main.go")...))
	if err := st.PutFile(ctx, types.File{
		FileHash:    fileHash,
		RepoHash:    repoHash,
		Path:        "main.go",
		ContentHash: types.NewHash([]byte("content")),
	}); err != nil {
		t.Fatalf("PutFile: %v", err)
	}

	// Create two nodes.
	nodeA := types.Node{
		NodeHash:      types.ComputeNodeHash(repoURL, "main", types.EmptyHash, "FuncA", "function"),
		FileHash:      fileHash,
		QualifiedName: repoURL + "://main.FuncA",
		Kind:          "function",
		Line:          10,
		Signature:     "func FuncA()",
	}
	nodeB := types.Node{
		NodeHash:      types.ComputeNodeHash(repoURL, "main", types.EmptyHash, "FuncB", "function"),
		FileHash:      fileHash,
		QualifiedName: repoURL + "://main.FuncB",
		Kind:          "function",
		Line:          20,
		Signature:     "func FuncB()",
	}
	if err := st.PutNode(ctx, nodeA); err != nil {
		t.Fatalf("PutNode A: %v", err)
	}
	if err := st.PutNode(ctx, nodeB); err != nil {
		t.Fatalf("PutNode B: %v", err)
	}

	// Create an edge: A calls B.
	edge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(nodeA.NodeHash, nodeB.NodeHash, "calls", "ast_resolved"),
		SourceHash: nodeA.NodeHash,
		TargetHash: nodeB.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := st.PutEdge(ctx, edge); err != nil {
		t.Fatalf("PutEdge: %v", err)
	}

	// Also add a node in a different "repo" to test filtering.
	otherRepoURL := "https://example.com/otherrepo"
	otherRepoHash := types.NewHash([]byte(otherRepoURL))
	if err := st.PutRepo(ctx, types.Repo{
		RepoHash: otherRepoHash,
		RepoURL:  otherRepoURL,
	}); err != nil {
		t.Fatalf("PutRepo other: %v", err)
	}
	otherFileHash := types.NewHash(append(otherRepoHash[:], []byte("other.go")...))
	if err := st.PutFile(ctx, types.File{
		FileHash:    otherFileHash,
		RepoHash:    otherRepoHash,
		Path:        "other.go",
		ContentHash: types.NewHash([]byte("other content")),
	}); err != nil {
		t.Fatalf("PutFile other: %v", err)
	}
	otherNode := types.Node{
		NodeHash:      types.ComputeNodeHash(otherRepoURL, "other", types.EmptyHash, "OtherFunc", "function"),
		FileHash:      otherFileHash,
		QualifiedName: otherRepoURL + "://other.OtherFunc",
		Kind:          "function",
		Line:          5,
	}
	if err := st.PutNode(ctx, otherNode); err != nil {
		t.Fatalf("PutNode other: %v", err)
	}

	st.Close()

	// --- Test 1: Export all (no filter) ---

	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	os.Stdout = w

	err = cmdExport([]string{"-db", dbPath})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdExport (no filter): %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, buf.String())
	}

	var nodes []map[string]interface{}
	if err := json.Unmarshal(result["nodes"], &nodes); err != nil {
		t.Fatalf("unmarshal nodes: %v", err)
	}
	// All 3 nodes should be present (FuncA, FuncB, OtherFunc).
	if len(nodes) != 3 {
		t.Errorf("export all: expected 3 nodes, got %d", len(nodes))
	}

	var edges []map[string]interface{}
	if err := json.Unmarshal(result["edges"], &edges); err != nil {
		t.Fatalf("unmarshal edges: %v", err)
	}
	if len(edges) != 1 {
		t.Errorf("export all: expected 1 edge, got %d", len(edges))
	}
	if len(edges) > 0 {
		if edges[0]["edge_type"] != "calls" {
			t.Errorf("edge type: got %v, want calls", edges[0]["edge_type"])
		}
		if edges[0]["confidence"].(float64) != 1.0 {
			t.Errorf("edge confidence: got %v, want 1.0", edges[0]["confidence"])
		}
	}

	// --- Test 2: Export with --repo filter ---

	r2, w2, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatal(pipeErr)
	}
	os.Stdout = w2

	err = cmdExport([]string{"-db", dbPath, "-repo", repoURL})

	w2.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("cmdExport (with repo filter): %v", err)
	}

	var buf2 bytes.Buffer
	if _, err := buf2.ReadFrom(r2); err != nil {
		t.Fatal(err)
	}

	var result2 map[string]json.RawMessage
	if err := json.Unmarshal(buf2.Bytes(), &result2); err != nil {
		t.Fatalf("invalid JSON (filtered): %v", err)
	}

	var filteredNodes []map[string]interface{}
	if err := json.Unmarshal(result2["nodes"], &filteredNodes); err != nil {
		t.Fatalf("unmarshal filtered nodes: %v", err)
	}
	// Only FuncA and FuncB should be present (filtered to exportrepo).
	if len(filteredNodes) != 2 {
		t.Errorf("export filtered: expected 2 nodes, got %d", len(filteredNodes))
	}

	// Verify the other repo's node is NOT in the filtered output.
	for _, n := range filteredNodes {
		name := n["qualified_name"].(string)
		if strings.Contains(name, "OtherFunc") {
			t.Errorf("filtered export should not contain OtherFunc, but found: %s", name)
		}
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(result2["metadata"], &metadata); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if metadata["repo"] != repoURL {
		t.Errorf("metadata repo: got %v, want %s", metadata["repo"], repoURL)
	}
	if metadata["node_count"].(float64) != 2 {
		t.Errorf("metadata node_count: got %v, want 2", metadata["node_count"])
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
