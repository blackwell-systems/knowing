package snapshot

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// mockGraphStoreWithCounts extends mockGraphStore to return realistic counts
// from DeleteNodesNotIn and DeleteEdgesNotIn so we can verify GCStats.
type mockGraphStoreWithCounts struct {
	mockGraphStore
	deleteNodesCount int64
	deleteEdgesCount int64
}

func (m *mockGraphStoreWithCounts) DeleteNodesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return m.deleteNodesCount, nil
}

func (m *mockGraphStoreWithCounts) DeleteEdgesNotIn(_ context.Context, _ map[types.Hash]struct{}) (int64, error) {
	return m.deleteEdgesCount, nil
}

func TestGarbageCollectFull_StatsPopulated(t *testing.T) {
	base := newMockGraphStore()

	// Register a repo so GarbageCollectFull can find it.
	repoHash := types.NewHash([]byte("https://github.com/example/repo"))
	base.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo",
	}

	// Populate a 3-snapshot chain; keeping 2 means 1 should be removed.
	buildSnapshotChain(base, repoHash, 3)

	store := &mockGraphStoreWithCounts{
		mockGraphStore:   *base,
		deleteNodesCount: 7,
		deleteEdgesCount: 3,
	}

	sm := NewSnapshotManager(store)
	ctx := context.Background()

	stats, err := sm.GarbageCollectFull(ctx, repoHash, 2)
	if err != nil {
		t.Fatalf("GarbageCollectFull: %v", err)
	}

	if stats.SnapshotsRemoved != 1 {
		t.Errorf("expected 1 snapshot removed, got %d", stats.SnapshotsRemoved)
	}
	if stats.NodesRemoved != 7 {
		t.Errorf("expected 7 nodes removed, got %d", stats.NodesRemoved)
	}
	if stats.EdgesRemoved != 3 {
		t.Errorf("expected 3 edges removed, got %d", stats.EdgesRemoved)
	}
	if stats.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func TestGarbageCollectFull_ZeroStats_WhenNothingPruned(t *testing.T) {
	base := newMockGraphStore()

	repoHash := types.NewHash([]byte("https://github.com/example/repo2"))
	base.repos[repoHash] = &types.Repo{
		RepoHash: repoHash,
		RepoURL:  "https://github.com/example/repo2",
	}

	// Only 1 snapshot; keeping 5 means nothing is removed.
	buildSnapshotChain(base, repoHash, 1)

	store := &mockGraphStoreWithCounts{
		mockGraphStore:   *base,
		deleteNodesCount: 0,
		deleteEdgesCount: 0,
	}

	sm := NewSnapshotManager(store)
	ctx := context.Background()

	stats, err := sm.GarbageCollectFull(ctx, repoHash, 5)
	if err != nil {
		t.Fatalf("GarbageCollectFull: %v", err)
	}

	if stats.SnapshotsRemoved != 0 {
		t.Errorf("expected 0 snapshots removed, got %d", stats.SnapshotsRemoved)
	}
	if stats.NodesRemoved != 0 {
		t.Errorf("expected 0 nodes removed, got %d", stats.NodesRemoved)
	}
	if stats.EdgesRemoved != 0 {
		t.Errorf("expected 0 edges removed, got %d", stats.EdgesRemoved)
	}
}
