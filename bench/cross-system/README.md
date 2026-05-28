# Cross-System Context Retrieval Benchmark

Rigorous comparison of context retrieval systems on identical tasks across 12 public repositories, 7 languages.

**Results:** [FINDINGS.md](FINDINGS.md) (competitive comparisons, per-repo breakdown, scale analysis)
**Run history:** [RUN-HISTORY.md](RUN-HISTORY.md)
**Methodology:** [METHODOLOGY.md](METHODOLOGY.md) (metrics, fixture design, statistical methods, limitations)
**Full specification:** [docs/research/cross-system-benchmark.md](../../docs/research/cross-system-benchmark.md)
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
| **codegraph** (19K stars) | Tree-sitter + FTS5 + heuristic scoring | `npm install -g @colbymchenry/codegraph` |
| **Aider** (~20K stars) | PageRank repo-map (file-level) | `pip install aider-chat` |
| **Gortex** | Go graph engine (tree-sitter, parallel) | `go install github.com/zzet/gortex` |
| **GitNexus** | Knowledge graph MCP | `npm install -g gitnexus` |
| **codebase-memory** (2.6K stars) | BM25 + semantic edges (155 grammars) | `codebase-memory-mcp` on PATH |
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

12 repos pinned to specific versions, 7 languages:

- **kubernetes** (Go, ~3.5M LOC) - v1.30.0, enriched with gopls
- **VS Code** (TypeScript, ~1M LOC) - 1.90.0, enriched with tsserver
- **django** (Python, ~300K LOC) - 5.1, enriched with pyright
- **terraform** (Go, ~2M LOC) - hashicorp/terraform, enriched with gopls
- **kafka** (Java, ~500K LOC) - apache/kafka, enriched with jdtls
- **cargo** (Rust, ~150K LOC) - 0.82.0, enriched with rust-analyzer
- **flask** (Python, ~15K LOC) - 3.1.0, enriched with pyright
- **caddy** (Go, ~75K LOC) - caddyserver/caddy, enriched with gopls
- **fastapi** (Python, ~30K LOC) - fastapi/fastapi, enriched with pyright
- **spark-java** (Java, ~14K LOC) - Spark micro-framework, enriched with jdtls
- **ocelot** (C#, ~30K LOC) - ThreeMammals/Ocelot, enriched with csharp-ls

237 tasks across 3 difficulty tiers (easy/medium/hard) with hand-labeled ground truth symbols.

## Directory Structure

```
bench/cross-system/
  FINDINGS.md          # competitive results (executive summary, per-repo breakdown)
  RUN-HISTORY.md       # chronological run log (internal reference)
  METHODOLOGY.md       # metrics, fixture design, statistical methods
  benchtype/           # shared types (leaf package, no internal imports)
  normalize/           # symbol canonicalization + tests
  metrics/             # P@K, R@K, NDCG, MRR, F1, token efficiency, stats
  adapters/            # system implementations + availability registry
  corpus/
    repos.yaml         # pinned repo definitions
    tasks/<repo>/<tier>/*.yaml  # ground truth fixtures
    repos/             # cloned repos (gitignored)
  scripts/
    clone-repos.sh     # shallow clone all 12 repos
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
