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
# Full cross-system benchmark (167 tasks, 10 repos, ~80 min with embeddings)
BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Single repo (fast iteration, no embeddings)
BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 10m

# With embeddings on single repo (~7 min)
BENCH_EMBEDDINGS=1 BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m

# Diagnostic env vars (compose freely, no reindex needed):
BENCH_EXCLUDE_EDGES=similar_to,type_hint_of   # exclude edge types from RWR walk
BENCH_BFS_DEPTH=2                             # limit walk depth (default 4)
BENCH_PREFER_TYPE_SEEDS=1                     # force type-seed preference
BENCH_HUB_DAMPEN=50                           # penalize nodes with in-degree >50
BENCH_RERANK_WEIGHT=0.5                       # blend original + embedding scores
BENCH_COHERENCE_BONUS=0.3                     # file-based packing coherence
BENCH_MAX_SEEDS=25                            # override max seed count
BENCH_ADAPTIVE_SEEDS=1                        # enable adaptive seed count
```

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

Competitive ratios to recalculate from new P@10:
- vs codegraph: P@10 / 0.135
- vs codebase-memory: P@10 / 0.137
- vs GitNexus: P@10 / 0.075
- vs Gortex: P@10 / 0.063
- vs grep: P@10 / 0.013

## Key Architecture

- `internal/context/` — retrieval pipeline (RWR, HITS, RRF, density-adaptive seeding, concept thesaurus, embedding re-ranker)
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
- `bench/cross-system/` — competitive benchmark (167 tasks, 10 repos, 7 competitors)
- `cmd/knowing/audit_supply_chain.go` — supply chain CLI (package-level verdict)

## Current State (session 16, 2026-05-26)

- **P@10 = 0.242** (167 tasks, 10 repos, 6 languages, 38 edge types)
- **Density-adaptive:** PreferTypeSeeds >40K nodes, adaptive seed count >10K nodes
- **Embedding re-ranker:** +17% P@10, pure re-rank (weight=0.0), vector cache 220ms
- **Competitive:** 1.79x codegraph, 1.77x codebase-memory, 3.23x GitNexus, 3.84x Gortex, 18.6x grep
- **Supply chain:** 1.0% FP on 200 clean packages (package-level verdict)
- **Identity:** "self-adapting code intelligence engine that gets smarter with scale"

## Key Findings (inform all future retrieval work)

1. **P@10 is reachability-determined.** 32-config parameter sweep + seed count sweep (10-50) proved zero variance. Only new edges or new seed sources move the metric. Don't tune weights.
2. **Dense graph dilution is a seed selection problem.** Edge exclusion, BFS depth, hub dampening all tested neutral. Fix: density-adaptive seed selection (PreferTypeSeeds + adaptive seed count).
3. **Embedding architecture > model.** Three models neutral as Channel 3 (independent search). Same models +17% as re-ranker. The integration point matters, not the model.
4. **Enrichment hurts retrieval.** LSP enrichment adds correct edges but dilutes RWR. Useful for audit, harmful for retrieval.
5. **42% of Django tasks score zero.** Vocabulary gaps: ground truth symbols share no keywords with task. No parameter tuning fixes this. Need new candidate sources.
6. **Gap injection concept is sound but BM25 is too noisy.** Embedding-filtered BM25 gap candidates: Django +3.2% but aggregate neutral. Need higher-precision candidate source.
7. **Coherence packing, bidirectional inheritance: both harmful.** Greedy density packing is near-optimal. Reverse inherits edges add noise without reachability.

## Experiment Summary (41 total across sessions 8-16)

### What works
| Experiment | Impact | Session |
|-----------|--------|---------|
| Inheritance propagation | +29% | 13 |
| Embedding re-ranker (pure, weight=0.0) | +17% | 15 |
| Adaptive seed count (>40K: 25, >10K: 20) | Django +14.2%, corpus +1.7% | 16 |
| PreferTypeSeeds (>40K nodes) | VS Code +44% | 14 |
| Docstring FTS indexing | +12.2% | 13 |
| Equivalence classes (115 concepts) | +8pp hard tier | 14 |
| Vector cache (SQLite) | 660ms -> 220ms | 16 |

### What doesn't work
Embeddings as Channel 3, blended re-rank, call-chain seeding, hub dampening, BFS depth reduction, framework thesaurus, coherence packing, bidirectional inheritance, raw BM25 gap injection, seed count tuning (10-50), gap parameter sweep (15 configs). All neutral or harmful. See docs/roadmap.md for details.

## Repos

- `blackwell-systems/knowing` — OSS engine (MIT, public)
- `blackwell-systems/knowing-supply-scan` — GHA action (MIT, public, v1.0.0)
- `blackwell-systems/platform` — API server (private, scaffold)

## Next Priorities

1. **Retrieval improvements**: density-adaptive alpha, FTS channel balancing, better gap candidate source. See docs/roadmap.md "Retrieval Improvements" for the full list of 11 queued experiments.
2. **Deploy platform API** (DigitalOcean Droplet + Cloudflare Tunnel)
3. **Publish benchmark paper** on Zenodo (docs/research/whitepapers/code-context-retrieval-benchmark.md)
4. **Publish supply chain blog post**
5. **v0.11.0 release**
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
| Embedding eval log | `bench/cross-system/EMBEDDING-EVAL.md` |
| Dense graph analysis | `docs/research/dense-graph-dilution-analysis.md` |
| Roadmap + experiment log | `docs/roadmap.md` |
| FP eval results (200 packages) | `bench/supply-chain/false-positive-results-v2.jsonl` |
| Blog (benchmark) | `/Users/dayna.blackwell/code/blog/content/posts/ai-code-context-tools-benchmark.md` |
| Blog (supply chain) | `/Users/dayna.blackwell/code/blog/content/posts/supply-chain-detection-without-executing-code.md` |

## Common Pitfalls

- **Persistent pack cache masks experiments.** `DisablePersistentCache()` is required in benchmark adapter or results are stale. The notes-table cache returns previous run's output.
- **BENCH_EMBEDDINGS=1 required for re-ranker.** Without it, embeddings don't load and P@10 stays at ~0.207 (no re-ranker). With it, P@10=0.242.
- **Django has 36 tasks in bench, not 33.** The fixture count varies by how the harness discovers them. Don't worry about task count mismatches between runs.
- **Kubernetes P@10 varies +-0.05 between runs.** High variance from 19-22 task subset loading and embedding non-determinism. Don't chase k8s fluctuations.
- **`knowing index` runs LSP enrichment by default.** Use `-no-enrich` when you only need tree-sitter edges (supply chain scanning, quick benchmarks). Saves ~14s per package.
- **The knowing binary on PATH may be stale.** After code changes, rebuild with `GOWORK=off go build -o /tmp/knowing-test ./cmd/knowing/` for testing. Or `go install`.
- **`command npm` not `npm`.** nvm shell hook interferes. Always use `command npm`.

## Conventions

- Always use `GOWORK=off` (go.work references shelfctl which may not be present)
- Run benchmark before AND after shipping any retrieval/engine changes
- Do NOT use em dashes in prose or documentation
- Use `command npm` to bypass nvm shell hook
- Check CI: `gh run list --limit 5`
- Commit messages: conventional commits (feat:, fix:, docs:)
- Do not commit CLAUDE.md to git (it's in .gitignore)
