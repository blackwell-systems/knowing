package context

import (
	"github.com/blackwell-systems/knowing/internal/types"
)

// PackDiff describes the difference between two context packs.
type PackDiff struct {
	// OldPackRoot is the PackRoot of the first pack.
	OldPackRoot types.Hash
	// NewPackRoot is the PackRoot of the second pack.
	NewPackRoot types.Hash

	// AddedSymbols are in new but not in old.
	AddedSymbols []string
	// RemovedSymbols are in old but not in new.
	RemovedSymbols []string
	// CommonSymbols are in both.
	CommonSymbols []string

	// Identical is true if the packs have the same symbols (PackRoots may
	// still differ if token budgets differ).
	Identical bool
}

// CompareContextPacks computes the symmetric difference between two context
// blocks. This answers "what changed in the context this agent would see?"
func CompareContextPacks(old, new *ContextBlock) PackDiff {
	diff := PackDiff{
		OldPackRoot: old.PackRoot,
		NewPackRoot: new.PackRoot,
	}

	oldSet := make(map[string]bool, len(old.Symbols))
	for _, s := range old.Symbols {
		oldSet[s.Node.QualifiedName] = true
	}

	newSet := make(map[string]bool, len(new.Symbols))
	for _, s := range new.Symbols {
		newSet[s.Node.QualifiedName] = true
	}

	for name := range newSet {
		if oldSet[name] {
			diff.CommonSymbols = append(diff.CommonSymbols, name)
		} else {
			diff.AddedSymbols = append(diff.AddedSymbols, name)
		}
	}

	for name := range oldSet {
		if !newSet[name] {
			diff.RemovedSymbols = append(diff.RemovedSymbols, name)
		}
	}

	diff.Identical = len(diff.AddedSymbols) == 0 && len(diff.RemovedSymbols) == 0
	return diff
}
