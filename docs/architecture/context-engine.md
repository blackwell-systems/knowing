# Context Engine

The context packing subsystem (`internal/context/`) produces token-budgeted, graph-ranked context blocks for agent consumption. It answers: "given a task or a set of changed files, which symbols from the knowledge graph should an agent see?" Three entry points exist: task-based (`ForTask`, keyword search from a description), file-based (`ForFiles`, blast-radius expansion from changed files), and PR-based (`ForPR`, RWR from all symbols in changed files). A fourth entry point, `ExplainSymbol`, runs the full retrieval pipeline and returns a detailed scoring breakdown for a specific symbol.

**Current performance:** P@10 = 0.189 cold start (14 repos, 277 tasks, 8 languages). 2.17x vs codegraph, 3.44x vs GitNexus, 3.63x vs Gortex, 12.6x vs grep. Focused seed selection + cluster-aware gap-fill (re-ranker disabled) + 164 equivalence classes. Query latency 2ms on k8s (with adjacency cache). Parameter sweep proved all RWR/ranking parameters are irrelevant (identical P@10 across 26 configs); P@10 is reachability-determined, not ranking-determined.

## Architecture

```
internal/context/
├── context.go          ContextEngine: ForTask, ForFiles, ForPR entry points, 5-channel RRF fusion, knapsack packing
├── explain.go          ExplainSymbol: full pipeline + detailed scoring breakdown; tieredSearchSet (unified method)
├── equivalence.go      Equivalence class seed retrieval: 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific) -> target symbols
├── universal_seeds.go  63 universal software concepts (weight 0.8), cross-repo retrieval
├── language_seeds.go   31 language-specific equivalence classes (Python, TS, Rust, Java, K8s)
├── graph_aliases.go    Auto-generated equivalence classes from caller/callee names (weight 0.7)
├── task_memory.go      Passive task memory: records top-5 symbols per call, 7-day decay recall
├── ranking.go          RankSymbols: weighted scoring formula with HITS authority + session boost
├── hits.go             ComputeHITS: hub/authority scores for subgraph reranking (top 200 nodes, 10 iterations)
├── session.go          SessionTracker: exponential-decay recency boost for symbols accessed in-session
├── sweep.go            SweepParams: sweepable RWR/ranking parameters (alpha, maxIter, scoreCutoff, maxSeeds, RRFk, ranking weights)
├── walk.go             RandomWalkWithRestart (alpha=0.2, 20 iterations, community-aware, rank-weighted seeds)
├── tokens.go           EstimateNodeTokens: per-symbol token cost estimation with format-aware scaling
└── format.go           FormatContextBlock: XML, Markdown, JSON output (GCF/GCB/TOON via internal/wire/)
```

## Scoring Formula

Each candidate symbol receives a weighted score. Two paths exist depending on whether HITS reranking is active:

**Without HITS (base formula):**

| Component | Weight | Source |
|-----------|--------|--------|
| Blast radius | 0.40 | Relative caller count (`callerCount / maxCallers`) |
| Confidence | 0.25 | Maximum edge confidence on the symbol |
| Recency | 0.20 | Time decay from `last_observed` field |
| Distance | 0.15 | `1 / (1 + hops_from_target)` |
| Feedback | 0.15 | Historical usefulness ratio (centered: >0.5 boosts, <0.5 penalizes) |
| Session boost | 0.20 | Exponential decay from recent session access (normalized from [0, 2.0] cap) |

**With HITS (applied to top-200 candidates):**

| Component | Weight | Source |
|-----------|--------|--------|
| Blast radius | 0.35 | Relative caller count |
| Confidence | 0.20 | Maximum edge confidence |
| Recency | 0.15 | Time decay from `last_observed` field |
| Distance | 0.15 | `1 / (1 + hops_from_target)` |
| Authority adj | variable | +0.25 * authority for seeds, -0.15 * authority for non-seeds |
| Hub bonus | +0.10 * hub | Applied only to seed entry points |
| Feedback | 0.15 | Historical usefulness ratio |
| Session boost | 0.20 | Exponential decay from recent session access |

The session boost is provided by `SessionTracker` (`internal/context/session.go`), which records symbols returned by context queries during the current server lifetime. Symbols accessed recently receive a boost with a 3-minute half-life (tuned for AI session cadence), capped at 2.0x to prevent runaway amplification. The MCP server maintains one tracker per process lifetime.

After initial scoring, the top candidates are reranked using HITS (Hyperlink-Induced Topic Search) authority scores. Nodes with high authority (heavily called functions, core types, key interfaces) are promoted when they are seed matches; non-seed authorities (generic infrastructure like `context.Context`) are penalized. Seed hubs (orchestrators, entry points) receive a smaller bonus for structural context.

## HITS Reranking (Hyperlink-Induced Topic Search)

After RWR produces a scored candidate set, HITS computes Authority and Hub scores on the same subgraph that RWR explored (not the full graph). This reshapes the ranking to promote task-specific authorities over generic infrastructure.

**Authority** scores identify nodes pointed to by many others: heavily called functions, important types, key interfaces. These are the structural centers of the subgraph.

**Hub** scores identify nodes that point to many others: orchestrators, entry points, dispatch functions. These are the connectors that tie a subsystem together.

**Implementation:** `ComputeHITS` in `internal/context/walk.go` runs 10 power iterations on the top-200 candidates from RWR. The algorithm alternately updates authority scores (sum of hub scores of all nodes pointing in) and hub scores (sum of authority scores of all nodes pointed to), normalizing after each iteration.

**Ranking adjustments** (applied in `RankSymbols`, `internal/context/ranking.go`):

| Condition | Adjustment | Rationale |
|-----------|-----------|-----------|
| Seed node with high authority | +0.25 * authority | Both keyword-relevant AND structurally central; strong signal of task relevance |
| Non-seed node with high authority | -0.15 * authority | Generic infrastructure (`types.Hash`, `context.Context`) called by everything but rarely task-relevant |
| Seed node with high hub score | +0.10 * hub | Entry points and orchestrators that connect task-relevant code |

The asymmetric treatment of authority is the key insight: authority alone is not a quality signal. A node with high authority that also matches the task keywords (seed) is genuinely important to the task. A node with high authority that does not match keywords is likely generic infrastructure that appears in every subgraph regardless of the task. By penalizing non-seed authorities, HITS suppresses the "popular but irrelevant" nodes that would otherwise dominate purely structural rankings.

## Density-Ranked Knapsack Packing

**The problem.** After scoring, we have N ranked candidate symbols but a fixed token budget (typically 4000-8000 tokens). A naive approach (take the top-N symbols by score until the budget is exhausted) wastes budget on large, low-value symbols. A 500-line orchestrator function scoring 0.9 consumes half the budget, displacing dozens of smaller high-value helpers. The knapsack approach maximizes total relevance delivered per token rather than per symbol.

**Density metric.** Each candidate's packing priority is determined by its value density: `score / estimated_tokens`. A 100-token function with score 0.8 has density 0.008; a 500-token function with score 0.9 has density 0.0018. Despite the higher raw score, the smaller function packs over 4x more value per token. When budget is constrained, including five small high-density symbols delivers more total relevance than one large symbol with a marginally higher score.

**Algorithm.** Candidates are sorted by density (score/tokens) in descending order, then greedily packed until the budget is exhausted. This is the fractional knapsack greedy approximation: sorting by value-density and taking greedily is provably optimal for the fractional variant, and near-optimal for the 0/1 variant when item sizes are small relative to the budget (which holds here, since most symbols are 50-200 tokens against a 4000+ token budget).

**Token estimation.** Symbol token cost is estimated at source line count multiplied by approximately 4 tokens per line (a heuristic calibrated against GPT-family tokenizers). This avoids requiring a tokenizer dependency at query time. The actual context output formats (XML, JSON, GCF, TOON) add per-symbol overhead from tags, delimiters, and metadata fields; format-aware scaling factors (see Token Estimation section below) adjust the budget accounting so packing decisions reflect the true rendered cost.

**Edge inclusion.** After selecting symbols via the knapsack, edges between selected symbols are included in the context pack. This gives the consuming agent structural context (who calls whom, who implements what) without spending budget on unreachable symbols. Only edges where both endpoints are in the selected set are included; edges pointing to symbols outside the pack are omitted.

**PackRoot.** The final pack is content-addressed: `PackRoot = hash(task_normalized + sorted(selected_node_hashes))`. Same query against the same graph state always produces the same PackRoot. This enables deduplication (skip re-retrieval for repeated queries), citation (reference a specific pack by hash), and cache keying (invalidate only when the underlying subgraph changes). See the Content-Addressed Context Packs section below for full details.

**Implementation.** `packIntoBudget` in `internal/context/context.go` accepts the ranked symbol list, token budget, and output format; returns a `ContextBlock` with the selected symbols, included edges, token usage, and PackRoot.

## 5-Channel RRF Seed Fusion

Seed selection uses Reciprocal Rank Fusion (`rrfFuseMulti`) across five channels:

| Channel | Weight | Source |
|---------|--------|--------|
| 1. Tiered keyword matching | 2.0 | 4-tier compound-first: exact > prefix > substring > path (interface seeding is a separate post-RRF step) |
| 2. BM25 FTS5 | 2.0 | SQLite FTS5 over 6 columns: symbol_name (10x), concepts (5x), file_path (4x), qualified_name (3x), doc (3x), signature (1x) |
| 3. Vector/embedding search | 0.0 | nomic-code via HNSW (neutral as seed channel; powers gap-fill seeds when BM25 < 5 candidates) |
| 4. Equivalence class matching | 2.0 | 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific) mapped to target symbols |
| 5. Path-context seeding | 1.5 | Extracts package/directory terms from task, finds type nodes in matching packages, injects as supplemental RWR seeds at weight 0.3 |

The `rrfFuseMulti` function handles N channels with per-channel weights, producing a single ranked seed set. When the fused set contains fewer than 5 candidates (threshold configurable via `BENCH_GAP_THRESHOLD`), gap-fill seeds supplement the channels by querying the embedding vector store for semantically similar symbols. This targets vocabulary gaps where ground truth shares no keywords with the task description. Impact: Django +43%, flask +22%, zero regressions. After RRF fusion (and gap-fill if triggered), interface-aware seeding adds all implementors of any interface/type in the candidate set as additional seeds. The path channel receives lower weight (1.5x) because it is valuable for bridging concepts to packages but less precise than name-matching channels.

## Equivalence Class Seed Retrieval

The equivalence class system (`internal/context/equivalence.go` + `language_seeds.go`) bridges the vocabulary gap between natural-language task descriptions and code symbol names. It contains 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific), each mapping concept names and phrases to target symbols. Cross-product expansion with action verbs generates additional phrase variants.

This was the biggest single-feature improvement: hard tier P@10 rose from 10% to 18% (+8pp). It is fused as RRF Channel 4 with weight 2.0.

## Universal Equivalence Classes

The universal seeds system (`internal/context/universal_seeds.go`) provides 63 domain-agnostic software concepts (authentication, caching, config, database, error handling, logging, middleware, routing, serialization, validation, etc.) as equivalence classes. These are weighted at 0.8, between the seed weight of 1.0 and the graph-derived weight of 0.7. Unlike the 21 knowing-specific classes in `equivalence.go` (which map phrases to specific symbols in the knowing codebase), universal seeds apply to any codebase. Cross-repo eval on gortex showed +6.7pp improvement (40% to 46.7%).

## Graph-Derived Aliases

The graph alias system (`internal/context/graph_aliases.go`) auto-generates equivalence classes by analyzing caller/callee symbol names in the graph. It selects the top-10 tiered candidates and assigns weight 0.7. This provides a zero-configuration fallback for repos that lack hand-curated seed mappings, deriving vocabulary from actual code relationships rather than static lists.

## Passive Task Memory

Migration 008 creates the `task_memory` table (columns: keywords, symbol_hash, score, timestamp). The task memory system (`internal/context/task_memory.go`) records the top-5 symbols from each `context_for_task` call with a raw score of 1.0. On subsequent calls, it matches keywords against stored entries with a 7-day linear decay. At recall time, matched symbols are boosted via the formula `0.5 + recall_score * 0.4` (range [0.5, 0.9], always positive), applied as a max against existing `FeedbackBoost` so memory never overrides stronger explicit feedback signals. Task memory persists across process restarts (stored in SQLite), so quality compounds with usage over time. This provides passive learning from agent behavior without requiring explicit feedback.

## Merkleized Feedback Validity (v0.5.0)

Migration 014 adds `neighborhood_root BLOB` to the feedback table. When recording feedback, `computeNeighborhoodRoot` in `internal/mcp/feedback.go` computes the SubgraphRoot of the symbol's package and stores it alongside the feedback. When querying feedback via `FeedbackBoosts`, only records where `neighborhood_root` matches the current SubgraphRoot are counted. This provides automatic expiration: feedback becomes invalid when the symbol's package changes (detected via SubgraphRoot mismatch). Backward compatible: NULL `neighborhood_root` uses the legacy path (no expiration). Performance overhead: 11% (255µs → 284µs for 100 symbols).

## BM25 Full-Text Search (FTS5 Index)

Migration 006 creates the SQLite FTS5 virtual table (`nodes_fts`). Migration 016 adds a `symbol_name` column that stores just the terminal symbol identifier (e.g., "QuerySet.filter" instead of the full qualified path). Migration 017 adds a `concepts` column that stores CamelCase-split tokens from file names and parent directories (e.g., "commandLineParser.ts" becomes "command Line Parser commandLineParser"). Migration 018 adds a `doc` column that indexes node docstrings for BM25 retrieval. The `extractSymbolName` function strips the repo URL, package path, and file extension prefix to produce the short form.

The FTS5 table indexes six columns with BM25 weights: `symbol_name` (10x), `concepts` (5x), `file_path` (4x), `qualified_name` (3x), `doc` (3x), `signature` (1x). The high weight on `symbol_name` ensures that keyword searches like "before_request" rank symbols by their actual name rather than by incidental path token frequency. The `concepts` column bridges the vocabulary gap where developers say "parser" but the symbol lives in `commandLineParser.ts`. The `doc` column bridges another vocabulary gap: task descriptions use natural language ("validate the request body") and docstrings also use natural language descriptions of what code does.

BM25 queries also inject file_path-targeted prefix terms extracted from the task description (e.g., `file_path:migration*`), matching symbols in packages whose names overlap with task keywords. This uses prefix matching to handle singular/plural directory names.

The FTS5 tokenizer uses `tokenchars '_'` so that snake_case identifiers (e.g., `before_request`) are indexed as single tokens rather than being split at the underscore.

Tokenization uses CamelCase-aware splitting (`splitForFTS`, `splitCamelCase`) so that a query for "Store" matches "SQLiteStore" or "NewSQLiteStore". `RebuildFTS` runs synchronously after snapshot computation (previously deferred to a background goroutine that was killed on CLI process exit, leaving FTS empty). Adds ~500ms to index time. BM25 is fused as RRF Channel 2 with weight 2.0.

## Embedding Re-ranker (Shipped, +17% P@10)

The embedding model is nomic-embed-text-v1.5 (768 dimensions, code-tuned) via hugot
pure-Go ONNX runtime. As an independent seed channel (Channel 3), embeddings are neutral
(three models tested identical to BM25). As **gap-fill seeds** (step 2b in
ForTask), they produce the biggest single improvement in project history: P@10 0.207 -> 0.247
(+17%), R@10 0.306 -> 0.362 (+18.3%).

**How gap-fill works:** When BM25 returns < 5 candidates, embedding vectors find semantically similar symbols via brute-force cosine
similarity between the task description and each candidate's embedding. `ReRankByHashes`
looks up pre-computed vectors from SQLite (populated at index time), only embedding the
query text (~120ms). Cache misses fall back to on-the-fly embedding and auto-persist.
Gap-fill latency: ~220ms cached vectors, ~120ms query embedding.

**Note:** Re-ranker disabled (session 19, net negative on P@10). Gap-fill seeds remain active.
Enable with `--embeddings` on `knowing mcp` or `BENCH_EMBEDDINGS=1` for benchmarks.
See `docs/architecture/embedding-reranker.md` for the full design.

## Docstring Extraction and FTS Indexing

Migration 007 adds a `doc` column to the nodes table. Migration 018 adds a `doc` column to `nodes_fts_content` and rebuilds the FTS virtual table to include it at BM25 weight 3.0. Docstrings are extracted for 6 languages via a shared `docextract` package (`internal/indexer/docextract/`):

- **Go**: `extractDocComment` walks `PrevSibling` comment nodes (`//` and `/* */`)
- **Python**: `extractPythonDocstring` extracts body-first string literals (triple-quoted docstrings)
- **TypeScript**: `FromPrecedingComments` handles JSDoc (`/** */`)
- **Rust**: `FromPrecedingComments` handles `///`, `//!`, and `/* */`
- **Java**: `FromPrecedingComments` handles Javadoc (`/** */`)
- **C#**: `FromPrecedingComments` handles XML doc comments (`///`)

All extractors cap at 500 characters. Doc comments are stored in the `Node.Doc` field and indexed in both the FTS5 `doc` column (for BM25 retrieval) and embedding text (for future code-tuned vector models). The docstring FTS column was the first change to move P@10 since the equivalence channel fix, improving full-corpus P@10 from 0.180 to 0.202 (+12.2%).

## Noise Filtering

Before scoring, `filterNoisySymbols` removes low-signal candidates:
- Phantom external nodes (kind "external" or qualified_name starting with "external://") created during LSP enrichment with no source code.
- Symbols whose file path contains `/dist/`, `/build/`, `/vendor/`, `/node_modules/`, `.min.`, or `.bundle.` segments.
- Test fixtures and helpers: `conftest.py`, `fixtures.py`, `/testutil`, `/testhelper`, `test_helper`.
- Symbols whose type name (not terminal symbol name) contains mock, fake, or stub (case-insensitive), matching patterns like `mockStore.PutEdge`.
- Symbols with very short terminal names (<=2 chars) that look minified, excluding known legitimate short names (`ID`, `OK`, `Go`, `Do`, `DB`, `IP`, `IO`).

## Test File Deprioritization

After scoring, symbols from test files receive a 0.3x penalty via `isTestFilePath` in `internal/context/context.go`. Detection is path-based: `/tests/`, `_test.go`, `.test.ts`, `.spec.ts`, `/__tests__/`, and similar conventions. The penalty is conditional: it is removed when the task description mentions testing (e.g., "add a test for", "fix the failing test"). This avoids penalizing test symbols when the user is actively working on tests. Failure analysis showed 36% of top-10 misses were test symbols.

## KeywordSet and extractKeywordSet

`extractKeywordSet` processes a task description into a structured `KeywordSet` with three tiers:

| Field | Source | Purpose |
|-------|--------|---------|
| `Exact` | Backtick-quoted identifiers (e.g., `` `before_request` ``) | Explicit symbol references; highest specificity |
| `Compounds` | Multi-part identifiers detected by structure (snake_case, CamelCase, dotted), verb-pattern targets, and bigram joins from adjacent words | Preserve compound semantics; prevent component drowning |
| `Components` | Individual words from identifier splitting, abbreviation expansions, priority terms | Fallback when compounds yield insufficient results |

The `Primary()` method returns `Exact + Compounds`; `All()` returns all three tiers in priority order. `tieredSearchSet` queries primary keywords first through exact/prefix tiers, only falling back to components when fewer than 5 results are found from compounds.

Bigram generation joins adjacent non-stop-words into both CamelCase and snake_case forms (e.g., "context engine" produces `ContextEngine` and `context_engine`), catching compound identifiers that the user wrote as separate words.

## ForTask Flow

1. Extract structured keywords via `extractKeywordSet` (backtick detection, stop-word filtering, CamelCase split, compound preservation, bigram generation).
2. Check caches: first the in-memory SubgraphCache, then the persistent `notes` table (migration 012) keyed by task hash; return immediately if snapshot hash matches (staleness detection).
3. Run 5-channel RRF seed fusion:
   - Channel 1 (weight 2.0): 4-tier compound-first keyword matching (exact > prefix > substring > path)
   - Channel 2 (weight 2.0): BM25 FTS5 search (compound identifiers targeted at symbol_name column; file_path-targeted prefix terms appended from path extraction)
   - Channel 3 (weight 0.0): Vector/embedding search (disabled)
   - Channel 4 (weight 2.0): Equivalence class matching (115 equivalence classes: 63 universal + 21 knowing-specific + 31 language-specific, plus graph-derived aliases at weight 0.7)
   - Channel 5 (weight 1.5): Path-context seeding (finds type/class nodes in packages matching task terms; prioritizes rich types with contains edges)
4. `rrfFuseMulti` merges all channels into a single ranked seed set (k=60, limit=40).
4b. Gap-fill seeds: if the fused candidate set has fewer than 5 results (configurable via `BENCH_GAP_THRESHOLD`), query the embedding vector store for semantically similar symbols as supplemental seeds. Two strategies: HNSW via `EmbedAndSearch` (fast, requires `--embeddings`) or brute-force via `LoadAndSearchFromStore` (works with pre-embedded vectors). Gap-fill only fires when primary channels are weak, so it cannot regress repos where lexical matching already works.
5. Interface-aware seeding: if any candidate is an interface/type, add all implementors as seeds.
6. Filter noisy symbols (externals, mocks, fixtures, build artifacts, minified names).
7. Select top-15 RRF candidates as RWR seeds (maxSeeds=15, rank-weighted restart probability). Inject up to 10 supplemental path seeds at weight 0.3 (from Channel 5 results not already in the seed set).
8. Community-aware RWR: if all seeds cluster in exactly 1 community, constrain the walk to that community. Otherwise, run unconstrained Random Walk with Restart (alpha=0.2, 20 iterations, rank-weighted seed probabilities).
9. Build scoring inputs from all nodes with RWR score >= 0.02 (phantom external and stdlib nodes excluded).
10. Apply feedback boosts (from FeedbackProvider with merkleized validity).
11. Apply session boosts (from SessionTracker, 3-minute half-life, cap 2.0).
12. Apply task memory boosts: recall matching keywords (7-day linear decay), boost formula `0.5 + recall_score * 0.4` (range [0.5, 0.9]), applied as max against existing FeedbackBoost.
13. If task is about testing (detected by keyword), disable test file penalty.
14. Run HITS on the top-200 candidates (10 iterations) to compute authority/hub scores.
15. Score all candidates via `RankSymbols` (blast_radius, confidence, recency, distance, feedback, session, HITS adjustments).
15b. ~~Embedding re-rank:~~ **Disabled (session 19).** Per-repo A/B test showed net negative on P@10 (9/13 repos hurt). Code preserved but not called. Gap-fill seeds (step 2b) provide the embedding value.
16. Pack into token budget via density-ranked knapsack (score/cost ratio ordering).
17. Record returned symbols in session tracker; record top-5 symbols to task memory for future recall.
18. Compute content-addressed PackRoot; persist to both in-memory SubgraphCache and persistent notes table (with snapshot hash for staleness detection).
19. Format output as GCF, GCB, TOON, JSON, XML, or Markdown.

## ForFiles Flow

1. Resolve each file path to nodes via `NodesByFilePath` (using repo hash + relative path).
2. For each node, retrieve all `calls` edges pointing to it (callers).
3. Add callers as distance-1 candidates (one-hop blast radius expansion).
4. Run HITS on top-200 candidates (10 iterations) for authority/hub differentiation.
5. Score via `RankSymbols` and density-pack into token budget.

## ForPR Flow

1. Resolve all changed file paths to symbols via `NodesByFilePath` (these are the PR's direct changes, distance-0 seeds).
2. Run Random Walk with Restart (alpha=0.2, 20 iterations) from all changed symbols to find the impact neighborhood.
3. Build scoring inputs from all nodes with RWR score >= 0.05 (higher threshold than ForTask since PR context is broader).
4. Run HITS on top-200 for authority/hub scoring.
5. Score via `RankSymbols` and density-pack into token budget (default 8000 tokens).

## Content-Addressed Context Packs (Phase 2, Shipped)

`ContextBlock` carries a `PackRoot` field computed by `computePackRoot()` in `internal/context/context.go`. The hash is derived from the normalized task description and the sorted hashes of all selected nodes:

```
PackRoot = hash(task_normalized + sorted(selected_node_hashes))
```

This produces a deterministic, content-addressed identity for every context pack. Same task against the same graph always yields the same PackRoot. Benchmark verification: 5 queries with 2 unique tasks produced exactly 2 unique PackRoots (perfect dedup). See `bench/merkle-diff/FINDINGS-context-packs.md`.

What PackRoot enables:

- **Cache lookup:** a cached pack is valid as long as its PackRoot has been seen before and the underlying subgraph has not changed. Skip re-running retrieval entirely for repeated tasks.
- **Citation by hash:** an agent can cite a PackRoot in a code review comment; the reviewer can replay the exact pack and inspect every scoring decision.
- **Cross-session replay:** a resumed session loads the prior pack by PackRoot and picks up exactly where it left off.
- **Pack diffing:** compare PackRoot values from two snapshots to see which symbols entered or left the retrieval set as the codebase evolved.
- **Feedback anchoring:** feedback records can be tied to a PackRoot instead of just a symbol hash, scoping validity to the exact graph state at feedback time.

The hierarchical Merkle tree (`internal/snapshot/hierarchical.go`) provides `SubgraphRoot`, which computes an O(1) cache key for any set of packages. Full subgraph caching (keying `context_for_task` against `SubgraphRoot` so that changes in unrelated packages do not invalidate the cache) is the remaining Phase 2 deliverable.

## Store-Layer Caching

`ContextEngine` reads from `GraphStore` for every node lookup, caller lookup, and edge traversal during the RWR walk. `SQLiteStore` maintains an in-process LRU cache (`sync.Map`, 50K-entry cap) on `GetNode` and `GetEdge`. For hot-path traversals (multi-hop RWR, HITS subgraph construction), this eliminates redundant SQL round-trips on nodes visited repeatedly across the walk. The cache is invalidated at the start of each index run, so context queries after an index always see fresh graph state.

## Integration Points

- **MCP tools**: `context_for_task`, `context_for_files`, `context_for_pr`, and `explain_symbol` in `internal/mcp/context_handlers.go` delegate to `ContextEngine`.
- **CLI**: `knowing context` subcommand (in `cmd/knowing/context.go`) provides the same functionality from the command line with `--task` or `--files` flags.
- **CLI**: `knowing why` subcommand (in `cmd/knowing/why.go`) runs the full retrieval pipeline and returns a detailed scoring breakdown for a specific symbol via `ExplainSymbol` (`internal/context/explain.go`).
- **Test scope**: `knowing test-scope` (in `cmd/knowing/testscope.go`) uses `NodesByFilePath` to resolve symbols in changed files and BFS backward through `calls` edges to find affected tests.

## RWR Edge Weights

The Random Walk with Restart algorithm (`internal/context/walk.go`) uses edge-type-specific weights to control transition probabilities. Edges with higher weights transfer more probability mass during the walk, making their targets rank higher. The weight map has 21 explicit entries; unknown edge types default to 0.3. Canonical weights are also defined in `internal/edgetype/constants.go` via the `RWRWeight()` function.

| Edge Type | RWR Weight | Rationale |
|-----------|-----------|-----------|
| `calls` | 1.0 | Direct invocation is the strongest structural signal |
| `implements` | 0.8 | Interface satisfaction indicates tight coupling |
| `implements_rpc` | 0.8 | gRPC service implementation (same as implements) |
| `overrides` | 0.8 | Method override affects all overriding methods |
| `contains` | 0.8 | Type -> method/field; enables RWR walks from type seeds to discover methods |
| `handles_route` | 0.7 | HTTP handler binding is structurally significant |
| `extends` | 0.7 | Class inheritance is structurally significant |
| `tests` | 0.6 | Test coverage is relevant but weaker than calls |
| `consumes_rpc` | 0.6 | gRPC client indicates inter-service dependency |
| `member_of` | 0.6 | Method/field -> type; enables RWR walks from methods to parent type then siblings |
| `imports` | 0.5 | Package-level proximity |
| `depends_on` | 0.5 | Resource dependency |
| `consumes_endpoint` | 0.5 | HTTP client call to endpoint |
| `tested_by` | 0.5 | CI workflow tests package |
| `references` | 0.4 | Non-call identifier usage |
| `throws` | 0.4 | Error type relationship |
| `deployed_by` | 0.4 | Deployment relationship |
| `gated_by_flag` | 0.3 | Feature flag gating (cross-cutting) |
| `decorates` | 0.3 | Decorator/annotation (cross-cutting) |
| `inherits` | 0.3 (default) | Child-to-parent-method via inheritance propagation; not explicitly in the weight map |
| `documents` | 0.2 | Doc comment (informational only) |
| `co_tested_with` | 0.5 | Lateral connection between symbols co-tested in the same test file |
| `type_hint_of` | 0.5 | Function parameter type annotation linking function to type |
| `similar_to` | 0.15 | Semantic similarity (Jaccard on tokenized bodies); weak signal to avoid overweighting |
| `accesses_field` | 0.6 | Method -> struct/class field it reads/writes (6 languages) |
| `reads_env` | 0.4 | Function -> environment variable it reads (supply chain detection) |
| `executes_process` | 0.5 | Function -> process it spawns (supply chain detection) |
| `owned_by` | 0.0 | Organizational; excluded from walk |
| `authored_by` | 0.0 | Organizational; excluded from walk |

Zero-weight edges (`owned_by`, `authored_by`) are genuinely excluded from the random walk. The implementation uses `w, ok := edgeWeight[e.EdgeType]; if !ok { w = 0.3 }` so that explicit 0.0 weights are respected rather than treated as "unknown."

Edge-type filtering: callers can scope the RWR walk by filtering edges before traversal. The `edgeWeight` map serves as both a weighting mechanism and an implicit type registry; edge types not in the map receive the default weight but still participate in the walk.

## Sweepable Parameters and Parameter Sweep

All RWR and ranking parameters are configurable via `SweepParams` (`internal/context/sweep.go`) for benchmarking:

| Parameter | Default | Purpose |
|-----------|---------|---------|
| Alpha | 0.2 | RWR restart probability |
| MaxIter | 20 | RWR iterations |
| ScoreCutoff | 0.02 | Minimum RWR score threshold for candidate inclusion |
| MaxSeeds | 15 | Maximum RWR seeds from RRF-ranked candidates |
| RRFk | 60 | RRF fusion constant |
| BlastW | 0.35 | Blast radius ranking weight |
| ConfW | 0.20 | Confidence ranking weight |
| RecencyW | 0.15 | Recency ranking weight |
| DistanceW | 0.15 | Distance ranking weight |
| TestPenalty | 0.3 | Test file score multiplier |

**Parameter sweep result:** A 26-configuration sweep across all parameters produced identical P@10=0.180 (pre-docstring FTS, now 0.247 with subsequent improvements including docstring FTS, density-adaptive seeding, and gap-fill seeds). This proves that P@10 is reachability-determined, not ranking-determined. The retrieval bottleneck is whether relevant symbols are reachable from seeds at all, not how they are ranked once found. New retrieval signals (additional seed channels, new edge types, docstring indexing, concept thesaurus, self-adapting type-seed preference) are the path forward; tuning existing parameters is futile.

## Contains and Member_of Edges

Migration adds structural `contains` (type -> method/field, weight 0.8) and `member_of` (method/field -> type, weight 0.6) edges derived from qualified name hierarchy. The `generateContainsEdges` function in `internal/indexer/indexer.go` identifies type/class/struct/interface nodes and connects them to child nodes whose qualified name is `ParentQN.ChildName`.

This connects 77%+ of previously-disconnected type nodes to their methods, enabling two critical RWR walk patterns:
- Type seed -> contains -> method discovery (e.g., "Operation" type finds `.state_forwards()`, `.database_forwards()`)
- Method match -> member_of -> parent type -> contains -> sibling methods

The path-context seeding channel (Channel 5) leverages these edges directly: it finds type nodes in matching packages and relies on contains edges for RWR to walk from those types to their methods.

## Token Estimation

`EstimateNodeTokens` computes a rough token cost per symbol based on the length of the qualified name, signature, and kind. This is an approximation sufficient for budget enforcement without requiring a tokenizer dependency.

`EstimateNodeTokensForFormat` (`internal/context/tokens.go`) extends this with format-aware scaling:

| Format | Token Cost (% of JSON) | Notes |
|--------|----------------------|-------|
| GCF | 16% | Local IDs and positional encoding; ~84% savings |
| GCB | 26% | Binary wire format; LLM never sees it directly |
| TOON | 40% | Token-Oriented Object Notation; tabular arrays (header + rows) |
| JSON/XML/Markdown | 100% | Full text representation |

The format parameter selects the scaling factor so that knapsack packing uses accurate budgets for the chosen output format.

## Wire Format Routing

Format rendering is split between two layers:
- `internal/context/format.go`: handles XML, Markdown, JSON via `FormatContextBlock`.
- `internal/wire/`: handles GCF, GCB, TOON, JSON (structured) via the codec registry. Routed through `formatBlock` in `internal/mcp/context_handlers.go`.

GCF output uses `wire.EncodeWithSession` for cross-call deduplication (the MCP server's session state tracks previously sent symbols). GCB and JSON (structured) use `wire.EncodeWith`. TOON uses the `toon-format/toon-go` library for spec-conformant encoding (`internal/wire/toon.go`).
