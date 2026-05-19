package snapshot

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeEdge(pkg, edgeType, content string) EdgeInput {
	return EdgeInput{
		EdgeHash:    types.NewHash([]byte(content)),
		PackagePath: pkg,
		EdgeType:    edgeType,
	}
}

func TestBuildHierarchicalTree_Empty(t *testing.T) {
	tree := BuildHierarchicalTree(nil)
	if tree.Root != types.EmptyHash {
		t.Errorf("expected empty root, got %s", tree.Root)
	}
	if tree.TotalEdges != 0 {
		t.Errorf("expected 0 edges, got %d", tree.TotalEdges)
	}
}

func TestBuildHierarchicalTree_Deterministic(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("pkg/store", "calls", "edge1"),
		makeEdge("pkg/store", "calls", "edge2"),
		makeEdge("pkg/store", "references", "edge3"),
		makeEdge("pkg/mcp", "calls", "edge4"),
		makeEdge("pkg/mcp", "implements", "edge5"),
	}

	tree1 := BuildHierarchicalTree(edges)
	tree2 := BuildHierarchicalTree(edges)

	if tree1.Root != tree2.Root {
		t.Errorf("not deterministic: %s != %s", tree1.Root, tree2.Root)
	}
	if len(tree1.PackageRoots) != 2 {
		t.Errorf("expected 2 package roots, got %d", len(tree1.PackageRoots))
	}
	if tree1.TotalEdges != 5 {
		t.Errorf("expected 5 edges, got %d", tree1.TotalEdges)
	}
}

func TestBuildHierarchicalTree_OrderIndependent(t *testing.T) {
	edges1 := []EdgeInput{
		makeEdge("a", "calls", "e1"),
		makeEdge("b", "calls", "e2"),
		makeEdge("a", "imports", "e3"),
	}
	edges2 := []EdgeInput{
		makeEdge("b", "calls", "e2"),
		makeEdge("a", "imports", "e3"),
		makeEdge("a", "calls", "e1"),
	}

	tree1 := BuildHierarchicalTree(edges1)
	tree2 := BuildHierarchicalTree(edges2)

	if tree1.Root != tree2.Root {
		t.Errorf("order-dependent: %s != %s", tree1.Root, tree2.Root)
	}
}

func TestBuildHierarchicalTree_PackageRoots(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("store", "calls", "e2"),
		makeEdge("mcp", "calls", "e3"),
	}

	tree := BuildHierarchicalTree(edges)

	if _, ok := tree.PackageRoots["store"]; !ok {
		t.Error("missing package root for 'store'")
	}
	if _, ok := tree.PackageRoots["mcp"]; !ok {
		t.Error("missing package root for 'mcp'")
	}
	if tree.PackageEdgeCounts["store"] != 2 {
		t.Errorf("expected 2 store edges, got %d", tree.PackageEdgeCounts["store"])
	}
}

func TestBuildHierarchicalTree_EdgeTypeRoots(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("pkg", "calls", "e1"),
		makeEdge("pkg", "imports", "e2"),
		makeEdge("pkg", "calls", "e3"),
	}

	tree := BuildHierarchicalTree(edges)

	callsRoot, ok := tree.EdgeTypeRoots["pkg:calls"]
	if !ok {
		t.Fatal("missing edge-type root for 'pkg:calls'")
	}
	importsRoot, ok := tree.EdgeTypeRoots["pkg:imports"]
	if !ok {
		t.Fatal("missing edge-type root for 'pkg:imports'")
	}
	if callsRoot == importsRoot {
		t.Error("calls and imports roots should differ")
	}
}

func TestDiffHierarchicalTrees_Identical(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("a", "calls", "e1"),
		makeEdge("b", "calls", "e2"),
	}

	tree1 := BuildHierarchicalTree(edges)
	tree2 := BuildHierarchicalTree(edges)

	diff := DiffHierarchicalTrees(tree1, tree2)
	if diff.RootChanged {
		t.Error("identical trees should not show root changed")
	}
	if len(diff.ChangedPackages) != 0 {
		t.Error("identical trees should have no changed packages")
	}
}

func TestDiffHierarchicalTrees_PackageChanged(t *testing.T) {
	old := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("mcp", "calls", "e2"),
	}
	new := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("mcp", "calls", "e3"), // changed
	}

	oldTree := BuildHierarchicalTree(old)
	newTree := BuildHierarchicalTree(new)

	diff := DiffHierarchicalTrees(oldTree, newTree)
	if !diff.RootChanged {
		t.Error("roots should differ")
	}
	if len(diff.ChangedPackages) != 1 || diff.ChangedPackages[0] != "mcp" {
		t.Errorf("expected changed=[mcp], got %v", diff.ChangedPackages)
	}
	if len(diff.AddedPackages) != 0 {
		t.Errorf("expected no added packages, got %v", diff.AddedPackages)
	}
}

func TestDiffHierarchicalTrees_PackageAdded(t *testing.T) {
	old := []EdgeInput{
		makeEdge("store", "calls", "e1"),
	}
	new := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("newpkg", "calls", "e2"),
	}

	diff := DiffHierarchicalTrees(
		BuildHierarchicalTree(old),
		BuildHierarchicalTree(new),
	)
	if len(diff.AddedPackages) != 1 || diff.AddedPackages[0] != "newpkg" {
		t.Errorf("expected added=[newpkg], got %v", diff.AddedPackages)
	}
}

func TestDiffHierarchicalTrees_PackageRemoved(t *testing.T) {
	old := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("oldpkg", "calls", "e2"),
	}
	new := []EdgeInput{
		makeEdge("store", "calls", "e1"),
	}

	diff := DiffHierarchicalTrees(
		BuildHierarchicalTree(old),
		BuildHierarchicalTree(new),
	)
	if len(diff.RemovedPackages) != 1 || diff.RemovedPackages[0] != "oldpkg" {
		t.Errorf("expected removed=[oldpkg], got %v", diff.RemovedPackages)
	}
}

func TestSubgraphRoot(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("a", "calls", "e1"),
		makeEdge("b", "calls", "e2"),
		makeEdge("c", "calls", "e3"),
	}

	tree := BuildHierarchicalTree(edges)

	// Same packages, same root.
	r1 := tree.SubgraphRoot([]string{"a", "b"})
	r2 := tree.SubgraphRoot([]string{"b", "a"}) // order independent
	if r1 != r2 {
		t.Error("subgraph root should be order-independent")
	}

	// Different packages, different root.
	r3 := tree.SubgraphRoot([]string{"a", "c"})
	if r1 == r3 {
		t.Error("different package sets should have different roots")
	}

	// Empty packages.
	r4 := tree.SubgraphRoot(nil)
	if r4 != types.EmptyHash {
		t.Error("empty package set should return empty hash")
	}
}

func TestEdgeTypeRoot(t *testing.T) {
	edges := []EdgeInput{
		makeEdge("a", "calls", "e1"),
		makeEdge("b", "calls", "e2"),
		makeEdge("a", "imports", "e3"),
	}

	tree := BuildHierarchicalTree(edges)

	callsRoot := tree.EdgeTypeRoot("calls")
	importsRoot := tree.EdgeTypeRoot("imports")
	unknownRoot := tree.EdgeTypeRoot("nonexistent")

	if callsRoot == types.EmptyHash {
		t.Error("calls root should not be empty")
	}
	if importsRoot == types.EmptyHash {
		t.Error("imports root should not be empty")
	}
	if callsRoot == importsRoot {
		t.Error("calls and imports roots should differ")
	}
	if unknownRoot != types.EmptyHash {
		t.Error("nonexistent edge type should return empty hash")
	}
}

func TestContextPackRoot_Deterministic(t *testing.T) {
	snapshot := types.NewHash([]byte("snapshot1"))
	nodes := []types.Hash{
		types.NewHash([]byte("node1")),
		types.NewHash([]byte("node2")),
	}

	r1 := ContextPackRoot("find auth handlers", snapshot, nodes)
	r2 := ContextPackRoot("find auth handlers", snapshot, nodes)

	if r1 != r2 {
		t.Error("context pack root should be deterministic")
	}

	// Different task, different root.
	r3 := ContextPackRoot("find logging handlers", snapshot, nodes)
	if r1 == r3 {
		t.Error("different tasks should produce different roots")
	}
}

func TestDiffHierarchicalTreesWithOptions_PackageFilter(t *testing.T) {
	old := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("mcp", "calls", "e2"),
		makeEdge("indexer", "calls", "e3"),
	}
	// Change mcp and indexer packages, leave store unchanged.
	new := []EdgeInput{
		makeEdge("store", "calls", "e1"),
		makeEdge("mcp", "calls", "e2-changed"),
		makeEdge("indexer", "calls", "e3-changed"),
	}

	oldTree := BuildHierarchicalTree(old)
	newTree := BuildHierarchicalTree(new)

	// Filter to only "store" and "mcp"; indexer changes should be invisible.
	opts := &DiffOptions{PackageFilter: []string{"store", "mcp"}}
	diff := DiffHierarchicalTreesWithOptions(oldTree, newTree, opts)

	if !diff.RootChanged {
		t.Error("roots should differ")
	}
	if len(diff.ChangedPackages) != 1 || diff.ChangedPackages[0] != "mcp" {
		t.Errorf("expected changed=[mcp], got %v", diff.ChangedPackages)
	}
	for _, pkg := range diff.ChangedPackages {
		if pkg == "indexer" {
			t.Error("indexer should be excluded by package filter")
		}
	}
	if diff.Truncated {
		t.Error("should not be truncated when cap is not set")
	}
}

func TestDiffHierarchicalTreesWithOptions_MaxChanges(t *testing.T) {
	old := []EdgeInput{
		makeEdge("a", "calls", "e1"),
		makeEdge("b", "calls", "e2"),
		makeEdge("c", "calls", "e3"),
		makeEdge("d", "calls", "e4"),
	}
	// All four packages change.
	new := []EdgeInput{
		makeEdge("a", "calls", "e1-x"),
		makeEdge("b", "calls", "e2-x"),
		makeEdge("c", "calls", "e3-x"),
		makeEdge("d", "calls", "e4-x"),
	}

	oldTree := BuildHierarchicalTree(old)
	newTree := BuildHierarchicalTree(new)

	opts := &DiffOptions{MaxChanges: 2}
	diff := DiffHierarchicalTreesWithOptions(oldTree, newTree, opts)

	totalReported := len(diff.ChangedPackages) + len(diff.AddedPackages) + len(diff.RemovedPackages)
	if totalReported > 2 {
		t.Errorf("expected at most 2 changes reported, got %d", totalReported)
	}
	if !diff.Truncated {
		t.Error("diff should be marked Truncated when MaxChanges is hit")
	}
}

func TestDiffHierarchicalTreesWithOptions_NilOpts(t *testing.T) {
	// nil opts should behave identically to DiffHierarchicalTrees.
	old := []EdgeInput{makeEdge("a", "calls", "e1")}
	new := []EdgeInput{makeEdge("a", "calls", "e2")}

	oldTree := BuildHierarchicalTree(old)
	newTree := BuildHierarchicalTree(new)

	d1 := DiffHierarchicalTrees(oldTree, newTree)
	d2 := DiffHierarchicalTreesWithOptions(oldTree, newTree, nil)

	if d1.RootChanged != d2.RootChanged {
		t.Error("RootChanged should match between wrapper and direct call")
	}
	if len(d1.ChangedPackages) != len(d2.ChangedPackages) {
		t.Errorf("ChangedPackages length mismatch: %d vs %d", len(d1.ChangedPackages), len(d2.ChangedPackages))
	}
	if d2.Truncated {
		t.Error("should not be truncated with nil opts")
	}
}
