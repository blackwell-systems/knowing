# GC (Garbage Collection Reachability Sweep) Benchmark

Statistical benchmarks for SnapshotManager.GarbageCollectFull() on the live
knowing codebase. Each statistical measurement runs 10 times with 2 warmup
runs discarded; statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Real nodes in graph: 7504
- Real edges in graph: 25866
- Orphaned nodes injected: 500 (hashes not referenced by any snapshot)
- Orphaned edges injected: 200 (pointing between orphaned nodes only)

## What GarbageCollectFull Does

1. **Step 1: Snapshot chain GC** — prune old snapshots beyond keepCount,
   preserving the most recent chain.
2. **Step 2: Reachability sweep** — collect all nodes reachable from the
   surviving snapshot via NodesByName and EdgesFrom. Build reachableNodes
   and reachableEdges sets.
3. **Step 3: Delete unreachable nodes** — call DeleteNodesNotIn with the
   reachable set; returns count of deleted rows.
4. **Step 4: Delete unreachable edges** — call DeleteEdgesNotIn with the
   reachable set; returns count of deleted rows.

## Correctness Assertions

- GCStats.NodesRemoved == 500 (exactly the injected orphan count; fake nodes
  use a QualifiedName prefix that NodesByName excludes, so none survive)
- GCStats.EdgesRemoved >= 200 (at least the injected orphan edges; may be
  higher if the live codebase has pre-existing dangling cross-repo edges
  whose targets are not indexed into the temp DB)
- All 7504 real nodes survive
- All 25866 real edges survive
- Second GC pass on clean DB removes 0 nodes and 0 edges

## Performance Contracts

- GC with 500 orphaned nodes on the knowing repo must complete in under 10
  seconds (wall clock). Test fails if violated.
- GC on a clean DB (no orphans) must also complete in under 10 seconds
  (median). Test fails if violated.

## Running

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestGCBenchmark -timeout 300s
```
