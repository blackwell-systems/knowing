# Design Principles

## Design Goals

- **Content-addressed**: every graph state is a hash; history, staleness, and integrity are structural properties, not bolted-on features. All hash inputs carry domain-type prefixes (`node\0`, `edge\0`, `snapshot\0`, `merkle\0`) so hashes from different entity types are structurally distinguishable, making cross-type hash collisions structurally impossible (same approach as git's `"blob <size>\0"` header)
- **Two-tier extraction**: 24 extractors across 12 languages + infrastructure; tree-sitter for fast AST parsing (seconds), LSP enrichment for type-resolved confidence (seconds more); graph is queryable after Tier 1
- **Git-driven incremental**: commits are the unit of change; git diff provides the exact changed file set; no filesystem walking or content hashing for change detection
- **Language-aware at boundaries**: Go calling Go is straightforward; Go calling a Python service via HTTP needs route mapping
- **MCP-native**: exposed as MCP tools, consumed by agents directly
- **Local-first, no paid LLM**: all indexing, retrieval, and ranking run locally without external API calls. Pure Go binary with no runtime dependencies beyond SQLite. Vector search (disabled by default) is the only component that would require an embedding model
- **Density-adaptive retrieval**: the system observes its own graph density at query time and adjusts seed selection strategy. On graphs exceeding 40K nodes, it automatically prefers type/interface nodes as RWR seeds (structural anchors walk down to methods via contains edges). This prevents the precision degradation that affects all static retrieval systems at scale. The system gets smarter with graph growth, not dumber.
- **Fast**: optimized for interactive agent queries over large multi-repo graphs
- **Deterministic**: same input at same commit always produces the same graph (verifiable via hash)
- **Hierarchical Merkle tree**: snapshots build a four-level tree (repo root -> package roots -> edge-type roots -> edge leaves); the flat tree was dropped after the hash domain prefix change made backward compatibility moot; `DiffHierarchicalTrees` is 216x faster on real data (~24.9K edges), 517x on 100K synthetic edges; `SubgraphRoot` gives O(1) cache keys per package set; `EdgeTypeRoot` answers "did call edges change?" in one lookup (see `internal/snapshot/hierarchical.go`)
- **Computation cache as a primitive**: every derived result (traversals, blast radius, semantic diffs) is a content-addressed artifact that can be stored, shared, synced, and referenced with the same guarantees as the graph itself
- **Artifact-boundary separation**: the system decomposes into execution (produces the graph), artifact (the graph itself), and intelligence (interprets the graph); intelligence features never write back to the graph and can operate entirely on a portable artifact

## Architectural Planes

knowing decomposes into three planes separated by an artifact boundary. This separation is structural, not organizational. Features are placed by a bright-line rule: if a feature's value depends on the system being alive, it belongs in the execution plane; if its value survives after the system stops, it belongs in the intelligence plane.

```
Execution Plane (produces the artifact)
├── Indexer (24 extractors across 12 languages + infrastructure)
│   ├── Go extractor (go/packages, full type resolution, `--full` flag)
│   ├── tree-sitter extractors (Go, Python, Ruby, TypeScript/JS, Rust, Java, C#, CSS, Protocol Buffers, GraphQL, SQL, OpenAPI/Swagger)
│   ├── Infrastructure extractors (Terraform HCL, Kubernetes YAML, Cloud YAML, Dockerfile, Makefile, Helm, GitLab CI, package.json/npm, .env, Event/MQ, CODEOWNERS)
│   ├── Docstring extraction (Go, Python, TypeScript, Rust, Java, C#) via shared `docextract` package
│   ├── Callback registration edges (event-driven patterns: `on`, `connect`, `subscribe`, `register`, `add_done_callback`)
│   ├── Interface method propagation (`overrides` edges from implementing methods to interface methods)
│   ├── Python import resolution (submodule imports resolved to file paths)
│   ├── Structural edges (`contains`/`member_of` from QN hierarchy; connects 77% of previously-disconnected type nodes)
│   └── SCIP ingest (`knowing ingest-scip`, external dependency surfaces)
├── Trace ingestion pipeline
│   ├── OTel span ingest
│   ├── HTTP access log ingest
│   └── Runtime symbol resolution (route path → graph node)
├── Daemon
│   ├── File watcher (fsnotify, git hook triggers)
│   ├── Incremental reindex (changed files only)
│   └── Snapshot manager (hierarchical Merkle tree computation, GC)
└── Graph store
    ├── SQLite backend (behind GraphStore interface)
    ├── Node/edge/snapshot storage
    └── Event log (append-only edge events)

════════════════════════════════════════════════════
Artifact boundary: the content-addressed graph
├── SQLite file (portable, self-contained)
├── Snapshot chain (root hashes, parent pointers)
├── Edge event log (full history)
├── Provenance metadata (per-edge)
└── Derived results (content-addressed computation artifacts)
════════════════════════════════════════════════════

Intelligence Plane (interprets the artifact)
├── Semantic PR diff (relationship-level impact per PR)
├── Graph-native test selection (affected tests from graph traversal)
├── Temporal reasoning (walk snapshots to find when incompatibilities appeared)
├── Organizational materialized views (team-scoped subgraphs, standing queries)
├── Ownership routing (who to notify, computed from graph edges)
├── Compliance audit (provenance verification, audit-date comparisons)
├── Confidence decay analysis (staleness scoring, reindex prioritization)
├── Agent coordination (pending mutations, multi-agent visibility)
├── Cross-machine cache sync (Merkle-based derived result exchange)
├── Federated graph queries (cross-instance queries via Merkle diff)
├── CI integration (GitHub Action for PR comments, threshold enforcement)
└── Staleness dashboard (subgraph freshness visualization)
```

**The artifact boundary rule:**

The content-addressed graph is the artifact contract. The execution plane produces it. The intelligence plane consumes it. Intelligence features never write edges, nodes, or snapshots back into the graph. They may produce derived results (which are themselves content-addressed artifacts stored alongside the graph), but derived results are a separate artifact class that does not participate in the Merkle DAG of the primary graph.

**Why this separation matters:**

The execution plane must be trusted. It determines what the graph contains, how symbols are identified, how edges are resolved, and how provenance is recorded. If the indexer is wrong, the graph is wrong. Trust in the execution plane is non-negotiable.

The intelligence plane does not need the same trust. It interprets the graph but cannot change it. A buggy semantic PR diff produces a bad report, not a bad graph. A slow temporal reasoning query wastes time, not integrity. Intelligence features can be opinionated, approximate, or even wrong without compromising the artifact. This asymmetry is the foundation of clean architectural separation.

**Applying the four boundary tests:**

| Test | Intelligence plane features | Result |
|------|---------------------------|--------|
| Air-gap test | Can they run on a different machine with only the SQLite file? | Yes. Copy the file, disconnect, query. |
| Shutdown test | Do they produce value if the indexer stops forever? | Yes. The last snapshot is still queryable. |
| Control flow test | Do they affect what the indexer produces? | No. They read the graph; they don't write to it. |
| Trust test | Would users trust the graph if these features were proprietary? | Yes. The graph is content-addressed and verifiable regardless. |

**The MCP tool split (28 tools):**

| Tool | Plane | Why |
|------|-------|-----|
| `index_repo` | Execution | Produces graph state |
| `cross_repo_callers` | Execution | Direct graph traversal (basic read) |
| `graph_query` | Execution | Direct graph query (basic read) |
| `repo_graph` | Execution | Direct graph read (repo-level view) |
| `blast_radius` | Intelligence | Computed analysis over the graph |
| `trace_dataflow` | Intelligence | Multi-hop interpreted traversal |
| `semantic_diff` | Intelligence | Snapshot comparison with classification |
| `pr_impact` | Intelligence | Semantic diff scoped to a PR |
| `snapshot_diff` | Intelligence | Structural diff between graph states |
| `stale_edges` | Intelligence | Staleness analysis |
| `ownership` | Intelligence | Cross-referencing graph with ownership metadata |
| `runtime_traffic` | Runtime | Query runtime-observed edges by service and route pattern |
| `dead_routes` | Runtime | Find route symbols with no recent observations |
| `trace_stats` | Runtime | Aggregate statistics about runtime-derived edges |
| `context_for_task` | Context | Token-budgeted context packing for a task description |
| `context_for_files` | Context | Blast-radius context for a set of changed files |
| `context_for_pr` | Context | PR-scoped context: RWR from changed symbols, callers, structural neighborhood |
| `feedback` | Feedback | Record/query symbol usefulness for ranking improvement |
| `test_scope` | Discovery | Find affected tests for changed files via BFS |
| `flow_between` | Discovery | Find all paths between two symbols via BFS |
| `plan_turn` | Discovery | Suggest relevant knowing tools for a task description |
| `communities` | Discovery | Louvain modularity-based graph clustering |
| `explain_symbol` | Context | Detailed scoring breakdown for a specific symbol given a task description |
| `ownership_query` | Discovery | Query code owners and authors for a file or symbol |
| `prove` | Audit | Generate Merkle inclusion proof for a relationship |
| `prove_absent` | Audit | Generate Merkle absence proof (relationship does NOT exist) |
| `fsck` | Audit | Verify graph integrity (hashes, referential integrity, snapshot chain) |

Basic graph reads (`cross_repo_callers`, `graph_query`, `repo_graph`) are execution-plane operations: they return what the graph contains without interpretation. Intelligence-plane tools compute, classify, compare, or aggregate, and they produce derived results that are themselves content-addressed artifacts. Context-plane tools (`context_for_task`, `context_for_files`, `context_for_pr`) are a specialized form of intelligence: they score and rank symbols from the graph, then pack them into a token budget for agent consumption.

**Runtime plane tools** require the underlying store to be a `SQLiteStore` (not just any `GraphStore` implementation). The MCP server obtains a `*SQLiteStore` via type assertion at construction time (`store.(*knowingstore.SQLiteStore)`), avoiding an import of the store package from the MCP handlers. If the assertion fails (e.g., when running against a mock store in tests), the runtime tools return an error indicating runtime queries are not available. This pattern keeps the MCP server decoupled from the concrete store implementation while providing access to runtime-specific query methods (`RuntimeEdgesByService`, `DeadRoutes`, `RuntimeEdgeStatsAggregate`) that are not part of the `GraphStore` interface.

**The trace ingestion boundary:**

Runtime trace ingestion straddles the planes. The ingest pipeline (normalizing spans, resolving symbols, writing edges) is execution: it produces graph state. The aggregation, confidence scoring, and decay analysis that operate on ingested edges are intelligence: they interpret what the ingest pipeline produced. The architecture separates these by interface: `TraceIngestor` belongs to the execution plane and writes to `GraphStore`; confidence decay and runtime aggregation caching belong to the intelligence plane and read from `GraphStore` and `ComputationCache`.

## Graph Structure

The knowledge graph uses 34 edge types (see `internal/edgetype/constants.go` and `docs/architecture/edge-types.md`). Notable structural properties:

- **Structural edges** (`contains`, `member_of`) connect type/class nodes to their methods and fields via qualified name hierarchy. These edges connected 77% of previously-disconnected type/class nodes (5,457/7,086 in k8s) that had zero edges, enabling RWR to walk from types to their methods and back.
- **Interface method propagation** creates `overrides` edges from implementing class methods to corresponding interface methods. When class C implements interface I and both define method M, C.M overrides I.M. This lets RWR walk from an interface method to all concrete implementations.
- **Callback registration edges** detect event-driven patterns (`on`, `connect`, `subscribe`, `register`, `add_done_callback`) and emit `calls` edges from the registrar to the callback argument, making event-driven code reachable via graph traversal.
- **Python import resolution** resolves submodule imports to actual file paths (e.g., `from operations import base` resolves through the module path to the target file), producing higher-confidence edges with accurate qualified names.

## Retrieval Pipeline

The context engine (`internal/context/`) implements task-based retrieval: given a natural-language task description, it returns a ranked, token-budgeted set of symbols from the graph.

**Full-text search:** FTS5 BM25 with 6 columns, weighted: `symbol_name` (10x), `concepts` (5x), `file_path` (4x), `qualified_name` (3x), `doc` (3x), `signature` (1x). The `concepts` column stores path-derived terms (directory names split on separators) so searching "migration" finds symbols in migration-related files. The `doc` column indexes docstrings extracted across 6 languages.

**Seed retrieval channels** (5, fused via Reciprocal Rank Fusion):

1. **Tiered keyword matching:** compound-first search (exact > prefix > substring > path) against qualified names
2. **BM25 full-text search:** compound-targeted query against the FTS5 index, with file_path prefix terms appended
3. **Vector (embedding) search:** semantic nearest-neighbor over symbol embeddings (disabled by default; requires embedding model)
4. **Equivalence class matching:** maps conceptual phrases in the task description to concrete symbol targets via curated + graph-derived alias dictionaries
5. **Path-context seeding:** extracts package/directory terms from the task description and finds TYPE/CLASS nodes in matching paths; types with `contains` edges serve as structural anchors for RWR walks

After seed retrieval, Random Walk with Restart (RWR) expands the seed set through the graph, HITS reranking promotes authorities, and community-aware scoring prevents single-cluster dominance.

**Benchmark results (fresh index, no enrichment):**

- P@10 = 0.207 across 167 tasks, 9 repos, 6 languages (Go, Python, TypeScript, Rust, Java, C#), 14K to 3.5M LOC
- Competitive advantage: vs codegraph 1.53x, vs GitNexus 2.76x, vs Gortex 3.29x, vs grep 15.9x
- Self-adapting type-seed preference: on dense graphs (>50K nodes), automatically prefers type/interface/class nodes as RWR seeds. VS Code +44%, zero regressions.
- Concept thesaurus: ~80 domain clusters expand BM25 queries with related code vocabulary.
- Parameter sweep (RWR alpha, seed count, score cutoff, blast radius weight, distance weight, confidence weight, RRF k, test penalty): all 9 parameters produce identical P@10. Quality is determined entirely by graph reachability (binary: is the symbol connected to any seed?), not by continuous parameter tuning.
- Implication: all P@10 improvements must target reachability (new edge types, new seed sources), not ranking (parameter adjustment).
