# FEATURES.md -- Comprehensive Feature Dump for AI Reference

Generated: 2026-05-15 (updated: 2026-05-22, features 77-114 added: hash domain prefixes, fsck, GC, lockfile, LRU cache, modular community detection, DiffOptions, indexed_at, verify, React viz, MCP resources, graph notes, Phase 3 complete, Merkle proofs, stats CLI, named refs, generation numbers, auto-GC, merkle-strata extraction; P2 edge types, parallel indexer, cross-system benchmark, language equivalence classes, cross-file import resolution, inheritance propagation, deeper call chains, test deprioritization; FTS concepts column, task memory persistence; compound-first keyword extraction, Java/C# import resolution; daemon lifecycle, untrack_repo)
Source: code inspection of all Go files across internal/, cmd/, and config
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
- **What it does:** Inserts multiple records in a single transaction using multi-row INSERT statements. Each statement packs multiple rows to reduce per-row overhead: edges use 100 rows/statement (900 params), nodes use 99 rows/statement (990 params), files use 249 rows/statement (996 params). Chunk sizes are chosen to stay under SQLite's 999 variable limit.
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
  - Cross-file import resolution via `buildPythonImportMap` resolves calls through import map (63 edges on Flask). See feature 107.
  - Class context tracking is basic (single-level nesting only).
  - ~~No call-site positions stored on edges.~~ (Fixed: call-site positions are now stored on Python call edges.)
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
- **Entry point:** `parallelExtract` in `internal/indexer/indexer.go`
- **What it does:** Fan-out/fan-in worker pool. Distributes file extraction across `runtime.GOMAXPROCS` goroutines (default 8 workers via `--workers` flag). Results collected via a channel and stored in submission order. Each worker uses a CGO watchdog timeout: extraction runs in a fire-and-forget goroutine with a 10-second timer select. If a file's tree-sitter parse exceeds 10s (stuck in CGO, which is not interruptible by Go context cancellation), the watchdog fires, the worker moves on, and the stuck goroutine's result is discarded.
- **Inputs:** Slice of `extractWork` items, number of workers.
- **Outputs:** Slice of `extractResult` in submission order.
- **Limitations/known gaps:** Only parallelizes AST extraction, not type-checking. Stuck CGO goroutines are not reclaimed (they complete in the background and their results are discarded).
- **Dependencies:** None.

### 10. Repository Indexer

- **Package(s):** `internal/indexer`
- **Entry point:** `indexer.NewIndexer(store, snapshot).IndexRepo(ctx, repoURL, repoPath, commitHash string) (*Snapshot, error)`
- **What it does:** Walks repository directory, skips `.git`, `.claude`, `vendor`, `node_modules`, `testdata`. Computes content hashes per file. Skips unchanged files (incremental). For changed files: deletes old nodes/edges via `cleanupStore` interface, re-extracts, records edge events ("added"/"removed"). For deleted files (present in store but absent on disk): removes all associated nodes and edges, ensuring the graph stays clean on re-index. Batch inserts all results. Computes snapshot. Runs cross-repo resolver. Tracks changed file paths for downstream enrichment.
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
- **What it does:** Two-pass enrichment using language servers (gopls, pyright, typescript-language-server, rust-analyzer, jdtls, OmniSharp) via `agent-lsp/pkg/lsp`:
  1. **Edge upgrade pass:** For each `ast_inferred` edge with call-site positions, queries `GetDefinition` at (file, line, col). If the language server confirms a definition, upgrades edge to `lsp_resolved` confidence 0.9.
  2. **Edge discovery pass:** For each file, gets document symbols. For types/interfaces, queries `GetImplementation`. For functions/methods, queries `GetReferences`. Creates new `lsp_resolved` edges.
  3. **Workspace readiness:** Waits up to 120s for async servers (e.g., jdtls) to finish workspace indexing before querying (`WaitForWorkspaceReadyTimeout`).
  4. **resolveNamePosition:** Corrects symbol positions for servers (pyright, pylsp) that set SelectionRange to the keyword (`class`, `def`) rather than the identifier name.
- **Inputs:** Repo hash, workspace root, optional changed file list for scoping.
- **Outputs:** Logs summary (edges processed, upgraded, discovered, errors) per language server.
- **Limitations/known gaps:**
  - Discovered edges use synthetic position-based hashes (not aligned with node hashes from extractors).
  - `RunScoped` opens only changed files, which may reduce cross-package resolution accuracy.
  - Errors in individual edge upgrades are counted but not surfaced.
  - Superseded by multi-language auto-detection (see Feature 65). Legacy single-server path still works for Go-only repos.
- **Dependencies:** `github.com/blackwell-systems/agent-lsp/pkg/lsp`, GraphStore.

### 12. Snapshot Manager (Hierarchical Merkle DAG)

- **Package(s):** `internal/snapshot`
- **Entry point:** `snapshot.NewSnapshotManager(store).ComputeSnapshot(ctx, repoHash, commitHash) (*Snapshot, error)`
- **What it does:** Collects all edge hashes for a repo (via nodes from `NodesByName`), builds both a hierarchical Merkle tree and a flat Merkle tree, chains the new snapshot to the latest existing snapshot, stores it. The hierarchical tree organizes edges by package and edge type (repo root -> package roots -> edge-type roots -> edge leaves); the flat tree is built alongside for backward compatibility. Both produce identical root hashes.
- **Inputs:** Repo hash, commit hash.
- **Outputs:** `*Snapshot` with Merkle root, parent pointer, node/edge counts.
- **Limitations/known gaps:**
  - `GarbageCollect` uses `DeleteSnapshot` to remove old snapshots and their associated edge events.
  - Synthetic file nodes not included in Merkle tree (only nodes with qualified names).
  - Flat `DiffMerkle` is a set-diff on leaves; use `DiffHierarchicalTrees` for package-scoped diffs (216x faster on ~24.9K edges).
- **Dependencies:** GraphStore.

### 13. Hierarchical Merkle Tree

- **Package(s):** `internal/snapshot`
- **Entry point:** `snapshot.BuildHierarchicalTree(edges []EdgeInput) *HierarchicalTree`
- **What it does:** Constructs a four-level Merkle tree from edges with package and edge-type metadata. Structure: repo root -> package roots -> edge-type roots -> edge leaves. `DiffHierarchicalTrees` compares package roots to find which packages changed (O(packages) instead of O(edges)). `SubgraphRoot` returns an O(1) cache key for any set of packages by combining their package roots. `EdgeTypeRoot` returns the root for a specific package and edge type in one lookup. `ContextPackRoot` enables content-addressed context pack deduplication across agents and sessions. See `bench/merkle-diff/` for benchmark results and `docs/architecture/merkle-algorithms.md` for the full algorithm specification.
- **Inputs:** Slice of `EdgeInput` values (each carrying EdgeHash, PackagePath, EdgeType).
- **Outputs:** `*HierarchicalTree` with Root, PackageRoots, EdgeTypeRoots, PackageEdgeCounts, TotalEdges.
- **Performance:** 216x faster diff than flat tree on the knowing repo (~24.9K edges), 517x on 100K synthetic edges. Subgraph root lookup: O(1), ~59ns. Build cost overhead: negligible.
- **Dependencies:** `snapshot.BuildMerkleTree` (used internally for each subtree level).

### 13a. Flat Merkle Tree (Backward Compatibility)

- **Package(s):** `internal/snapshot`
- **Entry point:** `snapshot.BuildMerkleTree(hashes []Hash) *MerkleTree`
- **What it does:** Sorts hashes lexicographically via `bytes.Compare`, builds binary Merkle tree (odd leaf paired with itself), returns root hash and sorted leaves. Built alongside the hierarchical tree by the snapshot manager. Both produce identical root hashes.
- **Inputs:** Slice of `Hash` values.
- **Outputs:** `*MerkleTree` with Root and Leaves.
- **Limitations/known gaps:** Full reconstruction (no incremental update). `DiffMerkle` is O(n) set comparison; prefer `DiffHierarchicalTrees` for performance-sensitive diff paths.
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
- **What it does:** Long-lived process with up to four goroutines:
  1. `watchLoop`: reads `CommitEvent` from `GitWatcher`, enqueues index requests.
  2. `indexWorker`: processes index requests sequentially with write lock, triggers background enrichment on success.
  3. MCP server (if configured): serves HTTP.
  4. `traceIngestLoop` (if TraceConfig enabled): runs SymbolResolver + Ingestor + OTLPReceiver with periodic FlushBatch and DecayConfidence.
  Provides `WatchRepo`/`UnwatchRepo` for dynamic repo management. `RLock`/`RUnlock` for concurrent read access.
- **Inputs:** `DaemonConfig` with Store, IndexFunc, EnrichFunc, MCPAddr, MCPServer, TraceConfig, DBPath.
- **Outputs:** Blocks until context cancellation. Returns error on startup failure.
- **Limitations/known gaps:**
  - Only HTTP MCP transport in daemon mode (stdio available via `ServeStdio` but not wired in daemon).
  - Index queue is buffered (128); overflows are silently dropped.
  - No post-commit hook installation (architecture describes it but only fsnotify-based GitWatcher is implemented).
  - No polling fallback (architecture describes it but not implemented).
  - `repos` map defaults URL to path if not explicitly set.
  - Trace ingestor startup errors are silently ignored.
  - Trace decay interval hardcoded to 1 hour.
- **Dependencies:** GitWatcher, MCPServer interface, IndexFunc callback, EnrichFunc callback, `internal/trace` (for trace ingestion).

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
- **What it does:** Wraps `mcp-go` server library. Registers 28 tools across seven planes (execution, intelligence, runtime, context, feedback, discovery, management) and 3 prompts (refactor_safely, review_pr, investigate_dead_code). Supports stdio and HTTP transports. Tool definitions include parameter schemas with descriptions and required flags. The Server holds a `sqlStore *SQLiteStore` (populated via type assertion from GraphStore) for runtime query tools.
- **Inputs:** GraphStore (runtime tools additionally require `*SQLiteStore`).
- **Outputs:** Serves MCP protocol over stdio or HTTP.
- **Limitations/known gaps:**
  - `semantic_diff` and `pr_impact` are thin wrappers over `SnapshotDiff` (not full semantic analysis as described in architecture).
  - `trace_dataflow` maps to `TransitiveCallees` (no actual data flow tracing).
  - `ownership` lists files and symbols (no CODEOWNERS integration, no team mapping).
  - `index_repo` uses a package-level `indexFunc` variable set via `SetIndexFunc` (not ideal).
  - Runtime tools (runtime_traffic, dead_routes, trace_stats) require `*SQLiteStore`; return error if GraphStore is a different implementation.
- **Dependencies:** `github.com/mark3labs/mcp-go`, GraphStore, `internal/store.SQLiteStore` (optional, for runtime tools).

### 20. CLI (`knowing` Binary)

- **Package(s):** `cmd/knowing`
- **Entry point:** `main.main() -> run(args)`
- **What it does:** Dispatches to subcommands: `serve`, `index`, `query`, `export`, `diff`, `context`, `mcp`, `reindex`, `init`, `test-scope`, `version`. Wires together all internal packages.
- **Inputs:** CLI arguments and flags.
- **Outputs:** Stdout text (query results, JSON export), SQLite database file.
- **Dependencies:** All internal packages.

### 21. Edge Event Recording

- **Package(s):** `internal/indexer` (writes), `internal/store` (stores)
- **Entry point:** Called automatically during `IndexRepo` after cleanup and re-extraction.
- **What it does:** For each index run with changed files: computes diff between old edges (deleted before re-extraction) and new edges (from fresh extraction). Records "added" events for truly new edges and "removed" events for truly deleted edges. Events are keyed by snapshot hash.
- **Inputs:** Old edges (from cleanup), new edges (from extraction), snapshot hash.
- **Outputs:** Rows in `edge_events` table.
- **Limitations/known gaps:** Errors in event recording are silently ignored. Events only recorded when `cleanupStore` interface is available.
- **Dependencies:** GraphStore.RecordEdgeEvent.

### 22. Runtime Trace Ingestion Pipeline

- **Package(s):** `internal/trace`
- **Entry point:** `trace.NewIngestor(db *sql.DB, resolver *SymbolResolver, config TraceIngestConfig) *Ingestor`
- **What it does:** Converts raw observability data (OTel spans, HTTP access logs) into runtime graph edges. The pipeline has four layers:
  1. **Types and Configuration** (`types.go`): Defines `TraceSpan` (normalized span representation with TraceID, SpanID, ParentSpanID, ServiceName, OperationName, PeerService, Attributes, StartTime, Duration), `HTTPLogEntry`, `RuntimeStats`, `IngestResult`, `RouteMapping`, `HealthState` enum (CONNECTED, RECONNECTING, DISCONNECTED, DISABLED), `TraceIngestConfig`, and the `TraceIngestor` interface (IngestSpans, IngestHTTPLogs, RuntimeEdgeStats, DecayConfidence).
  2. **Confidence Scoring** (`confidence.go`): `ConfidenceFromCount(count int) float64` maps observation volume to confidence (>1000 observations: 0.95, >=100: 0.85, >=10: 0.7, >=1: 0.5, 0: 0.2). `ComputeConfidence(observationCount, daysSinceLastObserved)` combines volume and recency (>90 days: 0.0, >30 days: 0.2, otherwise delegates to ConfidenceFromCount). `DecayBracket(days)` returns human-readable brackets: "active" (<=7d), "recent" (<=30d), "stale" (<=90d), "gc_eligible" (>90d). `ShouldGarbageCollect(lastObserved, gcThresholdDays)` returns true if edge exceeds threshold. `BuildProvenance(traceIDs []string)` returns `"otel_trace:{\"trace_ids\":[...]}"` with max 5 trace IDs, or plain `"otel_trace"` if empty.
  3. **Symbol Resolver** (`resolver.go`): `SymbolResolver` backed by `*sql.DB`. `Resolve(ctx, serviceName, routePattern, mappingType)` looks up the `route_symbols` table by exact match on (service_name, route_pattern, mapping_type); returns node hash with confidence 1.0 on hit, or a synthetic unresolved node hash (via `ComputeNodeHash` with kind "runtime_endpoint") with confidence 0.3 on miss. `ResolveSpan(ctx, TraceSpan)` extracts runtime identifiers from span attributes (`http.method`+`http.route` for HTTP, `rpc.service`+`rpc.method` for gRPC), resolves source as a service-kind node and target via Resolve against peer service (or own service if no peer).
  4. **Ingestor** (`ingestor.go`): Implements `TraceIngestor`. `IngestSpans` iterates spans, resolves source/target hashes via SymbolResolver, determines edge type from attributes (`runtime_calls` for HTTP, `runtime_rpc` for gRPC, `runtime_produces`/`runtime_consumes` for messaging), computes edge hash with provenance "otel_trace", upserts edges (incrementing observation_count and updating last_observed/confidence on existing edges, inserting with edge event on new edges). `IngestHTTPLogs` converts HTTP log entries to TraceSpan and delegates to IngestSpans. `RuntimeEdgeStats` aggregates statistics for otel-provenance edges. `DecayConfidence` reduces confidence to 0.2 on edges not observed in 30+ days. `AddToBatch(span)` accumulates spans with mutex protection; auto-flushes at configured BatchSize. `FlushBatch(ctx)` ingests all pending spans.
- **Inputs:** Trace spans (from OTLP gRPC or manual), HTTP log entries, configuration.
- **Outputs:** Graph edges with provenance `otel_trace`, observation counts, confidence scores.
- **Limitations/known gaps:**
  - `RuntimeEdgeStats` snapshot parameter is accepted but not used for filtering.
  - `AddToBatch` auto-flush errors are silently dropped.
  - No support for queue/messaging topic resolution beyond basic attribute detection.
  - Edge hash uses plain "otel_trace" (not the full provenance with trace IDs) so the same relationship always maps to the same hash.
- **Dependencies:** `*sql.DB` (direct database access, not via GraphStore), `SymbolResolver`.

### 23. OTLPReceiver (gRPC Trace Server)

- **Package(s):** `internal/trace`
- **Entry point:** `trace.NewOTLPReceiver(addr string, ingestor TraceIngestor) *OTLPReceiver`
- **What it does:** Real gRPC server implementing the OTLP trace receiver protocol (`coltracepb.TraceServiceServer`). Accepts `ExportTraceServiceRequest` messages over gRPC, extracts `service.name` from Resource attributes, converts OTLP Span protos to internal `TraceSpan` structs (including trace/span IDs as hex, peer.service from attributes, start time and duration from nanosecond timestamps, all attributes as string map), and passes them to the `Ingestor` via `AddToBatch`.
- **Inputs:** gRPC `ExportTraceServiceRequest` messages on configured address.
- **Outputs:** Spans forwarded to Ingestor batch. Health state transitions (DISABLED -> CONNECTED on Start, CONNECTED -> DISABLED on Stop).
- **Key methods:** `Start(ctx)` creates and starts gRPC server, `Stop()` gracefully stops, `Health()` returns current `HealthState`, `Export(ctx, req)` implements the OTLP service, `ExportSpans(ctx, spans)` accepts pre-converted TraceSpan (for tests), `ListenAddr()` returns actual bound address (useful for port :0).
- **Limitations/known gaps:**
  - `Export` uses type assertion `r.ingestor.(*Ingestor)` to call `AddToBatch` (not on the TraceIngestor interface).
  - No TLS support.
  - No reconnection logic (health state RECONNECTING exists but is not used).
- **Dependencies:** `go.opentelemetry.io/proto/otlp/collector/trace/v1`, `go.opentelemetry.io/proto/otlp/trace/v1`, `google.golang.org/grpc`.

### 24. Store Runtime Methods

- **Package(s):** `internal/store`
- **Entry point:** Methods on `*SQLiteStore` (in `sqlite_runtime.go`)
- **What it does:** Extends SQLiteStore with runtime-specific operations:
  - `PutRouteSymbol(ctx, serviceName, routePattern string, nodeHash Hash, mappingType string) error`: Upserts a route-to-node mapping in `route_symbols` table (INSERT OR REPLACE).
  - `GetRouteSymbol(ctx, serviceName, routePattern, mappingType string) (*RouteSymbolRow, error)`: Retrieves a route symbol by composite key; returns (nil, nil) if not found.
  - `UpdateObservation(ctx, edgeHash Hash, count int, lastObserved int64, confidence float64) error`: Updates observation_count, last_observed, and confidence on an edge.
  - `RuntimeEdgesByProvenance(ctx, provenancePrefix string) ([]Edge, error)`: Returns edges matching provenance LIKE prefix%, scanning all 11 edge columns including observation_count and last_observed.
  - `DecayRuntimeConfidence(ctx, staleDays int, newConfidence float64) (int, error)`: Reduces confidence on otel-provenance edges not observed in staleDays, returns rows affected.
  - `RuntimeEdgesByService(ctx, serviceName, routePattern string, limit int) ([]Edge, error)`: Filters runtime edges by service name via JOIN on route_symbols, with optional LIKE filter on route_pattern.
  - `DeadRoutes(ctx, staleDays int) ([]RouteSymbolRow, error)`: Returns route symbols with no runtime observations (or observations older than staleDays) via LEFT JOIN on edges.
  - `RuntimeEdgeStatsAggregate(ctx) (*RuntimeStatsRow, error)`: Aggregates statistics (total, active <=7d, stale >30d, GC-eligible >90d, by edge type) for all otel-provenance edges.
- **Inputs:** Context, typed parameters.
- **Outputs:** Typed structs. `RouteSymbolRow` contains ServiceName, RoutePattern, MappingType, NodeHash, CreatedAt. `RuntimeStatsRow` contains TotalEdges, ActiveEdges, StaleEdges, GCEligible, ByEdgeType map.
- **Dependencies:** `*sql.DB`, `internal/types`.

### 25. Edge Struct Extension (Runtime Fields)

- **Package(s):** `internal/types`
- **Entry point:** `Edge` struct in `types.go`
- **What it does:** Two new fields added to the Edge struct:
  - `ObservationCount int`: Total observations in current window (0 for static edges).
  - `LastObserved int64`: Unix timestamp of last observation (0 for static edges).
  These fields are populated by the trace ingestion pipeline and read by the store runtime methods. Static edges (from extractors/enrichment) always have zero values.
- **Dependencies:** None.

### 26. HTTP Route Extraction (Static Analysis)

- **Package(s):** `internal/indexer/gotsextractor`
- **Entry point:** `extractRouteSymbols(body *sitter.Node, opts ExtractOptions, pkgPath string, imports map[string]string) []routeSymbol` (called automatically during Extract for function and method bodies)
- **What it does:** During tree-sitter extraction, walks function/method bodies looking for HTTP route registration patterns. Detects calls to router packages and extracts route patterns. Supported router packages:
  - `net/http`: HandleFunc, Handle
  - `github.com/go-chi/chi` (v5): Get, Post, Put, Delete, Patch
  - `github.com/gin-gonic/gin`: GET, POST, PUT, DELETE, PATCH
  - `github.com/labstack/echo` (v4): GET, POST, PUT, DELETE, PATCH
  - `github.com/gorilla/mux`: HandleFunc, Handle
  For each detected route registration:
  1. Creates a `route_handler` node with QualifiedName `{repoURL}://{pkgPath}.{HTTPMethod} {routePattern}`, kind "route_handler".
  2. Creates a `handles_route` edge from the route node to the handler function node (if resolvable), with provenance "ast_inferred" and confidence 0.7.
  Uses heuristic resolution: resolves router package via import aliases, and for local variables (e.g., `r := chi.NewRouter()`) infers the package from context if the method name matches a known router method and the file imports a router package.
  **Supported frameworks (18 total across 6 languages):**
  - **Go (5 frameworks):** net/http, chi, gin, echo, gorilla/mux
  - **Python (3 frameworks):** Flask (`@app.get`, `@app.route`), FastAPI (`@app.get`, `@router.post`), Django (`path()` in urls.py)
  - **TypeScript (5 frameworks):** Express, Fastify, Hono, NestJS (`@Controller` + `@Get`/`@Post` decorators), Next.js App Router (exported `GET`/`POST` in `route.ts`)
  - **Other languages:** (remaining 5 frameworks across Terraform, SQL, K8s, CSS extractors produce structural edges, not HTTP route edges)
- **Inputs:** Function/method body AST node, extract options, import map.
- **Outputs:** `route_handler` nodes and `handles_route` edges appended to the ExtractResult.
- **Limitations/known gaps:**
  - Route extraction is heuristic; local variable inference may produce false positives if an unrelated type has a matching method name.
  - Only string literal route patterns are extracted (dynamic routes from variables are skipped).
  - The `route_symbols` table is not populated by the indexer; `PutRouteSymbol` must be called separately. The extractor produces graph nodes/edges but does not write to route_symbols.
  - No support for route groups/prefixes (only individual registrations).
- **Dependencies:** tree-sitter AST, `internal/types`.

### 27. Daemon Trace Ingestion Wiring

- **Package(s):** `internal/daemon`
- **Entry point:** `daemon.DaemonConfig.TraceConfig` and `Daemon.traceIngestLoop(ctx)`
- **What it does:** Adds a fourth goroutine to the daemon lifecycle for runtime trace ingestion. `TraceIngestConfig` (in daemon package) holds Enabled, OTLPEndpoint, BatchSize, and BatchInterval. When `TraceConfig` is non-nil and Enabled is true, `Start()` launches `traceIngestLoop` which:
  1. Opens a dedicated `*sql.DB` connection to the SQLite database (via `DaemonConfig.DBPath`).
  2. Creates a `trace.SymbolResolver` and `trace.Ingestor` with the config.
  3. Creates and starts a `trace.OTLPReceiver` on the configured endpoint.
  4. Runs two tickers: `BatchInterval` for periodic `FlushBatch`, and 1 hour for periodic `DecayConfidence`.
  5. On context cancellation, flushes remaining batch and stops the receiver.
- **CLI flags** (in `cmd/knowing/main.go` cmdServe):
  - `--trace` (bool): Enable runtime trace ingestion.
  - `--trace-endpoint` (string, default "localhost:4317"): OTLP gRPC endpoint.
  - `--trace-batch-size` (int, default 1000): Spans per batch.
  - Batch interval hardcoded to 10 seconds.
- **Limitations/known gaps:**
  - Trace ingestor startup errors are silently ignored (trace is non-critical).
  - Decay interval is hardcoded to 1 hour (not configurable).
  - Uses a separate DB connection; does not share the daemon's store connection.
- **Dependencies:** `internal/trace` (SymbolResolver, Ingestor, OTLPReceiver), `modernc.org/sqlite`.

### 28. MCP Runtime Tools

- **Package(s):** `internal/mcp`
- **Entry point:** `mcp.NewServer(store)` (tools registered automatically)
- **What it does:** Three new MCP tools in the "Runtime plane" category, requiring the underlying store to be `*SQLiteStore` (checked via type assertion):
  1. `runtime_traffic`: Queries runtime-observed edges by service name and optional route pattern (LIKE syntax). Parameters: `service_name` (required), `route_pattern` (optional), `limit` (optional, default 100). Calls `SQLiteStore.RuntimeEdgesByService`.
  2. `dead_routes`: Finds route symbols with no runtime observations in N days. Parameters: `stale_days` (optional, default 30). Calls `SQLiteStore.DeadRoutes`.
  3. `trace_stats`: Returns aggregate statistics about runtime-derived edges (total, active, stale, GC-eligible, by edge type). No parameters. Calls `SQLiteStore.RuntimeEdgeStatsAggregate`.
  Total MCP tools: 27. Also registers 3 MCP prompts (refactor_safely, review_pr, investigate_dead_code).
- **Limitations/known gaps:**
  - Runtime tools return "runtime queries not available: store does not support runtime methods" if the store is not a `*SQLiteStore`.
  - No authentication or rate limiting on runtime queries.
- **Dependencies:** `internal/store.SQLiteStore` (via type assertion from `types.GraphStore`).

### 29. Graph Export CLI

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing export [flags]`
- **What it does:** Exports the knowledge graph as JSON to stdout. Collects all nodes (optionally filtered by repo), then collects all outgoing edges for those nodes (deduplicated by EdgeHash). Outputs a JSON structure with `nodes` (node_hash, qualified_name, kind, line, signature), `edges` (edge_hash, source_hash, target_hash, edge_type, confidence, provenance), and `metadata` (repo, snapshot, exported_at timestamp, node_count, edge_count).
- **Flags:**
  - `--db` (default "knowing.db"): SQLite database path.
  - `--format` (default "json"): Output format (`json` with community annotations, or `dot` with Louvain subgraphs).
  - `--repo`: Filter by repo URL (filters nodes to those whose FileHash belongs to the specified repo).
  - `--snapshot`: Filter label for metadata (recorded but not used for actual edge filtering in current implementation).
- **Limitations/known gaps:**
  - `--snapshot` filter is cosmetic only; it does not filter edges by snapshot.
  - `--repo` filter queries all nodes via `NodesByName(ctx, "")` then filters in memory; no store-level filtering.
  - DOT format does not support `--repo` or `--snapshot` filtering.
- **Dependencies:** `internal/store`, `internal/types`.

### 30. CI/CD Pipeline

- **Package(s):** `.github/workflows/`, `.goreleaser.yml`
- **What it does:** Three GitHub Actions workflows and GoReleaser configuration:
  1. **CI** (`ci.yml`): Runs on push/PR to main. Steps: checkout, setup-go (from go.mod), build, vet, test (5min timeout), binary smoke test (`knowing version`). Sets `GOWORK=off`.
  2. **Release** (`release.yml`): Triggered by `v*` tags. Runs tests, then GoReleaser with Docker multi-arch builds, Homebrew tap publish, winget publish, npm publish (via `scripts/npm-publish.sh`), PyPI publish (via `scripts/pypi-build-wheels.sh`), and MCP Registry publish (via `mcp-publisher`).
  3. **Docs** (`docs.yml`): Deploys documentation to GitHub Pages on push to main. Uses mkdocs-material to build the `site` directory.
  4. **GoReleaser** (`.goreleaser.yml`): Version 2 config. Builds for linux/darwin/windows on amd64/arm64 with CGO_ENABLED=0. Produces archives, checksums, Homebrew formula (to `blackwell-systems/homebrew-tap`), Docker images on GHCR and Docker Hub (multi-arch manifests for amd64+arm64). Ldflags inject Version, commit, date.
- **Limitations/known gaps:**
  - No integration test step in CI (unit tests only).
  - Docker images reference `docker/Dockerfile` which is not in the file inventory above.
  - npm/PyPI publish scripts referenced but not documented here.
- **Dependencies:** GitHub Actions, GoReleaser v2, Docker, mkdocs-material.

### 31. Wire Format System (Pluggable Codec Registry)

- **Package(s):** `internal/wire`
- **Entry point:** `wire.EncodeWith(name string, p *Payload) (string, error)`, `wire.DecodeWith(name string, input string) (*Payload, error)`
- **What it does:** Provides a pluggable codec registry with 5 built-in encoders for graph payloads:
  1. **GCF (Graph Compact Format):** Text-only, graph-native encoding optimized for LLM token consumption. Uses local IDs (`@0`, `@1`), positional encoding, kind abbreviations, and group headers to achieve **84% token savings** over JSON (76.7% per-payload median, compounding to 84% with session statefulness). Supports session statefulness for progressive vocabulary building across tool calls.
  2. **Binary (GCB1):** Compact binary encoding using varint integers, enum IDs (1 byte), float32 scores, index-based edge references, and length-prefixed strings. Optimized for transport between services and persistent caching (~89% byte savings vs JSON).
  3. **JSON:** Standard JSON serialization for maximum compatibility and debuggability.
  4. **XML:** XML serialization for tool interoperability.
  5. **Markdown:** Human-readable markdown tables for documentation and debugging.
  Codecs are registered at init time. Custom codecs can be added via `wire.Register()`. The MCP tools and CLI pass the `format` parameter directly to the registry, making new codecs immediately available to all consumers. The binary codec is registered under the name "gcb" (Graph Compact Binary).
- **Session statefulness:** `wire.Session` tracks previously-transmitted symbols across multiple tool calls within a session. On repeated symbols, only the local ID reference is emitted (no full definition re-transmitted). This cross-call symbol deduplication yields **47% additional savings** on repeated symbols within a session, compounding on top of the per-payload compression.
- **Inputs:** `*Payload` struct containing tool name, token counts, symbols (with scores, kinds, provenance, components), and edges.
- **Outputs:** Encoded string (GCF, XML, or Markdown) or binary bytes (GCB). Decode returns `*Payload`.
- **Benchmark results:** 6 fixture cases (8 to 30 symbols) with encode p99 latency of 64 microseconds on Apple M4 Pro.
- **Limitations/known gaps:**
  - Binary codec (gcb) requires version bump for extensibility (GCF can append new fields freely).
  - Binary codec registered in its own init() separately from the other codecs.
- **Dependencies:** None beyond stdlib.

### 32. Terraform (HCL) Extractor

- **Package(s):** `internal/indexer/terraformextractor`
- **Entry point:** `terraformextractor.NewTerraformExtractor() *TerraformExtractor`
- **What it does:** Parses `.tf` files (HCL syntax) and extracts infrastructure-as-code relationships: resources, data sources, modules, variables, outputs, and dependency edges between them.
- **Dependencies:** HCL parser.

### 33. SQL Extractor

- **Package(s):** `internal/indexer/sqlextractor`
- **Entry point:** `sqlextractor.NewSQLExtractor() *SQLExtractor`
- **What it does:** Parses `.sql` files and extracts schema relationships: tables, views, columns, foreign keys, and dependency edges between them.
- **Dependencies:** SQL parser.

### 34. Kubernetes YAML Extractor

- **Package(s):** `internal/indexer/k8sextractor`
- **Entry point:** `k8sextractor.NewK8sExtractor() *K8sExtractor`
- **What it does:** Parses Kubernetes manifest files (`.yaml`/`.yml` with K8s resource kinds) and extracts deployments, services, configmaps, secrets, and their inter-resource references.
- **Dependencies:** YAML parser.

### 35. CSS Extractor

- **Package(s):** `internal/indexer/cssextractor`
- **Entry point:** `cssextractor.NewCSSExtractor() *CSSExtractor`
- **What it does:** Parses `.css` files and extracts selectors, custom properties (variables), and relationships between them.
- **Dependencies:** CSS parser.

### 36. MCP Self-Dogfooding (.mcp.json)

- **Package(s):** Repository root
- **Entry point:** `.mcp.json` in repo root
- **What it does:** Configures knowing to serve its own knowledge graph via MCP. Allows AI agents working in the knowing codebase to query the graph using knowing's own MCP tools, creating a self-referential development loop.
- **Limitations/known gaps:** Requires knowing to be built and available on PATH or configured as a local server.
- **Dependencies:** `knowing mcp` subcommand.

### 37. MCP Prompts

- **Package(s):** `internal/mcp`
- **Entry point:** `Server.registerPrompts()` (called from `NewServer`)
- **What it does:** Registers 3 MCP prompts that provide structured workflows for common tasks:
  1. **refactor_safely:** Guides agents through blast-radius analysis, speculative preview, apply, and verify steps for safe refactoring.
  2. **review_pr:** Structured PR review workflow using graph context to identify affected symbols and ownership.
  3. **investigate_dead_code:** Workflow for finding and confirming dead code using reference analysis and runtime traffic data.
- **Dependencies:** `github.com/mark3labs/mcp-go`, MCP Server.

### 38. `knowing mcp` Subcommand (Stdio MCP Server)

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing mcp [flags]`
- **What it does:** Launches the MCP server in stdio transport mode. Reads JSON-RPC messages from stdin, writes responses to stdout. Designed for use as a subprocess MCP server (e.g., configured in `.mcp.json` or Claude Desktop). Provides all 28 MCP tools and 3 prompts over stdio without requiring HTTP.
- **Flags:** `--db` (default: `~/.knowing/knowing.db`): SQLite database path.
- **Dependencies:** `internal/mcp`, `internal/store`.

### 39. `knowing reindex` Subcommand

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing reindex [flags]`
- **What it does:** Clears all nodes, edges, and edge events from the store, then re-indexes the specified repository from scratch. Useful when extractor logic has changed or when the graph has accumulated stale data that incremental indexing cannot clean up.
- **Flags:** `--db` (default: `~/.knowing/knowing.db`), repository path (positional).
- **Dependencies:** `internal/store`, `internal/indexer`.

### 40. GCF Session Statefulness (Cross-Call Symbol Deduplication)

- **Package(s):** `internal/wire`
- **Entry point:** `wire.NewSession() *Session`, `Session.Encode(codec string, p *Payload) (string, error)`
- **What it does:** Tracks previously-transmitted symbols across multiple tool calls within the same MCP session. When a symbol has already been sent in a prior response, only its local ID reference is emitted instead of the full definition. This provides **47% additional token savings** on repeated symbols within a session, compounding on top of per-payload GCF compression.
- **Inputs:** Codec name, `*Payload` to encode.
- **Outputs:** Encoded payload with deduplicated symbol references.
- **Dependencies:** `internal/wire` codec registry.

### 41. Snapshot Lifecycle Integration Test

- **Package(s):** root (`e2e_test.go`)
- **What it does:** End-to-end test covering the full snapshot lifecycle: index a repo, compute snapshot, re-index with changes, compute new snapshot, verify snapshot diff contains expected added/removed edges. Validates that the Merkle chain, edge event recording, and snapshot diff work correctly together.
- **Dependencies:** `internal/store`, `internal/indexer`, `internal/snapshot`.

### 42. MCP-Assert Suite (YAML Per-File Assertions)

- **Package(s):** test fixtures / CI
- **What it does:** Updated the mcp-assert test suite to use the new YAML format with per-file assertions. Each assertion file declares expected nodes, edges, and relationships for a specific source file, enabling targeted regression testing of extractor output.
- **Dependencies:** mcp-assert tooling, CI workflow.

### 43. Feedback MCP Tool (Agent Learning Loop)

- **Package(s):** `internal/mcp`, `internal/store`
- **Entry point:** `feedback` MCP tool (action: "record" or "query")
- **What it does:** Records whether a symbol was useful to an agent session and queries aggregate feedback stats. The `FeedbackProvider` interface is wired into `ContextEngine` so that ranking scores incorporate historical usefulness data, creating a compounding improvement loop.
- **Inputs:** `action` (record/query), `symbol_hash`, `session_id` (for record), `useful` (bool, for record).
- **Outputs:** For record: `{"status":"recorded"}`. For query: usefulness ratio and counts.
- **Dependencies:** SQLiteStore (RecordFeedback, QueryFeedback methods).

### 43a. Merkleized Feedback Validity (v0.5.0)

- **Package(s):** `internal/store`, `internal/mcp`
- **Entry point:** Migration 014, `RecordFeedback`, `FeedbackBoosts` in `internal/store/feedback.go`
- **What it does:** Feedback records now store `neighborhood_root` (SubgraphRoot of the symbol's package at feedback time). When querying feedback, only records where `neighborhood_root` matches the current SubgraphRoot are counted. This provides automatic expiration: feedback becomes invalid when the symbol's package changes (any edge modification in the package invalidates the neighborhood root).
- **Migration:** 014 adds `neighborhood_root BLOB` column and index to feedback table
- **Performance:** 11% overhead (255µs → 284µs for 100 symbols retrieving feedback boosts)
- **Backward compatibility:** NULL `neighborhood_root` = legacy path (no expiration)
- **Inputs:** Symbol hash, current SubgraphRoot map (package path → root hash)
- **Outputs:** Filtered feedback records (only matching neighborhood roots counted)
- **Why it matters:** Prevents stale feedback from influencing context ranking after code refactors. Feedback automatically expires when the local code neighborhood changes, without requiring manual cleanup or timestamp-based heuristics. Uses cryptographic identity (Merkle root) instead of timestamps for expiration.
- **Dependencies:** Hierarchical Merkle tree (`SubgraphRoot` computation), SQLiteStore feedback methods.

### 44. Test Scope MCP Tool (Affected Test Discovery)

- **Package(s):** `internal/mcp`
- **Entry point:** `test_scope` MCP tool
- **What it does:** Given a set of changed file paths, finds all symbols defined in those files via `NodesByFilePath`, then performs backward BFS through `calls` edges to discover test functions that transitively depend on the changed code. Outputs affected packages, function names, or a `go test -run` regex.
- **Inputs:** `files` (comma-separated paths), `output` (packages/functions/run), `depth` (max BFS depth, default 3).
- **Outputs:** JSON with mode, results array, and count.
- **Dependencies:** SQLiteStore (NodesByFilePath, AllRepos, EdgesTo).

### 45. Flow Between MCP Tool (Path Finding)

- **Package(s):** `internal/mcp`
- **Entry point:** `flow_between` MCP tool
- **What it does:** Finds all paths between two symbols using BFS traversal through the knowledge graph. Returns up to 10 paths with edge types at each step. Useful for understanding how two symbols are connected.
- **Inputs:** `source_symbol` (qualified name), `target_symbol` (qualified name), `max_depth` (default 5).
- **Outputs:** JSON with source, target, paths array, path_count, and truncated flag.
- **Dependencies:** GraphStore (NodesByQualifiedName, EdgesFrom).

### 46. Plan Turn MCP Tool (Task-to-Tool Recommender)

- **Package(s):** `internal/mcp`
- **Entry point:** `plan_turn` MCP tool
- **What it does:** Given a task description, extracts keywords and matches them against a static rule table to suggest which knowing MCP tools to call with pre-filled argument templates. Returns up to 4 ranked suggestions. Falls back to `context_for_task` if no specific tool matches.
- **Inputs:** `task` (task description string).
- **Outputs:** JSON with suggestions array (tool, reason, args).
- **Dependencies:** None (pure keyword matching).

### 47. Communities MCP Tool (Louvain Graph Clustering)

- **Package(s):** `internal/mcp`
- **Entry point:** `communities` MCP tool (action: "list" or "for_symbol")
- **What it does:** Detects communities in the knowledge graph using the Louvain modularity optimization algorithm. The `list` action returns all communities with top symbols, cohesion scores, dominant package labels, a `merkle_root` (Merkle root over the packages the community spans), and a `packages` list (sorted package paths). The `for_symbol` action returns the community containing a specific symbol and its neighboring communities.
- **Inputs:** `action` (list/for_symbol), `repo_url` (optional filter), `symbol` (required for for_symbol).
- **Outputs:** JSON with communities array or symbol-specific community result. Each community includes `merkle_root` and `packages` fields (Phase 2 Merkle, shipped).
- **Why it matters:** Disjoint `merkle_root` values across communities signal that two agents can work in parallel without Merkle-level conflicts. Community roots are also the natural invalidation scope: a change inside one community changes only that community's root, leaving all other community caches valid.
- **Dependencies:** GraphStore (NodesByName, EdgesFrom).

### 48. Cloud YAML Extractor (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless)

- **Package(s):** `internal/indexer/cloudextractor/`
- **Entry point:** `cloudextractor.NewCloudExtractor() *CloudExtractor`
- **What it does:** Single extractor that handles 4 cloud YAML formats via content-based detection (not file extension). Delegates to sub-extractors for CloudFormation/SAM, Docker Compose, GitHub Actions, and Serverless Framework. Produces resource nodes and relationship edges (`depends_on`, `calls`, `publishes`, `subscribes`, `connects_to`).
- **Inputs:** `ExtractOptions` with YAML file content.
- **Outputs:** `ExtractResult` with nodes (resources, functions, services, workflows) and edges.
- **Sub-extractors:** `cloudformation.go` (CFN/SAM), `compose.go` (Docker Compose), `actions.go` (GitHub Actions), `serverless.go` (Serverless Framework).
- **Dependencies:** `gopkg.in/yaml.v3`.

### 49. SCIP Ingest (External Dependency Symbols)

- **Package(s):** `internal/indexer/scipingest/`
- **Entry point:** `scipingest.NewSCIPIngester(store).IngestFile(ctx, SCIPIngestOptions) (*SCIPIngestResult, error)`
- **CLI:** `knowing ingest-scip -file <path> -repo <url>`
- **What it does:** Reads a `.scip` protobuf file produced by any SCIP-compatible indexer. Creates nodes for all symbol definitions and `references` edges for all symbol references found in the index. Enables cross-repo resolution without cloning and fully indexing external dependencies.
- **Inputs:** SCIP protobuf file path, repository URL.
- **Outputs:** `SCIPIngestResult` with `NodesCreated`, `EdgesCreated`, `DocsProcessed`.
- **Provenance:** `scip_resolved` (confidence 0.95).
- **Dependencies:** Protocol Buffers runtime for SCIP format parsing.

### 50. Dockerfile Extractor

- **Package(s):** `internal/indexer/dockerfileextractor/`
- **Entry point:** `dockerfileextractor.NewDockerfileExtractor() *DockerfileExtractor`
- **What it does:** Parses `Dockerfile` and `*.dockerfile` files. Extracts `FROM` base image dependencies, `COPY --from` multi-stage build references, and `EXPOSE` port declarations. Produces `depends_on` edges from build stages to base images and between multi-stage stages.
- **Edge types produced:** `depends_on` (FROM images, COPY --from stages).
- **Dependencies:** Dockerfile parser.

### 51. Makefile Extractor

- **Package(s):** `internal/indexer/makefileextractor/`
- **Entry point:** `makefileextractor.NewMakefileExtractor() *MakefileExtractor`
- **What it does:** Parses `Makefile`, `GNUmakefile`, and `*.mk` files. Extracts target definitions, prerequisite (dependency) lists, `include` directives, and variable references. Produces `depends_on` edges between targets and their prerequisites, and `imports` edges for `include` directives.
- **Edge types produced:** `depends_on` (target prerequisites), `imports` (include directives).
- **Dependencies:** Makefile parser.

### 52. Helm Chart Extractor

- **Package(s):** `internal/indexer/helmextractor/`
- **Entry point:** `helmextractor.NewHelmExtractor() *HelmExtractor`
- **What it does:** Parses Helm chart files (`Chart.yaml`, `values.yaml`, templates). Extracts chart metadata, dependency declarations, template references, and values injection points. Produces `depends_on` edges from charts to their declared dependencies.
- **Edge types produced:** `depends_on` (chart dependencies from Chart.yaml).
- **Dependencies:** `gopkg.in/yaml.v3`.

### 53. GitLab CI Extractor

- **Package(s):** `internal/indexer/gitlabciextractor/`
- **Entry point:** `gitlabciextractor.NewGitLabCIExtractor() *GitLabCIExtractor`
- **What it does:** Parses `.gitlab-ci.yml` and included CI configuration files. Extracts jobs, stages, `needs` dependencies between jobs, `extends` template inheritance, `include` file references, and artifact dependencies. Produces `depends_on` edges for `needs`, `extends` edges for template inheritance, and `imports` edges for includes.
- **Edge types produced:** `depends_on` (needs), `extends` (extends: .template), `imports` (include files).
- **Dependencies:** `gopkg.in/yaml.v3`.

### 54. package.json (npm) Extractor

- **Package(s):** `internal/indexer/npmextractor/`
- **Entry point:** `npmextractor.NewNpmExtractor() *NpmExtractor`
- **What it does:** Parses `package.json` files. Extracts `dependencies`, `devDependencies`, `peerDependencies`, and `scripts` declarations. Produces `depends_on` edges from the package to each declared dependency.
- **Edge types produced:** `depends_on` (npm dependencies).
- **Dependencies:** `encoding/json`.

### 55. GraphQL Extractor

- **Package(s):** `internal/indexer/graphqlextractor/`
- **Entry point:** `graphqlextractor.NewGraphQLExtractor() *GraphQLExtractor`
- **What it does:** Parses `.graphql` and `.gql` schema and operation files. Extracts type definitions (object types, input types, interfaces, enums, unions), field declarations, query/mutation/subscription operations, and directive usage. Produces `references` edges from fields to their type definitions, `implements` edges from object types to interfaces, and `calls` edges from operations to root type fields.
- **Edge types produced:** `references` (field type references), `implements` (type implements interface), `calls` (operation to field).
- **Dependencies:** GraphQL parser.

### 56. Ansible Extractor

- **Package(s):** `internal/indexer/ansibleextractor/`
- **Entry point:** `ansibleextractor.NewAnsibleExtractor() *AnsibleExtractor`
- **What it does:** Parses Ansible playbooks, roles, and task files (YAML). Extracts playbook-to-role references, task dependencies, variable references, and handler notifications. Produces `depends_on` edges from playbooks to roles and between tasks.
- **Edge types produced:** `depends_on` (role dependencies, task dependencies), `references` (variable references).
- **Dependencies:** `gopkg.in/yaml.v3`.

### 57. FindAllExtractors Multi-Dispatch

- **Package(s):** `internal/indexer/`
- **Entry point:** `FindAllExtractors(path string, extractors []types.Extractor) []types.Extractor` in `internal/indexer/extractor.go` (line 49)
- **Called from:** `extractFile` in `internal/indexer/indexer.go` (line 146)
- **What it does:** Returns all registered extractors whose `CanHandle` returns true for a given file path, rather than stopping at the first match. This enables event, schema, and infrastructure extractors to run alongside primary language extractors on the same file (e.g., a Go file that also contains Kafka producer calls gets processed by both the Go tree-sitter extractor and the event extractor).
- **Why it matters:** Without multi-dispatch, only one extractor could claim a file. With it, overlay extractors (event, schema) can detect cross-cutting patterns in files already handled by language extractors.

### 51. 5-Tier Context Engine Seeding

- **Package(s):** `internal/context/`
- **Entry point:** `seedsForTask` method in `internal/context/context.go`
- **What it does:** Identifies seed nodes for graph walking using five progressively broader matching tiers:
  1. **Exact match:** Node qualified name exactly matches a task keyword.
  2. **Prefix match:** Node qualified name starts with a task keyword.
  3. **Substring match:** Node qualified name contains a task keyword as a substring.
  4. **File-path matching:** Nodes whose file path matches task-referenced files.
  5. **Interface-aware seeding:** If a seed is an interface, its implementations are also seeded.
- **Why it matters:** Ensures the context engine finds relevant starting points even for vague task descriptions, while prioritizing precise matches higher in the ranking. Now fused as RRF Channel 1 (weight 2.0) alongside BM25, vector, and equivalence class channels.

### 52. Density-Ranked Knapsack Packing

- **Package(s):** `internal/context/`
- **Entry point:** `packIntoBudget` function in `internal/context/context.go`
- **What it does:** Given ranked symbols with scores and estimated token costs, selects the subset that maximizes total relevance within a token budget. Uses score/cost ratio (density) to greedily pack highest-value-per-token symbols first.
- **Why it matters:** Ensures LLM context windows are filled with maximum information density rather than simply taking the top-N symbols regardless of their serialization cost.

### 53. Per-Repo Database Isolation and KNOWING_DB Environment Variable

- **Package(s):** `cmd/knowing/`
- **Entry point:** `defaultDB()` function in `cmd/knowing/main.go`
- **What it does:** Each repo gets its own database at `~/.knowing/repos/<safe-name>.db`. `defaultDB()` checks the roster for the current directory's DB path; if no roster entry is found, it falls back to `~/.knowing/knowing.db`. The `KNOWING_DB` environment variable overrides this default. The `-db` flag overrides both. Falls back to creating `~/.knowing/` if it does not exist.
- **Why it matters:** Per-repo isolation prevents community detection, RWR, HITS, and BM25 from blending data across unrelated repositories. Cross-repo edges are a future feature (separate `cross-repo.db`).

### 74. Repo Roster (`knowing add`, `knowing remove`, `knowing list`)

- **Package(s):** `cmd/knowing/`
- **Entry point:** `knowing add [path]`, `knowing remove [path]`, `knowing list`
- **What it does:** Maintains a roster of registered repositories. `knowing add` registers a repo, assigns it a per-repo database at `~/.knowing/repos/<safe-name>.db`, and indexes it. `knowing remove` unregisters a repo (the per-repo database file is not deleted). `knowing list` prints all registered repos with path, URL, per-repo DB path, database file size, and last-indexed timestamp. `knowing init` also registers the repo in the roster.
- **Why it matters:** Provides a single command to onboard a repo with an isolated database. Each repo's graph algorithms operate only on its own data.

### 54. HITS Authority/Hub Scoring

- **Package(s):** `internal/context/`
- **Entry point:** `internal/context/hits.go`
- **What it does:** Implements the HITS (Hyperlink-Induced Topic Search) algorithm on the subgraph of candidate symbols. Computes authority and hub scores via iterative power iteration. Authority scores are used to re-rank symbols so that highly-referenced nodes (authorities) and highly-referencing nodes (hubs) surface appropriately in context output.
- **Dependencies:** Called from the ranking pipeline in `internal/context/ranking.go`.

### 55. Benchmark Harnesses (`bench/`)

- **Package(s):** `bench/`
- **What it does:** Six benchmark harnesses for measuring system quality:
  - `bench/feedback-loop/` : Measures agent feedback loop effectiveness (correct tool suggestions, learning rate).
  - `bench/context-relevance/` : Evaluates precision/recall of context engine output against ground-truth annotations.
  - `bench/token-savings/` : Measures token reduction from GCF session deduplication and knapsack packing.
  - `bench/edge-accuracy/` : Compares extracted edges against ground-truth call graphs (precision, recall, F1).
  - `bench/test-scope-accuracy/` : Validates test-scope output against actual test failures from mutation testing.
  - `bench/wire-format/` : Encode/decode performance and size comparison across all wire format codecs.
- **Why it matters:** Provides reproducible quality gates for context engine, extractors, and wire format changes.

### 56. BM25 Full-Text Search (FTS5 Index)

- **Package(s):** `internal/store`
- **Migrations:** `006_add_fts5_index.sql`, `016_fts_symbol_name.sql`, `017_fts_concepts_column.sql`
- **Entry point:** `RebuildFTS`, `SearchBM25Nodes`, `extractSymbolName` in `internal/store/sqlite.go`
- **What it does:** Creates an SQLite FTS5 virtual table (`nodes_fts`) over five columns: `symbol_name`, `concepts`, `qualified_name`, `signature`, and `file_path`. BM25 weights are: symbol_name=10x, concepts=5x, qualified_name=3x, signature=1x, file_path=1x. The `symbol_name` column (migration 016) stores just the terminal identifier (e.g., "QuerySet.filter") extracted by `extractSymbolName`, which strips repo URL, package path, and file extension prefix. The `concepts` column (migration 017) stores CamelCase-split tokens from file names and parent directories (e.g., "commandLineParser.ts" becomes "command Line Parser commandLineParser"), bridging the gap when developers say "parser" but the symbol is inside a differently-named file. FTS5 tokenizer uses `tokenchars '_'` so snake_case identifiers (e.g., `before_request`) match as single tokens. Uses CamelCase-aware tokenization (`splitForFTS`, `splitCamelCase`) so compound identifiers are searchable by individual terms. `RebuildFTS` runs synchronously after snapshot computation (was previously a background goroutine that was killed on CLI process exit, leaving FTS empty in `knowing index` mode). Adds ~500ms to index time.
- **Why it matters:** Improves recall for vague or partial task descriptions where substring matching alone misses relevant symbols. The high-weight `symbol_name` column ensures keyword searches match by actual symbol name rather than incidental path tokens. The `concepts` column provides vocabulary bridging from file/module names to symbols they contain, which is critical for non-Go repos where qualified names embed file paths.

### 57. Session-Aware Retrieval Boosts

- **Package(s):** `internal/context/`
- **Entry point:** `SessionTracker` in `internal/context/session.go`
- **What it does:** Tracks which symbols are returned by context queries during the current MCP server lifetime. Symbols accessed recently receive an exponential-decay boost (3-minute half-life, tuned for AI session cadence). The boost is capped at 2.0x and weighted at 0.20 in the ranking formula. One tracker is maintained per MCP server process, wired in the server constructor.
- **Why it matters:** Symbols the agent is actively working with surface higher in subsequent queries, reducing redundant graph traversal and improving relevance continuity within a session.

### 58. Noise Filtering (Mock/Stub/Fake Exclusion)

- **Package(s):** `internal/context/`
- **Entry point:** `filterNoisySymbols` in `internal/context/context.go`
- **What it does:** Removes low-signal symbols from context candidates before scoring. Excludes symbols with mock, fake, or stub in the qualified name (case-insensitive), and symbols whose file path contains `/build/` or `.bundle.` segments.
- **Why it matters:** Prevents test infrastructure and build artifacts from consuming token budget in context output.

### 59. Equivalence Class Seed Retrieval

- **Package(s):** `internal/context/`
- **Entry point:** `equivalence.go` in `internal/context/`
- **What it does:** Bridges the vocabulary gap between natural-language task descriptions and code symbol names. Contains 20 hand-curated concept classes (TRANSITIVE_IMPACT, SYMBOL_LOOKUP, DATAFLOW_TRACE, TEST_SELECTION, etc.) with 200+ phrases mapped to specific target symbols. Cross-product expansion with action verbs generates additional phrase variants. Fused as RRF Channel 4 with weight 2.0.
- **Why it matters:** Biggest single-feature improvement to retrieval accuracy. Hard tier P@10 rose from 10% to 18% (+8pp). Deterministic, zero dependencies, inspectable.

### 60. 4-Channel RRF Fusion

- **Package(s):** `internal/context/`
- **Entry point:** `rrfFuseMulti` in `internal/context/context.go`
- **What it does:** Replaces the previous single-channel tiered matching (with BM25 fallback) with a proper N-channel Reciprocal Rank Fusion system. Four channels: tiered keywords (weight 2.0), BM25 FTS5 (weight 2.0), vector/embedding search (weight 0.0, disabled), and equivalence class matching (weight 2.0). Each channel produces an independent ranked list; `rrfFuseMulti` merges them with per-channel weights. Weights were equalized (was 3:1:0:2) after cross-system benchmark showed BM25 and tiered find the same symbols.
- **Why it matters:** Enables systematic combination of heterogeneous retrieval signals. Symbols appearing in multiple channels get promoted. New retrieval methods can be added as channels without disrupting existing ones.

### 61. Doc Comment Extraction (Node.Doc Field)

- **Package(s):** `internal/indexer/gotsextractor`, `internal/store`
- **Migration:** `007_add_doc_column.sql`
- **Entry point:** `extractDocComment` in the Go tree-sitter extractor
- **What it does:** Extracts doc comments for functions, methods, and types using a language-agnostic function that walks tree-sitter `PrevSibling` nodes to collect adjacent comment blocks. Stored in the `Node.Doc` field (added by migration 007). Included in embedding text for future code-tuned models.
- **Why it matters:** Provides natural-language descriptions of symbols for embedding search and potential future BM25 enrichment.

### 62. BGE-small-en-v1.5 Embedding Model

- **Package(s):** `internal/context/`
- **What it does:** Replaces MiniLM-L6-v2 with BGE-small-en-v1.5 (384 dimensions, retrieval-tuned). Same infrastructure: hugot ONNX runtime, coder/hnsw index, RRF Channel 3. Currently disabled (weight 0.0) because off-the-shelf models tested net-negative on the eval.
- **Why it matters:** Infrastructure is preserved and ready to enable when a code-tuned model becomes available. Enable with `KNOWING_EMBEDDINGS=1`.

### 63. BFS Depth Limit on RWR Walk

- **Package(s):** `internal/context/`
- **Entry point:** `buildAdjacencyMap` in `internal/context/walk.go`
- **What it does:** Limits the BFS exploration in `buildAdjacencyMap` to 4 hops from seed nodes (previously unbounded). Reduces the size of the in-memory adjacency map for large graphs.
- **Why it matters:** Performance improvement for dense graphs without affecting ranking quality.

### 64. MCP Vector Index Notification

- **Package(s):** `internal/mcp/`
- **What it does:** Sends a `notifications/message` MCP notification when the vector index is ready after indexing completes.
- **Why it matters:** Allows MCP clients to know when embedding search is available without polling.

### 65. Multi-Language LSP Enrichment (Auto-Detection)

- **Package(s):** `internal/enrichment`
- **Entry point:** `DetectLSPServers` in `internal/enrichment/config.go`, `Enricher` in `internal/enrichment/enricher.go`
- **What it does:** Auto-detects language servers by checking project markers (`go.mod`, `tsconfig.json`, `pyproject.toml`, `Cargo.toml`, `pom.xml`, `*.csproj`) and PATH binaries. Supported servers: gopls, typescript-language-server, pylsp/pyright, rust-analyzer, jdtls, OmniSharp. Uses `LSPServerConfig` struct (`{command, extensions, language_id}`). Provides `SetLSPConfig` for explicit override and `LoadLSPConfig` for loading from `knowing-lsp.json`. Includes language-agnostic `openFilesForLanguage` and `isTestFile` (multi-language test detection). The enricher iterates all detected servers sequentially.
- **Why it matters:** Extends LSP enrichment beyond Go to all supported languages without manual configuration. Projects with multiple languages get enrichment across all of them in a single index run.

### 66. Passive Task Memory (Persistent)

- **Package(s):** `internal/context/`
- **Entry point:** `task_memory.go` in `internal/context/`
- **Migration:** `008_task_memory.sql` (creates `task_memory` table with columns: keywords, symbol_hash, score, timestamp)
- **What it does:** Records the top-5 symbols from each `context_for_task` call with boost score `0.5 + score * 0.4`. On subsequent calls, recall matches keywords against stored entries with a 7-day linear decay. Matched symbols receive a boost added to the `FeedbackBoost` channel at 0.3x scale. Persists across MCP server restarts via SQLite, so quality compounds with usage over time.
- **Why it matters:** Provides passive learning from agent behavior. Symbols that were relevant to similar tasks in the past surface higher in future queries, without requiring explicit agent feedback. Because it persists in SQLite, the system gets smarter across sessions, not just within a single process lifetime.

### 67. Universal Equivalence Classes

- **Package(s):** `internal/context/`
- **Entry point:** `universal_seeds.go` in `internal/context/`
- **What it does:** Defines 63 universal software concepts (authentication, caching, config, database, HTTP, testing, concurrency, etc.) as equivalence classes with weight 0.8 (between seed weight 1.0 and graph-derived weight 0.7). These are domain-agnostic patterns that apply across any codebase, complementing the 21 hand-curated knowing-specific equivalence classes in `equivalence.go` and 31 language-specific classes in `language_seeds.go` (115 total).
- **Why it matters:** Improves cross-repo retrieval accuracy. Cross-repo eval showed +6.7pp on the gortex benchmark (40% to 46.7%).

### 68. Graph-Derived Aliases

- **Package(s):** `internal/context/`
- **Entry point:** `graph_aliases.go` in `internal/context/`
- **What it does:** Auto-generates equivalence classes from caller/callee symbol names in the graph. Selects only the top-10 tiered candidates, weighted at 0.7. Derives vocabulary mappings from actual code relationships rather than hand-curated lists.
- **Why it matters:** Provides retrieval improvement for repos that lack hand-curated seed mappings. Designed as a zero-configuration fallback; marginal improvement observed on the knowing repo itself.

### 69. Cross-Repo Eval

- **Package(s):** `eval/`
- **Entry point:** `eval/crossrepo_test.go`
- **What it does:** Tests the retrieval pipeline on external Go codebases. Uses 30 fixtures adapted from the gortex project: 10 exact-match, 10 concept-match, and 10 multi-hop queries. Evaluates how well the context engine retrieves relevant symbols for tasks described in natural language against a codebase it was not tuned for.
- **Results:** exact 60%, concept 20%, multi_hop 60%, overall 46.7%.
- **Why it matters:** Validates that retrieval quality generalizes beyond the knowing repo itself. Provides a regression gate for changes to the context engine.

### 71. `explain_symbol` MCP Tool

- **Package(s):** `internal/mcp`
- **Entry point:** `handleExplainSymbol` in `internal/mcp/context_handlers.go`
- **What it does:** MCP equivalent of the `knowing why` CLI command. Given a task description and symbol name, runs the full retrieval pipeline and returns a Markdown-formatted scoring breakdown: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, and equivalence class matches.
- **Parameters:** `task_description` (required), `symbol` (required).
- **Why it matters:** Allows agents to programmatically inspect ranking behavior without invoking the CLI.

### 70. `knowing why` (Retrieval Explainability)

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing why -task "<task>" -symbol "<symbol>"`
- **What it does:** Runs the full retrieval pipeline for a given task description, then isolates and displays the scoring breakdown for one symbol. Shows: whether the symbol was a seed (and which channel/tier), RWR score, HITS authority/hub scores, blast radius (caller proxy and max), confidence, recency, distance, feedback weight, session boost, and equivalence class matches.
- **Inputs:** Task description (`-task`), symbol name (`-symbol` or positional), optional database path (`-db`).
- **Outputs:** Human-readable scoring breakdown printed to stdout.
- **Why it matters:** Every retrieval system needs an explain mode. Without it, ranking is a black box and bad recommendations cannot be debugged. This makes the pipeline inspectable and supports iterative tuning of equivalence classes, feedback, and scoring weights.

### 72. Enrich Blame (Git Authorship Metadata)

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing enrich blame [flags] <repo-path>` (in `cmd/knowing/enrich.go`)
- **Migration:** `009` (adds `last_author` and `last_commit_at` columns to nodes table)
- **What it does:** Runs `git blame --porcelain` on every file with nodes in the graph, then stamps each symbol with the author name and commit timestamp of the line where it is declared. Uses `UpdateNodeBlame` store method.
- **Inputs:** Repo path (positional), optional `-db` and `-url` flags.
- **Outputs:** Logs summary (symbols stamped, files skipped, errors).
- **Why it matters:** Enables ownership and recency queries at the symbol level, supporting "who last touched this?" and staleness detection without requiring full git log traversal at query time.

### 73. Enrich Coverage (Go Test Coverage Metadata)

- **Package(s):** `cmd/knowing`
- **Entry point:** `knowing enrich coverage [flags] <repo-path>` (in `cmd/knowing/enrich.go`)
- **Migration:** `010` (adds `coverage_pct` column to nodes table)
- **What it does:** Parses a Go cover profile (`go test -coverprofile`) and stamps each symbol with its coverage percentage. For each symbol, finds overlapping coverage blocks by line range and computes the ratio of covered to total statements. Symbols with no coverage data receive -1.
- **Inputs:** Repo path (positional), `-profile` (default `cover.out`), optional `-db` and `-url`.
- **Outputs:** Logs summary (symbols stamped, files without coverage data).
- **Why it matters:** Enables coverage-aware ranking and auditing. Agents can prioritize untested symbols for review or use coverage as a signal in context retrieval.

### 75. Content-Addressed Context Pack Roots (Phase 2 Merkle)

- **Package(s):** `internal/context`
- **Entry point:** `computePackRoot()` in `internal/context/context.go`; `PackRoot` field on `ContextBlock`
- **What it does:** Every `context_for_task` response carries a `PackRoot`: a deterministic SHA-256 hash derived from the normalized task description and the sorted hashes of all selected nodes. Same task against the same graph always produces the same `PackRoot`.
- **Inputs:** Task description string + selected node hash set (produced during knapsack packing).
- **Outputs:** `PackRoot` string on `ContextBlock`.
- **Benchmark:** 5 queries with 2 unique tasks = 2 unique PackRoots (perfect dedup). See `bench/merkle-diff/FINDINGS-context-packs.md`.
- **What it enables:** Cache lookup (skip retrieval if PackRoot seen before), citation by hash in code review, cross-session replay, context pack diffing between snapshots, feedback anchoring to a specific pack state.
- **Dependencies:** `internal/snapshot` (hash utilities).

### 76. Community Merkle Roots (Phase 2 Merkle)

- **Package(s):** `internal/mcp`
- **Entry point:** `communityInfo.MerkleRoot` and `communityInfo.Packages` fields in `internal/mcp/communities.go`
- **What it does:** After Louvain community detection, each community receives a Merkle root computed from the packages it spans. The `MerkleRoot` and `Packages` fields are included in every `communities` tool response.
- **Inputs:** Louvain output (community membership), package paths of member nodes.
- **Outputs:** `merkle_root` (string) and `packages` ([]string) per community in the `communities` tool JSON response.
- **Benchmark:** Community roots verified distinct per package set on live graph. See `bench/merkle-diff/FINDINGS-context-packs.md`.
- **What it enables:** Scoped cache invalidation (change in one community invalidates only that community's root), safe agent parallelization (disjoint roots = disjoint work), retrieval scoping (prefer seeds that share a community root with the task's primary symbols).
- **Dependencies:** `internal/community` (Louvain), `internal/snapshot` (hash utilities).

### 77. Hash Domain-Type Prefixes (BREAKING CHANGE)

- **Package(s):** `internal/types`
- **Entry point:** `ComputeNodeHash`, `ComputeEdgeHash`, `ComputeSnapshotHash`, `ComputeMerkleNodeHash` in `internal/types/types.go`
- **What it does:** Prepends a domain-type tag to every hash input before computing SHA-256: `node\0` for node hashes, `edge\0` for edge hashes, `snapshot\0` for snapshot hashes, and `merkle\0` for Merkle interior node hashes. This mirrors git's `"<type> <size>\0<content>"` object header, making cross-type hash collisions structurally impossible.
- **Why it matters:** Before this change, a snapshot root hash and a Merkle interior node hash could theoretically share a hash value (both are SHA-256 of binary data with no type tag). After this change, a node hash always begins with the compressed form of `"node\0..."` and cannot collide with an edge hash that begins with `"edge\0..."`.
- **Breaking change:** Any database populated before 2026-05-18 must be re-indexed. Gate this via schema migration that forces a full reindex on old databases.
- **Dependencies:** None beyond stdlib.

### 78. `VerifyNodeHash` / `VerifyEdgeHash` (Hash Integrity Functions)

- **Package(s):** `internal/types`
- **Entry point:** `VerifyNodeHash(n *Node) error`, `VerifyEdgeHash(e *Edge) error` in `internal/types/verify.go`
- **What it does:** Recomputes the hash from a stored node or edge's fields and compares to the stored hash. Returns an error if they differ, indicating the row was mutated after insertion. Used by `knowing fsck` and available for debug-mode integrity checking.
- **Why it matters:** Equivalent to git's `check_object_signature`. Enables detection of storage corruption, manual database edits, or bugs in the indexer that produce wrong hashes.
- **Dependencies:** `internal/types/types.go` (hash computation functions).

### 79. `knowing fsck` Integrity Checker

- **Package(s):** `cmd/knowing`, `internal/snapshot`
- **Entry point:** `knowing fsck [flags]` (in `cmd/knowing/fsck.go`); `Verify(ctx, repoHash) ([]VerifyError, error)` in `internal/snapshot/verify.go`
- **What it does:** Walks the graph and verifies: (1) edge referential integrity (every edge's source and target hashes exist in the nodes table), (2) node file integrity (every node's file hash exists in the files table), (3) hash recomputation (recomputes each node and edge hash from stored fields), (4) snapshot chain continuity (every parent pointer resolves), (5) SQLite page integrity via `PRAGMA integrity_check`.
- **Inputs:** Optional `-repo` to limit to one repo, `-quick` to run only the PRAGMA check.
- **Outputs:** Classified issues (ERROR or WARN) printed to stdout. Exit code 1 if any ERROR found.
- **Why it matters:** Equivalent to `git fsck`. Detects silent corruption, partial writes, and indexer bugs that would otherwise surface as wrong query results.
- **Dependencies:** GraphStore, `internal/types/verify.go`.

### 80. Daemon Lockfile (Prevent Multiple Instances)

- **Package(s):** `internal/daemon`
- **Entry point:** `internal/daemon/lockfile.go`
- **What it does:** Creates a lockfile at `<db_path>.lock` with the daemon PID on startup. On startup, checks whether the file exists and the PID is alive; if so, refuses to start with a clear error message. Lockfile removed on clean shutdown.
- **Why it matters:** Without this, two `knowing watch` processes on the same database compete at the SQLite WAL level: both respond to the same commit events, double-index, and interleave writes. This produces undefined graph state. The lockfile prevents the issue structurally.
- **Dependencies:** None beyond stdlib.

### 81. GC Reachability Sweep (`GarbageCollectFull`)

- **Package(s):** `internal/snapshot`, `internal/store`
- **Entry point:** `GarbageCollectFull(ctx, keepCount int) (GCStats, error)` in `internal/snapshot/gc.go`
- **What it does:** After deleting old snapshots (beyond `keepCount`), collects all node and edge hashes referenced by the surviving snapshots. Calls `DeleteNodesNotIn` and `DeleteEdgesNotIn` on the store to prune unreferenced rows. Returns `GCStats` with counts of deleted nodes, deleted edges, and deleted snapshots.
- **Why it matters:** The previous `GarbageCollect` only deleted snapshot rows; nodes and edges from deleted snapshots remained in the tables forever. On repos that are refactored frequently (functions renamed, packages restructured), the `nodes` table would grow without bound and degrade query performance.
- **Dependencies:** GraphStore (`DeleteNodesNotIn`, `DeleteEdgesNotIn`).

### 82. `DeleteNodesNotIn` / `DeleteEdgesNotIn` (GraphStore Additions)

- **Package(s):** `internal/store`, `internal/types`
- **Entry point:** `DeleteNodesNotIn(ctx, reachable []Hash) (int, error)`, `DeleteEdgesNotIn(ctx, reachable []Hash) (int, error)` on `SQLiteStore`
- **What it does:** Deletes all rows in the `nodes` (or `edges`) table whose primary key is not in the provided set. Implemented using a SQLite temporary table and a DELETE with a NOT IN subquery for efficiency.
- **Why it matters:** Required for the GC reachability sweep. These methods are added to the `GraphStore` interface so that any future store implementation must support GC.
- **Dependencies:** `modernc.org/sqlite`.

### 83. `indexed_at` Epoch Column (Migration 011)

- **Package(s):** `internal/store`
- **Migration:** `011_add_indexed_at.sql`
- **What it does:** Adds an `indexed_at INTEGER` column to both the `nodes` and `edges` tables. The indexer sets this to the current Unix timestamp on every insert or update. `GarbageCollectFull` uses this column to identify rows from superseded index runs that are no longer referenced by any surviving snapshot.
- **Why it matters:** Provides a freshness signal analogous to git's mtime freshening of loose objects. Objects with a recent `indexed_at` are actively referenced; objects with an old `indexed_at` that are not in the reachable set are safe to prune.
- **Dependencies:** `internal/store/migrate.go`.

### 84. In-Process LRU Cache on SQLiteStore

- **Package(s):** `internal/store`
- **Entry point:** `SQLiteStore.GetNode`, `SQLiteStore.GetEdge` in `internal/store/sqlite.go`
- **What it does:** Maintains a `sync.Map`-based LRU cache capped at 50K entries for `GetNode` and `GetEdge` lookups. On cache hit, returns immediately without a SQL round-trip. On cache miss, executes the SQL query and populates the cache. Cache invalidated at the start of each index run.
- **Why it matters:** Hot-path traversals (blast radius BFS, RWR walks, HITS subgraph construction) visit the same nodes repeatedly. Without caching, each visit incurs a SQL round-trip. At 50K entries, the cache covers the working set of any typical index-time query without unbounded memory growth.
- **Dependencies:** `sync` (stdlib).

### 85. `PRAGMA integrity_check` (`IntegrityCheck` Method)

- **Package(s):** `internal/store`
- **Entry point:** `IntegrityCheck(ctx) error` on `SQLiteStore` in `internal/store/sqlite.go`
- **What it does:** Runs `PRAGMA integrity_check` against the SQLite database and returns an error if any page-level corruption is detected (truncated pages, mismatched page checksums, malformed B-tree structure).
- **Why it matters:** Detects filesystem-level corruption before the application layer sees inconsistent data. Called by `knowing fsck --quick` and available for startup health checks.
- **Dependencies:** `modernc.org/sqlite`.

### 86. Modular Community Detection (`internal/community/`)

- **Package(s):** `internal/community`
- **Entry point:** `Algorithm` interface, `Register(name, factory)`, `New(name, graph) (Algorithm, error)` in `internal/community/registry.go`
- **What it does:** Defines an `Algorithm` interface with a single `Detect(graph) ([]Community, error)` method. A global registry maps algorithm names to factory functions. Registered algorithms: `louvain` (standard Louvain modularity), `louvain-fine` (higher-resolution Louvain), `label-propagation`. The `communities` MCP tool and `knowing export --algorithm` flag select among registered algorithms; new implementations register via `Register()` without changing any callers.
- **Why it matters:** Decouples algorithm selection from callers. Adding Leiden or any other community detection algorithm is a single `Register()` call.
- **Dependencies:** `internal/types` (graph primitives).

### 87. `DiffHierarchicalTreesWithOptions` (Package Filter + MaxChanges Cap)

- **Package(s):** `internal/snapshot`
- **Entry point:** `DiffHierarchicalTreesWithOptions(old, new *HierarchicalTree, opts DiffOptions) (*HierarchicalDiff, error)` in `internal/snapshot/hierarchical.go`
- **What it does:** Extends `DiffHierarchicalTrees` with a `DiffOptions` struct carrying: `PackageFilter []string` (limit the diff to named packages only, skipping all others) and `MaxChanges int` (return early and mark `Truncated: true` once this many changed edge-type roots are found).
- **Why it matters:** Mirrors git's pathspec filtering and `max_changes` early-exit from `tree-diff.c:462-465`. Allows tools like `blast_radius` to request a diff scoped to a single package without re-traversing the full tree, and prevents callers from being overwhelmed when a large batch commit touches many packages.
- **Dependencies:** `internal/snapshot/hierarchical.go`.

### 88. knowing-viz: Full React Migration

- **Package:** knowing-viz (separate repo, referenced from `docs/roadmap.md`)
- **What it does:** Complete migration from vanilla JS to React with Zustand state management, `@react-sigma/core` for 2D sigma.js rendering, and `react-force-graph-3d` for 3D force-directed rendering. Framer Motion provides transition animations. The graph rendering and state management are now fully decoupled from the data layer.
- **New features shipped with the migration:**
  - **Modular grouping system** (`src/grouping.ts`): 6 pluggable strategies (by package, community, edge type, file, author, none).
  - **Provenance edge filtering with counts**: per-tier counts shown in sidebar; toggle tiers on/off.
  - **Edge type filtering**: filter by `calls`, `references`, `throws`, `implements`, etc.
  - **Timeline snapshot picker**: select any snapshot from the chain; shows commit, timestamp, and node/edge counts.
  - **Blame author click-to-filter**: click any author name to filter graph to their symbols.
  - **Configurable max groups slider**: range 1-100 (was hardcoded at 20).
- **Why it matters:** Makes knowing-viz maintainable and extensible. React component model aligns with the modular grouping system. Zustand eliminates prop-drilling across deep component trees.

### 89. MCP Resources (8 Resources)

- **Package:** `internal/mcp/resources.go`
- **Registration:** `NewServer` in `internal/mcp/server.go`
- **What it does:** Exposes 8 read-only MCP resources that the MCP host can fetch without consuming a tool call. Resources are registered via `mcp-go`'s resource and resource template APIs.
- **Resources shipped:**
  - `knowing://report`: graph size, top node kinds, hotspot count, snapshot age. Session-opener orientation.
  - `knowing://schema`: node kinds, edge types, provenance tiers, qualified-ID hash format.
  - `knowing://stats`: node and edge counts broken down by repo and kind.
  - `knowing://repos`: all tracked repositories with node/edge counts and last-indexed timestamps.
  - `knowing://session`: live session metrics: context calls made, symbols served, cache hits/misses, uptime.
  - `knowing://index-health`: per-repo health status (healthy/stale/corrupted) and integrity check results.
  - `knowing://communities`: community list with cohesion scores and Merkle roots.
  - `knowing://community/{id}`: single community detail via resource template. Numeric ID resolves to members, key files, and cross-community connections.
- **Session counters:** `contextCalls` and `symbolsServed` are `sync/atomic` counters on the MCP Server struct. They are incremented in the context tool handlers (`context_for_task`, `context_for_files`, `context_for_pr`) and read by `knowing://session`.
- **Why it matters:** Agents can orient to the graph without spending a tool call. The session resource lets agents and users see accumulated value (how many symbols have been served this session).

### 90. Graph Notes Table (Phase 3 Foundation F1)

- **Package(s):** `internal/store`, `internal/types`
- **Migration:** `internal/store/migrations/012_add_notes.sql`
- **What it does:** General-purpose metadata layer that never affects Merkle computation. Attaches arbitrary key/value pairs to any content-addressed object (node, edge, snapshot, community, pack root). Modeled after git notes: a parallel metadata layer that never changes the identity of the object it annotates. Composite primary key `(object_hash, key)` with `INSERT OR REPLACE` upsert semantics matching the rest of the schema.
- **Type:** `types.Note` struct with `ObjectHash Hash`, `Key string`, `Value string`, `UpdatedAt int64`.
- **Interface methods (6):** `PutNote`, `GetNote`, `GetNotes`, `GetNotesByKey`, `DeleteNote`, `DeleteNotesByObject` on `GraphStore`.
- **Use cases:** Community assignment persistence (P1), context pack persistence (P2), quality scores, feedback annotations, agent session state.
- **Why it matters:** Foundation for all Phase 3 incremental recompute features. Notes store derived state (community assignments, cached context packs) without polluting the content-addressed identity layer. When a symbol's hash changes, its notes are naturally orphaned (no stale metadata).
- **Inputs:** `Note` struct with object hash, string key, string value, unix timestamp.
- **Outputs:** Standard GraphStore return patterns: `*Note` (nil if not found), `[]Note`, `error`.
- **Tests:** 8 tests in `internal/store/notes_test.go`: put/get, not-found, upsert semantics, get-all-for-object, get-by-key across objects, delete single, delete all for object, cross-object isolation.
- **Dependencies:** None beyond existing SQLite store.

### 91. Incremental Algorithm Interface (Phase 3 F2)

- **Package:** `internal/community`
- **What it does:** `IncrementalAlgorithm` interface with `DetectIncremental(g, previous, changedNodes)`. Seeds from previous community assignments, freezes unchanged nodes. Implemented on both Louvain (6.9x faster) and LabelPropagation (38.4x faster) for single-package changes.
- **Tests:** 8 tests in `internal/community/community_test.go`. Compile-time interface satisfaction checks.
- **Benchmark:** `bench/community-detection/bench_test.go` with performance contracts.

### 92. Community Assignment Persistence (Phase 3 P1)

- **Package:** `internal/community/persistence.go`
- **What it does:** `SaveAssignments`/`LoadAssignments`/`SaveChangedAssignments` persist community assignments to the notes table. `BatchPutNotes` on SQLiteStore wraps inserts in a single transaction (21x faster than individual inserts). Delta-save writes only changed assignments (5.0x e2e speedup).
- **Tests:** 7 tests in `internal/community/persistence_test.go`: save/load, empty, upsert, delta-only, deletes-removed, no-changes, incremental round-trip.
- **Benchmark:** `bench/community-detection/e2e_bench_test.go` with performance contracts.

### 93. Scoped FTS Rebuild (Phase 3 F3)

- **Package:** `internal/store/sqlite.go`
- **Entry point:** `SQLiteStore.RebuildFTSForPackages(ctx, packages []string)`
- **What it does:** Deletes and re-inserts FTS rows only for nodes whose qualified name starts with a changed package prefix. Falls back to full `RebuildFTS` if packages is empty. 2.9x faster than full rebuild for single-package edits.
- **Benchmark:** `bench/merkle-diff/fts_scoped_test.go` with performance contract.

### 94. Context Pack Persistence (Phase 3 P2)

- **Package:** `internal/context/context.go`
- **What it does:** Three-layer cache for `context_for_task`: SubgraphCache (42ns, in-memory) -> notes table (1.2ms, persists across restarts, snapshot-validated) -> cold retrieval (~160ms). Stored packs include snapshot hash for staleness detection. Cross-session replay verified.
- **Benchmark:** `bench/merkle-diff/context_pack_test.go` (TestContextPackPersistence).

### 95. Context Pack Deduplication (Phase 3 P5)

- **Package:** `internal/mcp/context_handlers.go`, `internal/wire/gcf.go`, `internal/wire/json.go`, `internal/wire/session.go`
- **What it does:** `context_for_task` MCP tool accepts optional `pack_root` parameter. If the current result's PackRoot matches, returns "unchanged" (165 bytes) instead of the full payload (2-30KB). PackRoot exposed in GCF header line and JSON `pack_root` field. 93-99% byte savings.
- **Tests:** `TestHandleContextForTask_PackRootDedup` in `internal/mcp/context_handlers_test.go`.
- **Benchmark:** `bench/merkle-diff/dedup_bench_test.go` with >90% savings contract.

### 96. Context Pack Comparison (Phase 3 P6)

- **Package:** `internal/context/pack_compare.go`
- **Entry point:** `CompareContextPacks(old, new *ContextBlock) PackDiff`
- **What it does:** Computes symmetric difference between two context blocks: added, removed, and common symbols. Answers "what changed in the context this agent would see?"
- **Tests:** 7 tests in `internal/context/pack_compare_test.go`.

### 97. Semantic Change Classification (Phase 3 P7)

- **Package:** `internal/snapshot/classify.go`
- **Entry point:** `ClassifyChanges(diff *HierarchicalDiff) ChangeClassification`
- **What it does:** Categorizes changes as Behavioral (calls, throws, publishes), Structural (imports, implements, extends), RuntimeDrift (runtime_calls, runtime_rpc), or MetadataOnly (references, handles_route). Added/removed packages count as structural. Highest severity wins.
- **Tests:** 10 tests in `internal/snapshot/classify_test.go`.

### 98. Incremental Louvain E2E (Phase 3 P3)

- **Package:** `internal/daemon/daemon.go`
- **What it does:** After each re-index, the daemon: computes Merkle diff (ChangedPackages), loads previous community assignments from notes, marks nodes in changed packages as movable, runs DetectIncremental, saves via delta-save. Full cycle: 2.5ms.

### 99. Incremental HITS/BM25 (Phase 3 P4)

- **Package:** `internal/daemon/daemon.go`
- **What it does:** After re-index, daemon calls `RebuildFTSForPackages` with changed packages from the Merkle diff. BM25 index rebuild scoped to changed packages.

### 100. Merkle Proofs (Phase 4)

- **Package:** `internal/snapshot/proof.go`
- **Entry points:** `GenerateProof(tree, edgeHash, packagePath, edgeType, edges)`, `VerifyProof(proof)`
- **What it does:** Three-level proof path: edge -> edge-type root -> package root -> repo root. Each level includes binary Merkle tree sibling hashes. Verifier recomputes root from edge hash and proof steps; any tampering fails verification. Proof size is logarithmic: 16 steps for 12K edges.
- **Performance:** Generation 72us, verification 1.2us on live graph.
- **Tests:** 9 tests in `internal/snapshot/proof_test.go`: single/multiple/many edges, tampering detection, not-found, nil tree, proof size.
- **Benchmark:** `bench/merkle-diff/proof_bench_test.go` with contracts (generation < 10ms, verification < 100us).

### 101. Canonical Package Path Extraction

- **Package:** `internal/snapshot/manager.go`
- **Entry point:** `ExtractPackagePath(qualifiedName string) (string, error)` (exported), `CollectEdgeInputs(ctx, repoHash) ([]EdgeInput, int, error)` (exported)
- **What it does:** Single canonical source for extracting package paths from qualified names and collecting edge inputs for tree construction. Previously 6 duplicate implementations existed; all consolidated to use these functions.

### 102. `knowing stats` CLI

- **Package:** `cmd/knowing/main.go` (cmdStats function)
- **What it does:** Prints cumulative graph statistics (repos, nodes, edges, files, snapshots, communities, graph notes) and feedback metrics (total, useful, not useful, unique symbols, merkleized count, usefulness rate).
- **Flags:** `-db` (database path), `-json` (structured output).
- **Why it matters:** Surfaces accumulated value of the knowledge graph without requiring MCP access. Shows how much feedback has been merkleized (neighborhood_root stored) vs. legacy.

### 103. Named Snapshot Refs

- **Package:** `cmd/knowing/main.go` (resolveSnapshotRef helper)
- **What it does:** Resolves human-friendly refs to snapshot hashes in `diff` and `audit-diff` commands. Supports `@latest`, `@prev`, `@first`, `@N` (Nth from most recent, 0-indexed).
- **Inspired by:** git's ref system (HEAD, HEAD~1, HEAD~N).
- **Why it matters:** Eliminates the need to copy 64-character hex hashes. `knowing diff @prev @latest` replaces `knowing diff <64 hex> <64 hex>`.

### 104. Generation Numbers on Snapshots

- **Package:** `internal/snapshot/manager.go`, `internal/types/types.go`
- **Migration:** `internal/store/migrations/015_snapshot_generation.sql`
- **What it does:** Each snapshot stores a `Generation` integer (`parent.Generation + 1`). Enables O(1) ancestry checks without walking the snapshot chain.
- **Inspired by:** git's commit-graph `generation_number`.
- **Why it matters:** Prunes chain walks during diff, GC, and bisection operations. At 1000+ snapshots, avoids O(N) linear scans.

### 105. Auto-GC Threshold

- **Package:** `internal/indexer/indexer.go` (maybeAutoGC function)
- **Constants:** `autoGCThreshold = 5000` (edge_events count), `autoGCKeepCount = 10` (snapshots preserved).
- **What it does:** After each index run, checks if edge_events exceed threshold. If so, triggers `GarbageCollect` keeping the 10 most recent snapshots and pruning orphaned nodes/edges.
- **Inspired by:** git's `gc.auto` threshold (6,700 loose objects triggers gc).
- **Why it matters:** Prevents unbounded edge_events growth and database bloat without requiring manual `knowing gc` invocations.

### 106. Merkle-Forest Library Extraction

- **Dependency:** `github.com/blackwell-systems/merkle-strata` v0.1.1
- **What it does:** Core Merkle tree construction (`computeMerkleRoot`, binary tree building, multi-level tree building) extracted to a standalone reusable library. `BuildMerkleTree` delegates to `forest.Build` with `WithPrefix([]byte("merkle\x00"))`. `BuildHierarchicalTree` delegates to `forest.BuildMultiLevel`. All exported APIs preserved unchanged.
- **Net impact:** -44 lines from knowing; construction logic reusable by other projects.
- **Library source:** https://github.com/blackwell-systems/merkle-strata

### 107. Cross-File Import Resolution (Python, TypeScript, Rust)

- **Package(s):** `internal/indexer/treesitter`, `internal/indexer/tsextractor`, `internal/indexer/rustextractor`
- **What it does:** Resolves call edges through import maps so that cross-file function calls produce proper `calls` edges to the definition rather than dangling references. Each language extractor builds an import map from `import`/`use` declarations, then resolves unresolved call targets through that map.
  - **Python:** `buildPythonImportMap` extracts `import X` and `from X import Y` statements. `resolveCallTarget` resolves call edges through the map. 63 resolved edges on Flask.
  - **TypeScript:** `buildTSImportMap` extracts `import`/`require` declarations. `resolveCallEdgeWithImports` resolves call targets. 5,684 resolved edges on TypeScript compiler.
  - **Rust:** `buildRustImportMap` extracts `use` declarations. `resolveCallEdgeWithImports` resolves `crate::`, `super::`, `self::` path prefixes. 9,795 resolved edges on Cargo.
- **Why it matters:** More resolved edges means the RWR walk has more paths to traverse, improving recall on cross-file tasks. Cross-system benchmark P@10 improved from 0.147 (Run 8) to 0.154 (Run 10) with Python + TS import resolution enabled. The critical finding: RWR (graph traversal) is the primary differentiator; import resolution helps because it creates more edges for RWR to walk.

### 108. Inheritance Propagation (Language-Agnostic)

- **Package(s):** `internal/indexer/indexer.go` (post-processing pass)
- **Entry point:** `propagateInheritance` called after extraction completes
- **What it does:** Finds all `extends` edges and creates `inherits` edges from child classes to parent class methods. Uses import-resolved qualified names to match extends edge targets to actual class node hashes. Works on any language whose extractor produces `extends` edges and `method` nodes (Python, TypeScript, Java, C#, Rust).
- **Results:** 83 edges in Flask, 14,539 edges in Django (deep class hierarchies). P@10 jumped from 0.155 to 0.200 (+29%), the largest single improvement in the cross-system benchmark (Run 13, d=0.81).
- **Why it matters:** Enables RWR to walk from child classes to parent class methods via inheritance chains. Previously, searching for "QuerySet.filter" only found the file defining QuerySet; now it also reaches every Model subclass that inherits filter.

### 109. Deeper Call Chain Extraction (Python)

- **Package(s):** `internal/indexer/treesitter`
- **What it does:** Walks into call arguments to extract nested calls, callbacks, and lambda references. Lambda bodies (`lambda: get_users()`) are walked for calls. Nested function bodies are walked with import resolution context (pyImports preserved). Previously `map(process, items)` only extracted the `map` call, missing `process` as a target.
- **Results:** Flask: 5,022 to 9,237 edges (+84%). Django: 151,431 to 185,393 edges (+22%).
- **Why it matters:** More call edges means more RWR connectivity, improving recall on cross-file tasks that involve higher-order patterns (callbacks, decorators, factory functions).

### 110. Test File Deprioritization

- **Package(s):** `internal/context/context.go`
- **Entry point:** `isTestFilePath(qualifiedName string) bool`
- **What it does:** Applies a 0.3x score penalty to symbols from test files during ranking. Detection is path-based: `/tests/`, `_test.go`, `.test.ts`, `.spec.ts`, `/__tests__/`, and similar conventions. The penalty is removed when the task description mentions testing (conditional, not absolute). Avoids false positives on production code with "test" in legitimate names.
- **Why it matters:** Failure analysis (Run 12) showed 36% of top-10 misses were test symbols crowding out production code. Deprioritization keeps test symbols available but ranks them below equally-scored production symbols.

### 111. Compound-First Keyword Extraction

- **Package(s):** `internal/context/`
- **Entry point:** `KeywordSet` struct and `tieredSearchSet` method in `internal/context/context.go`
- **What it does:** Replaces flat keyword lists with a structured `KeywordSet` containing three tiers: Exact (backtick-quoted identifiers from task descriptions, highest priority), Compounds (snake_case, CamelCase, dotted identifiers preserved whole), and Components (split words, lowest priority). The `tieredSearchSet` method queries compounds first and only falls back to components when fewer than 5 results are found. Backtick-quoted identifiers (e.g., `` `buildPythonImportMap` ``) bypass normal extraction and become Exact keywords, ensuring precise symbol targeting. This unified implementation replaces both the ForTask inline search and the ExplainSymbol method's separate logic, and fixes bm25Search to use `buildFTSQuery` everywhere.
- **Results:** Flask P@10: 0.321 to 0.329.
- **Why it matters:** Reduces noise from over-split identifiers. Previously, `route_handler` would match any symbol containing "route" or "handler"; now it first tries the compound `route_handler` and only splits if needed. Backtick quoting gives agents a way to request exact symbol lookups without ambiguity.

### 112. Cross-File Import Resolution (Java, C#)

- **Package(s):** `internal/indexer/javaextractor`, `internal/indexer/csharpextractor`
- **What it does:** Extends cross-file import resolution to the remaining OOP languages. `buildJavaImportMap` handles `import com.pkg.Class` and `import static` declarations. `buildCSharpImportMap` handles `using Namespace` and `using static` declarations. Both resolve call targets through the import map with provenance `ast_resolved` / confidence 0.85. Completes import resolution for all 5 OOP languages (Python, TypeScript, Rust, Java, C#).
- **Why it matters:** The cross-system benchmark corpus now includes Java (Spark) and C# (Ocelot) repos. Import resolution is required for RWR to traverse cross-file call edges in these languages. Without it, call targets are dangling hashes that never connect to definition nodes.

### 113. Daemon Lifecycle Commands

- **Package(s):** `cmd/knowing`, `internal/daemon`
- **Entry point:** `knowing daemon start|stop|status|restart`
- **What it does:** Provides explicit lifecycle management for the knowing daemon process. `start` launches the daemon (with optional `--detach` for background mode), `stop` sends SIGTERM via PID, `status` reports whether the daemon is running, and `restart` cycles stop/start. PID file stored at `~/.knowing/daemon.pid`.
- **Implementation:** `cmd/knowing/daemon.go` (CLI subcommand dispatch), `internal/daemon/pidfile.go` (PID file read/write/lock).
- **Limitations/known gaps:** None known.

### 114. `untrack_repo` (MCP Tool + CLI)

- **Package(s):** `internal/store`, `internal/mcp`
- **Entry point:** `knowing remove <path-or-url>` (CLI), `untrack_repo` MCP tool
- **What it does:** Evicts all data for a repository from the knowledge graph: nodes, edges, files, snapshots, feedback, task_memory, and graph_notes. The CLI command also removes the repo from the roster. Available as the 28th MCP tool (`untrack_repo`) with parameter `repo_url` (required).
- **Implementation:** `internal/store/evict.go` (data eviction logic), `internal/mcp/untrack.go` (MCP tool handler).
- **Limitations/known gaps:** Does not delete the per-repo database file unless `--purge` is passed (CLI only).

### GraphStore (`internal/types/interfaces.go`)

All 33 methods:

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
| DeleteSnapshot | `(ctx, Hash) error` | SQLiteStore | snapshot manager (GarbageCollect) |
| DeleteNodesNotIn | `(ctx, []Hash) (int, error)` | SQLiteStore | snapshot manager (GarbageCollectFull) |
| DeleteEdgesNotIn | `(ctx, []Hash) (int, error)` | SQLiteStore | snapshot manager (GarbageCollectFull) |
| IntegrityCheck | `(ctx) error` | SQLiteStore | knowing fsck |
| NodesByFilePath | `(ctx, string, string) ([]Node, error)` | SQLiteStore | context engine, test scope |
| PutNote | `(ctx, Note) error` | SQLiteStore | (Phase 3 consumers) |
| GetNote | `(ctx, Hash, string) (*Note, error)` | SQLiteStore | (Phase 3 consumers) |
| GetNotes | `(ctx, Hash) ([]Note, error)` | SQLiteStore | (Phase 3 consumers) |
| GetNotesByKey | `(ctx, string) ([]Note, error)` | SQLiteStore | (Phase 3 consumers) |
| DeleteNote | `(ctx, Hash, string) error` | SQLiteStore | (Phase 3 consumers) |
| DeleteNotesByObject | `(ctx, Hash) error` | SQLiteStore | (Phase 3 consumers) |
| Close | `() error` | SQLiteStore | cmd/knowing |

### Extractor (`internal/types/interfaces.go`)

| Method | Signature |
|--------|-----------|
| Name | `() string` |
| CanHandle | `(path string) bool` |
| Extract | `(ctx, ExtractOptions) (*ExtractResult, error)` |

Implementors: `gotsextractor.GoTreeSitterExtractor`, `goextractor.GoExtractor`, `treesitter.TreeSitterExtractor`, `tsextractor.TypeScriptExtractor`, `rustextractor.RustExtractor`, `javaextractor.JavaExtractor`, `csharpextractor.CSharpExtractor`, `terraformextractor.TerraformExtractor`, `sqlextractor.SQLExtractor`, `k8sextractor.K8sExtractor`, `cssextractor.CSSExtractor`, `protoextractor.ProtoExtractor`, `eventextractor.EventExtractor`, `schemaextractor.SchemaExtractor`
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

### TraceIngestor (`internal/trace/types.go`)

| Method | Signature |
|--------|-----------|
| IngestSpans | `(ctx, []TraceSpan) (IngestResult, error)` |
| IngestHTTPLogs | `(ctx, []HTTPLogEntry) (IngestResult, error)` |
| RuntimeEdgeStats | `(ctx, Hash) (*RuntimeStats, error)` |
| DecayConfidence | `(ctx) (int, error)` |

Implementors: `trace.Ingestor`
Consumers: `trace.OTLPReceiver`, `daemon.traceIngestLoop`

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

| # | Tool Name | Handler | Status | Store Methods Called | Description |
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
| 12 | `runtime_traffic` | `handleRuntimeTraffic` | FUNCTIONAL | `SQLiteStore.RuntimeEdgesByService` | Queries runtime edges by service and route pattern |
| 13 | `dead_routes` | `handleDeadRoutes` | FUNCTIONAL | `SQLiteStore.DeadRoutes` | Finds routes with no observations in N days |
| 14 | `trace_stats` | `handleTraceStats` | FUNCTIONAL | `SQLiteStore.RuntimeEdgeStatsAggregate` | Aggregate runtime edge statistics |
| 15 | `context_for_task` | `handleContextForTask` | FUNCTIONAL | `NodesByName`, graph traversal, HITS reranking | Returns ranked symbols relevant to a natural-language task description, encoded via wire format codec. Uses HITS authority scores to promote structurally important nodes. |
| 16 | `context_for_files` | `handleContextForFiles` | FUNCTIONAL | `NodesByName`, `EdgesFrom`, file lookups | Returns symbols and edges relevant to specified file paths, encoded via wire format codec |
| 17 | `context_for_pr` | `handleContextForPR` | FUNCTIONAL | `NodesByName`, `EdgesFrom`, file lookups, RWR scoring | Returns PR-scoped context: symbols in changed files plus their structural impact neighborhood (callers, callees, related types) |
| 18 | `feedback` | `handleFeedback` | FUNCTIONAL | `SQLiteStore.RecordFeedback`, `SQLiteStore.QueryFeedback` | Record or query symbol usefulness feedback |
| 19 | `test_scope` | `handleTestScope` | FUNCTIONAL | `NodesByFilePath`, BFS backward through calls edges | Find tests affected by changes to given files |
| 20 | `flow_between` | `handleFlowBetween` | FUNCTIONAL | `NodesByName`, BFS traversal | Find paths between two symbols |
| 21 | `plan_turn` | `handlePlanTurn` | FUNCTIONAL | keyword matching | Suggest which knowing MCP tools to call for a task |
| 22 | `communities` | `handleCommunities` | FUNCTIONAL | Louvain clustering on graph edges | Detect densely-connected symbol communities |
| 23 | `explain_symbol` | `handleExplainSymbol` | FUNCTIONAL | Full retrieval pipeline (RWR, HITS, scoring) | Explain why a symbol ranked where it did for a task |

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
- `runtime_traffic`: service_name (required), route_pattern (optional, LIKE syntax), limit (optional, default 100)
- `dead_routes`: stale_days (optional, default 30)
- `trace_stats`: (no parameters)
- `context_for_task`: task_description (required), token_budget (optional, default 50000), format (optional, default "xml"; accepts "gcf", "gcb", "json", "xml", "markdown")
- `context_for_files`: files (required, comma-separated file paths), repo_url (optional), token_budget (optional, default 50000), format (optional, default "xml"; accepts "gcf", "gcb", "json", "xml", "markdown")
- `context_for_pr`: files (required, comma-separated file paths), repo_url (optional), token_budget (optional, default 8000), format (optional, default "xml"; accepts "gcf", "gcb", "json", "xml", "markdown")
- `feedback`: action (required, "record" or "query"), symbol_hash (required), session_id (optional, required for record), useful (optional, required for record)
- `test_scope`: files (required, comma-separated file paths), output (optional: "packages", "functions", "run"), depth (optional, default 3)
- `flow_between`: source_symbol (required), target_symbol (required), max_depth (optional, default 5)
- `plan_turn`: task (required)
- `communities`: action (optional: "list" or "for_symbol"), repo_url (optional), symbol (optional, required for for_symbol)
- `explain_symbol`: task_description (required), symbol (required)

---

## CLI Commands

### `knowing serve`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`), `--addr` (default: `:8080`), `--trace` (enable trace ingestion), `--trace-endpoint` (default: `localhost:4317`), `--trace-batch-size` (default: 1000)
- **Positional args:** repo paths to watch (0 or more)
- **What it does:** Opens SQLite store, creates snapshot manager and indexer, registers all 26 extractor packages (Go, Python, Ruby, TypeScript/JS, Rust, Java, C#, Terraform, SQL, K8s YAML, Cloud YAML [CFN/SAM, Compose, Actions, Serverless], CSS, Protocol Buffers, Dockerfile, Makefile, Helm, GitLab CI, package.json/npm, GraphQL, Ansible), creates MCP server (28 tools across seven planes, 3 prompts), creates daemon with GitWatcher, optionally starts trace ingestion pipeline (OTLPReceiver + Ingestor + periodic flush/decay), watches listed repos, blocks until SIGINT/SIGTERM.
- **Extractors registered:** `gotsextractor` (Go), `treesitter` (Python), `tsextractor` (TypeScript/JS), `rustextractor` (Rust), `javaextractor` (Java), `csharpextractor` (C#), `terraformextractor` (Terraform HCL), `sqlextractor` (SQL), `k8sextractor` (Kubernetes YAML), `cssextractor` (CSS), `protoextractor` (Protocol Buffers). Route detection covers 18 frameworks across 6 languages (Go: 5, Python: 3, TypeScript: 5).
- **Enrichment:** Background EnrichFunc wired with scoped or full enrichment.
- **Trace ingestion:** When `--trace` is set, launches a fourth daemon goroutine that opens a dedicated DB connection, creates SymbolResolver + Ingestor + OTLPReceiver, flushes batches every 10s, and decays confidence every 1h.

### `knowing index`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`), `--url` (default: repo path), `--commit` (default: HEAD), `--full` (use go/packages instead of tree-sitter), `--workers` (default: 8, number of parallel extraction goroutines), `--skip-blame` (skip git blame authorship pass for faster structural-only index), `--no-enrich` (skip LSP enrichment for structural-only indexing)
- **Positional args:** repo-path (required)
- **What it does:** Opens store, creates indexer, registers extractor (tree-sitter by default, go/packages if `--full`), indexes repo with parallel extraction (8-worker goroutine pool, progress output to stderr every 2s), prints stats, rebuilds FTS synchronously (~500ms), runs LSP enrichment synchronously (if not `--full`). Phase separation: walk -> parallel extract -> batch store -> authorship -> snapshot -> FTS rebuild. On the knowing codebase (84K LOC, 429 files), completes in ~2.3 seconds.

### `knowing query`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`)
- **Positional args:** symbol-prefix (required)
- **What it does:** Opens store, queries `NodesByName` with prefix, prints nodes with their outbound edges.

### `knowing export`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`), `--format` (default: `json`; also accepts `dot`), `--repo` (filter by repo URL), `--snapshot` (filter label, cosmetic only)
- **No positional args.**
- **What it does:** Exports the full knowledge graph (or a repo-scoped subset) as JSON or Graphviz DOT to stdout. JSON output includes nodes with community IDs, edges with cross-community flags, Louvain-detected community listings, and metadata with counts. DOT output renders community clusters as subgraphs with cross-community edges highlighted in red.

### `knowing mcp`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`)
- **No positional args.**
- **What it does:** Launches the MCP server in stdio transport mode. Reads JSON-RPC messages from stdin and writes responses to stdout. Provides all 28 tools and 3 prompts. Designed for subprocess MCP usage (configured in `.mcp.json` or Claude Desktop).

### `knowing reindex`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`)
- **Positional args:** repo-path (required)
- **What it does:** Clears all nodes, edges, and edge events from the store, then re-indexes the specified repository from scratch. Useful when extractor logic has changed or when the graph has accumulated stale data.

### `knowing init`

- **Flags:** `--db` (default: per-repo, from roster), `--output` (default: `CLAUDE.md`)
- **No positional args.**
- **What it does:** Registers the repo in the roster, assigns a per-repo database at `~/.knowing/repos/<safe-name>.db`, indexes it, runs LSP enrichment, and generates a CLAUDE.md section with graph-derived project context (symbol counts, package counts, tool breadcrumbs). Nondestructive and idempotent: uses markers to replace only the generated section, leaving hand-written content intact.

### `knowing add`

- **Flags:** `--db` (default: per-repo, from roster), `--url` (auto-detected from git remote)
- **Positional args:** repo-path (required)
- **What it does:** Registers a repository in the roster and assigns it a per-repo database at `~/.knowing/repos/<safe-name>.db`. Indexes the repo into that isolated database.

### `knowing remove`

- **Flags:** `--db` (default: per-repo, from roster)
- **Positional args:** repo-path (required)
- **What it does:** Removes a repository from the roster. The per-repo database file is not deleted.

### `knowing list`

- **Flags:** none
- **No positional args.**
- **What it does:** Lists all repositories in the roster with path, URL, per-repo DB path, database file size, and last-indexed timestamp.

### `knowing context`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`), `--format` (default: `json`, accepts: `gcf`, `gcb`, `json`, `xml`, `markdown`), `--task` (natural-language task description), `--files` (comma-separated file paths), `--token-budget` (optional)
- **What it does:** CLI interface to the context engine. Returns ranked symbols and edges relevant to a task description or set of files. Output is encoded using the specified wire format codec. Equivalent to calling the `context_for_task` or `context_for_files` MCP tools from the command line.

### `knowing test-scope`

- **Flags:** `--db` (default: `~/.knowing/knowing.db`), `--files` (comma-separated changed files; defaults to `git diff HEAD`), `--output` (default: `packages`; also `functions`, `run`), `--depth` (default: 3)
- **No positional args.**
- **What it does:** Computes which tests are affected by changed files. Uses `NodesByFilePath` to find symbols in changed files, then BFS backward through `calls` edges up to `--depth` hops to find test functions. Output modes: `packages` (Go package paths), `functions` (qualified test names), `run` (`-run` regex for `go test`).

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
| edge_hash | BLOB PK | sha256(source + target + type + provenance) | GetEdge, DanglingEdges | PutEdge, BatchPutEdges, Ingestor |
| source_hash | BLOB FK | References nodes | EdgesFrom, TransitiveCallers/Callees, DeleteEdgesBySourceFile | PutEdge, Ingestor |
| target_hash | BLOB FK | References nodes | EdgesTo, DanglingEdges, RuntimeEdgesByService JOIN | PutEdge, Ingestor |
| edge_type | TEXT NOT NULL | calls, imports, implements, references, runtime_calls, runtime_rpc, runtime_produces, runtime_consumes, handles_route | EdgesFrom/To filter, traversal CTEs, RuntimeEdgeStatsAggregate | PutEdge, Ingestor |
| confidence | REAL DEFAULT 1.0 | 0.0-1.0 | Display | PutEdge, Ingestor, DecayRuntimeConfidence |
| provenance | TEXT DEFAULT 'ast_resolved' | ast_resolved, ast_inferred, lsp_resolved, otel_trace, otel_trace:{json} | Enricher filter, RuntimeEdgesByProvenance LIKE, display | PutEdge, Ingestor |
| callsite_line | INTEGER DEFAULT 0 | 1-indexed line of call expression | Enricher (GetDefinition position) | PutEdge (tree-sitter extractor) |
| callsite_col | INTEGER DEFAULT 0 | 0-indexed column of call expression | Enricher | PutEdge |
| callsite_file | TEXT DEFAULT '' | Relative file path | Enricher | PutEdge |
| observation_count | INTEGER NOT NULL DEFAULT 0 | Total runtime observations (0 for static edges) | RuntimeEdgesByProvenance, RuntimeEdgeStatsAggregate, Ingestor | Ingestor, UpdateObservation |
| last_observed | INTEGER NOT NULL DEFAULT 0 | Unix timestamp of last observation (0 for static edges) | DecayRuntimeConfidence, DeadRoutes, RuntimeEdgeStatsAggregate | Ingestor, UpdateObservation |

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
| snapshot_hash | BLOB PK | Hierarchical Merkle root (repo root -> package roots -> edge-type roots -> edge leaves); flat root also computed and is identical | GetSnapshot, chain walking | CreateSnapshot |
| parent_hash | BLOB FK | Previous snapshot | Chain walking, GC | CreateSnapshot |
| repo_hash | BLOB FK | Which repo | LatestSnapshot | CreateSnapshot |
| commit_hash | TEXT NOT NULL | Git commit | Display | CreateSnapshot |
| timestamp | INTEGER NOT NULL | Unix timestamp | LatestSnapshot ORDER BY | CreateSnapshot |
| node_count | INTEGER NOT NULL | Count at snapshot time | Display | CreateSnapshot |
| edge_count | INTEGER NOT NULL | Count at snapshot time | Display | CreateSnapshot |

### Table: `route_symbols`
| Column | Type | Description | Read by | Written by |
|--------|------|-------------|---------|-----------|
| service_name | TEXT NOT NULL | Service or package name | GetRouteSymbol, SymbolResolver.Resolve, RuntimeEdgesByService JOIN, DeadRoutes | PutRouteSymbol |
| route_pattern | TEXT NOT NULL | HTTP method + path (e.g., "GET /users/:id") | GetRouteSymbol, SymbolResolver.Resolve, RuntimeEdgesByService LIKE | PutRouteSymbol |
| node_hash | BLOB NOT NULL | References nodes (the handler function) | SymbolResolver.Resolve, RuntimeEdgesByService JOIN, DeadRoutes | PutRouteSymbol |
| mapping_type | TEXT NOT NULL | "http_route", "grpc_method", "queue_topic" | GetRouteSymbol, SymbolResolver.Resolve | PutRouteSymbol |
| created_at | INTEGER NOT NULL | Unix timestamp | (stored, queryable) | PutRouteSymbol |

Primary key: `(service_name, route_pattern, mapping_type)`.

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
| idx_route_symbols_node | route_symbols | node_hash | RuntimeEdgesByService JOIN, DeadRoutes JOIN |
| idx_edges_provenance | edges | provenance | RuntimeEdgesByProvenance LIKE, DecayRuntimeConfidence |
| idx_edges_last_observed | edges | last_observed | DecayRuntimeConfidence threshold, DeadRoutes |

### Migrations
| # | File | What it does |
|---|------|-------------|
| 001 | 001_initial_schema.sql | Creates all 6 tables + 7 indexes |
| 002 | 002_add_dangling_edge_support.sql | No-op (idx_edges_target from 001 already covers dangling queries) |
| 003 | 003_add_callsite_columns.sql | Adds callsite_line, callsite_col, callsite_file to edges table |
| 004 | 004_add_runtime_columns.sql | Adds observation_count and last_observed columns to edges table. Creates route_symbols table with composite PK (service_name, route_pattern, mapping_type). Adds idx_route_symbols_node, idx_edges_provenance, idx_edges_last_observed indexes. |
| 006 | 006_add_fts5_index.sql | Creates FTS5 virtual table `nodes_fts` over qualified_name, signature, file_path for BM25 full-text search (extended by migration 016). |
| 007 | 007_add_doc_column.sql | Adds `doc` column to nodes table for storing extracted doc comments. |
| 016 | 016_fts_symbol_name.sql | Adds `symbol_name` column to FTS content table. Stores terminal symbol identifier (stripped of repo URL, package path, file extension). Recreates FTS5 table with 4 columns: symbol_name, qualified_name, signature, file_path. |
| 017 | 017_fts_concepts_column.sql | Adds `concepts` column to FTS content table. Stores CamelCase-split file/module names as searchable concepts. Recreates FTS5 table with 5 columns: symbol_name, concepts, qualified_name, signature, file_path. |

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

### Runtime and Route Edge Types (Implemented)

| Edge Type | Producer(s) | Provenance | Confidence | Notes |
|-----------|------------|------------|------------|-------|
| `handles_route` | gotsextractor (route detection) | `ast_inferred` | 0.7 | Route handler node -> handler function node. Created during static extraction. |
| `runtime_calls` | trace.Ingestor (HTTP spans) | `otel_trace` | 0.5-0.95 (from observation count) | HTTP-attributed spans. Default edge type for unknown spans. |
| `runtime_rpc` | trace.Ingestor (gRPC spans) | `otel_trace` | 0.5-0.95 | gRPC-attributed spans (rpc.service + rpc.method). |
| `runtime_produces` | trace.Ingestor (messaging spans) | `otel_trace` | 0.5-0.95 | Messaging spans with messaging.destination attribute. |
| `runtime_consumes` | trace.Ingestor (messaging spans) | `otel_trace` | 0.5-0.95 | Messaging spans without messaging.destination attribute. |

### Defined in Architecture but NOT Produced by Any Code

| Edge Type | Category | Status |
|-----------|----------|--------|
| `rpc_calls` | Protocol | NOT IMPLEMENTED (see `implements_rpc`/`consumes_rpc` for gRPC) |
| `produces_event` | Protocol | NOT IMPLEMENTED (see `publishes` for messaging) |
| `consumes_event` | Protocol | NOT IMPLEMENTED (see `subscribes` for messaging) |
| `reads_field` | Schema | NOT IMPLEMENTED |
| `writes_field` | Schema | NOT IMPLEMENTED |
| `declares_route` | Schema | NOT IMPLEMENTED (see `handles_route` for route handlers) |
| `consumes_route` | Schema | NOT IMPLEMENTED (see `consumes_endpoint` for HTTP client calls) |
| `depends_on_service` | Infrastructure | NOT IMPLEMENTED |
| `runtime_queries` | Runtime | NOT IMPLEMENTED |

**Previously listed here, now implemented:**
- `deploys`: K8s YAML extractor (Service -> Deployment selector match)
- `connects_to`: Cloud extractor (Docker Compose `depends_on` and shared networks)
- `owned_by`: CODEOWNERS extractor (replaces planned `owned_by_team`/`owned_by_user`)

---

## Node Kinds

| Kind | What it represents | Produced by |
|------|--------------------|-------------|
| `function` | Package-level function or RPC method | gotsextractor, goextractor, treesitter, tsextractor, rustextractor, javaextractor, csharpextractor, protoextractor |
| `method` | Method on a type | gotsextractor, goextractor |
| `type` | Named type (struct, alias, message, enum) | gotsextractor, goextractor, treesitter (Python classes), tsextractor, rustextractor, javaextractor, csharpextractor, protoextractor (messages, enums) |
| `interface` | Interface type | gotsextractor, goextractor |
| `const` | Constant declaration | gotsextractor, goextractor |
| `var` | Variable declaration | gotsextractor, goextractor |
| `file` | Synthetic file-level node (for import edges) | gotsextractor, goextractor (implicit, not stored as a separate node) |
| `package` | Synthetic package node (import target) | gotsextractor, goextractor (implicit, not stored) |
| `module` | Python module (import target) | treesitter |
| `route_handler` | HTTP route registration (e.g., "GET /users/:id") | gotsextractor (route detection) |
| `service` | Service declaration (proto) or runtime service identity | protoextractor (proto service declarations), trace.SymbolResolver (synthetic, for span source nodes) |
| `runtime_endpoint` | Unresolved runtime target | trace.SymbolResolver (synthetic, when route_symbols lookup fails) |

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
| TraceConfig | `*TraceIngestConfig` | Runtime trace ingestion config (nil disables trace) |
| DBPath | `string` | SQLite database path for trace ingestor's dedicated connection |

### TraceIngestConfig (`internal/daemon/daemon.go`)

| Field | Type | Purpose |
|-------|------|---------|
| Enabled | `bool` | Whether trace ingestion is active |
| OTLPEndpoint | `string` | gRPC address for OTLP receiver (e.g., "localhost:4317") |
| BatchSize | `int` | Number of spans per batch before auto-flush |
| BatchInterval | `time.Duration` | Periodic flush interval |

### Extractor Registration

Configured in `cmd/knowing/main.go`. Default mode (`serve` and `index` without `--full`):
- `gotsextractor.NewGoTreeSitterExtractor()` (Go files)
- `treesitter.NewTreeSitterExtractor("python")` (Python files)
- `terraformextractor.NewTerraformExtractor()` (Terraform HCL files)
- `sqlextractor.NewSQLExtractor()` (SQL files)
- `k8sextractor.NewK8sExtractor()` (Kubernetes YAML manifests)
- `cssextractor.NewCSSExtractor()` (CSS files)

Full mode (`index --full`):
- `goextractor.NewGoExtractor()` (Go files)
- `treesitter.NewTreeSitterExtractor("python")` (Python files)
- `terraformextractor.NewTerraformExtractor()` (Terraform HCL files)
- `sqlextractor.NewSQLExtractor()` (SQL files)
- `k8sextractor.NewK8sExtractor()` (Kubernetes YAML manifests)
- `cssextractor.NewCSSExtractor()` (CSS files)

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
- **Roadmap: Edge Types workstream. IMPLEMENTED.**
- **Package(s):** `internal/indexer/scipingest/`, `cmd/knowing/ingestscip.go`
- **CLI:** `knowing ingest-scip -file <path> -repo <url>`
- **What it does:** Reads a `.scip` protobuf file (produced by `scip-go`, `scip-typescript`, etc.), creates nodes for all symbol definitions and `references` edges for all symbol references. Provenance `scip_resolved`, confidence 0.95.
- **Entry point:** `scipingest.NewSCIPIngester(store).IngestFile(ctx, SCIPIngestOptions)`
- **Returns:** `SCIPIngestResult` with `NodesCreated`, `EdgesCreated`, `DocsProcessed`.

### Protobuf/gRPC Edge Extraction
- **Roadmap: Edge Types workstream. IMPLEMENTED.**
- **Package(s):** `internal/indexer/protoextractor/`
- **What it does:** Parses `.proto` files with tree-sitter. Extracts service, message, enum, and RPC declarations. Produces `references` edges for field types and RPC request/response message types. See Feature #24 (Protocol Buffers Extractor).
- **Entry point:** `protoextractor.NewProtoExtractor()`

### HTTP Route Edge Extraction
- **Roadmap: Edge Types workstream.**
- **Status: IMPLEMENTED.** See Feature #26 (HTTP Route Extraction). Route detection covers 18 frameworks across 6 languages: Go (net/http, chi, gin, echo, gorilla/mux), Python (Flask, FastAPI, Django), TypeScript (Express, Fastify, Hono, NestJS, Next.js App Router). Creates `route_handler` nodes and `handles_route` edges. The `route_symbols` table exists for runtime symbol resolution, though the indexer does not yet auto-populate it from extracted route_handler nodes (PutRouteSymbol must be called separately).
- **Remaining gaps:** `declares_route`/`consumes_route` edge types from the architecture are not used; instead `handles_route` is used. Route groups/prefixes not supported. Dynamic route patterns not detected.

### Event/Message Queue Edge Extraction
- **Roadmap: Edge Types workstream. IMPLEMENTED.**
- **Package(s):** `internal/indexer/eventextractor/`
- **What it does:** Detects Kafka, NATS, SQS, and RabbitMQ/AMQP producer/consumer patterns across Go, TypeScript, Python, and Java. Produces `publishes`/`subscribes` edges with topic nodes.
- **Status:** Registered in CLI (`cmd/knowing/main.go` line 1102: `idx.Register(eventextractor.NewEventExtractor())`). Runs alongside primary language extractors via `FindAllExtractors` multi-dispatch (see Feature #50).
- **Entry point:** `eventextractor.NewEventExtractor()`

### Schema Edge Extraction (OpenAPI, JSON Schema)
- **Roadmap: Edge Types workstream. IMPLEMENTED.**
- **Package(s):** `internal/indexer/schemaextractor/`
- **What it does:** Parses OpenAPI/Swagger specs and JSON Schema files to extract schema, endpoint, and field nodes with reference edges.
- **Status:** Registered in CLI (`cmd/knowing/main.go` line 1105: `idx.Register(schemaextractor.NewSchemaExtractor())`). Runs alongside primary language extractors via `FindAllExtractors` multi-dispatch (see Feature #50).
- **Entry point:** `schemaextractor.NewSchemaExtractor()`

### Infrastructure Edge Extraction (Terraform, K8s, Cloud YAML)
- **Roadmap: Edge Types workstream. IMPLEMENTED.**
- **Package(s):** `internal/indexer/terraformextractor/`, `internal/indexer/k8sextractor/`, `internal/indexer/cloudextractor/`
- **Status:** Terraform, K8s YAML, and Cloud YAML (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework) extractors are all registered and active.
- **Cloud extractor edge types:** `depends_on`, `deploys`, `exposes`, `configures`, `publishes`, `subscribes`, `connects_to`.
- **K8s extractor edge types:** `deploys` (Service to Deployment via selector match), `exposes` (Ingress to Service), `configures` (ConfigMap/Secret to Deployment).
- **Terraform edge types:** `depends_on` (explicit resource dependencies), `calls` (module references).

### Ownership Edge Extraction (CODEOWNERS)
- **Roadmap: Edge Types workstream.**
- **What's needed:** CODEOWNERS parser, service catalog ingestion, `owned_by_team`/`owned_by_user` edges.
- **No code exists.** The `ownership` MCP tool exists but only lists files/symbols (no team mapping).

### Runtime Trace Ingestion
- **Architecture decision #13. Roadmap: Runtime Intelligence workstream.**
- **Status: IMPLEMENTED.** See Features #22-23 (Trace Pipeline, OTLPReceiver), #24 (Store Runtime Methods), #27 (Daemon Wiring), #28 (MCP Runtime Tools).
- **What was built:** `TraceIngestor` interface and `Ingestor` implementation in `internal/trace`, `SymbolResolver` for route-to-node mapping via `route_symbols` table, `OTLPReceiver` gRPC server accepting real OTLP spans, confidence scoring with observation-based decay, `runtime_calls`/`runtime_rpc`/`runtime_produces`/`runtime_consumes` edge types, batch accumulation, daemon wiring with periodic flush and decay, three MCP tools (runtime_traffic, dead_routes, trace_stats).
- **Remaining gaps:** No queue/topic resolution beyond basic attribute detection. No TLS on gRPC. RECONNECTING health state defined but unused. Synthetic unresolved nodes are created but not cleaned up. No automatic route_symbols population from static extraction.

### Semantic PR Diff (Full)
- **Architecture decision #14. Roadmap: Developer Visibility workstream.**
- **Current state:** `semantic_diff` and `pr_impact` MCP tools exist but are thin wrappers over `SnapshotDiff`.
- **What's needed:** Blast radius delta computation, ownership impact analysis, cross-repo edge classification, PR comment formatting, GitHub Action.
- **Partially implemented.** `pr_impact` computes blast radius of removed nodes but lacks ownership delta, staleness annotations, and the full output format described in architecture.

### Graph-Native Test Selection
- **Roadmap: Developer Visibility workstream.**
- **What was built:** `knowing test-scope` CLI command in `cmd/knowing/testscope.go`. Uses `NodesByFilePath` (store method that joins nodes to files via SQL) to find symbols in changed files, then BFS backward through `calls` edges to find test functions. Three output modes: `packages`, `functions`, `run` (regex for `go test -run`). Auto-detects changed files via `git diff HEAD` when `-files` is not specified.
- **Remaining gaps:** Only detects Go test functions (prefix "Test"). No table-driven subtest detection. No integration with CI (no JSON output mode for machine consumption).

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
- **Status:** TypeScript route/framework detection is implemented (5 frameworks: Express, Fastify, Hono, NestJS, Next.js App Router). Python route detection is implemented (3 frameworks: Flask, FastAPI, Django). Full TypeScript/Rust/Java AST extractors (function/type/call extraction beyond route detection) do not yet exist. Current language support: Go (tree-sitter + go/packages), Python (tree-sitter), Terraform (HCL), SQL, Kubernetes YAML, CSS, plus route detection for 18 frameworks across 6 languages.

### CI Integration (GitHub Action)
- **Architecture decision #14.**
- **Status: PARTIALLY IMPLEMENTED.** See Feature #30 (CI/CD Pipeline). CI workflow (ci.yml) runs build, vet, test, and binary smoke test on push/PR. Release workflow (release.yml) handles GoReleaser, Docker, Homebrew, winget, npm, PyPI, and MCP Registry publishing. Docs workflow (docs.yml) deploys mkdocs to GitHub Pages.
- **Remaining gaps:** No `blackwell-systems/knowing-action` for PR comment posting. No integration test step. No semantic PR diff comment automation.

### `--working-tree` Flag
- **Architecture describes for indexing uncommitted changes.**
- **What's needed:** Temporary snapshot not linked to main chain.
- **No code exists.**

### DeleteSnapshot in GraphStore
- **Status: IMPLEMENTED.** `SQLiteStore.DeleteSnapshot` removes a snapshot and its associated edge events. Used by `GarbageCollect` to perform real garbage collection of old snapshots.

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
| e2e_test.go | e2e_test.go | ~9 (includes snapshot lifecycle integration test) |

### Real Indexing Benchmarks

| Repo | Approach | Wall Time | Nodes | Edges | Cross-Repo Edges |
|------|----------|-----------|-------|-------|-----------------|
| knowing (self) | parallel extraction (8 workers) | **1.8s** | 7,224 | 24,936 (30 edge types) | -- |
| knowing (self, earlier) | tree-sitter + LSP | ~9s | 2,564 | 8,604 | -- |
| polywave-go | go/packages (baseline) | 16m 24s | 6,340 | 17,232 | -- |
| polywave-go | tree-sitter + LSP (final) | 9.1s | 2,564 | 8,604 (all lsp_resolved) + 213 discovered | -- |
| polywave-go + polywave-web | tree-sitter | -- | 6,340 + 1,569 | 17,232 + 5,939 | 228 |

**Parallel indexer stats (knowing codebase, 84K LOC):** 429 source files, 62 packages, 1,451 files/sec throughput, 8-worker goroutine pool, progress output every 2s. Flags: `--workers` (parallelism), `--skip-blame` (skip authorship for structural-only index).

---

## File Inventory

```
cmd/knowing/
  main.go                          -- CLI entry point (serve, index, query, export, init, version)
  init.go                          -- knowing init: generates CLAUDE.md with graph-derived context
  main_test.go

internal/types/
  types.go                         -- Hash, Node, Edge (with ObservationCount, LastObserved), File, Repo, Snapshot, EdgeEvent, EdgeProvenance, hash functions
  interfaces.go                    -- GraphStore, Extractor, ComputationCache, ExtractOptions, ExtractResult
  results.go                       -- CallerResult, CalleeResult, BlastRadiusResult, DiffResult, DerivedResult, TraversalOptions

internal/store/
  sqlite.go                        -- SQLiteStore: all 27+ GraphStore methods + batch ops
  sqlite_runtime.go                -- Runtime store methods: PutRouteSymbol, GetRouteSymbol, UpdateObservation, RuntimeEdgesByProvenance, DecayRuntimeConfidence, RuntimeEdgesByService, DeadRoutes, RuntimeEdgeStatsAggregate
  sqlite_runtime_test.go
  sqlite_test.go
  migrate.go                       -- Migration runner with go:embed
  migrations/
    001_initial_schema.sql          -- 6 tables, 7 indexes
    002_add_dangling_edge_support.sql -- No-op
    003_add_callsite_columns.sql    -- 3 ALTER TABLE statements
    004_add_runtime_columns.sql     -- observation_count, last_observed on edges; route_symbols table; 3 indexes

internal/trace/
  types.go                         -- TraceSpan, HTTPLogEntry, RuntimeStats, IngestResult, RouteMapping, HealthState, TraceIngestConfig, TraceIngestor interface, ConfidenceFromCount
  confidence.go                    -- ComputeConfidence, ShouldGarbageCollect, DecayBracket, BuildProvenance
  confidence_test.go
  resolver.go                      -- SymbolResolver: route-to-node mapping via route_symbols table
  resolver_test.go
  ingestor.go                      -- Ingestor: IngestSpans, IngestHTTPLogs, RuntimeEdgeStats, DecayConfidence, AddToBatch, FlushBatch
  ingestor_test.go
  otlp.go                          -- OTLPReceiver: gRPC server implementing OTLP trace receiver protocol
  otlp_test.go
  integration_test.go

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
    extractor.go                   -- GoTreeSitterExtractor: calls, imports with call-site positions + HTTP route extraction (route_handler nodes, handles_route edges)
    extractor_test.go
  treesitter/
    extractor.go                   -- TreeSitterExtractor (Python): functions, classes, imports, calls
    extractor_test.go
  terraformextractor/
    extractor.go                   -- TerraformExtractor: HCL resources, data sources, modules, variables, outputs
    extractor_test.go
  sqlextractor/
    extractor.go                   -- SQLExtractor: tables, views, columns, foreign keys
    extractor_test.go
  k8sextractor/
    extractor.go                   -- K8sExtractor: deployments, services, configmaps, secrets
    extractor_test.go
  cssextractor/
    extractor.go                   -- CSSExtractor: selectors, custom properties
    extractor_test.go
  eventextractor/
    extractor.go                   -- EventExtractor: Kafka, NATS, SQS, RabbitMQ producer/consumer patterns
    extractor_test.go
  schemaextractor/
    extractor.go                   -- SchemaExtractor: OpenAPI/Swagger specs, JSON Schema
    extractor_test.go

internal/context/
  context.go                       -- Context engine: task/file/PR context generation, RWR graph walk, token budgeting
  ranking.go                       -- RankSymbols: blast radius, confidence, recency, distance, HITS authority, session boost
  hits.go                          -- HITS algorithm: authority/hub scores for subgraph reranking
  session.go                       -- SessionTracker: exponential-decay recency boost (3-min half-life, 2.0x cap)
  context_test.go

internal/wire/
  registry.go                      -- Codec registry: Register, Get, List, EncodeWith, DecodeWith
  session.go                       -- Session: cross-call symbol deduplication (47% savings on repeats)
  gcf.go                           -- GCF text encoder (graph-native, LLM-optimized)
  gcf_decode.go                    -- GCF text decoder
  json.go                          -- JSON codec (encode/decode via standard library)
  binary.go                        -- Binary codec (varint + length-prefixed, registered as "gcb")
  xml.go                           -- XML codec
  markdown.go                      -- Markdown table codec
  gcf_test.go
  registry_test.go
  binary_test.go

internal/enrichment/
  enricher.go                      -- Enricher: LSP edge upgrade + edge discovery via multi-language servers
  enricher_test.go

internal/daemon/
  daemon.go                        -- Daemon: lifecycle, watchLoop, indexWorker, traceIngestLoop, GitHeadCommit
  gitwatcher.go                    -- GitWatcher: fsnotify on .git/HEAD, CommitEvent
  gitdiff.go                       -- GitDiffFiles: git diff --name-status
  daemon_test.go
  gitwatcher_test.go
  gitdiff_test.go

internal/mcp/
  server.go                        -- MCP Server: 17 tool definitions (execution + intelligence + runtime + context planes), stdio + HTTP transport
  handlers.go                      -- 14 handler implementations (11 original + 3 runtime)
  context_handlers.go              -- 4 context handler implementations: context_for_task, context_for_files, context_for_pr, explain_symbol
  prompts.go                       -- 3 MCP prompts: refactor_safely, review_pr, investigate_dead_code
  handlers_test.go

internal/resolver/
  resolver.go                      -- Resolver: dangling edge retargeting via hash recomputation
  resolver_test.go

e2e_test.go                        -- End-to-end integration test (multi-package Go module, includes snapshot lifecycle test)

.github/workflows/
  ci.yml                           -- CI: build, vet, test, binary smoke test
  release.yml                      -- Release: GoReleaser + Docker + Homebrew + winget + npm + PyPI + MCP Registry
  docs.yml                         -- Docs: mkdocs-material -> GitHub Pages

.goreleaser.yml                    -- GoReleaser v2 config: multi-platform builds, Docker manifests, Homebrew tap
.mcp.json                          -- MCP self-dogfooding: knowing serves its own graph to agents working in this repo

bench/
  feedback-loop/                   -- Agent feedback loop effectiveness benchmark
  context-relevance/               -- Context engine precision/recall benchmark
  token-savings/                   -- GCF session deduplication and knapsack savings benchmark
  edge-accuracy/                   -- Extractor edge precision/recall/F1 benchmark
  test-scope-accuracy/             -- Test-scope output vs actual test failures benchmark
  wire-format/                     -- Wire format codec encode/decode performance benchmark
    scorecard.md                   -- Auto-generated benchmark comparison table (6 fixture cases)
```

---

## Provenance Tiers (Implemented)

| Provenance | Confidence | Source | When Available |
|-----------|-----------|--------|----------------|
| `ast_inferred` | 0.7 | tree-sitter syntactic matching | After Tier 1 (~1.8s with parallel extraction) |
| `lsp_resolved` | 0.9 | LSP GetDefinition/GetImplementation/GetReferences (gopls, pyright, tsserver, rust-analyzer, jdtls, OmniSharp) | After Tier 2 (~8s more for LSP enrichment) |
| `ast_resolved` | 1.0 | go/packages full type resolution | `--full` flag only (~16min) |
| `otel_trace` | 0.5-0.95 | Runtime observation via OTLP spans | After trace ingestion (continuous). Confidence from ConfidenceFromCount: 1 obs=0.5, 10+=0.7, 100+=0.85, 1000+=0.95. Decays to 0.2 after 30 days without observations, 0.0 after 90 days. |
| `scip_resolved` | 0.95 | SCIP index file (via `knowing ingest-scip`) | After SCIP ingest. Near-full confidence from compiler-grade indexers. |

### Provenance Tiers (Defined in Architecture, NOT Implemented)

| Provenance | Confidence | Source | Status |
|-----------|-----------|--------|--------|
| `config_declared` | 0.8 | Terraform/K8s config | NOT IMPLEMENTED (infra extractors use ast_inferred instead) |
| `inferred_from_import` | 0.7 | Import without call site | NOT IMPLEMENTED (distinct from ast_inferred) |
| `openapi_declared` | 0.7 | OpenAPI/proto spec | NOT IMPLEMENTED |
| `text_matched` | 0.3 | String literal/comment heuristic | NOT IMPLEMENTED |
| `manual` | 1.0 | User-declared | NOT IMPLEMENTED |
| `runtime_unresolved` | 0.3 | Unresolved runtime endpoint | NOT IMPLEMENTED (synthetic nodes with this confidence exist but use provenance "otel_trace") |

---

## Symbol Identity Scheme

Format: `{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`

Hash computation: `sha256("node\0" + repoURL + "\x00" + packagePath + "\x00" + symbolName + "\x00" + symbolKind)`

Note: `contentHash` parameter exists in `ComputeNodeHash` for API compatibility but is NOT included in the hash. Node identity depends on (repo, package, name, kind) only. The `"node\0"` domain-type prefix prevents cross-type hash collisions.

Edge hash: `sha256("edge\0" + sourceHash.String() + "\x00" + targetHash.String() + "\x00" + edgeType + "\x00" + provenanceJSON)`

File hash: `sha256(repoHash + path + contentHash)`

Repo hash: `sha256(repoURL)`

Snapshot hash: Hierarchical Merkle root (repo root -> package roots -> edge-type roots -> edge leaves), implemented in `internal/snapshot/hierarchical.go`. The hierarchical root is the canonical snapshot hash; no flat tree is maintained. `DiffHierarchicalTrees` compares package roots for O(packages) diff instead of O(edges). Domain prefixes: `"snapshot\0"` wraps the Merkle root, `"merkle\0"` prefixes interior nodes. See `bench/merkle-diff/` for benchmark results.

---

## External Dependencies

| Dependency | Used by | Purpose |
|-----------|---------|---------|
| `modernc.org/sqlite` | internal/store, internal/daemon (trace) | Pure Go SQLite driver (no CGo) |
| `github.com/smacker/go-tree-sitter` | gotsextractor, treesitter | Tree-sitter Go bindings |
| `github.com/smacker/go-tree-sitter/golang` | gotsextractor | Go grammar |
| `github.com/smacker/go-tree-sitter/python` | treesitter | Python grammar |
| `golang.org/x/tools/go/packages` | goextractor | Go type-resolution package loader |
| `github.com/mark3labs/mcp-go` | internal/mcp | MCP protocol server library |
| `github.com/fsnotify/fsnotify` | internal/daemon | File system notification |
| `github.com/blackwell-systems/agent-lsp/pkg/lsp` | internal/enrichment | LSP client (multi-language server communication) |
| `github.com/blackwell-systems/agent-lsp/pkg/types` | internal/enrichment | LSP type definitions |
| `go.opentelemetry.io/proto/otlp/collector/trace/v1` | internal/trace | OTLP trace collector protobuf definitions |
| `go.opentelemetry.io/proto/otlp/trace/v1` | internal/trace | OTLP trace span protobuf definitions |
| `go.opentelemetry.io/proto/otlp/common/v1` | internal/trace | OTLP common type definitions (AnyValue) |
| `go.opentelemetry.io/proto/otlp/resource/v1` | internal/trace | OTLP resource definitions (service.name extraction) |
| `google.golang.org/grpc` | internal/trace | gRPC server for OTLP receiver |
