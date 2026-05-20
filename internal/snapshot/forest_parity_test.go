package snapshot

import (
	"testing"

	forest "github.com/blackwell-systems/merkle-forest"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestForestParity verifies that merkle-forest produces identical roots to
// knowing's internal BuildHierarchicalTree when using WithPrefix("merkle\x00").
//
// If this test passes, knowing can safely replace its internal Merkle
// implementation with merkle-forest as a dependency.
func TestForestParity(t *testing.T) {
	// Build a hierarchical tree using knowing's internal implementation.
	edges := []EdgeInput{
		{EdgeHash: types.NewHash([]byte("e1")), PackagePath: "pkg/auth", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e2")), PackagePath: "pkg/auth", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e3")), PackagePath: "pkg/auth", EdgeType: "imports"},
		{EdgeHash: types.NewHash([]byte("e4")), PackagePath: "pkg/store", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e5")), PackagePath: "pkg/store", EdgeType: "references"},
	}

	knowingTree := BuildHierarchicalTree(edges)

	// Build the same tree using merkle-forest.
	var inputs []forest.MultiLevelInput
	for _, e := range edges {
		inputs = append(inputs, forest.MultiLevelInput{
			Leaf:     forest.Hash(e.EdgeHash),
			Group:    e.PackagePath,
			Subgroup: e.EdgeType,
		})
	}

	forestTree := forest.BuildMultiLevel(inputs, forest.WithPrefix([]byte("merkle\x00")))

	// Compare roots.
	if types.Hash(forestTree.Root) != knowingTree.Root {
		t.Fatalf("ROOT MISMATCH:\n  knowing:       %x\n  merkle-forest: %x", knowingTree.Root, forestTree.Root)
	}

	// Compare package roots.
	for pkg, knowingRoot := range knowingTree.PackageRoots {
		forestRoot, ok := forestTree.GroupRoots[pkg]
		if !ok {
			t.Fatalf("merkle-forest missing group %q", pkg)
		}
		if types.Hash(forestRoot) != knowingRoot {
			t.Fatalf("GROUP ROOT MISMATCH for %q:\n  knowing:       %x\n  merkle-forest: %x", pkg, knowingRoot, forestRoot)
		}
	}

	// Compare edge-type roots.
	for key, knowingRoot := range knowingTree.EdgeTypeRoots {
		forestRoot, ok := forestTree.SubgroupRoots[key]
		if !ok {
			t.Fatalf("merkle-forest missing subgroup %q", key)
		}
		if types.Hash(forestRoot) != knowingRoot {
			t.Fatalf("SUBGROUP ROOT MISMATCH for %q:\n  knowing:       %x\n  merkle-forest: %x", key, knowingRoot, forestRoot)
		}
	}

	// Compare SubgraphRoot.
	knowingSub := knowingTree.SubgraphRoot([]string{"pkg/auth"})
	forestSub := forestTree.SubgraphRoot([]string{"pkg/auth"})
	if types.Hash(forestSub) != knowingSub {
		t.Fatalf("SUBGRAPH ROOT MISMATCH:\n  knowing:       %x\n  merkle-forest: %x", knowingSub, forestSub)
	}

	t.Logf("PARITY VERIFIED: merkle-forest produces identical output at all levels")
	t.Logf("  Root:            %x", knowingTree.Root)
	t.Logf("  pkg/auth root:   %x", knowingTree.PackageRoots["pkg/auth"])
	t.Logf("  pkg/store root:  %x", knowingTree.PackageRoots["pkg/store"])
	t.Logf("  SubgraphRoot:    %x", knowingSub)
}
