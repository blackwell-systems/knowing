# CLI Commands

## Export CLI

The `knowing export` subcommand exports the knowledge graph in JSON or Graphviz DOT format. The JSON export structure contains four top-level fields: `nodes` (with hash, qualified name, kind, line, signature, community ID), `edges` (with hash, source, target, type, confidence, provenance, cross_community flag), `communities` (Louvain-detected clusters with ID, label, and size), and `metadata` (with repo, snapshot, export timestamp, node/edge/community counts).

The DOT export renders the graph with Louvain community subgraphs as cluster subgraphs. Nodes are shaped by kind (box for functions, ellipse for types, hexagon for services). Cross-community edges are colored red to highlight architectural boundaries.

Filters:
- `--repo <url>`: filter nodes and edges to a single repository (by matching file hashes against repo files)
- `--snapshot <hash>`: record the snapshot in metadata (filtering by snapshot is informational)
- `--format json|dot`: output format (default: `json`). `json` includes community annotations; `dot` renders with Louvain subgraphs

## Watch CLI

The `knowing watch` subcommand (`cmd/knowing/watch.go`) runs an fsnotify file watcher on source directories. It debounces file saves (500ms default), calls `IndexRepo` for incremental re-extraction on each debounced event, and optionally runs scoped LSP enrichment in the background. This provides a lightweight alternative to the full daemon for developers who want continuous graph updates without running `knowing daemon`.

## Why CLI and ExplainSymbol

The `knowing why` subcommand (`cmd/knowing/why.go`) runs the full retrieval pipeline for a task description and returns a detailed scoring breakdown for a specific symbol. It answers: "why was (or wasn't) this symbol included in the context for this task?"

The underlying implementation is `ExplainSymbol` in `internal/context/explain.go`. It executes the same 4-channel RRF seed fusion, HITS reranking, and knapsack packing as `ForTask`, but instead of returning the packed context block, it returns the per-component score breakdown (blast radius, confidence, recency, distance, feedback, session boost, HITS authority/hub) for the queried symbol.

The MCP equivalent is the `explain_symbol` tool (#23) in `internal/mcp/context_handlers.go`, which accepts `task` and `symbol` parameters and returns the same scoring breakdown.

## Offline Enrichment Passes (`cmd/knowing/enrich.go`)

The `knowing enrich` subcommand runs offline enrichment passes that stamp per-symbol metadata from external data sources. Unlike Tier 2 LSP enrichment (which resolves edges), these passes add node-level metadata columns.

Two passes are available:

- **blame**: runs `git blame --porcelain` on every file with graph nodes, then stamps `last_author` and `last_commit_at` on each symbol (migration 009). Enables ownership queries and recency scoring at the symbol level.
- **coverage**: parses a Go cover profile (`go test -coverprofile`) and stamps `coverage_pct` on each symbol (migration 010). Computes the ratio of covered to total statements for overlapping coverage blocks. Symbols with no coverage data receive -1.

Both passes operate on an existing indexed database and are idempotent: re-running overwrites previous values. They are intentionally separate from the index pipeline so they can be run on demand (e.g., after CI produces a cover profile).
