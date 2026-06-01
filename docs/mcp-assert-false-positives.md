# mcp-assert Lint False Positives Report

**Date:** 2026-05-31
**mcp-assert version:** v1.x (installed via GHA action `blackwell-systems/mcp-assert-action@v1`)
**Server:** `knowing mcp -db knowing.db` (self-indexed)
**Total findings:** 94 errors, 31 warnings (125 total)
**CI status:** Failing. `--threshold 130` does not suppress: lint appears to exit 1 on any errors regardless of threshold.

## Error Breakdown

| Rule | Count | Category | False Positive? | Explanation |
|------|-------|----------|----------------|-------------|
| E105 | 60 | Missing parameter description | **Mixed** | Some parameters are self-explanatory (`repo_url`, `symbol_name`). Others could benefit from descriptions. Not all 60 are actionable. |
| E107 | 32 | Circular dependency | **Yes (all 32)** | mcp-assert traces tool description cross-references as a dependency graph. knowing's tools mention related tools in descriptions (e.g., "see also `blast_radius`"). These are documentation references, not call dependencies. Agents do not loop. |
| E112 | 5 | Sensitive data in schema | **Yes (all 5)** | Flags `token_budget` parameter as "sensitive data" (matches "token" substring). This is a token count for context packing, not an API token or secret. 3x `context_for_task/pr/files`. |
| E110 | 1 | Parameter not in description | **Partial** | Some parameters intentionally omitted from description for brevity. |

| Rule | Count | Category | False Positive? | Explanation |
|------|-------|----------|----------------|-------------|
| W116 | 20 | No return description | **Partial** | Tool descriptions focus on what the tool does, not return format. Adding return descriptions would improve agent UX but these aren't errors. |
| W110 | 7 | Parameters not mentioned | **Partial** | Similar to E110 but warning severity. |
| W103 | 3 | No enum/pattern/example | **Mixed** | Free-text parameters like `task` and `source_symbol` intentionally have no enum. |
| W112 | 1 | Sensitive parameter name | **Yes** | Same `token_budget` issue as E112. |

## Root Causes

### 1. E107 Circular Dependency (32 errors, all false positive)

knowing's MCP tool descriptions include "see also" references to related tools. For example, `context_for_task` mentions `explain_symbol` in its description, and `explain_symbol` mentions `ownership_query`. mcp-assert builds a dependency graph from these textual references and flags cycles.

These are **documentation cross-references**, not execution dependencies. An agent calling `context_for_task` does not need to call `explain_symbol` first, and there is no infinite loop. The tool descriptions are helping agents discover related capabilities.

**Fix needed in mcp-assert:** E107 should distinguish between "requires" dependencies (inputSchema references) and "see also" mentions (description text). Or provide `--skip-rules E107` flag.

### 2. E112 Sensitive Data (5 errors, all false positive)

The `token_budget` parameter appears on 3 context tools and 2 related tools. mcp-assert matches "token" as a sensitive substring (alongside "key", "secret", "password"). But `token_budget` is a context window size (integer, e.g., 5000), not an authentication token.

**Fix needed in mcp-assert:** Allowlist common non-sensitive uses of "token" (token_budget, token_count, token_limit) or support `--skip-rules E112`.

### 3. E105 Missing Parameter Descriptions (60 errors, mixed)

Many parameters have self-evident names (`repo_url`, `symbol_name`, `format`). Adding descriptions to all 60 would add boilerplate without helping agents. However, some non-obvious parameters could benefit from descriptions.

**Action:** Triage the 60 individually. Add descriptions where they'd genuinely help. Suppress the rest if `--skip-rules` becomes available.

## CI Friction

### --threshold flag not working for lint

The `--threshold 130` flag is set but lint still exits 1 with 125 findings (< 130). Two possible causes:

1. **Threshold counts errors only, not total:** 94 errors < 130 should still pass. But the exit message says "lint found 94 error(s)" which suggests it fails on any errors.
2. **Lint always exits 1 on errors:** Unlike `audit` mode which respects threshold, `lint` may treat any error as a hard failure. The `--threshold` flag would only apply to the audit scoring, not lint error/warning classification.

**Workaround needed:** Either `--skip-rules E107,E112` (not yet supported) or `|| true` to ignore lint failures (not ideal).

### Local vs CI discrepancy

Running locally produces 0 errors, 3 warnings. CI produces 94 errors, 31 warnings. The difference: CI self-indexes the knowing repo and runs MCP against that index. The self-indexed MCP server exposes all 28 tools. Locally, the tool might be using a different DB or configuration.

## Recommended Actions

1. **For mcp-assert maintainers:** Add `--skip-rules` flag to suppress known false positives (E107, E112). This is the clean fix.
2. **For knowing CI (short-term):** Change lint step to `mcp-assert lint ... || echo "mcp-assert lint: $? (non-blocking)"` and make it a non-blocking check until skip-rules is available.
3. **For knowing tool descriptions (medium-term):** Triage the 60 E105 findings and add descriptions where they'd genuinely help agent comprehension.
