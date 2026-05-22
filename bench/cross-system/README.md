# Cross-System Context Retrieval Benchmark

Rigorous comparison of context retrieval systems on identical tasks across 5 public repositories.

**Full specification:** [docs/research/cross-system-benchmark.md](../../docs/research/cross-system-benchmark.md)
**Running results:** [FINDINGS.md](FINDINGS.md)
**Study overview:** [bench/CONTEXT-PACKING-STUDY.md](../CONTEXT-PACKING-STUDY.md)

## Quick Start

```bash
# 1. Clone evaluation repos (shallow, ~2GB total)
./bench/cross-system/scripts/clone-repos.sh

# 2. Index with knowing
./bench/cross-system/scripts/index-repos.sh

# 3. Run benchmark (knowing + grep baseline by default)
GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m

# 4. Results appear in bench/cross-system/results/run-<timestamp>/
```

## Systems Compared

| System | Type | Requires |
|--------|------|----------|
| **knowing** | Content-addressed graph + RWR | Always available (it's us) |
| **grep** (baseline) | Raw ripgrep pattern search | `rg` on PATH |
| **GitNexus** | Knowledge graph MCP | `npm install -g gitnexus` |
| **Aider** | PageRank repo-map | `pip install aider-chat` |
| **CGC** | Graph DB (KuzuDB) MCP | `pip install codegraphcontext` |

Only installed systems are benchmarked. Missing systems are reported as "unavailable."

## Metrics

| Metric | What it measures |
|--------|-----------------|
| P@10 | Fraction of top-10 results that are relevant |
| R@10 | Fraction of ground truth found in top-10 |
| NDCG@10 | Ranking quality (rewards relevant results ranked higher) |
| MRR | How quickly you get the first relevant result |
| F1@10 | Harmonic mean of P@10 and R@10 |
| Token Eff | Relevant symbols per token consumed |

Statistical significance via Wilcoxon signed-rank test (paired, non-parametric).
Effect size via Cohen's d. Confidence intervals via bootstrap (10K resamples).

## Evaluation Corpus

5 repos pinned to specific versions:

- **kubernetes** (Go, ~3.5M LOC) - v1.30.0
- **TypeScript** (TypeScript, ~800K LOC) - v5.5.4
- **flask** (Python, ~15K LOC) - 3.1.0
- **cargo** (Rust, ~150K LOC) - 0.82.0
- **django** (Python, ~300K LOC) - 5.1

100 tasks across 3 difficulty tiers (easy/medium/hard) with hand-labeled ground truth symbols.

## Directory Structure

```
bench/cross-system/
  benchtype/           # shared types (leaf package, no internal imports)
  normalize/           # symbol canonicalization + tests
  metrics/             # P@K, R@K, NDCG, MRR, F1, token efficiency, stats
  adapters/            # system implementations + availability registry
  corpus/
    repos.yaml         # pinned repo definitions
    tasks/<repo>/<tier>/*.yaml  # ground truth fixtures
    repos/             # cloned repos (gitignored)
  scripts/
    clone-repos.sh     # shallow clone all 5 repos
    index-repos.sh     # index with knowing
  results/             # benchmark output (gitignored)
  harness_test.go      # main entry point
```

## Adding a New System

1. Create `adapters/<name>.go` implementing `benchtype.Adapter`
2. Add to `adapters/registry.go` `AllAdapters()` with availability check
3. Run the benchmark; the new system appears in results automatically

## Fairness

- knowing's own repo is excluded from evaluation
- All systems get the same task descriptions (no system-specific tuning)
- Cold start by default (no pre-existing feedback/state)
- Token counting uses tiktoken cl100k_base for all systems
- Statistical tests are paired (same tasks, different systems)
