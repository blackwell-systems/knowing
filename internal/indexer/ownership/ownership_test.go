package ownership

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestParseCodeowners_Basic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CODEOWNERS")
	content := `# CODEOWNERS file
*.go @backend-team
/docs/ @docs-team
internal/api/ @api-team @backend-team
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := ParseCodeowners(path)
	if err != nil {
		t.Fatalf("ParseCodeowners failed: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// First rule: *.go @backend-team
	if rules[0].Pattern != "*.go" {
		t.Errorf("rule[0].Pattern = %q, want %q", rules[0].Pattern, "*.go")
	}
	if len(rules[0].Owners) != 1 || rules[0].Owners[0] != "@backend-team" {
		t.Errorf("rule[0].Owners = %v, want [@backend-team]", rules[0].Owners)
	}

	// Second rule: /docs/ @docs-team
	if rules[1].Pattern != "/docs/" {
		t.Errorf("rule[1].Pattern = %q, want %q", rules[1].Pattern, "/docs/")
	}

	// Third rule: internal/api/ @api-team @backend-team
	if rules[2].Pattern != "internal/api/" {
		t.Errorf("rule[2].Pattern = %q, want %q", rules[2].Pattern, "internal/api/")
	}
	if len(rules[2].Owners) != 2 {
		t.Errorf("rule[2].Owners = %v, want 2 owners", rules[2].Owners)
	}
}

func TestParseCodeowners_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CODEOWNERS")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := ParseCodeowners(path)
	if err != nil {
		t.Fatalf("ParseCodeowners failed: %v", err)
	}
	if rules != nil {
		t.Errorf("expected nil rules for empty file, got %v", rules)
	}
}

func TestParseCodeowners_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CODEOWNERS")
	content := `# This is a comment
# Another comment

   # Indented comment

*.go @backend-team

# Final comment
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := ParseCodeowners(path)
	if err != nil {
		t.Fatalf("ParseCodeowners failed: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule (comments/blanks skipped), got %d", len(rules))
	}
	if rules[0].Pattern != "*.go" {
		t.Errorf("rule[0].Pattern = %q, want %q", rules[0].Pattern, "*.go")
	}
}

func TestFindCodeowners_RootLocation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CODEOWNERS")
	if err := os.WriteFile(path, []byte("*.go @team"), 0644); err != nil {
		t.Fatal(err)
	}

	result := FindCodeowners(dir)
	if result != path {
		t.Errorf("FindCodeowners = %q, want %q", result, path)
	}
}

func TestFindCodeowners_GithubLocation(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(githubDir, "CODEOWNERS")
	if err := os.WriteFile(path, []byte("*.go @team"), 0644); err != nil {
		t.Fatal(err)
	}

	result := FindCodeowners(dir)
	if result != path {
		t.Errorf("FindCodeowners = %q, want %q", result, path)
	}
}

func TestFindCodeowners_NotFound(t *testing.T) {
	dir := t.TempDir()

	result := FindCodeowners(dir)
	if result != "" {
		t.Errorf("FindCodeowners = %q, want empty string", result)
	}
}

func TestMatchPattern_GlobStar(t *testing.T) {
	// *.go matches any .go file regardless of directory
	tests := []struct {
		file string
		want bool
	}{
		{"main.go", true},
		{"internal/store/sqlite.go", true},
		{"README.md", false},
		{"pkg/types.go", true},
	}

	for _, tt := range tests {
		got := matchPattern("*.go", tt.file)
		if got != tt.want {
			t.Errorf("matchPattern(\"*.go\", %q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestMatchPattern_DirectorySlash(t *testing.T) {
	// internal/ matches all files under internal directory
	tests := []struct {
		file string
		want bool
	}{
		{"internal/store/sqlite.go", true},
		{"internal/mcp/server.go", true},
		{"pkg/types.go", false},
		{"cmd/main.go", false},
	}

	for _, tt := range tests {
		got := matchPattern("internal/", tt.file)
		if got != tt.want {
			t.Errorf("matchPattern(\"internal/\", %q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestMatchPattern_DoubleGlob(t *testing.T) {
	// **/test_*.go matches test files in any directory
	tests := []struct {
		file string
		want bool
	}{
		{"test_main.go", true},
		{"pkg/test_helpers.go", true},
		{"internal/store/test_sqlite.go", true},
		{"main.go", false},
		{"pkg/store.go", false},
	}

	for _, tt := range tests {
		got := matchPattern("**/test_*.go", tt.file)
		if got != tt.want {
			t.Errorf("matchPattern(\"**/test_*.go\", %q) = %v, want %v", tt.file, got, tt.want)
		}
	}
}

func TestExtractOwnership_BasicMatching(t *testing.T) {
	repoURL := "https://github.com/example/repo"
	files := []types.File{
		{FileHash: types.NewHash([]byte("file1")), Path: "internal/store/sqlite.go"},
		{FileHash: types.NewHash([]byte("file2")), Path: "docs/README.md"},
	}
	rules := []Rule{
		{Pattern: "internal/", Owners: []string{"@org/backend-team"}},
		{Pattern: "docs/", Owners: []string{"@docs-writer"}},
	}

	nodes, edges := ExtractOwnership(repoURL, files, rules)

	// Should produce 2 owner nodes and 2 edges
	if len(nodes) != 2 {
		t.Fatalf("expected 2 owner nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Verify edge types are owned_by
	for _, e := range edges {
		if e.EdgeType != "owned_by" {
			t.Errorf("edge type = %q, want %q", e.EdgeType, "owned_by")
		}
		if e.Confidence != 1.0 {
			t.Errorf("confidence = %f, want 1.0", e.Confidence)
		}
		if e.Provenance != "codeowners" {
			t.Errorf("provenance = %q, want %q", e.Provenance, "codeowners")
		}
	}
}

func TestExtractOwnership_LastMatchWins(t *testing.T) {
	repoURL := "https://github.com/example/repo"
	files := []types.File{
		{FileHash: types.NewHash([]byte("file1")), Path: "internal/store/sqlite.go"},
	}
	// Two rules match the same file; the last one wins (GitHub semantics)
	rules := []Rule{
		{Pattern: "internal/", Owners: []string{"@org/general-team"}},
		{Pattern: "internal/store/", Owners: []string{"@org/storage-team"}},
	}

	nodes, edges := ExtractOwnership(repoURL, files, rules)

	// Should produce 1 owner node (storage-team) and 1 edge
	if len(nodes) != 1 {
		t.Fatalf("expected 1 owner node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	// Verify the owner is storage-team (last match wins)
	if nodes[0].Kind != "team" {
		t.Errorf("owner kind = %q, want %q", nodes[0].Kind, "team")
	}
	expectedQN := repoURL + "://owners/org/storage-team"
	if nodes[0].QualifiedName != expectedQN {
		t.Errorf("owner QualifiedName = %q, want %q", nodes[0].QualifiedName, expectedQN)
	}
}

func TestExtractOwnership_TeamVsUser(t *testing.T) {
	repoURL := "https://github.com/example/repo"
	files := []types.File{
		{FileHash: types.NewHash([]byte("file1")), Path: "main.go"},
	}
	// Rule with both a team owner and a user owner
	rules := []Rule{
		{Pattern: "*.go", Owners: []string{"@org/backend-team", "@alice"}},
	}

	nodes, edges := ExtractOwnership(repoURL, files, rules)

	// Should produce 2 owner nodes (1 team, 1 user) and 2 edges
	if len(nodes) != 2 {
		t.Fatalf("expected 2 owner nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	// Find team and user nodes
	var teamNode, userNode *types.Node
	for i := range nodes {
		switch nodes[i].Kind {
		case "team":
			teamNode = &nodes[i]
		case "user":
			userNode = &nodes[i]
		}
	}

	if teamNode == nil {
		t.Fatal("expected a team node")
	}
	if userNode == nil {
		t.Fatal("expected a user node")
	}

	expectedTeamQN := repoURL + "://owners/org/backend-team"
	if teamNode.QualifiedName != expectedTeamQN {
		t.Errorf("team QualifiedName = %q, want %q", teamNode.QualifiedName, expectedTeamQN)
	}
	expectedUserQN := repoURL + "://owners/alice"
	if userNode.QualifiedName != expectedUserQN {
		t.Errorf("user QualifiedName = %q, want %q", userNode.QualifiedName, expectedUserQN)
	}
}

func TestExtractOwnership_EmptyRules(t *testing.T) {
	repoURL := "https://github.com/example/repo"
	files := []types.File{
		{FileHash: types.NewHash([]byte("file1")), Path: "main.go"},
	}

	nodes, edges := ExtractOwnership(repoURL, files, nil)
	if nodes != nil {
		t.Errorf("expected nil nodes, got %v", nodes)
	}
	if edges != nil {
		t.Errorf("expected nil edges, got %v", edges)
	}
}

func TestExtractOwnership_NoMatchingFiles(t *testing.T) {
	repoURL := "https://github.com/example/repo"
	files := []types.File{
		{FileHash: types.NewHash([]byte("file1")), Path: "assets/logo.png"},
		{FileHash: types.NewHash([]byte("file2")), Path: "assets/style.css"},
	}
	// Rules that won't match any of the files
	rules := []Rule{
		{Pattern: "*.go", Owners: []string{"@backend-team"}},
		{Pattern: "internal/", Owners: []string{"@org/core-team"}},
	}

	nodes, edges := ExtractOwnership(repoURL, files, rules)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}
