package snapshot

import (
	"bytes"
	"sort"

	forest "github.com/blackwell-systems/merkle-forest"
	"github.com/blackwell-systems/knowing/internal/types"
)

// forestPrefix matches knowing's historical "merkle\x00" domain prefix.
var forestPrefix = []byte("merkle\x00")

// MerkleTree represents a binary Merkle tree built from sorted hashes.
// The tree is constructed bottom-up: leaves are sorted edge hashes, and
// each internal node is SHA-256(left_child || right_child). The root
// hash uniquely identifies the set of leaves. Comparing two roots in O(1)
// determines whether the edge sets are identical.
type MerkleTree struct {
	Root   types.Hash   // the Merkle root (top of the tree)
	Leaves []types.Hash // sorted leaf hashes (the input edge hashes)
}

// BuildMerkleTree constructs a binary Merkle tree from a slice of edge hashes.
// Hashes are sorted lexicographically using bytes.Compare before tree construction.
// Returns a MerkleTree with the root hash and sorted leaves.
func BuildMerkleTree(hashes []types.Hash) *MerkleTree {
	if len(hashes) == 0 {
		return &MerkleTree{Root: types.EmptyHash, Leaves: nil}
	}

	sorted := make([]types.Hash, len(hashes))
	copy(sorted, hashes)
	sort.Slice(sorted, func(i, j int) bool {
		return bytes.Compare(sorted[i][:], sorted[j][:]) < 0
	})

	// Delegate to merkle-forest: build a single-group forest.
	forestHashes := make([]forest.Hash, len(sorted))
	for i, h := range sorted {
		forestHashes[i] = forest.Hash(h)
	}
	f := forest.Build(map[string][]forest.Hash{"_": forestHashes}, forest.WithPrefix(forestPrefix))

	return &MerkleTree{Root: types.Hash(f.Root), Leaves: sorted}
}

// combineHashes produces a parent node hash from two child hashes using
// types.ComputeMerkleNodeHash, which prefixes the concatenation with "merkle\0"
// to distinguish interior tree hashes from leaf hashes and snapshot roots.
// The combination is order-dependent: swapping left and right produces a
// different parent hash, preserving tree structure.
func combineHashes(left, right types.Hash) types.Hash {
	return types.ComputeMerkleNodeHash(left, right)
}

// DiffMerkle returns the leaf hashes that differ between two Merkle trees.
// Added are hashes present in newTree but not oldTree.
// Removed are hashes present in oldTree but not newTree.
func DiffMerkle(oldTree, newTree *MerkleTree) (added, removed []types.Hash) {
	oldSet := make(map[types.Hash]struct{}, len(oldTree.Leaves))
	for _, h := range oldTree.Leaves {
		oldSet[h] = struct{}{}
	}

	newSet := make(map[types.Hash]struct{}, len(newTree.Leaves))
	for _, h := range newTree.Leaves {
		newSet[h] = struct{}{}
	}

	for _, h := range newTree.Leaves {
		if _, ok := oldSet[h]; !ok {
			added = append(added, h)
		}
	}

	for _, h := range oldTree.Leaves {
		if _, ok := newSet[h]; !ok {
			removed = append(removed, h)
		}
	}

	return added, removed
}

// SortHashes sorts a slice of hashes lexicographically using bytes.Compare.
func SortHashes(hashes []types.Hash) {
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})
}
