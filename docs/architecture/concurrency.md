# Concurrency Model

The daemon is a single process with concurrent goroutines, not a distributed system. All coordination is in-process using Go's standard concurrency primitives.

## Goroutine Architecture

The daemon runs three primary goroutines, plus optional goroutines for MCP serving, LSP enrichment, and trace ingestion:

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Daemon Process                               │
│                                                                      │
│  ┌─────────────┐   indexCh    ┌──────────────┐                       │
│  │  watchLoop   │────────────>│  indexWorker  │                       │
│  │  goroutine   │  (buffered  │  goroutine    │                       │
│  │              │   chan, 128) │              │                       │
│  └──────┬───────┘             └──────┬───────┘                       │
│         │                            │                               │
│    reads from                   on success:                          │
│    GitWatcher.Events()          spawns background                     │
│    (fsnotify loop)              enrichment goroutine                  │
│         │                            │                               │
│  ┌──────┴───────┐             ┌──────┴───────┐                       │
│  │  GitWatcher   │             │  enrichment  │                       │
│  │  event loop   │             │  goroutine   │                       │
│  │  (debounce)   │             │  (per index) │                       │
│  └───────────────┘             └──────────────┘                       │
│                                                                      │
│  ┌───────────────┐            ┌───────────────────────────────────┐   │
│  │  MCP Server   │  (opt.)    │  traceIngestLoop goroutine (opt.) │   │
│  │  goroutine    │            │  ├── OTLPReceiver (gRPC server)   │   │
│  └───────────────┘            │  ├── batchTicker (FlushBatch)     │   │
│                               │  └── decayTicker (DecayConfidence)│   │
│                               └───────────────────────────────────┘   │
│                                                                      │
│  main goroutine: blocks on <-ctx.Done(), then shutdown()             │
└──────────────────────────────────────────────────────────────────────┘
```

**watchLoop goroutine:** Reads `CommitEvent` values from the `GitWatcher.Events()` channel. For each event, it combines changed, added, and deleted file lists into a single `indexRequest` and sends it to `indexCh`. If the channel is full (128-item buffer), the event is dropped. This goroutine never blocks on indexing; it only enqueues.

**indexWorker goroutine:** Reads `indexRequest` values from `indexCh` sequentially. For each request, it resolves the HEAD commit, acquires the daemon's write lock, calls `IndexFunc`, and releases the write lock. On success, it spawns a background goroutine for LSP enrichment. Requests are processed one at a time; there is never concurrent indexing.

**traceIngestLoop goroutine (optional):** Runs when `TraceConfig` is enabled. Opens a dedicated SQLite database connection (separate from the main store connection), creates a `SymbolResolver`, `Ingestor`, and `OTLPReceiver`, then starts the gRPC receiver. The goroutine runs two periodic tickers: a `BatchInterval` ticker that calls `FlushBatch` to ingest accumulated spans, and an hourly ticker that calls `DecayConfidence` to reduce confidence on stale runtime edges. On context cancellation, it performs a final `FlushBatch` with a background context to drain any remaining spans, then stops the `OTLPReceiver` and closes the database connection.

**main goroutine:** Calls `Start()`, which launches all goroutines, then blocks on `<-ctx.Done()`. When the context is cancelled (via `Stop()` or external signal), it calls `shutdown()`, which closes `indexCh`, closes the `GitWatcher`, and calls `wg.Wait()` to block until all goroutines have exited.

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
            │  (write lock) │        │  (read lock)  │
            └──────┬───────┘        └──────┴───────┘
                   │                       │
            d.mu.Unlock()            d.mu.RUnlock()
```

- **Queries hold the read lock.** Multiple agents can query the graph concurrently.
- **Indexing holds the write lock.** While the indexer is running, all queries wait. This guarantees that queries never see a partially-indexed state.
- **Enrichment does not hold the write lock.** After indexing completes and the write lock is released, a background goroutine runs LSP enrichment. Enrichment writes individual edges to the store (via `PutEdge`/`DeleteEdge`), relying on SQLite's WAL mode for concurrent access rather than the daemon-level mutex.

## Channel-Based Communication

| Channel | Direction | Buffer | Purpose |
|---------|-----------|--------|---------|
| `GitWatcher.events` | GitWatcher loop → watchLoop | 64 | Carries `CommitEvent` values (repo path, old/new commit, file lists) |
| `Daemon.indexQueue` | watchLoop → indexWorker | 128 | Carries `indexRequest` values (repo URL, path, changed files) |
| `GitWatcher.done` | GitWatcher loop → Close() | 0 (signal) | Signals that the event loop has exited; `Close()` blocks on `<-done` |

Both the `events` and `indexQueue` channels use non-blocking sends. If the consumer falls behind, events are dropped rather than blocking the producer. This is a deliberate choice: a stale commit event is worthless because the next commit event will supersede it.

## Clean Shutdown

All goroutines are tracked with `sync.WaitGroup`. The shutdown sequence is:

1. Context is cancelled (via `Stop()` or signal).
2. `shutdown()` closes `indexCh`, causing `indexWorker` to drain and exit.
3. `shutdown()` closes the `GitWatcher`, causing the fsnotify loop and `watchLoop` to exit.
4. `shutdown()` calls `wg.Wait()`, blocking until all goroutines (including any in-flight enrichment goroutines) have exited.

Enrichment goroutines check `ctx.Err()` at each loop iteration and exit promptly on cancellation.

## Worker Pool for Extraction

Tier 1 extraction (tree-sitter) uses a fan-out/fan-in worker pool:

```
┌──────────────────────────────────────────────────────┐
│  parallelExtract(work, numWorkers)                    │
│                                                      │
│  work channel (pre-buffered, all items enqueued)      │
│       │  │  │  │                                     │
│       ▼  ▼  ▼  ▼                                     │
│  ┌────┐ ┌────┐ ┌────┐ ┌────┐                         │
│  │ W1 │ │ W2 │ │ W3 │ │ W4 │  (GOMAXPROCS workers)  │
│  └──┬─┘ └──┬─┘ └──┬─┘ └──┬─┘                         │
│     │      │      │      │                           │
│     ▼      ▼      ▼      ▼                           │
│  results[0]  results[1]  results[2]  results[3]       │
│  (pre-sized array, indexed by submission order)       │
└──────────────────────────────────────────────────────┘
```

Key properties:
- **No shared mutable state.** Each worker writes to its own index in a pre-allocated results array. No locks on the output path.
- **Deterministic output.** Results are ordered by submission index, not completion order. Same input always produces same output order.
- **Bounded concurrency.** Worker count is `min(runtime.GOMAXPROCS, len(work))`.
- **Context-aware.** Workers check `ctx.Err()` before each extraction and return the context error for remaining items on cancellation.

## LSP Enrichment is Sequential

Language servers (gopls, pyright, rust-analyzer) do not support concurrent requests from the same client. The LSP protocol is request-response with a single message stream per client connection. The enricher iterates all detected language servers sequentially, processing each one in turn:

1. Auto-detect available language servers via project markers and PATH binaries (`DetectLSPServers`).
2. For each detected server:
   a. Open source files matching the server's extensions via `openFilesForLanguage` (`textDocument/didOpen`, sequential).
   b. For each `ast_inferred` edge with call-site positions, query `GetDefinition` (sequential).
   c. For each file, query `GetDocumentSymbols`, then `GetImplementation`/`GetReferences` per symbol (sequential).
   d. Close all files and shut down the language server.
3. Repeat for the next language server.

This is an inherent limitation of the LSP protocol, not a design choice. The enricher could use multiple language server instances for parallelism, but the memory cost of multiple server instances (each loading the full dependency graph) outweighs the latency benefit for typical repo sizes.

## SQLite WAL Mode

The graph store uses SQLite in Write-Ahead Logging (WAL) mode:

- **Concurrent readers:** Multiple goroutines (MCP handlers, enrichment reads) can read simultaneously without blocking each other.
- **Single writer:** Only one goroutine can write at a time. SQLite serializes writes internally. The daemon-level `sync.RWMutex` ensures the indexer is the sole writer during bulk indexing; enrichment writes individual edges after the mutex is released.
- **No read-write blocking:** Readers do not block writers, and writers do not block readers. A reader sees a consistent snapshot of the database as of the moment it started reading, even if a writer commits during the read.

## Auto-GC After Indexing

After each successful `IndexRepo` call, the indexer checks the `edge_events` table count. If it exceeds 5,000 rows, it triggers garbage collection (keeps the 10 most recent snapshots, prunes older ones). This runs inline in the index worker goroutine after the write lock is released. GC is fast (~70ms) and infrequent (only fires when edge_events accumulate past the threshold).

This matches git's `gc.auto` pattern (triggers pack-objects when loose object count exceeds 6,700).

## Daemon Lockfile

The daemon acquires a lockfile on startup to prevent multiple instances from writing to the same database. The lockfile is released on clean shutdown. This avoids SQLite corruption from concurrent writers in separate processes (WAL mode handles concurrent goroutines within one process, but not concurrent processes writing without coordination).

## Why This Model

The daemon is a single process on a single machine. It does not need distributed consensus, message brokers, or coordination services. Go's goroutines, channels, and mutexes provide exactly the concurrency primitives needed:

- Channels for producer-consumer pipelines (watcher to indexer).
- `sync.RWMutex` for read/write partitioning (queries vs. indexing).
- `sync.WaitGroup` for clean shutdown (all goroutines tracked).
- Worker pools for CPU-bound parallelism (tree-sitter extraction).
- Sequential processing where the external system requires it (LSP).
