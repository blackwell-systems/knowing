# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- `knowing mcp` subcommand for stdio MCP server mode (used by AI agents via .mcp.json)
- HITS (Hyperlink-Induced Topic Search) reranking on RWR subgraph: boosts task-relevant authorities, penalizes generic infrastructure hubs. Score differentiation improved from 0.01 spread to 0.35 spread across results.
- Random Walk with Restart (RWR) algorithm for graph-based relevance scoring in context engine
- Improved keyword extraction with stop word filtering, CamelCase splitting, and abbreviation expansion
- Relative normalization in ranking and base recency score for static-only edges

### Fixed

- `test-scope` command: fixed `symbolsInFiles` returning empty results due to stale FileHash mismatch after re-indexing
- `test-scope` command: fixed package path extraction producing invalid `go test` paths (was not stripping module prefix)
- Context engine `ForFiles` and `ForPR` now use `NodesByFilePath` join (was broken with stale FileHash matching)
- HITS node selection now operates on top-N by RWR score (was random map iteration order)
- Context engine uses substring search for keyword matching (was requiring exact match)
- mkdocs.yml and index.md added for docs workflow
- Architecture doc updated to reflect actual codebase structure

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
