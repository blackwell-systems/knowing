# Phase 2 Cache Benchmark

Statistically robust end-to-end benchmarks for the subgraph cache, daemon
invalidation, and DiffOptions. Each measurement runs 10 times with 2 warmup
runs discarded. Stats report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Nodes: ~2,300
- Edges: ~11,600
- Method: 10 measurement runs, 2 warmup (discarded), per data point

## Results

### context_for_task: Cold vs Warm

| Task | Cold median | Warm median | Speedup |
|------|-----------|-----------|---------|
| "find all MCP tool handlers" | 126ms | 1.84ms | 68x |
| "find authentication and authorization code" | 135ms | 1.74ms | 77x |
| "find database connection and query code" | 125ms | 1.37ms | 91x |
| "find the context retrieval pipeline" | 129ms | 1.90ms | 68x |
| "find test scope computation" | 131ms | 1.31ms | 100x |

Cold stddev: ~1ms. Warm stddev: ~0.07ms. Both are stable across runs.

### Raw Cache Lookup vs Full Warm Path

| Operation | Median | What it includes |
|-----------|--------|-----------------|
| Raw cache.Get() | 42ns | Hash lookup only |
| Full warm ForTask | 1.72ms | cache.Get() + JSON unmarshal + format |
| Deserialization overhead | 1.72ms | JSON unmarshal dominates the warm path |

The cache lookup itself is nanoseconds. The 1.7ms warm cost is entirely serialization.

### Agent Session Simulation

| Session type | Queries | Hits | Misses | Hit rate |
|-------------|---------|------|--------|----------|
| Optimistic (exact repeats) | 10 | 6 | 4 | 60% |
| Realistic (query variations) | 10 | 2 | 8 | 20% |

Realistic queries vary wording about the same topic ("find MCP handlers" vs "find MCP tool registration" vs "find blast_radius MCP handler"). These produce different normalized keys, so they miss the cache. Exact repeats hit reliably.

### Cache Disabled vs Enabled

| Condition | Median | Speedup |
|-----------|--------|---------|
| Cache disabled (nil) | ~160ms | baseline |
| Cache enabled (primed) | ~1.7ms | **93x** |

### Daemon Invalidation

| Operation | Latency |
|-----------|---------|
| DiffHierarchicalTrees (diff only) | ~6us |
| InvalidatePackages (eviction only) | ~40ns |
| Total diff + invalidate | ~6us |
| End-to-end including re-index | ~149ms |

The diff + invalidate overhead per re-index is microseconds. The re-index itself (parsing, SQLite writes) dominates at ~149ms.

### DiffOptions by Change Rate

| Mutation rate | Unfiltered | Filtered (1 pkg) | Speedup |
|--------------|-----------|-------------------|---------|
| 1% | ~6us | ~3us | 2x |
| 5% | ~7us | ~3us | 2.3x |
| 10% | ~8us | ~3us | 2.7x |
| 100% | ~29us | ~6us | 4.9x |

MaxChanges cap (3 changes) at 100% mutation: ~3.5us (8.5x faster than unfiltered).

## Performance Contracts (enforced)

These assertions fail the test if violated:

| Contract | Threshold | Actual | Status |
|----------|-----------|--------|--------|
| Cache hit median | < 5ms | 1.7ms | PASS |
| Daemon invalidation median | < 1ms | 42ns | PASS |
| Cache speedup median | > 20x | 93x | PASS |

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestPhase2CacheBenchmark -timeout 300s
```
