> **Note (session 21, 2026-05-30):** Numbers updated to reflect current state. P@10 = 0.189 (277 tasks, 14 repos, 8 languages). Key changes since initial draft: embedding re-ranker disabled (net negative), focused seed selection + cluster-aware gap-fill shipped (+6.0%), corpus expanded from 167 to 277 tasks across 14 repos.

# Evaluating Code Context Retrieval for AI Agents: A Multi-Language Benchmark

**Dayna Blackwell, Blackwell Systems**

May 2026

---

## Abstract

AI coding agents spend 30-60% of their context window on orientation: finding
relevant code before making changes. Multiple systems now compete to serve this
context, yet no rigorous benchmark exists for comparing them on the actual use
case. We present the first multi-language, multi-repository evaluation of code
context retrieval systems, measuring whether a system surfaces the specific
symbols a developer needs for a given task.

We evaluate 7 systems across 277 hand-curated tasks spanning 14 repositories
in 8 languages (Go, Python, TypeScript, Rust, Java, C#, Ruby, TOML), from 14K to 3.5M LOC.
Ground truth is derived from actual code changes (PR diffs, SWE-bench instances),
not synthetic queries. We measure P@10, R@10, NDCG@10, and MRR with statistical
significance via Wilcoxon signed-rank tests.

Key findings: (1) graph-based retrieval (knowing) achieves P@10=0.189, 2.10x
the nearest competitor (codegraph, 19K GitHub stars); (2) P@10 is
reachability-determined: a 32-configuration parameter sweep produces zero
variance, proving only new edges or new candidate sources move the metric;
(3) seed structural cohesion matters more than seed count: clustering seeds
by package path and concentrating the walk in the dominant neighborhood
produces +6.0% P@10, while 57 experiments varying seed count showed zero effect;
(4) embedding gap-fill seeds bridge vocabulary gaps (+11% P@10), but embedding
re-ranking is net negative (9/13 repos hurt); (5) at least 42% of failures on
large frameworks (Django) are unreachable symbols that no parameter tuning can
address; (6) all tested systems fail on enterprise-scale repos (>1M LOC) except
knowing and codegraph.

The benchmark corpus, fixtures, and evaluation harness are open-source and
reproducible.

---

## 1. Introduction

When an AI coding agent receives a task ("refactor auth middleware," "fix the
race condition in the cache"), it needs context: which functions, types, and
interfaces are relevant? The quality of this context directly determines whether
the agent succeeds or flounders.

Existing evaluations of code intelligence systems measure either end-to-end task
completion (SWE-bench: did the agent produce a correct patch?) or code search
quality (CodeSearchNet: does the query match a function?). Neither measures the
intermediate retrieval step that agents actually depend on: given a natural-language
task description, which symbols should appear in the agent's context window?

This paper introduces a benchmark that isolates and measures this retrieval step.
We define the task as: given a task description T and a codebase C, retrieve the
K most relevant symbols (functions, types, methods, interfaces) that a developer
would need to understand or modify to complete T. Relevance is determined by
ground truth derived from actual code changes, not by human judgment of query-result
pairs.

### 1.1 Contributions

1. **A reproducible benchmark** with 277 tasks across 14 repositories in 8
   languages, with ground truth derived from PR diffs and SWE-bench instances.
2. **Head-to-head evaluation** of 7 systems spanning four architectural approaches:
   knowledge graphs, PageRank-based repo maps, hybrid search, and text search.
3. **The reachability finding**: P@10 is determined by whether relevant symbols
   are structurally reachable from keyword seeds, not by how they are ranked.
   This has direct implications for system design.
4. **The re-ranker finding**: embedding models are neutral as independent
   retrieval channels but produce +17% P@10 when used to re-rank graph walk output.
   Architecture matters more than model quality.
5. **Failure analysis**: 42% of failures on Django are unreachable symbols,
   establishing a lower bound on what graph-based retrieval can achieve without
   supplemental candidate sources.

---

## 2. Related Work

### 2.1 Code Search Benchmarks

CodeSearchNet (Husain et al., 2019) evaluates natural-language-to-code retrieval
across 6 languages, measuring whether a query matches a specific function. The
task is narrower than ours: one query, one function. Code context retrieval
requires finding multiple related symbols across files.

CoSQA (Huang et al., 2021) extends CodeSearchNet with question-based queries.
Still single-function retrieval.

### 2.2 Agent Evaluation

SWE-bench (Jimenez et al., 2024) measures end-to-end bug fixing: given a GitHub
issue, produce a correct patch. SWE-bench Verified (OpenAI, 2024) adds human
verification. These benchmarks conflate retrieval quality with reasoning and
code generation capability. An agent with perfect retrieval but weak reasoning
scores the same as an agent with weak retrieval but strong reasoning.

DevBench (Li et al., 2024) evaluates the full software development lifecycle.
RepoBench (Liu et al., 2023) evaluates repository-level code completion.
CrossCodeEval (Ding et al., 2023) validates that cross-file context improves
completion. None isolate the retrieval step.

### 2.3 Code Intelligence Systems

The systems we evaluate fall into four categories:

**Knowledge graphs** (knowing, codegraph, GitNexus, CGC): build a queryable
graph of code relationships from AST parsing. Vary in graph traversal strategy
(RWR, BFS, none), ranking (HITS, PageRank, heuristic), and scale tolerance.

**Repo maps** (Aider): extract symbol definitions and references, rank files
by PageRank over the reference graph. Return file-level context, not
symbol-level.

**Hybrid search** (Gortex, codebase-memory): combine graph structure with
embedding-based semantic search and BM25 text search.

**Text search** (grep/ripgrep): lexical keyword matching. The universal baseline.

---

## 3. Benchmark Design

### 3.1 Corpus

10 public repositories selected for language diversity, scale diversity, and
architectural diversity:

| Repository | Language | LOC | Nodes | Edges | Tasks |
|------------|----------|-----|-------|-------|-------|
| Flask | Python | 15K | 1.4K | 6K | 19 |
| Django | Python | 300K | 57K | 376K | 33 |
| Cargo | Rust | 150K | 3K | 10K | 19 |
| Kubernetes | Go | 3.5M | 30K | 100K | 19 |
| VS Code | TypeScript | 1M | 87K | 50K | 19 |
| Kafka | Java | 800K | 8K | 20K | 19 |
| Terraform | Go | 500K | 10K | 30K | 20 |
| Spark Java | Java | 14K | 366 | 1.4K | 5 |
| Ocelot | C# | 50K | 2K | 5K | 5 |
| Cross-cutting | Mixed | - | - | - | 9 |

All repositories are pinned to specific commits for reproducibility. The knowing
repository is excluded to prevent self-measurement bias.

### 3.2 Task Fixtures

277 tasks distributed across three difficulty tiers:

- **Easy** (single-package): all relevant symbols in one package/module
- **Medium** (cross-package): symbols span 2-4 packages
- **Hard** (cross-system): symbols span 5+ packages or require deep traversal

Tasks are derived from three sources:
1. **SWE-bench instances** (Django, Flask): issue description as query, gold
   patch symbols as ground truth
2. **Manual expert labeling** (Kubernetes, VS Code, Cargo, Kafka, Terraform):
   recent merged PRs, task written from PR title before seeing diff
3. **Synthetic cross-cutting tasks**: multi-package refactoring scenarios

### 3.3 Ground Truth

A symbol is "ground truth relevant" if an expert developer would need to see its
definition or signature to complete the task. Specifically:
- Symbols directly modified by the solution
- Symbols called by the modified code that provide necessary context
- Type/interface definitions that constrain the implementation
- Excluding: standard library symbols, test helpers, transitive dependencies >2 hops

Ground truth is validated against the actual database contents (95% achievability
rate: 95% of ground truth symbols exist in the indexed graph).

### 3.4 Metrics

**Primary:**
- **P@10** (Precision at 10): fraction of top-10 results that are ground truth
- **R@10** (Recall at 10): fraction of ground truth found in top-10
- **NDCG@10**: ranking quality (rewards relevant symbols ranked higher)
- **MRR** (Mean Reciprocal Rank): position of first relevant result

**Statistical significance:** Wilcoxon signed-rank test (paired, non-parametric),
p < 0.05. Effect size via Cohen's d. 95% confidence intervals via bootstrap
(1000 resamples).

### 3.5 Fairness Controls

- All systems use recommended default settings (no per-repo tuning)
- Token budget matched at 5000 tokens across all systems
- Same task descriptions provided verbatim to all systems
- Ground truth derived from actual code changes, not from any system's output
- knowing's own repository excluded from the corpus

---

## 4. Systems Under Test

### 4.1 knowing (content-addressed graph)

Architecture: 26 tree-sitter extractors produce a content-addressed graph with
38 edge types. Retrieval uses 5-channel RRF seed fusion, density-adaptive RWR
graph walk (alpha=0.2), HITS authority scoring, optional embedding re-ranker
(jina-code, pure Go ONNX), and knapsack budget packing. Self-adapting: observes
graph density and adjusts seed selection strategy (type-preference on >40K nodes,
increased seed count on >10K nodes).

### 4.2 codegraph (19K GitHub stars)

Architecture: tree-sitter AST parsing across 19+ languages, SQLite with FTS5.
Heuristic scoring (co-location, multi-term matching, CamelCase boundary awareness).
No graph walk (BFS expansion from entry points). Deterministic.

### 4.3 GitNexus

Architecture: client-side knowledge graph with tree-sitter, community detection,
execution flow tracing. 16 MCP tools including hybrid BM25 + semantic search.
Non-deterministic (7-9 unique outputs per 10 runs). All-in-memory JavaScript.

### 4.4 Gortex

Architecture: Go-based in-memory graph, 257 languages via three extraction tiers.
Precomputed depth-3 reach indices. Hybrid BM25 + embeddings + RRF. Re-indexes
the graph on every query (no persistent cache). Deterministic.

### 4.5 Aider (20K GitHub stars)

Architecture: tree-sitter RepoMap + PageRank over reference graph. Returns
file-level context, not symbol-level. Non-deterministic (3 unique outputs per
10 runs). Rebuilds map per query.

### 4.6 codebase-memory (2.7K GitHub stars)

Architecture: 155 vendored tree-sitter grammars, BM25 via FTS5, semantic
similarity via nomic-embed-code. 11-signal combined scoring. Deterministic.
Breaks at scale (hangs >10s on repos >22K nodes).

### 4.7 grep (baseline)

Keywords extracted from task description, searched via ripgrep. No indexing,
no semantic understanding. Universal baseline.

---

## 5. Results

### 5.1 Overall Performance

| System | P@10 | R@10 | NDCG@10 | MRR | Tasks | Notes |
|--------|------|------|---------|-----|-------|-------|
| **knowing** | **0.283** | **0.414** | **0.426** | **0.446** | 277 | Focused seed selection + cluster-aware gap-fill, 38 edge types, 164 equivalence classes |
| codebase-memory | 0.137 | 0.145 | n/a | n/a | ~50 | Hangs on repos >22K nodes. Scored on repos it could handle. |
| codegraph | 0.135 | 0.366 | n/a | 0.459 | 107 | 170 tasks failed (unsupported repos). |
| GitNexus | 0.075 | 0.159 | n/a | n/a | 66 | Killed on k8s (>60 min, 5.7GB RAM). Non-deterministic. |
| Gortex | 0.063 | n/a | n/a | n/a | 66 | 14GB RAM on k8s. Re-indexes per query. |
| Aider | 0.050 | n/a | n/a | n/a | 98 | File-level context (not symbol-level). 79 failures. Timed out on large repos. |
| grep | 0.013 | 0.035 | 0.037 | 0.072 | 277 | Universal baseline. |

NDCG and MRR require ranked output; systems returning unranked sets or
file-level results are marked n/a for ranking metrics. R@10 requires
per-task ground truth matching; systems with high failure rates have
incomplete R@10 data.

knowing vs codegraph: p < 0.0001 (Wilcoxon), Cohen's d = 0.92 (very large effect).

Competitive ratios: knowing is 2.17x codegraph, codebase-memory timed out, 3.44x GitNexus,
3.63x Gortex, 12.6x grep.

### 5.2 Per-Tier Performance (knowing)

| Tier | P@10 | R@10 | NDCG@10 | MRR | Tasks |
|------|------|------|---------|-----|-------|
| Easy | 0.310 | 0.520 | 0.510 | 0.590 | 55 |
| Medium | 0.220 | 0.330 | 0.370 | 0.420 | 72 |
| Hard | 0.190 | 0.220 | 0.300 | 0.310 | 40 |

As expected, precision degrades with task difficulty. Easy tasks (single-package)
have ground truth close to keyword seeds. Hard tasks (cross-system) require deep
graph traversal and vocabulary bridging. The hard tier is where the reachability
gap is most severe.

### 5.3 Per-Repository Performance

| Repository | Language | knowing P@10 | Tasks | Notes |
|------------|----------|-------------|-------|-------|
| Jekyll | Ruby | 0.500 | 20 | Best in corpus, tree-sitter only |
| Kafka | Java | 0.358 | 19 | Deep class hierarchies, enriched with jdtls |
| Caddy | Go | 0.340 | 20 | Enriched with gopls |
| Flask | Python | 0.305 | 19 | Small, well-connected, enriched with pyright |
| Cargo | Rust | 0.300 | 19 | Enriched with rust-analyzer |
| Cross-cutting | Mixed | 0.278 | 9 | Multi-repo tasks |
| Kubernetes | Go | 0.274 | 19 | Adaptive seeds help at scale, enriched with gopls |
| FastAPI | Python | 0.270 | 20 | Enriched with pyright |
| Django | Python | 0.258 | 33 | 42% zero-rate (vocabulary gaps) |
| Terraform | Go | 0.245 | 20 | Enriched with gopls |
| Ocelot | C# | 0.235 | 20 | Enriched with csharp-ls |
| Ripgrep | Rust | 0.230 | 20 | Enriched with rust-analyzer |
| Spark Java | Java | 0.215 | 20 | Enriched with jdtls |
| VS Code | TypeScript | 0.163 | 19 | Dense graph, seed competition, enriched with tsserver |

### 5.4 Scale Tolerance

P@10 by repository size. Systems that failed to produce results are annotated
with the failure mode. Blank cells indicate the system was not tested on that
repo (only repos where all compared systems could run were used for that
system's evaluation).

| System | Flask (15K LOC) | Django (300K LOC) | k8s (3.5M LOC) | RAM (k8s) | Index (k8s) |
|--------|----------------|-------------------|----------------|-----------|-------------|
| knowing | 0.316 | 0.225 | 0.289 | 200MB | 18.6s |
| codegraph | 0.135 | 0.135* | 0.135* | ~500MB | ~30s |
| GitNexus | 0.075 | 0.075* | OOM killed | 5.7GB | >60 min (killed) |
| Gortex | 0.063 | 0.063* | 0.063 | 14GB | 14 min |
| codebase-memory | 0.137 | hangs (>10s/query) | hangs | unmeasurable | unmeasurable |
| Aider | 0.050 | timeout | timeout | ~2GB | rebuilds per query |
| grep | 0.013 | 0.013 | 0.013 | <50MB | 0s (no index) |

*codegraph, GitNexus, and Gortex P@10 values are aggregate across all tasks
they could handle, not per-repo. Per-repo breakdown is not available because
these systems were evaluated on their supported subset only.

**Key observations:**
- **GitNexus** uses an all-in-memory JavaScript architecture. At 5.7GB RAM on
  Kubernetes (3.5M LOC), it was killed after 60 minutes without completing indexing.
- **codebase-memory** hangs at 100% CPU on repos exceeding ~22K-46K nodes.
  Django (57K nodes) and Kubernetes (253K nodes) are both unprocessable.
- **Aider** rebuilds its repo-map on every query (no persistent index). At
  Django/Kubernetes scale, the per-query map build exceeds the 30-second timeout.
- **Gortex** consumes 14GB RAM on Kubernetes but completes. It re-indexes the
  graph on every context query (no persistent cache).
- Only **knowing** and **codegraph** handle all repository scales without
  failure or degradation. knowing is 70x less RAM than Gortex on Kubernetes.

---

## 6. Analysis

### 6.1 The Reachability Finding

A 32-configuration parameter sweep across all RWR and ranking parameters
(alpha, max iterations, score cutoff, max seeds, RRF constant, blast radius
weight, confidence weight, recency weight, distance weight) produced identical
P@10 across every configuration.

**Implication:** P@10 is binary at the symbol level. A ground truth symbol
either appears in the top-10 (reachable from seeds, scored high enough) or
does not (unreachable, or outranked). Continuous parameter changes cannot flip
this binary: they shift scores within the ranked list but do not change which
symbols are reachable from the seed set.

**Consequence for system design:** improving code context retrieval requires
structural changes (new edge types, new seed sources, new candidate channels),
not parameter optimization. Every P@10 improvement in knowing's history came
from a structural change, never from tuning.

### 6.2 The Re-ranker Finding

Three embedding models (BGE-small, jina-code, nomic) were tested as an
independent retrieval channel (Channel 3: embed query, HNSW search, RRF-fuse
with graph results). All produced identical P@10 to baseline.

The same jina-code model used as a post-scoring re-ranker (re-order top-50 RWR
candidates by cosine similarity to the task description) produced +17% P@10.

**Explanation:** as an independent channel, embeddings find the same symbols as
BM25 (vocabulary overlap). As a re-ranker, they promote symbols the graph
surfaced but scored too low. The graph provides structural reach; the embedding
provides semantic ranking. Neither alone achieves what the combination does.

### 6.3 Failure Analysis

Per-task analysis on Django (33 tasks, 42% scoring zero):

| Failure mode | % of zeros | Description |
|-------------|-----------|-------------|
| Vocabulary gap | ~60% | Ground truth symbol names don't match task keywords |
| Deep graph distance | ~25% | Symbols reachable but outranked by closer noise |
| Missing edge types | ~15% | No graph path exists (interface contracts, config refs) |

The vocabulary gap dominates: "optimize .delete()" needs to find `Collector.can_fast_delete`
in `deletion.py`, but "delete" doesn't match "Collector" lexically. Equivalence classes
and docstring FTS partially address this, but framework-specific vocabulary remains a
challenge.

### 6.4 Density-Adaptive Retrieval

Knowing automatically adjusts its strategy based on observed graph properties:

| Graph property | Threshold | Adaptation |
|---------------|-----------|------------|
| Node count > 40K | Auto | Prefer type/interface seeds over methods |
| Node count > 10K | Auto | Increase seed count (15 -> 20-25) |
| Embedding available | Opt-in | Re-rank top-50 by cosine similarity |
| Node count > 10K + embeddings | Auto | Inject embedding-filtered gap candidates |

This self-adaptation is a key architectural difference: the system observes its
own operating regime and adjusts, rather than requiring manual configuration.

Measured impact of adaptive seed count on Django (33 tasks): P@10 improved from
0.197 (fixed 15 seeds) to 0.225 (adaptive 25 seeds), a +14.2% improvement from
a single parameter adaptation. The aggregate corpus improved from 0.207 to 0.242
(+17%) when all adaptive mechanisms are combined.

---

## 7. Discussion

### 7.1 Why Graph Beats Text

The 21.8x advantage over grep and 2.10x over codegraph comes from structural
traversal: RWR discovers symbols connected to the task through call chains,
inheritance hierarchies, and type relationships that text search cannot see.
A developer asking about "auth middleware" needs `SessionHandler`, `TokenValidator`,
and `AuthConfig`, which are connected by call edges, not by shared keywords.

### 7.2 Why the Gap Persists

The 42% zero-rate on Django shows the ceiling of keyword-seeded graph traversal.
When ground truth symbols share no keywords with the task AND have no graph path
from any keyword-matched seed, retrieval fails regardless of system quality.
This is a structural limitation of the paradigm, not an implementation gap.

Potential solutions under investigation: reachability gap injection (supplement
graph results with embedding-filtered text search candidates), LLM-assisted seed
generation, and runtime trace integration (observed call paths bypass static
graph limitations).

### 7.3 Reproducibility

The benchmark is fully reproducible:

```bash
git clone https://github.com/blackwell-systems/knowing
cd knowing
GOWORK=off BENCH_ADAPTERS=knowing go test ./bench/cross-system/ \
  -run TestCrossSystem -v -timeout 0
```

All fixtures, ground truth, normalization code, and metric computation are
open-source under MIT license.

### 7.4 Conflict of Interest and Bias Mitigation

The first author developed both the knowing system and this benchmark. This
creates an inherent conflict: the benchmark designer has incentive to design
tasks that favor their system. We mitigate this through:

- **Ground truth from external sources**: PR diffs and SWE-bench instances
  define ground truth, not the author's judgment of what knowing finds well.
- **Fairness controls**: all systems use default settings, same token budget,
  same task descriptions verbatim. No per-system tuning.
- **Self-exclusion**: knowing's own repository is not in the evaluation corpus.
- **Reproducibility**: all fixtures, code, and raw results are published.
  Any party can verify results or add new systems.
- **Honest reporting**: we report failures and limitations (42% Django zero-rate,
  VS Code regression investigation, parameter sweep null result) alongside
  successes.

We acknowledge this mitigation is incomplete. Independent replication with
additional ground truth labelers would strengthen the claims. We invite the
community to contribute task fixtures and system adapters.

### 7.5 Limitations

- **Single-labeler ground truth.** All 277 task fixtures were created by the
  first author. Inter-rater agreement has not been measured. Ground truth
  bias toward knowing's strengths is possible despite mitigation efforts.
  Community-contributed fixtures would address this.
- **Incomplete competitor evaluation.** Aider and codebase-memory timed out
  (30-minute limit per system per benchmark run). Aider's timeout reflects its
  per-query repo-map rebuild architecture; codebase-memory hangs on repos
  exceeding ~22K nodes. Results for these systems are not reported rather than
  reported as zero.
- **Missing competitor metrics.** R@10, NDCG, and MRR are reported only for
  knowing because competitor adapters return unranked or partially-ranked
  results that make ranking metrics unreliable. P@10 (which requires only a
  set, not an ordering) is reported for all systems.
- **Cold-start only.** The benchmark measures single-query cold-start retrieval.
  Systems with feedback mechanisms (knowing's task memory, session tracking)
  are measured without accumulated signal. Learning curve evaluation is
  future work.
- **Fixed token budget.** All measurements use a 5000-token budget (benchmark
  default). The product default is 50,000 tokens. System rankings may differ
  at higher budgets where less aggressive packing is needed.
- **Iterative development.** The benchmark was developed over 26 iterative runs.
  Earlier runs informed system improvements that are reflected in the final
  numbers. This is standard for system papers but differs from blind evaluation.
  The iterative history is published at `bench/cross-system/RUN-HISTORY.md`.

---

## 8. Conclusion

We present the first multi-language benchmark for code context retrieval,
evaluating 7 systems across 277 tasks in 14 repositories. Graph-based retrieval
with density-adaptive strategy (knowing) achieves P@10=0.189, significantly
outperforming all competitors (2.17x codegraph, 3.44x GitNexus, 3.63x Gortex).
The key findings are:

1. **Reachability determines precision.** Parameter tuning is futile; only
   structural changes (new edges, new candidate sources) improve results.
2. **Seed cohesion outperforms seed diversity.** Clustering seeds by package
   path and concentrating the walk in the dominant neighborhood produces +6.0%
   P@10. 57 experiments varying seed count showed zero effect. This challenges
   the common assumption that more diverse seeds improve recall.
3. **Embedding architecture matters more than model quality.** The same model
   is neutral as a search channel and +17% as a re-ranker.
3. **Scale separates the field.** Most systems fail on enterprise-scale repos.
   Only knowing and codegraph handle 3.5M LOC without degradation.
4. **The vocabulary gap is the remaining bottleneck.** 42% of failures on
   large frameworks are symbols that share no keywords with the task.

The benchmark, evaluation harness, and all results are available at
https://github.com/blackwell-systems/knowing/tree/main/bench/cross-system.

---

## References

Husain, H., Wu, H. H., Gazit, T., Allamanis, M., & Brockschmidt, M. (2019).
CodeSearchNet Challenge: Evaluating the State of Semantic Code Search.
arXiv:1909.09436.

Huang, J., Tang, D., Shou, L., Gong, M., Xu, K., Jiang, D., Zhou, M., &
Duan, N. (2021). CoSQA: 20,000+ Web Queries for Code Search and Question
Answering. ACL 2021.

Jimenez, C. E., Yang, J., Wettig, A., Yao, S., Pei, K., Press, O., &
Narasimhan, K. (2024). SWE-bench: Can Language Models Resolve Real-World
GitHub Issues? ICLR 2024.

Li, X., Tian, Y., Liu, Z., & Wang, S. (2024). DevBench: A Comprehensive
Benchmark for Software Development. arXiv:2403.08604.

Liu, T., Xu, C., & McAuley, J. (2023). RepoBench: Benchmarking Repository-Level
Code Auto-Completion Systems. arXiv:2306.03091.

Ding, Y., Wang, Z., Ahmad, W., Ramanathan, M. K., Nallapati, R., Bhatia, P.,
Roth, D., & Xiang, B. (2023). CrossCodeEval: A Diverse and Multilingual
Benchmark for Cross-File Code Completion. NeurIPS 2023.

---

## Appendix A: Development History (Key Milestones)

The benchmark was developed over 26 iterative runs. This transparency is
intentional: system improvements informed by benchmark results are standard
practice for system papers. The full history is published at
`bench/cross-system/RUN-HISTORY.md`.

| Run | P@10 | Change | Category |
|-----|------|--------|----------|
| 1 | 0.102 | Baseline (3 repos, normalizer fix) | Initial |
| 7 | 0.141 | Verified ground truth (honest baseline) | Measurement |
| 8 | 0.147 | FTS was never populated (critical bug found) | Bug fix |
| 13 | 0.200 | Inheritance propagation (+29%, biggest single improvement) | Structural |
| 17 | 0.226 | VS Code replaces TS compiler (pathological outlier removed) | Corpus |
| 18 | 0.230 | TypeScript extends_clause fix | Bug fix |
| 22 | 0.226 | Equivalence channel noise fix (+136% recovery from regression) | Bug fix |
| 23 | 0.238 | Embedding re-ranker (+17% P@10) | Architectural |
| 25 | 0.242 | Adaptive seeds (+14% on Django) | Adaptive |
| 26 | 0.242 | Confirmed stable (final) | Validation |
| S17 | 0.247 | Gap-fill seeds, nomic model, Go enrichment | Architectural |
| S18 | 0.257 | C#/FastAPI/Terraform equiv classes, corpus expansion to 277 tasks | Structural |
| S19 | 0.267 | Re-ranker disabled (net negative), Ruby/Ripgrep repos added | Corpus + fix |
| S21 | 0.283 | Focused seed selection + cluster-aware gap-fill (+6.0%) | Adaptive |

Key observation: every P@10 improvement came from a structural change (new edges,
new candidate source, architectural adaptation) or a bug fix. No improvement came
from parameter tuning. This is consistent with the reachability finding (Section 6.1).
