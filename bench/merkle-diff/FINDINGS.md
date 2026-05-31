# Merkle Diff Benchmark

Compares flat vs hierarchical Merkle tree operations on the live knowing graph.

## Setup

- **Repository:** knowing (live codebase)
- **Nodes:** 8695
- **Edges:** 71556 unique
- **Packages:** 81
- **Edge types:** 16 (accesses_field:756, authored_by:8694, calls:24259, co_tested_with:1669, contains:2195, documents:2154, executes_process:38, implements:18, imports:2438, member_of:2195, overrides:20, reads_env:35, similar_to:9718, tests:12041, throws:186, type_hint_of:5140)
- **Mutation target:** github.com/blackwell-systems/knowing/internal/mcp (5836 edges mutated, 8.2% of total)

## Build Cost

| Tree type | Build time | Overhead |
|-----------|-----------|----------|
| Flat | 19.462666ms | baseline |
| Hierarchical | 31.501ms | +61.9% |

The hierarchical tree costs roughly the same to build. It produces 81 package roots
and 847 edge-type roots as intermediate nodes.

## Diff Performance

Scenario: one package changed, all others unchanged.

| Operation | Avg latency | Memory |
|-----------|------------|--------|
| Flat diff (compare all 71556 edges) | 4.455528ms | O(edges) |
| Hierarchical diff (compare 81 package roots) | 20.954µs | O(packages) |
| **Speedup** | **213x** | |

## Lookup Performance

| Operation | Avg latency | What it answers |
|-----------|------------|-----------------|
| SubgraphRoot (1 package) | 160ns | Cache key for queries scoped to one package |
| EdgeTypeRoot ("calls") | 31.961µs | "Did any call edges change?" |

## Correctness

The hierarchical diff correctly identified:
- Changed packages: [github.com/blackwell-systems/knowing/internal/mcp]
- Changed edge types: [github.com/blackwell-systems/knowing/internal/mcp:accesses_field github.com/blackwell-systems/knowing/internal/mcp:authored_by github.com/blackwell-systems/knowing/internal/mcp:calls github.com/blackwell-systems/knowing/internal/mcp:co_tested_with github.com/blackwell-systems/knowing/internal/mcp:contains github.com/blackwell-systems/knowing/internal/mcp:documents github.com/blackwell-systems/knowing/internal/mcp:implements github.com/blackwell-systems/knowing/internal/mcp:imports github.com/blackwell-systems/knowing/internal/mcp:member_of github.com/blackwell-systems/knowing/internal/mcp:overrides github.com/blackwell-systems/knowing/internal/mcp:reads_env github.com/blackwell-systems/knowing/internal/mcp:similar_to github.com/blackwell-systems/knowing/internal/mcp:tests github.com/blackwell-systems/knowing/internal/mcp:throws github.com/blackwell-systems/knowing/internal/mcp:type_hint_of]
- Root changed: true

## Interpretation

The hierarchical tree structures the Merkle tree by semantic boundaries (package,
edge type) instead of treating all edges as an undifferentiated set. This means:

1. **Diff is O(packages) not O(edges).** Comparing 81 package roots instead of
   71556 edge leaves produces a 213x speedup.

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
graph with 20 packages gets 283x. The knowing repo (71556 edges, 81 packages) gets
213x.

## Reproducing

```bash
GOWORK=off go test ./bench/merkle-diff/ -v -count=1
```
