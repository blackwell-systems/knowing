package snapshot

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// generateEdges creates a realistic graph with N edges across P packages and T edge types.
func generateEdges(n, packages, edgeTypes int) []EdgeInput {
	edges := make([]EdgeInput, n)
	pkgs := make([]string, packages)
	for i := range pkgs {
		pkgs[i] = fmt.Sprintf("internal/pkg%d", i)
	}
	types_ := []string{"calls", "imports", "implements", "references", "throws"}
	if edgeTypes < len(types_) {
		types_ = types_[:edgeTypes]
	}

	for i := 0; i < n; i++ {
		content := fmt.Sprintf("edge-%d-%d", i, rand.Int())
		edges[i] = EdgeInput{
			EdgeHash:    types.NewHash([]byte(content)),
			PackagePath: pkgs[rand.Intn(len(pkgs))],
			EdgeType:    types_[rand.Intn(len(types_))],
		}
	}
	return edges
}

// BenchmarkFlatDiff_10K benchmarks flat Merkle diff on 10K edges where 1 package changed.
func BenchmarkFlatDiff_10K(b *testing.B) {
	benchmarkFlatVsHierarchical(b, 10000, 20, 5)
}

// BenchmarkFlatDiff_50K benchmarks flat Merkle diff on 50K edges.
func BenchmarkFlatDiff_50K(b *testing.B) {
	benchmarkFlatVsHierarchical(b, 50000, 50, 5)
}

// BenchmarkFlatDiff_100K benchmarks flat Merkle diff on 100K edges.
func BenchmarkFlatDiff_100K(b *testing.B) {
	benchmarkFlatVsHierarchical(b, 100000, 100, 5)
}

func benchmarkFlatVsHierarchical(b *testing.B, edgeCount, packages, edgeTypes int) {
	// Build "old" state.
	oldEdges := generateEdges(edgeCount, packages, edgeTypes)

	// Build "new" state: change 1 package (5% of edges).
	newEdges := make([]EdgeInput, len(oldEdges))
	copy(newEdges, oldEdges)
	targetPkg := fmt.Sprintf("internal/pkg%d", 0)
	for i := range newEdges {
		if newEdges[i].PackagePath == targetPkg {
			content := fmt.Sprintf("changed-edge-%d-%d", i, rand.Int())
			newEdges[i].EdgeHash = types.NewHash([]byte(content))
		}
	}

	// Precompute trees.
	oldHTree := BuildHierarchicalTree(oldEdges)
	newHTree := BuildHierarchicalTree(newEdges)

	oldHashes := make([]types.Hash, len(oldEdges))
	for i, e := range oldEdges {
		oldHashes[i] = e.EdgeHash
	}
	newHashes := make([]types.Hash, len(newEdges))
	for i, e := range newEdges {
		newHashes[i] = e.EdgeHash
	}
	oldFlat := BuildMerkleTree(oldHashes)
	newFlat := BuildMerkleTree(newHashes)

	b.Run("flat_diff", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			DiffMerkle(oldFlat, newFlat)
		}
	})

	b.Run("hierarchical_diff", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			DiffHierarchicalTrees(oldHTree, newHTree)
		}
	})

	b.Run("hierarchical_subgraph_root", func(b *testing.B) {
		pkgs := []string{targetPkg, fmt.Sprintf("internal/pkg%d", 1)}
		for i := 0; i < b.N; i++ {
			oldHTree.SubgraphRoot(pkgs)
		}
	})

	b.Run("hierarchical_edge_type_root", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			oldHTree.EdgeTypeRoot("calls")
		}
	})

	b.Run("flat_build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BuildMerkleTree(oldHashes)
		}
	})

	b.Run("hierarchical_build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			BuildHierarchicalTree(oldEdges)
		}
	})
}
