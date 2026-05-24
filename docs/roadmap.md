# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Immediate Priorities

| # | Item | Why | Effort |
|---|------|-----|--------|
| 1 | **Parallel write backend** | SQLite single-writer funnels all extraction results through one goroutine. Even with producer-consumer pipeline, writes are serial. Need parallel write support for large repos. | High |
| 2 | **Real users** | Everything else is validated by benchmarks, not usage. Task memory compounds with use. | Ongoing |
| 3 | ~~**Session memory persistence**~~ | ~~SessionTracker is ephemeral. Persist session working sets to SQLite so resumed sessions compound.~~ **Shipped.** Task memory records top-5 symbols per call in `task_memory` table (SQLite). Persists across restarts. Boost formula: `0.5 + score * 0.4`. | ~~Medium~~ |
| 4 | ~~**`knowing stats`**~~ | ~~Show session value: context calls, symbols served, feedback rate.~~ **Shipped.** | Low |

## Storage Backend (P0 Performance)

Current: SQLite (single-writer, FTS5 deferred to background). Extraction is parallel (GOMAXPROCS workers, producer-consumer pipeline), but all DB writes funnel through one goroutine. Performance pragmas: `synchronous=NORMAL`, `mmap_size=256MB`, `cache_size=64MB`, `busy_timeout=5000`, `temp_store=MEMORY`. Multi-row batch INSERTs (edges: 100/statement, nodes: 99/statement, files: 249/statement) reduce per-row overhead.

### Options under evaluation

| Backend | Parallel writes | Query model | Deployment | Status |
|---------|----------------|-------------|-----------|--------|
| **SQLite sharded by package** | Yes (one file per package) | Cross-package queries need federation | Multiple files | Prototype next |
| **DuckDB** | Yes (appender API) | SQL, columnar scans | Single file, CGO | Evaluate |
| **BadgerDB/Pebble** | Yes (LSM concurrent memtable) | Key-value (custom query layer) | Single dir, pure Go | Evaluate |
| **SQLite + deferred FTS** | No (serial) | SQL + FTS5 | Single file | **Shipped (current)** |

### Sharding by package (leading candidate)

Packages are already the unit of Merkle computation, cache invalidation, diffing, and RWR scoring. One SQLite file per package means:
- Parallel writes: each extraction worker writes to its own package's DB
- No contention: workers never touch the same file
- Package-scoped queries are local reads
- Delete a package = delete the file
- Merkle computation per-package is already isolated
- Cross-package queries (blast radius, transitive callers) federate across shards

### Current performance (v0.6.0 + optimizations)

| Repo | Files | Edges | Extraction | Total (with deferred FTS) |
|------|-------|-------|-----------|--------------------------|
| knowing (84K LOC) | 448 | 25K | 0.4s | 1.7s |
| flask (15K LOC) | 97 | 9K | 0.04s | 0.3s |
| cargo (150K LOC) | 979 | 79K | 0.2s | 5.5s |
| kubernetes (3.5M LOC) | 4,877 | 268K | 18.6s | ~22s (data queryable immediately) |

## Cross-Repo Query Architecture

The context engine (ForTask, ExplainSymbol, RWR, HITS, BM25) has no repo-scoping anywhere in its query path. If multiple repos exist in the same database, cross-repo queries work with zero code changes. The challenge is the storage model: the roster currently assigns each repo its own SQLite file.

Two approaches are under evaluation:

### Option A: Unified Database (shared graph)

All repos index into a single `~/.knowing/knowing.db`. The roster tracks metadata (paths, URLs) but not separate DB files.

**Pros:**
- Zero engine changes. ForTask, BM25, RWR, FTS5 all work unchanged on the merged graph.
- Cross-repo edges resolve naturally (source and target in same DB).
- One FTS5 index covers all vocabulary. BM25 ranks across all repos in a single query.
- Simplest implementation (~30 LOC change: roster defaults to shared DB).
- Single snapshot chain covers all repos (Merkle diff shows cross-repo changes).
- `knowing remove` already deletes by repo_hash within a shared DB.

**Cons:**
- No isolation between projects. A personal side-project and work monorepo share one graph.
- Larger single file (5 repos x 30K edges = 150K edges, still trivial for SQLite, but conceptually messy).
- Can't delete a repo by deleting a file (must use `knowing remove` which does SQL DELETE).
- If the shared DB corrupts, all repos are affected.
- Users may not want their repos' symbols showing up when querying from a different project.

**Mitigation:** Add `--isolated` flag to `knowing add` for repos that should stay separate. Default to shared for most workflows.

### Option B: Federated Store (query-time merge)

A `FederatedStore` wrapper implements `GraphStore` over N underlying SQLiteStores. The primary store (current repo) receives writes; all roster stores are opened read-only for queries.

```go
type FederatedStore struct {
    primary *SQLiteStore      // writes go here
    others  []*SQLiteStore    // read-only roster DBs
}
```

Query federation strategy per method:
- `NodesByName`: query all stores, concat results, dedup by hash
- `SearchBM25Nodes`: query all stores, merge by score, take top-N
- `EdgesFrom`/`EdgesTo`: query all stores, concat (cross-repo edges live in source DB)
- `GetNode`: try primary first, then others (hash-based lookup)
- `FeedbackBoosts`: query all stores, merge maps
- Write methods (`PutNode`, `PutEdge`, `RecordFeedback`): primary only

**Pros:**
- Per-repo isolation by default. Each repo is a separate file with independent lifecycle.
- `knowing remove` is just closing and deleting a file.
- No corruption propagation between repos.
- Each repo can be backed up, synced, or deleted independently.
- No storage model change; existing per-repo DBs work as-is.
- Users opt-in to cross-repo by having multiple repos in their roster. No surprise data mixing.

**Cons:**
- N queries per method call (latency scales linearly with roster size). 3-5 repos: negligible (<5ms). 20+ repos: needs parallel goroutines.
- FTS5 indexes are per-DB; BM25 merge is approximate (scores from different corpus sizes aren't directly comparable without normalization).
- RWR adjacency map must load edges from all stores, making the first query slower.
- Cross-repo edges are split: source DB has the edge, target DB has the target node. `GetNode` must check multiple stores to resolve targets.
- Medium implementation effort (~200 LOC new type + method-by-method federation logic).
- Feedback recorded in the primary DB may reference nodes in other DBs (works, but feedback is stored asymmetrically).
- Community detection runs per-DB (Louvain on isolated subgraphs); cross-repo communities won't form.

### Comparison

| Dimension | Unified DB | Federated Store |
|-----------|-----------|-----------------|
| Implementation effort | ~30 LOC | ~200 LOC |
| Engine changes required | None | None (same interface) |
| Query latency | 1 query | N queries, merged |
| FTS5 quality | Unified corpus, accurate IDF | Per-corpus IDF, approximate merge |
| Cross-repo edges | Free (same table) | Resolved via multi-store lookup |
| Community detection | Cross-repo communities form naturally | Per-repo communities only |
| RWR walk | Seamless cross-repo | Cross-repo via edge concat |
| Isolation | None by default (opt-in via `--isolated`) | Full by default |
| Corruption blast radius | All repos | Single repo |
| Storage management | One file to manage | N files, cleaner lifecycle |
| `knowing remove` | SQL DELETE (fast) | Close + delete file (instant) |
| Feedback compounding | Cross-repo (symbol used in repo B helps repo A) | Asymmetric (feedback in primary only) |

### Decision

Not yet decided. The choice depends on real usage patterns:
- If most users work across 2-3 related repos (monorepo splits, frontend+backend): **unified DB** wins on simplicity and quality.
- If users have many unrelated projects and want clean separation: **federated store** wins on isolation.
- Both can coexist: unified by default with federated as the advanced mode, or vice versa.

Current status: per-repo isolation (no cross-repo queries). First real user who hits the limitation decides the approach.

## Operational

| Item | Description | Priority |
|------|-------------|----------|
| ~~`knowing stats`~~ | ~~Cumulative session value: context calls, symbols served, feedback rate, token savings.~~ **Shipped.** | ~~P2~~ |
| ~~Cross-repo awareness for non-Go extractors~~ | ~~TypeScript, Python, Rust, Java, and C# extractors use the local repo URL for all targets. Only the Go extractor has `inferRepoURL` with stdlib detection.~~ **Shipped.** All 5 OOP extractors now have `inferExternalRepoURL` with `"external://{packageName}"` or `"stdlib"` prefix. Python (site-packages + ~50 stdlib modules), TypeScript (bare specifiers), Rust (std::/core::/alloc::), Java (java.*/javax.*), C# (System.*/Microsoft.*). | ~~P2~~ |
| ~~Staleness reporting~~ | ~~`knowing stale` reports stale edges from changed files since last snapshot.~~ **Shipped.** `knowing stale` detects changed files via git diff, looks up stale nodes via `StaleNodesByFiles`, exits 1 when stale (CI-friendly). | ~~P2~~ |
| ~~Daemon lifecycle~~ | ~~`knowing daemon start --detach`, `status`, `stop`, `restart`.~~ **Shipped.** PID file at `~/.knowing/daemon.pid`, signal-based stop, process liveness check. | ~~P2~~ |
| ~~`untrack_repo` MCP tool + CLI~~ | ~~Evict a repo's nodes, edges, files, and snapshots.~~ **Shipped.** `knowing remove` + 28th MCP tool. Atomic deletion across all tables with per-table counts. | ~~P2~~ |
| ~~**Zero-config onboarding**~~ | ~~Auto-detect repos in workspace, index on first MCP query, no manual `knowing add` step.~~ **Shipped.** MCP server auto-indexes the git repository on first launch if no database exists. Detects git root, resolves repo URL, creates DB, indexes, and registers in roster. No manual `knowing index` or `knowing add` step needed. | ~~P1~~ |
| ~~**Implicit feedback from agent behavior**~~ | ~~Detect when agents use symbols from context results and auto-record positive feedback without agent cooperation.~~ **Shipped.** `ImplicitFeedback` tracker in context engine: registers returned symbols, detects usage via identifier matching (75% precision, 86% recall), records positive feedback for used symbols and negative for unused when the next context call flushes the batch. Wired into MCP server (`ObserveToolUse`), triggered by `graph_query` and `explain_symbol` handlers. P@10 lift pending feedback weight tuning (currently 0.15, both explicit and implicit are weight-limited at this graph scale). | ~~P1~~ |
| **Cross-repo context_for_task** | Search across ALL indexed repos simultaneously, not just one. Real projects span multiple repos (monorepo patterns, microservices). Merge results from all repos into one ranked list. See "Cross-Repo Query Architecture" section below. | P2 |
| **Incremental context ("next page")** | After an agent gets initial context, allow requesting the NEXT N symbols not yet seen. Avoids re-querying with bigger budget and getting duplicates. Session-stateful cursor. | P2 |
| **Staleness annotations on MCP responses** | When returning context, annotate symbols whose source files changed since last index. Agents know which results might be outdated without calling `knowing stale` separately. | P2 |
| **`explain_symbol` in context responses** | Inline "why ranked #3?" explanation in context results so agents can debug ranking without a separate tool call. Makes the system transparent. | P3 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |
| `class_hierarchy` MCP tool | Walk `extends` + `implements` + `overrides` edges. | P3 |
| `neighborhood` MCP tool | N symbols most densely connected to X within radius R. | P3 |
| GraphML/Cypher export | `knowing export -format graphml|cypher` for Neo4j, Gephi. | P3 |
| Active project scoping | `set_active_project` / `get_active_project` MCP tools. | P3 |

## Benchmarking Roadmap

14 benchmark harnesses exist today (see `bench/README.md`). The following gaps remain for a complete competitive evaluation story.

### P1: Would convince someone to adopt knowing

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **SWE-bench integration** | knowing + Claude solves N% more SWE-bench tasks than Claude alone. The definitive "does graph context help real agent work?" | Not started | High (full eval harness, 300 tasks, automated agent loop) |
| **Agent efficiency (Phase 2)** | Claude Code with knowing tools uses fewer tokens, fewer tool calls, higher correctness on discovery/ambiguity tasks. | Phase 1 complete (grep wins on known targets). **Phase 2 not started:** k8s-scale tasks, hook injection, ambiguous-name discovery, vague task-to-symbol mapping. Infrastructure ready. Full Phase 1 narrative: `bench/AGENT-EFFICIENCY-STUDY.md`. | Medium |
| **Real-session replay** | Replay 10+ real claudewatch session transcripts. Measure: context calls saved, symbols used that came from knowing, tasks where knowing provided the critical symbol. | Not started (implicit feedback tracker now exists for attribution) | High (transcript parser, attribution detection, manual annotation) |
| ~~**Cold start benchmark**~~ | ~~Time from `brew install` to first useful `context_for_task` result.~~ | ~~Zero-config implemented. Measured: auto-index fires on first MCP launch, indexes repos in <160ms (Flask) to ~18s (k8s), first context result available immediately after.~~ | ~~Low~~ |
| ~~**Aider head-to-head**~~ | ~~knowing vs Aider (tree-sitter + PageRank) on identical tasks.~~ | ~~**Done.** 5-way comparison (Run 20): knowing 4.5x more precise than Aider (P@10 0.226 vs 0.050). Aider adapter built, runs in cross-system harness. Full results in FINDINGS.md Runs 19-22.~~ | ~~Medium~~ |

### P2: Proves production readiness

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Query latency p50/p95/p99** | Instrument all 28 MCP tool handlers. Report latency distribution per tool across 1000 calls. | Single number (60ms avg) in cross-system FINDINGS; need distribution | Medium |
| **Indexing throughput (formalized)** | Dedicated harness: index 7 corpus repos, measure wall time, edges/sec, memory peak with variance reporting. | Numbers exist informally (Flask 0.1s, k8s 18.6s, competitive ratios in cross-system FINDINGS); needs reproducible go test harness | Low |
| **Incremental re-index cost** | Change 1 file in a 50K-edge repo, measure time to re-index just that file vs full. Isolates `--watch` per-edit cost. | Not benchmarked | Medium |
| **Staleness detection speed** | Benchmark `DiffHierarchicalTrees` on progressively larger graphs (10K, 50K, 100K, 500K edges). Show O(packages) not O(edges). | Have 517x number and Grafana scale test (714K edges); need the full scaling curve | Low |
| **Memory/disk footprint** | Measure RSS and DB file size after indexing each corpus repo. Compare to competitors. | Competitive numbers in cross-system FINDINGS (200MB vs 14GB/5.7GB); needs dedicated measurement harness | Low |

### P3: Completeness and rigor

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Multi-language extraction coverage** | For each of the 24 extractors: number of node types extracted, edge types produced, lines of test coverage. Comparison vs Sourcegraph SCIP, GitNexus, tree-sitter-graph. | Not started | Low (automated count + table) |
| **Grafana scale validation** | Full retrieval quality measurement on 714K-edge production graph (not just latency). P@10 with Grafana-specific task fixtures. | Latency test exists (`grafana_scale_test.go`); no retrieval quality measurement | Medium |
| **Graph integrity under load** | Spawn 10 concurrent indexers on overlapping repos. Run `knowing fsck` after. Proves content-addressing prevents corruption under concurrency. | Not started (fsck bench exists for single-indexer correctness) | Medium |
| **Concurrent query performance** | 100 parallel `context_for_task` calls on a 100K-edge graph. Measure throughput (queries/sec), latency degradation, and WAL checkpoint behavior. | Not started | Medium |
| **Cross-repo retrieval quality** | P@10 for tasks that span repo boundaries (e.g., "which frontend components call this backend endpoint?"). | Needs cross-repo implementation first | Medium |
| ~~**Feedback weight sensitivity**~~ | Moved to P1 regression prevention section (more urgent after Run 22 findings). | See above | - |
| ~~**LSP enrichment ROI**~~ | Moved to P1 regression prevention section (need to measure whether enrichment is net-positive after external node filter). | See above | - |

### P1: Competitive demolition demo

A head-to-head harness that runs identical queries against all systems and produces a side-by-side comparison. Non-technical audience (investors, potential users) can understand the result in 30 seconds.

**Format:** 5 tasks, 5 systems, table showing: response time, results returned, callers found, callers missed, RAM used, whether it even runs.

**Tasks designed to expose competitor failures:**
- "Find all callers of X" (GitNexus: can't index; grep: false positives; knowing: precise graph traversal)
- "What breaks if I remove this interface?" (Gortex: misses interface implementors; Repomix: dumps 300K tokens)
- "Affected tests for this file change" (no competitor has call-graph test prediction)
- "Index kubernetes and query it" (GitNexus: killed at 60min; CGC: impossible; knowing: 18.6s + 60ms query)
- "Give me 5K tokens of context for this task" (Repomix: 300K tokens, no ranking; knowing: ranked, budgeted)

**Output:** Markdown table + optional terminal recording (asciinema). Publishable on README, blog, Zenodo.

**Aider comparison: COMPLETE.** Results in cross-system FINDINGS Runs 19-22. knowing 4.5x
more precise than Aider (P@10 0.226 vs 0.050). Aider's tree-sitter + PageRank approach is
file-level; it can't rank individual symbols. The gap widens on larger repos.

**Status:** Data complete. Needs packaging into a polished demo format (markdown table +
asciinema recording) for publication.

### P1: Regression prevention and pipeline health

Items identified from the Run 22 regression investigation. These prevent future silent quality degradation.

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Channel balance regression test** | Assert that no single RRF channel returns >2x the combined results of other channels. The Run 22 regression was invisible until full benchmark re-run because no unit test caught unbounded equiv results. | Not started. Should be a `go test` in `internal/context/` that runs a fixed task against a fixture DB. | Low |
| **Per-repo P@10 tracking (CI gate)** | Track P@10 per repo across runs. Alert when any single repo drops >20% from its historical best. The Flask regression (0.321->0.20) went unnoticed because the full-corpus aggregate masked it. | Infrastructure exists (cross-system harness). Need: per-repo baseline file, comparison script, CI integration. | Medium |
| **Equivalence class expansion safety** | When adding new equiv classes, run the cross-system benchmark before/after and assert no repo drops >5%. Validates that new phrases/targets don't trigger the generic-noise pattern. | Not started. Could be a `pre-commit` check or manual gate on PRs touching `*_seeds.go`. | Low |
| **RRF channel contribution audit** | For each of the 117 tasks, log which channel contributed each of the top-10 final results. Produces a "channel attribution matrix" showing how often each channel is responsible for correct vs incorrect rankings. | Not started. Would answer: "is equiv still net-positive after the cap?" | Medium |
| **LSP enrichment ROI (fresh vs enriched)** | Run cross-system benchmark with and without LSP enrichment on the same repos. Quantifies the P@10 delta that enrichment adds. Current suspicion: enrichment helped in Run 18 (peak) but wasn't active in Runs 21-22. | Not started. Enrichment creates phantom externals (65% of Flask nodes); need to measure whether it's net-positive after the external filter. | Medium |
| **Feedback weight sweep** | Sweep feedback weight from 0.05 to 0.50. Plot P@10 vs weight on the feedback-loop bench (not cross-system, since cross-system is cold-start). Find the operating point where implicit/explicit feedback compounds. Currently 0.15 shows zero lift. | Infrastructure exists (`bench/feedback-loop/`). Need: parameter sweep harness, plot generation. | Low |

### Standalone Publication: Code Retrieval Evaluation Toolkit (CRET)

Extract knowing's benchmarking infrastructure as the SWE-bench equivalent for code
context retrieval. Full proposal: [docs/proposals/code-retrieval-eval-toolkit.md](../proposals/code-retrieval-eval-toolkit.md).

**Status:** Not started. Prerequisite complete (Aider comparison done, Run 19-22).

### Not yet benchmarked (tracked for completeness)

- **Proof verification throughput**: N proofs/sec verified (currently 1.2µs each = ~800K/sec theoretical)
- **Snapshot chain walk cost**: O(chain_length) for history queries
- **FTS5 rebuild cost vs graph size**: scaling curve for the deferred FTS rebuild
- **LSP enrichment ROI**: how much P@10 does LSP enrichment add over tree-sitter alone?
- **Language-specific P@10 breakdown**: already have per-repo numbers; need per-language aggregate

## Retrieval Pipeline

### Cross-System Benchmark Results (v0.6.3, 18 runs, 5 competitors)

97 manual fixtures + 10 SWE-bench across 5 repos (kubernetes, VS Code, flask, cargo, django). Competitive evaluation against 4 systems.

| System | P@10 | R@10 | Index k8s | Query latency | RAM (k8s) |
|--------|------|------|-----------|--------------|-----------|
| **knowing** | **0.226** | **0.396** | **18.6s** | **60ms** | **200MB** |
| Aider | 0.050 | - | N/A (file-level) | ~2.5s | - |
| Gortex | 0.229 (flask only) | - | 14.2 min | ~600ms | 14GB |
| GitNexus | 0.076 | 0.159 | >60 min (killed) | 612ms | 5.7GB |
| Repomix | N/A (no ranking) | 100% (dumps all) | N/A | N/A | N/A |
| grep | 0.020 | 0.035 | instant | instant | - |

**Statistical significance:**
- knowing vs grep: 11.3x (p<0.0001, d=0.92 very large)
- knowing vs Aider: 4.5x more precise (graph-based vs file-level PageRank)
- knowing vs GitNexus: 2.75x (p=0.0003, d=0.50 medium)
- knowing vs Repomix: 48x more token-efficient (4K vs 300K tokens)
- knowing vs Gortex: 1.4x on flask, 46x faster indexing on k8s, 70x less RAM

**Per-repo breakdown:** Django 0.330, Flask 0.336, VS Code ~0.25, Kubernetes 0.184, Cargo 0.123.

**Optimization ceiling diagnosed:** Graph connectivity exhausted. Remaining ~77% miss rate requires feedback compounding (cold-start floor 0.226, compounded ceiling ~0.40).

### Retrieval Improvements (ordered by expected impact)

| # | Item | Why (from benchmark data) | Expected Impact | Status |
|---|------|--------------------------|-----------------|--------|
| 1 | ~~**Ground truth fixture accuracy**~~ | ~~Many fixtures used language-native names not in the DB.~~ **Shipped.** Validated all 571 symbols, 95% match rate. validate-fixtures tool for ongoing verification. | Benchmark accuracy | ~~P0~~ |
| 2 | ~~**FTS/BM25 for non-Go qualified names**~~ | ~~FTS was broken (never populated in CLI mode) and tokenizer split on underscore.~~ **Shipped.** Migration 016 (symbol_name column, 10x weight), tokenchars '_', synchronous rebuild. | +0.006 P@10 | ~~P1~~ |
| 3 | ~~**Language-aware keyword extraction**~~ | ~~Task descriptions say "add a before_request hook" but the keyword extractor doesn't know "before_request" is a symbol name.~~ **Shipped.** `KeywordSet` struct separates Exact/Compounds/Components. Tiered search queries compounds first, components as fallback. Backtick-quoted identifiers are highest priority. Flask P@10 0.321->0.329. | +0.8pp Flask P@10 | ~~P1~~ |
| 4 | ~~**Equivalence classes for non-Go**~~ | ~~The 84 equivalence concepts were hand-tuned for Go patterns.~~ **Shipped (31 language-specific classes).** Python, TypeScript, Rust, Java, Kubernetes vocabulary. Total: 115 equivalence classes. | +2-4pp P@10 on non-Go repos | ~~P2~~ |
| 5 | ~~**Cross-file symbol resolution in Python/TS**~~ | ~~Python/TS calls didn't resolve through imports.~~ **Shipped.** Python buildPythonImportMap + resolveCallTarget (63 edges in flask). TypeScript buildTSImportMap + resolveCallEdgeWithImports (5,684 edges in TypeScript). | +0.013 P@10 | ~~P2~~ |
| 5a | ~~**Cross-file import resolution for Rust/Java/C#**~~ | ~~Same pattern needed for `use crate::module`, `import com.package.Class`, `using Namespace`.~~ **Shipped.** Rust (9,795 edges), Java (`buildJavaImportMap`, import + static import), C# (`buildCSharpImportMap`, using + using static). All use provenance `ast_resolved` / 0.85. Ocelot C# benchmark: P@10=0.14 (cold start, 5 tasks). | Ocelot P@10=0.14 | ~~P2~~ |
| 5b | **Terraform/infrastructure cross-file resolution** | `module.vpc.subnet_id` referencing another .tf file's output. Not in benchmark corpus but needed for real users. | User quality | P3 |
| 5c | ~~**Inheritance propagation**~~ | ~~Child classes couldn't reach parent methods via RWR.~~ **Shipped.** `propagateInheritance` creates `inherits` edges from child to parent methods. 83 edges Flask, 14,539 Django. | **+29% P@10 (Run 13)** | ~~P1~~ |
| 5d | ~~**Deeper call chain extraction (Python)**~~ | ~~Nested calls in arguments, lambdas, and inner functions were missed.~~ **Shipped.** Walk into call arguments, lambda bodies, nested functions. Flask +84% edges, Django +22% edges. | +0.001 P@10 (Run 14) | ~~P1~~ |
| 5e | ~~**Test file deprioritization**~~ | ~~36% of misses were test symbols.~~ **Shipped.** 0.3x penalty for test file symbols; conditional (removed when task mentions testing). | Noise reduction | ~~P1~~ |
| 6 | ~~**Session memory persistence**~~ | ~~The benchmark runs cold (no prior feedback). In real usage, feedback compounds. Session memory persistence carries learning across invocations.~~ **Shipped.** Task memory persists top-5 symbols per call in SQLite; boost `0.5 + score * 0.4`; 7-day linear decay. Cold-start benchmark cannot show improvement (each task unique, runs once); feedback-loop bench independently proves +20pp. | **Shipped** | ~~P2~~ |
| 6a | ~~**Phantom external node filtering**~~ | ~~External nodes from failed LSP enrichment dominated RWR results on repos with unresolved imports (Spark Java: 2282 externals, 63% of nodes).~~ **Shipped.** Filter at `filterNoisySymbols` (seed candidates) and RWR result loop (before scoring). Spark Java P@10 0.00->0.10. | Spark Java fixed | ~~P1~~ |
| 6b | ~~**Equivalence channel noise fix**~~ | ~~Unbounded equiv results (66) dominated RRF fusion on small graphs, flattening RWR scores. Single-word phrases ("request") triggered generic targets ("Get") matching every getter.~~ **Shipped.** Generic target filter (<=3 chars + blocklist), equiv cap at 2x(tiered+BM25), cleaned universal seeds. | **+124% P@10 (Run 22)** | ~~P0~~ |
| 7 | **More equivalence concepts (115 -> 150+)** | Graph-derived aliases help but are limited to the repo's own vocabulary. Need broader coverage of common patterns across ecosystems. Must respect Run 22 constraint: no single-word phrases, no generic targets. | +1-2pp P@10 | Ongoing |
| 8 | **Code-tuned embedding model** | BGE-small-en-v1.5 tested net-negative. A code-tuned model (CodeBERT, UniXcoder) might improve semantic matching between task descriptions and symbol names. | Unknown (needs evaluation) | Planned (optional) |
| 9 | ~~**Community-aware retrieval**~~ | ~~Constrain RWR walk to seed communities. Reduces noise from unrelated packages.~~ **Shipped.** `CommunityFilteredRWR` constrains BFS to seed communities when candidates cluster in 1-3 communities. Falls back to unconstrained walk on diverse queries (4+ communities). Benchmark adapter runs Louvain on index. | Benchmark pending | ~~P2~~ |

## Edge Type Expansion

30 edge types shipped. See [Edge Types Reference](architecture/edge-types.md) and [CHANGELOG](CHANGELOG.md) for full details.

| Category | Items | Status |
|----------|-------|--------|
| **Test coverage** | `tests` (test function to function under test) | **Shipped (P1).** |
| **Ownership** | `owned_by` (CODEOWNERS), `authored_by` (git blame) | **Shipped (P1).** |
| **Documentation** | `documents` (doc comment to symbol) | **Shipped (P2).** |
| **API contracts** | `consumes_endpoint` (HTTP client call), `implements_rpc` / `consumes_rpc` (gRPC) | **Shipped (P2).** |
| **Feature flags** | `gated_by_flag` (function gated by flag check) | **Shipped (P2).** |
| **Deployment** | `deployed_by` (service deployed by CI workflow), `tested_by` (package tested by CI) | **Shipped (P2).** |
| Runtime | `runtime_queries`, `runtime_connects_to` | P2 |
| Configuration | `configures` (config key to symbol that reads it) | P2 |
| Agent workflow | `suggested_for_task` / `used_by_agent` | P3 |

## Observability Ingestion

Beyond OTLP traces (shipped), these observability signals map to graph edges. The pattern: any system that records "X talked to Y" at runtime becomes a `runtime_*` edge. Static analysis says what CAN happen. Runtime signals say what DID happen. The diff is where findings live.

| Signal Source | Edge Types | What It Enables | Priority |
|---|---|---|---|
| Database query logs (pg_stat_statements, slow query log) | `queries_table`, `writes_table`, `reads_table` | "Change this table schema, what code breaks?" | P2 |
| HTTP access logs (nginx, ALB, API gateway) | `runtime_serves`, frequency metadata | Dead route detection without full APM | P2 |
| Message queue metrics (Kafka consumer lag, SQS depth) | `runtime_consumes`, `runtime_produces` | Verify static pub/sub edges against reality | P2 |
| Error tracking (Sentry, Bugsnag) | `runtime_throws`, error frequency | Prioritize blast radius by error-prone paths | P3 |
| ~~Feature flags (LaunchDarkly, Unleash)~~ | ~~`gated_by_flag`~~ | ~~"Disable this flag, what code becomes dead?"~~ | ~~P3~~ **Shipped (static extractor).** |
| ~~CI/CD pipeline (GitHub Actions, Jenkins)~~ | ~~`tested_by`, `deployed_by`~~ | ~~Test coverage as graph edges, deployment topology~~ | ~~P3~~ **Shipped (static extractor).** |
| ~~Git blame/log~~ | ~~`authored_by`, `recently_changed`~~ | ~~Ownership routing, change frequency for ranking~~ | ~~P3~~ **Shipped (P1, authorship extractor).** |
| Container orchestration (K8s events) | `runs_on`, `colocated_with` | Infrastructure topology in the graph | P4 |
| Service mesh (Envoy, Istio, Consul) | `runtime_connects_to` | Compare declared vs actual service topology | P4 |
| Continuous profiling (pprof) | `hot_path`, duration metadata | Weight blast radius by performance impact | P4 |

**Key insight:** Static edge with no runtime observation = dead code candidate. Runtime observation with no static edge = undocumented dependency. Both agree = high-confidence relationship.

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Community-aware retrieval | Constrain RWR walk to seed communities |
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Phase 4: Remaining Items

| Feature | Status |
|---------|--------|
| Merkleized feedback validity (expires when neighborhood_root changes) | **Shipped (v0.5.0).** Feedback records store the SubgraphRoot of the symbol's package. When querying, only feedback matching the current SubgraphRoot is counted, so feedback automatically expires when code changes. Adds 11% overhead (255µs → 284µs for 100 symbols). Migration 014. |
| Merkle proofs and audit primitives | **Shipped.** `knowing prove` (72µs), `knowing verify` (1.2µs), `knowing prove-absent`, `knowing audit` for compliance reports. |
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees; triggered at ~1M+ edges) | Planned |
| File-level roots (finer single-file invalidation) | **Deferred.** Package-level granularity is sufficient at current and projected scale (200K+ edges). Scoped FTS rebuild handles the primary use case. Revisit only if a user demonstrates single-file invalidation need. This locks the tree depth at 3 levels and clears the extraction stability gate. |

## Cross-Repo Validation

### Tier 1: Synthetic Multi-Repo Fixture (built)

3 Go modules at `test/cross-repo/`. Cross-repo edge resolution verified. Remaining dogfooding tests:

- `knowing prove` across repos
- `knowing prove-absent` across repos
- `knowing audit` across repos
- `knowing export` to knowing-viz with cross-repo edges
- `blast_radius` on module-a function showing callers in B and C
- Incremental invalidation across repos

### Tier 1.5: Java Monolith + Frontend (cross-language validation)

**Target:** Spring PetClinic (Java REST API) + React/Vue frontend consuming it.

**What it validates:**
- **Cross-language HTTP edges**: TypeScript `fetch()` → Java `@GetMapping` resolution
- **Java extractor correctness**: Spring Boot annotations, layered architecture (Controller → Service → Repository)
- **API contract detection**: Which frontend components consume which backend endpoints
- **Runtime vs static comparison**: Spin up service, generate OTLP traces, compare observed vs extracted edges
- **Full-stack test scope**: Change Java service → knowing surfaces which frontend tests to run
- **Dead endpoint detection**: REST endpoints defined but never called (static or runtime evidence)
- **Breaking change prevention**: "You're removing `/api/users` but 5 frontend components call it"

**Why useful:**
- Knowing is heavily validated on Go (dogfooding itself), less on Java/TypeScript
- REST API consumption edges aren't validated cross-language yet
- Enables full-stack test selection (backend change → frontend tests)
- Realistic monolith structure (50K LOC, deep call hierarchies, framework-heavy)

**Effort:** Low (4-8 hours to setup, index, validate)  
**Priority:** After session memory persistence (Priority #2). Useful once we have real users requesting Java/cross-language support.

### Tier 2: Grafana Ecosystem (scale validation)

Grafana + Loki + Tempo + Mimir (~1.3M LOC, 4 repos). Validates cross-repo at realistic scale. Run manually, not in CI.

## Production Scale: Permanent Runtime Record

The endgame: knowing with continuous OTLP trace ingestion alongside static analysis. After a year:

- Static edges: ~150K (stable)
- Runtime edges: millions (every observed call path)
- Snapshot chain: 365+ daily snapshots

### Git-Inspired Optimizations

Derived from a deep dive into git's C implementation (pack-objects, commit-graph, refs, bitmaps, merge-ort, shallow clones).

**Quick wins (< 1 day each):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| ~~Generation numbers on snapshots~~ | ~~commit-graph generation_number~~ | ~~O(1) ancestry checks ("is snapshot A ancestor of B?"), prune chain walks~~ **Shipped.** Migration 015, `Snapshot.Generation` field. |
| ~~Auto-GC with threshold~~ | ~~gc_auto_threshold=6700~~ | ~~Trigger GC when deleted edges exceed threshold; prevents unbounded edge_events growth~~ **Shipped.** Threshold 5000 edge_events, keeps 10 snapshots. |

**Medium (1-3 days):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| Filter-based graph materialization | list-objects-filter.c | Push predicates into SQL queries; context retrieval skips irrelevant subgraphs (2-5x speedup) |
| Persistent named snapshot refs | refs/packed-backend.c | `knowing tag stable`, `knowing diff stable..latest`; stored in snapshot_refs table |
| Bloom filters for package changes | commit-graph bloom filters | Per-snapshot bloom filter of changed packages; eliminates edge_events scan during diff |
| Snapshot-graph acceleration file | commit-graph binary format | Binary file with fanout+hashes+metadata avoids N SQL queries for chain walking |
| String interning for package paths | merge-ort strmap | Pointer equality for hot-path comparisons; reduce allocation pressure |

**Architectural (3-5 days):**

| Capability | Git Pattern | Why |
|-----------|-------------|-----|
| EWAH edge-reachability bitmaps | pack-bitmap.c | One bit per edge per snapshot; Diff = XOR + popcount instead of O(E) scan; blast_radius via precomputed reachability |
| XOR-compressed bitmap chains | stored_bitmap.xor | Store consecutive snapshot bitmaps as XOR deltas; 100 snapshots in <10KB vs 125KB |
| Delta-compressed snapshot packs | diff-delta.c, Rabin fingerprint | Sliding-window delta over edge groups; 40-60% smaller sync payloads |
| Promisor nodes (lazy cross-repo) | shallow.c promisor semantics | Record cross-repo edge targets as "promisor" nodes; fetch full data on-demand from source DB |
| Three-way graph merge | merge-ort.c staged computation | Federated sync with conflict awareness: confidence_conflict, provenance_conflict, type_conflict |

### What's Needed at Scale

| Capability | Why |
|-----------|-----|
| Lazy materialization | Load only visited subtrees at millions of edges |
| Merkle bisection | O(log N) snapshot search instead of O(N) |
| Parallel tree hashing | Concurrent bottom-up hash computation for 1M+ edge trees. Current `computeMerkleRoot` is single-threaded; goroutine pool pattern for leaf-level parallelism. |
| Partitioned storage | Static and runtime edges have different lifecycles |
| Runtime edge compaction | Collapse observation history |
| Federated sync | CI instance + production instance exchange diffs |
| Drift alerts | Static analysis vs production traffic divergence |
| Dashboard | Real-time runtime graph visualization |
| Automated compliance reports | Scheduled `knowing audit` with diff against prior |

### Commercial Angle

| Offering | Revenue model |
|----------|--------------|
| knowing Cloud | Managed hosting, per-service pricing |
| Compliance reporting | Automated quarterly audit reports with proofs |
| Federated sync service | Org-wide intelligence sharing |
| Drift detection | Alerts on static/runtime divergence |
| Enterprise dashboard | Cross-repo visualization, team analytics |

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Proposed graph overlay (staging area) | P2 | Medium |
| Delta-compressed snapshots | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (enrichment conflict resolution) | P4 | Low |
| Transfer protocol (federated sync) | P4 | High |
| Replace/grafts (edge correction) | P4 | Medium |
