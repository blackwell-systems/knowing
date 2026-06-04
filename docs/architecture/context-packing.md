# Graph-Aware Context Packing

The context packing system (`internal/context/`) transforms a task description or set of
changed files into a token-budgeted, relevance-ranked block of symbols from the knowledge
graph. It is the core of knowing's value proposition for AI agent workflows: given a
natural-language task, produce the most relevant code symbols that fit within a context
window.

## How It Works (End-to-End Flow)

The pipeline has eleven stages:

```
Task Description
    |
    v
[1. Cache Check]           -- compute cache key from normalized task; return cached result on hit
    |
    v
[2. Keyword Extraction]    -- produce KeywordSet (Exact/Compounds/Components); backtick-quoted identifiers get highest priority
    |
    v
[3. Seed Selection]        -- 5-channel RRF fusion (tiered keywords, BM25, vector, equivalence classes, path-context)
    |
    v
[4. Noise Filtering]       -- exclude external nodes, mock/stub/fake symbols, test fixtures, build artifacts
    |
    v
[5. Random Walk with Restart]  -- propagate relevance through graph structure (BFS depth limit: 4 hops)
    |
    v
[6. Scoring]               -- 7-component formula: blast_radius + confidence + recency + distance + session_boost + feedback + commit_recency
    |
    v
[7. HITS Reranking]        -- authority/hub scores promote structurally important nodes
    |
    v
[8. Token Budget Packing]  -- density-ranked greedy knapsack (score/cost ratio)
    |
    v
[9. PackRoot Computation]  -- hash(task_normalized + sorted selected_node_hashes) for content-addressed identity
    |
    v
[10. Cache Store]          -- store result in SubgraphCache keyed by PackRoot
    |
    v
[11. Output Formatting]    -- XML / Markdown / JSON / GCF / GCB with distance-based grouping
```

Three entry points exist:

- `ForTask(ctx, TaskOptions)`: starts from a task description string. Checks subgraph cache first (step 1). On miss: extracts keywords, finds seed nodes, runs RWR, scores, packs, caches result.
- `ForFiles(ctx, FileOptions)`: starts from a list of changed file paths. Finds all nodes in those files, adds their direct callers as distance-1 candidates, scores, packs.
- `ForPR(ctx, PROptions)`: similar to ForFiles but with a larger default budget (8000 tokens) and support for GCF output format.

## The Keyword Extraction Pipeline

The `extractKeywordSet` function (`context.go`) converts a free-text task description into
a structured `KeywordSet`. The pipeline has four phases:

**Phase 1: Backtick-quoted identifiers (Exact tier)**

Scan the description for backtick-quoted spans (e.g., `` `buildPythonImportMap` ``). These
are extracted verbatim as Exact keywords without splitting, filtering, or expansion. They
receive the highest search priority. A lowercase variant is also added when different from
the original.

**Phase 1.5: Code pattern detection (Compounds tier)**

`extractCodePatterns` detects unambiguous code references in the task description before
standard word extraction. Conservative: only fires on patterns that are clearly code.
Fires on method calls with parens (`.delete()`, `QuerySet.annotate()`), Class.method
dotted paths where the left side starts uppercase (`ModelAdmin.get_inlines`), and dotted
paths with underscores on either side (`django.utils.html.escape`). Does not fire on
prose abbreviations (e.g., i.e.), version numbers (3.9), or generic lowercase dotted
words without underscores. Both original case and lowercase variants are added to Compounds.

**Phase 2: Standard word extraction (Compounds and Components tiers)**

1. **Tokenize:** Split on whitespace with `strings.Fields`.
2. **Strip punctuation:** Remove leading/trailing `.,;:!?"'` and brackets from each word.
3. **Verb-pattern detection:** If the first word is an action verb (`add`, `fix`, `refactor`,
   etc.), the first noun after it becomes a priority target. Compound identifiers go directly
   to Compounds; simple words go to Components with a capitalized variant.
4. **Compound detection:** Words containing `_`, `.`, or splitting into multiple CamelCase
   parts are added to Compounds (both lowercase and original-case forms).
5. **Component splitting:** Decompose CamelCase and snake_case into individual words. Filter
   stop words, action verbs, and terms shorter than 2 characters. Expand abbreviations
   (`ctx` adds `context`, `cfg` adds `config`, etc.).

**Phase 3: Bigram compound generation**

Adjacent non-stop-words of sufficient length (4+ chars, not action verbs) are joined into
synthetic compounds: both CamelCase (`SnapshotDiffing`) and snake_case (`snapshot_diffing`)
forms. These augment the Compounds tier for multi-word concept matching.

### KeywordSet: Three-Tier Priority System

The extraction pipeline produces a `KeywordSet` struct rather than a flat list. The struct
separates keywords into three priority tiers:

| Tier | Contents | Example |
|------|----------|---------|
| **Exact** | Backtick-quoted identifiers from the task description | `` `buildPythonImportMap` `` |
| **Compounds** | snake_case, CamelCase, and dotted identifiers preserved whole | `route_handler`, `HandleLogin`, `flask.app` |
| **Components** | Split words from compound decomposition | `route`, `handler`, `handle`, `login` |

Backtick-quoted identifiers bypass normal extraction entirely: they are not split, filtered,
or expanded. They become Exact keywords with the highest search priority, giving agents a
way to request specific symbol lookups without ambiguity.

The `tieredSearchSet` method (which unified the ForTask inline search and the ExplainSymbol
method's separate implementation) queries primary keywords (Exact + Compounds via `Primary()`)
through exact and prefix tiers first. It only falls back to Components when the primary search
produces fewer than 5 results. This prevents over-split identifiers from flooding the seed set
with false matches. The BM25 path also benefits: `bm25Search` now uses `buildFTSQuery`
everywhere (previously the ExplainSymbol path had its own query builder).

### Example

Task: "add a new MCP tool for snapshot diffing"

KeywordSet produced:
- Exact: `[]` (no backtick-quoted identifiers)
- Compounds: `["SnapshotDiffing", "snapshot_diffing"]` (bigram generation from adjacent words)
- Components: `["mcp", "Mcp", "snapshot", "diffing", "tool"]` (priority term + split words)

Removed: "add" (action verb stop word), "a" (English stop word), "new" (filler word),
"for" (English stop word).

### Example (compound-first)

Task: "fix the `buildPythonImportMap` to handle relative imports"

KeywordSet produced:
- Exact: `["buildPythonImportMap", "buildpythonimportmap"]` (backtick-quoted + lowercase variant)
- Compounds: `["RelativeImports", "relative_imports"]` (bigram generation)
- Components: `["handle", "relative", "imports"]`

Search order: Exact first (direct symbol lookup), then compounds, then
components only if fewer than 5 results found.

## Random Walk with Restart

### Intuition

Random Walk with Restart (RWR) computes a relevance score for every node in the graph
relative to a set of seed nodes. Imagine a random walker starting at one of the seed nodes.
At each step, the walker either:

- Follows an outgoing or incoming edge to a neighbor (with probability `1 - alpha`), or
- Teleports back to a randomly chosen seed node (with probability `alpha`)

After many steps, the fraction of time spent at each node converges to a stationary
distribution. Nodes that are structurally close to the seeds, or that sit at the
intersection of many paths from the seeds, accumulate higher scores. Hub nodes (highly
connected) score higher because the walker passes through them more often.

### Parameters

- **seeds:** The initial node set (from keyword matching). Uniform weight across all seeds.
- **alpha:** Restart probability, fixed at 0.2. This means 20% chance of returning to a seed
  at each step. Lower alpha lets the walk explore further from seeds; higher alpha keeps
  scores concentrated near seeds.
- **maxIter:** Maximum iterations, fixed at 20. In practice, convergence (delta < 0.001)
  typically occurs within 5-10 iterations for knowing's graph sizes.

### Edge weights by type

The walk is not uniform across edges. Each edge type has a weight that determines the
fraction of probability mass that flows through it. The full weight map
(`edgeWeights` in `walk.go`):

| Weight | Edge Types |
|--------|-----------|
| 1.0 | `calls` |
| 0.8 | `implements`, `implements_rpc`, `overrides` |
| 0.7 | `handles_route`, `extends` |
| 0.6 | `tests`, `consumes_rpc`, `accesses_field` |
| 0.5 | `imports`, `depends_on`, `consumes_endpoint`, `tested_by`, `co_tested_with`, `type_hint_of`, `executes_process` |
| 0.4 | `reads_env`, `references`, `throws`, `deployed_by` |
| 0.3 | `gated_by_flag`, `decorates` |
| 0.2 | `documents` |
| 0.15 | `similar_to` |
| 0.0 | `contains`, `member_of`, `owned_by`, `authored_by` (structural, excluded from walk) |

LSP-enriched edges (`lsp_resolved` provenance) receive an additional 0.3x attenuation
(`lspEdgeWeight()` in `sweep.go`) to prevent enrichment from inflating centrality of
framework wiring symbols. Override with `BENCH_LSP_EDGE_WEIGHT`.

Unknown edge types default to 0.3. When a node has multiple outgoing edges, probability
is distributed proportionally to edge weight. A node with one `calls` edge (1.0) and one
`imports` edge (0.5) sends 2/3 of its flow along the calls edge and 1/3 along imports.

### Convergence

The algorithm iterates until the L1 norm of the difference between consecutive probability
vectors drops below 0.001, or maxIter is reached. After convergence, scores are normalized
to [0, 1] relative to the maximum score in the distribution.

### Implementation details

The `buildAdjacencyMap` function pre-loads the reachable subgraph (BFS from seeds, limited
to 4 hops) into in-memory adjacency maps before the iteration loop begins. This means the
RWR iteration requires zero database queries, making it fast even for large subgraphs. The
4-hop depth limit improves performance without affecting ranking quality.

Dead-end nodes (no outgoing edges) redistribute their probability back to the seed set,
effectively acting as an implicit restart.

## The Scoring Formula

After RWR produces raw relevance scores, the `RankSymbols` function computes a final
composite score. With HITS active (the primary path, applied to top-200):

```
score = blast_radius * 0.35 + confidence * 0.20 + recency * 0.15 + distance * 0.15 + authorityAdj + feedback + session + commit_recency
```

Without HITS (fallback), weights shift slightly to compensate for the missing authority signal:

```
score = blast_radius * 0.40 + confidence * 0.25 + recency * 0.20 + distance * 0.15 + feedback + session + commit_recency
```

Base weights are sweep-tunable via `internal/context/sweep.go` (`sweepBlastW`, `sweepConfW`, `sweepRecencyW`, `sweepDistanceW`). The non-HITS path adds +0.05 to blast, confidence, and recency.

Feedback weight is asymmetric: pos=0.25, neg=0.05 (centered around 0). Session weight is 0.20 (normalized from [0, 2.0] cap).

### Blast Radius (40% base, 35% with HITS)

Measures how many other symbols depend on this one. In the `ForTask` path, the RWR score
is used as a proxy (scaled to 0-100 and normalized relative to the maximum in the result
set). In the `ForFiles` path, the actual count of incoming `calls` edges is used.

Normalization is relative to the maximum in the current result set, not a hardcoded cap.
A symbol with the most callers in the set gets the full contribution.

### Confidence (25%)

Directly from the provenance tier of the best edge pointing to this node:
- `lsp_resolved`: 0.9, contributing up to 0.225
- `ast_inferred`: 0.7, contributing up to 0.175
- Runtime edges: varies from 0.2 to 0.95

### Recency (20%)

How recently this symbol was observed in runtime traces:
- Observed today: 1.0 (contributes 0.20)
- Observed within 7 days: 0.8 (contributes 0.16)
- Observed within 30 days: 0.5 (contributes 0.10)
- Observed over 30 days ago: 0.2 (contributes 0.04)
- No runtime data (static only): 0.3 (contributes 0.06)

The base score of 0.3 for static-only symbols prevents codebases without runtime
instrumentation from losing 20% of every symbol's score to zeros.

### Distance (15%)

Inverse of hops from the target: `1 / (1 + hops) * 0.15`.
- Distance 0 (direct keyword match / in-file symbol): 0.15
- Distance 1 (one hop away): 0.075
- Distance 2: 0.05

### Session Boost (20%)

Provided by `SessionTracker` (`internal/context/session.go`). Records which symbols are
returned by context queries during the current MCP server lifetime. A symbol accessed T
seconds ago receives a boost of `min(2.0, exp(-T * ln2 / 180))` (3-minute half-life). The
cap of 2.0x prevents runaway amplification. Symbols not accessed in the current session
receive a boost of 0.0, contributing nothing to the score.

The half-life is tuned for AI agent sessions where a context query every 30-90 seconds is
typical. Symbols accessed within the last minute receive near-maximum boost; those from
5+ minutes ago contribute negligibly.

### Feedback (asymmetric: pos=0.25, neg=0.05)

Historical usefulness signals from the `feedback` MCP tool. The usefulness ratio (useful / total) is centered around 0.5: symbols with >50% usefulness receive a positive boost, <50% receive a penalty. Weights are asymmetric: positive feedback contributes up to +0.25, negative feedback penalizes up to -0.05. This asymmetry means positive signal accumulates faster than negative signal erodes, reflecting the insight that explicit positive feedback is higher-confidence than negative.

As of v0.5.0, feedback records are merkleized: each stores the SubgraphRoot of the symbol's package at feedback time. When querying feedback, only records where `neighborhood_root` matches the current SubgraphRoot are counted. This provides automatic expiration: feedback becomes invalid when the symbol's package changes (any edge modification in the package invalidates the neighborhood root). Adds 11% overhead (255µs → 284µs for 100 symbols). Backward compatible: NULL `neighborhood_root` uses the legacy path (no expiration).

## Seed Retrieval: 5-Channel RRF Fusion

Seed selection uses Reciprocal Rank Fusion (RRF) across five channels. The function `rrfFuseMulti`
merges ranked lists from all channels into a single seed set:

| Channel | Weight | Source |
|---------|--------|--------|
| 1. Tiered keyword matching | 2.0 | 4-tier exact/prefix/substring/path matching (compound-first) |
| 2. BM25 FTS5 | 2.0 | SQLite FTS5 over 6 columns: symbol_name (10x), concepts (5x), file_path (4x), qualified_name (3x), doc (3x), signature (1x). Includes concept thesaurus expansion (~80 domain clusters) for keyword broadening. |
| 3. Vector/embedding search | 0.0 | BGE-small-en-v1.5 via HNSW (disabled pending code-tuned model) |
| 4. Equivalence class matching | 2.0 | 263 equivalence classes (30 framework files + universal + seed) with forced injection for high-confidence framework matches (gated by `isStrongEquivMatch`: multi-phrase or multi-word phrase required) |
| 5. Path-context seeding | 1.5 | Extracts package/directory terms from task, finds type nodes in matching packages, injects as supplemental RWR seeds at weight 0.3 |

### BM25 Full-Text Search (Channel 2)

Migration 006 creates the `nodes_fts` virtual table. Migration 016 adds a `symbol_name`
column that stores just the terminal symbol identifier (e.g., "QuerySet.filter" instead of
the full qualified path). Migration 017 adds a `concepts` column that stores CamelCase-split
tokens from file names and parent directories (e.g., "commandLineParser.ts" becomes
"command Line Parser commandLineParser"). Migration 018 adds a `doc` column that indexes
node docstrings for BM25 retrieval. The table now indexes six columns with BM25
weights: `symbol_name` (10x), `concepts` (5x), `file_path` (4x), `qualified_name` (3x),
`doc` (3x), `signature` (1x). The high weight on `symbol_name` ensures keyword searches
match by actual symbol name rather than by incidental path token frequency. The `concepts`
column bridges the vocabulary gap where developers search for "parser" but the symbol lives
in a differently-named file. The `doc` column bridges vocabulary between natural-language
task descriptions and code documentation. Tokenization uses CamelCase-aware splitting
(`splitForFTS`, `splitCamelCase`) so that compound identifiers (e.g., "SQLiteStore") are
indexed as individual terms ("SQLite", "Store"). `RebuildFTS` is called after batch indexing
to keep the FTS content synchronized with the nodes table.

### Embedding Search (Channel 3)

Infrastructure is fully shipped: hugot ONNX runtime, coder/hnsw index, and RRF channel
integration. The embedding model is BGE-small-en-v1.5 (384 dimensions, retrieval-tuned).
Currently disabled (weight 0.0) because off-the-shelf models tested net-negative on the
eval (see `eval/EXPERIMENTS.md`). Embed text includes doc comments (Node.Doc field) for
future code-tuned models. Enable with `KNOWING_EMBEDDINGS=1`.

### Equivalence Class Matching (Channel 4)

The equivalence class system (`internal/context/equivalence.go` + 30 `equiv_*.go` files + `universal_seeds.go`) bridges the vocabulary gap between natural-language task descriptions and code symbol names. It contains 263 hand-curated equivalence classes organized by framework (Django, Flask, FastAPI, Terraform, Kubernetes, Kafka, Rails, Spring, ASP.NET, etc.) plus universal and cross-cutting patterns. Language scoping via the `Lang` field prevents cross-language false positives. Additionally, learned vocabulary associations (from agent usage, `vocab_associations` table) are injected as equivalence classes at runtime.

Example: the phrase "blast radius" maps to concept TRANSITIVE_IMPACT, which targets symbols
like `TransitiveCallers`, `handleBlastRadius`, and `BlastRadius`.

Framework equiv classes with forced injection: P@10 0.176 -> 0.278 (+57%, session 23).

## Noise Filtering

Two filtering stages remove low-signal candidates:

**Stage 1: `filterNoisySymbols` (before RWR)**

Applied to the fused seed candidates before the random walk begins:
- Phantom external nodes: `kind == "external"` or qualified name starting with `"external://"`. These are unresolved targets from LSP enrichment with no source code.
- Build artifacts: paths containing `/dist/`, `/build/`, `/vendor/`, `/node_modules/`, `.min.`, or `.bundle.` segments.
- Test fixtures: paths matching `conftest.py.`, `fixtures.py.`, `/testutil`, `/testhelper`, or `test_helper`.
- Mock type names: symbols whose parent type name contains "mock", "fake", or "stub" (e.g., `mockStore.PutEdge`).
- Minified names: terminal symbol names of 2 characters or fewer (excluding common short names like `ID`, `OK`, `DB`).

**Stage 2: RWR result loop (before scoring)**

After the random walk produces relevance scores, the result loop independently filters external nodes again (kind "external" or "external://" prefix). This catches external nodes reached transitively by the walk that were not in the original seed set.

Together these stages prevent test infrastructure, build artifacts, and phantom references from consuming token budget.

## Token Budget Packing

The `packIntoBudget` function implements a density-ranked greedy knapsack:

1. For each ranked symbol, compute density = (score / token_cost) * proximityFactor.
   The proximity factor is `rwrScore^exponent` where the exponent adapts to the phantom
   ratio: `clamp(0.3 + 0.2 * phantomRatio, 0.3, 0.7)`. LSP-enriched edges receive 0.3x
   weight in the RWR walk (attenuating enrichment centrality inflation).
2. Sort by density descending (ties broken by raw score).
3. Greedily pack in density order: if adding a symbol would exceed the budget, skip it and try smaller ones.
4. Re-sort the packed symbols by score descending for output ordering.

This outperforms pure score-order packing on constrained budgets because small, high-value symbols (types, constants) are preferred over large, medium-value symbols (long functions).

The default budget is 50,000 tokens.

### Token estimation heuristic

`EstimateTokens` uses the approximation that code averages 4 characters per token:

```go
func EstimateTokens(text string) int {
    return len(text) / 4
}
```

`EstimateNodeTokens` estimates the cost of a single symbol as the token count of its
qualified name + kind + signature concatenated. This is a rough lower bound; actual context
output includes XML/Markdown structure overhead. The heuristic is intentionally conservative
to avoid under-filling the budget.

## Output Formats

Six output formats are available, split across two rendering paths:

- **Legacy path** (`FormatContextBlock` in `internal/context/format.go`): `xml` (default), `markdown`, `json`. Renders symbols only; does not include edges.
- **Wire codec path** (`formatBlock` in `internal/mcp/context_handlers.go`): `gcf`, `gcb`, `toon`. Uses `wire.FromContextBlock` to discover edges between included symbols, then encodes via the codec registry. GCF is the recommended format for agent workflows (100% LLM comprehension accuracy at 84% token savings vs JSON; see `eval/TestLLMFormatComprehension`).

### XML (legacy default)

Symbols are grouped by distance from the task target:

```xml
<context tokens_used="1234" token_budget="50000">
  <target_symbols>
    <symbol name="pkg.HandleLogin" kind="function" score="0.84" confidence="0.23" provenance="lsp_resolved" distance="0">
      <signature>func HandleLogin(w http.ResponseWriter, r *http.Request)</signature>
    </symbol>
  </target_symbols>
  <related_symbols>
    <symbol name="pkg.AuthService.Validate" kind="method" score="0.62" confidence="0.18" provenance="ast_inferred" distance="1">
      <signature>func (a *AuthService) Validate(token string) (bool, error)</signature>
    </symbol>
  </related_symbols>
  <extended_context>
    <symbol name="pkg.SessionStore.Get" kind="method" score="0.44" distance="2"/>
  </extended_context>
  <relationship_summary>
    <total_symbols>3</total_symbols>
    <by_distance>
      <distance hop="0" count="1"/>
      <distance hop="1" count="1"/>
      <distance hop="2" count="1"/>
    </by_distance>
  </relationship_summary>
</context>
```

The three groups:
- **target_symbols** (distance 0): Direct keyword matches; the symbols the task is about.
- **related_symbols** (distance 1): One hop away; direct callers/callees of targets.
- **extended_context** (distance 2+): Broader structural context; less detail shown.

### Markdown

```markdown
# Context (1234/50000 tokens)

## Target Symbols
- `pkg.HandleLogin` (function, score: 0.84, confidence: 0.23)
  Signature: `func HandleLogin(w http.ResponseWriter, r *http.Request)`

## Related Symbols (distance: 1)
- `pkg.AuthService.Validate` (method, score: 0.62)

## Extended Context (distance: 2+)
- `pkg.SessionStore.Get` (method, score: 0.44)
```

### JSON

Flat array of all symbols with full score component breakdown:

```json
{
  "tokens_used": 1234,
  "token_budget": 50000,
  "symbols": [
    {
      "qualified_name": "pkg.HandleLogin",
      "kind": "function",
      "score": 0.84,
      "signature": "func HandleLogin(...)",
      "provenance": "lsp_resolved",
      "distance": 0,
      "components": {
        "blast_radius": 0.40,
        "confidence": 0.225,
        "recency": 0.06,
        "distance": 0.15
      }
    }
  ]
}
```

## How to Interpret Scores

### Score ranges

- **0.70 - 1.00:** Highly relevant. Directly matched by keywords, has many callers, confirmed
  by LSP, recently observed in traces. Almost certainly belongs in the context.
- **0.40 - 0.70:** Moderately relevant. Structurally connected to the task target but not a
  direct match. Typically one-hop neighbors with moderate blast radius.
- **0.20 - 0.40:** Peripheral. Extended context that provides background but may not be
  essential. Two or more hops away, lower confidence, or few callers.
- **< 0.20:** Filtered out (RWR threshold is 0.05, and scoring rarely produces values in
  this range for included symbols).

### When scores are "flat"

If many symbols score similarly (e.g., a cluster at 0.44-0.48), it usually means:

- The RWR walk found a densely connected subgraph where probability distributes evenly.
- All symbols have similar blast radius (normalization makes them equal).
- The keywords matched a large portion of the graph, diluting the seed set.

Flat scores indicate that the graph structure alone cannot differentiate relevance for this
particular query. The distance component (15%) and confidence component (25%) become the
tiebreakers in these cases.

### Why some symbols score unexpectedly high

Hub nodes (functions called by many others) accumulate probability in the random walk
regardless of whether they are directly relevant to the task. A utility function like
`requireHash` that validates request parameters in every handler will score 0.78+ on
queries about any MCP tool because it sits at the intersection of all handler call chains.
This is by design: high blast-radius symbols deserve attention when making changes because
breaking them affects many callers.

## Tuning Guide

### When to increase the token budget

The default budget is 50,000 tokens. Increase it when:

- Working on a highly connected symbol with many transitive callers (blast radius > 20).
- The task spans multiple packages and you need cross-package context.
- You see the output truncating symbols with scores above 0.5 (they are relevant but did
  not fit).

### When keyword extraction fails

Keyword extraction is purely lexical. It fails when:

- The task uses synonyms or descriptions rather than identifiers. "make the auth faster" will
  not match `AuthService` unless the word "auth" appears in the qualified name. Workaround:
  include the actual symbol name in the task description.
- Abbreviations are not in the expansion map. If your codebase uses `txn` for `transaction`
  but it is not in the abbreviations map, matching will miss it.
- The task is too generic. "refactor the handlers" matches nothing because both "refactor"
  and "handlers" are filtered as stop words/action verbs.

### What "flat scores" means diagnostically

Flat scores across the result set indicate one of:

1. **Too many seeds:** The keyword matched 100+ nodes (the cap), distributing RWR probability
   too thinly. Solution: use more specific keywords in the task description.
2. **Dense subgraph:** The matched portion of the graph is a clique or near-clique, so RWR
   cannot differentiate. This is common in small packages where everything calls everything.
3. **Single-file scope:** If all matched symbols are in one file with no cross-file edges,
   RWR adds no signal beyond the keyword match itself.

## Caching

### Three-Layer Cache (Phase 3)

| Layer | Speed | Lifetime | Invalidation |
|---|---|---|---|
| SubgraphCache (in-memory) | 42ns | Process | Package roots change |
| Notes table (`graph_notes`, SQLite) | ~1.2ms | Persistent | Snapshot hash mismatch |
| Cold retrieval | ~160ms | N/A | Always fresh |

Context packs are persisted to the `graph_notes` table keyed by task hash (`types.NewHash([]byte("context_pack\x00" + normalized_task))`). On process restart, the notes table provides cached results validated against the current snapshot hash. If the snapshot has changed since the pack was stored, the cache entry is stale and cold retrieval runs.

### Incremental RWR Cache

RWR results are cached in the notes table keyed by `computeRWRCacheHash(sorted seeds + weights + alpha + per-package Merkle roots)` (`internal/context/walk.go`). On cache hit, the entire BFS adjacency load and iteration pass is skipped. Django cold 3.9s to warm 1.9s (2x speedup).

Cache invalidation is structural: when a package's Merkle root changes (code edited, new commit), only walks involving that package's seeds are recomputed. The cache key includes `collectSeedPackageRoots` output, which resolves each seed to its package's Merkle root via `snapshot.PackageRootForSymbol`. RWR cache is cleared alongside context packs when feedback is recorded (`internal/store/feedback.go`).

Diagnostic: `knowing debug-rwr-cache -db <path> <repo>` runs cold and warm queries, verifies result correctness, and reports hit/miss stats. Enable in benchmarks with `BENCH_RWR_CACHE=1` (off by default for honest measurement). Toggle via `RWRCacheEnabled` flag.

### Deduplication via `pack_root` (P5)

The `pack_root` parameter on `context_for_task` enables agent-side deduplication. Agents pass the PackRoot from the previous response; if the current result has the same PackRoot, the server returns `"unchanged"` instead of resending the full context. Benchmarked at 93-99% byte savings (557-7,661 tokens reduced to 26 tokens on repeated calls).

### Delta Context Packing (P6)

When the agent sends a `pack_root` that doesn't match the current result (the pack changed), the server computes a structural diff between the prior and current packs instead of retransmitting the full context. The diff uses set difference on node hashes (O(n) with hash maps) and produces a `DeltaPack` with removed symbols, added symbols, removed edges, and added edges.

The delta is encoded in GCF delta format:

```
GCF tool=context_for_task delta=true base_root=<prior> new_root=<current> tokens=30 savings=81%
## removed
fn github.com/example/project.OldHandler
## added
@0 fn github.com/example/project.NewHandler 0.85 rwr
## edges_added
github.com/example/project.Router -> github.com/example/project.NewHandler calls
```

Three outcomes for `pack_root`:
- **Same root**: "unchanged" (zero tokens, existing P5 behavior)
- **Different root, prior pack known**: delta encoding (removed + added sections only)
- **Different root, prior pack unknown**: full retransmission (fallback)

A 60% threshold prevents delta from being used when it would be larger than full retransmission (complete pack replacement). The server stores the last returned `ContextBlock` per pack_root in memory for the session lifetime.

**Benchmark results (session 27, `bench/delta-packing/`):**
- Re-query with 10% budget shift: **81.2% token savings**, 96.6% symbol overlap, 58% delta frequency
- Re-query with 20% budget shift: 69.7% savings, 98.1% overlap
- Cross-task (different tasks, same repo): 9% delta frequency (floor case; different tasks produce different symbol sets)

Implementation: `internal/context/delta.go` (DiffPacks), `internal/wire/delta.go` (EncodeDelta), `internal/mcp/context_handlers.go` (wiring).

## Vocabulary Expansion from Usage

The vocabulary expansion system (`internal/store/vocab.go`, `vocab_associations` table,
migration 021) learns keyword -> symbol associations from agent usage. This is the primary
active learning mechanism, replacing the earlier task memory system (confirmed neutral in
session 24 and disabled).

### Recording

When `recordImplicitFeedback` fires (on each new `ForTask` call), it flushes the previous
cycle's attribution results. Symbols that the agent used (detected via `DetectUsed` scanning
tool call content) get recorded as vocab associations: each task keyword is paired with each
used symbol name, filtered by `isVocabWorthy` (removes ~80 common English words like "use",
"not", "find", "whether" that create spurious cross-task associations). The
`vocab_associations` table uses UPSERT with a count increment, so repeated observations
reinforce the association.

### Activation Threshold

Learned associations require `count >= 2` to activate. A single observation is treated as
noise; two or more observations from different queries confirm the association. This prevents
one-off false matches from becoming permanent equivalence classes.

### Injection

Learned vocab associations enter the retrieval pipeline as equivalence classes with
`source: "learned"`. They compete through **RRF** (soft injection), unlike hand-curated
framework classes which use forced injection. This prevents learned associations from
displacing correct results on tasks with good BM25 coverage. Confidence weighting scales
the RRF weight from 0.3 (count=2, new) to 0.8 (count>=10, well-reinforced), rewarding
associations that are confirmed across multiple sessions.

### Cross-Task Bridging

Validated in session 26: vocab learned from task A helps task B via shared keywords.
Django +41.4% in isolation, full corpus 0.0% aggregate (safe). 100% of improvements
are cross-task (never self-reinforcement). 10-round compounding on 308 tasks:
P@10 peak +2.2%, MRR peak +8.1%. Preview filter decisions with
`knowing debug-vocab -task "description"`.

### Per-Cluster Scoping

Feedback and vocab associations are scoped to keyword clusters (migration 020,
`keyword_cluster` column on feedback table). The cluster is derived from sorted primary
keywords of the task. Noise demotion for "checkout" queries doesn't affect "order" queries.
This prevents cross-task interference that caused round 5 regression in session 24.

### Historical Note

The `TaskMemory` system (migration 008, `task_memory` table) recorded top-5 symbols per
call with keyword matching and 7-day decay. Session 24 proved this mechanism redundant with
the pipeline (BM25 and equiv classes already find the same symbols). Task memory creation
and recording are disabled. Infrastructure preserved.

## Limitations

1. **Limited semantic understanding of task descriptions.** The system uses substring matching,
   BM25, and equivalence classes for seed selection. The 263 equivalence classes bridge common
   vocabulary gaps (e.g., "blast radius" to `TransitiveCallers`), but concepts not covered
   by the curated classes still rely on lexical matching. The system cannot understand
   that "optimize database queries" relates to functions that issue SQL, unless those
   functions contain "database" or "query" in their names or are covered by an equivalence
   class.

2. **Embeddings disabled.** Vector similarity search infrastructure exists (BGE-small-en-v1.5,
   HNSW index, RRF channel) but is disabled (weight 0.0) because off-the-shelf models tested
   net-negative on the eval. Awaiting code-tuned or fine-tuned models.

3. **Substring matching only for tiered seeding.** `NodesByName` uses SQL `LIKE %keyword%`
   for candidate selection. This means a keyword "auth" matches `AuthService`,
   `OAuth2Handler`, and `unauthorized_error` equally.

4. **Duplicate nodes from multiple index runs.** If a file is indexed multiple times with
   slightly different qualified name computation (e.g., different module roots), the graph
   can contain duplicate nodes for the same symbol. This inflates candidate counts and
   dilutes RWR scores.

5. **Static call graph only for blast radius.** The `TransitiveCallers` query follows only
   `calls` edges. Runtime edges (`runtime_calls`, `runtime_rpc`) are not traversed, so
   cross-service call chains visible only in traces do not expand the blast radius. They
   influence the recency component of scoring but not the structural component.

6. **Token estimation is approximate.** The 4-characters-per-token heuristic works reasonably
   for code but can over- or under-estimate for heavily symbolic code (operators, brackets)
   versus prose-like identifiers.
