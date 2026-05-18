# Eval Framework: Retrieval Accuracy

**Auto-generated. Do not edit manually.**

## Methodology

Standardized evaluation of knowing's context engine across tiered task fixtures.
Each fixture defines a development task and hand-curated ground-truth symbols.
The engine returns ranked results; we measure Precision@10, Recall@10, and MRR.

**Tiers:**
- **Easy:** Single-package tasks (all relevant symbols in one package)
- **Medium:** Cross-package tasks (symbols span 2-3 packages)
- **Hard:** Cross-repo or multi-system tasks (runtime, daemon, resolver involved)

## Results

| Task | P@10 | R@10 | MRR | Tier |
|------|------|------|-----|------|
| Add a method to SQLiteStore that queries nodes by communi... | 80.0% | 133.3% | 1.00 | easy |
| Add a new language extractor for Ruby files | 0.0% | 0.0% | 0.00 | easy |
| Add a new wire format codec for MessagePack encoding | 20.0% | 40.0% | 1.00 | easy |
| Implement garbage collection for old snapshots beyond ret... | 10.0% | 25.0% | 0.33 | easy |
| Add a new MCP tool that returns symbol documentation | 90.0% | 180.0% | 1.00 | easy |
| Implement HITS hub/authority reranking in the context eng... | 50.0% | 62.5% | 0.50 | medium |
| Resolve dangling cross-repo edges by matching module path... | 20.0% | 33.3% | 0.17 | medium |
| Compute a semantic diff between two graph snapshots showi... | 20.0% | 33.3% | 1.00 | medium |
| Wire feedback scoring into the context engine so that pre... | 0.0% | 0.0% | 0.00 | medium |
| Find affected tests by tracing the call graph backward fr... | 30.0% | 42.9% | 0.17 | medium |
| Detect changed files from git, clean up stale nodes and e... | 0.0% | 0.0% | 0.00 | hard |
| Generate full PR impact analysis: semantic diff of change... | 20.0% | 22.2% | 0.50 | hard |
| Compute blast radius for a symbol change across multiple ... | 0.0% | 0.0% | 0.00 | hard |
| Given a task description, seed the graph walk from keywor... | 0.0% | 0.0% | 0.00 | hard |
| Ingest OpenTelemetry spans and create runtime edges with ... | 50.0% | 62.5% | 0.50 | hard |

## Per-Tier Summary

| Tier | Precision@10 | Recall@10 | MRR | Fixtures |
|------|-------------|-----------|-----|----------|
| easy | 40.0% | 75.7% | 0.67 | 5 |
| medium | 24.0% | 34.4% | 0.37 | 5 |
| hard | 14.0% | 16.9% | 0.20 | 5 |
| **Overall** | **26.0%** | **42.3%** | **0.41** | **15** |

## Reproducibility

```bash
GOWORK=off go test ./eval/ -v -count=1 -timeout 5m
```

Indexes the knowing repo into a temp DB and evaluates all fixtures.
Add new fixtures to `eval/fixtures/{easy,medium,hard}/` in YAML format.
