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
| 3 | **Feedback compounding** | `bench/feedback-loop/` | Does usage improve quality over time? |
| 4 | **Determinism** | `bench/merkle-diff/context_pack_test.go` | Same query + same graph = same output (PackRoot)? |
| 5 | **Scoped invalidation** | `bench/merkle-diff/` + `bench/community-detection/` | Can we skip recomputation for unchanged subgraphs? |
| 6 | **Scale** | `bench/cross-system/` (indexing perf) | Can we handle enterprise repos (3.5M LOC) in production time? |
| 7 | **Differential value** | `bench/context-relevance/` | Does each pipeline layer add measurable precision? |

## Cumulative Results (2026-05-23, Run 22)

### Dimension 1: Retrieval Precision

**Harness:** `bench/cross-system/` (117 manual fixtures, 7 repos, 5 languages)

| System | P@10 | R@10 | NDCG@10 | MRR |
|--------|------|------|---------|-----|
| knowing | 0.226 | 0.396 | 0.369 | 0.423 |
| Aider | 0.050 | - | - | - |
| grep | 0.020 | 0.035 | 0.037 | 0.072 |

**Verdict:** 11.3x precision advantage vs grep (p<0.0001, d=0.92, very large effect). 4.5x vs Aider. +60% cumulative from honest baseline.

Per-repo breakdown: Flask 0.336, Django 0.330, VS Code ~0.10, Kubernetes 0.184, Cargo 0.123. Optimization ceiling diagnosed: remaining ~77% miss rate requires feedback compounding (cold-start floor 0.226, compounded ceiling ~0.40).

### Dimension 2: Token Efficiency

**Harness:** `bench/token-savings/`

| Metric | knowing | Manual grep exploration |
|--------|---------|----------------------|
| Tokens consumed | 44.4% of baseline | 100% |
| Tool calls | 47.2% of baseline | 100% |
| Relevant symbols found | Same | Same |

**Verdict:** 55.6% fewer tokens, 52.8% fewer tool calls for equivalent coverage.

### Dimension 3: Feedback Compounding

**Harness:** `bench/feedback-loop/`

| Round | P@10 |
|-------|------|
| 0 (cold) | 16% |
| 1 | 36% |
| 5 | Converges (diminishing returns after round 3) |

**Verdict:** +20pp precision after one feedback round. Compounding effect proven.

**Note on session memory persistence:** The cross-system benchmark cannot demonstrate session memory improvement because each task is unique and runs once (no repeated queries). The feedback-loop bench independently proves +20pp compounding. Real-user value: quality compounds with usage; cold-start floor is 0.230, feedback-compounded ceiling is approximately 0.40.

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

| Repo | LOC | Files | Edges | Index Time |
|------|-----|-------|-------|-----------|
| kubernetes | 3.5M | 4,877 | 268,249 | 18.6s |
| VS Code | ~1M | 38,260 | 93,382 | 4.1s |
| Django | 400K | 2,937 | 185,393 | 3.3s |
| Cargo | 150K | 979 | 79,305 | 1.4s |
| Flask | 15K | 97 | 9,237 | 0.1s |
| **Total** | **~5.1M** | **47,150** | **635,566** | **~28s** |

**Verdict:** Enterprise-scale repos index in under 30s. Full 5-repo corpus in under 1 minute.

### Dimension 7: Differential Value

**Harness:** `bench/context-relevance/`

| Configuration | P@10 | Delta |
|---------------|------|-------|
| A: Keywords only | Baseline | - |
| B: Full engine (RWR + HITS + 5 seed tiers) | Baseline + score differentiation | Score distribution |
| C: Full engine + feedback | +9pp | Feedback is strongest single enhancement |

**Verdict:** Each layer contributes. Feedback is highest single value-add at current scale.

## Statistical Methodology

- **Pairwise comparison:** Wilcoxon signed-rank test (non-parametric, no normality assumption)
- **Effect size:** Cohen's d with bootstrap 95% CI
- **Significance threshold:** p < 0.05 (Bonferroni-corrected for multiple comparisons)
- **Ground truth:** Hand-curated fixtures (cross-system), independent Go import DAG (test-scope), go/ast resolution (edge-accuracy)
- **No circular validation:** Ground truth never derived from knowing's own output

## Known Limitations

1. **Absolute precision is 22.6%.** knowing beats grep 11.3x but ~77% of returned symbols still don't match ground truth. Root cause: graph connectivity exhausted on small repos; channel balance matters more than algorithm quality (Run 22). Remaining miss rate requires feedback compounding or semantic understanding. Cold-start floor 0.226, compounded ceiling ~0.40.

2. **Cold-start.** Feedback compounding (Dimension 3) requires usage. First-run precision is 22.6%, not 36%. Task memory now persists across restarts, but compounding requires repeated similar queries over time.

3. **Go bias.** Most benchmarks validated on Go code (knowing dogfoods itself). Cross-system benchmark partially addresses this with Python, TypeScript, Rust, Java, C# repos (7 repos total).

4. **Competitor coverage.** Benchmarked against Aider (4.5x less precise, file-level only), GitNexus (3x less precise, can't index enterprise repos), Gortex (2.3x less precise, 46x slower on k8s), and grep (baseline).

5. **Ground truth coverage.** 95% of ground truth symbols verified against DB (validate-fixtures tool). Remaining 5% are edge cases (external deps, inherited methods with name mismatches).

## Run History

| Run | Date | Change | P@10 | Notes |
|-----|------|--------|------|-------|
| 1 | 2026-05-21 | Baseline (3 repos indexed) | 0.102 | kubernetes + TypeScript empty |
| 2 | 2026-05-21 | + language equivalence classes | 0.102 | No change (FTS tokenization bottleneck) |
| 3 | 2026-05-21 | + ground truth achievability filter | 0.102 | Measurement fix, not retrieval fix |
| 4 | 2026-05-21 | CGO timeout fix (all 5 repos indexed) | - | Indexing only, no benchmark run |
| 5 | 2026-05-21 | Full run, all repos indexed | 0.149 | +46% from Run 1, d=0.53 |
| 6 | 2026-05-21 | FTS symbol_name column (migration 016) | ~0.166 | +11% from Run 5, d=0.62 |
| 7 | 2026-05-21 | Corrected ground truth (95% achievable) | 0.141 | Honest baseline, d=0.51 |
| 8 | 2026-05-21 | FTS fixed (was empty!) + tokenchars '_' | 0.147 | R@10 d=0.67, FTS now contributing |
| 9 | 2026-05-21 | + Python cross-file imports (63 edges) | 0.152 | Import resolution helps RWR walk |
| 10 | 2026-05-21 | + TS cross-file imports (5,684 edges) | 0.154 | 9.6x vs grep, RWR is primary differentiator |
| 11 | 2026-05-21 | + Rust cross-file imports (9,795 edges) | 0.155 | MRR +3.9% |
| 12 | 2026-05-21 | Test deprioritization + failure analysis | 0.155 | Diagnosed: RWR reach is the bottleneck |
| 13 | 2026-05-21 | **Inheritance propagation** | **0.200** | **+29%, d=0.81 (large), 12.5x vs grep** |
| 14 | 2026-05-21 | + Deeper call chains (Python) | **0.201** | +43% cumulative from baseline, d=0.78 |
| 15 | 2026-05-21 | + FTS concepts column (migration 017) + task memory boost fix | 0.203 | +44% cumulative, per-repo: Django 0.330, Flask 0.321, K8s 0.184, Cargo 0.123, TS 0.026 |
| 16 | 2026-05-21 | Round-2 memory compounding test | 0.203 | No improvement (graph density prerequisite) |
| 17 | 2026-05-21 | VS Code replaces TypeScript compiler | 0.226 | +60% cumulative, 11.3x vs grep, d=0.90 |
| 18 | 2026-05-21 | TS extractor extends_clause fix | **0.230** | **+63% cumulative, 11.5x vs grep, d=0.92** |
| 19 | 2026-05-23 | Java + C# corpus, Aider adapter | 0.185 | Fresh indexes (no enrichment), knowing 3.7x vs Aider |
| 20 | 2026-05-23 | Phantom external node fix | 0.185 | Spark Java 0.00->0.10 |
| 21 | 2026-05-23 | WIP debug (weighted RWR, seed cap) | 0.101 | Regression: equiv channel noise masked by aggregate |
| 22 | 2026-05-23 | **Equiv channel noise fix** | **0.226** | **+124% from regression, 4.5x vs Aider, channel balance** |

## Competitive Comparison Summary (Run 23)

| System | P@10 | Index k8s | Time-to-consistency | Token efficiency | RAM (k8s) |
|--------|------|-----------|---------------------|-----------------|-----------|
| **knowing** | **0.217** | **18.6s** | **167ms** | **48x vs Repomix** | **200MB** |
| codegraph (19K stars) | 0.133 | - | 805ms | - | - |
| Aider | 0.050 | N/A (file-level) | 3150ms (misses new symbols) | - | - |
| Gortex | ~0.10 | 14.2 min | minutes (no incremental) | - | 14GB |
| GitNexus | 0.076 | >60 min (killed) | minutes (full re-analyze) | - | 5.7GB |
| grep | 0.020 | instant | instant | - | - |

## Next Steps (priority order)

1. **Blog post / publication** (all competitive data collected, whitepaper updated)
2. **Channel balance regression test** (prevent Run 22 class of regression)
3. **LSP enrichment ROI measurement** (quantify enrichment vs fresh-index delta)
4. **Embedding model evaluation** (code-tuned model for semantic matching)
5. **codebase-memory-mcp adapter** (2.6K stars, BM25 + label boost, different category)


## Reproducing

```bash
# Index all 5 repos:
./bench/cross-system/scripts/clone-repos.sh
./bench/cross-system/scripts/index-repos.sh

# Run the full cross-system benchmark:
GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m

# Run individual dimension benchmarks:
GOWORK=off go test ./bench/feedback-loop/ -v -count=1
GOWORK=off go test ./bench/token-savings/ -v -count=1
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
GOWORK=off go test ./bench/context-relevance/ -v -count=1
GOWORK=off go test ./bench/community-detection/ -v -count=1
```
