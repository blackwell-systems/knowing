package diff

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/blackwell-systems/knowing/internal/types"
)

// PRImpact computes blast radius analysis for all symbols that changed between
// two snapshots. For each added, removed, or modified symbol, it finds callers
// and callees to assess impact scope.
func PRImpact(ctx context.Context, store types.GraphStore, oldSnapshot, newSnapshot types.Hash) (*PRImpactResult, error) {
	sdiff, err := SemanticDiff(ctx, store, oldSnapshot, newSnapshot)
	if err != nil {
		return nil, fmt.Errorf("semantic diff: %w", err)
	}

	result := &PRImpactResult{
		OldSnapshot: oldSnapshot.String(),
		NewSnapshot: newSnapshot.String(),
	}

	// Track unique callers and callees for the summary.
	uniqueCallers := make(map[string]bool)
	uniqueCallees := make(map[string]bool)

	// Removed nodes: breaking changes. Look up callers in old snapshot, callees in old snapshot.
	for _, nc := range sdiff.NodesRemoved {
		nodeHash, err := parseHash(nc.NodeHash)
		if err != nil {
			return nil, fmt.Errorf("parse removed node hash: %w", err)
		}

		impact := SymbolImpact{
			Symbol:     nc,
			ChangeType: "removed",
		}

		br, err := store.BlastRadius(ctx, nodeHash, oldSnapshot)
		if err != nil {
			return nil, fmt.Errorf("blast radius for removed %s: %w", nc.QualifiedName, err)
		}
		if br != nil {
			for _, callers := range br.ByRepo {
				for _, c := range callers {
					change := nodeToChange(c.Caller)
					impact.Callers = append(impact.Callers, change)
					uniqueCallers[change.NodeHash] = true
				}
			}
		}

		callees, err := store.TransitiveCallees(ctx, nodeHash, 3, oldSnapshot)
		if err != nil {
			return nil, fmt.Errorf("callees for removed %s: %w", nc.QualifiedName, err)
		}
		for _, c := range callees {
			change := nodeToChange(c.Node)
			impact.Callees = append(impact.Callees, change)
			uniqueCallees[change.NodeHash] = true
		}

		impact.CallerCount = len(impact.Callers)
		impact.CalleeCount = len(impact.Callees)
		result.ChangedSymbols = append(result.ChangedSymbols, impact)
	}

	// Added nodes: look up callees in new snapshot.
	for _, nc := range sdiff.NodesAdded {
		nodeHash, err := parseHash(nc.NodeHash)
		if err != nil {
			return nil, fmt.Errorf("parse added node hash: %w", err)
		}

		impact := SymbolImpact{
			Symbol:     nc,
			ChangeType: "added",
		}

		callees, err := store.TransitiveCallees(ctx, nodeHash, 3, newSnapshot)
		if err != nil {
			return nil, fmt.Errorf("callees for added %s: %w", nc.QualifiedName, err)
		}
		for _, c := range callees {
			change := nodeToChange(c.Node)
			impact.Callees = append(impact.Callees, change)
			uniqueCallees[change.NodeHash] = true
		}

		impact.CalleeCount = len(impact.Callees)
		result.ChangedSymbols = append(result.ChangedSymbols, impact)
	}

	// Modified nodes: look up blast radius in new snapshot.
	for _, mod := range sdiff.NodesModified {
		nodes, err := store.NodesByQualifiedName(ctx, mod.QualifiedName)
		if err != nil {
			return nil, fmt.Errorf("lookup modified node %s: %w", mod.QualifiedName, err)
		}
		if len(nodes) == 0 {
			continue
		}
		node := nodes[0]

		impact := SymbolImpact{
			Symbol:     nodeToChange(node),
			ChangeType: "modified",
		}

		br, err := store.BlastRadius(ctx, node.NodeHash, newSnapshot)
		if err != nil {
			return nil, fmt.Errorf("blast radius for modified %s: %w", mod.QualifiedName, err)
		}
		if br != nil {
			for _, callers := range br.ByRepo {
				for _, c := range callers {
					change := nodeToChange(c.Caller)
					impact.Callers = append(impact.Callers, change)
					uniqueCallers[change.NodeHash] = true
				}
			}
		}

		callees, err := store.TransitiveCallees(ctx, node.NodeHash, 3, newSnapshot)
		if err != nil {
			return nil, fmt.Errorf("callees for modified %s: %w", mod.QualifiedName, err)
		}
		for _, c := range callees {
			change := nodeToChange(c.Node)
			impact.Callees = append(impact.Callees, change)
			uniqueCallees[change.NodeHash] = true
		}

		impact.CallerCount = len(impact.Callers)
		impact.CalleeCount = len(impact.Callees)
		result.ChangedSymbols = append(result.ChangedSymbols, impact)
	}

	// Collect affected edges from the semantic diff.
	result.AffectedEdges = append(result.AffectedEdges, sdiff.EdgesAdded...)
	result.AffectedEdges = append(result.AffectedEdges, sdiff.EdgesRemoved...)

	// Compute summary.
	totalCallers := len(uniqueCallers)
	result.Summary = ImpactSummary{
		TotalSymbolsChanged:  len(result.ChangedSymbols),
		TotalCallersAffected: totalCallers,
		TotalCalleesAffected: len(uniqueCallees),
		RiskLevel:            riskLevel(totalCallers),
	}

	return result, nil
}

// parseHash decodes a hex-encoded hash string into a types.Hash.
func parseHash(s string) (types.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.Hash{}, fmt.Errorf("decode hash %q: %w", s, err)
	}
	if len(b) != 32 {
		return types.Hash{}, fmt.Errorf("hash %q has length %d, want 32", s, len(b))
	}
	var h types.Hash
	copy(h[:], b)
	return h, nil
}

// riskLevel categorizes the risk based on total callers affected.
func riskLevel(totalCallers int) string {
	switch {
	case totalCallers > 20:
		return "high"
	case totalCallers > 5:
		return "medium"
	default:
		return "low"
	}
}
