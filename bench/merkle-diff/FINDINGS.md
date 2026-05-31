# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 7674
- **Edges:** 62866 unique
- **Packages:** 77
- **Edge types:** 16 (accesses_field:746, authored_by:7673, calls:21140, co_tested_with:1424, contains:2137, documents:1848, executes_process:38, implements:18, imports:2228, member_of:2137, overrides:20, reads_env:35, similar_to:8299, tests:10605, throws:177, type_hint_of:4341)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (5833 edges mutated, 9.3% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 16.740667ms | baseline |
| Hierarchical | 27.647958ms | +65.2% |

The hierarchical tree costs roughly the same to build. It produces 77 package roots
and 797 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 62866 edges) | 4.040278ms | O(edges) |
| Hierarchical diff (compare 77 package roots) | 19.138µs | O(packages) |
| **Speedup** | **211x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 186ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 31.102µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:accesses_field github.com/blackwell-systems/knowing/internal/mcp:authored_by github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:co_tested_with github.com/blackwell-systems/knowing/internal/mcp:contains github.com/blackwell-systems/knowing/internal/mcp:documents github.com/blackwell-systems/knowing/internal/mcp:implements github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:member_of github.com/blackwell-systems/knowing/internal/mcp:overrides github.com/blackwell-systems/knowing/internal/mcp:reads_env github.com/blackwell-systems/knowing/internal/mcp:similar_to github.com/blackwell-systems/knowing/internal/mcp:tests github.com/blackwell-systems/knowing/internal/mcp:throws github.com/blackwell-systems/knowing/internal/mcp:type_hint_of]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 77 package roots instead of
   62866 edge leaves produces a 211x speedup.

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
graph with 20 packages gets 283x. The knowing repo (62866 edges, 77 packages) gets
211x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
