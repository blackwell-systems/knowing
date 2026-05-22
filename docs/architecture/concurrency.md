# Concurrency Model

The daemon is a single process with concurrent goroutines, not a distributed system. All coordination is in-process using Go's standard concurrency primitives.

## Goroutine Architecture

The daemon runs four primary goroutines, plus optional goroutines for enrichment and trace ingestion:

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Daemon Process                               │
│                                                                      │
│  ┌─────────────┐   indexQueue  ┌──────────────┐                      │
│  │  watchLoop   │────────────>│  indexWorker  │                      │
│  │  goroutine   │  (buffered  │  goroutine    │                      │
│  │              │   chan, 128) │              │                      │
│  └──────┬───────┘             └──────┬───────┘                      │
│         │                            │                              │
│    reads from                   on success:                         │
│    GitWatcher.Events()          spawns background                   │
│    (64-item buffered chan)       enrichment goroutine               │
│         │                            │                              │
│  ┌──────┴───────┐             ┌──────┴───────┐                      │
│  │  GitWatcher   │             │  enrichment  │                      │
│  │  event loop   │             │  goroutine   │                      │
│  │  (debounce)   │             │  (per index) │                      │
│  └───────────────┘             └──────────────┘                      │
│                                                                      │
│  ┌───────────────┐            ┌───────────────────────────────────┐  │
│  │  MCP Server   │  (opt.)    │  traceIngestLoop goroutine (opt.) │  │
│  │  goroutine    │            │  ├── OTLPReceiver (gRPC server)   │  │
│  └───────────────┘            │  ├── batchTicker (FlushBatch)     │  │
│                               │  └── decayTicker (DecayConfidence)│  │
│                               └───────────────────────────────────┘  │
│                                                                      │
│  main goroutine: blocks on <-ctx.Done(), then shutdown()             │
└──────────────────────────────────────────────────────────────────────┘
```

**watchLoop goroutine:** Reads `CommitEvent` values from `d.watcher.Events()` (64-item buffered channel). For each event, it combines changed, added, and deleted file lists into a single `indexRequest` and sends it to `d.indexQueue`. If the channel is full (128-item buffer), the event is dropped via a non-blocking `select default` branch. This goroutine never blocks on indexing; it only enqueues.

**indexWorker goroutine:** Reads `indexRequest` values from `d.indexQueue` sequentially using `for req := range d.indexQueue`. For each request, it resolves the HEAD commit (pure file reads, no git subprocess), acquires the daemon's write lock (`d.mu.Lock()`), calls `IndexFunc`, performs cache invalidation and incremental community detection (all inside the write lock), then releases the lock. On success, it spawns a background goroutine (tracked via `d.wg.Add(1)`) for LSP enrichment. Requests are processed one at a time; there is never concurrent indexing.

**traceIngestLoop goroutine (optional):** Runs when `TraceConfig` is enabled. Opens a dedicated SQLite database connection (separate from the main store connection), creates a `SymbolResolver`, `Ingestor`, and `OTLPReceiver`, then starts the gRPC receiver. The goroutine runs two periodic tickers: a `BatchInterval` ticker that calls `FlushBatch` to ingest accumulated spans, and an hourly ticker that calls `DecayConfidence` to reduce confidence on stale runtime edges. On context cancellation, it performs a final `FlushBatch` with a background context to drain any remaining spans, then stops the `OTLPReceiver` and closes the database connection.

**main goroutine:** Calls `Start()`, which launches all goroutines, then blocks on `<-ctx.Done()`. When the context is cancelled (via `Stop()` or external signal), it calls `shutdown()`, which closes `d.indexQueue`, closes the `GitWatcher`, and calls `d.wg.Wait()` to block until all goroutines (including any in-flight enrichment goroutines) have exited.

## Read/Write Coordination

The daemon uses `sync.RWMutex` to coordinate between indexing (writes) and MCP queries (reads):

```
            ┌──────────────┐        ┌──────────────┐
            │  indexWorker  │        │  MCP handler  │
            │              │        │   (query)     │
            └──────┬───────┘        └──────┬───────┘
                   │                       │
            d.mu.Lock()              d.mu.RLock()
                   │                       │
            ┌──────┴───────┐        ┌──────┴───────┐
            │  run IndexFunc│        │  read graph   │
            │  + cache inval│        │  (read lock)  │
            │  + communities│        │              │
            │  + FTS rebuild│        │              │
            │  (write lock) │        └──────┴───────┘
            └──────┬───────┘               │
                   │                d.mu.RUnlock()
            d.mu.Unlock()
```

- **Queries hold the read lock.** Multiple agents can query the graph concurrently.
- **Indexing holds the write lock.** While the indexer is running (including cache invalidation, incremental community detection, and scoped FTS rebuild), all queries wait. This guarantees that queries never see a partially-indexed state.
- **Enrichment does not hold the write lock.** After indexing completes and the write lock is released, a background goroutine runs LSP enrichment. Enrichment writes individual edges to the store (via `PutEdge`/`DeleteEdge`), relying on SQLite's WAL mode for concurrent access rather than the daemon-level mutex.
- **GarbageCollect acquires the write lock.** The `Daemon.GarbageCollect` method takes `d.mu.Lock()` before calling the gcFunc callback, preventing concurrent index writes during the reachability sweep.

## Channel-Based Communication

| Channel | Direction | Buffer | Purpose |
|---------|-----------|--------|---------|
| `GitWatcher.events` | GitWatcher loop -> watchLoop | 64 | Carries `CommitEvent` values (repo path, old/new commit, file lists) |
| `Daemon.indexQueue` | watchLoop -> indexWorker | 128 | Carries `indexRequest` values (repo URL, path, changed files) |
| `GitWatcher.done` | GitWatcher loop -> Close() | 0 (signal) | Signals that the event loop has exited; `Close()` blocks on `<-done` |

Both the `events` and `indexQueue` channels use non-blocking sends. If the consumer falls behind, events are dropped rather than blocking the producer. This is a deliberate choice: a stale commit event is worthless because the next commit event will supersede it.

## Clean Shutdown

All goroutines are tracked with `sync.WaitGroup`. The shutdown sequence is:

1. Context is cancelled (via `Stop()` or signal).
2. `shutdown()` releases the daemon lockfile.
3. `shutdown()` closes `d.indexQueue`, causing `indexWorker` to drain remaining items and exit.
4. `shutdown()` closes the `GitWatcher` (which closes the fsnotify watcher, causing the internal loop to exit and close `done`; then Close blocks on `<-done`). The `watchLoop` goroutine exits because `ctx.Done()` fires or the events channel is closed.
5. `shutdown()` calls `d.wg.Wait()`, blocking until all goroutines (including any in-flight enrichment goroutines) have exited.

Enrichment goroutines check `ctx.Err()` at each loop iteration and exit promptly on cancellation.

## Producer-Consumer Indexer Pipeline

Inside `IndexRepo`, extraction and storage run as a concurrent pipeline:

```
┌──────────────────────────────────────────────────────────────────┐
│  IndexRepo producer-consumer pipeline                             │
│                                                                  │
│  ┌────────────────────────────────────────────┐                  │
│  │  Work channel (pre-filled with file indices)│                  │
│  │  workCh: buffered to len(work)              │                  │
│  └─────┬──────┬──────┬──────┬─────────────────┘                  │
│        │      │      │      │                                    │
│        ▼      ▼      ▼      ▼                                    │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐                                    │
│  │ W1 │ │ W2 │ │ W3 │ │ W4 │  (GOMAXPROCS extraction workers)   │
│  └──┬─┘ └──┬─┘ └──┬─┘ └──┬─┘                                    │
│     │      │      │      │                                       │
│     └──────┴──────┴──────┘                                       │
│              │                                                    │
│              ▼                                                    │
│     resultCh (buffered: numWorkers*2)                            │
│              │                                                    │
│              ▼                                                    │
│  ┌──────────────────────┐                                        │
│  │  Storage writer       │  (single goroutine, owns all DB writes)│
│  │  Flushes every 500    │                                        │
│  │  files via batch INSERTs                                      │
│  └──────────────────────┘                                        │
└──────────────────────────────────────────────────────────────────┘
```

Key properties:
- **Two WaitGroups, two phases.** `extractWg` tracks extraction workers; when all workers finish, a coordinator goroutine closes `resultCh`. The storage writer (which is the calling goroutine of `IndexRepo`, not a separate goroutine) reads from `resultCh` until it is closed.
- **Worker count.** `min(Concurrency (default GOMAXPROCS), len(work))`. Never exceeds available files.
- **Streaming commits.** The storage writer flushes to SQLite every 500 files (or at the end). Partial data persists on kill.
- **No shared mutable state between workers.** Each worker reads a file index from `workCh`, runs extraction in a fire-and-forget goroutine (for the CGO watchdog), and sends the result on `resultCh`. Workers do not share output arrays.

## CGO Watchdog Pattern

Tree-sitter calls cross the CGO boundary and are non-interruptible by Go's context cancellation. The indexer uses a fire-and-forget watchdog to enforce a per-file timeout:

```go
done := make(chan extractResult, 1)
go func(opts ExtractOptions) {
    r, f, err := idx.extractFile(ctx, opts)
    done <- extractResult{result: r, file: f, err: err}
}(opts)

timer := time.NewTimer(10 * time.Second)
select {
case er := <-done:
    timer.Stop()
    resultCh <- fileResult{...}
case <-timer.C:
    // Extraction stuck in CGO. Send empty result and move on.
    // The background goroutine will complete eventually; its result
    // is discarded (nobody reads from done after this point).
    resultCh <- fileResult{result: &ExtractResult{}}
}
```

If a file takes longer than 10 seconds (stuck in tree-sitter CGO), the worker moves on. The orphaned goroutine runs to completion but its result is never consumed. `context.WithTimeout` cannot cancel CGO calls, so this is the only safe mechanism for bounding latency.

## Thread-Safe Extractors

Tree-sitter parsers are **not goroutine-safe**. Each `Extract()` call creates its own parser:

- `GoTreeSitterExtractor.Extract()`: creates `sitter.NewParser()` per call, or reuses a pre-parsed tree passed via `opts.ParsedTree` (which is also created per-file by the first extractor to run).
- `TreeSitterExtractor.Extract()` (Python): creates `sitter.NewParser()` per call.
- The tree from parsing is returned via `result.ParsedTree` so subsequent extractors for the same file can reuse it. The tree is closed by the indexer after all extractors for that file finish.

This design means extraction workers can run in parallel without locks: no shared parser state exists.

## SQLite WAL Mode and Pragmas

The graph store uses SQLite in Write-Ahead Logging (WAL) mode with performance-tuned pragmas:

```sql
PRAGMA journal_mode=WAL
PRAGMA synchronous=NORMAL    -- WAL is safe with NORMAL (fsync on checkpoint, not every commit)
PRAGMA mmap_size=268435456   -- 256MB memory-mapped I/O for read-heavy workloads
PRAGMA cache_size=-64000     -- 64MB page cache (negative value = KB)
PRAGMA busy_timeout=5000     -- 5s retry on lock contention instead of immediate SQLITE_BUSY
PRAGMA temp_store=MEMORY     -- temp tables/indexes in memory
```

Concurrency guarantees:

- **Concurrent readers:** Multiple goroutines (MCP handlers, enrichment reads) can read simultaneously without blocking each other.
- **Single writer:** Only one goroutine can write at a time. SQLite serializes writes internally. The daemon-level `sync.RWMutex` ensures the indexer is the sole writer during bulk indexing; enrichment writes individual edges after the mutex is released.
- **No read-write blocking:** Readers do not block writers, and writers do not block readers. A reader sees a consistent snapshot of the database as of the moment it started reading, even if a writer commits during the read.
- **Busy timeout:** If a write conflicts with another write (e.g., enrichment PutEdge racing with enrichment DeleteEdge), SQLite retries for up to 5 seconds before returning SQLITE_BUSY.

### Batch INSERT Strategy

The storage writer uses multi-row INSERT statements wrapped in a single transaction:

- **Nodes:** chunks of 99 (990 parameters, under SQLite's 999 variable limit per statement)
- **Edges:** chunks of 100 (900 parameters)
- **Files:** chunks of 249 (996 parameters)

All batch operations use `INSERT OR REPLACE` (upsert) semantics. This eliminates row-level locking concerns; the entire batch commits atomically.

## In-Process Node/Edge Cache

`SQLiteStore` layers a `sync.Map`-based cache over SQLite for hot-path traversals (blast_radius can walk hundreds of edges):

- **nodeCache / edgeCache:** `sync.Map` instances keyed by `types.Hash`, storing `*types.Node` / `*types.Edge`. Thread-safe without explicit locking.
- **Bounded size:** `atomic.Int64` counters track entry count. When the cache reaches 50,000 entries (nodes or edges independently), the entire cache is cleared and rebuilt from scratch. This is a conservative full-eviction strategy.
- **Invalidation:** `InvalidateCache()` clears both caches completely. Called at the start of each index run so freshly written rows are not shadowed by stale cached values.
- **DeleteEdge / DeleteNodesByFile:** Evict affected entries from the cache on mutation.

## SubgraphCache (Result Cache)

The `SubgraphCache` in `internal/cache/subgraph.go` memoizes expensive query results (blast_radius, test_scope, context_for_task):

- **Thread-safe:** `sync.RWMutex` protects all operations. `Get` uses `RLock` for the fast path (entry found and not expired). `Put`, `Invalidate`, `Clear`, and `InvalidatePackages` use the exclusive lock.
- **TTL-bounded:** Each entry has a 1-hour TTL. Expired entries are removed on access.
- **Bounded size:** Maximum 10,000 entries (default). Random eviction on capacity overflow.
- **Merkle-keyed invalidation:** After each index run, the daemon computes a hierarchical tree diff (old vs. new) to identify changed packages, then calls `InvalidatePackages(changedPkgs, newTree)` to evict stale entries keyed by package root hashes.

## MCP Server Concurrency

The MCP server handles multiple concurrent client requests. Each request is dispatched to its own goroutine by the mcp-go library:

- **Stdio mode:** `ServeStdio` processes requests over stdin/stdout. The underlying library handles serialization/deserialization.
- **HTTP mode:** `ServeHTTP` starts an HTTP server where each connection's requests are handled concurrently.
- **Read-safety:** All MCP tool handlers read from the shared `types.GraphStore`. SQLite WAL mode allows concurrent reads. No MCP handler holds the daemon write lock.
- **Atomic counters:** Session counters (`contextCalls`, `symbolsServed`) use `atomic.Int64` for lock-free concurrent updates.
- **Background vector indexing:** On startup (when `KNOWING_EMBEDDINGS=1`), `buildVectorIndex` runs in a background goroutine to embed existing nodes. This uses the store's read path only.

## No Concurrency in the Retrieval Path

The `ContextEngine.ForTask` pipeline is fully sequential within a single request:

1. Keyword extraction (CPU, in-memory)
2. Cache lookup (SubgraphCache.Get, takes RLock briefly)
3. Tiered keyword search (sequential DB queries)
4. BM25 full-text search (single SQLite query)
5. Vector search (if enabled; single call)
6. Equivalence class matching (sequential)
7. Reciprocal Rank Fusion (CPU, in-memory)
8. Random Walk with Restart (in-memory iteration over pre-loaded adjacency map)
9. HITS computation (in-memory iteration)
10. Symbol ranking and knapsack packing (CPU, in-memory)
11. Cache store (SubgraphCache.Put, takes Lock briefly)

No goroutines are spawned within `ForTask`. The RWR's `buildAdjacencyMap` does a BFS pre-load of edges (4 hops from seeds) into in-memory maps, then the iteration loop operates entirely on those maps with zero concurrent access.

## LSP Enrichment is Sequential

Language servers do not support concurrent requests from the same client. The LSP protocol is request-response with a single message stream per client connection. The enricher iterates all detected language servers sequentially, processing each one in turn:

1. Auto-detect available language servers via project markers and PATH binaries (`DetectLSPServers`).
2. For each detected server:
   a. Start the server process (`lsp.NewLSPClient`, `client.Initialize`).
   b. Open source files matching the server's extensions (`textDocument/didOpen`, sequential).
   c. Wait for workspace ready (`WaitForWorkspaceReadyTimeout`, up to 120s for large projects).
   d. Upgrade ast_inferred call edges: for each edge with call-site positions, query `GetDefinition` (sequential).
   e. Discover new edges: for each file, query `GetDocumentSymbols`, then `GetImplementation`/`GetReferences` per symbol (sequential).
   f. Close all files and shut down the language server (`Shutdown`).
3. Create phantom external nodes for dangling edge targets (sequential DB writes).

This is an inherent limitation of the LSP protocol, not a design choice. The enricher could use multiple language server instances for parallelism, but the memory cost of multiple server instances (each loading the full dependency graph) outweighs the latency benefit for typical repo sizes.

**Enrichment writes do not hold the daemon write lock.** After the write lock is released, enrichment uses SQLite's WAL mode for safe concurrent access (the store handles `busy_timeout` internally). This means MCP queries can proceed during enrichment.

## Community Detection Runs Synchronously Inside Write Lock

Incremental community detection (`runIncrementalCommunities`) executes inside the `indexWorker`'s write-lock window:

1. Load nodes for the repo (`NodesByName`).
2. Build adjacency list from edges (up to 5,000 nodes for performance cap).
3. Load previous community assignments from graph notes.
4. Identify changed nodes using the Merkle diff's `changedPkgs`.
5. Run `DetectIncremental` (label propagation or Louvain, only changed nodes can move).
6. Delta-save changed assignments back to notes.

This is safe because it runs within `d.mu.Lock()`, so no concurrent reads can see an inconsistent state. The 5,000-node cap prevents community detection from dominating the write-lock hold time.

## FTS Rebuild

The Full-Text Search index rebuild has two modes:

**CLI mode (synchronous):** After `IndexRepo` returns, `RebuildFTS` runs synchronously in the same goroutine. This ensures FTS is populated before the process exits. Running FTS as a background goroutine was a previous design that caused a race condition: CLI processes exit immediately after `IndexRepo` returns, killing the goroutine before FTS completes.

**Daemon mode (scoped, inside write lock):** After indexing, the daemon calls `RebuildFTSForPackages(changedPkgs)` inside the write lock. This rebuilds only the FTS entries for packages that changed (proportional to the diff, not the total graph).

`RebuildFTS` itself uses internal parallelism for the CPU-bound `splitForFTS` computation (8 workers splitting CamelCase/snake_case tokens), then does a sequential batch INSERT within a single transaction. The parallelism is safe because workers write to a pre-sized `prepped` array at their own index ranges.

## Authorship Extraction (Parallel, Best-Effort)

After the main extraction pipeline completes, git blame authorship extraction runs in parallel:

- Worker count: same as extraction workers (`min(GOMAXPROCS, len(files))`)
- Pattern: pre-sized `blameResults` array indexed by file position (no shared mutable state)
- Each worker calls `git blame` as a subprocess for its file, then stores results at its array index
- `blameWg.Wait()` joins all workers before collecting results into a flat list
- Batch-stored to SQLite after collection

This is safe because each worker writes to a distinct index in the results array. No locks are needed.

## Snapshot Computation

Snapshots are computed in-memory from pipeline data (the `allEdges` and `allNodes` slices accumulated during extraction). There is no DB re-read for snapshot computation. `ComputeSnapshotFromEdges` receives `edgeInputs` built by iterating the in-memory slices. This avoids WAL contention between the snapshot reader and the batch writer.

## Auto-GC After Indexing

After each successful `IndexRepo` call, the indexer checks the `edge_events` table count. If it exceeds 5,000 rows, it triggers garbage collection (keeps the 10 most recent snapshots, prunes older ones). In CLI mode, this runs inline after the write completes. In daemon mode, the `GarbageCollect` method acquires the write lock to prevent conflicts.

This matches git's `gc.auto` pattern (triggers pack-objects when loose object count exceeds 6,700).

## Daemon Lockfile

The daemon acquires a lockfile on startup (`AcquireLockfile`) to prevent multiple instances from writing to the same database. The lockfile is released in `shutdown()`. This avoids SQLite corruption from concurrent writers in separate processes (WAL mode handles concurrent goroutines within one process, but not concurrent processes writing without coordination).

## Changed Files Protected by Mutex

The `Indexer.lastChangedFiles` field stores file paths from the most recent index run so the daemon can scope LSP enrichment. Access is protected by `idx.changedMu` (a `sync.Mutex`). The indexer writes the list under the lock at the end of `IndexRepo`; the daemon reads it via `LastChangedFiles()` which copies under the same lock.

## Why This Model

The daemon is a single process on a single machine. It does not need distributed consensus, message brokers, or coordination services. Go's goroutines, channels, and mutexes provide exactly the concurrency primitives needed:

- Channels for producer-consumer pipelines (watcher to indexer, extraction workers to storage writer).
- `sync.RWMutex` for read/write partitioning (queries vs. indexing).
- `sync.WaitGroup` for clean shutdown (all goroutines tracked).
- `sync.Map` for lock-free concurrent read caching (node/edge cache).
- Worker pools with pre-sized arrays for CPU-bound parallelism (tree-sitter extraction, authorship, FTS split).
- Fire-and-forget goroutines with timer-based watchdogs for non-interruptible CGO.
- Sequential processing where the external system requires it (LSP, SQLite single-writer).
- `sync.RWMutex` for the SubgraphCache (read-heavy, infrequent writes on invalidation).
