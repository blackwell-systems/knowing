# Architecture

This document is the navigation hub for knowing's architecture documentation. The full architecture specification has been split into focused subdocuments, each covering one area of the system. Every word, diagram, table, and code block from the original monolithic document lives in exactly one subdoc below.

## Reading Order

If you are new to knowing, read these in order:

1. **Concepts** (foundational vocabulary)
2. **System Overview** (component map, extraction pipeline, edge types)
3. **Data Flow** (end-to-end trace of a single commit)
4. **Concurrency Model** (goroutine architecture, locking, shutdown)
5. **Runtime Trace Ingestion** (production observability edges)
6. **Context Engine** (retrieval pipeline, scoring, packing)
7. **Wire Formats** (GCF, binary, JSON codec system)
8. **CLI Commands** (export, watch, why, enrich)
9. **Design Principles** (goals, architectural planes, MCP tool split)
10. **Deep Dives** (15 foundational architecture decisions)

If you are looking for a specific topic, use the index below.

---

## Subdocuments

### [Core Concepts](concepts.md)

Defines every term used across the architecture: content-addressed storage, Merkle DAG, knowledge graph vs. tree vs. table, domain primitives (Node, Edge, Hash, Snapshot, Provenance), event sourcing, staleness (structural and heuristic), why content addressing eliminates re-indexing, and the artifact boundary. Read this first; every other document assumes familiarity with these definitions.

### [System Overview](system-overview.md)

The component map and system diagram showing how local repos, external dependencies, and agents connect through the graph store. Covers the language-agnostic graph model, two-tier extraction (tree-sitter + LSP enrichment), HTTP route extraction across 18 frameworks in 6 languages, the full extractor table (25 registered extractors), multi-language auto-detection, call-site positions, indexing tiers (deep vs. shallow), change detection and incremental indexing (git-based, not filesystem-based), edge events, and the complete edge type taxonomy.

### [Concurrency Model](concurrency.md)

The daemon's goroutine architecture: watchLoop, indexWorker, traceIngestLoop, and MCP server goroutines. Covers read/write coordination via `sync.RWMutex`, channel-based communication (buffer sizes, non-blocking sends, drop policy), clean shutdown via `sync.WaitGroup`, the fan-out/fan-in worker pool for tree-sitter extraction, why LSP enrichment is sequential, and SQLite WAL mode for concurrent access.

### [Data Flow](data-flow.md)

Traces a single developer commit through the entire pipeline: GitWatcher detection, git diff resolution, indexRequest enqueue, write-lock acquisition, IndexFunc execution (deleted/changed/added files), snapshot computation, write-lock release, and background LSP enrichment. Includes the timing summary table showing lock duration and query-blocking windows.

### [Runtime Trace Ingestion](runtime-traces.md)

The subsystem that creates graph edges from production observability data. Covers the OTLP receiver pipeline, span normalization, batch accumulation, symbol resolution (route_symbols table lookup), edge creation and deduplication, edge type classification from span attributes, confidence scoring (observation volume + recency), time-based decay brackets, HTTP log ingestion, and runtime/static edge coexistence in the same edges table.

### [CLI Commands](cli-commands.md)

Documentation for the export CLI (JSON and Graphviz DOT formats with Louvain community annotations), the watch CLI (fsnotify-based continuous indexing), the why CLI and ExplainSymbol (scoring breakdown for context retrieval decisions), and offline enrichment passes (git blame for ownership/recency, coverage profile parsing for per-symbol coverage).

### [Context Engine](context-engine.md)

The retrieval pipeline that produces token-budgeted, graph-ranked context blocks for agent consumption. Covers the file layout of `internal/context/`, the scoring formula (with and without HITS reranking), density-ranked knapsack packing, 4-channel RRF seed fusion, equivalence class seed retrieval (84 classes), universal equivalence classes, graph-derived aliases, passive task memory, BM25 full-text search, embedding search infrastructure (shipped but disabled), doc comment extraction, noise filtering, the ForTask and ForFiles flows, integration points (MCP tools, CLI, test scope), and token estimation with format-aware scaling.

### [Wire Format and Codec System](wire-formats.md)

The pluggable codec registry in `internal/wire/`. Covers the four built-in codecs (GCF at ~76.7% token savings, binary at ~74% byte savings, JSON as baseline, TOON for structured interchange), the layered architecture mapping codecs to system layers, GCF session statefulness (47% additional savings via deduplication), the binary wire layout specification, and the benchmark harness with six fixture cases.

### [Design Principles](design-principles.md)

The nine design goals (content-addressed, two-tier, git-driven incremental, language-aware at boundaries, MCP-native, fast, deterministic, computation cache as primitive, artifact-boundary separation). Covers the three architectural planes (execution, artifact boundary, intelligence), the bright-line placement rule, the four boundary tests (air-gap, shutdown, control flow, trust), the MCP tool split across planes (23 tools), runtime plane type assertion pattern, and the trace ingestion boundary.

### [Deep Dives](deep-dives.md)

Fifteen foundational architecture decisions, each with rationale, alternatives considered, and retrofitting cost assessment:

1. Content-Addressed Graph (Merkle DAG)
2. Symbol Identity Scheme
3. Append-Only Edge Log (Event Sourcing)
4. Edge Provenance
5. Content-Addressed File Identity
6. Causal Ordering Across Repos
7. Schema Migration Framework
8. Deterministic Reindexing
9. Storage: SQLite Ledger + Pebble Acceleration Index (with full SQL schema)
10. Storage Interface (Backend Swappability, with full GraphStore interface)
11. Process Model
12. Content-Addressed Computation Cache (L1/L2/L3 tiers, six distribution capabilities)
13. Runtime Trace Ingestion (detailed design with TraceIngestor interface)
14. Semantic PR Diff (output format, CI integration, MCP tools)
15. Artifact-Boundary Plane Separation

Includes the summary table mapping each decision to its core principle and retrofit difficulty.

### [Merkle Tree Algorithms](merkle-algorithms.md)

Thirteen algorithms that exploit knowing's hierarchical Merkle tree structure to enable cheap invalidation, subgraph caching, and incremental recompute. Covers the upgrade from a flat snapshot hash to a multi-level tree (repo root, package roots, file roots, symbol roots, edge-type roots); Merkleized caching for `blast_radius`, `context_for_task`, and `test_scope` against subgraph roots rather than the global root; diff-guided incremental recompute that skips Louvain and HITS when unchanged packages can be identified by root comparison; content-addressed context packs with a `ContextPackRoot` for agent deduplication, cross-session replay, and retrieval history; Merkle proofs for agent trust and CI enforcement; federated graph sync via root exchange; semantic change classification from edge-type root diffs; snapshot-aware retrieval scoring; Merkleized feedback validity keyed to neighborhood roots; community rooting for safe agent parallelization; Merkle-based bisection for topology regression hunting; proof of absence for security audits; and lazy materialization for large-repo scalability. Includes a four-phase implementation plan.
