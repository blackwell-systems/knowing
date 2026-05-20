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
| Add a new MCP tool that returns symbol documentation | 90.0% | 180.0% | 1.00 | easy |
| Add a method to SQLiteStore that queries nodes by communi... | 40.0% | 66.7% | 0.33 | easy |
| Add a new language extractor for Ruby files | 0.0% | 0.0% | 0.00 | easy |
| Add a new wire format codec for MessagePack encoding | 0.0% | 0.0% | 0.00 | easy |
| Implement garbage collection for old snapshots beyond ret... | 30.0% | 75.0% | 1.00 | easy |
| Add full-text search to the store layer for finding nodes... | 40.0% | 100.0% | 0.33 | easy |
| Register a new language extractor with the indexer | 30.0% | 60.0% | 1.00 | easy |
| Create a new snapshot of the current graph state | 10.0% | 25.0% | 0.33 | easy |
| Fix keyword extraction logic in the context engine | 20.0% | 66.7% | 0.50 | easy |
| Encode a graph payload using the wire format codec | 10.0% | 25.0% | 0.33 | easy |
| Initialize and start the knowing daemon process | 0.0% | 0.0% | 0.00 | easy |
| Query edges coming into and out of a node in the store | 40.0% | 100.0% | 0.25 | easy |
| Expose community detection results through the MCP server | 0.0% | 0.0% | 0.00 | easy |
| Batch insert multiple nodes into the SQLite store | 70.0% | 140.0% | 1.00 | easy |
| Compute a semantic diff between two versions of a file | 20.0% | 100.0% | 0.50 | easy |
| Pack ranked symbols into a token budget for the context w... | 40.0% | 100.0% | 0.50 | easy |
| Resolve dangling cross-repo symbol references | 20.0% | 100.0% | 1.00 | easy |
| Search for similar symbols using vector embeddings | 40.0% | 100.0% | 0.50 | easy |
| Find all transitive callers of a function in the graph store | 0.0% | 0.0% | 0.00 | easy |
| Enrich graph edges with LSP type information | 70.0% | 233.3% | 1.00 | easy |
| Resolve dangling cross-repo edges by matching module path... | 20.0% | 33.3% | 0.50 | medium |
| Combine vector similarity search with graph-based ranking... | 20.0% | 33.3% | 0.33 | medium |
| Link symbols that reference external packages to their de... | 10.0% | 16.7% | 0.10 | medium |
| Determine which tests need to run based on which files ch... | 0.0% | 0.0% | 0.00 | medium |
| Build a vector index over all graph nodes so they can be ... | 20.0% | 33.3% | 0.50 | medium |
| After indexing, enrich extracted edges with type informat... | 70.0% | 116.7% | 1.00 | medium |
| Given a set of file paths, retrieve relevant symbols usin... | 0.0% | 0.0% | 0.00 | medium |
| Handle an index-repo request from the MCP protocol, runni... | 90.0% | 150.0% | 1.00 | medium |
| Score runtime trace edges with confidence values based on... | 30.0% | 50.0% | 1.00 | medium |
| Analyze a pull request to show which symbols are affected... | 20.0% | 33.3% | 0.33 | medium |
| Track which agent session produced each graph payload for... | 0.0% | 0.0% | 0.00 | medium |
| Compare two graph snapshots and report which nodes and ed... | 50.0% | 83.3% | 0.25 | medium |
| Watch for git commits and trigger re-indexing of changed ... | 10.0% | 16.7% | 0.25 | medium |
| Use random walk with restart over the graph store edges t... | 20.0% | 28.6% | 0.33 | medium |
| Expose blast radius analysis through the MCP protocol so ... | 70.0% | 116.7% | 1.00 | medium |
| Implement HITS hub/authority reranking in the context eng... | 30.0% | 37.5% | 0.50 | medium |
| Find affected tests by tracing the call graph backward fr... | 40.0% | 57.1% | 0.50 | medium |
| Wire feedback scoring into the context engine so that pre... | 0.0% | 0.0% | 0.00 | medium |
| Compute a semantic diff between two graph snapshots showi... | 40.0% | 66.7% | 1.00 | medium |
| Index a Go repository and persist the extracted graph int... | 40.0% | 57.1% | 1.00 | medium |
| For a pull request review, diff the changed files semanti... | 20.0% | 25.0% | 0.50 | hard |
| Keep the code graph fresh by watching for file changes, r... | 40.0% | 50.0% | 1.00 | hard |
| Bootstrap the system from scratch: parse a Go codebase wi... | 10.0% | 12.5% | 0.33 | hard |
| When an agent reports that retrieved context was unhelpfu... | 30.0% | 37.5% | 0.33 | hard |
| Index multiple repositories, resolve cross-repo symbol re... | 20.0% | 25.0% | 0.50 | hard |
| Detect changed files from git, clean up stale nodes and e... | 10.0% | 11.1% | 0.25 | hard |
| Detect when the stored graph has drifted from the actual ... | 30.0% | 37.5% | 0.50 | hard |
| Before an agent renames a function, check all downstream ... | 0.0% | 0.0% | 0.00 | hard |
| Ingest OpenTelemetry spans and create runtime edges with ... | 20.0% | 25.0% | 0.33 | hard |
| Implement a hybrid retrieval strategy that combines BM25 ... | 20.0% | 25.0% | 0.50 | hard |
| Ingest production telemetry traces, resolve span names to... | 0.0% | 0.0% | 0.00 | hard |
| Build an end-to-end pipeline where an AI coding agent ask... | 10.0% | 12.5% | 0.17 | hard |
| Generate full PR impact analysis: semantic diff of change... | 30.0% | 33.3% | 0.33 | hard |
| Compute blast radius for a symbol change across multiple ... | 20.0% | 25.0% | 0.12 | hard |
| Given a task description, seed the graph walk from keywor... | 70.0% | 70.0% | 1.00 | hard |

## Per-Tier Summary

| Tier | Precision@10 | Recall@10 | MRR | Fixtures |
|------|-------------|-----------|-----|----------|
| easy | 28.5% | 73.6% | 0.48 | 20 |
| medium | 29.0% | 46.5% | 0.48 | 20 |
| hard | 22.0% | 26.0% | 0.39 | 15 |
| **Overall** | **26.9%** | **50.8%** | **0.46** | **55** |

## Reproducibility

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Indexes the knowing repo into a temp DB and evaluates all fixtures.
Add new fixtures to `eval/fixtures/{easy,medium,hard}/` in YAML format.
