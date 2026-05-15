# Requirements: knowing

## Language + Ecosystem

- **Language:** Go
- **Module path:** `github.com/blackwell-systems/knowing`
- **Go version:** 1.23+
- **Key dependencies:**
  - `golang.org/x/tools/go/packages` (Go type resolution for indexer)
  - `github.com/mattn/go-sqlite3` or `modernc.org/sqlite` (SQLite, prefer modernc for CGo-free)
  - `github.com/cockroachdb/pebble` (adjacency-list acceleration index, deferred until benchmarks justify)
  - `github.com/mark3labs/mcp-go` (MCP server framework)
  - `github.com/fsnotify/fsnotify` (file watching)
  - `github.com/smacker/go-tree-sitter` (multi-language AST parsing)

## Project Type

Persistent daemon exposing an MCP server (stdio and HTTP). Single Go binary.

## Key Concerns (major responsibility areas)

1. **Graph Store** (`internal/store/`): content-addressed storage behind `GraphStore` interface. SQLite backend. Nodes, edges, files, repos, snapshots, edge events, schema migrations. The artifact.
2. **Indexer** (`internal/indexer/`): crawls repositories, computes content hashes, resolves symbols, produces nodes and edges. Language-agnostic extractor framework with a common `Extractor` interface. Two implementations: Go extractor (`go/packages` for full type resolution) and tree-sitter extractor (multi-language AST parsing, initially targeting Python or TypeScript as proof the interface works).
3. **Snapshot Manager** (`internal/snapshot/`): Merkle root computation, snapshot chain management, Merkle diff between snapshots, garbage collection of old snapshots.
4. **MCP Server** (`internal/mcp/`): MCP tool handlers (`cross_repo_callers`, `blast_radius`, `trace_dataflow`, `repo_graph`, `stale_edges`, `ownership`, `snapshot_diff`, `semantic_diff`, `pr_impact`, `index_repo`, `graph_query`). Routes queries to the graph store.
5. **Daemon** (`internal/daemon/`): long-lived process lifecycle, file watcher integration (fsnotify + git hooks), incremental reindex orchestration, concurrent read/write coordination.
6. **Types** (`internal/types/`): all shared interfaces (`GraphStore`, `ComputationCache`, `TraceIngestor`), domain types (`Hash`, `Node`, `Edge`, `File`, `Snapshot`, `EdgeEvent`, `DerivedResult`), provenance types, traversal result types. The contract layer.

## Entry Points

- `cmd/knowing/main.go`: CLI entry point. Subcommands: `serve` (daemon), `index` (one-shot indexing), `query` (CLI queries), `version`.

## Storage

- **Primary:** SQLite with WAL mode. Single file is the portable artifact.
- **Schema:** See `docs/architecture.md` decision #9 for full schema (repos, files, nodes, edges, edge_events, snapshots, schema_version tables).
- **Migrations:** Embedded numbered SQL files in `internal/store/migrations/`. Applied on startup via `//go:embed`.
- **Future:** Pebble adjacency-list acceleration index alongside SQLite. Not in bootstrap scope.

## External Integrations

- **MCP:** stdio and HTTP transport via `mcp-go` library.
- **Git:** shell out to `git` for commit hashes, file watching triggers, blame metadata.
- **Future (not in bootstrap):** OpenTelemetry trace ingestion, GitHub Actions integration.

## Source Codebase

No existing implementation code. Architecture designed from scratch across 15 decisions in `docs/architecture.md`. Key interfaces are already drafted in the architecture doc:

- `GraphStore` interface (decision #10): all graph read/write operations
- `ComputationCache` interface (decision #12): content-addressed derived results
- `TraceIngestor` interface (decision #13): runtime trace ingestion (future)
- `HybridStore` sketch (decision #9): SQLite + Pebble routing (future)

**Files to read for domain model:**
- `docs/architecture.md` (full architecture, interfaces, schemas, decisions)
- `docs/roadmap.md` (workstreams and dependency constraints)
- `README.md` (vision, MCP tools, design goals)

## Architectural Decisions Already Made

All 15 decisions in `docs/architecture.md` are binding. Key constraints for the scout:

1. **Content-addressed:** every node, edge, and snapshot is identified by its content hash (SHA-256). Hash computation formulas are specified in decision #1.
2. **Symbol identity:** `{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}` (decision #2).
3. **Event sourcing:** edges are never mutated. New indexing runs produce new edge events. Old events remain (decision #3).
4. **Provenance:** every edge carries source, confidence, indexer_version, source_commit, source_file_hash, timestamp (decision #4).
5. **Deterministic:** same repo at same commit = same graph hash. No map iteration in output paths, no timestamps in hash inputs, no randomness (decision #8).
6. **GraphStore interface:** all graph operations go through the abstract interface. No SQL outside the SQLite backend (decision #10).
7. **Artifact-boundary separation:** execution plane produces the graph, intelligence plane interprets it. Intelligence features have read-only GraphStore access (decision #15).
8. **Schema migrations:** embedded numbered SQL files, applied on startup (decision #7).

## Bootstrap Scope

The bootstrap targets the Graph Core workstream only: the foundation everything else builds on.

**In scope:**
- Types package with all shared interfaces and domain types
- SQLite-backed GraphStore implementation with full schema and migrations
- Snapshot manager (Merkle root computation, snapshot chain, diff)
- Language-agnostic extractor framework with common `Extractor` interface
- Go extractor (go/packages type resolution, symbol identity, cross-module edges)
- tree-sitter extractor (multi-language AST parsing, initially one non-Go language as proof of interface)
- MCP server with core tool handlers
- Daemon with file watching and incremental reindex
- CLI entry point with subcommands

**Out of scope (future workstreams):**
- Pebble acceleration index
- Runtime trace ingestion
- Semantic PR diff / CI integration
- Agent coordination (pending mutations)
- Federated graphs
- ComputationCache implementation (interface defined but implementation deferred)
