package snapshot

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/testutil"
	"github.com/blackwell-systems/knowing/internal/types"
)

// mockGraphStore embeds *testutil.MockGraphStore for no-op defaults and
// overrides methods the snapshot tests need with custom tracking behavior.
type mockGraphStore struct {
	*testutil.MockGraphStore

	// files stores files by repo hash (for test setup reference only).
	files map[types.Hash][]types.File

	// nodesByNameResult controls what NodesByName returns.
	nodesByNameResult []types.Node

	// edgesFromResult controls what EdgesFrom returns per source hash.
	edgesFromResult map[types.Hash][]types.Edge

	// latestSnapshotResult is the latest snapshot for any repo.
	latestSnapshotResult *types.Snapshot

	// createdSnapshots tracks snapshots that were created.
	createdSnapshots []types.Snapshot

	// snapshotDiffResult is returned by SnapshotDiff.
	snapshotDiffResult *types.DiffResult
}

func newMockGraphStore() *mockGraphStore {
	return &mockGraphStore{
		MockGraphStore:  testutil.NewMockGraphStore(),
		files:           make(map[types.Hash][]types.File),
		edgesFromResult: make(map[types.Hash][]types.Edge),
	}
}

func (m *mockGraphStore) PutNode(_ context.Context, n types.Node) error {
	m.Nodes[n.NodeHash] = &n
	return nil
}

func (m *mockGraphStore) PutEdge(_ context.Context, e types.Edge) error {
	m.Edges[e.EdgeHash] = &e
	return nil
}

func (m *mockGraphStore) PutRepo(_ context.Context, r types.Repo) error {
	m.Repos[r.RepoHash] = &r
	return nil
}

func (m *mockGraphStore) CreateSnapshot(_ context.Context, s types.Snapshot) error {
	m.createdSnapshots = append(m.createdSnapshots, s)
	m.Snapshots[s.SnapshotHash] = &s
	m.latestSnapshotResult = &s
	return nil
}

func (m *mockGraphStore) NodesByName(_ context.Context, _ string) ([]types.Node, error) {
	return m.nodesByNameResult, nil
}

func (m *mockGraphStore) EdgesFrom(_ context.Context, sourceHash types.Hash, _ string) ([]types.Edge, error) {
	return m.edgesFromResult[sourceHash], nil
}

func (m *mockGraphStore) SnapshotDiff(_ context.Context, _, _ types.Hash) (*types.DiffResult, error) {
	return m.snapshotDiffResult, nil
}

func (m *mockGraphStore) LatestSnapshot(_ context.Context, _ types.Hash) (*types.Snapshot, error) {
	return m.latestSnapshotResult, nil
}

// --- Merkle tests ---

func TestMerkleRoot_Deterministic(t *testing.T) {
	h1 := types.NewHash([]byte("edge-1"))
	h2 := types.NewHash([]byte("edge-2"))
	h3 := types.NewHash([]byte("edge-3"))

	// Build tree with hashes in different orders; should produce same root.
	tree1 := BuildMerkleTree([]types.Hash{h1, h2, h3})
	tree2 := BuildMerkleTree([]types.Hash{h3, h1, h2})
	tree3 := BuildMerkleTree([]types.Hash{h2, h3, h1})

	if tree1.Root != tree2.Root {
		t.Errorf("different input orders produced different roots: %s vs %s", tree1.Root, tree2.Root)
	}
	if tree1.Root != tree3.Root {
		t.Errorf("different input orders produced different roots: %s vs %s", tree1.Root, tree3.Root)
	}

	// Leaves should be in sorted order.
	for i := 1; i < len(tree1.Leaves); i++ {
		if tree1.Leaves[i-1].String() > tree1.Leaves[i].String() {
			t.Error("leaves are not sorted")
		}
	}
}

func TestMerkleRoot_EmptyEdges(t *testing.T) {
	tree := BuildMerkleTree(nil)
	if tree.Root != types.EmptyHash {
		t.Errorf("empty tree should have zero hash, got %s", tree.Root)
	}
	if tree.Leaves != nil {
		t.Error("empty tree should have nil leaves")
	}
}

func TestMerkleDiff_IdentifiesChanges(t *testing.T) {
	h1 := types.NewHash([]byte("edge-1"))
	h2 := types.NewHash([]byte("edge-2"))
	h3 := types.NewHash([]byte("edge-3"))
	h4 := types.NewHash([]byte("edge-4"))

	oldTree := BuildMerkleTree([]types.Hash{h1, h2, h3})
	newTree := BuildMerkleTree([]types.Hash{h2, h3, h4})

	added, removed := DiffMerkle(oldTree, newTree)

	if len(added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(added))
	}
	if added[0] != h4 {
		t.Errorf("expected added hash %s, got %s", h4, added[0])
	}

	if len(removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(removed))
	}
	if removed[0] != h1 {
		t.Errorf("expected removed hash %s, got %s", h1, removed[0])
	}
}

// --- Snapshot manager tests ---

func setupMockStore() (*mockGraphStore, types.Hash) {
	store := newMockGraphStore()

	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.Repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// Create some nodes and edges.
	node1 := types.Node{
		NodeHash:      types.NewHash([]byte("node-1")),
		QualifiedName: "https://github.com/example/repo://pkg.Func1",
		Kind:          "function",
	}
	node2 := types.Node{
		NodeHash:      types.NewHash([]byte("node-2")),
		QualifiedName: "https://github.com/example/repo://pkg.Func2",
		Kind:          "function",
	}

	store.Nodes[node1.NodeHash] = &node1
	store.Nodes[node2.NodeHash] = &node2
	store.nodesByNameResult = []types.Node{node1, node2}

	edge1 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-1")),
		SourceHash: node1.NodeHash,
		TargetHash: node2.NodeHash,
		EdgeType:   "calls",
	}
	edge2 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-2")),
		SourceHash: node2.NodeHash,
		TargetHash: node1.NodeHash,
		EdgeType:   "calls",
	}

	store.Edges[edge1.EdgeHash] = &edge1
	store.Edges[edge2.EdgeHash] = &edge2
	store.edgesFromResult[node1.NodeHash] = []types.Edge{edge1}
	store.edgesFromResult[node2.NodeHash] = []types.Edge{edge2}

	return store, repoHash
}

func TestComputeSnapshot_CreatesChain(t *testing.T) {
	store, repoHash := setupMockStore()
	sm := NewSnapshotManager(store)
	ctx := context.Background()

	// First snapshot: no parent.
	snap1, err := sm.ComputeSnapshot(ctx, repoHash, "commit-abc")
	if err != nil {
		t.Fatalf("ComputeSnapshot failed: %v", err)
	}
	if !snap1.ParentHash.IsZero() {
		t.Error("first snapshot should have zero parent hash")
	}
	if snap1.CommitHash != "commit-abc" {
		t.Errorf("commit hash mismatch: got %s", snap1.CommitHash)
	}
	if snap1.NodeCount != 2 {
		t.Errorf("node count: expected 2, got %d", snap1.NodeCount)
	}
	if snap1.EdgeCount != 2 {
		t.Errorf("edge count: expected 2, got %d", snap1.EdgeCount)
	}

	// Change edges so second snapshot has a different Merkle root.
	node1 := store.nodesByNameResult[0]
	edge3 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-3")),
		SourceHash: node1.NodeHash,
		TargetHash: node1.NodeHash,
		EdgeType:   "references",
	}
	store.edgesFromResult[node1.NodeHash] = append(store.edgesFromResult[node1.NodeHash], edge3)

	// Second snapshot: should chain to first.
	snap2, err := sm.ComputeSnapshot(ctx, repoHash, "commit-def")
	if err != nil {
		t.Fatalf("ComputeSnapshot failed: %v", err)
	}
	if snap2.ParentHash != snap1.SnapshotHash {
		t.Errorf("second snapshot should chain to first: parent=%s, expected=%s",
			snap2.ParentHash, snap1.SnapshotHash)
	}
}

func TestComputeSnapshot_Deterministic(t *testing.T) {
	ctx := context.Background()

	// Create two independent stores with the same data.
	store1, repoHash1 := setupMockStore()
	store2, repoHash2 := setupMockStore()

	sm1 := NewSnapshotManager(store1)
	sm2 := NewSnapshotManager(store2)

	snap1, err := sm1.ComputeSnapshot(ctx, repoHash1, "commit-1")
	if err != nil {
		t.Fatalf("ComputeSnapshot 1 failed: %v", err)
	}

	snap2, err := sm2.ComputeSnapshot(ctx, repoHash2, "commit-2")
	if err != nil {
		t.Fatalf("ComputeSnapshot 2 failed: %v", err)
	}

	// Same edges should produce the same Merkle root, regardless of commit hash.
	if snap1.SnapshotHash != snap2.SnapshotHash {
		t.Errorf("same edges should produce same snapshot hash: %s vs %s",
			snap1.SnapshotHash, snap2.SnapshotHash)
	}
}

func buildSnapshotChain(store *mockGraphStore, repoHash types.Hash, count int) {
	// Directly build a chain of snapshots in the store.
	var prev types.Hash
	for i := 0; i < count; i++ {
		h := types.NewHash([]byte(fmt.Sprintf("snapshot-%d", i)))
		snap := types.Snapshot{
			SnapshotHash: h,
			ParentHash:   prev,
			RepoHash:     repoHash,
			CommitHash:   fmt.Sprintf("commit-%d", i),
			NodeCount:    i + 1,
		}
		store.Snapshots[h] = &snap
		store.latestSnapshotResult = &snap
		prev = h
	}
}

func TestGarbageCollect_KeepsRecent(t *testing.T) {
	store := newMockGraphStore()
	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	sm := NewSnapshotManager(store)
	ctx := context.Background()

	// Build a chain of 5 snapshots directly in the store.
	buildSnapshotChain(store, repoHash, 5)

	// GC keeping 3.
	removed, err := sm.GarbageCollect(ctx, repoHash, 3)
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
}

func TestComputeSnapshot_WithRealData(t *testing.T) {
	store := newMockGraphStore()
	ctx := context.Background()

	// Set up repo.
	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.Repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// Create files (stored for reference; snapshot uses nodes/edges).
	file1 := types.File{
		FileHash:    types.NewHash([]byte("file-1")),
		RepoHash:    repoHash,
		Path:        "pkg/main.go",
		ContentHash: types.NewHash([]byte("content-1")),
	}
	file2 := types.File{
		FileHash:    types.NewHash([]byte("file-2")),
		RepoHash:    repoHash,
		Path:        "pkg/util.go",
		ContentHash: types.NewHash([]byte("content-2")),
	}
	store.files[repoHash] = []types.File{file1, file2}

	// Create nodes belonging to the repo.
	node1 := types.Node{
		NodeHash:      types.NewHash([]byte("node-1")),
		FileHash:      file1.FileHash,
		QualifiedName: "https://github.com/example/repo://pkg.Main",
		Kind:          "function",
	}
	node2 := types.Node{
		NodeHash:      types.NewHash([]byte("node-2")),
		FileHash:      file1.FileHash,
		QualifiedName: "https://github.com/example/repo://pkg.Init",
		Kind:          "function",
	}
	node3 := types.Node{
		NodeHash:      types.NewHash([]byte("node-3")),
		FileHash:      file2.FileHash,
		QualifiedName: "https://github.com/example/repo://pkg.Helper",
		Kind:          "function",
	}

	store.Nodes[node1.NodeHash] = &node1
	store.Nodes[node2.NodeHash] = &node2
	store.Nodes[node3.NodeHash] = &node3
	store.nodesByNameResult = []types.Node{node1, node2, node3}

	// Create edges between the nodes.
	edge1 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-1")),
		SourceHash: node1.NodeHash,
		TargetHash: node2.NodeHash,
		EdgeType:   "calls",
	}
	edge2 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-2")),
		SourceHash: node1.NodeHash,
		TargetHash: node3.NodeHash,
		EdgeType:   "calls",
	}
	edge3 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-3")),
		SourceHash: node2.NodeHash,
		TargetHash: node3.NodeHash,
		EdgeType:   "calls",
	}

	store.Edges[edge1.EdgeHash] = &edge1
	store.Edges[edge2.EdgeHash] = &edge2
	store.Edges[edge3.EdgeHash] = &edge3

	store.edgesFromResult[node1.NodeHash] = []types.Edge{edge1, edge2}
	store.edgesFromResult[node2.NodeHash] = []types.Edge{edge3}
	// node3 has no outgoing edges

	sm := NewSnapshotManager(store)

	// Compute snapshot.
	snap, err := sm.ComputeSnapshot(ctx, repoHash, "abc123")
	if err != nil {
		t.Fatalf("ComputeSnapshot failed: %v", err)
	}

	// Verify counts.
	if snap.NodeCount != 3 {
		t.Errorf("expected 3 nodes, got %d", snap.NodeCount)
	}
	if snap.EdgeCount != 3 {
		t.Errorf("expected 3 edges, got %d", snap.EdgeCount)
	}
	if snap.CommitHash != "abc123" {
		t.Errorf("expected commit abc123, got %s", snap.CommitHash)
	}
	if snap.SnapshotHash.IsZero() {
		t.Error("snapshot hash should not be zero")
	}
	if !snap.ParentHash.IsZero() {
		t.Error("first snapshot should have zero parent hash")
	}

	// Verify determinism: compute again with fresh store, same data.
	store2 := newMockGraphStore()
	store2.Repos[repoHash] = store.Repos[repoHash]
	store2.Nodes = store.Nodes
	store2.Edges = store.Edges
	store2.nodesByNameResult = store.nodesByNameResult
	store2.edgesFromResult = store.edgesFromResult

	sm2 := NewSnapshotManager(store2)
	snap2, err := sm2.ComputeSnapshot(ctx, repoHash, "different-commit")
	if err != nil {
		t.Fatalf("second ComputeSnapshot failed: %v", err)
	}

	if snap.SnapshotHash != snap2.SnapshotHash {
		t.Errorf("same graph data should produce same snapshot hash: %s vs %s",
			snap.SnapshotHash, snap2.SnapshotHash)
	}
}

// --- extractPackagePath tests ---

func TestExtractPackagePath_Valid(t *testing.T) {
	pkg, err := ExtractPackagePath("https://github.com/example/repo://pkg/sub.Func1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pkg != "pkg/sub" {
		t.Errorf("expected %q, got %q", "pkg/sub", pkg)
	}
}

func TestExtractPackagePath_MissingSeparator(t *testing.T) {
	_, err := ExtractPackagePath("no-separator")
	if err == nil {
		t.Fatal("expected error for missing '://' separator, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Errorf("error should contain 'malformed', got: %v", err)
	}
}

func TestExtractPackagePath_NoDot(t *testing.T) {
	_, err := ExtractPackagePath("https://github.com/example/repo://nodot")
	if err == nil {
		t.Fatal("expected error for missing dot separator, got nil")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Errorf("error should contain 'malformed', got: %v", err)
	}
}

func TestComputeSnapshot_SkipsMalformedNodes(t *testing.T) {
	store := newMockGraphStore()
	ctx := context.Background()

	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.Repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// Well-formed node.
	goodNode := types.Node{
		NodeHash:      types.NewHash([]byte("node-good")),
		QualifiedName: "https://github.com/example/repo://pkg.Func1",
		Kind:          "function",
	}
	// Malformed node: no "://" separator.
	badNode := types.Node{
		NodeHash:      types.NewHash([]byte("node-bad")),
		QualifiedName: "no-separator",
		Kind:          "function",
	}

	store.Nodes[goodNode.NodeHash] = &goodNode
	store.Nodes[badNode.NodeHash] = &badNode
	store.nodesByNameResult = []types.Node{goodNode, badNode}

	goodEdge := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-good")),
		SourceHash: goodNode.NodeHash,
		TargetHash: badNode.NodeHash,
		EdgeType:   "calls",
	}
	badEdge := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-bad")),
		SourceHash: badNode.NodeHash,
		TargetHash: goodNode.NodeHash,
		EdgeType:   "calls",
	}

	store.Edges[goodEdge.EdgeHash] = &goodEdge
	store.Edges[badEdge.EdgeHash] = &badEdge
	store.edgesFromResult[goodNode.NodeHash] = []types.Edge{goodEdge}
	store.edgesFromResult[badNode.NodeHash] = []types.Edge{badEdge}

	sm := NewSnapshotManager(store)
	snap, err := sm.ComputeSnapshot(ctx, repoHash, "commit-test")
	if err != nil {
		t.Fatalf("ComputeSnapshot should succeed even with malformed node, got: %v", err)
	}

	// Both nodes' edges are collected (edge deduplication is by hash, not package),
	// but the malformed node's edges are excluded from the hierarchical tree.
	// The snapshot edge count reflects all edges seen (2 in this case).
	if snap.EdgeCount != 2 {
		t.Errorf("expected 2 edges in snapshot (both edges collected), got %d", snap.EdgeCount)
	}
	if snap.NodeCount != 2 {
		t.Errorf("expected 2 nodes, got %d", snap.NodeCount)
	}
}

func TestGarbageCollect_PreservesChainIntegrity(t *testing.T) {
	store := newMockGraphStore()
	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	sm := NewSnapshotManager(store)
	ctx := context.Background()

	// Build a chain of 3 snapshots directly.
	buildSnapshotChain(store, repoHash, 3)

	// GC keeping all 3 should remove nothing.
	removed, err := sm.GarbageCollect(ctx, repoHash, 3)
	if err != nil {
		t.Fatalf("GarbageCollect failed: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed when keeping all, got %d", removed)
	}

	// Verify chain is still walkable: latest -> middle -> oldest.
	latest := store.latestSnapshotResult
	if latest == nil {
		t.Fatal("latest snapshot is nil")
	}
	mid, err := store.GetSnapshot(ctx, latest.ParentHash)
	if err != nil || mid == nil {
		t.Fatal("middle snapshot missing from chain")
	}
	oldest, err := store.GetSnapshot(ctx, mid.ParentHash)
	if err != nil || oldest == nil {
		t.Fatal("oldest snapshot missing from chain")
	}
	if !oldest.ParentHash.IsZero() {
		t.Error("oldest snapshot should have zero parent")
	}

	// GC with keepCount < 1 should error.
	_, err = sm.GarbageCollect(ctx, repoHash, 0)
	if err == nil {
		t.Error("expected error for keepCount=0")
	}
}
