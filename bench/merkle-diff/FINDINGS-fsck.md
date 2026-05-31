# Fsck (Graph Integrity Verification) Benchmark

Statistical benchmarks for SnapshotManager.Verify() on the live knowing
codebase. Each measurement runs 10 times with 2 warmup runs discarded;
statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Nodes verified: 10782
- Edges verified: 51276
- Snapshots in chain: checked for parent continuity

## Checks Performed by Verify

1. **Edge referential integrity** — for every edge, verify that both the source
   node and the target node exist in the graph. Reports dangling_edge (ERROR)
   for any missing endpoint.
2. **Hash recomputation** — recompute the canonical hash for each edge and each
   node and compare against the stored hash. Reports hash_mismatch (ERROR for
   edges, WARN for nodes) on any discrepancy.
3. **Snapshot chain continuity** — walk the snapshot chain from latest to root;
   report broken_chain (ERROR) for any snapshot whose parent hash is not found.

## Performance Contract

- Verify on the knowing repo (10782 nodes, 51276 edges) must complete in under 30
  seconds (median). Test fails if violated.

## Corruption Detection

The benchmark deliberately injects a fake edge whose TargetHash does not exist
in the nodes table, then re-runs Verify and confirms the dangling_edge error is
reported. This validates that the integrity checker is not a no-op.

## Running

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestFsckBenchmark -timeout 300s
```
