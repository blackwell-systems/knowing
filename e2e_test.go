//go:build e2e

package knowing_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/goextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestE2E_IndexAndQuery exercises the full indexing pipeline end-to-end:
// create a temp Go module, index it, then query the resulting graph.
func TestE2E_IndexAndQuery(t *testing.T) {
	ctx := context.Background()

	// --- 1. Create temp multi-package Go module ---
	tmpDir := t.TempDir()
	writeTestModule(t, tmpDir)

	// Initialize a git repo so IndexRepo has a valid repo path.
	initGitRepo(t, tmpDir)

	// --- 2. Create SQLite store, SnapshotManager, Indexer ---
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer st.Close()

	sm := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, sm)

	// --- 3. Register GoExtractor ---
	idx.Register(goextractor.NewGoExtractor())

	// --- 4. IndexRepo ---
	repoURL := "testmod"
	snap, err := idx.IndexRepo(ctx, repoURL, tmpDir, "abc123")
	if err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}

	// --- 5. Assert snapshot NodeCount > 0, EdgeCount > 0 ---
	if snap.NodeCount == 0 {
		t.Fatal("expected NodeCount > 0")
	}
	if snap.EdgeCount == 0 {
		t.Fatal("expected EdgeCount > 0")
	}
	t.Logf("snapshot: nodes=%d edges=%d hash=%s", snap.NodeCount, snap.EdgeCount, snap.SnapshotHash)

	// --- 6. Find Hello node, verify callers include main ---
	helloNodes, err := st.NodesByName(ctx, repoURL+"://")
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}

	var helloNode *types.Node
	for i, n := range helloNodes {
		if strings.Contains(n.QualifiedName, ".Hello") && n.Kind == "function" {
			helloNode = &helloNodes[i]
			break
		}
	}
	if helloNode == nil {
		t.Fatal("Hello function node not found")
	}

	callers, err := st.TransitiveCallers(ctx, helloNode.NodeHash, 3, snap.SnapshotHash)
	if err != nil {
		t.Fatalf("TransitiveCallers: %v", err)
	}

	foundMainCaller := false
	for _, c := range callers {
		if strings.Contains(c.Node.QualifiedName, "main") {
			foundMainCaller = true
			break
		}
	}
	if !foundMainCaller {
		t.Log("callers of Hello:")
		for _, c := range callers {
			t.Logf("  depth=%d name=%s", c.Depth, c.Node.QualifiedName)
		}
		t.Fatal("expected main to be a caller of Hello")
	}

	// --- 7. BlastRadius of Hello has TotalCount >= 1 ---
	br, err := st.BlastRadius(ctx, helloNode.NodeHash, snap.SnapshotHash)
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}
	if br.TotalCount < 1 {
		t.Fatalf("expected BlastRadius TotalCount >= 1, got %d", br.TotalCount)
	}
	t.Logf("blast radius of Hello: total=%d", br.TotalCount)

	// --- 8. Verify implements edge (EnglishGreeter -> Greeter) ---
	var greeterNode *types.Node
	var englishGreeterNode *types.Node
	for i, n := range helloNodes {
		switch {
		case strings.HasSuffix(n.QualifiedName, ".Greeter") && n.Kind == "interface":
			greeterNode = &helloNodes[i]
		case strings.HasSuffix(n.QualifiedName, ".EnglishGreeter") && n.Kind == "type":
			englishGreeterNode = &helloNodes[i]
		}
	}

	if greeterNode == nil {
		t.Fatal("Greeter interface node not found")
	}
	if englishGreeterNode == nil {
		t.Fatal("EnglishGreeter type node not found")
	}

	implEdges, err := st.EdgesFrom(ctx, englishGreeterNode.NodeHash, "implements")
	if err != nil {
		t.Fatalf("EdgesFrom(EnglishGreeter, implements): %v", err)
	}

	foundImpl := false
	for _, e := range implEdges {
		if e.TargetHash == greeterNode.NodeHash {
			foundImpl = true
			break
		}
	}
	if !foundImpl {
		t.Fatal("expected implements edge from EnglishGreeter to Greeter")
	}
	t.Log("implements edge EnglishGreeter -> Greeter: found")

	// --- 9. Verify determinism (re-index, same snapshot hash) ---
	snap2, err := idx.IndexRepo(ctx, repoURL, tmpDir, "abc123")
	if err != nil {
		t.Fatalf("IndexRepo (re-index): %v", err)
	}
	if snap2.SnapshotHash != snap.SnapshotHash {
		t.Fatalf("determinism check failed: first=%s second=%s", snap.SnapshotHash, snap2.SnapshotHash)
	}
	t.Log("determinism check: passed (same snapshot hash on re-index)")
}

// writeTestModule creates a minimal multi-package Go module in dir.
func writeTestModule(t *testing.T, dir string) {
	t.Helper()

	// go.mod
	writeFile(t, filepath.Join(dir, "go.mod"), `module testmod

go 1.23
`)

	// main.go
	writeFile(t, filepath.Join(dir, "main.go"), `package main

import "testmod/pkg"

func main() {
	pkg.Hello()
}
`)

	// pkg/lib.go
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}

	writeFile(t, filepath.Join(pkgDir, "lib.go"), `package pkg

import "fmt"

// Hello prints a greeting.
func Hello() {
	fmt.Println("hello")
}
`)

	// pkg/types.go
	writeFile(t, filepath.Join(pkgDir, "types.go"), `package pkg

// Greeter is a greeting interface.
type Greeter interface {
	Greet() string
}
`)

	// pkg/impl.go
	writeFile(t, filepath.Join(pkgDir, "impl.go"), `package pkg

// EnglishGreeter greets in English.
type EnglishGreeter struct{}

// Greet returns an English greeting.
func (e EnglishGreeter) Greet() string {
	return "hello"
}
`)
}

// initGitRepo initializes a bare git repository in dir so that
// the indexer sees a valid repo root.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	// Create a minimal .git directory structure that satisfies the indexer's
	// directory walk (it skips .git dirs). We don't need a real git repo.
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
}

// writeFile creates a file at path with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
