# Eval Framework: Retrieval Accuracy

**Auto-generated. Do not edit manually.**

## Methodology

Standardized evaluation of knowing's context engine across tiered task fixtures.
Each fixture defines a development task and hand-curated ground-truth symbols.
The engine returns ranked results; we measure Precision@10, Recall@10, and MRR.

**Tiers:**
- **Easy:** Single-package tasks (all relevant symbols in one package)
- **Medium:** Cross-package tasks (symbols span 2-3 packages)
- **Hard:** Cross-repo or multi-system tasks (runtime, daemon, resolver involved)

## Results

| Task | P@10 | R@10 | MRR | Tier |
|------|------|------|-----|------|
| Add a new MCP tool that returns symbol documentation | 80.0% | 160.0% | 1.00 | easy |
| Add a method to SQLiteStore that queries nodes by communi... | 40.0% | 66.7% | 1.00 | easy |
| Add a new language extractor for Ruby files | 0.0% | 0.0% | 0.00 | easy |
| Add a new wire format codec for MessagePack encoding | 10.0% | 20.0% | 0.10 | easy |
| Implement garbage collection for old snapshots beyond ret... | 30.0% | 75.0% | 1.00 | easy |
| Add full-text search to the store layer for finding nodes... | 10.0% | 25.0% | 0.33 | easy |
| Register a new language extractor with the indexer | 30.0% | 60.0% | 1.00 | easy |
| Create a new snapshot of the current graph state | 0.0% | 0.0% | 0.00 | easy |
| Fix keyword extraction logic in the context engine | 20.0% | 66.7% | 0.20 | easy |
| Encode a graph payload using the wire format codec | 20.0% | 50.0% | 0.50 | easy |
| Initialize and start the knowing daemon process | 10.0% | 25.0% | 0.11 | easy |
| Query edges coming into and out of a node in the store | 30.0% | 75.0% | 0.12 | easy |
| Expose community detection results through the MCP server | 0.0% | 0.0% | 0.00 | easy |
| Batch insert multiple nodes into the SQLite store | 50.0% | 100.0% | 1.00 | easy |
| Compute a semantic diff between two versions of a file | 20.0% | 100.0% | 0.50 | easy |
| Pack ranked symbols into a token budget for the context w... | 20.0% | 50.0% | 0.17 | easy |
| Resolve dangling cross-repo symbol references | 10.0% | 50.0% | 1.00 | easy |
| Search for similar symbols using vector embeddings | 40.0% | 100.0% | 0.50 | easy |
| Find all transitive callers of a function in the graph store | 0.0% | 0.0% | 0.00 | easy |
| Enrich graph edges with LSP type information | 60.0% | 200.0% | 1.00 | easy |
| Resolve dangling cross-repo edges by matching module path... | 10.0% | 16.7% | 0.50 | medium |
| Combine vector similarity search with graph-based ranking... | 10.0% | 16.7% | 0.14 | medium |
| Link symbols that reference external packages to their de... | 0.0% | 0.0% | 0.00 | medium |
| Determine which tests need to run based on which files ch... | 0.0% | 0.0% | 0.00 | medium |
| Build a vector index over all graph nodes so they can be ... | 20.0% | 33.3% | 0.50 | medium |
| After indexing, enrich extracted edges with type informat... | 50.0% | 83.3% | 1.00 | medium |
| Given a set of file paths, retrieve relevant symbols usin... | 0.0% | 0.0% | 0.00 | medium |
| Handle an index-repo request from the MCP protocol, runni... | 40.0% | 66.7% | 0.50 | medium |
| Score runtime trace edges with confidence values based on... | 20.0% | 33.3% | 0.25 | medium |
| Analyze a pull request to show which symbols are affected... | 20.0% | 33.3% | 0.20 | medium |
| Track which agent session produced each graph payload for... | 0.0% | 0.0% | 0.00 | medium |
| Compare two graph snapshots and report which nodes and ed... | 30.0% | 50.0% | 0.17 | medium |
| Watch for git commits and trigger re-indexing of changed ... | 10.0% | 16.7% | 0.17 | medium |
| Use random walk with restart over the graph store edges t... | 20.0% | 28.6% | 0.33 | medium |
| Expose blast radius analysis through the MCP protocol so ... | 60.0% | 100.0% | 1.00 | medium |
| Implement HITS hub/authority reranking in the context eng... | 10.0% | 12.5% | 0.14 | medium |
| Find affected tests by tracing the call graph backward fr... | 50.0% | 71.4% | 0.33 | medium |
| Wire feedback scoring into the context engine so that pre... | 0.0% | 0.0% | 0.00 | medium |
| Compute a semantic diff between two graph snapshots showi... | 40.0% | 66.7% | 1.00 | medium |
| Index a Go repository and persist the extracted graph int... | 20.0% | 28.6% | 0.50 | medium |
| For a pull request review, diff the changed files semanti... | 20.0% | 25.0% | 0.50 | hard |
| Keep the code graph fresh by watching for file changes, r... | 40.0% | 50.0% | 1.00 | hard |
| Bootstrap the system from scratch: parse a Go codebase wi... | 10.0% | 12.5% | 0.25 | hard |
| When an agent reports that retrieved context was unhelpfu... | 20.0% | 25.0% | 0.33 | hard |
| Index multiple repositories, resolve cross-repo symbol re... | 10.0% | 12.5% | 0.33 | hard |
| Detect changed files from git, clean up stale nodes and e... | 10.0% | 11.1% | 0.25 | hard |
| Detect when the stored graph has drifted from the actual ... | 10.0% | 12.5% | 0.50 | hard |
| Before an agent renames a function, check all downstream ... | 0.0% | 0.0% | 0.00 | hard |
| Ingest OpenTelemetry spans and create runtime edges with ... | 10.0% | 12.5% | 0.33 | hard |
| Implement a hybrid retrieval strategy that combines BM25 ... | 10.0% | 12.5% | 1.00 | hard |
| Ingest production telemetry traces, resolve span names to... | 0.0% | 0.0% | 0.00 | hard |
| Build an end-to-end pipeline where an AI coding agent ask... | 10.0% | 12.5% | 0.20 | hard |
| Generate full PR impact analysis: semantic diff of change... | 20.0% | 22.2% | 0.33 | hard |
| Compute blast radius for a symbol change across multiple ... | 20.0% | 25.0% | 0.20 | hard |
| Given a task description, seed the graph walk from keywor... | 30.0% | 30.0% | 0.50 | hard |

## Per-Tier Summary

| Tier | Precision@10 | Recall@10 | MRR | Fixtures |
|------|-------------|-----------|-----|----------|
| easy | 24.0% | 61.2% | 0.48 | 20 |
| medium | 20.5% | 32.9% | 0.34 | 20 |
| hard | 14.7% | 17.6% | 0.38 | 15 |
| **Overall** | **20.2%** | **39.0%** | **0.40** | **55** |

## Reproducibility

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Indexes the knowing repo into a temp DB and evaluates all fixtures.
Add new fixtures to `eval/fixtures/{easy,medium,hard}/` in YAML format.
