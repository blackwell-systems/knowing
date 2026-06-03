# CLI Commands

## Repository Management

### `knowing add`

Registers a repository in the global roster and assigns it a per-repo database path.

**Usage:** `knowing add [-url <url>] [<repo-path>]`

**Flags:**
- `--url`: Repository URL (auto-detected from git remote if empty)

If `<repo-path>` is omitted, defaults to the current directory.

### `knowing remove`

Removes a repository from the roster and evicts all associated data (nodes, edges, files, snapshots, feedback, task_memory, graph_notes). Also available as the `untrack_repo` MCP tool.

**Usage:** `knowing remove [-purge] <repo-path-or-url>`

**Flags:**
- `--purge`: Also delete the per-repo database file

**Implementation:** `internal/store/evict.go`, `internal/mcp/untrack.go`

### `knowing list`

Lists all tracked repositories with their paths, URLs, database sizes, and status.

### `knowing reset`

Deletes all graph data for a repository (nodes, edges, snapshots) without removing the database file. Run `knowing index` afterward to re-index.

**Flags:**
- `--db` (default: roster lookup or `~/.knowing/knowing.db`): database path

### `knowing vacuum`

Compacts the database file after deletions. Reports size before, after, and space saved.

**Flags:**
- `--db` (default: roster lookup or `~/.knowing/knowing.db`): database path

### `knowing init`

Full "get started in one command" experience. Indexes the repo, enriches with LSP, generates CLAUDE.md, and configures the MCP server for Claude Code.

**Usage:** `knowing init [flags] [<repo-path>]`

**Flags:**
- `--db`: database path (default: roster default)
- `--url`: repository URL (default: auto-detect from git remote)
- `--skip-mcp`: skip Claude Code MCP configuration
- `--skip-enrich`: skip LSP enrichment

**Steps:**
1. Detect git root and repo URL
2. Index the repository (tree-sitter, Go only in init)
3. Detect and run LSP enrichment
4. Generate CLAUDE.md (idempotent, marker-based)
5. Configure Claude Code MCP server in `~/.claude.json`
6. Print summary

## Indexing

### `knowing index`

One-shot indexing of a repository with parallel extraction.

**Usage:** `knowing index [flags] <repo-path>`

**Flags:**
- `--db` (default: roster lookup or `~/.knowing/knowing.db`): database path
- `--url` (default: auto-detect from go.mod or absolute path): repository URL for node identity
- `--commit` (default: HEAD, resolved to actual SHA): commit hash to record in snapshot
- `--full`: use go/packages instead of tree-sitter (slower, guaranteed type resolution)
- `--workers` (default: 8): number of parallel extraction goroutines
- `--skip-blame`: skip git blame authorship pass (faster, no authored_by edges)
- `--no-enrich`: skip LSP enrichment after indexing (faster, edges stay at 0.7 confidence)

**Phases (parallel extraction):**
1. **Walk:** enumerate source files, compute content hashes, skip unchanged
2. **Parallel extract:** fan-out to worker goroutine pool; each worker creates a per-call tree-sitter parser (thread-safe, no shared mutable state)
3. **Batch store:** single SQLite transaction for all nodes, edges, and file records
4. **Authorship:** `git blame` stamps `last_author`/`last_commit_at` (skippable via `--skip-blame`)
5. **Snapshot:** hierarchical Merkle tree computation (repo -> package -> edge-type -> leaf)

**Progress output:** writes to stderr every 2 seconds with files processed and extraction rate. On the knowing codebase (84K LOC, 429 source files, 62 packages), produces 7,224 nodes and ~24.9K edges in 1.8 seconds (1,451 files/sec throughput).

**Extractors registered (22):** Go (tree-sitter or full), Python, TypeScript/JavaScript, Rust, Java, Ruby, C#, Terraform HCL, SQL, Kubernetes YAML, Cloud infrastructure (CloudFormation, Docker Compose, GitHub Actions, Serverless), CSS/SCSS, Protocol Buffers, Event/MQ patterns (Kafka, NATS, SQS, RabbitMQ), OpenAPI/JSON Schema, Dockerfile, GraphQL, .env, Makefile, Helm charts, GitLab CI, package.json.

### `knowing reindex`

Clears all nodes, edges, and edge events, then re-indexes the repository from scratch. Solves stale data and duplicate prefix issues. Runs LSP enrichment after indexing unless `--full` is used.

**Usage:** `knowing reindex [flags] <repo-path>`

**Flags:**
- `--db`: database path
- `--url`: repository URL
- `--commit`: commit hash to record
- `--full`: use go/packages extractor

### `knowing enrich`

Runs offline enrichment passes that stamp per-symbol metadata from external data sources. Unlike Tier 2 LSP enrichment (which resolves edges), these passes add node-level metadata columns.

**Usage:** `knowing enrich {blame|coverage} [flags] <repo-path>`

**Subcommands:**

- **blame**: runs `git blame --porcelain` on every file with graph nodes, then stamps `last_author` and `last_commit_at` on each symbol. Enables ownership queries and recency scoring at the symbol level.
  - Flags: `--db`, `--url`

- **coverage**: parses a Go cover profile (`go test -coverprofile`) and stamps `coverage_pct` on each symbol. Computes the ratio of covered to total statements for overlapping coverage blocks. Symbols with no coverage data receive -1.
  - Flags: `--db`, `--url`, `--profile` (default: `cover.out`)

- **lsp**: runs LSP enrichment standalone on an existing DB. Upgrades call edges and discovers new edges via language servers.
  - Flags: `--db`, `--url`

- **resolver**: runs in-process resolvers retroactively on an existing DB. Adds `resolver_resolved` edges (confidence 0.9) for 7 languages without external LSP.
  - Flags: `--db`

All enrichment passes are idempotent: re-running overwrites previous values.

### `knowing ingest-scip`

Imports a SCIP index file (produced by a SCIP-compatible indexer) into the knowledge graph. Creates nodes and edges for all symbols and references found in the index. Used for external dependencies that cannot be indexed directly.

**Usage:** `knowing ingest-scip -file <path> -repo <url> [flags]`

**Flags:**
- `--db`: database path
- `--file` (required): path to the `.scip` index file
- `--repo` (required): repository URL to associate

### `knowing watch`

Lightweight fsnotify file watcher that re-indexes on save. Does not start an MCP server or ingest traces (use `knowing serve` for that). Keeps the graph up to date during active development.

**Usage:** `knowing watch [flags] <repo-path>`

**Flags:**
- `--db`: database path
- `--url`: repository URL (auto-detected if empty)
- `--no-enrich`: skip LSP enrichment after reindex
- `--debounce` (default: 500): debounce interval in milliseconds

Watches all source directories, debounces file saves, calls `IndexRepo` for incremental re-extraction, and optionally runs scoped LSP enrichment in the background on changed files.

## Querying

### `knowing query`

Searches the knowledge graph for nodes matching a qualified name prefix and prints each node with its outgoing edges.

**Usage:** `knowing query [flags] <symbol-prefix>`

**Flags:**
- `--db`: database path

### `knowing context`

Generates graph-ranked context for a task description or set of changed files. Queries the knowledge graph, ranks symbols by blast radius, confidence, recency, and distance, then formats the output within a token budget.

**Usage:** `knowing context [flags]`

**Flags:**
- `--task`: task description for context generation
- `--files`: comma-separated list of changed file paths
- `--budget` (default: 50000): token budget
- `--format` (default: `xml`): output format (`gcf`/`gcb`/`json`/`xml`/`markdown`)
- `--db`: database path
- `--repo`: repository URL for file resolution

**Constraints:** either `--task` or `--files` must be specified, but not both. The `gcf`, `gcb`, and `json` formats use the wire protocol (content-addressed payloads with hash references). The `xml` and `markdown` formats produce human-readable output.

### `knowing test-scope`

Computes which tests are affected by a set of changed files by walking the call graph backward from changed symbols to find test functions that transitively call them.

**Usage:** `knowing test-scope [flags]`

**Flags:**
- `--db`: database path
- `--files`: comma-separated changed files (default: `git diff HEAD`)
- `--output` (default: `packages`): output mode (`packages`, `functions`, `run`)
- `--depth` (default: 3): maximum call-graph traversal depth

**Output modes:**
- `packages`: unique Go package paths (for `go test ./...`)
- `functions`: fully qualified test function names
- `run`: `-run` regex for `go test` (e.g., `^(TestFoo|TestBar)$`)

### `knowing why`

Explains why a symbol ranked where it did for a given task. Shows the full scoring breakdown: seed tier, RWR score, HITS authority, blast radius, confidence, recency, session boost, feedback weight.

**Usage:** `knowing why [<symbol>] -task "<description>" [flags]`

**Flags:**
- `--task` (required): task description
- `--symbol`: symbol name to explain (can also be provided as positional argument)
- `--db`: database path

The underlying implementation is `ExplainSymbol` in `internal/context/explain.go`. It executes the same 4-channel RRF seed fusion, HITS reranking, and knapsack packing as `ForTask`, but instead of returning the packed context block, it returns the per-component score breakdown for the queried symbol.

### `knowing diff`

Computes the semantic diff between two snapshots. Shows nodes added, removed, and modified, plus edges added and removed.

**Usage:** `knowing diff [flags] <old-ref> <new-ref>`

**Flags:**
- `--db`: database path
- `--format` (default: `text`): output format (`text` or `json`)

**Snapshot refs:** `@latest`, `@prev`, `@first`, `@N` (Nth from latest, 0-indexed), or a full 64-char hex hash.

### `knowing export`

Exports the knowledge graph in JSON or Graphviz DOT format.

**Flags:**
- `--db`: database path
- `--format` (default: `json`): output format (`json` or `dot`)
- `--repo`: filter nodes and edges to a single repository
- `--snapshot`: record the snapshot in metadata
- `--algorithm` (default: `louvain`): community detection algorithm (`louvain`, `louvain-fine`, `label-propagation`)

The JSON export contains four top-level fields: `nodes` (with hash, qualified name, kind, line, signature, community ID, last_author, coverage_pct, doc), `edges` (with hash, source, target, type, confidence, provenance, cross_community flag), `communities` (detected clusters with ID, label, and size), and `metadata` (with repo, snapshot, export timestamp, node/edge/community counts).

The DOT export renders the graph with community subgraphs as cluster subgraphs. Nodes are shaped by kind (box for functions, ellipse for types, hexagon for services). Cross-community edges are colored red to highlight architectural boundaries.

### `knowing stats`

Shows cumulative graph statistics and feedback metrics: repos, nodes, edges, files, snapshots, communities, graph notes, and feedback breakdown (total, useful, not useful, unique symbols, merkleized, usefulness rate).

**Flags:**
- `--db`: database path
- `--json`: output as JSON (single-line)

### `knowing stale`

Detects files changed since the last snapshot and reports stale nodes. Intended as a CI gate: exits with code 1 when stale files are found, code 0 when the graph is fresh.

**Usage:** `knowing stale [flags]`

**Flags:**
- `--db` (default: roster lookup or `~/.knowing/knowing.db`): database path
- `--repo`: repository path (default: current directory)

**How it works:**
1. Runs `git diff` against the commit recorded in the latest snapshot
2. Passes changed file paths to the `StaleNodesByFiles` store method
3. Reports counts of stale nodes per file
4. Exits with code 1 if any stale files are found, code 0 otherwise

**Implementation:** `cmd/knowing/stale.go`, `internal/store/sqlite.go` (`StaleNodesByFiles` method).

**Examples:**

```bash
# Check if the graph is stale relative to HEAD
knowing stale

# Check a specific repo
knowing stale --repo ./my-service

# Use in CI (fails the step if stale)
knowing stale || knowing index .
```

## Integrity and Proofs

### `knowing fsck`

Verifies graph integrity: referential checks, hash recomputation, and snapshot chain continuity. Read-only (never modifies data). Uses the global roster to classify dangling edges as cross-repo vs truly dangling.

**Usage:** `knowing fsck [flags]`

**Flags:**
- `--db`: database path
- `--quick`: run PRAGMA integrity_check only (fast, no graph checks)
- `--repo`: repository URL to verify (default: all repos)

### `knowing prove`

Generates a Merkle proof that a specific edge exists in the current snapshot. The proof is a JSON object that can be verified offline with `knowing verify`.

**Usage:** `knowing prove -source <symbol> -target <symbol> [-type calls] [flags]`

**Flags:**
- `--db`: database path
- `--source` (required): qualified name of the source symbol
- `--target` (required): qualified name of the target symbol
- `--type` (default: `calls`): edge type (calls, imports, implements, etc.)
- `--repo`: repository URL (default: auto-detect from current directory)
- `-o`: write proof to file instead of stdout
- `--human`: human-readable output instead of JSON

### `knowing prove-absent`

Proves that a relationship does NOT exist in the current snapshot. The proof shows the two adjacent leaves that bracket where the missing edge would be, proving there is no room for it. Cryptographically verifiable.

**Usage:** `knowing prove-absent -source <symbol> -target <symbol> [-type calls] [flags]`

**Flags:**
- `--db`: database path
- `--source` (required): qualified name of the source symbol
- `--target` (required): qualified name of the target symbol
- `--type` (default: `calls`): edge type
- `--repo`: repository URL (default: auto-detect)
- `-o`: write proof to file instead of stdout
- `--human`: human-readable output instead of JSON

### `knowing verify`

Verifies a Merkle proof offline (no database or network needed). Checks the proof cryptographically against the claimed root hash.

**Usage:** `knowing verify -proof <file>` or `knowing verify <file>`

**Flags:**
- `--proof`: path to proof JSON file (or `-` for stdin); can also be provided as positional argument

### `knowing audit`

Generates a structured compliance report for the current snapshot. Includes: integrity check, edge summary, cross-package edges, and optionally Merkle proofs for every cross-package relationship.

**Usage:** `knowing audit [-proofs] [-o report.json] [flags]`

**Flags:**
- `--db`: database path
- `--repo`: repository URL (default: auto-detect)
- `-o`: write report to file (default: stdout)
- `--proofs`: include Merkle proofs for all cross-package edges
- `--max-edges` (default: 500): maximum cross-package edges to include

### `knowing audit-diff`

Compares two audit point snapshots and produces a structured change report with classification (behavioral/structural/runtime_drift/metadata_only), added/removed edge type summaries.

**Usage:** `knowing audit-diff <old-snapshot> <new-snapshot> [flags]`

**Flags:**
- `--db`: database path
- `--repo`: repository URL (default: auto-detect)
- `-o`: write report to file (default: stdout)

**Snapshot refs:** same as `knowing diff` (`@latest`, `@prev`, `@N`, hex hash).

## Server

### `knowing daemon`

Manages the daemon lifecycle: start, stop, status, and restart.

**Usage:**
- `knowing daemon start [--detach]`
- `knowing daemon stop`
- `knowing daemon status`
- `knowing daemon restart`

**Subcommands:**
- `start`: Launch the daemon. With `--detach`, forks to background.
- `stop`: Send SIGTERM to the running daemon (reads PID from `~/.knowing/daemon.pid`).
- `status`: Print whether the daemon is running, its PID, and uptime.
- `restart`: Stop then start the daemon.

**Flags (start):**
- `--detach`: Run in background (daemonize)
- `--db`: database path
- `--addr` (default: `:8080`): HTTP address for MCP server

**PID file:** `~/.knowing/daemon.pid`

**Implementation:** `cmd/knowing/daemon.go`, `internal/daemon/pidfile.go`

### `knowing mcp`

Runs the MCP server over stdio. This is the mode used by AI agents via `.mcp.json` or `~/.claude.json` configuration. Opens the database and serves MCP tool calls until stdin closes or SIGINT/SIGTERM.

**Usage:** `knowing mcp [flags]`

**Flags:**
- `--db`: database path
- `--watch`: watch repo for file changes and re-index on save
- `--repo`: repository path to watch (required with `--watch`, defaults to cwd)
- `--url`: repository URL (auto-detected if empty)
- `--no-enrich`: skip LSP enrichment after reindex (only with `--watch`)
- `--no-feedback`: disable implicit feedback (noise demotion)
- `--embeddings`: enable embedding gap-fill (off by default)
- `--debounce` (default: 500): debounce interval in ms (only with `--watch`)

With `--watch`, combines MCP serving and live graph updates in one process.

### `knowing serve`

Starts the full daemon: opens the database, creates the indexer, launches the MCP server over HTTP, watches repos for file changes, and runs background enrichment. Blocks until SIGINT/SIGTERM.

**Usage:** `knowing serve [flags] [repo-paths...]`

**Flags:**
- `--db` (default: `~/.knowing/knowing.db`): database path
- `--addr` (default: `:8080`): HTTP address for MCP server
- `--trace`: enable runtime trace ingestion (OTLP gRPC)
- `--trace-endpoint` (default: `localhost:4317`): OTLP gRPC endpoint
- `--trace-batch-size` (default: 1000): number of spans per batch

## Debugging and Diagnostics

### `knowing debug-seeds`

Shows the full seed selection pipeline for a task: keywords, BM25, path boost, ForTask top 10.

**Usage:** `knowing debug-seeds -task "description" [-db path] [repo-path]`

### `knowing debug-fts`

Runs a raw FTS5 query against the node index.

**Usage:** `knowing debug-fts -query "term1 OR term2" [-db path] [-limit N]`

### `knowing debug-walk`

Shows the RWR walk from a specific seed node: edge types, nodes reached, score distribution.

**Usage:** `knowing debug-walk -seed "SymbolName" [-db path] [-top N] [-alpha 0.2]`

### `knowing debug-vocab`

Shows learned vocabulary associations from agent usage. With `-task`, previews
which keywords pass the noise filter without needing a database.

**Usage:** `knowing debug-vocab [-db path] [-keyword filter] [-min-count N] [-top N] [-task "description"]`

### `knowing debug-rwr-cache`

Tests RWR cache behavior: runs ForTask cold (populates cache), then warm (tests
cache hit). Reports latency speedup, cache hit/miss stats, result correctness,
and cache storage size.

**Usage:** `knowing debug-rwr-cache -task "description" [-db path] [-stats]`

### `knowing debug-feedback`

Shows implicit feedback records per symbol with cluster breakdown.

**Usage:** `knowing debug-feedback [-db path] [-symbol name] [-min-count N] [-top N]`

### `knowing debug-equiv`

Shows which equivalence classes match a task description from all sources.

**Usage:** `knowing debug-equiv -task "description" [-db path] [repo-path]`

### `knowing debug-pack`

Shows packing decisions: density ranking, token cost, proximity, budget utilization.

**Usage:** `knowing debug-pack -task "description" [-db path] [-budget N] [repo-path]`

### `knowing bench-task`

Runs a single benchmark task with P@10 and per-symbol HIT/MISS analysis.

**Usage:** `knowing bench-task -task <task-id> [-corpus path] [-budget N]`

## Other

### `knowing version`

Prints version, commit hash, and build date (set by goreleaser ldflags at build time).
