# Implementation Log

Running log of how knowing was implemented using polywave parallel agent coordination.

## Pre-Implementation State (2026-05-14)

Architecture locked across 15 decisions in `docs/architecture.md`. No code yet. Roadmap defined as five parallel workstreams with explicit dependency constraints in `docs/roadmap.md`.

### Architecture decisions in place before first line of code:

1. Content-addressed graph (Merkle DAG)
2. Symbol identity scheme (`{repo}://{module_path}/{package_path}.{TypeName}.{MemberName}`)
3. Append-only edge log (event sourcing)
4. Edge provenance with confidence tiers
5. Content-addressed file identity
6. Causal ordering across repos (Lamport timestamps, git timestamps initially)
7. Schema migration framework (embedded numbered SQL migrations)
8. Deterministic reindexing (same input = same output, always)
9. Storage: SQLite ledger + Pebble acceleration index (ship SQLite, add Pebble when benchmarks justify)
10. Storage interface (`GraphStore` for backend swappability)
11. Process model (persistent daemon with MCP interface)
12. Content-addressed computation cache (derived results as shareable artifacts)
13. Runtime trace ingestion (OTel spans, HTTP logs as graph edges)
14. Semantic PR diff (relationship-level impact per PR)
15. Artifact-boundary plane separation (execution plane produces the graph, intelligence plane interprets it)

### Key interfaces drafted in architecture doc:

- `GraphStore` (all graph operations)
- `ComputationCache` (content-addressed derived results)
- `TraceIngestor` (runtime observability data → graph edges)
- `HybridStore` (SQLite ledger + Pebble acceleration routing)

### Dependency constraints for implementation ordering:

```
Graph Core ──────────────────────────────> all other workstreams
HTTP route edges (route-to-symbol map) ──> Runtime symbol resolution
Runtime symbol resolution ────────────────> Trace ingestion pipeline
Trace ingestion pipeline ─────────────────> Runtime edge creation
Edge provenance model ────────────────────> Confidence decay
Snapshot chain + SnapshotDiff ────────────> Semantic PR diff
Snapshot chain + SnapshotDiff ────────────> Temporal reasoning
Call graph + traversal ───────────────────> Graph-native test selection
Ownership edges ──────────────────────────> Ownership routing
MCP server ───────────────────────────────> Pending mutations
Snapshot chain + Merkle sync ─────────────> Federated graphs
```

---

## Bootstrap

*Pending: `/polywave bootstrap "knowing: content-addressed knowledge graph daemon in Go"`*
