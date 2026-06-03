# Retrieval Diagnostic Tools

Tools for investigating retrieval quality issues, running ablation studies,
and understanding how the pipeline behaves on different graph densities.

## Interactive Debugging Tools (session 23)

Three CLI tools for diagnosing individual task failures. These are the
fastest path to understanding why a task scores zero.

### debug-seeds: seed pipeline visibility

Shows the full seed selection pipeline for a task: keywords extracted,
path terms, BM25 results, and final ForTask top 10 with scores.

```bash
knowing debug-seeds -task "add a custom validator" -db <path> <repo>
```

**Output sections:**
1. Keywords (Primary, Components, Compounds)
2. Path terms
3. BM25 results (FTS query + top 15 matches)
4. Path boost analysis (if active)
5. Final ForTask top 10 with scores

**Use when:** A task scores zero and you want to see WHERE the pipeline
fails: empty keywords? BM25 returning wrong symbols? Right seeds but
wrong walk results?

### debug-fts: raw FTS5 query probe

Runs a raw FTS5 query against the node index. Test query formulations
without running the full pipeline.

```bash
knowing debug-fts -query "transform* OR destroy*" -db <path> [-limit N]
```

**Supports:** prefix matching (`term*`), column targets (`symbol_name:"term"`),
OR/AND operators, quoted phrases (`"email validator"`).

**Use when:** You want to verify what BM25 actually returns for specific
query terms, or test whether a symbol is findable by FTS at all.

### debug-walk: RWR walk visualization

Shows the RWR walk from specific seed nodes: edge types on seeds (1-hop),
nodes reached, score distribution, and top-N ranking.

```bash
knowing debug-walk -seed "DestroyEdgeTransformer" -db <path> [-top N] [-alpha 0.2]
```

**Output sections:**
1. Matched seed nodes
2. Edge type breakdown per seed (outgoing/incoming by type)
3. RWR walk results (nodes reached, top-N by score)
4. Score distribution (max, min, top-10 mass, total mass)

**Use when:** Seeds are correct but the walk doesn't reach the expected
targets. Shows whether the graph structure supports propagation.

### bench-task: single-task benchmark

Runs a single benchmark task and shows P@10 with per-symbol HIT/MISS
analysis. Shows where each ground truth symbol ranks.

```bash
knowing bench-task -task "terraform-hard-004" [-corpus path] [-budget N]
```

**Output sections:**
1. Task metadata (repo, tier, description, ground truth)
2. Top 10 retrieved with HIT/MISS markers
3. P@10 score
4. Ground truth analysis: FOUND (rank N) or MISSING for each symbol
5. Recall over the full result set

**Use when:** You want to understand a specific task's performance without
running the full benchmark. Essential for the zero-task audit cycle:
audit zeros -> add equiv classes -> verify with bench-task -> run repo.

### debug-feedback: implicit feedback inspector

Shows feedback records for a symbol: positive/negative counts, per-cluster
breakdown, and score. Diagnose why a symbol is being demoted or boosted.

```bash
knowing debug-feedback -db <path>                     # all feedback
knowing debug-feedback -db <path> -symbol QuerySet     # filter by symbol name
knowing debug-feedback -db <path> -min-count 3         # only high-activity symbols
```

**Output:**
```
=== Feedback Records ===
Filter: symbol="QuerySet"  min-count=1

SYMBOL                                              +     -  TOTAL  SCORE  CLUSTERS
------                                              --    --  -----  -----  --------
QuerySet.annotate                                    3     5      8   0.38         2
QuerySet.filter                                      7     2      9   0.78         3

=== Per-Cluster Breakdown ===
CLUSTER                                                             +     -  TOTAL
-------                                                             --    --  -----
a1b2c3d4e5f6...                                                      2     3      5
f7e8d9c0b1a2...                                                      1     2      3
```

**Use when:** Understanding why a symbol ranks differently across sessions.
Diagnosing cross-task interference (fixed by per-cluster scoping).

### debug-equiv: equivalence class inspector

Shows which equivalence classes match a task description, from all three
sources: hand-curated, graph-derived, and learned vocabulary associations.

```bash
knowing debug-equiv -task "checkout flow" -db <path> <repo>
```

**Output sections:**
1. Source 1: Hand-curated matches (concept, phrases, targets, weight, language)
2. Source 2: Graph-derived (info about tiered seeds feeding graph alias generation)
3. Source 3: Learned vocab associations (keyword -> symbol with count)
4. Keywords extracted (primary, all, repo language)

**Use when:** Debugging why an equiv class did or didn't fire. Understanding
which source (curated vs graph vs learned) contributed specific symbols.
Essential for the zero-task audit cycle.

### debug-pack: packing decision inspector

Shows which symbols were packed into the token budget and why. Displays
density ranking, token cost, proximity factor, and file distribution.

```bash
knowing debug-pack -task "checkout flow" -db <path> <repo>
knowing debug-pack -task "checkout flow" -db <path> -budget 3000 <repo>
```

**Output:**
```
=== Packing Debug ===
Task: "checkout flow"
Budget: 5000 tokens
Packed: 42 symbols, 4891 tokens
PackRoot: a1b2c3d4e5f6...

--- Packed Symbols (by density rank) ---
RANK  SYMBOL                                         SCORE   COST  DENSTY    RWR
----  ------                                         -----   ----  ------    ---
   1  Order                                          0.903     85  0.0106  0.950
   2  Order.can_cancel                               0.845     42  0.0201  0.870
   3  CheckoutLine                                   0.761     63  0.0121  0.820

Budget: 4891/5000 used (109 remaining, 98% utilization)

File distribution (8 files):
  12  order/models.py
   8  checkout/views.py
```

**Use when:** Understanding why a specific symbol was included or excluded.
Diagnosing budget allocation issues. Comparing packing decisions between
enriched and unenriched DBs.

### debug-vocab: vocabulary association inspector

Shows learned keyword -> symbol associations from the `vocab_associations`
table. These are recorded when agents use symbols after `context_for_task`
queries, building automatic equivalence classes over time.

```bash
knowing debug-vocab -db <path>                    # show all associations
knowing debug-vocab -db <path> -keyword checkout   # filter by keyword
knowing debug-vocab -db <path> -min-count 2        # only confirmed (count >= 2)
knowing debug-vocab -db <path> -top 100            # show more results
knowing debug-vocab -task "fix email validation"   # preview vocab filter (no DB needed)
```

**Vocab filter preview** (`-task` flag): shows which keywords from a task
description would pass the noise filter and be recorded as vocab associations.
Useful for debugging why a keyword isn't creating learned associations.

**Output:**
```
=== Learned Vocabulary Associations ===
Filter: keyword=""  min-count=1

KEYWORD               SYMBOL                          COUNT
-------               ------                          -----
checkout              can_cancel                          3
checkout              Order                               2
migration             MigrationLoader                     2

Total: 3 associations across 2 keywords
```

**Use when:** Understanding what the system has learned from agent usage.
Debugging false associations. Verifying that vocabulary expansion is
recording the expected keyword -> symbol mappings.

## Measurement Protocol (session 23)

**CRITICAL:** Clear task memory before any A/B comparison. Task memory
persists in corpus DBs and inflates measurements over time.

```bash
# Clear task memory from all corpus DBs
for db in bench/cross-system/corpus/repos/*/.knowing/graph.db; do
  sqlite3 "$db" "DELETE FROM task_memory;"
done

# Clear test cache (stale binaries cause phantom results)
go clean -testcache

# Run benchmark
BENCH_ADAPTERS=knowing GOWORK=off go test ./bench/cross-system/ \
  -run "^TestCrossSystem$" -v -timeout 0
```

Task memory is disabled in the benchmark adapter since session 23.
The clearing step is a safety net for any accumulated state.

## Benchmark-Only Tools (bench/cross-system/cmd/)

These are developer-only utilities not shipped in the knowing binary.

### failure-analysis: categorize P@10 misses

```bash
go run ./bench/cross-system/cmd/failure-analysis --repo <name> [--task <id>]
```

Categorizes every ground truth miss into: `same_package`, `related_name`,
`test_symbol`, `noise`. Shows ground truth vs returned for each task.

### validate-fixtures: verify ground truth exists in graph

```bash
go run ./bench/cross-system/cmd/validate-fixtures
```

Checks that all ground truth symbols in task fixtures exist in the
corresponding repo's graph DB. Catches stale fixtures after re-indexing.

### session-bench: session-level benchmark runner

```bash
go run ./bench/cross-system/cmd/session-bench
```

Runs benchmarks with session-specific configuration.

## Query-Time Edge Exclusion

Exclude specific edge types from the RWR walk without reindexing. Filters
edges at BFS expansion time and adjacency map construction.

```bash
# Exclude similarity edges (test if they cause dilution)
BENCH_EXCLUDE_EDGES=similar_to BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v

# Exclude multiple edge types
BENCH_EXCLUDE_EDGES=similar_to,type_hint_of BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v

# Test with only core edges (calls + imports)
BENCH_EXCLUDE_EDGES=similar_to,type_hint_of,co_tested_with,contains,member_of,authored_by \
  BENCH_ADAPTERS=knowing go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v
```

**How it works:** Sets `knowingctx.ExcludeEdgeTypes` (package-level variable). Checked
in both the cached adjacency BFS path and the fallback per-node BFS path. Edges of
excluded types are skipped during frontier expansion AND excluded from the final
adjacency maps used by RWR iteration.

**Use cases:**
- Identify which edge types cause dilution on specific repos
- Run ablation studies (which edges are load-bearing for P@10)
- Quick hypothesis testing without reindexing (seconds vs minutes)

## BFS Depth Control

Limit how deep the BFS expansion goes when building the adjacency map for RWR.

```bash
# Depth 2 (only direct neighbors and their neighbors)
BENCH_BFS_DEPTH=2 BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v

# Depth 3 (default is 4)
BENCH_BFS_DEPTH=3 BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v
```

**How it works:** Sets `knowingctx.BFSMaxDepth`. Both the cached and fallback BFS paths
respect this. Lower depth means fewer nodes enter the RWR walk, which concentrates
probability mass on nodes closer to seeds.

**Use cases:**
- Test if dense-graph dilution is from walk depth (reaching the entire graph)
- Find optimal depth for specific repo sizes

## Index-Time Edge Type Filter

Only generate specific edge types during indexing. Skips both extraction-time
edges and post-processing steps (similarity, co-tested, contains, etc.).

```bash
# Index with only calls + imports (fastest, minimal graph)
knowing index --edge-types calls,imports --no-enrich --skip-blame /path/to/repo

# Index with structural edges but no similarity (much faster on large repos)
knowing index --edge-types calls,imports,implements,extends,overrides,references,contains,member_of,type_hint_of \
  --no-enrich --skip-blame /path/to/repo

# Full index (default, all edge types)
knowing index /path/to/repo
```

**How it works:** `Indexer.EdgeTypes` map filters edges at batch-write time and
guards each post-processing step (inheritance propagation, interface propagation,
contains generation, similarity computation, co-tested generation).

**Use cases:**
- Fast iteration during development (skip expensive similarity computation)
- Ablation: which edge types are needed for P@10 on a specific repo
- Debug dilution: index progressively with more edge types, measure at each step

## Type-Seed Preference Override

Force type/interface/class nodes to be preferred as RWR seeds, regardless of
graph density. Useful for testing whether seed kind matters on a specific repo.

```bash
# Manual type-seed preference (auto-enables on >40K nodes)
BENCH_PREFER_TYPE_SEEDS=1 BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v
```

**How it works:** Sets `knowingctx.PreferTypeSeeds = true`. After RRF fusion,
candidates are reordered to place type/interface/class nodes before
method/function nodes. RWR seeds are then selected from this reordered list.
Types make better seeds because their `contains` edges reach all their methods.

**Use cases:**
- Verify that dense-graph dilution is from seed kind (methods competing with types)
- Compare type-seeded vs method-seeded results on any graph density
- Confirm whether auto-detection threshold (40K nodes) is appropriate for a repo

## Combining Tools

The env vars compose. You can use index-time filtering for the DB and query-time
exclusion for rapid testing within a single indexed DB:

```bash
# Index with everything (one-time, slow)
knowing index --no-enrich --skip-blame /path/to/repo

# Then rapid-fire test different configurations (fast, no reindex)
BENCH_EXCLUDE_EDGES=similar_to BENCH_BFS_DEPTH=3 BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 30m -v
```

## Session 24-27 Diagnostic Env Vars

Env vars added in sessions 24-27 for specific ablation and feature testing:

| Env Var | Default | What it controls |
|---------|---------|-----------------|
| `BENCH_LSP_EDGE_WEIGHT` | `0.3` | Weight multiplier for LSP-enriched edges in RWR. Attenuates enrichment centrality inflation. Sweep: 0.1-1.0. |
| `BENCH_PROXIMITY_EXP` | `0.3` | Proximity exponent for packing density. Higher = more aggressive proximity preference. Sweep: 0.1-0.9. |
| `BENCH_FEEDBACK_WEIGHT` | `none` | Feedback confidence weighting mode: `none` (raw), `sqrt` (symmetric), `linear` (steep), `asym` (full positives, sqrt negatives). |
| `BENCH_IMPLICIT_FEEDBACK` | `0` | Enable implicit feedback (noise demotion) in benchmarks. Set to `1` to activate. |
| `BENCH_COMPOUND_ROUNDS` | `5` | Number of rounds for `TestCompounding`. |
| `BENCH_FOCUSED_SEEDS` | `1` | Enable focused seed selection (cluster by package path). Set to `0` to disable for A/B testing. |
| `BENCH_PACK_STRATEGY` | `density` | Packing algorithm: `density` (default), `file-grouped`, `top-k`. |
| `BENCH_RWR_CACHE` | `0` | Enable RWR result caching in benchmarks. Off by default for honest measurement. |

These compose with the existing `BENCH_EXCLUDE_EDGES`, `BENCH_BFS_DEPTH`, `BENCH_PREFER_TYPE_SEEDS`, `BENCH_REPOS`, and `BENCH_ADAPTERS` env vars.

## Repo-Specific Benchmarking

```bash
# Single repo (fast feedback)
BENCH_REPOS=vscode BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 10m -v

# Multiple repos
BENCH_REPOS=vscode,kubernetes BENCH_ADAPTERS=knowing \
  go test ./bench/cross-system/ -run TestCrossSystem -timeout 15m -v
```

## Failure Analysis

Categorizes every P@10 ground truth miss into actionable buckets.

```bash
# Full failure analysis across all repos
GOWORK=off go test ./bench/cross-system/ -run TestFailureAnalysis -timeout 30m -v

# Single repo
BENCH_REPOS=vscode GOWORK=off go test ./bench/cross-system/ -run TestFailureAnalysis -timeout 10m -v
```

**Categories:**
- `matched`: ground truth symbol appeared in top-10 (not a miss)
- `ranked_low`: reachable via RWR but ranked outside top-10 (ranking problem)
- `unreachable`: no RWR path exists from any seed to this symbol (graph connectivity problem)
- `not_in_db`: symbol doesn't exist in the indexed graph (extraction gap)
- `no_seeds`: keyword extraction produced zero seeds (should never happen)

**When to use:** After any retrieval change, run failure analysis to understand
WHERE the improvement/regression happened. A change that moves symbols from
`unreachable` to `ranked_low` is structural progress (new paths). A change that
moves symbols from `ranked_low` to `matched` is ranking progress (better scoring).

## Parameter Sweep

Tests all tunable parameters with zero variance expected (proves reachability-determination).

```bash
# Full 32-config sweep (slow, ~60 min)
GOWORK=off go test ./bench/cross-system/ -run TestParameterSweep -timeout 60m -v

# Quick validation (6 configs)
BENCH_REPOS=flask,django GOWORK=off go test ./bench/cross-system/ -run TestParameterSweep -timeout 30m -v
```

**When to use:** After any structural change, verify that parameter tuning is still
irrelevant. If a code change makes parameter tuning START mattering, that's a signal
the change introduced a non-reachability-dependent path (which might be fragile).

## Known Patterns

### Dense Graph Dilution (session 14)

**Symptom:** P@10 drops when more nodes/edges are added to the graph, even though
the correct symbols are still present and reachable.

**Root cause:** BM25/tiered search returns more candidates for the same keywords.
RRF fusion picks different (worse) seeds. RWR then walks from wrong starting points.

**Diagnostic sequence:**
1. Run with `BENCH_EXCLUDE_EDGES` for each edge type. If none recovers, it's not edges.
2. Run with `BENCH_BFS_DEPTH=2`. If no recovery, it's not walk depth.
3. Check keyword extraction output for the failing task. If all keywords are generic
   single words ("code", "action", "trigger"), the problem is seed specificity.
4. Count FTS matches for each keyword (`SELECT COUNT(*) FROM nodes WHERE qualified_name
   LIKE '%keyword%'`). If any keyword matches >1000 nodes, it's too generic for the
   graph density.

**Known triggers:**
- Correct TS extraction (export_statement fix): 43K -> 87K nodes (session 14)
- k8s staging module inclusion: 117K -> 253K nodes (session 12)
- LSP enrichment: adds 42K edges from pyright (session 13)

**Fix approaches (shipped):**
- **Self-adapting type-seed preference** (PreferTypeSeeds): on dense graphs (>40K nodes), automatically prefer type/interface/class nodes as RWR seeds. VS Code +44%. Available as `BENCH_PREFER_TYPE_SEEDS=1` manual override.
- **Phrase-boosted BM25**: adjacent word bigrams as FTS5 phrase queries ("code actions" as quoted phrase).
- **Concept thesaurus**: ~80 domain clusters expand BM25 queries with related code vocabulary.

**Under investigation:**
- Per-package BM25: smaller FTS index per scope restores IDF discrimination
- Local embeddings: vector similarity is density-independent (see `docs/proposals/pure-go-embeddings.md`)

### Enrichment Dilution (sessions 12-13, resolved session 25)

**Symptom:** LSP enrichment adds correct edges but P@10 drops on some repos.

**Root cause (session 25):** Not phantom probability sinks. Not packing density. The cause
is enriched real nodes gaining inflated centrality: LSP discovers edges that connect
webhook/event handler symbols to many other symbols, inflating their RWR score above the
implementation symbols that ground truth expects.

**Diagnostic:** Compare P@10 with `-no-enrich` vs enriched. If unenriched is better,
enrichment is diluting. Check which symbols gained the most centrality.

**Fix (shipped session 25):** LSP edge weight attenuation. Edges with `lsp_resolved`
provenance receive 0.3x weight in the RWR walk (override with `BENCH_LSP_EDGE_WEIGHT`).
This prevents enrichment from inflating centrality of framework wiring symbols.
Enriched saleor regression halved (from -23% to -11%). Full corpus: neutral.

**Current state:** Enrichment is strongly positive for retrieval when attenuated.
Go: k8s 0.000 -> 0.232, terraform ~0.095 -> 0.275. Python: +0.040.
The value comes from phantom nodes + type_hint_of edges creating new reachability paths.

### Feedback BFS Flooding (session 13)

**Symptom:** Feedback compounding stops working or regresses P@10.

**Root cause:** Weight-0 edges (contains, member_of, authored_by) being traversed during
adjacency BFS expansion. These structural edges connect thousands of nodes that shouldn't
participate in the feedback-weighted walk.

**Diagnostic:** Check if `edgeWeights` has weight-0 entries that are NOT excluded from
BFS frontier expansion in `buildAdjacencyMap`.

**Fix:** Skip frontier expansion through weight-0 edges (already implemented in walk.go).

## Interpreting Results

**Edge exclusion has no effect:** The dilution is not from that edge type.
The walk already ignores it (weight-0 edges are never traversed).

**BFS depth has no effect:** The problem is upstream of the walk (seed selection,
BM25 quality, keyword extraction). The walk is correctly focused but starts from
wrong seeds.

**Both have no effect:** The problem is in the retrieval front-end (tiered search,
BM25, RRF fusion). The graph structure and walk are correct; the seeds are bad.

**Session 14 finding:** On VS Code (87K nodes), neither edge exclusion nor BFS
depth recovered P@10. Root cause confirmed: seed selection degrades on dense FTS
indexes because generic keywords match too many candidates.
