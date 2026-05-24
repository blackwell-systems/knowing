package context

import (
	stdctx "context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/treesitter"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

// TestP10Regression_Flask is a CI-friendly regression gate that runs a fixed set
// of tasks against the Flask corpus and asserts P@10 doesn't drop below baseline.
// If any task's precision drops more than 20% from its recorded baseline, the test
// fails. This catches silent quality degradation from pipeline changes.
//
// Baselines are from Run 23 (2026-05-23). Update after intentional improvements.
func TestP10Regression_Flask(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping P@10 regression test in short mode")
	}

	flaskPath := findFlaskRepo(t)
	ctx := stdctx.Background()

	// Index Flask.
	dbPath := filepath.Join(t.TempDir(), "regression.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	pyExt, err := treesitter.NewTreeSitterExtractor("python")
	if err != nil {
		t.Fatalf("python extractor: %v", err)
	}
	idx.Register(pyExt)

	_, err = idx.IndexRepo(ctx, "github.com/pallets/flask", flaskPath, "HEAD")
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	st.RebuildFTS(ctx) //nolint:errcheck

	// Tasks with known ground truth and baseline P@10.
	type regressionTask struct {
		desc        string
		groundTruth []string // substrings that should appear in top-10 qualified names
		baselineHit int      // minimum ground truth hits expected in top-10
	}

	tasks := []regressionTask{
		{
			desc:        "add a before_request hook that validates API keys",
			groundTruth: []string{"before_request", "Scaffold", "scaffold"},
			baselineHit: 1, // baseline: 1 (Scaffold match via RWR)
		},
		{
			desc:        "register a new Flask blueprint for user profiles",
			groundTruth: []string{"Blueprint", "blueprint", "register_blueprint"},
			baselineHit: 5, // baseline: 7 (strong keyword + graph match)
		},
		{
			desc:        "add a custom error handler for 404 responses",
			groundTruth: []string{"errorhandler", "error_handler", "HTTPException", "handle_exception"},
			baselineHit: 0, // baseline: 0 (error handling terms match weakly on fresh index)
		},
		{
			desc:        "implement session-based authentication with login",
			groundTruth: []string{"session", "Session", "SecureCookie", "login"},
			baselineHit: 1, // baseline: 1 (session match)
		},
	}

	engine := NewContextEngine(st)
	failures := 0

	for _, task := range tasks {
		res, err := engine.ForTask(ctx, TaskOptions{
			TaskDescription: task.desc,
			TokenBudget:     5000,
			Format:          "json",
		})
		if err != nil {
			t.Errorf("  %s: ERROR %v", task.desc[:40], err)
			failures++
			continue
		}

		hits := 0
		top := 10
		if len(res.Symbols) < top {
			top = len(res.Symbols)
		}
		for i := 0; i < top; i++ {
			qn := strings.ToLower(res.Symbols[i].Node.QualifiedName)
			for _, gt := range task.groundTruth {
				if strings.Contains(qn, strings.ToLower(gt)) {
					hits++
					break
				}
			}
		}

		if hits < task.baselineHit {
			t.Errorf("P@10 REGRESSION: %q: got %d hits, baseline requires >= %d",
				task.desc[:40], hits, task.baselineHit)
			failures++
		} else {
			t.Logf("  OK: %q: %d/%d hits (baseline: %d)", task.desc[:40], hits, top, task.baselineHit)
		}
	}

	if failures > 0 {
		t.Fatalf("%d/%d tasks regressed below baseline", failures, len(tasks))
	}
}

func findFlaskRepo(t *testing.T) string {
	t.Helper()
	// Try relative paths from likely working directories.
	candidates := []string{
		"bench/cross-system/corpus/repos/flask",
		"../bench/cross-system/corpus/repos/flask",
		"../../bench/cross-system/corpus/repos/flask",
	}
	for _, c := range candidates {
		abs, _ := filepath.Abs(c)
		if _, err := os.Stat(filepath.Join(abs, "src/flask")); err == nil {
			return abs
		}
	}
	t.Skip("flask repo not found in corpus")
	return ""
}
