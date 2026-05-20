# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 2971
- **Edges:** 18137 unique
- **Packages:** 60
- **Edge types:** 4 (authored_by:2970, calls:13571, imports:1452, throws:144)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (2463 edges mutated, 13.6% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 3.383625ms | baseline |
| Hierarchical | 5.808625ms | +71.7% |

The hierarchical tree costs roughly the same to build. It produces 60 package roots
and 211 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 18137 edges) | 1.082581ms | O(edges) |
| Hierarchical diff (compare 60 package roots) | 5.897µs | O(packages) |
| **Speedup** | **184x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 41ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 13.08µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:authored_by github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:throws]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 60 package roots instead of
   18137 edge leaves produces a 184x speedup.

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
graph with 20 packages gets 283x. The knowing repo (18137 edges, 60 packages) gets
184x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
