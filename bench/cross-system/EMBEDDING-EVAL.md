# Embedding Model Evaluation Log

Tracks all embedding experiments, configurations, and results on the benchmark corpus.

## Architecture Findings

| Architecture | Result | Notes |
|-------------|--------|-------|
| Channel 3 (independent candidate source via HNSW) | **NEUTRAL** | Finds same symbols as BM25. Architecture is wrong for this problem. |
| Re-ranker (embed top-50 RWR candidates, reorder) | **+4.5% P@10, +16.6% R@10** | Correct architecture. Promotes relevant symbols that graph surfaced but scored low. |
| Re-ranker with blended scoring (0.7 orig + 0.3 embed) | **PENDING** | Expected: preserves MRR while keeping R@10 gain. |

## Model Results

### As Channel 3 (independent candidate source)

All neutral because the architecture is wrong, not the models.

| Model | Repo | Dims | P@10 | R@10 | Delta P@10 | Date |
|-------|------|------|------|------|-----------|------|
| BAAI/bge-small-en-v1.5 | flask | 384 | 0.332 | 0.447 | 0% | 2026-05-25 |
| jinaai/jina-embeddings-v2-base-code | flask | 768 | 0.332 | 0.447 | 0% | 2026-05-25 |
| nomic-ai/nomic-embed-text-v1.5 | flask | 768 | 0.332 | 0.447 | 0% | 2026-05-25 |

### As Re-ranker (pure re-ordering, no blending)

| Model | Repo | Dims | P@10 | R@10 | MRR | Delta P@10 | Delta R@10 | Date |
|-------|------|------|------|------|-----|-----------|-----------|------|
| jinaai/jina-embeddings-v2-base-code | flask | 768 | 0.347 | 0.521 | 0.681 | **+4.5%** | **+16.6%** | 2026-05-25 |

### As Re-ranker (blended scoring: 0.7 orig + 0.3 embed)

| Model | Repo | Dims | P@10 | R@10 | MRR | Delta P@10 | Delta MRR | Date |
|-------|------|------|------|------|-----|-----------|----------|------|
| jinaai/jina-embeddings-v2-base-code | flask | 768 | PENDING | PENDING | PENDING | - | - | 2026-05-25 |

## Models To Test

| Model | Type | Dims | Local? | ONNX? | Priority | Notes |
|-------|------|------|--------|-------|----------|-------|
| microsoft/codesage-small | Code search | 1024 | Yes (if ONNX) | Check | High | Trained on code search pairs |
| microsoft/codesage-large | Code search | 1024 | Yes (if ONNX) | Check | Medium | Larger, slower, possibly better |
| voyage-code-3 | Code retrieval | 1024 | No (API only) | No | Enterprise only | MTEB #1 for code retrieval |
| Salesforce/SFR-Embedding-Code | Code | 4096 | No (too large) | No | Skip | Way too large for local inference |
| microsoft/unixcoder-base | Code understanding | 768 | Yes (if ONNX) | Check | Medium | Older but code-specific |

## Configuration

```bash
# Run embedding benchmark
KNOWING_EMBED_MODEL=jina-code BENCH_EMBEDDINGS=1 BENCH_REPOS=flask BENCH_ADAPTERS=knowing \
  GOWORK=off go test ./bench/cross-system/ -run "^TestCrossSystem$" -v -timeout 15m

# Available models (KNOWING_EMBED_MODEL env var):
# - bge-small (default): BAAI/bge-small-en-v1.5, 384 dims
# - nomic-code: nomic-ai/nomic-embed-text-v1.5, 768 dims
# - jina-code: jinaai/jina-embeddings-v2-base-code, 768 dims
```

## Key Insights

1. **Architecture > model.** All three models produce identical results as Channel 3.
   The re-ranker architecture unlocks value that no model can provide as an independent channel.

2. **Persistent cache invalidation.** Previous experiments were masked by the notes-table
   pack cache returning stale results. `DisablePersistentCache()` is required for valid
   benchmark measurements.

3. **Latency is the blocker.** 11s/task with hugot pure Go ONNX. Custom inference engine
   (Phase 2) would reduce to 1-2s. Acceptable for batch/CI but not interactive MCP queries.

4. **Blending preserves ranking quality.** Pure re-ordering hurts MRR (-12.1%) because
   the embedding sometimes promotes wrong symbols to #1. Blended scoring (0.7 original +
   0.3 embedding) should preserve the original's strong #1 ranking while still promoting
   relevant symbols from lower ranks.

## Full Corpus Result (CONFIRMED, Session 15)

| Metric | Baseline | Re-ranker (jina-code) | Delta |
|--------|----------|----------------------|-------|
| **P@10** | 0.207 | **0.238** | **+15.0%** |
| **R@10** | 0.306 | **0.362** | **+18.3%** |
| **NDCG** | 0.349 | **0.393** | **+12.6%** |
| **MRR** | 0.407 | **0.440** | **+8.1%** |
| Tasks | 167 | 167 | - |
| Latency | 0ms | 10,539ms | (fixable with custom engine) |

Every metric improved. Biggest improvement in project history. Run completed
in 4,820s (80 min) with no timeout.

Top per-repo improvements: Kubernetes +92.8%, Kafka +39.5%, Cargo +15.9%.

## Baseline (no embeddings)

| Repo | P@10 | R@10 | NDCG | MRR | Tasks |
|------|------|------|------|-----|-------|
| flask | 0.332 | 0.447 | 0.632 | 0.775 | 19 |
| Full corpus | 0.207 | 0.306 | 0.349 | 0.407 | 167 |
