package context

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestVocabExpansion_EndToEnd proves the full vocabulary expansion cycle:
// 1. Index a symbol that is NOT findable by keyword
// 2. ForTask fails to find it (vocabulary gap)
// 3. Record vocab association: keyword -> symbol (simulating agent usage)
// 4. ForTask now finds it via learned equivalence class
func TestVocabExpansion_EndToEnd(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Create a repo and file.
	repoURL := "test://vocab-test"
	repoHash := types.NewHash([]byte(repoURL))
	fileHash := types.NewHash([]byte("order.py"))
	contentHash := types.NewHash([]byte("content1"))

	s.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: repoURL})
	s.PutFile(ctx, types.File{FileHash: fileHash, RepoHash: repoHash, Path: "order.py", ContentHash: contentHash})

	// Create target symbol: Order.can_cancel
	// This symbol name shares NO keywords with "checkout".
	targetHash := types.ComputeNodeHash(repoURL, "order.py", fileHash, "can_cancel", "method")
	s.PutNode(ctx, types.Node{
		NodeHash:      targetHash,
		FileHash:      fileHash,
		QualifiedName: repoURL + "://order.py.Order.can_cancel",
		Kind:          "method",
		Signature:     "def can_cancel(self) -> bool",
		Doc:           "Check if the order can be cancelled",
	})

	// Also create some other symbols so ForTask has something to find.
	for _, name := range []string{"process_payment", "validate_cart", "apply_discount"} {
		h := types.ComputeNodeHash(repoURL, "order.py", fileHash, name, "function")
		s.PutNode(ctx, types.Node{
			NodeHash:      h,
			FileHash:      fileHash,
			QualifiedName: repoURL + "://order.py." + name,
			Kind:          "function",
		})
	}
	s.RebuildFTS(ctx)

	engine := NewContextEngine(s)

	// Step 1: ForTask("checkout") should NOT find can_cancel (no keyword overlap).
	block1, err := engine.ForTask(ctx, TaskOptions{
		TaskDescription: "checkout",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatalf("ForTask(1): %v", err)
	}
	if containsSymbol(block1, "can_cancel") {
		t.Fatal("can_cancel should NOT be found before vocab expansion")
	}

	// Step 2: Simulate learned association.
	// In real usage, this happens when the agent uses can_cancel after a checkout query.
	// Record twice (count >= 2 threshold).
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", targetHash)
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", targetHash)

	// Clear any cached results.
	engine.DisablePersistentCache()

	// Step 3: ForTask("checkout") should now find can_cancel via learned equiv class.
	block2, err := engine.ForTask(ctx, TaskOptions{
		TaskDescription: "checkout",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatalf("ForTask(2): %v", err)
	}
	if !containsSymbol(block2, "can_cancel") {
		t.Error("can_cancel should be found after vocab expansion")
		t.Logf("returned %d symbols:", len(block2.Symbols))
		for i, sym := range block2.Symbols {
			t.Logf("  [%d] %s (score: %.3f)", i, sym.Node.QualifiedName, sym.Score)
		}
		// Verify the association is in the DB.
		assocs, _ := s.LearnedVocabAssociations(context.Background(), []string{"checkout"}, 2)
		t.Logf("vocab associations: %d", len(assocs))
		for _, a := range assocs {
			t.Logf("  %s -> %s (count=%d)", a.Keyword, a.SymbolName, a.Count)
		}
		// Check keywords stored on engine.
		t.Logf("engine taskKeywords: %v", engine.taskKeywords)
		// Check if NodesByName finds the target.
		nodes, _ := s.NodesByName(context.Background(), "%can_cancel")
		t.Logf("NodesByName(%%can_cancel): %d nodes", len(nodes))
		for _, n := range nodes {
			t.Logf("  %s", n.QualifiedName)
		}
	}
}

// TestVocabExpansion_BelowThreshold verifies that a single observation
// is not enough to create a learned class (prevents noise).
func TestVocabExpansion_BelowThreshold(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	repoURL := "test://vocab-threshold"
	repoHash := types.NewHash([]byte(repoURL))
	fileHash := types.NewHash([]byte("f.py"))
	contentHash := types.NewHash([]byte("c1"))

	s.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: repoURL})
	s.PutFile(ctx, types.File{FileHash: fileHash, RepoHash: repoHash, Path: "f.py", ContentHash: contentHash})

	targetHash := types.ComputeNodeHash(repoURL, "f.py", fileHash, "rare_func", "function")
	s.PutNode(ctx, types.Node{
		NodeHash:      targetHash,
		FileHash:      fileHash,
		QualifiedName: repoURL + "://f.py.rare_func",
		Kind:          "function",
	})
	s.RebuildFTS(ctx)

	// Only record once (below threshold).
	s.RecordVocabAssociation(ctx, "magic", "rare_func", targetHash)

	engine := NewContextEngine(s)
	engine.DisablePersistentCache()

	block, err := engine.ForTask(ctx, TaskOptions{
		TaskDescription: "magic",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if containsSymbol(block, "rare_func") {
		t.Error("rare_func should NOT be found with only 1 observation (below threshold)")
	}
}

func containsSymbol(block *ContextBlock, name string) bool {
	if block == nil {
		return false
	}
	for _, sym := range block.Symbols {
		qn := sym.Node.QualifiedName
		lastDot := len(qn) - 1
		for lastDot >= 0 && qn[lastDot] != '.' {
			lastDot--
		}
		symName := qn[lastDot+1:]
		if symName == name {
			return true
		}
	}
	return false
}
