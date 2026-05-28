# Adaptive Retrieval

knowing's retrieval pipeline observes its own graph properties at query time and
adjusts its strategy accordingly. No manual configuration. The same binary handles
a 1.4K-node Flask repo and a 552K-node VS Code repo with different strategies,
automatically. This is the project's central thesis: a code retrieval system that
adapts to its graph outperforms any fixed-strategy system, and the gap widens with
scale.

**Current result:** P@10 = 0.246 cold start, 0.253 with compounding (237 tasks,
12 repos, 7 languages). 1.82x codegraph, 3.28x GitNexus, 3.90x Gortex.

## The Problem with Fixed-Strategy Retrieval

Every competing code retrieval system uses a fixed strategy regardless of graph
properties. codegraph uses the same BM25+heuristic scoring on a 1K-node repo
and a 100K-node repo. GitNexus builds the same knowledge graph structure whether
the codebase has 5 packages or 500.

This works at small scale. At large scale, it fails:
- BM25 keyword competition: "Handle" matches 10,000 symbols on k8s
- Seed quality degrades: methods outnumber types 10:1, but methods are worse seeds
- Disconnection increases: larger graphs have more isolated subgraphs
- Vocabulary gaps widen: more symbols = more ground truth with zero keyword overlap

The result: fixed-strategy systems get less precise as codebases grow. knowing
gets more precise because it detects these conditions and compensates.

## Six Self-Adapting Mechanisms

### 1. PreferTypeSeeds (density-adaptive seed selection)

**Trigger:** `GraphNodeCount > 40,000`

**What it does:** Reorders RRF candidates so type/interface/class nodes are
selected as RWR seeds before methods/functions. Types make better seeds on dense
graphs because they have `contains` edges to their methods, producing more
productive walks.

**Why it's needed:** On dense graphs, BM25 returns many method-level matches
that compete for seed slots. Methods can only walk upward to callers (competing
with thousands of other methods for the same keywords). Types walk downward to
their methods via `contains` edges, reaching an entire subsystem from one seed.

**Measured impact:** VS Code +44% (0.095 -> 0.137). Zero regressions on any repo.
Auto-enables based on node count; no manual threshold configuration.

**Session 17 finding:** Phantom nodes from LSP enrichment inflate GraphNodeCount
(k8s: 72K real -> 242K with phantoms). This was initially suspected as a bug
(triggering PreferTypeSeeds on repos that aren't truly dense). Testing showed
it's correct behavior: enrichment edges make the graph genuinely denser, and
PreferTypeSeeds benefits from the inflated count. The phantom density signal is
valid.

### 2. Adaptive Seed Count (scale-aware exploration)

**Trigger:** `GraphNodeCount > 10,000` (20 seeds) or `> 40,000` (25 seeds)

**What it does:** Increases the number of RWR seeds on larger graphs. Default is
15 seeds. Larger graphs have higher disconnection rates; more seeds compensate by
covering more subgraphs.

**Why it's needed:** On a 1K-node Flask repo, 15 seeds cover most of the graph
within 4 RWR hops. On a 57K-node Django repo, 15 seeds leave large regions
unreachable. More seeds = broader coverage = more ground truth found.

**Measured impact:** Django +14.2% (0.197 -> 0.225). Full corpus +1.7% (0.238 -> 0.242).

### 3. Embedding Gap-Fill Seeds (vocabulary-adaptive fallback)

**Trigger:** `len(candidates) < 5` after BM25/tiered/equivalence seed selection

**What it does:** When primary keyword-based channels return fewer than 5 seed
candidates, queries the embedding vector store for semantically similar symbols.
Embedding-based seeds can bridge vocabulary gaps that BM25 structurally cannot
(task says "validate the request body", ground truth is `FormValidator.clean()`).

**Why it's needed:** 42% of Django tasks scored zero because ground truth symbols
share no keywords with the task description. BM25 cannot find what it cannot
match lexically. Embeddings find symbols by semantic similarity regardless of
keyword overlap.

**Why it can't regress:** Gap-fill only fires when primary channels are weak
(< 5 candidates). On repos where BM25 already works (kafka 0.342, terraform
0.295), the threshold is never reached. The mechanism is self-selecting:
it intervenes only where the existing pipeline is already failing.

**Measured impact:** Django +43% (0.176 -> 0.252). Flask +22% (0.263 -> 0.321).
Full corpus +11.2% (0.223 -> 0.248). Zero regressions across all 12 repos.

### 4. Task Memory Compounding (learning-adaptive boosting)

**Trigger:** Any repeated or similar query within or across sessions

**What it does:** Records the top-5 symbols returned by each `context_for_task`
call. On future queries with similar keywords, recalls stored symbols and boosts
them via the feedback channel. Boost formula: `0.5 + recall_score * 0.4`
(range [0.5, 0.9]), with 7-day linear decay.

**Why it's needed:** Real agent sessions repeat similar queries. An agent
investigating "auth middleware" today should benefit from what worked yesterday.
The system gets smarter with use without any explicit feedback mechanism.

**Measured impact:** +3.8% to +11.5% P@10 from round 1 to round 2 (varies by
how much gap-fill already recovered). R@10 improves more (+7.9% to +15.0%)
because memory expands the set of reachable symbols.

**Interaction with gap-fill:** Gap-fill introduces new symbols that were
previously unreachable (recovered zeros). These symbols enter task memory.
On round 2, they get boosted alongside BM25-found symbols. The compounding
surface area grows because there's more to compound. Gap-fill + compounding
interact multiplicatively: neither achieves the combined effect alone.

### 5. Merkleized Feedback Expiration (staleness-adaptive validity)

**Trigger:** Code change in the symbol's package (SubgraphRoot mismatch)

**What it does:** Each feedback record stores the Merkle root of the symbol's
package at recording time. When querying feedback, only records where
`neighborhood_root` matches the current SubgraphRoot are counted. Feedback
automatically becomes invisible when the code it references changes.

**Why it's needed:** Stale feedback is worse than no feedback. A symbol that was
useful yesterday but was refactored today should not receive a boost. Traditional
feedback systems require explicit expiration policies or manual cleanup.
Content-addressed Merkle roots provide structural expiration: the feedback is
valid if and only if the code hasn't changed.

**Measured overhead:** 11% (255us -> 284us for 100 symbols). The Merkle root
comparison is a single hash equality check.

### 6. LSP Enrichment Interaction (enrichment-adaptive reachability)

**Trigger:** LSP enrichment creates phantom nodes + type_hint_of edges exist

**What it does:** LSP enrichment discovers new edges (implements, references)
and creates phantom external nodes (stdlib/dependency types). When type_hint_of
edges (from tree-sitter extraction) connect functions to those phantom nodes,
RWR can walk between functions that reference the same external type. Neither
enrichment alone nor type_hint_of alone produces this reachability; they interact
multiplicatively.

**Why it's adaptive:** The reachability benefit scales with enrichment coverage.
Repos with more enrichment (Go with gopls: 192K new edges on k8s) see larger
improvements than repos with less enrichment. The system automatically leverages
whatever enrichment is available without configuration.

**Measured impact:** Kubernetes 0.000 -> 0.232 (first-time enrichment). Terraform
~0.095 -> 0.275. Python repos +0.040. Session 13 measured enrichment as neutral
(before type_hint_of existed). Session 17 revised: enrichment is strongly positive
when combined with type_hint_of edges.

## Ablation Summary

Each mechanism measured independently on the full corpus:

| Mechanism | Without | With | Delta | Trigger |
|-----------|---------|------|-------|---------|
| PreferTypeSeeds | 0.202 | 0.207 | +2.5% | Node count > 40K |
| Adaptive seed count | 0.238 | 0.242 | +1.7% | Node count > 10K/40K |
| Gap-fill seeds | 0.223 | 0.248 | +11.2% | Candidates < 5 |
| Task memory | 0.248 (cold) | 0.253 (warm) | +3.8% | Any repeated query |
| Feedback expiration | N/A | N/A | correctness | Code change |
| Enrichment + type_hint | 0.200 (no enrich) | 0.248 (enriched) | +24% | LSP available |

Combined: P@10 = 0.246 cold, 0.253 warm (237 tasks, 12 repos).
Without any adaptation: ~0.180 (estimated from pre-enrichment, pre-gap-fill baseline).

## Why Fixed-Strategy Systems Can't Compete

Competitors would need to implement all six mechanisms to match knowing's
adaptive behavior. But the mechanisms interact: PreferTypeSeeds benefits from
phantom density (mechanism 6). Gap-fill benefits from pre-embedded vectors
(infrastructure). Task memory benefits from gap-fill (more symbols to remember).
The system is greater than the sum of its parts.

More importantly, the adaptive approach is structural: it follows from
content-addressed storage (feedback expiration via Merkle roots), graph-native
retrieval (density-adaptive seeding via node count), and embedding integration
(gap-fill via brute-force cosine). Each adaptation is a natural consequence of
the architecture, not an ad-hoc heuristic bolted on.

## Source Files

| File | Mechanism |
|------|-----------|
| `internal/context/context.go` | Gap-fill seeds (lines 711-745), PreferTypeSeeds (line 758) |
| `internal/context/walk.go` | GraphNodeCount, adaptive seed count, PreferTypeSeeds flag |
| `internal/context/task_memory.go` | Task memory recording and recall |
| `internal/context/ranking.go` | Feedback boost integration |
| `internal/store/sqlite.go` | Merkleized feedback (neighborhood_root), GetAllEmbeddings |
| `internal/embedding/searcher.go` | LoadAndSearchFromStore (brute-force gap-fill) |
| `internal/enrichment/enricher.go` | LSP enrichment (phantom nodes, edge discovery) |

## Related Documents

- [Retrieval Pipeline](retrieval-pipeline.md): full pipeline reference (seed channels, RWR, HITS, scoring)
- [Context Engine](context-engine.md): ForTask flow with all adaptive mechanisms in sequence
- [Enrichment Pipeline](enrichment-pipeline.md): LSP enrichment that creates phantom reachability
- [Embedding Re-ranker](embedding-reranker.md): the embedding infrastructure that gap-fill builds on
