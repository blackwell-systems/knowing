package snapshot

import (
	"testing"

	strata "github.com/blackwell-systems/merkle-strata"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestStrataParity verifies that merkle-strata produces identical roots to
// knowing's internal BuildHierarchicalTree when using WithPrefix("merkle\x00").
//
// If this test passes, knowing can safely replace its internal Merkle
// implementation with merkle-forest as a dependency.
func TestStrataParity(t *testing.T) {
	// Build a hierarchical tree using knowing's internal implementation.
	edges := []EdgeInput{
		{EdgeHash: types.NewHash([]byte("e1")), PackagePath: "pkg/auth", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e2")), PackagePath: "pkg/auth", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e3")), PackagePath: "pkg/auth", EdgeType: "imports"},
		{EdgeHash: types.NewHash([]byte("e4")), PackagePath: "pkg/store", EdgeType: "calls"},
		{EdgeHash: types.NewHash([]byte("e5")), PackagePath: "pkg/store", EdgeType: "references"},
	}

	knowingTree := BuildHierarchicalTree(edges)

	// Build the same tree using merkle-strata.
	var inputs []strata.MultiLevelInput
	for _, e := range edges {
		inputs = append(inputs, strata.MultiLevelInput{
			Leaf:     strata.Hash(e.EdgeHash),
			Group:    e.PackagePath,
			Subgroup: e.EdgeType,
		})
	}

	strataTree := strata.BuildMultiLevel(inputs, strata.WithPrefix([]byte("merkle\x00")))

	// Compare roots.
	if types.Hash(strataTree.Root) != knowingTree.Root {
		t.Fatalf("ROOT MISMATCH:\n  knowing:       %x\n  merkle-strata: %x", knowingTree.Root, strataTree.Root)
	}

	// Compare package roots.
	for pkg, knowingRoot := range knowingTree.PackageRoots {
		forestRoot, ok := strataTree.GroupRoots[pkg]
		if !ok {
			t.Fatalf("merkle-forest missing group %q", pkg)
		}
		if types.Hash(forestRoot) != knowingRoot {
			t.Fatalf("GROUP ROOT MISMATCH for %q:\n  knowing:       %x\n  merkle-strata: %x", pkg, knowingRoot, forestRoot)
		}
	}

	// Compare edge-type roots.
	for key, knowingRoot := range knowingTree.EdgeTypeRoots {
		forestRoot, ok := strataTree.SubgroupRoots[key]
		if !ok {
			t.Fatalf("merkle-forest missing subgroup %q", key)
		}
		if types.Hash(forestRoot) != knowingRoot {
			t.Fatalf("SUBGROUP ROOT MISMATCH for %q:\n  knowing:       %x\n  merkle-strata: %x", key, knowingRoot, forestRoot)
		}
	}

	// Compare SubgraphRoot.
	knowingSub := knowingTree.SubgraphRoot([]string{"pkg/auth"})
	forestSub := strataTree.SubgraphRoot([]string{"pkg/auth"})
	if types.Hash(forestSub) != knowingSub {
		t.Fatalf("SUBGRAPH ROOT MISMATCH:\n  knowing:       %x\n  merkle-strata: %x", knowingSub, forestSub)
	}

	t.Logf("PARITY VERIFIED: merkle-strata produces identical output at all levels")
	t.Logf("  Root:            %x", knowingTree.Root)
	t.Logf("  pkg/auth root:   %x", knowingTree.PackageRoots["pkg/auth"])
	t.Logf("  pkg/store root:  %x", knowingTree.PackageRoots["pkg/store"])
	t.Logf("  SubgraphRoot:    %x", knowingSub)
}
