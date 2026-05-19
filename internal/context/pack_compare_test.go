package context

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeBlock(names []string) *ContextBlock {
	block := &ContextBlock{}
	for _, name := range names {
		block.Symbols = append(block.Symbols, RankedSymbol{
			Node: types.Node{
				NodeHash:      types.NewHash([]byte(name)),
				QualifiedName: name,
			},
		})
	}
	return block
}

func TestCompareContextPacks_Identical(t *testing.T) {
	old := makeBlock([]string{"a", "b", "c"})
	new := makeBlock([]string{"a", "b", "c"})
	diff := CompareContextPacks(old, new)

	if !diff.Identical {
		t.Error("expected Identical=true for same symbols")
	}
	if len(diff.AddedSymbols) != 0 {
		t.Errorf("expected 0 added, got %d", len(diff.AddedSymbols))
	}
	if len(diff.RemovedSymbols) != 0 {
		t.Errorf("expected 0 removed, got %d", len(diff.RemovedSymbols))
	}
	if len(diff.CommonSymbols) != 3 {
		t.Errorf("expected 3 common, got %d", len(diff.CommonSymbols))
	}
}

func TestCompareContextPacks_AllNew(t *testing.T) {
	old := makeBlock([]string{"a", "b"})
	new := makeBlock([]string{"x", "y", "z"})
	diff := CompareContextPacks(old, new)

	if diff.Identical {
		t.Error("expected Identical=false")
	}
	if len(diff.AddedSymbols) != 3 {
		t.Errorf("expected 3 added, got %d", len(diff.AddedSymbols))
	}
	if len(diff.RemovedSymbols) != 2 {
		t.Errorf("expected 2 removed, got %d", len(diff.RemovedSymbols))
	}
	if len(diff.CommonSymbols) != 0 {
		t.Errorf("expected 0 common, got %d", len(diff.CommonSymbols))
	}
}

func TestCompareContextPacks_Overlap(t *testing.T) {
	old := makeBlock([]string{"a", "b", "c"})
	new := makeBlock([]string{"b", "c", "d"})
	diff := CompareContextPacks(old, new)

	if diff.Identical {
		t.Error("expected Identical=false")
	}
	if len(diff.AddedSymbols) != 1 {
		t.Errorf("expected 1 added (d), got %d: %v", len(diff.AddedSymbols), diff.AddedSymbols)
	}
	if len(diff.RemovedSymbols) != 1 {
		t.Errorf("expected 1 removed (a), got %d: %v", len(diff.RemovedSymbols), diff.RemovedSymbols)
	}
	if len(diff.CommonSymbols) != 2 {
		t.Errorf("expected 2 common (b,c), got %d", len(diff.CommonSymbols))
	}
}

func TestCompareContextPacks_EmptyOld(t *testing.T) {
	old := makeBlock(nil)
	new := makeBlock([]string{"a", "b"})
	diff := CompareContextPacks(old, new)

	if diff.Identical {
		t.Error("expected Identical=false")
	}
	if len(diff.AddedSymbols) != 2 {
		t.Errorf("expected 2 added, got %d", len(diff.AddedSymbols))
	}
	if len(diff.RemovedSymbols) != 0 {
		t.Errorf("expected 0 removed, got %d", len(diff.RemovedSymbols))
	}
}

func TestCompareContextPacks_EmptyNew(t *testing.T) {
	old := makeBlock([]string{"a", "b"})
	new := makeBlock(nil)
	diff := CompareContextPacks(old, new)

	if diff.Identical {
		t.Error("expected Identical=false")
	}
	if len(diff.AddedSymbols) != 0 {
		t.Errorf("expected 0 added, got %d", len(diff.AddedSymbols))
	}
	if len(diff.RemovedSymbols) != 2 {
		t.Errorf("expected 2 removed, got %d", len(diff.RemovedSymbols))
	}
}

func TestCompareContextPacks_BothEmpty(t *testing.T) {
	old := makeBlock(nil)
	new := makeBlock(nil)
	diff := CompareContextPacks(old, new)

	if !diff.Identical {
		t.Error("expected Identical=true for two empty packs")
	}
}
