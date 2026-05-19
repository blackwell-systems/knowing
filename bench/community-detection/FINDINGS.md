# Community Detection Benchmark: Full vs Incremental

Statistical benchmarks for full and incremental community detection on the live
knowing codebase. Each measurement runs 10 times with 2 warmup runs discarded.

## Setup

- Repository: knowing (live codebase)
- Nodes: 2,472
- Edges: 3,280
- Changed package: `internal/store` (146 nodes, 5.9% of graph)
- Method: 10 measurement runs, 2 warmup (discarded)

## Results

### Louvain

| Scenario | Median | Speedup vs full |
|----------|--------|-----------------|
| Full detection | 2.99ms | baseline |
| Incremental (1 package changed, 5.9%) | 436us | 6.9x |
| Incremental (0 changes, all frozen) | 375us | 8.0x |

Communities detected: 1,151 (full), same structure preserved incrementally.

### Label Propagation

| Scenario | Median | Speedup vs full |
|----------|--------|-----------------|
| Full detection | 14.3ms | baseline |
| Incremental (1 package changed, 5.9%) | 372us | 38.4x |
| Incremental (0 changes, all frozen) | 180us | 79.2x |

Communities detected: 948 (full).

### How Incremental Detection Works

1. Seed community assignments from previous run (stored in notes table after P1 ships).
2. Only nodes in `changedNodes` are allowed to move during optimization passes.
3. Frozen nodes keep their previous community ID.
4. New nodes (not in previous) get fresh IDs and are allowed to move.

### Why Label Propagation Benefits More

Label propagation iterates over all nodes each pass (O(N * iterations)). Freezing
95% of nodes cuts iteration work by 95%. Louvain's optimization loop already
converges quickly (few passes), so the per-iteration savings are smaller relative
to the overhead of computing sigma_tot and ki for all nodes.

### Correctness

Verified: incremental detection with 0 changes produces identical assignments to
full detection (bit-for-bit match on all 2,472 node assignments).

## Performance Contracts

| Contract | Threshold | Actual | Status |
|----------|-----------|--------|--------|
| Louvain full | < 5s | 2.99ms | PASS (1,672x under) |
| Louvain incremental (0 changes) | < 5ms | 375us | PASS (13x under) |
| LP full | < 5s | 14.3ms | PASS (350x under) |

## End-to-End Daemon Path

The E2E benchmark measures the full production cycle: load previous assignments
from SQLite, mark changed nodes from package list, run DetectIncremental, save
new assignments back.

### Before BatchPutNotes

Save was 2,484 individual `INSERT OR REPLACE` calls: 134ms. The algorithm
speedup (445us vs 3ms) was invisible against the I/O cost.

### After BatchPutNotes

Single transaction with prepared statement: 6.4ms save (21x faster).

| Step | Time | % of cycle |
|------|------|-----------|
| Load 2,486 assignments | 1.7ms | 16% |
| Incremental detect | 411us | 4% |
| Save 2,486 assignments | 8.8ms | 81% |
| **Total incremental** | **11ms** | |
| Full detect (no I/O) | 3.1ms | |

### E2E Statistical Measurement

| Path | Median |
|------|--------|
| Full e2e (detect + save) | 12.2ms |
| Incremental e2e (load + mark + detect + save) | 11.4ms |
| **E2E speedup** | **1.1x** |

The e2e speedup is modest because save dominates both paths equally. The real
win is absolute latency: 11ms total for the daemon community update cycle,
well under the 1s performance contract. For comparison, the re-index itself
takes ~8 seconds; community detection adds 0.1% overhead.

### Future optimization

Save only changed assignments (delta save) instead of all 2,486. This would
make save proportional to changed nodes (~146 for 1-package edit) instead of
total nodes, reducing save from 8.8ms to ~0.5ms estimated.

## Running

```bash
GOWORK=off go test ./bench/community-detection/ -v -count=1 -run TestCommunityBenchmark -timeout 120s
GOWORK=off go test ./bench/community-detection/ -v -count=1 -run TestE2ECommunityBenchmark -timeout 120s
```
