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

**Recommendation: use `gcf` for all agent workflows.** LLM comprehension eval
(session 27, `eval/TestLLMFormatComprehension`) proved GCF achieves 100% accuracy
on structured extraction tasks at 16% of JSON's token cost. JSON scored 66.7%
(miscounts on large payloads). The "wait for comprehension validation" caveat is
resolved: GCF is both the most compact and the most accurately comprehended format.

- Use `gcf` for all AI agent workflows. Recommended default.
- Use `toon` when sharing context with external tools that support TOON but not GCF. Also 100% accuracy, but 3.5x more tokens than GCF.
- Use `json` when debugging, piping to jq, or integrating with systems that expect JSON.
- Use `xml` for human-readable output only. Does not include edge data in output.

The format only affects the output encoding. The retrieval pipeline (seed matching, RWR, HITS, RRF fusion, scoring) is identical regardless of format. With `gcf`, more symbols fit within the same token budget because each symbol costs fewer tokens.

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
              | "resource" | "table" | "class" | "selector" | "field"
              | "route" | "ext" | "file" | "pkg" | "svc" ;
qname         = non-whitespace-text ;
score         = float ;
provenance    = "ast_inferred" | "ast_resolved" | "lsp_resolved"
              | "scip_resolved" | "otel_trace" | "structural" | token ;
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

The same data in JSON: ~965 tokens. In GCF: ~233 tokens. **75.9% savings** (simple heuristic). Using the word+punctuation token estimator from FINDINGS.md: 84.0% median across 6 fixtures.

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
| `field` | field |
| `route` | route_handler |
| `ext` | external |
| `file` | file |
| `pkg` | package |
| `svc` | service |

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

**Kind (16 entries):** function=1, type=2, method=3, interface=4, var=5, const=6, resource=7, table=8, class=9, selector=10, field=11, route_handler=12, external=13, file=14, package=15, service=16

**Provenance (7 entries):** ast_inferred=1, ast_resolved=2, lsp_resolved=3, otel_trace=4, scip_resolved=5, runtime_observed=6, structural=7

**Edge type (38 entries):** calls=1, imports=2, implements=3, references=4, handles_route=5, depends_on=6, deploys=7, exposes=8, configures=9, extends=10, overrides=11, decorates=12, throws=13, owned_by=14, authored_by=15, tests=16, runtime_calls=17, runtime_rpc=18, runtime_produces=19, runtime_consumes=20, contains=21, member_of=22, documents=23, consumes_endpoint=24, implements_rpc=25, consumes_rpc=26, gated_by_flag=27, deployed_by=28, tested_by=29, publishes=30, subscribes=31, connects_to=32, similar_to=33, co_tested_with=34, type_hint_of=35, accesses_field=36, reads_env=37, executes_process=38

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

**Median token savings (GCF vs JSON): 76.7%** (simple heuristic). FINDINGS.md word+punctuation estimator: **84.0% median**.

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
knowing context -task "refactor auth" -format gcf      # LLM-optimized
knowing context -task "refactor auth" -format json     # human/debug
knowing context -task "refactor auth" -format gcb      # pipe to another service
knowing context -task "refactor auth" -format toon     # external tooling
```

### MCP Tools

```json
{
  "name": "context_for_task",
  "arguments": {
    "task_description": "refactor auth middleware",
    "token_budget": 5000,
    "format": "gcf"
  }
}
```

Default is `xml` for MCP tools (human-readable structured output). CLI default is also `xml`. Agents that understand GCF should request it explicitly for 84%+ token savings.

---

## Format Comprehension Eval

### Token Cost Benchmark (deterministic)

Measures token cost across formats for the same payload. No LLM involved.

Run: `GOWORK=off go test ./eval/ -run TestFormatComprehension -v`

| Format | Avg tokens | vs JSON |
|--------|-----------|---------|
| JSON | 1,818 | baseline |
| XML | 1,818 | 100% |
| TOON | 707 | **39%** |
| GCF | 265 | **15%** |

### LLM Comprehension Benchmark (session 27)

Sends the same context payload in each format to an actual LLM and measures
whether it can answer structured questions correctly. Six questions with
objectively verifiable answers: top symbol identification, symbol count, edge
count, kind extraction, seed/related group count, and edge type enumeration.

Run: `GOWORK=off go test ./eval/ -run TestLLMFormatComprehension -v -timeout 30m`

(Uses `claude -p` by default. Set `EVAL_BACKEND=api` with `ANTHROPIC_API_KEY` for direct API calls.)

| Format | Accuracy | Avg Tokens | vs JSON |
|--------|----------|-----------|---------|
| **gcf** | **100%** (6/6) | **2,687** | **16%** |
| **toon** | **100%** (6/6) | 9,427 | 58% |
| json | 66.7% (4/6) | 16,372 | baseline |
| xml | 66.7% (4/6) | 5,026 | 31% |

**Per-question results:**

| Question | json | xml | toon | gcf |
|----------|------|-----|------|-----|
| top_symbol (identify highest-scored) | PASS | PASS | PASS | PASS |
| symbol_count (count all symbols) | FAIL (113 vs 133) | PASS | PASS | PASS |
| edge_count (count all edges) | FAIL (120 vs 131) | FAIL (0 vs 131) | PASS | PASS |
| top_kind (kind of top symbol) | PASS | PASS | PASS | PASS |
| seed_count (distance-0 symbols) | PASS | PASS | PASS | PASS |
| edge_type_list (enumerate edge types) | PASS | FAIL (no edges in XML) | PASS | PASS |

**Key findings:**

1. **GCF comprehension is validated.** 100% accuracy on all 6 structured extraction tasks. The concern that LLMs couldn't parse GCF's `@N` local IDs and `@target<@source` edge notation was unfounded.

2. **JSON is the worst performer on large payloads.** At 36K tokens (symbol_count task), the LLM miscounted symbols (113 vs 133). GCF at 5K tokens counted correctly. Verbosity hurts comprehension, not just token cost.

3. **XML doesn't include edge data.** The XML formatter (`FormatContextBlock`) renders symbols but not edges, causing edge-related questions to fail. This is a format limitation, not an LLM limitation.

4. **GCF is 6.1x more token-efficient than JSON** (2,687 vs 16,372 avg tokens) with higher accuracy. There is no reason to prefer any other format for agent workflows.

**Recommendation: GCF should be the default format for all agent workflows.** The previous recommendation to use TOON "until GCF comprehension is validated" is superseded by these results.

---

## Delta Encoding

When an agent passes a `pack_root` from a prior call and the current result differs, the server computes a structural diff and returns only what changed. This is a fourth level of token optimization layered on top of GCF.

```
GCF tool=context_for_task delta=true base_root=aaa111 new_root=bbb222 tokens=30 savings=81%
## removed
fn github.com/example/project.OldHandler
## added
@0 fn github.com/example/project.NewHandler 0.85 rwr
## edges_added
github.com/example/project.Router -> github.com/example/project.NewHandler calls
```

Three outcomes when `pack_root` is sent:
1. **Same root**: "unchanged" (zero tokens)
2. **Different root, prior known**: delta encoding (removed + added sections)
3. **Different root, prior unknown**: full retransmission (fallback)

Delta is only used when it saves more than 40% over full retransmission (60% threshold in `DiffPacks.IsWorthIt`). The diff operates on node hashes (set difference, O(n)).

**Benchmark (session 27):** 81.2% token savings at 96.6% symbol overlap on re-query scenarios. See [context-packing.md](context-packing.md) for the full protocol and `bench/delta-packing/` for the benchmark.

## Implementation

| File | Purpose |
|------|---------|
| `internal/wire/registry.go` | Codec registry (Register, Get, List, EncodeWith, DecodeWith) |
| `internal/wire/gcf.go` | GCF text encoder, `Payload`/`Symbol`/`Edge`/`Components` types |
| `internal/wire/gcf_decode.go` | GCF text decoder |
| `internal/wire/session.go` | `Session` type for cross-call symbol deduplication, `EncodeWithSession` |
| `internal/wire/delta.go` | `DeltaPayload` type, `EncodeDelta` for incremental context delivery |
| `internal/wire/bridge.go` | `FromContextBlock`: converts `ContextBlock` to wire `Payload` with edge discovery |
| `internal/wire/json.go` | JSON codec (encode/decode via standard library) |
| `internal/wire/toon.go` | TOON codec (official toon-format/toon-go library) |
| `internal/wire/binary.go` | GCB binary codec (varint + length-prefixed, 38 edge type enums) |
| `internal/context/delta.go` | `DiffPacks`: structural diff between two `ContextBlock` values |
| `internal/wire/gcf_test.go` | GCF unit tests |
| `internal/wire/session_test.go` | Session deduplication tests |
| `internal/wire/delta_test.go` | Delta encoding tests |
| `internal/wire/registry_test.go` | Registry and JSON codec tests |
| `internal/wire/binary_test.go` | Binary codec tests |
| `bench/wire-format/` | Benchmark harness with 6 fixture cases in `cases/` |
| `bench/wire-format/scorecard.md` | Auto-generated comparison table |
| `bench/wire-format/FINDINGS.md` | Detailed results and interpretation |
| `bench/delta-packing/` | Delta packing benchmark (cross-task + re-query simulation) |
| `eval/format_comprehension_test.go` | Token cost benchmark (deterministic, no LLM) |
| `eval/format_llm_comprehension_test.go` | LLM comprehension eval (6 questions, 4 formats, cli/api backends) |

---

## Comparison to External Approaches

### Table formats (TSV/columnar)

Column headers + TSV rows achieve ~27% savings by eliminating field name repetition. But they treat graph data as flat tables: edge references still require full identifier strings in each row. GCF's local ID system pushes savings to 75%+.

### Binary formats (protobuf, MessagePack, FlatBuffers)

Optimize for machine parsing speed and byte size. An LLM cannot read a protobuf payload directly; it must be decoded to text first, eliminating the savings at the point of consumption. knowing's binary codec fills this niche for machine-to-machine paths, while GCF serves the LLM path.

### JSON-LD / RDF

Verbose by design (full URIs, type annotations, context declarations). Optimizes for semantic interoperability across systems, not token-constrained LLM consumption.
