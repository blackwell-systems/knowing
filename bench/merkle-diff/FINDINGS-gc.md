# GC (Garbage Collection Reachability Sweep) Benchmark

Statistical benchmarks for `SnapshotManager.GarbageCollectFull()` on the live
knowing codebase. Each measurement runs 10 times with 2 warmup runs discarded;
statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Real nodes: 2,338
- Real edges: 11,664
- Orphaned nodes injected: 500
- Orphaned edges injected: 200
- Method: 10 measurement runs, 2 warmup (discarded)

## Results

### GC Performance

| Scenario | Time | Nodes removed | Edges removed |
|----------|------|--------------|--------------|
| First run (500 orphans present) | 70ms | 500 | 1,411 (200 injected + 1,211 pre-existing dangling) |
| Clean DB (steady state, median) | 53ms | 0 | 0 |

### What GarbageCollectFull Does

1. **Snapshot chain GC**: prune old snapshots beyond keepCount
2. **Reachability sweep**: collect all nodes reachable via NodesByName and EdgesFrom from surviving snapshots
3. **Delete unreachable nodes**: `DeleteNodesNotIn(reachableNodes)` using temporary table for efficient NOT IN
4. **Delete unreachable edges**: `DeleteEdgesNotIn(reachableEdges)` using temporary table

### Correctness (verified by assertions)

| Assertion | Result |
|-----------|--------|
| All 500 injected orphan nodes pruned | PASS |
| All 200 injected orphan edges pruned | PASS |
| All 2,338 real nodes survived | PASS |
| All 11,664 real edges survived | PASS |
| Second GC pass removes nothing | PASS (0 nodes, 0 edges) |
| Injected nodes not found by hash lookup after GC | PASS |

### Pre-existing Dangling Edges

GC also pruned 1,211 pre-existing dangling edges from cross-repo references
(edges whose targets live in other repos not indexed into the temp DB). This is
correct behavior: edges pointing to non-existent nodes are unreachable and should
be cleaned up.

## Performance Contracts

| Contract | Threshold | Actual | Status |
|----------|-----------|--------|--------|
| GC with orphans | < 10s | 70ms | PASS (140x under) |
| GC clean DB median | < 10s | 53ms | PASS (190x under) |

## Running

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestGCBenchmark -timeout 300s
```
