# knowing CLI Reference

Command-line interface for the knowing knowledge graph: index repositories,
query symbols, export graphs, compute semantic diffs, and run the daemon.

## Installation

See [distribution.md](distribution.md) for installation instructions.

## Usage

```
knowing <subcommand> [flags]
```

Subcommands: `serve`, `daemon`, `index`, `query`, `export`, `diff`, `context`, `why`, `mcp`, `watch`, `reindex`, `init`, `add`, `remove`, `list`, `stats`, `stale`, `reset`, `vacuum`, `test-scope`, `ingest-scip`, `enrich`, `fsck`, `prove`, `verify`, `prove-absent`, `audit`, `audit-diff`, `audit-supply-chain`, `version`.

## Environment

**`KNOWING_DB`**: Override the default database path for all subcommands. When
set, the `-db` flag defaults to this value instead of the per-repo database.

By default, `defaultDB()` checks the roster for the current directory and
returns the per-repo DB path (`~/.knowing/repos/<safe-name>.db`). If no roster
entry is found, it falls back to `~/.knowing/knowing.db`.

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

**For MCP/agent use (zero-config):** Add the MCP server config and start a
session. The server auto-indexes the git repository on first launch:

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

See [MCP Tools Reference](mcp-tools.md) for all 28 tools and configuration
details.

**For CLI use:** Index a repository, verify the index, query it:

```bash
# 1. Index a local repo (registers in roster, assigns per-repo database)
knowing add ./path/to/repo

# 2. Verify the index worked (check node/edge counts)
knowing stats

# 3. Query for a symbol by name prefix
knowing query "MyService"

# 4. Get context for a task
knowing context -task "refactor the auth handler" -format gcf

# 5. Export the full graph as JSON
knowing export > graph.json
```

If `knowing stats` shows 0 nodes or very few edges, or if context results
seem wrong, see [Troubleshooting](#troubleshooting) below.

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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
- Registers all 23 extractors (12 languages + 13 infrastructure/cloud
  + CODEOWNERS).
- The MCP server exposes an `index_repo` tool that triggers indexing through
  the same pipeline as file-watch events.

---

### daemon

Manage the knowing daemon lifecycle.

```
knowing daemon start [--detach]
knowing daemon stop
knowing daemon status
knowing daemon restart
```

Controls the background daemon process. The daemon provides continuous indexing,
file watching, and MCP serving. The PID file is stored at `~/.knowing/daemon.pid`.

**Subcommands:**

| Subcommand | Description |
|------------|-------------|
| `start` | Start the daemon. With `--detach`, runs in the background. |
| `stop` | Stop a running daemon (sends SIGTERM to the PID in `~/.knowing/daemon.pid`). |
| `status` | Check whether the daemon is running. Prints PID and uptime if active. |
| `restart` | Stop and restart the daemon (equivalent to `stop` followed by `start`). |

**Flags (start):**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--detach` | bool | `false` | Run the daemon in the background (daemonize) |
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-addr` | string | `:8080` | HTTP address for the MCP server |

**Examples:**

```bash
# Start the daemon in the foreground
knowing daemon start

# Start in the background (detached)
knowing daemon start --detach

# Check if the daemon is running
knowing daemon status

# Stop the daemon
knowing daemon stop

# Restart (stop + start)
knowing daemon restart
```

**Notes:**

- The PID file at `~/.knowing/daemon.pid` is created on start and removed on stop.
- `knowing daemon status` exits with code 0 if running, 1 if not.
- Implementation: `cmd/knowing/daemon.go`, `internal/daemon/pidfile.go`.

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-url` | string | *(repo-path)* | Repository URL (e.g. `github.com/org/repo`). Defaults to the repo path if omitted. |
| `-commit` | string | `HEAD` | Commit hash to record. Resolves to the actual git HEAD if set to `HEAD`. |
| `-full` | bool | `false` | Use full type resolution via `go/packages` instead of fast tree-sitter extraction |
| `-workers` | int | `8` | Number of parallel extraction goroutines |
| `-skip-blame` | bool | `false` | Skip git blame authorship extraction (faster structural-only index, no `authored_by` edges) |
| `-no-enrich` | bool | `false` | Skip LSP enrichment after extraction (structural-only indexing) |

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

# Fast structural-only index (no blame, no LSP enrichment)
knowing index --skip-blame --no-enrich ./repo

# Control parallelism (useful on resource-constrained machines)
knowing index --workers 4 ./repo
```

**Notes:**

- Uses a producer-consumer pipeline with parallel extraction (default 8 workers).
  Progress output to stderr shows files processed and extraction rate.
- Registers all 23 extractors (12 languages + 13 infrastructure/cloud +
  CODEOWNERS). Includes a 10-second per-file watchdog timeout for stuck CGO calls.
- When `-full` is used, the Go packages extractor provides full type
  resolution; LSP enrichment is skipped because the extractor already produces
  high-confidence edges.
- Prints a summary after indexing: repo hash, snapshot hash, node count, and
  edge count.
- If `-commit` is `HEAD` (the default), the tool resolves it to the actual git
  HEAD commit hash of the repository.
- With `--skip-blame`, the authorship phase (git blame) is skipped entirely.
  This is significantly faster on large repos (kubernetes: 18.6s with
  `--skip-blame --no-enrich`, vs minutes with full blame).

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-format` | string | `json` | Output format: `json` (with community annotations) or `dot` (Graphviz with Louvain subgraphs) |
| `-algorithm` | string | `louvain` | Community detection algorithm: `louvain`, `louvain-fine`, or `label-propagation` |
| `-repo` | string | *(empty)* | Filter by repository URL. When set, only nodes belonging to files in that repo are included. |
| `-snapshot` | string | *(empty)* | Filter by snapshot hash (recorded in metadata; does not currently filter nodes) |

**Examples:**

```bash
# Export the entire graph as JSON (with community annotations, default Louvain)
knowing export > graph.json

# Export filtered to one repo
knowing export -repo github.com/org/repo > repo.json

# Export as Graphviz DOT with Louvain community subgraphs
knowing export -format dot > graph.dot

# Export using label propagation community detection
knowing export -algorithm label-propagation > graph.json

# Export using fine-grained Louvain (higher resolution)
knowing export -algorithm louvain-fine > graph.json

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
knowing diff [flags] <old-ref> <new-ref>
```

Compares two snapshots and reports added, removed, and modified nodes and edges.

**Snapshot refs:** Both arguments accept named refs or raw hex hashes:

| Ref | Meaning |
|-----|---------|
| `@latest` | Most recent snapshot |
| `@prev` | Previous snapshot (alias for `@1`) |
| `@first` | Oldest snapshot |
| `@N` | Nth from most recent (0 = latest, 1 = prev, 2 = two back, ...) |
| `abc123...` | Raw 64-character hex hash |

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-format` | string | `text` | Output format: `text` or `json` |

**Examples:**

```bash
# Diff the last two snapshots
knowing diff @prev @latest

# Diff the oldest against the newest
knowing diff @first @latest

# Diff 3 snapshots back against latest
knowing diff @3 @latest

# JSON diff for programmatic consumption
knowing diff -format json @prev @latest

# Using raw hex hashes (still supported)
knowing diff abc123...old abc123...new
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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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

**Output formats:**

| Format | Best for | Description |
|--------|----------|-------------|
| `xml` | Human reading (default) | Structured XML with symbol names, scores, and source snippets |
| `gcf` | LLM/agent consumption | Compact line-oriented format, 84% fewer tokens than JSON |
| `json` | Programmatic use | Full metadata including scores, hashes, file paths |
| `markdown` | Documentation | Readable Markdown with code blocks |
| `gcb` | Service transport | Binary format for caching and inter-service use |
| `toon` | Debugging | Minimal cartoon format |

For agent integrations, `gcf` is recommended: it packs the most context into the
fewest tokens. For human debugging, use the default `xml` or `json` to see
scores and understand why symbols were included.

**Notes:**

- Specify either `-task` or `-files`, not both.
- Output is written to stdout. Pipe or redirect as needed.
- The token budget controls how much context is included. The engine ranks
  symbols by relevance and packs them greedily until the budget is exhausted.
- If results seem irrelevant, use `knowing why -task "..." -symbol "ExpectedSymbol"`
  to understand why a specific symbol ranked where it did. See
  [Troubleshooting](#troubleshooting) for more diagnostic steps.
- The quality of results depends heavily on the task description. Use specific
  symbol names (especially backtick-quoted identifiers) for best results.

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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
closes or SIGINT/SIGTERM is received. All 28 MCP tools are available.

When `--watch` is enabled, the MCP server also watches the repository for file
changes and re-indexes automatically on save. This combines the MCP server with
the file watcher in a single process, so agents always query up-to-date graph
data without needing a separate `knowing watch` or `knowing serve` process.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-watch` | bool | `false` | Watch repo for file changes and re-index on save |
| `-repo` | string | *(cwd)* | Repository path to watch (used with `-watch`, defaults to current working directory) |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |
| `-no-enrich` | bool | `false` | Skip LSP enrichment after reindex (only with `-watch`) |
| `-debounce` | int | `500` | Debounce interval in milliseconds (only with `-watch`) |
| `-embeddings` | bool | `false` | Enable local embedding gap-fill (off by default, confirmed neutral on cold-start benchmarks). |
| `-embed-model` | string | `nomic-code` | Embedding model: `nomic-code` (default), `jina-code`, `bge-small`. All models run locally. Only used with `-embeddings`. |

**Examples:**

```bash
# Start the stdio MCP server (DB auto-resolved from roster for cwd)
knowing mcp

# Start with file watching enabled (re-indexes on save)
knowing mcp --watch

# Enable embeddings (off by default, currently neutral on benchmarks)
knowing mcp --watch --embeddings

# Use a specific embedding model (requires --embeddings)
knowing mcp --watch --embeddings --embed-model jina-code

# Watch a specific repo path with custom debounce
knowing mcp --watch -repo ./my-repo -debounce 1000
```

**`.mcp.json` configuration for Claude Code:**

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch"],
      "transport": "stdio"
    }
  }
}
```

The `-db` flag is no longer needed in most cases. `defaultDB()` checks the
roster for the current directory and returns the per-repo DB path automatically.

**Notes:**

- **Zero-config:** If no database exists, the MCP server auto-detects the git
  repository, indexes it, and registers it in the roster. No manual `knowing
  index` or `knowing add` step is required.
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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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

Register the current repo in the global roster, index it, and generate a
CLAUDE.md file with graph-derived project context.

```
knowing init [flags]
```

Registers the repository in the roster, assigns it a per-repo database at
`~/.knowing/repos/<safe-name>.db`, indexes it, runs LSP enrichment, and produces
a minimal orientation section for CLAUDE.md containing symbol counts, package
counts, and breadcrumbs pointing agents to the most useful MCP tools. The
CLAUDE.md operation is nondestructive and idempotent: it uses markers to replace
only the generated section, leaving any hand-written content intact.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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

### add

Register a repository in the roster and index it into a per-repo database.

```
knowing add [flags] <repo-path>
```

Adds the repository at the given path to the roster and assigns it a dedicated
database at `~/.knowing/repos/<safe-name>.db`. After registration the repo is
indexed using the standard tree-sitter fast path with automatic LSP enrichment.
Each repo gets its own isolated database, so graph algorithms (community
detection, RWR, HITS, BM25) operate only on that repo's data.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |

**Examples:**

```bash
# Register and index a repo (assigns ~/.knowing/repos/<safe-name>.db)
knowing add ./my-repo

# Register with an explicit URL
knowing add -url github.com/org/repo ./my-repo

# Override the per-repo database path
knowing add -db /var/lib/knowing/data.db ./my-repo
```

**Notes:**

- If the repo is already in the roster the command re-indexes it.
- Equivalent to `knowing init` followed by `knowing index`, but also registers
  the repo in the roster for `knowing list` and `knowing remove`.

---

### remove

Remove a repository from the roster and evict all its data.

```
knowing remove [flags] <repo-path-or-url>
```

Removes the repository at the given path (or matching URL) from the roster and
evicts all associated data: nodes, edges, files, snapshots, feedback,
task_memory, and graph_notes. This is the CLI equivalent of the `untrack_repo`
MCP tool.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-purge` | bool | `false` | Also delete the per-repo database file |

**Examples:**

```bash
# Remove a repo and evict all its graph data
knowing remove ./my-repo

# Remove by URL
knowing remove github.com/org/old-service

# Remove and delete the database file
knowing remove --purge ./my-repo
```

**Notes:**

- Evicts all data for the repo (nodes, edges, files, snapshots, feedback,
  task_memory, graph_notes).
- With `--purge`, also deletes the per-repo `.db` file from disk.
- Implementation: `internal/store/evict.go`.

---

### list

List all repositories in the roster.

```
knowing list [flags]
```

Prints every registered repository with its path, URL, per-repo database path,
database file size, and last-indexed timestamp.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |

**Examples:**

```bash
# List all registered repos
knowing list
```

---

### stats

Show cumulative graph statistics and feedback metrics.

```
knowing stats [flags]
```

Prints a summary of the knowledge graph: repos, nodes, edges, files, snapshots,
communities, graph notes. Also shows feedback metrics: total feedback records,
useful vs. not useful, unique symbols with feedback, merkleized feedback count,
and usefulness rate.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-json` | bool | `false` | Output as JSON |

**Examples:**

```bash
# Show graph stats for the current repo
knowing stats

# JSON output for programmatic consumption
knowing stats -json

# Stats for a specific database
knowing stats -db ~/.knowing/repos/my-repo.db
```

**Interpreting the output:**

A healthy index should show:

- **Nodes:** Proportional to your codebase size. A 50K-LOC TypeScript repo
  typically produces 2,000-10,000 nodes (functions, classes, types, variables).
- **Edges:** More edges than nodes (typically 2-5x). These are the relationships
  (calls, imports, implements, etc.) that power retrieval. Very few edges
  (< 100) suggests the extractors are not finding relationships in your code.
- **Snapshots:** At least 1. Each `knowing index` or `knowing add` creates a
  snapshot.
- **Files:** Should match the number of source files in your repo (minus
  files in .gitignore).

If node or edge counts are unexpectedly low, try `knowing reindex .` for a
clean re-extraction, and check that your language is
[supported](../../README.md#languages-and-formats).

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- Feedback metrics include merkleized count (feedback records with a
  `neighborhood_root` stored, enabling automatic expiration).

---

### stale

Detect files changed since the last snapshot and report stale nodes.

```
knowing stale [flags]
```

Runs `git diff` against the commit recorded in the latest snapshot, passes changed
file paths to the `StaleNodesByFiles` store method, and reports counts of stale
nodes per file. Designed as a CI gate: exits with code 1 when stale files are
found, code 0 when the graph is fresh.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-repo` | string | *(cwd)* | Repository path (defaults to current directory) |

**Examples:**

```bash
# Check if the graph is stale relative to HEAD
knowing stale

# Check a specific repo
knowing stale --repo ./my-service

# Use in CI (fails the step if stale)
knowing stale || knowing index .
```

**Notes:**

- Requires a pre-built database with at least one snapshot. Run `knowing index` first.
- Only detects staleness relative to the latest snapshot's recorded commit.
- Implementation: `cmd/knowing/stale.go`, `internal/store/sqlite.go` (`StaleNodesByFiles`).

---

### reset

Delete all graph data for a repository without removing the database file.

```
knowing reset [flags]
```

Truncates all tables (nodes, edges, snapshots, edge_events, files, feedback) but preserves the schema and DB file. Run `knowing index` afterward to rebuild.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |

**Examples:**

```bash
# Reset the current repo's graph
knowing reset

# Reset a specific database
knowing reset -db ~/.knowing/repos/my-repo.db
```

---

### vacuum

Compact the database file after deletions.

```
knowing vacuum [flags]
```

Runs SQLite VACUUM to reclaim space from deleted rows. Reports before/after file size and savings.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |

**Examples:**

```bash
# Compact after a reset or GC
knowing vacuum
# Output:
#   Before: 27.7 MB
#   After:  25.9 MB
#   Saved:  1.9 MB (6.7%)
```

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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
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

### enrich

Run offline enrichment passes on an already-indexed database.

```
knowing enrich {lsp|blame|coverage} [flags] <repo-path>
```

Three enrichment passes are available: `lsp` (LSP-based edge upgrade and
discovery), `blame` (git authorship), and `coverage` (Go test coverage).

#### enrich lsp

Run LSP enrichment on an already-indexed database. Auto-detects language
servers (gopls, pyright, typescript-language-server, rust-analyzer, jdtls,
OmniSharp/csharp-ls) and runs the three-phase enrichment pipeline: workspace
readiness, edge upgrade, and edge discovery. Creates phantom external nodes
for stdlib/dependency types.

```
knowing enrich lsp [flags] <repo-path>
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-url` | string | *(auto-detected)* | Repository URL |
| `-concurrency` | int | `8` | Parallel LSP request count (post-warmup bumps to 128 for edge upgrades) |

**Examples:**

```bash
# Run LSP enrichment (auto-detects language servers)
knowing enrich lsp ./my-repo

# With explicit database and concurrency
knowing enrich lsp -db /path/to/knowing.db -concurrency 16 ./my-repo

# With explicit repo URL
knowing enrich lsp -url https://github.com/org/repo ./my-repo
```

**Notes:**

- Requires a pre-built database with nodes from a prior `knowing index` run.
- For Go repos, uses two-phase gopls warmup (didOpen + retry) before high-concurrency blast.
- For Go workspaces with `go.work`, spawns one gopls per module. Progress persisted
  in `.knowing/enrich-progress.json` for crash recovery.
- Enrichment is strongly positive for retrieval: +0.040 P@10 on Python repos,
  kubernetes 0.000 -> 0.159. See [enrichment-pipeline.md](../architecture/enrichment-pipeline.md).

---

#### enrich blame

Stamps `last_author` and `last_commit_at` on every symbol by running
`git blame` on each file that has nodes in the graph. Migration 009 adds the
blame columns to the nodes table.

```
knowing enrich blame [flags] <repo-path>
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |

**Examples:**

```bash
# Stamp blame metadata on all symbols
knowing enrich blame ./my-repo

# Use a specific database and explicit repo URL
knowing enrich blame -db /var/lib/knowing/data.db -url github.com/org/repo ./my-repo
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- Skips files that no longer exist on disk.
- Logs a summary: symbols stamped, files skipped, errors.

#### enrich coverage

Stamps `coverage_pct` on symbols from a Go cover profile. Migration 010 adds
the `coverage_pct` column to the nodes table. For each symbol, the pass finds
overlapping coverage blocks and computes the percentage of covered statements.
Symbols with no coverage data receive a value of -1.

```
knowing enrich coverage [flags] <repo-path>
```

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-url` | string | *(auto-detected)* | Repository URL (auto-detected from git remote if empty) |
| `-profile` | string | `cover.out` | Path to Go cover profile |

**Examples:**

```bash
# Stamp coverage data from the default cover.out
knowing enrich coverage ./my-repo

# Use a specific cover profile and explicit repo URL
knowing enrich coverage -profile cover.out -url github.com/org/repo ./my-repo

# Use an absolute path to the cover profile
knowing enrich coverage -profile /tmp/cover.out -db /var/lib/knowing/data.db ./my-repo
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- The cover profile must be in Go's standard format (`go test -coverprofile`).
- Cover profiles use full module paths; the pass matches by suffix against
  the file paths stored in the graph.
- Logs a summary: symbols stamped, files without coverage data.

---

### debug-seeds

Show the full seed selection pipeline for a task description.

```bash
knowing debug-seeds -task "description" [-db path] [repo-path]
```

**Flags:**
- `-task` (required): task description to debug
- `-db`: path to SQLite database (default: `.knowing/graph.db`)
- `-url`: repository URL (auto-detected if empty)

**Output:** keywords (Primary, Components, Compounds), path terms, BM25 query
and results, path boost analysis, final ForTask top 10 with scores.

---

### debug-fts

Run a raw FTS5 query against the node index.

```bash
knowing debug-fts -query "term1 OR term2" [-db path] [-limit N]
```

**Flags:**
- `-query` (required): FTS5 query string
- `-db`: path to SQLite database
- `-limit`: max results (default 30)

**Supports:** prefix (`term*`), column targets (`symbol_name:"term"`),
OR/AND, quoted phrases (`"email validator"`).

---

### debug-walk

Show the RWR walk from a specific seed node.

```bash
knowing debug-walk -seed "SymbolName" [-db path] [-top N] [-alpha 0.2]
```

**Flags:**
- `-seed` (required): symbol name or prefix to use as seed
- `-db`: path to SQLite database
- `-top`: number of top results to show (default 20)
- `-alpha`: RWR restart probability (default 0.2)
- `-iter`: max RWR iterations (default 20)

**Output:** matched seeds, edge type breakdown per seed, RWR top-N ranking,
score distribution statistics.

---

### bench-task

Run a single benchmark task with detailed hit/miss analysis.

```bash
knowing bench-task -task <task-id> [-corpus path] [-budget N]
```

**Flags:**
- `-task` (required): task ID (e.g., `terraform-hard-004`)
- `-corpus`: path to corpus directory (default `bench/cross-system/corpus`)
- `-budget`: token budget (default 5000)

**Output:** task metadata, top 10 with HIT/MISS markers, P@10 score,
ground truth analysis (FOUND with rank or MISSING), recall.

Persistent cache is automatically disabled for fresh results.

---

### fsck

Check the integrity of the knowledge graph database.

```
knowing fsck [flags]
```

Walks the graph and verifies:

1. **Edge referential integrity:** every edge's source and target hashes refer to existing nodes.
2. **Hash recomputation:** recomputes each node hash and edge hash from its stored fields and compares to the stored value. A mismatch indicates the row was mutated after insertion.
3. **Snapshot chain continuity:** every snapshot's parent pointer refers to an existing snapshot.
4. **SQLite page integrity:** runs `PRAGMA integrity_check` to detect filesystem-level corruption before the application layer sees inconsistent data.

Issues are classified as ERROR (data inconsistency) or WARN (advisory, graph is queryable).

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-repo` | string | *(all repos)* | Limit the check to a specific repository URL |
| `-quick` | bool | `false` | Run only `PRAGMA integrity_check`; skip hash recomputation and edge walks |

**Examples:**

```bash
# Full integrity check on the current repo's database
knowing fsck

# Quick SQLite-level check only
knowing fsck --quick

# Check a specific repo's database
knowing fsck -db ~/.knowing/repos/my-repo.db

# Limit to one repo when the database contains multiple repos
knowing fsck -repo github.com/org/repo
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- Hash recomputation will report mismatches if the database was built before the hash domain prefix change (2026-05-18 release). In that case, re-index the repo.
- The `--quick` mode (PRAGMA only) runs in milliseconds. Full recomputation time scales with node and edge count.
- Exit code 0 if no ERRORs found (warnings do not affect exit code). Exit code 1 if any ERROR is found.

---

### prove

Generate a Merkle proof that a specific relationship exists in the current snapshot.
The proof is a JSON file that can be shared, stored, or verified offline.

```
knowing prove -source <symbol> -target <symbol> [-type calls] [-repo url] [-o file]
```

The proof contains a three-level path from the edge hash to the repo root, with
sibling hashes at each level. An auditor or CI system can verify the proof without
access to the database.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-source` | string | *(required)* | Qualified name (or prefix) of the source symbol |
| `-target` | string | *(required)* | Qualified name (or prefix) of the target symbol |
| `-type` | string | `calls` | Edge type to prove (`calls`, `imports`, `implements`, etc.) |
| `-repo` | string | *(auto-detect)* | Repository URL |
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-o` | string | *(stdout)* | Write proof to file instead of stdout |

**Examples:**

```bash
# Prove that ForTask calls ComputeHITS
knowing prove -source "%ForTask" -target "%ComputeHITS" -type calls -o proof.json

# Prove an import relationship
knowing prove -source "%server" -target "%types" -type imports
```

**Output:** JSON with `source`, `target`, `edge_type`, `snapshot_hash`, and `proof`
(containing `edge_hash`, `package_path`, three levels of proof steps, and the repo root).

See [Merkle Proofs architecture doc](../architecture/merkle-proofs.md) for format details.

---

### verify

Verify a Merkle proof offline. No database needed.

```
knowing verify <proof.json>
knowing verify -proof <file>
```

Recomputes the repo root from the edge hash and proof steps. If the recomputed
root matches the claimed root, the proof is valid: the edge existed in that
snapshot. Any tampering (modified edge, swapped sibling, changed root) fails.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-proof` | string | *(required, or positional arg)* | Path to proof JSON (or `-` for stdin) |

**Examples:**

```bash
# Verify a proof file
knowing verify proof.json

# Pipe from prove
knowing prove -source "%ForTask" -target "%ComputeHITS" | knowing verify -proof -
```

**Output:**
- `VERIFIED` with edge details and step count (exit code 0)
- `FAILED` if the proof does not verify (exit code 1)

---

### prove-absent

Prove that a relationship does NOT exist in the current snapshot.

```
knowing prove-absent -source <symbol> -target <symbol> [-type calls] [-repo url] [-o file]
```

The proof shows the two adjacent sorted leaves that bracket where the missing edge would be. Because `BuildMerkleTree` sorts by `bytes.Compare`, adjacency of those two leaves proves there is no gap between them, and therefore the edge does not exist. The proof includes inclusion proofs for both neighbors, verified against the same root.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-source` | string | *(required)* | Qualified name (or prefix) of the source symbol |
| `-target` | string | *(required)* | Qualified name (or prefix) of the target symbol |
| `-type` | string | `calls` | Edge type to prove absent (`calls`, `imports`, `implements`, etc.) |
| `-repo` | string | *(auto-detect)* | Repository URL |
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-o` | string | *(stdout)* | Write proof to file instead of stdout |

**Examples:**

```bash
knowing prove-absent -source "%PaymentService" -target "%UserDataService" -type calls
knowing prove-absent -source "%AuthHandler" -target "%StripeClient" -type calls -o absence.json
```

**Output:** JSON with `source`, `target`, `edge_type`, `snapshot_hash`, and `absence_proof` (containing the two bracketing neighbor hashes, their inclusion proofs, and the repo root).

See [Merkle Proofs architecture doc](../architecture/merkle-proofs.md#proof-of-absence) for format details.

---

### audit

Generate a structured compliance report: integrity check, graph summary, cross-package edges with provenance and confidence, and optional Merkle proofs. One command for a complete audit artifact.

```
knowing audit [-proofs] [-o report.json] [-repo url] [-max-edges 500]
```

The report includes an integrity check (equivalent to `knowing fsck`), a graph summary (node count, edge count, package count, edge type breakdown), all cross-package edges with provenance and confidence, and optionally a Merkle proof for each cross-package relationship. The snapshot hash is tied to the current git commit.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-repo` | string | *(auto-detect)* | Repository URL |
| `-o` | string | *(stdout)* | Write report to file instead of stdout |
| `-proofs` | bool | `false` | Include a Merkle proof for each cross-package edge |
| `-max-edges` | int | `500` | Maximum number of cross-package edges to include |

**Examples:**

```bash
# Generate a full compliance report with Merkle proofs
knowing audit -proofs -o quarterly-audit.json

# Generate a report for a specific repo with a higher edge limit
knowing audit -max-edges 1000 -repo github.com/org/repo
```

**Notes:**

- Requires a pre-built database. Run `knowing index` first.
- The `-proofs` flag generates a Merkle proof for every cross-package edge in the report. Performance: 5 proofs in ~210us.
- Output is a single JSON file containing all audit data. Suitable for archiving alongside git commit hashes as a compliance artifact.

---

### audit-diff

Compare two audit point snapshots: added and removed edge counts, change classification, and edge type breakdown.

```
knowing audit-diff <old-ref> <new-ref> [-o report.json]
```

Compares two snapshots and reports how the cross-package edge set changed between them. Each changed edge is classified by type: `behavioral` (call or import edges changed), `structural` (implements or reference edges changed), `runtime_drift` (runtime-observed edges changed), or `metadata_only` (only confidence or provenance changed, not the edge itself).

**Snapshot refs:** Same as `diff`: `@latest`, `@prev`, `@first`, `@N`, or raw hex hash.

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(per-repo, from roster)* | Path to the SQLite database |
| `-repo` | string | *(auto-detect)* | Repository URL |
| `-o` | string | *(stdout)* | Write report to file instead of stdout |

**Examples:**

```bash
# Compare last two snapshots
knowing audit-diff @prev @latest -o latest-changes.json

# Compare two quarterly audit snapshots by hash
knowing audit-diff abc123... def456... -o q1-q2-diff.json

# Pipe to stdout
knowing audit-diff @prev @latest
```

**Notes:**

- Requires a pre-built database containing both snapshots.
- Change classifications: `behavioral` (calls, imports), `structural` (implements, references), `runtime_drift` (runtime_calls, runtime_rpc), `metadata_only` (confidence or provenance change only).
- Output is a JSON report with added edge count, removed edge count, classification breakdown, and per-edge details.

---

### audit-supply-chain

Detect supply chain attack patterns by combining structural diff with isolation scoring.

```
knowing audit-supply-chain --base <ref> [--head <ref>] [flags]
```

Analyzes files in the database for supply chain risk signals: structurally isolated code
(zero inbound edges from legitimate modules) with outbound edges to dangerous APIs
(credential reads, process spawning, network exfiltration). Computes an isolation score
(0.0 to 1.0) per file and traces capability paths (e.g., "reads GITHUB_TOKEN, spawns curl").

**Flags:**

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-db` | string | *(default DB)* | Path to the SQLite database |
| `-base` | string | *(required)* | Baseline snapshot ref |
| `-head` | string | `@latest` | Current snapshot ref |
| `-threshold` | float | `0.3` | Isolation score threshold for flagging suspicious files |
| `-scan-all` | bool | `false` | Scan all files (skip diff, for when clean/compromised are in separate DBs) |
| `-fail-on-suspicious` | bool | `false` | Exit non-zero if any file exceeds threshold |
| `-o` | string | *(stdout)* | Write JSON report to file |

**Examples:**

```bash
# Scan all files in a database for supply chain patterns
knowing audit-supply-chain --base @latest --head @latest --scan-all -db suspect.db

# Compare two versions (same DB containing both snapshots)
knowing audit-supply-chain --base abc123 --head def456 -o report.json

# CI gate: fail if suspicious code detected
knowing audit-supply-chain --base @prev --head @latest --fail-on-suspicious
```

**Output (JSON):**

Reports suspicious files with isolation scores, credential reads (`reads_env`),
process spawning (`executes_process`), and capability paths showing the attack chain.

```json
{
  "summary": {"files_analyzed": 3, "files_suspicious": 1, "env_reads_total": 2, "process_exec_total": 1},
  "suspicious_files": [{
    "file": "src/malicious.ts",
    "score": 0.9,
    "reads_env": ["env://GITHUB_TOKEN", "env://NPM_TOKEN"],
    "executes_process": ["process://curl"]
  }],
  "capability_paths": [{"from": "env://GITHUB_TOKEN", "to": "process://curl"}]
}
```

**Safety:** Analyzes code via tree-sitter AST parsing. Never executes JavaScript, Python, or any other code.

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

---

## Troubleshooting

### Verifying your index

After running `knowing add .` or `knowing index`, verify the graph was built
correctly:

```bash
# 1. Check node and edge counts
knowing stats
# A healthy 50K-LOC TypeScript repo typically produces 2K-10K nodes
# and 5K-30K edges. If you see 0 nodes, indexing failed silently.
# If you see nodes but very few edges (< 100), the extractors may
# not be detecting relationships in your code structure.

# 2. Search for a symbol you know exists
knowing query "MyKnownClassName"
# If this returns "No nodes found", the symbol was not extracted.
# Try a broader prefix: knowing query "MyKnown"

# 3. Check graph integrity
knowing fsck
# Should report 0 errors. If it reports hash mismatches, re-index:
# knowing reindex .

# 4. Check if the graph is stale relative to your latest commit
knowing stale
# Exits 0 if fresh, 1 if stale. Re-index if stale.
```

### "Context returns unrelated symbols"

This is the most common issue for new users. The retrieval pipeline relies on
keyword matching to find entry points (seeds), then walks the graph outward
from those seeds. If the seeds are wrong, everything downstream is wrong.

**Step 1: Check that indexing produced enough edges.**

```bash
knowing stats
```

If the edge count is very low relative to your codebase size, the extractors
may not be finding relationships. Common causes:

- **Monorepo with code in nested directories:** `knowing add .` indexes from
  the current directory. Make sure you are in the repository root.
- **Non-standard project structure:** knowing discovers files by walking the
  git worktree. Files not tracked by git are skipped.
- **Unsupported language features:** Tree-sitter extraction covers common
  patterns but may miss framework-specific conventions (e.g., decorator-based
  routing in custom frameworks). Check [supported languages and
  frameworks](../../README.md#languages-and-formats).

**Step 2: Check seed quality with `knowing why`.**

```bash
knowing why -task "your task description" -symbol "ExpectedSymbol"
```

This shows the full scoring breakdown. Key things to look for:

- **"Not found in result set"**: The symbol was not reachable from any seed.
  This means keyword extraction did not find entry points close to this symbol
  in the graph. Try a task description that uses the exact names from your code
  (e.g., "fix the `AuthMiddleware` timeout" instead of "fix the auth timeout").
- **Seed: no**: The symbol was found but was not a seed. It was discovered via
  graph walk. If it ranked low, the walk may not have reached it strongly enough.
- **Distance: high**: The symbol is many hops from the seeds. The pipeline
  naturally deprioritizes distant symbols.

**Step 3: Use specific symbol names in your task.**

The pipeline works best when the task description contains terms that match
symbol names in your code. Compare:

- Vague: "fix the authentication bug" (matches many things)
- Specific: "fix the `validateToken` timeout in `AuthMiddleware`" (matches
  exactly the symbols you need)

Backtick-quoted identifiers in the task get the highest search priority.

**Step 4: Verify equivalence classes are active.**

The MCP server startup log should show:

```
[knowing] Embeddings: OFF | Gap-fill seeds: OFF | Equivalence classes: 263
```

The 263 framework equivalence classes bridge vocabulary gaps between task
descriptions and code symbols. They are always active and require no
configuration. Embeddings are off by default (confirmed neutral on cold-start
benchmarks). Use `--embeddings` to opt in if experimenting.

**Step 5: Re-index with a clean slate.**

If the database is from an older version or was built incrementally over many
changes, a clean re-index can help:

```bash
knowing reindex .
```

### "knowing query returns no results"

- **Symbol name mismatch:** `knowing query` does prefix matching on qualified
  names. The qualified name includes the repo URL and package path
  (e.g., `github.com/org/repo/pkg.FuncName`). Try a shorter prefix:
  `knowing query "FuncName"` instead of the full path.
- **Database not found:** knowing resolves the database via the roster. Run
  `knowing list` to see registered repos. If your repo is not listed, run
  `knowing add .` from the repo root.
- **Wrong directory:** The roster maps directories to databases. Run commands
  from inside the repository, or use `-db` to point to the database explicitly.

### "MCP server not responding" or agent can't use knowing tools

- **Binary not on PATH:** Verify with `which knowing`. If not found, ensure
  your install method added it to PATH. For Homebrew: `brew link knowing`.
- **First launch is slow:** The MCP server auto-indexes on first launch. For
  large repos this can take 10-30 seconds. The agent may time out waiting.
  Pre-index with `knowing add .` before starting the agent session.
- **stdio transport issues:** Some agent frameworks require explicit
  `"transport": "stdio"` in the config. Check the
  [MCP Tools Reference](mcp-tools.md) for config examples.
- **Verify from CLI:** If the MCP server should be working, verify the
  underlying data is present: `knowing stats` and `knowing query "SomeSymbol"`
  from the same directory where the agent runs.

### "Hash mismatch" errors from `knowing fsck`

Hash mismatches mean the stored data does not match its content-addressed hash.
This happens when:

- **Version upgrade:** Hash domain prefixes were added in v0.3.0. Databases
  from older versions will show mismatches. Fix: `knowing reindex .`
- **Database corruption:** Rare, but possible after unclean shutdowns. Fix:
  `knowing reset && knowing index .`

### Performance issues

- **Indexing is slow:** Use `--no-enrich --skip-blame` for fast structural-only
  indexing. LSP enrichment and git blame add precision but take longer.
- **Queries are slow:** First query on a large graph builds the adjacency cache
  (can take seconds). Subsequent queries use the cache (2ms). If queries are
  consistently slow, check `knowing stats` for graph size; graphs above 100K
  nodes are expected to take longer.
- **Large database file:** Run `knowing vacuum` to compact after deletions.

### Getting help

If none of the above resolves your issue:

1. Run `knowing stats -json` and `knowing fsck` to collect diagnostic info.
2. Open an issue at [github.com/blackwell-systems/knowing](https://github.com/blackwell-systems/knowing/issues) with the diagnostic output and a description of what you expected vs. what happened.
