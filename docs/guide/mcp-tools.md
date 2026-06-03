# knowing MCP Tools Reference

Complete reference for the 28 MCP tools exposed by knowing's MCP server.

## Connecting to the Server

knowing exposes its MCP server over two transports:

- **stdio**: The default for local agent integrations. The daemon reads from stdin and writes to stdout.
- **HTTP (Streamable HTTP)**: For networked access. Pass `--addr :8080` (or any address) when starting the daemon.

### .mcp.json Configuration

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

**Zero-config:** No `-db` flag or `knowing index` step is needed. On first
launch, if the database doesn't exist, the MCP server auto-detects the git
repository, indexes it, and registers it in the roster. Subsequent sessions
resolve the database automatically via the roster.

The `--watch` flag enables integrated file watching. The MCP server monitors the
repository for changes and re-indexes automatically on save, so agents always
query up-to-date graph data. Additional flags for watch mode: `-repo` (repo
path, defaults to cwd), `-no-enrich` (skip LSP enrichment), `-debounce` (ms,
default 500). Omit `--watch` if you manage indexing separately.

For HTTP transport, configure a client that connects to the Streamable HTTP endpoint:

```json
{
  "mcpServers": {
    "knowing": {
      "url": "http://localhost:8080",
      "transport": "streamable-http"
    }
  }
}
```

### Data Requirements

Most tools operate on the static knowledge graph (nodes, edges, snapshots). The
graph is built automatically on first MCP server launch (zero-config) or
manually via `knowing index`. Three tools in the Runtime category require OTLP
trace data ingested by the daemon's trace pipeline; they return an error if the
underlying store is not a SQLiteStore with runtime tables.

| Category | Requires runtime trace data |
|----------|----------------------------|
| Indexing | No |
| Graph queries | No |
| Snapshot | No |
| Analysis | No |
| Ownership | No |
| Context | No |
| Runtime | **Yes** |
| Feedback | No (requires SQLiteStore) |
| Discovery | No (test_scope requires SQLiteStore) |

---

## Indexing

### `index_repo`

Index a repository to build the knowledge graph. Records the request for the daemon to process.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `repo_url` | string | yes | URL of the repository to index |
| `repo_path` | string | yes | Local filesystem path to the repository |
| `commit_hash` | string | no | Git commit hash to index (defaults to HEAD) |

**Return format:**

```json
"index_repo complete: repo_url=https://github.com/org/repo repo_path=/home/user/repo commit=abc123"
```

Plain text confirmation string on success. On error, returns an MCP error result with a message.

**Example:**

```json
{
  "tool": "index_repo",
  "arguments": {
    "repo_url": "https://github.com/org/myservice",
    "repo_path": "/home/user/code/myservice",
    "commit_hash": "a1b2c3d4e5f6"
  }
}
```

---

## Graph Queries

### `cross_repo_callers`

Find all transitive callers of a symbol across repositories.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `target_hash` | string | yes | Hex-encoded SHA-256 hash of the target node (64 characters) |
| `max_depth` | integer | no | Maximum traversal depth (default 5) |

**Return format:**

```json
[
  {
    "Node": {
      "NodeHash": "abcd1234...",
      "FileHash": "...",
      "QualifiedName": "github.com/org/repo://pkg.Func",
      "Kind": "function",
      "Line": 42,
      "Signature": "func Func(ctx context.Context) error"
    },
    "Depth": 2
  }
]
```

Array of `CallerResult` objects, each containing the caller `Node` and its `Depth` (hop count from the target).

**Example:**

```json
{
  "tool": "cross_repo_callers",
  "arguments": {
    "target_hash": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
    "max_depth": 3
  }
}
```

### `blast_radius`

Compute the blast radius of a symbol: all transitive callers grouped by repository.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `target_hash` | string | yes | Hex-encoded SHA-256 hash of the target node |
| `snapshot_hash` | string | no | Snapshot hash for point-in-time query (omit for latest) |

**Return format:**

```json
{
  "Target": {
    "NodeHash": "...",
    "QualifiedName": "github.com/org/repo://pkg.Func",
    "Kind": "function",
    "Line": 10,
    "Signature": "func Func()"
  },
  "ByRepo": {
    "github.com/org/repo-a": [
      {
        "Caller": { "NodeHash": "...", "QualifiedName": "...", "Kind": "function", "Line": 55, "Signature": "..." },
        "Depth": 1,
        "Confidence": 0.9,
        "Provenance": [
          { "Source": "ast_resolved", "Confidence": 1.0, "IndexerVersion": "v1", "SourceCommit": "abc123", "Timestamp": 1715000000 }
        ]
      }
    ]
  },
  "TotalCount": 12,
  "Truncated": false
}
```

Returns a `BlastRadiusResult` with callers grouped by repository URL. Each caller includes confidence (minimum along the path) and the full provenance chain.

**Example:**

```json
{
  "tool": "blast_radius",
  "arguments": {
    "target_hash": "a1b2c3d4..."
  }
}
```

### `graph_query`

Query graph nodes by qualified name prefix.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `prefix` | string | yes | Qualified name prefix to search for |

**Return format:**

```json
[
  {
    "NodeHash": "...",
    "FileHash": "...",
    "QualifiedName": "github.com/org/repo://pkg.MyFunc",
    "Kind": "function",
    "Line": 15,
    "Signature": "func MyFunc(x int) string"
  }
]
```

Array of `Node` objects matching the prefix.

**Example:**

```json
{
  "tool": "graph_query",
  "arguments": {
    "prefix": "github.com/org/repo://internal/store"
  }
}
```

### `stale_edges`

Find edges in the graph that are stale (no longer valid in the latest snapshot).

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `snapshot_hash` | string | yes | Hex-encoded snapshot hash to check for staleness |

**Return format:**

```json
[
  {
    "EdgeHash": "...",
    "SourceHash": "...",
    "TargetHash": "...",
    "EdgeType": "calls",
    "Confidence": 0.7,
    "Provenance": "ast_inferred"
  }
]
```

Array of `Edge` objects that exist in the graph but are not present in the specified snapshot.

**Example:**

```json
{
  "tool": "stale_edges",
  "arguments": {
    "snapshot_hash": "fedcba98..."
  }
}
```

### `repo_graph`

Get all files and their nodes for a repository.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `repo_hash` | string | yes | Hex-encoded hash of the repository |

**Return format:**

```json
[
  {
    "FileHash": "...",
    "RepoHash": "...",
    "Path": "internal/store/sqlite.go",
    "ContentHash": "..."
  }
]
```

Array of `File` objects belonging to the repository.

**Example:**

```json
{
  "tool": "repo_graph",
  "arguments": {
    "repo_hash": "abcdef01..."
  }
}
```

---

## Snapshot

### `snapshot_diff`

Compute the structural diff between two snapshots.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `old_snapshot` | string | yes | Hex-encoded hash of the old snapshot |
| `new_snapshot` | string | yes | Hex-encoded hash of the new snapshot |

**Return format:**

```json
{
  "OldSnapshot": "...",
  "NewSnapshot": "...",
  "EdgesAdded": [ { "EdgeHash": "...", "SourceHash": "...", "TargetHash": "...", "EdgeType": "calls", "Confidence": 1.0, "Provenance": "ast_resolved" } ],
  "EdgesRemoved": [],
  "NodesAdded": [ { "NodeHash": "...", "QualifiedName": "...", "Kind": "function" } ],
  "NodesRemoved": []
}
```

Returns a `DiffResult` with raw lists of added/removed nodes and edges.

**Example:**

```json
{
  "tool": "snapshot_diff",
  "arguments": {
    "old_snapshot": "aaa111...",
    "new_snapshot": "bbb222..."
  }
}
```

---

## Runtime

These three tools require OTLP trace data. They query runtime-observed edges (provenance starting with `otel_`) stored in the SQLite database. If the store does not support runtime methods, the tools return an error: `"runtime queries not available: store does not support runtime methods"`.

### `runtime_traffic`

Query runtime-observed edges filtered by service name and optional route pattern.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `service_name` | string | yes | Service name to filter edges by |
| `route_pattern` | string | no | Route pattern filter (SQL LIKE syntax, e.g. `"/api/v1/%"`) |
| `limit` | integer | no | Maximum number of edges to return (default 100) |

**Return format:**

```json
[
  {
    "EdgeHash": "...",
    "SourceHash": "...",
    "TargetHash": "...",
    "EdgeType": "calls",
    "Confidence": 0.85,
    "Provenance": "otel_trace",
    "CallSiteLine": 0,
    "CallSiteCol": 0,
    "CallSiteFile": "",
    "ObservationCount": 1523,
    "LastObserved": 1715000000
  }
]
```

Array of `Edge` objects with runtime observation fields populated (`ObservationCount`, `LastObserved`).

**Example:**

```json
{
  "tool": "runtime_traffic",
  "arguments": {
    "service_name": "api-gateway",
    "route_pattern": "/api/v2/%",
    "limit": 50
  }
}
```

### `dead_routes`

Find route symbols that have no runtime observations in the specified number of days, indicating potentially dead routes.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `stale_days` | integer | no | Number of days without observations to consider a route dead (default 30) |

**Return format:**

```json
[
  {
    "ServiceName": "api-gateway",
    "RoutePattern": "/api/v1/legacy",
    "MappingType": "handler",
    "NodeHash": "...",
    "CreatedAt": 1710000000
  }
]
```

Array of `RouteSymbolRow` objects representing routes with no recent observations.

**Example:**

```json
{
  "tool": "dead_routes",
  "arguments": {
    "stale_days": 60
  }
}
```

### `trace_stats`

Get aggregate statistics about runtime-derived edges, including counts of active, stale, and GC-eligible edges by type.

**Parameters:**

None.

**Return format:**

```json
{
  "TotalEdges": 1200,
  "ActiveEdges": 800,
  "StaleEdges": 300,
  "GCEligible": 100,
  "ByEdgeType": {
    "calls": 950,
    "references": 250
  }
}
```

- `ActiveEdges`: observed in the last 7 days
- `StaleEdges`: not observed in the last 30 days
- `GCEligible`: not observed in the last 90 days

**Example:**

```json
{
  "tool": "trace_stats",
  "arguments": {}
}
```

---

## Analysis

### `semantic_diff`

Compute semantic diff between two snapshots, including added/removed nodes and edges with context.

Uses the `diff` package for enriched output with qualified names and human-readable metadata (compared to the raw `snapshot_diff`).

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `old_snapshot` | string | yes | Hex-encoded hash of the old snapshot |
| `new_snapshot` | string | yes | Hex-encoded hash of the new snapshot |

**Return format:**

```json
{
  "old_snapshot": "aaa111...",
  "new_snapshot": "bbb222...",
  "nodes_added": [
    { "qualified_name": "github.com/org/repo://pkg.NewFunc", "kind": "function", "file": "pkg/new.go", "line": 10, "signature": "func NewFunc()", "node_hash": "..." }
  ],
  "nodes_removed": [],
  "nodes_modified": [
    {
      "qualified_name": "github.com/org/repo://pkg.ExistingFunc",
      "kind": "function",
      "edges_added": [ { "source_name": "...", "target_name": "...", "edge_type": "calls", "confidence": 1.0 } ],
      "edges_removed": []
    }
  ],
  "edges_added": [],
  "edges_removed": [],
  "summary": {
    "nodes_added": 1,
    "nodes_removed": 0,
    "nodes_modified": 1,
    "edges_added": 2,
    "edges_removed": 0
  }
}
```

Returns a `SemanticDiffResult` with enriched node/edge changes and a summary.

**Example:**

```json
{
  "tool": "semantic_diff",
  "arguments": {
    "old_snapshot": "aaa111...",
    "new_snapshot": "bbb222..."
  }
}
```

### `pr_impact`

Analyze the impact of changes between two snapshots, including blast radius of all changed symbols.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `old_snapshot` | string | yes | Hex-encoded hash of the old (base) snapshot |
| `new_snapshot` | string | yes | Hex-encoded hash of the new (head) snapshot |

**Return format:**

```json
{
  "old_snapshot": "aaa111...",
  "new_snapshot": "bbb222...",
  "changed_symbols": [
    {
      "symbol": { "qualified_name": "...", "kind": "function", "file": "...", "line": 42, "node_hash": "..." },
      "change_type": "added",
      "callers": [ { "qualified_name": "...", "kind": "function" } ],
      "callees": [],
      "caller_count": 3,
      "callee_count": 0
    }
  ],
  "affected_edges": [
    { "source_name": "...", "target_name": "...", "edge_type": "calls", "confidence": 1.0 }
  ],
  "summary": {
    "total_symbols_changed": 5,
    "total_callers_affected": 12,
    "total_callees_affected": 3,
    "risk_level": "medium"
  }
}
```

Returns a `PRImpactResult` with per-symbol blast radius and an overall risk assessment.

**Example:**

```json
{
  "tool": "pr_impact",
  "arguments": {
    "old_snapshot": "aaa111...",
    "new_snapshot": "bbb222..."
  }
}
```

### `trace_dataflow`

Trace data flow from a symbol: all transitive callees.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `source_hash` | string | yes | Hex-encoded SHA-256 hash of the source node |
| `max_depth` | integer | no | Maximum traversal depth (default 5) |

**Return format:**

```json
[
  {
    "Node": {
      "NodeHash": "...",
      "QualifiedName": "github.com/org/repo://pkg.HelperFunc",
      "Kind": "function",
      "Line": 88
    },
    "Depth": 1
  }
]
```

Array of `CalleeResult` objects, each containing the callee `Node` and its `Depth` from the source.

**Example:**

```json
{
  "tool": "trace_dataflow",
  "arguments": {
    "source_hash": "abcdef01...",
    "max_depth": 3
  }
}
```

---

## Ownership

### `ownership`

List all files and top-level symbols in a repository, useful for understanding code ownership.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `repo_hash` | string | yes | Hex-encoded hash of the repository |

**Return format:**

```json
[
  {
    "file": {
      "FileHash": "...",
      "RepoHash": "...",
      "Path": "internal/store/sqlite.go",
      "ContentHash": "..."
    },
    "nodes": [
      {
        "NodeHash": "...",
        "QualifiedName": "github.com/org/repo://internal/store.SQLiteStore",
        "Kind": "type",
        "Line": 20,
        "Signature": "type SQLiteStore struct"
      }
    ]
  }
]
```

Array of objects pairing each `File` with its `Node` list. Files with no extracted symbols have an empty or absent `nodes` field.

**Example:**

```json
{
  "tool": "ownership",
  "arguments": {
    "repo_hash": "abcdef01..."
  }
}
```

### `ownership_query`

Query code ownership: find owners (from CODEOWNERS) and authors (from git blame) for a file or symbol. Unlike `ownership` (which lists files and symbols), this tool queries `owned_by` and `authored_by` edges to answer "who owns this code?" questions.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `repo_hash` | string | yes | Hash of the repository (64-char hex) |
| `file_path` | string | no | File path relative to repo root |
| `symbol` | string | no | Qualified symbol name to query authorship for |

At least one of `file_path` or `symbol` should be provided.

**Return format:**

```json
{
  "file_path": "internal/store/sqlite.go",
  "codeowners": [
    { "name": "@backend-team", "kind": "team" }
  ],
  "authors": [
    { "name": "developer@example.com", "kind": "author", "symbol": "SQLiteStore.GetNode" }
  ]
}
```

Returns owners from CODEOWNERS-derived `owned_by` edges and authors from git blame-derived `authored_by` edges.

**Example:**

```json
{
  "tool": "ownership_query",
  "arguments": {
    "repo_hash": "abcdef01...",
    "file_path": "internal/store/sqlite.go"
  }
}
```

---

## Context

### `context_for_task`

Generate graph-ranked, token-budgeted context for a task description.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task_description` | string | yes | Natural language description of the task |
| `token_budget` | integer | no | Token budget (default 50000) |
| `format` | string | no | Output format: `gcf`, `gcb`, `toon`, `json`, `xml` (default), or `markdown` |
| `pack_root` | string | no | PackRoot from a prior `context_for_task` call (64-char hex). Three outcomes: (1) same PackRoot: returns `"unchanged"` (zero retransmission); (2) different PackRoot, prior pack known: returns **delta encoding** with only added/removed symbols (81% token savings); (3) different PackRoot, prior pack unknown: full retransmission. |

**Return format:**

```json
{
  "content": "<context>...</context>",
  "token_count": 12345,
  "symbols_included": 42,
  "symbols_available": 200,
  "pack_root": "<64-char hex>"
}
```

Returns a formatted context block containing ranked symbols from the graph, packed within the specified token budget. The `pack_root` field in the response is a content-addressed identity for this pack: same task + same graph = same PackRoot. Pass it back on the next call to enable deduplication.

**Example:**

```json
{
  "tool": "context_for_task",
  "arguments": {
    "task_description": "refactor auth middleware to use new token validation",
    "token_budget": 30000,
    "format": "xml"
  }
}
```

### `context_for_files`

Generate blast-radius context weighted by runtime observations for a set of changed files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `files` | string | yes | Comma-separated list of changed file paths relative to repo root |
| `repo_url` | string | no | Repository URL for resolving file hashes |
| `token_budget` | integer | no | Token budget (default 50000) |
| `format` | string | no | Output format: `gcf`, `gcb`, `toon`, `json`, `xml` (default), or `markdown` |

**Return format:**

```json
{
  "content": "<context>...</context>",
  "token_count": 8000,
  "symbols_included": 25,
  "symbols_available": 80
}
```

Returns context focused on the blast radius of the specified files: symbols defined in those files, their callers, and related symbols ranked by confidence and graph distance.

**Example:**

```json
{
  "tool": "context_for_files",
  "arguments": {
    "files": "internal/auth/handler.go,internal/auth/middleware.go",
    "repo_url": "github.com/org/repo",
    "token_budget": 40000,
    "format": "gcf"
  }
}
```

### `context_for_pr`

Generate relationship-aware context for a pull request. Identifies all symbols in changed files, runs graph-based relevance scoring (RWR) from them, and surfaces the full structural impact neighborhood including callers, callees, and related types. One call at PR-open time replaces multiple manual context queries.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `files` | string | yes | Comma-separated list of changed file paths relative to repo root (from the PR diff) |
| `repo_url` | string | no | Repository URL for resolving file hashes |
| `token_budget` | integer | no | Maximum token budget (default 8000, larger than per-edit calls) |
| `format` | string | no | Output format: `gcf`, `gcb`, `toon`, `json`, `xml` (default), or `markdown` |

**Return format:**

```json
{
  "content": "<context>...</context>",
  "token_count": 6500,
  "symbols_included": 30,
  "symbols_available": 120
}
```

Returns a context block optimized for PR review: symbols from the changed files, their callers/callees, and structurally related types, ranked by graph proximity and runtime traffic.

**Example:**

```json
{
  "tool": "context_for_pr",
  "arguments": {
    "files": "internal/mcp/server.go,internal/context/context.go",
    "repo_url": "https://github.com/org/repo",
    "token_budget": 8000,
    "format": "gcf"
  }
}
```

### `explain_symbol`

Explain why a symbol ranked where it did for a given task. Shows the full scoring breakdown: seed channel/tier, RWR score, HITS authority/hub, blast radius, confidence, recency, distance, feedback weight, session boost, commit recency, and equivalence class matches (hand-curated, graph-derived, learned vocab). MCP equivalent of the `knowing why` CLI command.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task_description` | string | yes | Task description to evaluate the symbol against |
| `symbol` | string | yes | Symbol name or qualified name to explain |

**Return format:**

Markdown-formatted scoring breakdown including all ranking signals and the symbol's final composite score.

**Example:**

```json
{
  "tool": "explain_symbol",
  "arguments": {
    "task_description": "refactor auth middleware",
    "symbol": "SessionHandler"
  }
}
```

---

## Feedback

### `feedback`

Record or query symbol usefulness feedback from agents. Used to improve ranking over time. Feedback records are merkleized (SubgraphRoot expiration) and cluster-scoped (keyword_cluster prevents cross-task interference). Vocabulary associations are recorded automatically when agents use symbols after context queries.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | yes | Action to perform: `record` or `query` |
| `symbol_hash` | string | yes | Hex-encoded SHA-256 hash of the symbol (64 characters) |
| `session_id` | string | no | Session identifier (required for `action=record`) |
| `useful` | boolean | no | Whether the symbol was useful (required for `action=record`) |

**Return format (action=record):**

```json
{"status":"recorded"}
```

**Return format (action=query):**

```json
{
  "symbol_hash": "...",
  "useful_count": 5,
  "not_useful_count": 1,
  "usefulness_ratio": 0.833
}
```

**Example:**

```json
{
  "tool": "feedback",
  "arguments": {
    "action": "record",
    "symbol_hash": "a1b2c3d4...",
    "session_id": "session-abc123",
    "useful": true
  }
}
```

---

## Discovery

### `test_scope`

Find tests affected by changes to the given files. Performs backward BFS through call edges to discover test functions that transitively depend on symbols in the specified files.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `files` | string | yes | Comma-separated file paths relative to repo root |
| `output` | string | no | Output format: `packages` (default), `functions`, or `run` (go test -run regex) |
| `depth` | number | no | Maximum BFS traversal depth (default 3) |

**Return format:**

```json
{
  "mode": "packages",
  "results": ["./internal/store", "./internal/mcp"],
  "count": 2
}
```

**Example:**

```json
{
  "tool": "test_scope",
  "arguments": {
    "files": "internal/store/sqlite.go,internal/mcp/server.go",
    "output": "run",
    "depth": 4
  }
}
```

### `flow_between`

Find paths between two symbols in the knowledge graph using BFS traversal.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `source_symbol` | string | yes | Qualified name of the source symbol |
| `target_symbol` | string | yes | Qualified name of the target symbol |
| `max_depth` | integer | no | Maximum BFS depth (default 5) |

**Return format:**

```json
{
  "source": "github.com/org/repo://pkg.FuncA",
  "target": "github.com/org/repo://pkg.FuncB",
  "paths": [
    {
      "steps": [
        {"symbol": "pkg.FuncA", "edge_type": "calls"},
        {"symbol": "pkg.Helper", "edge_type": "calls"},
        {"symbol": "pkg.FuncB", "edge_type": ""}
      ]
    }
  ],
  "path_count": 1,
  "truncated": false
}
```

Returns up to 10 paths. `truncated` is true if more paths exist beyond the limit.

**Example:**

```json
{
  "tool": "flow_between",
  "arguments": {
    "source_symbol": "github.com/org/repo://internal/mcp.Server",
    "target_symbol": "github.com/org/repo://internal/store.SQLiteStore",
    "max_depth": 4
  }
}
```

### `plan_turn`

Given a task description, suggests which knowing MCP tools to call with pre-filled arguments. Returns up to 4 ranked suggestions based on keyword matching.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `task` | string | yes | Description of the task you want to accomplish |

**Return format:**

```json
{
  "suggestions": [
    {
      "tool": "test_scope",
      "reason": "task relates to testing or affected test scope",
      "args": {"files": "<fill: comma-separated changed file paths>", "output": "run"}
    }
  ]
}
```

**Example:**

```json
{
  "tool": "plan_turn",
  "arguments": {
    "task": "find tests affected by changes to the store layer"
  }
}
```

### `communities`

Detect communities in the knowledge graph using modularity clustering. Returns densely-connected groups of symbols.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `action` | string | no | `list` (default) returns all communities; `for_symbol` returns the community containing a specific symbol |
| `algorithm` | string | no | Community detection algorithm: `louvain` (default), `louvain-fine` (higher resolution), or `label-propagation`. Additional algorithms can be registered via the `internal/community/` registry without changing this interface. |
| `repo_url` | string | no | Filter nodes to a specific repository URL prefix |
| `symbol` | string | no | Qualified symbol name (required when `action=for_symbol`) |

**Return format (action=list):**

```json
{
  "communities": [
    {
      "id": 0,
      "size": 25,
      "top_symbols": ["store.SQLiteStore", "store.NewSQLiteStore", "store.Migrate"],
      "cohesion": 0.82,
      "dominant_package": "store",
      "merkle_root": "a3f7c2...",
      "packages": ["internal/store", "internal/store/migrations"]
    }
  ],
  "node_count": 500,
  "edge_count": 2100
}
```

Each community entry now includes:
- `merkle_root`: Merkle root computed over the packages the community spans. Two agents working on communities with disjoint `merkle_root` values operate on disjoint subtrees and cannot conflict at the Merkle level.
- `packages`: sorted list of package paths spanned by this community.

**Return format (action=for_symbol):**

```json
{
  "symbol": "github.com/org/repo://internal/store.SQLiteStore",
  "community": { "id": 0, "size": 25, "top_symbols": [...], "cohesion": 0.82, "dominant_package": "store", "merkle_root": "a3f7c2...", "packages": ["internal/store"] },
  "neighbors": [{ "id": 1, "size": 15, ... }]
}
```

**Example:**

```json
{
  "tool": "communities",
  "arguments": {
    "action": "list",
    "repo_url": "github.com/org/repo"
  }
}
```

---

## Audit Tools

### prove

Generate a cryptographic Merkle proof that a relationship EXISTS between two symbols.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source` | string | yes | Source symbol name (substring match) |
| `target` | string | yes | Target symbol name (substring match) |
| `edge_type` | string | no | Edge type to prove (default: `calls`) |

**Returns:** JSON proof artifact with hash path from edge leaf through edge-type root, package root, to repo root. Verifiable offline.

---

### prove_absent

Generate a cryptographic proof that a relationship does NOT exist between two symbols.

**Parameters:**

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source` | string | yes | Source symbol name (substring match) |
| `target` | string | yes | Target symbol name (substring match) |
| `edge_type` | string | no | Edge type to prove absent (default: `calls`) |

**Returns:** Absence proof using adjacent sorted Merkle leaves. Proves the gap where the missing edge would be. Verifiable offline.

**Use cases:**
- Prove service isolation ("PaymentService cannot call UserDB")
- Validate architectural boundaries
- Compliance certification ("no unauthorized dependency exists")

---

### fsck

Verify graph integrity.

**Parameters:** None.

**Returns:** Pass/fail with details:
- SQLite PRAGMA integrity_check
- Referential integrity (edges with missing source/target nodes)
- Snapshot chain continuity (broken parent references)
- Node and edge counts

---

## Management

### `untrack_repo`

Evict all data for a repository from the knowledge graph. Removes nodes, edges, files, snapshots, feedback, task_memory, and graph_notes associated with the specified repository. Use when a repo is no longer needed and its data should be purged.

**Parameters:**

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `repo_url` | string | yes | Repository URL to evict (e.g. `github.com/org/repo`) |

**Return format:**

```json
{
  "status": "evicted",
  "repo_url": "github.com/org/repo",
  "nodes_deleted": 1234,
  "edges_deleted": 5678,
  "files_deleted": 89
}
```

Returns a summary of evicted data counts. On error (e.g., repo not found), returns an MCP error result with a message.

**Example:**

```json
{
  "tool": "untrack_repo",
  "arguments": {
    "repo_url": "github.com/org/old-service"
  }
}
```

---

## Resources

knowing exposes 8 MCP resources. Resources are read directly by the MCP host without consuming a tool call slot. They are useful for agent orientation at session start.

| Resource URI | What it returns |
|---|---|
| `knowing://report` | Graph size, top node kinds, hotspot count, and snapshot age. Useful as a session-opening orientation summary. |
| `knowing://schema` | Node kinds, edge types, provenance tiers, and the qualified-ID hash format. Helps agents interpret graph data. |
| `knowing://stats` | Node and edge counts broken down by repo and kind. |
| `knowing://repos` | All tracked repositories with node/edge counts and last-indexed timestamps. |
| `knowing://session` | Live session metrics: context calls made, symbols served, cache hits and misses, and server uptime. Backed by atomic counters on the MCP Server struct incremented in context handlers. |
| `knowing://index-health` | Per-repo health status (healthy, stale, or corrupted) and integrity check results. |
| `knowing://communities` | Community list with cohesion scores and Merkle roots from the latest community detection run. |
| `knowing://community/{id}` | Single community detail (resource template). Accepts a numeric community ID and returns members, key files, and cross-community connections. |

Resources are implemented in `internal/mcp/resources.go`.

---

## Hash Format

All hash parameters are hex-encoded SHA-256 digests (64 lowercase hex characters, representing 32 bytes). You can obtain node and repo hashes from `graph_query`, `repo_graph`, or `index_repo` results.

## Error Handling

Tool errors are returned as MCP `CallToolResult` with `IsError=true` and a text message. Common error patterns:

- `"missing required argument: <name>"`: a required parameter was not provided
- `"invalid hash \"...\": expected 32 bytes, got N"`: the hash string was not a valid 64-character hex string
- `"runtime queries not available: store does not support runtime methods"`: a runtime tool was called but the store is not a SQLiteStore
- `"index_repo: no indexing function configured; call SetIndexFunc first"`: the server was created without wiring the indexer
