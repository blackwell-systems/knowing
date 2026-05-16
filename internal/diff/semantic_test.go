package diff

import (
	"context"
	"fmt"
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

func TestSemanticDiff_OnlyNodeAdditions(t *testing.T) {
	// When SnapshotDiff returns nodes in NodesAdded (via store), SemanticDiff
	// should enrich them. With the current SQLite store, NodesAdded comes from
	// the store's DiffResult. We create edge events that indirectly show new
	// nodes, but the store only tracks edge-level events. This test verifies
	// that when no edge events exist, the diff is empty (no node-only tracking).
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// Create nodes but no edge events.
	_ = putNode(t, s, file, "main.NewFunc1", "function")
	_ = putNode(t, s, file, "main.NewFunc2", "function")

	oldSnap := types.NewHash([]byte("add-old"))
	newSnap := types.NewHash([]byte("add-new"))

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	// SQLite SnapshotDiff does not track node-level events, so NodesAdded is empty.
	if len(result.NodesAdded) != 0 {
		t.Errorf("expected 0 nodes added (store does not track node events), got %d", len(result.NodesAdded))
	}
	if result.Summary.NodesAdded != 0 {
		t.Errorf("summary NodesAdded: want 0, got %d", result.Summary.NodesAdded)
	}
	if result.Summary.NodesRemoved != 0 {
		t.Errorf("summary NodesRemoved: want 0, got %d", result.Summary.NodesRemoved)
	}
}

func TestSemanticDiff_OnlyNodeRemovals(t *testing.T) {
	// Similar to additions: the SQLite store does not produce node-level diff
	// entries. When nodes are removed but no edge events reference the snapshot,
	// NodesRemoved stays empty.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	_ = putNode(t, s, file, "main.OldFunc", "function")

	oldSnap := types.NewHash([]byte("rm-old"))
	newSnap := types.NewHash([]byte("rm-new"))

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	if len(result.NodesRemoved) != 0 {
		t.Errorf("expected 0 nodes removed (store does not track node events), got %d", len(result.NodesRemoved))
	}
	if result.Summary.NodesRemoved != 0 {
		t.Errorf("summary NodesRemoved: want 0, got %d", result.Summary.NodesRemoved)
	}
}

func TestSemanticDiff_MultipleModifiedNodes(t *testing.T) {
	// Edge changes from different source nodes should produce multiple modified nodes.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	// Three source nodes, each with a new outgoing edge to a shared target.
	src1 := putNode(t, s, file, "main.Src1", "function")
	src2 := putNode(t, s, file, "main.Src2", "function")
	src3 := putNode(t, s, file, "main.Src3", "function")
	target := putNode(t, s, file, "main.Target", "function")

	e1 := putEdge(t, s, src1, target, "calls")
	e2 := putEdge(t, s, src2, target, "calls")
	e3 := putEdge(t, s, src3, target, "calls")

	oldSnap := types.NewHash([]byte("multi-old"))
	newSnap := types.NewHash([]byte("multi-new"))

	recordEdgeEvent(t, s, e1, "added", newSnap)
	recordEdgeEvent(t, s, e2, "added", newSnap)
	recordEdgeEvent(t, s, e3, "added", newSnap)

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	if len(result.EdgesAdded) != 3 {
		t.Fatalf("expected 3 added edges, got %d", len(result.EdgesAdded))
	}
	// All three source nodes should be marked as modified.
	if len(result.NodesModified) != 3 {
		t.Fatalf("expected 3 modified nodes, got %d", len(result.NodesModified))
	}
	if result.Summary.NodesModified != 3 {
		t.Errorf("summary NodesModified: want 3, got %d", result.Summary.NodesModified)
	}

	// Collect modified node names and verify all three sources are present.
	modNames := make(map[string]bool)
	for _, mod := range result.NodesModified {
		modNames[mod.QualifiedName] = true
	}
	for _, want := range []string{"main.Src1", "main.Src2", "main.Src3"} {
		if !modNames[want] {
			t.Errorf("expected %s in modified nodes, got %v", want, modNames)
		}
	}
}

func TestSemanticDiff_LargeDiff(t *testing.T) {
	// Verify that 12 edge changes are handled correctly.
	s := newTestStore(t)
	ctx := context.Background()
	repo := putRepo(t, s, "https://example.com/repo")
	file := putFile(t, s, repo, "main.go")

	const edgeCount = 12
	sources := make([]types.Node, edgeCount)
	targets := make([]types.Node, edgeCount)
	edges := make([]types.Edge, edgeCount)

	for i := 0; i < edgeCount; i++ {
		sources[i] = putNode(t, s, file, fmt.Sprintf("main.Src%d", i), "function")
		targets[i] = putNode(t, s, file, fmt.Sprintf("main.Tgt%d", i), "function")
		edges[i] = putEdge(t, s, sources[i], targets[i], "calls")
	}

	oldSnap := types.NewHash([]byte("large-old"))
	newSnap := types.NewHash([]byte("large-new"))

	// Half added, half removed.
	for i := 0; i < edgeCount/2; i++ {
		recordEdgeEvent(t, s, edges[i], "added", newSnap)
	}
	for i := edgeCount / 2; i < edgeCount; i++ {
		recordEdgeEvent(t, s, edges[i], "removed", newSnap)
	}

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	if result.Summary.EdgesAdded != edgeCount/2 {
		t.Errorf("summary EdgesAdded: want %d, got %d", edgeCount/2, result.Summary.EdgesAdded)
	}
	if result.Summary.EdgesRemoved != edgeCount/2 {
		t.Errorf("summary EdgesRemoved: want %d, got %d", edgeCount/2, result.Summary.EdgesRemoved)
	}
	// All 12 source nodes should be modified (none were added/removed as nodes).
	if result.Summary.NodesModified != edgeCount {
		t.Errorf("summary NodesModified: want %d, got %d", edgeCount, result.Summary.NodesModified)
	}
}

func TestNodeToChange_ZeroValues(t *testing.T) {
	// nodeToChange with a zero-value Node should produce a valid NodeChange
	// with empty strings and zero ints.
	n := types.Node{}
	change := nodeToChange(n)

	if change.QualifiedName != "" {
		t.Errorf("expected empty QualifiedName, got %q", change.QualifiedName)
	}
	if change.Kind != "" {
		t.Errorf("expected empty Kind, got %q", change.Kind)
	}
	if change.Line != 0 {
		t.Errorf("expected Line 0, got %d", change.Line)
	}
	if change.Signature != "" {
		t.Errorf("expected empty Signature, got %q", change.Signature)
	}
	// NodeHash should be the hex encoding of a zero hash (64 hex zeros).
	if len(change.NodeHash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars: %q", len(change.NodeHash), change.NodeHash)
	}
}

func TestNodeToChange_PreservesFields(t *testing.T) {
	// Verify all fields are carried through from Node to NodeChange.
	n := types.Node{
		NodeHash:      types.NewHash([]byte("test-node")),
		QualifiedName: "pkg.MyFunc",
		Kind:          "method",
		Line:          99,
		Signature:     "func (s *S) MyFunc(ctx context.Context) error",
	}
	change := nodeToChange(n)

	if change.QualifiedName != "pkg.MyFunc" {
		t.Errorf("QualifiedName: got %q, want %q", change.QualifiedName, "pkg.MyFunc")
	}
	if change.Kind != "method" {
		t.Errorf("Kind: got %q, want %q", change.Kind, "method")
	}
	if change.Line != 99 {
		t.Errorf("Line: got %d, want 99", change.Line)
	}
	if change.Signature != "func (s *S) MyFunc(ctx context.Context) error" {
		t.Errorf("Signature: got %q", change.Signature)
	}
	// File is not set on the Node, so it should be empty.
	if change.File != "" {
		t.Errorf("File: got %q, want empty", change.File)
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
