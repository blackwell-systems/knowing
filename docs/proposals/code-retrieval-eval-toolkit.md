# Proposal: Code Retrieval Evaluation Toolkit (CRET)

## Summary

Extract knowing's benchmarking infrastructure as a standalone, publishable evaluation
framework for code context retrieval systems. The SWE-bench equivalent for the step
BEFORE the agent writes code.

## Problem

Every code intelligence tool claims "better context" but none publish reproducible
evaluations against competitors on shared ground truth. There is no standard benchmark
for comparing code retrieval systems. Each vendor publishes cherry-picked examples or
synthetic metrics that don't translate to real-world value.

SWE-bench standardized agent coding evaluation. Nothing equivalent exists for code
context retrieval (which systems return the most relevant symbols for a task?).

## What Already Exists (in knowing)

| Component | Location | What it does |
|-----------|----------|-------------|
| Cross-system harness | `bench/cross-system/` | 16 repos, 300 tasks, statistical testing |
| Ground truth fixtures | `bench/cross-system/corpus/tasks/` | Hand-curated, DB-validated, 3 difficulty tiers + SWE-bench + cross-cutting |
| Corpus repos | `bench/cross-system/corpus/repos/` | 16 repos across 8 languages (Go, Python, Rust, Java, TypeScript, C#, Ruby, TOML/HCL) |
| Corpus manifest | `bench/cross-system/corpus/MANIFEST.yaml` | Reproducible corpus definition with git SHAs |
| Adapter interface | `bench/cross-system/benchtype/` | `Index(path)` + `Retrieve(path, task, budget)` |
| 7 adapters | `bench/cross-system/adapters/` | knowing, grep, GitNexus, Gortex, Aider, codegraph (CGC), codebase-memory |
| Statistical engine | `bench/cross-system/metrics/` | Wilcoxon signed-rank, Cohen's d, bootstrap CI, P@K, R@K, NDCG, MRR, F1 |
| Fixture validator | `bench/cross-system/cmd/validate-fixtures/` | Verifies ground truth exists in indexed DB |
| Failure analyzer | `bench/cross-system/cmd/failure-analysis/` | Categorizes misses: same_package, related_name, test_symbol, noise |
| Compounding harness | `bench/cross-system/compounding_test.go` | Multi-round memory/vocab compounding measurement |
| Cross-task vocab test | `bench/cross-system/cross_task_vocab_test.go` | Proves cross-task vocabulary bridging with attribution |
| Methodology doc | `bench/cross-system/METHODOLOGY.md` | Cold-start protocol, task memory contamination discovery, honest measurement |
| Findings doc | `bench/cross-system/FINDINGS.md` | Per-repo breakdown, competitive ratios, run history |

## Corpus

| Repo | Language | Nodes | Tasks | Enrichment |
|------|----------|-------|-------|------------|
| django | Python | ~15K | 36 | pyright |
| flask | Python | ~8K | 10 | pyright |
| fastapi | Python | ~5K | 10 | pyright |
| cargo | Rust | ~30K | 20 | rust-analyzer |
| kubernetes | Go | ~80K | 20 | gopls |
| terraform | Go | ~40K | 20 | gopls |
| caddy | Go | ~12K | 20 | gopls |
| vscode | TypeScript | ~87K | 20 | tsserver |
| ocelot | C# | ~8K | 20 | csharp-ls |
| spark-java | Java | ~15K | 20 | jdtls |
| kafka | Java | ~25K | 20 | jdtls |
| jekyll | Ruby | ~4K | 10 | ruby-lsp |
| ripgrep | Rust | ~6K | 10 | rust-analyzer |
| rails | Ruby | ~30K | 20 | ruby-lsp |
| saleor | Python | ~20K | 20 | pyright |
| hugo | Go | ~20K | 12 | gopls |

**300 tasks** across 3 difficulty tiers (easy, medium, hard), SWE-bench derived tasks,
and cross-cutting tasks that span multiple packages or concern architectural patterns.

## What CRET Would Be

A standalone repo (`blackwell-systems/cret` or `code-retrieval-eval`) containing:

### Core

```
cret/
├── corpus/
│   ├── MANIFEST.yaml      # Reproducible corpus definition (repos, SHAs, setup)
│   ├── corpus-setup.sh    # One-command corpus setup
│   ├── repos/             # 16 repos (cloned by setup script)
│   └── tasks/             # 308 task fixtures with ground truth
├── adapters/
│   ├── interface.go       # Adapter interface: Index + Retrieve + Name
│   ├── grep.go            # Baseline adapter (always available)
│   ├── aider.go           # Aider repo-map adapter
│   ├── gitnexus.go        # GitNexus adapter
│   ├── codegraph.go       # codegraph (CGC) adapter
│   ├── gortex.go          # Gortex adapter
│   └── template.go        # Template for adding new adapters
├── metrics/
│   ├── precision.go       # P@K, R@K, NDCG, MRR, F1@K
│   ├── stats.go           # Wilcoxon signed-rank, Cohen's d, bootstrap CI
│   └── report.go          # Markdown report generation
├── cmd/
│   ├── validate-fixtures/ # Verify ground truth against DB
│   └── failure-analysis/  # Categorize retrieval misses
├── harness_test.go        # Main benchmark entry point
├── METHODOLOGY.md         # Cold-start protocol, contamination avoidance
├── FINDINGS.md            # Latest results (auto-generated)
└── README.md              # Usage, adding adapters, interpreting results
```

### Adapter Interface (the public API)

```go
type Adapter interface {
    Name() string
    Index(repoPath string) (durationMs int64, err error)
    Retrieve(repoPath string, task Task, tokenBudget int) (RetrievalResult, error)
    SupportsLearning() bool
    RecordFeedback(repoPath string, task Task, relevantSymbols []string) error
    Reset(repoPath string) error
}

type Task struct {
    ID          string   `yaml:"id"`
    Repo        string   `yaml:"repo"`
    Tier        string   `yaml:"tier"`         // easy, medium, hard
    Description string   `yaml:"description"`  // the task query given to each system
    GroundTruth []string `yaml:"ground_truth"` // qualified symbol names
    Tags        []string `yaml:"tags"`
    Source      string   `yaml:"source"`       // swe-bench, manual, synthetic
}

type RetrievalResult struct {
    System     string
    TaskID     string
    Symbols    []RetrievedSymbol
    TokensUsed int
    LatencyMs  int64
    Error      string
}
```

Anyone can add their system: implement the interface, run the benchmark, get comparable
P@10/R@10/NDCG numbers against all other systems on the same ground truth.

### Measurement Protocol

Critical methodology that distinguishes CRET from ad-hoc benchmarks:

1. **Cold start required.** Task memory must be empty before measurement. Session 23
   discovered 26K stale entries in terraform alone, inflating all prior measurements.
2. **No embeddings in baseline.** Embedding gap-fill confirmed neutral on cold start
   (3 runs identical). Embeddings are a separate dimension, not part of core P@10.
3. **Wilcoxon signed-rank** for pairwise comparison (not paired t-test: P@10 is
   not normally distributed).
4. **Bootstrap confidence intervals** for aggregate metrics.
5. **Ground truth validated against DB:** `validate-fixtures` confirms every ground
   truth symbol exists in the indexed graph. 95%+ achievable rate required.
6. **Reproducible with single command:** `go test ./bench/cross-system/ -timeout 0`

## Why This Matters

1. **Sets the evaluation standard.** Once published, competitors must either beat
   knowing on CRET or build their own benchmark (which validates the need for one).

2. **Credibility.** Publishing the framework that evaluates your own product alongside
   competitors signals confidence. "We built the benchmark AND we win on it."

3. **Community contribution.** Others can add adapters for their systems (Sourcegraph,
   Cody, Continue, Cursor). Each addition strengthens the benchmark's authority.

4. **Citability.** With a Zenodo DOI, researchers can cite CRET when evaluating code
   intelligence systems. Becomes the reference benchmark for the field.

5. **Moat.** The ground truth fixtures (300 tasks, 16 repos, hand-curated and validated)
   took significant effort. Competitors can use the framework but can't easily replicate
   the fixture quality.

## Current Results (session 26, 2026-06-02)

| System | P@10 | vs knowing |
|--------|------|------------|
| **knowing** | **0.330** | baseline |
| codegraph (19K stars) | 0.087 | 3.23x behind |
| GitNexus | 0.055 | 5.11x behind |
| Gortex | 0.052 | 5.40x behind |
| Aider | 0.023 | 12.2x behind |
| grep | 0.015 | 18.7x behind |
| codebase-memory | timeout | 22/297 tasks in 60 min |

## Publication Strategy

1. **GitHub release** as standalone repo with MIT license
2. **Zenodo DOI** for academic citability
3. **Blog post**: "We built the SWE-bench for code context retrieval. Here's what we found."
4. **Paper**: NeurIPS Datasets & Benchmarks or EMNLP Resources track
5. **LinkedIn**: announce to developer tools audience

## Prerequisites

- Clean up adapter code to remove knowing-internal dependencies
- Write standalone README with "add your system in 5 minutes" guide
- Ensure corpus repos are fetchable via MANIFEST.yaml (git clone URLs + SHAs)
- Package metrics as importable Go library
- Add 3-5 more repos for diversity (one non-English, one ML codebase)

## Effort

Medium (2-3 days to extract, clean, and package). The hard work (308 fixtures,
7 adapters, metrics, statistical testing, methodology) is already done.

## Risks

- Competitors may refuse to engage (ignore the benchmark)
- Ground truth bias: fixtures written by knowing's developer may inadvertently favor
  knowing's retrieval patterns. Mitigation: independent validation by users, diverse
  fixture sources (SWE-bench derived, manual, cross-cutting)
- Maintenance burden: keeping adapters working as competitors update their APIs
- Corpus size: 300 tasks may be insufficient for statistical power on per-repo
  comparisons. AI-generated evaluation corpus (roadmap item) would address this.

## Timeline

Extract and publish after the blog post lands and the benchmark numbers are stable.
The framework ships with knowing winning on its own benchmark, which is the strongest
possible launch position.
