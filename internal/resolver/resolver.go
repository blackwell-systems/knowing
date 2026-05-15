// Package resolver finds dangling cross-repo edges and retargets them to
// the correct node by matching across repos using hash recomputation.
//
// When repo A calls repo B's function, the extractor may compute the target
// hash using repo A's URL instead of repo B's URL. The resolver detects these
// mismatches and corrects them.
package resolver

import (
	"context"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// Store defines the subset of store operations needed by the resolver.
// This allows the resolver to compile independently and be tested with
// either a real SQLiteStore or a mock.
type Store interface {
	DanglingEdges(ctx context.Context) ([]types.Edge, error)
	AllRepos(ctx context.Context) ([]types.Repo, error)
	GetNode(ctx context.Context, hash types.Hash) (*types.Node, error)
	NodesByName(ctx context.Context, qualifiedPrefix string) ([]types.Node, error)
	DeleteEdge(ctx context.Context, hash types.Hash) error
	PutEdge(ctx context.Context, e types.Edge) error
}

// ResolveResult captures the outcome of resolving a single dangling edge.
type ResolveResult struct {
	OriginalEdge types.Edge
	ResolvedNode *types.Node
	Action       string // "retargeted" or "skipped"
	Reason       string
}

// ResolveStats contains aggregate statistics from a resolution pass.
type ResolveStats struct {
	TotalDangling int
	Retargeted    int
	Skipped       int
	Errors        int
}

// Resolver resolves cross-repo dangling edges by recomputing hashes
// with the correct repo URL.
type Resolver struct {
	store Store
}

// NewResolver creates a Resolver backed by the given store.
func NewResolver(store Store) *Resolver {
	return &Resolver{store: store}
}

// Resolve finds all dangling edges and attempts to retarget them.
// It returns aggregate statistics.
func (r *Resolver) Resolve(ctx context.Context) (*ResolveStats, error) {
	_, stats, err := r.ResolveWithDetails(ctx)
	return stats, err
}

// ResolveWithDetails finds all dangling edges and attempts to retarget them,
// returning per-edge results along with aggregate statistics.
func (r *Resolver) ResolveWithDetails(ctx context.Context) ([]ResolveResult, *ResolveStats, error) {
	danglingEdges, err := r.store.DanglingEdges(ctx)
	if err != nil {
		return nil, nil, err
	}

	stats := &ResolveStats{TotalDangling: len(danglingEdges)}
	if len(danglingEdges) == 0 {
		return nil, stats, nil
	}

	// Load all nodes and all repos.
	allNodes, err := r.store.NodesByName(ctx, "")
	if err != nil {
		return nil, nil, err
	}

	repos, err := r.store.AllRepos(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Collect all unique repo URLs.
	repoURLs := make([]string, 0, len(repos))
	for _, repo := range repos {
		repoURLs = append(repoURLs, repo.RepoURL)
	}

	// Build reverse lookup: for each node, for each candidate repo URL,
	// compute what the hash WOULD be if the node's repo URL were replaced
	// with the candidate URL. Map that "wrong" hash to the actual node.
	wrongHashToNode := make(map[types.Hash]*types.Node)

	for i := range allNodes {
		node := &allNodes[i]
		nodeRepoURL, pkgPath, symbolName := extractHashInputs(*node)
		if nodeRepoURL == "" || symbolName == "" {
			continue
		}

		// Verify our parsing by recomputing the node's own hash.
		recomputed := types.ComputeNodeHash(nodeRepoURL, pkgPath, types.EmptyHash, symbolName, node.Kind)
		if recomputed != node.NodeHash {
			// Parsing is wrong for this node; skip it.
			continue
		}

		for _, candidateURL := range repoURLs {
			if candidateURL == nodeRepoURL {
				continue // Same repo, no mismatch possible.
			}
			wrongHash := types.ComputeNodeHash(candidateURL, pkgPath, types.EmptyHash, symbolName, node.Kind)
			wrongHashToNode[wrongHash] = node
		}
	}

	// Resolve each dangling edge.
	var results []ResolveResult
	for _, edge := range danglingEdges {
		resolvedNode, ok := wrongHashToNode[edge.TargetHash]
		if !ok {
			stats.Skipped++
			results = append(results, ResolveResult{
				OriginalEdge: edge,
				Action:       "skipped",
				Reason:       "no matching node found in any repo",
			})
			continue
		}

		// Retarget: delete old edge, create new one with the correct target.
		newEdgeHash := types.ComputeEdgeHash(edge.SourceHash, resolvedNode.NodeHash, edge.EdgeType, edge.Provenance)
		newEdge := types.Edge{
			EdgeHash:   newEdgeHash,
			SourceHash: edge.SourceHash,
			TargetHash: resolvedNode.NodeHash,
			EdgeType:   edge.EdgeType,
			Confidence: edge.Confidence,
			Provenance: edge.Provenance,
		}

		if err := r.store.DeleteEdge(ctx, edge.EdgeHash); err != nil {
			stats.Errors++
			results = append(results, ResolveResult{
				OriginalEdge: edge,
				Action:       "skipped",
				Reason:       "delete failed: " + err.Error(),
			})
			continue
		}

		if err := r.store.PutEdge(ctx, newEdge); err != nil {
			stats.Errors++
			results = append(results, ResolveResult{
				OriginalEdge: edge,
				Action:       "skipped",
				Reason:       "put failed: " + err.Error(),
			})
			continue
		}

		stats.Retargeted++
		results = append(results, ResolveResult{
			OriginalEdge: edge,
			ResolvedNode: resolvedNode,
			Action:       "retargeted",
			Reason:       "matched node in repo " + resolvedNode.QualifiedName,
		})
	}

	return results, stats, nil
}

// extractHashInputs parses a node's QualifiedName and Kind to recover the
// parameters originally passed to ComputeNodeHash.
//
// QualifiedName formats:
//   - Functions/types: "{repoURL}://{pkgPath}.{symbolName}"
//   - Methods:         "{repoURL}://{pkgPath}.{typeName}.{methodName}"
//
// ComputeNodeHash inputs:
//   - Functions/types: (repoURL, pkgPath, _, symbolName, kind)
//   - Methods:         (repoURL, pkgPath, _, methodName, kind)
//     where pkgPath is the Go import path (does NOT include the type name)
func extractHashInputs(node types.Node) (repoURL, pkgPath, symbolName string) {
	parts := strings.SplitN(node.QualifiedName, "://", 2)
	if len(parts) != 2 {
		return "", "", ""
	}
	repoURL = parts[0]
	remainder := parts[1] // e.g., "testmod/pkg.Hello" or "testmod/pkg.Type.Method"

	lastDot := strings.LastIndex(remainder, ".")
	if lastDot < 0 {
		return repoURL, remainder, ""
	}
	symbolName = remainder[lastDot+1:]
	prefix := remainder[:lastDot] // e.g., "testmod/pkg" or "testmod/pkg.Type"

	if node.Kind == "method" {
		// For methods, prefix contains "pkgPath.TypeName".
		// We need just pkgPath, so find the last dot in prefix.
		secondLastDot := strings.LastIndex(prefix, ".")
		if secondLastDot >= 0 {
			pkgPath = prefix[:secondLastDot]
		} else {
			pkgPath = prefix
		}
	} else {
		pkgPath = prefix
	}
	return repoURL, pkgPath, symbolName
}
