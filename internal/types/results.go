package types

// CallerResult is a node with its distance from the query target.
type CallerResult struct {
	Node  Node
	Depth int
}

// CalleeResult is a node with its distance from the query source.
type CalleeResult struct {
	Node  Node
	Depth int
}

// BlastRadiusResult groups transitive callers by repository.
type BlastRadiusResult struct {
	Target     Node
	ByRepo     map[string][]CallerWithProvenance
	TotalCount int
	Truncated  bool
}

// CallerWithProvenance pairs a caller node with the edge provenance chain.
type CallerWithProvenance struct {
	Caller     Node
	Depth      int
	Confidence float64
	Provenance []EdgeProvenance
}

// DiffResult contains the structural diff between two snapshots.
type DiffResult struct {
	OldSnapshot  Hash
	NewSnapshot  Hash
	EdgesAdded   []Edge
	EdgesRemoved []Edge
	NodesAdded   []Node
	NodesRemoved []Node
}

// DerivedResult is a content-addressed computation result.
type DerivedResult struct {
	ResultHash   Hash
	QueryType    string
	QueryParams  Hash
	SnapshotRoot Hash
	Data         []byte
	ComputedAt   int64
	ComputedBy   string
}

// TraversalOptions controls bounded traversal with early termination.
type TraversalOptions struct {
	MaxDepth      int
	MaxResults    int
	MinConfidence float64
}
