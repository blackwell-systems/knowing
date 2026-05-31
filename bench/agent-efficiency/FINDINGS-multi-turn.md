# Multi-Turn Agent Efficiency Results

| Task | Mode | Tokens | Tools | Explore | Knowing | Build | Wall (s) |
|------|------|--------|-------|---------|---------|-------|----------|
| refactor-return-type | control | 1465 | 32 | 14 | 0 | true | 122.4 |
| refactor-return-type | treatment | 1749 | 36 | 14 | 2 | true | 168.5 |
| add-json-flag | control | 138 | 5 | 1 | 0 | true | 36.6 |
| add-json-flag | treatment | 180 | 7 | 2 | 1 | true | 40.1 |
| ambient-context | control | 117 | 4 | 4 | 0 | true | 5.8 |
| ambient-context | treatment | 3419 | 11 | 7 | 3 | true | 31.3 |

## Paired Comparison

| Task | Token Δ | Tool Δ | Explore Δ | Build Ctrl | Build Treat |
|------|---------|--------|-----------|------------|-------------|
| refactor-return-type | +284 | +4 | +0 | true | true |
| add-json-flag | +42 | +2 | +1 | true | true |
| ambient-context | +3302 | +7 | +3 | true | true |
