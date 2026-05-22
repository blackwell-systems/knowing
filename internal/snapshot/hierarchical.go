package snapshot

import (
	"crypto/sha256"
	"sort"
	"strings"

	strata "github.com/blackwell-systems/merkle-strata"
	"github.com/blackwell-systems/knowing/internal/types"
)

// HierarchicalTree represents a Merkle tree structured by semantic boundaries:
// repo root -> package roots -> edge-type roots -> leaf edges.
//
// This structure enables:
//   - O(1) "did package X change?" checks via PackageRoots comparison
//   - O(1) "did call edges change?" checks via EdgeTypeRoots comparison
//   - Subgraph caching: cache results against package or edge-type roots
//   - Incremental recompute: only rebuild derived data for changed subtrees
//   - Lazy materialization: load only the subtrees a retrieval walk visits
//
// The Root field is backward-compatible with the flat MerkleTree: it's computed
// from sorted package roots, which are computed from sorted edge-type roots per
// package, which are computed from sorted edge hashes. The root is deterministic
// and will match the flat tree when the same edges are present.
type HierarchicalTree struct {
	Root types.Hash

	// PackageRoots maps package path to its Merkle root.
	// A package root is hash(sorted edge-type roots for that package).
	PackageRoots map[string]types.Hash

	// EdgeTypeRoots maps "package:edgeType" to its Merkle root.
	// An edge-type root is hash(sorted edge hashes of that type in that package).
	EdgeTypeRoots map[string]types.Hash

	// PackageEdgeCounts tracks edge count per package for quick stats.
	PackageEdgeCounts map[string]int

	// TotalEdges is the total number of edges across all packages.
	TotalEdges int

	// ml is the underlying strata.MultiLevel for SubgraphRoot delegation.
	ml *strata.MultiLevel
}

// EdgeInput is the input for building a hierarchical tree: an edge hash with
// its source package and edge type metadata.
type EdgeInput struct {
	EdgeHash    types.Hash
	PackagePath string // extracted from source node's qualified name
	EdgeType    string // calls, imports, implements, references, etc.
}

// BuildHierarchicalTree constructs a hierarchical Merkle tree from edge inputs.
//
// Structure:
//
//	repo root = merkle(sorted package roots)
//	  package root = merkle(sorted edge-type roots for this package)
//	    edge-type root = merkle(sorted edge hashes of this type in this package)
//	      leaf = edge hash
func BuildHierarchicalTree(edges []EdgeInput) *HierarchicalTree {
	if len(edges) == 0 {
		return &HierarchicalTree{
			Root:              types.EmptyHash,
			PackageRoots:      map[string]types.Hash{},
			EdgeTypeRoots:     map[string]types.Hash{},
			PackageEdgeCounts: map[string]int{},
		}
	}

	// Convert EdgeInputs to strata.MultiLevelInput.
	inputs := make([]strata.MultiLevelInput, len(edges))
	pkgEdgeCounts := make(map[string]int)
	for i, e := range edges {
		group := e.PackagePath
		if group == "" {
			group = "_root"
		}
		inputs[i] = strata.MultiLevelInput{
			Leaf:     strata.Hash(e.EdgeHash),
			Group:    group,
			Subgroup: e.EdgeType,
		}
		pkgEdgeCounts[group]++
	}

	// Build via merkle-strata with knowing's domain prefix.
	ml := strata.BuildMultiLevel(inputs, strata.WithPrefix(strataPrefix))

	// Convert results back to knowing types.
	packageRoots := make(map[string]types.Hash, len(ml.GroupRoots))
	for k, v := range ml.GroupRoots {
		packageRoots[k] = types.Hash(v)
	}
	edgeTypeRoots := make(map[string]types.Hash, len(ml.SubgroupRoots))
	for k, v := range ml.SubgroupRoots {
		edgeTypeRoots[k] = types.Hash(v)
	}

	return &HierarchicalTree{
		Root:              types.Hash(ml.Root),
		PackageRoots:      packageRoots,
		EdgeTypeRoots:     edgeTypeRoots,
		PackageEdgeCounts: pkgEdgeCounts,
		TotalEdges:        len(edges),
		ml:                ml,
	}
}

// DiffHierarchical compares two hierarchical trees and returns which packages
// and edge types changed. This is O(packages) instead of O(edges).
type HierarchicalDiff struct {
	// ChangedPackages lists packages whose root changed (content differs).
	ChangedPackages []string

	// AddedPackages lists packages present in new but not old.
	AddedPackages []string

	// RemovedPackages lists packages present in old but not new.
	RemovedPackages []string

	// ChangedEdgeTypes lists "package:edgeType" keys whose root changed.
	ChangedEdgeTypes []string

	// RootChanged is true if the overall repo root differs.
	RootChanged bool

	// Truncated is true when the diff was cut short by a MaxChanges cap.
	Truncated bool
}

// DiffOptions controls the behaviour of DiffHierarchicalTreesWithOptions.
type DiffOptions struct {
	// PackageFilter restricts the diff to the listed package paths. When
	// empty, all packages are compared (default behaviour).
	PackageFilter []string

	// MaxChanges caps the total number of changed/added/removed packages
	// reported. Once the cap is reached the diff is marked Truncated and no
	// further packages are added. 0 means no cap.
	MaxChanges int
}

// DiffHierarchicalTrees compares two hierarchical trees at each level.
// It is a convenience wrapper around DiffHierarchicalTreesWithOptions with
// nil options (no filter, no cap).
func DiffHierarchicalTrees(oldTree, newTree *HierarchicalTree) *HierarchicalDiff {
	return DiffHierarchicalTreesWithOptions(oldTree, newTree, nil)
}

// DiffHierarchicalTreesWithOptions compares two hierarchical trees with
// optional package filtering and a maximum-changes cap.
//
// When opts.PackageFilter is non-empty, only the listed packages are
// examined; the resulting diff reflects only those packages. When
// opts.MaxChanges is positive, the diff stops accumulating entries once
// that many total changed/added/removed packages have been recorded and
// sets HierarchicalDiff.Truncated = true.
func DiffHierarchicalTreesWithOptions(oldTree, newTree *HierarchicalTree, opts *DiffOptions) *HierarchicalDiff {
	diff := &HierarchicalDiff{
		RootChanged: oldTree.Root != newTree.Root,
	}

	if !diff.RootChanged {
		return diff
	}

	// Build package filter set for O(1) lookup.
	var filterSet map[string]bool
	if opts != nil && len(opts.PackageFilter) > 0 {
		filterSet = make(map[string]bool, len(opts.PackageFilter))
		for _, p := range opts.PackageFilter {
			filterSet[p] = true
		}
	}

	maxChanges := 0
	if opts != nil {
		maxChanges = opts.MaxChanges
	}

	// totalChanges tracks entries across all three change lists for cap enforcement.
	totalChanges := 0

	// reachedCap returns true and marks Truncated when the cap has been hit.
	reachedCap := func() bool {
		if maxChanges > 0 && totalChanges >= maxChanges {
			diff.Truncated = true
			return true
		}
		return false
	}

	// Compare package roots.
	for pkg, newRoot := range newTree.PackageRoots {
		if filterSet != nil && !filterSet[pkg] {
			continue
		}
		if reachedCap() {
			break
		}
		oldRoot, exists := oldTree.PackageRoots[pkg]
		if !exists {
			diff.AddedPackages = append(diff.AddedPackages, pkg)
			totalChanges++
		} else if oldRoot != newRoot {
			diff.ChangedPackages = append(diff.ChangedPackages, pkg)
			totalChanges++
		}
	}
	for pkg := range oldTree.PackageRoots {
		if filterSet != nil && !filterSet[pkg] {
			continue
		}
		if reachedCap() {
			break
		}
		if _, exists := newTree.PackageRoots[pkg]; !exists {
			diff.RemovedPackages = append(diff.RemovedPackages, pkg)
			totalChanges++
		}
	}

	// Compare edge-type roots (only for changed/added packages to avoid full scan).
	changedPkgSet := make(map[string]bool)
	for _, pkg := range diff.ChangedPackages {
		changedPkgSet[pkg] = true
	}
	for _, pkg := range diff.AddedPackages {
		changedPkgSet[pkg] = true
	}

	for key, newRoot := range newTree.EdgeTypeRoots {
		pkg := key[:strings.LastIndex(key, ":")]
		if !changedPkgSet[pkg] {
			continue
		}
		oldRoot, exists := oldTree.EdgeTypeRoots[key]
		if !exists || oldRoot != newRoot {
			diff.ChangedEdgeTypes = append(diff.ChangedEdgeTypes, key)
		}
	}

	sort.Strings(diff.ChangedPackages)
	sort.Strings(diff.AddedPackages)
	sort.Strings(diff.RemovedPackages)
	sort.Strings(diff.ChangedEdgeTypes)

	return diff
}

// SubgraphRoot computes a cache key for a subgraph defined by a set of packages.
// This is useful for caching results of operations that only depend on certain
// packages (e.g., blast_radius for a symbol in package X).
func (ht *HierarchicalTree) SubgraphRoot(packages []string) types.Hash {
	if ht.ml != nil {
		return types.Hash(ht.ml.SubgraphRoot(packages))
	}

	// Fallback for trees constructed without ml (e.g., zero-value).
	if len(packages) == 0 {
		return types.EmptyHash
	}

	sorted := make([]string, len(packages))
	copy(sorted, packages)
	sort.Strings(sorted)

	var roots []types.Hash
	for _, pkg := range sorted {
		if root, ok := ht.PackageRoots[pkg]; ok {
			roots = append(roots, root)
		}
	}

	if len(roots) == 0 {
		return types.EmptyHash
	}

	tree := BuildMerkleTree(roots)
	return tree.Root
}

// EdgeTypeRoot returns the Merkle root for a specific edge type across all
// packages. Useful for checking "did any call edges change?" without
// scanning the full tree.
func (ht *HierarchicalTree) EdgeTypeRoot(edgeType string) types.Hash {
	var roots []types.Hash
	for key, root := range ht.EdgeTypeRoots {
		if strings.HasSuffix(key, ":"+edgeType) {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return types.EmptyHash
	}
	tree := BuildMerkleTree(roots)
	return tree.Root
}

// ContextPackRoot computes a content-addressed key for a context pack:
// the combination of a task identifier, the snapshot state, and the selected
// symbols. Two identical queries against the same graph state produce the
// same root, enabling deduplication and caching.
func ContextPackRoot(taskNormalized string, snapshotRoot types.Hash, selectedNodes []types.Hash) types.Hash {
	h := sha256.New()
	h.Write([]byte(taskNormalized))
	h.Write(snapshotRoot[:])
	SortHashes(selectedNodes)
	for _, n := range selectedNodes {
		h.Write(n[:])
	}
	var result types.Hash
	copy(result[:], h.Sum(nil))
	return result
}
