# Wire Formats

knowing supports multiple wire format encodings for graph data, each optimized for a different layer of the system. All formats encode the same `Payload` structure and are selected via the `format` parameter on MCP tools and CLI commands.

## Architecture

```
                          ┌─────────────┐
                          │  Graph Store │
                          └──────┬──────┘
                                 │ Payload
                    ┌────────────┼────────────┐
                    │            │            │
              ┌─────▼─────┐ ┌───▼────┐ ┌────▼────┐
              │   binary   │ │  gcf   │ │  json   │
              │ (transport)│ │ (LLM)  │ │ (compat)│
              └─────┬─────┘ └───┬────┘ └────┬────┘
                    │            │            │
              ┌─────▼─────┐ ┌───▼────┐ ┌────▼────┐
              │  daemon ↔  │ │ agent  │ │  human  │
              │  services  │ │context │ │  debug  │
              └───────────┘ └────────┘ └─────────┘
```

| Codec | Optimizes for | Consumer | Token savings vs JSON | When to use |
|-------|---------------|----------|----------------------|-------------|
| `gcf` | Maximum compression | AI agents in tight-budget workflows | **84%** | Default for MCP tools, session dedup, repeated calls |
| `toon` | Open standard + compression | External tools, interop, any TOON consumer | **~60%** | When sharing context outside knowing's ecosystem |
| `binary` | Bytes on wire, speed | Services, caches, daemon IPC | N/A (not text) | Internal transport between knowing processes |
| `json` | Compatibility | Humans, generic API consumers, debugging | 0% (baseline) | When downstream consumers need standard JSON |
| `xml` | Structured markup | XML-based toolchains | ~-20% (larger) | Legacy integrations |
| `markdown` | Human readability | Documentation, display | ~10% | Human consumption, not agent workflows |

## GCF (Graph Compact Format)

Text-only, graph-native encoding designed for LLM consumption. Exploits three properties of graph data that flat formats cannot:

1. **Referential identity.** Nodes get local IDs (`@0`, `@1`). Edges reference by ID instead of repeating full qualified names.
2. **Graph topology.** Edges encoded as `@target<@source type` instead of verbose JSON objects.
3. **Hierarchical grouping.** Distance-based sections (`## targets`, `## related`) eliminate per-row distance fields.

### Grammar

```
payload       = header { node-line | edge-line | group-line | comment } ;
header        = "GCF" SP "tool=" token { SP key-value } LF ;
group-line    = "##" SP text LF ;
node-line     = "@" id SP kind SP qname SP score SP provenance LF ;
edge-line     = "@" target "<" "@" source SP edge-type [ SP status ] LF ;
comment       = "#" SP text LF ;

id            = DIGIT { DIGIT } ;
kind          = "fn" | "type" | "method" | "iface" | "var" | "const"
              | "resource" | "table" | "class" | "selector" ;
qname         = non-whitespace-text ;
score         = float ;
provenance    = "ast_inferred" | "lsp_resolved" | "otel_trace" | token ;
status        = "added" | "removed" ;
```

### Example

```
GCF tool=context_for_task budget=5000 tokens=1847 symbols=10
## targets
@0 fn github.com/blackwell-systems/knowing/internal/mcp.requireHash 0.78 lsp_resolved
@1 method github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools 0.74 lsp_resolved
@2 fn github.com/blackwell-systems/knowing/internal/mcp.requireStringArg 0.67 lsp_resolved
@3 fn github.com/blackwell-systems/knowing/internal/mcp.getIntArg 0.66 lsp_resolved
## related
@4 fn github.com/blackwell-systems/knowing/internal/mcp.NewServer 0.54 lsp_resolved
@5 fn github.com/blackwell-systems/knowing/cmd/knowing.cmdServe 0.51 ast_inferred
@6 fn github.com/blackwell-systems/knowing/cmd/knowing.cmdMCP 0.46 ast_inferred
## extended
@7 type github.com/blackwell-systems/knowing/internal/mcp.Server 0.42 lsp_resolved
@8 iface github.com/blackwell-systems/knowing/internal/types.GraphStore 0.38 lsp_resolved
@9 type github.com/blackwell-systems/knowing/internal/store.SQLiteStore 0.35 lsp_resolved
## edges
@0<@4 calls
@0<@5 calls
@0<@6 calls
@1<@4 calls
@2<@5 calls
@2<@6 calls
@9<@8 implements
@4<@8 references
```

The same data in JSON: ~965 tokens. In GCF: ~233 tokens. **75.9% savings.**

### Format Elements

**Header:** `GCF tool=<name> budget=<N> tokens=<N> symbols=<N>`

**Group headers:** `## targets` (distance 0), `## related` (distance 1), `## extended` (distance 2+), `## edges`

**Node lines:** `@{id} {kind} {qualified_name} {score} {provenance}`

**Edge lines:** `@{target}<@{source} {edge_type} [{status}]`

The `<` arrow points toward the target. `@0<@4 calls` means "@4 calls @0."

**Kind abbreviations:**

| Short | Full |
|-------|------|
| `fn` | function |
| `type` | type |
| `method` | method |
| `iface` | interface |
| `var` | var |
| `const` | const |
| `resource` | resource |
| `table` | table |
| `class` | class |
| `selector` | selector |

### Session Statefulness

Across multiple tool calls in a session, previously-transmitted nodes can be referenced without retransmission:

```
GCF tool=context_for_files tokens=800 symbols=5 session=true
## targets
@0  # previously transmitted
@7 fn github.com/blackwell-systems/knowing/internal/mcp.handleBlastRadius 0.62 lsp_resolved
## edges
@0<@7 calls
```

Multi-call workflows get progressively cheaper as the session builds a shared vocabulary of known symbols.

### Where Token Savings Come From

1. **No field names.** GCF uses positional encoding. JSON repeats 9 field names per symbol.
2. **Local ID edge references.** `@0<@4 calls` (5 tokens) vs full qualified name pairs (30+ tokens).
3. **Group headers.** One `## related` replaces N `"distance": 1` fields.
4. **Kind abbreviations.** `fn` vs `"function"`, `iface` vs `"interface"`.
5. **No structural delimiters.** No `{}`, `[]`, `:`, `","`, indentation whitespace.

---

## Binary (GCB1)

Compact binary encoding optimized for transport between services and persistent caching. Not readable by humans or LLMs directly; designed for machine-to-machine paths where byte efficiency and encode/decode speed matter.

### Wire Layout

```
[magic:4 "GCB1"][version:1]
[header]
  tool        : length-prefixed string
  tokens_used : varint
  token_budget: varint
  num_symbols : varint
  num_edges   : varint
[symbols × num_symbols]
  qname       : length-prefixed string
  kind        : uint8 (enum)
  score       : float32 (4 bytes, little-endian)
  provenance  : uint8 (enum)
  distance    : uint8
  signature   : length-prefixed string
  blast_radius: float32
  confidence  : float32
  recency     : float32
  distance_c  : float32
[edges × num_edges]
  source_idx  : varint (index into symbols array)
  target_idx  : varint (index into symbols array)
  edge_type   : uint8 (enum)
  status      : uint8 (enum)
```

### Design Choices

- **Varint encoding** for integers: small values (symbol counts, indices) use 1 byte instead of 4-8.
- **Enum IDs** for kinds, provenances, edge types, statuses: 1 byte instead of variable-length strings.
- **float32** for scores and components: 4 bytes with sufficient precision (scores only need 2 decimal places).
- **Index-based edges**: edges reference symbols by array index (varint), not by name.
- **Length-prefixed strings**: no null terminators, no escaping, no delimiters.
- **No padding or alignment**: every byte carries data.

### Enum Tables

**Kind:** function=1, type=2, method=3, interface=4, var=5, const=6, resource=7, table=8, class=9, selector=10

**Provenance:** ast_inferred=1, ast_resolved=2, lsp_resolved=3, otel_trace=4

**Edge type:** calls=1, imports=2, implements=3, references=4, handles_route=5, depends_on=6, deploys=7, exposes=8, configures=9

**Status:** unchanged/empty=0, added=1, removed=2

### When to Use Binary

- Daemon-to-client IPC (the daemon stores and transmits binary; the client decodes to Payload then re-encodes to GCF or JSON for its consumer)
- Response caching (binary is the most space-efficient on-disk format)
- Cross-service communication where both ends are knowing-aware
- Streaming large graph results where decode latency matters

### Tradeoffs vs GCF

| | Binary | GCF |
|--|--------|-----|
| Byte size | Smallest | ~10-15% larger (text overhead) |
| Token count | N/A (not text) | Optimized |
| LLM-readable | No | Yes |
| Human-readable | No | Yes |
| Score precision | float32 (~7 digits) | 2 decimal places |
| Components preserved | Yes (full) | No (stripped for token savings) |
| Extensibility | Requires version bump | Append new fields freely |

---

## JSON

Standard JSON serialization. Maximum compatibility, zero configuration, works with any consumer. The baseline against which other formats are measured.

```json
{
  "tool": "context_for_task",
  "tokens_used": 1847,
  "token_budget": 5000,
  "symbols": [
    {
      "qualified_name": "github.com/blackwell-systems/knowing/internal/mcp.requireHash",
      "kind": "function",
      "score": 0.78,
      "signature": "func requireHash(args map[string]any, key string) (types.Hash, error)",
      "provenance": "lsp_resolved",
      "distance": 0,
      "components": {
        "blast_radius": 0.40,
        "confidence": 0.25,
        "recency": 0.06,
        "distance": 0.15
      }
    }
  ],
  "edges": [
    {
      "source": "github.com/blackwell-systems/knowing/internal/mcp.NewServer",
      "target": "github.com/blackwell-systems/knowing/internal/mcp.requireHash",
      "edge_type": "calls"
    }
  ]
}
```

Use JSON when: debugging, piping to `jq`, integrating with tools that expect JSON, or when the consumer is not token-constrained.

---

## Benchmark Comparison

All three codecs encoding the same 10-symbol, 8-edge payload:

| Codec | Bytes | Tokens | vs JSON (bytes) | vs JSON (tokens) |
|-------|-------|--------|-----------------|------------------|
| JSON | 4,153 | 965 | baseline | baseline |
| GCF | 1,079 | 233 | -74% | -75.9% |
| Binary | 448 | N/A | -89% | N/A |

Full scorecard across 6 fixture cases (8 to 30 symbols):

| Case | JSON(T) | GCF(T) | Token Savings |
|------|---------|--------|---------------|
| context_for_task (10 sym) | 965 | 233 | 75.9% |
| context_for_task (30 sym) | 2,968 | 649 | 78.1% |
| context_for_files (15 sym) | 1,490 | 334 | 77.6% |
| blast_radius (8 sym) | 835 | 208 | 75.1% |
| semantic_diff (12 sym) | 1,206 | 295 | 75.5% |
| graph_query (20 sym) | 2,078 | 423 | 79.6% |

**Median token savings (GCF vs JSON): 76.7%**

Encode p99 latency: 64 microseconds (30-symbol payload, Apple M4 Pro).

---

## Codec Registry

The `internal/wire` package provides a pluggable codec registry. Formats are selected by name at runtime:

```go
import "github.com/blackwell-systems/knowing/internal/wire"

// Encode with a named codec.
output, err := wire.EncodeWith("gcf", payload)

// Decode with a named codec.
payload, err := wire.DecodeWith("binary", input)

// List all registered codecs.
for _, c := range wire.List() {
    fmt.Printf("%s: %s\n", c.Name, c.Description)
}

// Register a custom codec.
wire.Register(&wire.Codec{
    Name:        "msgpack",
    Description: "MessagePack encoding for external interop",
    Encode:      myMsgpackEncoder,
    Decode:      myMsgpackDecoder,
})
```

Built-in codecs are registered at init time. Custom codecs can be added by any package that imports `internal/wire` and calls `Register()` before use.

### Adding a New Codec

Implement two functions matching these signatures:

```go
type Encoder func(p *Payload) (string, error)
type Decoder func(input string) (*Payload, error)
```

Then register:

```go
func init() {
    wire.Register(&wire.Codec{
        Name:        "my-format",
        Description: "What this format optimizes for",
        Encode:      myEncoder,
        Decode:      myDecoder,
    })
}
```

The MCP tools and CLI pass the `format` parameter directly to the registry, so new codecs become available to all consumers immediately.

---

## Usage

### CLI

```bash
knowing context --task "refactor auth" --format gcf      # LLM-optimized
knowing context --task "refactor auth" --format json     # human/debug
knowing context --task "refactor auth" --format binary   # pipe to another service
```

### MCP Tools

```json
{
  "name": "context_for_task",
  "arguments": {
    "task": "refactor auth middleware",
    "token_budget": 5000,
    "format": "gcf"
  }
}
```

Default is `json` for backwards compatibility. Agents that understand GCF should request it explicitly for 75%+ token savings.

---

## Implementation

| File | Purpose |
|------|---------|
| `internal/wire/registry.go` | Codec registry (Register, Get, List, EncodeWith, DecodeWith) |
| `internal/wire/gcf.go` | GCF text encoder |
| `internal/wire/gcf_decode.go` | GCF text decoder |
| `internal/wire/json.go` | JSON codec (encode/decode via standard library) |
| `internal/wire/binary.go` | Binary codec (varint + length-prefixed) |
| `internal/wire/gcf_test.go` | GCF unit tests |
| `internal/wire/registry_test.go` | Registry and JSON codec tests |
| `internal/wire/binary_test.go` | Binary codec tests |
| `bench/wire-format/` | Benchmark harness with fixture cases |
| `bench/wire-format/scorecard.md` | Auto-generated comparison table |

---

## Comparison to External Approaches

### Table formats (TSV/columnar)

Column headers + TSV rows achieve ~27% savings by eliminating field name repetition. But they treat graph data as flat tables: edge references still require full identifier strings in each row. GCF's local ID system pushes savings to 75%+.

### Binary formats (protobuf, MessagePack, FlatBuffers)

Optimize for machine parsing speed and byte size. An LLM cannot read a protobuf payload directly; it must be decoded to text first, eliminating the savings at the point of consumption. knowing's binary codec fills this niche for machine-to-machine paths, while GCF serves the LLM path.

### JSON-LD / RDF

Verbose by design (full URIs, type annotations, context declarations). Optimizes for semantic interoperability across systems, not token-constrained LLM consumption.
