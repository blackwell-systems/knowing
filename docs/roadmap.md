# Roadmap

What's shipped is in the [changelog](CHANGELOG.md). This document covers what's next.

## Current State (v0.12.0, 2026-05-30)

**P@10 = 0.283 cold start, 0.284 with compounding** (277 tasks, 14 repos, 8 languages, nomic-embed-text-v1.5 model). 38 edge types. 23 extractors. 164 equivalence classes. 58 experiments across 14 sessions.
2.10x codegraph, 2.07x codebase-memory, 3.77x GitNexus, 4.49x Gortex, 21.8x grep.
Embedding re-ranker: disabled (3 models tested, all net negative, architecture closed). Gap-fill seeds (embedding-based): +11.2%. Equivalence classes (C#, FastAPI, Terraform, Rust): +4%. Task memory compounding: +4.9%. Ruby enrichment (ruby-lsp): Jekyll #1 in corpus at 0.370. Parallel benchmark: 5.5 min (was 80 min sequential) via PreloadVectors.
**Structural ceiling reached (session 20):** 55 experiments confirm P@10 is reachability-determined. Incremental path exhausted. Next-generation approaches needed (see below).

## Immediate Priorities

| # | Item | Why | Effort | Expected Impact |
|---|------|-----|--------|-----------------|
| 2 | **Deploy platform API** | api.blackwell-systems.com. DigitalOcean Droplet, Cloudflare Tunnel, bare metal. DEPLOY.md + deploy.sh committed. Server provisioned (142.93.52.251). Go building on 1GB Droplet is slow; upgrade to 2GB or cross-compile. | In progress | Live product |
| 3 | **AI-generated evaluation corpus** | LLM generates tasks + ground truth, DB-validated before use: all symbols must exist in nodes table, >= 3 symbols, span >= 2 files. Auto-difficulty from graph properties. Run 1000, keep ~600. Weekly CI. Hybrid: hand-curated for regression, AI-generated for statistical coverage. | Medium | Eval credibility |
| 4 | **Blog post** | Numbers are publishable: 14 repos, 8 languages, 277 tasks. LinkedIn audience is warm (11K views on mcp-assert). | 2 hours | Visibility |
| 5 | **Add more corpus repos** | Candidates: celery (Python, 80K LOC), Spring Boot (Java/Kotlin). Homebrew blocked on Ruby LSP (22a). Target: 16+ repos, 300 tasks. | 2 hours each | Corpus credibility |
| 6 | **Supply chain whitepaper** | False positive evaluation done (1.0% on 200 packages). Draft has TanStack + event-stream case studies. | Medium | Publication |
| 8 | **GHA Marketplace action** | Package supply chain scanner for paid distribution. Free tier for public repos. | Medium | Commercial |

### Tested Neutral or Harmful (sessions 14-19)

| Approach | Result | Session | Details |
|----------|--------|---------|---------|
| **Embeddings as Channel 3** (seed source) | Neutral | 15 | Three models find same symbols as BM25. Architecture was wrong. |
| **Blended re-rank** (weight > 0.0) | Harmful | 15 | Pure re-rank (weight=0.0) wins P@10/R@10. Blending preserves MRR but sacrifices recall. |
| **Call-chain seeding** | Neutral | 14 | Callees already reachable via RWR traversal; diffuses probability mass |
| **Hub dampening** | Neutral | 14 | No effect on VS Code (0.095 unchanged at any threshold) |
| **BFS depth reduction** | Neutral | 14 | No effect (depth 2/3/4 all produce same P@10) |
| **Expanded framework thesaurus** ("backend"->"base") | Harmful | 14 | Too noisy for BM25 |
| **accesses_field for P@10** | Neutral | 15 | Fields already reachable via call edges. Adds graph completeness, not retrieval. |
| ~~LSP enrichment for P@10~~ | **Revised: strongly positive** | 13, 17 | Session 13 found neutral (tested confidence upgrades only). Session 17: Python enrichment +0.040 P@10 (django+flask). Go enrichment: k8s 0.000 -> 0.159 (192K new edges, 169K phantom nodes). Enrichment creates phantom external nodes; type_hint_of edges connect functions to those nodes. **Moved to Tested Positive table.** |
| **Coherence-aware packing** (CoherenceBonus=0.3) | Harmful (-1.8%) | 16 | Greedy density packing already near-optimal. File-based coherence adds noise. |
| **Bidirectional inheritance edges** | Harmful (-2.5%) | 16 | Reverse inherits add noise without new reachability. Django zeros are vocabulary gaps. |
| **BM25 gap injection (no embedding filter)** | Harmful (-1.4%) | 16 | Raw BM25 candidates too noisy. Displaces good graph results. |
| **Seed count sweep** (10-50 on Django) | Neutral | 16 | 10 and 15 and 25 seeds all produce P@10=0.222-0.228. Confirms parameter irrelevance. |
| **Gap injection parameter sweep** (15 configs) | Neutral | 16 | Threshold 0.1-0.5, maxgap 3-10 all produce P@10=0.225-0.228 on Django. Parameters don't matter. |
| **Density-adaptive RWR alpha** (0.15 on dense) | Neutral | 17 | Alpha=0.15 on dense repos (flask 5.9, cargo 13.5, kafka 12.5): P@10 0.280 vs baseline 0.278. Within run variance. |
| **Density-adaptive inherits weight** (1.0 on deep) | Neutral | 17 | Boosted implements/overrides/extends to 1.0 on repos >1.5% inherits edges. Django +0.009, kafka+flask -0.008. Net neutral. |
| **Interface type hint propagation** (post-processing) | Neutral | 17 | Connect type_hint_of targets to sibling implementors. Edge structure mismatch: type_hint_of and implements share 0 target hashes on Java/Python. Go (k8s): 393 edges on 523K, P@10 neutral. Needs extractor-level fix. |
| **Disconnection rate adaptive seeding** | Neutral | 20 | Measured disconnection rate (% zero-inbound nodes) across 12 repos: 0.2% (kafka) to 22.7% (caddy). Added seed bonus proportional to rate. Only flask/spark affected (+2 seeds). Django 0.261 (baseline 0.256), flask 0.337 (0.347), spark 0.260 (0.255). All within variance. Redundant with node count thresholds; confirms seed quantity doesn't move P@10. |
| **Porter stemming in FTS5** | Neutral (-0.003) | 20 | Added `porter` tokenizer to FTS5 so "validates" matches "Validator". Django +0.006, cargo -0.009. Full corpus 0.264 vs 0.267 baseline. Stemming expands BM25 recall but brings in noisy seeds that dilute RWR on dense graphs. |
| **Django framework equiv classes** | Harmful (-0.011) | 20 | Two versions tested. V1 (broad phrases): +0.008 (variance). V2 (targeted phrases matching task descriptions): -0.011. Equiv class targets duplicated symbols BM25 already finds, adding noise. Equiv classes only help when targets are NOT findable by BM25. |
| **SCIP ingestion for Rust** | Rejected | 20 | rust-analyzer SCIP on cargo: 124K edges, all connecting project code to external types (stdlib, serde, dependencies). Zero project-internal edges that tree-sitter didn't already find. Unfiltered: P@10 = 0.150 (-0.127). Filtered (project-only): +0 edges, identical to baseline. SCIP's value is cross-crate resolution, but those targets are always external. Macro-expanded edges (derive Serialize) create impl edges TO external types, not between project symbols. Dead end for P@10. |
| **Graph pruning / ghost edges** | Neutral | 20 | Three configs on cargo: (1) exclude similar_to: 0.245 (-0.014, reachability lost). (2) exclude references: 0.268 (+0.009, noise removed). (3) ghost references at 0.05 weight: 0.264 (+0.005). Full corpus ghost: 0.264 (-0.003). Density-adaptive ghost (threshold 5.0): 0.264 (-0.003). Per-repo wins cancel losses. Pruning/ghosting is edge weight tuning, which 57 experiments confirm doesn't move aggregate P@10. |

### Re-test Candidates (post-enrichment graph structure change)

Go enrichment fundamentally changed graph structure on k8s (268K -> 705K edges, 72K -> 242K nodes, 169K phantom nodes) and terraform (similar). Three previously-neutral experiments were tested on pre-enrichment graphs and rejected for graph-structural reasons. With the new graph density, the structural premises that led to their rejection may no longer hold.

| Approach | Original Result | Why it might flip | Priority |
|----------|----------------|-------------------|----------|
| **Coherence-aware packing** | Harmful -1.8% (session 16) | Tested on sparse graphs where most symbols clustered in same files. 192K new cross-package reference edges create meaningful cross-package neighbors. Coherence bonus rewards packing graph-neighbors; more real neighbors = less noise. | Medium |
| **BFS depth reduction** | Neutral (session 14) | 705K edges on k8s means RWR covers far more ground per step. Depth 4 may diffuse probability into phantom nodes. Shorter depth could keep the walk focused on real code. | Medium |

**Not re-testing:** Bidirectional inheritance (directionality problem), blended re-rank (architecture), embeddings as Channel 3 (vocabulary/fusion), framework thesaurus (BM25 noise), seed count/gap parameter sweeps (parameter irrelevance confirmed). These were rejected for reasons unrelated to graph structure.

## Enrichment Performance

gopls on-demand package loading dominates enrichment time on large Go repos. The two-phase warmup (didOpen + retry) solved the "zero upgrades" problem. Both Go repos are now fully enriched:

- **Terraform**: 82K new edges discovered, 73K phantom nodes, 12 min total
- **Kubernetes**: 192K new edges discovered, 169K phantom nodes, 58 min gopls (root module only). Sub-modules (30 staging packages) are intentionally excluded from indexing: staging code is dependency code that dilutes RWR (-20% P@10 when included). Multi-module enrichment infrastructure works but has nothing to enrich since staging files aren't indexed.

The persistent daemon (#3) is the real fix for repeat runs; everything else works around the cold start.

| # | Item | What it does | Expected Impact | Effort |
|---|------|-------------|-----------------|--------|
| 1 | **Per-package gopls for single-module repos** | Spawn one gopls per top-level package directory, each loads only its subtree. Already implemented for go.work repos (multi-module enrichment). Extend to single-module repos by synthetically partitioning. | 3-5x faster on large repos (parallel init, each instance loads fewer packages) | Medium |
| 2 | **Lazy/streaming LSP requests** | Fire LSP requests immediately without waiting for gopls to fully initialize. gopls queues and answers as packages load. Early requests may timeout (10s per-symbol limit), later ones succeed. Currently the enricher blocks on the first response, which waits for full init. | Eliminates init wait; trades some skipped symbols for 5-10 min wall clock savings | Low |
| 3 | **Persistent gopls daemon (`-remote` mode)** | Run gopls as a persistent background process that stays warm between enrichment runs. Second enrichment of the same repo is near-instant (workspace already loaded). | Near-zero init on repeat runs. Requires daemon lifecycle management. | Medium |
| 4 | **Incremental enrichment via CLI** | Expose `RunScoped(changedFiles)` through `knowing enrich lsp --files <list>`. Only enrich symbols in changed files. Already implemented in the enricher (used by daemon mode), but the CLI always runs full enrichment. | 10-100x faster for incremental changes (enrich 5 files vs 2,000) | Low |
| 5 | **Parallel git blame** | `git blame` runs per-file sequentially (~40% of index time on large repos). Parallelize across files since blame is read-only. Or: batch blame using `git log --follow` for recent authorship. | 2-4x faster authorship extraction | Low |
| 6 | **Node.js heap size for tsserver** | Set `NODE_OPTIONS="--max-old-space-size=8192"` when spawning tsserver. Default heap (~4GB) causes GC thrashing on large TypeScript repos (vscode: 34 min enrichment, majority in GC). More heap = less GC = faster enrichment. | 2-3x faster TS enrichment on large repos | Low |
| 7 | **Deno LSP for TypeScript** | Use `deno lsp` (Rust-based) instead of tsserver for TypeScript enrichment. No GC, no Node.js heap limits. Add as alternative in enrichment config detection (check for `deno` on PATH, prefer over tsserver). Test on vscode to compare enrichment time and edge quality. | Potentially 5-10x faster TS enrichment | Low |
| 8 | **Import-based phantom nodes for Go (skip gopls)** | Parse Go import statements and generate phantom stub nodes for stdlib/dependency types without running gopls. Now that gopls enrichment works (k8s: +0.159 P@10), the value proposition changed: this is a fast fallback for environments without gopls, not the primary path. gopls discovers 192K edges + 169K phantoms on k8s; import parsing would get only the phantoms. | Fast fallback for Go enrichment without gopls | Low (deprioritized) |

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
| kubernetes (3.5M LOC) | 4,877 | 705K (268K ast + 287K lsp + 150K other) | 18.6s extraction + 58 min enrichment | ~22s queryable (enrichment async) |

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
| **Cross-repo context_for_task** | Search across ALL indexed repos simultaneously, not just one. Real projects span multiple repos (monorepo patterns, microservices). Merge results from all repos into one ranked list. See "Cross-Repo Query Architecture" section below. | P2 |
| **Incremental context ("next page")** | After an agent gets initial context, allow requesting the NEXT N symbols not yet seen. Avoids re-querying with bigger budget and getting duplicates. Session-stateful cursor. | P2 |
| **Staleness annotations on MCP responses** | When returning context, annotate symbols whose source files changed since last index. Agents know which results might be outdated without calling `knowing stale` separately. | P2 |
| **CLI `--format gcf` output** | `knowing context` only supports json/xml/markdown. Adding gcf/gcb for direct agent consumption without MCP. | P3 |
| `knowing daemon install-service` | Generate launchd plist (macOS) or systemd user unit (Linux). | P3 |
| Per-repo config (`.knowing.yaml`) | Excludes, local overrides, workspace membership. | P3 |

## Diagnostic Tools (for dense-graph investigation)

These tools are needed to investigate and resolve the dense-graph dilution problem
(VS Code 87K nodes, P@10 drops from 0.163 to 0.084 with correct extraction).
See `docs/research/dense-graph-dilution-analysis.md` for full investigation plan.

| # | Tool | What it enables | Effort |
|---|------|----------------|--------|
| 1 | **Query-time edge exclusion** | `BENCH_EXCLUDE_EDGES=similar_to` filters edges during RWR without reindexing. Enables rapid hypothesis testing (test each edge type's contribution in seconds, not minutes). Add type filter to adjacency map loading. | Low (5 lines) |
| 2 | **Hub analysis tool** | Reports top-N nodes by in-degree for a given DB. Identifies probability sinks that absorb RWR mass on dense graphs. Answers: "which nodes accumulate walk probability regardless of query?" | Low (30 lines) |
| 3 | **RWR score distribution tool** | For a given task, reports score distribution (min, max, median, p90, gap between rank-1 and rank-50). Diagnoses whether the walk is diffusing (flat distribution) or focused (steep dropoff). | Low (20 lines) |
| 4 | **Top-10 comparison tool** | For a given task, shows top-10 results from two different DBs (or configs) side-by-side. Answers: "which new nodes pushed correct results out of the top 10?" | Medium (50 lines) |

## Benchmarking Roadmap

14 benchmark harnesses exist today (see `bench/README.md`). The following gaps remain for a complete competitive evaluation story.

### P1: Would convince someone to adopt knowing

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **SWE-bench integration** | knowing + Claude solves N% more SWE-bench tasks than Claude alone. The definitive "does graph context help real agent work?" | Not started | High (full eval harness, 300 tasks, automated agent loop) |
| **Real-session replay** | Replay 10+ real claudewatch session transcripts. Measure: context calls saved, symbols used that came from knowing, tasks where knowing provided the critical symbol. | Not started (implicit feedback tracker now exists for attribution) | High (transcript parser, attribution detection, manual annotation) |

### P2: Proves production readiness

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Query latency p50/p95/p99** | Instrument all 28 MCP tool handlers. Report latency distribution per tool across 1000 calls. | Single number (2ms cached) exists; need per-tool distribution | Medium |

### P3: Completeness and rigor

| Benchmark | What it proves | Status | Effort |
|-----------|---------------|--------|--------|
| **Ruby benchmark repo (Rails)** | Adds 7th language to corpus. Rails mirrors Django: heavy framework conventions, deep class hierarchy, method_missing magic. Tests whether retrieval improvements generalize to Ruby. Candidates: Rails (large, MVC), Devise (auth, focused), Sidekiq (jobs, moderate). Requires 10-20 task fixtures with ground truth symbols. | Not started | Medium (fixture curation is the bottleneck) |
| **Multi-language extraction coverage** | For each of the 24 extractors: number of node types extracted, edge types produced, lines of test coverage. Comparison vs Sourcegraph SCIP, GitNexus, tree-sitter-graph. | Not started | Low (automated count + table) |
| **Grafana scale validation** | Full retrieval quality measurement on 714K-edge production graph (not just latency). P@10 with Grafana-specific task fixtures. | Latency test exists (`grafana_scale_test.go`); no retrieval quality measurement | Medium |
| **Graph integrity under load** | Spawn 10 concurrent indexers on overlapping repos. Run `knowing fsck` after. Proves content-addressing prevents corruption under concurrency. | Not started (fsck bench exists for single-indexer correctness) | Medium |
| **Concurrent query performance** | 100 parallel `context_for_task` calls on a 100K-edge graph. Measure throughput (queries/sec), latency degradation, and WAL checkpoint behavior. | Not started | Medium |
| **Cross-repo retrieval quality** | P@10 for tasks that span repo boundaries (e.g., "which frontend components call this backend endpoint?"). | Needs cross-repo implementation first | Medium |



### Standalone Publication: Code Retrieval Evaluation Toolkit (CRET)

Extract knowing's benchmarking infrastructure as the SWE-bench equivalent for code
context retrieval. Full proposal: [docs/proposals/code-retrieval-eval-toolkit.md](../proposals/code-retrieval-eval-toolkit.md).

**Status:** Not started. Prerequisite complete (Aider comparison done, Run 19-22).

### Release Infrastructure

| Item | Description | Status |
|------|-------------|--------|
| **Corpus DB tarball in releases** | Attach `corpus-dbs-vX.Y.Z.tar.gz` to each GitHub release as a separate asset (not bundled with binaries). Contains all 12 pre-built benchmark DBs with enrichment + pre-embedded vectors (1.6GB). Enables instant corpus restore via `make corpus-restore TARBALL=...` instead of 30+ min rebuild. DBs are gitignored and can't be recovered from git; losing them means hours of re-indexing + re-enrichment + re-embedding. | **HIGH PRIORITY** |
| **Corpus DB integrity check** | CI job that runs `knowing fsck` on each corpus DB after release to verify no corruption. | Not started |

### Not yet benchmarked (tracked for completeness)

- **Proof verification throughput**: N proofs/sec verified (currently 1.2µs each = ~800K/sec theoretical)
- **Snapshot chain walk cost**: O(chain_length) for history queries
- **FTS5 rebuild cost vs graph size**: scaling curve for the deferred FTS rebuild
- **Language-specific P@10 breakdown**: already have per-repo numbers; need per-language aggregate

## Retrieval Pipeline

Current results: see [bench/cross-system/FINDINGS.md](../bench/cross-system/FINDINGS.md).
P@10=0.283 cold (277 tasks, 14 repos, 8 languages). 2.10x vs codegraph, 3.77x vs GitNexus, 4.49x vs Gortex, 21.8x vs grep. Query latency 2ms on k8s (with adjacency cache). Embedding gap-fill adds 220ms (cached vectors). Focused seed selection + cluster-aware gap-fill: +6.0% over previous high. Equivalence classes: +4%.

**Key findings:** (1) 32-config parameter sweep proved P@10 is reachability-determined; ranking parameters are irrelevant. (2) Embedding re-ranker was initially measured at +17% but session 19 per-repo A/B test showed it was net negative (9/13 repos hurt). The +17% was from gap-fill seeds sharing the BENCH_EMBEDDINGS flag. Re-ranker disabled; gap-fill seeds remain (+11%).

### Retrieval Improvements

| # | Item | Why | Status |
|---|------|-----|--------|
| 7 | ~~Bidirectional inheritance edges~~ | Tested session 16: Django -2.5%, Flask -1.5%. Reverse inherits edges add noise without new reachability. Django's 42% zero-rate is vocabulary gaps, not connectivity gaps. | **Rejected** |
| 9 | ~~Density-adaptive RWR alpha~~ | Tested session 17: alpha=0.15 on dense repos (flask 5.9, cargo 13.5, kafka 12.5). P@10 0.280 vs baseline 0.278 (+0.002, within run variance). Neutral. Confirms parameter tuning doesn't move the metric. | **Rejected** |
| 9 | ~~Density-adaptive inherits weight~~ | Tested session 17: boosted implements/overrides/extends to 1.0 on repos with >1.5% inherits edges. Django +0.009, kafka+flask -0.008. Net neutral. | **Rejected** |
| 10 | **Adaptive seed count by structural richness** | % of type nodes with contains edges indicates how productive type seeds are. High % (>60%): fewer seeds needed (types reach methods). Low %: more seeds needed to compensate. | To test |
| 11 | **Community count adaptive walk** | Many small communities: community-scoped RWR is effective. Few large communities: unconstrained walk is better. Threshold currently hardcoded; should adapt to detected modularity. | Experiment |
| 12 | **FTS hit rate channel balancing** | Adaptive RRF weights per query based on channel result counts. Likely neutral: parameter sweep (32 configs) and entry point channel (session 19) both confirmed P@10 is reachability-determined, not ranking-determined. RRF weight changes reshuffle seed order but don't change what's reachable. | Low priority (likely neutral) |
| 13 | ~~Disconnection rate adaptive seeding~~ | Tested session 20: measured disconnection rate across 12 repos (0.2% kafka to 22.7% caddy). No repo exceeds 30%. Implemented as seed bonus: `int(rate * 20)` extra seeds. Only flask and spark see any change (+2 seeds, 15->17) because all other repos already exceed the bonus via node count thresholds. Results: django 0.261 (baseline 0.256), flask 0.337 (baseline 0.347), spark 0.260 (baseline 0.255). All within variance. **Redundant with existing node count thresholds.** Seed count doesn't move P@10 (confirmed by prior 32-config sweep). | **Neutral** |
| 14 | ~~Hub node dampening (H1)~~ | Re-tested session 17 on enriched graphs (BENCH_HUB_DAMPEN=50). P@10 = 0.219 vs 0.220 baseline. Still neutral. Edge weights already handle high-degree nodes. | **Rejected** |
| 15 | ~~Entry point seed channel~~ | Tested session 19: route_handler/service nodes as Channel 6 in RRF (weight 1.5x, keyword-filtered, cached). Django +10% without embeddings (0.250 -> 0.275), but **neutral on full corpus with embeddings** (0.264 vs 0.266 baseline). Embedding re-ranker already captures what entry point seeding provides. Route handlers have phantom targets (handles_route -> external), limiting RWR reachability from entry points. | **Neutral** |
| 16 | **More equivalence concepts** | Only add when a specific task fixture exposes a gap. Must respect Run 22 constraint (no single-word phrases, no generic targets). | On-demand |
| 16b | **Rust equivalence classes** | Cargo at 0.216 with rust-analyzer enrichment. Zero Rust-specific equiv classes. Macro vocabulary gap: task says "serialize", ground truth is `Serialize::serialize`. Candidates: serde (serialize/deserialize/from_str), tokio (async/spawn/runtime), error handling (thiserror/anyhow/Result/From), derive traits (Clone/Debug/Default), web (axum/Router/handler/extract). 10-15 classes. Also: SCIP ingestion (`rust-analyzer scip .`) would capture proc-macro expanded code that tree-sitter misses. | On-demand |
| 17a | ~~Gap-fill threshold tuning~~ | Tested < 3, < 8, < 10 vs baseline < 5. All within variance (+-0.005). Threshold doesn't matter: tasks with 0-4 and 0-9 candidates are largely the same set. **Neutral.** | Rejected |
| 17b | **Graduated gap-fill weight** | Binary activation (on/off at threshold 5) could be graduated: lower weight (0.5) when BM25 found 3-4 seeds, full weight only when BM25 found 0-1. Proportional intervention instead of binary. | Experiment |
| 17c | ~~Embedding re-ranker with code-tuned model~~ | Session 20: tested jina-embeddings-v2-base-code (code-tuned) as re-ranker. Django P@10 = 0.258 (vs 0.261 no re-rank, 0.256 no embeddings). Round 2: 0.253 (vs 0.267 baseline). Code-tuned model does not fix the re-ranker architecture problem. The issue is not model quality; cosine similarity cannot capture structural relevance regardless of training data. Three models tested across sessions 15-20 (nomic-text, jina-code, bge-small), all neutral or harmful as re-rankers. **Re-ranker architecture is closed.** | **Rejected** |
| 19a | **Parallel benchmark P@10 variance** | `BENCH_PARALLEL=1` has +-0.009 P@10 variance vs sequential (0.264 stable). **SHIPPED:** (4) PreloadVectors: eager vector cache at init, round 1 25 min -> 5.3 min. (1) Shared ONNX Embedder: single session, less memory. **Tested, didn't help:** semaphore (4 concurrent repos): 0.238, worse than unbounded. Shared embedder alone: 0.255, didn't reduce variance. **Root cause:** non-deterministic goroutine scheduling affects RWR walk convergence, not I/O or ONNX contention. Per-task scores differ between sequential and parallel on identical inputs (cargo-easy-001: 0.40 seq vs 0.00 parallel). **Remaining ideas:** (5) Serialize HNSW index to file. (6) `PRAGMA mmap_size`. (2) Pre-compute query embeddings for benchmark-only. None address the root cause. Sequential remains official scoring mode; parallel is for iteration speed only. | Investigated, open |
| 20 | **sqlite-vec integration** | Replace brute-force cosine with sqlite-vec ANN for persistent search. Current brute-force from SQLite works but scales O(n). sqlite-vec would give O(log n) queries. Pure Go option: `viant/sqlite-vec`. | Infrastructure (not urgent: brute-force is fast enough for current corpus sizes) |
| 22 | **More corpus repos** | Every enriched repo at 0.200+ lifts the aggregate. Candidates: celery (Python, 80K LOC), Spring Boot (Java/Kotlin). Target: 16+ repos, 300 tasks. | Corpus expansion |
| 22a | **Homebrew corpus repo (blocked)** | 278K LOC Ruby, 8,476 nodes, density 15.2. Tree-sitter P@10 = 0.275 (no embeddings). 20 fixtures written. **Blocked on Ruby LSP enrichment.** Investigated extensively (session 19): (1) ruby-lsp's composed bundle uses `bundle exec` which fails when project has `BUNDLE_DISABLE_SHARED_GEMS`/`BUNDLE_PATH` in `.bundle/config` (Homebrew-specific). Even with gem in Gemfile + lockfile + vendor/bundle, bundler 4.0 can't find the executable. (2) `BUNDLE_GEMFILE=""` bypasses bundler but ruby-lsp produces zero semantic edges (syntax only). (3) solargraph too slow (9+ min on 23K LOC Jekyll, timeout on 278K LOC Homebrew). (4) `.bundle/config` rename: ruby-lsp caches composed bundle state. **Root cause:** ruby-lsp requires functioning bundler context for semantic resolution, Homebrew's bundler config is incompatible. **Unblock path:** try on a Ruby repo without custom bundler config (Discourse, Sidekiq), or wait for ruby-lsp `--use-launcher` flag to mature. | Blocked |
| 23 | **Fixture quality review** | Manual review of 60 agent-created fixtures (caddy, ocelot, fastapi). Agent ground truth may include technically correct but practically unhelpful symbols. Tuning fixture quality is higher ROI than code changes. A wrong ground truth symbol penalizes the system unfairly. Will be partially obsoleted by AI-generated evaluation corpus (#5 in Immediate Priorities). | Quality |
| 18 | **Feedback parameter sweep (warm-start)** | Session boost (0.20), task memory formula (0.5+score*0.4), decay (7-day linear), top-N (5) are untuned. Only affects real-user compounding. | When users exist |
### Continuous Adaptation (moat, not P@10)

The adaptive infrastructure is knowing's core differentiator. Competitors use fixed strategies. knowing observes its own graph and adjusts retrieval automatically. Seven mechanisms ship today (PreferTypeSeeds, adaptive seed count, equiv classes, gap-fill, task memory, Merkleized feedback, LSP phantom nodes).

**Honest assessment (session 20):** All five items below are parameter optimization in different flavors. 51 experiments across sessions 8-20 have proven that P@10 is reachability-determined: only new edges or new seed sources move the metric. Seed count sweeps (32 configs), gap threshold sweeps (15 configs), edge weight sweeps, and disconnection-rate seeding all produced zero variance. These items are valuable for product differentiation ("self-adapting") and user experience on diverse codebases, but they will not move P@10 on the benchmark.

| # | Item | What adapts | Priority |
|---|------|-------------|----------|
| 30 | **Graph topology features for seed strategy** | Disconnection rate, path length, clustering coefficient shape walk strategy. Partially tested (#48: disconnection rate alone was redundant with node count). | Moat (won't move P@10) |
| 27 | **Per-query confidence estimation** | Estimate seed quality pre-RWR, adjust gap-fill aggressiveness. But gap threshold sweep (15 configs) was neutral. | Moat (won't move P@10) |
| 26 | **Continuous density-proportional seeding** | Smooth function replacing threshold steps. But seed count sweep (10-50) was zero variance. | Moat (won't move P@10) |
| 28 | **Learned edge weights from ground truth** | Train optimal RWR weights from 277-task corpus. But 32-config parameter sweep was zero variance. | Moat (won't move P@10) |
| 29 | **Feedback-driven per-repo thresholds** | System discovers own parameters from task memory. Parameters don't matter, but UX improves. | Moat (requires users) |
| 25 | ~~Co-change edges from git history~~ | Tested session 20: full redesign with proper concurrency (writeMu, atomic stats, producer-consumer). Deepened all 12 corpus clones to 200+ commits. Three configs tested: (1) min=1 cap=50: Django +0.013, k8s +0.042, but cargo -0.066. Full corpus 0.263 (-0.004). (2) min=1 cap=5: cargo -0.018. (3) min=2 cap=5: cargo -0.004, full corpus 0.267 (exactly baseline). Per-repo wins and losses cancel out. Bulk refactor commits create O(n^2) noisy pairs that dilute RWR on dense graphs; filtering the noise also filters the signal. | **Neutral** |

### Next-generation retrieval (beyond incremental experiments)

55 experiments across sessions 8-20 exhausted the incremental path (adding edges, tuning parameters, swapping models). Session 21 broke through the 0.267 ceiling with focused seed selection (#36): cluster seeds by package path and concentrate the walk in the dominant structural neighborhood. Combined with cluster-aware gap-fill, P@10 = 0.283 (+6.0%). The insight: seed quality (structural cohesion) matters more than seed quantity. 57 experiments proved count doesn't matter, but cohesion was an untested dimension.

| # | Item | Approach | Why it might work |
|---|------|----------|-------------------|
| 31 | **Query-time LLM symbol prediction** | Ask an LLM to predict likely symbol names from the task description before retrieval. "In Django, a field validator would be `clean`, `BaseValidator.__call__`." Inject predictions as high-confidence seeds. | Solves the vocabulary gap with intelligence instead of string matching. The "find" half done by reasoning, not BM25. Trade-off: adds LLM latency and cost. Could be optional (local model or API). |
| 32 | ~~Per-repo graph pruning~~ | Tested session 20 (#56). Three configs on cargo: exclude similar_to (0.245, reachability lost), exclude references (0.268, +0.009), ghost references at 0.05 weight (0.264). Full corpus ghost edges: 0.264 (-0.003). Density-adaptive ghost (threshold 5.0): same. Per-repo wins cancel losses. | **Neutral** |
| 33 | **Two-phase retrieval (search-walk-search)** | Phase 1: current BM25+RWR finds a neighborhood (~500 nodes). Phase 2: run BM25 again within that neighborhood only, re-seeding with the most relevant matches. First walk finds the structural area; second search finds specific symbols within it. No ML required, uses existing infrastructure. Most practical next step. | The core problem: 15-25 seeds dilute the walk across the graph. Two-phase narrows the search space before the final ranking. Phase 1 answers "what area of the code?" Phase 2 answers "which specific symbols?" |
| 34 | **Ground truth expansion** | Current 277 tasks may have incomplete ground truth. If the system finds useful symbols that aren't in the ground truth, it's penalized unfairly. Systematic review: for each zero-scoring task, examine what the system actually returns and judge relevance independently. | Free P@10 if ground truth is wrong. Session 20 confirmed fixtures are valid (symbols exist, are connected), but relevance of returned symbols was not reviewed. The system might be returning contextually useful symbols that aren't in the curated ground truth. |
| 35 | **Query-conditioned walk** | Weight edges differently per query during RWR, not just by edge type. "Validate request body" amplifies edges toward validators, attenuates edges toward serializers. The walk becomes query-aware. Could use query keywords to boost edges whose target node names match, or train a lightweight model to predict per-query edge relevance. | The fundamental bottleneck: RWR walks blind from seeds. It doesn't know what it's looking for. On a graph with density 13.5, each step splits probability 13 ways. Query-conditioned edges focus the walk toward the answer. |
| 36 | ~~Focused seed selection~~ | **SHIPPED session 21 (#58).** Cluster RRF candidates by package path, promote largest cluster. Combined with cluster-aware gap-fill (embedding seeds filtered to dominant package). Full corpus: 0.283 vs 0.267 (+0.016, +6.0%). Django: 0.275 vs 0.253 (+8.7%). First experiment to break the session 20 ceiling. | **Shipped** |
| 37 | **Learned scoring from ground truth** | Train a lightweight model (logistic regression, small NN) on the 277-task corpus. Features: BM25 rank, node degree, path distance to nearest type node, edge type distribution, package depth. Predict: is this candidate ground truth? Even a simple model could outperform hand-tuned RankSymbols formula. | We have labeled data (277 tasks with ground truth) that we only use for evaluation, never for training. Cross-validation across repos prevents overfitting. Risk: overfitting. Mitigation: leave-one-repo-out validation. |
| 38 | **Per-query edge type selection** | For a "middleware" query, prefer calls/implements edges. For a "configuration" query, prefer configures/imports. Map query concepts to edge type weight profiles. Hand-curated profiles (like equiv classes) or learned from the corpus. | Different task types traverse different parts of the graph. A query about "error handling" should walk along throws/catches edges; a query about "routing" should walk along handles_route edges. Current RWR uses fixed weights for all queries. |

## Edge Type Expansion

38 edge types shipped. See [Edge Types Reference](architecture/edge-types.md) and [CHANGELOG](CHANGELOG.md) for full details. Recent additions: `accesses_field` (36th, 6 languages), `reads_env` (37th, supply chain), `executes_process` (38th, supply chain).

**Remaining failure analysis (sessions 13-14):**
- Django: 117/192 ground truth symbols unreachable. Root cause: framework base classes referenced by type hint and interface contract, not direct call.
- Kubernetes: 71/116 unreachable. Root cause: interface-heavy architecture where functions accept interfaces but ground truth is concrete implementations.
- Kafka: 50/93 unreachable. Root cause: consumer/producer patterns referenced via type parameters and configuration.

### P2: Structural edges

| Category | Items | Status |
|----------|-------|--------|
| Runtime | `runtime_queries`, `runtime_connects_to` | Planned |
| Configuration | `configures` (config key to symbol that reads it) | Planned |
| Agent workflow | `suggested_for_task` / `used_by_agent` | Planned |

## Observability Ingestion

Beyond OTLP traces (shipped), these observability signals map to graph edges. The pattern: any system that records "X talked to Y" at runtime becomes a `runtime_*` edge. Static analysis says what CAN happen. Runtime signals say what DID happen. The diff is where findings live.

| Signal Source | Edge Types | What It Enables | Priority |
|---|---|---|---|
| Database query logs (pg_stat_statements, slow query log) | `queries_table`, `writes_table`, `reads_table` | "Change this table schema, what code breaks?" | P2 |
| HTTP access logs (nginx, ALB, API gateway) | `runtime_serves`, frequency metadata | Dead route detection without full APM | P2 |
| Message queue metrics (Kafka consumer lag, SQS depth) | `runtime_consumes`, `runtime_produces` | Verify static pub/sub edges against reality | P2 |
| Error tracking (Sentry, Bugsnag) | `runtime_throws`, error frequency | Prioritize blast radius by error-prone paths | P3 |
| Container orchestration (K8s events) | `runs_on`, `colocated_with` | Infrastructure topology in the graph | P4 |
| Service mesh (Envoy, Istio, Consul) | `runtime_connects_to` | Compare declared vs actual service topology | P4 |
| Continuous profiling (pprof) | `hot_path`, duration metadata | Weight blast radius by performance impact | P4 |

**Key insight:** Static edge with no runtime observation = dead code candidate. Runtime observation with no static edge = undocumented dependency. Both agree = high-confidence relationship.

## Underexploited Capabilities

| Item | Next step |
|------|-----------|
| Edge event log | Temporal queries: "when did this dependency appear?" |
| Leiden algorithm | Add via community registry when a Go implementation exists |

## Phase 4: Remaining Items

| Feature | Status |
|---------|--------|
| Federated sync (exchange roots, transfer only differing branches) | Planned |
| Merkle-based bisection (binary search on snapshot chain) | Planned |
| Lazy materialization (load only visited subtrees; triggered at ~1M+ edges) | Planned |

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

## Git Design Audit (open items from docs/architecture/git-design-audit.md)

All CRITICAL and HIGH items shipped (session 12). Remaining are LOW priority.

| # | Item | Priority | Effort | Verdict |
|---|------|----------|--------|---------|
| 9.2 | `MaxOpenConns(1)` on SQLite | **Do now** | 1 line | Free perf. Single writer, no reason for connection pool. |
| 5.2 | Incremental snapshot computation | Do eventually | 3h | Real speedup on large repos. Compute snapshot from changed files only. |
| 7.1 | Named snapshot refs (`snapshot_refs` table) | Do eventually | 4h | Needed for `knowing tag v1.0` and diff-mode supply chain product. |
| 7.2 | Reflog table | Only if 7.1 ships | 2h | Audit trail for ref mutations. Pointless without named refs. |
| 5.1 | ReconstructEdgeSet from event log | **Skip** | 1 week | Over-engineering. SQLite has the full edge table. Nobody replays events. |
| 2.3 | Edge observation column split | **Skip** | 1 day | Premature optimization. No repo has hit row-size bottleneck. |
| 10.1 | Merkle-diff sync protocol | **Not yet** | 2 weeks | Zero users need multi-machine sync. Build when someone asks. |
| 10.2 | `knowing export` / `knowing import` | Maybe | 1 week | Useful for platform API. But `cp knowing.db` works today. |

## Git-Inspired (Not Yet Built)

| Item | Priority | Effort |
|------|----------|--------|
| Proposed graph overlay (staging area) | P2 | Medium |
| Delta-compressed snapshots | P3 | High |
| N-way hierarchical diff | P3 | Medium |
| Rerere (enrichment conflict resolution) | P4 | Low |
| Transfer protocol (federated sync) | P4 | High |
| Replace/grafts (edge correction) | P4 | Medium |
