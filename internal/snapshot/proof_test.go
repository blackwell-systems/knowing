package snapshot

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func h(s string) types.Hash {
	return types.NewHash([]byte(s))
}

func TestGenerateAndVerifyProof_SingleEdge(t *testing.T) {
	edge := EdgeInput{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"}
	edges := []EdgeInput{edge}
	tree := BuildHierarchicalTree(edges)

	proof, err := GenerateProof(tree, edge.EdgeHash, "pkg/a", "calls", edges)
	if err != nil {
		t.Fatalf("GenerateProof: %v", err)
	}
	if proof.RepoRoot != tree.Root {
		t.Errorf("proof root %s != tree root %s", proof.RepoRoot, tree.Root)
	}
	if !VerifyProof(proof) {
		t.Error("proof failed verification")
	}
}

func TestGenerateAndVerifyProof_MultipleEdges(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e2"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e3"), PackagePath: "pkg/a", EdgeType: "imports"},
		{EdgeHash: h("e4"), PackagePath: "pkg/b", EdgeType: "calls"},
		{EdgeHash: h("e5"), PackagePath: "pkg/b", EdgeType: "implements"},
	}
	tree := BuildHierarchicalTree(edges)

	// Prove each edge.
	for _, e := range edges {
		proof, err := GenerateProof(tree, e.EdgeHash, e.PackagePath, e.EdgeType, edges)
		if err != nil {
			t.Fatalf("GenerateProof(%s): %v", e.EdgeHash, err)
		}
		if proof.RepoRoot != tree.Root {
			t.Errorf("proof root mismatch for %s", e.EdgeHash)
		}
		if !VerifyProof(proof) {
			t.Errorf("proof failed verification for edge %s (pkg=%s, type=%s)",
				e.EdgeHash, e.PackagePath, e.EdgeType)
		}
	}
}

func TestVerifyProof_TamperedEdge(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e2"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	proof, err := GenerateProof(tree, h("e1"), "pkg/a", "calls", edges)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with the edge hash.
	proof.EdgeHash = h("tampered")
	if VerifyProof(proof) {
		t.Error("tampered proof should fail verification")
	}
}

func TestVerifyProof_TamperedSibling(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e2"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e3"), PackagePath: "pkg/b", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	proof, err := GenerateProof(tree, h("e1"), "pkg/a", "calls", edges)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper with a sibling at the package level.
	if len(proof.PackageToRepoRoot) > 0 {
		proof.PackageToRepoRoot[0].Sibling = h("tampered-sibling")
	}
	if VerifyProof(proof) {
		t.Error("tampered sibling should fail verification")
	}
}

func TestGenerateProof_EdgeNotFound(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	_, err := GenerateProof(tree, h("nonexistent"), "pkg/a", "calls", edges)
	if err == nil {
		t.Error("expected error for nonexistent edge")
	}
}

func TestGenerateProof_PackageNotFound(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	_, err := GenerateProof(tree, h("e1"), "pkg/nonexistent", "calls", edges)
	if err == nil {
		t.Error("expected error for nonexistent package")
	}
}

func TestGenerateProof_NilTree(t *testing.T) {
	_, err := GenerateProof(nil, h("e1"), "pkg/a", "calls", nil)
	if err == nil {
		t.Error("expected error for nil tree")
	}
}

func TestGenerateAndVerifyProof_ManyEdges(t *testing.T) {
	// Build a larger tree with 50 edges across 5 packages and 3 edge types.
	var edges []EdgeInput
	pkgs := []string{"pkg/a", "pkg/b", "pkg/c", "pkg/d", "pkg/e"}
	etypes := []string{"calls", "imports", "implements"}
	for i := 0; i < 50; i++ {
		edges = append(edges, EdgeInput{
			EdgeHash:    types.NewHash([]byte{byte(i), byte(i >> 8)}),
			PackagePath: pkgs[i%len(pkgs)],
			EdgeType:    etypes[i%len(etypes)],
		})
	}
	tree := BuildHierarchicalTree(edges)

	// Prove every 5th edge.
	for i := 0; i < len(edges); i += 5 {
		e := edges[i]
		proof, err := GenerateProof(tree, e.EdgeHash, e.PackagePath, e.EdgeType, edges)
		if err != nil {
			t.Fatalf("GenerateProof(edge %d): %v", i, err)
		}
		if !VerifyProof(proof) {
			t.Errorf("proof failed for edge %d (pkg=%s, type=%s)", i, e.PackagePath, e.EdgeType)
		}
	}
}

func TestGenerateAbsenceProof_EdgeNotInTree(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e2"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e3"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	// Prove that a non-existent edge is absent.
	missing := h("nonexistent")
	proof, err := GenerateAbsenceProof(tree, missing, "pkg/a", "calls", edges)
	if err != nil {
		t.Fatalf("GenerateAbsenceProof: %v", err)
	}
	if !VerifyAbsenceProof(proof) {
		t.Error("absence proof failed verification")
	}
	if proof.LeftNeighbor == nil && proof.RightNeighbor == nil {
		t.Error("expected at least one neighbor")
	}
}

func TestGenerateAbsenceProof_EdgeExists(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	_, err := GenerateAbsenceProof(tree, h("e1"), "pkg/a", "calls", edges)
	if err == nil {
		t.Error("expected error when proving absence of existing edge")
	}
}

func TestGenerateAbsenceProof_PackageNotInTree(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	// Package doesn't exist: trivial absence.
	proof, err := GenerateAbsenceProof(tree, h("missing"), "pkg/nonexistent", "calls", edges)
	if err != nil {
		t.Fatalf("GenerateAbsenceProof: %v", err)
	}
	if !VerifyAbsenceProof(proof) {
		t.Error("trivial absence proof failed verification")
	}
	// No neighbors needed for trivial proof.
	if proof.LeftNeighbor != nil || proof.RightNeighbor != nil {
		t.Error("trivial proof should have no neighbors")
	}
}

func TestGenerateAbsenceProof_ManyEdges(t *testing.T) {
	var edges []EdgeInput
	for i := 0; i < 50; i++ {
		edges = append(edges, EdgeInput{
			EdgeHash:    types.NewHash([]byte{byte(i)}),
			PackagePath: "pkg/a",
			EdgeType:    "calls",
		})
	}
	tree := BuildHierarchicalTree(edges)

	// Pick a hash that's NOT in the set.
	missing := types.NewHash([]byte("definitely-not-in-set"))
	proof, err := GenerateAbsenceProof(tree, missing, "pkg/a", "calls", edges)
	if err != nil {
		t.Fatalf("GenerateAbsenceProof: %v", err)
	}
	if !VerifyAbsenceProof(proof) {
		t.Error("absence proof for 50-edge tree failed verification")
	}
}

func TestVerifyAbsenceProof_TamperedNeighbor(t *testing.T) {
	edges := []EdgeInput{
		{EdgeHash: h("e1"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e2"), PackagePath: "pkg/a", EdgeType: "calls"},
		{EdgeHash: h("e3"), PackagePath: "pkg/a", EdgeType: "calls"},
	}
	tree := BuildHierarchicalTree(edges)

	missing := h("nonexistent")
	proof, err := GenerateAbsenceProof(tree, missing, "pkg/a", "calls", edges)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper: swap the missing hash to equal a neighbor, breaking the ordering invariant.
	if proof.LeftNeighbor != nil {
		proof.MissingHash = *proof.LeftNeighbor // missing == left, violates left < missing
	} else if proof.RightNeighbor != nil {
		proof.MissingHash = *proof.RightNeighbor // missing == right, violates missing < right
	}
	if VerifyAbsenceProof(proof) {
		t.Error("tampered absence proof (ordering violated) should fail verification")
	}
}

func TestProofSize(t *testing.T) {
	// Verify proof is logarithmic: 50 edges should need O(log N) steps per level.
	var edges []EdgeInput
	for i := 0; i < 50; i++ {
		edges = append(edges, EdgeInput{
			EdgeHash:    types.NewHash([]byte{byte(i)}),
			PackagePath: "pkg/a",
			EdgeType:    "calls",
		})
	}
	tree := BuildHierarchicalTree(edges)

	proof, err := GenerateProof(tree, edges[25].EdgeHash, "pkg/a", "calls", edges)
	if err != nil {
		t.Fatal(err)
	}

	// 50 leaves -> binary tree depth ~6. Proof should have ~6 steps at level 1.
	t.Logf("Proof steps: level1=%d, level2=%d, level3=%d",
		len(proof.EdgeToEdgeTypeRoot),
		len(proof.EdgeTypeToPackageRoot),
		len(proof.PackageToRepoRoot))

	if len(proof.EdgeToEdgeTypeRoot) > 10 {
		t.Errorf("proof level 1 has %d steps (expected <=10 for 50 leaves)",
			len(proof.EdgeToEdgeTypeRoot))
	}
	if !VerifyProof(proof) {
		t.Error("proof failed verification")
	}
}
