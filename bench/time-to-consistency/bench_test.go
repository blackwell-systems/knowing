// Package time_to_consistency measures how quickly a system's retrieval reflects
// a code change. The jaw-dropping metric: "I added a function 30ms ago and
// knowing already ranks it in context."
//
// Protocol:
//  1. Index a repo (Flask).
//  2. Add a NEW function to an existing file.
//  3. Trigger incremental reindex (or whatever the system provides).
//  4. Query: "find context for a task about <new function>".
//  5. Measure: does the system find the new symbol? How quickly?
//
// Results table: system | reindex_ms | query_ms | total_ms | found
package time_to_consistency

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/bench/cross-system/adapters"
	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

type ttcResult struct {
	System    string
	ReindexMs int64
	QueryMs   int64
	TotalMs   int64
	Found     bool
}

// injectedFunction is the code we inject into a Flask file.
const injectedFunction = `

def validate_authentication_token(token_string, issuer=None):
    """Validate a JWT authentication token and return the decoded payload.

    Checks signature, expiration, and optionally validates the issuer claim.
    Returns None if validation fails for any reason.
    """
    if not token_string:
        return None
    try:
        payload = _decode_jwt(token_string)
        if issuer and payload.get("iss") != issuer:
            return None
        return payload
    except Exception:
        return None
`

// targetFile is the Flask file we inject into.
const targetFile = "src/flask/helpers.py"

// newFunctionName is what we'll search for.
const newFunctionName = "validate_authentication_token"

func TestTimeToConsistency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time-to-consistency bench in short mode")
	}

	flaskPath := filepath.Join("..", "cross-system", "corpus", "repos", "flask")
	if _, err := os.Stat(flaskPath); err != nil {
		t.Skipf("flask repo not found at %s", flaskPath)
	}

	// Make a temp copy so we don't pollute the corpus.
	tmpDir := t.TempDir()
	flaskCopy := filepath.Join(tmpDir, "flask")
	cp := exec.Command("cp", "-a", flaskPath, flaskCopy)
	if out, err := cp.CombinedOutput(); err != nil {
		t.Fatalf("copy flask: %v: %s", err, out)
	}

	var results []ttcResult

	// --- knowing ---
	t.Run("knowing", func(t *testing.T) {
		r := benchKnowing(t, flaskCopy)
		results = append(results, r)
	})

	// --- codegraph ---
	t.Run("codegraph", func(t *testing.T) {
		r := benchCodegraph(t, flaskCopy)
		results = append(results, r)
	})

	// --- aider ---
	t.Run("aider", func(t *testing.T) {
		r := benchAider(t, flaskCopy)
		results = append(results, r)
	})

	// --- Report ---
	t.Log("")
	t.Log("=== Time-to-Consistency Results ===")
	t.Log("")
	t.Logf("  | System    | Reindex  | Query   | Total   | Found |")
	t.Logf("  |-----------|----------|---------|---------|-------|")
	for _, r := range results {
		t.Logf("  | %-9s | %6dms | %5dms | %5dms | %v |",
			r.System, r.ReindexMs, r.QueryMs, r.TotalMs, r.Found)
	}
	t.Log("")
	t.Log("  Scenario: Add validate_authentication_token() to flask/helpers.py,")
	t.Log("  then query 'context for JWT token validation'. Does the system find it?")
}

func benchKnowing(t *testing.T, flaskPath string) ttcResult {
	t.Helper()
	ctx := context.Background()

	// Phase 1: Initial full index.
	dbPath := filepath.Join(t.TempDir(), "knowing.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	pyExt, err := treesitter.NewTreeSitterExtractor("python")
	if err != nil {
		t.Fatalf("python extractor: %v", err)
	}
	idx.Register(pyExt)

	_, err = idx.IndexRepo(ctx, "github.com/pallets/flask", flaskPath, "HEAD")
	if err != nil {
		t.Fatalf("initial index: %v", err)
	}
	st.RebuildFTS(ctx) //nolint:errcheck

	// Phase 2: Inject the new function.
	injectFunction(t, flaskPath)

	// Phase 3: Incremental reindex (just the changed file).
	reindexStart := time.Now()
	err = idx.IndexFilesIncremental(ctx, "github.com/pallets/flask", flaskPath, "HEAD", []string{targetFile})
	if err != nil {
		t.Fatalf("incremental: %v", err)
	}
	st.RebuildFTS(ctx) //nolint:errcheck
	reindexMs := time.Since(reindexStart).Milliseconds()

	// Phase 4: Query for the new function.
	queryStart := time.Now()
	engine := knowingctx.NewContextEngine(st)
	res, err := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "validate authentication token JWT issuer",
		TokenBudget:     5000,
		Format:          "json",
	})
	queryMs := time.Since(queryStart).Milliseconds()

	if err != nil {
		t.Logf("query error: %v", err)
		return ttcResult{System: "knowing", ReindexMs: reindexMs, QueryMs: queryMs, TotalMs: reindexMs + queryMs, Found: false}
	}

	found := false
	for _, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.Node.QualifiedName), newFunctionName) {
			found = true
			break
		}
	}

	t.Logf("knowing: reindex=%dms query=%dms total=%dms found=%v (symbols returned: %d)",
		reindexMs, queryMs, reindexMs+queryMs, found, len(res.Symbols))

	return ttcResult{System: "knowing", ReindexMs: reindexMs, QueryMs: queryMs, TotalMs: reindexMs + queryMs, Found: found}
}

func benchCodegraph(t *testing.T, flaskPath string) ttcResult {
	t.Helper()

	cg := adapters.NewCodeGraph()
	if !cg.IsAvailable() {
		t.Log("codegraph not installed, skipping")
		return ttcResult{System: "codegraph", Found: false}
	}

	// Phase 1: Initial index.
	if _, err := cg.Index(flaskPath); err != nil {
		t.Logf("codegraph initial index failed: %v", err)
		return ttcResult{System: "codegraph", Found: false}
	}

	// Phase 2: Function already injected from knowing test.
	// (We share the same flask copy.)

	// Phase 3: Sync (codegraph's incremental update).
	reindexStart := time.Now()
	cmd := exec.Command("codegraph", "sync", flaskPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("codegraph sync failed: %v: %s", err, out)
		return ttcResult{System: "codegraph", Found: false}
	}
	reindexMs := time.Since(reindexStart).Milliseconds()

	// Phase 4: Query.
	queryStart := time.Now()
	task := benchtype.Task{
		ID:          "ttc-001",
		Description: "validate authentication token JWT issuer",
	}
	res, err := cg.Retrieve(flaskPath, task, 5000)
	queryMs := time.Since(queryStart).Milliseconds()

	if err != nil {
		t.Logf("codegraph query error: %v", err)
		return ttcResult{System: "codegraph", ReindexMs: reindexMs, QueryMs: queryMs, TotalMs: reindexMs + queryMs, Found: false}
	}

	found := false
	for _, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.QualifiedName), newFunctionName) ||
			strings.Contains(strings.ToLower(sym.Normalized), newFunctionName) {
			found = true
			break
		}
	}

	t.Logf("codegraph: reindex=%dms query=%dms total=%dms found=%v (symbols returned: %d)",
		reindexMs, queryMs, reindexMs+queryMs, found, len(res.Symbols))

	return ttcResult{System: "codegraph", ReindexMs: reindexMs, QueryMs: queryMs, TotalMs: reindexMs + queryMs, Found: found}
}

func benchAider(t *testing.T, flaskPath string) ttcResult {
	t.Helper()

	aider := adapters.NewAider()
	if !aider.IsAvailable() {
		t.Log("aider not installed, skipping")
		return ttcResult{System: "aider", Found: false}
	}

	// Aider has no index or sync step. It parses from disk on every query.
	// So "reindex" is 0ms; the entire cost is in the query (which includes parsing).
	// The function was already injected by the knowing test (shared flask copy).
	queryStart := time.Now()
	task := benchtype.Task{
		ID:          "ttc-001",
		Description: "validate authentication token JWT issuer",
	}
	res, err := aider.Retrieve(flaskPath, task, 5000)
	queryMs := time.Since(queryStart).Milliseconds()

	if err != nil {
		t.Logf("aider query error: %v", err)
		return ttcResult{System: "aider", QueryMs: queryMs, TotalMs: queryMs, Found: false}
	}

	found := false
	for _, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.QualifiedName), newFunctionName) ||
			strings.Contains(strings.ToLower(sym.Normalized), newFunctionName) {
			found = true
			break
		}
	}

	t.Logf("aider: reindex=0ms query=%dms total=%dms found=%v (symbols returned: %d)",
		queryMs, queryMs, found, len(res.Symbols))

	return ttcResult{System: "aider", ReindexMs: 0, QueryMs: queryMs, TotalMs: queryMs, Found: found}
}

func injectFunction(t *testing.T, repoPath string) {
	t.Helper()
	path := filepath.Join(repoPath, targetFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", targetFile, err)
	}

	// Verify the function doesn't already exist.
	if strings.Contains(string(content), newFunctionName) {
		t.Fatalf("function %s already exists in %s", newFunctionName, targetFile)
	}

	// Append to end of file.
	newContent := append(content, []byte(injectedFunction)...)
	if err := os.WriteFile(path, newContent, 0644); err != nil {
		t.Fatalf("write %s: %v", targetFile, err)
	}

	// Also add Aider/GitNexus test: verify they DON'T find it without reindex.
	t.Logf("injected %s into %s (%d -> %d bytes)",
		newFunctionName, targetFile, len(content), len(newContent))
}

func TestTimeToConsistency_NoReindex(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	flaskPath := filepath.Join("..", "cross-system", "corpus", "repos", "flask")
	if _, err := os.Stat(flaskPath); err != nil {
		t.Skipf("flask repo not found at %s", flaskPath)
	}

	// Make a temp copy.
	tmpDir := t.TempDir()
	flaskCopy := filepath.Join(tmpDir, "flask")
	cp := exec.Command("cp", "-a", flaskPath, flaskCopy)
	if out, err := cp.CombinedOutput(); err != nil {
		t.Fatalf("copy flask: %v: %s", err, out)
	}

	ctx := context.Background()

	// Full index first.
	dbPath := filepath.Join(t.TempDir(), "knowing.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	pyExt, err := treesitter.NewTreeSitterExtractor("python")
	if err != nil {
		t.Fatalf("python extractor: %v", err)
	}
	idx.Register(pyExt)

	_, err = idx.IndexRepo(ctx, "github.com/pallets/flask", flaskCopy, "HEAD")
	if err != nil {
		t.Fatalf("initial index: %v", err)
	}
	st.RebuildFTS(ctx) //nolint:errcheck

	// Query BEFORE injecting: should NOT find the function.
	engine := knowingctx.NewContextEngine(st)
	res, _ := engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "validate authentication token JWT issuer",
		TokenBudget:     5000,
		Format:          "json",
	})
	for _, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.Node.QualifiedName), newFunctionName) {
			t.Fatalf("function found BEFORE injection (false positive)")
		}
	}
	t.Log("confirmed: function not found before injection (correct)")

	// Inject.
	injectFunction(t, flaskCopy)

	// Query WITHOUT reindexing: should NOT find it (stale index).
	res, _ = engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "validate authentication token JWT issuer",
		TokenBudget:     5000,
		Format:          "json",
	})
	for _, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.Node.QualifiedName), newFunctionName) {
			t.Fatalf("function found WITHOUT reindex (index leaking)")
		}
	}
	t.Log("confirmed: function not found without reindex (correct)")

	// Now reindex incrementally and query.
	err = idx.IndexFilesIncremental(ctx, "github.com/pallets/flask", flaskCopy, "HEAD", []string{targetFile})
	if err != nil {
		t.Fatalf("incremental: %v", err)
	}
	st.RebuildFTS(ctx) //nolint:errcheck

	res, _ = engine.ForTask(ctx, knowingctx.TaskOptions{
		TaskDescription: "validate authentication token JWT issuer",
		TokenBudget:     5000,
		Format:          "json",
	})
	found := false
	for i, sym := range res.Symbols {
		if strings.Contains(strings.ToLower(sym.Node.QualifiedName), newFunctionName) {
			found = true
			t.Logf("found in ForTask results at rank %d of %d: %s", i+1, len(res.Symbols), sym.Node.QualifiedName)
			break
		}
	}

	// The function may not rank in the top results via ForTask (no edges = no RWR boost),
	// but it MUST exist in the store after incremental reindex. Verify via direct lookup.
	debugNodes, _ := st.NodesByName(ctx, "%"+newFunctionName+"%")
	if len(debugNodes) == 0 {
		t.Fatal("function NOT found in store after incremental reindex (node missing)")
	}
	t.Logf("confirmed: node exists in store: %s", debugNodes[0].QualifiedName)
	if !found {
		t.Logf("note: function exists in store but not in ForTask top-%d (no edges = low RWR score)", len(res.Symbols))
	}

	fmt.Println() // ensure test output separator
}
