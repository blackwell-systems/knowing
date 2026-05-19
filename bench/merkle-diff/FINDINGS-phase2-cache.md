# Phase 2 Cache Benchmark

End-to-end benchmarks for the subgraph cache, daemon invalidation, and DiffOptions.

## Setup

- Repository: knowing (live codebase)
- Nodes: 2329
- Edges: 11570

## Results

Run the benchmark to see current numbers:

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestPhase2CacheBenchmark -timeout 120s
```

The benchmark measures:
1. **context_for_task cold vs warm**: first call (full retrieval) vs second call (cache hit)
2. **Agent session simulation**: 10 queries with repeats, measures cache hit rate
3. **Daemon invalidation latency**: DiffHierarchicalTrees + InvalidatePackages overhead per re-index
4. **DiffOptions speedup**: filtered vs unfiltered vs max-changes-capped diffs
