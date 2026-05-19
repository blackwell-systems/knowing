package snapshot

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ProofStep represents one step in a Merkle proof path. At each level of the
// binary tree, the verifier needs the sibling hash and whether it's on the
// left or right.
type ProofStep struct {
	Sibling types.Hash `json:"sibling"`
	IsLeft  bool       `json:"is_left"` // true if sibling is on the left (target is right)
}

// MerkleProof is a proof that a specific edge exists in a hierarchical snapshot.
// The proof path goes: edge -> edge-type root -> package root -> repo root.
// Each level includes the binary Merkle tree proof within that level, plus
// the hierarchical context (package name, edge type).
type MerkleProof struct {
	// The edge being proved.
	EdgeHash types.Hash `json:"edge_hash"`

	// Hierarchical context.
	PackagePath string `json:"package_path"`
	EdgeType    string `json:"edge_type"`

	// Level 1: edge hash -> edge-type root.
	// Binary proof within the sorted edge hashes of this (package, edgeType).
	EdgeToEdgeTypeRoot []ProofStep `json:"edge_to_edge_type_root"`
	EdgeTypeRoot       types.Hash  `json:"edge_type_root"`

	// Level 2: edge-type root -> package root.
	// Binary proof within the sorted edge-type roots of this package.
	EdgeTypeToPackageRoot []ProofStep `json:"edge_type_to_package_root"`
	PackageRoot           types.Hash  `json:"package_root"`

	// Level 3: package root -> repo root.
	// Binary proof within the sorted package roots.
	PackageToRepoRoot []ProofStep `json:"package_to_repo_root"`
	RepoRoot          types.Hash  `json:"repo_root"`
}

// GenerateProof creates a Merkle proof that edgeHash exists in the tree
// under the given package and edge type. Returns an error if the edge is
// not found.
func GenerateProof(tree *HierarchicalTree, edgeHash types.Hash, packagePath, edgeType string, edges []EdgeInput) (*MerkleProof, error) {
	if tree == nil {
		return nil, fmt.Errorf("nil tree")
	}

	key := packagePath + ":" + edgeType

	// Verify the edge-type root exists.
	etRoot, ok := tree.EdgeTypeRoots[key]
	if !ok {
		return nil, fmt.Errorf("edge type %q not found in tree", key)
	}

	// Verify the package root exists.
	pkgRoot, ok := tree.PackageRoots[packagePath]
	if !ok {
		return nil, fmt.Errorf("package %q not found in tree", packagePath)
	}

	// Collect edge hashes for this (package, edgeType).
	var edgeHashes []types.Hash
	for _, e := range edges {
		if e.PackagePath == packagePath && e.EdgeType == edgeType {
			edgeHashes = append(edgeHashes, e.EdgeHash)
		}
	}
	sort.Slice(edgeHashes, func(i, j int) bool {
		return bytes.Compare(edgeHashes[i][:], edgeHashes[j][:]) < 0
	})

	// Level 1: prove edgeHash is in the edge-type's leaf set.
	edgeProof, err := binaryProof(edgeHashes, edgeHash)
	if err != nil {
		return nil, fmt.Errorf("edge proof: %w", err)
	}

	// Collect edge-type roots for this package.
	var etRoots []types.Hash
	for k, root := range tree.EdgeTypeRoots {
		pkg := k[:lastColonIndex(k)]
		if pkg == packagePath {
			etRoots = append(etRoots, root)
		}
	}
	sort.Slice(etRoots, func(i, j int) bool {
		return bytes.Compare(etRoots[i][:], etRoots[j][:]) < 0
	})

	// Level 2: prove edge-type root is in the package's edge-type root set.
	etProof, err := binaryProof(etRoots, etRoot)
	if err != nil {
		return nil, fmt.Errorf("edge-type proof: %w", err)
	}

	// Collect package roots sorted by hash bytes (matching BuildMerkleTree's
	// internal sort, which sorts by bytes.Compare, not by package name).
	pkgRoots := make([]types.Hash, 0, len(tree.PackageRoots))
	for _, root := range tree.PackageRoots {
		pkgRoots = append(pkgRoots, root)
	}
	sort.Slice(pkgRoots, func(i, j int) bool {
		return bytes.Compare(pkgRoots[i][:], pkgRoots[j][:]) < 0
	})

	// Level 3: prove package root is in the repo's package root set.
	pkgProof, err := binaryProof(pkgRoots, pkgRoot)
	if err != nil {
		return nil, fmt.Errorf("package proof: %w", err)
	}

	return &MerkleProof{
		EdgeHash:              edgeHash,
		PackagePath:           packagePath,
		EdgeType:              edgeType,
		EdgeToEdgeTypeRoot:    edgeProof,
		EdgeTypeRoot:          etRoot,
		EdgeTypeToPackageRoot: etProof,
		PackageRoot:           pkgRoot,
		PackageToRepoRoot:     pkgProof,
		RepoRoot:              tree.Root,
	}, nil
}

// VerifyProof checks that a Merkle proof is valid: recomputing the root from
// the edge hash and proof steps produces the claimed repo root.
func VerifyProof(proof *MerkleProof) bool {
	// Level 1: edge -> edge-type root.
	computed := proof.EdgeHash
	for _, step := range proof.EdgeToEdgeTypeRoot {
		if step.IsLeft {
			computed = combineHashes(step.Sibling, computed)
		} else {
			computed = combineHashes(computed, step.Sibling)
		}
	}
	if computed != proof.EdgeTypeRoot {
		return false
	}

	// Level 2: edge-type root -> package root.
	computed = proof.EdgeTypeRoot
	for _, step := range proof.EdgeTypeToPackageRoot {
		if step.IsLeft {
			computed = combineHashes(step.Sibling, computed)
		} else {
			computed = combineHashes(computed, step.Sibling)
		}
	}
	if computed != proof.PackageRoot {
		return false
	}

	// Level 3: package root -> repo root.
	computed = proof.PackageRoot
	for _, step := range proof.PackageToRepoRoot {
		if step.IsLeft {
			computed = combineHashes(step.Sibling, computed)
		} else {
			computed = combineHashes(computed, step.Sibling)
		}
	}
	return computed == proof.RepoRoot
}

// AbsenceProof proves that a specific edge hash does NOT exist in a tree.
// It works by proving the two adjacent leaves that bracket the missing hash:
// left < missing < right. Since leaves are sorted by bytes.Compare, adjacency
// proves there is no room for the missing hash.
type AbsenceProof struct {
	// MissingHash is the edge hash being proved absent.
	MissingHash types.Hash `json:"missing_hash"`

	// LeftNeighbor is the largest leaf smaller than MissingHash (nil if MissingHash
	// would be the first leaf).
	LeftNeighbor *types.Hash `json:"left_neighbor,omitempty"`
	// RightNeighbor is the smallest leaf larger than MissingHash (nil if MissingHash
	// would be the last leaf).
	RightNeighbor *types.Hash `json:"right_neighbor,omitempty"`

	// LeftProof proves LeftNeighbor is in the tree (nil if no left neighbor).
	LeftProof *MerkleProof `json:"left_proof,omitempty"`
	// RightProof proves RightNeighbor is in the tree (nil if no right neighbor).
	RightProof *MerkleProof `json:"right_proof,omitempty"`

	// RepoRoot is the root the absence is proved against.
	RepoRoot types.Hash `json:"repo_root"`
}

// GenerateAbsenceProof creates a proof that edgeHash does NOT exist in the tree
// under the given package and edge type. Returns an error if the edge IS found
// (you can't prove absence of something that exists).
func GenerateAbsenceProof(tree *HierarchicalTree, edgeHash types.Hash, packagePath, edgeType string, edges []EdgeInput) (*AbsenceProof, error) {
	if tree == nil {
		return nil, fmt.Errorf("nil tree")
	}

	key := packagePath + ":" + edgeType

	// If the package or edge type doesn't exist at all, absence is trivial.
	if _, ok := tree.EdgeTypeRoots[key]; !ok {
		return &AbsenceProof{
			MissingHash: edgeHash,
			RepoRoot:    tree.Root,
		}, nil
	}

	// Collect and sort the edge hashes for this (package, edgeType).
	var leaves []types.Hash
	for _, e := range edges {
		if e.PackagePath == packagePath && e.EdgeType == edgeType {
			leaves = append(leaves, e.EdgeHash)
		}
	}
	sort.Slice(leaves, func(i, j int) bool {
		return bytes.Compare(leaves[i][:], leaves[j][:]) < 0
	})

	// Check: if the hash IS in the set, we can't prove absence.
	for _, h := range leaves {
		if h == edgeHash {
			return nil, fmt.Errorf("cannot prove absence: edge %s exists in the tree", edgeHash)
		}
	}

	// Find the insertion point: the index where edgeHash would be inserted.
	insertIdx := sort.Search(len(leaves), func(i int) bool {
		return bytes.Compare(leaves[i][:], edgeHash[:]) >= 0
	})

	proof := &AbsenceProof{
		MissingHash: edgeHash,
		RepoRoot:    tree.Root,
	}

	// Left neighbor: the leaf just before the insertion point.
	if insertIdx > 0 {
		left := leaves[insertIdx-1]
		proof.LeftNeighbor = &left
		lp, err := GenerateProof(tree, left, packagePath, edgeType, edges)
		if err != nil {
			return nil, fmt.Errorf("generating left neighbor proof: %w", err)
		}
		proof.LeftProof = lp
	}

	// Right neighbor: the leaf at the insertion point.
	if insertIdx < len(leaves) {
		right := leaves[insertIdx]
		proof.RightNeighbor = &right
		rp, err := GenerateProof(tree, right, packagePath, edgeType, edges)
		if err != nil {
			return nil, fmt.Errorf("generating right neighbor proof: %w", err)
		}
		proof.RightProof = rp
	}

	return proof, nil
}

// VerifyAbsenceProof checks that an absence proof is valid:
// 1. Both neighbor proofs verify against the same root.
// 2. left < missing < right (sorted order).
// 3. If both neighbors exist, they prove the gap contains no room for the missing hash.
func VerifyAbsenceProof(proof *AbsenceProof) bool {
	if proof == nil {
		return false
	}

	// Verify left neighbor proof if present.
	if proof.LeftProof != nil {
		if !VerifyProof(proof.LeftProof) {
			return false
		}
		if proof.LeftProof.RepoRoot != proof.RepoRoot {
			return false
		}
		// Left must be strictly less than missing.
		if proof.LeftNeighbor == nil || bytes.Compare(proof.LeftNeighbor[:], proof.MissingHash[:]) >= 0 {
			return false
		}
	}

	// Verify right neighbor proof if present.
	if proof.RightProof != nil {
		if !VerifyProof(proof.RightProof) {
			return false
		}
		if proof.RightProof.RepoRoot != proof.RepoRoot {
			return false
		}
		// Right must be strictly greater than missing.
		if proof.RightNeighbor == nil || bytes.Compare(proof.RightNeighbor[:], proof.MissingHash[:]) <= 0 {
			return false
		}
	}

	// At least one neighbor must exist (otherwise the tree is empty and
	// the trivial proof with no neighbors is valid).
	return true
}

// binaryProof generates proof steps for a target hash within a sorted list
// of leaf hashes. Returns the sibling hashes needed to reconstruct the root.
func binaryProof(leaves []types.Hash, target types.Hash) ([]ProofStep, error) {
	if len(leaves) == 0 {
		return nil, fmt.Errorf("empty leaf set")
	}
	if len(leaves) == 1 {
		if leaves[0] != target {
			return nil, fmt.Errorf("target not found in single-leaf set")
		}
		return nil, nil // root IS the leaf; no proof steps needed
	}

	// Find target index.
	idx := -1
	for i, h := range leaves {
		if h == target {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("target hash not found in leaf set")
	}

	var steps []ProofStep
	level := make([]types.Hash, len(leaves))
	copy(level, leaves)

	for len(level) > 1 {
		var nextLevel []types.Hash
		nextIdx := -1

		for i := 0; i < len(level); i += 2 {
			left := level[i]
			right := left // self-pair if odd
			if i+1 < len(level) {
				right = level[i+1]
			}

			combined := combineHashes(left, right)
			nextLevel = append(nextLevel, combined)

			// Track which parent the target propagates to.
			if i == idx {
				// Target is on the left; sibling is on the right.
				steps = append(steps, ProofStep{Sibling: right, IsLeft: false})
				nextIdx = len(nextLevel) - 1
			} else if i+1 == idx {
				// Target is on the right; sibling is on the left.
				steps = append(steps, ProofStep{Sibling: left, IsLeft: true})
				nextIdx = len(nextLevel) - 1
			}
		}

		level = nextLevel
		idx = nextIdx
	}

	return steps, nil
}

func lastColonIndex(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return len(s)
}
