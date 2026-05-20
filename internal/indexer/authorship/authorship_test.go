package authorship

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// setupGitRepo creates a temporary git repo with a committed file.
// The file content is committed by the given author.
func setupGitRepo(t *testing.T, fileName, content, authorName, authorEmail string) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize git repo.
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.name", authorName)
	run(t, dir, "git", "config", "user.email", authorEmail)

	// Write the file.
	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Commit.
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "initial commit")

	return dir
}

// setupMultiAuthorRepo creates a temp git repo where a file has lines from
// multiple authors. It does this by having one author commit the initial file,
// then a second author modifies part of it.
func setupMultiAuthorRepo(t *testing.T, fileName string) string {
	t.Helper()
	dir := t.TempDir()

	// Initialize.
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.name", "Alice")
	run(t, dir, "git", "config", "user.email", "alice@example.com")

	// Alice writes the initial file: a function with 5 lines.
	content := `package main

func Hello() {
	// line 1
	// line 2
	// line 3
	// line 4
}

func World() {
	// world line 1
	// world line 2
	// world line 3
}
`
	filePath := filepath.Join(dir, fileName)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "alice initial")

	// Bob modifies lines inside Hello (lines 4-6 in the function body).
	run(t, dir, "git", "config", "user.name", "Bob")
	run(t, dir, "git", "config", "user.email", "bob@example.com")

	content2 := `package main

func Hello() {
	// bob line 1
	// bob line 2
	// bob line 3
	// bob line 4
}

func World() {
	// world line 1
	// world line 2
	// world line 3
}
`
	if err := os.WriteFile(filePath, []byte(content2), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "bob modifies Hello")

	return dir
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-01T00:00:00+0000", "GIT_COMMITTER_DATE=2024-01-01T00:00:00+0000")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", name, args, err, out)
	}
}

func TestExtractAuthorship_SingleAuthor(t *testing.T) {
	content := `package main

func Greet() {
	println("hello")
}
`
	dir := setupGitRepo(t, "main.go", content, "Alice", "alice@example.com")

	file := types.File{
		FileHash: types.NewHash([]byte("test-file")),
		Path:     "main.go",
	}

	nodeHash := types.ComputeNodeHash("https://github.com/test/repo", "main", types.EmptyHash, "Greet", "function")
	nodes := []types.Node{
		{NodeHash: nodeHash, Kind: "function", Line: 3},
	}

	authorNodes, edges, err := ExtractAuthorship("https://github.com/test/repo", dir, file, nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(authorNodes) != 1 {
		t.Fatalf("expected 1 author node, got %d", len(authorNodes))
	}
	if authorNodes[0].Kind != "author" {
		t.Errorf("expected kind 'author', got %q", authorNodes[0].Kind)
	}
	if authorNodes[0].QualifiedName != "https://github.com/test/repo://authors/Alice" {
		t.Errorf("unexpected qualified name: %s", authorNodes[0].QualifiedName)
	}

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "authored_by" {
		t.Errorf("expected edge type 'authored_by', got %q", edges[0].EdgeType)
	}
	if edges[0].Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", edges[0].Confidence)
	}
	if edges[0].Provenance != "git_blame" {
		t.Errorf("expected provenance 'git_blame', got %q", edges[0].Provenance)
	}
	if edges[0].SourceHash != nodeHash {
		t.Errorf("edge source hash does not match node hash")
	}
	if edges[0].TargetHash != authorNodes[0].NodeHash {
		t.Errorf("edge target hash does not match author node hash")
	}
}

func TestExtractAuthorship_MultipleAuthors(t *testing.T) {
	dir := setupMultiAuthorRepo(t, "main.go")

	file := types.File{
		FileHash: types.NewHash([]byte("test-file")),
		Path:     "main.go",
	}

	// Hello function starts at line 3. After Bob's modifications, all 4
	// body lines belong to Bob. The function declaration (line 3) and
	// closing brace (line 8) still belong to Alice.
	helloHash := types.ComputeNodeHash("https://github.com/test/repo", "main", types.EmptyHash, "Hello", "function")
	worldHash := types.ComputeNodeHash("https://github.com/test/repo", "main", types.EmptyHash, "World", "function")
	nodes := []types.Node{
		{NodeHash: helloHash, Kind: "function", Line: 3},
		{NodeHash: worldHash, Kind: "function", Line: 10},
	}

	authorNodes, edges, err := ExtractAuthorship("https://github.com/test/repo", dir, file, nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Hello's primary author should be Bob (4 lines vs Alice's 2 for func/brace).
	var helloEdge, worldEdge types.Edge
	for _, e := range edges {
		if e.SourceHash == helloHash {
			helloEdge = e
		} else if e.SourceHash == worldHash {
			worldEdge = e
		}
	}

	// Find Bob's author node.
	var bobHash, aliceHash types.Hash
	for _, n := range authorNodes {
		if n.QualifiedName == "https://github.com/test/repo://authors/Bob" {
			bobHash = n.NodeHash
		} else if n.QualifiedName == "https://github.com/test/repo://authors/Alice" {
			aliceHash = n.NodeHash
		}
	}

	if helloEdge.TargetHash != bobHash {
		t.Errorf("expected Hello's primary author to be Bob")
	}
	if worldEdge.TargetHash != aliceHash {
		t.Errorf("expected World's primary author to be Alice")
	}
}

func TestExtractAuthorship_NoNodes(t *testing.T) {
	file := types.File{
		FileHash: types.NewHash([]byte("test-file")),
		Path:     "main.go",
	}

	authorNodes, edges, err := ExtractAuthorship("https://github.com/test/repo", "/nonexistent", file, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if authorNodes != nil {
		t.Errorf("expected nil author nodes, got %v", authorNodes)
	}
	if edges != nil {
		t.Errorf("expected nil edges, got %v", edges)
	}
}

func TestExtractAuthorship_GitBlameError(t *testing.T) {
	dir := t.TempDir()
	// Initialize a git repo but don't create the file we'll blame.
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.name", "Test")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	// Need at least one commit for git blame to work on other files.
	if err := os.WriteFile(filepath.Join(dir, "dummy.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")

	file := types.File{
		FileHash: types.NewHash([]byte("test-file")),
		Path:     "nonexistent.go",
	}

	nodes := []types.Node{
		{NodeHash: types.NewHash([]byte("node1")), Kind: "function", Line: 1},
	}

	_, _, err := ExtractAuthorship("https://github.com/test/repo", dir, file, nodes)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestParseBlamePorcelain(t *testing.T) {
	// Simulate porcelain output for 3 lines, 2 different authors.
	// Note: for the second line from the same commit (Alice), git blame
	// does NOT repeat the author headers; only the SHA line + content line appear.
	porcelain := "abc1234567890123456789012345678901234567 1 1 2\n" +
		"author Alice\n" +
		"author-mail <alice@example.com>\n" +
		"author-time 1700000000\n" +
		"author-tz +0000\n" +
		"committer Alice\n" +
		"committer-mail <alice@example.com>\n" +
		"committer-time 1700000000\n" +
		"committer-tz +0000\n" +
		"summary initial commit\n" +
		"filename main.go\n" +
		"\tpackage main\n" +
		"abc1234567890123456789012345678901234567 2 2\n" +
		"\t\n" +
		"def1234567890123456789012345678901234567 3 3 1\n" +
		"author Bob\n" +
		"author-mail <bob@example.com>\n" +
		"author-time 1700001000\n" +
		"author-tz +0000\n" +
		"committer Bob\n" +
		"committer-mail <bob@example.com>\n" +
		"committer-time 1700001000\n" +
		"committer-tz +0000\n" +
		"summary second commit\n" +
		"filename main.go\n" +
		"\tfunc Hello() {\n"

	lines, err := parseBlamePorcelain(porcelain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	if lines[0].author != "Alice" {
		t.Errorf("line 1: expected author 'Alice', got %q", lines[0].author)
	}
	if lines[1].author != "Alice" {
		t.Errorf("line 2: expected author 'Alice', got %q", lines[1].author)
	}
	if lines[2].author != "Bob" {
		t.Errorf("line 3: expected author 'Bob', got %q", lines[2].author)
	}
}

func TestExtractAuthorship_MultipleSymbols(t *testing.T) {
	content := `package main

func FuncA() {
	println("a")
}

func FuncB() {
	println("b")
}
`
	dir := setupGitRepo(t, "main.go", content, "Charlie", "charlie@example.com")

	file := types.File{
		FileHash: types.NewHash([]byte("test-file")),
		Path:     "main.go",
	}

	hashA := types.ComputeNodeHash("https://github.com/test/repo", "main", types.EmptyHash, "FuncA", "function")
	hashB := types.ComputeNodeHash("https://github.com/test/repo", "main", types.EmptyHash, "FuncB", "function")
	nodes := []types.Node{
		{NodeHash: hashA, Kind: "function", Line: 3},
		{NodeHash: hashB, Kind: "function", Line: 7},
	}

	authorNodes, edges, err := ExtractAuthorship("https://github.com/test/repo", dir, file, nodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both functions authored by Charlie, so only 1 author node.
	if len(authorNodes) != 1 {
		t.Fatalf("expected 1 author node (dedup), got %d", len(authorNodes))
	}
	if authorNodes[0].QualifiedName != "https://github.com/test/repo://authors/Charlie" {
		t.Errorf("unexpected qualified name: %s", authorNodes[0].QualifiedName)
	}

	// But 2 edges (one per symbol).
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Both edges should point to the same author node.
	for _, e := range edges {
		if e.TargetHash != authorNodes[0].NodeHash {
			t.Errorf("edge target does not match Charlie's author node")
		}
		if e.EdgeType != "authored_by" {
			t.Errorf("expected edge type 'authored_by', got %q", e.EdgeType)
		}
	}
}

func TestFindPrimaryAuthor(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		want   string
	}{
		{
			name:   "single author",
			counts: map[string]int{"Alice": 10},
			want:   "Alice",
		},
		{
			name:   "clear winner",
			counts: map[string]int{"Alice": 3, "Bob": 7},
			want:   "Bob",
		},
		{
			name:   "tie breaks lexicographically",
			counts: map[string]int{"Zara": 5, "Alice": 5},
			want:   "Alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findPrimaryAuthor(tt.counts)
			if got != tt.want {
				t.Errorf("findPrimaryAuthor() = %q, want %q", got, tt.want)
			}
		})
	}
}
