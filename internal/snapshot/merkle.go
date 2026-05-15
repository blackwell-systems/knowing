package snapshot

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/blackwell-systems/knowing/internal/types"
)

// MerkleTree represents a binary Merkle tree built from sorted hashes.
// It supports efficient diff detection between two trees.
type MerkleTree struct {
	Root   types.Hash
	Leaves []types.Hash
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

	root := computeMerkleRoot(sorted)
	return &MerkleTree{Root: root, Leaves: sorted}
}

// computeMerkleRoot recursively computes the Merkle root from sorted hashes.
func computeMerkleRoot(hashes []types.Hash) types.Hash {
	if len(hashes) == 1 {
		return hashes[0]
	}

	var nextLevel []types.Hash
	for i := 0; i < len(hashes); i += 2 {
		if i+1 < len(hashes) {
			combined := combineHashes(hashes[i], hashes[i+1])
			nextLevel = append(nextLevel, combined)
		} else {
			// Odd leaf: promote to next level (pair with itself)
			combined := combineHashes(hashes[i], hashes[i])
			nextLevel = append(nextLevel, combined)
		}
	}

	return computeMerkleRoot(nextLevel)
}

// combineHashes produces a parent hash from two child hashes.
func combineHashes(left, right types.Hash) types.Hash {
	var data [64]byte
	copy(data[:32], left[:])
	copy(data[32:], right[:])
	return sha256.Sum256(data[:])
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
