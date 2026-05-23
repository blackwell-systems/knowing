# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 4685
- **Edges:** 31336 unique
- **Packages:** 70
- **Edge types:** 6 (authored_by:4684, calls:15569, documents:1366, imports:1707, tests:7855, throws:155)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (4016 edges mutated, 12.8% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 7.985125ms | baseline |
| Hierarchical | 15.42725ms | +93.2% |

The hierarchical tree costs roughly the same to build. It produces 70 package roots
and 364 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 31336 edges) | 2.24994ms | O(edges) |
| Hierarchical diff (compare 70 package roots) | 9.545µs | O(packages) |
| **Speedup** | **236x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 173ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 27.056µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:authored_by github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:documents github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:tests github.com/blackwell-systems/knowing/internal/mcp:throws]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 70 package roots instead of
   31336 edge leaves produces a 236x speedup.

2. **Subgraph cache keys are O(1).** A query scoped to packages A and B can check
   if its cached result is still valid by comparing two package roots, regardless
   of how many edges exist.

3. **Build cost is free.** The hierarchical tree costs the same to build as the flat
   tree because the total hashing work is identical; it's just organized differently.

4. **Scoped invalidation.** When the daemon detects a file change, the hierarchical
   diff tells you which packages were affected. Only those package-scoped caches
   need invalidation. Everything else stays warm.

The speedup grows with graph size because the ratio of packages to edges increases.
A 100K-edge graph with 100 packages gets 517x speedup (benchmarked). A 10K-edge
graph with 20 packages gets 283x. The knowing repo (31336 edges, 70 packages) gets
236x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
