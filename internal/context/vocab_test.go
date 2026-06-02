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

// TestVocabExpansion_CrossTask proves cross-task vocabulary bridging:
// Task A ("payment processing") teaches vocab associations for "payment" keyword.
// Task B ("payment refund") benefits because it shares the "payment" keyword,
// even though its own keyword "refund" has no association.
//
// This validates the primary value proposition of vocab expansion: one task's
// learning helps a DIFFERENT task via shared keywords.
func TestVocabExpansion_CrossTask(t *testing.T) {
	ctx := context.Background()
	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	repoURL := "test://cross-task-vocab"
	repoHash := types.NewHash([]byte(repoURL))
	fileHash := types.NewHash([]byte("gateway.py"))
	contentHash := types.NewHash([]byte("content1"))

	s.PutRepo(ctx, types.Repo{RepoHash: repoHash, RepoURL: repoURL})
	s.PutFile(ctx, types.File{FileHash: fileHash, RepoHash: repoHash, Path: "gateway.py", ContentHash: contentHash})

	// Target symbols: names and QN share NO keywords with "payment" or "refund".
	// "settle_ledger" under "Ledger" type (not "PaymentGateway") is invisible to BM25
	// for "payment refund". After Task A teaches "payment" -> settle_ledger, Task B finds it.
	settleHash := types.ComputeNodeHash(repoURL, "gateway.py", fileHash, "settle_ledger", "function")
	s.PutNode(ctx, types.Node{
		NodeHash:      settleHash,
		FileHash:      fileHash,
		QualifiedName: repoURL + "://gateway.py.Ledger.settle_ledger",
		Kind:          "function",
		Signature:     "def settle_ledger(self, batch_id: str) -> bool",
	})

	reconcileHash := types.ComputeNodeHash(repoURL, "gateway.py", fileHash, "reconcile_batch", "function")
	s.PutNode(ctx, types.Node{
		NodeHash:      reconcileHash,
		FileHash:      fileHash,
		QualifiedName: repoURL + "://gateway.py.Ledger.reconcile_batch",
		Kind:          "function",
		Signature:     "def reconcile_batch(self) -> list",
	})

	// Noise symbols so ForTask has something to match against.
	for _, name := range []string{"validate_input", "format_response", "log_error"} {
		h := types.ComputeNodeHash(repoURL, "gateway.py", fileHash, name, "function")
		s.PutNode(ctx, types.Node{
			NodeHash:      h,
			FileHash:      fileHash,
			QualifiedName: repoURL + "://gateway.py." + name,
			Kind:          "function",
		})
	}
	s.RebuildFTS(ctx)

	engine := NewContextEngine(s)
	engine.DisablePersistentCache()

	// Step 1: Task B ("payment refund") cannot find "settle_ledger" (no keyword overlap).
	blockBefore, err := engine.ForTask(ctx, TaskOptions{
		TaskDescription: "payment refund",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatalf("ForTask(before): %v", err)
	}
	if containsSymbol(blockBefore, "settle_ledger") {
		t.Fatal("settle_ledger should NOT be found before cross-task vocab learning")
	}

	// Step 2: Task A ("payment processing") runs and the agent uses settle_ledger and reconcile_batch.
	// Simulate vocab recording: keyword "payment" -> settle_ledger, reconcile_batch.
	// Record twice each (count >= 2 threshold).
	for i := 0; i < 2; i++ {
		s.RecordVocabAssociation(ctx, "payment", "settle_ledger", settleHash)
		s.RecordVocabAssociation(ctx, "payment", "reconcile_batch", reconcileHash)
	}

	// Step 3: Task B ("payment refund") now finds settle_ledger via learned vocab.
	// Task B's keywords include "payment" (shared with Task A's vocab), so the
	// learned association bridges the vocabulary gap.
	engine2 := NewContextEngine(s)
	engine2.DisablePersistentCache()

	blockAfter, err := engine2.ForTask(ctx, TaskOptions{
		TaskDescription: "payment refund",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatalf("ForTask(after): %v", err)
	}

	foundSettle := containsSymbol(blockAfter, "settle_ledger")
	foundReconcile := containsSymbol(blockAfter, "reconcile_batch")

	if !foundSettle && !foundReconcile {
		t.Error("cross-task vocab bridging failed: neither settle_ledger nor reconcile_batch found after Task A taught 'payment' associations")
		t.Logf("returned %d symbols:", len(blockAfter.Symbols))
		for i, sym := range blockAfter.Symbols {
			t.Logf("  [%d] %s (score: %.3f)", i, sym.Node.QualifiedName, sym.Score)
		}
		assocs, _ := s.LearnedVocabAssociations(context.Background(), []string{"payment"}, 2)
		t.Logf("vocab associations for 'payment': %d", len(assocs))
		for _, a := range assocs {
			t.Logf("  %s -> %s (count=%d)", a.Keyword, a.SymbolName, a.Count)
		}
	}

	if foundSettle {
		t.Log("cross-task bridging confirmed: 'settle_ledger' found via 'payment' keyword learned from Task A")
	}
	if foundReconcile {
		t.Log("cross-task bridging confirmed: 'reconcile_batch' found via 'payment' keyword learned from Task A")
	}

	// Step 4: Verify that a completely unrelated task does NOT benefit.
	// "logging errors" shares no keywords with "payment".
	engine3 := NewContextEngine(s)
	engine3.DisablePersistentCache()

	blockUnrelated, err := engine3.ForTask(ctx, TaskOptions{
		TaskDescription: "logging errors",
		TokenBudget:     5000,
		RepoURL:         repoURL,
	})
	if err != nil {
		t.Fatalf("ForTask(unrelated): %v", err)
	}
	if containsSymbol(blockUnrelated, "settle_ledger") {
		t.Error("unrelated task 'logging errors' should NOT find settle_ledger (no keyword overlap)")
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
