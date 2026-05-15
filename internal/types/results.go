// Package types result types for graph queries and traversals.
package types

// CallerResult is a single node in a transitive callers traversal,
// paired with its depth (hop count) from the query target.
type CallerResult struct {
	Node  Node
	Depth int // 1 = direct caller, 2 = caller of a caller, etc.
}

// CalleeResult is a single node in a transitive callees traversal,
// paired with its depth (hop count) from the query source.
type CalleeResult struct {
	Node  Node
	Depth int // 1 = direct callee, 2 = callee of a callee, etc.
}

// BlastRadiusResult groups all transitive callers of a target node by
// the repository they belong to. This powers the blast_radius MCP tool,
// showing how a change to one symbol ripples across the codebase.
type BlastRadiusResult struct {
	Target     Node                              // the node whose blast radius was computed
	ByRepo     map[string][]CallerWithProvenance // repo URL -> callers in that repo
	TotalCount int                               // total number of callers across all repos
	Truncated  bool                              // true if traversal hit the max depth limit
}

// CallerWithProvenance pairs a caller node with the edge provenance chain
// from that caller back to the target. Confidence is the minimum confidence
// along the call path.
type CallerWithProvenance struct {
	Caller     Node
	Depth      int
	Confidence float64          // minimum confidence along the call path (0.0 to 1.0)
	Provenance []EdgeProvenance // ordered provenance chain from caller to target
}

// DiffResult contains the structural diff between two snapshots.
// Used by the snapshot_diff and semantic_diff MCP tools to report
// what changed between two points in time.
type DiffResult struct {
	OldSnapshot  Hash
	NewSnapshot  Hash
	EdgesAdded   []Edge // edges present in NewSnapshot but not OldSnapshot
	EdgesRemoved []Edge // edges present in OldSnapshot but not NewSnapshot
	NodesAdded   []Node // nodes present in NewSnapshot but not OldSnapshot
	NodesRemoved []Node // nodes present in OldSnapshot but not NewSnapshot
}

// DerivedResult is a content-addressed cached computation result.
// Used by ComputationCache to store and retrieve expensive query results
// (e.g., blast radius, transitive callers) keyed by query parameters
// and snapshot root.
type DerivedResult struct {
	ResultHash   Hash   // content-addressed hash of this result
	QueryType    string // type of query (e.g., "blast_radius", "transitive_callers")
	QueryParams  Hash   // hash of the query parameters
	SnapshotRoot Hash   // snapshot root at the time of computation
	Data         []byte // serialized result data
	ComputedAt   int64  // unix timestamp when computed
	ComputedBy   string // identifier of the computing agent/indexer
}

// TraversalOptions controls bounded graph traversal with early termination.
// Used to prevent unbounded recursion in transitive caller/callee queries.
type TraversalOptions struct {
	MaxDepth      int     // maximum hop count from the starting node
	MaxResults    int     // maximum number of results to return
	MinConfidence float64 // minimum edge confidence to follow (0.0 to 1.0)
}
