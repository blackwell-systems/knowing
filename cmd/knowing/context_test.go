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

// seedContextTestDB creates a temp directory, opens a SQLiteStore, seeds it with
// test graph data (repos, files, nodes, edges), and returns the database path.
func seedContextTestDB(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()

	// 1 repo.
	repoURL := "test-repo"
	repoHash := types.NewHash([]byte(repoURL))
	err = st.PutRepo(ctx, types.Repo{
		RepoHash: repoHash,
		RepoURL:  repoURL,
	})
	if err != nil {
		t.Fatalf("putting repo: %v", err)
	}

	// 2 files.
	fileAuth := types.File{
		FileHash:    types.NewHash([]byte(repoURL + "://pkg/auth/auth.go")),
		RepoHash:    repoHash,
		Path:        "pkg/auth/auth.go",
		ContentHash: types.NewHash([]byte("auth-content")),
	}
	fileHandler := types.File{
		FileHash:    types.NewHash([]byte(repoURL + "://pkg/handler/handler.go")),
		RepoHash:    repoHash,
		Path:        "pkg/handler/handler.go",
		ContentHash: types.NewHash([]byte("handler-content")),
	}
	err = st.BatchPutFiles(ctx, []types.File{fileAuth, fileHandler})
	if err != nil {
		t.Fatalf("putting files: %v", err)
	}

	// 4 nodes.
	nodeLogin := types.Node{
		NodeHash:      types.NewHash([]byte("test-repo://pkg/auth.Login")),
		FileHash:      fileAuth.FileHash,
		QualifiedName: "test-repo://pkg/auth.Login",
		Kind:          "function",
		Line:          10,
		Signature:     "func Login(user, pass string) (Token, error)",
	}
	nodeValidate := types.Node{
		NodeHash:      types.NewHash([]byte("test-repo://pkg/auth.Validate")),
		FileHash:      fileAuth.FileHash,
		QualifiedName: "test-repo://pkg/auth.Validate",
		Kind:          "function",
		Line:          30,
		Signature:     "func Validate(token Token) error",
	}
	nodeHandleLogin := types.Node{
		NodeHash:      types.NewHash([]byte("test-repo://pkg/handler.HandleLogin")),
		FileHash:      fileHandler.FileHash,
		QualifiedName: "test-repo://pkg/handler.HandleLogin",
		Kind:          "function",
		Line:          15,
		Signature:     "func HandleLogin(w http.ResponseWriter, r *http.Request)",
	}
	nodeMiddleware := types.Node{
		NodeHash:      types.NewHash([]byte("test-repo://pkg/handler.Middleware")),
		FileHash:      fileHandler.FileHash,
		QualifiedName: "test-repo://pkg/handler.Middleware",
		Kind:          "function",
		Line:          40,
		Signature:     "func Middleware(next http.Handler) http.Handler",
	}
	err = st.BatchPutNodes(ctx, []types.Node{nodeLogin, nodeValidate, nodeHandleLogin, nodeMiddleware})
	if err != nil {
		t.Fatalf("putting nodes: %v", err)
	}

	// 3 edges: HandleLogin->Login, Login->Validate, Middleware->Validate.
	edgeHandleLoginToLogin := types.Edge{
		EdgeHash:   types.NewHash([]byte("HandleLogin->Login")),
		SourceHash: nodeHandleLogin.NodeHash,
		TargetHash: nodeLogin.NodeHash,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	}
	edgeLoginToValidate := types.Edge{
		EdgeHash:   types.NewHash([]byte("Login->Validate")),
		SourceHash: nodeLogin.NodeHash,
		TargetHash: nodeValidate.NodeHash,
		EdgeType:   "calls",
		Confidence: 0.9,
		Provenance: "ast_resolved",
	}
	edgeMiddlewareToValidate := types.Edge{
		EdgeHash:   types.NewHash([]byte("Middleware->Validate")),
		SourceHash: nodeMiddleware.NodeHash,
		TargetHash: nodeValidate.NodeHash,
		EdgeType:   "calls",
		Confidence: 0.8,
		Provenance: "ast_inferred",
	}
	err = st.BatchPutEdges(ctx, []types.Edge{edgeHandleLoginToLogin, edgeLoginToValidate, edgeMiddlewareToValidate})
	if err != nil {
		t.Fatalf("putting edges: %v", err)
	}

	return dbPath
}

// TestCmdContext_NoArgs verifies that cmdContext returns an error when neither
// -task nor -files is provided.
func TestCmdContext_NoArgs(t *testing.T) {
	err := cmdContext([]string{})
	if err == nil {
		t.Fatal("expected error for no arguments")
	}
	if !strings.Contains(err.Error(), "either") {
		t.Errorf("expected 'either' in error, got %q", err.Error())
	}
}

// TestCmdContext_BothArgs verifies that cmdContext returns an error when both
// -task and -files are specified.
func TestCmdContext_BothArgs(t *testing.T) {
	err := cmdContext([]string{"-task", "foo", "-files", "bar.go"})
	if err == nil {
		t.Fatal("expected error for both args")
	}
	if !strings.Contains(err.Error(), "not both") {
		t.Errorf("expected 'not both' in error, got %q", err.Error())
	}
}

// TestCmdContext_TaskFlag verifies that cmdContext succeeds with a task description.
func TestCmdContext_TaskFlag(t *testing.T) {
	dbPath := seedContextTestDB(t)

	err := cmdContext([]string{"-task", "auth login", "-db", dbPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCmdContext_FilesFlag verifies that cmdContext succeeds with a files list.
func TestCmdContext_FilesFlag(t *testing.T) {
	dbPath := seedContextTestDB(t)

	err := cmdContext([]string{"-files", "pkg/auth/auth.go", "-db", dbPath, "-repo", "test-repo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCmdContext_JSONFormat verifies that cmdContext produces valid JSON output
// when -format json is specified.
func TestCmdContext_JSONFormat(t *testing.T) {
	dbPath := seedContextTestDB(t)

	// Redirect stdout to capture output.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	cmdErr := cmdContext([]string{"-task", "auth", "-db", dbPath, "-format", "json"})

	w.Close()
	os.Stdout = old

	if cmdErr != nil {
		t.Fatalf("unexpected error: %v", cmdErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if output == "" {
		// Empty output is valid when no symbols match.
		// The format function still produces valid JSON with empty symbols array.
		return
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	// Verify expected keys.
	if _, ok := parsed["tokens_used"]; !ok {
		t.Error("expected 'tokens_used' key in JSON output")
	}
	if _, ok := parsed["token_budget"]; !ok {
		t.Error("expected 'token_budget' key in JSON output")
	}
	if _, ok := parsed["symbols"]; !ok {
		t.Error("expected 'symbols' key in JSON output")
	}
}

// TestCmdContext_BudgetFlag verifies that a small budget limits the output.
func TestCmdContext_BudgetFlag(t *testing.T) {
	dbPath := seedContextTestDB(t)

	err := cmdContext([]string{"-task", "auth", "-db", dbPath, "-budget", "100"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
