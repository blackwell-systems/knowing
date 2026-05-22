# Proposal: Storage Backend Evaluation

## Problem

SQLite's single-writer model and FTS5 rebuild cost make indexing large repos (kubernetes: 3.5M LOC, 268K edges, 117K nodes) take minutes in the finalization phase even after extraction is fast. The pipeline writes edges in seconds, but FTS rebuild takes 5+ minutes.

## Current Bottlenecks (measured on kubernetes)

| Phase | Time | Bottleneck |
|-------|------|-----------|
| File walk + filtering | < 1s | N/A |
| Parallel extraction (4877 files) | ~14s wall | CPU-bound (tree-sitter) |
| Streaming batch INSERT (268K edges) | ~4s | SQLite WAL writes |
| FTS5 rebuild (117K nodes) | 5-10 min | `INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild')` |
| Snapshot computation (Merkle tree) | ~2s | CPU-bound (sort + hash) |

FTS5 rebuild is 80%+ of total time. It's a SQLite-internal operation that re-tokenizes all content and rebuilds the inverted index. Can't be parallelized, can't be streamed.

## Options

### 1. SQLite + deferred FTS (minimal change)

Keep SQLite. Skip FTS rebuild during index. Build it lazily on first BM25 query or in a background goroutine after index returns.

| Pro | Con |
|-----|-----|
| Zero migration | First text search is slow (cold FTS) |
| Graph immediately queryable | BM25 unavailable until rebuild completes |
| Simplest change (3 lines) | |

**Expected kubernetes index time: ~15s** (extraction + batch insert + snapshot, no FTS)

### 2. SQLite + external search index (Bleve/Tantivy)

Keep SQLite for graph storage. Use a dedicated search library for full-text search instead of FTS5.

| Option | Write speed | Search quality | Deployment |
|--------|------------|---------------|-----------|
| Bleve (Go) | Parallel writers | Good (BM25 + fuzzy) | Single binary |
| Tantivy (Rust via CGO) | Very fast parallel | Excellent (Lucene-grade) | CGO dependency |
| Custom inverted index | Full control | Basic BM25 | Zero deps |

**Expected kubernetes index time: ~20s** (extraction + graph INSERT + parallel search index build)

### 3. DuckDB (columnar analytics DB)

Replace SQLite with DuckDB for the graph store. DuckDB is optimized for bulk INSERT and analytical queries (which graph traversal resembles).

| Pro | Con |
|-----|-----|
| 10-100x faster bulk INSERT | No WAL mode (less concurrent read support) |
| Columnar compression (smaller on disk) | Different SQL dialect |
| Parallel query execution | Less mature ecosystem |
| Native Parquet export | Go binding less battle-tested |

**Expected kubernetes index time: ~5-10s** (DuckDB bulk load is extremely fast)

### 4. BadgerDB / Pebble (LSM key-value)

Replace SQLite with an LSM tree (like LevelDB/RocksDB but pure Go). Optimized for write-heavy workloads.

| Pro | Con |
|-----|-----|
| Extremely fast writes (append-only log) | No SQL (custom query layer needed) |
| Built-in compression | Lose SQLite's ad-hoc query ability |
| Concurrent writers | Need custom FTS implementation |
| Pure Go (no CGO) | More code to maintain |

**Expected kubernetes index time: ~5s** (write-optimized, no FTS bottleneck if using custom index)

### 5. Hybrid: SQLite for queries + flat files for bulk ingest

Write extraction results to a binary flat file during indexing. After extraction, bulk-load into SQLite using `.import` or custom page construction. Separate the write path from the query path.

| Pro | Con |
|-----|-----|
| Fastest possible write (sequential file I/O) | Two-phase: write then load |
| SQLite stays for queries (no migration) | Custom binary format to maintain |
| Existing query code unchanged | Slight complexity increase |

**Expected kubernetes index time: ~10s** (write to file in parallel, bulk-load to SQLite, skip FTS during load)

## Recommendation

**Start with Option 1 (deferred FTS).** It's 3 lines of code and drops kubernetes from 10+ min to ~15s. The graph is immediately queryable for everything except text search. FTS builds in the background.

**If that's not enough:** Option 5 (hybrid flat file + SQLite bulk load) keeps the existing query infrastructure while fixing the write path.

**If we're redesigning:** Option 3 (DuckDB) is the most interesting for a graph workload. Its columnar storage and parallel execution map well to "scan all edges of type X in package Y" patterns. But it's a major migration.

**Don't do:** Option 4 (key-value store). Losing SQL means rewriting every query in the system. The graph traversal code relies heavily on SQLite's recursive CTEs.

## Decision Criteria

1. Does the benchmark need to run end-to-end today? → Option 1 (defer FTS)
2. Is write speed the primary product concern? → Option 3 or 5
3. Is the portable single-file property essential? → Stay SQLite (Options 1, 2, 5)
4. Are we willing to maintain two storage engines? → Option 2 or 5
