package diff

import (
	"context"
	"encoding/hex"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SemanticDiff computes an enriched semantic diff between two snapshots.
// It takes the raw DiffResult from store.SnapshotDiff and enriches it with
// node metadata (qualified names, signatures) and detects modified nodes
// (nodes whose edges changed without the node being added/removed).
func SemanticDiff(ctx context.Context, store types.GraphStore, oldSnapshot, newSnapshot types.Hash) (*SemanticDiffResult, error) {
	diff, err := store.SnapshotDiff(ctx, oldSnapshot, newSnapshot)
	if err != nil {
		return nil, err
	}

	result := &SemanticDiffResult{
		OldSnapshot: oldSnapshot.String(),
		NewSnapshot: newSnapshot.String(),
	}

	// Track node hashes that are added or removed so we can detect modifications.
	addedSet := make(map[types.Hash]bool)
	removedSet := make(map[types.Hash]bool)

	// Enrich NodesAdded.
	for _, n := range diff.NodesAdded {
		result.NodesAdded = append(result.NodesAdded, nodeToChange(n))
		addedSet[n.NodeHash] = true
	}

	// Enrich NodesRemoved.
	for _, n := range diff.NodesRemoved {
		result.NodesRemoved = append(result.NodesRemoved, nodeToChange(n))
		removedSet[n.NodeHash] = true
	}

	// Group edge changes by source node hash to detect modifications.
	type edgeChanges struct {
		added   []EdgeChange
		removed []EdgeChange
	}
	sourceEdgeChanges := make(map[types.Hash]*edgeChanges)

	// Process EdgesAdded.
	for _, e := range diff.EdgesAdded {
		ec := enrichEdge(ctx, store, e)
		result.EdgesAdded = append(result.EdgesAdded, ec)

		if !addedSet[e.SourceHash] && !removedSet[e.SourceHash] {
			if _, ok := sourceEdgeChanges[e.SourceHash]; !ok {
				sourceEdgeChanges[e.SourceHash] = &edgeChanges{}
			}
			sourceEdgeChanges[e.SourceHash].added = append(sourceEdgeChanges[e.SourceHash].added, ec)
		}
	}

	// Process EdgesRemoved.
	for _, e := range diff.EdgesRemoved {
		ec := enrichEdge(ctx, store, e)
		result.EdgesRemoved = append(result.EdgesRemoved, ec)

		if !addedSet[e.SourceHash] && !removedSet[e.SourceHash] {
			if _, ok := sourceEdgeChanges[e.SourceHash]; !ok {
				sourceEdgeChanges[e.SourceHash] = &edgeChanges{}
			}
			sourceEdgeChanges[e.SourceHash].removed = append(sourceEdgeChanges[e.SourceHash].removed, ec)
		}
	}

	// Build NodeModification entries for source nodes that had edge changes
	// but were not themselves added or removed.
	for sourceHash, changes := range sourceEdgeChanges {
		node, err := store.GetNode(ctx, sourceHash)
		if err != nil {
			return nil, err
		}
		qn := ""
		kind := ""
		if node != nil {
			qn = node.QualifiedName
			kind = node.Kind
		}
		result.NodesModified = append(result.NodesModified, NodeModification{
			QualifiedName: qn,
			Kind:          kind,
			EdgesAdded:    changes.added,
			EdgesRemoved:  changes.removed,
		})
	}

	// Build summary.
	result.Summary = DiffSummary{
		NodesAdded:    len(result.NodesAdded),
		NodesRemoved:  len(result.NodesRemoved),
		NodesModified: len(result.NodesModified),
		EdgesAdded:    len(result.EdgesAdded),
		EdgesRemoved:  len(result.EdgesRemoved),
	}

	return result, nil
}

// nodeToChange converts a types.Node to a NodeChange for display.
func nodeToChange(n types.Node) NodeChange {
	return NodeChange{
		QualifiedName: n.QualifiedName,
		Kind:          n.Kind,
		Line:          n.Line,
		Signature:     n.Signature,
		NodeHash:      hex.EncodeToString(n.NodeHash[:]),
	}
}

// enrichEdge resolves source and target node names for an edge.
func enrichEdge(ctx context.Context, store types.GraphStore, e types.Edge) EdgeChange {
	ec := EdgeChange{
		EdgeType:   e.EdgeType,
		Confidence: e.Confidence,
		Provenance: e.Provenance,
	}

	src, err := store.GetNode(ctx, e.SourceHash)
	if err == nil && src != nil {
		ec.SourceName = src.QualifiedName
	}

	tgt, err := store.GetNode(ctx, e.TargetHash)
	if err == nil && tgt != nil {
		ec.TargetName = tgt.QualifiedName
	}

	return ec
}
