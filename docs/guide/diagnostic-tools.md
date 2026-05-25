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
