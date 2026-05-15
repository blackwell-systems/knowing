# FEATURES.md -- Comprehensive Feature Dump for AI Reference

Generated: 2026-05-15
Source: code inspection of all 39 Go files, 12,203 LOC, 123 tests passing
Repo: github.com/blackwell-systems/knowing

---

## Implemented Features

### 1. Content-Addressed Graph Store (SQLite)

- **Package(s):** `internal/store`, `internal/types`
- **Entry point:** `store.NewSQLiteStore(dbPath string) (*SQLiteStore, error)`
- **What it does:** SQLite-backed implementation of the `GraphStore` interface. Uses WAL mode for concurrent read/write. Stores nodes, edges, files, repos, snapshots, and edge events. All graph operations go through the abstract interface.
- **Inputs:** Database file path. All write methods accept typed structs (Node, Edge, File, Repo, Snapshot, EdgeEvent).
- **Outputs:** Returns typed Go structs on reads. Errors on failure.
- **Limitations/known gaps:**
  - No `DeleteSnapshot` method (GarbageCollect in snapshot manager counts but cannot actually delete).
  - `TransitiveCallers` and `TransitiveCallees` ignore the `snapshot` parameter (queries all edges, not point-in-time).
  - `BlastRadius` hardcodes maxDepth=5 and Confidence=1.0 for all callers (does not compute minimum-confidence path).
  - No Pebble acceleration index (designed but not implemented; SQLite-only).
  - No `derived_results` table (ComputationCache interface exists but no implementation).
  - `schema_version` table uses simple max(version) tracking.
- **Dependencies:** `modernc.org/sqlite` (pure Go, no CGo).

### 2. Schema Migration Framework

- **Package(s):** `internal/store`
- **Entry point:** `store.Migrate(db *sql.DB) error`
- **What it does:** Reads embedded SQL migration files (`go:embed migrations/*.sql`), applies them in numeric order inside transactions, tracks applied versions in `schema_version` table.
- **Inputs:** Open `*sql.DB` handle.
- **Outputs:** Schema at latest version. Error if any migration fails (transaction rolled back).
- **Limitations/known gaps:** No rollback/down migrations. No migration locking for concurrent processes.
- **Dependencies:** None beyond stdlib.

### 3. Batch Insert Operations

- **Package(s):** `internal/store`
- **Entry point:** `SQLiteStore.BatchPutNodes`, `BatchPutEdges`, `BatchPutFiles`
- **What it does:** Inserts multiple records in a single transaction using prepared statements. Significantly faster than individual inserts for large index runs.
- **Inputs:** Slices of typed structs.
- **Outputs:** Error on failure; transaction is rolled back.
- **Limitations/known gaps:** Not part of the `GraphStore` interface; accessed via type assertion to `batchStore` interface in the indexer.
- **Dependencies:** GraphStore.

### 4. Go Tree-Sitter Extractor (Default, Fast Path)

- **Package(s):** `internal/indexer/gotsextractor`
- **Entry point:** `gotsextractor.NewGoTreeSitterExtractor() *GoTreeSitterExtractor`
- **What it does:** Parses `.go` files with tree-sitter Go grammar. Extracts declaration nodes (functions, methods, types, interfaces, consts, vars) and syntactic call/import edges. No type resolution. Produces edges with provenance `ast_inferred`, confidence 0.7. Stores call-site positions (line, col, file) on call edges for LSP enrichment.
- **Inputs:** `ExtractOptions` with file content, repo info, module root.
- **Outputs:** `ExtractResult` with sorted, deduplicated nodes and edges.
- **Limitations/known gaps:**
  - Cannot resolve interface satisfaction (no `implements` edges).
  - Cannot resolve non-call references (no `references` edges).
  - Cannot disambiguate overloaded names or aliased imports with certainty.
  - Cannot detect embedded type methods.
  - Method node hashes use kind="method" but the same method name across different types gets the same hash (no type in hash).
- **Dependencies:** `github.com/smacker/go-tree-sitter`, `go-tree-sitter/golang`.

### 5. Go Full Type Resolution Extractor (Legacy, `--full` Flag)

- **Package(s):** `internal/indexer/goextractor`
- **Entry point:** `goextractor.NewGoExtractor() *GoExtractor`
- **What it does:** Uses `go/packages` with full type resolution (`NeedTypes | NeedTypesInfo`). Produces edges with provenance `ast_resolved`, confidence 1.0. Extracts `calls`, `imports`, `implements`, and `references` edge types. Can extract from pre-loaded packages via `ExtractWithPackage`.
- **Inputs:** `ExtractOptions` (loads packages from `ModuleRoot`).
- **Outputs:** `ExtractResult` with sorted, deduplicated nodes and edges.
- **Limitations/known gaps:**
  - 16+ minute cold index time for large repos (type-checking transitive deps).
  - Per-file `packages.Load` is the bottleneck (not parallelizable from caller side).
  - Available via `knowing index --full` only.
- **Dependencies:** `golang.org/x/tools/go/packages`.

### 6. Bulk Package Loader

- **Package(s):** `internal/indexer/goextractor`
- **Entry point:** `goextractor.BulkLoad(ctx, dir string) (*LoadedPackages, error)`
- **What it does:** Calls `packages.Load("./...")` once to load all packages in a module. Returns a map of file path to pre-loaded `*packages.Package`. Used with `ExtractWithPackage` to avoid per-file Load overhead.
- **Inputs:** Context, directory path.
- **Outputs:** `LoadedPackages` map.
- **Limitations/known gaps:** Did not achieve the expected speedup (total type-checking work is the same; see implementation log). Currently unused in the default path (tree-sitter is default).
- **Dependencies:** `golang.org/x/tools/go/packages`.

### 7. Python Tree-Sitter Extractor

- **Package(s):** `internal/indexer/treesitter`
- **Entry point:** `treesitter.NewTreeSitterExtractor("python") (*TreeSitterExtractor, error)`
- **What it does:** Parses `.py` files with tree-sitter Python grammar. Extracts functions, classes (as type nodes), methods, import statements, and call expressions. Provenance `ast_resolved`, confidence 1.0.
- **Inputs:** `ExtractOptions` with Python file content.
- **Outputs:** `ExtractResult`.
- **Limitations/known gaps:**
  - Only Python is supported (other languages return error).
  - Import resolution is text-based (no Python module resolution).
  - Class context tracking is basic (single-level nesting only).
  - No call-site positions stored on edges.
- **Dependencies:** `github.com/smacker/go-tree-sitter`, `go-tree-sitter/python`.

### 8. Extractor Registry

- **Package(s):** `internal/indexer`
- **Entry point:** `indexer.NewExtractorRegistry() *ExtractorRegistry`
- **What it does:** Maintains ordered list of extractors. `FindExtractor(path)` returns the first extractor whose `CanHandle(path)` returns true.
- **Inputs:** File path string.
- **Outputs:** `types.Extractor` or nil.
- **Limitations/known gaps:** First-match semantics; no priority system.
- **Dependencies:** None.

### 9. Worker Pool (Parallel Extraction)

- **Package(s):** `internal/indexer`
- **Entry point:** `parallelExtract(ctx, work []extractWork, numWorkers int) []extractResult`
- **What it does:** Fan-out/fan-in worker pool. Distributes file extraction across `runtime.GOMAXPROCS` goroutines. Results stored in pre-sized array indexed by submission order (deterministic, no locks).
- **Inputs:** Slice of `extractWork` items, number of workers.
- **Outputs:** Slice of `extractResult` in submission order.
- **Limitations/known gaps:** Only parallelizes AST extraction, not type-checking. Currently not used in the default `IndexRepo` path (sequential extraction is used).
- **Dependencies:** None.

### 10. Repository Indexer

- **Package(s):** `internal/indexer`
- **Entry point:** `indexer.NewIndexer(store, snapshot).IndexRepo(ctx, repoURL, repoPath, commitHash string) (*Snapshot, error)`
- **What it does:** Walks repository directory, skips `.git`, `.claude`, `vendor`, `node_modules`, `testdata`. Computes content hashes per file. Skips unchanged files (incremental). For changed files: deletes old nodes/edges via `cleanupStore` interface, re-extracts, records edge events ("added"/"removed"). Batch inserts all results. Computes snapshot. Runs cross-repo resolver. Tracks changed file paths for downstream enrichment.
- **Inputs:** Repo URL, filesystem path, commit hash.
- **Outputs:** `*Snapshot` with node/edge counts.
- **Limitations/known gaps:**
  - File walker is sequential (not parallelized).
  - `changedFiles` parameter on `IndexFunc` in daemon config is accepted but not used to scope extraction (full walk always happens; skipping is by content hash comparison).
  - Edge event recording ignores errors (best-effort).
  - IndexerVer hardcoded to "v1".
- **Dependencies:** GraphStore, SnapshotComputer, ExtractorRegistry, Resolver.

### 11. LSP Enrichment

- **Package(s):** `internal/enrichment`
- **Entry point:** `enrichment.NewEnricher(store, workspaceRoot).Run(ctx, repoHash)` or `.RunScoped(ctx, repoHash, changedFiles)`
- **What it does:** Two-pass enrichment using gopls via `agent-lsp/pkg/lsp`:
  1. **Edge upgrade pass:** For each `ast_inferred` edge with call-site positions, queries `GetDefinition` at (file, line, col). If gopls confirms a definition, upgrades edge to `lsp_resolved` confidence 0.9.
  2. **Edge discovery pass:** For each file, gets document symbols. For types/interfaces, queries `GetImplementation`. For functions/methods, queries `GetReferences`. Creates new `lsp_resolved` edges.
- **Inputs:** Repo hash, workspace root, optional changed file list for scoping.
- **Outputs:** Logs summary (edges processed, upgraded, discovered, errors).
- **Limitations/known gaps:**
  - Discovered edges use synthetic position-based hashes (not aligned with node hashes from extractors).
  - `RunScoped` opens only changed files, which may reduce gopls cross-package resolution accuracy.
  - Errors in individual edge upgrades are counted but not surfaced.
  - No TypeScript/Rust/Python enrichment (gopls only).
- **Dependencies:** `github.com/blackwell-systems/agent-lsp/pkg/lsp`, GraphStore.

### 12. Snapshot Manager (Merkle DAG)

- **Package(s):** `internal/snapshot`
- **Entry point:** `snapshot.NewSnapshotManager(store).ComputeSnapshot(ctx, repoHash, commitHash) (*Snapshot, error)`
- **What it does:** Collects all edge hashes for a repo (via nodes from `NodesByName`), builds a binary Merkle tree from sorted hashes, chains the new snapshot to the latest existing snapshot, stores it.
- **Inputs:** Repo hash, commit hash.
- **Outputs:** `*Snapshot` with Merkle root, parent pointer, node/edge counts.
- **Limitations/known gaps:**
  - `GarbageCollect` counts snapshots to remove but cannot actually delete them (no `DeleteSnapshot` in GraphStore).
  - Synthetic file nodes not included in Merkle tree (only nodes with qualified names).
  - Merkle diff (`DiffMerkle`) is a set-diff on leaves, not a tree-walk optimization.
- **Dependencies:** GraphStore.

### 13. Merkle Tree

- **Package(s):** `internal/snapshot`
- **Entry point:** `snapshot.BuildMerkleTree(hashes []Hash) *MerkleTree`
- **What it does:** Sorts hashes lexicographically via `bytes.Compare`, builds binary Merkle tree (odd leaf paired with itself), returns root hash and sorted leaves.
- **Inputs:** Slice of `Hash` values.
- **Outputs:** `*MerkleTree` with Root and Leaves.
- **Limitations/known gaps:** Full reconstruction (no incremental update). `DiffMerkle` is O(n) set comparison, not O(log n) tree walk.
- **Dependencies:** None.

### 14. Cross-Repo Edge Resolver

- **Package(s):** `internal/resolver`
- **Entry point:** `resolver.NewResolver(store).Resolve(ctx) (*ResolveStats, error)`
- **What it does:** Finds dangling edges (target hash points to no existing node). For each, recomputes what the target hash would be under every other repo's URL. If a match is found, deletes the old edge and creates a new one with the correct target. Fixes the extractor's tendency to compute target hashes with the calling repo's URL instead of the target repo's URL.
- **Inputs:** None (reads all dangling edges and all repos from store).
- **Outputs:** `ResolveStats` with counts of retargeted, skipped, errors.
- **Limitations/known gaps:**
  - O(nodes * repos) hash computation for reverse lookup table.
  - Only fixes repo URL mismatches; cannot resolve module path vs filesystem path mismatches (that was fixed in the extractor).
  - `ResolveWithDetails` returns per-edge results for debugging.
- **Dependencies:** GraphStore (via `Store` subset interface).

### 15. Daemon

- **Package(s):** `internal/daemon`
- **Entry point:** `daemon.NewDaemon(cfg DaemonConfig).Start(ctx) error`
- **What it does:** Long-lived process with three goroutines:
  1. `watchLoop`: reads `CommitEvent` from `GitWatcher`, enqueues index requests.
  2. `indexWorker`: processes index requests sequentially with write lock, triggers background enrichment on success.
  3. MCP server (if configured): serves HTTP.
  Provides `WatchRepo`/`UnwatchRepo` for dynamic repo management. `RLock`/`RUnlock` for concurrent read access.
- **Inputs:** `DaemonConfig` with Store, IndexFunc, EnrichFunc, MCPAddr, MCPServer.
- **Outputs:** Blocks until context cancellation. Returns error on startup failure.
- **Limitations/known gaps:**
  - Only HTTP MCP transport in daemon mode (stdio available via `ServeStdio` but not wired in daemon).
  - Index queue is buffered (128); overflows are silently dropped.
  - No post-commit hook installation (architecture describes it but only fsnotify-based GitWatcher is implemented).
  - No polling fallback (architecture describes it but not implemented).
  - `repos` map defaults URL to path if not explicitly set.
- **Dependencies:** GitWatcher, MCPServer interface, IndexFunc callback, EnrichFunc callback.

### 16. Git-Based Change Detection (GitWatcher)

- **Package(s):** `internal/daemon`
- **Entry point:** `daemon.NewGitWatcher(debounce time.Duration) (*GitWatcher, error)`
- **What it does:** Watches `.git/HEAD` and `.git/refs/heads/*` via fsnotify (1-2 file descriptors per repo). On change, debounces, reads new HEAD commit hash, compares to stored value. If different, calls `GitDiffFiles` to resolve changed/added/deleted files, emits `CommitEvent` on channel.
- **Inputs:** Debounce duration, repo paths via `Add`.
- **Outputs:** `CommitEvent` channel with repo path, old/new commit, changed/added/deleted file lists.
- **Limitations/known gaps:**
  - Non-blocking event send (drops events if consumer is slow).
  - No post-commit hook support (fsnotify only).
  - No polling fallback.
  - `repoForPath` does linear scan of all tracked repos.
- **Dependencies:** `github.com/fsnotify/fsnotify`.

### 17. Git Diff Resolution

- **Package(s):** `internal/daemon`
- **Entry point:** `daemon.GitDiffFiles(repoPath, oldCommit, newCommit string) (changed, added, deleted []string, err error)`
- **What it does:** Shells out to `git diff --name-status oldCommit newCommit`. Parses M/A/D/R status codes. Renames treated as delete+add. If oldCommit is empty (initial index), uses `git ls-files` to list all tracked files.
- **Inputs:** Repo path, old commit hash, new commit hash.
- **Outputs:** Three slices of relative file paths (changed, added, deleted).
- **Limitations/known gaps:** Requires `git` binary on PATH. Rename handling splits into two entries (no rename tracking).
- **Dependencies:** `git` CLI.

### 18. Git HEAD Reader

- **Package(s):** `internal/daemon`
- **Entry point:** `daemon.GitHeadCommit(repoPath string) (string, error)`
- **What it does:** Reads `.git/HEAD` directly (no git binary). Resolves symbolic refs by reading loose ref files, falls back to `packed-refs`.
- **Inputs:** Repository path.
- **Outputs:** 40-character commit hash string.
- **Limitations/known gaps:** Does not handle worktrees (assumes `.git` is a directory, not a file).
- **Dependencies:** None (pure file I/O).

### 19. MCP Server

- **Package(s):** `internal/mcp`
- **Entry point:** `mcp.NewServer(store GraphStore) *Server`
- **What it does:** Wraps `mcp-go` server library. Registers 11 tools. Supports stdio and HTTP transports. Tool definitions include parameter schemas with descriptions and required flags.
- **Inputs:** GraphStore.
- **Outputs:** Serves MCP protocol over stdio or HTTP.
- **Limitations/known gaps:**
  - `semantic_diff` and `pr_impact` are thin wrappers over `SnapshotDiff` (not full semantic analysis as described in architecture).
  - `trace_dataflow` maps to `TransitiveCallees` (no actual data flow tracing).
  - `ownership` lists files and symbols (no CODEOWNERS integration, no team mapping).
  - `index_repo` uses a package-level `indexFunc` variable set via `SetIndexFunc` (not ideal).
- **Dependencies:** `github.com/mark3labs/mcp-go`, GraphStore.

### 20. CLI (`knowing` Binary)

- **Package(s):** `cmd/knowing`
- **Entry point:** `main.main() -> run(args)`
- **What it does:** Dispatches to subcommands: `serve`, `index`, `query`, `version`. Wires together all internal packages.
- **Inputs:** CLI arguments and flags.
- **Outputs:** Stdout text, SQLite database file.
- **Dependencies:** All internal packages.

### 21. Edge Event Recording

- **Package(s):** `internal/indexer` (writes), `internal/store` (stores)
- **Entry point:** Called automatically during `IndexRepo` after cleanup and re-extraction.
- **What it does:** For each index run with changed files: computes diff between old edges (deleted before re-extraction) and new edges (from fresh extraction). Records "added" events for truly new edges and "removed" events for truly deleted edges. Events are keyed by snapshot hash.
- **Inputs:** Old edges (from cleanup), new edges (from extraction), snapshot hash.
- **Outputs:** Rows in `edge_events` table.
- **Limitations/known gaps:** Errors in event recording are silently ignored. Events only recorded when `cleanupStore` interface is available.
- **Dependencies:** GraphStore.RecordEdgeEvent.

---

## Interfaces

### GraphStore (`internal/types/interfaces.go`)

All 27 methods:

| Method | Signature | Implementors | Consumers |
|--------|-----------|-------------|-----------|
| PutNode | `(ctx, Node) error` | SQLiteStore | indexer |
| PutEdge | `(ctx, Edge) error` | SQLiteStore | indexer, enricher, resolver |
| PutFile | `(ctx, File) error` | SQLiteStore | indexer |
| PutRepo | `(ctx, Repo) error` | SQLiteStore | indexer |
| RecordEdgeEvent | `(ctx, EdgeEvent) error` | SQLiteStore | indexer |
| CreateSnapshot | `(ctx, Snapshot) error` | SQLiteStore | snapshot manager |
| GetNode | `(ctx, Hash) (*Node, error)` | SQLiteStore | mcp, resolver, snapshot manager |
| GetEdge | `(ctx, Hash) (*Edge, error)` | SQLiteStore | enricher |
| GetSnapshot | `(ctx, Hash) (*Snapshot, error)` | SQLiteStore | snapshot manager |
| GetRepo | `(ctx, Hash) (*Repo, error)` | SQLiteStore | mcp, enricher |
| NodesByName | `(ctx, prefix string) ([]Node, error)` | SQLiteStore | mcp, enricher, snapshot manager, resolver |
| EdgesFrom | `(ctx, Hash, edgeType string) ([]Edge, error)` | SQLiteStore | enricher, snapshot manager |
| EdgesTo | `(ctx, Hash, edgeType string) ([]Edge, error)` | SQLiteStore | (available but unused in current code) |
| DanglingEdges | `(ctx) ([]Edge, error)` | SQLiteStore | resolver |
| AllRepos | `(ctx) ([]Repo, error)` | SQLiteStore | indexer, resolver |
| NodesByQualifiedName | `(ctx, name string) ([]Node, error)` | SQLiteStore | (available but unused in current code) |
| DeleteEdge | `(ctx, Hash) error` | SQLiteStore | enricher, resolver |
| DeleteNodesByFile | `(ctx, Hash) (int, error)` | SQLiteStore | indexer (via cleanupStore) |
| DeleteEdgesBySourceFile | `(ctx, Hash) ([]Edge, error)` | SQLiteStore | indexer (via cleanupStore) |
| EdgesBySourceFile | `(ctx, Hash) ([]Edge, error)` | SQLiteStore | indexer (via cleanupStore) |
| TransitiveCallers | `(ctx, Hash, int, Hash) ([]CallerResult, error)` | SQLiteStore | mcp |
| TransitiveCallees | `(ctx, Hash, int, Hash) ([]CalleeResult, error)` | SQLiteStore | mcp |
| BlastRadius | `(ctx, Hash, Hash) (*BlastRadiusResult, error)` | SQLiteStore | mcp |
| SnapshotDiff | `(ctx, Hash, Hash) (*DiffResult, error)` | SQLiteStore | mcp, snapshot manager |
| StaleEdges | `(ctx, Hash) ([]Edge, error)` | SQLiteStore | mcp |
| LatestSnapshot | `(ctx, Hash) (*Snapshot, error)` | SQLiteStore | enricher, snapshot manager |
| FilesByRepo | `(ctx, Hash) ([]File, error)` | SQLiteStore | indexer, enricher, mcp |
| FileByPath | `(ctx, Hash, string) (*File, error)` | SQLiteStore | indexer |
| Close | `() error` | SQLiteStore | cmd/knowing |

### Extractor (`internal/types/interfaces.go`)

| Method | Signature |
|--------|-----------|
| Name | `() string` |
| CanHandle | `(path string) bool` |
| Extract | `(ctx, ExtractOptions) (*ExtractResult, error)` |

Implementors: `gotsextractor.GoTreeSitterExtractor`, `goextractor.GoExtractor`, `treesitter.TreeSitterExtractor`
Consumers: `indexer.ExtractorRegistry`, `indexer.Indexer`

### ComputationCache (`internal/types/interfaces.go`)

| Method | Signature |
|--------|-----------|
| Get | `(ctx, Hash) (*DerivedResult, error)` |
| GetByQuery | `(ctx, string, Hash, Hash) (*DerivedResult, error)` |
| Put | `(ctx, DerivedResult) error` |
| Invalidate | `(ctx, Hash, Hash, DiffResult) (int, error)` |

Implementors: **NONE. Interface defined, no implementation exists.**
Consumers: **NONE.**

### SnapshotComputer (`internal/indexer/indexer.go`)

| Method | Signature |
|--------|-----------|
| ComputeSnapshot | `(ctx, Hash, string) (*Snapshot, error)` |

Implementors: `snapshot.SnapshotManager`
Consumers: `indexer.Indexer`

### MCPServer (`internal/daemon/daemon.go`)

| Method | Signature |
|--------|-----------|
| ServeStdio | `(ctx) error` |
| ServeHTTP | `(ctx, string) error` |

Implementors: `mcp.Server`
Consumers: `daemon.Daemon`

### batchStore (`internal/indexer/indexer.go`, unexported)

| Method | Signature |
|--------|-----------|
| BatchPutNodes | `(ctx, []Node) error` |
| BatchPutEdges | `(ctx, []Edge) error` |
| BatchPutFiles | `(ctx, []File) error` |

Implementors: `store.SQLiteStore`
Consumers: `indexer.Indexer` (via type assertion)

### cleanupStore (`internal/indexer/indexer.go`, unexported)

| Method | Signature |
|--------|-----------|
| DeleteNodesByFile | `(ctx, Hash) (int, error)` |
| DeleteEdgesBySourceFile | `(ctx, Hash) ([]Edge, error)` |
| EdgesBySourceFile | `(ctx, Hash) ([]Edge, error)` |

Implementors: `store.SQLiteStore` (via GraphStore)
Consumers: `indexer.Indexer` (via type assertion)

### resolver.Store (`internal/resolver/resolver.go`)

Subset interface of GraphStore for resolver decoupling:

| Method | Consumers |
|--------|-----------|
| DanglingEdges | resolver |
| AllRepos | resolver |
| GetNode | resolver |
| NodesByName | resolver |
| DeleteEdge | resolver |
| PutEdge | resolver |

---

## MCP Tools

| # | Tool Name | Handler | Status | GraphStore Methods Called | Description |
|---|-----------|---------|--------|-------------------------|-------------|
| 1 | `index_repo` | `handleIndexRepo` | FUNCTIONAL | Delegates to `indexFunc` (package-level var) | Indexes a repo via configured index function |
| 2 | `cross_repo_callers` | `handleCrossRepoCallers` | FUNCTIONAL | `TransitiveCallers` | Finds all transitive callers of a symbol |
| 3 | `graph_query` | `handleGraphQuery` | FUNCTIONAL | `NodesByName` | Searches nodes by qualified name prefix |
| 4 | `repo_graph` | `handleRepoGraph` | FUNCTIONAL | `FilesByRepo` | Returns all files for a repo |
| 5 | `blast_radius` | `handleBlastRadius` | FUNCTIONAL | `BlastRadius` (which calls `TransitiveCallers`, `GetNode`, repo lookup) | Full impact analysis for a symbol |
| 6 | `trace_dataflow` | `handleTraceDataflow` | FUNCTIONAL (limited) | `TransitiveCallees` | Traces outbound call edges (not true data flow) |
| 7 | `stale_edges` | `handleStaleEdges` | FUNCTIONAL | `StaleEdges` | Finds edges whose source file content hash changed |
| 8 | `snapshot_diff` | `handleSnapshotDiff` | FUNCTIONAL | `SnapshotDiff` | Structural diff between two snapshots via edge_events |
| 9 | `semantic_diff` | `handleSemanticDiff` | THIN WRAPPER | `SnapshotDiff` | Same as snapshot_diff with summary string added |
| 10 | `pr_impact` | `handlePRImpact` | THIN WRAPPER | `SnapshotDiff`, `BlastRadius` | Computes blast radius of removed nodes between snapshots |
| 11 | `ownership` | `handleOwnership` | FUNCTIONAL (limited) | `GetRepo`, `NodesByName`, `FilesByRepo` | Lists files and symbols per repo (no CODEOWNERS) |

Parameters per tool:

- `index_repo`: repo_url (required), repo_path (required), commit_hash (optional)
- `cross_repo_callers`: target_hash (required), max_depth (optional, default 5)
- `graph_query`: prefix (required)
- `repo_graph`: repo_hash (required)
- `blast_radius`: target_hash (required), snapshot_hash (optional)
- `trace_dataflow`: source_hash (required), max_depth (optional, default 5)
- `stale_edges`: snapshot_hash (required)
- `snapshot_diff`: old_snapshot (required), new_snapshot (required)
- `semantic_diff`: old_snapshot (required), new_snapshot (required)
- `pr_impact`: old_snapshot (required), new_snapshot (required)
- `ownership`: repo_hash (required)

---

## CLI Commands

### `knowing serve`

- **Flags:** `--db` (default: `knowing.db`), `--addr` (default: `:8080`)
- **Positional args:** repo paths to watch (0 or more)
- **What it does:** Opens SQLite store, creates snapshot manager and indexer, registers Go tree-sitter and Python extractors, creates MCP server, creates daemon with GitWatcher, watches listed repos, blocks until SIGINT/SIGTERM.
- **Extractors registered:** `gotsextractor` (Go, tree-sitter), `treesitter` (Python).
- **Enrichment:** Background EnrichFunc wired with scoped or full enrichment.

### `knowing index`

- **Flags:** `--db` (default: `knowing.db`), `--url` (default: repo path), `--commit` (default: HEAD), `--full` (use go/packages instead of tree-sitter)
- **Positional args:** repo-path (required)
- **What it does:** Opens store, creates indexer, registers extractor (tree-sitter by default, go/packages if `--full`), indexes repo, prints stats, runs LSP enrichment synchronously (if not `--full`).

### `knowing query`

- **Flags:** `--db` (default: `knowing.db`)
- **Positional args:** symbol-prefix (required)
- **What it does:** Opens store, queries `NodesByName` with prefix, prints nodes with their outbound edges.

### `knowing version`

- **No flags.**
- **What it does:** Prints `knowing v0.1.0`.

---

## Storage Schema

### Table: `repos`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| repo_hash | BLOB PK | sha256(repo_url) | GetRepo, AllRepos, snapshot, mcp | PutRepo |
| repo_url | TEXT NOT NULL | URL or filesystem path | AllRepos, NodesByName prefix matching | PutRepo |
| last_commit | TEXT | Latest indexed commit hash | (stored, not currently queried) | PutRepo |
| last_indexed | INTEGER | Unix timestamp | (stored, not currently queried) | PutRepo |

### Table: `files`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| file_hash | BLOB PK | sha256(repo_hash + path + content_hash) | FilesByRepo, FileByPath, node lookups | PutFile, BatchPutFiles |
| repo_hash | BLOB FK | References repos | FilesByRepo | PutFile |
| path | TEXT NOT NULL | Relative path within repo | FileByPath | PutFile |
| content_hash | BLOB NOT NULL | sha256(file_contents) | StaleEdges, incremental skip | PutFile |

### Table: `nodes`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| node_hash | BLOB PK | sha256(repo + pkg + name + kind) | GetNode, TransitiveCallers/Callees, BlastRadius | PutNode, BatchPutNodes |
| file_hash | BLOB FK | References files | DeleteNodesByFile, BlastRadius repo lookup | PutNode |
| qualified_name | TEXT NOT NULL | `{repo}://{pkg}.{Type}.{Name}` | NodesByName (LIKE prefix%), NodesByQualifiedName (exact) | PutNode |
| kind | TEXT NOT NULL | function, type, method, interface, const, var | Display, resolver | PutNode |
| line | INTEGER | 1-indexed source line | Display | PutNode |
| signature | TEXT | Human-readable type signature | Display | PutNode |

### Table: `edges`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| edge_hash | BLOB PK | sha256(source + target + type + provenance) | GetEdge, DanglingEdges | PutEdge, BatchPutEdges |
| source_hash | BLOB FK | References nodes | EdgesFrom, TransitiveCallers/Callees, DeleteEdgesBySourceFile | PutEdge |
| target_hash | BLOB FK | References nodes | EdgesTo, DanglingEdges | PutEdge |
| edge_type | TEXT NOT NULL | calls, imports, implements, references | EdgesFrom/To filter, traversal CTEs | PutEdge |
| confidence | REAL DEFAULT 1.0 | 0.0-1.0 | Display | PutEdge |
| provenance | TEXT DEFAULT 'ast_resolved' | ast_resolved, ast_inferred, lsp_resolved | Enricher filter, display | PutEdge |
| callsite_line | INTEGER DEFAULT 0 | 1-indexed line of call expression | Enricher (GetDefinition position) | PutEdge (tree-sitter extractor) |
| callsite_col | INTEGER DEFAULT 0 | 0-indexed column of call expression | Enricher | PutEdge |
| callsite_file | TEXT DEFAULT '' | Relative file path | Enricher | PutEdge |

### Table: `edge_events`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| event_id | INTEGER PK AUTOINCREMENT | Auto-incrementing ID | -- | RecordEdgeEvent |
| edge_hash | BLOB NOT NULL | Which edge | SnapshotDiff JOIN | RecordEdgeEvent |
| event_type | TEXT NOT NULL | "added" or "removed" | SnapshotDiff filter | RecordEdgeEvent |
| snapshot_hash | BLOB NOT NULL | Which snapshot recorded this | SnapshotDiff filter | RecordEdgeEvent |
| source_commit | TEXT NOT NULL | Git commit that caused this | (stored, queryable) | RecordEdgeEvent |
| indexer_ver | TEXT NOT NULL | Indexer version | (stored, queryable) | RecordEdgeEvent |
| timestamp | INTEGER NOT NULL | Unix timestamp | (stored, queryable) | RecordEdgeEvent |

### Table: `snapshots`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| snapshot_hash | BLOB PK | Merkle root of sorted edge hashes | GetSnapshot, chain walking | CreateSnapshot |
| parent_hash | BLOB FK | Previous snapshot | Chain walking, GC | CreateSnapshot |
| repo_hash | BLOB FK | Which repo | LatestSnapshot | CreateSnapshot |
| commit_hash | TEXT NOT NULL | Git commit | Display | CreateSnapshot |
| timestamp | INTEGER NOT NULL | Unix timestamp | LatestSnapshot ORDER BY | CreateSnapshot |
| node_count | INTEGER NOT NULL | Count at snapshot time | Display | CreateSnapshot |
| edge_count | INTEGER NOT NULL | Count at snapshot time | Display | CreateSnapshot |

### Table: `schema_version`
| Column | Type | Description |
|--------|------|-------------|
| version | INTEGER PK | Highest applied migration number |

### Indexes
| Name | Table | Column(s) | Purpose |
|------|-------|-----------|---------|
| idx_nodes_qualified | nodes | qualified_name | NodesByName LIKE queries |
| idx_nodes_file | nodes | file_hash | DeleteNodesByFile, join lookups |
| idx_edges_source | edges | source_hash | EdgesFrom, DeleteEdgesBySourceFile |
| idx_edges_target | edges | target_hash | EdgesTo, DanglingEdges, TransitiveCallers |
| idx_edges_type | edges | edge_type | Edge type filtering |
| idx_edge_events_snapshot | edge_events | snapshot_hash | SnapshotDiff queries |
| idx_files_repo | files | repo_hash | FilesByRepo |

### Migrations
| # | File | What it does |
|---|------|-------------|
| 001 | 001_initial_schema.sql | Creates all 6 tables + 7 indexes |
| 002 | 002_add_dangling_edge_support.sql | No-op (idx_edges_target from 001 already covers dangling queries) |
| 003 | 003_add_callsite_columns.sql | Adds callsite_line, callsite_col, callsite_file to edges table |

---

## Edge Types

### Implemented in Code

| Edge Type | Producer(s) | Provenance | Confidence | Notes |
|-----------|------------|------------|------------|-------|
| `calls` | gotsextractor (tree-sitter) | `ast_inferred` | 0.7 | Syntactic string matching. Call-site positions stored. |
| `calls` | goextractor (go/packages) | `ast_resolved` | 1.0 | Full type resolution. No call-site positions. |
| `calls` | enricher (LSP upgrade) | `lsp_resolved` | 0.9 | Upgraded from ast_inferred via GetDefinition confirmation. |
| `calls` | treesitter (Python) | `ast_resolved` | 1.0 | Syntactic matching (no type resolver for Python). |
| `imports` | gotsextractor | `ast_inferred` | 0.7 | File-level node to package node. |
| `imports` | goextractor | `ast_resolved` | 1.0 | File-level node to package node. |
| `imports` | treesitter (Python) | `ast_resolved` | 1.0 | Module-level imports. |
| `implements` | goextractor | `ast_resolved` | 1.0 | Concrete type -> interface (via `types.Implements`). |
| `implements` | enricher (LSP discovery) | `lsp_resolved` | 0.9 | Discovered via GetImplementation on types/interfaces. |
| `references` | goextractor | `ast_resolved` | 1.0 | Non-call identifier usages (via TypesInfo.Uses). |
| `references` | enricher (LSP discovery) | `lsp_resolved` | 0.9 | Discovered via GetReferences on functions/methods. |

### Defined in Architecture but NOT Produced by Any Code

| Edge Type | Category | Status |
|-----------|----------|--------|
| `rpc_calls` | Protocol | NOT IMPLEMENTED |
| `produces_event` | Protocol | NOT IMPLEMENTED |
| `consumes_event` | Protocol | NOT IMPLEMENTED |
| `reads_field` | Schema | NOT IMPLEMENTED |
| `writes_field` | Schema | NOT IMPLEMENTED |
| `declares_route` | Schema | NOT IMPLEMENTED |
| `consumes_route` | Schema | NOT IMPLEMENTED |
| `deploys` | Infrastructure | NOT IMPLEMENTED |
| `connects_to` | Infrastructure | NOT IMPLEMENTED |
| `depends_on_service` | Infrastructure | NOT IMPLEMENTED |
| `owned_by_team` | Ownership | NOT IMPLEMENTED |
| `owned_by_user` | Ownership | NOT IMPLEMENTED |
| `runtime_calls` | Runtime | NOT IMPLEMENTED |
| `runtime_rpc` | Runtime | NOT IMPLEMENTED |
| `runtime_produces` | Runtime | NOT IMPLEMENTED |
| `runtime_consumes` | Runtime | NOT IMPLEMENTED |
| `runtime_queries` | Runtime | NOT IMPLEMENTED |

---

## Node Kinds

| Kind | What it represents | Produced by |
|------|--------------------|-------------|
| `function` | Package-level function | gotsextractor, goextractor, treesitter |
| `method` | Method on a type | gotsextractor, goextractor |
| `type` | Named type (struct, alias, etc.) | gotsextractor, goextractor, treesitter (Python classes) |
| `interface` | Interface type | gotsextractor, goextractor |
| `const` | Constant declaration | gotsextractor, goextractor |
| `var` | Variable declaration | gotsextractor, goextractor |
| `file` | Synthetic file-level node (for import edges) | gotsextractor, goextractor (implicit, not stored as a separate node) |
| `package` | Synthetic package node (import target) | gotsextractor, goextractor (implicit, not stored) |
| `module` | Python module (import target) | treesitter |

---

## Configuration

### DaemonConfig (`internal/daemon/daemon.go`)

| Field | Type | Purpose |
|-------|------|---------|
| Store | `types.GraphStore` | Backing store for all graph operations |
| IndexFunc | `func(ctx, repoURL, repoPath, commitHash string, changedFiles []string) error` | Callback to index a repo |
| MCPAddr | `string` | HTTP listen address for MCP server |
| MCPServer | `MCPServer` interface | MCP server implementation |
| EnrichFunc | `func(ctx, repoHash Hash, workspaceRoot string, changedFiles []string) error` | Background LSP enrichment callback |

### Extractor Registration

Configured in `cmd/knowing/main.go`. Default mode (`serve` and `index` without `--full`):
- `gotsextractor.NewGoTreeSitterExtractor()` (Go files)
- `treesitter.NewTreeSitterExtractor("python")` (Python files)

Full mode (`index --full`):
- `goextractor.NewGoExtractor()` (Go files)
- `treesitter.NewTreeSitterExtractor("python")` (Python files)

### File Walker Skip Directories

Hardcoded in `indexer.IndexRepo`: `.git`, `.claude`, `vendor`, `node_modules`, `testdata`.

### GitWatcher Debounce

Hardcoded in `cmd/knowing/main.go` via daemon startup: `500 * time.Millisecond`.

---

## Planned but NOT Implemented

### Pebble Acceleration Index
- **Architecture decision #9.** Adjacency-list store for fast graph traversal.
- **What's needed:** `PebbleStore` implementing same traversal methods, `HybridStore` routing queries between SQLite and Pebble, rebuild-from-SQLite mechanism.
- **Trigger:** p95 blast radius latency > 1s after caching, or edge count > 1M.

### Content-Addressed Computation Cache
- **Architecture decision #12.** L1 in-memory LRU, L2 materialized results in SQLite, L3 bounded traversal.
- **Interface defined:** `ComputationCache` in `types/interfaces.go`. `DerivedResult`, `TraversalOptions` types exist.
- **What's needed:** Implementation of `ComputationCache`, `derived_results` table, cache invalidation via Merkle diff, L1 LRU with snapshot-aware eviction.
- **No code exists.** The `derived_results` table is not in any migration.

### SCIP Ingest (External Dependency Indexing)
- **Roadmap: Edge Types workstream.**
- **What's needed:** SCIP index parser, shallow public API surface extraction, integration with graph store.
- **No code exists.**

### Protobuf/gRPC Edge Extraction
- **Roadmap: Edge Types workstream.**
- **What's needed:** Proto file parser, `rpc_calls` edge type, service-to-service mapping.
- **No code exists.**

### HTTP Route Edge Extraction
- **Roadmap: Edge Types workstream.**
- **What's needed:** Route registration parser (e.g., `router.POST("/path", handler)`), route-to-symbol mapping table, `declares_route`/`consumes_route` edges.
- **No code exists.** Architecture notes this mapping is critical for runtime trace ingestion.

### Event/Message Queue Edge Extraction
- **Roadmap: Edge Types workstream.**
- **What's needed:** Kafka/NATS/SQS topic producer/consumer detection, `produces_event`/`consumes_event` edges.
- **No code exists.**

### Schema Edge Extraction (OpenAPI, JSON Schema)
- **Roadmap: Edge Types workstream.**
- **What's needed:** OpenAPI spec parser, `reads_field`/`writes_field` edges.
- **No code exists.**

### Infrastructure Edge Extraction (Terraform, K8s)
- **Roadmap: Edge Types workstream.**
- **What's needed:** Terraform HCL parser, K8s manifest parser, `deploys`/`connects_to`/`depends_on_service` edges.
- **No code exists.**

### Ownership Edge Extraction (CODEOWNERS)
- **Roadmap: Edge Types workstream.**
- **What's needed:** CODEOWNERS parser, service catalog ingestion, `owned_by_team`/`owned_by_user` edges.
- **No code exists.** The `ownership` MCP tool exists but only lists files/symbols (no team mapping).

### Runtime Trace Ingestion
- **Architecture decision #13. Roadmap: Runtime Intelligence workstream.**
- **What's needed:** `TraceIngestor` interface implementation, OTel span normalization, symbol resolution (route path to graph node), `runtime_*` edge types with observation-based confidence, confidence decay logic.
- **Interface defined in architecture doc but NOT in code.** No `TraceIngestor` interface in `types/interfaces.go`.
- **No code exists.**

### Semantic PR Diff (Full)
- **Architecture decision #14. Roadmap: Developer Visibility workstream.**
- **Current state:** `semantic_diff` and `pr_impact` MCP tools exist but are thin wrappers over `SnapshotDiff`.
- **What's needed:** Blast radius delta computation, ownership impact analysis, cross-repo edge classification, PR comment formatting, GitHub Action.
- **Partially implemented.** `pr_impact` computes blast radius of removed nodes but lacks ownership delta, staleness annotations, and the full output format described in architecture.

### Graph-Native Test Selection
- **Roadmap: Developer Visibility workstream.**
- **What's needed:** Test file detection, graph traversal from changed symbols to test functions, `knowing test-scope` CLI command.
- **No code exists.**

### Ownership Routing
- **Roadmap: Developer Visibility workstream.**
- **What's needed:** Ownership edges (from CODEOWNERS), "who to notify" computation from graph edges.
- **No code exists.**

### Staleness Dashboard
- **Roadmap: Developer Visibility workstream.**
- **What's needed:** Subgraph freshness visualization, web UI or report generation.
- **No code exists.** `stale_edges` MCP tool exists for data.

### Pending Mutations (Agent Coordination)
- **Roadmap: Agent Coordination workstream.**
- **What's needed:** In-flight change announcements, proposed state overlay.
- **No code exists.**

### Temporal Reasoning
- **Roadmap: Agent Coordination workstream.**
- **What's needed:** Snapshot chain backward walk to find when incompatibilities appeared.
- **No code exists.** Snapshot chain and SnapshotDiff exist as building blocks.

### Federated Graphs
- **Roadmap: Agent Coordination workstream.**
- **What's needed:** Cross-instance Merkle diff exchange, instance ownership registry.
- **No code exists.**

### Lamport Timestamps (Causal Ordering)
- **Architecture decision #6.**
- **Current state:** Uses git commit timestamps. Lamport clocks described but not implemented.
- **What's needed:** Monotonic counter per repo, counter propagation on cross-repo re-index triggers.

### Post-Commit Hook Installation
- **Architecture describes as primary change detection method.**
- **What's needed:** Hook script generation, unix socket communication, daemon hook receiver.
- **No code exists.** Only fsnotify-based GitWatcher implemented.

### Polling Fallback
- **Architecture describes as last-resort change detection.**
- **What's needed:** Periodic `git rev-parse HEAD` comparison.
- **No code exists.**

### TypeScript/Rust/Java Tree-Sitter Extractors
- **Architecture lists as planned.**
- **What's needed:** Tree-sitter grammar binding, extractor implementing Extractor interface.
- **No code exists.** Only Go (tree-sitter + go/packages) and Python (tree-sitter) extractors.

### CI Integration (GitHub Action)
- **Architecture decision #14.**
- **What's needed:** `blackwell-systems/knowing-action`, workflow YAML, PR comment posting.
- **No code exists.**

### `--working-tree` Flag
- **Architecture describes for indexing uncommitted changes.**
- **What's needed:** Temporary snapshot not linked to main chain.
- **No code exists.**

### DeleteSnapshot in GraphStore
- **Noted as TODO in snapshot/manager.go line 96.**
- **What's needed:** Method on GraphStore + SQLite implementation. Required for GarbageCollect to actually work.

---

## Metrics

### Lines of Code per Package

| Package | LOC (including tests) |
|---------|------|
| internal/types | 250 |
| internal/store | 1,763 |
| internal/snapshot | 785 |
| internal/indexer | 1,510 |
| internal/indexer/goextractor | 1,449 |
| internal/indexer/gotsextractor | 1,075 |
| internal/indexer/treesitter | 567 |
| internal/enrichment | 892 |
| internal/daemon | 1,372 |
| internal/mcp | 1,205 |
| internal/resolver | 602 |
| cmd/knowing | 502 |
| e2e_test.go | (at repo root) |
| **Total** | **12,203** |

### Test Count per Package

| Package | Test file(s) | Test count (total passing: 123) |
|---------|-----------|----|
| cmd/knowing | main_test.go | ~5 |
| internal/store | sqlite_test.go | ~14 |
| internal/snapshot | manager_test.go | ~7 |
| internal/indexer | indexer_test.go, worker_test.go | ~6 |
| internal/indexer/goextractor | extractor_test.go, loader_test.go | ~10 |
| internal/indexer/gotsextractor | extractor_test.go | ~13 |
| internal/indexer/treesitter | extractor_test.go | ~9 |
| internal/enrichment | enricher_test.go | ~6 |
| internal/daemon | daemon_test.go, gitwatcher_test.go, gitdiff_test.go | ~12 |
| internal/mcp | handlers_test.go | ~6 |
| internal/resolver | resolver_test.go | ~8 |
| internal/snapshot | manager_test.go | ~7 |
| e2e_test.go | e2e_test.go | ~9 |

### Real Indexing Benchmarks

| Repo | Approach | Wall Time | Nodes | Edges | Cross-Repo Edges |
|------|----------|-----------|-------|-------|-----------------|
| knowing (self) | tree-sitter + LSP | ~9s | 231 (initial), 2,564 (polywave-go scale) | 672 (initial), 8,604 (polywave-go scale) | -- |
| polywave-go | go/packages (baseline) | 16m 24s | 6,340 | 17,232 | -- |
| polywave-go | tree-sitter + LSP (final) | 9.1s | 2,564 | 8,604 (all lsp_resolved) + 213 discovered | -- |
| polywave-go + polywave-web | tree-sitter | -- | 6,340 + 1,569 | 17,232 + 5,939 | 228 |

---

## File Inventory

```
cmd/knowing/
  main.go                          -- CLI entry point (serve, index, query, version)
  main_test.go

internal/types/
  types.go                         -- Hash, Node, Edge, File, Repo, Snapshot, EdgeEvent, EdgeProvenance, hash functions
  interfaces.go                    -- GraphStore, Extractor, ComputationCache, ExtractOptions, ExtractResult
  results.go                       -- CallerResult, CalleeResult, BlastRadiusResult, DiffResult, DerivedResult, TraversalOptions

internal/store/
  sqlite.go                        -- SQLiteStore: all 27+ GraphStore methods + batch ops
  sqlite_test.go
  migrate.go                       -- Migration runner with go:embed
  migrations/
    001_initial_schema.sql          -- 6 tables, 7 indexes
    002_add_dangling_edge_support.sql -- No-op
    003_add_callsite_columns.sql    -- 3 ALTER TABLE statements

internal/snapshot/
  manager.go                       -- SnapshotManager: compute, diff, GC, chain walking
  merkle.go                        -- MerkleTree: build, diff, sort
  manager_test.go

internal/indexer/
  indexer.go                       -- Indexer: IndexRepo, IndexFile, cleanup, edge events, resolver
  extractor.go                     -- ExtractorRegistry
  worker.go                        -- parallelExtract worker pool
  indexer_test.go
  worker_test.go
  goextractor/
    extractor.go                   -- GoExtractor (go/packages): calls, imports, implements, references
    loader.go                      -- BulkLoad (packages.Load("./..."))
    extractor_test.go
    loader_test.go
  gotsextractor/
    extractor.go                   -- GoTreeSitterExtractor: calls, imports with call-site positions
    extractor_test.go
  treesitter/
    extractor.go                   -- TreeSitterExtractor (Python): functions, classes, imports, calls
    extractor_test.go

internal/enrichment/
  enricher.go                      -- Enricher: LSP edge upgrade + edge discovery via gopls
  enricher_test.go

internal/daemon/
  daemon.go                        -- Daemon: lifecycle, watchLoop, indexWorker, GitHeadCommit
  gitwatcher.go                    -- GitWatcher: fsnotify on .git/HEAD, CommitEvent
  gitdiff.go                       -- GitDiffFiles: git diff --name-status
  daemon_test.go
  gitwatcher_test.go
  gitdiff_test.go

internal/mcp/
  server.go                        -- MCP Server: 11 tool definitions, stdio + HTTP transport
  handlers.go                      -- 11 handler implementations
  handlers_test.go

internal/resolver/
  resolver.go                      -- Resolver: dangling edge retargeting via hash recomputation
  resolver_test.go

e2e_test.go                        -- End-to-end integration test (multi-package Go module)
```

---

## Provenance Tiers (Implemented)

| Provenance | Confidence | Source | When Available |
|-----------|-----------|--------|----------------|
| `ast_inferred` | 0.7 | tree-sitter syntactic matching | After Tier 1 (~1.5s) |
| `lsp_resolved` | 0.9 | gopls GetDefinition/GetImplementation/GetReferences | After Tier 2 (~8s more) |
| `ast_resolved` | 1.0 | go/packages full type resolution | `--full` flag only (~16min) |

### Provenance Tiers (Defined in Architecture, NOT Implemented)

| Provenance | Confidence | Source | Status |
|-----------|-----------|--------|--------|
| `scip_imported` | 0.9 | SCIP index import | NOT IMPLEMENTED |
| `config_declared` | 0.8 | Terraform/K8s config | NOT IMPLEMENTED |
| `inferred_from_import` | 0.7 | Import without call site | NOT IMPLEMENTED (distinct from ast_inferred) |
| `openapi_declared` | 0.7 | OpenAPI/proto spec | NOT IMPLEMENTED |
| `text_matched` | 0.3 | String literal/comment heuristic | NOT IMPLEMENTED |
| `manual` | 1.0 | User-declared | NOT IMPLEMENTED |
| `otel_trace` | 0.5-0.95 | Runtime observation | NOT IMPLEMENTED |
| `runtime_unresolved` | 0.3 | Unresolved runtime endpoint | NOT IMPLEMENTED |

---

## Symbol Identity Scheme

Format: `{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`

Hash computation: `sha256(repoURL + "\x00" + packagePath + "\x00" + symbolName + "\x00" + symbolKind)`

Note: `contentHash` parameter exists in `ComputeNodeHash` for API compatibility but is NOT included in the hash. Node identity depends on (repo, package, name, kind) only.

Edge hash: `sha256(sourceHash.String() + "\x00" + targetHash.String() + "\x00" + edgeType + "\x00" + provenanceJSON)`

File hash: `sha256(repoHash + path + contentHash)`

Repo hash: `sha256(repoURL)`

Snapshot hash: Merkle root of sorted edge hashes (binary tree, odd leaf paired with itself).

---

## External Dependencies

| Dependency | Used by | Purpose |
|-----------|---------|---------|
| `modernc.org/sqlite` | internal/store | Pure Go SQLite driver (no CGo) |
| `github.com/smacker/go-tree-sitter` | gotsextractor, treesitter | Tree-sitter Go bindings |
| `github.com/smacker/go-tree-sitter/golang` | gotsextractor | Go grammar |
| `github.com/smacker/go-tree-sitter/python` | treesitter | Python grammar |
| `golang.org/x/tools/go/packages` | goextractor | Go type-resolution package loader |
| `github.com/mark3labs/mcp-go` | internal/mcp | MCP protocol server library |
| `github.com/fsnotify/fsnotify` | internal/daemon | File system notification |
| `github.com/blackwell-systems/agent-lsp/pkg/lsp` | internal/enrichment | LSP client (gopls communication) |
| `github.com/blackwell-systems/agent-lsp/pkg/types` | internal/enrichment | LSP type definitions |
