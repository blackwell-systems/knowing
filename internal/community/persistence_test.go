package community

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func tempStore(t *testing.T) types.GraphStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSaveAndLoadAssignments(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	a1, a2, a3 := hash("node-a"), hash("node-b"), hash("node-c")
	assignments := map[types.Hash]int{
		a1: 0,
		a2: 0,
		a3: 1,
	}

	if err := SaveAssignments(ctx, st, assignments); err != nil {
		t.Fatalf("SaveAssignments: %v", err)
	}

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatalf("LoadAssignments: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(loaded))
	}
	if loaded[a1] != 0 || loaded[a2] != 0 || loaded[a3] != 1 {
		t.Errorf("loaded assignments don't match: %v", loaded)
	}
}

func TestLoadAssignments_Empty(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatalf("LoadAssignments: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for empty notes, got %v", loaded)
	}
}

func TestSaveAssignments_Upsert(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	a1 := hash("node-a")

	// First save: community 0.
	if err := SaveAssignments(ctx, st, map[types.Hash]int{a1: 0}); err != nil {
		t.Fatal(err)
	}

	// Second save: community 5 (upsert).
	if err := SaveAssignments(ctx, st, map[types.Hash]int{a1: 5}); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[a1] != 5 {
		t.Errorf("expected community 5 after upsert, got %d", loaded[a1])
	}
}

func TestSaveChangedAssignments_DeltaOnly(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	a1, a2, a3 := hash("node-a"), hash("node-b"), hash("node-c")
	previous := map[types.Hash]int{a1: 0, a2: 0, a3: 1}
	if err := SaveAssignments(ctx, st, previous); err != nil {
		t.Fatal(err)
	}

	// Change a3 from community 1 to community 0. a1, a2 unchanged.
	current := map[types.Hash]int{a1: 0, a2: 0, a3: 0}
	if err := SaveChangedAssignments(ctx, st, current, previous); err != nil {
		t.Fatalf("SaveChangedAssignments: %v", err)
	}

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[a3] != 0 {
		t.Errorf("a3 should be community 0 after delta save, got %d", loaded[a3])
	}
	if loaded[a1] != 0 || loaded[a2] != 0 {
		t.Error("unchanged nodes should retain their assignments")
	}
}

func TestSaveChangedAssignments_DeletesRemoved(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	a1, a2 := hash("node-a"), hash("node-b")
	previous := map[types.Hash]int{a1: 0, a2: 1}
	if err := SaveAssignments(ctx, st, previous); err != nil {
		t.Fatal(err)
	}

	// a2 removed from graph.
	current := map[types.Hash]int{a1: 0}
	if err := SaveChangedAssignments(ctx, st, current, previous); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Errorf("expected 1 assignment after removal, got %d", len(loaded))
	}
	if _, exists := loaded[a2]; exists {
		t.Error("removed node a2 should not have an assignment")
	}
}

func TestSaveChangedAssignments_NoChanges(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	a1 := hash("node-a")
	same := map[types.Hash]int{a1: 0}
	if err := SaveAssignments(ctx, st, same); err != nil {
		t.Fatal(err)
	}

	// Same assignments: should be a no-op.
	if err := SaveChangedAssignments(ctx, st, same, same); err != nil {
		t.Fatalf("SaveChangedAssignments: %v", err)
	}

	loaded, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[a1] != 0 {
		t.Errorf("expected community 0, got %d", loaded[a1])
	}
}

func TestIncrementalWithPersistedAssignments(t *testing.T) {
	st := tempStore(t)
	ctx := context.Background()

	g := buildTwoClusterGraph()
	algo := &Louvain{Resolution: 1.0, MaxPasses: 20}

	// Full detection.
	full := algo.Detect(g)

	// Persist.
	if err := SaveAssignments(ctx, st, full); err != nil {
		t.Fatal(err)
	}

	// Load and use for incremental.
	previous, err := LoadAssignments(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if previous == nil {
		t.Fatal("expected non-nil previous assignments")
	}

	// Incremental with no changes should match.
	noChanges := make(map[types.Hash]bool)
	inc := algo.DetectIncremental(g, previous, noChanges)

	for _, n := range g.Nodes {
		if inc[n] != full[n] {
			t.Errorf("node %s: incremental %d != full %d", n, inc[n], full[n])
		}
	}
}
