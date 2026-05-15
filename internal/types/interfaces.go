package types

import "context"

// GraphStore defines the operations the graph engine requires from its
// backing store. SQLite implements this today; an adjacency-list or
// external graph backend can implement it tomorrow without changing callers.
type GraphStore interface {
	PutNode(ctx context.Context, n Node) error
	PutEdge(ctx context.Context, e Edge) error
	PutFile(ctx context.Context, f File) error
	PutRepo(ctx context.Context, r Repo) error
	RecordEdgeEvent(ctx context.Context, ev EdgeEvent) error
	CreateSnapshot(ctx context.Context, s Snapshot) error
	GetNode(ctx context.Context, hash Hash) (*Node, error)
	GetEdge(ctx context.Context, hash Hash) (*Edge, error)
	GetSnapshot(ctx context.Context, hash Hash) (*Snapshot, error)
	GetRepo(ctx context.Context, hash Hash) (*Repo, error)
	NodesByName(ctx context.Context, qualifiedPrefix string) ([]Node, error)
	EdgesFrom(ctx context.Context, sourceHash Hash, edgeType string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetHash Hash, edgeType string) ([]Edge, error)
	TransitiveCallers(ctx context.Context, target Hash, maxDepth int, snapshot Hash) ([]CallerResult, error)
	TransitiveCallees(ctx context.Context, source Hash, maxDepth int, snapshot Hash) ([]CalleeResult, error)
	BlastRadius(ctx context.Context, target Hash, snapshot Hash) (*BlastRadiusResult, error)
	SnapshotDiff(ctx context.Context, oldRoot, newRoot Hash) (*DiffResult, error)
	StaleEdges(ctx context.Context, snapshot Hash) ([]Edge, error)
	LatestSnapshot(ctx context.Context, repoHash Hash) (*Snapshot, error)
	FilesByRepo(ctx context.Context, repoHash Hash) ([]File, error)
	FileByPath(ctx context.Context, repoHash Hash, path string) (*File, error)
	Close() error
}

// Extractor produces nodes and edges from source files.
type Extractor interface {
	Name() string
	CanHandle(path string) bool
	Extract(ctx context.Context, opts ExtractOptions) (*ExtractResult, error)
}

// ExtractOptions contains the inputs for an extraction run.
type ExtractOptions struct {
	RepoURL    string
	RepoHash   Hash
	CommitHash string
	FilePath   string
	FileHash   Hash
	Content    []byte
	ModuleRoot string
}

// ExtractResult contains the nodes and edges produced by an extractor.
type ExtractResult struct {
	Nodes []Node
	Edges []Edge
}

// ComputationCache manages content-addressed derived results.
// Interface defined now; implementation deferred per requirements.
type ComputationCache interface {
	Get(ctx context.Context, resultHash Hash) (*DerivedResult, error)
	GetByQuery(ctx context.Context, queryType string, params Hash, snapshot Hash) (*DerivedResult, error)
	Put(ctx context.Context, result DerivedResult) error
	Invalidate(ctx context.Context, oldSnapshot, newSnapshot Hash, diff DiffResult) (evicted int, err error)
}
