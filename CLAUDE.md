# knowing

Self-adapting code intelligence engine. Single Go binary, zero runtime deps.
Gets smarter with scale, not dumber: observes its own graph density and adjusts
retrieval strategy automatically. 38 edge types, 23 extractors, 28 MCP tools.

## Build & Test

```bash
GOWORK=off go build ./...           # build (GOWORK=off required: go.work refs missing module)
GOWORK=off go test ./internal/...   # unit tests
GOWORK=off go test ./cmd/...        # CLI tests
GOWORK=off go test ./bench/...      # benchmark harnesses (some need pre-indexed repos)
```

## Benchmark (P@10 evaluation)

```bash
# Full corpus, sequential (official numbers, ~20 min with pre-embedded vectors)
BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Full corpus, parallel (iteration mode, ~5 min, P@10 ~0.022 lower due to ONNX CPU contention)
BENCH_PARALLEL=1 BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Single repo (fast iteration, no embeddings)
BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 10m

# With embeddings on single repo (~2 min with pre-embedded vectors)
BENCH_EMBEDDINGS=1 BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m

# Pre-embed all nodes (one-time, ~2 hours for full corpus, skips phantoms)
knowing enrich embeddings -db <repo>/.knowing/graph.db

# Diagnostic env vars (compose freely, no reindex needed):
BENCH_EXCLUDE_EDGES=similar_to,type_hint_of   # exclude edge types from RWR walk
BENCH_BFS_DEPTH=2                             # limit walk depth (default 4)
BENCH_PREFER_TYPE_SEEDS=1                     # force type-seed preference
BENCH_HUB_DAMPEN=50                           # penalize nodes with in-degree >50
BENCH_RERANK_WEIGHT=0.5                       # blend original + embedding scores
BENCH_COHERENCE_BONUS=0.3                     # file-based packing coherence
BENCH_MAX_SEEDS=25                            # override max seed count
BENCH_ADAPTIVE_SEEDS=1                        # enable adaptive seed count
BENCH_GAP_THRESHOLD=5                         # gap-fill activation threshold (default 5)
BENCH_PARALLEL=1                              # parallel repo execution (fast, ~0.022 P@10 lower)
```

## Testing Methodology

Django is the acid test repo for retrieval experiments:
- 33 tasks (largest single-repo fixture set)
- 42% zero-rate problem (vocabulary gaps), so improvements that move Django are structural
- Where adaptive seeds showed +14.2%, bidirectional inheritance showed -2.5%, gap injection +3.2%

**Protocol:**
1. **Django only, no embeddings (~30s):** quick signal on structural changes
   ```bash
   BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 10m
   ```
2. **Django with embeddings (~7min):** confirms interaction with re-ranker
   ```bash
   BENCH_EMBEDDINGS=1 BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m
   ```
3. **Full corpus with embeddings (~80min):** only if Django moves positively
   ```bash
   BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0
   ```

If Django is neutral or negative, don't run the full corpus. If Django is positive,
the full corpus confirms whether it generalizes or gets absorbed by run variance.

**Important:** Not all experiments affect Django. Check graph density first:
```bash
sqlite3 <repo>/.knowing/graph.db "SELECT COUNT(*) FROM edges; SELECT COUNT(*) FROM nodes;"
```
Density = edges/nodes. Current densities: cargo 13.5, kafka 12.5, terraform 9.5,
ocelot 8.3, spark-java 7.7, kubernetes 6.2, flask 5.9, vscode 4.7, django 2.8.
If the experiment only affects dense graphs (like adaptive alpha), test on dense repos
(flask, cargo, kafka) instead of Django.

**Output capture:** Always capture full output to a file (`2>&1 | tee /tmp/file.log`
or `> /tmp/file.log 2>&1`). Never pipe through `tail` as it loses early output.

## When Benchmark Numbers Change

After any P@10 improvement, these files ALL need updating with the new aggregate,
per-repo breakdown, and competitive ratios. This is a standard procedure:

1. **bench/cross-system/FINDINGS.md** — executive summary, per-repo table, competitive advantages
2. **bench/CONTEXT-PACKING-STUDY.md** — Dimension 1, competitive summary, run history
3. **bench/README.md** — cross-system row in summary table
4. **docs/guide/introduction.md** — operational characteristics, measured performance table
5. **docs/architecture/retrieval-pipeline.md** — eval baseline line
6. **docs/architecture/system-overview.md** — benchmark section
7. **docs/architecture/design-principles.md** — benchmark results
8. **docs/architecture/context-engine.md** — current performance
9. **docs/architecture/embedding-reranker.md** — impact numbers
10. **docs/roadmap.md** — retrieval pipeline section
11. **docs/research/cross-system-benchmark.md** — key results table
12. **Blog post** (`/Users/dayna.blackwell/code/blog/content/posts/ai-code-context-tools-benchmark.md`) — headline table, per-repo, ratios
13. **npm/knowing/README.md** — package description on npmjs.com
14. **pypi/README.md** — package description on PyPI
15. **CLAUDE.md** — this file, Current State section
16. **README.md** — Numbers table

Competitive ratios (honest matching, session 21):
- vs codegraph: 0.184 / 0.087 = 2.11x
- vs GitNexus: 0.184 / 0.055 = 3.35x
- vs Gortex: 0.184 / 0.052 = 3.54x
- vs Aider: 0.184 / 0.023 = 8.0x
- vs grep: 0.184 / 0.015 = 12.3x
- codebase-memory: timed out (22/297 tasks in 60 min)
Note: old ratios used inflated P@10 from raw substring matching. See `docs/research/session-21-measurement-calibration.md`.

## Key Architecture

- `internal/context/` — retrieval pipeline (RWR, HITS, RRF, density-adaptive seeding, concept thesaurus, embedding gap-fill)
- `internal/context/walk.go` — RWR, adjacency map, PreferTypeSeeds, GraphNodeCount, ReRankOriginalWeight, CoherenceBonus, adaptive seed count
- `internal/context/context.go` — ForTask pipeline, VectorReRanker interface, reRankWithEmbeddings, packIntoBudget
- `internal/embedding/` — jina-code via hugot (pure Go ONNX), vector cache in SQLite, ReRankByHashes
- `internal/indexer/` — 23 extractors (tree-sitter), post-processing (inheritance, interface propagation, contains, similarity, co-tested, type-hint)
- `internal/store/` — SQLite backend (GraphStore interface, embeddings table migration 019)
- `internal/edgetype/` — 38 edge type constants
- `internal/diff/isolation.go` — supply chain isolation scoring (benign process targets, test exclusion, env-only attenuation)
- `internal/snapshot/` — hierarchical Merkle tree (via merkle-strata library)
- `internal/mcp/` — MCP server (28 tools, 8 resources)
- `internal/enrichment/` — LSP enrichment (multi-module gopls, per-symbol timeout, progress persistence)
- `bench/cross-system/` — competitive benchmark (277 tasks, 14 repos, 7 competitors)
- `cmd/knowing/audit_supply_chain.go` — supply chain CLI (package-level verdict)

## Current State (session 23, 2026-05-31)

- **P@10 = 0.206 cold start** (297 tasks, 15 repos, no task memory, no embeddings, honest measurement, confirmed 2 runs)
- **Embeddings: confirmed neutral.** Three runs: 0.176, 0.175, 0.176. Gap-fill seeds add nothing on cold start. Previous "gap-fill works" finding (session 17) was task memory contamination. Embedding infrastructure is dead weight for retrieval accuracy.
- **Task memory contamination (session 23):** all P@10 measurements from sessions 8-22 were inflated by accumulated task memory in corpus DBs. True cold-start is ~0.014 lower than reported. Within-session A/B deltas remain valid.
- **Measurement calibration (session 21):** old P@10 was 0.283 with inflated substring matching. Fixed with `dotBoundedContains()`. See `docs/research/session-21-measurement-calibration.md`.
- **Focused seed selection:** cluster seeds by package path, concentrate walk in dominant neighborhood (+6% relative)
- **Density-adaptive:** PreferTypeSeeds >40K nodes, adaptive seed count >10K nodes
- **LSP enrichment:** strongly positive. Go: k8s 0.000->0.232, terraform ~0.095->0.275. Python: +0.040
- **Competitive (cold, honest):** 2.37x codegraph, 3.75x GitNexus, 3.96x Gortex, 8.96x Aider, 13.7x grep. codebase-memory timed out.
- **Supply chain:** 1.0% FP on 200 clean packages (package-level verdict)
- **Identity:** "self-adapting code intelligence engine that gets smarter with scale"

## Key Findings (inform all future retrieval work)

1. **P@10 is reachability-determined.** 32-config parameter sweep + seed count sweep (10-50) proved zero variance. Only new edges or new seed sources move the metric. Don't tune weights.
2. **Dense graph dilution is a seed selection problem.** Edge exclusion, BFS depth, hub dampening all tested neutral. Fix: density-adaptive seed selection (PreferTypeSeeds + adaptive seed count).
3. **Embedding architecture: entirely neutral on cold start.** Session 23 confirmed with clean measurement (no task memory): P@10 = 0.176 with and without embeddings (3 runs). Previous "gap-fill works" was task memory contamination. Re-rank also net negative (session 19). The graph structure and BM25 carry everything.
4. **Enrichment is strongly positive for retrieval.** Session 13 found neutral (tested confidence upgrades only). Session 17 revised: Go enrichment k8s 0.000->0.232, terraform ~0.095->0.275. Python +0.040. Phantom nodes + type_hint_of edges create shared-type reachability paths. Neither works alone.
5. **42% of Django tasks score zero.** Vocabulary gaps: ground truth symbols share no keywords with task. No parameter tuning fixes this. Need new candidate sources.
6. **Gap injection concept is sound but BM25 is too noisy.** Embedding-filtered BM25 gap candidates: Django +3.2% but aggregate neutral. Need higher-precision candidate source.
7. **Coherence packing, bidirectional inheritance: both harmful.** Greedy density packing is near-optimal. Reverse inherits edges add noise without reachability.

## Experiment Summary (59 total across sessions 8-21)

### What works
| Experiment | Impact | Session |
|-----------|--------|---------|
| Focused seed selection + cluster-aware gap-fill | +6% relative P@10 (structural cohesion over quantity) | 21 |
| Inheritance propagation | +29% | 13 |
| ~~Embedding re-ranker~~ (pure, weight=0.0) | REVERTED: net negative (session 19, 9/13 repos hurt) | 15, 19 |
| Adaptive seed count (>40K: 25, >10K: 20) | Django +14.2%, corpus +1.7% | 16 |
| PreferTypeSeeds (>40K nodes) | VS Code +44% | 14 |
| Docstring FTS indexing | +12.2% | 13 |
| Task memory compounding | +11.5% P@10 round-over-round | 17 |
| Go enrichment (two-phase warmup) | k8s 0.000->0.232, terraform ~0.095->0.275 | 17 |
| Ruby enrichment (ruby-lsp) | Jekyll 0.325->0.370 (+14%) | 19 |
| C# equivalence classes (15 concepts) | Ocelot +51%, corpus +4% | 18 |
| Equivalence classes (115 concepts) | +8pp hard tier | 14 |
| Vector cache (SQLite) | 660ms -> 220ms | 16 |
| Dangling type_hint_of resolution | 3,836 edges fixed across 4 repos | 17 |

### What doesn't work
Embeddings as Channel 3, blended re-rank, call-chain seeding, hub dampening, BFS depth reduction, framework thesaurus, coherence packing, bidirectional inheritance, raw BM25 gap injection, seed count tuning (10-50), gap parameter sweep (15 configs), density-adaptive RWR alpha, density-adaptive inherits weight, interface type hint propagation (edge structure mismatch), GraphNodeCount excluding phantoms (phantoms are valid density signal), entry point seed channel (Django +10% without embeddings, neutral on full corpus with embeddings). All neutral or harmful. See docs/roadmap.md for details.

## Repos

- `blackwell-systems/knowing` — OSS engine (MIT, public)
- `blackwell-systems/knowing-supply-scan` — GHA action (MIT, public, v1.0.0)
- `blackwell-systems/platform` — API server (private, scaffold)

## Next Priorities

1. **Deploy platform API** (DigitalOcean Droplet + Cloudflare Tunnel). DEPLOY.md and deploy.sh ready.
2. **AI-generated evaluation corpus**: LLM generates tasks + ground truth, DB-validated. Hybrid: hand-curated for regression, AI-generated for statistical coverage.
3. **Blog post**: numbers are publishable, LinkedIn audience is warm.
4. **Add hugo to corpus**: Go web server, 75K LOC, enriched with gopls.
5. **Zig extractor**: tree-sitter-zig grammar exists, vendor parser.c. LinkedIn interest.
6. **Org conversion** (blackwell-systems user -> org for Marketplace)

## Documentation Map

| Topic | Location |
|-------|----------|
| Retrieval pipeline (full reference) | `docs/architecture/retrieval-pipeline.md` |
| Embedding re-ranker + vector cache | `docs/architecture/embedding-reranker.md` |
| Context engine (ForTask flow, scoring) | `docs/architecture/context-engine.md` |
| Data flow (commit to graph) | `docs/architecture/data-flow.md` |
| Supply chain detection | `docs/proposals/supply-chain-detection-demo.md` |
| Supply chain whitepaper | `docs/research/whitepapers/supply-chain-proof-of-absence.md` |
| Benchmark paper | `docs/research/whitepapers/code-context-retrieval-benchmark.md` |
| Benchmark results | `bench/cross-system/FINDINGS.md` |
| Benchmark methodology | `bench/cross-system/METHODOLOGY.md` |
| Corpus manifest (reproducibility) | `bench/cross-system/corpus/MANIFEST.yaml` |
| Corpus setup script | `bench/cross-system/corpus/corpus-setup.sh` |
| Embedding eval log | `bench/cross-system/EMBEDDING-EVAL.md` |
| Dense graph analysis | `docs/research/dense-graph-dilution-analysis.md` |
| Roadmap + experiment log | `docs/roadmap.md` |
| FP eval results (200 packages) | `bench/supply-chain/false-positive-results-v2.jsonl` |
| Blog (benchmark) | `/Users/dayna.blackwell/code/blog/content/posts/ai-code-context-tools-benchmark.md` |
| Blog (supply chain) | `/Users/dayna.blackwell/code/blog/content/posts/supply-chain-detection-without-executing-code.md` |

## Common Pitfalls

- **Persistent pack cache masks experiments.** `DisablePersistentCache()` is required in benchmark adapter or results are stale. The notes-table cache returns previous run's output.
- **BENCH_EMBEDDINGS=1 required for gap-fill.** Without it, embeddings don't load and gap-fill seeds are unavailable.
- **Django has 33 tasks in bench.** Consistently 33 fixtures on disk.
- **Kubernetes P@10 varies +-0.05 between runs.** High variance from 19-22 task subset loading and embedding non-determinism. Don't chase k8s fluctuations.
- **`knowing index` runs LSP enrichment by default.** Use `-no-enrich` when you only need tree-sitter edges (supply chain scanning, quick benchmarks). Saves ~14s per package.
- **The knowing binary on PATH may be stale.** After code changes, rebuild with `GOWORK=off go build -o /tmp/knowing-test ./cmd/knowing/` for testing. Or `go install`.
- **`command npm` not `npm`.** nvm shell hook interferes. Always use `command npm`.
- **Don't use `timeout` on long-running commands.** Let indexing, enrichment, and benchmarks run until they finish. Kill manually if they go too long. `timeout` causes premature kills on processes that are legitimately slow (gopls loading, tsserver type-checking, kafka authorship).
- **Never delete benchmark corpus DBs.** The DBs at `bench/cross-system/corpus/repos/<repo>/.knowing/graph.db` are gitignored and can't be restored from git. Enrichment status: Python (django, flask, fastapi) enriched with pyright. Java (spark-java, kafka) enriched with jdtls. TypeScript (vscode) enriched with tsserver. Go (terraform, kubernetes, caddy) enriched with gopls (two-phase warmup). Rust (cargo) enriched with rust-analyzer. C# (ocelot) enriched with csharp-ls. All 12 repos are enriched. If you need to test with modified indexing, copy the DB first.
- **DB experiment procedure.** Never modify corpus DBs in place. Master backups at `~/code/knowing-corpus-backup/` (15 repos, 5.6GB, all enriched). Copy from master to a working path, enrich the copy, checkpoint WAL (`PRAGMA wal_checkpoint(TRUNCATE)`), remove stale SHM/WAL files at destination (`rm -f *.db-shm *.db-wal`), then swap. Restore from master after. Stale SHM files cause "database disk image is malformed" even with a clean main file. See `bench/cross-system/METHODOLOGY.md` for full procedure.
- **`go clean -testcache` after code reverts.** After reverting code with `git checkout -- *.go`, `go test` may use a cached binary compiled from the pre-revert code. Always run `go clean -testcache` before the next benchmark. Session 20: cargo showed phantom regression (0.277 -> 0.150) from stale test binary.
- **Test the problem repo first.** When an experiment regresses a specific repo, test that repo in isolation before running full corpus. Saves 20+ minutes per iteration.
- **Experiment workflow.** (1) Copy master backup -> working copy. (2) Modify working copy. (3) Checkpoint WAL, delete SHM/WAL at destination. (4) Swap. (5) `go clean -testcache`. (6) Clear task memory. (7) Test problem repo first. (8) If positive, full corpus. (9) Restore from master after.
- **CRITICAL: Clear task memory before A/B benchmarks.** Task memory persists in corpus DBs across runs. Each benchmark run records (keywords -> symbols) that boost subsequent runs, creating phantom improvements. Clear with: `for db in bench/cross-system/corpus/repos/*/.knowing/graph.db; do sqlite3 "$db" "DELETE FROM task_memory;"; done`. Session 23 discovery: all previous measurements were contaminated by accumulated task memory (26K entries in terraform alone). True cold-start P@10 requires empty task_memory table.

## Debugging & Analysis Tools

### Retrieval debugging
- **`knowing debug-seeds -task "description" -db <path> <repo>`**: shows the full seed selection pipeline: keywords extracted, path terms, BM25 results, path boost annotations, final ForTask top 10 with scores. Use to diagnose why a task returns wrong symbols.
- **`knowing debug-fts -query "term1 OR term2" -db <path> [-limit N]`**: runs a raw FTS5 query against the node index. Use to test query formulations and see what BM25 returns for different term combinations. Supports prefix (`term*`), column targets (`symbol_name:"term"`), and OR/AND operators.
- **`knowing debug-walk -seed "SymbolName" -db <path> [-top N] [-alpha 0.2]`**: shows RWR walk from specific seed nodes: edge types on seeds (1-hop), nodes reached, score distribution, and top-N ranking. Use to diagnose why seeds don't propagate to expected targets.
- **`knowing bench-task -task <task-id> [-corpus path] [-budget N]`**: runs a single benchmark task and shows P@10 with per-symbol HIT/MISS analysis. Shows where each ground truth symbol ranks (or if it's missing entirely). Much faster than running the full benchmark for iterating on one task.
- **`go run ./bench/cross-system/cmd/failure-analysis --repo <name> [--task <id>]`**: categorizes P@10 misses into: same_package, related_name, test_symbol, noise. Shows ground truth vs returned for each task.
- **`go run ./bench/cross-system/cmd/validate-fixtures`**: verifies ground truth symbols exist in the graph DB.
- **`BENCH_DEBUG_ZEROS=1`**: env var for the benchmark. Logs ground truth + returned top 10 for every zero-scoring task.

### Enrichment debugging
- **`knowing enrich resolver [-db path] <repo>`**: runs in-process resolvers retroactively on an existing DB. Adds resolver_resolved edges without re-extracting.
- **`knowing enrich lsp [-db path] <repo>`**: runs external LSP enrichment standalone.
- Enricher logs: "X edges processed, Y upgraded, Z skipped, W errors" shows enrichment completeness.

### Benchmark tools
- **`BENCH_REPOS=django`**: filter to single repo for fast iteration.
- **`BENCH_ADAPTERS=knowing`**: skip competitors.
- **`BENCH_EMBEDDINGS=1`**: enable embedding gap-fill (slower but more accurate).
- **`BENCH_FOCUSED_SEEDS=0`**: disable focused seed selection (for A/B testing).

## Debugging Hung Processes

When a process appears stuck, use these tools to diagnose instead of guessing:

- **`sample <pid> 1`** (macOS): samples native call stacks for 1 second. Shows where threads are spending time (active CPU work vs idle waits). If all threads are in `pthread_cond_wait`, the process is blocked on I/O, not CPU-bound.
- **`kill -SIGQUIT <pid>`** (Go processes): dumps all goroutine stacks to stderr. Shows the exact function and line where each goroutine is blocked. Go's built-in equivalent of strace for goroutines. The process exits after the dump.
- **`ps aux | grep <name> | awk '{print "CPU:", $3"%", "TIME:", $10}'`**: quick check whether a process is actively working (high CPU) or idle (low CPU with high wall clock time).

Example workflow from session 17:
1. `sample` showed gopls threads idle in `pthread_cond_wait` while enricher also idle: both waiting for each other, not CPU-bound.
2. `SIGQUIT` goroutine dump showed 8 goroutines blocked in `sendRequest -> selectgo -> GetDefinition`: enricher sending requests gopls wasn't answering.
3. `sample` on tsserver showed `StringIndexOf` and `GarbageCollection`: actively type-checking, not stuck.

These tools turn "it's hanging" into "here's exactly where and why."

## Conventions

- Always use `GOWORK=off` (go.work references shelfctl which may not be present)
- Run benchmark before AND after shipping any retrieval/engine changes
- Do NOT use em dashes in prose or documentation
- Use `command npm` to bypass nvm shell hook
- Check CI: `gh run list --limit 5`
- Commit messages: conventional commits (feat:, fix:, docs:)
- Do not commit CLAUDE.md to git (it's in .gitignore)
