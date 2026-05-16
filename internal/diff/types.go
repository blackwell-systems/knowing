package diff

// SemanticDiffResult is the enriched diff between two snapshots.
type SemanticDiffResult struct {
	OldSnapshot   string             `json:"old_snapshot"`
	NewSnapshot   string             `json:"new_snapshot"`
	NodesAdded    []NodeChange       `json:"nodes_added"`
	NodesRemoved  []NodeChange       `json:"nodes_removed"`
	NodesModified []NodeModification `json:"nodes_modified,omitempty"`
	EdgesAdded    []EdgeChange       `json:"edges_added"`
	EdgesRemoved  []EdgeChange       `json:"edges_removed"`
	Summary       DiffSummary        `json:"summary"`
}

// NodeChange is a node with enriched metadata for display.
type NodeChange struct {
	QualifiedName string `json:"qualified_name"`
	Kind          string `json:"kind"`
	File          string `json:"file,omitempty"`
	Line          int    `json:"line,omitempty"`
	Signature     string `json:"signature,omitempty"`
	NodeHash      string `json:"node_hash"`
}

// NodeModification represents a node whose edges changed without
// the node itself being added or removed.
type NodeModification struct {
	QualifiedName string       `json:"qualified_name"`
	Kind          string       `json:"kind"`
	EdgesAdded    []EdgeChange `json:"edges_added,omitempty"`
	EdgesRemoved  []EdgeChange `json:"edges_removed,omitempty"`
}

// EdgeChange is an edge with enriched source/target metadata.
type EdgeChange struct {
	SourceName string  `json:"source_name"`
	TargetName string  `json:"target_name"`
	EdgeType   string  `json:"edge_type"`
	Confidence float64 `json:"confidence"`
	Provenance string  `json:"provenance,omitempty"`
}

// DiffSummary provides aggregate counts.
type DiffSummary struct {
	NodesAdded    int `json:"nodes_added"`
	NodesRemoved  int `json:"nodes_removed"`
	NodesModified int `json:"nodes_modified"`
	EdgesAdded    int `json:"edges_added"`
	EdgesRemoved  int `json:"edges_removed"`
}

// PRImpactResult is the blast radius analysis for a PR.
type PRImpactResult struct {
	OldSnapshot    string         `json:"old_snapshot"`
	NewSnapshot    string         `json:"new_snapshot"`
	ChangedSymbols []SymbolImpact `json:"changed_symbols"`
	AffectedEdges  []EdgeChange   `json:"affected_edges"`
	Summary        ImpactSummary  `json:"summary"`
}

// SymbolImpact describes the blast radius of a single changed symbol.
type SymbolImpact struct {
	Symbol      NodeChange   `json:"symbol"`
	ChangeType  string       `json:"change_type"`
	Callers     []NodeChange `json:"callers,omitempty"`
	Callees     []NodeChange `json:"callees,omitempty"`
	CallerCount int          `json:"caller_count"`
	CalleeCount int          `json:"callee_count"`
}

// ImpactSummary provides aggregate impact metrics.
type ImpactSummary struct {
	TotalSymbolsChanged  int    `json:"total_symbols_changed"`
	TotalCallersAffected int    `json:"total_callers_affected"`
	TotalCalleesAffected int    `json:"total_callees_affected"`
	RiskLevel            string `json:"risk_level"`
}
