# System Overview

knowing is a self-adapting code intelligence engine. It builds a content-addressed knowledge graph of cross-repository code relationships, observes the structural properties of that graph (density, connectivity, community structure), and adjusts its retrieval strategy accordingly. On sparse graphs, keyword search finds the right symbols directly. On dense enterprise graphs (>40K nodes), the system automatically shifts to structural navigation: preferring type hierarchies as walk entry points, using phrase-aware matching to cut through keyword competition, and concentrating probability mass on structural anchors rather than diffusing it across thousands of method-level candidates.

## Components

```
knowing daemon (long-lived)
  ├── Change Detector (git-based: post-commit hooks, .git/HEAD watch, polling fallback)
  ├── Indexer (two-tier: tree-sitter extraction + LSP enrichment)
  ├── Graph Store (SQLite behind GraphStore interface, WAL mode, 50K-entry in-process LRU cache)
  ├── MCP Server (stdio or HTTP, 28 tools across execution/intelligence/runtime/context/feedback/discovery/management planes; 8 read-only resources for agent orientation)
  ├── Snapshot Manager (computes hierarchical Merkle trees via merkle-strata library, GCs old snapshots + orphaned nodes/edges, auto-GC at 5000 edge_events)
  ├── Lockfile (internal/daemon/lockfile.go, prevents multiple instances on same database)
  └── Trace Ingestor (OTel spans, HTTP logs → runtime edges)
```

## System Diagram

```
+------------------+     +------------------+     +------------------+
|   Local Repos    |     |  External Deps   |     |   Agent (MCP)    |
|  (Tier 1: deep)  |     | (Tier 2: shallow)|     |                  |
+--------+---------+     +--------+---------+     +--------+---------+
         |                         |                        |
         v                         v                        |
+--------+---------+     +---------+--------+               |
|  AST Parser      |     |  SCIP/LSP Ingest |               |
|  (go/packages,   |     |  (public API     |               |
|   tree-sitter)   |     |   surface only)  |               |
+--------+---------+     +---------+--------+               |
         |                         |                        |
         +------------+------------+                        |
                      v                                     |
         +------------+------------+     +------------------+
         |   Content-Addressed     |     |  Non-Code Ingest |
         |      Graph Store        |<----| (Terraform, K8s, |
         |  (Merkle DAG, SQLite)   |     |  CODEOWNERS,     |
         |                         |     |  OpenAPI specs)  |
         +------------+------------+     +------------------+
                      |
              +-------+-------+
              v               v
+-------------+---+   +------+-----------+
| Snapshot Chain  |   | Runtime Ingest   |
| (root hashes    |   | (OTel traces,    |
|  linked like    |   |  production      |
|  git commits)   |   |  traffic logs)   |
+-----------------+   +------------------+
```

## Language Model

The graph model is language-agnostic. Symbols, edges, hashes, provenance, and snapshots carry no language-specific semantics. A Go function, a Python class, and a TypeScript route handler all produce the same node and edge structures, identified by the same hash scheme, stored in the same graph. The extractor produces them; the graph doesn't care what language they came from.

Adding a new language means writing a tree-sitter extractor that produces nodes and edges. No changes to the graph store, snapshot chain, MCP server, cache, or any other component.

## Two-Tier Extraction

Indexing uses a two-tier architecture that separates fast symbol extraction from expensive type resolution. The graph is queryable after Tier 1 completes (seconds); Tier 2 enriches it with type-resolved confidence (seconds more).

```
Tier 1: tree-sitter (fast, all languages)
  ├── Parse AST via tree-sitter grammar
  ├── Extract declaration nodes (functions, types, methods, interfaces)
  ├── Extract syntactic call edges (string-matched, not type-resolved)
  ├── Extract import edges
  ├── Store call-site positions (line, column, file) on each call edge
  ├── Provenance: "ast_inferred", confidence: 0.7
  └── Completes in ~1.8 seconds for a 7,224-node repo (84K LOC, parallel extraction with 8 workers)

Tier 2: LSP enrichment (type-resolved, per-language)
  ├── Start language server (gopls, pyright, rust-analyzer)
  ├── Open all source files (textDocument/didOpen)
  ├── Upgrade call edges: query GetDefinition at call-site positions
  │   └── Confirmed edges upgraded to "lsp_resolved", confidence: 0.9
  ├── Discover new edges: query GetImplementation, GetReferences on symbols
  │   └── implements and references edges (tree-sitter cannot produce these)
  ├── Close all files, shutdown language server
  └── Time varies by language: Python ~10 min, Rust ~1 min, Go 12-58 min (gopls warmup), TypeScript ~34 min (GC-bound)
```

**Why two tiers instead of one:**

Full type resolution via `go/packages` (or equivalent per-language) requires loading and type-checking the entire transitive dependency graph. For a Go repo with heavy dependencies, this takes 16+ minutes. The cost is proportional to the dependency graph size, not the repo size, and cannot be parallelized from the caller's side.

tree-sitter parses syntax without type checking. It produces the same declaration nodes and most of the same call edges in seconds. The edges have lower confidence (syntactic string matching vs. type-resolved targeting) but are correct for the vast majority of direct calls.

LSP enrichment bridges the gap. Language servers (gopls, pyright, etc.) perform type checking incrementally on opened files rather than in a single batch pass. gopls resolves 8,600+ edges in ~8 seconds because it processes files incrementally as they're opened, leveraging its own internal caching. Note: Tier 1 tree-sitter extraction now uses parallel extraction (8-worker goroutine pool) and completes in 1.8 seconds for the knowing codebase (84K LOC, 429 files, 7,224 nodes, ~24.9K edges).

**Data flow:**

```
Repository on disk
    │
    ▼
Tier 1: tree-sitter extraction
    │  ├── File walker (skips .git, .claude, vendor, node_modules, testdata)
    │  ├── Content hash comparison (skip unchanged files)
    │  ├── Worker pool (runtime.GOMAXPROCS goroutines, fan-out/fan-in)
    │  │   └── Each worker: parse file → extract nodes + edges → return results
    │  ├── Deleted file detection (compare walked files against stored files)
    │  │   └── Files no longer on disk: cleanup via DeleteEdgesBySourceFile + DeleteNodesByFile
    │  ├── Batch insert (nodes, edges, files in single transaction)
    │  └── Snapshot computation (hierarchical Merkle tree: repo root -> package roots -> edge-type roots -> edge leaves)
    │
    ▼
Graph is queryable (ast_inferred edges, confidence 0.7)
    │
    ▼
Tier 2: LSP enrichment
    │  ├── Start language server (gopls for Go, pyright for Python, etc.)
    │  ├── Two-phase warmup: open one file (didOpen), retry GetDefinition until server responds
    │  ├── Edge upgrade pass:
    │  │   ├── For each ast_inferred edge with call-site position:
    │  │   │   ├── Query GetDefinition at (CallSiteFile, CallSiteLine, CallSiteCol)
    │  │   │   ├── If definition resolved: upgrade to lsp_resolved (0.9)
    │  │   │   └── If not resolved: leave as ast_inferred (0.7)
    │  │   └── Preserves call-site positions on upgraded edges
    │  ├── Edge discovery pass:
    │  │   ├── For each file: GetDocumentSymbols
    │  │   ├── For types/interfaces: GetImplementation → implements edges
    │  │   └── For functions/methods: GetReferences → references edges
    │  ├── Close all files, shutdown language server
    │  └── New edges stored as lsp_resolved (0.9)
    │
    ▼
Graph is enriched (upgraded edges are lsp_resolved; many remain ast_inferred)
```

**Parallel Indexer (Tier 1):**

The indexer uses a producer-consumer pipeline (configurable via `--workers` flag, default GOMAXPROCS) with phase separation for deterministic, high-throughput extraction:

1. **Walk phase:** enumerate source files, skip unchanged (content hash comparison)
2. **Parallel extract phase:** producer fans out files to consumer workers; each worker creates a per-call tree-sitter parser (thread-safe, no shared state). Results written to a pre-sized array indexed by submission order (no locks, deterministic output). A per-file CGO watchdog (10s timeout) prevents stuck tree-sitter calls from blocking the pipeline; if the timer fires, the worker sends an empty result and moves on (fire-and-forget).
3. **Batch store phase:** single transaction for all nodes, edges, and file records (multi-row INSERT, 100-500 rows per statement)
4. **Authorship phase:** `git blame` stamps (skippable via `--skip-blame` for faster structural-only index)
5. **Snapshot phase:** hierarchical Merkle tree computation (in-memory from pipeline data, no DB re-read)

Progress output to stderr every 2 seconds shows files processed and extraction rate. On the knowing codebase (84K LOC, 429 source files, 62 packages), the parallel indexer produces 7,224 nodes and ~24.9K edges in 1.8 seconds (1,451 files/sec throughput).

The worker pool handles tree-sitter extraction only; LSP enrichment uses its own concurrency model (128 concurrent workers post-warmup for edge upgrades, 8 concurrent for discovery batches). Language servers handle concurrent requests well once warmed up.

**Call-site positions:**

Edges carry `CallSiteLine` (1-indexed), `CallSiteCol` (0-indexed), and `CallSiteFile` (relative path) fields that store the source location of the call expression, not the declaration. tree-sitter provides these naturally from AST node positions. The Python tree-sitter extractor (`internal/indexer/treesitter/extractor.go`) additionally threads the enclosing function's node hash through `walkNode`, so each call edge records both its position and its containing scope. The enricher uses call-site positions to query `GetDefinition` at the exact call site, confirming that the syntactic call target matches the type-resolved target. Without call-site positions, LSP enrichment cannot upgrade existing edges (it can only discover new ones).

**textDocument/didOpen requirement:**

LSP servers require files to be opened via `textDocument/didOpen` before they can resolve cross-package references. The enricher uses a two-phase approach: during warmup, it opens a single probe file to trigger package loading (gopls uses lazy loading and needs this stimulus). During the discovery phase, files are opened in batches of 50 to limit memory pressure. Without any `didOpen`, `GetDefinition`, `GetImplementation`, and `GetReferences` return empty results because the server has not loaded any packages.

**What tree-sitter cannot do (explicit limitations):**

| Capability | Why tree-sitter can't | How LSP enrichment covers it |
|-----------|----------------------|---------------------------|
| Resolve interface satisfaction | Requires type checker to compare method sets | GetImplementation queries |
| Resolve non-call references | Requires TypesInfo.Uses from type checker | GetReferences queries |
| Disambiguate overloaded names | Requires type resolution for receiver types | GetDefinition at call site |
| Resolve aliased imports | Matches string alias to import path, may guess wrong | GetDefinition confirms the actual target |
| Detect embedded type methods | Requires understanding type embedding | GetImplementation covers promoted methods |

These limitations exist only between Tier 1 and Tier 2 completion. After enrichment, all limitations are resolved.

**Extractors (23 registered via extractor registry + CODEOWNERS inline):**

| Language / Format | Tier 1 (fast) | Tier 2 (enrichment) | LSP server |
|----------|--------------|--------------------|-----------| 
| Go | `gotsextractor` (tree-sitter Go grammar) | `enrichment` (agent-lsp pkg/lsp) | gopls |
| Python | `treesitter` (tree-sitter Python grammar) | enrichment | pyright |
| TypeScript/JS | `tsextractor` (tree-sitter TS grammar) | enrichment | tsserver |
| Rust | `rustextractor` (tree-sitter Rust grammar) | enrichment | rust-analyzer |
| Java | `javaextractor` (tree-sitter Java grammar) | enrichment | jdtls |
| C# | `csharpextractor` (tree-sitter C# grammar) | enrichment | OmniSharp |
| Ruby | `rubyextractor` (tree-sitter Ruby grammar) | n/a | n/a |
| CSS/SCSS | `cssextractor` (tree-sitter CSS grammar) | n/a | n/a |
| Protocol Buffers | `protoextractor` (tree-sitter protobuf grammar) | n/a | n/a |
| GraphQL | `graphqlextractor` (tree-sitter GraphQL grammar) | n/a | n/a |
| OpenAPI/Swagger/JSON Schema | `schemaextractor` (yaml.v3 + JSON parser) | n/a | n/a |
| Go (legacy) | `goextractor` (go/packages, `--full` flag) | n/a (already type-resolved) | n/a |
| Terraform (HCL) | `terraformextractor` (HCL parser) | n/a | n/a |
| SQL | `sqlextractor` (SQL parser) | n/a | n/a |
| Kubernetes YAML | `k8sextractor` (yaml.v3) | n/a | n/a |
| Cloud YAML | `cloudextractor` (yaml.v3, 4 sub-extractors: CFN/SAM, Compose, Actions, Serverless) | n/a | n/a |
| Dockerfile | `dockerfileextractor` (line parser) | n/a | n/a |
| Makefile | `makefileextractor` (line parser) | n/a | n/a |
| Helm Charts | `helmextractor` (yaml.v3) | n/a | n/a |
| GitLab CI | `gitlabciextractor` (yaml.v3) | n/a | n/a |
| package.json/npm | `packagejsonextractor` (JSON parser) | n/a | n/a |
| Event/Message Queue | `eventextractor` (cross-language producer/consumer detection) | n/a | n/a |
| .env files | `envextractor` (line parser) | n/a | n/a |
| CODEOWNERS | `ownership` (inline in indexer, not via registry; emits `owned_by` edges with confidence 1.0) | n/a | n/a |

**Cross-file import resolution (5 OOP languages):**

Python, TypeScript, Rust, Java, and C# extractors build per-file import maps during Tier 1 extraction. When a call target matches an imported name, the edge is resolved to its qualified source with provenance `ast_resolved` and confidence 0.85 (up from `ast_inferred` / 0.7). This creates cross-file call edges without requiring LSP enrichment, improving graph connectivity for RWR walks.

| Language | Import map builder | Resolution strategy |
|----------|-------------------|-------------------|
| Python | `buildPythonImportMap` | `import`/`from...import` statements |
| TypeScript | `buildTSImportMap` | `import`/`require` declarations |
| Rust | `buildRustImportMap` | `use` declarations, `crate::`/`super::`/`self::` paths |
| Java | `buildJavaImportMap` | `import com.pkg.Class`, `import static` |
| C# | `buildCSharpImportMap` | `using Namespace`, `using static` |

**Cross-repo awareness (all 6 language extractors with LSP enrichment):**

Each extractor detects when an import target is external (third-party or stdlib) and computes a target hash using `"external://{packageName}"`, `"stdlib"`, or the inferred repo URL as the prefix instead of the local repo URL. This gives cross-repo identity to import edges without requiring full registry lookups.

| Language | Stdlib detection | External detection |
|----------|-----------------|-------------------|
| Go | No dots in first path segment (e.g., `fmt`, `net`) | First 3 path segments as repo URL (e.g., `github.com/org/repo`) |
| Python | ~50 known stdlib modules | `site-packages/` in path |
| TypeScript | n/a | Bare specifiers (non-relative imports) |
| Rust | `std::`/`core::`/`alloc::` | Other non-crate paths |
| Java | `java.*`/`javax.*` | Third-party by package prefix |
| C# | `System.*`/`Microsoft.*` | Third-party by namespace |

The Go tree-sitter extractor (`gotsextractor`) is the default. The go/packages extractor (`goextractor`) is available via `knowing index --full` as a deliberate escape hatch for cases requiring guaranteed single-pass type resolution at the cost of 16+ minutes. This is a design choice: two-tier is the architecture, `--full` exists for validation and edge cases where LSP enrichment is unavailable (air-gapped environments, missing gopls).

**LSP client:**

LSP enrichment uses `github.com/blackwell-systems/agent-lsp/pkg/lsp`, a pure Go LSP client library with no CGo dependencies. It manages language server subprocess lifecycles (spawn, initialize, request/response, shutdown) and supports multi-server routing for polyglot repos. The enricher opens all source files before querying to give the language server full workspace context, then queries GetDefinition (edge upgrade), GetImplementation (implements edges), and GetReferences (references edges).

**Multi-language auto-detection:**

The enricher auto-detects available language servers via `DetectLSPServers` (`internal/enrichment/config.go`). Detection checks for project markers (`go.mod`, `tsconfig.json`, `pyproject.toml`, `Cargo.toml`, `pom.xml`, `*.csproj`) and verifies that the corresponding binary exists in PATH. Each detected server is described by an `LSPServerConfig` struct containing `command`, `extensions`, and `language_id`. The enricher iterates all detected servers sequentially, opening only files matching each server's extensions via the language-agnostic `openFilesForLanguage` helper. Test file detection (`isTestFile`) handles multi-language conventions (`_test.go`, `.test.ts`, `test_*.py`, etc.).

For explicit control, `SetLSPConfig` overrides auto-detection and `LoadLSPConfig` reads from a `knowing-lsp.json` file. Supported servers: gopls, typescript-language-server, pylsp/pyright, rust-analyzer, jdtls, OmniSharp.

**Provenance tiers after two-tier extraction:**

| Provenance | Confidence | Source | When |
|-----------|-----------|--------|------|
| `ast_inferred` | 0.7 | tree-sitter syntactic matching | After Tier 1 (seconds) |
| `lsp_resolved` | 0.9 | LSP GetDefinition confirmation | After Tier 2 (seconds more) |
| `ast_resolved` | 0.85 | Import-map resolution (Python, TS, Rust, Java, C#) or go/packages | After Tier 1 (import-resolved) or `--full` flag |

## HTTP Route Extraction

During Tier 1 tree-sitter extraction, the Go extractor (`gotsextractor`) detects HTTP route handler registrations and creates graph nodes and edges that bridge static analysis and runtime trace ingestion.

**Detection:** The extractor walks function and method bodies for call expressions matching known HTTP router registration patterns. It recognizes five router packages:

| Package | Methods detected |
|---------|-----------------|
| `net/http` | `HandleFunc`, `Handle` |
| `github.com/go-chi/chi` (v1 and v5) | `Get`, `Post`, `Put`, `Delete`, `Patch` |
| `github.com/gin-gonic/gin` | `GET`, `POST`, `PUT`, `DELETE`, `PATCH` |
| `github.com/labstack/echo` (v1 and v4) | `GET`, `POST`, `PUT`, `DELETE`, `PATCH` |
| `github.com/gorilla/mux` | `HandleFunc`, `Handle` |

Detection uses a fast pre-filter (method name must be in the union of all known route methods) followed by import path verification. For local variables (e.g., `r := chi.NewRouter()`), the extractor infers the router package from the file's import set.

**Multi-language framework coverage:** Route extraction extends beyond Go to all supported languages. The full set of detected frameworks (18 total across 6 languages):

| Language | Frameworks | Detection strategy |
|----------|-----------|-------------------|
| Go | net/http, chi, gin, echo, gorilla/mux | Method call on router variable + import path verification |
| TypeScript | Express.js, Fastify, Hono (shared `app.method` pattern), NestJS (`@Controller` + `@Get`/`@Post` decorators), Next.js App Router (exported `GET`/`POST`/`PUT`/`DELETE` in `route.ts` files) | Call expression matching or decorator/export detection |
| Python | Flask, FastAPI (`@app.get`/`@router.post` decorator parsing), Django (`path()`/`re_path()` in `urls.py`) | Decorator call matching or url pattern function calls |
| Rust | Actix-web, Axum, Rocket | Attribute macros and router builder methods |
| Java | Spring MVC, JAX-RS | `@RequestMapping`/`@GetMapping` and `@Path`/`@GET` annotations |
| C# | ASP.NET Core (minimal APIs and controller routing) | `app.Map*` calls and `[HttpGet]`/`[Route]` attributes |

**Graph output:** Each detected route registration produces:

1. A `route_handler` node whose `QualifiedName` encodes the repo, package, HTTP method, and route pattern (e.g., `github.com/org/repo://api.GET /users/:id`). The `Signature` field stores the route pattern.
2. A `handles_route` edge from the route handler node to the handler function node, with provenance `ast_inferred` and confidence `0.7`.

**Route symbols table:** The route handler nodes are the static-analysis side of a bridge to runtime traces. After indexing, the `route_symbols` table maps `(service_name, route_pattern, mapping_type)` to the route handler node's hash. The runtime trace `SymbolResolver` looks up this table to connect observed HTTP traffic to the correct graph node. Without route extraction during indexing, the resolver falls back to synthetic unresolved nodes with confidence `0.3`.

## Indexing Tiers (Repository Scope)

- **Local repositories (deep)**: Full two-tier extraction. tree-sitter for declarations and calls, LSP enrichment for type resolution and edge discovery. Every symbol, call, import, implements, and reference relationship is extracted.
- **External dependencies (shallow)**: Public API surface only, ingested via SCIP indices or LSP queries. Enough to connect cross-repo edges without parsing all transitive source.

## Change Detection and Incremental Indexing

Changes to the graph are driven by git commits, not filesystem events. A commit is the atomic unit of source code change: it has a hash, a parent, a diff, and it's permanent. Everything else (editor autosaves, build artifacts, IDE metadata) is noise that the change pipeline must not react to.

**Core principle:** The snapshot chain mirrors the git commit chain. Every snapshot's `CommitHash` field points to the git commit that produced it. The graph at any commit is reconstructable by looking up its snapshot.

**Change detection (prioritized):**

```
1. Post-commit hook (primary)
   │  Daemon installs a git hook that sends (repoPath, oldHead, newHead)
   │  via unix socket. Instant, precise, zero polling overhead.
   │
2. .git/HEAD watch (fallback)
   │  fsnotify on .git/HEAD + .git/refs/heads/* (one file descriptor,
   │  not thousands). On change: read new HEAD, compare to last known.
   │  For environments where hooks can't be installed.
   │
3. Polling (last resort)
      Every N seconds: git rev-parse HEAD, compare to stored value.
      For NFS, SMB, or other environments where neither hooks nor
      fsnotify work reliably.
```

**Change resolution:**

When a new commit is detected, the daemon resolves the exact change set from git:

```go
oldHead := repo.LastCommit          // stored in repos table
newHead := gitRevParseHead(repoPath)
changed := gitDiffFiles(repoPath, oldHead, newHead)     // modified files
deleted := gitDiffFilesDeleted(repoPath, oldHead, newHead) // removed files
added   := gitDiffFilesAdded(repoPath, oldHead, newHead)   // new files
```

No directory walking. No content hashing. No false positives. The change set comes directly from git's own diff, which is authoritative.

**Incremental index pipeline:**

```
Commit detected (oldHead → newHead)
    │
    ▼
1. Resolve changed/deleted/added files via git diff
    │
    ▼
2. For deleted files:
   ├── Delete all nodes where file_hash matches
   ├── Delete all edges where source node was in deleted file
   └── Record "removed" edge events in append-only log
    │
    ▼
3. For changed files:
   ├── Delete old nodes/edges (same as deleted files)
   ├── Re-extract via tree-sitter (Tier 1)
   ├── Compute edge diff (new edges vs. old edges for this file)
   └── Record "added" and "removed" edge events
    │
    ▼
4. For added files:
   ├── Extract via tree-sitter (Tier 1)
   └── Record "added" edge events
    │
    ▼
5. Compute new snapshot
   ├── Hierarchical Merkle tree (repo root -> package roots -> edge-type roots -> edge leaves)
   ├── Link to parent snapshot (previous snapshot for this repo)
   └── Store commit hash in snapshot record
    │
    ▼
6. Scoped LSP enrichment (Tier 2)
   ├── Only enrich edges from changed/added files
   ├── Skip unchanged files entirely
   └── Language servers may have workspace context from previous runs
    │
    ▼
7. Cross-repo edge resolution
   └── Resolve any new dangling edges created by the changes
```

**Why git-based, not filesystem-based:**

| Concern | Filesystem watching | Git-based detection |
|---------|-------------------|-------------------|
| False positives | Editor autosaves, build artifacts, IDE metadata, temp files | Zero. Only committed changes. |
| File descriptor pressure | One FD per watched file (hits ulimit on repos with 10K+ files) | One FD for .git/HEAD, or zero with hooks/polling |
| Branch switch floods | Hundreds of events, debouncing required, still re-walks everything | One event: oldHead != newHead. git diff gives exact file set. |
| Deleted file detection | Unreliable (depends on OS event ordering) | `git diff --diff-filter=D` gives exact list |
| Change granularity | "This file's mtime changed" (no context) | "These files changed between commit A and commit B" |
| Snapshot-commit alignment | Snapshots taken at arbitrary times based on when events fire | Every snapshot corresponds to exactly one commit |
| History reconstruction | "Something changed around timestamp T" | "Commit abc123 produced snapshot xyz789 with these edge changes" |
| Determinism | Different event ordering on different OSes | Same git diff on any machine produces the same change set |

**Uncommitted changes:**

The graph indexes committed state only. Uncommitted changes are transient (may be undone, stashed, or abandoned), violate determinism (same repo at same "state" produces different graphs depending on working tree), and create noise in the snapshot chain. For users who need to index working tree state, `knowing index --working-tree` creates a temporary snapshot not linked to the main chain.

**Multi-repo change coordination:**

Each repo has its own change detector. A commit in repo A triggers indexing of repo A only. After the new snapshot is computed, the cross-repo resolver runs to reconnect any edges that reference symbols in other repos. Repo B's subgraph is untouched unless repo B also commits.

**Edge events (append-only log):**

Every incremental index records edge events: which edges were added and which were removed, keyed by the snapshot hash. This is the data that makes `SnapshotDiff` work: comparing two snapshots is a range scan on edge_events filtered by snapshot hash. Without edge events, the Merkle DAG has roots but no record of what changed between them.

```
edge_events table:
  event_id      INTEGER PRIMARY KEY
  edge_hash     BLOB NOT NULL        -- which edge
  event_type    TEXT NOT NULL         -- "added" or "removed"
  snapshot_hash BLOB NOT NULL        -- which snapshot recorded this event
  source_commit TEXT NOT NULL         -- git commit that caused this change
  indexer_ver   TEXT NOT NULL         -- indexer version
  timestamp     INTEGER NOT NULL     -- unix timestamp
```

**GraphStore methods for incremental cleanup:**

```go
// Delete all nodes derived from a specific file.
DeleteNodesByFile(ctx context.Context, fileHash Hash) error

// Delete all edges whose source node belongs to a specific file.
DeleteEdgesBySourceFile(ctx context.Context, fileHash Hash) error

// Get all edges whose source node belongs to a specific file.
// Used to compute the "removed" set before deletion.
EdgesBySourceFile(ctx context.Context, fileHash Hash) ([]Edge, error)
```

## Integrity and Garbage Collection

**In-process LRU cache:** `SQLiteStore.GetNode` and `SQLiteStore.GetEdge` maintain a `sync.Map`-based LRU cache capped at 50K entries. Hot-path traversals (blast radius, RWR walks) eliminate redundant SQL round-trips. The cache is invalidated at the start of each index run.

**Daemon lockfile:** `internal/daemon/lockfile.go` creates `<db_path>.lock` with the daemon PID on startup. A second daemon instance on the same database fails with a clear error instead of competing for the SQLite WAL.

**GC reachability sweep:** `GarbageCollectFull` (in `internal/snapshot/gc.go` and `internal/store/gc.go`) runs after deleting old snapshots. It collects all node and edge hashes referenced by surviving snapshots, then calls `DeleteNodesNotIn` and `DeleteEdgesNotIn` on the store to prune orphaned rows. Returns a `GCStats` struct with counts of pruned nodes and edges. This prevents the `nodes` table from growing without bound on frequently-refactored repos.

**Auto-GC threshold:** After each index run, if the `edge_events` table exceeds 5,000 rows, the indexer automatically triggers GC (keeping the 10 most recent snapshots). This is inspired by git's `gc.auto` threshold (6,700 loose objects). The auto-GC prevents unbounded edge_events growth without manual intervention.

**Generation numbers on snapshots:** Each snapshot carries a `Generation` integer (migration 015). On creation, `Generation = parent.Generation + 1` (or 0 for the first snapshot). This enables O(1) ancestry checks ("is snapshot A an ancestor of B?" reduces to `A.Generation < B.Generation` when A is on B's chain) and prunes unnecessary chain walks during diff and GC operations.

**`knowing fsck`:** CLI integrity checker (`cmd/knowing/fsck.go` + `internal/snapshot/verify.go`). Verifies edge referential integrity, hash recomputation (detects mutated rows), snapshot chain continuity, and SQLite-level page integrity via `PRAGMA integrity_check`. Issues classified as ERROR or WARN.

**`indexed_at` epoch (migration 011):** `indexed_at INTEGER` column on the `nodes` and `edges` tables. `GarbageCollectFull` uses this to identify objects from superseded index runs.

## Edge Types

The graph connects symbols with typed, provenance-annotated edges:

| Category | Edge types |
|----------|-----------|
| Code | `calls`, `imports`, `implements`, `references`, `extends`, `overrides`, `decorates`, `throws` |
| Structural | `contains` (type -> method, weight 0.0 in RWR walk, used by path seeding directly), `member_of` (method -> type, weight 0.0 in RWR walk) |
| Route | `handles_route` (route handler node to handler function, from static extraction) |
| Infrastructure | `depends_on` (Terraform, SQL, CSS), `deploys` (K8s Service to Deployment), `exposes` (K8s Ingress to Service), `configures` (K8s ConfigMap/Secret to Deployment) |
| Messaging | `publishes`, `subscribes`, `connects_to` |
| Test coverage | `tests` (test function to production function), `tested_by` (package tested by CI workflow) |
| Ownership | `owned_by` (CODEOWNERS extractor, confidence 1.0), `authored_by` (git blame) |
| Documentation | `documents` (doc comment to symbol) |
| API contracts | `consumes_endpoint` (HTTP client call), `implements_rpc` (gRPC service impl), `consumes_rpc` (gRPC client) |
| Feature flags | `gated_by_flag` (function gated by feature flag check) |
| Deployment | `deployed_by` (service deployed by CI workflow) |
| Runtime | `runtime_calls`, `runtime_rpc`, `runtime_produces`, `runtime_consumes` |

38 edge types total across static, structural, infrastructure, runtime, ownership, supply chain, and operational categories. The `contains` and `member_of` edges connect types to their methods and fields bidirectionally. The `co_tested_with` edge connects symbols referenced from the same test file (lateral discovery). The `type_hint_of` edge connects functions to parameter types. The `accesses_field` edge connects methods to the struct fields they read/write. Supply chain edges (`reads_env`, `executes_process`) detect credential access and process spawning patterns. See [Edge Types Reference](edge-types.md) for full details.

## Wire Formats

Four codecs registered in `internal/wire/registry.go`, plus two legacy formats:

| Format | Purpose | Savings vs JSON |
|--------|---------|-----------------|
| **GCF** (Graph Compact Format) | LLM consumption: line-oriented, positional fields, local IDs (`@0`, `@1`), edges as `@target<@source type` | 84% fewer tokens |
| **GCB** (Graph Compact Binary) | Service transport and caching: varint-encoded, length-prefixed | 74% fewer bytes |
| **JSON** | Human debugging, generic consumers | Baseline |
| **XML** (legacy) | Human-readable MCP default (does not include edges) | ~0% |
| **Markdown** (legacy) | Human consumption, documentation | ~10% |

GCF is the recommended format for all agent workflows. LLM comprehension eval (session 27, `eval/TestLLMFormatComprehension`) proved GCF achieves 100% accuracy on structured extraction tasks, outperforming JSON (66.7%) at 16% of the token cost. Session-stateful deduplication (`EncodeWithSession`) reduces repeated symbols by ~47% across consecutive calls. Delta encoding (`EncodeDelta`) saves 81% on re-queries where the pack changed slightly. See [Wire Formats](wire-formats.md) and [Wire Formats Guide](wire-formats-guide.md) for encoding details, benchmarks, and LLM comprehension results.

## Retrieval Pipeline

The context engine (`internal/context/`) transforms a task description into a token-budgeted, relevance-ranked block of symbols. This is the primary intelligence output of the system.

**Pipeline stages:**

```
Task Description
    |
    v
[1. Keyword Extraction]        compound-first: KeywordSet with Exact/Compounds/Components tiers
    |
    v
[2. Seed Retrieval]            5-channel RRF fusion (tiered keyword, BM25, vector [opt-in], equivalence classes, path-context)
    |
    v
[3. Interface-Aware Seeding]   add implementors of matched interface types
    |
    v
[4. Noise Filtering]           exclude mocks, stubs, fakes, phantom external nodes, dist/, vendor/
    |
    v
[5. Random Walk with Restart]  propagate relevance through graph (alpha=0.2, 20 iterations)
    |
    v
[6. HITS Reranking]            authority/hub scores on top-200 RWR nodes
    |
    v
[7. Scoring]                   7-component formula (blast radius, confidence, recency, distance, feedback, session, commit recency)
    |
    v
[7b. Embedding Re-rank]        (disabled, confirmed neutral on cold-start benchmarks)
    |
    v
[8. Budget Packing]            density-ranked greedy knapsack (score/cost ratio)
    |
    v
[9. Vocab Expansion]           record keyword->symbol associations from agent usage (learned equiv classes)
```

**Key design choices:**

- **Compound-first keyword extraction:** The `KeywordSet` struct separates Exact (backtick-quoted identifiers), Compounds (snake_case, CamelCase, dotted names), and Components (split words). Compounds are queried before components; components only used as fallback when compounds yield fewer than 5 results.
- **BM25 via FTS5 (6 weighted columns):** `symbol_name` (10x), `concepts` (5x), `file_path` (4x), `doc` (3x), `qualified_name` (3x), `signature` (1x). The `doc` column indexes docstrings extracted across 6 languages (Go, Python, TypeScript, Rust, Java, C#) via the shared `docextract` package, bridging the vocabulary gap between natural-language task descriptions and code documentation.
- **Path-context seeding (Channel 5):** Extracts package/directory terms from task descriptions, finds TYPE nodes in matching packages (prioritizing types with `contains` edges), and injects them as supplemental RWR seeds. Bridges concept-to-implementation gap (e.g., "migration" finds types in migrations/ package, then RWR walks to their methods).
- **Phantom external node filtering:** External nodes (kind="external", from unresolved LSP targets) are filtered at seed retrieval and RWR result collection. Without this, repos with many unresolved imports have phantom nodes dominating all top positions.
- **Vocabulary expansion from usage:** When an agent uses a symbol after a `context_for_task` query, the keyword -> symbol association is recorded in `vocab_associations` (migration 021). After 2+ observations, the association becomes a learned equivalence class with soft RRF injection (confidence-weighted 0.3-0.8, unlike framework classes which use forced injection). Per-cluster scoping (migration 020) prevents cross-task interference. This is the primary learning mechanism; task memory (session 24) was confirmed neutral and disabled.
- **Merkleized feedback expiration:** Feedback records store the package Merkle root. When code changes, the root changes, and stale feedback becomes invisible automatically.

**Benchmark:** Cross-system benchmark (17 repos, 302 tasks, 8 languages): P@10=0.330 cold start. 13 self-adapting mechanisms. LSP edge attenuation (0.3x for lsp_resolved). Per-cluster implicit feedback with vocabulary expansion. FTS fallback decomposition. Multi-phrase equiv gate. Query latency: 2ms on k8s with adjacency cache. See [Retrieval Pipeline](retrieval-pipeline.md) for the full architecture reference.
