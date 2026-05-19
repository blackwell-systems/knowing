package types

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// TestComputeNodeHash_DomainPrefix verifies that the "node\0" domain prefix
// is included and changes the hash compared to a raw field concatenation.
func TestComputeNodeHash_DomainPrefix(t *testing.T) {
	repoURL := "github.com/org/repo"
	packagePath := "pkg/sub"
	symbolName := "MyFunc"
	symbolKind := "function"

	got := ComputeNodeHash(repoURL, packagePath, EmptyHash, symbolName, symbolKind)

	// Compute raw hash without prefix to confirm they differ.
	rawData := fmt.Sprintf("%s\x00%s\x00%s\x00%s", repoURL, packagePath, symbolName, symbolKind)
	rawHash := sha256.Sum256([]byte(rawData))

	if got == rawHash {
		t.Errorf("ComputeNodeHash returned same hash as unprefixed computation; domain prefix not applied")
	}

	// Also verify that the same inputs always produce the same output (determinism).
	got2 := ComputeNodeHash(repoURL, packagePath, EmptyHash, symbolName, symbolKind)
	if got != got2 {
		t.Errorf("ComputeNodeHash is not deterministic")
	}
}

// TestComputeEdgeHash_DomainPrefix verifies that the "edge\0" domain prefix
// is included and changes the hash compared to a raw field concatenation.
func TestComputeEdgeHash_DomainPrefix(t *testing.T) {
	sourceHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Caller", "function")
	targetHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Callee", "function")
	edgeType := "calls"
	provenance := "ast_resolved"

	got := ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)

	// Compute raw hash without prefix.
	rawData := fmt.Sprintf("%s\x00%s\x00%s\x00%s", sourceHash, targetHash, edgeType, provenance)
	rawHash := sha256.Sum256([]byte(rawData))

	if got == rawHash {
		t.Errorf("ComputeEdgeHash returned same hash as unprefixed computation; domain prefix not applied")
	}

	// Determinism check.
	got2 := ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)
	if got != got2 {
		t.Errorf("ComputeEdgeHash is not deterministic")
	}
}

// TestComputeSnapshotHash_DomainPrefix verifies that ComputeSnapshotHash produces
// a different hash than the raw Merkle root it wraps.
func TestComputeSnapshotHash_DomainPrefix(t *testing.T) {
	merkleRoot := NewHash([]byte("some-merkle-root"))

	got := ComputeSnapshotHash(merkleRoot)

	if got == merkleRoot {
		t.Errorf("ComputeSnapshotHash returned the same hash as the raw Merkle root; domain prefix not applied")
	}

	// Determinism check.
	got2 := ComputeSnapshotHash(merkleRoot)
	if got != got2 {
		t.Errorf("ComputeSnapshotHash is not deterministic")
	}
}

// TestComputeMerkleNodeHash_DomainPrefix verifies that the Merkle interior hash
// differs from raw concatenation of the two children.
func TestComputeMerkleNodeHash_DomainPrefix(t *testing.T) {
	left := NewHash([]byte("left-leaf"))
	right := NewHash([]byte("right-leaf"))

	got := ComputeMerkleNodeHash(left, right)

	// Raw concatenation hash (64 bytes, no prefix).
	var rawData [64]byte
	copy(rawData[:32], left[:])
	copy(rawData[32:], right[:])
	rawHash := sha256.Sum256(rawData[:])

	if got == rawHash {
		t.Errorf("ComputeMerkleNodeHash returned same hash as raw concatenation; domain prefix not applied")
	}

	// Determinism check.
	got2 := ComputeMerkleNodeHash(left, right)
	if got != got2 {
		t.Errorf("ComputeMerkleNodeHash is not deterministic")
	}

	// Order-dependence check: swapping left/right must yield a different hash.
	swapped := ComputeMerkleNodeHash(right, left)
	if got == swapped {
		t.Errorf("ComputeMerkleNodeHash(left, right) == ComputeMerkleNodeHash(right, left); hash is order-independent, tree structure is not preserved")
	}
}

// TestVerifyNodeHash_Match creates a node with a correctly computed hash and
// verifies that VerifyNodeHash returns nil.
func TestVerifyNodeHash_Match(t *testing.T) {
	repoURL := "github.com/org/repo"
	packagePath := "pkg/sub"
	symbolName := "MyFunc"
	symbolKind := "function"

	nodeHash := ComputeNodeHash(repoURL, packagePath, EmptyHash, symbolName, symbolKind)
	n := Node{
		NodeHash:      nodeHash,
		QualifiedName: repoURL + "://" + packagePath + "." + symbolName,
		Kind:          symbolKind,
	}

	if err := VerifyNodeHash(n, repoURL, packagePath); err != nil {
		t.Errorf("VerifyNodeHash returned unexpected error for valid node: %v", err)
	}
}

// TestVerifyNodeHash_Mismatch tampers with a node field and verifies that
// VerifyNodeHash returns a non-nil error.
func TestVerifyNodeHash_Mismatch(t *testing.T) {
	repoURL := "github.com/org/repo"
	packagePath := "pkg/sub"
	symbolName := "MyFunc"
	symbolKind := "function"

	// Use a hash computed with a different symbol kind to cause a mismatch.
	badHash := ComputeNodeHash(repoURL, packagePath, EmptyHash, symbolName, "type")
	n := Node{
		NodeHash:      badHash,
		QualifiedName: repoURL + "://" + packagePath + "." + symbolName,
		Kind:          symbolKind, // "function" != "type" used in hash
	}

	if err := VerifyNodeHash(n, repoURL, packagePath); err == nil {
		t.Errorf("VerifyNodeHash returned nil for tampered node; expected mismatch error")
	}
}

// TestVerifyEdgeHash_Match creates an edge with a correctly computed hash and
// verifies that VerifyEdgeHash returns nil.
func TestVerifyEdgeHash_Match(t *testing.T) {
	sourceHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Caller", "function")
	targetHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Callee", "function")
	edgeType := "calls"
	provenance := "ast_resolved"

	edgeHash := ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)
	e := Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgeType,
		Provenance: provenance,
	}

	if err := VerifyEdgeHash(e); err != nil {
		t.Errorf("VerifyEdgeHash returned unexpected error for valid edge: %v", err)
	}
}

// TestVerifyEdgeHash_Mismatch tampers with an edge field and verifies that
// VerifyEdgeHash returns a non-nil error.
func TestVerifyEdgeHash_Mismatch(t *testing.T) {
	sourceHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Caller", "function")
	targetHash := ComputeNodeHash("github.com/org/repo", "pkg", EmptyHash, "Callee", "function")
	edgeType := "calls"
	provenance := "ast_resolved"

	edgeHash := ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance)
	e := Edge{
		EdgeHash:   edgeHash,
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   "imports", // tampered: different edge type
		Provenance: provenance,
	}

	if err := VerifyEdgeHash(e); err == nil {
		t.Errorf("VerifyEdgeHash returned nil for tampered edge; expected mismatch error")
	}
}
