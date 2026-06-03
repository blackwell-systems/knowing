package store

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestVocabAssociation_RecordAndQuery(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hash := types.NewHash([]byte("Order.can_cancel"))

	// Record once: count should be 1.
	if err := s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash); err != nil {
		t.Fatalf("RecordVocabAssociation: %v", err)
	}

	// Query with minCount=1: should find it.
	assocs, err := s.LearnedVocabAssociations(ctx, []string{"checkout"}, 1)
	if err != nil {
		t.Fatalf("LearnedVocabAssociations: %v", err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association, got %d", len(assocs))
	}
	if assocs[0].SymbolName != "can_cancel" || assocs[0].Count != 1 {
		t.Errorf("got %+v", assocs[0])
	}

	// Query with minCount=2: should NOT find it (only 1 observation).
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2)
	if err != nil {
		t.Fatalf("LearnedVocabAssociations(min=2): %v", err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 associations with minCount=2, got %d", len(assocs))
	}

	// Record again: count should increment to 2.
	if err := s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash); err != nil {
		t.Fatalf("RecordVocabAssociation(2nd): %v", err)
	}

	// Now minCount=2 should find it.
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2)
	if err != nil {
		t.Fatalf("LearnedVocabAssociations(min=2 after 2nd): %v", err)
	}
	if len(assocs) != 1 || assocs[0].Count != 2 {
		t.Errorf("expected count=2, got %+v", assocs)
	}
}

func TestVocabAssociation_MultipleKeywords(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hashA := types.NewHash([]byte("Order.can_cancel"))
	hashB := types.NewHash([]byte("Cart.checkout"))

	// "checkout" -> can_cancel (2x), "order" -> can_cancel (1x), "checkout" -> Cart.checkout (1x)
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hashA)
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hashA)
	s.RecordVocabAssociation(ctx, "order", "can_cancel", hashA)
	s.RecordVocabAssociation(ctx, "checkout", "checkout", hashB)

	// Query both keywords with minCount=1.
	assocs, err := s.LearnedVocabAssociations(ctx, []string{"checkout", "order"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Should get 3: checkout->can_cancel(2), checkout->checkout(1), order->can_cancel(1)
	if len(assocs) != 3 {
		t.Fatalf("expected 3 associations, got %d", len(assocs))
	}
	// First should be highest count (checkout -> can_cancel, count=2).
	if assocs[0].Count != 2 || assocs[0].Keyword != "checkout" {
		t.Errorf("first assoc should be checkout->can_cancel(2), got %+v", assocs[0])
	}

	// Query with minCount=2: only checkout->can_cancel.
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout", "order"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association with minCount=2, got %d", len(assocs))
	}
}

func TestVocabAssociation_UnrelatedKeyword(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hash := types.NewHash([]byte("Order.can_cancel"))
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash)
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash)

	// Query with unrelated keyword should return nothing.
	assocs, err := s.LearnedVocabAssociations(ctx, []string{"migration"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 for unrelated keyword, got %d", len(assocs))
	}
}

func TestVocabAssociation_MerkleExpiration(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hash := types.NewHash([]byte("Order.can_cancel"))
	rootV1 := types.NewHash([]byte("package-root-v1"))
	rootV2 := types.NewHash([]byte("package-root-v2"))

	// Record association with subgraph root v1 (twice to meet threshold).
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash, rootV1)
	s.RecordVocabAssociation(ctx, "checkout", "can_cancel", hash, rootV1)

	// Query without roots: association visible (backward compatible).
	assocs, err := s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association without roots filter, got %d", len(assocs))
	}
	if assocs[0].SubgraphRoot != rootV1 {
		t.Error("stored subgraph root doesn't match v1")
	}

	// Query with matching root: association visible.
	currentRoots := map[types.Hash]types.Hash{hash: rootV1}
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2, currentRoots)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association with matching root, got %d", len(assocs))
	}

	// Query with changed root (v2): association EXPIRED (invisible).
	changedRoots := map[types.Hash]types.Hash{hash: rootV2}
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2, changedRoots)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 0 {
		t.Errorf("expected 0 associations after root change (Merkle expiration), got %d", len(assocs))
	}

	// Query with unknown symbol hash (not in roots map): association visible
	// (we don't filter symbols we don't have roots for).
	partialRoots := map[types.Hash]types.Hash{types.NewHash([]byte("other")): rootV2}
	assocs, err = s.LearnedVocabAssociations(ctx, []string{"checkout"}, 2, partialRoots)
	if err != nil {
		t.Fatal(err)
	}
	if len(assocs) != 1 {
		t.Fatalf("expected 1 association when symbol not in roots map, got %d", len(assocs))
	}
}
