# Embedding Architecture (Re-ranker Disabled, Gap-Fill Active)

> **Status (session 19):** The embedding re-ranker has been disabled after per-repo
> A/B testing showed it is net negative on P@10 (9/13 repos hurt, net -0.050).
> The +17% improvement previously attributed to re-ranking was actually from gap-fill
> seeds sharing the same `BENCH_EMBEDDINGS=1` flag. Gap-fill seeds remain active
> and provide +11% P@10 by bridging vocabulary gaps. The embedding infrastructure
> (model download, vector cache, PreloadVectors) is still used for gap-fill.
> Code is preserved for future investigation with code-tuned models.

The embedding re-ranker was a post-RWR stage that reordered the top-50 candidates
by cosine similarity to the task description. It uses a pre-trained code embedding
model (nomic-embed-text-v1.5, default) running locally via pure-Go ONNX inference.
No API calls, no cloud services, no charges.

**Impact:** P@10 0.207 -> 0.247 (+19%), R@10 0.306 -> 0.380 (+24%) on the
full 167-task cross-system benchmark. Every metric improved. Biggest single
improvement in project history.

## Architecture

The re-ranker sits between scoring (step 7) and budget packing (step 8) in the
retrieval pipeline:

```
[7. Scoring]           6-component formula
    |
    v
[7b. Embedding Re-rank] embed query, cosine-sort top-50 candidates
    |
    v
[8. Budget Packing]    density-ranked greedy knapsack
```

### Why re-ranking, not independent search

Three models (BGE, jina-code, nomic) were tested as an independent Channel 3
(embed query, HNSW search, RRF-fuse with graph results). All produced identical
P@10 to baseline. The models find the same symbols as BM25 because the
vocabulary gap between task descriptions and symbol names is a structural
problem, not a model quality problem.

Re-ranking works because the graph already surfaces relevant candidates via
structural relationships (calls, imports, type membership). The embedding model
then promotes candidates whose textual description is semantically close to the
task, even when their graph score was low. The architecture matters more than
the model.

### Why pure re-rank (weight=0.0)

A parameter sweep tested blended scoring at weights 0.0, 0.5, 0.7, 0.85, 0.95,
1.0 (where weight = original score contribution). Pure re-rank (weight=0.0)
produced the best P@10 and R@10 on the full corpus. Higher weights preserve MRR
(rank of the single best result) but sacrifice recall. Since consumers read all
10 results (not just #1), pure re-rank is the correct default.

Configurable via `ReRankOriginalWeight` in `internal/context/walk.go`.

## Vector Cache

Embedding inference is the bottleneck: ~13ms per text in batch, ~660ms for the
full 51-text re-rank call (1 query + 50 candidates). The vector cache eliminates
redundant inference by storing pre-computed vectors in SQLite.

### How it works

```
Index time:
  IndexBatch(nodes) -> EmbedBatch(texts) -> store vectors in HNSW + SQLite

Re-rank time (cached):
  ReRankByHashes(query, nodeHashes, fallbackTexts)
    1. Embed query (1 text, ~120ms)
    2. Read 50 vectors from SQLite by node_hash (~100ms)
    3. Compute 50 cosine similarities (~0.006ms)
    Total: ~220ms

Re-rank time (uncached, first run):
  ReRankByHashes(query, nodeHashes, fallbackTexts)
    1. Embed query (1 text)
    2. Cache miss on all hashes
    3. Embed 50 fallback texts (~540ms)
    4. Persist vectors to SQLite for next time
    Total: ~660ms (same as old path, but only happens once)
```

### Storage

The `embeddings` table stores vectors keyed by (node_hash, model):

```sql
CREATE TABLE embeddings (
    node_hash  BLOB NOT NULL,
    model      TEXT NOT NULL,
    vector     BLOB NOT NULL,
    PRIMARY KEY (node_hash, model)
);
```

Vectors are serialized as little-endian float32 arrays. At 768 dimensions
(jina-code), each vector is 3072 bytes. Storage overhead for typical repos:

| Repo size | Nodes embedded | Cache size |
|-----------|---------------|------------|
| Small (1K nodes) | 1,000 | ~3 MB |
| Medium (5K nodes) | 5,000 | ~15 MB |
| Large (50K nodes) | 50,000 | ~150 MB |

The `model` column allows multiple models to coexist. Switching models
(via `KNOWING_EMBED_MODEL`) does not invalidate vectors from other models.

### Cache lifecycle

- **Population:** `IndexBatch` writes vectors on every index run. Re-indexing
  updates vectors for changed nodes (UPSERT).
- **Cache miss:** `ReRankByHashes` falls back to on-the-fly embedding and
  auto-persists the result. First query after a fresh index is uncached;
  all subsequent queries hit cache.
- **Invalidation:** Node hashes are content-addressed. When a symbol changes,
  its hash changes, so stale vectors are naturally orphaned. Old vectors from
  deleted nodes remain but are never queried (no hash match).

## Latency Profile

Measured on Apple Silicon, nomic-code (768 dims), hugot v0.7.2 pure-Go ONNX.

| Operation | Time |
|-----------|------|
| Model load (one-time) | 73ms |
| Single embed | 120ms |
| Batch 51 (old re-rank, no cache) | 660ms |
| **Cached re-rank (embed 1 + SQLite read)** | **220ms** |
| 50x cosine similarity (768-dim) | 0.006ms |

All time is ONNX inference (tokenization + forward pass). Cosine computation
is negligible. The cached path is 3x faster than uncached.

### Why not GPU acceleration

CoreML/Metal support exists in hugot via the ORT (ONNX Runtime) backend, but
it requires CGO and a platform-specific shared library (`libonnxruntime.dylib`).
This would break knowing's zero-runtime-deps guarantee. With the vector cache,
the remaining inference cost is a single query embedding (~120ms), which is
acceptable for interactive use. GPU acceleration would save ~90ms at the cost of
portability.

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `--embeddings` | off | Enable embedding re-ranker on `knowing mcp` |
| `--embed-model` | nomic-code | Model: `nomic-code` (default), `jina-code`, `bge-small` |
| `KNOWING_EMBED_MODEL` | nomic-code | Env var (CLI flag takes precedence) |
| `BENCH_EMBEDDINGS` | 0 | Enable in benchmark adapter |
| `BENCH_RERANK_WEIGHT` | 0.0 | Override `ReRankOriginalWeight` in benchmarks |
| `ReRankOriginalWeight` | 0.0 | Blend weight (0.0 = pure re-rank, 1.0 = no re-rank) |

## Code Map

| File | Purpose |
|------|---------|
| `internal/embedding/embedding.go` | `Embedder`: hugot session, model loading, `Embed`/`EmbedBatch` |
| `internal/embedding/searcher.go` | `Searcher`: HNSW index, `IndexBatch`, `ReRank`, `ReRankByHashes`, vector cache |
| `internal/context/context.go` | `VectorReRanker` interface, `reRankWithEmbeddings` call site |
| `internal/context/walk.go` | `ReRankOriginalWeight` (blend parameter) |
| `internal/store/migrations/019_add_embeddings.sql` | Schema for vector cache |
| `internal/store/sqlite.go` | `BatchPutEmbeddings`, `GetEmbeddings` |
| `bench/cross-system/EMBEDDING-EVAL.md` | Evaluation log: all experiments, results, latency data |
| `docs/proposals/pure-go-embeddings.md` | Original proposal and discovery narrative |

## Key Findings

1. **Architecture > model.** Three models produced identical results as
   independent search. The re-ranker architecture unlocked value that no model
   switch could provide.

2. **Pure re-rank beats blending.** Weight=0.0 is optimal because agents consume
   all 10 results, not just the top-1. Blending preserves MRR at the cost of
   recall.

3. **Cache eliminates the latency problem.** The "11s/task" number that
   originally motivated a custom inference engine was total indexing time, not
   per-query cost. With cached vectors, re-rank is 220ms per query.

4. **Persistent pack cache must be disabled for experiments.** The notes-table
   cache returns stale results, masking all delta measurements. Always call
   `DisablePersistentCache()` in benchmarks.

5. **Embeddings help most where graph connectivity is sparse.** Kubernetes
   (+92.8%) and Kafka (+39.5%) saw the largest gains in session 15. Initial
   regressions on VS Code (-16%) and Ocelot (-30.8%) reported in session 15
   were not reproducible in session 16 testing: both repos showed 0% P@10
   delta with neutral-to-positive NDCG and MRR improvements. The regressions
   were likely artifacts of the pre-vector-cache build.
