# Cross-System Benchmark Methodology

This document describes the experimental methodology used in the cross-system context
retrieval benchmark. It covers fixture design, ground truth validation, statistical
methods, regression detection, and known limitations.

**Note on the state of the field:** No competing code context retrieval system
publishes a reproducible evaluation benchmark. codegraph (19K GitHub stars),
GitNexus (40K stars), Gortex, codebase-memory, and Aider all ship without
published precision metrics, ground truth corpora, or reproducibility tooling.
This benchmark is the first multi-system evaluation in the space. We acknowledge
its limitations (single labeler, curated corpus) and document them transparently.
We welcome external reproduction and independent ground truth validation.

## Design Principles

1. **Same input, different systems.** Every system receives identical task descriptions.
   No system-specific prompt engineering or query adaptation.
2. **Cold start.** No pre-existing feedback, session history, or learned state. Each
   task is independent. This measures retrieval quality, not memory.
3. **Manual ground truth.** Fixtures are hand-labeled by a developer who read the source
   code. No LLM-generated ground truth (avoids circular evaluation).
4. **Paired statistical tests.** Systems are compared on the same tasks, eliminating
   task-difficulty variance. Wilcoxon signed-rank (non-parametric, no normality assumption).
5. **Effect size over p-values.** Cohen's d reported alongside significance. A
   statistically significant result with d=0.1 is not meaningful.

## Fixture Design

### Difficulty Tiers

| Tier | Criteria | Example |
|------|----------|---------|
| Easy | Single symbol, obvious name in task | "Add a before_request hook" -> `Scaffold.before_request` |
| Medium | Multiple relevant symbols, some requiring structural knowledge | "Implement request caching" -> `Flask.full_dispatch_request`, `RequestContext`, etc. |
| Hard | Requires understanding of call chains, inheritance, or cross-file relationships | "Add custom error page for 404" -> `errorhandler`, `_find_error_handler`, `HTTPException` |

### Ground Truth Labeling

Each fixture specifies:
- `relevant_symbols`: list of qualified names that a developer would need to see
- `critical_symbols`: subset that are essential (used for MRR scoring)
- `difficulty`: easy / medium / hard
- `reasoning`: why these symbols are relevant (prevents stale fixtures)

### Validation

The `validate-fixtures` tool (`scripts/validate-fixtures.go`) verifies:
1. Every symbol in ground truth exists in the indexed database
2. Qualified names resolve to exactly one node (no ambiguity)
3. Match rate >= 95% (Run 7 established this threshold)

Fixtures with unresolvable symbols are flagged and corrected or removed.

## Corpus Selection

### Selection policy

**No cherry-picking.** Every repository attempted has been included in the corpus.
The only exclusion is Homebrew (Ruby), where fixture validation is incomplete
(tasks exist in `tasks-pending/` but ground truth symbols have not been verified
against the graph). Homebrew is indexed and will be activated when fixtures are
validated. No repository has ever been excluded based on knowing's performance
on it.

Repos are selected for language diversity and community recognition, not for
any property that would favor knowing's architecture. Several repos where knowing
performs poorly remain in the corpus: VS Code (P@10=0.168, dense graph seed
competition), Django (P@10=0.183, 42% vocabulary gap zero rate), and Kubernetes
(P@10=0.168, massive graph). Saleor (a Django app, not the framework itself) was
added in v0.13.0 to validate that equivalence classes generalize to application code.

### Criteria

- Public, well-known repositories (reproducible by anyone)
- Multiple languages (Go, Python, TypeScript, Rust, Java, C#, Ruby)
- Range of sizes (14K LOC Flask to 3.5M LOC Kubernetes)
- Pinned to specific versions (deterministic indexing, see `corpus/MANIFEST.yaml`)
- No knowing's own repository (avoids self-measurement bias)
- No exclusions based on performance (all attempted repos are included)

### Current Corpus (16 repos, 308 tasks, 8 languages)

| Repo | Language | LOC | Nodes | Edges | Tasks | Why |
|------|----------|-----|-------|-------|-------|-----|
| Kubernetes | Go | 3.5M | 242K | 705K | 19 | Massive Go monorepo, enriched with gopls |
| VS Code | TypeScript | 1M | 552K | ~4.4M | 19 | Large TS, extension architecture |
| Django | Python | 300K | 55K | ~370K | 33 | Large Python, deep inheritance (ORM) |
| Terraform | Go | 500K | 99K | ~184K | 20 | Go, provider plugin architecture, enriched with gopls |
| Kafka | Java | 800K | ~105K | ~1.3M | 19 | Enterprise Java, deep class hierarchies |
| Cargo | Rust | 150K | 81K | ~137K | 19 | Rust, complex module system |
| Caddy | Go | 75K | 23K | 47K | 20 | Go web server, enriched with gopls |
| FastAPI | Python | 30K | 18K | 51K | 20 | Modern Python, type-annotated, enriched with pyright |
| Ocelot | C# | 50K | 17K | 53K | 20 | C# API gateway, enriched with csharp-ls |
| Saleor | Python | 180K | 34K | 285K | 11 | Django e-commerce app (framework-USING validation) |
| Flask | Python | 15K | ~6K | ~9K | 19 | Small, well-structured, dense class hierarchy |
| Rails | Ruby | 200K | ~40K | ~200K | 20 | Ruby on Rails framework, enriched with ruby-lsp |
| Ripgrep | Rust | 50K | ~15K | ~40K | 20 | Rust CLI tool, regex engine |
| Spark Java | Java | 14K | ~1.4K | ~1.4K | 20 | Small Java web framework |
| Jekyll | Ruby | 30K | ~14K | ~35K | 20 | Ruby static site generator |
| Cross-cutting | Mixed | - | - | - | 9 | Multi-repo/multi-language tasks |

## Adapter Interface

Each system implements:

```go
type Adapter interface {
    Name() string
    Available() bool
    RetrieveContext(task benchtype.Task, repoPath string, budget int) ([]benchtype.Result, error)
}
```

- `Available()` checks if the system is installed and configured
- `RetrieveContext()` returns ranked symbols for a task description within a token budget
- Systems that fail or timeout on a task score 0 for that task (not excluded)

## Metrics

### Primary (reported in all runs)

#### P@10 (Precision at 10)

**Formula:** (number of relevant symbols in top-10 results) / 10

**Interpretation:** "Of the 10 symbols you showed me, how many did I actually need?"
This is the headline metric because it directly measures whether the context is useful.
A developer reading 10 symbols wants most of them to be relevant, not noise.

- P@10 = 0.40 means 4 of 10 results are relevant (good for hard tasks)
- P@10 = 0.80 means 8 of 10 are relevant (excellent)
- P@10 = 0.00 means the system completely missed (none of the top-10 are useful)

**Why P@10 and not P@5 or P@20:** The context engine returns ~5-30 symbols depending
on budget. 10 is the sweet spot: enough to measure ranking quality without rewarding
systems that dump everything.

#### R@10 (Recall at 10)

**Formula:** (number of relevant symbols in top-10 results) / (total relevant symbols for this task)

**Interpretation:** "Of all the symbols I needed, how many did you find in 10 results?"
High recall means the system doesn't miss important symbols. Low recall means you'd
need to request more context or search manually.

- R@10 = 1.00 means all ground truth symbols appeared in top-10 (perfect recall)
- R@10 = 0.50 means half the ground truth was found
- Hard tasks with 8+ ground truth symbols rarely achieve R@10 > 0.50 in 10 slots

**Tension with precision:** A system can achieve high recall by returning everything
(Repomix strategy: dump 300K tokens, R=100%, P~0%). The P@10/R@10 pair prevents gaming.

#### NDCG@10 (Normalized Discounted Cumulative Gain)

**Formula:** DCG@10 / idealDCG@10, where DCG = sum(relevance_i / log2(i+1))

**Interpretation:** "Are the most relevant symbols ranked first?" NDCG penalizes
systems that find relevant symbols but rank them below irrelevant ones. A system
with P@10=0.40 but all 4 relevant results in positions 1-4 scores higher NDCG than
one with the same 4 results scattered at positions 2, 5, 7, 9.

- NDCG = 1.0 means perfect ranking (all relevant symbols at the top)
- NDCG > P@10 indicates good ranking (relevant results clustered at top)
- NDCG < P@10 indicates poor ranking (relevant results buried below noise)

#### MRR (Mean Reciprocal Rank)

**Formula:** 1 / (rank of first relevant symbol)

**Interpretation:** "How quickly do I get something useful?" For an agent that reads
results sequentially, MRR measures how many results it must scan before finding
something relevant.

- MRR = 1.00 means the first result is relevant (ideal for agents)
- MRR = 0.50 means the first relevant result is at position 2
- MRR = 0.10 means you have to read 10 results to find anything useful

**Why MRR matters for agents:** Claude Code reads context top-to-bottom. If the first
symbol is the right one, the agent can start working immediately. Low MRR means the
agent wastes context window on irrelevant symbols before finding what it needs.

### Secondary

| Metric | Formula | Interpretation |
|--------|---------|----------------|
| Token efficiency | relevant_symbols / tokens_consumed | Higher = more signal per token. Penalizes systems that return verbose context. |
| Latency | wall-clock ms from query to response | End-to-end including graph traversal, RWR, scoring, formatting. |
| Failure rate | tasks with errors / total tasks | Systems that crash or timeout score 0 (not excluded). High failure rate indicates fragility. |

### How to Read the Results Table

```
| System  | P@10  | R@10  | NDCG@10 | MRR   | TokenEff | Latency(ms) | Tasks |
|---------|-------|-------|---------|-------|----------|-------------|-------|
| knowing | 0.226 | 0.396 | 0.369   | 0.423 | 0.0023   | 2582        | 117   |
```

Reading this row: knowing returns relevant symbols 22.6% of the time in its top-10
(P@10). It finds 39.6% of all ground truth symbols within 10 results (R@10). Its
ranking is decent (NDCG 0.369). On average, the first relevant symbol appears around
position 2-3 (MRR 0.423). It uses about 0.0023 relevant symbols per token consumed.
Average latency is 2.6 seconds across all 10 repos (dominated by Kubernetes at ~5s).

### Interpreting Differences Between Systems

- **P@10 difference of 0.05+** is meaningful (5% more results are relevant)
- **MRR difference of 0.10+** means reaching the first useful result 1 position sooner
- **Effect size (d) > 0.5** means the difference is reliably detectable across tasks
- **p < 0.01** means the difference is unlikely to be random noise

A system with lower P@10 but higher MRR might be better for agents (gets the first
answer fast). A system with higher P@10 but lower MRR is better for comprehensive
understanding (more relevant results overall, just not ranked first).

## Statistical Methods

### Paired Wilcoxon Signed-Rank Test

Compares two systems on the same tasks. Non-parametric (no normality assumption).
Null hypothesis: median difference = 0.

- p < 0.05: statistically significant
- p < 0.001: highly significant (reported as p<0.0001 when below float precision)

### Effect Size (Cohen's d)

| d | Interpretation |
|---|----------------|
| 0.2 | Small |
| 0.5 | Medium |
| 0.8 | Large |
| 1.0+ | Very large |

### Confidence Intervals

Bootstrap with 10K resamples. Reports 95% CI for the difference between systems.

## Experiment Protocol

### Testing workflow

Django is the acid test repo for retrieval experiments:
- 33 tasks (largest single-repo fixture set)
- 42% zero-rate problem (vocabulary gaps), so improvements that move Django are structural
- Where adaptive seeds showed +14.2%, bidirectional inheritance showed -2.5%, gap injection +3.2%

**Three-step protocol:**

1. **Django only, no embeddings (~30s):** quick signal on structural changes
   ```bash
   BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 10m
   ```
2. **Django with embeddings (~7min):** confirms interaction with re-ranker
   ```bash
   BENCH_EMBEDDINGS=1 BENCH_REPOS=django BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m
   ```
3. **Full corpus with embeddings (~90min):** only if Django moves positively
   ```bash
   BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0
   ```

If Django is neutral or negative, don't run the full corpus. If Django is positive,
the full corpus confirms whether it generalizes or gets absorbed by run variance.

**Important:** Not all experiments affect Django. Check graph density first:
```bash
sqlite3 <repo>/.knowing/graph.db "SELECT COUNT(*) FROM edges; SELECT COUNT(*) FROM nodes;"
```
If the experiment only affects dense graphs (like adaptive alpha), test on dense repos
(flask, cargo, kafka) instead of Django.

### Output capture

Always capture full output to a file (`2>&1 | tee /tmp/file.log` or `> /tmp/file.log 2>&1`).
Never pipe through `tail` or `grep` as it loses early output (embedding progress, task counts).

### Corpus DB safety

Corpus DBs (`corpus/repos/<repo>/.knowing/graph.db`) are gitignored and cannot be
recovered from git. Enrichment takes hours to rebuild. Never modify them in place
for experiments.

**Full experiment workflow:**
1. Keep a master backup set untouched (e.g., `cp *.db /tmp/corpus-backup/`)
2. Copy FROM the master to a working path for the experiment
3. Run enrichment/modifications on the working copy only
4. Checkpoint WAL: `sqlite3 <copy>.db "PRAGMA wal_checkpoint(TRUNCATE);"`
5. Delete stale SHM/WAL at destination: `rm -f <dest>.db-shm <dest>.db-wal`
6. Swap the checkpointed copy into the benchmark path
7. Clear test cache: `go clean -testcache` (stale binary causes phantom regressions)
8. Test the problem repo first (saves 20+ min per iteration vs full corpus)
9. If positive on problem repo, run full corpus
10. After the experiment, restore the original from the master backup
11. If experiment approved, working copy becomes the new master backup

Skipping step 4 causes "database disk image is malformed" (WAL not flushed).
Skipping step 5 causes malformed errors even with a clean main file (stale SHM).
Skipping step 7 causes phantom regressions from cached test binaries with old code.

### TestCrossSystemRound2

The harness includes a `TestCrossSystemRound2` test that runs automatically after the
main benchmark. It re-runs all tasks with task memory from round 1, measuring the
compounding effect. Round 2 reports cold-start and warm-start P@10 with the delta.
Session 17 measured +1.9% P@10 from compounding on 237 tasks (with gap-fill + nomic model).

## Regression Detection

### How regressions are caught

1. **Per-repo tracking:** Each run records P@10 per repo. A >20% drop in any single
   repo indicates a regression even if the aggregate is stable.
2. **Channel contribution logging:** Debug mode logs which retrieval channel (tiered,
   BM25, equivalence, vector) contributed each seed candidate.
3. **Run-over-run comparison:** FINDINGS.md records every run with delta from previous.

### Run 22 case study (equivalence channel noise)

The P@10 dropped from 0.230 to 0.101 over several commits. The regression was invisible
in code review because:
- No unit test checked channel result counts
- The aggregate masked per-repo drops (Flask dropped 0.321->0.20 but k8s held steady)
- Experimental WIP commits accumulated without benchmark re-runs

**Prevention measures identified:**
- Channel balance assertion (no channel >2x others)
- Per-repo baseline comparison in CI
- Equiv expansion safety gate (benchmark before/after adding classes)

## Known Limitations

### Overfitting risk

The same 222 fixtures are used across runs. Improvements may overfit to
these specific tasks. Mitigation: fixtures are diverse (12 repos, 3 tiers, 7
languages) and the cross-system comparison with competitors uses the same fixtures
(so overfitting would equally benefit competitors). Session 17 added 60 new
fixtures (caddy, ocelot, fastapi) to reduce overfitting to the original set.

### Cold-start only

The benchmark measures cold-start retrieval. Real usage benefits from feedback
compounding (+20pp after 5 rounds in feedback-loop bench). The benchmark
understates knowing's value for repeated users.

### Ground truth incompleteness

Not all relevant symbols are labeled. A system that returns an unlabeled-but-useful
symbol scores 0 for that position. This creates false negatives. Mitigation: periodic
fixture review and expansion.

### Token budget interaction

All systems receive a 5000-token budget. Systems that return fewer tokens may have
higher precision but lower recall. The token efficiency metric accounts for this
but P@10 does not.

### Small-graph vs large-graph behavior

knowing's pipeline behaves differently at different graph scales:
- Small graphs (<3000 nodes): RWR converges to near-uniform scores. Channel
  balance is critical (Run 22 finding).
- Large graphs (>30K nodes): RWR differentiates well. More edges = better recall.
  The dominant factor is graph connectivity, not channel balance.

Benchmarking only on small repos (Flask) would miss large-graph advantages.
Benchmarking only on large repos (k8s) would miss small-graph pathologies.
The corpus covers both.

## Reproducing Results

### From scratch (external reproducer)

The corpus is fully reproducible from pinned commit hashes. No pre-built
artifacts are required.

```bash
# 1. Build knowing
GOWORK=off go build -o /usr/local/bin/knowing ./cmd/knowing/

# 2. Set up the corpus (clone repos at exact commits, build graph DBs)
cd bench/cross-system/corpus
./corpus-setup.sh clone     # ~5 min, clones 15 repos at pinned commits
./corpus-setup.sh index     # ~5 min, tree-sitter extraction only
./corpus-setup.sh enrich    # ~2 hours, requires language servers (optional)
./corpus-setup.sh embed     # ~30 min, pre-embeds vectors (optional)

# 3. Verify corpus matches manifest
./corpus-setup.sh verify

# 4. Run the benchmark
cd ../../..
BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off \
  go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0
```

**Without enrichment or embeddings:** The corpus DBs are pre-enriched with LSP.
Embeddings are confirmed neutral on cold start (session 23). Task memory is
disabled in the benchmark adapter (session 23, was contaminating measurements).
Official P@10 = 0.278 (honest cold-start, no task memory, no embeddings).

**Corpus manifest:** `corpus/MANIFEST.yaml` records the exact commit hash,
repository URL, expected node/edge/embedding counts, and enrichment server
for every repo. The `corpus-setup.sh verify` command checks that the local
corpus matches the manifest.

### Quick run (already have corpus)

```bash
# Full benchmark (all systems, all repos)
BENCH_EMBEDDINGS=1 BENCH_ADAPTERS=knowing GOWORK=off \
  go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 0

# Single system, single repo
BENCH_REPOS=flask BENCH_ADAPTERS=knowing GOWORK=off \
  go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 5m

# With competitors
uv venv /tmp/aider-bench --python 3.11
source /tmp/aider-bench/bin/activate
uv pip install aider-chat
GOWORK=off go test ./bench/cross-system/ -run TestCrossSystem -v -timeout 30m \
  -bench.adapters=knowing,aider
```

### Determinism

Results are deterministic for a given index state. Clear context caches before
re-running if testing pipeline changes:

```bash
for db in bench/cross-system/corpus/repos/*/.knowing/graph.db; do
  sqlite3 "$db" "DELETE FROM graph_notes WHERE key = 'context_pack'"
done
```

### What affects reproducibility

| Factor | Impact | How to control |
|--------|--------|----------------|
| Repo commit | High | Pinned in MANIFEST.yaml, enforced by corpus-setup.sh |
| LSP enrichment | High | Same language server version produces same edges |
| Embedding model | Medium | nomic-embed-text-v1.5 pinned, auto-downloads |
| ONNX runtime | Low | Float32 determinism across platforms |
| SQLite FTS5 | Low | BM25 ranking is deterministic for same index |
| Go version | None | No floating-point dependence in tree-sitter extraction |
