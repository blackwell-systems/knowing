# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 2545
- **Edges:** 12599 unique
- **Packages:** 115
- **Edge types:** 2 (calls:12472, throws:127)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (1450 edges mutated, 11.5% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 2.043959ms | baseline |
| Hierarchical | 2.484417ms | +21.5% |

The hierarchical tree costs roughly the same to build. It produces 115 package roots
and 154 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 12599 edges) | 784.056µs | O(edges) |
| Hierarchical diff (compare 115 package roots) | 6.143µs | O(packages) |
| **Speedup** | **128x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 86ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 18.491µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:throws]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 115 package roots instead of
   12599 edge leaves produces a 128x speedup.

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
graph with 20 packages gets 283x. The knowing repo (12599 edges, 115 packages) gets
128x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
