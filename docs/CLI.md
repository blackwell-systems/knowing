# knowing CLI Reference

Command-line interface for the knowing knowledge graph: index repositories,
query symbols, export graphs, compute semantic diffs, and run the daemon.

## Installation

See [DISTRIBUTION.md](DISTRIBUTION.md) for installation instructions.

## Usage

```
knowing <subcommand> [flags]
```

Subcommands: `serve`, `index`, `query`, `export`, `diff`, `version`.

## Environment

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
- Registers tree-sitter extractors for Go and Python by default.
- The MCP server exposes an `index_repo` tool that triggers indexing through
  the same pipeline as file-watch events.

---

### index

One-shot indexing of a repository.

```
knowing index [flags] <repo-path>
```

Parses the repository using tree-sitter (fast path) and stores nodes and edges
in the SQLite database. After indexing, runs LSP enrichment automatically
unless `-full` is specified.

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

- Registers tree-sitter extractors for Go and Python.
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
| `-format` | string | `json` | Output format (only `json` is currently supported) |
| `-repo` | string | *(empty)* | Filter by repository URL. When set, only nodes belonging to files in that repo are included. |
| `-snapshot` | string | *(empty)* | Filter by snapshot hash (recorded in metadata; does not currently filter nodes) |

**Examples:**

```bash
# Export the entire graph
knowing export > graph.json

# Export filtered to one repo
knowing export -repo github.com/org/repo > repo.json

# Export from a non-default database
knowing export -db /tmp/test.db -repo github.com/org/repo
```

**Notes:**

- Output is pretty-printed JSON with two-space indentation.
- The JSON structure contains three top-level keys: `nodes`, `edges`, and
  `metadata`.
- Each node includes: `node_hash`, `qualified_name`, `kind`, `line`,
  `signature`.
- Each edge includes: `edge_hash`, `source_hash`, `target_hash`, `edge_type`,
  `confidence`, `provenance`.
- Metadata includes: `repo`, `snapshot`, `exported_at` (RFC 3339 timestamp),
  `node_count`, `edge_count`.

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
