#!/usr/bin/env bash
# Task definitions for knowing codebase (93K LOC Go).

REPO_PATH="$REPO_ROOT"
REPO_NAME="knowing"

get_prompt_knowing() {
  case "$1" in
    add-json-flag)
      echo 'Add a --json flag to the `knowing stale` command (in cmd/knowing/stale.go) that outputs results as JSON instead of human-readable text. The flag should be a bool, default false. When set, output a JSON object with fields: stale_files ([]string of changed file paths), stale_node_count (int total stale nodes), checked_at (ISO 8601 timestamp). Keep the existing human-readable output as the default.'
      ;;
    add-symbol-info-tool)
      echo 'Add a `symbol_info` MCP tool to the knowing server. It takes a `qualified_name` string parameter and returns JSON with: the node'\''s kind, file_path (from File table), line number, incoming edges (callers with their qualified names), and outgoing edges (callees with their qualified names). Register it in registerTools() in internal/mcp/server.go. Implementation goes in internal/mcp/handlers.go. Use store.NodesByQualifiedName to find the node, then store.EdgesTo and store.EdgesFrom for edges.'
      ;;
    refactor-return-type)
      echo 'Refactor the `InferExternalRepoURL` function in internal/resolve/external.go. Currently it returns a plain string ("external://...", "stdlib", or ""). Change the return type to: type ExternalResult struct { URL string; Kind string } where Kind is "external", "stdlib", or "local" (for empty string returns). Update the function signature, its implementation, ALL callers across the codebase, and all tests. You must find every file that calls this function yourself.'
      ;;
    find-rwr-convergence-issue)
      echo 'The context engine'\''s Random Walk with Restart in internal/context/walk.go sometimes produces nearly identical scores for unrelated symbols when the graph has disconnected components. Read walk.go, understand how the restart probability works, identify why disconnected components get similar scores, and add a code comment at the relevant location explaining the root cause and a proposed fix. Do not change the algorithm logic, just add the explanatory comment.'
      ;;
    add-diff-since-flag)
      echo 'Add a `--since` flag to the `knowing diff` command in cmd/knowing/main.go (the cmdDiff function). The flag accepts a Go duration string (e.g., "24h", "168h" for 7 days). When set, it should: 1) Run `git log --since=<duration> --name-only --pretty=format:` to get files changed in that period, 2) Look up which packages those files belong to using store.NodesByFilePath, 3) Output a table of packages sorted by change count (most changed first). Format: one package per line with change count.'
      ;;
    cross-package-test-coverage)
      echo 'Add a new benchmark test in bench/test-scope-accuracy/ called TestCrossPackageTestCoverage. It should: 1) Index the knowing repo into a temp DB, 2) For each package that has a _test.go file, count how many of its functions are called by tests in OTHER packages (not its own test file), 3) Report: package name, total exported functions, cross-package test callers, coverage percentage. Use store.EdgesTo to find incoming '\''tests'\'' edges from other packages. Output as a t.Log table sorted by coverage ascending (least covered first).'
      ;;
    interface-implementors)
      echo 'The GraphStore interface in internal/types/interfaces.go defines the contract that all store implementations must satisfy. I want to add a new method to this interface: SymbolsByKind(ctx context.Context, kind string) ([]Node, error). Add this method to the interface, then implement it in EVERY type that implements GraphStore. You must find all implementors yourself. Do not assume you know which files implement this interface. After adding the method everywhere, verify the build passes.'
      ;;
    cascading-breakage)
      echo 'I want to understand what would break if I deleted the ComputeNodeHash function from internal/types/hash.go. Do NOT actually delete it. Instead, find every direct caller of ComputeNodeHash, then for each caller find THEIR callers (second-hop dependents). Report a two-level dependency tree: level 1 (direct callers with file:line) and level 2 (callers of callers). This requires transitive analysis, not just grep for the function name.'
      ;;
    ambient-context)
      echo 'I am about to modify the RankSymbols function in internal/context/ranking.go. Before I start editing, I need to understand: 1) What calls RankSymbols and what do those callers expect from it? 2) What does RankSymbols call internally (its dependencies)? 3) What test functions exercise RankSymbols? 4) Are there any other ranking-related functions in the same package that interact with it? Give me a complete map of this function'\''s neighborhood so I can edit it safely.'
      ;;
    *)
      echo ""
      ;;
  esac
}

get_verify_knowing() {
  case "$1" in
    add-json-flag)          echo "GOWORK=off go build ./cmd/knowing/" ;;
    add-symbol-info-tool)   echo "GOWORK=off go build ./..." ;;
    refactor-return-type)   echo "GOWORK=off go build ./... && GOWORK=off go test ./internal/resolve/ -timeout 1m" ;;
    find-rwr-convergence-issue) echo "GOWORK=off go build ./internal/context/" ;;
    add-diff-since-flag)    echo "GOWORK=off go build ./cmd/knowing/" ;;
    cross-package-test-coverage) echo "GOWORK=off go vet ./bench/test-scope-accuracy/" ;;
    interface-implementors) echo "GOWORK=off go build ./..." ;;
    cascading-breakage)     echo "true" ;;
    ambient-context)        echo "true" ;;
    *)                      echo "true" ;;
  esac
}

KNOWING_TASKS="add-json-flag add-symbol-info-tool refactor-return-type find-rwr-convergence-issue add-diff-since-flag cross-package-test-coverage interface-implementors cascading-breakage ambient-context"
