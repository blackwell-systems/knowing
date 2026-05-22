# Concurrency Primer

This guide explains where knowing uses Go's concurrency primitives and, more importantly, where it does not. Most of the codebase is sequential. Concurrency exists in exactly four subsystems, each for a specific reason. If you are adding code to knowing, read this before reaching for `go`.

## Overview

knowing uses goroutines, channels, `sync.WaitGroup`, `sync.Mutex`, and `sync.RWMutex` in specific places. The indexer pipeline is the main concurrent subsystem. The retrieval path (RWR, HITS, ranking, packing) is entirely sequential. The MCP server handles concurrent requests but delegates all coordination to SQLite's WAL mode.

The guiding principle: add concurrency only where a measurable bottleneck exists. Extraction is CPU-bound (tree-sitter parsing). SQLite writes are IO-bound. Separating them into a pipeline doubles throughput. Retrieval already completes in under 100ms, so parallelizing it would add complexity for no user benefit.

## The Indexer Pipeline

The indexer (`internal/indexer/indexer.go`) uses a producer-consumer pipeline to overlap CPU-bound extraction with IO-bound storage.

**Architecture:**

```
[workCh] --> N extraction workers --> [resultCh] --> 1 storage writer --> SQLite
```

- **N extraction workers** (defaults to `runtime.GOMAXPROCS(0)`) read file indices from `workCh` and produce `fileResult` values into `resultCh`.
- **1 writer goroutine** (the main goroutine of `IndexRepo`) reads from `resultCh` and batches results into SQLite.
- **`extractWg`** tracks when all workers have finished, so the feeder goroutine knows when to close `resultCh`.
- **Channel backpressure:** `resultCh` is buffered at `numWorkers*2`. If the writer falls behind, workers block on the send rather than accumulating unbounded results in memory.

```go
resultCh := make(chan fileResult, numWorkers*2)
```

**Why this design:** extraction is CPU-bound (tree-sitter parsing takes 1-10ms per file). SQLite writes are IO-bound (batch INSERT across a transaction). Running them in the same goroutine means the CPU sits idle during writes and the disk sits idle during parsing. The pipeline keeps both saturated.

## The CGO Watchdog Pattern

**Problem:** tree-sitter is a C library called via CGO. Go's `context.WithTimeout` cancels goroutines by closing a channel, but CGO calls cannot be interrupted by Go. A file with pathological nesting (e.g., deeply nested JSON) can block a tree-sitter parse for minutes.

**Solution:** fire-and-forget goroutine with a timer select.

```go
done := make(chan extractResult, 1)
go func() {
    r, f, err := idx.extractFile(ctx, opts)
    done <- extractResult{result: r, file: f, err: err}
}()

timer := time.NewTimer(10 * time.Second)
select {
case er := <-done:
    timer.Stop()
    resultCh <- fileResult{...}
case <-timer.C:
    // Extraction stuck in CGO. Skip this file.
    resultCh <- fileResult{result: &types.ExtractResult{}, ...}
}
```

If the timer fires first, the file is skipped. The background goroutine eventually completes and its result is discarded (nobody reads from `done`). The goroutine "leaks" in the sense that it continues running, but Go's runtime cleans it up on process exit.

**Why this is safe:** extraction is stateless per-file. A leaked goroutine holds no locks, writes nothing to the database, and mutates no shared state. The only cost is memory for the parse tree until the CGO call returns.

## SQLite Single-Writer

SQLite in WAL (Write-Ahead Logging) mode allows any number of concurrent readers but exactly one writer at a time. knowing's architecture respects this constraint at every level.

**Pragmas** (set in `internal/store/sqlite.go`):

| Pragma | Value | Why |
|--------|-------|-----|
| `journal_mode` | WAL | Concurrent readers during writes |
| `synchronous` | NORMAL | fsync on checkpoint only, not every commit |
| `busy_timeout` | 5000 | Retry for 5s on lock contention instead of immediate SQLITE_BUSY |
| `mmap_size` | 268435456 | 256MB memory-mapped IO for read-heavy workloads |
| `cache_size` | -64000 | 64MB page cache |

**Batch INSERT strategy:** multi-row INSERT statements pack multiple rows per SQL statement to reduce transaction overhead. The chunk sizes are chosen to stay under SQLite's 999-variable limit:

- Edges: 100 per statement (9 params each = 900)
- Nodes: 99 per statement (10 params each = 990)
- Files: 249 per statement (4 params each = 996)

**Design rule:** all writes flow through the single writer goroutine in the indexer pipeline. Extraction workers never touch the database directly. They produce results into `resultCh`, and the writer goroutine consumes them.

## Thread-Safe Extractors

Each `Extract()` call creates its own tree-sitter parser instance. Parsers are cheap to create (approximately 1 microsecond) but are NOT goroutine-safe. The tree-sitter C library uses internal state that is mutated during parsing.

The extractor struct itself (e.g., `GoExtractor`, `PythonExtractor`) is stateless: it has no mutable fields. This means many goroutines can call `Extract()` on the same extractor instance concurrently, because each call allocates its own parser on the stack.

**Rules:**
- Never share a `*sitter.Parser` between goroutines.
- Never store a parser as a field on an extractor struct.
- Creating a parser per-call is intentional, not wasteful.

When multiple extractors handle the same file (e.g., Go extractor + proto extractor), the first one that parses sets `opts.ParsedTree`; subsequent extractors reuse the same parsed tree. This reuse happens within a single goroutine (the extraction loop for one file), so there is no concurrency concern.

## No Concurrency in Retrieval

The context engine (`internal/context/walk.go`) and all retrieval paths (`ForTask`, `ForFiles`, `ForPR`) are fully sequential. Here is why:

- **RWR (Random Walk with Restart):** pre-loads the adjacency map via BFS from seeds (depth-limited to 4 hops), then iterates over the in-memory map. No goroutines. The BFS and iteration are pure computation on maps and slices.
- **HITS:** runs on a pre-selected subset of nodes. Pure linear algebra on slices.
- **Ranking and packing:** sorting and knapsack on scored nodes. Pure computation.

**Why not parallelize?** Retrieval completes in under 100ms for typical queries (tested on graphs with 200K+ edges). The bottleneck is the initial SQLite queries to load the adjacency map, which cannot be parallelized (they share a single DB connection). Adding goroutines would introduce synchronization overhead for no measurable benefit.

## MCP Server Request Handling

The MCP server (`internal/mcp/server.go`) handles multiple client requests concurrently:

- **Stdio mode:** the mcp-go library multiplexes JSON-RPC requests over stdin/stdout, dispatching each to its own goroutine.
- **HTTP mode:** standard `net/http` server spawns a goroutine per connection.

All request handlers share a single `SQLiteStore` instance. This is safe because:

1. WAL mode allows unlimited concurrent readers.
2. Read-only handlers (blast_radius, context_for_task, etc.) only call SELECT queries.
3. Write handlers (index_repo, feedback) go through SQLite's internal serialization. If two writes overlap, `busy_timeout=5000` causes the second to retry for up to 5 seconds.

**SubgraphCache:** the result cache (`internal/cache/subgraph.go`) is protected by `sync.RWMutex`. Multiple goroutines can read cached results simultaneously (shared lock). Cache writes and invalidations acquire an exclusive lock. This is safe because cache operations are fast (map lookup/insert) and never hold the lock across IO operations.

**Atomic counters:** session-level metrics (`contextCalls`, `symbolsServed`) use `atomic.Int64` for lock-free concurrent increments.

## Daemon File Watcher

The daemon (`internal/daemon/daemon.go`) runs three concurrent goroutines coordinated by a `sync.RWMutex`:

1. **watchLoop:** reads `CommitEvent` values from `GitWatcher` and enqueues index requests into a buffered channel.
2. **indexWorker:** drains the index queue sequentially, holding the daemon's write lock during each index run.
3. **MCP server:** serves queries, acquiring a read lock for each request.

**Debounce pattern:** the `GitWatcher` monitors `.git/HEAD` and ref files via fsnotify. A single git operation (commit, rebase, merge) can trigger multiple file writes in rapid succession. Each write resets a per-repo `time.AfterFunc` timer (default 500ms). Only when the timer fires (no writes for 500ms) does the watcher emit a `CommitEvent`. This coalesces rapid changes into a single re-index.

```go
p.timer = time.AfterFunc(gw.debounce, func() {
    gw.handleCommitChange(repoPath)
})
```

**Lock protocol:**
- `indexWorker` acquires `d.mu.Lock()` (exclusive) during indexing. MCP queries block until indexing completes.
- MCP handlers acquire `d.mu.RLock()` (shared). Multiple queries run concurrently.
- After indexing completes, a background goroutine spawns for LSP enrichment. This goroutine does NOT hold the write lock, so queries resume immediately.

**Why exclusive lock during indexing?** Without it, a query could read a half-written graph (some files indexed, others not). The lock ensures readers always see a complete, consistent snapshot.

## Common Pitfalls for Contributors

1. **Don't add goroutines to the retrieval path.** It is fast enough (<100ms). Goroutines add complexity, make debugging harder, and provide no measurable speedup for in-memory computation.

2. **Don't share tree-sitter parsers between goroutines.** The C library uses internal mutable state. Create a new parser per `Extract()` call.

3. **Don't write to SQLite from extraction workers.** Use the `resultCh` channel. The single-writer goroutine owns all database mutations during indexing. Writing from workers causes SQLITE_BUSY errors and data races.

4. **Don't use `context.WithTimeout` to cancel CGO calls.** It does not work. Go contexts cancel by closing a channel; CGO calls cannot observe channel state. Use the watchdog pattern (fire-and-forget goroutine + timer select) instead.

5. **Don't hold locks across channel sends.** If the channel is full (backpressure), the send blocks. If you are holding a mutex, other goroutines waiting for that mutex also block. This creates a deadlock where the channel consumer needs the lock to make progress but cannot acquire it.

6. **Don't bypass the daemon's `sync.RWMutex`.** If you are adding a new background task that modifies the graph, it must acquire the write lock. If it only reads, it must acquire the read lock. Forgetting this causes readers to see inconsistent state.

7. **Don't run expensive computation inside a lock.** The daemon's write lock blocks all MCP queries. Keep the critical section as short as possible: do expensive work (tree-sitter parsing, git blame) outside the lock, then acquire the lock only for the final database writes.

## Quick Reference Table

| Subsystem | Primitives Used | Concurrency Level |
|-----------|----------------|-------------------|
| Indexer extraction | goroutines, channels, WaitGroup | N workers (GOMAXPROCS) |
| Indexer storage | single goroutine reading from channel | 1 writer |
| CGO watchdog | goroutine, select, Timer | 1 per file (fire-and-forget) |
| SQLite store | WAL mode, busy_timeout pragma | N readers, 1 writer |
| Node/edge cache | sync.Map, atomic.Int64 | lock-free reads |
| SubgraphCache | sync.RWMutex | N readers, exclusive writes |
| MCP server | goroutine per request (net/http / stdio) | unbounded readers |
| Daemon coordination | sync.RWMutex, buffered channel | write lock during index |
| File watcher | fsnotify, time.AfterFunc, Mutex | 1 event loop goroutine |
| RWR / HITS / ranking | none | fully sequential |
| FTS rebuild | WaitGroup, goroutines (split phase) | 8 workers for string splitting |
| Git blame extraction | WaitGroup, channel | N workers (GOMAXPROCS) |
