# Proposal: Pure Go Transformer Inference for Local Embeddings

## Motivation

The retrieval pipeline's remaining bottleneck is the 45.6% of ground truth symbols that are
unreachable from keyword seeds. Vector similarity (embeddings) bypasses the graph entirely:
"custom migration operation" embeds close to `Operation.state_forwards` regardless of graph
connectivity or keyword matching.

The distribution constraint: knowing is a single Go binary with zero runtime dependencies.
Any embedding solution must preserve this property.

## Proposal

Implement MiniLM-L6-v2 (or nomic-embed-text-v1.5) inference in pure Go. No CGO, no ONNX
runtime, no Python sidecar. The model weights are loaded from a file on first use
(downloaded once to `~/.knowing/models/`).

## Model Selection

| Model | Params | Dims | Size | Expected latency (pure Go) |
|-------|--------|------|------|---------------------------|
| all-MiniLM-L6-v2 | 22M | 384 | 88MB | ~5-10ms/embedding |
| nomic-embed-text-v1.5 | 137M | 768 | 548MB | ~50-100ms/embedding |
| snowflake-arctic-embed-xs | 22M | 384 | 88MB | ~5-10ms/embedding |

Recommend: **all-MiniLM-L6-v2** (smallest, fastest, well-benchmarked on code retrieval).

## Architecture

```
~/.knowing/models/minilm-l6-v2.bin   (88MB, downloaded on first `knowing index --embed`)
                    |
                    v
internal/embedding/inference.go       (pure Go transformer forward pass)
    - LoadModel(path) -> *Model
    - Model.Embed(text) -> []float32 (384-dim)
    - Model.EmbedBatch(texts) -> [][]float32
                    |
                    v
internal/embedding/searcher.go        (existing: ANN index over embedded symbols)
    - Searcher.EmbedAndSearch(query, k) -> []Hash
```

## Implementation Plan

### Layer 1: Tokenizer (~200 LOC)
- WordPiece tokenizer with 30K vocab (embedded via `go:embed` or loaded from model file)
- `Tokenize(text) -> []int` (token IDs)
- Special tokens: [CLS], [SEP], padding

### Layer 2: Tensor operations (~300 LOC)
- `type Tensor struct { Data []float32; Shape []int }`
- `MatMul(a, b Tensor) Tensor` (naive triple loop, sufficient for 384x384)
- `LayerNorm(x, gamma, beta Tensor) Tensor`
- `GELU(x Tensor) Tensor`
- `Softmax(x Tensor) Tensor`
- `MeanPool(x Tensor, mask []bool) Tensor`

### Layer 3: Transformer forward pass (~200 LOC)
- `type TransformerLayer struct { QKV, Out, FF1, FF2, LN1, LN2 weights }`
- `func (l *TransformerLayer) Forward(x Tensor) Tensor`
- Multi-head attention: split heads, Q*K^T/sqrt(d), softmax, *V, concat, project
- Feed-forward: Linear(384->1536) -> GELU -> Linear(1536->384)

### Layer 4: Model loading (~200 LOC)
- Parse SafeTensors or custom binary format
- Map weight names to layer structs
- Validate shapes on load

### Layer 5: Integration
- `ContextEngine.SetVector(vs VectorSearcher)` already exists
- `internal/embedding/Searcher` already implements `EmbedAndSearch`
- Wire: on index, embed all symbol signatures. On query, embed task description.
- Store embeddings in a separate file (`~/.knowing/embeddings.bin`) or notes table.

## Performance Expectations

- **Embedding one query**: ~5-10ms (6 layers x 384x384 matmul, M-series Mac)
- **Indexing 10K symbols**: ~50-100s (batch, amortized). Run once at index time.
- **ANN search (HNSW)**: ~1ms for top-30 from 100K vectors
- **Total query overhead**: ~6-11ms (embed query + ANN search). Negligible vs current 2ms.

## Binary Size Impact

- Model weights: 88MB (stored externally in `~/.knowing/models/`, not in binary)
- Inference code: ~5KB compiled Go
- Tokenizer vocab: ~2MB (embedded via go:embed or in model file)
- **Net binary size increase: ~2MB** (vocab only; model downloaded separately)

## Distribution

```bash
# First time: downloads model (~88MB, one-time)
knowing index --embed /path/to/repo

# Subsequent: uses cached model
knowing mcp --watch  # embeddings auto-update on file change
```

Model download from GitHub releases or CDN. Checksum verification via SHA-256.
No network calls during normal operation (model is local).

## Alternatives Considered

| Approach | Pros | Cons |
|----------|------|------|
| **Pure Go (this proposal)** | Single binary, zero deps, novel | 2-3 day effort, no precedent |
| Sidecar process | Ships faster (1 day) | Two binaries, Python/Rust dep |
| ONNX via CGO | Fast inference, proven | Breaks cross-compile, +50MB binary |
| External API (Ollama) | Zero code | Requires running server |

## Success Criteria

- P@10 improvement of +5-15% on full corpus (bridging unreachable symbols)
- Query latency < 15ms total (embed + search + existing pipeline)
- Binary size increase < 5MB (model is external)
- Zero CGO, zero external dependencies
- Works on macOS arm64, Linux amd64, Linux arm64

## Timeline

- Day 1: Tokenizer + tensor ops + basic forward pass (inference works)
- Day 2: Model loading + integration with ContextEngine + index-time embedding
- Day 3: ANN index (HNSW in pure Go) + query-time search + benchmark
- Buffer: 1 day for optimization if >20ms latency

## References

- MiniLM paper: Wang et al., "MiniLM: Deep Self-Attention Distillation" (2020)
- Existing infrastructure: `internal/embedding/searcher.go`, `internal/embedding/embedder.go`
- Session 14 finding: 45.6% unreachable symbols need non-graph signal
