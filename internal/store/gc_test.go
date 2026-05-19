package store

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestDeleteNodesNotIn(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	repo := makeRepo(t, s, "https://github.com/test/repo")
	file := makeFile(t, s, repo, "pkg/main.go")

	// Create 5 nodes.
	n1 := makeNode(t, s, file, "Func1", "function")
	n2 := makeNode(t, s, file, "Func2", "function")
	n3 := makeNode(t, s, file, "Func3", "function")
	n4 := makeNode(t, s, file, "Func4", "function")
	n5 := makeNode(t, s, file, "Func5", "function")

	// Keep only 3 of them.
	keep := map[types.Hash]struct{}{
		n1.NodeHash: {},
		n2.NodeHash: {},
		n3.NodeHash: {},
	}

	deleted, err := s.DeleteNodesNotIn(ctx, keep)
	if err != nil {
		t.Fatalf("DeleteNodesNotIn: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// Verify that n4 and n5 are gone and n1, n2, n3 remain.
	_ = n4
	_ = n5
	remaining, err := s.NodesByName(ctx, "")
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}
	if len(remaining) != 3 {
		t.Errorf("expected 3 remaining nodes, got %d", len(remaining))
	}

	remainingHashes := make(map[types.Hash]struct{}, len(remaining))
	for _, n := range remaining {
		remainingHashes[n.NodeHash] = struct{}{}
	}
	for _, h := range []types.Hash{n1.NodeHash, n2.NodeHash, n3.NodeHash} {
		if _, ok := remainingHashes[h]; !ok {
			t.Errorf("expected node %s to remain but it was deleted", h)
		}
	}
}

func TestDeleteEdgesNotIn(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	repo := makeRepo(t, s, "https://github.com/test/repo-edges")
	file := makeFile(t, s, repo, "pkg/main.go")

	n1 := makeNode(t, s, file, "Func1", "function")
	n2 := makeNode(t, s, file, "Func2", "function")
	n3 := makeNode(t, s, file, "Func3", "function")
	n4 := makeNode(t, s, file, "Func4", "function")

	// Create 4 edges.
	e1 := makeEdge(t, s, n1, n2, "calls")
	e2 := makeEdge(t, s, n2, n3, "calls")
	e3 := makeEdge(t, s, n3, n4, "calls")
	e4 := makeEdge(t, s, n4, n1, "calls")

	// Keep only 2 of them.
	keep := map[types.Hash]struct{}{
		e1.EdgeHash: {},
		e2.EdgeHash: {},
	}

	deleted, err := s.DeleteEdgesNotIn(ctx, keep)
	if err != nil {
		t.Fatalf("DeleteEdgesNotIn: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted, got %d", deleted)
	}

	// Verify e1 and e2 remain; e3 and e4 are gone.
	_ = e3
	_ = e4
	e1check, err := s.GetEdge(ctx, e1.EdgeHash)
	if err != nil || e1check == nil {
		t.Errorf("expected e1 to remain, got err=%v, edge=%v", err, e1check)
	}
	e2check, err := s.GetEdge(ctx, e2.EdgeHash)
	if err != nil || e2check == nil {
		t.Errorf("expected e2 to remain, got err=%v, edge=%v", err, e2check)
	}
	e3check, err := s.GetEdge(ctx, e3.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge e3: %v", err)
	}
	if e3check != nil {
		t.Error("expected e3 to be deleted but it still exists")
	}
	e4check, err := s.GetEdge(ctx, e4.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge e4: %v", err)
	}
	if e4check != nil {
		t.Error("expected e4 to be deleted but it still exists")
	}
}

func TestDeleteNodesNotIn_EmptyKeepSet(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	repo := makeRepo(t, s, "https://github.com/test/repo-empty-keep")
	file := makeFile(t, s, repo, "pkg/main.go")

	makeNode(t, s, file, "Func1", "function")
	makeNode(t, s, file, "Func2", "function")
	makeNode(t, s, file, "Func3", "function")

	// Empty keep set: all nodes should be deleted.
	deleted, err := s.DeleteNodesNotIn(ctx, map[types.Hash]struct{}{})
	if err != nil {
		t.Fatalf("DeleteNodesNotIn: %v", err)
	}
	if deleted != 3 {
		t.Errorf("expected 3 deleted, got %d", deleted)
	}

	remaining, err := s.NodesByName(ctx, "")
	if err != nil {
		t.Fatalf("NodesByName: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining nodes, got %d", len(remaining))
	}
}

func TestDeleteEdgesNotIn_EmptyDB(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Call on empty DB.
	deleted, err := s.DeleteEdgesNotIn(ctx, map[types.Hash]struct{}{})
	if err != nil {
		t.Fatalf("DeleteEdgesNotIn on empty DB: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted from empty DB, got %d", deleted)
	}
}
