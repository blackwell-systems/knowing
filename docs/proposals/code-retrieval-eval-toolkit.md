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
| Cross-system harness | `bench/cross-system/` | 7 repos, 117 tasks, statistical testing |
| Ground truth fixtures | `bench/cross-system/corpus/tasks/` | Hand-curated, validated (95% achievable) |
| Corpus repos | `bench/cross-system/corpus/repos/` | Flask, Django, Cargo, VS Code, Kubernetes, Spark, Ocelot |
| Adapter interface | `bench/cross-system/benchtype/` | `Index(path)` + `Retrieve(path, task, budget)` |
| 6 adapters | `bench/cross-system/adapters/` | knowing, grep, GitNexus, Gortex, Aider, CGC |
| Statistical engine | `bench/cross-system/metrics/` | Paired t-test, Cohen's d, confidence intervals, NDCG, MRR |
| Fixture validator | `bench/cross-system/cmd/validate-fixtures/` | Verifies ground truth exists in indexed DB |
| Failure analyzer | `bench/cross-system/cmd/failure-analysis/` | Diagnoses why specific tasks fail |
| Agent efficiency | `bench/agent-efficiency/` | Multi-repo, multi-mode, transcript comparison |
| Honest negatives | `bench/AGENT-EFFICIENCY-STUDY.md` | Documented where graph tools DON'T help |

## What CRET Would Be

A standalone repo (`blackwell-systems/cret` or `code-retrieval-eval`) containing:

### Core

```
cret/
├── corpus/
│   ├── repos/           # 7 repos (flask, django, cargo, vscode, kubernetes, spark, ocelot)
│   └── tasks/           # 117 task fixtures with ground truth
├── adapters/
│   ├── interface.go     # Adapter interface: Index + Retrieve + Name
│   ├── grep.go          # Baseline adapter (always available)
│   ├── aider.go         # Aider repo-map adapter
│   ├── gitnexus.go      # GitNexus adapter
│   └── template.go      # Template for adding new adapters
├── metrics/
│   ├── precision.go     # P@K, R@K, NDCG, MRR
│   ├── stats.go         # Paired t-test, Cohen's d, CI
│   └── report.go        # Markdown report generation
├── harness_test.go      # Main benchmark entry point
├── README.md            # Usage, adding adapters, interpreting results
└── FINDINGS.md          # Latest results (auto-generated)
```

### Adapter Interface (the public API)

```go
type Adapter interface {
    Name() string
    Index(repoPath string) (durationMs int64, err error)
    Retrieve(repoPath string, task Task, tokenBudget int) (RetrievalResult, error)
    IsAvailable() bool
}

type Task struct {
    ID          string
    Repo        string
    Description string
    GroundTruth []string  // qualified symbol names
    Difficulty  string    // easy, medium, hard
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

Anyone can add their system: implement 3 methods, run the benchmark, get comparable
P@10/R@10/NDCG numbers against all other systems on the same ground truth.

## Why This Matters

1. **Sets the evaluation standard.** Once published, competitors must either beat
   knowing on CRET or build their own benchmark (which validates the need for one).

2. **Credibility.** Publishing the framework that evaluates your own product alongside
   competitors signals confidence. "We built the benchmark AND we win on it."

3. **Community contribution.** Others can add adapters for their systems (Sourcegraph,
   Cody, Continue, Cursor). Each addition strengthens the benchmark's authority.

4. **Citability.** With a Zenodo DOI, researchers can cite CRET when evaluating code
   intelligence systems. Becomes the reference benchmark for the field.

5. **Moat.** The ground truth fixtures (117 tasks, 7 repos, hand-curated and validated)
   took significant effort. Competitors can use the framework but can't easily replicate
   the fixture quality.

## Publication Strategy

1. **GitHub release** as standalone repo with MIT license
2. **Zenodo DOI** for academic citability
3. **Blog post**: "We built the SWE-bench for code context retrieval. Here's what we found."
4. **Optional**: Workshop paper at ICSE/ASE/MSR (Mining Software Repositories)

## Prerequisites

- Complete the Aider head-to-head comparison (in progress)
- Run at least one full benchmark with all 6 adapters
- Clean up adapter code to remove knowing-internal dependencies
- Write standalone README with "add your system in 5 minutes" guide
- Ensure corpus repos are fetchable (git clone URLs, not vendored)

## Effort

Medium (2-3 days to extract, clean, and package). The hard work (fixtures, adapters,
metrics, statistical testing) is already done.

## Risks

- Competitors may refuse to engage (ignore the benchmark)
- Ground truth bias: fixtures written by knowing's developer may inadvertently favor
  knowing's retrieval patterns. Mitigation: independent validation by users.
- Maintenance burden: keeping adapters working as competitors update their APIs

## Timeline

After the Aider comparison is complete and the results are strong, extract and publish.
The framework ships with knowing winning on its own benchmark, which is the strongest
possible launch position.
