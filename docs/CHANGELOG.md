# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Performance

- **Single-pass body walk.** The Go extractor now visits each AST node in a function body exactly once, dispatching to all pattern detectors (calls, throws, routes, feature flags, endpoints) at each node. Previously 5 separate recursive walks per function body (5N node visits → N). This is the biggest extraction speedup: eliminates 80% of AST node visits.
- **Tree sharing across extractors.** When multiple extractors handle the same file (e.g., GoTreeSitterExtractor + EventExtractor for .go files), the file is parsed ONCE and the tree root is shared. Eliminates redundant tree-sitter parsing.
- **Parallel extraction.** File extraction runs across GOMAXPROCS worker goroutines (configurable via `--workers`). Thread-safe extractors create per-call parser instances.
- **Streaming commits.** Results are committed in batches of 500 files instead of one transaction at the end. Partial data survives a kill (resumable indexing).
- **Generated file detection.** Files with "Code generated", "DO NOT EDIT", or "AUTO-GENERATED" in the first 512 bytes are skipped.
- **Smart directory filtering.** Skips staging/, third_party/, _output/, hack/, generated protobuf, vendor, node_modules, dot-dirs (except .github), build output dirs.
- **Parallel FTS string splitting.** The expensive `splitForFTS` computation runs across 8 workers before the sequential SQLite INSERT phase.
- **Parallel git blame.** Authorship extraction (git blame per file) runs in parallel. `--skip-blame` flag skips it entirely for fast structural-only index.
- **Per-file timeout (10s).** Files that take too long to extract (usually huge generated test files) are skipped rather than blocking the pipeline.
- **Progress output.** Live progress to stderr: `[N/M] files/s, edges, ETA` during extraction, `Stored N/M files, nodes, edges` during storage. Total elapsed time on completion.

### Benchmarking

- **Cross-system context retrieval benchmark.** `bench/cross-system/`: rigorous comparison of context retrieval quality across 5 systems (knowing, GitNexus, Aider repo-map, CGC, raw grep) on identical tasks. 100 ground truth fixtures across 5 repos (kubernetes, TypeScript, flask, cargo, django), 3 difficulty tiers. Metrics: P@K, R@K, NDCG@10, MRR, F1, token efficiency. Statistical significance via Wilcoxon signed-rank test + Cohen's d + bootstrap CI. Spec: `docs/research/cross-system-benchmark.md`.
- **Benchmark adapters.** 5 system adapters with auto-detection of installed dependencies. Adapter registry reports available vs unavailable systems at runtime.
- **Symbol normalization.** Cross-system symbol matching handles knowing/GitNexus/Aider/SCIP/grep output format differences. 14 test cases covering all formats.

### Documentation

- **Product split proposal.** `docs/proposals/product-split.md`: staged approach to restricted product surfaces (knowing-scope for CI, knowing-audit for compliance). Monorepo binaries first, face repos only with proven demand. Edge-type filtering as product differentiator.
- **Comprehensive docs update.** All tool counts (27), edge type counts (30), and feature references updated across README, design-principles, system-overview, context-engine, roadmap, and CHANGELOG.

### Edge Type Expansion (P1)

- **`tests` edges.** Go tree-sitter extractor detects Test*/Benchmark* functions in `_test.go` files and creates `tests` edges to each production function they call. Makes test coverage a graph-queryable relationship. Provenance: ast_inferred, confidence 0.7, RWR weight 0.6.
- **`authored_by` edges.** New `internal/indexer/authorship` package runs git blame per file and creates `authored_by` edges from each symbol to its primary author (most lines). Synthetic author nodes (kind="author"). Provenance: git_blame, confidence 1.0, RWR weight 0.0.
- **`ownership_query` MCP tool.** Queries `owned_by` and `authored_by` graph edges to answer "who owns this code?" Accepts file_path or symbol name, returns CODEOWNERS teams and git blame authors. Tool #27.
- **`internal/edgetype` constants package.** Single source of truth for all edge type string constants. `RWRWeight()` function returns canonical weights for each type.
- **RWR zero-weight fix.** Changed `if w == 0 { w = 0.3 }` to `w, ok := map[...]; if !ok { w = 0.3 }` in walk.go so explicit 0.0 weights (owned_by, authored_by) are respected. Ownership edges are genuinely excluded from the random walk.
- **Ownership extractor tests.** 14 tests covering ParseCodeowners, FindCodeowners, matchPattern, and ExtractOwnership.

### Edge Type Expansion (P2)

- **`documents` edges.** Go tree-sitter extractor creates synthetic doc_comment nodes and `documents` edges from them to the documented function, method, or type. Makes documentation a graph-queryable relationship. Provenance: ast_inferred, confidence 0.9, RWR weight 0.2.
- **`consumes_endpoint` edges.** Go and TypeScript extractors detect HTTP client calls (`http.Get(...)`, `fetch("/api/...")`, `axios.get(...)`) and create `consumes_endpoint` edges from the calling function to a synthetic endpoint node. Bridges frontend-to-backend and service-to-service API contracts. Provenance: ast_inferred, confidence 0.6, RWR weight 0.5.
- **`implements_rpc` edges.** Go tree-sitter extractor detects structs embedding `pb.Unimplemented*Server` patterns and creates `implements_rpc` edges to the corresponding proto service. Makes gRPC service implementations graph-queryable. Provenance: ast_inferred, confidence 0.9, RWR weight 0.8.
- **`consumes_rpc` edges.** Go tree-sitter extractor detects `pb.New*Client(conn)` patterns and creates `consumes_rpc` edges from the calling function to the proto service. Maps gRPC client dependencies. Provenance: ast_inferred, confidence 0.8, RWR weight 0.6.
- **`gated_by_flag` edges.** Go tree-sitter extractor detects feature flag SDK calls (`client.BoolVariation("flag")`, `unleash.IsEnabled("flag")`) and creates `gated_by_flag` edges from the function to a synthetic flag node. Enables "what code is behind this flag?" queries. Provenance: ast_inferred, confidence 0.8, RWR weight 0.3.
- **`deployed_by` edges.** GitHub Actions extractor detects deployment steps (docker push, kubectl apply, cloud deploy actions) and creates `deployed_by` edges from deployment targets to workflow nodes. Maps CI/CD deployment topology. Provenance: ast_inferred, confidence 0.9, RWR weight 0.4.
- **`tested_by` edges.** GitHub Actions extractor detects test commands (go test, npm test, pytest, cargo test) in workflow run steps and creates `tested_by` edges from package/module nodes to the testing workflow job. Makes CI test coverage graph-queryable. Provenance: ast_inferred, confidence 0.8, RWR weight 0.5.
- **Edge type count now 30.** All edge types registered in `internal/edgetype/constants.go` with canonical `RWRWeight()` function.
- **All 18 edge weights in walk.go.** The `edgeWeight` map in `internal/context/walk.go` now covers all static edge types with explicit weights; zero-weight fix ensures ownership/authorship edges are genuinely excluded.

### Phase 4: Proofs and Audit (Continued)

- **Proof of absence.** `GenerateAbsenceProof`/`VerifyAbsenceProof` prove an edge does NOT exist by showing adjacent sorted leaves that bracket the missing hash. No tree restructuring needed: the sorted binary tree already has the ordering invariant. `knowing prove-absent` CLI command.
- **`knowing audit` CLI.** Generates a structured compliance report: integrity check (fsck), graph summary, all cross-package edges with provenance and confidence, optional Merkle proofs for every cross-package relationship. One command for a complete audit artifact.
- **`knowing audit-diff` CLI.** Compares two audit point snapshots: added/removed edge counts, change classification (behavioral/structural/runtime_drift/metadata_only).
- **Hash JSON marshaling.** `types.Hash` now implements `MarshalJSON`/`UnmarshalJSON` as hex strings. Proofs serialize as 3KB (was 11KB with byte arrays). `types.ParseHash` for hex decoding.

### Graph Correctness

- **Removed-edge diffs fixed (P0).** Migration 013 adds `source_hash`, `target_hash`, `edge_type`, `confidence`, `provenance` to `edge_events`. `RecordEdgeEvent` stores full edge data. `SnapshotDiff` reads from events directly via COALESCE, no longer joins to deleted edges. `knowing diff` now correctly shows removed edges.
- **Synthetic file nodes stored (P0).** Go tree-sitter extractor creates file nodes (kind="file") when import edges exist. Import edge sources are no longer dangling.
- **Phantom external nodes.** Extractor creates `kind="external"` nodes for stdlib/external targets at extraction time. LSP enricher runs a post-enrichment sweep for any remaining dangling targets. Result: zero dangling edges on a correctly indexed repo. `knowing fsck` reports 0 errors.
- **`knowing fsck` roster awareness.** Dangling edges classified as `cross_repo` (target in another roster DB), `stdlib`, or `truly_dangling`. Only truly_dangling counts as an error. Roster stores opened once per verify call.
- **Cross-repo method call resolution.** Extractor uses kind="method" for selector expression calls on non-import operands. LSP enricher resolves definitions across repos via roster lookup.
- **`ExtractPackagePath` method name fix.** Splits at first dot after last slash (not last dot overall). Fixes false hash mismatches on method nodes like `pkg.Type.Method`.

### Cross-Repo

- **Roster moved to `internal/roster` shared package.** Both CLI and indexer use the same roster code. Eliminates duplicate logic.
- **Roster-based module mapping.** Indexer's `buildModuleToRepoMap` merges the global roster's module map. Cross-repo edge targets now use the correct repo URL.
- **Synthetic cross-repo test fixture.** 3 Go modules (module-a shared library, module-b imports A, module-c imports A+B) with real cross-repo edges. setup.sh initializes independent git repos.
- **Cross-repo findings documented.** 5 architectural proofs, full dangling edge classification, P0 verification results.
- **6 duplicate `extractPackage` functions consolidated** to canonical `snapshot.ExtractPackagePath`.
- **`CollectEdgeInputs` exported** as canonical edge source for tree construction and proof generation.

### Positioning

- **Dual identity (agents + audit/compliance)** established across README, tagline, GitHub description, GitHub topics, competitive analysis.
- **Audit & compliance guide** (`docs/guide/audit-compliance.md`): 6 provable claims, 6 workflows, comparison table.
- **Merkle proofs architecture doc** (`docs/architecture/merkle-proofs.md`): proof format, verification algorithm, batch proofs, absence proofs.
- **Data model architecture doc** (`docs/architecture/data-model.md`): full schema, 13 migrations, cross-repo identity, phantom nodes.
- **GitHub topics** added: `audit`, `compliance`, `merkle-proof`, `software-supply-chain`, `static-analysis`.

### Documentation

- CLI guide: `prove`, `verify`, `prove-absent`, `audit`, `audit-diff` (24 subcommands).
- Architecture docs swept: 11 files fixed for flat tree references, equivalence class counts, Phase 4 status.
- Roadmap: Production Scale vision, Grafana ecosystem validation, cross-repo awareness for non-Go extractors.
- All benchmark FINDINGS refreshed.

## [v0.4.0] - 2026-05-19

### Phase 3: Incremental Recompute (Complete)

All 11 items shipped. The system now skips work the Merkle tree proves unchanged.

- **F1: Graph notes table.** `graph_notes` (migration 012): general-purpose metadata layer. 6 GraphStore methods, `BatchPutNotes` for 21x faster bulk writes.
- **F2: Incremental Algorithm interface.** `DetectIncremental` on Louvain (6.9x) and LabelPropagation (38.4x). Freezes unchanged nodes; only changed nodes move.
- **F3: Scoped FTS rebuild.** `RebuildFTSForPackages` scopes BM25 index to changed packages (2.9x faster).
- **P1: Community assignment persistence.** Save/Load via notes table. Communities survive daemon restart.
- **P2: Context pack persistence.** Three-layer cache: SubgraphCache (42ns) -> notes table (1.2ms) -> cold retrieval. Cross-session replay verified.
- **P3: Incremental Louvain e2e.** Daemon wired: diff -> ChangedPackages -> load previous -> DetectIncremental -> delta-save. Full cycle: 2.5ms.
- **P4: Incremental HITS/BM25.** Daemon calls scoped FTS rebuild with changed packages from Merkle diff.
- **P5: Context pack deduplication.** `pack_root` parameter on `context_for_task`. Returns "unchanged" (165 bytes) instead of full payload (2-30KB). 93-99% byte savings. PackRoot exposed in GCF header and JSON output.
- **P6: Context pack comparison.** `CompareContextPacks` returns added/removed/common symbols between two packs.
- **P7: Semantic change classification.** `ClassifyChanges` returns Behavioral/Structural/RuntimeDrift/MetadataOnly based on which edge-type roots changed.
- **P8: Delta-save community assignments.** Writes only changed assignments. 5.0x e2e speedup (12.6ms -> 2.5ms).

### Phase 4: Proofs (Started)

- **Merkle proofs.** `GenerateProof`/`VerifyProof` in `internal/snapshot/proof.go`. Three-level proof path: edge -> edge-type root -> package root -> repo root. Generation: 72us. Verification: 1.2us. Proof size: 16 steps, 656 bytes on the live graph (12,604 edges).

### Code Quality

- **Benchmark robustness.** Performance contracts added to context-relevance, test-scope-accuracy, edge-accuracy, merkle-diff, community-detection e2e. Every quality metric now has a regression floor.
- **Canonical package path extraction.** 6 duplicate `extractPackage` functions consolidated to `snapshot.ExtractPackagePath`. `CollectEdgeInputs` exported as canonical edge source. Prevents divergence between tree construction and proof generation.
- **14 benchmark harnesses** with self-documenting FINDINGS.md and performance contracts.

### Documentation

- Features.md updated to 101 features (91-101 for Phase 3 + Phase 4).
- Architecture docs swept: 11 files fixed (flat tree references, equivalence class counts, Phase 4 status).
- MCP tools docs: `pack_root` parameter documented.
- Wire format docs: PackRoot in GCF header and JSON output.
- Context packing docs: three-layer cache, P5 dedup, P6 comparison.
- npm and PyPI READMEs rewritten.
- README: 14 benchmarks, three-layer cache section, updated diagram.

## [v0.3.0] - 2026-05-19

### Breaking

- **Hash domain prefixes (requires re-index):** All hash computations now include git-style type separation prefixes: `node\0`, `edge\0`, `snapshot\0`, `merkle\0`. Eliminates cross-type hash collisions. Databases built before this release must be re-indexed (`knowing index <path>`). Run `knowing fsck` after re-indexing to verify integrity.

### Architecture

- **Hierarchical Merkle tree (Phase 1+2, complete):** `HierarchicalTree` struct with `BuildHierarchicalTree`, `DiffHierarchicalTrees`, `SubgraphRoot`, `EdgeTypeRoot`, and `ContextPackRoot`. Wired into `SnapshotManager.ComputeSnapshot`. The hierarchical root is the canonical snapshot hash; no flat tree is maintained. Benchmarked at 114x faster diff (11K edges, 111 packages), 517x at 100K synthetic edges. Subgraph root lookups at 59ns.
- **Subgraph cache:** Content-addressed cache keyed by Merkle package roots. 93x faster repeat queries (160ms -> 1.7ms). Daemon invalidation via `DiffHierarchicalTrees` scoped to changed packages (~6us overhead per re-index). 42ns raw cache lookups.
- **Content-addressed context packs:** `PackRoot` field on `ContextBlock`, computed as `hash(task_normalized + sorted(selected_node_hashes))`. Same task + same graph = same PackRoot. Perfect deduplication verified.
- **Community Merkle roots:** Each Louvain community carries a Merkle root over the packages it spans. Roots verified distinct per community on live graph.
- **`DiffOptions` with `PackageFilter` and `MaxChanges` cap:** Callers can scope diffs to specific packages and cap result size. Matches git tree-diff's pathspec filtering.
- **Modular community detection:** `Algorithm` interface and registry in `internal/community/`. Louvain (two presets), label propagation. `knowing export --algorithm` and `communities` MCP tool accept algorithm parameter.
- **Graph notes table (Phase 3 Foundation F1):** `graph_notes` table (migration 012) for metadata that never affects Merkle computation. Composite key `(object_hash, key)` with upsert semantics. 6 new `GraphStore` methods. Foundation for community assignment persistence and context pack persistence.
- **`knowing fsck` integrity checker:** Edge referential integrity, hash recomputation, snapshot chain continuity. Classifies issues as ERROR or WARN. 98ms median on live graph.
- **GC reachability sweep:** `GarbageCollectFull` prunes orphaned nodes and edges after snapshot deletion. 70ms with 500 injected orphans, 53ms steady state.
- **Daemon lockfile:** Prevents multiple daemon instances on the same database. PID tracking with stale lock cleanup.
- **`PRAGMA integrity_check` on startup:** Catches SQLite-level corruption before the application layer sees inconsistent data.
- **In-process LRU cache:** 50K-entry cap on `GetNode`/`GetEdge`. Eliminates redundant SQL round-trips on hot-path traversals.
- **`VerifyNodeHash` / `VerifyEdgeHash`:** Recomputation functions for integrity verification.
- **`indexed_at` epoch column:** Migration 011. Used by GC to identify stale objects.
- **`extractPackagePath` error handling:** Returns error on malformed qualified names instead of silently grouping under `_root`.

### Added

- **8 MCP resources:** `knowing://report`, `knowing://schema`, `knowing://stats`, `knowing://repos`, `knowing://session`, `knowing://index-health`, `knowing://communities`, `knowing://community/{id}`. Read-only orientation for agents at zero tool-call cost.
- **`knowing mcp` subcommand:** Stdio MCP server mode for AI agent integration via `.mcp.json`.
- **`knowing watch` subcommand:** Lightweight file watcher with debounce and optional LSP enrichment.
- **`knowing mcp --watch` flag:** Combined MCP server + file watching in a single process.
- **Per-repo database isolation:** Each repo gets its own database at `~/.knowing/repos/<safe-name>.db`. Community detection, RWR, HITS, BM25 operate on isolated data.
- **Repo roster:** `knowing add`, `knowing remove`, `knowing list` for multi-repo management.
- **`knowing enrich blame`:** Git blame metadata (last_author, last_commit_at) on symbols.
- **`knowing enrich coverage`:** Coverage percentages from Go cover profiles on symbols.
- **84 equivalence classes:** Expanded from 41 with 43 universal software concepts.
- **TOON wire format support:** Token estimation and format-aware packing.
- **14 benchmark harnesses:** Merkle diff (core + context packs + Phase 2 persistence + P5 dedup + proof + FTS scoped), community detection, subgraph cache, fsck, GC, context relevance, edge accuracy, test scope, token savings, feedback loop. All self-documenting with FINDINGS.md.

### Correctness

- Multiple daemon instances on the same database now produce a clear startup error.
- GC prunes orphaned nodes and edges (not just old snapshots), preventing unbounded table growth.
- `extractPackagePath` no longer silently groups malformed names under `_root`.
- `DiffHierarchicalTrees` nil-tree guard added (previously would panic).
- `DeleteNodesNotIn` / `DeleteEdgesNotIn` added to `GraphStore` for GC.

### Visualization (knowing-viz)

- **Full React migration:** React + Zustand + `@react-sigma/core` + `react-force-graph-3d` + Framer Motion.
- **6 modular grouping strategies:** Package, community, edge type, file, author, none.
- **Provenance and edge type filtering:** Composable filters with live counts.
- **Timeline snapshot picker:** View graph at any point in the snapshot chain.
- **Blame author click-to-filter:** Click author name to filter to their symbols.
- **Configurable max groups slider:** Range 1-100 (previously hardcoded at 20).

### Documentation

- Docs reorg: flat `docs/` migrated to `guide/`, `architecture/`, `research/`, `operations/`, `internal/`.
- ADR for hierarchical Merkle tree (`docs/architecture/adr-hierarchical-merkle.md`).
- Git design audit (`docs/architecture/git-design-audit.md`): 10-area comparison, 23 recommendations, all CRITICAL/HIGH/MEDIUM fixed.
- Whitepaper revised: "The Hierarchical Identity Architecture" with editorial review applied.
- Features.md updated to 90 features with 33 GraphStore methods.
- Roadmap updated with Phase 3 foundation and feature specifications.

### Changed

- Positioning reframed as "intelligence versioning system."
- Flat Merkle tree dropped; hierarchical root is canonical snapshot hash.
- npm and PyPI READMEs rewritten for current capabilities.

## [v0.2.0] - 2026-05-18

### Added

- `knowing why` subcommand: explains why a symbol ranked where it did in retrieval results. Shows seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, and equivalence class matches. Usage: `knowing why -task "refactor auth" -symbol "SessionHandler"`.
- `explain_symbol` MCP tool (tool #23): MCP equivalent of `knowing why`. Parameters: `task_description` (required), `symbol` (required). Returns markdown-formatted scoring breakdown. Registered in the context packing tools group.
- `knowing ingest-scip` subcommand: imports SCIP protobuf index files for external dependency symbols (provenance `scip_resolved`, confidence 0.95)
- SCIP ingestor (`internal/indexer/scipingest/`): reads `.scip` files, creates nodes and `references` edges for all symbols and references
- Cloud extractor (`internal/indexer/cloudextractor/`): single extractor handling 4 cloud YAML formats (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework)
- New edge types from cloud extractor: `publishes`, `subscribes`, `connects_to`
- Event extractor package (`internal/indexer/eventextractor/`): detects Kafka/NATS/SQS/AMQP producer/consumer patterns across Go, TypeScript, Python, Java (package exists, not yet registered in CLI)
- Schema extractor package (`internal/indexer/schemaextractor/`): OpenAPI/JSON Schema references (package exists, not yet registered in CLI)
- `KNOWING_DB` environment variable for global database path (used by all CLI subcommands)
- `knowing mcp` subcommand for stdio MCP server mode (used by AI agents via .mcp.json)
- 5 new MCP tools (total now 23): `feedback`, `test_scope`, `flow_between`, `plan_turn`, `communities`
- `feedback` MCP tool: record/query symbol usefulness for agent learning loop (FeedbackProvider interface wired into ContextEngine)
- `test_scope` MCP tool: backward BFS from changed symbols to find affected test functions
- `flow_between` MCP tool: BFS path finding between two symbols (up to 10 paths)
- `plan_turn` MCP tool: keyword-based task-to-tool recommender with pre-filled argument suggestions
- `communities` MCP tool: Louvain modularity clustering with `list` and `for_symbol` actions
- `knowing export -format dot`: Graphviz DOT export with Louvain community subgraphs and cross-community edge highlighting
- Community-annotated JSON export: nodes include `community` ID, edges include `cross_community` flag, top-level `communities` array with labels and sizes
- `NodesByFilePath` store method (joins nodes to files via SQL) for test-scope and context engine
- Feedback benchmark (`bench/feedback-loop/`) proving compounding thesis
- HITS (Hyperlink-Induced Topic Search) reranking on RWR subgraph: boosts task-relevant authorities, penalizes generic infrastructure hubs. Score differentiation improved from 0.01 spread to 0.35 spread across results.
- Density-ranked knapsack packing: score/cost ratio optimization maximizes total relevance within token budgets. Small high-value symbols (types, interfaces) now beat large medium-value symbols when budget is tight.
- Protobuf/gRPC extractor: extracts service, message, enum, and RPC declarations from .proto files with references edges for field types and RPC request/response types
- All 25 extractors now registered in CLI: Go, Python, TypeScript/JS, Rust, Java, C#, Terraform, SQL, K8s YAML, Cloud YAML (CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless), CSS, Protocol Buffers, Dockerfile, Makefile, Helm Charts, GitLab CI, package.json/npm, GraphQL, Ansible
- Dockerfile extractor (`internal/indexer/dockerfileextractor/`): extracts FROM base image dependencies, COPY --from multi-stage build references, EXPOSE port declarations
- Makefile extractor (`internal/indexer/makefileextractor/`): extracts target dependencies, include directives, variable references
- Helm chart extractor (`internal/indexer/helmextractor/`): extracts chart dependencies from Chart.yaml, template references, values injection
- GitLab CI extractor (`internal/indexer/gitlabciextractor/`): extracts job needs, extends templates, include files, artifact dependencies
- package.json extractor (`internal/indexer/npmextractor/`): extracts npm dependencies, devDependencies, peerDependencies, scripts
- GraphQL extractor (`internal/indexer/graphqlextractor/`): extracts type definitions, field type references, interface implementations, operation-to-field calls
- Ansible extractor (`internal/indexer/ansibleextractor/`): extracts playbook roles, task dependencies, variable references, handler notifications
- Random Walk with Restart (RWR) algorithm for graph-based relevance scoring in context engine
- Improved keyword extraction with stop word filtering, CamelCase splitting, and abbreviation expansion
- Relative normalization in ranking and base recency score for static-only edges
- BM25 full-text search via FTS5 index (migration 006): `nodes_fts` virtual table over qualified_name, signature, file_path with CamelCase-aware tokenization. Supplements 5-tier keyword seeding when fewer than 8 candidates found.
- Session-aware retrieval boosts (`internal/context/session.go`): `SessionTracker` applies exponential-decay recency boost (3-minute half-life, capped at 2.0x, 0.20 weight) to symbols returned by recent context queries. One tracker per MCP server lifetime.
- Noise filtering (`filterNoisySymbols`): excludes mock/stub/fake symbols and `/build/`/`.bundle.` file paths from context candidates.
- Equivalence class seed retrieval (`internal/context/equivalence.go`): 20 hand-curated concept classes (TRANSITIVE_IMPACT, SYMBOL_LOOKUP, DATAFLOW_TRACE, TEST_SELECTION, etc.) with 200+ phrases mapped to target symbols. Cross-product expansion with action verbs. Fused as RRF Channel 4 (weight 2.0). Biggest single-feature improvement: hard tier P@10 10% to 18%.
- 4-channel RRF fusion (`rrfFuseMulti` in `internal/context/context.go`): replaces single-channel tiered matching with N-channel Reciprocal Rank Fusion. Channel 1: tiered keywords (weight 3.0), Channel 2: BM25 FTS5 (weight 1.0), Channel 3: vector/embedding (weight 0.0, disabled), Channel 4: equivalence classes (weight 2.0).
- Doc comment extraction (Node.Doc field): migration 007 adds `doc` column to nodes table. Go tree-sitter extractor extracts doc comments for functions, methods, and types via language-agnostic `extractDocComment` function (uses tree-sitter PrevSibling). Included in embedding text.
- BGE-small-en-v1.5 embedding model: replaces MiniLM-L6-v2 (same 384 dims, retrieval-tuned). Currently disabled (weight 0.0) as off-the-shelf models tested net-negative. Infrastructure preserved: hugot ONNX runtime, coder/hnsw, RRF channel.
- BFS depth limit on RWR walk (`buildAdjacencyMap` in `internal/context/walk.go`): limited to 4 hops from seeds for performance improvement.
- Eval expanded to 55 fixtures (20 easy, 20 medium, 15 hard). Current baseline: Easy 38.5%, Medium 32.0%, Hard 18.0%, Overall 30.5% P@10, 0.53 MRR.
- MCP `notifications/message` notification when vector index is ready after indexing.

- Bigram compound keyword extraction: joins adjacent non-stop-words into CamelCase and snake_case variants ("blast radius" -> BlastRadius, blast_radius) for multi-word symbol matching.
- Universal equivalence classes (`internal/context/universal_seeds.go`): 20 any-repo software concepts (authentication, caching, configuration, database, HTTP, testing, concurrency, etc.) at weight 0.8. Cross-repo eval +6.7pp on gortex.
- Graph-derived aliases (`internal/context/graph_aliases.go`): auto-generates equivalence classes from caller/callee symbol names. Top-10 tiered candidates, weight 0.7.
- Passive task memory (`internal/context/task_memory.go`): migration 008 adds `task_memory` table. Records top-5 returned symbols per `context_for_task` call. Recall matches keywords with 7-day linear decay, boosts via FeedbackBoost channel at 0.3x scale.
- Multi-language LSP enrichment (`internal/enrichment/config.go`): auto-detects language servers (gopls, typescript-language-server, pylsp/pyright, rust-analyzer, jdtls, OmniSharp) by checking project markers and PATH. `LSPServerConfig` struct, `DetectLSPServers`, `SetLSPConfig`, `LoadLSPConfig` for knowing-lsp.json override.
- Multi-language enrichment pipeline (`internal/enrichment/enricher.go`): removed hardcoded .go file checks in `discoverNewEdges`, now uses language filter from `runForServer` for all 6 detected language servers. Added `resolveNamePosition()` to fix pyright's SelectionRange pointing at keywords (class/def) instead of symbol names. Removed dead `openAllFiles`/`closeAllFiles` Go-only legacy methods.
- Python call-site positions: `CallSiteLine`, `CallSiteCol`, `CallSiteFile` on call edges. Provenance changed from `ast_resolved/1.0` to `ast_inferred/0.7`. Enclosing function node hash threaded through `walkNode` so call edges use real function nodes as sources (enables enricher to find them via `NodesByName`).
- Workspace readiness wait in enricher: calls `WaitForWorkspaceReadyTimeout(120s)` after opening files, before querying. Servers that index asynchronously (jdtls, tsserver) now wait for `$/progress` tokens to complete. Synchronous servers (gopls) return immediately.
- Java (jdtls) enrichment validated: 870/1,046 edges upgraded (83.2%), 155 new edges discovered on Spring Petclinic.
- TypeScript enrichment validated: 90/91 edges upgraded (98.9%), 36 new edges discovered on anthropic-docs-mcp-ts.
- Python enrichment validated: 6,906/8,310 edges upgraded (83.1%), 15,211 new edges discovered on FastAPI.
- TOON wire format encoder (`internal/wire/toon.go`): uses official `toon-format/toon-go` library. TOON v3.0 open standard (~60% token savings). Registered as `format: "toon"`.
- Format-aware token estimation (`EstimateNodeTokensForFormat`): GCF packs 5-7x more symbols per token budget. At 1K tokens: 28 (JSON) vs 197 (GCF) symbols.
- Cross-repo eval (`eval/crossrepo_test.go`): 30 gortex fixtures (10 exact, 10 concept, 10 multi_hop). Tests pipeline on external codebase with zero config. Result: 46.7% R@10.
- Information density benchmark: grep output is 0-8% relevant, knowing output is 20-80% relevant. 3-14x more useful information per token.
- `knowing init` full setup command: indexes repo, auto-detects and runs LSP enrichment, generates CLAUDE.md, configures Claude Code MCP server in ~/.claude.json. One command to go from zero to operational.
- Whitepapers moved to `docs/whitepapers/` with descriptive names: `content-addressed-graph-intelligence.md`, `gcf-wire-format.md`, `shared-intelligence-layer.md`.
- 4 breakout documentation guides: `docs/retrieval-pipeline.md`, `docs/equivalence-classes.md`, `docs/hooks-integration.md`, `docs/eval-framework.md`.
- 23 experiments documented in `eval/EXPERIMENTS.md` with 12 key insights.
- Roadmap expanded with `knowing why`, session memory persistence, negative feedback, `knowing stats`, `knowing watch`, staleness reporting, and underexploited capabilities sections.

### Fixed

- Eval framework `isRelevant` matching now handles `package.Type.Method` qualified names correctly
- `filterNoisySymbols` added to remove mock/stub/fake symbols that polluted context results
- `test-scope` command: fixed `symbolsInFiles` returning empty results due to stale FileHash mismatch after re-indexing
- `test-scope` command: fixed package path extraction producing invalid `go test` paths (was not stripping module prefix)
- Context engine `ForFiles` and `ForPR` now use `NodesByFilePath` join (was broken with stale FileHash matching)
- HITS node selection now operates on top-N by RWR score (was random map iteration order)
- Context engine uses substring search for keyword matching (was requiring exact match)
- mkdocs.yml and index.md added for docs workflow
- Architecture doc updated to reflect actual codebase structure
- Enrichment queries returning empty results on async language servers (jdtls, tsserver) due to querying before workspace indexing completed

## 2026-05-15

### Added

#### Core Graph Engine
- Content-addressed knowledge graph with Merkle DAG snapshots (SHA-256 node/edge/root hashes)
- SQLite-backed GraphStore with WAL mode, 20+ methods, recursive CTEs for transitive queries
- 4 schema migrations (initial, dangling edges, call-site columns, runtime observation columns)
- Append-only edge event log with "added"/"removed" recording on every index run
- Snapshot chain with parent pointers, Merkle root computation, diff, and garbage collection
- Content-addressed file identity for rename survival and deduplication
- Deterministic reindexing (same input produces identical snapshot hashes)

#### Incremental Change Detection
- Git-based change detection: watches `.git/HEAD` and `.git/refs/heads/*` (1-2 file descriptors)
- `GitDiffFiles` resolves changed/added/deleted files via `git diff --name-status`
- Old symbol cleanup: `DeleteNodesByFile` and `DeleteEdgesBySourceFile` remove stale data before re-extraction
- Edge event recording: computes diff between old and new edges per file, writes to edge_events table
- Scoped enrichment: LSP enrichment processes only edges from changed files
- Snapshot-commit alignment: every snapshot corresponds to a single commit

#### Language Extractors
- Go tree-sitter extractor (default fast path): declarations, imports, call edges with positions, confidence 0.7
- Go packages extractor (`--full` flag): full type resolution via `go/packages`, confidence 1.0
- Python tree-sitter extractor: functions, classes, methods, imports, calls
- TypeScript/JavaScript extractor: Express.js route detection
- Rust extractor: Actix, Axum, Rocket route detection
- Java extractor: Spring annotation route detection
- C# extractor: ASP.NET attribute route detection
- HTTP route detection for 10+ framework patterns (net/http, chi, gin, echo, gorilla/mux, Express, Actix, Axum, Rocket, Spring, ASP.NET)
- Worker pool parallelism (`runtime.GOMAXPROCS` goroutines, order-preserving fan-out/fan-in)

#### LSP Enrichment
- Two-tier extraction: tree-sitter for instant graph (~1.5s), LSP for accuracy (background)
- Enrichment via `agent-lsp/pkg/lsp` starts gopls, opens all Go files, upgrades edges to `lsp_resolved` (0.9 confidence)
- Call-site positions (line, column, file) stored on edges for LSP confirmation
- Discovery of `implements` and `references` edges via document symbols
- Cold index benchmark: 9.1 seconds (108x faster than go/packages baseline of 16m 24s)

#### Cross-Repo Resolution
- `internal/resolver/` package for retargeting dangling edges across repositories
- 4 GraphStore methods: `DanglingEdges`, `AllRepos`, `NodesByQualifiedName`, `DeleteEdge`
- `ModuleToRepoURL` map populated from go.mod of indexed repos
- Verified: 228 cross-repo edges between polywave-web and polywave-go

#### Runtime Trace Ingestion
- OTel trace pipeline: `TraceSpan` normalization, span-to-edge conversion, batch accumulation
- OTLP gRPC receiver (`collectortrace.TraceServiceServer`) on configurable endpoint
- Symbol resolver: maps HTTP routes and gRPC methods to graph node hashes via `route_symbols` table
- Observation-based confidence scoring: 0.95 (>1000 obs), 0.85 (100+), 0.7 (10+), 0.5 (1+), 0.2 (stale)
- Confidence decay over time without re-observation; GC-eligible after 90 days
- Batch accumulation with configurable flush interval
- Daemon `traceIngestLoop` goroutine with periodic flush and decay
- Migration 004: `observation_count`, `last_observed` columns on edges; `route_symbols` table

#### Semantic PR Diff
- `internal/diff/` package: `SemanticDiff` (enriches snapshot diff with node metadata, detects modifications)
- `PRImpact`: blast radius for changed symbols, risk classification (low/medium/high), transitive callees (depth 3)
- GitHub Action (`pr-semantic-diff.yml`): indexes both branches, computes diff, posts/updates PR comment

#### Graph-Aware Context Packing
- `internal/context/` package: `ContextEngine` with task-based and file-based context queries
- Random Walk with Restart for relevance scoring from seed nodes
- Token-budgeted output in XML, Markdown, or JSON format
- Ranking by blast radius, confidence, recency, and graph distance
- Keyword extraction with stop word filtering and CamelCase splitting

#### Developer CLI
- `knowing index` (default: tree-sitter fast path; `--full`: go/packages)
- `knowing serve` (daemon with MCP server, git watcher, optional `--trace` for OTel ingestion)
- `knowing diff` (semantic PR diff with JSON and human-readable output)
- `knowing export` (full graph dump for visualization, `--format json`, `--repo` filter)
- `knowing context` (`--task` or `--files`, `--budget`, `--format`)
- `knowing query` (symbol search by qualified name prefix)
- `knowing mcp` (stdio MCP server for AI agent integration)
- `knowing version`

#### MCP Server (16 tools over stdio + HTTP)
- Execution plane: `index_repo`, `cross_repo_callers`, `graph_query`, `repo_graph`
- Intelligence plane: `blast_radius`, `trace_dataflow`, `stale_edges`, `snapshot_diff`, `semantic_diff`, `pr_impact`, `ownership`
- Runtime plane: `runtime_traffic`, `dead_routes`, `trace_stats`
- Context plane: `context_for_task`, `context_for_files`

#### Infrastructure
- CI workflow (`.github/workflows/ci.yml`): build, vet, test on push/PR
- Release workflow (`.github/workflows/release.yml`): GoReleaser with 6 platform binaries
- Docs workflow (`.github/workflows/docs.yml`): mkdocs-material to GitHub Pages
- GoReleaser v2 config: Homebrew formula, Docker multi-arch images, npm/PyPI/Winget publishing
- Distribution strategy: Homebrew, Scoop, Winget, npm, PyPI, Docker (GHCR + Docker Hub), go install, curl|sh

#### Documentation
- Architecture doc with 15 design decisions, concepts section, concurrency model, data flow
- FEATURES.md: 30 features with packages, entry points, limitations
- CLI reference (`docs/CLI.md`): all subcommands with flags and examples
- MCP tools reference (`docs/MCP-TOOLS.md`): all 16 tools with parameters and return formats
- Distribution strategy (`docs/DISTRIBUTION.md`)
- Runtime trace design (`docs/runtime-traces.md`)
- Implementation log (`docs/implementation-log.md`)
- Deployment models (`docs/deployment.md`)
- Package-level and exported-symbol doc comments across all 18 packages

### Fixed

- `ComputeNodeHash` no longer includes contentHash in hash computation (was causing cross-package caller queries to return empty)
- GoExtractor uses `types.EmptyHash` consistently for node hash computation
- `File.ContentHash` correctly set to `sha256(file_contents)` instead of FileHash
- MCP `handleOwnership` uses NodesByName grouping instead of nonexistent "contains" edges
- Cross-repo resolver: module path vs filesystem path mismatch in repo URL resolution
- Enrichment: removed broken per-edge upgrade path (declaration position != call-site position)
- File walker: skip `.claude` and `testdata` directories to prevent 3x node inflation
- Enrichment: open all files via `textDocument/didOpen` before cross-package LSP queries

### Changed

- Default indexing switched from go/packages (16 min) to tree-sitter + LSP (9 seconds)
- Daemon uses GitWatcher (commit-driven) instead of FileWatcher (filesystem-event-driven)
- MCP server expanded from 11 to 16 tools
- IndexRepo records edge events and cleans up stale nodes/edges before re-extraction

## 2026-05-14

### Added

- Separate roadmap document (`docs/roadmap.md`) with parallel workstreams and dependency constraints
- Storage interface (`GraphStore`) for backend swappability
- Three-tier traversal cache design (L1 LRU, L2 materialized closures, L3 bounded traversal)
- Runtime trace ingestion architecture design
- Semantic PR diff design
- `TraceIngestor` interface for normalizing observability data into graph edges
- `SemanticDiffResult`, `BlastRadiusDelta`, `OwnershipDelta` types for PR impact analysis

### Changed

- Removed "v0" hedging language; architecture treats full system as the target
- README roadmap slimmed to summary table linking to full roadmap doc

## 2026-05-13

### Added

- Content-addressed architecture document (`docs/architecture.md`) with 11 foundational design decisions
- Merkle DAG graph model: node hashes, edge hashes, snapshot root hashes
- Symbol identity scheme (`{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`)
- Append-only edge log with event sourcing
- Edge provenance model with confidence tiers
- Content-addressed file identity for rename survival
- Causal ordering via Lamport timestamps
- Schema migration framework (embedded numbered SQL migrations)
- Deterministic reindexing rules
- SQLite storage decision with full schema
- Daemon process model with MCP transport (stdio and HTTP)
- Brand assets: banner PNG and social preview JPG

## 2026-05-12

### Added

- Initial README: problem statement, core idea, cross-boundary edge types
- Positioning, roadmap, and comparison sections
