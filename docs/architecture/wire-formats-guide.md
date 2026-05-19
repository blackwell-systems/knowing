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

## How to Select a Format

**CLI:**
```bash
knowing context -task "add caching" -format gcf
knowing context -task "add caching" -format toon
knowing context -task "add caching" -format json
```

**MCP tools** (via `format` parameter):
```json
{"tool": "context_for_task", "arguments": {"task_description": "add caching", "format": "gcf"}}
```

**Recommendation:**
- Use `gcf` when the consumer is an AI agent in a knowing-aware workflow (hooks, repeated calls, session dedup). This is the default in MCP mode.
- Use `toon` when feeding context to external tools or agents that support TOON but not GCF.
- Use `json` when debugging, piping to jq, or integrating with systems that expect JSON.
- Use `xml` as the default for human-readable MCP responses (current MCP default).

The format only affects the output encoding. The retrieval pipeline (seed matching, RWR, HITS, RRF fusion, scoring) is identical regardless of format. With `gcf` or `toon`, more symbols fit within the same token budget because each symbol costs fewer tokens.

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

**Header:** `GCF tool=<name> budget=<N> tokens=<N> symbols=<N> [pack_root=<64-char hex>]`

When a PackRoot is computed (all `context_for_task` calls), the header includes a `pack_root` field. Agents can store this value and pass it back as the `pack_root` parameter on the next call: if the result is unchanged, the server returns `"unchanged"` instead of resending the full context (93-99% byte savings).

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
  "pack_root": "<64-char hex>",
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

## TOON (Token-Oriented Object Notation)

Structured text format using the open [TOON v3.0 spec](https://github.com/toon-format/spec). TOON sits between JSON (maximum compatibility, verbose) and GCF (maximum compression, custom). Its defining feature is tabular encoding: uniform object collections are written as a header row plus data rows, a pattern every LLM recognizes from markdown tables.

**Implementation:** `internal/wire/toon.go` uses the official `toon-format/toon-go` library.

### Format Characteristics

- **Token efficiency:** 39% of JSON token cost at the same payload size. Approximately 2.7x more tokens than GCF.
- **LLM familiarity:** Medium-high. The tabular header-row-plus-data-rows structure is a pattern every model has seen. No custom ID references.
- **Human readability:** Yes. The format is structured text, legible without tooling.
- **Open standard:** Interoperable with any consumer that supports TOON, not just knowing tools.

### When to Use TOON

- When feeding context to external tools or agents that support TOON but not GCF.
- When sharing context outside knowing's ecosystem and you need better compression than JSON without committing to a knowing-specific format.
- As the default format for agent workflows where GCF comprehension has not been verified. TOON's tabular structure is safe; GCF's integer ID references are novel to most models.

### Example

A TOON-encoded payload for the same 10-symbol, 8-edge context used in the GCF example:

```
tool: context_for_task
tokens_used: 1847
token_budget: 5000
symbols:
  name | kind | score | signature | provenance | distance
  github.com/blackwell-systems/knowing/internal/mcp.requireHash | fn | 0.78 | func requireHash(args map[string]any, key string) (types.Hash, error) | lsp_resolved | 0
  github.com/blackwell-systems/knowing/internal/mcp.Server.registerTools | method | 0.74 | func (s *Server) registerTools() | lsp_resolved | 0
  ...
edges:
  source | target | type
  github.com/blackwell-systems/knowing/internal/mcp.NewServer | github.com/blackwell-systems/knowing/internal/mcp.requireHash | calls
  ...
```

The tabular layout eliminates field name repetition for each row (same saving mechanism as TSV), while the TOON spec adds nested object support and a standardized schema declaration.

### Format Comprehension Eval Results

From `eval/TestFormatComprehension` (6 fixture tasks, 5000 token budget):

| Format | Avg tokens | vs JSON |
|--------|-----------|---------|
| JSON | 1,818 | baseline |
| XML | 1,818 | 100% |
| TOON | 707 | **39%** |
| GCF | 265 | **15%** |

TOON at 39% of JSON token cost is the recommended default for production agent workflows until GCF comprehension is validated by a live LLM eval. See the [Format Comprehension Eval](#format-comprehension-eval) section below for per-fixture results.

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

## Format Comprehension Eval

Benchmark measuring token cost across all formats for the same context payload. All formats contain identical information (same symbols, edges, metadata); only the encoding differs.

Run: `GOWORK=off go test ./eval/ -run TestFormatComprehension -v`

| Format | Avg tokens | vs JSON | LLM familiarity | Recommendation |
|--------|-----------|---------|-----------------|----------------|
| JSON | 1,818 | baseline | Universal | Debugging, generic consumers |
| XML | 1,818 | 100% | High | Legacy integrations |
| TOON | 707 | **39%** | Medium (open standard, tabular) | Safe default for agents |
| GCF | 265 | **15%** | Low (custom format) | Maximum compression when verified |

Per-fixture results (6 comprehension tasks, 5000 token budget):

| Fixture | JSON | XML | TOON | GCF | TOON/JSON | GCF/JSON |
|---------|------|-----|------|-----|-----------|----------|
| top_3_by_score | 1,658 | 1,658 | 645 | 243 | 38.9% | 14.7% |
| symbol_count | 3,105 | 3,105 | 1,206 | 452 | 38.8% | 14.6% |
| edge_count | 1,386 | 1,386 | 540 | 201 | 39.0% | 14.5% |
| kind_extraction | 1,658 | 1,658 | 645 | 243 | 38.9% | 14.7% |
| seed_vs_related | 1,431 | 1,431 | 557 | 210 | 38.9% | 14.7% |
| edge_types | 1,669 | 1,669 | 650 | 240 | 38.9% | 14.4% |

**Key insight:** TOON uses an open standard with tabular formatting (header row + data rows) that LLMs recognize from markdown tables. GCF is 2.7x more compact than TOON but uses a custom format with local integer IDs that models may not have seen in training. TOON is the safer default; GCF is the power option for agents that have been verified to parse it correctly.

**Why not just use GCF?** Token savings only matter if the model comprehends the format. A format that saves 85% of tokens but causes 5% parsing errors is worse than one that saves 61% with 0% errors. TOON's tabular format is a pattern every LLM understands (it looks like a markdown table). GCF's `$1 -> $3` edge references are novel. Until a live LLM eval proves GCF comprehension matches JSON, TOON is the safer recommendation for production agent workflows.

---

## Implementation

| File | Purpose |
|------|---------|
| `internal/wire/registry.go` | Codec registry (Register, Get, List, EncodeWith, DecodeWith) |
| `internal/wire/gcf.go` | GCF text encoder |
| `internal/wire/gcf_decode.go` | GCF text decoder |
| `internal/wire/json.go` | JSON codec (encode/decode via standard library) |
| `internal/wire/toon.go` | TOON codec (official toon-format/toon-go library) |
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
