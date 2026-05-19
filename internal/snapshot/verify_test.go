package snapshot

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestVerify_CleanRepo(t *testing.T) {
	store := newMockGraphStore()
	ctx := context.Background()

	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// Create two nodes with correct hashes for the clean repo test.
	// We use simple hash values; the verification will check against computed hashes,
	// so these nodes use raw NewHash which won't match ComputeNodeHash. We accept
	// that WARN-level hash_mismatch may appear for pre-migration data. The key
	// check for CleanRepo is: no dangling edges, no broken chains.
	node1 := types.Node{
		NodeHash:      types.ComputeNodeHash("https://github.com/example/repo", "pkg", types.EmptyHash, "Func1", "function"),
		QualifiedName: "https://github.com/example/repo://pkg.Func1",
		Kind:          "function",
	}
	node2 := types.Node{
		NodeHash:      types.ComputeNodeHash("https://github.com/example/repo", "pkg", types.EmptyHash, "Func2", "function"),
		QualifiedName: "https://github.com/example/repo://pkg.Func2",
		Kind:          "function",
	}

	store.nodes[node1.NodeHash] = &node1
	store.nodes[node2.NodeHash] = &node2
	store.nodesByNameResult = []types.Node{node1, node2}

	// Create an edge with the correct hash.
	edge1 := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(node1.NodeHash, node2.NodeHash, "calls", ""),
		SourceHash: node1.NodeHash,
		TargetHash: node2.NodeHash,
		EdgeType:   "calls",
		Provenance: "",
	}
	store.edges[edge1.EdgeHash] = &edge1
	store.edgesFromResult[node1.NodeHash] = []types.Edge{edge1}

	// Create one snapshot with no parent.
	snapHash := types.NewHash([]byte("snap-1"))
	snap := types.Snapshot{
		SnapshotHash: snapHash,
		ParentHash:   types.EmptyHash,
		RepoHash:     repoHash,
		CommitHash:   "abc123",
	}
	store.snapshots[snapHash] = &snap
	store.latestSnapshotResult = &snap

	sm := NewSnapshotManager(store)
	verifyErrs, err := sm.Verify(ctx, repoHash)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Filter for ERROR-level only: clean repo should have no errors.
	for _, ve := range verifyErrs {
		if ve.Level == "ERROR" {
			t.Errorf("unexpected ERROR: kind=%s message=%s", ve.Kind, ve.Message)
		}
	}
}

func TestVerify_DanglingEdge(t *testing.T) {
	store := newMockGraphStore()
	ctx := context.Background()

	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	node1 := types.Node{
		NodeHash:      types.NewHash([]byte("node-1")),
		QualifiedName: "https://github.com/example/repo://pkg.Func1",
		Kind:          "function",
	}
	store.nodes[node1.NodeHash] = &node1
	store.nodesByNameResult = []types.Node{node1}

	// Edge targets a non-existent node.
	nonExistentHash := types.NewHash([]byte("does-not-exist"))
	edge1 := types.Edge{
		EdgeHash:   types.NewHash([]byte("edge-dangling")),
		SourceHash: node1.NodeHash,
		TargetHash: nonExistentHash,
		EdgeType:   "calls",
	}
	store.edges[edge1.EdgeHash] = &edge1
	store.edgesFromResult[node1.NodeHash] = []types.Edge{edge1}

	sm := NewSnapshotManager(store)
	verifyErrs, err := sm.Verify(ctx, repoHash)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Must have at least one dangling_edge error.
	found := false
	for _, ve := range verifyErrs {
		if ve.Kind == "dangling_edge" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one dangling_edge error, got: %+v", verifyErrs)
	}
}

func TestVerify_BrokenChain(t *testing.T) {
	store := newMockGraphStore()
	ctx := context.Background()

	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	store.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// No nodes or edges needed; just test chain integrity.
	store.nodesByNameResult = []types.Node{}

	// Create a snapshot whose parent hash does not exist in the store.
	missingParentHash := types.NewHash([]byte("missing-parent"))
	latestSnapHash := types.NewHash([]byte("latest-snap"))
	latestSnap := types.Snapshot{
		SnapshotHash: latestSnapHash,
		ParentHash:   missingParentHash, // points to non-existent snapshot
		RepoHash:     repoHash,
		CommitHash:   "def456",
	}
	store.snapshots[latestSnapHash] = &latestSnap
	store.latestSnapshotResult = &latestSnap
	// Note: missingParentHash is NOT in store.snapshots

	sm := NewSnapshotManager(store)
	verifyErrs, err := sm.Verify(ctx, repoHash)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// Must have at least one broken_chain error.
	found := false
	for _, ve := range verifyErrs {
		if ve.Kind == "broken_chain" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected at least one broken_chain error, got: %+v", verifyErrs)
	}
}
