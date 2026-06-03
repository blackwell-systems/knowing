package wire

import (
	"fmt"
	"strings"
)

// DeltaPayload is the input structure for delta GCF encoding.
// It represents the diff between a prior context pack (base_root) and
// the current result (new_root).
type DeltaPayload struct {
	Tool     string
	BaseRoot string // pack_root the agent has
	NewRoot  string // pack_root of the current result
	Removed  []Symbol
	Added    []Symbol
	// RemovedEdges and AddedEdges use the Edge type with Status set.
	RemovedEdges []Edge
	AddedEdges   []Edge
	// Token accounting for the delta.
	DeltaTokens int
	FullTokens  int
}

// EncodeDelta serializes a DeltaPayload into GCF delta format.
// The agent applies this diff to its cached context to reconstruct the
// current pack without full retransmission.
//
// Format:
//
//	GCF tool=<tool> delta=true base_root=<prior> new_root=<current> tokens=<N> savings=<pct>%
//	## removed
//	@0 fn qualified.Name
//	## added
//	@1 fn qualified.Name 0.85 rwr
//	## edges_removed
//	qualified.Source -> qualified.Target calls
//	## edges_added
//	qualified.Source -> qualified.Target calls
func EncodeDelta(d *DeltaPayload) string {
	var b strings.Builder

	// Header.
	savings := 0.0
	if d.FullTokens > 0 {
		savings = 100.0 * (1.0 - float64(d.DeltaTokens)/float64(d.FullTokens))
	}
	b.WriteString(fmt.Sprintf("GCF tool=%s delta=true base_root=%s new_root=%s tokens=%d savings=%.0f%%\n",
		d.Tool, d.BaseRoot, d.NewRoot, d.DeltaTokens, savings))

	// Removed symbols: short references (agent already has the full declaration).
	if len(d.Removed) > 0 {
		b.WriteString("## removed\n")
		for _, s := range d.Removed {
			kind := kindAbbrev[s.Kind]
			if kind == "" {
				kind = s.Kind
			}
			b.WriteString(fmt.Sprintf("%s %s\n", kind, s.QualifiedName))
		}
	}

	// Added symbols: full declarations (agent doesn't have these).
	if len(d.Added) > 0 {
		b.WriteString("## added\n")
		symIndex := make(map[string]int, len(d.Added))
		for i, s := range d.Added {
			symIndex[s.QualifiedName] = i
			kind := kindAbbrev[s.Kind]
			if kind == "" {
				kind = s.Kind
			}
			b.WriteString(fmt.Sprintf("@%d %s %s %.2f %s\n",
				i, kind, s.QualifiedName, s.Score, s.Provenance))
		}
	}

	// Removed edges.
	if len(d.RemovedEdges) > 0 {
		b.WriteString("## edges_removed\n")
		for _, e := range d.RemovedEdges {
			b.WriteString(fmt.Sprintf("%s -> %s %s\n", e.Source, e.Target, e.EdgeType))
		}
	}

	// Added edges.
	if len(d.AddedEdges) > 0 {
		b.WriteString("## edges_added\n")
		for _, e := range d.AddedEdges {
			b.WriteString(fmt.Sprintf("%s -> %s %s\n", e.Source, e.Target, e.EdgeType))
		}
	}

	return b.String()
}
