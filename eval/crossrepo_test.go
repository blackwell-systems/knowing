package eval

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
)

// crossRepoFixture is a simplified fixture for testing on external repos.
// Uses their format: query + expected IDs (any-hit recall).
type crossRepoFixture struct {
	ID       string
	Tier     string // "exact", "concept", "multi_hop"
	Query    string
	Expected []string // symbol substrings to match against
}

func TestCrossRepo_Gortex(t *testing.T) {
	gortexPath := "/Users/dayna.blackwell/code/gortex"
	if _, err := os.Stat(gortexPath); err != nil {
		t.Skip("gortex repo not available")
	}

	ctx := context.Background()
	dbPath := t.TempDir() + "/gortex.db"
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	_, err = idx.IndexRepo(ctx, "github.com/zzet/gortex", gortexPath, "HEAD")
	if err != nil {
		t.Fatalf("index gortex: %v", err)
	}

	engine := knowingctx.NewContextEngine(st)

	// Adapted subset of gortex fixtures (10 exact, 10 concept, 10 multi_hop).
	fixtures := gortexFixtures()

	topK := 10
	tokenBudget := 5000

	type tierResult struct {
		hits  int
		total int
	}
	tiers := map[string]*tierResult{
		"exact":     {},
		"concept":   {},
		"multi_hop": {},
	}

	for _, fix := range fixtures {
		block, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: fix.Query,
			TokenBudget:     tokenBudget,
			Format:          "json",
		})
		if err != nil {
			t.Logf("  SKIP %s: %v", fix.ID, err)
			continue
		}

		k := topK
		if len(block.Symbols) < k {
			k = len(block.Symbols)
		}

		// Any-hit recall: did ANY expected ID appear in top-K?
		hit := false
		for i := 0; i < k; i++ {
			for _, exp := range fix.Expected {
				if strings.Contains(block.Symbols[i].Node.QualifiedName, exp) {
					hit = true
					break
				}
			}
			if hit {
				break
			}
		}

		tr := tiers[fix.Tier]
		tr.total++
		if hit {
			tr.hits++
		}
	}

	t.Log("")
	t.Log("=== CROSS-REPO EVAL: gortex (no knowing-specific seeds) ===")
	t.Log("")
	t.Logf("%-12s | %6s | %s", "Tier", "R@10", "N")
	t.Logf("%-12s-+-%6s-+-%s", "------------", "------", "---")
	var totalHits, totalN int
	for _, tier := range []string{"exact", "concept", "multi_hop"} {
		tr := tiers[tier]
		recall := 0.0
		if tr.total > 0 {
			recall = float64(tr.hits) / float64(tr.total) * 100
		}
		t.Logf("%-12s | %5.1f%% | %d", tier, recall, tr.total)
		totalHits += tr.hits
		totalN += tr.total
	}
	overallRecall := 0.0
	if totalN > 0 {
		overallRecall = float64(totalHits) / float64(totalN) * 100
	}
	t.Logf("%-12s | %5.1f%% | %d", "OVERALL", overallRecall, totalN)

	// Write results.
	f, _ := os.Create(findRepoRoot(t) + "/eval/CROSS_REPO_FINDINGS.md")
	if f != nil {
		defer f.Close()
		fmt.Fprintf(f, "# Cross-Repo Eval: gortex\n\n")
		fmt.Fprintf(f, "**No knowing-specific equivalence classes.** Tests the general pipeline\n")
		fmt.Fprintf(f, "(keyword tiers + BM25 + bigram compounds + graph-derived aliases + RRF)\n")
		fmt.Fprintf(f, "on an external Go codebase with no hand-curated seed dictionary.\n\n")
		fmt.Fprintf(f, "| Tier | R@10 | N |\n|------|------|---|\n")
		for _, tier := range []string{"exact", "concept", "multi_hop"} {
			tr := tiers[tier]
			recall := 0.0
			if tr.total > 0 {
				recall = float64(tr.hits) / float64(tr.total) * 100
			}
			fmt.Fprintf(f, "| %s | %.1f%% | %d |\n", tier, recall, tr.total)
		}
		fmt.Fprintf(f, "| **Overall** | **%.1f%%** | **%d** |\n", overallRecall, totalN)
	}
}

func gortexFixtures() []crossRepoFixture {
	return []crossRepoFixture{
		// 10 exact-tier fixtures (symbol name queries)
		{ID: "exact-Server", Tier: "exact", Query: "Server", Expected: []string{"mcp.Server", "server.Server"}},
		{ID: "exact-NewServer", Tier: "exact", Query: "NewServer", Expected: []string{"mcp.NewServer", "server.NewServer"}},
		{ID: "exact-Graph", Tier: "exact", Query: "graph.Graph type", Expected: []string{"graph.Graph"}},
		{ID: "exact-Node", Tier: "exact", Query: "graph.Node type", Expected: []string{"graph.Node"}},
		{ID: "exact-Indexer", Tier: "exact", Query: "Indexer struct", Expected: []string{"indexer.Indexer"}},
		{ID: "exact-BM25Backend", Tier: "exact", Query: "BM25Backend", Expected: []string{"search.BM25Backend", "bm25.BM25Backend"}},
		{ID: "exact-HybridBackend", Tier: "exact", Query: "HybridBackend", Expected: []string{"search.HybridBackend", "hybrid.HybridBackend"}},
		{ID: "exact-GoExtractor", Tier: "exact", Query: "GoExtractor", Expected: []string{"golang.GoExtractor"}},
		{ID: "exact-RegisterAll", Tier: "exact", Query: "RegisterAll", Expected: []string{"register.RegisterAll", "languages.RegisterAll"}},
		{ID: "exact-VectorBackend", Tier: "exact", Query: "VectorBackend", Expected: []string{"search.VectorBackend", "vector.VectorBackend"}},

		// 10 concept-tier fixtures (natural-language queries)
		{ID: "concept-go-extract", Tier: "concept", Query: "Go language extractor", Expected: []string{"GoExtractor"}},
		{ID: "concept-hybrid-fuse", Tier: "concept", Query: "combine text and vector search with RRF", Expected: []string{"HybridBackend", "NewHybrid"}},
		{ID: "concept-callers", Tier: "concept", Query: "who calls this function", Expected: []string{"handleGetCallers", "GetCallers"}},
		{ID: "concept-reindex", Tier: "concept", Query: "incremental reindex after file changes", Expected: []string{"IncrementalReindex"}},
		{ID: "concept-index-repo", Tier: "concept", Query: "run the full repository indexer", Expected: []string{"handleIndexRepository", "IndexRepo"}},
		{ID: "concept-find-usages", Tier: "concept", Query: "find every reference to a symbol", Expected: []string{"handleFindUsages", "FindUsages"}},
		{ID: "concept-detect-lang", Tier: "concept", Query: "detect language from file extension", Expected: []string{"GetByExtension", "Registry"}},
		{ID: "concept-http-extract", Tier: "concept", Query: "extract HTTP route handlers from source", Expected: []string{"HTTPExtractor"}},
		{ID: "concept-smart-context", Tier: "concept", Query: "assemble minimum context for a task", Expected: []string{"handleSmartContext", "SmartContext"}},
		{ID: "concept-bm25-index", Tier: "concept", Query: "text search backend with TF-IDF ranking", Expected: []string{"BM25Backend", "BM25"}},

		// 10 multi_hop fixtures (relational queries)
		{ID: "multi-extractors", Tier: "multi_hop", Query: "all language extractors registered in the system", Expected: []string{"GoExtractor", "TypeScriptExtractor", "RegisterAll"}},
		{ID: "multi-search-stack", Tier: "multi_hop", Query: "the complete search pipeline from query to ranked results", Expected: []string{"BM25Backend", "HybridBackend", "VectorBackend"}},
		{ID: "multi-mcp-tools", Tier: "multi_hop", Query: "MCP tool handlers that modify the graph", Expected: []string{"handleIndexRepository", "handleEditSymbol", "handleWriteFile"}},
		{ID: "multi-embedding-chain", Tier: "multi_hop", Query: "embedding providers and how they connect to search", Expected: []string{"Provider", "HugotProvider", "VectorBackend"}},
		{ID: "multi-index-pipeline", Tier: "multi_hop", Query: "full indexing pipeline from file to graph node", Expected: []string{"Indexer", "IndexFile", "Graph", "AddNode"}},
		{ID: "multi-session-track", Tier: "multi_hop", Query: "session state tracking and symbol modification history", Expected: []string{"sessionState", "recordModified", "handleGetSymbolHistory"}},
		{ID: "multi-code-edit", Tier: "multi_hop", Query: "safely edit code symbols with impact checking", Expected: []string{"handleEditSymbol", "handleGetEditingContext", "handleGetCallers"}},
		{ID: "multi-graph-ops", Tier: "multi_hop", Query: "graph operations for adding and removing nodes", Expected: []string{"AddNode", "EvictFile", "EvictRepo"}},
		{ID: "multi-repo-mgmt", Tier: "multi_hop", Query: "managing multiple repositories in the graph", Expected: []string{"MultiIndexer", "TrackRepo", "UntrackRepo", "RepoForFile"}},
		{ID: "multi-contract-extract", Tier: "multi_hop", Query: "extract API contracts from source files", Expected: []string{"HTTPExtractor", "GRPCExtractor", "OpenAPIExtractor"}},
	}
}
