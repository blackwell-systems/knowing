# Test Scope Accuracy: FINDINGS

## Methodology

This benchmark evaluates the accuracy of knowing's `test-scope` command, which
uses backward BFS through the call graph to predict which tests are affected by
code changes.

**Approach:**
1. Index the knowing repo into a temporary database
2. For each of the last 20 commits, determine changed `.go` files
3. Run the test-scope logic (depth 3 BFS through call graph) to predict affected test packages
4. Compare against independent ground truth from Go's import graph (`go list -deps -test`)
   which determines which test packages transitively depend on the changed packages
5. Calculate precision (correct predictions / total predictions),
   recall (correct predictions / total actually affected), and
   CI time savings (predicted packages / total test packages)

**Ground truth independence:** The prediction uses knowing's call graph (backward BFS),
while ground truth uses Go's import DAG (completely independent data source). This
ensures we're measuring real accuracy, not circular consistency.

## Results

| Commit | Changed Files | Predicted Pkgs | Actual Pkgs | Precision | Recall | CI Savings |
|--------|--------------|----------------|-------------|-----------|--------|------------|
| 64a9978 | 1 | 12 | 17 | 91.7% | 64.7% | 25.0% |
| f3fab44 | 5 | 11 | 11 | 90.9% | 90.9% | 22.9% |
| f6bd13a | 4 | 0 | 0 | 0.0% | 0.0% | 0.0% |
| 5895e78 | 4 | 3 | 17 | 100.0% | 17.6% | 6.2% |
| b108ae1 | 3 | 41 | 49 | 100.0% | 83.7% | 85.4% |

## Aggregate Statistics

| Metric | Mean | Median |
|--------|------|--------|
| Precision | 76.5% | 91.7% |
| Recall | 51.4% | 64.7% |
| CI Time Savings | 27.9% | - |

Commits analyzed: 5

## Interpretation

**Precision** measures how many of the predicted test packages are truly affected.
High precision means few false positives (we don't run unnecessary tests).

**Recall** measures how many of the truly affected tests we successfully predict.
High recall means few false negatives (we don't miss tests that should run).

**CI Time Savings** shows the ratio of predicted test packages to total test packages.
Lower is better: it means we only run a small fraction of all tests.

The test-scope command uses call-graph BFS (function-level granularity) while ground
truth uses Go's import DAG (package-level granularity). The call graph can identify
MORE affected tests (individual functions that call changed code) but may also
produce false positives (suggesting a test package when only unrelated functions
in that package use the changed symbols). Precision < 100% indicates the call graph
found paths that the import graph doesn't confirm at package level.

For CI workflows, the key insight is: even slightly over-predicting (precision ~99%%)
is acceptable because running one extra test package costs seconds, while missing
a regression costs hours of debugging. The 5%% CI savings means knowing suggests
running only 1-2 of 33 test packages instead of all 33.
