package snapshot

import (
	"context"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// VerifyError represents a single integrity check finding.
type VerifyError struct {
	Level   string     // "ERROR" or "WARN"
	Kind    string     // "dangling_edge", "hash_mismatch", "broken_chain", "missing_file"
	Hash    types.Hash // hash of the entity with the issue
	Message string     // human-readable description
}

// Verify performs integrity verification on a repo's graph.
// Checks: edge referential integrity, hash recomputation, snapshot chain continuity.
func (sm *SnapshotManager) Verify(ctx context.Context, repoHash types.Hash) ([]VerifyError, error) {
	var errs []VerifyError

	repo, err := sm.store.GetRepo(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("getting repo: %w", err)
	}
	if repo == nil {
		return nil, fmt.Errorf("repo not found: %s", repoHash)
	}

	nodes, err := sm.store.NodesByName(ctx, repo.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("querying nodes by name: %w", err)
	}

	// Check edge referential integrity and hash recomputation.
	edgeSeen := make(map[types.Hash]struct{})
	for _, node := range nodes {
		edges, err := sm.store.EdgesFrom(ctx, node.NodeHash, "")
		if err != nil {
			return nil, fmt.Errorf("querying edges from node %s: %w", node.NodeHash, err)
		}
		for _, edge := range edges {
			if _, ok := edgeSeen[edge.EdgeHash]; ok {
				continue
			}
			edgeSeen[edge.EdgeHash] = struct{}{}

			// a. Edge referential integrity: verify source and target nodes exist.
			src, err := sm.store.GetNode(ctx, edge.SourceHash)
			if err != nil {
				return nil, fmt.Errorf("getting source node: %w", err)
			}
			if src == nil {
				errs = append(errs, VerifyError{
					Level:   "ERROR",
					Kind:    "dangling_edge",
					Hash:    edge.EdgeHash,
					Message: fmt.Sprintf("edge %s references non-existent node %s", edge.EdgeHash, edge.SourceHash),
				})
			}

			tgt, err := sm.store.GetNode(ctx, edge.TargetHash)
			if err != nil {
				return nil, fmt.Errorf("getting target node: %w", err)
			}
			if tgt == nil {
				errs = append(errs, VerifyError{
					Level:   "ERROR",
					Kind:    "dangling_edge",
					Hash:    edge.EdgeHash,
					Message: fmt.Sprintf("edge %s references non-existent node %s", edge.EdgeHash, edge.TargetHash),
				})
			}

			// b. Hash recomputation (edges).
			if hashErr := types.VerifyEdgeHash(edge); hashErr != nil {
				errs = append(errs, VerifyError{
					Level:   "ERROR",
					Kind:    "hash_mismatch",
					Hash:    edge.EdgeHash,
					Message: hashErr.Error(),
				})
			}
		}

		// c. Node hash verification.
		pkgPath, pkgErr := ExtractPackagePath(node.QualifiedName)
		if pkgErr == nil {
			if hashErr := types.VerifyNodeHash(node, repo.RepoURL, pkgPath); hashErr != nil {
				errs = append(errs, VerifyError{
					Level:   "WARN",
					Kind:    "hash_mismatch",
					Hash:    node.NodeHash,
					Message: hashErr.Error(),
				})
			}
		}
	}

	// d. Snapshot chain continuity.
	chain, err := sm.walkChain(ctx, repoHash)
	if err != nil {
		return nil, fmt.Errorf("walking snapshot chain: %w", err)
	}

	// Build a set of known snapshot hashes.
	snapshotSet := make(map[types.Hash]struct{}, len(chain))
	for _, snap := range chain {
		snapshotSet[snap.SnapshotHash] = struct{}{}
	}

	for _, snap := range chain {
		if snap.ParentHash.IsZero() {
			continue
		}
		parent, err := sm.store.GetSnapshot(ctx, snap.ParentHash)
		if err != nil {
			return nil, fmt.Errorf("getting parent snapshot: %w", err)
		}
		if parent == nil {
			errs = append(errs, VerifyError{
				Level:   "ERROR",
				Kind:    "broken_chain",
				Hash:    snap.SnapshotHash,
				Message: fmt.Sprintf("snapshot %s parent %s not found", snap.SnapshotHash, snap.ParentHash),
			})
		}
	}

	return errs, nil
}
