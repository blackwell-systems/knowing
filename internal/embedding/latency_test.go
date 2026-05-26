package embedding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestLatencyProfile(t *testing.T) {
	fmt.Println("=== Embedding Latency Profile ===")
	fmt.Printf("Model: jina-code (dims=%d)\n\n", Dims)

	// Time: model load
	t0 := time.Now()
	e, err := New()
	if err != nil {
		t.Fatalf("create embedder: %v", err)
	}
	defer e.Close()
	fmt.Printf("Model load:    %v\n", time.Since(t0))

	ctx := context.Background()

	// Warm-up (first inference may include JIT/init)
	_, _ = e.Embed(ctx, "warmup")

	// Time: single embed
	t1 := time.Now()
	_, err = e.Embed(ctx, "function ContextEngine ForTask retrieves context for a task description")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Single embed:  %v\n", time.Since(t1))

	// Time: batch of 51 (query + 50 candidates = old re-rank call)
	texts51 := make([]string, 51)
	texts51[0] = "find the function that handles HTTP request routing"
	for i := 1; i < 51; i++ {
		texts51[i] = fmt.Sprintf("function symbol_%d does something with various parameters and returns error values", i)
	}
	t4 := time.Now()
	_, err = e.EmbedBatch(ctx, texts51)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Batch 51 (old re-rank): %v\n", time.Since(t4))

	// === Cached Re-rank Path ===
	// Set up a temp SQLite store and populate it with cached vectors.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	defer s.Close()

	searcher := NewSearcher(e)
	searcher.SetStore(s)

	// Create fake nodes and index them (populates both HNSW and SQLite cache).
	nodes := make([]types.Node, 50)
	hashes := make([]types.Hash, 50)
	fallbackTexts := make([]string, 50)
	for i := range 50 {
		h := types.NewHash([]byte(fmt.Sprintf("symbol_%d_qualified_name", i)))
		nodes[i] = types.Node{
			NodeHash:      h,
			Kind:          "function",
			QualifiedName: fmt.Sprintf("pkg.symbol_%d", i),
			Signature:     fmt.Sprintf("func symbol_%d(ctx context.Context) error", i),
		}
		hashes[i] = h
		fallbackTexts[i] = fmt.Sprintf("function symbol_%d does something with various parameters", i)
	}

	// Index (embeds + persists to SQLite).
	if err := searcher.IndexBatch(ctx, nodes, nil); err != nil {
		t.Fatalf("index batch: %v", err)
	}

	// Verify vectors are in the store.
	cached, err := s.GetEmbeddings(ctx, modelRepo, hashes[:5])
	if err != nil {
		t.Fatalf("get embeddings: %v", err)
	}
	fmt.Printf("\nCached vectors verified: %d of 5 found in SQLite\n", len(cached))

	// Time: cached re-rank (only embeds query, reads 50 vectors from SQLite).
	query := "find the function that handles HTTP request routing"
	tCached := time.Now()
	scores, err := searcher.ReRankByHashes(ctx, query, hashes, fallbackTexts)
	if err != nil {
		t.Fatalf("ReRankByHashes: %v", err)
	}
	cachedElapsed := time.Since(tCached)
	fmt.Printf("Cached re-rank (50 candidates): %v\n", cachedElapsed)

	// Sanity: scores should be non-zero.
	nonZero := 0
	for _, sc := range scores {
		if sc > 0 {
			nonZero++
		}
	}
	fmt.Printf("Non-zero scores: %d of %d\n", nonZero, len(scores))

	// Time: uncached re-rank (fresh store, no vectors cached).
	freshDB := filepath.Join(tmpDir, "fresh.db")
	freshStore, err := store.NewSQLiteStore(freshDB)
	if err != nil {
		t.Fatalf("create fresh store: %v", err)
	}
	defer freshStore.Close()

	freshSearcher := NewSearcher(e)
	freshSearcher.SetStore(freshStore)

	tUncached := time.Now()
	_, err = freshSearcher.ReRankByHashes(ctx, query, hashes, fallbackTexts)
	if err != nil {
		t.Fatalf("uncached ReRankByHashes: %v", err)
	}
	uncachedElapsed := time.Since(tUncached)
	fmt.Printf("Uncached re-rank (50 candidates): %v\n", uncachedElapsed)

	// Verify fresh store now has vectors (written on miss).
	freshCached, _ := freshStore.GetEmbeddings(ctx, modelRepo, hashes[:5])
	fmt.Printf("Vectors persisted on miss: %d of 5\n", len(freshCached))

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Old path (embed 51 texts):      %v\n", time.Duration(0)) // placeholder, measured above
	fmt.Printf("Cached path (embed 1 + lookup):  %v\n", cachedElapsed)
	fmt.Printf("Uncached path (embed 51 + save): %v\n", uncachedElapsed)
	if uncachedElapsed > 0 {
		speedup := float64(uncachedElapsed) / float64(cachedElapsed)
		fmt.Printf("Speedup (cached vs uncached):    %.1fx\n", speedup)
	}

	// Verify the file exists for the store.
	fi, _ := os.Stat(dbPath)
	if fi != nil {
		fmt.Printf("Vector cache DB size:            %d KB\n", fi.Size()/1024)
	}
}
