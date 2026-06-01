> **Note (session 19):** The embedding re-ranker was found to be net negative on P@10 (9/13 repos hurt) and has been disabled. The +17% improvement attributed to the re-ranker in this document was actually from gap-fill seeds sharing the same env var. See roadmap item 17c.

# Dense Graph Dilution: Analysis Plan

## Resolution (Session 16, 2026-05-26)

The dense graph dilution problem is substantially resolved:
- **PreferTypeSeeds** (session 14): VS Code 0.084 -> 0.137 (+63%)
- **Embedding re-ranker** (session 15): aggregate 0.207 -> 0.242 (+17%), VS Code stable at 0.137
- **Adaptive seed count** (session 16): Django 0.197 -> 0.225 (+14.2%) by increasing seeds on large graphs
- **Full corpus**: P@10 = 0.242, zero regressions on any repo
- VS Code re-ranker regression (session 15: -16%) resolved in session 16: 0% delta

Success criteria status: VS Code P@10=0.137 (target was >=0.150, close but not met).
Aggregate P@10=0.242 (target was >=0.210, exceeded by +15%). No regressions on any repo.

Remaining investigation: VS Code 0.137 is still below the 0.163 it achieved with broken
extraction (43K nodes). The 87K-node correct extraction still dilutes seeds. Further
density-adaptive strategies (adaptive alpha, FTS channel balancing) are on the roadmap.

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

## Results So Far (Session 14)

### Edge exclusion ablation (BENCH_EXCLUDE_EDGES)

| Config | VS Code P@10 |
|--------|-------------|
| All edges (dense 87K) | 0.084 |
| Exclude similar_to | 0.095 |
| Exclude type_hint_of | 0.095 |
| Exclude both | 0.095 |

**Verdict: edge types are not the root cause.** Removing the two most voluminous
edge types (similarity: ~90K edges, type_hint_of: ~33K edges) recovers only 0.011.

### BFS depth sweep (BENCH_BFS_DEPTH)

| Depth | VS Code P@10 |
|-------|-------------|
| 2 | 0.100 |
| 3 | 0.100 |
| 4 (default) | 0.084 |

**Verdict: walk parameters are not the root cause.** Limiting BFS expansion to
2 hops (vs 4) doesn't recover precision because there's no adjacency cache on
this DB (fallback BFS path), but seeds themselves are already bad.

### Diagnosis: Seed Selection Degradation

The problem is in the **retrieval front-end** (BM25/tiered search/RRF fusion),
not in the walk. With 87K nodes in the FTS index (vs 43K), the same keywords
match more candidates. The wrong candidates win RRF because:

1. More symbols have the same name fragments (dense TS codebase with many exports)
2. BM25 IDF values shift (common terms become LESS discriminative with more documents)
3. Tiered search returns more prefix matches from the expanded node set
4. RRF fusion picks the wrong top-15 seeds from this expanded candidate pool

The old P@10=0.163 was artificially high because with only 43K nodes (broken
extraction), the ONLY symbols matching task keywords were the correct ones.
There was no competition. With correct extraction, there are 10-50x more
competitors for every keyword, and the ground truth gets outranked.

**This is NOT a walk/RWR problem. It's a seed quality problem on dense FTS indexes.**

### Resolution: PreferTypeSeeds (SHIPPED)

**Hypothesis H8 (node-kind-aware seed selection) CONFIRMED.**

Reordering RRF candidates to prefer type/interface/class nodes as RWR seeds:

| Config | VS Code P@10 |
|--------|-------------|
| Baseline (all hypotheses failed) | 0.095 |
| H1: Hub dampening (threshold 50) | 0.095 (no effect) |
| **H8: PreferTypeSeeds** | **0.137 (+44%)** |
| H1 + H8 combined | 0.137 (H1 adds nothing) |

**Full corpus with PreferTypeSeeds: P@10 = 0.207. Zero regressions.**

The fix is self-adapting: auto-enables when `GraphNodeCount > 40000`. The threshold
was determined empirically (VS Code DB has 49,451 nodes). Django (42K), kafka (80K),
and k8s (117K) all trigger and are unaffected.

**Why types are better seeds:** On dense graphs, generic keywords match thousands of
methods. Methods can only walk UP (to their callers), competing with thousands of other
methods for the same RWR probability. Types walk DOWN (to their methods via `contains`
edges), reaching an entire class's API surface from a single seed. The walk from a type
is more productive because it covers a coherent set of related symbols.

### Implications (updated session 21)

1. Local embeddings still the biggest lever (bypasses keyword competition entirely)
2. ~~Node-kind-aware seed selection may help~~ CONFIRMED and shipped as self-adapting
3. ~~Per-file or per-package BM25~~ Superseded by focused seed selection (session 21): clustering RRF candidates by package path and promoting the largest cluster achieves the same effect (scoping to a coherent neighborhood) without modifying the BM25 index.
4. The parameter sweep finding still holds: P@10 is reachability-determined. But now we
   know that "reachability" starts at seed selection, not just graph structure.
5. **Density-adaptive retrieval** is now a core design principle with 8 self-adapting mechanisms.
6. **Focused seed selection** (session 21): concentrates the walk in the dominant structural neighborhood (+6.0% relative). Session 23 added **framework equivalence classes** (263 classes with forced injection, +57% P@10) which bypass the dilution problem entirely by injecting known-good symbols directly. Embeddings (gap-fill) confirmed neutral (session 23). Dense graph dilution is solved by a combination of seed cohesion and vocabulary bridging via framework knowledge.

### Open questions

- Can focused seed selection be made density-adaptive (only activate on dense graphs)?
  Session 21 tested: it helps across all densities, not just dense graphs. Currently always-on.
- Two-phase retrieval (walk, then re-search within neighborhood) was tested neutral/harmful
  on Django and dense repos. Focused seeds may already capture the benefit.
- Would per-community BM25 indices help further? Communities are very small (avg 8 nodes
  in Django) so the constraint may be too tight.

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
