# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

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
