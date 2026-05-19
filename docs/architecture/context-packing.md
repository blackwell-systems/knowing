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
[2. Keyword Extraction]    -- split CamelCase, expand abbreviations, filter stop words
    |
    v
[3. Seed Selection]        -- 4-channel RRF fusion (tiered keywords, BM25, vector, equivalence classes)
    |
    v
[4. Noise Filtering]       -- exclude mock/stub/fake symbols and build artifacts
    |
    v
[5. Random Walk with Restart]  -- propagate relevance through graph structure (BFS depth limit: 4 hops)
    |
    v
[6. Scoring]               -- weighted formula: blast_radius + confidence + recency + distance + session_boost + feedback + task_memory
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

The `extractKeywords` function (`context.go`) converts a free-text task description into
searchable terms. The pipeline:

1. **Tokenize:** Split on whitespace with `strings.Fields`.
2. **Strip punctuation:** Remove leading/trailing `.,;:!?"'` and brackets from each word.
3. **Split identifiers:** Decompose CamelCase (`HandleLogin` becomes `Handle`, `Login`) and
   snake_case (`route_handler` becomes `route`, `handler`) into component words.
4. **Filter stop words:** Remove English stop words (`the`, `a`, `in`, `to`), common
   programming terms (`func`, `type`, `var`, `err`), and action verbs that describe intent
   but do not identify code (`refactor`, `fix`, `update`, `create`, `implement`).
5. **Length filter:** Discard anything shorter than 2 characters.
6. **Expand abbreviations:** Map known short forms to their full versions. `ctx` adds
   `context`, `cfg` adds `config`, `svc` adds `service`, etc. Both the abbreviation and
   expansion are included as search terms.
7. **Preserve compound terms:** If the original word was multi-part (contained `_` or split
   into multiple CamelCase components), keep the full lowercase form as an additional term
   for exact matching.
8. **Sort by length descending:** Longer (more specific) terms are searched first. This
   ensures that `HandleLogin` is matched before `Handle`, reducing noise from overly broad
   short terms.

### Example

Task: "add a new MCP tool for snapshot diffing"

After extraction: `["snapshot", "diffing", "tool", "mcp"]`

Removed: "add" (action verb stop word), "a" (English stop word), "new" (programming stop
word), "for" (English stop word).

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
fraction of probability mass that flows through it:

| Edge Type | Weight | Rationale |
|---|---|---|
| `calls` | 1.0 | Direct call relationships are the strongest structural signal |
| `implements` | 0.8 | Interface implementations create tight coupling |
| `handles_route` | 0.7 | Route bindings connect HTTP surface to handlers |
| `imports` | 0.5 | Package-level dependency (weaker than function-level) |
| `references` | 0.4 | Type/constant usage (weakest static signal) |
| unknown/runtime | 0.3 | Default for any edge type not in the weight map |

When a node has multiple outgoing edges, probability is distributed proportionally to edge
weight. A node with one `calls` edge (weight 1.0) and one `imports` edge (weight 0.5) sends
2/3 of its flow along the calls edge and 1/3 along the imports edge.

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
composite score. Without HITS:

```
score = blast_radius * 0.40 + confidence * 0.25 + recency * 0.20 + distance * 0.15 + feedback + session
```

With HITS active (applied to top-200), weights shift to accommodate authority adjustments:

```
score = blast_radius * 0.35 + confidence * 0.20 + recency * 0.15 + distance * 0.15 + authorityAdj + feedback + session
```

Feedback weight is 0.15 (centered around 0). Session weight is 0.20 (normalized from [0, 2.0] cap).

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

## Seed Retrieval: 4-Channel RRF Fusion

Seed selection uses Reciprocal Rank Fusion (RRF) across four channels, replacing the
previous single-channel tiered matching with BM25 fallback. The function `rrfFuseMulti`
merges ranked lists from all channels into a single seed set:

| Channel | Weight | Source |
|---------|--------|--------|
| 1. Tiered keyword matching | 3.0 | 5-tier exact/prefix/substring/path/interface matching |
| 2. BM25 FTS5 | 1.0 | SQLite FTS5 over qualified_name, signature, file_path (CamelCase-aware tokenization) |
| 3. Vector/embedding search | 0.0 | BGE-small-en-v1.5 via HNSW (disabled pending code-tuned model) |
| 4. Equivalence class matching | 2.0 | 84 equivalence classes (63 universal + 21 knowing-specific) with 1000+ phrases mapped to target symbols |

### BM25 Full-Text Search (Channel 2)

Migration 006 creates an `nodes_fts` virtual table over `qualified_name`, `signature`, and
`file_path`. Tokenization uses CamelCase-aware splitting (`splitForFTS`, `splitCamelCase`)
so that compound identifiers (e.g., "SQLiteStore") are indexed as individual terms
("SQLite", "Store"). `RebuildFTS` is called after batch indexing to keep the FTS content
synchronized with the nodes table.

### Embedding Search (Channel 3)

Infrastructure is fully shipped: hugot ONNX runtime, coder/hnsw index, and RRF channel
integration. The embedding model is BGE-small-en-v1.5 (384 dimensions, retrieval-tuned).
Currently disabled (weight 0.0) because off-the-shelf models tested net-negative on the
eval (see `eval/EXPERIMENTS.md`). Embed text includes doc comments (Node.Doc field) for
future code-tuned models. Enable with `KNOWING_EMBEDDINGS=1`.

### Equivalence Class Matching (Channel 4)

The equivalence class system (`internal/context/equivalence.go` + `universal_seeds.go`) bridges the vocabulary gap between natural-language task descriptions and code symbol names. It contains 84 equivalence classes: 21 knowing-specific (TRANSITIVE_IMPACT, SYMBOL_LOOKUP, DATAFLOW_TRACE, TEST_SELECTION, etc.) and 63 universal software concepts (covering security, monitoring, scheduling, rate limiting, search, websockets, retry/circuit-breaker, health checks, feature flags, and more). Each class has 5-10 phrases mapped to common symbol name targets across Go/TS/Python/Java/Rust. Cross-product expansion with action verbs generates additional phrase variants.

Example: the phrase "blast radius" maps to concept TRANSITIVE_IMPACT, which targets symbols
like `TransitiveCallers`, `handleBlastRadius`, and `BlastRadius`.

This was the biggest single-feature improvement: hard tier 10% to 18% P@10 (+8pp).

## Noise Filtering

Before scoring, `filterNoisySymbols` removes low-signal candidates:
- Symbols with mock, fake, or stub in the qualified name (case-insensitive).
- Symbols whose file path contains `/build/` or `.bundle.` segments.

This prevents test infrastructure and build artifacts from consuming token budget.

## Token Budget Packing

The `packIntoBudget` function implements a density-ranked greedy knapsack:

1. For each ranked symbol, compute density = score / token_cost.
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

The `FormatContextBlock` function renders the packed symbols into one of three formats.

### XML (default)

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

## Limitations

1. **Limited semantic understanding of task descriptions.** The system uses substring matching,
   BM25, and equivalence classes for seed selection. The 84 equivalence classes bridge common
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

6. **No incremental RWR.** The entire reachable subgraph is loaded and walked on every cache miss.
   For very large graphs (thousands of nodes in the BFS frontier), this could become slow.
   In practice, the 100-candidate cap on seed nodes limits the reachable subgraph size.
   The subgraph cache (`internal/cache/subgraph.go`) eliminates this cost for repeat queries:
   identical tasks against unchanged code return cached results instantly.

7. **Token estimation is approximate.** The 4-characters-per-token heuristic works reasonably
   for code but can over- or under-estimate for heavily symbolic code (operators, brackets)
   versus prose-like identifiers.
