# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
