# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 4694
- **Edges:** 30875 unique
- **Packages:** 68
- **Edge types:** 6 (authored_by:4693, calls:15324, documents:1357, imports:1651, tests:7697, throws:153)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (4031 edges mutated, 13.1% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 8.870917ms | baseline |
| Hierarchical | 15.087625ms | +70.1% |

The hierarchical tree costs roughly the same to build. It produces 68 package roots
and 355 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 30875 edges) | 2.150765ms | O(edges) |
| Hierarchical diff (compare 68 package roots) | 10.698µs | O(packages) |
| **Speedup** | **201x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 192ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 25.861µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:authored_by github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:documents github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:tests github.com/blackwell-systems/knowing/internal/mcp:throws]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 68 package roots instead of
   30875 edge leaves produces a 201x speedup.

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
graph with 20 packages gets 283x. The knowing repo (30875 edges, 68 packages) gets
201x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
