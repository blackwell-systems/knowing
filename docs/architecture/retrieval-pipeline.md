# Retrieval Pipeline

The retrieval pipeline (`internal/context/`) transforms a task description into a
token-budgeted, relevance-ranked block of symbols from the knowledge graph. It is the
core of knowing's value proposition: given a natural-language task, produce the most
relevant code symbols that fit within a context window.

This document is the authoritative reference for how the context engine finds and ranks
symbols. It supersedes `context-packing.md`.

**Current eval baseline:** 55 fixtures (20 easy, 20 medium, 15 hard), 31.6% P@10, 0.58 MRR (internal eval). Cross-system benchmark (~117 manual fixtures, 7 repos): P@10=0.217, 1.63x vs codegraph (19K stars), 4.3x vs Aider, 11x vs grep (d=0.92 very large effect).

## Pipeline Overview

```
Task Description
    |
    v
[1. Keyword Extraction]        KeywordSet: Exact (backtick), Compounds (CamelCase/snake), Components (fallback)
    |
    v
[2. Seed Retrieval]            4-channel RRF fusion (tiered, equivalence, BM25, vector)
    |
    v
[3. Interface-Aware Seeding]   add implementors of matched interface types
    |
    v
[4. Noise Filtering]           exclude externals, mocks, stubs, dist/, vendor/, minified
    |
    v
[5. Random Walk with Restart]  propagate relevance through graph (alpha=0.2, early termination on top-K stability)
    |
    v
[6. HITS Reranking]            authority/hub scores on top-200 RWR nodes
    |
    v
[7. Scoring]                   6-component formula with feedback + session boosts
    |
    v
[8. Budget Packing]            density-ranked greedy knapsack (score/cost ratio)
    |
    v
[9. Session + Memory Record]   track returned symbols for future boosts
```

Three entry points:

- **`ForTask(ctx, TaskOptions)`**: starts from a task description string. Full pipeline.
- **`ForFiles(ctx, FileOptions)`**: starts from changed file paths. Finds file symbols,
  adds callers as distance-1 candidates, scores, packs. No keyword extraction or RWR.
- **`ForPR(ctx, PROptions)`**: starts from PR changed files. Runs RWR from file symbols
  to find the broader impact neighborhood. Budget defaults to 8,000 tokens.

---

## 1. Keyword Extraction

The `extractKeywordSet` function converts a free-text task description into a structured
`KeywordSet` with three specificity tiers. The tiered structure enables compound-first
retrieval: higher-specificity keywords are queried before lower-specificity fallbacks.

### KeywordSet structure

| Tier | Source | Purpose |
|------|--------|---------|
| **Exact** | Backtick-quoted identifiers (e.g., `` `before_request` ``) | Explicit symbol references from the user |
| **Compounds** | snake_case, CamelCase, dotted identifiers + bigram-generated compounds | Multi-part identifiers with high specificity |
| **Components** | Individual words split from identifiers, abbreviation expansions, priority terms | Fallback when compounds yield insufficient results |

### Extraction steps

**Phase 1: Backtick-quoted identifiers (Exact tier)**

Backtick-delimited text (e.g., `` `before_request` ``) is extracted as explicit symbol
references. Only valid identifiers are accepted (no spaces, <= 100 chars). Both original
case and lowercase variant are added.

**Phase 2: Standard word extraction (Compounds + Components)**

1. **Verb-pattern detection**: if the first word is an action verb ("add", "implement",
   "build", etc.), the first non-filler noun after it is boosted. If that noun is a
   compound identifier (contains `_`, `.`, or mixed case), it goes to Compounds;
   otherwise it becomes a priority term in Components (with capitalized variant).
2. **Process each word**: split CamelCase and snake_case identifiers into component
   parts. If the original word is a compound identifier (multi-part or contains `_`/`.`),
   add it to Compounds. Split parts go to Components.
3. **Filter stop words**: English stop words (`the`, `a`, `in`), programming terms
   (`func`, `type`, `var`, `err`), and action verbs (`refactor`, `fix`, `update`).
4. **Length filter**: discard anything shorter than 2 characters.
5. **Expand abbreviations**: `ctx` adds `context`, `cfg` adds `config`, `svc` adds
   `service`, etc. Both forms are included in Components.

**Phase 3: Bigram compound generation (Compounds)**

Adjacent non-stop-words (both >= 3 chars, neither an action verb) are joined into
CamelCase and snake_case compounds. "blast radius" produces `BlastRadius` and
`blast_radius`. "transitive callers" produces `TransitiveCallers` and
`transitive_callers`. At least one word must be >= 4 chars.

**Final assembly:**

Components are ordered with priority terms (verb-pattern nouns) first, then remaining
terms sorted by length descending (more specific terms first).

### Example

Task: `"add a new MCP tool for snapshot diffing"`

- "add" filtered (action verb), "a" filtered (stop word), "new" filtered (programming
  stop word), "for" filtered (stop word)
- Priority term extracted: "snapshot" (first noun after verb "add") -> Components
- Bigram compounds: `SnapshotDiffing`, `snapshot_diffing` -> Compounds
- Result KeywordSet:
  - Exact: `[]`
  - Compounds: `["SnapshotDiffing", "snapshot_diffing"]`
  - Components: `["snapshot", "Snapshot", "diffing", "tool", "mcp"]`

---

## 2. Seed Retrieval: 4-Channel RRF Fusion

Seed selection uses Reciprocal Rank Fusion (RRF) to merge four independent retrieval
channels into a single seed set of up to 40 candidates.

### RRF formula

For each channel with weight `w`, a node at position `rank` contributes:

```
w / (k + rank + 1)
```

where `k=60` is the standard RRF constant. Nodes appearing in multiple channels
accumulate scores from all of them, promoting multi-channel hits.

### Channel weights

| Channel | Weight | What it does |
|---------|--------|-------------|
| Tiered keyword matching | 2.0 | Exact > prefix > substring > path matching |
| BM25 FTS5 | 2.0 | Lexical recall over CamelCase-split symbol names |
| Equivalence classes | 2.0 | Concept-level vocabulary bridging |
| Vector/embedding search | 0.0 | Disabled; infrastructure preserved for future code-tuned models |

Weights were equalized (was tiered=3, BM25=1, equiv=2) after cross-system benchmark
investigation showed BM25 and tiered find the same symbols in practice. Equalizing
removes artificial suppression of BM25 recall without degrading precision.

### Channel 1: Tiered keyword matching (weight 2.0)

Uses compound-first `tieredSearchSet`: queries Exact+Compounds (the "primary" keywords)
through tiers 1-2 first, and falls back to Components only when fewer than 5 results
are found. This prevents split components like "before" and "request" from drowning out
the actual compound "before_request".

**Phase 1: Primary keywords (Exact + Compounds) through tiers 1-2:**

| Tier | Condition | What matches | Cap |
|------|-----------|-------------|-----|
| 1: Exact | Always runs | Symbol name equals primary keyword (case-insensitive) | No cap |
| 2: Prefix | < 15 results so far | Symbol name starts with primary keyword | 30 total |

**Phase 2: Component fallback (only when Phase 1 yields < 5 results):**

| Tier | Condition | What matches | Cap |
|------|-----------|-------------|-----|
| 1: Exact | Phase 1 < 5 results | Symbol name equals component keyword (case-insensitive) | No cap |
| 2: Prefix | < 15 results so far | Symbol name starts with component keyword | 30 total |

**Phase 3: All keywords through remaining tiers (always runs):**

| Tier | Condition | What matches | Cap |
|------|-----------|-------------|-----|
| 3: Substring | < 5 results so far | Keyword (4+ chars) appears anywhere in qualified name | 20 total |
| 4: File path | < 30 results so far | Keyword (3+ chars) appears as a path segment | 40 total |
| 5: Interface | After RRF fusion | If any candidate is an interface/type, add all implementors | No cap |

Tier 5 runs after RRF fusion, not within the tiered channel itself. It finds `implements`
edges pointing to matched interface types and adds the implementing types as additional
seeds.

### Channel 2: Equivalence classes (weight 2.0)

Four layers of equivalence classes are checked (115 curated concepts total):

**Layer 1: Seed dictionary** (`equivalence.go`). 21 hand-curated concept classes mapping
natural-language phrases to target symbols. Each concept has a canonical ID, a list of
phrases, and a list of target symbol names. Example:

```
Concept:  TRANSITIVE_IMPACT
Phrases:  "blast radius", "impact analysis", "downstream callers", "ripple effect", ...
Targets:  TransitiveCallers, BlastRadius, blastRadiusTool, handleBlastRadius
Weight:   1.0
```

Phrases are expanded via cross-product with action verbs ("find blast radius", "compute
blast radius", etc.). Matching is case-insensitive substring containment of any phrase
within the task description.

**Layer 2: Universal classes** (`universal_seeds.go`). 63 classes for concepts common to
all software projects: ENTRY_POINT, CONFIGURATION, ERROR_HANDLING, DATABASE, HTTP_SERVER,
AUTHENTICATION, TESTING, CONCURRENCY, CLI, etc. These have weight 0.8 (lower than
knowing-specific seeds at 1.0) and ship as defaults for any repo.

**Layer 3: Language-specific classes** (`language_seeds.go`). 31 classes extending
universal concepts with Python, TypeScript, Rust, Java, and C# symbol patterns. The
universal classes use Go-centric targets (NewConfig, HandleError, ParseFlags); these
add language-specific equivalents (e.g., PY_ENTRY_POINT targets `create_app`, `wsgi_app`;
TS_ROUTING targets `Router`, `useRouter`). Weight 0.8, same as universal. Added after
cross-system benchmark showed knowing scoring lower on non-Go repos partly because
keyword seeding missed language-specific symbol names.

**Layer 4: Graph-derived aliases** (`graph_aliases.go`). Auto-generated from the graph
structure of top-10 tiered results. For each seed node, the system:

1. Looks up callers and callees via `EdgesTo`/`EdgesFrom`.
2. Extracts meaningful words from neighbor symbol names (CamelCase splitting, filtering
   generic words like "handle", "new", "get").
3. Builds bigram phrases from consecutive meaningful words.
4. Creates an equivalence class with weight 0.7 mapping those phrases to the seed symbol.

Example: `TransitiveCallers` is called by `handleBlastRadius`. Splitting yields
`["Blast", "Radius"]`. These become phrases mapping back to `TransitiveCallers`.
So a query containing "blast radius" finds `TransitiveCallers` even though those words
do not appear in its name.

All equivalence targets are resolved to actual nodes via `NodesByName` with exact
symbol-name matching (case-insensitive). Two guards prevent noise:

1. **Generic target filter:** targets <=3 chars or in a blocklist (`get`, `set`, `do`,
   `new`, `run`, `put`, `post`, `call`, `add`, `pop`) are skipped. These resolve to
   hundreds of unrelated symbols on any non-trivial codebase.
2. **Result cap:** equiv results are capped at 2x(tiered+BM25 count). On small graphs,
   unbounded equiv results dominate RRF fusion and flatten RWR scores (Run 22 finding:
   66 equiv results vs 11 primary results caused +136% regression).

**What was tried and rejected (experiment 21):** auto-generating concepts from
CamelCase-split symbol names. CamelCase splitting already makes symbol names searchable
via BM25; auto-concepts only add value when they generate conceptual aliases that differ
from the name, which requires domain understanding.

### Channel 3: BM25 FTS5 (weight 2.0)

Always runs when available. Uses `buildFTSQuery` to construct a compound-targeted query:
compound identifiers (snake_case, CamelCase, dotted) are quoted as phrases and targeted
at the `symbol_name` column for maximum BM25 relevance; simple words are joined with OR
for broad matching. Returns up to 30 results ordered by BM25 relevance.

**What is indexed:**

The FTS5 table indexes five columns from each node (migration 016 added `symbol_name`, migration 017 added `concepts`):

| Column | BM25 weight | Content |
|--------|-------------|---------|
| `symbol_name` | 10.0 | Terminal symbol identifier only (e.g., "QuerySet.filter" instead of full qualified path) |
| `concepts` | 5.0 | CamelCase-split tokens from file name and parent directory (e.g., "commandLineParser.ts" becomes "command Line Parser commandLineParser") |
| `qualified_name` | 3.0 | CamelCase-split qualified name (original tokens preserved alongside splits) |
| `signature` | 1.0 | CamelCase-split function signature |
| `file_path` | 1.0 | File path from the files table |

The `symbol_name` column is extracted by `extractSymbolName` in `sqlite.go`, which strips
the repo URL prefix (everything before `://`), the package/file path (everything up to
the last `/`), and the file extension (`.go`, `.py`, `.rs`, etc.). This means a keyword
search for "before_request" directly matches `Scaffold.before_request` even when the full
qualified name is `github.com/pallets/flask://flask/scaffold.py.Scaffold.before_request`.

**Tokenization** (`splitForFTS` in `sqlite.go`): splits on path separators (`/`, `.`,
`:`, `(`, `)`, `,`, `*`), then splits each segment on CamelCase boundaries and
underscores. Both the original compound token and its parts are indexed.

The FTS5 tokenizer uses `tokenchars '_'` (migration 016) so that snake_case identifiers
like `before_request` are indexed as single tokens. Without this, `before_request` would
be split into `before` and `request`, preventing exact-phrase matching.

Example: `github.com/foo/store.SQLiteStore.SearchBM25Nodes` becomes:
`github com foo store SQLiteStore SQLite Store SearchBM25Nodes Search BM25 Nodes`

`RebuildFTS` runs synchronously after snapshot computation during indexing. It was
previously deferred to a background goroutine, but CLI processes (`knowing index`) exit
immediately after `IndexRepo` returns, killing the goroutine before FTS completes. This
left the FTS index empty in CLI mode. The synchronous rebuild adds ~500ms to index time.

### Channel 4: Vector search (weight 0.0, disabled)

Infrastructure is complete: ONNX runtime, HNSW index, RRF channel wiring. Disabled
because all tested models (MiniLM-L6-v2, BGE-small-en-v1.5) produced net-negative results.
General-purpose embedding models do not understand code vocabulary gaps (e.g., "blast
radius" should match `TransitiveCallers`). Enable with `KNOWING_EMBEDDINGS=1` for
experimentation.

---

## 3. Noise Filtering

`filterNoisySymbols` removes candidates before scoring:

- **Phantom external nodes**: nodes with `kind="external"` or qualified name starting with
  `external://` or `stdlib://`. These are unresolved targets from LSP enrichment (external
  dependencies) or standard library symbols with no user source code. On repos with partial
  LSP coverage (e.g., Spark Java: 2282 phantom nodes, 63% of all nodes), they act as
  probability sinks that starve real symbols of RWR score.
- **Build artifacts**: paths containing `/dist/`, `/build/`, `/vendor/`, `/node_modules/`
- **Minified code**: paths containing `.min.` or `.bundle.`
- **Test fixtures and helpers**: paths containing `conftest.py.`, `fixtures.py.`,
  `/testutil`, `/testhelper`, or `test_helper`
- **Test mocks**: symbols whose type name contains `mock`, `fake`, or `stub`
  (e.g., `mockStore.PutEdge`, `fakeClient.Do`)
- **Minified names**: symbols with 2-character-or-shorter names (except common short names
  like `ID`, `OK`, `DB`, `IP`, `IO`, `Go`, `Do`)

Mock filtering was validated in experiment 5: mocks were ranking above real implementations
because test files generated many caller edges. Phantom node filtering was added in
cross-system Run 20 after Spark Java scored 0.00 P@10 due to external framework references
overwhelming the graph.

---

## 4. Random Walk with Restart

RWR computes a relevance score for every reachable node relative to the seed set.

### Intuition

A random walker starts at a seed node. At each step it either follows an edge to a
neighbor (probability `1 - alpha`) or teleports back to a random seed (probability
`alpha`). The stationary distribution assigns higher scores to nodes that are
structurally close to the seeds and sit at the intersection of many paths.

### Parameters

| Parameter | Value | Notes |
|-----------|-------|-------|
| `alpha` (restart probability) | 0.2 | 20% chance of returning to a seed each step |
| `maxIter` | 20 | Hard cap; early termination usually exits by iteration 8-10 |
| Convergence threshold | 0.001 | L1 norm of difference between consecutive distributions |
| Top-K stability | 10 nodes, 2 iterations | Break early when top-10 ranking unchanged for 2 consecutive iterations |
| BFS depth limit | 4 hops | Pre-loaded subgraph for zero-query iteration |
| RWR score threshold | 0.02 | Nodes below this are discarded before scoring |

### Edge weights

| Edge type | Weight | Rationale |
|-----------|--------|-----------|
| `calls` | 1.0 | Direct call relationships; strongest structural signal |
| `implements` | 0.8 | Interface implementations; tight coupling |
| `implements_rpc` | 0.8 | RPC service implementations; same tier as interface impls |
| `overrides` | 0.8 | Method overrides in OOP; tight coupling |
| `handles_route` | 0.7 | Route bindings; HTTP surface to handlers |
| `extends` | 0.7 | Class inheritance; parent-child relationship |
| `tests` | 0.6 | Test-to-subject relationship |
| `consumes_rpc` | 0.6 | RPC client call sites |
| `imports` | 0.5 | Package-level dependency; weaker than function-level |
| `depends_on` | 0.5 | Build/module dependency |
| `consumes_endpoint` | 0.5 | HTTP client call sites |
| `tested_by` | 0.5 | Inverse of tests |
| `references` | 0.4 | Type/constant usage; weakest structural signal |
| `throws` | 0.4 | Exception throw sites |
| `deployed_by` | 0.4 | Deployment relationship |
| `gated_by_flag` | 0.3 | Feature flag gates |
| `decorates` | 0.3 | Decorator/annotation relationships |
| `documents` | 0.2 | Documentation links; minimal structural coupling |
| `owned_by` | 0.0 | Ownership metadata; zero walk weight (not structural) |
| `authored_by` | 0.0 | Authorship metadata; zero walk weight (not structural) |
| `inherits` | 0.3 (default) | Child-to-parent-method via inheritance propagation; uses default weight |
| unknown | 0.3 | Default for any edge type not in the weight map |

When a node has multiple outgoing edges, probability is distributed proportionally to
edge weight. A node with one `calls` (1.0) and one `imports` (0.5) edge sends 2/3 of
its flow along calls and 1/3 along imports.

### Implementation

`buildAdjacencyMap` pre-loads the reachable subgraph into in-memory adjacency maps before
iteration begins. The RWR loop requires zero database queries. Two loading strategies:

1. **Pre-computed adjacency cache** (preferred): a compact binary blob (65 bytes/edge)
   stored in the `graph_notes` table at index time. Format: `[num_edges:4 LE]` followed by
   `[source:32][target:32][type_id:1]` per edge. Loads in one SQLite read, then runs BFS
   in memory. Works for repos up to 500K edges (~32MB base64). Cache version: v2.
2. **BFS fallback**: per-node `EdgesFrom`/`EdgesTo` queries, 4-hop depth limit from seeds.
   Used when no cache exists or the cache is stale.

Dead-end nodes (no outgoing edges) redistribute their probability back to the seed set,
effectively acting as an implicit restart.

After convergence, scores are normalized to [0, 1] relative to the maximum. The RWR
score is scaled to an integer 0-100 range to serve as the `CallerCount` proxy in the
scoring formula.

Before scoring, the RWR result loop skips phantom external nodes (`kind="external"` or
`external://` prefix). These nodes may accumulate RWR probability from import edges but
contain no source code and must not enter the scoring pipeline. This filter is separate
from `filterNoisySymbols` (which runs on seed candidates before RWR) and catches external
nodes that were reached by walk propagation rather than seeding.

### Critical finding: RWR is the primary differentiator

Cross-system benchmark Runs 7-10 demonstrated that RWR (graph traversal) is the
primary source of knowing's retrieval advantage over text search. FTS adds minimally
because tiered search already finds the same symbols by keyword. Import resolution
(Python: 63 edges, TypeScript: 5,684 edges, Rust: 9,795 edges) helps because it
creates more edges for RWR to walk, improving recall on cross-file tasks.

### Graph connectivity improvements that feed RWR

Two index-time enrichments significantly expand RWR's reachable subgraph:

**Inheritance propagation** (`propagateInheritance` in the indexer post-processing pass):
For each `extends` edge, creates `inherits` edges from child classes to all parent class
methods. This is language-agnostic (works on any extractor producing `extends` edges and
`method` nodes). Flask: 83 edges. Django: 14,539 edges. Cross-system P@10 jumped from
0.155 to 0.200 (+29%), the single largest improvement of any change (Run 13). The
mechanism: RWR can now walk from `Flask` to `Scaffold.before_request` via the inheritance
chain, whereas previously it could not reach parent methods from child class seeds.

**Deeper call chain extraction** (Python extractor):
Walks into call arguments, lambda bodies, and nested function definitions to extract
nested calls and callbacks. Previously `map(process, items)` only extracted the `map`
call, missing `process` as a target. Flask: 5,022 to 9,237 edges (+84%). Django: 151K to
185K edges (+22%). More edges means more RWR connectivity.

### LSP enrichment: measured net-neutral for retrieval

LSP enrichment (running a language server to resolve references at higher confidence)
was measured on Flask and Django with identical results: **zero retrieval quality difference**
with or without enrichment (80/80 and 50/50 real symbols in top-10 respectively).

Why: LSP upgrades existing tree-sitter edges from 0.7 to 0.9 confidence, but RWR weights
by edge type (calls=1.0, imports=0.5), not confidence. The edges already exist; LSP just
makes them slightly more certain. LSP also creates phantom external nodes (59-67% of all
nodes on Flask/Django) that bloat the FTS index and add 53% latency overhead.

LSP enrichment remains useful for non-retrieval purposes (blast radius confidence display,
dead code detection requiring high-confidence edges) but the retrieval pipeline does not
benefit from it. The tree-sitter extractor + import resolution + inheritance propagation
is self-sufficient for RWR-based ranking.

Benchmark: `bench/time-to-consistency/TestEnrichmentROI` and `TestEnrichmentROI_Django`.

### What was tried and rejected

- **Confidence-weighted transitions** (experiment 13): weighting by edge confidence
  (lsp_resolved 0.9 > ast_inferred 0.7) caused generic infrastructure nodes to
  accumulate too much probability. -11pp on easy tier.
- **Lower alpha (0.15)** (experiment 14): more exploration helped hard tier (+1.3pp) but
  destroyed easy (-11pp). More exploration means more noise for queries with good seeds.
- **Adaptive alpha** (experiment 15): adapting walk depth to seed count still hurt easy.
  The problem is seed quality, not walk parameters.

---

## 5. HITS Reranking

After RWR, HITS (Hyperlink-Induced Topic Search) runs on the top-200 nodes (by RWR
score) to separate structurally important symbols from merely proximate ones.

### How HITS works on code graphs

HITS computes two scores per node:

- **Authority**: sum of hub scores of nodes pointing to it. In code: heavily called
  functions, core types, key interfaces.
- **Hub**: sum of authority scores of nodes it points to. In code: orchestrators, entry
  points, functions that wire things together.

The algorithm iterates (10 iterations), normalizing by L2 norm each step.

### How HITS scores are used in ranking

HITS does not replace the scoring formula; it adds an `authorityAdj` component:

| Node type | Condition | Adjustment |
|-----------|-----------|------------|
| Seed + high authority (> 0.05) | Task-relevant AND structurally central | `+authority * 0.25` |
| Non-seed + high authority (> 0.2) | Generic infrastructure (types.Hash, GraphStore) | `-authority * 0.15` |
| Seed + high hub (> 0.1) | Entry point that orchestrates task-relevant code | `+hub * 0.10` |
| Non-seed hub | Not used | No adjustment |

This design penalizes generic infrastructure that RWR promotes (because everything calls
it) while boosting seed nodes that happen to also be structurally central.

---

## 6. Scoring Formula

`RankSymbols` computes a final composite score from 6 components. Two modes exist
depending on whether HITS scores are available.

### Base mode (no HITS)

```
score = blast_radius * 0.40
      + confidence   * 0.25
      + recency      * 0.20
      + distance     * 0.15
      + feedback
      + session
```

### HITS-enhanced mode (default for ForTask)

```
score = blast_radius * 0.35
      + confidence   * 0.20
      + recency      * 0.15
      + distance     * 0.15
      + authorityAdj
      + feedback
      + session
```

### Component details

**Blast Radius (0.40 base / 0.35 HITS)**

In `ForTask`: the RWR score scaled to 0-100, normalized relative to the maximum in the
current result set. In `ForFiles`: actual count of incoming `calls` edges.

Normalization is always relative, not absolute. The symbol with the most callers in the
set gets the full contribution.

**Confidence (0.25 base / 0.20 HITS)**

The highest confidence value from all edges pointing to this node. Ranges from 0.0 to
1.0 based on provenance tier:
- `lsp_resolved`: 0.9
- `ast_inferred`: 0.7
- Runtime edges: 0.2 to 0.95 depending on observation count

**Recency (0.20 base / 0.15 HITS)**

How recently this symbol was observed in runtime traces:

| Age | Score |
|-----|-------|
| Within 1 day | 1.0 |
| Within 7 days | 0.8 |
| Within 30 days | 0.5 |
| Over 30 days | 0.2 |
| No runtime data (static only) | 0.3 |

The 0.3 base for static-only symbols prevents codebases without runtime instrumentation
from losing 20% of every symbol's score.

**Distance (0.15)**

Inverse of graph hops from the target: `1 / (1 + hops) * weight`.
- Distance 0 (seed/direct match): full contribution
- Distance 1 (one hop): half contribution

In `ForTask`, distance is binary: 0 for seeds (direct keyword matches), 1 for all
RWR-discovered nodes.

**Feedback (asymmetric: pos=0.25, neg=0.05)**

From `FeedbackProvider`. Score is `useful/(useful+not_useful)`, range [0, 1]. Asymmetric
weighting (tuned via 28-point grid sweep): score 1.0 contributes +0.25 (`FeedbackPosWeight`),
score 0.5 contributes 0, score 0.0 contributes -0.05 (`FeedbackNegWeight`). Strong boost
for confirmed-useful symbols; gentle penalty for potentially-irrelevant ones prevents
over-penalizing symbols incorrectly marked "not useful".

As of v0.5.0, feedback records are merkleized: each stores the SubgraphRoot of the symbol's
package at feedback time. When querying, only records where `neighborhood_root` matches the
current SubgraphRoot are counted, providing automatic expiration when code changes. Adds
11% overhead (255µs → 284µs for 100 symbols).

Task memory boosts (see section 8) compound into this channel at 0.3x scale.

**Session (0.20 weight)**

From `SessionTracker`. Raw boost range [0, 2.0], normalized to [0, 1] before weighting.
Maximum contribution is +0.20 for a symbol accessed multiple times very recently.

### Test file deprioritization

Symbols from test files receive a 0.3x score penalty after the composite score is
computed. Detection uses `isTestFilePath` (path-based, not name-based): patterns include
`/tests/`, `_test.go`, `.test.ts`, `.spec.ts`, `/__tests__/`, and similar conventions.

The penalty is conditional: when the task description mentions testing (e.g., "add a test
for", "fix the test that"), the penalty is removed so test symbols rank normally.

This was added in Run 12 of the cross-system benchmark. On its own the impact was
marginal (P@10 held at 0.155), but it reduces noise from test symbols appearing in the
top-10 results (36% of misses were test symbols per failure analysis).

---

## 7. Session-Aware Boosts

`SessionTracker` (`session.go`) records which symbols are returned by context queries
during the current MCP server lifetime.

### Decay model

Each access timestamp is stored. The boost from a single access at time `t` decays as:

```
boost = exp(-t * ln2 / halfLife)
```

where `halfLife = 180 seconds` (3 minutes). Multiple accesses compound additively but
are capped at 2.0.

| Time since access | Single-access boost |
|-------------------|-------------------|
| 0 seconds | 1.0 |
| 3 minutes | 0.5 |
| 6 minutes | 0.25 |
| 9 minutes | 0.125 |

The 3-minute half-life is tuned for AI agent sessions where context queries fire every
30-90 seconds. Symbols from 5+ minutes ago contribute negligibly.

### Cap

Maximum boost is 2.0, preventing runaway dominance from a symbol appearing in every
query. Boosts below 0.01 are discarded.

### History per symbol

Up to 20 access timestamps are retained per symbol. Older entries are dropped on new
access.

### Recording

After packing, all returned symbol hashes are recorded via `RecordBatch`. This means
the act of returning a symbol makes it more likely to appear in subsequent queries,
creating a useful recency bias within a working session.

---

## 8. Task Memory (Passive Learning)

`TaskMemory` (`task_memory.go`) persists which symbols were useful for which tasks,
enabling the pipeline to learn from past interactions. Over time, the system develops
per-repo vocabulary: "when a developer asks about X, these symbols tend to be what they
actually need." Task memory persists across process restarts via the SQLite `task_memory`
table, so quality compounds with usage even across MCP server restarts.

### Recording

After packing, the top 5 symbols (by score) are stored alongside normalized keywords
from the task description. Keywords are produced by `NormalizeKeywords` (the 10 longest
terms from keyword extraction, joined with spaces). The association is stored in the
`task_memory` SQLite table with a timestamp and a boost score of 1.0. The boost score
is transformed during recall (see "Integration with scoring" below) via the formula
`0.5 + recallScore * 0.4` (range [0.5, 0.9]).

### Recall

On each query, `Recall` searches the task_memory table for rows where any query keyword
(3+ chars) appears in the stored keywords (via SQL `LIKE %keyword%`). Each matching row
contributes its stored score, decayed by age.

**Decay model:**

- Memories less than 7 days old: full weight (decay = 1.0)
- Memories older than 7 days: linear decay (`7 / age_in_days`)

A memory from 14 days ago has half the weight of a fresh one. A memory from 70 days ago
has 1/10 the weight.

### Integration with scoring

Memory boosts are integrated into the feedback channel via replacement (not addition).
The memory recall score is transformed to a boost value:

```
memoryBoost = 0.5 + (recallScore * 0.4)    // range [0.5, 0.9]
if FeedbackBoost < memoryBoost {
    FeedbackBoost = memoryBoost             // replace only if memory is stronger
}
```

This ensures memory always produces a positive boost (range [0.5, 0.9]) without
overwhelming explicit feedback (which can reach 1.0). The positive feedback weight
is 0.25, so memory's effective maximum contribution is `0.9 * 0.25 = 0.225`
when centered around the neutral point (where 0.5 maps to zero contribution), giving
an effective range of 0 to +0.10 (about 10% of total score).

---

## 9. Budget Packing

`packIntoBudget` selects symbols to maximize total relevance within the token budget
using density-ranked packing.

### Algorithm

1. Compute density for each symbol: `density = score / estimated_tokens`.
2. Sort by density descending (ties broken by raw score).
3. Greedily pack: for each item in density order, include it if it fits within the
   remaining budget. Skip items that do not fit and continue trying smaller ones.
4. Re-sort the packed set by score descending for output ordering.

This is a greedy fractional knapsack approximation. It prefers small high-value symbols
(types, constants) over large medium-value symbols (long functions) when budget is tight.

### Token estimation

`EstimateNodeTokens` estimates cost as the token count of `qualified_name + kind +
signature` concatenated, using the approximation of 4 characters per token. This is a
lower bound; actual output includes XML/Markdown overhead.

### Default budgets

- `ForTask`: 50,000 tokens
- `ForFiles`: 50,000 tokens
- `ForPR`: 8,000 tokens

### Persistent pack cache

`ForTask` uses a two-layer caching strategy:

1. **In-memory SubgraphCache**: keyed by `sha256("task\x00" + normalized_task)`. Provides
   instant replay within a single process lifetime.
2. **Persistent notes table** (`graph_notes`): keyed by
   `sha256("context_pack\x00" + normalized_task)`, stored via `PutNote` with key
   `"context_pack"`. Survives process restarts, enabling cross-session replay.

**Staleness detection**: the persisted pack includes the `SnapshotHash` at write time.
On cache hit, the engine compares it against `LatestSnapshot` for the repo. If the
snapshot hash differs (i.e., the graph was re-indexed), the cached pack is considered
stale and the full pipeline re-runs. This ensures results reflect the current code state
while avoiding redundant computation across identical queries.

---

## Tuning Guidance

### What to change

**Token budget.** Increase beyond 50,000 when working on highly connected symbols or
cross-package tasks. Decrease for faster, more focused results.

**Equivalence classes.** The highest-ROI tuning lever. Adding phrases to existing
concepts is cheap, safe, and has consistent returns. Adding new concepts for
domain-specific vocabulary gaps is the primary way to improve hard-tier retrieval.
Current count: 115 (21 seed + 63 universal + 31 language-specific). Further expansion
targets domain-specific concepts not covered by universal/language layers.

**BM25 column weights.** The current weights (symbol_name: 10.0, concepts: 5.0,
qualified_name: 3.0, signature: 1.0, file_path: 1.0) were tuned to prioritize terminal
symbol name matches. The symbol_name column (added in migration 016) carries the highest
weight because developers search by symbol name, not by full qualified path. The concepts
column (migration 017) stores CamelCase-split file/module names, bridging the gap when
developers say "parser" but the symbol is inside `commandLineParser.ts`. Adjusting these
could improve BM25 precision for specific codebases.

### What not to change

**RWR alpha.** Experiments 13-16 exhaustively tested alpha values (0.12, 0.15, 0.20,
0.25) and adaptive schemes. Every change that helped hard tier destroyed easy tier. The
problem is seed quality, not walk depth. Leave alpha at 0.2.

**RRF channel weights.** Unweighted (1:1) fusion was catastrophic in early eval
(experiment 7, -28pp easy tier). The current 2:2:0:2 ratio (tiered:BM25:vector:equivalence)
was validated on the cross-system benchmark (Runs 7-10): equalizing tiered and BM25
improved P@10 because both channels find the same symbols, so suppressing BM25 was
wasting recall without improving precision.

**RWR convergence threshold.** 0.001 provides good balance. Tighter convergence has
negligible impact on ranking; looser convergence risks instability.

**Embedding weight.** Keep at 0.0 until a code-tuned model is available. General-purpose
models (MiniLM, BGE-small) tested net-negative at every weight level (experiments 9-12).

### What the experiments taught (21 experiments, summarized)

1. **The eval was the biggest bug.** Fixing the `isRelevant()` matching function was
   worth +8pp overall (experiment 4). Always validate the measurement before tuning the
   system.

2. **Seed quality dominates everything.** RWR parameter tuning is a dead end when seeds
   are wrong. Improving seed selection (equivalence classes, bigram compounds) produced
   all meaningful gains.

3. **RRF fusion weights depend on channel overlap.** Initially tiered >> BM25 was correct
   (experiment 7). Cross-system benchmark Runs 7-10 revealed tiered and BM25 find the
   same symbols, so equalizing them (2:2:2) improved recall without precision loss.

4. **Off-the-shelf embeddings do not help code retrieval.** MiniLM and BGE-small lack
   code-domain vocabulary understanding. "Blast radius" and `TransitiveCallers` are
   semantically identical in this domain but distant in embedding space.

5. **Bigram compounds are high ROI.** Simple heuristic, no dependencies, cracks
   previously impossible fixtures (experiment 8).

6. **Mock filtering matters.** Test mocks accumulate many caller edges and outrank real
   implementations without filtering (experiment 5).

7. **Equivalence classes are the highest-ROI retrieval feature.** 21 concepts produced
   +8pp hard tier, +3.8pp overall (experiment 18). Local, deterministic, zero cost.

8. **Targeted beats untargeted.** Equivalence classes (explicit phrase-to-symbol mapping)
   outperform BM25 enrichment (dumping neighbor names into the index). This applies to
   doc comments, neighbor names, and any "add more text to the index" approach
   (experiments 17, 20).

9. **Expanding existing classes is cheap and safe.** Near-zero risk, consistent returns
   (experiment 19, +1.1pp overall).

10. **Graph-derived aliases work through the equivalence system.** The right abstraction
    for graph-derived knowledge is targeted phrase-to-symbol mappings, not untargeted
    text enrichment in BM25 (lesson from experiment 20 failure vs. graph_aliases.go
    success).

---

## Score Interpretation

### Explain mode (`knowing why`)

To inspect the full scoring breakdown for a specific symbol, use `knowing why`.
This runs the complete pipeline (keyword extraction, seed selection, RWR, HITS,
scoring) and isolates one symbol's contribution from each component: seed
channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency,
distance, feedback weight, session boost, and equivalence class matches.

```bash
knowing why -task "refactor auth" -symbol "SessionHandler"
```

See the [CLI reference](../guide/cli.md#why) for full usage and example output.

### Ranges

| Range | Meaning |
|-------|---------|
| 0.70 - 1.00 | Highly relevant. Direct keyword match, many callers, confirmed by LSP, recently observed. |
| 0.40 - 0.70 | Moderately relevant. Structurally connected but not a direct match. |
| 0.20 - 0.40 | Peripheral. Extended context, 2+ hops away, lower confidence. |
| < 0.20 | Filtered out by RWR threshold (0.02) or scoring. |

### Flat scores

When many symbols score similarly (cluster at 0.44-0.48), it usually means:

- The RWR walk found a densely connected subgraph with even probability distribution.
- All symbols have similar blast radius (relative normalization makes them equal).
- Keywords matched too broadly, diluting the seed set.

Distance (0.15) and confidence (0.20-0.25) become the tiebreakers.

### Unexpectedly high scores

Hub nodes (called by many others) accumulate RWR probability regardless of direct task
relevance. A utility function like `requireHash` that validates parameters in every
handler will score 0.78+ on any handler-related query. This is by design: high
blast-radius symbols deserve attention when making changes.

---

## Design Position: Why Equivalence Classes Over Embeddings

Other retrieval tools built their pipelines with embeddings as the primary
concept-matching layer. Without embeddings, they only have BM25 (lexical). So embeddings
fill a critical gap for them.

knowing's pipeline has equivalence classes filling the same role, and they outperform
embeddings on our eval. Adding embeddings on top of equivalence classes is redundant:
both try to bridge "blast radius" to "TransitiveCallers". When one system already
handles it, the other just adds noise.

Embeddings would be net positive only if they catch concepts that equivalence classes
miss (vocabulary not manually defined) AND do it with fewer false positives than the
current signal-to-noise ratio. A code-tuned model might achieve the first condition, but
the second is the hard part: the pipeline is already precise enough that additional noise
hurts the easy tier.

**The strategic conclusion:** equivalence classes + graph-derived aliases + task memory is
knowing's retrieval path. It is local, deterministic, inspectable, and compounds with
use. That is the moat, not a better embedding model. The embedding infrastructure stays
as an optional plugin (KNOWING_EMBEDDINGS=1), not the core strategy.

This is validated by 23 experiments (see `eval/EXPERIMENTS.md`):
- Experiments 9-12: MiniLM and BGE-small tested net-negative at every weight level
- Experiment 17: doc comments in BM25/embed text did not compensate for model weakness
- Experiment 18: equivalence classes produced +8pp hard tier, the largest single-feature gain
- Cross-repo eval: 46.7% R@10 on a foreign codebase with zero configuration, using universal
  equivalence classes and graph-derived aliases (no embeddings)

---

## Limitations

1. **Vocabulary gap beyond equivalence classes.** The 115 curated concepts (21 seed + 63
   universal + 31 language-specific) cover common patterns but not every domain concept.
   Queries using terminology not covered by any class fall back to lexical matching only.

2. **Embeddings disabled.** Vector search infrastructure exists but is disabled (weight 0).
   Optional via KNOWING_EMBEDDINGS=1 for experimentation with code-tuned models.

3. **LIKE-based tiered matching.** `NodesByName` uses SQL `LIKE %keyword%`, so "auth"
   matches `AuthService`, `OAuth2Handler`, and `unauthorized_error` equally.

4. **Static call graph for blast radius.** `TransitiveCallers` follows only `calls`
   edges. Runtime cross-service edges influence recency scoring but not structural
   ranking.

5. **Full subgraph load per query.** The reachable subgraph is loaded on every query
   (from pre-computed cache or BFS fallback). The 40-candidate seed cap and 4-hop BFS
   depth limit bound the subgraph size. Early termination (top-K stability) reduces
   iteration count on large graphs.

6. **Token estimation is approximate.** The 4-characters-per-token heuristic works
   reasonably for code but can over- or under-estimate for symbolic or prose-heavy code.

---

## Source Files

| File | What it contains |
|------|-----------------|
| `internal/context/context.go` | `ForTask`, `ForFiles`, `ForPR`, `extractKeywordSet`, `KeywordSet`, `buildFTSQuery`, `rrfFuseMulti`, `packIntoBudget`, `filterNoisySymbols` |
| `internal/context/explain.go` | `ExplainSymbol`, `tieredSearchSet`, `bm25Search`, `vectorSearch`, `equivSearch` |
| `internal/context/equivalence.go` | `seedEquivalenceClasses`, `matchEquivalenceClasses`, `EquivalenceClass` type |
| `internal/context/universal_seeds.go` | `universalEquivalenceClasses` (63 cross-project concepts) |
| `internal/context/language_seeds.go` | `languageEquivalenceClasses` (31 language-specific concepts) |
| `internal/context/graph_aliases.go` | `graphDerivedAliases`, `extractMeaningfulWords` |
| `internal/context/ranking.go` | `RankSymbols`, `ScoringInput`, `ScoreComponents`, `recencyFromTimestamp` |
| `internal/context/walk.go` | `RandomWalkWithRestart`, `buildAdjacencyMap`, `BuildAdjacencyCache`, `topKFromProb`, `adjEdgeTypeToID` |
| `internal/context/hits.go` | `ComputeHITS`, `HITSScores` |
| `internal/context/session.go` | `SessionTracker`, `SessionBoosts` |
| `internal/context/task_memory.go` | `TaskMemory`, `Recall`, `RecordBatch`, `NormalizeKeywords` |
| `internal/store/sqlite.go` | `SearchBM25Nodes`, `RebuildFTS`, `splitForFTS`, `splitCamelCase` |
| `eval/EXPERIMENTS.md` | All 21 experiment logs with results |
