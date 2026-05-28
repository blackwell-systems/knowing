# Architecture Documentation

The full architecture specification for knowing, split into focused subdocuments. Each covers one area of the system.

**New to knowing?** Start with the **[Introduction](../guide/introduction.md)**: builds understanding from zero with no assumed background. Covers the problem, content-addressing, hierarchical Merkle trees, proofs, and learning with worked examples.

## Reading Order

If you are new to knowing, read these in order:

1. **Concepts** (foundational vocabulary)
2. **System Overview** (component map, two-tier extraction, edge types)
3. **Extraction Pipeline** (tree-sitter, 23 extractors, post-processing)
4. **Enrichment Pipeline** (LSP enrichment, phantom nodes, gopls warmup)
5. **Retrieval Pipeline** (seeds, RWR, HITS, scoring, embedding re-ranker)
6. **Data Flow** (end-to-end trace of a single commit)
7. **Edge Types** (38 edge types with RWR weights)
8. **Embedding Re-ranker** (+17% P@10, vector cache, architecture)
9. **Context Engine** (ForTask/ForFiles/ForPR entry points, scoring formula)
10. **Wire Formats** (GCF, binary, JSON codec system)
11. **Design Principles** (goals, architectural planes, MCP tool split)
12. **Deep Dives** (15 foundational architecture decisions)
13. **Merkle Tree Algorithms** (hierarchical roots, subgraph caching)
14. **Git Design Audit** (gap analysis vs. git's reference implementation)

## Documents

| Document | What it covers |
|----------|---------------|
| [concepts.md](concepts.md) | Content-addressed storage, Merkle DAG, domain primitives (Node, Edge, Hash, Snapshot, Provenance), event sourcing, staleness, artifact boundary |
| [system-overview.md](system-overview.md) | Component map, language-agnostic graph model, two-tier extraction (tree-sitter + LSP), 26 extractor packages, parallel indexer (8 workers, 1.8s), multi-language auto-detection, edge type taxonomy |
| [data-flow.md](data-flow.md) | End-to-end trace of a single commit: git detection, indexing, snapshot computation (hierarchical Merkle), LSP enrichment |
| [concurrency.md](concurrency.md) | Daemon goroutine architecture, RWMutex coordination, channel buffer sizes, SQLite WAL mode |
| [runtime-traces.md](runtime-traces.md) | OTLP trace ingestion, span-to-edge mapping, confidence scoring, production observability edges |
| [context-engine.md](context-engine.md) | Retrieval pipeline: 5-channel seed fusion, RWR, HITS, BM25, knapsack packing, 115 equivalence classes, concept thesaurus |
| [wire-formats.md](wire-formats.md) | GCF (84% token savings), binary, JSON codec, TOON, format comprehension eval |
| [cli-commands.md](cli-commands.md) | All CLI commands: index, export (with --algorithm flag), watch, why, enrich (blame, coverage), mcp, init, fsck |
| [data-model.md](data-model.md) | SQLite schema, 17 migrations, identity vs metadata layers, cross-repo edges, Merkle tree storage, per-repo isolation, GraphStore interface, why SQLite. |
| [design-principles.md](design-principles.md) | Nine design goals, three architectural planes, MCP tool split, artifact boundary |
| [deep-dives.md](deep-dives.md) | 15 foundational architecture decisions with rationale and retrofit cost |
| [merkle-algorithms.md](merkle-algorithms.md) | 13 Merkle tree algorithms: hierarchical roots, subgraph caching, incremental recompute, context packs, proofs, federated sync, semantic change classification, bisection. Phase 1+2+3 shipped. |
| [merkle-proofs.md](merkle-proofs.md) | Merkle proof format, generation/verification, CLI (`knowing prove`/`knowing verify`), performance (72us generate, 1.2us verify), use cases (audit, CI gates, federated trust). |
| [adr-hierarchical-merkle.md](adr-hierarchical-merkle.md) | Architecture decision record: why the hierarchical Merkle tree changes knowing's identity from integrity mechanism to performance architecture. |
| [git-design-audit.md](git-design-audit.md) | Systematic audit of knowing's content-addressed design against git's reference implementation: 10 areas, 23 recommendations, severity-ranked. |
| [cross-repo.md](cross-repo.md) | Per-repo isolation model, content-addressed cross-repo identity, roster infrastructure, module mapping, phantom external nodes, limitations, and architectural proofs from the cross-repo fixture test. |
| [semantic-pr-diff.md](semantic-pr-diff.md) | Relationship-level PR diff: design, output format, implementation (`internal/diff/`), MCP tools (`snapshot_diff`, `semantic_diff`, `pr_impact`), CLI (`knowing audit-diff`), GitHub Actions workflow. |
| [extraction-pipeline.md](extraction-pipeline.md) | Tree-sitter extraction: 23 extractors, multi-dispatch, post-processing (9 steps), producer-consumer pipeline, content-addressed hashing, incremental indexing, CLI usage. |
| [enrichment-pipeline.md](enrichment-pipeline.md) | LSP enrichment: three phases (readiness, upgrade, discovery), phantom nodes, two-phase gopls warmup, multi-module Go support, per-symbol timeout, performance characteristics. |
| [retrieval-pipeline.md](retrieval-pipeline.md) | Full retrieval reference: keyword extraction, 5-channel RRF seed fusion, RWR (parameters, edge weights, adjacency cache), HITS, scoring formula, embedding re-ranker, budget packing, session/task memory. |
| [embedding-reranker.md](embedding-reranker.md) | Embedding re-ranker: +19% P@10, nomic-embed-text model (default), pure Go ONNX, SQLite vector cache (220ms), ReRankByHashes, gap-fill seeds. |
| [edge-types.md](edge-types.md) | Full catalog of 38 edge types with RWR weights, categories, and provenance. |
| [equivalence-classes.md](equivalence-classes.md) | Equivalence class system: 115 concepts across 4 layers (seed, universal, language-specific, graph-derived). |
| [context-packing.md](context-packing.md) | Context packing: density-ranked greedy knapsack, token estimation, persistent pack cache, staleness detection. |
| [hooks-integration.md](hooks-integration.md) | Git hooks integration: post-commit, post-checkout, pre-push hooks for daemon change detection. |
| [wire-formats-guide.md](wire-formats-guide.md) | Practical guide to wire format usage and integration. |
