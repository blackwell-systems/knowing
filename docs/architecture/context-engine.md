# Context Engine

The context packing subsystem (`internal/context/`) produces token-budgeted, graph-ranked context blocks for agent consumption. It answers: "given a task or a set of changed files, which symbols from the knowledge graph should an agent see?" Two entry points exist: task-based (keyword search from a description) and file-based (blast-radius expansion from changed files).

## Architecture

```
internal/context/
├── context.go          ContextEngine: ForTask, ForFiles entry points, 4-channel RRF fusion, knapsack packing
├── equivalence.go      Equivalence class seed retrieval: 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific) -> target symbols
├── universal_seeds.go  63 universal software concepts (weight 0.8), cross-repo retrieval
├── language_seeds.go   31 language-specific equivalence classes (Python, TS, Rust, Java, K8s)
├── graph_aliases.go    Auto-generated equivalence classes from caller/callee names (weight 0.7)
├── task_memory.go      Passive task memory: records top-5 symbols per call, 7-day decay recall
├── ranking.go          RankSymbols: weighted scoring formula with HITS authority + session boost
├── hits.go             ComputeHITS: hub/authority scores for subgraph reranking
├── session.go          SessionTracker: exponential-decay recency boost for symbols accessed in-session
├── walk.go             Random Walk with Restart (RWR) for graph proximity scoring (4-hop BFS depth limit)
├── tokens.go           EstimateNodeTokens: per-symbol token cost estimation
└── format.go           FormatContextBlock: XML, Markdown, JSON output
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

## Density-Ranked Knapsack Packing

Symbols are not packed by raw score alone. The packer uses a density-ranked greedy fractional knapsack approach: symbols are sorted by their score/cost ratio (where cost is estimated token count), so that high-value, low-cost symbols are included preferentially over expensive symbols (long functions) when the budget is tight. This maximizes the total relevance delivered per token.

## 4-Channel RRF Seed Fusion

Seed selection uses Reciprocal Rank Fusion (`rrfFuseMulti`) across four channels:

| Channel | Weight | Source |
|---------|--------|--------|
| 1. Tiered keyword matching | 2.0 | 5-tier exact/prefix/substring/path/interface matching |
| 2. BM25 FTS5 | 2.0 | SQLite FTS5 over symbol_name, qualified_name, signature, file_path |
| 3. Vector/embedding search | 0.0 | BGE-small-en-v1.5 via HNSW (disabled pending code-tuned model) |
| 4. Equivalence class matching | 2.0 | 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific) mapped to target symbols |

This replaces the previous approach of tiered matching with conditional BM25 fallback. The `rrfFuseMulti` function handles N channels with per-channel weights, producing a single ranked seed set.

## Equivalence Class Seed Retrieval

The equivalence class system (`internal/context/equivalence.go` + `language_seeds.go`) bridges the vocabulary gap between natural-language task descriptions and code symbol names. It contains 115 equivalence classes (63 universal + 21 knowing-specific + 31 language-specific), each mapping concept names and phrases to target symbols. Cross-product expansion with action verbs generates additional phrase variants.

This was the biggest single-feature improvement: hard tier P@10 rose from 10% to 18% (+8pp). It is fused as RRF Channel 4 with weight 2.0.

## Universal Equivalence Classes

The universal seeds system (`internal/context/universal_seeds.go`) provides 63 domain-agnostic software concepts (authentication, caching, config, database, error handling, logging, middleware, routing, serialization, validation, etc.) as equivalence classes. These are weighted at 0.8, between the seed weight of 1.0 and the graph-derived weight of 0.7. Unlike the 21 knowing-specific classes in `equivalence.go` (which map phrases to specific symbols in the knowing codebase), universal seeds apply to any codebase. Cross-repo eval on gortex showed +6.7pp improvement (40% to 46.7%).

## Graph-Derived Aliases

The graph alias system (`internal/context/graph_aliases.go`) auto-generates equivalence classes by analyzing caller/callee symbol names in the graph. It selects the top-10 tiered candidates and assigns weight 0.7. This provides a zero-configuration fallback for repos that lack hand-curated seed mappings, deriving vocabulary from actual code relationships rather than static lists.

## Passive Task Memory

Migration 008 creates the `task_memory` table (columns: keywords, symbol_hash, score, timestamp). The task memory system (`internal/context/task_memory.go`) records the top-5 symbols from each `context_for_task` call. On subsequent calls, it matches keywords against stored entries with a 7-day linear decay. Matched symbols receive a boost added to the `FeedbackBoost` channel at 0.3x scale. This provides passive learning from agent behavior without requiring explicit feedback.

## Merkleized Feedback Validity (v0.5.0)

Migration 014 adds `neighborhood_root BLOB` to the feedback table. When recording feedback, `computeNeighborhoodRoot` in `internal/mcp/feedback.go` computes the SubgraphRoot of the symbol's package and stores it alongside the feedback. When querying feedback via `FeedbackBoosts`, only records where `neighborhood_root` matches the current SubgraphRoot are counted. This provides automatic expiration: feedback becomes invalid when the symbol's package changes (detected via SubgraphRoot mismatch). Backward compatible: NULL `neighborhood_root` uses the legacy path (no expiration). Performance overhead: 11% (255µs → 284µs for 100 symbols).

## BM25 Full-Text Search (FTS5 Index)

Migration 006 creates the SQLite FTS5 virtual table (`nodes_fts`). Migration 016 adds a `symbol_name` column that stores just the terminal symbol identifier (e.g., "QuerySet.filter" instead of the full qualified path). The `extractSymbolName` function strips the repo URL, package path, and file extension prefix to produce this short form.

The FTS5 table now indexes four columns with BM25 weights: `symbol_name` (10x), `qualified_name` (3x), `signature` (1x), `file_path` (1x). The high weight on `symbol_name` ensures that keyword searches like "before_request" rank symbols by their actual name rather than by incidental path token frequency.

The FTS5 tokenizer uses `tokenchars '_'` so that snake_case identifiers (e.g., `before_request`) are indexed as single tokens rather than being split at the underscore.

Tokenization uses CamelCase-aware splitting (`splitForFTS`, `splitCamelCase`) so that a query for "Store" matches "SQLiteStore" or "NewSQLiteStore". `RebuildFTS` runs synchronously after snapshot computation (previously deferred to a background goroutine that was killed on CLI process exit, leaving FTS empty). Adds ~500ms to index time. BM25 is fused as RRF Channel 2 with weight 2.0.

## Embedding Search (Infrastructure Shipped, Disabled)

The embedding model is BGE-small-en-v1.5 (384 dimensions, retrieval-tuned), replacing the initially tested MiniLM-L6-v2. Infrastructure: hugot ONNX runtime, coder/hnsw index, RRF Channel 3 (weight 0.0). Off-the-shelf models tested net-negative on the eval (see `eval/EXPERIMENTS.md`). Embed text includes doc comments (Node.Doc field, extracted via tree-sitter) for future code-tuned models. Enable with `KNOWING_EMBEDDINGS=1`.

## Doc Comment Extraction

Migration 007 adds a `doc` column to the nodes table. The Go tree-sitter extractor extracts doc comments for functions, methods, and types using a language-agnostic `extractDocComment` function that walks tree-sitter `PrevSibling` nodes to collect adjacent comment blocks. Doc comments are stored in the `Node.Doc` field and included in embedding text for improved vector search quality when a code-tuned model becomes available.

## Noise Filtering

Before scoring, `filterNoisySymbols` removes low-signal candidates:
- Symbols with mock, fake, or stub in the qualified name (case-insensitive).
- Symbols whose file path contains `/build/` or `.bundle.` segments.

## ForTask Flow

1. Extract keywords from the task description (stop-word filtered, CamelCase split, deduplicated).
2. Recall task memory: match keywords against stored entries (7-day linear decay), add matched symbols to FeedbackBoost at 0.3x scale.
3. Run 4-channel RRF seed fusion:
   - Channel 1 (weight 2.0): 5-tier keyword matching (exact > prefix > substring > path > interface)
   - Channel 2 (weight 2.0): BM25 FTS5 search
   - Channel 3 (weight 0.0): Vector/embedding search (disabled)
   - Channel 4 (weight 2.0): Equivalence class matching (115 equivalence classes: 63 universal + 21 knowing-specific + 31 language-specific, plus graph-derived aliases at weight 0.7)
4. `rrfFuseMulti` merges all channels into a single ranked seed set.
5. Filter noisy symbols (mocks, stubs, fakes, build artifacts).
6. For each candidate node, retrieve callers and callees (distance-0 and distance-1 neighborhood).
7. Score all candidates via `RankSymbols` (including session boost).
8. Run HITS on the top-200 candidates to compute authority/hub scores and boost rankings.
9. Pack into token budget via density-ranked knapsack (score/cost ratio ordering).
10. Record top-5 symbols to task memory for future recall.
11. Format output as XML, Markdown, JSON, GCF, or GCB.

## ForFiles Flow

1. Resolve each file path to a `File` record via `FileByPath`.
2. Find all nodes in each file (by `FileHash` match).
3. Expand the blast radius by one hop (all callers of each node).
4. Score, HITS-rerank, and density-pack identically to ForTask.

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

The Random Walk with Restart algorithm (`internal/context/walk.go`) uses edge-type-specific weights to control transition probabilities. Edges with higher weights transfer more probability mass during the walk, making their targets rank higher. The weight map has 20 explicit entries; unknown edge types default to 0.3.

| Edge Type | RWR Weight | Rationale |
|-----------|-----------|-----------|
| `calls` | 1.0 | Direct invocation is the strongest structural signal |
| `implements` | 0.8 | Interface satisfaction indicates tight coupling |
| `implements_rpc` | 0.8 | gRPC service implementation (same as implements) |
| `overrides` | 0.8 | Method override affects all overriding methods |
| `handles_route` | 0.7 | HTTP handler binding is structurally significant |
| `extends` | 0.7 | Class inheritance is structurally significant |
| `tests` | 0.6 | Test coverage is relevant but weaker than calls |
| `consumes_rpc` | 0.6 | gRPC client indicates inter-service dependency |
| `imports` | 0.5 | Package-level proximity |
| `depends_on` | 0.5 | Resource dependency |
| `consumes_endpoint` | 0.5 | HTTP client call to endpoint |
| `tested_by` | 0.5 | CI workflow tests package |
| `references` | 0.4 | Non-call identifier usage |
| `throws` | 0.4 | Error type relationship |
| `deployed_by` | 0.4 | Deployment relationship |
| `gated_by_flag` | 0.3 | Feature flag gating (cross-cutting) |
| `decorates` | 0.3 | Decorator/annotation (cross-cutting) |
| `documents` | 0.2 | Doc comment (informational only) |
| `owned_by` | 0.0 | Organizational; excluded from walk |
| `authored_by` | 0.0 | Organizational; excluded from walk |

Zero-weight edges (`owned_by`, `authored_by`) are genuinely excluded from the random walk. The implementation uses `w, ok := edgeWeight[e.EdgeType]; if !ok { w = 0.3 }` so that explicit 0.0 weights are respected rather than treated as "unknown."

Edge-type filtering: callers can scope the RWR walk by filtering edges before traversal. The `edgeWeight` map serves as both a weighting mechanism and an implicit type registry; edge types not in the map receive the default weight but still participate in the walk.

## Token Estimation

`EstimateNodeTokens` computes a rough token cost per symbol based on the length of the qualified name, signature, and kind. This is an approximation sufficient for budget enforcement without requiring a tokenizer dependency.

`EstimateNodeTokensForFormat` (`internal/context/tokens.go`) extends this with format-aware scaling. GCF encoding costs approximately 16% of the equivalent JSON token count; GCB (binary) costs approximately 26%. The format parameter selects the scaling factor so that knapsack packing uses accurate budgets for the chosen output format.
