# Wire Format Benchmark: GCF vs JSON vs Binary

**Auto-generated from test run. Do not edit manually.**

## Thesis

The Graph Context Format (GCF) reduces token consumption for LLM-facing output
by replacing verbose JSON keys with a compact, line-oriented format. Binary
encoding (GCB) minimizes byte size for transport/storage.

## Methodology

Six YAML fixture files define realistic context payloads (varying sizes, edge counts).
Each payload is encoded with all three codecs. Token counts use a word+punctuation
heuristic (~0.85 correlation with cl100k_base for structured text). Byte sizes are
measured directly.

## Results

| Case | JSON (bytes) | GCF (bytes) | Binary (bytes) | JSON (tokens) | GCF (tokens) | GCF Savings | Binary Savings |
|------|-------------|-------------|----------------|---------------|--------------|-------------|----------------|
| 01_context_for_task_small | 5823 | 1079 | 1354 | 1347 | 233 | 82.7% | 76.7% |
| 02_context_for_task_medium | 18780 | 3299 | 4902 | 4132 | 649 | 84.3% | 73.9% |
| 03_context_for_files | 9205 | 1701 | 2412 | 2056 | 334 | 83.8% | 73.8% |
| 04_blast_radius | 5447 | 983 | 1395 | 1223 | 208 | 83.0% | 74.4% |
| 05_semantic_diff | 8406 | 1445 | 2039 | 1868 | 295 | 84.2% | 75.7% |
| 06_graph_query | 12814 | 2318 | 3828 | 2782 | 423 | 84.8% | 70.1% |

**Overall GCF token savings:** 84.0%
**Overall binary byte savings:** 73.7%
**Median GCF token savings:** 84.0% (target: >= 35%)
**Median binary byte savings:** 74.1% (target: >= 70%)

## Interpretation

### Why GCF saves 80%+ tokens

JSON's verbosity comes from repeated keys (`qualified_name`, `provenance`, `components`),
nested braces, and quoted strings. GCF uses a header line followed by positional fields
separated by `|`. This eliminates key repetition entirely. Edge references use local
integer IDs (`$1 -> $3`) instead of repeating full qualified names.

### Why binary saves 70%+ bytes

GCB uses varint encoding for integers, length-prefixed strings without JSON escaping,
and a flat binary layout with no structural overhead (no braces, no commas, no whitespace).
The savings come from eliminating formatting characters that represent ~30% of JSON output.

### What this means for agent workflows

An agent consuming context at 3000 tokens/response saves ~2500 tokens per call with GCF.
Over a 10-call session, that's 25K tokens saved from the context window, freeing capacity
for source code and tool output. The format is designed to be LLM-parseable (line-oriented,
no ambiguous nesting) while maximizing information density.

## Additional Guarantees

- Round-trip integrity: encode -> decode -> re-encode produces identical output for all codecs
- No case where GCF uses MORE tokens than JSON (monotonically better)
- No case where binary uses MORE bytes than JSON (monotonically better)
- p99 encode latency < 1ms for all codecs on all fixtures

## Reproducibility

```bash
GOWORK=off go test ./bench/wire-format/ -v -count=1
```
