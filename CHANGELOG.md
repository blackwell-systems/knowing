# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

#### Community-Aware Random Walk with Restart
- RWR walk now constrained to seed communities when candidates cluster in 1-3 communities
- `CommunityFilteredRWR`: BFS expansion skips nodes outside the allowed community set
- `buildAdjacencyMapFiltered`: community-filtered variant of the adjacency pre-load
- `CommunitiesForNodes` on SQLiteStore: batch lookup of community_id notes
- When seeds span 4+ communities (diverse query), falls back to unconstrained walk (backward compatible)
- Prevents RWR from drifting into unrelated packages on large repos
- Benchmark adapter now runs Louvain community detection on index (matching daemon behavior)

#### Cross-File Import Resolution (Java, C#)
- **Java**: `buildJavaImportMap` extracts `import com.pkg.Class` and `import static com.pkg.Class.method` declarations into a lookup map
- **C#**: `buildCSharpImportMap` extracts `using Namespace.Sub` and `using static Namespace.Class` directives
- Both resolve call targets through the import map when the object name matches an imported class (uppercase-first heuristic)
- Resolved edges get provenance `ast_resolved` with confidence 0.85 (up from `ast_inferred` / 0.7)
- Follows the established Rust pattern (`buildRustImportMap` / `resolveCallEdgeWithImports`)
- Wildcard imports (`import com.pkg.*`) correctly skipped (cannot resolve individual names)
- Completes cross-file import resolution for all 5 OOP languages: Python, TypeScript, Rust, Java, C#

### Fixed

#### Phantom External Nodes Dominating Retrieval Results
- External nodes (kind="external", `external://` prefix) from failed LSP enrichment entered results via RWR walk
- On repos with many phantom nodes (e.g., Spark Java: 2282 externals), they occupied all top-10 positions
- Fix: filter at two points: `filterNoisySymbols` (seed candidates) and RWR result loop (before scoring)
- Spark Java: P@10 0.00 -> 0.10 (was returning only phantom nodes, now finds real symbols)

### Changed

#### Compound-First Keyword Extraction (Language-Aware Tiered Search)
- Tiered search now queries compound identifiers (snake_case, CamelCase, dotted) before their split components
- New `KeywordSet` struct separates Exact (backtick-quoted), Compounds, and Components by specificity tier
- Backtick-quoted identifiers in task descriptions (e.g., `` `before_request` ``) are treated as highest-priority exact symbol names
- Components ("before", "request") only used as fallback when compounds yield < 5 results
- Eliminated code duplication: `ForTask` and `ExplainSymbol` now share a single `tieredSearchSet` method
- Fixed `bm25Search` in ExplainSymbol to use `buildFTSQuery` (compound-targeted) instead of naive OR join
- Flask P@10: 0.321 -> 0.329 (+0.8pp). Overall P@10: 0.230 (neutral, no regression)

### Added

#### Passive Task Memory Persistence (Session Compounding)
- MCP server records top-5 returned symbols in `task_memory` table after each `context_for_task` call
- Future queries with similar keywords recall stored symbols and boost them (0.5 + score * 0.4)
- Persists across process restarts via SQLite (migration 008 `task_memory` table)
- Fixed memory boost scoring: was producing negative boosts (score < 0.5 treated as penalty)
- Real-user impact: quality compounds over time as the system learns which symbols matter for which tasks
- Independent proof: `bench/feedback-loop/` shows +20pp precision after one feedback round

#### FTS Concepts Column (File-Name Derived Vocabulary Bridging)
- New `concepts` column in FTS index stores CamelCase-split tokens from file names and directories
- "src/compiler/commandLineParser.ts" -> concepts "compiler command Line Parser commandLineParser"
- BM25 weights: symbol_name=10x, concepts=5x, qualified_name=3x, signature=1x, file_path=1x
- Migration 017 adds concepts column and recreates FTS virtual table
- Bridges vocabulary gap where developers say "parser" but symbol is "parseOptionValue"

#### TypeScript extends_clause Fix
- Tree-sitter TypeScript nests `extends_clause` inside `class_heritage` (not direct child of class_declaration)
- Extractor now searches one level deeper for the heritage wrapper
- VS Code: 901 extends edges + 337 inheritance edges (was 0)
- P@10 0.226 -> 0.230 with VS Code inheritance propagation active

#### Deeper Call Chain Extraction (Python)
- Walk into call arguments to extract nested calls, callbacks, and lambda references
- Previously: `map(process, items)` only extracted the `map` call, missing `process` as a target
- Now: all identifier and call references inside arguments produce call edges
- Lambda bodies (`lambda: get_users()`) are walked for calls
- Nested function bodies walked with import resolution context (pyImports preserved)
- Flask: 5,022 -> 9,237 edges (+84%). Django: 151,431 -> 185,393 edges (+22%).

#### Cross-File Import Resolution (Python, TypeScript, Rust)
- **Python**: `buildPythonImportMap` extracts `import`/`from...import` statements, `resolveCallTarget` resolves call edges through the import map. 63 resolved cross-file edges on Flask.
- **TypeScript**: `buildTSImportMap` extracts `import`/`require` declarations, `resolveCallEdgeWithImports` resolves call targets through the map. 5,684 resolved cross-file edges on TypeScript compiler.
- **Rust**: `buildRustImportMap` extracts `use` declarations, `resolveCallEdgeWithImports` resolves `crate::`, `super::`, `self::` paths. 9,795 resolved cross-file edges on Cargo.
- Import resolution creates more edges for RWR to walk, improving recall on cross-file tasks.

#### Inheritance Propagation (language-agnostic)
- `propagateInheritance` post-processing pass finds all `extends` edges and creates `inherits` edges from child classes to parent class methods
- Enables RWR to walk from `Flask` -> `Scaffold.before_request` via inheritance chain
- Uses import-resolved qualified names to match extends edge targets to actual class node hashes
- 83 edges in Flask, 14,539 edges in Django (deep class hierarchies)
- Works on any language whose extractor produces `extends` edges and `method` nodes (Python, TypeScript, Java, C#, Rust)

#### Test File Deprioritization
- 0.3x score penalty for symbols from test files in ranking
- Detection by file path patterns (not symbol names): `/tests/`, `_test.go`, `.test.ts`, `.spec.ts`, `/__tests__/`
- Penalty removed when task description mentions testing (conditional, not absolute)
- Avoids false positives on production code with "test" in legitimate names

#### Failure Analysis Tool
- `bench/cross-system/cmd/failure-analysis/` diagnoses miss categories across all benchmark tasks
- Categories: noise (56%), test_symbol (36%), related_name (5%), same_package (2%)
- Key finding: bottleneck is RWR reach (graph connectivity), not ranking

### Fixed

#### FTS was never populated in CLI mode (critical)
- Background goroutine running `RebuildFTS` was killed on process exit before completing
- FTS index was always empty in `knowing index` (CLI) mode; only daemon kept it populated
- Fix: `RebuildFTS` now runs synchronously after snapshot computation
- FTS adds ~500ms to index time (acceptable for correct results)

#### FTS tokenizer: underscore now a token character
- `before_request` was tokenized as two tokens (`before`, `request`), preventing exact match
- Migration 016 updated: `tokenchars '_'` added to FTS5 tokenizer configuration
- Multi-word identifiers using snake_case now match as single tokens

### Changed

#### RRF channel weights equalized (tiered=2, BM25=2, equivalence=2)
- Was: tiered=3, BM25=1, equivalence=2
- Investigation showed BM25 and tiered find the same symbols in practice
- Equalizing weights removes artificial suppression of BM25 channel
- Cross-system benchmark: P@10 improved from 0.141 to 0.154 across Runs 7-10

#### P2 Edge Type Expansion (24 -> 30 edge types)
- `documents`: comment/docstring association with documented symbols
- `gated_by_flag`: feature flag references (LaunchDarkly, OpenFeature, custom `isEnabled` patterns)
- `consumes_endpoint`: HTTP client call sites in Go (`http.Get/Post/Do`) and TypeScript (`fetch/axios`)
- `implements_rpc`: gRPC service method implementations linked to proto definitions
- `consumes_rpc`: gRPC client call sites linked to proto service methods
- `deployed_by`: GitHub Actions workflow deploys linked to deployed services
- `tested_by`: GitHub Actions workflow test jobs linked to tested packages
- All 7 new types have RWR weights in `internal/edgetype` constants package
- Total: 30 edge types, 27 MCP tools

#### Indexer Performance Overhaul
- **Parallel extraction**: GOMAXPROCS workers with producer-consumer pipeline
- **Streaming commits**: batch of 500 files committed to SQLite immediately (kill-safe)
- **Single-pass body walk**: one recursive AST traversal dispatches calls/throws/routes/flags/endpoints (was 5 separate traversals)
- **Shared tree parsing**: tree-sitter parses once per file, all extractors share the result
- **Thread-safe extractors**: per-call parser creation (11 extractors fixed for parallel use)
- **In-memory snapshot**: `ComputeSnapshotFromEdges` builds Merkle tree from pipeline data (no DB re-read)
- **Synchronous FTS**: full-text search rebuilds synchronously after snapshot (~500ms)
- **Skip edge events on first index**: no parent = no diff to record (saves 268K INSERT ops)
- **Skip generated files**: checks first 512 bytes for `Code generated`/`DO NOT EDIT` markers
- **Skip non-source dirs**: `.git`, `vendor`, `node_modules`, `staging`, `third_party`, etc.
- **Per-file timeout**: 10s watchdog with fire-and-forget for stuck CGO calls
- **Progress output**: real-time `[N/total] X files/s, Y edges, ETA Zs` on stderr
- **`--skip-blame` flag**: skip git blame authorship extraction (expensive on large repos)
- **`--no-enrich` flag**: skip LSP enrichment for structural-only indexing
- **`--workers N` flag**: control extraction parallelism

#### Cross-System Benchmark Framework
- 100 tasks across 5 repos (kubernetes, VS Code, Django, Cargo, Flask)
- 5 difficulty levels: easy, medium, hard, cross-file, architectural
- Metrics: P@K, R@K, NDCG@10, MRR, token efficiency, latency
- Statistical rigor: Wilcoxon signed-rank, Cohen's d, bootstrap CI
- Adapter interface for pluggable retrieval systems (knowing, grep, future: gitnexus, aider)
- Symbol normalization for cross-system comparison
- Ground truth achievability filter (only count symbols present in DB)

#### Language Equivalence Classes
- 31 language-specific equivalence classes for improved keyword matching
- Python: `__init__`/constructor, `self`/`this`, `def`/`function`, Django/Flask patterns
- TypeScript: React hooks, Express/Fastify/Hono patterns, `interface`/`type`
- Rust: trait/impl, `Result`/`Option`, `unwrap`/`expect`
- Java: Spring annotations, `@Override`/`implements`
- Kubernetes: resource type aliases, `spec`/`template`/`containers`

#### FTS terminal symbol name column (retrieval quality)
- New `symbol_name` column in FTS index stores just the terminal identifier (e.g., `QuerySet.filter` instead of the full `github.com/django/django://django/db/models/query.py.QuerySet.filter`)
- BM25 weights: symbol_name=10x, qualified_name=3x, signature=1x, file_path=1x
- `extractSymbolName` strips repo URL, package path, and file extension prefix
- Eliminates path token dilution that buried relevant symbols in BM25 ranking
- Migration 016: adds `symbol_name` column, recreates FTS5 virtual table
- Expected impact: +5-10pp P@10 on non-Go repos where qualified names include file paths

#### Cross-system benchmark: all 5 repos indexed
- kubernetes: 4,877 files, 117,401 nodes, 268,249 edges (18.6s)
- VS Code: 38,260 files, 43,379 nodes, 93,382 edges (4.1s)
- Django: 2,937 files, 42,947 nodes, 185,393 edges (3.3s)
- Cargo: 979 files, 8,075 nodes, 79,305 edges (1.4s)
- Flask: 97 files, 1,658 nodes, 9,237 edges (0.1s)
- Total: 47,150 files in ~52s

### Fixed

#### Indexer: CGO timeout hang on large repos
- Tree-sitter CGO calls are not interruptible by Go context cancellation
- `context.WithTimeout` was ineffective: stuck CGO call blocks worker goroutine forever
- Pipeline deadlock: `extractWg.Wait()` never returns -> `close(resultCh)` never fires -> consumer loop hangs indefinitely
- Fix: watchdog goroutine pattern with timer select. Extraction runs in a fire-and-forget goroutine; 10s timer races against it. If timer wins, worker sends empty result and moves on.
- Result: kubernetes (4877 files, 268K edges) indexes in 18.6s. Was hanging indefinitely.

#### FTS + snapshot WAL contention
- Running FTS rebuild concurrently with snapshot computation caused both to stall
- Fix: sequential ordering (snapshot first, then FTS in background)

#### Test: mockSnapshotComputer parent chain behavior
- `TestIndexRepo_CleanupOnChange` was failing because mock always returned zero `ParentHash`
- Edge event recording condition (`snap.ParentHash != zero`) was never true in tests
- Fix: mock now tracks call count and returns proper parent chain on subsequent invocations

#### SQLite performance pragmas
- `synchronous=NORMAL`: safe with WAL, skips fsync per-commit (only on checkpoint)
- `mmap_size=256MB`: memory-mapped reads skip userspace buffer copy
- `cache_size=64MB`: larger page cache reduces disk I/O on warm workloads
- `busy_timeout=5000`: graceful retry on lock contention
- `temp_store=MEMORY`: temp indexes in RAM

#### Multi-row batch INSERT
- Edges: 100 rows per INSERT statement (was 1 row per exec)
- Nodes: 99 rows per INSERT statement
- Files: 249 rows per INSERT statement
- Reduces per-row SQL parsing overhead and CGO crossing count

### Changed
- Indexer architecture: sequential file loop replaced with producer-consumer pipeline
- Snapshot computation: from DB re-read to in-memory construction (9ms for knowing, 95ms for kubernetes)
- SQLite batch writes: single-row prepared statement loop replaced with multi-row VALUES
- Edge types: 24 -> 30 (7 new P2 types)
- MCP tools: 24 -> 27 (ownership_query + prove + prove_absent + fsck)

## 2026-05-19

#### MCP audit tools (27 tools total with ownership_query)
- `prove` MCP tool: generate inclusion proofs from agent conversations
- `prove_absent` MCP tool: generate absence proofs from agent conversations
- `fsck` MCP tool: verify graph integrity from agent conversations
- Enables agent-native compliance workflows without CLI

#### Database management
- `knowing reset`: delete all graph data (nodes, edges, snapshots) without removing DB file
- `knowing vacuum`: compact database after deletions (reports before/after size)
- `knowing remove --purge`: remove from roster AND delete the DB file
- `snapMgr` now initialized in plain MCP stdio mode (prove tools work without --watch)

#### Human-readable proof output
- `knowing prove -human` and `knowing prove-absent -human` for terminal-friendly output
- Clean format for screenshots and demos (default remains JSON)

#### Java extractor: proper package paths
- Qualified names now use Java package declaration (e.g., `org.springframework.samples.petclinic.owner.OwnerController`)
- Previously embedded absolute file paths; now extracts from `package_declaration` AST node
- Validated on Spring PetClinic (47 files, 5522 nodes, 3048 edges, 21 Spring routes)

#### Grafana scale validation
- Indexed Grafana (~500K LOC Go+TypeScript): 338K nodes, 714K edges, 15,921 files
- Hierarchical tree build: 88ms for 249K edges (3,552 packages)
- Context retrieval operational at 50x primary codebase scale

#### Named snapshot refs
- `knowing diff @latest @prev` (diff last two snapshots)
- `knowing diff @0 @3` (offset from most recent)
- `knowing audit-diff @prev @latest`
- Supports: `@latest`, `@first`, `@prev`, `@N` (offset), or raw hex hash
- Inspired by git's ref system (HEAD, HEAD~1)

### Changed

#### Merkle tree implementation extracted to `merkle-forest` library
- Internal `computeMerkleRoot` replaced by `github.com/blackwell-systems/merkle-forest` v0.1.1
- `BuildMerkleTree` delegates to `forest.Build` with `WithPrefix([]byte("merkle\x00"))` for hash parity
- `BuildHierarchicalTree` delegates to `forest.BuildMultiLevel`
- All exported API preserved unchanged (zero-breaking-change refactor)
- `combineHashes` retained for proof.go compatibility
- Net: -44 lines from knowing, delegated to standalone library
- Library: https://github.com/blackwell-systems/merkle-forest

### Added

#### `knowing stats` CLI
- Cumulative graph statistics: repos, nodes, edges, files, snapshots, communities, graph notes
- Feedback metrics: total, useful, not useful, unique symbols, merkleized count, usefulness rate
- Supports `-json` flag for structured output
- Supports `-db` flag for custom database path

#### Generation numbers on snapshots
- Schema migration 015: `generation INTEGER NOT NULL DEFAULT 0` on snapshots table
- `Snapshot.Generation` field: `parent.Generation + 1` on each new snapshot
- Enables O(1) ancestry checks without walking the chain
- Inspired by git's commit-graph `generation_number`

#### Auto-GC threshold
- After indexing, if `edge_events` table exceeds 5,000 rows, automatically prunes old snapshots (keeps 10)
- Inspired by git's `gc.auto` threshold (6,700 loose objects triggers gc)
- Prevents unbounded edge_events growth without manual intervention

#### Merkleized Feedback Validity (v0.5.0)
- Feedback records now store `neighborhood_root` (SubgraphRoot of symbol's package)
- Feedback automatically expires when code changes (neighborhood changes)
- 11% overhead (255µs baseline -> 284µs per 100 symbols)
- Schema migration 014: `neighborhood_root` column + index on feedback table
- `computeNeighborhoodRoot` helper in MCP server computes package root for a symbol
- `FeedbackBoosts` method accepts optional `neighborhoodRoots` map for merkleized expiration

#### Merkle Proofs and Audit Primitives
- `knowing prove`: generates cryptographic Merkle proofs (72µs, ~3KB)
- `knowing verify`: offline verification without database access (1.2µs)
- `knowing prove-absent`: absence proofs using adjacent sorted leaves
- `knowing audit`: compliance report with integrity check, edge inventory, and Merkle proofs
- Auto-substring matching in prove/prove-absent (no `%` prefix needed)
- Human-readable prove/verify output

#### Cross-Repo Resolution
- Phantom external nodes for stdlib/external edge targets
- Enricher creates phantom nodes for all dangling edges post-enrichment
- `ExtractPackagePath` handles method qualified names correctly
- Fsck roster awareness + cross-repo method resolution

### Changed

- Cross-repo edges now fully resolved via roster-based module mapping
- Tree depth locked at 3 levels (repo -> package -> edge-type)

### Added

#### Extractors (6 -> 17 languages)
- Protobuf/gRPC extractor: service, message, enum, RPC declarations with type reference edges
- Event/MQ extractor: Kafka, NATS, SQS, RabbitMQ patterns across Go/TS/Python/Java
- Schema extractor: OpenAPI 3.x, Swagger 2.x, JSON Schema document parsing
- Cloud extractor package: CloudFormation/SAM, Docker Compose, GitHub Actions, Serverless Framework
- Terraform HCL extractor: resources, data sources, modules, variables with dependency edges
- SQL extractor: tables, views, functions, procedures with FK/reference edges
- K8s YAML extractor: deployments, services, configmaps with label-selector edges
- CSS extractor: class/ID selectors, custom properties, var() dependency edges
- Python: Flask, FastAPI, Django route detection
- TypeScript: Fastify, Hono, NestJS, Next.js route detection
- FindAllExtractors multi-dispatch: all matching extractors run per file (not just first)
- All 25 extractors registered in CLI (includes 7 new infrastructure extractors: Dockerfile, Makefile, Helm, GitLab CI, package.json/npm, GraphQL, Ansible)

#### SCIP Ingest
- `internal/indexer/scipingest/` package: parses SCIP protobuf index files
- `knowing ingest-scip` CLI command for external dependency resolution
- Provenance `scip_resolved` at confidence 0.95

#### Context Engine
- HITS (Hyperlink-Induced Topic Search) reranking on RWR subgraph
- Density-ranked knapsack packing: score/cost ratio optimization for token budgets
- 5-tier seeding: exact, prefix, substring, file-path matching, interface-aware
- FeedbackProvider interface wired into ContextEngine with centered scoring
- Community-scoped RWR preparation (interface defined, activates when store implements)
- Random Walk with Restart (RWR) algorithm for graph-based relevance scoring
- Improved keyword extraction with stop word filtering, CamelCase splitting, abbreviation expansion
- Relative normalization in ranking and base recency score for static-only edges

#### MCP Server (16 -> 22 tools)
- `knowing mcp` subcommand for stdio MCP server mode
- `feedback` tool: record/query symbol usefulness for agent learning loop
- `test_scope` tool: backward BFS from changed symbols to affected test functions
- `flow_between` tool: BFS path finding between two symbols (up to 10 paths)
- `plan_turn` tool: keyword-based task-to-tool recommender with pre-filled arguments
- `communities` tool: Louvain modularity clustering with `list` and `for_symbol` actions
- `context_for_pr` tool (17th tool, added earlier in session)
- 3 MCP prompts: `refactor_safely`, `review_pr`, `investigate_dead_code`

#### Wire Format
- Graph Compact Format (GCF): line-oriented LLM-optimized encoding (84% token savings vs JSON)
- Graph Compact Binary (GCB): varint-encoded transport format (74% byte savings vs JSON)
- Session statefulness: cross-call deduplication (47% dedup on repeated symbols)
- Round-trip integrity: encode -> decode -> re-encode for all codecs

#### Benchmarks (6 harnesses with auto-generated FINDINGS.md)
- `bench/feedback-loop/`: precision 16% -> 36% (+20pp) with feedback compounding
- `bench/context-relevance/`: 3 configs x 10 fixtures, feedback adds +9pp precision
- `bench/token-savings/`: 52.8% fewer tool calls, 55.6% fewer tokens vs manual grep
- `bench/edge-accuracy/`: tree-sitter vs go/ast comparison (26.7% confirmation, 53.6% imports)
- `bench/test-scope-accuracy/`: predictions vs Go import DAG ground truth (98.9% precision)
- `bench/wire-format/`: GCF 84% token savings, GCB 74% byte savings across 6 fixtures

#### CLI
- `knowing test-scope`: find affected tests from changed files via call graph BFS
- `knowing init`: auto-generated CLAUDE.md with progressive disclosure
- `knowing export -format dot`: Graphviz DOT with Louvain community subgraphs
- `knowing reindex`: rebuild graph without full re-extraction
- Community-annotated JSON export: nodes include `community` ID, edges include `cross_community` flag

#### Infrastructure
- `KNOWING_DB` env var for global database path (all subcommands)
- Global MCP config support in ~/.claude.json (knowing available in every Claude session)
- Claude Code hooks with A/B measurement harness (proven net-positive after benchmarking)
- Docker image publishing in goreleaser config
- PyPI and npm distribution packages
- mcp-assert CI action for MCP server correctness testing
- `NodesByFilePath` store method (joins nodes to files via SQL)
- Migration 005: feedback table for persistent symbol usefulness tracking
- `DeleteSnapshot` for real garbage collection

### Fixed

- `test-scope` command: `symbolsInFiles` returning empty results (stale FileHash mismatch)
- `test-scope` command: package path extraction producing invalid `go test` paths
- Context engine `ForFiles`/`ForPR` broken with stale FileHash matching (now uses NodesByFilePath)
- HITS node selection on random map iteration order (now sorted by RWR score first)
- Context engine exact match requirement (now uses substring search)
- K8s extractor not matching `kubernetes-manifests/` directory names (was exact `/kubernetes/`)
- All subcommands now use KNOWING_DB env var (mcp.go was still hardcoded)
- 9 extractors were dead code (registered but never called due to first-match dispatch)
- Duplicate `extractPackage` helper in testscope.go and communities.go
- Community label deduplication (Louvain producing 3 "mcp" communities)
- Indexer cleans up nodes/edges from deleted files
- Duplicate nodes from mismatched repo URL vs go.mod module path
- Architecture doc updated to reflect actual codebase structure
- All 6 benchmark harnesses audited: stale FINDINGS data corrected, circular ground truth replaced with independent Go import DAG, missing FINDINGS.md generated, misleading interpretations rewritten

### Changed

- Extractors: 6 -> 17 languages (Go, Python, TS/JS, Rust, Java, C#, Terraform, SQL, K8s, CSS, Proto, Event/MQ, Schema, CloudFormation, Docker Compose, GitHub Actions, Serverless)
- MCP server: 16 -> 22 tools
- Wire format renamed from KWF/KWB to GCF/GCB (Graph Compact Format/Binary)
- Default hooks now recommended (proven net-positive with benchmarks)

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
