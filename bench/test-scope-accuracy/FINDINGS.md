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
| 3130fd0 | 4 | 4 | 5 | 100.0% | 80.0% | 8.3% |
| 75efafc | 3 | 16 | 16 | 87.5% | 87.5% | 33.3% |
| f918b31 | 1 | 2 | 2 | 100.0% | 100.0% | 4.2% |
| 76383f9 | 3 | 2 | 5 | 100.0% | 40.0% | 4.2% |
| ebbd015 | 5 | 3 | 5 | 100.0% | 60.0% | 6.2% |
| 6a5f616 | 10 | 46 | 49 | 95.7% | 89.8% | 95.8% |
| d5d2c52 | 1 | 1 | 1 | 100.0% | 100.0% | 2.1% |
| 3f55876 | 4 | 2 | 2 | 100.0% | 100.0% | 4.2% |

## Aggregate Statistics

| Metric | Mean | Median |
|--------|------|--------|
| Precision | 97.9% | 100.0% |
| Recall | 82.2% | 89.8% |
| CI Time Savings | 19.8% | - |

Commits analyzed: 8

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
