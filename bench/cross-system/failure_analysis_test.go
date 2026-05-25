// Package crosssystem_test provides failure analysis for P@10 misses.
//
// This test runs each task through the full retrieval pipeline and for every
// ground truth symbol that is NOT returned in the top-10, categorizes the
// failure reason:
//
//   - not_in_db: the symbol doesn't exist in the indexed database at all
//   - no_seeds: keyword extraction produced no matching seeds for this task
//   - unreachable: symbol exists in DB but has zero RWR score from extracted seeds
//   - ranked_low: symbol is reachable but ranked below position 10
//   - matched: symbol was successfully retrieved (for counting hits)
//
// Usage:
//
//	go test ./bench/cross-system/ -run TestFailureAnalysis -v -timeout 30m
//	BENCH_REPOS=kubernetes go test ./bench/cross-system/ -run TestFailureAnalysis -v -timeout 30m
package crosssystem_test

import (
	stdctx "context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/normalize"
	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// FailureReason categorizes why a ground truth symbol was missed.
type FailureReason string

const (
	ReasonNotInDB      FailureReason = "not_in_db"
	ReasonNoSeeds      FailureReason = "no_seeds"
	ReasonUnreachable  FailureReason = "unreachable"
	ReasonRankedLow    FailureReason = "ranked_low"
	ReasonMatched      FailureReason = "matched"
)

// FailureDetail records one ground truth symbol's diagnosis.
type FailureDetail struct {
	TaskID      string
	Repo        string
	GroundTruth string
	Reason      FailureReason
	RWRScore    float64 // 0 if unreachable
	Rank        int     // 0 if not in top results, else 1-based rank
	DBMatches   int     // number of DB nodes matching this ground truth terminal name
	SeedCount   int     // number of seeds extracted for this task
	Keywords    []string
}

func TestFailureAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("failure analysis requires pre-indexed repos; run with -timeout 30m")
	}

	rawTasks := loadTasks(t, "corpus/tasks")
	if len(rawTasks) == 0 {
		t.Fatal("no task fixtures found in corpus/tasks/")
	}

	// Use raw tasks (don't filter achievable) so we can see not_in_db failures.
	repoFilter := buildRepoFilter(t)

	stores := make(map[string]*store.SQLiteStore)
	defer func() {
		for _, s := range stores {
			s.Close()
		}
	}()

	var allDetails []FailureDetail

	for _, task := range rawTasks {
		if !repoAllowed(task.Repo, repoFilter) {
			continue
		}

		repoPath := filepath.Join("corpus", "repos", task.Repo)
		dbPath := filepath.Join(repoPath, ".knowing", "graph.db")

		// Open or reuse store.
		s, ok := stores[task.Repo]
		if !ok {
			var err error
			s, err = store.NewSQLiteStore(dbPath)
			if err != nil {
				t.Logf("[%s] cannot open store: %v (skipping)", task.ID, err)
				continue
			}
			stores[task.Repo] = s
		}

		ctx := stdctx.Background()

		// Step 1: Extract keywords (same logic as ForTask).
		ks := knowingctx.ExtractKeywordSet(task.Description)
		keywords := ks.Primary()
		if len(keywords) == 0 {
			keywords = ks.All()
		}

		// Step 2: Run seed retrieval (tiered search + BM25 fusion).
		engine := knowingctx.NewContextEngine(s)
		var repoURL string
		if repos, err := s.AllRepos(ctx); err == nil && len(repos) > 0 {
			repoURL = repos[0].RepoURL
		}

		result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task.Description,
			TokenBudget:     5000,
			Format:          "json",
			RepoURL:         repoURL,
		})
		if err != nil {
			t.Logf("[%s] ForTask error: %v", task.ID, err)
			continue
		}

		// Build set of retrieved symbols (normalized) for matching.
		retrievedQNs := make([]string, len(result.Symbols))
		for i, sym := range result.Symbols {
			retrievedQNs[i] = sym.Node.QualifiedName
		}

		// Step 3: For each ground truth symbol, determine WHY it was missed.
		for _, gt := range task.GroundTruth {
			detail := FailureDetail{
				TaskID:      task.ID,
				Repo:        task.Repo,
				GroundTruth: gt,
				SeedCount:   len(result.Symbols),
				Keywords:    keywords,
			}

			// Check if matched in top-10.
			matched := false
			for i, qn := range retrievedQNs {
				if i >= 10 {
					break
				}
				if normalize.MatchesGroundTruth(qn, gt) {
					detail.Reason = ReasonMatched
					detail.Rank = i + 1
					matched = true
					break
				}
			}
			if matched {
				allDetails = append(allDetails, detail)
				continue
			}

			// Check if matched below top-10 (ranked_low).
			for i, qn := range retrievedQNs {
				if i < 10 {
					continue
				}
				if normalize.MatchesGroundTruth(qn, gt) {
					detail.Reason = ReasonRankedLow
					detail.Rank = i + 1
					matched = true
					break
				}
			}
			if matched {
				allDetails = append(allDetails, detail)
				continue
			}

			// Check if the symbol exists in the DB at all.
			gtTerminal := gtTerminalName(gt)
			dbNodes, _ := s.NodesByName(ctx, "%"+gtTerminal)
			matchingNodes := filterGTMatches(dbNodes, gt)
			detail.DBMatches = len(matchingNodes)

			if len(matchingNodes) == 0 {
				detail.Reason = ReasonNotInDB
				allDetails = append(allDetails, detail)
				continue
			}

			// Symbol exists in DB. Check if it has any RWR score from our seeds.
			// Run a lightweight reachability check: does RWR from our seeds reach it?
			if len(keywords) == 0 {
				detail.Reason = ReasonNoSeeds
				allDetails = append(allDetails, detail)
				continue
			}

			// Check reachability: does the full pipeline reach this node at all?
			// We already ran ForTask above with full budget. If the symbol didn't appear
			// in ANY position in the results, it's unreachable from the extracted seeds.
			foundAnywhere := false
			for _, qn := range retrievedQNs {
				if normalize.MatchesGroundTruth(qn, gt) {
					foundAnywhere = true
					break
				}
			}

			if !foundAnywhere {
				// Double-check: run RWR directly to see if there's ANY score.
				// The ForTask pipeline has a 0.02 threshold; the symbol might have
				// a tiny score below that cutoff.
				detail.Reason = ReasonUnreachable
				// Try to find it via direct RWR if we can get seed hashes.
				seedHashes := getSeedHashes(ctx, s, keywords)
				if len(seedHashes) > 0 {
					scores, err := knowingctx.RandomWalkWithRestart(ctx, s, seedHashes, 0.2, 20)
					if err == nil {
						for _, node := range matchingNodes {
							if score, ok := scores[node.NodeHash]; ok && score > detail.RWRScore {
								detail.RWRScore = score
							}
						}
					}
				}
				if detail.RWRScore > 0 {
					// It has SOME score but below the 0.02 threshold, so it's too far.
					detail.Reason = ReasonRankedLow
				}
			} else {
				detail.Reason = ReasonRankedLow
			}

			allDetails = append(allDetails, detail)
		}
	}

	// Summarize results.
	t.Log("\n=== FAILURE ANALYSIS SUMMARY ===\n")

	// Count by reason.
	reasonCounts := make(map[FailureReason]int)
	for _, d := range allDetails {
		reasonCounts[d.Reason]++
	}
	total := len(allDetails)
	t.Logf("Total ground truth symbols analyzed: %d", total)
	t.Logf("")
	t.Logf("| Reason | Count | Pct |")
	t.Logf("|--------|-------|-----|")
	for _, reason := range []FailureReason{ReasonMatched, ReasonRankedLow, ReasonUnreachable, ReasonNoSeeds, ReasonNotInDB} {
		count := reasonCounts[reason]
		pct := 100.0 * float64(count) / float64(total)
		t.Logf("| %s | %d | %.1f%% |", reason, count, pct)
	}

	// Per-repo breakdown.
	t.Log("\n=== PER-REPO BREAKDOWN ===\n")
	repoReasons := make(map[string]map[FailureReason]int)
	for _, d := range allDetails {
		if repoReasons[d.Repo] == nil {
			repoReasons[d.Repo] = make(map[FailureReason]int)
		}
		repoReasons[d.Repo][d.Reason]++
	}
	for repo, reasons := range repoReasons {
		repoTotal := 0
		for _, c := range reasons {
			repoTotal += c
		}
		hits := reasons[ReasonMatched]
		t.Logf("%s: %d/%d matched (%.1f%%), not_in_db=%d, unreachable=%d, ranked_low=%d, no_seeds=%d",
			repo, hits, repoTotal, 100.0*float64(hits)/float64(repoTotal),
			reasons[ReasonNotInDB], reasons[ReasonUnreachable],
			reasons[ReasonRankedLow], reasons[ReasonNoSeeds])
	}

	// Per-task breakdown for misses.
	t.Log("\n=== PER-TASK MISS DETAILS ===\n")
	type taskMiss struct {
		taskID  string
		reason  FailureReason
		gt      string
		score   float64
		rank    int
		keywords []string
	}
	var misses []taskMiss
	for _, d := range allDetails {
		if d.Reason == ReasonMatched {
			continue
		}
		misses = append(misses, taskMiss{
			taskID:  d.TaskID,
			reason:  d.Reason,
			gt:      d.GroundTruth,
			score:   d.RWRScore,
			rank:    d.Rank,
			keywords: d.Keywords,
		})
	}
	// Sort: unreachable first (most actionable), then not_in_db, then ranked_low.
	sort.Slice(misses, func(i, j int) bool {
		priority := map[FailureReason]int{
			ReasonUnreachable: 0,
			ReasonNotInDB:     1,
			ReasonNoSeeds:     2,
			ReasonRankedLow:   3,
		}
		if priority[misses[i].reason] != priority[misses[j].reason] {
			return priority[misses[i].reason] < priority[misses[j].reason]
		}
		return misses[i].taskID < misses[j].taskID
	})

	for _, m := range misses {
		extra := ""
		if m.score > 0 {
			extra = fmt.Sprintf(" (RWR=%.4f)", m.score)
		}
		if m.rank > 0 {
			extra = fmt.Sprintf(" (rank=%d)", m.rank)
		}
		t.Logf("  [%s] %s: %s -> %s%s", m.reason, m.taskID, m.gt, strings.Join(m.keywords[:min(len(m.keywords), 5)], ","), extra)
	}

	// Actionable summary: what would move P@10?
	t.Log("\n=== ACTIONABLE INSIGHTS ===\n")
	unreachableCount := reasonCounts[ReasonUnreachable]
	notInDBCount := reasonCounts[ReasonNotInDB]
	rankedLowCount := reasonCounts[ReasonRankedLow]
	matchedCount := reasonCounts[ReasonMatched]

	t.Logf("Current effective P@10: %.3f (%d/%d matched)", float64(matchedCount)/float64(total), matchedCount, total)
	t.Logf("")
	t.Logf("Improvement levers:")
	t.Logf("  1. Fix UNREACHABLE (%d symbols): add missing edges/paths so RWR can reach them", unreachableCount)
	t.Logf("  2. Fix NOT_IN_DB (%d symbols): add extractors or fix fixtures", notInDBCount)
	t.Logf("  3. Fix RANKED_LOW (%d symbols): improve ranking once reachable", rankedLowCount)
	t.Logf("")
	if unreachableCount > 0 {
		t.Logf("UNREACHABLE symbols are the #1 lever: each new path = step-function P@10 improvement.")
		// Group unreachable by task to show which tasks would benefit most.
		taskUnreachable := make(map[string]int)
		for _, d := range allDetails {
			if d.Reason == ReasonUnreachable {
				taskUnreachable[d.TaskID]++
			}
		}
		type taskCount struct {
			id    string
			count int
		}
		var sorted []taskCount
		for id, c := range taskUnreachable {
			sorted = append(sorted, taskCount{id, c})
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
		t.Logf("  Tasks with most unreachable ground truth:")
		for i, tc := range sorted {
			if i >= 10 {
				break
			}
			t.Logf("    %s: %d unreachable symbols", tc.id, tc.count)
		}
	}
}

// getSeedHashes extracts seed node hashes by running tiered keyword matching.
func getSeedHashes(ctx stdctx.Context, s *store.SQLiteStore, keywords []string) []types.Hash {
	var hashes []types.Hash
	seen := make(map[types.Hash]bool)
	for _, kw := range keywords {
		nodes, err := s.NodesByName(ctx, "%"+kw)
		if err != nil {
			continue
		}
		for _, n := range nodes {
			if seen[n.NodeHash] {
				continue
			}
			// Only take exact/prefix matches on terminal name.
			lastDot := strings.LastIndex(n.QualifiedName, ".")
			symName := n.QualifiedName
			if lastDot >= 0 {
				symName = n.QualifiedName[lastDot+1:]
			}
			if strings.EqualFold(symName, kw) || strings.HasPrefix(strings.ToLower(symName), strings.ToLower(kw)) {
				seen[n.NodeHash] = true
				hashes = append(hashes, n.NodeHash)
			}
		}
		if len(hashes) >= 15 {
			break
		}
	}
	return hashes
}

// filterGTMatches finds DB nodes that match a ground truth entry.
func filterGTMatches(nodes []types.Node, gt string) []types.Node {
	var matches []types.Node
	for _, n := range nodes {
		if normalize.MatchesGroundTruth(n.QualifiedName, gt) {
			matches = append(matches, n)
		}
	}
	return matches
}

// gtTerminalName extracts the terminal symbol name from a ground truth entry.
// "pkg/scheduler/framework/interface.FilterPlugin" -> "FilterPlugin"
func gtTerminalName(gt string) string {
	// Try last dot.
	if idx := strings.LastIndex(gt, "."); idx >= 0 {
		return gt[idx+1:]
	}
	// Try last slash.
	if idx := strings.LastIndex(gt, "/"); idx >= 0 {
		return gt[idx+1:]
	}
	return gt
}

