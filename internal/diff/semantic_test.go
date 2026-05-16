package diff

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func newTestStore(t *testing.T) types.GraphStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func putRepo(t *testing.T, s types.GraphStore, url string) types.Repo {
	t.Helper()
	r := types.Repo{
		RepoHash:    types.NewHash([]byte(url)),
		RepoURL:     url,
		LastCommit:  "abc123",
		LastIndexed: time.Now().Unix(),
	}
	if err := s.PutRepo(context.Background(), r); err != nil {
		t.Fatalf("PutRepo: %v", err)
	}
	return r
}

func putFile(t *testing.T, s types.GraphStore, repo types.Repo, path string) types.File {
	t.Helper()
	content := []byte(path + " content")
	f := types.File{
		FileHash:    types.NewHash(append(repo.RepoHash[:], []byte(path)...)),
		RepoHash:    repo.RepoHash,
		Path:        path,
		ContentHash: types.NewHash(content),
	}
	if err := s.PutFile(context.Background(), f); err != nil {
		t.Fatalf("PutFile: %v", err)
	}
	return f
}

func putNode(t *testing.T, s types.GraphStore, file types.File, name, kind string) types.Node {
	t.Helper()
	n := types.Node{
		NodeHash:      types.ComputeNodeHash("test", "pkg", file.ContentHash, name, kind),
		FileHash:      file.FileHash,
		QualifiedName: name,
		Kind:          kind,
		Line:          42,
		Signature:     "func " + name + "()",
	}
	if err := s.PutNode(context.Background(), n); err != nil {
		t.Fatalf("PutNode: %v", err)
	}
	return n
}

func putEdge(t *testing.T, s types.GraphStore, source, target types.Node, edgeType string) types.Edge {
	t.Helper()
	e := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(source.NodeHash, target.NodeHash, edgeType, "{}"),
		SourceHash: source.NodeHash,
		TargetHash: target.NodeHash,
		EdgeType:   edgeType,
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := s.PutEdge(context.Background(), e); err != nil {
		t.Fatalf("PutEdge: %v", err)
	}
	return e
}

func recordEdgeEvent(t *testing.T, s types.GraphStore, edge types.Edge, eventType string, snapshot types.Hash) {
	t.Helper()
	ev := types.EdgeEvent{
		EdgeHash:     edge.EdgeHash,
		EventType:    eventType,
		SnapshotHash: snapshot,
		SourceCommit: "test-commit",
		IndexerVer:   "v1",
		Timestamp:    time.Now().Unix(),
	}
	if err := s.RecordEdgeEvent(context.Background(), ev); err != nil {
		t.Fatalf("RecordEdgeEvent: %v", err)
	}
}

func TestSemanticDiff_EdgeChanges(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	n1 := putNode(t, s, file, "main.Foo", "function")
	n2 := putNode(t, s, file, "main.Bar", "function")

	edge := putEdge(t, s, n1, n2, "calls")

	oldSnap := types.NewHash([]byte("old-snap"))
	newSnap := types.NewHash([]byte("new-snap"))

	// Record edge added in new snapshot.
	recordEdgeEvent(t, s, edge, "added", newSnap)

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	// Edge should be enriched with source/target names.
	if len(result.EdgesAdded) != 1 {
		t.Fatalf("expected 1 added edge, got %d", len(result.EdgesAdded))
	}
	ea := result.EdgesAdded[0]
	if ea.SourceName != "main.Foo" {
		t.Errorf("expected source name main.Foo, got %s", ea.SourceName)
	}
	if ea.TargetName != "main.Bar" {
		t.Errorf("expected target name main.Bar, got %s", ea.TargetName)
	}
	if ea.EdgeType != "calls" {
		t.Errorf("expected edge type calls, got %s", ea.EdgeType)
	}

	// n1 should appear as a modified node (edge changed, but node not added/removed).
	if len(result.NodesModified) != 1 {
		t.Fatalf("expected 1 modified node, got %d", len(result.NodesModified))
	}
	mod := result.NodesModified[0]
	if mod.QualifiedName != "main.Foo" {
		t.Errorf("expected modified node main.Foo, got %s", mod.QualifiedName)
	}

	// Summary should reflect counts.
	if result.Summary.EdgesAdded != 1 {
		t.Errorf("summary edges added: want 1, got %d", result.Summary.EdgesAdded)
	}
	if result.Summary.NodesModified != 1 {
		t.Errorf("summary nodes modified: want 1, got %d", result.Summary.NodesModified)
	}
}

func TestSemanticDiff_EmptyDiff(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	snap := types.NewHash([]byte("same-snap"))

	result, err := SemanticDiff(ctx, s, snap, snap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	if result.Summary.NodesAdded != 0 {
		t.Errorf("expected 0 nodes added, got %d", result.Summary.NodesAdded)
	}
	if result.Summary.NodesRemoved != 0 {
		t.Errorf("expected 0 nodes removed, got %d", result.Summary.NodesRemoved)
	}
	if result.Summary.NodesModified != 0 {
		t.Errorf("expected 0 nodes modified, got %d", result.Summary.NodesModified)
	}
	if result.Summary.EdgesAdded != 0 {
		t.Errorf("expected 0 edges added, got %d", result.Summary.EdgesAdded)
	}
	if result.Summary.EdgesRemoved != 0 {
		t.Errorf("expected 0 edges removed, got %d", result.Summary.EdgesRemoved)
	}
}

func TestSemanticDiff_EdgeRemoved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	n1 := putNode(t, s, file, "main.Alpha", "function")
	n2 := putNode(t, s, file, "main.Beta", "function")

	edge := putEdge(t, s, n1, n2, "calls")

	oldSnap := types.NewHash([]byte("old"))
	newSnap := types.NewHash([]byte("new"))

	// Record edge as removed in new snapshot.
	recordEdgeEvent(t, s, edge, "removed", newSnap)

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	if len(result.EdgesRemoved) != 1 {
		t.Fatalf("expected 1 removed edge, got %d", len(result.EdgesRemoved))
	}
	if result.Summary.EdgesRemoved != 1 {
		t.Errorf("summary edges removed: want 1, got %d", result.Summary.EdgesRemoved)
	}
	// n1 (source of removed edge) should be modified.
	if len(result.NodesModified) != 1 {
		t.Fatalf("expected 1 modified node, got %d", len(result.NodesModified))
	}
	if result.NodesModified[0].QualifiedName != "main.Alpha" {
		t.Errorf("expected modified node main.Alpha, got %s", result.NodesModified[0].QualifiedName)
	}
}
