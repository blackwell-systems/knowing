# Implementation Log

Running log of how knowing was implemented using polywave parallel agent coordination.

## Pre-Implementation State (2026-05-14)

Architecture locked across 15 decisions in `docs/architecture.md`. No code yet. Roadmap defined as five parallel workstreams with explicit dependency constraints in `docs/roadmap.md`.

### Architecture decisions in place before first line of code:

1. Content-addressed graph (Merkle DAG)
2. Symbol identity scheme (`{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`)
3. Append-only edge log (event sourcing)
4. Edge provenance with confidence tiers
5. Content-addressed file identity
6. Causal ordering across repos (Lamport timestamps, git timestamps initially)
7. Schema migration framework (embedded numbered SQL migrations)
8. Deterministic reindexing (same input = same output, always)
9. Storage: SQLite ledger + Pebble acceleration index (ship SQLite, add Pebble when benchmarks justify)
10. Storage interface (`GraphStore` for backend swappability)
11. Process model (persistent daemon with MCP interface)
12. Content-addressed computation cache (derived results as shareable artifacts)
13. Runtime trace ingestion (OTel spans, HTTP logs as graph edges)
14. Semantic PR diff (relationship-level impact per PR)
15. Artifact-boundary plane separation (execution plane produces the graph, intelligence plane interprets it)

### Key interfaces drafted in architecture doc:

- `GraphStore` (all graph operations)
- `ComputationCache` (content-addressed derived results)
- `TraceIngestor` (runtime observability data → graph edges)
- `HybridStore` (SQLite ledger + Pebble acceleration routing)

### Dependency constraints for implementation ordering:

```
Graph Core ──────────────────────────────> all other workstreams
HTTP route edges (route-to-symbol map) ──> Runtime symbol resolution
Runtime symbol resolution ────────────────> Trace ingestion pipeline
Trace ingestion pipeline ─────────────────> Runtime edge creation
Edge provenance model ────────────────────> Confidence decay
Snapshot chain + SnapshotDiff ────────────> Semantic PR diff
Snapshot chain + SnapshotDiff ────────────> Temporal reasoning
Call graph + traversal ───────────────────> Graph-native test selection
Ownership edges ──────────────────────────> Ownership routing
MCP server ───────────────────────────────> Pending mutations
Snapshot chain + Merkle sync ─────────────> Federated graphs
```

---

## Bootstrap (2026-05-15)

Implemented using `/polywave bootstrap "knowing: content-addressed knowledge graph daemon in Go"`.

### Scout

Scout read the architecture doc (1100 lines), requirements, roadmap, and README. Produced a 1100-line IMPL manifest decomposing the project into 6 concerns, 7 agents, and 26 files across 2 waves.

**Decomposition:**

| Wave | Agent | Package | Files | Responsibility |
|------|-------|---------|-------|----------------|
| 0 | scaffold | `internal/types/` | 4 | Hash, Node, Edge, GraphStore, Extractor, ComputationCache interfaces |
| 1 | A | `internal/store/` | 4 | SQLite GraphStore (WAL mode, migrations, recursive CTE traversal) |
| 1 | B | `internal/snapshot/` | 3 | Merkle tree, snapshot chain, diff, GC |
| 1 | C | `internal/indexer/` + `goextractor/` | 5 | ExtractorRegistry, Indexer, Go extractor (go/packages) |
| 1 | D | `internal/indexer/treesitter/` | 2 | tree-sitter Python extractor (proof of Extractor interface) |
| 1 | E | `internal/mcp/` | 3 | MCP server with 11 tool handlers (stdio + HTTP) |
| 1 | F | `internal/daemon/` | 3 | Daemon lifecycle, fsnotify watcher, debounce, RWMutex coordination |
| 2 | G | `cmd/knowing/` | 2 | CLI wiring (serve, index, query, version) |

### Scaffold

4 files committed: `go.mod`, `internal/types/types.go`, `internal/types/interfaces.go`, `internal/types/results.go`. All shared interfaces and domain types established as contracts before any agent started.

Duration: 72 seconds.

### Critic

Triggered (6 agents exceeds 3-agent threshold). All 10 checks passed. Zero errors, zero warnings. Correctly noted external deps not yet in go.mod (expected for bootstrap).

Duration: 66 seconds.

### Wave 1

6 agents launched in parallel, each in its own git worktree.

**Agent results:**

| Agent | Duration | Tests | Key implementation details |
|-------|----------|-------|--------------------------|
| A (SQLite store) | ~5 min | 14 pass | All 20 GraphStore methods, WAL mode, go:embed migrations, recursive CTEs for transitive callers |
| B (Snapshot mgr) | ~5 min | 7 pass | Merkle root from sorted edge hashes, bytes.Compare sorting, snapshot chain with parent pointers |
| C (Indexer + Go) | ~5 min | 12 pass | ExtractorRegistry, local SnapshotComputer interface, go/packages type resolution, implements + references edges |
| D (tree-sitter) | 175s | 9 pass | Python extractor via go-tree-sitter, functions/classes/methods/imports/calls extraction |
| E (MCP server) | ~5 min | 6 pass | 11 tools registered, stdio + HTTP transport, intelligence handlers are read-only |
| F (Daemon) | 183s | 6 pass | DaemonConfig with callbacks, fsnotify with 500ms debounce, sync.RWMutex coordination |

**Incident:** Computer crash mid-wave killed agents A, B, C, E while running. All had committed implementation code to their branches but B, C, E were missing test files and C was missing implements/references edge extraction.

**Recovery:** Reviewed all 4 incomplete branches against the IMPL spec using parallel review agents. Launched a single repair agent that added missing tests to B (7 tests), tests + implements/references edges to C (12 tests), and tests to E (6 tests). Set completion reports manually via `polywave-tools set-completion`.

**Merge:** Agent A fast-forwarded. Agent B merged clean. Agents C, D, E, F had go.mod/go.sum conflicts (each added its own dependencies). Resolved with `git checkout --theirs go.mod go.sum && go mod tidy` for each.

**Post-merge verification:** `go build ./...` PASS, `go test ./...` ALL 7 packages pass, `go vet ./...` PASS.

### Wave 2

Single agent (G) wired all packages together in `cmd/knowing/main.go`.

Duration: 112 seconds. 2 files, 2 tests. Clean merge, no conflicts.

**Subcommands:** `serve` (daemon with MCP server), `index` (one-shot repo indexing), `query` (symbol search), `version`.

**Post-merge verification:** All 8 packages build and test clean. 56+ tests total.

### Final State

IMPL closed and archived to `docs/IMPL/complete/IMPL-bootstrap.yaml`.

```
cmd/
  knowing/
    main.go              <- CLI entry point (serve, index, query, version)
    main_test.go
internal/
  types/
    types.go             <- Hash, Node, Edge, File, Repo, Snapshot, EdgeEvent, provenance
    interfaces.go        <- GraphStore, Extractor, ComputationCache interfaces
    results.go           <- CallerResult, BlastRadiusResult, DiffResult, DerivedResult
  store/
    sqlite.go            <- SQLiteStore implementing GraphStore (20 methods)
    sqlite_test.go       <- 14 tests
    migrate.go           <- Migration runner (go:embed)
    migrations/
      001_initial_schema.sql
  snapshot/
    manager.go           <- SnapshotManager: Merkle root, chain, diff, GC
    merkle.go            <- Merkle tree construction and diff
    manager_test.go      <- 7 tests
  indexer/
    indexer.go           <- Indexer: orchestrates extractors
    extractor.go         <- ExtractorRegistry
    indexer_test.go      <- 4 tests
    goextractor/
      extractor.go       <- Go extractor (go/packages, implements, references)
      extractor_test.go  <- 8 tests
    treesitter/
      extractor.go       <- tree-sitter Python extractor
      extractor_test.go  <- 9 tests
  mcp/
    server.go            <- MCP server (stdio + HTTP)
    handlers.go          <- 11 tool handlers
    handlers_test.go     <- 6 tests
  daemon/
    daemon.go            <- Daemon lifecycle, coordination
    watcher.go           <- fsnotify file watcher with debounce
    daemon_test.go       <- 6 tests
```

8 packages. 26 files. 56+ tests. Single Go binary.
