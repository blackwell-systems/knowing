# Configuration Reference

All environment variables, CLI flags, and MCP server options in one place.

## Runtime Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KNOWING_DB` | `.knowing/graph.db` | Override default database path for all subcommands |
| `KNOWING_NO_FEEDBACK` | off | Disable implicit feedback (noise demotion) in MCP server. Set to `1` to disable. |
| `KNOWING_EMBEDDINGS` | off | Enable embedding gap-fill in MCP server. Set to `1` to enable. |

## MCP Server Flags

| Flag | Env Equivalent | Description |
|------|---------------|-------------|
| `--no-feedback` | `KNOWING_NO_FEEDBACK=1` | Disable implicit feedback (noise demotion). Useful for A/B testing. |
| `--embeddings` | `KNOWING_EMBEDDINGS=1` | Enable embedding gap-fill (off by default, confirmed neutral on cold start). |
| `--no-enrich` | n/a | Skip LSP enrichment during `--watch` reindex. Saves ~14s per package. |

## Benchmark Environment Variables

### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `BENCH_REPOS` | all | Filter to a single repo for fast iteration (e.g., `django`, `cargo`). |
| `BENCH_ADAPTERS` | all installed | Filter to specific adapters (e.g., `knowing`). Skip competitors. |
| `BENCH_EMBEDDINGS` | off | Enable embedding gap-fill. Required for vector re-rank experiments. |
| `BENCH_PARALLEL` | off | Parallel repo execution. ~4x faster but ~0.022 lower P@10 from ONNX CPU contention. |
| `BENCH_DEBUG_ZEROS` | off | Log ground truth + returned top 10 for every zero-scoring task. |

### Retrieval Tuning (diagnostic, compose freely)

| Variable | Default | Description |
|----------|---------|-------------|
| `BENCH_EXCLUDE_EDGES` | none | Comma-separated edge types to exclude from RWR walk (e.g., `similar_to,type_hint_of`). |
| `BENCH_BFS_DEPTH` | 4 | Limit RWR walk depth. |
| `BENCH_PREFER_TYPE_SEEDS` | off | Force type-seed preference regardless of graph size. |
| `BENCH_HUB_DAMPEN` | off | Penalize nodes with in-degree above this threshold. |
| `BENCH_MAX_SEEDS` | auto | Override maximum seed count (default: density-adaptive). |
| `BENCH_ADAPTIVE_SEEDS` | on for >10K nodes | Enable adaptive seed count based on graph density. |
| `BENCH_RERANK_WEIGHT` | 0.0 | Blend weight for embedding re-rank (0.0 = disabled, confirmed net negative). |
| `BENCH_COHERENCE_BONUS` | 0.0 | File-based packing coherence bonus (confirmed harmful). |
| `BENCH_GAP_THRESHOLD` | 5 | Gap-fill activation threshold (seeds needed before gap-fill kicks in). |
| `BENCH_FOCUSED_SEEDS` | on | Focused seed selection. Set to `0` to disable for A/B testing. |
| `BENCH_PROXIMITY_EXP` | 0.3 | Proximity exponent for RWR packing (`density * rwrScore^exp`). Higher = stronger proximity preference. |
| `BENCH_LSP_EDGE_WEIGHT` | 0.3 | Weight multiplier for LSP-enriched edges (`lsp_resolved` provenance) in RWR walk. Default 0.3 attenuates enrichment edges to prevent centrality inflation. Validated: enriched saleor +19.8%, full corpus neutral. |

### Implicit Feedback / Compounding

| Variable | Default | Description |
|----------|---------|-------------|
| `BENCH_IMPLICIT_FEEDBACK` | off | Enable implicit feedback (noise demotion) in benchmarks. Required for compounding tests. |
| `BENCH_COMPOUND_ROUNDS` | 5 | Number of rounds for `TestCompounding`. |
| `BENCH_FEEDBACK_WEIGHT` | `none` | Confidence weighting mode for feedback scores. Options: `none` (raw score, default), `sqrt` (symmetric sqrt confidence), `linear` (steeper linear confidence), `asym` (full-strength positives, sqrt-weighted negatives only). |
| `BENCH_PACK_STRATEGY` | `density` | Packing algorithm for context assembly. Options: `density` (score/tokens * RWR proximity, default), `file-grouped` (pack densest files first), `top-k` (highest score first). |

### Vocabulary Expansion

Vocabulary expansion learns keyword -> symbol associations from agent usage.
When an agent asks "how does checkout work?" and then opens `Order.can_cancel`,
the system records `checkout -> can_cancel`. After 2+ observations of the same
association, it becomes a learned equivalence class that bridges vocabulary gaps
on future queries.

**How it works in benchmarks:**
1. Set `BENCH_IMPLICIT_FEEDBACK=1` to enable implicit feedback
2. The compounding test (`TestCompounding`) simulates agent usage by feeding
   ground truth symbol names into `DetectUsed` after each task
3. `recordImplicitFeedback` fires on the next `ForTask`, recording vocab
   associations for positively attributed symbols
4. Rounds 2+ benefit from learned associations

**How it works in production (MCP):**
1. Agent calls `context_for_task` (returns symbols)
2. Agent uses some symbols (opens files, edits code)
3. `DetectUsed` scans tool call content for symbol references
4. On the next `context_for_task`, `FlushAll` records vocab associations
5. Future queries benefit from learned keyword -> symbol mappings

**Storage:** `vocab_associations` table with `(keyword, symbol_hash)` unique
constraint and `count` column for reinforcement. Migration 021.

**Threshold:** `count >= 2` required for a learned association to activate.
Single observations are ignored to prevent noise.

**Noise filter:** Common English words (~80 words like "use", "not", "find",
"whether") are filtered from recording via `isVocabWorthy`. Only domain-specific
keywords (>= 4 chars, not in noise/stop word lists) create associations. Preview
with `knowing debug-vocab -task "description"`.

**Soft injection:** Learned vocab competes through RRF scoring, not forced to the
top of results. This prevents displacement of correct results on tasks with good
BM25 coverage. Only hand-curated framework classes get forced injection.

**Confidence weighting:** Observation count scales the RRF weight from 0.3
(count=2, new association) to 0.8 (count>=10, well-reinforced). Associations that
are confirmed across multiple sessions get stronger positioning.

**Cross-task bridging:** Validated on 308 tasks: vocab learned from task A helps
task B via shared keywords. Django +41.4% in isolation, full corpus 0.0% aggregate
(safe). 10-round compounding: +2.2% P@10 peak, +8.1% MRR peak (round 7).

## Index / Enrichment Flags

| Flag | Description |
|------|-------------|
| `-no-enrich` | Skip LSP enrichment during `knowing index`. Tree-sitter edges only. |
| `-db <path>` | Override database path. |

## Debug Commands

| Command | Description |
|---------|-------------|
| `knowing debug-seeds -task "..." -db <path> <repo>` | Show full seed selection pipeline for a task. |
| `knowing debug-fts -query "..." -db <path>` | Run raw FTS5 query against the node index. |
| `knowing debug-walk -seed "Name" -db <path>` | Show RWR walk from specific seed nodes. |
| `knowing bench-task -task <id> -corpus <path> <repo>` | Run single benchmark task with P@10 analysis. |
