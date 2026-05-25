# Retrieval Diagnostic Tools

Tools for investigating retrieval quality issues, running ablation studies,
and understanding how the pipeline behaves on different graph densities.

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

**Fix approaches (under investigation):**
- Phrase-aware BM25: adjacent word bigrams as FTS5 phrase queries
- Node-kind-aware seed selection: prefer types/interfaces over methods for dense graphs
- Per-package BM25: smaller FTS index per scope restores IDF discrimination
- Local embeddings: vector similarity is density-independent

### Enrichment Dilution (session 12-13)

**Symptom:** LSP enrichment adds correct edges but P@10 drops.

**Root cause:** Enrichment adds edges to already-well-connected nodes (pyright resolves
every call). These extra edges don't create new reachability; they just spread probability
mass further along existing paths.

**Diagnostic:** Compare P@10 with `--no-enrich` vs enriched. If unenriched is better,
enrichment is diluting.

**Fix:** Don't use enrichment for retrieval quality. Enrichment is for audit/confidence
(provenance upgrade 0.7 -> 0.9), not for retrieval.

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
