# Changelog

All notable changes to this project will be documented in this file.

## Unreleased

### Added

- System overview, component diagram, edge type taxonomy, and design goals in architecture doc
- Separated README into standard format (problem, usage, tools) with detailed docs in `docs/`
- Changelog

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
