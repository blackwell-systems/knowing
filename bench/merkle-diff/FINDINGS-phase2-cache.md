# Phase 2 Cache Benchmark

Statistically robust end-to-end benchmarks for the subgraph cache, daemon
invalidation, and DiffOptions. Each measurement runs 10 times with 2 warmup
runs discarded; stats report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Nodes: 7250
- Edges: 25028

## Benchmarks

1. **ContextForTask_ColdVsWarm** — cold (cache miss, fresh engine) vs warm
   (cache hit). Each path measured 10 times with 2 warmup runs.
2. **RawCacheLookup_vs_FullWarm** — raw cache.Get() latency vs full warm
   ForTask path (shows serialization overhead separately).
3. **AgentSession_RealisticVariation** — two sessions: optimistic (exact
   repeats, high hit rate) vs realistic (query variations about the same
   topic, lower hit rate due to differing normalized keys).
4. **DaemonInvalidation** — diff + invalidate only (NOT including re-index,
   clearly labeled), plus end-to-end timing including re-index.
5. **DiffOptions_ChangeRates** — filtered vs unfiltered diff at 1%, 5%,
   10%, and 100% mutation rates.
6. **CacheDisabled_vs_Enabled** — absolute improvement baseline: same query
   with cache=nil vs with a primed cache.

## Performance Contracts

- Cache hit median < 5ms (test fails if violated)
- Daemon invalidation median < 1ms (test fails if violated)
- Cache speedup median > 20x (conservative floor, test fails if violated)

## Running

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestPhase2CacheBenchmark -timeout 300s
```
