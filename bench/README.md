# Benchmarks

Benchmark harnesses that prove knowing's value with hard data. Each benchmark
is a standalone Go test package that runs measurements and produces findings.

**Evaluation Overview:** [EVALUATION-OVERVIEW.md](EVALUATION-OVERVIEW.md) (umbrella document tying all benchmarks into a coherent evaluation program)
**Cross-System Methodology:** [cross-system/METHODOLOGY.md](cross-system/METHODOLOGY.md) (metrics, fixture design, statistical methods, regression detection)
**Cross-System Specification:** [docs/research/cross-system-benchmark.md](../docs/research/cross-system-benchmark.md) (full methodology, 16 repos, fairness controls, ground truth protocol)

## Summary

| Benchmark | What it proves | Key result |
|-----------|---------------|------------|
| [cross-system](cross-system/) | Graph retrieval beats all competitors across languages and scales | 16 repos, 291 tasks, 7 competitors. P@10=0.321 vs codegraph 0.087 (3.69x) vs grep 0.015 (21.4x). Cold start, honest measurement. |
| [context-packing](context-packing/) | Density-ranked packing produces better context than naive strategies | Compares 4 strategies: density-ranked, top-K score, file-grouped, random |
| [feedback-loop](feedback-loop/) | Implicit feedback improves precision during sessions | Django +5.9% P@10 after 3 rounds. Per-cluster scoping prevents cross-task interference. |
| [context-relevance](context-relevance/) | Each engine layer adds measurable value | Feedback adds +9pp precision over baseline |
| [token-savings](token-savings/) | knowing reduces agent exploration cost | 55.6% fewer tokens, 52.8% fewer tool calls |
| [edge-accuracy](edge-accuracy/) | Two-tier extraction provides meaningful signal | 53.6% import confirmation, 26.7% overall |
| [test-scope-accuracy](test-scope-accuracy/) | Call-graph BFS predicts affected tests | 98.9% precision vs independent Go import DAG |
| [wire-format](wire-format/) | GCF is dramatically more token-efficient than JSON | 84% token savings, 74% byte savings |
| [merkle-diff](merkle-diff/) | Hierarchical Merkle tree enables scoped invalidation and determinism | 216x faster diff, 517x on 100K edges, perfect PackRoot dedup |
| [community-detection](community-detection/) | Incremental detection skips work the Merkle tree proves unchanged | Louvain 6.9x faster, LP 38.4x faster |
| [time-to-consistency](time-to-consistency/) | knowing reflects code changes faster than competitors | 167ms vs codegraph 805ms vs Aider 3150ms. Adjacency cache: 4,717x speedup. |
| [incremental-reindex](incremental-reindex/) | Incremental reindex is proportional to changed files | 26ms (3 files) vs 12.8s full (494x faster) |
| [supply-chain](supply-chain/) | Supply chain detection with low false positive rate | 1.0% FP on 200 clean packages (package-level verdict) |
| [agent-efficiency](agent-efficiency/) | When knowing helps and when it doesn't | Phase 2: k8s 99.9% noise elimination (10 ranked from 10,840 grep matches) |

## Cross-System Compounding Tests

The cross-system harness includes tests for learning mechanisms:

| Test | What it measures | Key result |
|------|-----------------|------------|
| `TestCompounding` | Task memory + feedback + vocab over N rounds | +6.8% P@10 over 10 rounds (Django), band [0.203, 0.217] |
| `TestCrossTaskVocab` | Cross-task vocabulary bridging | +41.4% Django, 0.0% aggregate (safe). 100% of improvements are cross-task. |

## Running

```bash
# Run cross-system benchmark (cold start, ~20 min):
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Run context packing benchmark:
BENCH_REPOS=django GOWORK=off go test ./bench/context-packing/ -v -timeout 30m

# Run compounding test (10 rounds, Django, ~90 min):
BENCH_REPOS=django BENCH_COMPOUND_ROUNDS=10 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCompounding -v -timeout 0

# Run cross-task vocab validation:
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossTaskVocab -v -timeout 0

# Run a specific benchmark:
GOWORK=off go test ./bench/feedback-loop/ -v -count=1

# Skip slow benchmarks in quick iteration:
GOWORK=off go test ./bench/... -short
```

## Design Principles

1. **Self-contained.** Each benchmark creates a temp database, indexes the repo,
   runs measurements, and cleans up. No external state or pre-existing database.

2. **Auto-generated findings.** Each test writes its own `FINDINGS.md` with
   current numbers. Run the test to refresh the report.

3. **Independent ground truth.** Benchmarks compare knowing's output against
   independent data sources (Go import graph, go/ast type resolution, manual
   ground truth fixtures) rather than circular self-validation.

4. **Honest interpretation.** FINDINGS.md documents what the data shows and
   what it does not. Limitations and caveats are stated explicitly.

5. **Cold start measurement.** Cross-system benchmarks require empty task memory.
   Session 23 discovered accumulated task memory inflating all prior measurements.
