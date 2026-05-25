# Proposal: Local Embeddings for Retrieval

## Status: Phase 1 COMPLETE, Phase 2 ON HOLD

Phase 1 (hugot integration) is implemented and wired into the retrieval pipeline.
Phase 2 (custom inference engine) is ON HOLD pending a better embedding strategy.

### Embedding Benchmark Results (Session 15, 2026-05-25)

**Finding: BGE-small-en-v1.5 with current text representation is NEUTRAL for P@10.**

Ran the full corpus (167 tasks, 9 repos) with BENCH_EMBEDDINGS=1, capping at 5,000
symbols per repo. Results across ~80 tasks completed before timeout:
- P@10 scores are identical to baseline (0.207-0.208)
- No per-task improvement observed
- Embedding latency adds ~0ms to retrieval (HNSW search is <1ms)
- Index time adds ~70s per repo (5000 x 14ms)

**Why neutral:** The BGE model was trained on general text similarity, not code symbol
retrieval. The `buildEmbedText` function produces text like "function Transitive Callers
in package context with signature (ctx, task) ([]Hash, error)" which embeds reasonably
but doesn't differentiate between code symbols better than BM25 keyword matching already
does. The vocabulary gap between task descriptions ("refactor auth middleware") and
symbol names ("SessionHandler", "TokenValidator") requires a code-specific embedding
model, not a general-purpose one.

**What would move the needle:**
1. A code-retrieval-tuned model (CodeBERT, UniXcoder, or fine-tuned BGE on code pairs)
2. Better text representation (include docstrings, file context, caller names)
3. Hybrid scoring (use embedding similarity as a re-ranker on BM25 candidates, not as
   a standalone channel)

**Decision:** Keep the infrastructure (it works mechanically). Revisit when a
code-retrieval-specific model is available in a size suitable for local inference.
The custom Phase 2 engine is not worth building until the embedding signal itself
proves valuable.

## Motivation

The retrieval pipeline's remaining bottleneck is the 45.6% of ground truth symbols that are
unreachable from keyword seeds. Vector similarity (embeddings) bypasses the graph entirely:
"custom migration operation" embeds close to `Operation.state_forwards` regardless of graph
connectivity or keyword matching.

The distribution constraint: knowing is a single Go binary with zero runtime dependencies.
Any embedding solution must preserve this property.

## Current State (Phase 1: Working)

The embedding system is **already implemented** and gated behind `KNOWING_EMBEDDINGS=1`:

- `internal/embedding/embedding.go`: hugot (pure Go ONNX runtime) + BGE-small-en-v1.5
- `internal/embedding/searcher.go`: HNSW index (coder/hnsw), VectorSearcher interface
- Integration: MCP server calls `SetVector()`, Channel 3 in retrieval pipeline runs EmbedAndSearch
- Builds with `CGO_ENABLED=0` (hugot is pure Go, no CGO needed)
- Model: BAAI/bge-small-en-v1.5, 384 dims, ~30MB, auto-downloaded on first use

### Measured Performance (M4 Pro, 2026-05-25)

| Operation | Latency |
|-----------|---------|
| Single embedding (cold start) | 200ms |
| Single embedding (warm) | ~20ms |
| Batch of 8 | 135ms (16.8ms/embedding) |
| Batch of 64 | 880ms (13.8ms/embedding) |
| HNSW search (top-30 from 5K vectors) | <1ms |

### Existing Libraries

| Library | Stars | Status | Notes |
|---------|-------|--------|-------|
| `knights-analytics/hugot` | 606 | Active (last push 2026-05-24) | Pure Go ONNX runtime, used in Phase 1 |
| `nlpodyssey/cybertron` | 329 | Maintained (last push 2024-06) | Full transformer inference in Go (BERT, BART, etc.) |
| `nlpodyssey/spago` | 1,850 | Active (last push 2025-04) | ML framework that cybertron builds on |
| `coder/hnsw` | - | Active | Pure Go HNSW index, used in Phase 1 |

## Decision: Two-Phase Approach

### Phase 1: Validate with hugot (DONE)

Use hugot (pure Go ONNX) to prove the retrieval improvement. Already implemented.
Gated behind `KNOWING_EMBEDDINGS=1` to keep it opt-in until P@10 delta is validated.

Remaining work:
- Run full benchmark corpus with embeddings enabled
- Measure P@10 delta vs baseline (0.207)
- If improvement confirmed, make embeddings default (remove env gate)

### Phase 2: Custom inference engine (IF Phase 1 succeeds)

Replace hugot with a minimal custom forward pass. Justification:

1. **Performance.** hugot is ~14ms/embedding (pure Go, unoptimized matmul). Custom with
   SIMD (ARM NEON / x86 AVX) could achieve 3-5ms. With INT8 quantization: 2-3ms.
2. **Binary size.** hugot brings a full ONNX runtime (~15MB in binary). Custom inference
   for a single model would be ~5KB compiled + 2MB vocab.
3. **Concurrency.** hugot serializes all inference behind a mutex. Custom code can run
   independent embeddings on separate goroutines.
4. **Dependency risk.** hugot is maintained but external. Custom code is fully owned.

The model is small enough (6 layers, 384 dims, 22M params) that custom implementation
is ~700 LOC total. The hard part (proving it works in Go) is already validated by hugot,
cybertron, and spago.

### Phase 2 Architecture

```
~/.knowing/models/bge-small-en-v1.5.safetensors  (30MB, downloaded on first use)
                    |
                    v
internal/embedding/inference.go       (custom transformer forward pass, ~500 LOC)
    - LoadModel(path) -> *Model
    - Model.Embed(text) -> []float32 (384-dim)
    - Model.EmbedBatch(texts) -> [][]float32
                    |
                    v
internal/embedding/tokenizer.go       (WordPiece, ~150 LOC)
    - Tokenize(text) -> []int
                    |
                    v
internal/embedding/simd_arm64.s       (optional: NEON matmul for M-series)
internal/embedding/simd_amd64.s       (optional: AVX2 matmul for x86)
```

### Phase 2 Performance Targets

| Operation | hugot (current) | Custom (target) |
|-----------|----------------|-----------------|
| Single embed | 14ms | 3-5ms |
| Batch of 64 | 880ms | 200-300ms |
| Index 5K symbols | 70s | 15-25s |
| Index 50K symbols | 700s | 150-250s |
| Query overhead | 15ms | 4-6ms |

## Indexing Strategy

### Full index (first time)
Embed all symbols. For large repos, cap at most-connected symbols (high edge count
implies importance). Persist vectors to `~/.knowing/embeddings.bin`.

### Incremental index (file change)
Only embed new/modified symbols. HNSW supports dynamic insertion. Remove old vectors
for changed symbols, insert new ones. Typical cost: 10-50 symbols x 14ms = 140-700ms.

### Benchmark mode
Cap at 5,000 symbols per repo for feasibility. 5000 x 14ms = 70s/repo, ~10 min total.
Sufficient to validate P@10 impact.

## Success Criteria

- P@10 improvement of +3-15% on full corpus (bridging unreachable symbols)
- Query latency < 20ms total (embed + search + existing pipeline)
- Binary size increase < 5MB (model is external, downloaded on first use)
- Zero CGO, zero external dependencies at runtime
- Works on macOS arm64, Linux amd64, Linux arm64

## References

- BGE paper: Xiao et al., "C-Pack: Packaged Resources To Advance General Chinese Embedding" (2023)
- MiniLM paper: Wang et al., "MiniLM: Deep Self-Attention Distillation" (2020)
- Existing code: `internal/embedding/embedding.go`, `internal/embedding/searcher.go`
- Session 14 finding: 45.6% unreachable symbols need non-graph signal
- Session 15 finding: hugot works in pure Go, 14ms/embedding, no CGO needed
