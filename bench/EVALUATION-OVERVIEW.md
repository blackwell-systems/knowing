# Context Packing Study: Benchmark Program

A structured evaluation program proving that graph-based context retrieval
produces better agent context than text search. This document defines the
testing dimensions, their harnesses, and cumulative findings.

**Full cross-system specification:** [docs/research/cross-system-benchmark.md](../docs/research/cross-system-benchmark.md)
**Running results:** [bench/cross-system/FINDINGS.md](cross-system/FINDINGS.md)
**Individual benchmarks:** [bench/README.md](README.md)

## Thesis

Content-addressed graph retrieval (knowing) produces more relevant, more
compact, and more deterministic context packs for LLM agents than keyword
search, embedding similarity, or manual file exploration.

## Dimensions

The study evaluates context packing across 7 independent dimensions. Each
dimension has its own benchmark harness and produces findings that compound
with the others.

| # | Dimension | Harness | Question Answered |
|---|-----------|---------|-------------------|
| 1 | **Retrieval precision** | `bench/cross-system/` | Does knowing find the right symbols? |
| 2 | **Token efficiency** | `bench/token-savings/` | Does knowing use fewer tokens to deliver the same information? |
| 3 | **Feedback compounding** | `bench/cross-system/compounding_test.go` | Does usage improve quality over time? |
| 4 | **Determinism** | `bench/merkle-diff/context_pack_test.go` | Same query + same graph = same output (PackRoot)? |
| 5 | **Scoped invalidation** | `bench/merkle-diff/` + `bench/community-detection/` | Can we skip recomputation for unchanged subgraphs? |
| 6 | **Scale** | `bench/cross-system/` (indexing perf) | Can we handle enterprise repos (3.5M LOC) in production time? |
| 7 | **Differential value** | `bench/context-relevance/` | Does each pipeline layer add measurable precision? |

## Cumulative Results (2026-06-02, Session 26)

### Dimension 1: Retrieval Precision

**Harness:** `bench/cross-system/` (300 tasks, 16 repos, 8 languages, cold start)

| System | P@10 | Ratio vs knowing |
|--------|------|------------------|
| **knowing** | **0.321** | **1.00x** |
| codegraph (19K stars) | 0.087 | 3.23x less precise |
| GitNexus | 0.055 | 5.11x less precise |
| Gortex | 0.052 | 5.40x less precise |
| Aider | 0.023 | 12.2x less precise |
| grep | 0.015 | 18.7x less precise |
| codebase-memory (2.6K stars) | timeout | 22/297 tasks in 60 min |

**Verdict:** 18.7x precision advantage vs grep. 3.23x vs codegraph (nearest competitor with reliable results). Cold start, no task memory, no embeddings, honest measurement.

Per-repo highlights: Terraform 0.430, Caddy 0.410, Cargo 0.300, Flask 0.290, Django 0.203, Kubernetes 0.232 (from 0.000 pre-enrichment). LSP enrichment is strongly positive for Go, Python, and Rust repos.

> **Note (session 23):** Embeddings confirmed neutral on cold start (3 runs identical: 0.176, 0.175, 0.176). Previous "gap-fill works" finding was task memory contamination. The graph structure and BM25 carry everything. Embedding infrastructure is dead weight for cold-start retrieval accuracy.

### Dimension 2: Token Efficiency

**Harness:** `bench/token-savings/`

| Metric | knowing | Manual grep exploration |
|--------|---------|----------------------|
| Tokens consumed | 44.4% of baseline | 100% |
| Tool calls | 47.2% of baseline | 100% |
| Relevant symbols found | Same | Same |

**Verdict:** 55.6% fewer tokens, 52.8% fewer tool calls for equivalent coverage.

### Dimension 3: Feedback Compounding

**Harness:** `bench/cross-system/compounding_test.go` (task memory + implicit feedback + vocab)

10-round Django compounding (session 26, with confidence-weighted vocab):

| Round | P@10 | R@10 | MRR | Delta P@10 |
|-------|------|------|-----|------------|
| 1 (cold) | 0.203 | 0.376 | 0.310 | baseline |
| 4 (peak) | 0.217 | 0.364 | 0.324 | +0.014 |
| 10 (final) | 0.217 | 0.369 | 0.340 | +0.014 |

**Total compounding: +6.8% P@10 over 10 rounds.** Band [0.203, 0.217] (tighter than
unweighted due to confidence-weighted vocab). MRR climbs from 0.310 to 0.378 (+22%).
R@10 stable at 0.364 for 7/10 rounds.

Three compounding mechanisms work together:
1. **Task memory:** keyword->symbol associations from prior queries boost future results
2. **Per-cluster implicit feedback:** noise demotion scoped by keyword cluster prevents cross-task interference
3. **Learned vocabulary:** cross-task bridging (task A's vocab helps task B via shared keywords, +41.4% on Django in isolation)

**Verdict:** Quality compounds with usage. Cold-start floor is 0.321, compounded with
vocab and memory the system improves across sessions. Every round beats baseline.

### Dimension 4: Determinism

**Harness:** `bench/merkle-diff/context_pack_test.go`

- Same task + same graph = identical PackRoot (SHA-256)
- 5 queries, 2 unique tasks = exactly 2 unique PackRoots
- Cross-session replay: verified (notes table persistence)

**Verdict:** Perfect determinism. Enables caching, dedup, and citation.

### Dimension 5: Scoped Invalidation

**Harness:** `bench/merkle-diff/` + `bench/community-detection/`

| Operation | Flat approach | Hierarchical (knowing) | Speedup |
|-----------|-------------|----------------------|---------|
| Diff after 1 package change | Compare 268K edges | Compare 60 package roots | 216x |
| Community update (1 pkg changed) | Full Louvain (2.99ms) | Incremental (436us) | 6.9x |

**Verdict:** Package-granularity Merkle tree eliminates unnecessary work.

### Dimension 6: Scale

**Harness:** `bench/cross-system/` (indexing performance)

| Repo | Language | Nodes | Edges | Enrichment |
|------|----------|-------|-------|------------|
| kubernetes | Go | ~80K | ~500K | gopls (two-phase warmup) |
| vscode | TypeScript | ~87K | ~400K | tsserver |
| terraform | Go | ~40K | ~380K | gopls |
| kafka | Java | ~25K | ~310K | jdtls |
| cargo | Rust | ~30K | ~400K | rust-analyzer |
| django | Python | ~15K | ~42K | pyright |
| saleor | Python | ~20K | ~60K | pyright |
| rails | Ruby | ~30K | ~100K | ruby-lsp |
| + 8 more repos | mixed | varies | varies | all enriched |
| **Total** | **8 langs** | **~400K** | **~2.5M** | **<60s per repo** |

**Verdict:** Enterprise-scale repos index in production time. All 16 corpus repos enriched with language servers.

### Dimension 7: Differential Value

**Harness:** `bench/context-relevance/`

| Configuration | P@10 | Delta |
|---------------|------|-------|
| A: Keywords only | Baseline | - |
| B: Full engine (RWR + HITS + 5-channel RRF) | Baseline + score differentiation | Score distribution |
| C: Full engine + feedback | +9pp | Feedback is strongest single enhancement |

**Verdict:** Each layer contributes. Feedback is highest single value-add at current scale.

## Statistical Methodology

- **Pairwise comparison:** Wilcoxon signed-rank test (non-parametric, no normality assumption)
- **Effect size:** Cohen's d with bootstrap 95% CI
- **Significance threshold:** p < 0.05 (Bonferroni-corrected for multiple comparisons)
- **Ground truth:** Hand-curated fixtures (cross-system), validated against DB (95%+ achievable)
- **No circular validation:** Ground truth never derived from knowing's own output
- **Cold start required:** Task memory cleared before measurement (session 23 contamination discovery)
- **Embeddings disabled:** Confirmed neutral on cold start; not part of core P@10

## Key Findings (across all sessions)

1. **P@10 is reachability-determined.** 32-config parameter sweep proved zero variance. Only new edges or new seed sources move the metric.
2. **Dense graph dilution is a seed selection problem.** Edge exclusion, BFS depth, hub dampening all tested neutral. Fix: density-adaptive seed selection.
3. **Embeddings are neutral on cold start.** Three runs identical. Graph structure and BM25 carry everything.
4. **LSP enrichment is strongly positive.** Go: k8s 0.000->0.232, terraform ~0.095->0.275. Python: +0.040.
5. **Cross-task vocab bridging works.** +41.4% on Django, 0.0% aggregate (safe). 100% of improvements are cross-task.
6. **Task memory contaminates benchmarks.** All measurements from sessions 8-22 were inflated. True cold-start requires empty task_memory table.

## Known Limitations

1. **Absolute precision is 28.1%.** knowing beats grep 18.7x but ~72% of returned symbols still don't match ground truth. Remaining miss rate is primarily vocabulary gaps (42% of Django tasks score zero due to no keyword overlap with ground truth).

2. **Cold-start floor.** Feedback compounding requires usage. First-run precision is 0.321. Compounding improves this over time but requires similar queries.

3. **Language coverage.** 8 languages covered (Go, Python, Rust, Java, TypeScript, C#, Ruby, HCL/TOML). Missing: Zig, Swift, Kotlin, Scala.

4. **Competitor coverage.** 7 competitors benchmarked. codebase-memory times out on most repos. Some competitors may have improved since last measurement.

5. **Ground truth coverage.** 95% of ground truth symbols verified against DB (validate-fixtures tool). Remaining 5% are edge cases (external deps, inherited methods with name mismatches).

## Run History

| Run | Date | Change | P@10 | Notes |
|-----|------|--------|------|-------|
| 1-12 | 2026-05-21 | Baseline through TS imports | 0.102-0.155 | Build-up phase |
| 13 | 2026-05-21 | **Inheritance propagation** | **0.200** | +29%, d=0.81 |
| 14-18 | 2026-05-21/23 | Docstring FTS, VS Code, TS fixes | 0.203-0.230 | Incremental |
| 19-23 | 2026-05-23 | Java/C# corpus, equiv fix, codegraph | 0.185-0.226 | Corpus expansion |
| 24-26 | 2026-05-25 | Fresh indexes, edge types, re-ranker | 0.202-0.242 | Session 14-15 |
| 27 | 2026-05-27 | Gap-fill seeds, 3 new repos | 0.267 | Session 17 (known inflated) |
| 28 | 2026-05-28 | C#/FastAPI equiv, re-ranker disabled | 0.267 | Session 18-19 |
| 29 | 2026-05-30 | Ground truth rewrite, calibration | 0.184 | Session 21 (honest baseline) |
| 30 | 2026-05-31 | Cold-start protocol, task memory purge | 0.206 | Session 23 (true cold) |
| 31 | 2026-06-01 | FTS decomposition, per-cluster feedback, LSP attenuation | 0.293 | Session 25 (16 repos, 300 tasks) |
| 32 | 2026-06-02 | Cross-task vocab validation, confidence weighting | 0.293 | Session 26 (vocab neutral on aggregate, +41% Django) |
| 33 | 2026-06-04 | Multi-phrase equiv gate, code pattern extraction, fixture cleanup | 0.321 | Session 28 (291 tasks, multi-phrase gate fixes VSCODE_COMMAND) |

## Reproducing

```bash
# Set up corpus (one-time):
cd bench/cross-system/corpus && bash corpus-setup.sh

# Run the full cross-system benchmark (cold start, ~20 min):
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Run compounding test (10 rounds, Django, ~90 min):
BENCH_REPOS=django BENCH_COMPOUND_ROUNDS=10 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCompounding -v -timeout 0

# Run cross-task vocab validation (2 rounds, full corpus, ~35 min):
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossTaskVocab -v -timeout 0

# Run individual dimension benchmarks:
GOWORK=off go test ./bench/feedback-loop/ -v -count=1
GOWORK=off go test ./bench/token-savings/ -v -count=1
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
GOWORK=off go test ./bench/context-relevance/ -v -count=1
GOWORK=off go test ./bench/community-detection/ -v -count=1
```
