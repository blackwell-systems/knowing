package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func tempDB(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// makeRepo creates and persists a repo, returning the Repo struct.
func makeRepo(t *testing.T, s *SQLiteStore, url string) types.Repo {
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

// makeFile creates and persists a file, returning the File struct.
func makeFile(t *testing.T, s *SQLiteStore, repo types.Repo, path string) types.File {
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

// makeNode creates and persists a node, returning the Node struct.
func makeNode(t *testing.T, s *SQLiteStore, file types.File, name, kind string) types.Node {
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

// makeEdge creates and persists an edge, returning the Edge struct.
func makeEdge(t *testing.T, s *SQLiteStore, source, target types.Node, edgeType string) types.Edge {
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

func TestNewSQLiteStore_CreatesDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	// DB file should exist.
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("database file was not created")
	}

	// Schema version should be 1.
	var v int
	if err := store.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&v); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if v != 10 {
		t.Fatalf("expected schema version 10, got %d", v)
	}
}

func TestPutNode_GetNode_RoundTrip(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	n := types.Node{
		NodeHash:      types.ComputeNodeHash("https://example.com/repo", "main", file.ContentHash, "Foo", "function"),
		FileHash:      file.FileHash,
		QualifiedName: "main.Foo",
		Kind:          "function",
		Line:          10,
		Signature:     "func Foo()",
	}
	if err := s.PutNode(ctx, n); err != nil {
		t.Fatalf("PutNode: %v", err)
	}

	got, err := s.GetNode(ctx, n.NodeHash)
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got == nil {
		t.Fatal("GetNode returned nil")
	}
	if got.QualifiedName != n.QualifiedName {
		t.Errorf("QualifiedName = %q, want %q", got.QualifiedName, n.QualifiedName)
	}
	if got.Kind != n.Kind {
		t.Errorf("Kind = %q, want %q", got.Kind, n.Kind)
	}
	if got.NodeHash != n.NodeHash {
		t.Errorf("NodeHash mismatch")
	}
}

func TestPutEdge_GetEdge_RoundTrip(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src := makeNode(t, s, file, "main.Caller", "function")
	tgt := makeNode(t, s, file, "main.Callee", "function")

	e := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(src.NodeHash, tgt.NodeHash, "calls", "{}"),
		SourceHash: src.NodeHash,
		TargetHash: tgt.NodeHash,
		EdgeType:   "calls",
		Confidence: 0.95,
		Provenance: "ast_resolved",
	}
	if err := s.PutEdge(ctx, e); err != nil {
		t.Fatalf("PutEdge: %v", err)
	}

	got, err := s.GetEdge(ctx, e.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge: %v", err)
	}
	if got == nil {
		t.Fatal("GetEdge returned nil")
	}
	if got.EdgeType != "calls" {
		t.Errorf("EdgeType = %q, want calls", got.EdgeType)
	}
	if got.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", got.Confidence)
	}
}

func TestEdgesFrom_FiltersByType(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src := makeNode(t, s, file, "main.Src", "function")
	tgt1 := makeNode(t, s, file, "main.Tgt1", "function")
	tgt2 := makeNode(t, s, file, "main.Tgt2", "type")

	makeEdge(t, s, src, tgt1, "calls")
	makeEdge(t, s, src, tgt2, "imports")

	// Filter by "calls" should return only 1.
	edges, err := s.EdgesFrom(ctx, src.NodeHash, "calls")
	if err != nil {
		t.Fatalf("EdgesFrom: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "calls" {
		t.Errorf("expected calls, got %s", edges[0].EdgeType)
	}

	// Empty type returns all.
	all, err := s.EdgesFrom(ctx, src.NodeHash, "")
	if err != nil {
		t.Fatalf("EdgesFrom all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(all))
	}
}

func TestEdgesTo_FiltersByType(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src1 := makeNode(t, s, file, "main.A", "function")
	src2 := makeNode(t, s, file, "main.B", "function")
	tgt := makeNode(t, s, file, "main.Target", "function")

	makeEdge(t, s, src1, tgt, "calls")
	makeEdge(t, s, src2, tgt, "references")

	edges, err := s.EdgesTo(ctx, tgt.NodeHash, "calls")
	if err != nil {
		t.Fatalf("EdgesTo: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
}

func TestTransitiveCallers_DepthLimit(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	// Build chain: A -> B -> C -> D
	a := makeNode(t, s, file, "main.A", "function")
	b := makeNode(t, s, file, "main.B", "function")
	c := makeNode(t, s, file, "main.C", "function")
	d := makeNode(t, s, file, "main.D", "function")

	makeEdge(t, s, a, b, "calls")
	makeEdge(t, s, b, c, "calls")
	makeEdge(t, s, c, d, "calls")

	snapshot := types.Hash{} // unused in current impl

	// Callers of D with depth 2 should find C (depth 1) and B (depth 2), not A.
	callers, err := s.TransitiveCallers(ctx, d.NodeHash, 2, snapshot)
	if err != nil {
		t.Fatalf("TransitiveCallers: %v", err)
	}
	if len(callers) != 2 {
		t.Fatalf("expected 2 callers, got %d", len(callers))
	}

	// Callers of D with depth 10 should find A, B, C.
	all, err := s.TransitiveCallers(ctx, d.NodeHash, 10, snapshot)
	if err != nil {
		t.Fatalf("TransitiveCallers depth 10: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 callers, got %d", len(all))
	}
}

func TestBlastRadius_GroupsByRepo(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	repo1 := makeRepo(t, s, "https://example.com/repo1")
	repo2 := makeRepo(t, s, "https://example.com/repo2")
	file1 := makeFile(t, s, repo1, "main.go")
	file2 := makeFile(t, s, repo2, "lib.go")

	target := makeNode(t, s, file1, "main.Target", "function")
	caller1 := makeNode(t, s, file1, "main.Caller1", "function")
	caller2 := makeNode(t, s, file2, "lib.Caller2", "function")

	makeEdge(t, s, caller1, target, "calls")
	makeEdge(t, s, caller2, target, "calls")

	snapshot := types.Hash{}
	br, err := s.BlastRadius(ctx, target.NodeHash, snapshot)
	if err != nil {
		t.Fatalf("BlastRadius: %v", err)
	}

	if br.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", br.TotalCount)
	}
	if len(br.ByRepo) != 2 {
		t.Errorf("expected 2 repos, got %d", len(br.ByRepo))
	}
	if len(br.ByRepo["https://example.com/repo1"]) != 1 {
		t.Errorf("expected 1 caller in repo1")
	}
	if len(br.ByRepo["https://example.com/repo2"]) != 1 {
		t.Errorf("expected 1 caller in repo2")
	}
}

func TestCreateSnapshot_ChainLinks(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")

	snap1 := types.Snapshot{
		SnapshotHash: types.NewHash([]byte("snap1")),
		ParentHash:   types.Hash{},
		RepoHash:     repo.RepoHash,
		CommitHash:   "commit1",
		Timestamp:    1000,
		NodeCount:    10,
		EdgeCount:    5,
	}
	if err := s.CreateSnapshot(ctx, snap1); err != nil {
		t.Fatalf("CreateSnapshot 1: %v", err)
	}

	snap2 := types.Snapshot{
		SnapshotHash: types.NewHash([]byte("snap2")),
		ParentHash:   snap1.SnapshotHash,
		RepoHash:     repo.RepoHash,
		CommitHash:   "commit2",
		Timestamp:    2000,
		NodeCount:    12,
		EdgeCount:    7,
	}
	if err := s.CreateSnapshot(ctx, snap2); err != nil {
		t.Fatalf("CreateSnapshot 2: %v", err)
	}

	// Verify chain link.
	got, err := s.GetSnapshot(ctx, snap2.SnapshotHash)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("GetSnapshot returned nil")
	}
	if got.ParentHash != snap1.SnapshotHash {
		t.Error("snap2.ParentHash should link to snap1")
	}

	// LatestSnapshot should return snap2 (higher timestamp).
	latest, err := s.LatestSnapshot(ctx, repo.RepoHash)
	if err != nil {
		t.Fatalf("LatestSnapshot: %v", err)
	}
	if latest == nil {
		t.Fatal("LatestSnapshot returned nil")
	}
	if latest.SnapshotHash != snap2.SnapshotHash {
		t.Error("LatestSnapshot should return snap2")
	}
}

func TestSnapshotDiff_DetectsChanges(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	n1 := makeNode(t, s, file, "main.Foo", "function")
	n2 := makeNode(t, s, file, "main.Bar", "function")

	edge := makeEdge(t, s, n1, n2, "calls")

	oldSnap := types.NewHash([]byte("old-snap"))
	newSnap := types.NewHash([]byte("new-snap"))

	// Record an edge event for the new snapshot.
	ev := types.EdgeEvent{
		EdgeHash:     edge.EdgeHash,
		EventType:    "added",
		SnapshotHash: newSnap,
		SourceCommit: "abc",
		IndexerVer:   "v1",
		Timestamp:    time.Now().Unix(),
	}
	if err := s.RecordEdgeEvent(ctx, ev); err != nil {
		t.Fatalf("RecordEdgeEvent: %v", err)
	}

	diff, err := s.SnapshotDiff(ctx, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SnapshotDiff: %v", err)
	}
	if len(diff.EdgesAdded) != 1 {
		t.Errorf("expected 1 added edge, got %d", len(diff.EdgesAdded))
	}
	if len(diff.EdgesRemoved) != 0 {
		t.Errorf("expected 0 removed edges, got %d", len(diff.EdgesRemoved))
	}
}

func TestMigrate_AppliesInOrder(t *testing.T) {
	s := tempDB(t)

	// Verify all expected tables exist by querying them.
	tables := []string{"repos", "files", "nodes", "edges", "edge_events", "snapshots", "schema_version"}
	for _, table := range tables {
		var count int
		err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %s does not exist: %v", table, err)
		}
	}

	// Running Migrate again should be idempotent.
	if err := Migrate(s.db); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestStaleEdges_FindsMismatches(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")

	// Create a file and nodes/edges based on it.
	file := makeFile(t, s, repo, "main.go")
	n1 := makeNode(t, s, file, "main.Foo", "function")
	n2 := makeNode(t, s, file, "main.Bar", "function")
	makeEdge(t, s, n1, n2, "calls")

	// Initially no stale edges (only one version of the file).
	snapshot := types.Hash{}
	stale, err := s.StaleEdges(ctx, snapshot)
	if err != nil {
		t.Fatalf("StaleEdges: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected 0 stale edges initially, got %d", len(stale))
	}

	// Now insert a new version of the same file (same repo+path, different content hash).
	newFile := types.File{
		FileHash:    types.NewHash([]byte("new-version")),
		RepoHash:    repo.RepoHash,
		Path:        "main.go",
		ContentHash: types.NewHash([]byte("updated content")),
	}
	if err := s.PutFile(ctx, newFile); err != nil {
		t.Fatalf("PutFile new version: %v", err)
	}

	stale, err = s.StaleEdges(ctx, snapshot)
	if err != nil {
		t.Fatalf("StaleEdges after update: %v", err)
	}
	if len(stale) != 1 {
		t.Fatalf("expected 1 stale edge, got %d", len(stale))
	}
}

func TestNodesByName_PrefixMatch(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	makeNode(t, s, file, "main.FooBar", "function")
	makeNode(t, s, file, "main.FooBaz", "function")
	makeNode(t, s, file, "main.Qux", "function")

	// Prefix "main.Foo" should match FooBar and FooBaz.
	nodes, err := s.NodesByName(ctx, "main.Foo")
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	// Prefix "main." should match all 3.
	all, err := s.NodesByName(ctx, "main.")
	if err != nil {
		t.Fatalf("NodesByName all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(all))
	}
}

func TestFilesByRepo_And_FileByPath(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")

	makeFile(t, s, repo, "main.go")
	makeFile(t, s, repo, "lib.go")

	files, err := s.FilesByRepo(ctx, repo.RepoHash)
	if err != nil {
		t.Fatalf("FilesByRepo: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	f, err := s.FileByPath(ctx, repo.RepoHash, "main.go")
	if err != nil {
		t.Fatalf("FileByPath: %v", err)
	}
	if f == nil {
		t.Fatal("FileByPath returned nil")
	}
	if f.Path != "main.go" {
		t.Errorf("Path = %q, want main.go", f.Path)
	}

	// Non-existent path.
	missing, err := s.FileByPath(ctx, repo.RepoHash, "missing.go")
	if err != nil {
		t.Fatalf("FileByPath missing: %v", err)
	}
	if missing != nil {
		t.Error("expected nil for missing path")
	}
}

func TestBatchPutNodes(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	// Build 150 nodes.
	nodes := make([]types.Node, 150)
	for i := range nodes {
		name := fmt.Sprintf("main.Func%d", i)
		nodes[i] = types.Node{
			NodeHash:      types.ComputeNodeHash("test", "pkg", file.ContentHash, name, "function"),
			FileHash:      file.FileHash,
			QualifiedName: name,
			Kind:          "function",
			Line:          i + 1,
			Signature:     "func " + name + "()",
		}
	}

	if err := s.BatchPutNodes(ctx, nodes); err != nil {
		t.Fatalf("BatchPutNodes: %v", err)
	}

	// Verify all nodes are readable.
	for i, n := range nodes {
		got, err := s.GetNode(ctx, n.NodeHash)
		if err != nil {
			t.Fatalf("GetNode[%d]: %v", i, err)
		}
		if got == nil {
			t.Fatalf("GetNode[%d] returned nil", i)
		}
		if got.QualifiedName != n.QualifiedName {
			t.Errorf("GetNode[%d] name = %q, want %q", i, got.QualifiedName, n.QualifiedName)
		}
	}

	// Verify count via prefix query.
	all, err := s.NodesByName(ctx, "main.Func")
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}
	if len(all) != 150 {
		t.Fatalf("expected 150 nodes, got %d", len(all))
	}
}

func TestBatchPutEdges(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	// Create source and 100 target nodes, then batch insert edges.
	src := makeNode(t, s, file, "main.Caller", "function")
	edges := make([]types.Edge, 100)
	for i := range edges {
		tgtName := fmt.Sprintf("main.Target%d", i)
		tgt := makeNode(t, s, file, tgtName, "function")
		edges[i] = types.Edge{
			EdgeHash:   types.ComputeEdgeHash(src.NodeHash, tgt.NodeHash, "calls", "{}"),
			SourceHash: src.NodeHash,
			TargetHash: tgt.NodeHash,
			EdgeType:   "calls",
			Confidence: 0.9,
			Provenance: "ast_resolved",
		}
	}

	if err := s.BatchPutEdges(ctx, edges); err != nil {
		t.Fatalf("BatchPutEdges: %v", err)
	}

	// Verify all edges are readable.
	got, err := s.EdgesFrom(ctx, src.NodeHash, "calls")
	if err != nil {
		t.Fatalf("EdgesFrom: %v", err)
	}
	if len(got) != 100 {
		t.Fatalf("expected 100 edges, got %d", len(got))
	}
}

func TestBatchPutFiles(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")

	files := make([]types.File, 120)
	for i := range files {
		path := fmt.Sprintf("pkg/file%d.go", i)
		content := []byte(fmt.Sprintf("content-%d", i))
		files[i] = types.File{
			FileHash:    types.NewHash(append(repo.RepoHash[:], []byte(path)...)),
			RepoHash:    repo.RepoHash,
			Path:        path,
			ContentHash: types.NewHash(content),
		}
	}

	if err := s.BatchPutFiles(ctx, files); err != nil {
		t.Fatalf("BatchPutFiles: %v", err)
	}

	// Verify all files are readable.
	got, err := s.FilesByRepo(ctx, repo.RepoHash)
	if err != nil {
		t.Fatalf("FilesByRepo: %v", err)
	}
	if len(got) != 120 {
		t.Fatalf("expected 120 files, got %d", len(got))
	}

	// Spot check a specific file.
	f, err := s.FileByPath(ctx, repo.RepoHash, "pkg/file42.go")
	if err != nil {
		t.Fatalf("FileByPath: %v", err)
	}
	if f == nil {
		t.Fatal("FileByPath returned nil for pkg/file42.go")
	}
	if f.Path != "pkg/file42.go" {
		t.Errorf("Path = %q, want pkg/file42.go", f.Path)
	}
}

func TestBatchPutNodes_EmptySlice(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Empty batch should succeed without error.
	if err := s.BatchPutNodes(ctx, nil); err != nil {
		t.Fatalf("BatchPutNodes(nil): %v", err)
	}
	if err := s.BatchPutEdges(ctx, nil); err != nil {
		t.Fatalf("BatchPutEdges(nil): %v", err)
	}
	if err := s.BatchPutFiles(ctx, nil); err != nil {
		t.Fatalf("BatchPutFiles(nil): %v", err)
	}
}

func TestDanglingEdges(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	src := makeNode(t, s, file, "main.Caller", "function")
	tgt := makeNode(t, s, file, "main.Callee", "function")

	// Edge with a valid target (should NOT appear in dangling).
	makeEdge(t, s, src, tgt, "calls")

	// Edge with a nonexistent target (should appear in dangling).
	fakeTargetHash := types.NewHash([]byte("nonexistent-target"))
	danglingEdge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(src.NodeHash, fakeTargetHash, "calls", "{}"),
		SourceHash: src.NodeHash,
		TargetHash: fakeTargetHash,
		EdgeType:   "calls",
		Confidence: 0.8,
		Provenance: "heuristic",
	}
	if err := s.PutEdge(ctx, danglingEdge); err != nil {
		t.Fatalf("PutEdge dangling: %v", err)
	}

	edges, err := s.DanglingEdges(ctx)
	if err != nil {
		t.Fatalf("DanglingEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 dangling edge, got %d", len(edges))
	}
	if edges[0].EdgeHash != danglingEdge.EdgeHash {
		t.Errorf("wrong dangling edge returned")
	}
}

func TestAllRepos(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	makeRepo(t, s, "https://example.com/beta")
	makeRepo(t, s, "https://example.com/alpha")

	repos, err := s.AllRepos(ctx)
	if err != nil {
		t.Fatalf("AllRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	// Should be sorted alphabetically by repo_url.
	if repos[0].RepoURL != "https://example.com/alpha" {
		t.Errorf("first repo = %q, want https://example.com/alpha", repos[0].RepoURL)
	}
	if repos[1].RepoURL != "https://example.com/beta" {
		t.Errorf("second repo = %q, want https://example.com/beta", repos[1].RepoURL)
	}
}

func TestNodesByQualifiedName(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	makeNode(t, s, file, "main.FooBar", "function")
	makeNode(t, s, file, "main.FooBaz", "function")

	// Exact match, not prefix.
	nodes, err := s.NodesByQualifiedName(ctx, "main.FooBar")
	if err != nil {
		t.Fatalf("NodesByQualifiedName: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].QualifiedName != "main.FooBar" {
		t.Errorf("QualifiedName = %q, want main.FooBar", nodes[0].QualifiedName)
	}

	// A prefix that doesn't exactly match should return nothing.
	none, err := s.NodesByQualifiedName(ctx, "main.Foo")
	if err != nil {
		t.Fatalf("NodesByQualifiedName prefix: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected 0 nodes for prefix match, got %d", len(none))
	}
}

func TestDeleteEdge(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src := makeNode(t, s, file, "main.A", "function")
	tgt := makeNode(t, s, file, "main.B", "function")

	edge := makeEdge(t, s, src, tgt, "calls")

	// Verify edge exists.
	got, err := s.GetEdge(ctx, edge.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge before delete: %v", err)
	}
	if got == nil {
		t.Fatal("edge should exist before delete")
	}

	// Delete it.
	if err := s.DeleteEdge(ctx, edge.EdgeHash); err != nil {
		t.Fatalf("DeleteEdge: %v", err)
	}

	// Verify it's gone.
	got, err = s.GetEdge(ctx, edge.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge after delete: %v", err)
	}
	if got != nil {
		t.Error("edge should be nil after delete")
	}
}

func TestDeleteNodesByFile(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file1 := makeFile(t, s, repo, "main.go")
	file2 := makeFile(t, s, repo, "other.go")

	// Two nodes in file1, one node in file2.
	n1 := makeNode(t, s, file1, "main.Foo", "function")
	n2 := makeNode(t, s, file1, "main.Bar", "function")
	n3 := makeNode(t, s, file2, "other.Baz", "function")

	count, err := s.DeleteNodesByFile(ctx, file1.FileHash)
	if err != nil {
		t.Fatalf("DeleteNodesByFile: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 deleted, got %d", count)
	}

	// Verify file1 nodes are gone.
	got1, err := s.GetNode(ctx, n1.NodeHash)
	if err != nil {
		t.Fatalf("GetNode n1: %v", err)
	}
	if got1 != nil {
		t.Error("n1 should be deleted")
	}
	got2, err := s.GetNode(ctx, n2.NodeHash)
	if err != nil {
		t.Fatalf("GetNode n2: %v", err)
	}
	if got2 != nil {
		t.Error("n2 should be deleted")
	}

	// Verify file2 node remains.
	got3, err := s.GetNode(ctx, n3.NodeHash)
	if err != nil {
		t.Fatalf("GetNode n3: %v", err)
	}
	if got3 == nil {
		t.Error("n3 should still exist")
	}
}

func TestDeleteEdgesBySourceFile(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file1 := makeFile(t, s, repo, "main.go")
	file2 := makeFile(t, s, repo, "other.go")

	// Nodes in file1 and file2.
	src1 := makeNode(t, s, file1, "main.Caller", "function")
	src2 := makeNode(t, s, file2, "other.Caller", "function")
	tgt := makeNode(t, s, file1, "main.Target", "function")

	// Edge from file1's node, edge from file2's node.
	e1 := makeEdge(t, s, src1, tgt, "calls")
	e2 := makeEdge(t, s, src2, tgt, "calls")

	deleted, err := s.DeleteEdgesBySourceFile(ctx, file1.FileHash)
	if err != nil {
		t.Fatalf("DeleteEdgesBySourceFile: %v", err)
	}
	if len(deleted) != 1 {
		t.Fatalf("expected 1 deleted edge, got %d", len(deleted))
	}
	if deleted[0].EdgeHash != e1.EdgeHash {
		t.Errorf("wrong edge returned; got %v, want %v", deleted[0].EdgeHash, e1.EdgeHash)
	}

	// Verify e1 is gone.
	got1, err := s.GetEdge(ctx, e1.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge e1: %v", err)
	}
	if got1 != nil {
		t.Error("e1 should be deleted")
	}

	// Verify e2 remains.
	got2, err := s.GetEdge(ctx, e2.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge e2: %v", err)
	}
	if got2 == nil {
		t.Error("e2 should still exist")
	}
}

func TestEdgesBySourceFile(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file1 := makeFile(t, s, repo, "main.go")
	file2 := makeFile(t, s, repo, "other.go")

	src1 := makeNode(t, s, file1, "main.Caller", "function")
	src2 := makeNode(t, s, file2, "other.Caller", "function")
	tgt := makeNode(t, s, file1, "main.Target", "function")

	e1 := makeEdge(t, s, src1, tgt, "calls")
	makeEdge(t, s, src2, tgt, "calls")

	// Query edges by source file.
	edges, err := s.EdgesBySourceFile(ctx, file1.FileHash)
	if err != nil {
		t.Fatalf("EdgesBySourceFile: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].EdgeHash != e1.EdgeHash {
		t.Errorf("wrong edge returned")
	}

	// Verify read-only: edges should still be in the store.
	got, err := s.GetEdge(ctx, e1.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge after EdgesBySourceFile: %v", err)
	}
	if got == nil {
		t.Error("edge should still exist after read-only query")
	}
}

func TestGetNode_NotFound(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	got, err := s.GetNode(ctx, types.NewHash([]byte("nonexistent")))
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent node")
	}
}

func TestIntegrityCheck_Healthy(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	if err := s.IntegrityCheck(ctx); err != nil {
		t.Fatalf("IntegrityCheck on healthy DB: %v", err)
	}
}
