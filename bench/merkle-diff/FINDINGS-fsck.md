# Fsck (Graph Integrity Verification) Benchmark

Statistical benchmarks for `SnapshotManager.Verify()` on the live knowing
codebase. Each measurement runs 10 times with 2 warmup runs discarded;
statistics report min, median, p95, mean, and stddev.

## Setup

- Repository: knowing (live codebase)
- Nodes verified: 2,338
- Edges verified: 11,664
- Snapshots in chain: checked for parent continuity
- Method: 10 measurement runs, 2 warmup (discarded)

## Results

### Verification Performance

| Metric | Value |
|--------|-------|
| Median | 98ms |
| Min | ~90ms |
| P95 | ~110ms |
| Stddev | ~5ms |

98ms to verify 2,338 nodes + 11,664 edges + snapshot chain. Well within the 30s performance contract (300x under budget).

### Checks Performed

| Check | What it verifies | Severity on failure |
|-------|-----------------|-------------------|
| Edge referential integrity | Both source and target node exist for every edge | ERROR |
| Edge hash recomputation | Stored hash matches recomputed hash from fields | ERROR |
| Node hash recomputation | Stored hash matches recomputed hash from fields | WARN |
| Snapshot chain continuity | Every snapshot's ParentHash points to an existing snapshot | ERROR |

### Corruption Detection

The benchmark deliberately injects a fake edge whose TargetHash does not exist
in the nodes table, then re-runs Verify and confirms the `dangling_edge` error
is reported for the injected hash. This validates that the integrity checker
detects real corruption, not just returns "all clear."

### Pre-existing Findings

The live DB shows ~8,492 dangling-edge findings from cross-repo edge references
(edges whose targets are in other repos not indexed into the temp DB). These are
expected in a multi-repo graph and are logged informationally. On a single-repo
DB, fsck should report zero errors.

## Performance Contract

| Contract | Threshold | Actual | Status |
|----------|-----------|--------|--------|
| Verify median | < 30s | 98ms | PASS (300x under) |

## Running

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestFsckBenchmark -timeout 300s
```
