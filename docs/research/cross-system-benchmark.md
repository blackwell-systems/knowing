# Cross-System Context Retrieval Benchmark

**Running results:** [bench/cross-system/FINDINGS.md](../../bench/cross-system/FINDINGS.md)
**Study overview:** [bench/CONTEXT-PACKING-STUDY.md](../../bench/CONTEXT-PACKING-STUDY.md)
**Implementation:** [bench/cross-system/](../../bench/cross-system/)

## Current Status (2026-05-25)

### Implementation Progress

| Component | Status | Notes |
|-----------|--------|-------|
| Benchmark harness | Done | `harness_test.go`, metrics, normalization, statistical tests |
| Evaluation corpus (9 repos) | Done | kubernetes, VS Code, flask, cargo, django, spark-java, ocelot, kafka, next.js |
| Task fixtures (167 total) | Done | 6 languages (Go, Python, TypeScript, Rust, Java, C#) |
| Ground truth validation | Done | 95% match rate, validate-fixtures tool |
| knowing adapter | Done | P@10=0.242 (Run 26), 38 edge types, embedding re-ranker |
| grep adapter | Done | P@10=0.013 (baseline) |
| codegraph adapter | Done | P@10=0.135, 107/167 tasks (10 failed on unsupported repos) |
| GitNexus adapter | Done | P@10=0.075, 66/167 tasks (killed on k8s: >60 min, 5.7GB RAM) |
| Gortex adapter | Done | P@10=0.063, 66/167 tasks (14 min k8s indexing, 14GB RAM) |
| Aider adapter | Evaluated | Timed out on 30 min limit |
| codebase-memory adapter | Evaluated | Timed out on 30 min limit |
| SCIP adapter | Not built | Requires per-language SCIP index generation |
| Statistical analysis | Done | Wilcoxon, Cohen's d, bootstrap CI, 26 runs |
| SWE-bench integration | Done | 10 fixtures; finding: fault localization != context retrieval |
| Embedding re-ranker | Done | +15% P@10, +18.3% R@10 on full corpus (Run 26) |
| Failure analysis | Done | 56% noise, 36% test symbols; RWR reach is bottleneck |

### Key Results (Run 26, 167 tasks, 9 repos)

| System | P@10 | R@10 | NDCG@10 | MRR | vs knowing |
|--------|------|------|---------|-----|------------|
| **knowing** | **0.242** | **0.362** | **0.393** | **0.440** | baseline |
| codegraph (19K stars) | 0.135 | - | - | - | 0.57x |
| GitNexus | 0.075 | - | - | - | 0.32x |
| Gortex | 0.063 | - | - | - | 0.26x |
| grep | 0.013 | - | - | - | 0.05x |

**Competitive ratios:** 1.79x codegraph, 3.23x GitNexus, 3.84x Gortex, 18.6x grep.
Statistical significance: p<0.0001, d=0.92 (very large effect on recall).

### Per-Repo Performance (Run 26)

| Repo | Language | P@10 | Delta vs baseline | Tasks |
|------|----------|------|-------------------|-------|
| Flask | Python | 0.336 | - | 19 |
| Django | Python | 0.330 | - | 20 |
| Kafka | Java | 0.195 | +39.5% (re-ranker) | 20 |
| Kubernetes | Go | 0.184 | +92.8% (re-ranker) | 28 |
| VS Code | TypeScript | 0.137 | -16% (re-ranker) | 20 |
| Cargo | Rust | 0.123 | +15.9% (re-ranker) | 20 |
| spark-java | Java | - | - | 20 |
| ocelot | C# | - | - | 20 |
| next.js | TypeScript | - | - | 20 |

### Key Findings

1. **RWR (graph traversal) is the primary differentiator**, not FTS/BM25
2. **Inheritance propagation was the breakthrough** (+29% in one change)
3. **Quality scales with graph density**: dense hierarchies (Django) >> flat codebases (Cargo)
4. **Embedding re-ranker is the biggest single improvement** (+17% P@10, +18.3% R@10). Architecture matters more than model: three models were neutral as independent search, but re-ranking top-50 RWR candidates by cosine similarity promotes relevant symbols the graph surfaced but scored low.
5. **Density-adaptive seeding** auto-enables PreferTypeSeeds on graphs >40K nodes, preventing precision degradation at scale
6. **P@10 is reachability-determined.** 32-config parameter sweep proved zero variance. Only new edges or new seed sources move the metric.
7. **SWE-bench measures fault localization**, not context retrieval (different capability)
8. **38 edge types** including accesses_field, reads_env, executes_process (supply chain detection)

### Remaining Work

| Item | Priority | Effort | Impact |
|------|----------|--------|--------|
| SCIP adapter | Low | 2 days | Precision ceiling reference |
| Blog post updates | Medium | 1 day | Public credibility with latest numbers |
| ~~VS Code regression~~ | Done | - | Resolved in session 16: 0% delta (was -16%) |
| ~~Ocelot regression~~ | Done | - | Resolved in session 16: 0% delta (was -30.8%) |

---

## 1. Motivation

AI coding agents spend 30-60% of their context window on orientation: finding
the right code to read before making changes. The quality of this context
directly determines task success. Multiple systems now compete to serve this
context: knowledge graphs, repo maps, code search engines, and raw text search.

No rigorous, reproducible benchmark exists comparing these systems on the actual
use case: "given a coding task, which system retrieves the most relevant symbols
in the fewest tokens?"

This benchmark answers that question with:
- Fixed evaluation corpus (specific repos, specific tasks, specific ground truth)
- Formal metrics with statistical significance testing
- Fairness controls that prevent home-field advantage for any system
- Reproducible methodology anyone can run

The goal is publishable data that honestly shows where knowing wins, where it
loses, and where systems are equivalent.

---

## 2. Systems Under Test

Seven systems covering the primary architectural approaches to code context retrieval.
Evaluated: knowing, codegraph, GitNexus, Gortex, grep. Attempted but timed out: Aider, codebase-memory.

### 2.1 knowing (content-addressed graph)

**Invocation:**
```bash
# Index the target repo
knowing index --repo <path> --module <module-path>

# Retrieve context
knowing context-for-task --task "<description>" --budget <tokens> --format json
```

**What to capture:**
- `symbols[]` array with qualified names, scores, distances
- `tokens_used` (actual token count)
- Wall-clock latency (index time + query time separately)
- Session state (first query vs repeated query on same repo)

**Configuration:**
- Token budget: match across all systems (benchmark uses 5000 tokens for fair comparison; product default is 50000)
- Format: JSON (for automated parsing; not GCF, to avoid format advantage)
- No pre-existing feedback (cold start unless measuring learning curve)

### 2.2 GitNexus (knowledge graph MCP)

**Invocation:**
```bash
# Index
gitnexus index <path>

# Query via MCP (simulated tool call)
# Tool: search_codebase
# Input: { "query": "<task description>", "limit": 20 }
```

**What to capture:**
- Returned symbols/code snippets
- Token count of response (tiktoken cl100k_base)
- Wall-clock latency
- Graph construction time

**Configuration:**
- Default settings (no tuning for specific repos)
- Native Tree-sitter parsing (not WASM mode)
- LadybugDB backend (default)

**Installation:** `npm install -g gitnexus` (verify version at benchmark time)

### 2.3 Aider repo-map (PageRank on reference graph)

**Invocation:**
```python
# Aider's repo-map is internal; extract via its API
from aider.repomap import RepoMap

rm = RepoMap(
    root=repo_path,
    main_model=model,  # needed for token counting
    io=io_instance,
)
repo_map_text = rm.get_repo_map(
    chat_files=[],           # no files in chat (cold start)
    other_fnames=all_files,  # all repo files as candidates
    mentioned_fnames=set(),
    mentioned_idents=extract_identifiers(task_description),
)
```

**What to capture:**
- Full repo-map text (tree-context format)
- Token count (tiktoken, Aider's own counting)
- Which files/symbols appear in the map
- Wall-clock generation time

**Configuration:**
- `map_tokens=5000` (match other systems' budget)
- No files pre-loaded in chat (cold start)
- Mentioned identifiers extracted from task description (Aider's normal flow)

**Installation:** `pip install aider-chat` (pin version)

### 2.4 Sourcegraph / SCIP-based indexers

**Invocation:**
```bash
# Generate SCIP index
scip-go  # or scip-typescript, scip-python, etc.

# Query via Sourcegraph CLI or API
src search -query="<symbols from task>" -json

# Alternative: use scip CLI directly for symbol lookup
scip snapshot --from index.scip --format json
```

**What to capture:**
- Symbols returned with definitions and references
- Precision of cross-file references (compiler-accurate)
- Token count of formatted output
- Index generation time

**Configuration:**
- Language-appropriate SCIP indexer
- Local mode (no Sourcegraph instance required for SCIP)
- For Sourcegraph API comparison: use sourcegraph.com search on public repos

**Note:** SCIP provides precise navigation, not task-oriented retrieval. The
benchmark adapter must translate a task description into symbol queries (extract
identifiers, search for definitions). This represents the "expert user with
precise tools" baseline.

### 2.5 Raw grep/ripgrep baseline

**Invocation:**
```bash
# Extract keywords from task description (simple: split on spaces, filter stopwords)
keywords=$(echo "$task" | extract_keywords)

# Search
for kw in $keywords; do
    rg -n --type <lang> "$kw" <repo_path> | head -20
done
```

**What to capture:**
- Lines returned per keyword
- Unique files touched
- Token count (4 tokens/line estimate, verified with tiktoken)
- Which ground-truth symbols appear in output
- Wall-clock time

**Configuration:**
- Keywords: task description split on whitespace, stopwords removed, CamelCase split
- Per-keyword limit: 20 lines (simulates agent grep behavior)
- Total budget: stop when cumulative tokens exceed 5000
- File type filter: match target language

**Adapter script:** `bench/cross-system/adapters/grep_baseline.py`

### 2.6 CodeGraphContext (CGC)

**Invocation:**
```bash
# Index
cgc index <path> --backend kuzu

# Query via MCP tool
# Tool: search_symbols
# Input: { "query": "<task description>", "limit": 20 }

# Or via CLI
cgc search "<keywords>" --limit 20
```

**What to capture:**
- Returned symbols with metadata
- Token count of response
- Wall-clock latency
- Index time and database size

**Configuration:**
- KuzuDB backend (default, embedded)
- Default Tree-sitter parsing
- No SCIP enhancement (baseline comparison)

**Installation:** `pip install codegraphcontext` (pin version)

---

## 3. Evaluation Corpus

### 3.1 Repository Selection

The corpus uses **9 repositories** chosen for diversity along these axes:

| Repo | Language | Size (LOC) | Why |
|------|----------|------------|-----|
| [kubernetes/kubernetes](https://github.com/kubernetes/kubernetes) | Go | ~3.5M | Large, well-structured, deep call chains |
| [microsoft/vscode](https://github.com/microsoft/vscode) | TypeScript | ~1M | Large, classes/services/DI/inheritance |
| [pallets/flask](https://github.com/pallets/flask) | Python | ~30K | Small, clear package boundaries, well-documented |
| [rust-lang/cargo](https://github.com/rust-lang/cargo) | Rust | ~200K | Medium, strong type system, module hierarchy |
| [django/django](https://github.com/django/django) | Python | ~350K | Large framework, cross-package dependencies |
| [apache/kafka](https://github.com/apache/kafka) | Java | ~800K | Enterprise Java, deep class hierarchies |
| [sparklemotion/spark-java](https://github.com/perwendel/spark) | Java | ~14K | Small Java web framework |
| [ThreeMammals/Ocelot](https://github.com/ThreeMammals/Ocelot) | C# | ~50K | .NET API gateway, C# coverage |
| [vercel/next.js](https://github.com/vercel/next.js) | TypeScript | ~500K | Large TS framework, module boundaries |

**Exclusion:** The knowing repo itself is NOT in the evaluation corpus. This
prevents home-field advantage from fixtures tuned to knowing's own structure.

**Version pinning:** Each repo is pinned to a specific commit SHA at benchmark
creation time. Document in `bench/cross-system/corpus/repos.yaml`:

```yaml
repos:
  - name: kubernetes
    url: https://github.com/kubernetes/kubernetes
    commit: <sha>
    language: go
    module: k8s.io/kubernetes
  - name: vscode
    url: https://github.com/microsoft/vscode
    commit: <sha>
    language: typescript
  - name: flask
    url: https://github.com/pallets/flask
    commit: <sha>
    language: python
  - name: cargo
    url: https://github.com/rust-lang/cargo
    commit: <sha>
    language: rust
  - name: django
    url: https://github.com/django/django
    commit: <sha>
    language: python
```

### 3.2 Ground Truth Tasks

Each repository gets **~20 tasks** (167 total across 9 repos), distributed across 3 difficulty tiers:

| Tier | Tasks/repo | Characteristics |
|------|-----------|-----------------|
| Easy (single-package) | 8 | All relevant symbols in one package/module |
| Medium (cross-package) | 8 | Symbols span 2-4 packages |
| Hard (cross-system) | 4 | Symbols span 5+ packages, require deep traversal |

### 3.3 Task Sources

Tasks are derived from three sources to ensure realism:

#### Source A: SWE-bench instances (40 tasks)

Select 40 tasks from [SWE-bench](https://github.com/princeton-nlp/SWE-bench)
that target our corpus repos (django, flask). For each:
1. Use the issue description as the task query
2. Use the gold patch's modified symbols as ground truth
3. Include any symbols the patch imports or calls that were not previously imported

This gives realistic "developer needs context for this issue" scenarios with
objectively correct ground truth (the symbols the fix actually used).

#### Source B: Manual expert labeling (40 tasks)

For repos not in SWE-bench (kubernetes, VS Code, cargo), create tasks
manually by:
1. Pick a recent merged PR (last 6 months)
2. Write a task description from the PR title/description (before seeing the diff)
3. Label ground truth from the PR's actual symbol modifications and their
   immediate callers/callees

This simulates "developer reads the issue, asks for context before implementing."

#### Source C: Synthetic cross-cutting tasks (20 tasks)

Create tasks that stress cross-package retrieval:
- "Refactor error handling across the HTTP stack"
- "Add tracing to all database operations"
- "Update authentication to support OAuth2"

Ground truth: manually trace which symbols would need modification, using
the repo's actual architecture. Two independent labelers; inter-rater agreement
required (see Section 7).

### 3.4 Task Fixture Format

```yaml
# bench/cross-system/corpus/tasks/kubernetes/easy/01-add-pod-status-field.yaml
id: "k8s-easy-01"
repo: kubernetes
commit: <sha>  # must match repos.yaml
source: "manual"  # or "swe-bench" or "synthetic"
source_ref: "https://github.com/kubernetes/kubernetes/pull/12345"  # if from PR
difficulty: easy
task: "Add a new condition type to PodStatus for tracking init container readiness"
ground_truth:
  - pkg/apis/core/types.PodConditionType
  - pkg/apis/core/types.PodStatus
  - pkg/apis/core/types.PodCondition
  - pkg/kubelet/status/status_manager.SetPodStatus
  - staging/src/k8s.io/api/core/v1/types.PodConditionType
tags: [single-package, type-extension, api-types]
notes: "Derived from PR #12345. Ground truth = symbols modified + direct callers."
```

---

## 4. Metrics

### 4.1 Primary Metrics

#### Precision@K

Fraction of returned results that are actually relevant.

```
Precision@K = |{relevant} ∩ {returned top-K}| / K
```

Measured at K = 5, 10, 20. K=10 is the primary comparison point (matches
knowing's existing eval). K=5 captures "first screen" quality. K=20 captures
deeper retrieval.

#### Recall@K

Fraction of ground-truth symbols found in the top-K results.

```
Recall@K = |{relevant} ∩ {returned top-K}| / |{ground-truth}|
```

Same K values. Note: Recall@K can exceed 1.0 if the system returns multiple
symbols matching the same ground-truth entry (via substring matching).

#### NDCG@K (Normalized Discounted Cumulative Gain)

Rewards systems that rank relevant symbols higher.

```
DCG@K = Σ_{i=1}^{K} rel(i) / log2(i + 1)
IDCG@K = DCG for the ideal ranking
NDCG@K = DCG@K / IDCG@K
```

Where `rel(i) = 1` if result at rank i is relevant, 0 otherwise. NDCG@10 is
the primary ordering metric.

#### Token Efficiency

Relevant symbols per token consumed.

```
TokenEfficiency = |{relevant} ∩ {returned}| / tokens_consumed
```

Where `tokens_consumed` is the total token count of the system's output
(measured via tiktoken cl100k_base). This penalizes verbose systems that
return relevant results buried in noise.

### 4.2 Secondary Metrics

#### Mean Reciprocal Rank (MRR)

```
MRR = 1/|Q| * Σ_{q∈Q} 1/rank_q
```

Where `rank_q` is the position of the first relevant result for query q.
Captures "how quickly does the developer get oriented?"

#### Time to Context (TTC)

Wall-clock seconds from query submission to complete response. Measured as:
- `TTC_cold`: first query on a freshly indexed repo
- `TTC_warm`: subsequent query on an already-indexed repo
- `TTC_index`: one-time indexing cost (amortized across queries)

#### F1@K

Harmonic mean of Precision@K and Recall@K.

```
F1@K = 2 * (P@K * R@K) / (P@K + R@K)
```

### 4.3 Longitudinal Metrics (Learning Curve)

Only applicable to systems with feedback/learning mechanisms (knowing, potentially GitNexus):

#### Precision Delta After Feedback

Run the same task set twice:
1. Round 1: cold start (no prior feedback)
2. Between rounds: record feedback for correct results
3. Round 2: same tasks, measure improvement

```
LearningGain = P@10_round2 - P@10_round1
```

### 4.4 Staleness Metrics

#### Staleness Recovery Time

1. Index the repo at commit C1
2. Introduce a code change (simulate commit C2: rename a function, add a file)
3. Query a task that depends on the changed code
4. Measure: does the system detect the stale context? How quickly does it update?

```
StalenessRecovery = time from code change to correct context response
```

Systems without incremental update (Aider, grep) get TTC_cold as their recovery time.

---

## 5. Methodology

### 5.1 Environment

All benchmarks run on the same machine to eliminate hardware variance:
- Apple M-series (M2 Pro or better) or equivalent Linux (8+ cores, 32GB RAM)
- All repos cloned locally (no network latency for file access)
- Each system gets a warm filesystem cache (read all files once before timing)
- Three runs per measurement; report median

### 5.2 Execution Protocol

For each (system, repo, task) triple:

```
1. Clone repo at pinned commit (or verify existing clone matches)
2. Clear any system-specific caches/databases
3. INDEX PHASE:
   - Start timer
   - Run system's indexing command
   - Stop timer -> index_time
4. QUERY PHASE (cold):
   - Start timer
   - Submit task description to system
   - Collect response
   - Stop timer -> query_time_cold
5. QUERY PHASE (warm, 3 repetitions):
   - Start timer
   - Submit same task description again
   - Collect response
   - Stop timer -> query_time_warm (take median)
6. PARSE PHASE:
   - Extract symbols from response (system-specific parser)
   - Normalize to qualified names
   - Match against ground truth
   - Compute metrics
```

### 5.3 Symbol Normalization

Different systems return symbols in different formats. Normalize all to:

```
<package_path>.<TypeName>.<MethodName>
```

Rules:
- Strip leading module paths (e.g., `k8s.io/kubernetes/` prefix)
- Preserve package-relative paths (e.g., `pkg/kubelet/status/status_manager`)
- Functions: `package.FuncName`
- Methods: `package.Type.Method`
- Types: `package.TypeName`

Normalization code lives in `bench/cross-system/normalize.go`.

### 5.4 Ground Truth Matching

A returned symbol matches a ground-truth entry if:
- The normalized ground-truth string is a substring of the normalized result, OR
- The normalized result is a substring of the normalized ground-truth string

This handles:
- Partial qualification (`store.NodesByName` matches `internal/store.SQLiteStore.NodesByName`)
- Over-qualification (system returns full path, ground truth uses short form)

Matching code reuses knowing's existing `isRelevant()` logic from `bench/context-relevance/`.

### 5.5 Token Counting

All systems' output is measured with tiktoken (cl100k_base encoding):

```python
import tiktoken
enc = tiktoken.get_encoding("cl100k_base")
tokens = len(enc.encode(system_output_text))
```

This provides a uniform cost metric regardless of how each system formats output.

### 5.6 Statistical Significance

For system comparisons, report:
- Mean and standard deviation across all tasks
- Per-tier breakdown (easy/medium/hard)
- Paired Wilcoxon signed-rank test (p < 0.05) for each system pair
- Effect size (Cohen's d) for practical significance
- 95% confidence intervals via bootstrap (1000 resamples)

A difference is only claimed as "significant" if both:
1. p < 0.05 on paired test
2. Cohen's d > 0.3 (at least small effect size)

---

## 6. Fairness Controls

### 6.1 No Home-Field Advantage

- knowing's own repo is excluded from the corpus
- No fixtures derived from knowing's existing eval (those test knowing-specific structure)
- All systems get the same task descriptions verbatim (no rephrasing for any system)
- Ground truth is derived from actual code changes, not from knowing's graph structure

### 6.2 Configuration Fairness

Each system uses its **recommended defaults**:
- No system-specific tuning for benchmark repos
- No custom configuration files beyond what `<system> init` generates
- Token budgets matched across all systems (5000 tokens primary, also test 2000 and 10000)
- If a system has no budget control, capture its full output but measure metrics at matched K

### 6.3 Cold Start vs Warm Start

Report both:
- **Cold start:** No prior indexing, no feedback, no session history
- **Warm start:** After indexing (but before any task-specific feedback)
- **Learned:** After one round of feedback (only for systems that support it)

This prevents penalizing systems that require upfront investment (indexing) while
also measuring that investment's payoff.

### 6.4 Query Formulation Fairness

The task description is passed verbatim to all systems. Systems that require
different query formats (e.g., keyword extraction for grep) use a **fixed,
documented adapter** that:
- Extracts keywords via the same algorithm for all keyword-based systems
- Does not use system-specific query optimization
- Is published as part of the benchmark code

### 6.5 Version Pinning

All system versions are pinned and documented:
```yaml
# bench/cross-system/versions.yaml
systems:
  knowing: v0.10.1  # 38 edge types, embedding re-ranker, density-adaptive
  codegraph: latest (npm)
  gitnexus: latest (npm)
  gortex: latest (go install)
  grep/ripgrep: 14.x.x
  aider: latest (pip) # timed out
  codebase-memory: latest (npm) # timed out
```

### 6.6 Independent Ground Truth Verification

Ground truth labels are verified by a second reviewer who did NOT create the
original labels. Disagreements are resolved by examining the actual PR diff
and documenting the resolution.

---

## 7. Ground Truth Labeling Protocol

### 7.1 Who Labels

- **Primary labeler:** Developer familiar with the target repo (reads code, understands architecture)
- **Verifier:** Second developer who independently reviews labels against the source PR/issue

### 7.2 Labeling Criteria

A symbol is "ground truth relevant" if an expert developer would need to see
its definition or signature to accomplish the task. Specifically:

**Include:**
- Symbols directly modified by the task's solution
- Symbols called by the modified code that the developer needs to understand
- Type definitions the developer needs to see to write correct code
- Interface definitions that constrain the implementation

**Exclude:**
- Standard library symbols (os.Open, fmt.Sprintf, etc.)
- Test helpers and mock implementations
- Symbols the developer would already know from the task description
- Transitive dependencies more than 2 hops from modified code

### 7.3 Labeling Process

```
For each task:
1. Read the task description (issue/PR title + body)
2. Read the gold-standard solution (PR diff)
3. List all symbols modified in the diff
4. For each modified symbol:
   a. Add its direct callers (1 hop) that provide necessary context
   b. Add type definitions it references
   c. Add interface definitions it must satisfy
5. Remove standard library symbols
6. Remove test-only symbols (unless the task is about tests)
7. Cap at 15 symbols per task (forces prioritization of most important)
8. Record confidence: HIGH (obviously needed) or MEDIUM (helpful but not critical)
```

### 7.4 Inter-Rater Agreement

Measure Cohen's kappa between primary labeler and verifier:
- kappa > 0.8: strong agreement, labels are reliable
- 0.6 < kappa < 0.8: moderate agreement, discuss disagreements
- kappa < 0.6: weak agreement, re-examine labeling criteria

Target: kappa > 0.75 before running the benchmark.

### 7.5 Label Storage Format

```yaml
# bench/cross-system/corpus/tasks/kubernetes/easy/01-add-pod-status-field.yaml
ground_truth:
  - symbol: "pkg/apis/core/types.PodConditionType"
    confidence: HIGH
    reason: "Type being extended"
  - symbol: "pkg/kubelet/status/status_manager.SetPodStatus"
    confidence: MEDIUM
    reason: "Caller that validates conditions"
labeler: "developer-A"
verifier: "developer-B"
agreement: 0.85  # Cohen's kappa for this task
disputes:
  - symbol: "pkg/apis/core/validation.ValidatePodStatus"
    resolution: "included"
    rationale: "Developer must understand validation to add a new condition"
```

---

## 8. Analysis Framework

### 8.1 Primary Comparisons

#### Table 1: Overall Performance (primary result)

```
| System      | P@5  | P@10 | P@20 | R@10 | NDCG@10 | MRR  | TokenEff |
|-------------|------|------|------|------|---------|------|----------|
| knowing     |      |      |      |      |         |      |          |
| GitNexus    |      |      |      |      |         |      |          |
| Aider       |      |      |      |      |         |      |          |
| Sourcegraph |      |      |      |      |         |      |          |
| grep        |      |      |      |      |         |      |          |
| CGC         |      |      |      |      |         |      |          |
```

#### Table 2: Per-Tier Breakdown

```
| System   | Easy P@10 | Easy R@10 | Med P@10 | Med R@10 | Hard P@10 | Hard R@10 |
|----------|-----------|-----------|----------|----------|-----------|-----------|
| ...      |           |           |          |          |           |           |
```

#### Table 3: Per-Repo Breakdown

Shows whether any system has a language-specific advantage.

#### Table 4: Token Efficiency at Fixed Recall

For each system, find the minimum token budget needed to achieve R@10 >= 0.5.
Lower is better.

### 8.2 Visualizations

1. **Precision-Recall curves** (one per system, overlaid): vary K from 1 to 50
2. **Token efficiency scatter**: X = tokens consumed, Y = recall achieved (one point per task)
3. **Radar chart**: 6 axes (P@10, R@10, NDCG@10, TokenEff, TTC, MRR), one polygon per system
4. **Per-task heatmap**: rows = tasks (sorted by difficulty), columns = systems, color = P@10
5. **Learning curve** (knowing only): P@10 over 5 feedback rounds
6. **Box plots**: distribution of P@10 scores per system (shows variance, not just mean)
7. **Statistical significance matrix**: pairwise p-values between all system pairs

### 8.3 Failure Analysis

For each system, categorize failures:
- **Vocabulary miss:** task uses different words than the code (e.g., "authentication" vs "auth")
- **Depth miss:** relevant symbol is >2 hops from any keyword match
- **Noise overwhelm:** relevant symbols exist in results but below K cutoff
- **Language gap:** system doesn't support the target language well
- **Scale failure:** system degrades on large repos (>1M LOC)

Document the top-3 failure modes per system. This informs improvement priorities.

### 8.4 Output Format

Results are written to:
```
bench/cross-system/results/
  run-<timestamp>/
    raw/
      knowing-kubernetes-easy-01.json
      gitnexus-kubernetes-easy-01.json
      ...
    aggregated/
      overall.csv
      per-tier.csv
      per-repo.csv
      per-task.csv
    analysis/
      significance-tests.json
      failure-analysis.md
      FINDINGS.md  # auto-generated summary
```

### 8.5 Interpretation Guidelines

When reporting results:
- Never claim "X is better than Y" without statistical significance
- Report effect sizes alongside p-values
- Acknowledge when systems solve different problems (SCIP provides navigation, not discovery)
- Note any system that was disadvantaged by the benchmark design
- Separate "retrieval quality" from "system maturity" in conclusions

---

## 9. Iteration Protocol

### 9.1 Using Results to Improve knowing

After each benchmark run:

1. **Identify failure categories** (Section 8.3) for knowing specifically
2. **Prioritize by impact:** which failure mode, if fixed, would improve the most tasks?
3. **Implement the fix** in knowing's context engine
4. **Re-run the benchmark** on the same corpus (no fixture changes between iterations)
5. **Record the delta** in the experiment log

### 9.2 When to Update Ground Truth

Ground truth fixtures are updated ONLY when:
- A labeling error is discovered (wrong symbol, incorrect match criteria)
- The pinned repo commit changes (new benchmark version)
- Inter-rater agreement reveals ambiguity requiring clarification

Ground truth is NEVER updated to make any system look better.

### 9.3 Benchmark Versioning

```
bench/cross-system/CHANGELOG.md

## v1.0 (initial)
- 7 repos, ~117 tasks, 7 systems
- Pinned commits: [list SHAs]

## v1.1 (if ground truth corrections needed)
- Corrected 3 task labels per Section 7.4 review
- No system version changes
```

### 9.4 Comparative Learning

When another system outperforms knowing on a category:
1. Analyze WHY (what retrieval signal do they use that we don't?)
2. Document in `bench/cross-system/analysis/competitive-lessons.md`
3. Assess feasibility of adopting the technique
4. If adopted, re-run to verify improvement

---

## 10. Implementation Plan

### Phase 1: Infrastructure (1 week)

**Goal:** Benchmark harness that can run any adapter against any fixture.

| Task | Effort | Output |
|------|--------|--------|
| Create `bench/cross-system/` directory structure | 1h | Directory layout |
| Write `repos.yaml` with 5 pinned repos | 2h | Corpus definition |
| Implement symbol normalization (`normalize.go`) | 4h | Shared normalization |
| Implement metric computation (`metrics.go`) | 4h | P@K, R@K, NDCG, MRR, TokenEff |
| Write adapter interface (`adapter.go`) | 2h | Common interface for all systems |
| Implement knowing adapter | 2h | Calls `ForTask` directly |
| Implement grep baseline adapter | 3h | Shell-out to rg, parse results |
| Write result aggregation and FINDINGS.md generation | 4h | Auto-report |
| Statistical significance tests (Wilcoxon, bootstrap CI) | 4h | `stats.go` |

**Deliverables:**
- `bench/cross-system/harness_test.go` (main entry point)
- `bench/cross-system/adapters/` (one file per system)
- `bench/cross-system/metrics/` (computation)
- `bench/cross-system/normalize.go` (symbol normalization)

### Phase 2: Ground Truth (2 weeks)

**Goal:** ~117 labeled tasks across 7 repos with inter-rater verification.

| Task | Effort | Output |
|------|--------|--------|
| Select 40 SWE-bench instances for django/flask | 8h | 40 fixture YAMLs |
| Label 40 manual tasks for kubernetes/VS Code/cargo | 16h | 40 fixture YAMLs |
| Create 20 synthetic cross-cutting tasks | 8h | 20 fixture YAMLs |
| Second-reviewer verification pass | 12h | Verified labels with kappa scores |
| Resolve disagreements, document | 4h | Final ground truth |

**Deliverables:**
- `bench/cross-system/corpus/tasks/<repo>/<tier>/*.yaml` (100 files)
- `bench/cross-system/corpus/labeling-report.md` (inter-rater agreement)

### Phase 3: Adapters (1 week)

**Goal:** All 6 systems integrated and producing parseable output.

| Task | Effort | Output |
|------|--------|--------|
| Implement GitNexus adapter | 4h | MCP tool call + response parser |
| Implement Aider repo-map adapter | 6h | Python bridge to extract repo-map |
| Implement SCIP/Sourcegraph adapter | 6h | Index generation + symbol lookup |
| Implement CGC adapter | 4h | MCP tool call + response parser |
| Verify all adapters on 3 test fixtures | 4h | Smoke test passing |

**Deliverables:**
- `bench/cross-system/adapters/gitnexus.go`
- `bench/cross-system/adapters/aider.py` (Python, called via subprocess)
- `bench/cross-system/adapters/scip.go`
- `bench/cross-system/adapters/cgc.go`

### Phase 4: Execution and Analysis (1 week)

**Goal:** Full benchmark run with publishable results.

| Task | Effort | Output |
|------|--------|--------|
| Clone all 7 repos at pinned commits | 1h | Local corpus |
| Run full benchmark (all systems x all tasks) | 8h | Raw results |
| Generate analysis (tables, charts, significance) | 4h | FINDINGS.md |
| Failure analysis per system | 8h | `failure-analysis.md` |
| Write summary narrative | 4h | Publishable conclusions |

**Deliverables:**
- `bench/cross-system/results/run-<timestamp>/FINDINGS.md`
- `bench/cross-system/results/run-<timestamp>/analysis/`

### Total Estimated Effort

| Phase | Duration | Person-hours |
|-------|----------|-------------|
| Phase 1: Infrastructure | 1 week | 26h |
| Phase 2: Ground Truth | 2 weeks | 48h |
| Phase 3: Adapters | 1 week | 24h |
| Phase 4: Execution | 1 week | 25h |
| **Total** | **5 weeks** | **123h** |

---

## 11. Prior Art and References

### 11.1 Existing Benchmarks in Code Intelligence

| Benchmark | What it measures | Relevance |
|-----------|-----------------|-----------|
| [SWE-bench](https://github.com/princeton-nlp/SWE-bench) | End-to-end issue resolution by AI agents | Source of realistic tasks + ground truth patches |
| [RepoEval](https://arxiv.org/abs/2306.03091) | Repository-level code completion | Measures context retrieval for completion (narrower than our use case) |
| [CrossCodeEval](https://arxiv.org/abs/2310.11248) | Cross-file code completion | Validates that cross-file context improves completion; our benchmark measures context quality directly |
| [RepoBench](https://arxiv.org/abs/2306.03091) | Repository-level benchmarking | Similar structure; we adapt their multi-level difficulty approach |
| [DevBench](https://arxiv.org/abs/2403.08604) | Full software development lifecycle | Broader scope; our benchmark isolates the retrieval step |
| [SWE-bench Verified](https://openai.com/index/introducing-swe-bench-verified/) | Human-verified subset of SWE-bench | Use this subset for highest-quality ground truth |

### 11.2 What We Build On

- **SWE-bench task format:** We adopt their issue description as task query and gold patch as ground truth source. We extend by extracting individual symbols from patches rather than measuring pass/fail on the whole issue.

- **RepoEval's repo selection:** Their approach of selecting repos by size/language/domain diversity informs our corpus selection.

- **CrossCodeEval's cross-file analysis:** Their finding that cross-file context significantly improves LLM performance validates our benchmark's focus on retrieval quality.

### 11.3 How This Benchmark Differs

Existing benchmarks measure **end-to-end task completion** (does the agent solve the issue?). This benchmark isolates the **retrieval step** (does the context system surface the right symbols?). This distinction matters because:

1. End-to-end benchmarks confound retrieval quality with LLM capability
2. Context retrieval is the only variable we can control (LLM is fixed)
3. Retrieval quality is measurable independently of generation quality
4. Results are actionable: they tell us exactly which symbols each system misses

---

## 12. Directory Structure

```
bench/cross-system/
  README.md                           # Quick start and overview
  harness_test.go                     # Main benchmark entry point
  metrics/
    precision.go                      # P@K computation
    recall.go                         # R@K computation
    ndcg.go                           # NDCG computation
    mrr.go                            # MRR computation
    token_efficiency.go               # TokenEff computation
    stats.go                          # Significance tests, bootstrap CI
  adapters/
    adapter.go                        # Interface definition
    knowing.go                        # knowing adapter
    gitnexus.go                       # GitNexus adapter
    aider.py                          # Aider repo-map adapter (Python)
    scip.go                           # SCIP/Sourcegraph adapter
    cgc.go                            # CodeGraphContext adapter
    grep_baseline.go                  # ripgrep baseline adapter
  normalize.go                        # Symbol normalization
  corpus/
    repos.yaml                        # Pinned repository definitions
    tasks/
      kubernetes/
        easy/                         # 8 fixtures
        medium/                       # 8 fixtures
        hard/                         # 4 fixtures
      typescript/
        easy/
        medium/
        hard/
      flask/
        easy/
        medium/
        hard/
      cargo/
        easy/
        medium/
        hard/
      django/
        easy/
        medium/
        hard/
    labeling-report.md                # Inter-rater agreement scores
  results/
    run-<timestamp>/
      raw/                            # Per-system, per-task JSON
      aggregated/                     # CSV summaries
      analysis/
        significance-tests.json
        failure-analysis.md
        competitive-lessons.md
      FINDINGS.md                     # Auto-generated report
  versions.yaml                       # Pinned system versions
  CHANGELOG.md                        # Benchmark versioning
```

---

## 13. Running the Benchmark

### Quick Start (single system, single repo)

```bash
# Run knowing against flask corpus only
GOWORK=off go test ./bench/cross-system/ -v \
  -run TestCrossSystem/knowing/flask \
  -timeout 10m
```

### Full Run (all systems, all repos)

```bash
# Ensure all repos are cloned
./bench/cross-system/scripts/clone-corpus.sh

# Ensure all systems are installed
./bench/cross-system/scripts/verify-systems.sh

# Full benchmark (takes ~2 hours)
GOWORK=off go test ./bench/cross-system/ -v \
  -count=3 \
  -timeout 4h
```

### Single System Comparison

```bash
# Compare knowing vs grep baseline only
GOWORK=off go test ./bench/cross-system/ -v \
  -run "TestCrossSystem/(knowing|grep)" \
  -timeout 30m
```

### Regenerate Analysis

```bash
# Re-analyze existing raw results without re-running systems
GOWORK=off go test ./bench/cross-system/ -v \
  -run TestAnalyzeResults \
  -timeout 5m
```

---

## 14. Success Criteria

The benchmark is considered successful (regardless of which system wins) if:

1. **Reproducibility:** Two independent runs produce results within 5% of each other
2. **Discrimination:** At least one system pair shows statistically significant difference
3. **Coverage:** Results span the full range (no system gets 0% or 100% on all tasks)
4. **Fairness validation:** No system's authors object to the methodology after review
5. **Actionability:** Results identify at least 3 concrete improvements for knowing's engine

### Confirmed Outcomes (26 runs)

Based on 26 iterative benchmark runs across 167 tasks:
- knowing wins on **precision** (P@10=0.242, 1.79x the nearest competitor codegraph)
- knowing wins on **recall** (R@10=0.362, only system with full-corpus recall data)
- knowing wins on **token efficiency** (GCF format, graph-aware packing)
- knowing wins on **scalability** (18s index on kubernetes, 200MB RAM vs 14GB for Gortex)
- grep wins on **time to first result** (no indexing overhead, but 18.6x less precise)
- codegraph is the strongest competitor (P@10=0.135) but fails on 60/167 tasks
- Aider and codebase-memory both timed out on the 30-min limit
- GitNexus cannot index enterprise repos (killed at >60 min on kubernetes)
- **Embedding re-ranker** was the biggest single improvement: +17% P@10, +18.3% R@10
- **Dense graph repos benefit most** from re-ranker (Kubernetes +92.8%). Session 15 regressions (VS Code -16%, Ocelot -30.8%) resolved in session 16: both show 0% P@10 delta.

---

## 15. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| System X not installable/broken | Medium | Drops one comparison | Document as "unable to evaluate," proceed with others |
| Ground truth disagreement > 25% | Low | Unreliable results | Re-examine criteria, bring in third labeler |
| Token counting inconsistency | Medium | Unfair comparison | Single tiktoken path for ALL systems |
| System requires paid API | Medium | Cannot reproduce freely | Document cost; provide cached results for verification |
| Repo too large to index in time | Low | Benchmark takes days | Set 30-min timeout per system per repo; skip with documented reason |
| knowing loses badly | Medium | Uncomfortable results | Publish honestly; losing is data; use to prioritize improvements |

---

## 16. Ethical Considerations

- All evaluated systems are used within their license terms
- GitNexus (PolyForm NC) is used for research/evaluation (permitted under NC)
- No private/proprietary code in the corpus (all repos are public)
- Results will be published with methodology, enabling contestation
- If any system's maintainers provide corrections to our integration, we incorporate them
