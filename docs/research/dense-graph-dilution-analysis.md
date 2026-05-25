# Dense Graph Dilution: Analysis Plan

## Problem Statement

When the TypeScript extractor correctly extracts exported declarations (export_statement fix),
VS Code goes from 43K nodes / 131K edges to 87K nodes / 422K edges. P@10 drops from 0.163
to 0.084. The aggregate drops from 0.210 to 0.201.

This is the same phenomenon observed with:
- k8s staging (136K extra nodes, P@10 -20%, session 12)
- LSP enrichment (42K extra edges from pyright, P@10 negative, session 13)

All three share the same root cause: more nodes/edges in the graph spread RWR probability
mass across more candidates, pushing correct results below rank 10.

## Hypothesis Space

### H1: Hub node dominance (high in-degree attractors)

**Theory:** Common utility types (EventEmitter, Disposable, URI, CancellationToken) receive
type_hint_of edges from thousands of functions. RWR accumulates probability on these hub
nodes regardless of query relevance because they're reachable from ANY seed.

**Test:** Measure in-degree distribution of the top-50 RWR-scored nodes for a failing VS Code
task. If they're dominated by high-in-degree utility types, the hypothesis is confirmed.

**Fix if confirmed:** Hub dampening (divide RWR transition probability by sqrt(in-degree) of
target node). Or: exclude nodes with in-degree > threshold from final ranking (not from walk).

### H2: Similarity edge explosion on large graphs

**Theory:** ComputeSimilarityEdges is O(n^2) within each package. On 87K nodes with many
large packages, it generates massive numbers of similarity edges that create random-looking
connections. These make everything 2-hops from everything else, flattening RWR scores.

**Test:** Reindex VS Code with `--edge-types` excluding `similar_to`. Compare P@10.

**Fix if confirmed:** Cap similarity edges per node (max 3-5), or only compute for nodes
with < N existing edges (don't add similarity to well-connected nodes).

### H3: type_hint_of edges to builtin-like types

**Theory:** TypeScript types like `Promise`, `Thenable`, `Event`, `ReadonlyArray` are used
everywhere. They're not in `isTSBuiltin` but function like builtins (too generic to be useful
as retrieval signals). type_hint_of edges to these create paths that don't carry meaning.

**Test:** Count type_hint_of target distribution. If top-10 targets account for >50% of all
type_hint_of edges, the hypothesis is confirmed.

**Fix if confirmed:** Extend `isTSBuiltin` with VS Code-specific common types, or
dynamically filter type_hint_of targets with in-degree > N.

### H4: Community structure collapse

**Theory:** On the sparse 43K-node graph, community detection produced meaningful clusters
(editor components, extensions, platform services). On the dense 87K-node graph, communities
are too large or too few, so the single-community RWR filter never activates.

**Test:** Compare community count and max community size between sparse and dense VS Code indexes.

**Fix if confirmed:** Hierarchical community detection (split large communities), or use
package-based scoping instead of Louvain communities.

### H5: Correct extraction surfaces a ground truth problem

**Theory:** The VS Code ground truth fixtures were written when only 72 TS nodes were
extracted. The fixtures may reference the only symbols that COULD appear in results with
the sparse graph. With proper extraction, the "correct" answer might actually be different
symbols than what the fixtures specify.

**Test:** For each failing VS Code task, check if the ground truth symbols still exist in the
new index and whether they're still the BEST answer for the task description.

**Fix if confirmed:** Update fixtures to reflect proper extraction reality.

## Experiment Protocol

### Phase 1: Characterize the failure (diagnostic, no code changes)

1. **Per-task delta**: For each VS Code task, compare top-10 results between sparse (old) and
   dense (new) extraction. Which symbols entered top-10? Which left? Why?

2. **Hub analysis**: For the dense VS Code graph, compute in-degree distribution. Identify
   the top-20 highest in-degree nodes. Are they in the top-10 results for most tasks?

3. **Score distribution**: Plot RWR score distribution for a sample task on sparse vs dense.
   If dense produces a flatter distribution (all scores similar), the walk is diffusing.

4. **Edge type contribution**: Run VS Code benchmark with each edge type excluded individually
   (using `--edge-types` filter). Identify which edge type's inclusion causes the most damage.

### Phase 2: Test hypotheses (one change at a time)

5. **H2 test**: `--edge-types calls,imports,implements,extends,overrides,references,contains,member_of,type_hint_of,co_tested_with,inherits,decorates,handles_route,throws`
   (all except similar_to). Measure VS Code P@10.

6. **H3 test**: Add `isTSBuiltin` entries for VS Code common types (Disposable, Event, URI,
   CancellationToken, etc.) and reindex. Measure VS Code P@10.

7. **H1 test**: After RWR, filter results where the node's in-degree > 100 (hub dampening).
   Measure VS Code P@10.

### Phase 3: Implement the fix

8. Whichever hypothesis produces the largest P@10 recovery gets implemented as a permanent
   fix. Must not regress other repos.

## Success Criteria

- VS Code P@10 >= 0.150 with correct extraction (87K nodes)
- No regression on kafka (0.253), terraform (0.275), flask (0.332)
- Aggregate P@10 >= 0.210

## Timeline

Phase 1 (diagnostic): ~30 min (mostly running benchmarks with filters)
Phase 2 (hypothesis testing): ~1 hour (reindex + benchmark per hypothesis)
Phase 3 (implementation): depends on which hypothesis wins
