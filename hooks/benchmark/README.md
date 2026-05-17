# Hook Benchmark

Automated A/B test that measures whether knowing hooks improve agent task completion.

## How it works

1. Defines a set of tasks (file edits with known correct outcomes)
2. Runs each task with hooks ON and OFF
3. Measures: context relevance, token overhead, hit rate
4. Produces a verdict: "hooks help" / "hooks hurt" / "inconclusive"

## Running

```bash
# Index the repo first
knowing index -db knowing.db .

# Run the benchmark
go test -tags hookbench ./hooks/benchmark/ -v
```

## What it measures

For each task, the benchmark:
- Calls `knowing context` with the task file (simulating what the hook does)
- Checks whether the returned symbols include the symbols that the task actually touches
- Measures the "precision" (what % of injected symbols are relevant) and "recall" (what % of needed symbols were injected)

If precision is low: the hook is injecting noise (wasting tokens).
If recall is high: the hook is providing useful context (saves tool calls).

## Verdict criteria

- **PASS (hooks help)**: mean precision >= 30% AND mean recall >= 50%
  (At least half the symbols the task needs are in the injected context,
  and at least a third of what's injected is relevant)
- **FAIL (hooks hurt)**: mean precision < 15%
  (Most of what's injected is noise)
- **INCONCLUSIVE**: between thresholds
