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
| Add a new MCP tool that returns symbol documentation | 10.0% | 20.0% | 0.33 | easy |
| Add a method to SQLiteStore that queries nodes by communi... | 50.0% | 83.3% | 1.00 | easy |
| Add a new language extractor for Ruby files | 10.0% | 16.7% | 0.12 | easy |
| Add a new wire format codec for MessagePack encoding | 40.0% | 80.0% | 1.00 | easy |
| Implement garbage collection for old snapshots beyond ret... | 50.0% | 125.0% | 1.00 | easy |
| Add full-text search to the store layer for finding nodes... | 0.0% | 0.0% | 0.00 | easy |
| Register a new language extractor with the indexer | 30.0% | 60.0% | 1.00 | easy |
| Create a new snapshot of the current graph state | 40.0% | 100.0% | 1.00 | easy |
| Fix keyword extraction logic in the context engine | 90.0% | 300.0% | 1.00 | easy |
| Encode a graph payload using the wire format codec | 40.0% | 100.0% | 1.00 | easy |
| Initialize and start the knowing daemon process | 30.0% | 75.0% | 0.33 | easy |
| Query edges coming into and out of a node in the store | 10.0% | 25.0% | 0.33 | easy |
| Expose community detection results through the MCP server | 20.0% | 50.0% | 0.50 | easy |
| Batch insert multiple nodes into the SQLite store | 40.0% | 80.0% | 1.00 | easy |
| Compute a semantic diff between two versions of a file | 40.0% | 200.0% | 1.00 | easy |
| Pack ranked symbols into a token budget for the context w... | 50.0% | 125.0% | 1.00 | easy |
| Resolve dangling cross-repo symbol references | 20.0% | 100.0% | 1.00 | easy |
| Search for similar symbols using vector embeddings | 40.0% | 100.0% | 1.00 | easy |
| Find all transitive callers of a function in the graph store | 40.0% | 80.0% | 1.00 | easy |
| Enrich graph edges with LSP type information | 10.0% | 33.3% | 0.17 | easy |
| Resolve dangling cross-repo edges by matching module path... | 40.0% | 66.7% | 1.00 | medium |
| Combine vector similarity search with graph-based ranking... | 0.0% | 0.0% | 0.00 | medium |
| Link symbols that reference external packages to their de... | 20.0% | 33.3% | 1.00 | medium |
| Determine which tests need to run based on which files ch... | 0.0% | 0.0% | 0.00 | medium |
| Build a vector index over all graph nodes so they can be ... | 10.0% | 16.7% | 0.12 | medium |
| After indexing, enrich extracted edges with type informat... | 0.0% | 0.0% | 0.00 | medium |
| Given a set of file paths, retrieve relevant symbols usin... | 20.0% | 33.3% | 0.50 | medium |
| Handle an index-repo request from the MCP protocol, runni... | 10.0% | 16.7% | 0.17 | medium |
| Score runtime trace edges with confidence values based on... | 30.0% | 50.0% | 1.00 | medium |
| Analyze a pull request to show which symbols are affected... | 10.0% | 16.7% | 0.50 | medium |
| Track which agent session produced each graph payload for... | 20.0% | 40.0% | 1.00 | medium |
| Compare two graph snapshots and report which nodes and ed... | 30.0% | 50.0% | 1.00 | medium |
| Watch for git commits and trigger re-indexing of changed ... | 80.0% | 133.3% | 1.00 | medium |
| Use random walk with restart over the graph store edges t... | 30.0% | 42.9% | 1.00 | medium |
| Expose blast radius analysis through the MCP protocol so ... | 70.0% | 116.7% | 1.00 | medium |
| Implement HITS hub/authority reranking in the context eng... | 20.0% | 25.0% | 1.00 | medium |
| Find affected tests by tracing the call graph backward fr... | 60.0% | 85.7% | 1.00 | medium |
| Wire feedback scoring into the context engine so that pre... | 80.0% | 114.3% | 1.00 | medium |
| Compute a semantic diff between two graph snapshots showi... | 60.0% | 100.0% | 1.00 | medium |
| Index a Go repository and persist the extracted graph int... | 40.0% | 57.1% | 1.00 | medium |
| For a pull request review, diff the changed files semanti... | 10.0% | 12.5% | 0.33 | hard |
| Keep the code graph fresh by watching for file changes, r... | 50.0% | 62.5% | 1.00 | hard |
| Bootstrap the system from scratch: parse a Go codebase wi... | 0.0% | 0.0% | 0.00 | hard |
| When an agent reports that retrieved context was unhelpfu... | 80.0% | 100.0% | 1.00 | hard |
| Index multiple repositories, resolve cross-repo symbol re... | 30.0% | 37.5% | 1.00 | hard |
| Detect changed files from git, clean up stale nodes and e... | 10.0% | 11.1% | 0.11 | hard |
| Detect when the stored graph has drifted from the actual ... | 40.0% | 50.0% | 1.00 | hard |
| Before an agent renames a function, check all downstream ... | 30.0% | 37.5% | 1.00 | hard |
| Ingest OpenTelemetry spans and create runtime edges with ... | 20.0% | 25.0% | 1.00 | hard |
| Implement a hybrid retrieval strategy that combines BM25 ... | 40.0% | 50.0% | 0.50 | hard |
| Ingest production telemetry traces, resolve span names to... | 30.0% | 37.5% | 1.00 | hard |
| Build an end-to-end pipeline where an AI coding agent ask... | 30.0% | 37.5% | 1.00 | hard |
| Generate full PR impact analysis: semantic diff of change... | 50.0% | 55.6% | 1.00 | hard |
| Compute blast radius for a symbol change across multiple ... | 40.0% | 50.0% | 0.33 | hard |
| Given a task description, seed the graph walk from keywor... | 10.0% | 10.0% | 0.14 | hard |

## Per-Tier Summary

| Tier | Precision@10 | Recall@10 | MRR | Fixtures |
|------|-------------|-----------|-----|----------|
| easy | 33.0% | 87.7% | 0.74 | 20 |
| medium | 31.5% | 49.9% | 0.71 | 20 |
| hard | 31.3% | 38.4% | 0.69 | 15 |
| **Overall** | **32.0%** | **60.5%** | **0.72** | **55** |

## Reproducibility

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Indexes the knowing repo into a temp DB and evaluates all fixtures.
Add new fixtures to `eval/fixtures/{easy,medium,hard}/` in YAML format.
