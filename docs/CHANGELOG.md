# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- **`context_for_pr` MCP tool** (17th tool): RWR-scored context from changed files for PR review
- **HITS reranking** in context engine: hub/authority scoring on RWR subgraph for improved relevance ordering
- **Full hook suite** (5 hooks): SessionStart, PreEdit, PreCompact, PostTask, Subagent; complete agent lifecycle coverage
- **Hook benchmark** proving net-positive value (+305 tokens saved, 90% coverage)
- **`DeleteSnapshot`** implemented for real garbage collection
- **Snapshot lifecycle integration test** (end-to-end)
- **Route detection (Python):** Flask, FastAPI, and Django framework support in Python extractor
- **Route detection (TypeScript):** Fastify, Hono, NestJS, and Next.js support in TypeScript extractor (18 frameworks total)
- **Wire format system** (`internal/wire/` package): GCF (Graph Compact Format) text encoder/decoder, GCB (Graph Compact Binary) codec, JSON codec, pluggable registry, benchmark harness with 6 fixtures achieving 84% median token savings
- **GCF session statefulness:** cross-call symbol deduplication via `wire.Session`; previously-transmitted symbols emitted as bare references, delivering 47% additional savings on repeated symbols within a session
- **Wire format integration:** `format` parameter on `context_for_task` and `context_for_files` MCP tools; `--format gcf|gcb|json` on CLI `knowing context` command
- **Infrastructure schema extractors** (4 new languages, total 10): Terraform HCL, SQL, Kubernetes YAML, CSS; all using tree-sitter parsing with comprehensive test suites
- **`knowing mcp` subcommand** for stdio MCP server mode
- **`knowing reindex` subcommand** with pre-loaded RWR adjacency map
- **MCP prompts:** `refactor_safely`, `review_pr`, `investigate_dead_code`
- **Random Walk with Restart** (`internal/context/`): graph-based relevance scoring for context packing
- **Context engine improvements:** keyword extraction optimization with stop words, CamelCase splitting, abbreviation expansion; substring search for keyword matching; relative blast radius normalization; base recency 0.3 for static edges
- **CI:** mcp-assert action for MCP server correctness testing (0 lint issues)
- **Docs:** architecture.md (fixed drift against codebase), edge-types.md, context-packing.md, GCF.md, mkdocs.yml + index.md for docs workflow, deep dive on content addressing and Merkle DAG
- **Test coverage:** dedicated tests for RWR, diff, resolver, context, rustextractor, and mcp packages
- System overview, component diagram, edge type taxonomy, and design goals in architecture doc
- Separated README into standard format (problem, usage, tools) with detailed docs in `docs/`
- Changelog

### Fixed

- Tiered seed matching in context engine (exact > prefix > substring) produces differentiated scores
- Deleted file cleanup in indexer (nodes/edges from removed files are garbage collected)
- mcp-assert suite updated to new YAML format
- Context engine uses substring search for keyword matching
- Ranking uses relative normalization and base recency for static edges
- Resolved repo URL from go.mod module path to prevent duplicate nodes
- Added examples to MCP tool string parameters to resolve W103 lint warnings

### Changed

- Wire format renamed from KWF/KWB to GCF/GCB (Graph Compact Format / Graph Compact Binary) for standalone spec adoption
- Banner image metadata stripped
- `.knowing` binary added to .gitignore

## 2026-05-15

### Added

- **Bootstrap implementation:** 8 packages, 26 files, 56+ tests. Single Go binary.
  - `internal/types/`: Hash, Node, Edge, GraphStore, Extractor, ComputationCache interfaces
  - `internal/store/`: SQLite-backed GraphStore (20 methods, WAL mode, go:embed migrations, recursive CTEs)
  - `internal/snapshot/`: Merkle root computation, snapshot chain, diff, GC
  - `internal/indexer/`: Language-agnostic extractor framework, Go extractor (go/packages), tree-sitter Python extractor
  - `internal/mcp/`: MCP server with 11 tool handlers (stdio + HTTP transport)
  - `internal/daemon/`: Persistent daemon, fsnotify file watcher, debounce, RWMutex coordination
  - `cmd/knowing/`: CLI with serve, index, query, version subcommands
- **End-to-end indexing:** Index real Go repos, store graph, compute snapshots, query callers
  - Batch insert methods (BatchPutNodes, BatchPutEdges, BatchPutFiles)
  - Git HEAD resolution (reads .git/HEAD directly, no git dependency)
  - Functional MCP `index_repo` handler (not a stub)
  - Integration test with 9 assertions covering full pipeline
- **Cross-repo edge resolver:** `internal/resolver/` package
  - 4 new GraphStore methods: DanglingEdges, AllRepos, NodesByQualifiedName, DeleteEdge
  - Migration 002 for dangling edge support
  - `resolveTargetRepoURL` in GoExtractor uses go/packages Module info
  - `ModuleToRepoURL` map in ExtractOptions, populated from go.mod of indexed repos
  - 228 cross-repo edges between polywave-web and polywave-go
- **Indexing optimization (in progress):**
  - `BulkLoad` in goextractor/loader.go (single `packages.Load("./...")` call)
  - `ExtractWithPackage` method (accepts pre-loaded package, avoids per-file Load)
  - Worker pool in indexer/worker.go (`parallelExtract` with runtime.NumCPU workers)
- Deployment models documentation (`docs/deployment.md`)
- Implementation log (`docs/implementation-log.md`) with polywave wave-by-wave details

### Fixed

- `ComputeNodeHash` no longer includes contentHash in hash computation (was causing all cross-package caller queries to return empty results)
- GoExtractor uses `types.EmptyHash` consistently for node hash computation
- `File.ContentHash` correctly set to `sha256(file_contents)` instead of FileHash
- MCP `handleOwnership` uses NodesByName grouping instead of nonexistent "contains" edges

### Changed

- README expanded with vision statement, audience-segmented questions, broader positioning
- Architecture doc expanded to 15 decisions (added storage interface, computation cache, runtime trace ingestion, semantic PR diff, artifact-boundary plane separation, SQLite+Pebble hybrid model)
- GitHub repo description and topics updated to reflect broader vision
- Branch protection ruleset added (mirroring agent-lsp)

## 2026-05-14

### Added

- Separate roadmap document (`docs/roadmap.md`) with parallel workstreams, per-item dependencies, status tracking, and parallelization notes
- Storage interface (`GraphStore`) for backend swappability: SQLite first, adjacency-list backends later without changing callers
- Three-tier traversal cache design: L1 in-memory LRU, L2 materialized closures in SQLite, L3 bounded traversal with early termination; all invalidated by content-addressed hash comparison
- Runtime trace ingestion architecture: OpenTelemetry span ingest, gRPC trace metadata, HTTP access logs as graph edges with observation-based confidence scoring
- Semantic PR diff design: relationship-level impact diff on every PR via MCP tool and GitHub Action
- `TraceIngestor` interface for normalizing observability data into graph edges
- `SemanticDiffResult`, `BlastRadiusDelta`, `OwnershipDelta` types for PR impact analysis
- Roadmap restructured from sequential phases to parallel workstreams with explicit dependency constraints

### Changed

- Removed "v0" hedging language throughout; architecture treats the full system as the target
- README roadmap slimmed to summary table linking to full roadmap doc

## 2026-05-13

### Added

- Content-addressed architecture document (`docs/architecture.md`) with 11 foundational design decisions
- Merkle DAG graph model: node hashes, edge hashes, snapshot root hashes
- Symbol identity scheme (`{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`)
- Append-only edge log with event sourcing
- Edge provenance model with confidence tiers (ast_resolved, scip_imported, lsp_resolved, config_declared, text_matched, manual)
- Content-addressed file identity for rename survival and deduplication
- Causal ordering via Lamport timestamps (git timestamps as initial approximation)
- Schema migration framework (embedded numbered SQL migrations)
- Deterministic reindexing rules (no map iteration, no timestamps in hashes, no randomness)
- SQLite storage decision with full schema (repos, files, nodes, edges, edge_events, snapshots)
- Daemon process model with MCP transport (stdio and HTTP)
- "Why Content-Addressed?" section explaining CAS tradeoffs and precedent
- "Why Not Just Use Code Search?" positioning section
- "Relationship to agent-lsp" boundary definition
- Brand assets: banner PNG and social preview JPG

## 2026-05-12

### Added

- Initial README: problem statement, core idea, cross-boundary edge types
- Positioning, roadmap, and comparison sections
