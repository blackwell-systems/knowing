# Architecture Documentation

These subdocuments contain the full architecture specification for knowing. Each covers one area of the system. See [overview.md](overview.md) for the navigation hub and reading order.

## Documents

| Document | What it covers |
|----------|---------------|
| [concepts.md](concepts.md) | Foundational vocabulary: content addressing, Merkle DAG, domain primitives (Node, Edge, Hash, Snapshot, Provenance), artifact boundary |
| [system-overview.md](system-overview.md) | Component map, language-agnostic graph model, two-tier extraction (tree-sitter + LSP), 25 extractors, edge type taxonomy |
| [data-flow.md](data-flow.md) | End-to-end trace of a single commit through the pipeline: git detection, indexing, snapshot computation, enrichment |
| [concurrency.md](concurrency.md) | Daemon goroutine architecture, RWMutex coordination, channel buffer sizes, SQLite WAL mode |
| [runtime-traces.md](runtime-traces.md) | OTLP trace ingestion, span-to-edge mapping, production observability edges |
| [context-engine.md](context-engine.md) | Retrieval pipeline: seed tiers, RWR, HITS, BM25, knapsack packing, equivalence classes |
| [wire-formats.md](wire-formats.md) | GCF binary format, JSON codec, TOON, format comprehension eval results |
| [cli-commands.md](cli-commands.md) | All CLI commands: index, export, watch, why, enrich, mcp, init |
| [design-principles.md](design-principles.md) | Architectural goals, MCP tool design split, local-first philosophy |
| [deep-dives.md](deep-dives.md) | 15 foundational architecture decisions with rationale |
| [merkle-algorithms.md](merkle-algorithms.md) | 13 Merkle tree algorithms: hierarchical roots, subgraph caching, incremental recompute, context packs, proofs, federated sync, bisection |

## Reading Order

If new to knowing: concepts, system-overview, data-flow, context-engine, then anything relevant to your work.
