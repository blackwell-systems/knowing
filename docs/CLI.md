# knowing CLI Reference

Command-line interface for the knowing knowledge graph: index repositories,
query symbols, export graphs, compute semantic diffs, and run the daemon.

## Installation

See [DISTRIBUTION.md](DISTRIBUTION.md) for installation instructions.

## Usage

```
knowing <subcommand> [flags]
```

Subcommands: `serve`, `index`, `query`, `export`, `diff`, `context`, `why`, `mcp`, `watch`, `reindex`, `init`, `test-scope`, `ingest-scip`, `version`.

## Environment

**`KNOWING_DB`**: Set this environment variable to specify the default database
path for all subcommands. When set, the `-db` flag defaults to this value
instead of `knowing.db`.

```bash
export KNOWING_DB=/var/lib/knowing/data.db
knowing index ./repo   # uses /var/lib/knowing/data.db automatically
```

Set `GOWORK=off` when building or running from source to avoid workspace
interference:

```
GOWORK=off go build ./cmd/knowing
```

## Quick Start

Index a repository, query it, and export the graph:

```bash
# 1. Index a local repo (uses tree-sitter fast path, then runs LSP enrichment)
knowing index -url github.com/org/repo ./path/to/repo

# 2. Query for a symbol by name prefix
knowing query "MyService"

# 3. Export the full graph as JSON
knowing export > graph.json

# 4. Export filtered to a single repo
knowing export -repo github.com/org/repo > repo-graph.json
```

For continuous indexing with file watching, use the daemon:

```bash
knowing serve ./path/to/repo
```

## Subcommands

### serve

Start the daemon with MCP server, file watching, and background reindexing.

```
knowing serve [flags] [repo-path ...]
```

The daemon watches the specified repository directories for changes, triggers
reindexing on new commits, runs LSP enrichment in the background, and exposes
an MCP server over HTTP.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-addr` | string | `:8080` | HTTP address for the MCP server |
| `-trace` | bool | `false` | Enable runtime trace ingestion |
| `-trace-endpoint` | string | `localhost:4317` | OTLP gRPC endpoint for trace ingestion |
| `-trace-batch-size` | int | `1000` | Number of spans per batch |

Positional arguments after flags are treated as repository paths to watch.

**Examples:**

```bash
# Start daemon watching one repo on the default port
knowing serve ./my-repo

# Custom database and address, watching two repos
knowing serve -db /var/lib/knowing/data.db -addr :9090 ./repo-a ./repo-b

# Enable trace ingestion from a custom OTLP endpoint
knowing serve -trace -trace-endpoint collector.local:4317 ./my-repo
```

**Notes:**

- The daemon blocks until it receives SIGINT or SIGTERM.
- Registers all 13 language extractors (Go, Python, TypeScript/JS, Rust, Java,
  C#, Terraform, SQL, Kubernetes YAML, Cloud YAML [CloudFormation/SAM, Docker
  Compose, GitHub Actions, Serverless Framework], CSS, Protocol Buffers).
- The MCP server exposes an `index_repo` tool that triggers indexing through
  the same pipeline as file-watch events.

---

### index

One-shot indexing of a repository.

```
knowing index [flags] <repo-path>
```

Parses the repository using tree-sitter (fast path) and stores nodes and edges
in the SQLite database. After indexing, runs LSP enrichment automatically for
all detected language servers (gopls, pyright, typescript-language-server,
rust-analyzer, jdtls, OmniSharp) unless `-full` is specified.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-url` | string | *(repo-path)* | Repository URL (e.g. `github.com/org/repo`). Defaults to the repo path if omitted. |
| `-commit` | string | `HEAD` | Commit hash to record. Resolves to the actual git HEAD if set to `HEAD`. |
| `-full` | bool | `false` | Use full type resolution via `go/packages` instead of fast tree-sitter extraction |

The first positional argument is required and specifies the local path to the
repository.

**Examples:**

```bash
# Index using fast tree-sitter path (default) with automatic LSP enrichment
knowing index -url github.com/org/repo ./repo

# Index with full type resolution (skips LSP enrichment since edges are
# already high-confidence)
knowing index -full -url github.com/org/repo ./repo

# Index into a specific database at a specific commit
knowing index -db /tmp/test.db -commit abc123def456 ./repo

# Minimal invocation (URL defaults to repo path, commit defaults to HEAD)
knowing index ./repo
```

**Notes:**

- Registers all 13 language extractors (Go, Python, TypeScript/JS, Rust, Java,
  C#, Terraform, SQL, Kubernetes YAML, Cloud YAML [CloudFormation/SAM, Docker
  Compose, GitHub Actions, Serverless Framework], CSS, Protocol Buffers).
- When `-full` is used, the Go packages extractor provides full type
  resolution; LSP enrichment is skipped because the extractor already produces
  high-confidence edges.
- Prints a summary after indexing: repo hash, snapshot hash, node count, and
  edge count.
- If `-commit` is `HEAD` (the default), the tool resolves it to the actual git
  HEAD commit hash of the repository.

---

### query

Search the knowledge graph by symbol name prefix.

```
knowing query [flags] <symbol-prefix>
```

Finds all nodes whose qualified name starts with the given prefix and prints
each node with its outgoing edges.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |

The first positional argument is required and specifies the symbol name prefix
to search for.

**Examples:**

```bash
# Find all symbols starting with "Server"
knowing query "Server"

# Query a specific database
knowing query -db /var/lib/knowing/data.db "http.Handler"
```

**Notes:**

- Output format is one line per node: `QualifiedName (Kind) [hash]`, followed
  by indented lines for each outgoing edge: `-> target_hash [edge_type]`.
- Prints "No nodes found." if no matches exist.

---

### export

Export the knowledge graph as JSON to stdout.

```
knowing export [flags]
```

Collects all nodes and their outgoing edges, then writes a JSON document
containing nodes, edges, and metadata.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-format` | string | `json` | Output format: `json` (with community annotations) or `dot` (Graphviz with Louvain subgraphs) |
| `-repo` | string | *(empty)* | Filter by repository URL. When set, only nodes belonging to files in that repo are included. |
| `-snapshot` | string | *(empty)* | Filter by snapshot hash (recorded in metadata; does not currently filter nodes) |

**Examples:**

```bash
# Export the entire graph as JSON (with community annotations)
knowing export > graph.json

# Export filtered to one repo
knowing export -repo github.com/org/repo > repo.json

# Export as Graphviz DOT with Louvain community subgraphs
knowing export -format dot > graph.dot

# Export from a non-default database
knowing export -db /tmp/test.db -repo github.com/org/repo
```

**Notes:**

- JSON output is pretty-printed with two-space indentation.
- The JSON structure contains four top-level keys: `nodes`, `edges`,
  `communities`, and `metadata`.
- Each node includes: `node_hash`, `qualified_name`, `kind`, `line`,
  `signature`, `community` (Louvain community ID, or -1 for ungrouped).
- Each edge includes: `edge_hash`, `source_hash`, `target_hash`, `edge_type`,
  `confidence`, `provenance`, `cross_community` (true if the edge spans
  community boundaries).
- Communities includes: `id`, `label` (dominant package name), `size`.
- Metadata includes: `repo`, `snapshot`, `exported_at` (RFC 3339 timestamp),
  `node_count`, `edge_count`, `community_count`.
- DOT format renders community clusters as Graphviz `subgraph cluster_N` blocks
  with cross-community edges highlighted in red.

---

### diff

Compute a semantic diff between two snapshots.

```
knowing diff [flags] <old-snapshot-hash> <new-snapshot-hash>
```

Compares two snapshots by hash and reports added, removed, and modified nodes
and edges.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-format` | string | `text` | Output format: `text` or `json` |

Both positional arguments are required. Snapshot hashes must be 64-character
hex strings (32 bytes).

**Examples:**

```bash
# Text diff between two snapshots
knowing diff abc123...old abc123...new

# JSON diff for programmatic consumption
knowing diff -format json abc123...old abc123...new

# Using a specific database
knowing diff -db /var/lib/knowing/data.db abc123...old abc123...new
```

**Notes:**

- Text output shows sections for nodes added (`+`), removed (`-`), and
  modified (`~`), followed by edges added and removed, and a summary line.
- Modified nodes include counts of edges added and removed for each node.
- JSON output is pretty-printed with two-space indentation.
- Snapshot hashes are displayed as shortened 8-character prefixes in text
  output headers.

---

### context

Generate graph-aware context for a task or set of changed files.

```
knowing context [flags]
```

Queries the knowledge graph, ranks symbols by structural importance (blast
radius, confidence, recency, graph distance), and formats the output within a
token budget. Use `-task` for task-based context or `-files` for file-based
blast radius context. Exactly one of the two must be specified.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-task` | string | *(empty)* | Task description for context generation |
| `-files` | string | *(empty)* | Comma-separated list of changed file paths |
| `-budget` | int | `50000` | Token budget |
| `-format` | string | `xml` | Output format: `xml`, `markdown`, `json`, `gcf`, `gcb`, or `toon` |
| `-repo` | string | *(empty)* | Repository URL for file resolution (used with `-files`) |

**Examples:**

```bash
# Generate context for a task description
knowing context -task "refactor auth middleware" -budget 50000

# Generate blast radius context for changed files
knowing context -files "internal/auth/handler.go,internal/auth/middleware.go" -repo github.com/org/repo

# Output as JSON with a smaller budget
knowing context -task "add caching to user lookup" -budget 20000 -format json
```

**Notes:**

- Specify either `-task` or `-files`, not both.
- Output is written to stdout. Pipe or redirect as needed.
- The token budget controls how much context is included. The engine ranks
  symbols by relevance and packs them greedily until the budget is exhausted.

---

### why

Explain why a symbol ranked where it did in retrieval results.

```
knowing why [flags] [symbol]
```

Shows the full scoring breakdown for a symbol in the context of a given task:
seed channel, seed tier, RWR score, HITS authority/hub, blast radius, confidence,
recency, distance, feedback weight, session boost, and equivalence class matches.
Useful for debugging ranking behavior and understanding why a symbol was (or was
not) surfaced by the context engine.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-task` | string | *(required)* | Task description for context generation |
| `-symbol` | string | *(required, or positional)* | Symbol name to explain |

The symbol can be provided as the first positional argument or via the `-symbol`
flag. The `-task` flag is always required.

**Examples:**

```bash
# Explain why a symbol ranked where it did for a task
knowing why -task "refactor auth" -symbol "SessionHandler"

# Positional symbol argument
knowing why SessionHandler -task "refactor auth"

# Using a specific database
knowing why -db /var/lib/knowing/data.db -task "add caching" -symbol "CacheStore"
```

**Example output:**

```
knowing why: ...internal/context.RankSymbols
  Kind: function
  Rank: #5 of 65 symbols
  Total score: 0.6384

  Discovery:
    Seed: yes (direct keyword match)
    Channel: tiered
    Tier: path
    Keywords: retrieval, context, ranking

  Score breakdown:
    Blast radius:  0.1934  (caller proxy 21, max 38)
    Confidence:    0.1000
    Recency:       0.0450
    Distance:      0.1500  (distance=0)
    Feedback:      +0.1500

  Graph signals:
    RWR score:     0.2173
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- Runs the full retrieval pipeline internally (keyword extraction, seed selection,
  RWR, HITS, scoring) and then isolates the requested symbol's breakdown.
- If the symbol is not found in the result set, reports that it was filtered out
  or did not match any seeds.

---

### mcp

Run the MCP server over stdio.

```
knowing mcp [flags]
```

This is the mode used by AI agents via `.mcp.json` configuration. Opens the
database and serves MCP tool calls over stdin/stdout until the input stream
closes or SIGINT/SIGTERM is received. All 23 MCP tools are available.

When `--watch` is enabled, the MCP server also watches the repository for file
changes and re-indexes automatically on save. This combines the MCP server with
the file watcher in a single process, so agents always query up-to-date graph
data without needing a separate `knowing watch` or `knowing serve` process.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-watch` | bool | `false` | Watch repo for file changes and re-index on save |
| `-repo` | string | *(cwd)* | Repository path to watch (used with `-watch`, defaults to current working directory) |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |
| `-no-enrich` | bool | `false` | Skip LSP enrichment after reindex (only with `-watch`) |
| `-debounce` | int | `500` | Debounce interval in milliseconds (only with `-watch`) |

**Examples:**

```bash
# Start the stdio MCP server (used by agent tooling, not invoked directly)
knowing mcp -db knowing.db

# Start with file watching enabled (re-indexes on save)
knowing mcp --watch -db knowing.db

# Watch a specific repo path with custom debounce
knowing mcp --watch -repo ./my-repo -db knowing.db -debounce 1000
```

**`.mcp.json` configuration for Claude Code:**

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch", "-db", "/path/to/knowing.db"],
      "transport": "stdio"
    }
  }
}
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- The server blocks until stdin is closed or a signal is received.
- This subcommand replaces direct use of `knowing serve` for agent integrations
  that only need stdio MCP access without the HTTP server or file watcher.
- With `--watch`, the server monitors the repository for file changes and
  re-indexes changed files automatically. This keeps the graph fresh without
  requiring a separate watcher process.

---

### watch

Lightweight file watcher that re-indexes changed files on save.

```
knowing watch [flags] <repo-path>
```

Watches the specified repository directory for file changes using filesystem
events. When source files are modified, the watcher debounces changes and
re-indexes only the affected files. Optionally runs LSP enrichment after each
reindex cycle. This is a standalone watcher without an MCP server; use
`knowing mcp --watch` if you also need MCP tool access.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |
| `-no-enrich` | bool | `false` | Skip LSP enrichment after reindex |
| `-debounce` | int | `500` | Debounce interval in milliseconds |

The first positional argument is required and specifies the local path to the
repository to watch.

**Examples:**

```bash
# Watch a repo and re-index on file changes
knowing watch ./my-repo

# Watch with a specific database and no LSP enrichment
knowing watch -db /var/lib/knowing/data.db -no-enrich ./my-repo

# Custom debounce interval (1 second)
knowing watch -debounce 1000 ./my-repo
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- Blocks until SIGINT or SIGTERM is received.
- Only source files are monitored (.go, .ts, .py, .rs, .java, .cs, etc.).
- For MCP server with integrated file watching, use `knowing mcp --watch`
  instead.

---

### init

Generate a CLAUDE.md file with graph-derived project context.

```
knowing init [flags]
```

Queries the knowledge graph and produces a minimal orientation section for
CLAUDE.md containing symbol counts, package counts, and breadcrumbs pointing
agents to the most useful MCP tools. The operation is nondestructive and
idempotent: it uses markers to replace only the generated section, leaving any
hand-written content intact.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-output` | string | `CLAUDE.md` | Output file path |

**Examples:**

```bash
# Generate CLAUDE.md in the current directory (requires a pre-built database)
knowing init

# Generate into a specific file from a specific database
knowing init -db /var/lib/knowing/data.db -output .claude/CLAUDE.md
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- If no file exists at the output path, creates one with the generated section.
- If the file exists without markers, appends the generated section.
- If the file exists with markers (`<!-- knowing:generated:start -->` /
  `<!-- knowing:generated:end -->`), replaces only the section between markers.
- Never touches content outside the markers.

---

### reindex

Clear all graph data and re-index a repository from scratch.

```
knowing reindex [flags] <repo-path>
```

Removes all existing nodes, edges, and edge events from the database, then
performs a fresh index of the specified repository. Useful when extractor logic
has changed or when the graph has accumulated stale data that incremental
indexing cannot clean up.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-url` | string | *(repo-path)* | Repository URL |
| `-commit` | string | `HEAD` | Commit hash to record |
| `-full` | bool | `false` | Use full type resolution via `go/packages` |

**Examples:**

```bash
# Re-index from scratch using the fast tree-sitter path
knowing reindex ./my-repo

# Re-index with full type resolution
knowing reindex -full -url github.com/org/repo ./my-repo
```

**Notes:**

- Performs `TruncateGraph` before re-indexing; all prior data is lost.
- After indexing, runs LSP enrichment unless `-full` is specified.

---

### test-scope

Compute which tests are affected by a set of changed files.

```
knowing test-scope [flags]
```

Walks the call graph backward (BFS) from symbols defined in the changed files
to find test functions that transitively call them. Outputs affected test
packages, function names, or a `-run` regex for `go test`.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-files` | string | *(git diff HEAD)* | Comma-separated list of changed files. If omitted, detects changes via `git diff HEAD`. |
| `-output` | string | `packages` | Output mode: `packages`, `functions`, or `run` |
| `-depth` | int | `3` | Maximum call-graph traversal depth |

**Examples:**

```bash
# Detect affected tests from working tree changes (default)
knowing test-scope

# Explicit file list, output as -run regex
knowing test-scope -files "internal/store/sqlite.go,internal/mcp/server.go" -output run

# Output affected test function names
knowing test-scope -output functions

# Use a non-default database and traversal depth
knowing test-scope -db /tmp/test.db -depth 5
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- When `-files` is omitted, uses `git diff --name-only HEAD` to detect
  changes. Only source files (.go, .ts, .py, .rs, .java, .cs) are considered.
- The `packages` output mode extracts Go package paths from qualified names.
- The `run` output mode produces a regex suitable for `go test -run "^(TestA|TestB)$"`.
- Uses `NodesByFilePath` to resolve symbols in changed files, then BFS
  backward through `calls` edges to find test functions (nodes with
  "Test" prefix in their qualified name).

---

### ingest-scip

Import a SCIP index for external dependency symbols.

```
knowing ingest-scip [flags]
```

Reads a `.scip` protobuf file produced by a SCIP-compatible indexer (such as
`scip-go`, `scip-typescript`, or `scip-java`) and creates nodes and edges in
the knowledge graph for all symbols and references found in the index. This
enables knowing to resolve cross-repo references to external dependencies
without indexing their source code directly.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | `knowing.db` | Path to the SQLite database |
| `-file` | string | *(required)* | Path to the `.scip` index file |
| `-repo` | string | *(required)* | Repository URL to associate (e.g. `github.com/org/repo`) |

**Examples:**

```bash
# Ingest a SCIP index for an external dependency
knowing ingest-scip -file ./index.scip -repo github.com/org/library

# Ingest into a specific database
knowing ingest-scip -db /var/lib/knowing/data.db -file deps.scip -repo github.com/org/dep
```

**Notes:**

- Both `-file` and `-repo` are required.
- Produces nodes and edges with provenance `scip_resolved` (confidence 0.95).
- Useful for shallow indexing of external dependencies that you do not want to
  clone and fully index.
- Prints a summary after ingestion: nodes created, edges created, documents
  processed.

---

### version

Print version information.

```
knowing version
```

Prints the version string (currently `knowing v0.1.0`) and exits.

**Example:**

```bash
knowing version
```
