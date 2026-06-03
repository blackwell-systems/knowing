# Proposal: Incremental RWR (Merkle-Cached Walks)

## Problem

Every `ForTask` call runs a full RWR: BFS-load the reachable subgraph (4 hops),
iterate until convergence (~5-10 iterations), score all nodes. On cached adjacency
maps this takes ~2ms. On cold loads it takes 9+ seconds.

For repeated or similar queries against unchanged code, this is wasted work. The
seeds are the same, the graph is the same, so the scores are the same. We should
cache the scores and skip the walk entirely.

## Design

### Cache key

```
rwr_cache_key = hash("rwr\0" || sorted(seed_hashes) || sorted(relevant_package_roots))
```

- `seed_hashes`: the actual RWR seed set (after RRF fusion, focused selection, etc.)
- `relevant_package_roots`: SubgraphRoots for the packages containing the seeds.
  Computed from the hierarchical Merkle tree. When any seed's package changes,
  the cache key changes and the old entry expires structurally.

### Cache value

Serialized RWR result:
- `map[types.Hash]float64` (node -> score)
- `map[types.Hash]int` (node -> BFS distance)
- Compact binary format (same pattern as adjacency cache)

### Invalidation

Three levels, from cheapest to most expensive:

1. **Structural (automatic):** package root changes -> cache key changes -> miss.
   No explicit invalidation needed. This is the Merkle guarantee.

2. **TTL (safety net):** 1 hour default. Catches edge cases where the Merkle tree
   isn't updated (manual DB edits, etc.).

3. **Explicit:** `DeleteNote` with the RWR cache key on adjacency cache invalidation.
   When the adjacency cache is rebuilt (new index run), RWR caches are also cleared.

### Integration points

**In `RandomWalkWithRestartWeighted`:**

```go
// Before BFS/iteration:
cacheKey := computeRWRCacheKey(seeds, packageRoots)
if cached := getRWRCache(ctx, store, cacheKey); cached != nil {
    return cached.scores, cached.distances, nil
}

// After iteration:
putRWRCache(ctx, store, cacheKey, scores, distances)
```

**Package root computation:**

The hierarchical Merkle tree is built at index time and stored in the snapshot.
At query time, we need SubgraphRoots for the seed packages. Two approaches:

A. **Lazy computation (preferred):** compute package roots from the latest snapshot's
   hierarchical tree. The tree is already serialized in the snapshot table. Load it
   once per query, extract roots for seed packages. Cache the tree in memory for the
   session.

B. **Pre-computed:** store package roots in the adjacency cache alongside edges.
   When the adjacency cache is rebuilt, package roots are included. No additional
   snapshot lookup needed.

Option A is more correct (always uses the latest tree). Option B is faster (no
snapshot lookup). Start with A, optimize to B if the snapshot lookup is measurable.

### Persistence

RWR cache entries are stored in the `graph_notes` table (same as adjacency cache
and context packs). Key: `rwr_cache_key`. Value: binary-serialized scores + distances.

This means RWR caches survive process restarts. On the first query after restart,
if the code hasn't changed, the cached scores are still valid (Merkle-verified).

## Validation Experiments

### Experiment 1: Cache hit rate

**Objective:** measure what fraction of ForTask queries would hit a cached RWR result.

**Method:** run the cross-system benchmark (308 tasks), log seed sets and cache keys.
Count how many tasks produce the same cache key as a previous task (exact seed overlap).
Also count "near misses" (>80% seed overlap) that could benefit from incremental updates.

**Diagnostic tool:** `knowing debug-rwr-cache -db <path> -task "description"` that shows:
- Computed seeds
- Cache key
- Whether the cache would hit
- Package roots used in the key

### Experiment 2: Latency improvement

**Objective:** measure the latency reduction from cached RWR results.

**Method:** run ForTask twice with the same task description. First call is cold (builds
cache). Second call should hit cache. Measure:
- Cold latency (full BFS + iteration)
- Warm latency (cache hit, skip walk)
- Cache serialization/deserialization overhead

**Expected:** cold ~2ms (with adjacency cache), warm <0.1ms. The BFS and iteration
are skipped entirely on cache hit.

### Experiment 3: Correctness verification

**Objective:** prove that cached results produce identical P@10 to uncached results.

**Method:** run the cross-system benchmark twice: once with RWR cache enabled, once
disabled. P@10 must be identical (zero delta). Any difference indicates a cache
key that doesn't capture all relevant state.

### Experiment 4: Staleness detection

**Objective:** prove that code changes correctly invalidate cached walks.

**Method:**
1. Index a repo, run ForTask, populate RWR cache
2. Modify a file in the seed's package (add/remove an edge)
3. Re-index (updates the hierarchical tree)
4. Run the same ForTask again
5. Verify: cache miss (different package root), fresh walk, potentially different scores

### Experiment 5: Integration with vocab expiration

**Objective:** prove that SubgraphRoot computation (needed for RWR cache keys) also
enables vocab association expiration.

**Method:** after implementing RWR cache, wire SubgraphRoots into vocab recording
and lookup. Run the 10-round compounding test. Verify that late-round decay is
reduced (stale associations expire when their package changes).

## Implementation Plan

### Phase 1: Cache infrastructure (this session)
- [ ] `computeRWRCacheKey(seeds, packageRoots)` function
- [ ] `getRWRCache` / `putRWRCache` functions (notes table)
- [ ] Binary serialization for RWR results
- [ ] Integration in `RandomWalkWithRestartWeighted`
- [ ] Package root computation from latest snapshot

### Phase 2: Validation
- [ ] Experiment 1: cache hit rate on cross-system benchmark
- [ ] Experiment 2: latency measurement (cold vs warm)
- [ ] Experiment 3: P@10 correctness verification
- [ ] `debug-rwr-cache` diagnostic tool

### Phase 3: Vocab expiration (3c)
- [ ] Wire SubgraphRoots into vocab recording
- [ ] Wire SubgraphRoots into vocab lookup (Merkle filtering)
- [ ] 10-round compounding test with expiration

### Phase 4: Incremental updates (future)
- [ ] For near-miss cache keys (>80% seed overlap), update the cached scores
  incrementally rather than recomputing from scratch
- [ ] Delta-walk: only walk from new/changed seeds, merge with cached scores
  for unchanged seeds

## Expected Impact

- **Latency:** 2ms -> <0.1ms on repeated/similar queries (20x improvement)
- **Throughput:** MCP server handles higher query volume without CPU increase
- **`--watch` mode:** only changed packages trigger fresh walks; unchanged packages use cached scores
- **Moat:** requires content-addressed storage + hierarchical Merkle tree. Competitors using mutable graphs cannot implement this without a full architecture rewrite.

## Source Files

| File | Purpose |
|------|---------|
| `internal/context/walk.go` | RWR implementation, adjacency cache, BFS |
| `internal/cache/subgraph.go` | SubgraphCache (in-memory, Merkle-invalidated) |
| `internal/snapshot/hierarchical.go` | Hierarchical Merkle tree, SubgraphRoot |
| `internal/store/sqlite.go` | Notes table (persistent cache) |
