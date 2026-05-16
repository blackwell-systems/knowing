# knowing MCP Tools Reference

Complete reference for the 14 MCP tools exposed by knowing's MCP server.

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
      "args": ["--db", "/path/to/knowing.db"],
      "transport": "stdio"
    }
  }
}
```

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

Most tools operate on the static knowledge graph (nodes, edges, snapshots) built by `index_repo`. Three tools in the Runtime category require OTLP trace data ingested by the daemon's trace pipeline; they return an error if the underlying store is not a SQLiteStore with runtime tables.

| Category | Requires runtime trace data |
|----------|----------------------------|
| Indexing | No |
| Graph queries | No |
| Snapshot | No |
| Analysis | No |
| Ownership | No |
| Runtime | **Yes** |

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

---

## Hash Format

All hash parameters are hex-encoded SHA-256 digests (64 lowercase hex characters, representing 32 bytes). You can obtain node and repo hashes from `graph_query`, `repo_graph`, or `index_repo` results.

## Error Handling

Tool errors are returned as MCP `CallToolResult` with `IsError=true` and a text message. Common error patterns:

- `"missing required argument: <name>"`: a required parameter was not provided
- `"invalid hash \"...\": expected 32 bytes, got N"`: the hash string was not a valid 64-character hex string
- `"runtime queries not available: store does not support runtime methods"`: a runtime tool was called but the store is not a SQLiteStore
- `"index_repo: no indexing function configured; call SetIndexFunc first"`: the server was created without wiring the indexer
