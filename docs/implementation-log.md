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

### Friction and Lessons

**1. Leftover MCP server hook blocked scout agent (~20 min wasted)**

A PreToolUse hook (`cbm-code-discovery-gate`) left over from an MCP server demo blocked subagents' first file access per process. The scout read all files, designed the full manifest, then couldn't write the IMPL doc. Not a polywave issue; an environment artifact. Deleted the hook and relaunched.

**Lesson:** Audit PreToolUse hooks before running polywave. Any hook that gates Read/Write/Glob/Grep on first invocation per process will hit every subagent independently.

**2. `prepare-wave` / `validate` schema disagreement (recurring, ~4 min wasted total)**

`prepare-wave` writes operational state keys into the IMPL YAML (e.g., `original_branch`, state transitions) that `validate`'s schema rejects as `V013_UNKNOWN_KEY`. The `validate_agent_launch` hook (H5) calls `validate` before allowing agent launches, blocking every launch attempt.

**Root cause:** Schema disagreement between two `polywave-tools` subcommands. `prepare-wave` writes keys that `validate` doesn't recognize.

**Workaround:** Run `polywave-tools validate --fix` between `prepare-wave` and agent launch. Required for every wave (Wave 1 and Wave 2 both hit this).

**Fix options:** (a) Validator allowlists keys that `prepare-wave` writes, (b) `prepare-wave` writes operational state to `.polywave-state/` instead of the IMPL doc, (c) H5 hook skips V013 errors for known operational keys.

**3. Stale `GOWORK` env var (recurring environment issue)**

A `GOWORK` environment variable pointing to a non-existent `go.work` file from another project caused `go build`, `go test`, and baseline gates to fail. Required `GOWORK=off` on every go command and in agent launch prompts.

**Lesson:** Polywave's baseline gates surface environment issues clearly (the error message was unambiguous), but agents need to be told about workarounds explicitly in their launch prompts.

**4. Computer crash mid-Wave 1**

Machine went down while 4 of 6 agents were running. All 6 had committed implementation code to branches, but 3 agents (B, C, E) hadn't written test files and agent C hadn't added implements/references edges.

**Recovery process:**
1. `polywave-tools agent-status` identified which agents had completion reports
2. `polywave-tools reconcile-state` advanced IMPL state based on branch evidence
3. Parallel review agents verified each branch against the IMPL spec
4. Single repair agent added missing tests and edges across 3 branches
5. Manual completion reports via `polywave-tools set-completion`

**Lesson:** Branch-per-agent model is crash-resilient. No agent work was lost. Recovery was mechanical (~20 min) rather than requiring re-runs. Agents should commit incrementally (after each file, not just at the end) to minimize loss window.

**5. go.mod/go.sum merge conflicts (expected, flagged in pre-mortem)**

4 of 6 Wave 1 merges had go.mod/go.sum conflicts because each agent added its own dependencies. The scout's pre-mortem predicted this as "high likelihood, low impact."

**Resolution:** `git checkout --theirs go.mod go.sum && go mod tidy` for each conflict. `finalize-wave` couldn't handle this automatically (blocked on conflict prediction). Had to merge manually then run `finalize-wave --skip-merge`.

**Lesson:** For Go projects with multiple agents adding dependencies, go.mod conflicts are inevitable. A `--auto-resolve-gomod` flag on `finalize-wave` would eliminate this manual step.

**6. Agent C missed required edge types**

The IMPL brief explicitly listed `implements` and `references` edges as required. Agent C implemented calls and imports but skipped the other two. Caught by post-crash code review, not by any automated check.

**Lesson:** Postcondition checks in the IMPL brief (e.g., `grep -q "implements" extractor.go`) would have caught this during agent verification. The current postconditions checked method count and struct names but not behavioral completeness.

**7. Scout used invalid file_ownership format for scaffolds**

Scout wrote `agent: scaffold` / `wave: 0` in file_ownership entries, which the validator rejects (expects uppercase agent IDs, wave > 0). Scaffold files should only appear in the scaffolds section, not in file_ownership.

**Lesson:** Minor YAML fix, but shows the scout's bootstrap template handling could be tighter. Validator caught it immediately.

---

## E2E Index (2026-05-15)

Implemented using `/polywave scout "end-to-end: make knowing index work on a real Go repository"`.

### Scout

Scout read all 11 source files and the architecture doc. Found a critical bug: `ComputeNodeHash` included `contentHash` in the hash, but call edge targets used `EmptyHash` for that field, so cross-package caller queries returned nothing. This would never have been caught without trying to index a real repo.

Produced IMPL with 5 agents across 2 waves.

### Wave 1 (4 parallel agents)

| Agent | Task | Duration | Key changes |
|-------|------|----------|-------------|
| A | Fix hash + batch inserts | 124s | Removed contentHash from ComputeNodeHash, added BatchPutNodes/Edges/Files to SQLiteStore |
| B | Fix extractor + indexer | 201s | Changed all ComputeNodeHash calls to use EmptyHash, added batch insert to indexer, fixed FileHash/ContentHash computation |
| C | MCP + snapshot fixes | 184s | Made index_repo handler functional (not a stub), fixed ownership handler, added real-data snapshot test |
| D | Git integration + CLI | 242s | Added GitHeadCommit (reads .git/HEAD directly, no git dependency), wired MCP indexFunc, made tree-sitter optional |

**Merge:** All 4 branches merged clean. No go.mod conflicts this time (no new dependencies added). Zero friction.

**Post-merge:** All tests pass across all 8 packages.

### Wave 2 (integration test)

Single agent (E) created `e2e_test.go` at repo root. The test:
1. Creates a temp multi-package Go module (main.go, pkg/lib.go, pkg/types.go, pkg/impl.go)
2. Indexes it end-to-end (store, indexer, snapshot)
3. Verifies nodes and edges exist
4. Queries cross-package callers (main calls pkg.Hello)
5. Queries blast radius
6. Verifies implements edges (EnglishGreeter implements Greeter)
7. Re-indexes and confirms identical snapshot hash (deterministic)

All 9 assertions pass. Duration: 126s.

### Dogfood: knowing indexed itself

```
$ knowing index /Users/dayna.blackwell/code/knowing
Nodes: 231, Edges: 672
```

The system works. 231 symbols and 672 relationships extracted from its own codebase.

### Friction

**None.** This was the cleanest polywave run of the session:
- Scout: 674s, found the real bug, designed correct fix
- Critic: passed clean (1 advisory warning, non-blocking)
- Wave prep: no validate --fix needed (user fixed the polywave-tools schema disagreement between runs)
- Wave 1 merge: zero conflicts (no new dependencies)
- Wave 2: clean
- Total wall time: ~20 minutes from scout to self-indexing

---

## Cross-Repo Resolver (2026-05-15)

Implemented using `/polywave scout "Cross-repo edge resolver"`.

### Problem

After indexing polywave-go and polywave-web, cross-repo edges showed 0. When repo A calls repo B's function, the extractor computed the target hash using repo A's URL, but the node was stored with repo B's URL. 4,712 dangling edges from polywave-web.

### Scout

Identified the root cause in `goextractor/extractor.go` line 306: `ComputeNodeHash(opts.RepoURL, targetPkg, ...)` uses the calling repo's URL for all targets. Designed a two-pronged fix: preventive (extractor resolves correct repo URL) + corrective (new resolver package retargets dangling edges).

### Waves

**Wave 1 (3 agents):**

| Agent | Duration | Task |
|-------|----------|------|
| A | 38s | Added 4 methods to GraphStore interface (DanglingEdges, AllRepos, NodesByQualifiedName, DeleteEdge) |
| B | 124s | SQLite implementations + migration 002 + tests |
| C | 203s | New `internal/resolver/` package with reverse hash lookup |

**Post-merge issue:** 4 packages had mock GraphStores that didn't implement the new methods. Repair agent fixed all 4 mocks + schema version assertion in 33s.

**Wave 2 (1 agent):**

| Agent | Duration | Task |
|-------|----------|------|
| D | 276s | Fixed extractor hash computation, wired resolver into IndexRepo |

### Post-IMPL manual fix

Cross-repo edges still showed 0 after the IMPL because `resolveTargetRepoURL` returned Go module paths (`github.com/org/repo`) but nodes were stored with filesystem paths (`/Users/.../repo`). Added `ModuleToRepoURL` map to `ExtractOptions`, populated by reading `go.mod` from each indexed repo. Single targeted fix.

### Result

```
$ knowing index polywave-go   # 6,340 nodes, 17,232 edges
$ knowing index polywave-web  # 1,569 nodes, 5,939 edges
Cross-repo edges: 228
```

polywave-web's `runAnalyzeDeps` calls polywave-go's `analyzer.BuildGraph`. polywave-web's `runScaffold` calls polywave-go's `engine.RunScaffold`. The system correctly identifies cross-repo function calls.

### Friction

1. **Critic caught a real issue:** Agent D's mock missing 4 new GraphStore methods. Would have broken Wave 2 compilation. Worth the 2-minute critic overhead.
2. **Cascading mock breakage:** Extending a shared interface breaks every mock in the codebase. 4 packages needed stubs. This is the #1 cascading issue with interface changes in polywave.
3. **Module path vs filesystem path:** The scout correctly identified the hash mismatch but didn't anticipate that repo URLs in the store are filesystem paths while `go/packages` returns module paths. Required a manual fix after the IMPL. Only surfaced by testing on real repos.

### Polywave post-rebrand note

This entire session was the first real usage of polywave after a massive rebranding refactor (scout-and-wave to polywave). Several bugs surfaced that were missed configuration paths from the rebrand:
- `check_scout_boundaries` hook: relative vs absolute path comparison (line 40)
- `prepare-wave`/`validate` schema disagreement: `original_branch` key not in validator schema
- `SCOUT_COMPLETE` as invalid state value (not in allowed state enum)
- `cbm-code-discovery-gate` hook (leftover from MCP server demo, not rebrand-related)

All bugs were in the configuration layer, hooks, and state machine labels. Polywave's core coordination logic (worktrees, briefs, merge, verify) worked correctly throughout.

---

## Optimize Indexing (2026-05-15, in progress)

### Baseline benchmark

Cold index of polywave-go (6,340 nodes, 17,232 edges):
- **16 minutes 24 seconds** wall time
- 594s user + 2,358s system CPU (300% utilization)
- Root cause: `go/packages.Load` called once per file (~100+ invocations of the full Go type checker)

Incremental index (no changes): **1.7 seconds**

### Target

Cold index under 30 seconds (33x improvement). Single `go/packages.Load("./...")` call, worker pool for extraction, package result distribution.

### Scout

Scout analyzed the current per-file `go/packages.Load` bottleneck. Designed a 4-agent, 2-wave plan:

**Wave 1 (3 parallel agents):**
- Agent A: `BulkLoad()` in `goextractor/loader.go` (single `packages.Load("./...")` call, returns map of file path to loaded package)
- Agent B: Refactor `goextractor/extractor.go` with `ExtractWithPackage()` (accepts pre-loaded package, avoids per-file Load)
- Agent C: Worker pool in `indexer/worker.go` (`parallelExtract()` with `runtime.NumCPU()` workers)

**Wave 2 (1 agent):**
- Agent D: Wire everything together in `IndexRepo` (BulkLoad -> build work items -> parallelExtract)

### Wave 1 (3 parallel agents)

| Agent | Duration | Task |
|-------|----------|------|
| A | 130s | Created `goextractor/loader.go` with `BulkLoad()`: single `packages.Load("./...")` call, returns `LoadedPackages` map |
| B | 118s | Refactored extractor: extracted shared logic into `extractFromPackage()`, added `ExtractWithPackage()` for pre-loaded packages |
| C | 121s | Created `indexer/worker.go`: fan-out/fan-in worker pool with `runtime.GOMAXPROCS` goroutines, pre-sized results array, order-preserving |

Merge: clean, no conflicts.

### Wave 2 (1 agent)

| Agent | Duration | Task |
|-------|----------|------|
| D | 189s | Wired BulkLoad + ExtractWithPackage + parallelExtract into IndexRepo, with fallback to per-file loading on BulkLoad failure |

Merge: clean.

### What changed (before vs after)

**Before (per-file loading):**
- `IndexRepo` walked all files sequentially
- For each `.go` file, called `go/packages.Load(".")` in the file's directory
- Each Load call independently resolved the full module dependency graph, ran the Go type checker, and built the AST
- For polywave-go (~100 Go files across ~30 packages), this meant ~100 invocations of the full type checker
- Each invocation redundantly re-resolved the same transitive dependencies

**After (bulk loading + worker pool):**
- `IndexRepo` calls `goextractor.BulkLoad("./...")` once to load all packages in the module
- Builds a `LoadedPackages` map of file path to pre-loaded `*packages.Package`
- Creates work items: Go files with a pre-loaded package use `ExtractWithPackage()` (skip Load), others use standard `Extract()`
- Feeds work items to `parallelExtract()` with `runtime.GOMAXPROCS` workers
- Workers extract ASTs, compute hashes, and produce nodes/edges in parallel
- Results collected in submission order (deterministic)

### Benchmark result: FAILED TO MEET TARGET

| Metric | Before (per-file) | After (bulk ./...) | After (per-package) | Target |
|--------|-------------------|--------------------|--------------------|--------|
| Cold index polywave-go | 16m 24s | ~12m+ (killed) | **16m 31s** | 30s |
| CPU time | 594s user + 2358s sys | TBD | 595s user + 2409s sys | - |
| Incremental (no changes) | 1.7s | 1.7s (unchanged) | - |

**Why the optimization didn't work as expected:**

The bottleneck was misidentified. We assumed the per-file `packages.Load` overhead was in redundant package resolution. The fix (single `packages.Load("./...")`) eliminated redundant per-file loads, but `packages.Load("./...")` itself is expensive for large repos.

For polywave-go, `./...` loads every package in the module plus all transitive dependencies (hundreds of packages including stdlib, third-party deps). This single call does the full type-checking work that was previously spread across ~100 per-file calls. The total type-checking work is similar; it's just done once instead of 100 times. But Go's build cache already amortized much of the redundancy in the per-file approach.

The worker pool parallelizes AST extraction (which was already fast), not the type-checking (which is the actual bottleneck). `go/packages.Load` is the bottleneck, and it's not parallelizable from our side.

**Root cause:** `go/packages` with `NeedTypes | NeedTypesInfo` triggers the full Go type checker, which resolves all transitive dependencies for every package loaded. This is the same work regardless of whether you load per-file, per-package, or per-repo. The type-checking cost is proportional to the dependency graph size, not the number of Load calls.

For polywave-go, the transitive dependency graph includes hundreds of packages (stdlib + third-party). Type-checking all of them takes ~600s of CPU time no matter how we invoke `go/packages`. The per-file approach (16m24s), bulk `./...` approach (~12m+), and per-package approach (16m31s) all converge to the same total work.

**Attempted approaches and results:**

| Approach | Wall time | Why it didn't help |
|----------|-----------|-------------------|
| Per-file Load (100+ calls) | 16m 24s | Baseline. Redundant loads but Go build cache partially amortizes. |
| Bulk `./...` Load (1 call) + worker pool | ~12m+ (killed) | Single Load is still expensive. Workers parallelize extraction (fast) not type-checking (slow). |
| Per-package Load (~30 calls) + worker pool | 16m 31s | Each package load still type-checks its transitive deps. No improvement over baseline. |

**Conclusion:** The `go/packages` type checker is the fundamental bottleneck. No loading strategy can avoid the cost of type-checking the transitive dependency graph. The only path to fast cold indexing is avoiding full type-checking on the critical path.

### Next step: two-tier extraction

The only viable approach for fast cold indexing:

**Tier 1 (fast, immediate):** tree-sitter or `go/parser` AST-only pass. No type resolution. Produces function/type/method declarations and syntactic call expressions. Edges get provenance `ast_inferred` with lower confidence. Index completes in seconds.

**Tier 2 (slow, background):** `go/packages` type resolution OR LSP enrichment via `github.com/blackwell-systems/agent-lsp/pkg/lsp` (pure Go LSP client library, already exists). Upgrades `ast_inferred` edges to `ast_resolved` or `lsp_resolved`. Adds `implements` and `references` edges that require type info. Runs asynchronously after the graph is already queryable.

The graph is usable immediately after Tier 1. Tier 2 improves accuracy over time. This is a well-established pattern in code intelligence tools (tree-sitter for speed, LSP for accuracy).

agent-lsp's `pkg/lsp` package provides a battle-tested LSP client (hover, definition, references, implementations, call hierarchy) with no CGo dependencies. knowing can import it directly for Tier 2 enrichment instead of building its own LSP client.

### Two-Tier Extraction: Implemented (2026-05-15)

4 agents across 2 waves:

**Wave 1 (parallel):**
- Agent A (233s): Go tree-sitter extractor (`internal/indexer/gotsextractor/`). 13 tests. Uses `smacker/go-tree-sitter/golang` grammar. Produces declarations + syntactic calls with `ast_inferred` provenance, confidence 0.7.
- Agent B (286s): LSP enrichment pass (`internal/enrichment/`). 6 tests. Uses `agent-lsp/pkg/lsp` to start gopls. Upgrades `ast_inferred` edges to `lsp_resolved` confidence 0.9. Discovers implements/references edges via document symbols.

**Wave 2 (parallel):**
- Agent C (127s): CLI wiring. Added `--full` flag to `knowing index` (default: tree-sitter fast path). `serve` command defaults to tree-sitter. Synchronous enrichment after CLI index.
- Agent D (63s): Daemon `EnrichFunc` callback. Background enrichment goroutine after each successful index, tracked via WaitGroup for clean shutdown.

### Benchmark: Two-Tier Results

| Approach | Wall time | CPU | Nodes | Edges |
|----------|-----------|-----|-------|-------|
| go/packages only (baseline) | 16m 24s | 594s user + 2358s sys | 6,340 | 17,232 |
| Tree-sitter + LSP (before walker fix) | 5m 15s | 60s user + 97s sys | 19,770 | 64,122 |
| **Tree-sitter + LSP (after walker fix)** | **37s** | **7.3s user + 10.8s sys** | **2,564** | **8,604** |

**26x faster wall time, 81x less CPU.** From 16 minutes 24 seconds to 37 seconds.

**Fixes applied:**
- Added `.claude` and `testdata` to directory skip list in file walker (was indexing polywave agent worktree copies, inflating counts 3x)
- Enrichment logs collapsed to single summary line (was producing 33MB of per-edge error output)

**Enrichment errors fixed:** The per-edge upgrade approach (querying gopls GetDefinition for each `ast_inferred` edge) was fundamentally flawed: tree-sitter stores declaration positions in nodes, not call-site positions. Sending GetDefinition at a declaration line returns the declaration itself, useless for edge validation. Removed the per-edge upgrade path entirely. The enricher now focuses on `discoverNewEdges` via document symbols (GetImplementation, GetReferences), which queries gopls at symbol positions it understands.

### Final benchmark (clean run, all fixes applied)

| Metric | go/packages (baseline) | Tree-sitter + LSP (final) | Improvement |
|--------|----------------------|--------------------------|-------------|
| Wall time | 16m 24s | **36.5s** | **27x faster** |
| CPU time | 594s user + 2,358s sys | **6.7s user + 10.7s sys** | **89x less CPU** |
| Nodes | 6,340 | 2,564 | - |
| Edges | 17,232 | 8,604 | - |
| Enrichment errors | n/a | **0** | Clean |
| gopls scan | n/a | 402 files, 35s | Clean shutdown |

Tree-sitter pass: ~1.5 seconds. Graph queryable almost instantly. gopls enrichment runs in remaining ~35 seconds with zero errors.

**What made the difference:**
1. **tree-sitter instead of go/packages** for the fast path. No type checking, no transitive dependency resolution. Just AST parsing. ~1.5s vs 16m for the same repo.
2. **Skipping .claude and testdata directories** in the file walker. Eliminated 3x node inflation from polywave worktree copies.
3. **LSP enrichment via agent-lsp's pkg/lsp** for background type resolution. gopls handles the type-checking work, but after the graph is already queryable.
4. **Removing the broken per-edge upgrade** that produced 8,604 errors. The enricher now does what it's good at (document symbol discovery) and skips what it can't do (call-site resolution without positions).

**Node count difference (2,564 vs 6,340):** The tree-sitter extractor finds fewer nodes than go/packages because it extracts declarations from the repo's own source files only. go/packages also created nodes for cross-package call targets (external functions referenced but not defined in the repo). The tree-sitter approach is correct for the execution plane; cross-repo targets are handled by the resolver.

### What was lost by switching from go/packages to tree-sitter

| Capability | go/packages | tree-sitter | Impact |
|-----------|-------------|-------------|--------|
| Type-resolved call targets | Exact (knows `pkg.Foo` calls `other.Bar` via type info) | Syntactic (matches `pkg.Bar()` by string, may misidentify overloaded names) | Call edges have lower confidence (0.7 vs 1.0) |
| `implements` edges | Full (uses `types.Implements()` on all concrete/interface pairs) | None (tree-sitter can't determine interface satisfaction) | Lost until LSP enrichment discovers them |
| `references` edges | Full (uses `TypesInfo.Uses` for all identifier usages) | None (tree-sitter doesn't track non-call usages) | Lost until LSP enrichment discovers them |
| Cross-package type resolution | Exact (resolves imports to canonical package paths) | Heuristic (matches import alias to path via import declarations) | May misresolve aliased imports |
| Method receiver types | Exact (knows the receiver type from type checker) | Syntactic (parses `func (r *Type) Method()` as text) | Correct for simple cases, may fail on embedded types |
| Qualified name accuracy | Guaranteed correct via type checker | Derived from go.mod module path + relative directory | Correct for standard layouts, may fail for non-standard module structures |

**What this means in practice:** The tree-sitter graph is complete for declarations and syntactic call edges. It's missing `implements` and `references` edges entirely until LSP enrichment adds them. Call edges are present but with lower confidence because they're string-matched rather than type-resolved. For blast radius queries ("who calls this function?"), the tree-sitter graph gives the right answer for direct calls. It misses indirect calls through interfaces until enrichment runs.

**The `--full` flag** preserves access to the go/packages extractor for users who need full type resolution and are willing to wait 16 minutes. Default is fast (tree-sitter), opt-in is thorough (go/packages).

### Closing the gap: call-site positions + LSP edge upgrade (2026-05-15)

**Problem:** After two-tier extraction, the 8,604 tree-sitter call edges stayed at `ast_inferred` (0.7 confidence) permanently. The enricher discovered 213 new implements/references edges but couldn't upgrade existing edges because it didn't have call-site positions.

**Solution (3 changes):**

1. **Added call-site fields to Edge type:** `CallSiteLine` (1-indexed), `CallSiteCol` (0-indexed), `CallSiteFile` (relative path). Migration 003 adds columns to the edges table. These store where the call expression is in the source, not where the declaration is.

2. **Tree-sitter extractor populates call-site positions:** When creating a call edge, the extractor reads the tree-sitter node's `StartPoint()` (row, column) and stores them. Every call edge now has exact source location.

3. **Enricher uses call-site positions for edge upgrades:** Before any LSP queries, the enricher opens all Go files via `textDocument/didOpen` (gopls needs this for cross-package resolution). Then for each `ast_inferred` edge with call-site info, it calls `GetDefinition` at that position. If gopls confirms a definition exists, the edge is upgraded to `lsp_resolved` (0.9 confidence).

**Why opening files was critical:** The first attempt at edge upgrades produced 8,604 errors because gopls had no documents open. LSP servers require `textDocument/didOpen` before they can resolve cross-package references. Moving the file-open step before both edge upgrades and edge discovery fixed both paths.

**Evolution of the enrichment approach:**

| Attempt | Strategy | Result |
|---------|----------|--------|
| 1 | Query GetDefinition at source node declaration line | 8,604 errors (declaration position, not call site) |
| 2 | Skip per-edge upgrade, only discover new edges | 0 upgraded, 0 discovered (files not opened) |
| 3 | Open files first, discover new edges | 0 upgraded, 213 discovered (no call-site positions) |
| 4 | Add call-site positions, open files, upgrade + discover | **8,604 upgraded, 213 discovered, 0 errors** |

### Final benchmark: complete two-tier extraction

| Metric | go/packages (baseline) | Two-tier final | Improvement |
|--------|----------------------|----------------|-------------|
| Wall time | 16m 24s | **9.1s** | **108x faster** |
| CPU time | 594s user + 2,358s sys | **10.4s user + 7.1s sys** | **57x less CPU** |
| Nodes | 6,340 | 2,564 | - |
| Edges (tree-sitter) | - | 8,604 (ast_inferred 0.7) | Available in ~1.5s |
| Edges (after enrichment) | 17,232 (ast_resolved 1.0) | 8,604 (lsp_resolved 0.9) + 213 new | All upgraded in ~8s |
| implements edges | Included | 213 discovered by LSP | Parity |
| references edges | Included | Discovered by LSP | Parity |
| Enrichment errors | n/a | 0 | Clean |

**The gap with go/packages is closed.** Every edge is now at lsp_resolved confidence. implements and references edges are discovered. The only remaining differences: node count (tree-sitter finds declarations only, not cross-repo targets) and edge count (tree-sitter's syntactic matching may produce different call edges than go/packages' type resolution). For blast radius queries, both approaches produce equivalent results.

**What made 108x possible:**
1. tree-sitter for fast AST parsing (no type checker, ~1.5s)
2. LSP enrichment via agent-lsp's pkg/lsp (gopls does the type checking, but incrementally on opened files, ~8s)
3. Call-site positions in edges (enables per-edge LSP confirmation)
4. Opening all files before querying (gopls needs workspace context)
5. Single-pass architecture: tree-sitter + enrichment in one `knowing index` command

---

## Incremental Change Handling (2026-05-15, in progress)

### Problem

The content-addressed Merkle DAG, append-only edge event log, and snapshot diffing are core architectural decisions (#1, #3, #14). But they're currently hollow: the infrastructure exists in the schema but no data flows through it.

**What's broken:**

| Component | Architecture decision | Current state |
|-----------|---------------------|---------------|
| Edge event log | Decision #3: append-only, never lose history | **Always empty.** IndexRepo batch-inserts edges directly, never calls RecordEdgeEvent. |
| Snapshot diff | Decision #1: compare two root hashes to see what changed | **Always returns empty.** SnapshotDiff queries edge_events which is empty. |
| Staleness detection | Decision #1: hash mismatch = stale | **Detects but doesn't act.** StaleEdges finds mismatches but nothing cleans up stale data. |
| Old symbol cleanup | Decision #5: content-addressed file identity | **Not implemented.** Changed files get re-extracted but old nodes/edges from the previous version remain as ghosts. |
| Incremental enrichment | Decision #12: two-tier extraction | **Enricher runs on all edges every time.** No way to enrich only changed files. |

**The consequence:** Re-indexing a repo accumulates garbage. If a function is renamed from `Foo` to `Bar`, the graph has both `Foo` (ghost from old extraction) and `Bar` (new extraction). Blast radius queries return stale callers. The event log can't answer "when did this edge appear?" because it has no events. Snapshot diffs can't show "what changed since the last deploy?" because they depend on the event log.

This is the gap between "architecture on paper" and "architecture in practice." The decisions are correct; the implementation doesn't honor them yet.

### What needs to change

1. **GraphStore: add DeleteNodesByFile, DeleteEdgesBySourceFile** to remove all symbols from a changed file before re-extracting
2. **IndexRepo: cleanup before re-extract.** For each changed file, delete old nodes/edges, then extract fresh, then compute edge diff (what was added, what was removed) and record events
3. **Edge events: record on every index run.** After computing the diff between old and new edges for a file, write "added" and "removed" events to the edge_events table
4. **Snapshot diff: works once edge events are populated.** No code change needed; it already queries edge_events correctly
5. **Enricher: accept changed file set.** Only enrich edges from files that changed, not the entire repo
6. **Daemon: pass changed file list.** The file watcher knows which files changed; pass that to the enricher

### Scout

Launched. Analyzing the codebase to design the IMPL.
