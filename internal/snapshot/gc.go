package snapshot

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// GarbageCollectFull extends the basic snapshot chain GC with a reachability
// sweep that prunes orphaned nodes and edges not referenced by any surviving
// snapshot. Returns: snapshots removed, nodes pruned, edges pruned.
func (sm *SnapshotManager) GarbageCollectFull(ctx context.Context, repoHash types.Hash, keepCount int) (removed int, prunedNodes int64, prunedEdges int64, err error) {
	// Step 1: Remove old snapshots.
	removed, err = sm.GarbageCollect(ctx, repoHash, keepCount)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("snapshot GC: %w", err)
	}

	// Step 2: Get the repo to resolve the RepoURL for node queries.
	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return removed, 0, 0, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return removed, 0, 0, fmt.Errorf("repo not found: %s", repoHash)
	}

	// Step 3: Get all nodes for this repo.
	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return removed, 0, 0, fmt.Errorf("querying nodes: %w", err)
	}

	// Step 4: Build reachable sets by iterating all remaining nodes and their edges.
	reachableNodes := make(map[types.Hash]struct{}, len(nodes))
	reachableEdges := make(map[types.Hash]struct{})

	for _, node := range nodes {
		reachableNodes[node.NodeHash] = struct{}{}

		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return removed, 0, 0, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, edge := range edges {
			reachableEdges[edge.EdgeHash] = struct{}{}
			reachableNodes[edge.SourceHash] = struct{}{}
			reachableNodes[edge.TargetHash] = struct{}{}
		}
	}

	// Step 5: Delete unreachable nodes.
	prunedNodes, err = sm.store.DeleteNodesNotIn(ctx, reachableNodes)
	if err != nil {
		return removed, 0, 0, fmt.Errorf("deleting unreachable nodes: %w", err)
	}

	// Step 6: Delete unreachable edges.
	prunedEdges, err = sm.store.DeleteEdgesNotIn(ctx, reachableEdges)
	if err != nil {
		return removed, prunedNodes, 0, fmt.Errorf("deleting unreachable edges: %w", err)
	}

	return removed, prunedNodes, prunedEdges, nil
}
