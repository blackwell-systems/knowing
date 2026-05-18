# Wire Format and Codec System

The wire package (`internal/wire/`) provides a pluggable codec registry that encodes and decodes the graph payloads produced by context packing, MCP tools, and the export CLI. Three built-in codecs serve different layers of the system; additional codecs can be registered at runtime.

## Codec Registry

The registry is a thread-safe map of named codecs. Each codec implements an `Encoder` (Payload to string) and a `Decoder` (string to Payload). The public API:

| Function | Purpose |
|----------|---------|
| `wire.Register(codec)` | Add a codec to the registry (panics on duplicate name) |
| `wire.EncodeWith(name, payload)` | Encode a payload using the named codec |
| `wire.DecodeWith(name, input)` | Decode a string back into a payload using the named codec |
| `wire.Get(name)` | Retrieve a codec by name |
| `wire.List()` | Return all registered codecs (sorted) |

## Built-In Codecs

| Codec | Format | Use Case | Savings |
|-------|--------|----------|---------|
| **GCF** (Graph Compact Format) | Text, graph-native line protocol | Agent/LLM consumption. Token-optimized with structured delimiters. | ~76.7% token savings vs JSON |
| **binary** | Varint + length-prefixed binary | Daemon IPC, caching, transport between services. Magic header `GCB1`, version byte, packed symbols and edges. | ~74% byte savings vs JSON |
| **json** | Standard JSON | Human/debug use, compatibility baseline. Maximum readability, verbose. | (baseline) |
| **toon** | TOON (Typed Object-Oriented Notation) | Structured interchange with external tooling. Uses the official `toon-format/toon-go` library (`internal/wire/toon.go`). | Comparable to GCF |

## Layered Architecture

The three codecs map to distinct system layers:

```
┌──────────────────────────────────────────────────────┐
│  Agent / LLM Context Window                          │
│  Format: GCF (text, token-efficient)                 │
├──────────────────────────────────────────────────────┤
│  Daemon IPC / Computation Cache / Storage            │
│  Format: binary (compact, fast parse)                │
├──────────────────────────────────────────────────────┤
│  Human Debugging / Export CLI / Tests                 │
│  Format: JSON (readable, compatible)                 │
└──────────────────────────────────────────────────────┘
```

- **GCF** is the default for MCP tool responses and context packing output. It minimizes token consumption inside LLM context windows while remaining plain-text parseable.
- **Binary** is used for daemon-to-daemon communication and the content-addressed computation cache. Its varint+length-prefixed layout avoids parsing overhead and produces compact byte streams.
- **JSON** serves as the compatibility baseline for `knowing export`, debugging, and integration with external systems that expect standard serialization.

**GCF session statefulness:** The MCP server maintains a per-connection `wire.Session` that tracks which symbols have already been transmitted to the client. On subsequent GCF responses within the same connection, previously-sent nodes are emitted as bare references (hash-only, no full payload) rather than complete symbol records. This deduplication delivers 47% additional token savings beyond GCF's baseline compression, compounding across multi-turn agent conversations where the same subgraph is referenced repeatedly.

## Binary Wire Layout

```
[magic:4][version:1][header][symbols...][edges...]

Header:  tool(str) tokens_used(varint) token_budget(varint) num_symbols(varint) num_edges(varint)
Symbol:  qname(str) kind(uint8) score(float32) provenance(uint8) distance(uint8) signature(str) components(4xfloat32)
Edge:    source_idx(varint) target_idx(varint) edge_type(uint8) status(uint8)
```

Symbols are indexed by position; edges reference symbols by their zero-based index, avoiding repeated string encoding.

## Benchmark Harness

The `bench/wire-format/` directory contains a benchmark suite that measures encoding size, token count, and round-trip fidelity across six fixture cases:

| Fixture | Scenario |
|---------|----------|
| `01_context_for_task_small` | Small task context (few symbols) |
| `02_context_for_task_medium` | Medium task context (typical agent query) |
| `03_context_for_files` | File-based blast radius expansion |
| `04_blast_radius` | Full blast radius output |
| `05_semantic_diff` | PR semantic diff payload |
| `06_graph_query` | Raw graph query result |

Run benchmarks with `go test -bench=. ./bench/wire-format/`. The scorecard (`bench/wire-format/scorecard.md`) tracks savings ratios against the JSON baseline.
