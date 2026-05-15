# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- System overview, component diagram, edge type taxonomy, and design goals in architecture doc
- Separated README into standard format (problem, usage, tools) with detailed docs in `docs/`
- Changelog

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
