# Wire Format and Codec System

The wire package (`internal/wire/`) provides a pluggable codec registry that encodes and decodes the graph payloads produced by context packing, MCP tools, and the export CLI. Core GCF types and encoding are provided by the standalone [`gcf-go`](https://github.com/blackwell-systems/gcf-go) library; the wire package re-exports them and adds knowing-specific codecs (binary, JSON) via the registry. Three built-in codecs serve different layers of the system; additional codecs can be registered at runtime.

## Codec Registry

The registry is a thread-safe map of named codecs. Each codec implements an `Encoder` (Payload to string) and a `Decoder` (string to Payload). The public API:

| Function | Purpose |
|----------|---------|
| `wire.Register(codec)` | Add a codec to the registry (panics on duplicate name) |
| `wire.EncodeWith(name, payload)` | Encode a payload using the named codec |
| `wire.DecodeWith(name, input)` | Decode a string back into a payload using the named codec |
| `wire.Get(name)` | Retrieve a codec by name |
| `wire.List()` | Return all registered codecs (sorted) |
| `wire.ListNames()` | Return comma-separated list of registered codec names |

## Built-In Codecs

| Codec | Format | Use Case | Savings |
|-------|--------|----------|---------|
| **gcf** (Graph Compact Format) | Text, graph-native line protocol | Agent/LLM consumption. Token-optimized with structured delimiters. | 84.0% token savings vs JSON (median) |
| **gcb** (Graph Compact Binary) | Varint + length-prefixed binary | Daemon IPC, caching, transport between services. Magic header `GCB1`, version byte, packed symbols and edges. | 74.1% byte savings vs JSON (median) |
| **json** | Standard JSON | Human/debug use, compatibility baseline. Maximum readability, verbose. | (baseline) |

## Layered Architecture

The three codecs map to distinct system layers:

```
┌──────────────────────────────────────────────────────┐
│  Agent / LLM Context Window                          │
│  Format: GCF (text, token-efficient, 84% savings)    │
├──────────────────────────────────────────────────────┤
│  Daemon IPC / Computation Cache / Storage            │
│  Format: GCB (compact binary, fast parse, 74%)       │
├──────────────────────────────────────────────────────┤
│  Human Debugging / Export CLI / Tests                 │
│  Format: JSON (readable, compatible)                 │
└──────────────────────────────────────────────────────┘
```

- **GCF** is the default for MCP tool responses and context packing output. It minimizes token consumption inside LLM context windows while remaining plain-text parseable.
- **GCB** is used for daemon-to-daemon communication and the content-addressed computation cache. Its varint+length-prefixed layout avoids parsing overhead and produces compact byte streams.
- **JSON** serves as the compatibility baseline for `knowing export`, debugging, and integration with external systems that expect standard serialization.

### GCF Session Deduplication

The MCP server maintains a per-connection `gcf.Session` (from [`gcf-go`](https://github.com/blackwell-systems/gcf-go)) that tracks which symbols have already been transmitted to the client. On subsequent GCF responses within the same connection, previously-sent nodes are emitted as bare references (`@N  # previously transmitted`) rather than complete symbol records. `gcf.EncodeWithSession` partitions symbols into new (full declaration) and known (bare reference) before encoding.

### GCF Delta Encoding

When the agent sends a `pack_root` from a prior call and the current result differs, the server computes a structural diff (`internal/context/delta.go`) and returns only what changed via `gcf.EncodeDelta` (from [`gcf-go`](https://github.com/blackwell-systems/gcf-go)). The delta format uses `## removed`, `## added`, `## edges_removed`, `## edges_added` sections. A 60% threshold ensures delta is only used when it saves meaningfully over full retransmission.

**Benchmark (session 27, `bench/delta-packing/`):** 81.2% token savings at 96.6% symbol overlap on re-query scenarios. See `docs/architecture/context-packing.md` for full protocol.

### Three-Level Token Savings Stack

| Level | What it does | Savings | When |
|-------|-------------|---------|------|
| GCF baseline | Compact line protocol vs JSON | 84% | Every response |
| Session dedup | Bare references for previously-sent symbols | Additional ~47% on repeats | Multi-turn conversations |
| Delta encoding | Only added/removed symbols transmitted | 81% on re-queries | Same task, pack changed |

## Binary Wire Layout

```
[magic:4][version:1][header][symbols...][edges...]

Header:  tool(str) tokens_used(varint) token_budget(varint) num_symbols(varint) num_edges(varint)
Symbol:  qname(str) kind(uint8) score(float32) provenance(uint8) distance(uint8) signature(str) components(4xfloat32)
Edge:    source_idx(varint) target_idx(varint) edge_type(uint8) status(uint8)
```

Symbols are indexed by position; edges reference symbols by their zero-based index, avoiding repeated string encoding.

## Core Types

### Payload

The `Payload` struct (defined in [`gcf-go`](https://github.com/blackwell-systems/gcf-go), re-exported by `internal/wire/gcf.go`) is the universal input/output for all codecs:

```go
type Payload struct {
    Tool        string   // MCP tool name (e.g., "context_for_task")
    TokensUsed  int      // actual tokens consumed
    TokenBudget int      // requested budget
    PackRoot    string   // content-addressed identity (64-char hex hash)
    Symbols     []Symbol
    Edges       []Edge
}
```

### Symbol

Each symbol carries its qualified name, kind, relevance score, provenance tier, graph distance from seeds, optional signature, and score component breakdown:

```go
type Symbol struct {
    QualifiedName string
    Kind          string     // function, type, method, interface, etc.
    Score         float64
    Provenance    string     // lsp_resolved, ast_inferred, etc.
    Distance      int        // 0=target, 1=related, 2+=extended
    Signature     string
    Components    Components // BlastRadius, Confidence, Recency, Distance
}
```

### Edge

Edges reference symbols by qualified name. The `Status` field supports diff responses:

```go
type Edge struct {
    Source   string // qualified name of source symbol
    Target   string // qualified name of target symbol
    EdgeType string // calls, implements, imports, etc.
    Status   string // "added", "removed", "unchanged" (for diff responses)
}
```

### DeltaPayload

Used by `EncodeDelta` for incremental context delivery:

```go
type DeltaPayload struct {
    Tool         string
    BaseRoot     string   // pack_root the agent has
    NewRoot      string   // pack_root of the current result
    Removed      []Symbol
    Added        []Symbol
    RemovedEdges []Edge
    AddedEdges   []Edge
    DeltaTokens  int
    FullTokens   int
}
```

## Bridge: ContextBlock to Payload

`FromContextBlock` (`internal/wire/bridge.go`) converts the internal `ContextBlock` (from the context engine) into a wire `Payload`. If the block already has edges, those are used directly. Otherwise, edges between included symbols are discovered from the store via `EdgesFrom` queries. This bridge is the boundary between the retrieval layer and the wire layer.

## Benchmark Harness

The `bench/wire-format/` directory contains a benchmark suite that measures encoding size, token count, and round-trip fidelity across six fixture cases in `cases/`:

| Fixture | Scenario |
|---------|----------|
| `cases/01_context_for_task_small.yaml` | Small task context (few symbols) |
| `cases/02_context_for_task_medium.yaml` | Medium task context (typical agent query) |
| `cases/03_context_for_files.yaml` | File-based blast radius expansion |
| `cases/04_blast_radius.yaml` | Full blast radius output |
| `cases/05_semantic_diff.yaml` | PR semantic diff payload |
| `cases/06_graph_query.yaml` | Raw graph query result |

Run benchmarks with `GOWORK=off go test -bench=. ./bench/wire-format/`.

Results tracked in:
- `bench/wire-format/scorecard.md`: savings ratios against JSON baseline
- `bench/wire-format/FINDINGS.md`: detailed per-case analysis with interpretation

Latest results: GCF 84.0% median token savings, GCB 74.1% median byte savings.

## Source Files

| File | Purpose |
|------|---------|
| [`gcf-go`](https://github.com/blackwell-systems/gcf-go) | Standalone GCF library: `Payload`/`Symbol`/`Edge`/`Components` types, `Encode`, `Decode`, `Session`, `EncodeWithSession`, `DeltaPayload`, `EncodeDelta` |
| [`gcf` spec](https://github.com/blackwell-systems/gcf) | GCF specification v1.0: grammar, encoding rules, session statefulness, delta extension |
| `internal/wire/gcf.go` | Type aliases and delegating wrappers re-exporting gcf-go for backward compatibility |
| `internal/wire/binary.go` | GCB binary encoder/decoder, varint layout, kind/provenance/edge-type ID maps |
| `internal/wire/json.go` | JSON encoder/decoder (compatibility baseline) |
| `internal/wire/registry.go` | Codec registry (`Register`, `Get`, `List`, `EncodeWith`, `DecodeWith`) |
| `internal/wire/bridge.go` | `FromContextBlock`: converts `ContextBlock` to wire `Payload` with edge discovery |
| `bench/wire-format/bench_test.go` | Encoding size, token count, and round-trip benchmarks |
| `bench/wire-format/scorecard.md` | Auto-generated savings scorecard |
| `bench/wire-format/FINDINGS.md` | Detailed benchmark results and interpretation |
