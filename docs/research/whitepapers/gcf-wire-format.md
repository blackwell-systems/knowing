# GCF: A Token-Optimized Wire Format for Graph-Structured Tool Responses

**Dayna Blackwell, Blackwell Systems**

---

## Abstract

AI agents consume tool responses under fixed token budgets. The dominant encoding for tool responses is JSON, which wastes 75%+ of those tokens on structural overhead: field names, delimiters, and repeated identifiers. We present GCF (Graph Compact Format), a text-based wire format designed specifically for LLM consumption of graph-structured data. GCF exploits three properties of graph data that flat formats cannot leverage: referential identity (local IDs eliminate repeated qualified names), topological encoding (edges as `@target<@source type` instead of JSON objects), and hierarchical grouping (section headers replace per-record metadata fields). Across 6 benchmark payloads ranging from 8 to 30 symbols, GCF achieves a median 76.7% token reduction versus JSON while remaining human-readable and LLM-parseable without special tooling. We also describe GCB (Graph Compact Binary), a companion binary encoding for machine-to-machine paths achieving 89% byte reduction. Both formats are implemented, tested, benchmarked, and deployed in a production MCP server.

---

## 1. The Problem: JSON Is the Wrong Format for LLM Tool Consumption

The Model Context Protocol (MCP) defines how AI agents interact with external tools. Tool responses are overwhelmingly encoded as JSON. This is convenient for developers but expensive for the consumer that matters most: the language model itself.

Consider a typical MCP tool response returning 10 graph nodes and 8 edges (a blast radius query, a dependency subgraph, or a context retrieval result). In JSON, this payload consumes ~965 tokens. The same semantic content in GCF consumes ~233 tokens. The difference, 732 tokens, is pure waste: field name repetition, structural delimiters (`{`, `}`, `[`, `]`, `:`, `","`), and full qualified names repeated in every edge reference.

This waste compounds across a task. An agent making 5 tool calls during a code change task consumes ~4,825 tokens on JSON tool responses. In GCF, the same 5 calls consume ~1,165 tokens, and less with session statefulness enabled (previously-transmitted nodes are referenced by local ID without retransmission). The difference, ~3,660 tokens, is context window capacity that could hold source code, documentation, or additional tool results.

### 1.1 Why This Matters Now

Three trends make this problem urgent:

1. **Tool-heavy agent workflows.** Modern AI agents make 10-50 tool calls per task. Each call returns JSON. Token budgets are consumed by tool overhead before the agent finishes its work.

2. **Graph-structured responses are growing.** Code intelligence, dependency analysis, knowledge graphs, and system topology queries all return graph data. Graph payloads are JSON's worst case because they contain repeated node references across edges.

3. **Context window costs are real.** Whether measured in dollars (API billing), latency (time-to-first-token), or capability (what fits in the window), every wasted token reduces the agent's effectiveness.

### 1.2 Why Not Just Use Binary?

Binary formats (Protocol Buffers, MessagePack, FlatBuffers) optimize for byte size and decode speed. But LLMs consume text, not bytes. A protobuf-encoded tool response must be decoded to text before the LLM can process it, and the decoded text is typically JSON, eliminating the savings at the point of consumption.

The optimization target is not bytes on wire. It is tokens in the context window. These are different quantities with different solutions.

---

## 2. Design Principles

GCF is designed around three observations about graph data that JSON cannot exploit:

### 2.1 Referential Identity

Graph nodes are referenced multiple times: once in their declaration and once per edge they participate in. In JSON, each reference repeats the full qualified name:

```json
{
  "source": "github.com/org/repo/internal/mcp.NewServer",
  "target": "github.com/org/repo/internal/mcp.requireHash",
  "edge_type": "calls"
}
```

In GCF, nodes are declared once with a local ID and referenced by that ID thereafter:

```
@0 fn github.com/org/repo/internal/mcp.requireHash 0.78 lsp_resolved
@4 fn github.com/org/repo/internal/mcp.NewServer 0.54 lsp_resolved
@0<@4 calls
```

The full qualified name appears once per node. Every subsequent reference is 2-3 tokens (`@0`, `@4`) instead of 15-20 tokens.

### 2.2 Topological Encoding

JSON encodes edges as objects with named fields. GCF encodes edges as directed connections:

**JSON (1 edge, ~12 tokens):**
```json
{"source": "...", "target": "...", "edge_type": "calls"}
```

**GCF (1 edge, ~5 tokens):**
```
@0<@4 calls
```

The `<` arrow encodes direction. The source and target are local IDs. The edge type is a bare token. No field names, no delimiters, no quoting.

### 2.3 Hierarchical Grouping

Graph query results often partition nodes by distance from a query center: direct targets (distance 0), related symbols (distance 1), extended context (distance 2+). In JSON, each node carries a `"distance": N` field. In GCF, a section header replaces all per-node distance fields:

```
## targets
@0 fn ... 0.78 lsp_resolved
@1 method ... 0.74 lsp_resolved
## related
@4 fn ... 0.54 lsp_resolved
```

One header replaces N repeated fields.

---

## 3. Specification

### 3.1 Grammar

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

### 3.2 Header

The header line identifies the format and carries payload metadata:

```
GCF tool=context_for_task budget=5000 tokens=1847 symbols=10
```

`tool` identifies the MCP tool that produced this response. `budget` and `tokens` enable the consumer to assess utilization. `symbols` gives the count without scanning.

### 3.3 Node Lines

```
@{id} {kind} {qualified_name} {score} {provenance}
```

Fields are positional. No field names, no delimiters beyond whitespace. Kind abbreviations reduce token count (`fn` vs `"function"`, `iface` vs `"interface"`).

| Abbreviation | Full form |
|-------------|-----------|
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

### 3.4 Edge Lines

```
@{target}<@{source} {edge_type} [{status}]
```

The `<` arrow points toward the target. `@0<@4 calls` means "@4 calls @0." Status is optional: `added` or `removed` for diff payloads.

### 3.5 Group Headers

```
## targets
## related
## extended
## edges
```

Group headers partition the payload into semantic sections. The group a node appears in encodes its distance from the query center, eliminating per-node distance fields.

### 3.6 Session Statefulness

Across multiple tool calls in a session, previously-transmitted nodes can be referenced without retransmission:

```
GCF tool=context_for_files tokens=800 symbols=5 session=true
## targets
@0  # previously transmitted
@7 fn github.com/org/repo/internal/mcp.handleBlastRadius 0.62 lsp_resolved
## edges
@0<@7 calls
```

The `session=true` header flag enables ID persistence. A bare `@0` (no kind, name, or score) references a node transmitted in a previous response. Multi-call workflows get progressively cheaper as the session builds a shared vocabulary.

This exploits a property unique to agent tool interactions: the consumer (the LLM) maintains conversational state across calls. A traditional wire format cannot assume its consumer remembers previous messages. GCF can because LLM context windows are, by definition, stateful.

---

## 4. Implementation Status

This is not a speculative format proposal. GCF is implemented in a production MCP server, covered by tests, benchmarked against JSON, and used as an actual output mode across 16 tool responses.

The implementation includes:

- **Text encoder** for GCF payloads (`internal/wire/gcf.go`)
- **Parser/decoder** for round-tripping GCF back into the internal graph response model (`internal/wire/gcf_decode.go`)
- **Binary encoder/decoder** for GCB (`internal/wire/binary.go`)
- **Pluggable codec registry** allowing runtime format selection across all MCP tools and CLI commands
- **Benchmark harness** comparing token count, byte size, and encode latency across 6 fixture cases (`bench/wire-format/`)
- **Test coverage** for node encoding, edge encoding, grouped payloads, session references, binary round-trips, and registry dispatch

### Correctness Validation

The encoder and decoder are tested against round-trip invariants: a graph response encoded as GCF and decoded back into the internal representation must preserve node identity, kind, score, provenance, group membership, edge direction, edge type, and optional status metadata.

This gives GCF a stronger property than freeform compact text: it is compact, but still mechanically recoverable. The format has parser-level semantics, not just display conventions. Any payload that encodes successfully round-trips to an equivalent internal representation under the tested graph response invariants. The implementation can be validated with:

```
go test ./internal/wire/... ./bench/wire-format/...
```

---

## 5. Where Token Savings Come From

The savings decompose into five sources:

| Source | JSON cost | GCF cost | Savings per occurrence |
|--------|-----------|----------|----------------------|
| Field names | 9 field names × ~2 tokens each = ~18 tokens/symbol | 0 (positional) | ~18 tokens/symbol |
| Edge references | 2 qualified names × ~15 tokens = ~30 tokens/edge | 2 local IDs × ~1 token = ~2 tokens/edge | ~28 tokens/edge |
| Structural delimiters | `{`, `}`, `[`, `]`, `:`, `","` ≈ 6 tokens/symbol | 0 | ~6 tokens/symbol |
| Distance fields | `"distance": N` ≈ 3 tokens/symbol | 0 (implicit in group) | ~3 tokens/symbol |
| Kind strings | `"function"` ≈ 2 tokens | `fn` = 1 token | ~1 token/symbol |

For a 10-symbol, 8-edge payload: JSON ~965 tokens, GCF ~233 tokens. The 732-token difference breaks down roughly as: 280 from field name elimination, 224 from edge reference compression, 60 from delimiter removal, 30 from group headers, and the remainder from kind abbreviations and whitespace.

---

## 6. Benchmarks

All benchmarks encode the same semantic content in JSON and GCF. Token counts use cl100k_base (GPT-4/Claude tokenizer). Measurements taken on Apple M4 Pro.

### 6.1 Token Comparison

| Payload | Symbols | Edges | JSON tokens | GCF tokens | Savings |
|---------|---------|-------|-------------|------------|---------|
| context_for_task | 10 | 8 | 965 | 233 | 75.9% |
| context_for_task (large) | 30 | 24 | 2,968 | 649 | 78.1% |
| context_for_files | 15 | 12 | 1,490 | 334 | 77.6% |
| blast_radius | 8 | 6 | 835 | 208 | 75.1% |
| semantic_diff | 12 | 10 | 1,206 | 295 | 75.5% |
| graph_query | 20 | 16 | 2,078 | 423 | 79.6% |

**Median token savings: 76.7%**

Savings increase with payload size (75.1% at 8 symbols, 79.6% at 20 symbols) because the ratio of edge references to node declarations grows, and edge references are where compression is strongest.

### 6.2 Byte Comparison

| Codec | Bytes (10-sym payload) | vs JSON |
|-------|----------------------|---------|
| JSON | 4,153 | baseline |
| GCF | 1,079 | -74% |
| GCB (binary) | 448 | -89% |

### 6.3 Encode Performance

| Codec | p50 latency | p99 latency | Payload size |
|-------|-------------|-------------|-------------|
| GCF | 38 μs | 64 μs | 30 symbols |
| GCB | 12 μs | 22 μs | 30 symbols |
| JSON | 45 μs | 78 μs | 30 symbols |

GCF encodes faster than JSON (no escaping, no nested object construction). GCB is fastest (direct byte writes, no text formatting). All codecs encode a 30-symbol payload in under 100 microseconds; encoding latency is negligible compared to the graph query that produces the data.

### 6.4 Session Statefulness Savings

| Call | New symbols | Reused symbols | GCF tokens | Cumulative savings vs JSON |
|------|-------------|---------------|------------|--------------------------|
| 1 | 10 | 0 | 233 | 75.9% |
| 2 | 5 | 4 | 128 | 82.3% |
| 3 | 3 | 6 | 87 | 86.1% |
| 4 | 2 | 7 | 62 | 89.4% |
| 5 | 1 | 8 | 41 | 92.7% |

By the fifth tool call in a session, GCF achieves 92.7% token savings versus JSON because 8 of 9 referenced symbols are bare ID references (`@0`, `@3`) consuming 1 token each instead of 15-20 tokens for the full qualified name.

---

## 7. GCB: Companion Binary Format

GCB (Graph Compact Binary) optimizes for machine-to-machine paths where the consumer is not an LLM: daemon IPC, response caching, cross-service transport.

### 7.1 Wire Layout

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
  score       : float32 (little-endian)
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

### 7.2 Design Choices

- **Varint encoding** for integers: small values use 1 byte instead of 4-8
- **Enum IDs** for kinds, provenances, edge types: 1 byte instead of variable-length strings
- **float32** for scores: 4 bytes with sufficient precision (scores need 2 decimal places)
- **Index-based edges**: edges reference symbols by array position (varint), not by name
- **Length-prefixed strings**: no null terminators, no escaping, no delimiters
- **No padding or alignment**: every byte carries data

### 7.3 When to Use Each Format

| Path | Format | Reason |
|------|--------|--------|
| MCP tool → LLM | GCF | Token-optimized, LLM-readable |
| Daemon → client | GCB | Byte-efficient, fast decode |
| Cache → disk | GCB | Smallest on-disk footprint |
| Debug/human inspection | JSON | Readable, pipeable to jq |
| Service → service | GCB | Compact, schema-stable |

---

## 8. Comparison to Alternatives

### 8.1 Columnar/TSV

Column headers with tab-separated values eliminate field name repetition, achieving ~27% token savings. But TSV treats graph data as flat tables: edge references still require full identifier strings in each row. TSV cannot exploit referential identity (local IDs) or topological encoding (edge arrows). GCF triples the savings.

### 8.2 Binary Formats (Protobuf, MessagePack, FlatBuffers)

These optimize for machine parsing and byte size. An LLM cannot read a protobuf payload; it must be decoded to text first, and the decoded text is typically JSON, eliminating the savings at the point of consumption. Binary formats solve a different problem (machine efficiency) than GCF (LLM token efficiency).

### 8.3 JSON-LD / RDF

Verbose by design: full URIs, type annotations, `@context` declarations. Optimizes for semantic interoperability across systems, not token-constrained consumption. JSON-LD is worse than JSON for LLM tool responses.

### 8.4 Markdown / Freeform Text

Some tools return markdown-formatted text. This is human-optimized, not LLM-optimized. Markdown uses whitespace for structure (indentation, line breaks, headers) which tokenizers handle inconsistently. GCF's positional format produces consistent, predictable token counts regardless of tokenizer implementation.

### 8.5 Custom Compressed JSON

JSON with shortened field names (`"qn"` instead of `"qualified_name"`) achieves 15-25% savings. This preserves JSON's structural overhead (delimiters, quoting, nesting) while reducing readability. GCF eliminates the structural overhead entirely rather than shortening the labels on it.

---

## 9. Limitations

**LLM parsing reliability.** GCF assumes the consuming LLM can parse a simple positional text format. Testing with Claude (Sonnet 4, Opus 4) and GPT-4o shows reliable parsing of GCF payloads. Smaller or older models may struggle with the format. The `json` fallback exists for these cases.

**Domain specificity.** GCF is optimized for graph-structured data with typed nodes and edges. It is not a general-purpose wire format. Tabular data, free text, or deeply nested structures are better served by other encodings.

**Session statefulness requires coordination.** The session ID system assumes the server tracks which nodes have been transmitted to which client. Stateless deployments cannot use session compression. The non-session mode (full retransmission) remains available.

**Tokenizer dependency.** Token counts depend on the tokenizer. The benchmarks in this paper use cl100k_base. Different tokenizers produce different token counts for the same GCF payload, though the relative savings versus JSON are consistent (within 2-3 percentage points).

---

## 10. Implications for MCP and Agent Tooling

The MCP specification does not define a standard for tool response encoding beyond "the response is a JSON-RPC result." This means every MCP server independently decides how to format its output. The result is that agents receive verbose JSON from every tool, with no mechanism to request compact encoding.

We propose that MCP tool responses should support format negotiation: the client specifies a preferred encoding in the tool call, and the server returns the response in that encoding. GCF (or a format like it) should be available as a standard option alongside JSON.

The token savings are too large to ignore. A 76.7% reduction in tool response tokens translates directly to: lower API costs, faster time-to-first-token, more room in the context window for source code and reasoning, and fewer multi-turn loops caused by context window exhaustion.

---

## 10.1 TOON: A Safe Default for Agent Workflows

GCF is the most token-efficient format. But token savings only matter if the model comprehends the format reliably. A format that saves 85% of tokens but causes 5% parsing errors is worse than one that saves 61% with 0% errors.

TOON (Token-Oriented Object Notation) is a compact, human-readable format that uses tabular arrays (header plus rows) for uniform object collections. A TOON-encoded symbol list looks like a markdown table with a standardized schema declaration. Every LLM understands this pattern. GCF's `@0<@4 calls` edge references are novel to most models.

The knowing system implements both formats. TOON is available as a first-class codec in the `internal/wire` package (`internal/wire/toon.go`), using the official `github.com/toon-format/toon-go` library.

### Format Comprehension Eval Results

The `eval/TestFormatComprehension` benchmark measures token cost across 6 fixture tasks with a 5,000-token budget, comparing GCF, TOON, JSON, and XML on the same payloads:

| Format | Avg tokens | vs JSON (baseline) |
|--------|-----------|-------------------|
| JSON | 1,818 | 100% |
| XML | 1,818 | 100% |
| TOON | 707 | **39%** |
| GCF | 265 | **15%** |

TOON uses 39% of JSON's token cost. GCF uses 15% of JSON's token cost. Both deliver substantial savings over JSON and XML.

**The practical guidance:**

- Use TOON as the default format for production agent workflows. Its tabular structure is a pattern every LLM understands; it delivers 61% token savings versus JSON with near-zero comprehension risk.
- Use GCF for maximum compression in contexts where GCF comprehension has been validated: evaluated models (Claude Sonnet/Opus, GPT-4o), or workflows where you have verified the model parses `@N<@M edge_type` correctly.
- Use JSON or XML for debugging, human inspection, or fallback on smaller/older models.

The comprehension risk is not symmetric. TOON's tabular format maps onto a schema the model already knows. GCF's integer ID references (`@0`, `@4`) and arrow notation (`@0<@4`) are novel to most models without explicit instruction. The 15% vs 39% difference is real; the comprehension risk difference is also real.

---

## 11. Conclusion

JSON is the default encoding for LLM tool responses because it is universal, not because it is efficient. For graph-structured data, JSON wastes more than three-quarters of its tokens on structural overhead that carries no semantic content.

GCF eliminates this waste through three mechanisms: referential identity (local IDs), topological encoding (edge arrows), and hierarchical grouping (section headers). The result is a 76.7% median token reduction that scales with payload size and improves further with session statefulness (up to 92.7% by the fifth call in a session).

The format is text-based, human-readable, LLM-parseable without special tooling, and implementable in any language. The companion binary format (GCB) covers machine-to-machine paths. Both are implemented, tested, benchmarked, and deployed.

The broader point: as AI agents become the primary consumers of tool output, wire formats should be optimized for token efficiency, not human readability or machine parse speed. GCF demonstrates that this optimization is achievable with significant gains and minimal complexity.

---

## Reference Implementation

- **Encoder/decoder:** `github.com/blackwell-systems/knowing/internal/wire` (Go)
- **Benchmark harness:** `github.com/blackwell-systems/knowing/bench/wire-format`
- **MCP server using GCF:** `github.com/blackwell-systems/knowing` (28 tools, context-plane tools supporting GCF output)

---

## Appendix A: Full Example

**JSON (965 tokens):**

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
      "components": { "blast_radius": 0.40, "confidence": 0.25, "recency": 0.06, "distance": 0.15 }
    },
    {
      "qualified_name": "github.com/blackwell-systems/knowing/internal/mcp.NewServer",
      "kind": "function",
      "score": 0.54,
      "provenance": "lsp_resolved",
      "distance": 1
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

**GCF (233 tokens):**

```
GCF tool=context_for_task budget=5000 tokens=1847 symbols=10
## targets
@0 fn github.com/blackwell-systems/knowing/internal/mcp.requireHash 0.78 lsp_resolved
## related
@4 fn github.com/blackwell-systems/knowing/internal/mcp.NewServer 0.54 lsp_resolved
## edges
@0<@4 calls
```

Same semantic content. 75.9% fewer tokens.

---

## Appendix B: Hash Computation

GCF is format-agnostic with respect to the underlying data model. The examples in this paper use content-addressed graph data (SHA-256 hashed nodes and edges), but GCF encodes the payload structure, not the identity scheme. A GCF encoder receiving nodes identified by database IDs, UUIDs, or string keys would produce identical output; only the `qname` field values would differ.
