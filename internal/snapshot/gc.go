package snapshot

import (
	"context"
	"fmt"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// GCStats records the outcome of a full garbage collection run.
type GCStats struct {
	// SnapshotsRemoved is the number of old snapshots pruned from the chain.
	SnapshotsRemoved int

	// NodesRemoved is the number of orphaned nodes deleted from the graph.
	NodesRemoved int64

	// EdgesRemoved is the number of orphaned edges deleted from the graph.
	EdgesRemoved int64

	// Duration is the wall-clock time taken by the full GC run.
	Duration time.Duration
}

// GarbageCollectFull extends the basic snapshot chain GC with a reachability
// sweep that prunes orphaned nodes and edges not referenced by any surviving
// snapshot. It returns a GCStats struct describing what was removed.
func (sm *SnapshotManager) GarbageCollectFull(ctx context.Context, repoHash types.Hash, keepCount int) (GCStats, error) {
	start := time.Now()
	var stats GCStats

	// Step 1: Remove old snapshots.
	removed, err := sm.GarbageCollect(ctx, repoHash, keepCount)
	if err != nil {
		return stats, fmt.Errorf("snapshot GC: %w", err)
	}
	stats.SnapshotsRemoved = removed

	// Step 2: Get the repo to resolve the RepoURL for node queries.
	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return stats, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return stats, fmt.Errorf("repo not found: %s", repoHash)
	}

	// Step 3: Get all nodes for this repo.
	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return stats, fmt.Errorf("querying nodes: %w", err)
	}

	// Step 4: Build reachable sets by iterating all remaining nodes and their edges.
	reachableNodes := make(map[types.Hash]struct{}, len(nodes))
	reachableEdges := make(map[types.Hash]struct{})

	for _, node := range nodes {
		reachableNodes[node.NodeHash] = struct{}{}

		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return stats, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, edge := range edges {
			reachableEdges[edge.EdgeHash] = struct{}{}
			reachableNodes[edge.SourceHash] = struct{}{}
			reachableNodes[edge.TargetHash] = struct{}{}
		}
	}

	// Step 5: Delete unreachable nodes.
	prunedNodes, err := sm.store.DeleteNodesNotIn(ctx, reachableNodes)
	if err != nil {
		return stats, fmt.Errorf("deleting unreachable nodes: %w", err)
	}
	stats.NodesRemoved = prunedNodes

	// Step 6: Delete unreachable edges.
	prunedEdges, err := sm.store.DeleteEdgesNotIn(ctx, reachableEdges)
	if err != nil {
		return stats, fmt.Errorf("deleting unreachable edges: %w", err)
	}
	stats.EdgesRemoved = prunedEdges

	stats.Duration = time.Since(start)
	return stats, nil
}
