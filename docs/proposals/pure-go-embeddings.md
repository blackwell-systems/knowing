# Proposal: Local Embeddings for Retrieval

## Status: SHIPPED. Full corpus +15% P@10, +18.3% R@10. Vector cache: 660ms -> 220ms.

Phase 1 (hugot integration) is shipped and validated on full 167-task corpus.
Phase 2 (custom inference engine) is NO LONGER NEEDED: the "11s/task" number was
total indexing time, not per-query cost. With the SQLite vector cache, re-rank
latency is 220ms (embed 1 query + read 50 cached vectors). Acceptable for
interactive use. See `docs/architecture/embedding-reranker.md` for full details.

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
1. **Re-ranker architecture** (most promising, see below)
2. Better text representation (include docstrings, file context, caller names)
3. Test on repos where BM25 is known to fail (k8s, django SWE tasks)

### Re-ranker Results (VALIDATED, Session 15)

**Flask (19 tasks), jina-embeddings-v2-base-code as re-ranker:**

| Metric | Baseline | Re-ranker | Delta |
|--------|----------|-----------|-------|
| P@10 | 0.332 | 0.347 | **+4.5%** |
| R@10 | 0.447 | 0.521 | **+16.6%** |
| NDCG | 0.632 | 0.615 | -2.7% |
| MRR | 0.775 | 0.681 | -12.1% |
| Latency | 0ms | 11,642ms | (hugot, fixable) |

The re-ranker finds symbols that the graph walk surfaced but ranked too low.
R@10 +16.6% means significantly more ground truth symbols appear in the top 10.
MRR drop is tunable (blend original rank score with embedding similarity instead
of pure re-ordering).

**Critical finding:** The problem with Channel 3 (independent candidate source) was
that it found the SAME symbols as BM25. The re-ranker succeeds because it operates
on a DIFFERENT set (the RWR output, which includes symbols reachable via graph walk
that BM25 would never find by keyword alone). The architecture matters more than the model.

**Next steps:**
1. Blend scoring (0.7 * original_rank + 0.3 * embedding_similarity) to preserve MRR
2. Run on full corpus (need latency fix first)
3. Phase 2 custom engine to reduce 11s -> 1-2s latency

### Re-ranker Architecture

The current integration adds embeddings as Channel 3 (independent signal fused via RRF).
This fails because:
- On well-named codebases (flask), BM25 already finds the right symbols. Embeddings
  add nothing because there's no vocabulary gap to bridge.
- On large codebases (k8s, django), we can only embed 5K/253K symbols due to latency.
  The unreachable symbols we need aren't in the 5K we embedded.
- RRF fusion treats embedding results as equal-weight candidates. But embedding
  similarity is a weak signal for code retrieval (the models aren't code-specific enough).

**Proposed fix: use embeddings as a re-ranker, not a candidate source.**

```
Current (Channel 3, independent):
  BM25 candidates ─┐
  Vector candidates ─┼─> RRF fusion -> RWR walk -> pack
  Path candidates  ─┘

Proposed (re-ranker on BM25 output):
  BM25 candidates -> RWR walk -> top-50 candidates -> embed query + candidates
                                                    -> cosine re-rank -> top-15 -> pack
```

Why this could work:
1. No need to pre-embed all symbols (embed only the ~50 RWR candidates at query time)
2. 50 embeddings x 14ms = 700ms (acceptable if results improve significantly)
3. The model scores relevance between the task description and each candidate's text
4. This catches cases where RWR surfaced the right neighborhood but ranked wrong within it
5. Works with any model (BGE, jina, nomic) since it's pairwise similarity, not retrieval

Why it might not work:
- If the correct symbols aren't in the top-50 RWR candidates at all (true unreachability),
  re-ranking can't help. Only an independent candidate source can.
- 700ms latency overhead might be unacceptable for interactive use (but fine for batch/CI)

**Implementation cost:** ~50 LOC change in `packIntoBudget` or a new `reRankWithEmbeddings`
step between RWR scoring and packing. No model change needed, no pre-indexing needed.

### Tested models (all neutral as Channel 3)

| Model | Type | Dims | Flask P@10 | Delta |
|-------|------|------|-----------|-------|
| BAAI/bge-small-en-v1.5 | General retrieval | 384 | 0.332 | 0% |
| jinaai/jina-embeddings-v2-base-code | Code-specific | 768 | 0.332 | 0% |
| nomic-ai/nomic-embed-text-v1.5 | Code-aware | 768 | 0.332 | 0% |

All three are neutral because the integration architecture (independent channel) is wrong
for this problem, not because the models are bad.

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
