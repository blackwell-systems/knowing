package types

import "context"

// GraphStore defines the operations the graph engine requires from its
// backing store. SQLite implements this today; an adjacency-list or
// external graph backend can implement it tomorrow without changing callers.
//
// The interface is organized into four groups:
//   - Write operations: PutNode, PutEdge, PutFile, PutRepo, RecordEdgeEvent, CreateSnapshot
//   - Point lookups: GetNode, GetEdge, GetSnapshot, GetRepo
//   - Query operations: NodesByName, EdgesFrom, EdgesTo, DanglingEdges, etc.
//   - Graph traversals: TransitiveCallers, TransitiveCallees, BlastRadius, SnapshotDiff
//
// All methods accept a context for cancellation and timeout propagation.
// Methods that return a pointer return nil (not an error) when the entity
// is not found.
type GraphStore interface {
	// Write operations (upsert semantics via INSERT OR REPLACE).
	PutNode(ctx context.Context, n Node) error
	PutEdge(ctx context.Context, e Edge) error
	PutFile(ctx context.Context, f File) error
	PutRepo(ctx context.Context, r Repo) error
	RecordEdgeEvent(ctx context.Context, ev EdgeEvent) error
	CreateSnapshot(ctx context.Context, s Snapshot) error

	// Point lookups by hash. Return nil when not found (no error).
	GetNode(ctx context.Context, hash Hash) (*Node, error)
	GetEdge(ctx context.Context, hash Hash) (*Edge, error)
	GetSnapshot(ctx context.Context, hash Hash) (*Snapshot, error)
	GetRepo(ctx context.Context, hash Hash) (*Repo, error)

	// Query operations.
	NodesByName(ctx context.Context, qualifiedPrefix string) ([]Node, error)
	EdgesFrom(ctx context.Context, sourceHash Hash, edgeType string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetHash Hash, edgeType string) ([]Edge, error)
	DanglingEdges(ctx context.Context) ([]Edge, error)
	AllRepos(ctx context.Context) ([]Repo, error)
	NodesByQualifiedName(ctx context.Context, qualifiedName string) ([]Node, error)

	// Delete operations for incremental re-indexing and garbage collection.
	DeleteEdge(ctx context.Context, hash Hash) error
	DeleteNodesByFile(ctx context.Context, fileHash Hash) (int, error)
	DeleteEdgesBySourceFile(ctx context.Context, fileHash Hash) ([]Edge, error)
	EdgesBySourceFile(ctx context.Context, fileHash Hash) ([]Edge, error)
	DeleteSnapshot(ctx context.Context, hash Hash) error

	// Graph traversals (implemented as recursive CTEs in SQLite).
	TransitiveCallers(ctx context.Context, target Hash, maxDepth int, snapshot Hash) ([]CallerResult, error)
	TransitiveCallees(ctx context.Context, source Hash, maxDepth int, snapshot Hash) ([]CalleeResult, error)
	BlastRadius(ctx context.Context, target Hash, snapshot Hash) (*BlastRadiusResult, error)

	// Snapshot operations.
	SnapshotDiff(ctx context.Context, oldRoot, newRoot Hash) (*DiffResult, error)
	StaleEdges(ctx context.Context, snapshot Hash) ([]Edge, error)
	LatestSnapshot(ctx context.Context, repoHash Hash) (*Snapshot, error)

	// File queries.
	FilesByRepo(ctx context.Context, repoHash Hash) ([]File, error)
	FileByPath(ctx context.Context, repoHash Hash, path string) (*File, error)

	// Close releases the underlying database connection.
	Close() error
}

// Extractor produces nodes and edges from source files. The indexer
// maintains a registry of extractors and dispatches each file to the
// first extractor whose CanHandle returns true. Implementations include
// GoExtractor (full type resolution), GoTreeSitterExtractor (fast AST-only),
// and TreeSitterExtractor (Python via tree-sitter).
type Extractor interface {
	// Name returns a human-readable identifier for this extractor (e.g., "go", "go-treesitter").
	Name() string
	// CanHandle returns true if this extractor can process the file at the given path.
	// The path is relative to the repository root.
	CanHandle(path string) bool
	// Extract parses the file described by opts and returns extracted nodes and edges.
	// Returns an empty result (not an error) if no symbols are found.
	Extract(ctx context.Context, opts ExtractOptions) (*ExtractResult, error)
}

// ExtractOptions contains all inputs needed for a single file extraction run.
// The indexer populates these fields and passes them to the selected Extractor.
type ExtractOptions struct {
	RepoURL    string // the repo URL (or local path) as registered in the store
	RepoHash   Hash   // sha256(RepoURL)
	CommitHash string // git commit hash being indexed
	FilePath   string // file path relative to the repository root
	FileHash   Hash   // content-addressed file hash: sha256(repoHash || path || contentHash)
	Content    []byte // raw file contents
	ModuleRoot string // absolute path to the module/repo root on disk (for go.mod resolution)
	// ModuleToRepoURL maps Go module paths to stored repo URLs. This is
	// populated by the indexer from the repos table so extractors can
	// resolve cross-repo call targets to the correct stored repo URL
	// rather than using heuristic inference from the import path.
	// Example: "github.com/org/repo" -> "/Users/user/code/repo"
	ModuleToRepoURL map[string]string
}

// ExtractResult contains the nodes and edges produced by an extractor.
type ExtractResult struct {
	Nodes []Node
	Edges []Edge
}

// ComputationCache manages content-addressed derived computation results
// (e.g., cached blast radius queries). Results are keyed by a combination
// of query type, parameters, and snapshot root, and are automatically
// invalidated when the snapshot changes.
//
// Interface defined now; implementation is deferred.
type ComputationCache interface {
	// Get retrieves a cached result by its content-addressed hash.
	Get(ctx context.Context, resultHash Hash) (*DerivedResult, error)
	// GetByQuery retrieves a cached result by query type, parameter hash, and snapshot root.
	GetByQuery(ctx context.Context, queryType string, params Hash, snapshot Hash) (*DerivedResult, error)
	// Put stores a computed result.
	Put(ctx context.Context, result DerivedResult) error
	// Invalidate evicts cached results that depend on edges changed between
	// oldSnapshot and newSnapshot. Returns the number of evicted entries.
	Invalidate(ctx context.Context, oldSnapshot, newSnapshot Hash, diff DiffResult) (evicted int, err error)
}
