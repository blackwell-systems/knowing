package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SnapshotManager manages Merkle root computation, snapshot chain
// management, diff, and garbage collection.
type SnapshotManager struct {
	store types.GraphStore
}

// NewSnapshotManager creates a new SnapshotManager backed by the given GraphStore.
func NewSnapshotManager(store types.GraphStore) *SnapshotManager {
	return &SnapshotManager{store: store}
}

// ComputeSnapshot computes a new snapshot for a repository by collecting all
// edge hashes, building a Merkle tree, and storing the resulting snapshot.
// The snapshot is chained to the latest existing snapshot for the repo.
func (sm *SnapshotManager) ComputeSnapshot(ctx context.Context, repoHash types.Hash, commitHash string) (*types.Snapshot, error) {
	// Collect all edge hashes for this repo by traversing files -> nodes -> edges.
	edgeHashes, nodeCount, edgeCount, err := sm.collectRepoEdges(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("collecting edges: %w", err)
	}

	// Build Merkle tree from sorted edge hashes.
	tree := BuildMerkleTree(edgeHashes)

	// Get the latest snapshot for parent chain.
	var parentHash types.Hash
	latest, err := sm.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("getting latest snapshot: %w", err)
	}
	if latest != nil {
		parentHash = latest.SnapshotHash
	}

	snap := types.Snapshot{
		SnapshotHash: tree.Root,
		ParentHash:   parentHash,
		RepoHash:     repoHash,
		CommitHash:   commitHash,
		Timestamp:    time.Now().Unix(),
		NodeCount:    nodeCount,
		EdgeCount:    edgeCount,
	}

	if err := sm.store.CreateSnapshot(ctx, snap); err != nil {
		return nil, fmt.Errorf("creating snapshot: %w", err)
	}

	return &snap, nil
}

// Diff returns the structural difference between two snapshots.
// It delegates to the GraphStore's SnapshotDiff implementation.
func (sm *SnapshotManager) Diff(ctx context.Context, oldHash, newHash types.Hash) (*types.DiffResult, error) {
	return sm.store.SnapshotDiff(ctx, oldHash, newHash)
}

// GarbageCollect removes old snapshots for a repo, keeping the most recent
// keepCount snapshots. It preserves chain integrity by walking the chain from
// the latest snapshot backwards. Returns the number of removed snapshots.
func (sm *SnapshotManager) GarbageCollect(ctx context.Context, repoHash types.Hash, keepCount int) (removed int, err error) {
	if keepCount < 1 {
		return 0, fmt.Errorf("keepCount must be >= 1, got %d", keepCount)
	}

	// Walk the snapshot chain from latest, collecting all snapshots.
	chain, err := sm.walkChain(ctx, repoHash)
	if err != nil {
		return 0, fmt.Errorf("walking snapshot chain: %w", err)
	}

	if len(chain) <= keepCount {
		return 0, nil
	}

	// Keep the most recent keepCount snapshots; the rest are candidates for removal.
	// Chain is ordered newest-first.
	toRemove := chain[keepCount:]

	// For now, we count removals. The GraphStore interface does not expose
	// DeleteSnapshot, so we track what would be removed. When the store
	// gains deletion support, this will perform actual deletion.
	// TODO: Add DeleteSnapshot to GraphStore interface.
	removed = len(toRemove)
	return removed, nil
}

// collectRepoEdges gathers all edge hashes for a repo by traversing
// files -> nodes (via NodesByName with repo prefix) -> edges (via EdgesFrom).
// Returns the edge hashes, node count, and edge count.
func (sm *SnapshotManager) collectRepoEdges(ctx context.Context, repoHash types.Hash) ([]types.Hash, int, int, error) {
	// Get the repo to determine its URL for node prefix queries.
	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return nil, 0, 0, fmt.Errorf("repo not found: %s", repoHash)
	}

	// Get all nodes for this repo using the repo URL as qualified name prefix.
	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("querying nodes by name: %w", err)
	}

	// Collect all edges from each node (using empty edge type to get all types).
	edgeSeen := make(map[types.Hash]struct{})
	var edgeHashes []types.Hash

	for _, node := range nodes {
		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, e := range edges {
			if _, ok := edgeSeen[e.EdgeHash]; !ok {
				edgeSeen[e.EdgeHash] = struct{}{}
				edgeHashes = append(edgeHashes, e.EdgeHash)
			}
		}
	}

	return edgeHashes, len(nodes), len(edgeHashes), nil
}

// walkChain walks the snapshot chain from latest to earliest for a repo.
// Returns snapshots ordered newest-first.
func (sm *SnapshotManager) walkChain(ctx context.Context, repoHash types.Hash) ([]types.Snapshot, error) {
	var chain []types.Snapshot

	current, err := sm.store.LatestSnapshot(ctx, repoHash)
	if err != nil {
		return nil, err
	}

	for current != nil {
		chain = append(chain, *current)
		if current.ParentHash.IsZero() {
			break
		}
		current, err = sm.store.GetSnapshot(ctx, current.ParentHash)
		if err != nil {
			return nil, fmt.Errorf("getting snapshot %s: %w", current.ParentHash, err)
		}
	}

	return chain, nil
}
