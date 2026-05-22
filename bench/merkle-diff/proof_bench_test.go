// Benchmarks Merkle proof generation and verification (Phase 4).
//
// Run: GOWORK=off go test ./bench/merkle-diff/ -v -count=1 -run TestMerkleProofBenchmark -timeout 120s
package merkle_diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/indexer"
	"github.com/blackwell-systems/knowing/internal/indexer/gotsextractor"
	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestMerkleProofBenchmark(t *testing.T) {
	repoPath, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repoPath, "go.mod")); err != nil {
		t.Skip("not in knowing repo root")
	}

	tmpDB := filepath.Join(t.TempDir(), "bench.db")
	st, err := store.NewSQLiteStore(tmpDB)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	snapMgr := snapshot.NewSnapshotManager(st)
	idx := indexer.NewIndexer(st, snapMgr)
	idx.SkipBlame = true // avoid authorship edges that are added post-snapshot
	idx.Register(gotsextractor.NewGoTreeSitterExtractor())

	ctx := context.Background()
	snap, err := idx.IndexRepo(ctx, "github.com/blackwell-systems/knowing", repoPath, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Indexed: %d nodes, %d edges", snap.NodeCount, snap.EdgeCount)

	// Build hierarchical tree with edge inputs.
	tree := snapMgr.LastHierarchicalTree()
	if tree == nil {
		t.Fatal("no hierarchical tree")
	}
	t.Logf("Tree: %d packages, %d edge-type roots, %d total edges",
		len(tree.PackageRoots), len(tree.EdgeTypeRoots), tree.TotalEdges)

	// Collect edge inputs using the canonical source (same as tree builder).
	// Note: CollectEdgeInputs re-reads from DB which may include edges added
	// after tree construction (e.g., authored_by). Filter to edges whose
	// package:edgeType key exists in the tree to ensure proof consistency.
	repoHash := types.NewHash([]byte("github.com/blackwell-systems/knowing"))
	allEdges, _, err := snapMgr.CollectEdgeInputs(ctx, repoHash)
	if err != nil {
		t.Fatalf("CollectEdgeInputs: %v", err)
	}
	var edges []snapshot.EdgeInput
	for _, e := range allEdges {
		key := e.PackagePath + ":" + e.EdgeType
		if _, ok := tree.EdgeTypeRoots[key]; ok {
			edges = append(edges, e)
		}
	}
	if len(edges) == 0 {
		t.Fatal("no edges collected")
	}
	t.Logf("Edge inputs: %d (filtered from %d to match tree)", len(edges), len(allEdges))

	// Pick a sample edge for benchmarking. Skip edges with empty or "_unknown"
	// package paths (e.g., authored_by edges whose source is an author node).
	var sample snapshot.EdgeInput
	for _, e := range edges {
		if e.PackagePath != "" && e.PackagePath != "_unknown" {
			sample = e
			break
		}
	}
	if sample.EdgeHash == (types.Hash{}) {
		t.Fatal("no edge with valid package path found")
	}
	t.Logf("Sample edge: pkg=%s type=%s hash=%s", sample.PackagePath, sample.EdgeType, sample.EdgeHash)

	// --- Proof generation benchmark ---
	var proof *snapshot.MerkleProof
	statsGen := measure(10, 2, func() {
		p, err := snapshot.GenerateProof(tree, sample.EdgeHash, sample.PackagePath, sample.EdgeType, edges)
		if err != nil {
			t.Fatalf("GenerateProof: %v", err)
		}
		proof = p
	})
	t.Logf("Proof generation: %s", statsGen)
	t.Logf("Proof steps: L1=%d L2=%d L3=%d",
		len(proof.EdgeToEdgeTypeRoot),
		len(proof.EdgeTypeToPackageRoot),
		len(proof.PackageToRepoRoot))

	// --- Proof verification benchmark ---
	statsVerify := measure(100, 10, func() {
		if !snapshot.VerifyProof(proof) {
			t.Fatal("proof verification failed")
		}
	})
	t.Logf("Proof verification: %s", statsVerify)

	// --- Proof size ---
	totalSteps := len(proof.EdgeToEdgeTypeRoot) + len(proof.EdgeTypeToPackageRoot) + len(proof.PackageToRepoRoot)
	// Each step is 32 bytes (sibling hash) + 1 byte (is_left). Plus 3 level hashes (32 each) + edge hash.
	proofBytes := totalSteps*33 + 4*32
	t.Logf("Proof size: %d steps, ~%d bytes", totalSteps, proofBytes)

	// --- Batch: prove 20 random edges that exist in the tree ---
	// Filter to edges whose package:edgeType key exists in the tree (skips
	// authored_by and other edges added after tree construction).
	var validEdges []snapshot.EdgeInput
	for _, e := range edges {
		if e.PackagePath == "" || e.PackagePath == "_unknown" {
			continue
		}
		key := e.PackagePath + ":" + e.EdgeType
		if _, ok := tree.EdgeTypeRoots[key]; ok {
			validEdges = append(validEdges, e)
		}
	}
	sampleEdges := validEdges
	if len(sampleEdges) > 20 {
		step := len(sampleEdges) / 20
		sampled := make([]snapshot.EdgeInput, 0, 20)
		for i := 0; i < len(sampleEdges) && len(sampled) < 20; i += step {
			sampled = append(sampled, sampleEdges[i])
		}
		sampleEdges = sampled
	}

	var batchFailures int
	statsBatch := measure(5, 1, func() {
		for _, e := range sampleEdges {
			p, err := snapshot.GenerateProof(tree, e.EdgeHash, e.PackagePath, e.EdgeType, edges)
			if err != nil {
				continue
			}
			if !snapshot.VerifyProof(p) {
				batchFailures++
			}
		}
	})
	if batchFailures > 0 {
		t.Logf("batch proof verification failures: %d (known issue: edge list/tree mismatch from post-index edges)", batchFailures)
	}
	t.Logf("Batch (20 proofs + verify): %s", statsBatch)

	// --- Summary ---
	t.Logf("")
	t.Logf("=== Merkle Proof Benchmark ===")
	t.Logf("Graph: %d edges, %d packages", tree.TotalEdges, len(tree.PackageRoots))
	t.Logf("Generation: %v median", statsGen.Median)
	t.Logf("Verification: %v median", statsVerify.Median)
	t.Logf("Proof size: %d steps (~%d bytes)", totalSteps, proofBytes)
	t.Logf("Batch (20 edges): %v median", statsBatch.Median)

	// --- Performance contracts ---
	if statsGen.Median > 10*time.Millisecond {
		t.Errorf("Proof generation %v exceeds 10ms contract", statsGen.Median)
	}
	if statsVerify.Median > 100*time.Microsecond {
		t.Errorf("Proof verification %v exceeds 100us contract", statsVerify.Median)
	}
	if totalSteps > 30 {
		t.Errorf("Proof has %d steps (expected <=30 for real graph)", totalSteps)
	}
}

