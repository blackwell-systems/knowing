package context

import (
	"github.com/blackwell-systems/knowing/internal/types"
)

// DeltaPack represents the difference between two context packs.
// Used for incremental context delivery: instead of retransmitting the full
// pack when a few symbols changed, send only what was added and removed.
type DeltaPack struct {
	BaseRoot     types.Hash     // pack_root the agent has (prior)
	NewRoot      types.Hash     // pack_root of the current result
	Removed      []RankedSymbol // symbols in prior but not in current
	Added        []RankedSymbol // symbols in current but not in prior
	Unchanged    []RankedSymbol // symbols in both (for edge diffing context)
	RemovedEdges []ContextEdge  // edges in prior but not in current
	AddedEdges   []ContextEdge  // edges in current but not in prior
	// Token estimates for the delta vs full pack.
	DeltaTokens int // estimated tokens for the delta encoding
	FullTokens  int // estimated tokens for the full encoding
}

// DiffPacks computes the structural difference between a prior and current
// ContextBlock. The diff is based on node hashes (content-addressed identity),
// not position or score. A symbol that moved in rank but didn't change content
// appears in neither Removed nor Added.
func DiffPacks(prior, current *ContextBlock, format string) *DeltaPack {
	delta := &DeltaPack{
		BaseRoot: prior.PackRoot,
		NewRoot:  current.PackRoot,
	}

	// Build hash sets for O(1) lookup.
	priorSet := make(map[types.Hash]RankedSymbol, len(prior.Symbols))
	for _, s := range prior.Symbols {
		priorSet[s.Node.NodeHash] = s
	}
	currentSet := make(map[types.Hash]RankedSymbol, len(current.Symbols))
	for _, s := range current.Symbols {
		currentSet[s.Node.NodeHash] = s
	}

	// Symbols in prior but not in current: removed.
	for h, s := range priorSet {
		if _, ok := currentSet[h]; !ok {
			delta.Removed = append(delta.Removed, s)
		}
	}

	// Symbols in current but not in prior: added.
	// Symbols in both: unchanged.
	for h, s := range currentSet {
		if _, ok := priorSet[h]; !ok {
			delta.Added = append(delta.Added, s)
		} else {
			delta.Unchanged = append(delta.Unchanged, s)
		}
	}

	// Edge diffing: set difference on (source, target, type) triples.
	type edgeKey struct {
		source, target, edgeType string
	}
	priorEdges := make(map[edgeKey]ContextEdge, len(prior.Edges))
	for _, e := range prior.Edges {
		priorEdges[edgeKey{e.Source, e.Target, e.EdgeType}] = e
	}
	currentEdges := make(map[edgeKey]ContextEdge, len(current.Edges))
	for _, e := range current.Edges {
		currentEdges[edgeKey{e.Source, e.Target, e.EdgeType}] = e
	}

	for k, e := range priorEdges {
		if _, ok := currentEdges[k]; !ok {
			delta.RemovedEdges = append(delta.RemovedEdges, e)
		}
	}
	for k, e := range currentEdges {
		if _, ok := priorEdges[k]; !ok {
			delta.AddedEdges = append(delta.AddedEdges, e)
		}
	}

	// Estimate token costs.
	delta.FullTokens = estimatePackTokens(current.Symbols, current.Edges, format)
	delta.DeltaTokens = estimateDeltaTokens(delta, format)

	return delta
}

// IsWorthIt returns true if the delta encoding saves enough tokens to justify
// the overhead. If the delta is more than 60% of the full pack size, just
// send the full pack (the section headers and context overhead aren't worth it).
func (d *DeltaPack) IsWorthIt() bool {
	if d.FullTokens == 0 {
		return false
	}
	return float64(d.DeltaTokens) < 0.6*float64(d.FullTokens)
}

// SavingsPercent returns the token savings as a percentage (0-100).
func (d *DeltaPack) SavingsPercent() float64 {
	if d.FullTokens == 0 {
		return 0
	}
	return 100.0 * (1.0 - float64(d.DeltaTokens)/float64(d.FullTokens))
}

// SymbolOverlapPercent returns what fraction of the current pack's symbols
// were already in the prior pack.
func (d *DeltaPack) SymbolOverlapPercent() float64 {
	total := len(d.Unchanged) + len(d.Added)
	if total == 0 {
		return 0
	}
	return 100.0 * float64(len(d.Unchanged)) / float64(total)
}

// estimatePackTokens estimates the total token cost of a full context pack.
func estimatePackTokens(symbols []RankedSymbol, edges []ContextEdge, format string) int {
	tokens := 10 // header overhead
	for _, s := range symbols {
		tokens += EstimateNodeTokensForFormat(s.Node, format)
	}
	// Each edge is roughly 1 line in GCF: "@N<@M edgetype" ~ 4-6 tokens.
	tokens += len(edges) * 5
	return tokens
}

// estimateDeltaTokens estimates the token cost of the delta encoding.
func estimateDeltaTokens(d *DeltaPack, format string) int {
	tokens := 15 // header + section headers overhead (delta=true, base_root, new_root, sections)
	for _, s := range d.Added {
		tokens += EstimateNodeTokensForFormat(s.Node, format)
	}
	// Removed symbols are just "@N fn qualified.Name" (short reference).
	// ~8 tokens per removed symbol (QN + kind).
	tokens += len(d.Removed) * 8
	// Edge diffs: same as full edges, ~5 tokens each.
	tokens += len(d.AddedEdges) * 5
	tokens += len(d.RemovedEdges) * 5
	return tokens
}
