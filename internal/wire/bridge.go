package wire

import (
	stdctx "context"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/types"
)

// FromContextBlock converts a ContextBlock into a wire.Payload, optionally
// querying the store for edges between the included symbols.
func FromContextBlock(ctx stdctx.Context, block *knowingctx.ContextBlock, tool string, store types.GraphStore) (*Payload, error) {
	p := &Payload{
		Tool:        tool,
		TokensUsed:  block.TokensUsed,
		TokenBudget: block.TokenBudget,
	}

	// Convert symbols.
	hashToQName := make(map[types.Hash]string, len(block.Symbols))
	qnameSet := make(map[string]bool, len(block.Symbols))

	for _, sym := range block.Symbols {
		p.Symbols = append(p.Symbols, Symbol{
			QualifiedName: sym.Node.QualifiedName,
			Kind:          sym.Node.Kind,
			Score:         sym.Score,
			Provenance:    sym.Provenance,
			Distance:      sym.Distance,
			Signature:     sym.Node.Signature,
			Components: Components{
				BlastRadius: sym.Components.BlastRadius,
				Confidence:  sym.Components.Confidence,
				Recency:     sym.Components.Recency,
				Distance:    sym.Components.Distance,
			},
		})
		hashToQName[sym.Node.NodeHash] = sym.Node.QualifiedName
		qnameSet[sym.Node.QualifiedName] = true
	}

	// If block already has edges, use those.
	if len(block.Edges) > 0 {
		for _, e := range block.Edges {
			p.Edges = append(p.Edges, Edge{
				Source:   e.Source,
				Target:   e.Target,
				EdgeType: e.EdgeType,
			})
		}
		return p, nil
	}

	// Otherwise, discover edges between included symbols from the store.
	if store != nil {
		for _, sym := range block.Symbols {
			edges, err := store.EdgesFrom(ctx, sym.Node.NodeHash, "")
			if err != nil {
				continue
			}
			for _, e := range edges {
				targetQName, ok := hashToQName[e.TargetHash]
				if !ok {
					continue // target not in our symbol set
				}
				sourceQName := sym.Node.QualifiedName
				p.Edges = append(p.Edges, Edge{
					Source:   sourceQName,
					Target:   targetQName,
					EdgeType: e.EdgeType,
				})
			}
		}
	}

	return p, nil
}
