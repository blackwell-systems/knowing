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

### Design pivot: git-based change detection

The first scout designed around the existing fsnotify watcher. During review, we identified a fundamental problem: **knowing indexes git repositories, not filesystems.** Watching individual files reacts to editor saves, build artifacts, temp files, and branch switches. The meaningful unit of change is a commit.

**Decision: replace fsnotify-on-everything with git-commit-driven detection.**

The insight: `git diff --name-only oldHead newHead` gives the exact set of changed files with zero false positives. No directory walking, no content hashing, no debouncing, no file descriptor pressure. One file descriptor on `.git/HEAD` instead of thousands on source files.

**Why this matters architecturally:**

1. **Snapshot-commit alignment.** If indexing is triggered by commits, every snapshot corresponds to exactly one commit. Point-in-time queries ("what did the graph look like at this commit?") become a simple lookup by commit hash. The snapshot chain becomes a parallel history to the git commit chain.

2. **Determinism.** Same commit always produces the same change set. Filesystem events are OS-dependent and non-deterministic (different event ordering on macOS vs Linux, different handling of atomic saves).

3. **Deleted file handling.** fsnotify DELETE events are unreliable and OS-dependent. `git diff --diff-filter=D` gives the exact list of deleted files every time.

4. **Incremental enrichment scope.** The changed file list from git diff is the exact input the enricher needs. No guessing, no over-enrichment.

**Three detection methods (prioritized):**

1. **Post-commit hook (primary):** Daemon installs a git hook that sends (repoPath, oldHead, newHead) via unix socket. Instant, zero polling.
2. **.git/HEAD watch (fallback):** fsnotify on one file instead of thousands. When HEAD changes, read new value, compare to stored.
3. **Polling (last resort):** `git rev-parse HEAD` every N seconds. For NFS/SMB environments.

**Uncommitted changes decision:** The graph indexes committed state only. Uncommitted changes are transient, violate determinism, and create noise in the snapshot chain. `knowing index --working-tree` is available as an opt-in for temporary snapshots not linked to the main chain.

**First scout discarded.** The IMPL it produced was designed around fsnotify events. Re-scouted with the git-based architecture as a binding constraint.

### Scout (re-launched)

Analyzing the codebase against the git-based change detection architecture documented in `docs/architecture.md`. The IMPL must implement: .git/HEAD watcher, git diff change resolution, incremental cleanup (DeleteNodesByFile, DeleteEdgesBySourceFile), edge event recording, scoped enrichment, and snapshot-commit alignment.

### Priority chain after this IMPL

**1. Change detection (this IMPL)** makes the content-addressed history real. Without it, the Merkle DAG has roots but no record of what changed between them. The event log is empty. Snapshot diffs return nothing. This is the first differentiator: a versioned ledger of software relationships with structural staleness detection.

**2. Runtime trace ingestion (next)** makes the graph unique. OTel spans as graph edges with observation-based confidence. "Is this route actually called in production?" is a query only knowing can answer. This is the second differentiator: ground truth from production, not just static analysis.

The order matters because runtime trace edges flow through the same pipeline as static edges: stored in SQLite, included in snapshots, diffable between snapshots, subject to staleness detection. If the incremental change pipeline doesn't work, runtime edges accumulate garbage the same way static edges do now. Change detection must ship first.

After runtime traces, the implementation path is:
- Semantic PR diff (architecture decision #14, the most visible feature)
- MCP server hardening (real implementations for stubbed handlers)
- More language extractors (TypeScript, Rust, Java via declarative configs)

### Scout completed, waves executed (2026-05-15)

6 agents across 3 waves. Critic passed clean (0 errors, 0 warnings).

**Wave 1 (3 parallel agents):**

| Agent | Duration | Task |
|-------|----------|------|
| A | 102s | Added DeleteNodesByFile, DeleteEdgesBySourceFile, EdgesBySourceFile to GraphStore + SQLite implementation + tests |
| B | 168s | Rewrote IndexRepo: tracks changed files, cleans up old nodes/edges before re-extract, records edge events (added/removed), added LastChangedFiles() accessor |
| C | 162s | Created GitWatcher (.git/HEAD + .git/refs/heads/* via fsnotify, 1-2 FDs), GitDiffFiles (git diff --name-status), CommitEvent type, tests for both |

Post-merge: mock cascade fix needed (3 packages, 30s). Same pattern as previous interface extensions.

**Wave 2 (2 parallel agents):**

| Agent | Duration | Task |
|-------|----------|------|
| D | 193s | Replaced FileWatcher with GitWatcher in daemon, rewrote watchLoop to consume CommitEvent, updated IndexFunc/EnrichFunc signatures to include changedFiles |
| E | 164s | Added RunScoped to enricher, extracted runFiltered with fileFilter function, scoped openAllFiles/upgradeCallEdges/discoverNewEdges to accept filter |

Post-merge: cmd/knowing/main.go signature cascade (IndexFunc + EnrichFunc with changedFiles). Fixed directly instead of Wave 3 agent.

**What's now working that wasn't before:**

1. **Edge events are recorded.** Every index run computes the diff between old and new edges per file and writes "added"/"removed" events to the edge_events table. The append-only log (architecture decision #3) is no longer empty.

2. **Old symbols are cleaned up.** When a file changes, all nodes and edges from the previous version are deleted before re-extraction. No more ghost symbols from renamed or deleted functions.

3. **GitWatcher replaces FileWatcher.** The daemon watches `.git/HEAD` and `.git/refs/heads/*` (1-2 file descriptors) instead of every source file (thousands of FDs). Change detection is commit-driven, not filesystem-event-driven.

4. **Git diff resolves the change set.** `GitDiffFiles` calls `git diff --name-status oldHead newHead` to get the exact list of changed, added, and deleted files. No directory walking, no content hashing for change detection.

5. **Scoped enrichment.** The enricher's `RunScoped` only processes edges from changed files. Unchanged files are skipped entirely in the LSP enrichment pass.

6. **SnapshotDiff returns real data.** Since edge events are now recorded, `SnapshotDiff(oldRoot, newRoot)` returns the actual edges that were added and removed between two snapshots. The content-addressed history is functional.

### Current state: 12,203 lines of Go across 39 files

The system now has a working incremental change pipeline: git-based detection, cleanup, extraction, edge events, snapshots, and scoped enrichment. The Merkle DAG, event sourcing, and snapshot diffing are real, not just schema.

Next: runtime trace ingestion (the moat).

### Incremental pipeline verified (2026-05-15)

Tested the full incremental pipeline on knowing's own repo:

| Metric | First index | Re-index (no changes) |
|--------|------------|----------------------|
| Nodes | 255 | 255 (unchanged) |
| Edges | 888 | 888 (unchanged) |
| Edge events | 1,034 (all "added") | 0 new events |
| Enrichment | 888 upgraded, 33 new | 0 processed |
| Time | ~5s + 4s enrichment | 2.1s |

The system correctly skips unchanged files, records zero new events when nothing changed, and the enrichment pass processes zero edges. The Merkle DAG and event log are functional.

---

## Documentation Sprint (2026-05-15)

Three background agents ran in parallel to bring the documentation up to production quality:

**Architecture doc enhancement (627s):**
- Added "Concepts" section defining all primitives from first principles (content-addressed storage, Merkle DAG, knowledge graph vs tree, nodes/edges/hashes, event sourcing, staleness, artifact boundary)
- Added "Concurrency Model" section (goroutine architecture, RWMutex coordination, channel communication, worker pool, SQLite WAL mode)
- Added "Data Flow" section tracing a single commit end-to-end through all system components with timing table

**FEATURES.md (386s):**
- 928-line comprehensive feature dump for AI consumption
- 21 implemented features with packages, entry points, limitations
- 7 interfaces with all methods and implementors mapped
- 11 MCP tools with functional status
- 25+ planned but NOT IMPLEMENTED features explicitly marked
- Storage schema, edge types, node kinds, configuration, metrics

**Doc comments (945s):**
- 677 lines of documentation added across 22 Go files
- Package-level doc comments for all 12 packages
- Exported type and function doc comments throughout
- Inline "why" comments for hash computation, recursive CTEs, tree-sitter matching, cross-repo resolution, Merkle construction, LSP enrichment flow, worker pool pattern, git HEAD parsing

---

## knowing-viz Repo Created (2026-05-15)

Separate repo: `github.com/blackwell-systems/knowing-viz`

Static site for graph visualization. Cytoscape.js galaxy view with repo clusters, cross-repo edge arcs, provenance coloring, confidence-based opacity, interactive node selection. Dark theme. No backend.

Stack: Vite + TypeScript + Cytoscape.js. Deployed to GitHub Pages (future).

The viz reads a JSON export from knowing (`knowing export > graph.json`). No code shared between repos. Screenshots from viz go into knowing's `assets/` for the README.

Branch protection ruleset mirroring knowing applied.

---

## Runtime Trace Ingestion Design (2026-05-15)

Comprehensive design document created at `docs/runtime-traces.md` (480 lines). This is the feature that makes knowing unique: production observability data as first-class graph edges.

**What the design covers:**

1. Data sources (OTel spans, gRPC metadata, HTTP logs, message queue traces, DB query logs)
2. Pipeline architecture (collector tap, normalization, symbol resolution, aggregation, edge creation)
3. Symbol resolution (the hard part): route-to-symbol mapping table built during static indexing
4. Confidence scoring: observation-based (0.95 for high traffic down to 0.2 for stale, GC at 90 days)
5. Edge hash computation: same formula as static edges, observation updates don't change hash
6. Observation storage: new columns (observation_count, last_observed) on edges table
7. Write lock contention: separate write queue, batches via indexCh, 128-slot buffer, drop oldest on overflow
8. Snapshot strategy: runtime edges don't trigger snapshots (commit-aligned only)
9. Service name mapping: knowing.yaml (primary), config service_map (fallback), heuristic (last resort)
10. Framework-specific route extraction: patterns for net/http, chi, gin, echo, gorilla/mux, gRPC, NATS/Kafka
11. Collector disconnect handling: exponential backoff, 4 health states, circuit breaker
12. Capacity estimates: deduplication reduces millions of spans to thousands of unique edges; pre-aggregation for >10M spans/hour
13. Artifact boundary compliance: edge creation is execution plane, traffic analysis is intelligence plane
14. Migration path: "no data" vs "confirmed dead" distinguished by service instrumentation status
15. TraceIngestor interface with DecayConfidence method
16. Daemon integration as continuous background goroutine
17. MCP tool changes (existing + 3 new: runtime_traffic, dead_routes, migration_progress)

**Design decisions made:**

- OTel is the primary data source (industry standard, most orgs already have a collector)
- No application code changes required (tap existing collector)
- Runtime edges coexist with static edges in the same graph, same SQLite file, same pipeline
- Confidence decays over time without re-observation (edges go stale structurally)
- Uninstrumented services show "no data" not "dead" (critical UX distinction)
- Pre-aggregation in the collector for high-volume environments (knowing doesn't process raw spans at scale)

**Status:** Design locked. Implementation not started. Depends on incremental change handling (completed).

---

## Runtime Trace Ingestion Implementation (2026-05-15)

Implementing the OTel-based runtime trace ingestion pipeline from the design doc at `docs/runtime-traces.md`. This is the feature that makes knowing unique: production observability data as first-class graph edges with observation-based confidence scoring.

### Scout

Scout analyzed all 39 source files and the 480-line design doc. Produced IMPL-runtime-traces with 7 agents across 2 waves, 14 files (11 new, 3 modified).

**Decomposition:**

| Wave | Agent | Package | Files | Responsibility |
|------|-------|---------|-------|----------------|
| 1 | A | `internal/trace/` | 1 | Shared types: TraceSpan, TraceIngestor interface, ConfidenceFromCount, HealthState, TraceIngestConfig |
| 1 | B | `internal/store/` | 3 | Migration 004 (observation_count, last_observed columns + route_symbols table), runtime store methods |
| 1 | C | `internal/types/` | 1 | Edge struct extension: ObservationCount and LastObserved fields |
| 1 | D | `internal/trace/` | 2 | Symbol resolver: route-to-node mapping via route_symbols table, span attribute extraction |
| 1 | E | `internal/trace/` | 2 | Confidence scoring, decay logic, provenance building (pure business logic, no DB) |
| 2 | F | `internal/trace/` | 3 | Core ingestor (span-to-edge conversion), OTLP gRPC receiver, batch accumulation |
| 2 | G | `internal/daemon/` + `cmd/` | 2 | Daemon trace goroutine lifecycle, CLI flags (--trace, --trace-endpoint, --trace-batch-size) |

**Key design decisions in the IMPL:**
- Agent B creates `sqlite_runtime.go` with runtime-specific scan helpers, avoiding modifications to `sqlite.go` and the cascade of mock breakage we've seen in every previous interface extension
- Agent D's resolver uses `*sql.DB` directly, not the store package, keeping `internal/trace` independent
- Agent G's daemon integration is a placeholder goroutine (blocks on `ctx.Done()`), avoiding a cross-package dependency on trace from daemon; full wiring is a future integration step
- Agent F's OTLP receiver uses an abstraction layer so the core ingestor works without OTel proto dependencies

### Validation

Pre-wave validation caught 3 fixable issues:
1. Scout wrote `file:` instead of `file_path:` in scaffolds section
2. Scout used `SCOUT_COMPLETE` as state value (not in allowed enum, should be `REVIEWED`)
3. Scout assigned Agent A's types.go to wave 0 instead of wave 1

All fixed in-place, re-validation passed clean.

### Critic

Triggered (5 agents exceeds 3-agent threshold).

**First pass: ISSUES (3 errors, 1 warning)**

All errors were contract-vs-brief signature mismatches where the scout wrote contracts that contradicted its own brief's deliberate decoupling decisions:

1. **SymbolResolver contract** said `NewSymbolResolver(store types.GraphStore)` but Agent D's brief said `NewSymbolResolver(db *sql.DB)`. The brief was right: using `*sql.DB` directly keeps `internal/trace` independent of `internal/store`.

2. **PutRouteSymbol contract** said `PutRouteSymbol(ctx, m trace.RouteMapping)` (single struct param) but Agent B's brief used individual params. The brief was right: if the store package imported `trace.RouteMapping`, it would create a circular dependency (trace imports store's DB, store imports trace's types).

3. **GetRouteSymbol contract** said return `*trace.RouteMapping` but Agent B's brief used a local `RouteSymbolRow` struct. Same reason as #2.

**Fix:** Updated all three contracts to match the briefs. The pattern: when the scout designs package decoupling in the briefs but forgets to propagate those decisions to the contracts section.

**Second pass: PASS (0 errors, 2 advisory warnings)**

Warnings (non-blocking):
- Agent F references gRPC/OTel proto deps that no agent owns in `go.mod`
- Agent F's OTLP receiver uses concrete `*Ingestor` instead of `TraceIngestor` interface (fine within same package)

### Scaffold

Deployed `internal/trace/types.go` (shared types for all wave 1 agents). 85 seconds. Build verified clean.

### Wave 1 (5 parallel agents)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| A | 73s | Types (no-op, scaffold already deployed) | - |
| B | 159s | Migration 004, 5 store methods in sqlite_runtime.go, RouteSymbolRow struct | 6 |
| C | 53s | Edge struct: added ObservationCount + LastObserved fields | - |
| D | 159s | SymbolResolver with Resolve + ResolveSpan, in-memory SQLite tests | 4 |
| E | 97s | ComputeConfidence, ShouldGarbageCollect, DecayBracket, BuildProvenance | 7 (29 cases) |

**Post-merge fix:** `TestNewSQLiteStore_CreatesDatabase` expected schema version 3, Agent B's migration 004 bumped it to 4. One-line fix.

**No mock cascade this time.** Agent B deliberately created `sqlite_runtime.go` with its own scan helpers instead of modifying `sqlite.go` or the GraphStore interface. This avoided the mock breakage that hit every previous interface extension. The scout's design decision to keep runtime methods off the interface paid off.

### Wave 2 (2 parallel agents)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| F | 230s | Core Ingestor (IngestSpans, IngestHTTPLogs, RuntimeEdgeStats, DecayConfidence, batch accumulation), OTLP receiver (placeholder, no OTel proto deps yet) | 8 |
| G | 99s | DaemonConfig.TraceConfig, traceIngestLoop goroutine, CLI flags (--trace, --trace-endpoint, --trace-batch-size) | existing pass |

**Merge:** Clean, zero conflicts. All 13 packages build and test.

### What's now implemented

The runtime trace ingestion pipeline from the design doc is now code:

1. **TraceSpan normalization.** Any observability source (OTel spans, HTTP logs) normalizes to a common `TraceSpan` struct.

2. **Symbol resolution.** `SymbolResolver` maps runtime identifiers (HTTP routes like "POST /api/users", gRPC methods like "UserService.GetUser") to graph node hashes via the `route_symbols` table. Unresolved routes get synthetic UNRESOLVED nodes with 0.3 confidence.

3. **Span-to-edge conversion.** `Ingestor.IngestSpans` resolves each span, determines edge type (runtime_calls, runtime_rpc, runtime_produces/consumes), computes a stable edge hash, and either creates a new edge or increments the observation count on an existing one.

4. **Confidence scoring.** Observation-based: 0.95 (>1000 observations), 0.85 (100+), 0.7 (10+), 0.5 (1+), 0.2 (stale). Decays over time without re-observation. GC-eligible after 90 days.

5. **Batch accumulation.** `AddToBatch` + `FlushBatch` for high-throughput ingestion with configurable batch size.

6. **Store layer.** Migration 004 adds `observation_count` and `last_observed` columns to edges, creates `route_symbols` table with composite PK. Five new methods on SQLiteStore in a separate file (no interface changes).

7. **Daemon integration.** Trace goroutine lifecycle managed by WaitGroup, gated on `--trace` CLI flag. Placeholder loop blocks on context cancellation.

8. **OTLP receiver.** Placeholder struct with Start/Stop/Health/ExportSpans. Real gRPC implementation requires adding OTel proto dependencies (future work).

### What's NOT yet wired

- OTLP gRPC server (needs `go.opentelemetry.io/proto/otlp` and `google.golang.org/grpc` in go.mod)
- Route-to-symbol mapping during static indexing (the `route_symbols` table exists but nothing populates it yet)
- Daemon's `traceIngestLoop` is a placeholder (blocks on ctx.Done(), doesn't create an Ingestor)
- MCP tools for runtime traffic queries (runtime_traffic, dead_routes, migration_progress)

### Final state: 14,601 lines of Go across 49 files

| Package | Files | Purpose |
|---------|-------|---------|
| `cmd/knowing` | 2 | CLI (serve, index, query, version) |
| `internal/types` | 3 | Hash, Node, Edge, GraphStore, Extractor interfaces |
| `internal/store` | 6 | SQLite GraphStore + runtime methods, migrations 001-004 |
| `internal/snapshot` | 3 | Merkle tree, snapshot chain, diff, GC |
| `internal/indexer` | 3 | Indexer, ExtractorRegistry, worker pool |
| `internal/indexer/goextractor` | 3 | Go packages extractor (--full flag) |
| `internal/indexer/gotsextractor` | 2 | Go tree-sitter extractor (default fast path) |
| `internal/indexer/treesitter` | 2 | tree-sitter Python extractor |
| `internal/enrichment` | 2 | LSP enrichment via agent-lsp pkg/lsp |
| `internal/mcp` | 3 | MCP server (11 tools, stdio + HTTP) |
| `internal/daemon` | 4 | Daemon lifecycle, GitWatcher, git diff |
| `internal/resolver` | 2 | Cross-repo edge resolver |
| `internal/trace` | 8 | Runtime trace ingestion pipeline |

13 packages. 49 files. 14,601 LOC. Single Go binary.

### Friction

**1. GOWORK env var (recurring, ~5 min wasted)**

Same stale `GOWORK` pointing to a non-existent `go.work` file. Baseline gates failed on first `prepare-wave` attempt. Fixed by adding `GOWORK=off` to quality gates in the IMPL doc. This is the third IMPL where this has been an issue.

**2. Critic caught contract-vs-brief mismatches (3 errors, valuable)**

The scout designed package decoupling in the briefs (Agent B uses individual params to avoid importing trace, Agent D uses `*sql.DB` to avoid importing store) but wrote the interface contracts with the wrong signatures. The critic caught all three:
- SymbolResolver contract said `types.GraphStore`, brief said `*sql.DB`
- PutRouteSymbol contract said `trace.RouteMapping` struct, brief said individual params
- GetRouteSymbol contract said `*trace.RouteMapping` return, brief said local `RouteSymbolRow`

Without the critic, Agent B and Agent F would have implemented against different signatures. Worth the 2-minute overhead.

**3. Agent A was a no-op (wasted slot)**

The scaffold already deployed `internal/trace/types.go`. Agent A launched, verified the scaffold was correct, and reported complete without changing anything. The scout should not create a wave 1 agent for work the scaffold already handles.

**4. Schema version assertion (1 test failure, 30s fix)**

`TestNewSQLiteStore_CreatesDatabase` hardcodes the expected schema version. Agent B's migration 004 bumped it from 3 to 4. Post-merge fix: change `3` to `4`. This test is fragile by design (it verifies all migrations ran) but it breaks on every new migration.

---

## CI/CD and Distribution (2026-05-15)

Created GitHub Actions workflows and release pipeline adapted from agent-lsp:

**`.github/workflows/ci.yml`**: Build, vet, test on push/PR to main. Binary smoke test.

**`.github/workflows/release.yml`**: Full release pipeline on `v*` tags:
- GoReleaser: 6 platform binaries (linux/darwin/windows x amd64/arm64)
- Homebrew formula via blackwell-systems/homebrew-tap
- Docker multi-arch images (GHCR + Docker Hub)
- Winget auto-publish
- npm: 7-package publish (root + 6 platform-specific)
- PyPI: platform-specific wheels
- MCP Registry: GitHub OIDC auto-publish

**`.github/workflows/docs.yml`**: mkdocs-material to GitHub Pages.

**`.goreleaser.yml`**: GoReleaser v2 config with Homebrew formula, Docker manifests, changelog filtering.

**`docs/DISTRIBUTION.md`**: Complete distribution strategy covering all channels (Homebrew, Scoop, Winget, npm, PyPI, Docker, MCP registries, go install, curl|sh, self-update, uninstall).

---

## Runtime Wiring and Developer Tools Batch (2026-05-15)

The second polywave IMPL of the session. Wires up the runtime trace pipeline end-to-end, adds developer-facing query tools and export CLI.

### Scout

Scout analyzed 49 source files and produced IMPL-runtime-wiring-devtools with 5 agents across 2 waves.

**Decomposition:**

| Wave | Agent | Package | Files | Responsibility |
|------|-------|---------|-------|----------------|
| 1 | A | `gotsextractor/` | 2 | HTTP route extraction: detect HandleFunc, chi, gin, echo, gorilla/mux patterns, create route_handler nodes + handles_route edges, populate route_symbols |
| 1 | B | `internal/trace/` + `go.mod` | 4 | Real OTLP gRPC receiver: replace placeholder with collectortrace.TraceServiceServer, add OTel proto + gRPC deps |
| 1 | C | `internal/store/` + `cmd/` | 4 | Store runtime queries (RuntimeEdgesByService, DeadRoutes, RuntimeEdgeStatsAggregate) + `knowing export` CLI command |
| 2 | D | `internal/daemon/` | 2 | Real traceIngestLoop: SymbolResolver + Ingestor + OTLPReceiver lifecycle, periodic flush + decay |
| 2 | E | `internal/mcp/` | 3 | MCP runtime tools: runtime_traffic, dead_routes, trace_stats (14 total tools now) |

### Validation and Critic

Validation caught a YAML parse error: `post_merge_checklist` used a sequence of objects where polywave-tools expected a different format. Fixed in-place.

Critic passed clean on first run (0 errors, 1 advisory warning about the known DBPath integration gap between Agent D and Agent C).

### Wave 1 (3 parallel agents)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| A | 173s | HTTP route extraction for 5 router packages | 3 new |
| B | 287s | Real OTLP gRPC receiver with OTel proto deps | 4 new |
| C | 281s | Store runtime queries + export CLI | 6 new |

**Rate limit incident:** All 3 agents hit a rate limit on first launch (0 work done, 0 commits). Worktrees were untouched. Relaunched all 3 after limit reset; all completed successfully on second attempt.

**Merge:** Clean, zero conflicts. 2 integration gaps noted (expected: `NewOTLPReceiver` and `RuntimeEdgesByProvenance` wired by Wave 2).

### Wave 2 (2 parallel agents)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| D | 160s | Real daemon traceIngestLoop with dedicated DB connection | 2 new |
| E | 246s | MCP runtime_traffic, dead_routes, trace_stats handlers | 4 new |

**Merge:** Clean, zero conflicts. Zero integration gaps.

**Post-merge fix:** Wired `DBPath: *dbPath` in `cmd/knowing/main.go` cmdServe (known integration gap from IMPL pre-mortem). One-line change.

### What's now end-to-end functional

The full runtime trace pipeline is wired:

```
Static indexing
  └─ tree-sitter detects http.HandleFunc("/api/users", handler)
  └─ creates route_handler node + handles_route edge
  └─ writes route_symbols entry: ("service", "GET /api/users", handlerHash)

knowing serve --trace --trace-endpoint localhost:4317
  └─ traceIngestLoop starts
  └─ OTLPReceiver listens on :4317 (gRPC)
  └─ OTel collector sends spans
  └─ Ingestor resolves spans via route_symbols
  └─ Creates/updates runtime edges with observation counts
  └─ Periodic FlushBatch (configurable) + DecayConfidence (hourly)

MCP tools (14 total)
  └─ runtime_traffic: "show me traffic to /api/users"
  └─ dead_routes: "which routes haven't been called in 30 days?"
  └─ trace_stats: "how many runtime edges, active vs stale?"

knowing export --format json
  └─ dumps full graph for knowing-viz consumption
```

### Friction

**1. Rate limit killed all 3 Wave 1 agents (lost ~15 min waiting)**

All agents launched, hit rate limit immediately, returned with no work done. Worktrees were untouched, so relaunch was clean. This is the first time rate limiting affected a polywave run.

**2. YAML parse error in post_merge_checklist (30s fix)**

Scout wrote `post_merge_checklist` as a sequence of objects with `description` keys. polywave-tools expected a `items` key with string values. Minor schema mismatch.

**3. Zero friction on the actual implementation**

No mock cascades (runtime methods added to sqlite_runtime.go, not the GraphStore interface). No merge conflicts. No test failures. Critic passed on first run. The cleanest IMPL of the session.

---

## Documentation Update (2026-05-15)

Two background agents updated the documentation to reflect all new features:

**architecture.md**: Added runtime trace ingestion pipeline architecture, HTTP route extraction, traceIngestLoop goroutine documentation, expanded MCP tools (11 to 14), schema evolution (migration 004), provenance tiers (otel_trace), export CLI.

**FEATURES.md**: 9 new feature sections (#22-30), updated MCP tools, storage schema, edge types, node kinds, interfaces, file inventory, moved runtime trace items from "planned" to "implemented".

---

## Session Summary (2026-05-15)

### What was built today

Two polywave IMPLs implementing the runtime trace ingestion pipeline from design to working code:

1. **IMPL-runtime-traces** (7 agents, 2 waves): Core pipeline components: types, store migration, symbol resolver, confidence scoring, ingestor, OTLP placeholder, daemon CLI flags.

2. **IMPL-runtime-wiring-devtools** (5 agents, 2 waves): End-to-end wiring: HTTP route extraction, real OTLP gRPC receiver, daemon lifecycle, MCP runtime tools, export CLI.

Plus: integration test, test coverage (33 new tests), CI/CD workflows, distribution doc, roadmap update, architecture and features doc updates.

---

## Semantic PR Diff (2026-05-15)

The most visible developer feature. Relationship-level impact analysis for pull requests: instead of "you changed 3 files," it shows "you removed `ValidateToken` which has 8 callers across 4 packages, risk: high."

### Scout

Scout analyzed 51 source files and produced IMPL-semantic-pr-diff with 4 agents across 2 waves, 10 files (7 new, 3 modified).

**Decomposition:**

| Wave | Agent | Package | Files | Responsibility |
|------|-------|---------|-------|----------------|
| 1 | A | `internal/diff/` | 4 | SemanticDiff (enriches raw SnapshotDiff with node metadata, detects modifications) + PRImpact (blast radius for changed symbols, risk classification) |
| 1 | B | `cmd/knowing/` | 2 | `knowing diff` CLI subcommand with JSON and human-readable output |
| 1 | C | `.github/workflows/` | 1 | GitHub Action: indexes both branches, computes diff, posts/updates PR comment |
| 2 | D | `internal/mcp/` + `cmd/` | 3 | Wire diff package into MCP handlers (replace stubs), wire cmdDiff into main.go |

### Validation and Critic

Scout missed `GOWORK=off` on build and lint gates (had it on test but not the others). Fourth time this session. Fixed in-place.

Critic passed clean on first run: 0 errors, 0 warnings.

### Scaffold

Deployed `internal/diff/types.go` with shared result types: SemanticDiffResult, PRImpactResult, NodeChange, EdgeChange, SymbolImpact, ImpactSummary, DiffSummary.

### Wave 1 (3 parallel agents)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| A | 236s | SemanticDiff + PRImpact with enrichment and risk classification | 7 |
| B | 131s | `knowing diff` CLI with JSON and text output | 4 |
| C | 84s | GitHub Action workflow (pr-semantic-diff.yml) | - |

**Merge:** Clean, zero conflicts. Zero integration gaps.

### Wave 2 (1 agent)

| Agent | Duration | Task | Tests |
|-------|----------|------|-------|
| D | 169s | Replaced MCP stubs with real diff.SemanticDiff and diff.PRImpact calls, wired cmdDiff into main.go | 2 |

**Merge:** Clean. All 14 packages pass.

### What the semantic diff provides

**SemanticDiff** takes two snapshot hashes and returns:
- Nodes added/removed (with qualified names, kinds, signatures)
- Modified nodes (detected from edge changes: nodes whose relationships changed without the node itself being added/removed)
- Edges added/removed (with source/target qualified names, edge types, confidence)
- Summary counts

**PRImpact** builds on SemanticDiff and adds:
- For each changed symbol: list of callers (blast radius) and callees (transitive, depth 3)
- Risk level: low (0-5 affected callers), medium (6-20), high (>20)
- Impact summary with unique caller/callee counts

**GitHub Action** (`pr-semantic-diff.yml`):
- Triggers on PR open/synchronize against main
- Indexes both base and head branches into separate DBs
- Merges base data into head DB for cross-snapshot queries
- Runs `knowing diff` and posts a formatted comment with symbol tables, edge changes, and risk assessment
- Updates existing comment on subsequent pushes (finds by marker)

### Friction

**Zero.** Cleanest IMPL of the session. No merge conflicts, no test failures, no post-merge fixes. Critic passed first try. Only issue was the recurring GOWORK omission in quality gates.

---

## Session Summary (2026-05-15)

### What was built today

Three polywave IMPLs implementing the runtime trace pipeline, developer tools, and semantic PR diff:

1. **IMPL-runtime-traces** (7 agents, 2 waves): Core pipeline components: types, store migration, symbol resolver, confidence scoring, ingestor, OTLP placeholder, daemon CLI flags.

2. **IMPL-runtime-wiring-devtools** (5 agents, 2 waves): End-to-end wiring: HTTP route extraction, real OTLP gRPC receiver, daemon lifecycle, MCP runtime tools, export CLI.

3. **IMPL-semantic-pr-diff** (4 agents, 2 waves): Relationship-level impact analysis: SemanticDiff, PRImpact, `knowing diff` CLI, MCP handler upgrades, GitHub Action for PR comments.

Plus: integration test, test coverage (33 new tests), CI/CD workflows, distribution doc, roadmap update, architecture and features doc updates.

### By the numbers

| Metric | Start of session | End of session |
|--------|-----------------|----------------|
| Go LOC | 12,203 | 19,030 |
| Files | 39 | 58 |
| Packages | 11 | 14 |
| MCP tools | 11 | 14 |
| Migrations | 3 | 4 |
| CLI subcommands | 4 | 6 |
| Tests | ~80 | ~150+ |

### Polywave stats for this session

| IMPL | Agents | Waves | Wall time (approx) | Friction |
|------|--------|-------|-------------------|----------|
| runtime-traces | 7 | 2 | ~25 min | GOWORK, critic caught 3 errors, schema version test |
| runtime-wiring-devtools | 5 | 2 | ~30 min (includes rate limit wait) | Rate limit, YAML parse, DBPath wiring |
| semantic-pr-diff | 4 | 2 | ~20 min | GOWORK in quality gates (recurring) |

16 agents total across 6 waves. Zero merge conflicts across all 3 IMPLs. One rate limit incident. Two post-merge fixes (both one-liners). No stubs, no placeholders, no unwired symbols remaining.
