# Architecture Documentation

The full architecture specification for knowing, split into focused subdocuments. Each covers one area of the system.

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
11. **Merkle Tree Algorithms** (hierarchical roots, subgraph caching, incremental recompute)
12. **Git Design Audit** (gap analysis vs. git's reference implementation, actionable recommendations)

## Documents

| Document | What it covers |
|----------|---------------|
| [concepts.md](concepts.md) | Content-addressed storage, Merkle DAG, domain primitives (Node, Edge, Hash, Snapshot, Provenance), event sourcing, staleness, artifact boundary |
| [system-overview.md](system-overview.md) | Component map, language-agnostic graph model, two-tier extraction (tree-sitter + LSP), 25 extractors, multi-language auto-detection, edge type taxonomy |
| [data-flow.md](data-flow.md) | End-to-end trace of a single commit: git detection, indexing, snapshot computation (hierarchical Merkle), LSP enrichment |
| [concurrency.md](concurrency.md) | Daemon goroutine architecture, RWMutex coordination, channel buffer sizes, SQLite WAL mode |
| [runtime-traces.md](runtime-traces.md) | OTLP trace ingestion, span-to-edge mapping, confidence scoring, production observability edges |
| [context-engine.md](context-engine.md) | Retrieval pipeline: 4-channel seed fusion, RWR, HITS, BM25, knapsack packing, 84 equivalence classes |
| [wire-formats.md](wire-formats.md) | GCF (84% token savings), binary, JSON codec, TOON, format comprehension eval |
| [cli-commands.md](cli-commands.md) | All CLI commands: index, export (with --algorithm flag), watch, why, enrich (blame, coverage), mcp, init, fsck |
| [design-principles.md](design-principles.md) | Nine design goals, three architectural planes, MCP tool split, artifact boundary |
| [deep-dives.md](deep-dives.md) | 15 foundational architecture decisions with rationale and retrofit cost |
| [merkle-algorithms.md](merkle-algorithms.md) | 13 Merkle tree algorithms: hierarchical roots, subgraph caching, incremental recompute, context packs, proofs, federated sync, semantic change classification, bisection. Phase 1+2+3 shipped. |
| [merkle-proofs.md](merkle-proofs.md) | Merkle proof format, generation/verification, CLI (`knowing prove`/`knowing verify`), performance (72us generate, 1.2us verify), use cases (audit, CI gates, federated trust). |
| [adr-hierarchical-merkle.md](adr-hierarchical-merkle.md) | Architecture decision record: why the hierarchical Merkle tree changes knowing's identity from integrity mechanism to performance architecture. |
| [git-design-audit.md](git-design-audit.md) | Systematic audit of knowing's content-addressed design against git's reference implementation: 10 areas, 23 recommendations, severity-ranked. |
