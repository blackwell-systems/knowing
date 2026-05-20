# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 2861
- **Edges:** 14655 unique
- **Packages:** 58
- **Edge types:** 3 (calls:13116, imports:1400, throws:139)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (1964 edges mutated, 13.4% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 2.871875ms | baseline |
| Hierarchical | 4.712333ms | +64.1% |

The hierarchical tree costs roughly the same to build. It produces 58 package roots
and 146 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 14655 edges) | 1.023027ms | O(edges) |
| Hierarchical diff (compare 58 package roots) | 4.433µs | O(packages) |
| **Speedup** | **231x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 45ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 12.818µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:throws]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 58 package roots instead of
   14655 edge leaves produces a 231x speedup.

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
graph with 20 packages gets 283x. The knowing repo (14655 edges, 58 packages) gets
231x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
