package diff

import (
	"context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// TestIntegrationSemanticDiffPipeline exercises the full semantic diff pipeline
// end-to-end against a real SQLite database: node/edge insertion, snapshot
// creation, simulated refactoring (rename a function), and verification of the
// enriched diff and PR impact outputs.
func TestIntegrationSemanticDiffPipeline(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// --- Step 1: Set up two packages with nodes and edges ---

	repo := putRepo(t, s, "https://example.com/myapp")
	authFile := putFile(t, s, repo, "pkg/auth/auth.go")
	handlersFile := putFile(t, s, repo, "pkg/handlers/login.go")

	validateToken := putNode(t, s, authFile, "pkg/auth.ValidateToken", "function")
	loginHandler := putNode(t, s, handlersFile, "pkg/handlers.LoginHandler", "function")

	// LoginHandler calls ValidateToken.
	oldCallEdge := putEdge(t, s, loginHandler, validateToken, "calls")

	// --- Step 2: Create the first snapshot (old state) ---

	oldSnap := types.NewHash([]byte("snapshot-v1"))
	if err := s.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: oldSnap,
		RepoHash:     repo.RepoHash,
		CommitHash:   "aaa111",
		Timestamp:    time.Now().Unix(),
		NodeCount:    2,
		EdgeCount:    1,
	}); err != nil {
		t.Fatalf("CreateSnapshot (old): %v", err)
	}

	// Record the old edge as added in the old snapshot (baseline).
	recordEdgeEvent(t, s, oldCallEdge, "added", oldSnap)

	// --- Step 3: Simulate a refactoring: ValidateToken -> VerifyJWT ---

	// Add the new function.
	verifyJWT := putNode(t, s, authFile, "pkg/auth.VerifyJWT", "function")

	// Update LoginHandler's edge: now it calls VerifyJWT instead of ValidateToken.
	newCallEdge := putEdge(t, s, loginHandler, verifyJWT, "calls")

	// --- Step 4: Create the second snapshot (new state) ---

	newSnap := types.NewHash([]byte("snapshot-v2"))
	if err := s.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: newSnap,
		RepoHash:     repo.RepoHash,
		CommitHash:   "bbb222",
		Timestamp:    time.Now().Unix(),
		NodeCount:    3,
		EdgeCount:    2,
	}); err != nil {
		t.Fatalf("CreateSnapshot (new): %v", err)
	}

	// Record edge events for the new snapshot.
	recordEdgeEvent(t, s, oldCallEdge, "removed", newSnap)
	recordEdgeEvent(t, s, newCallEdge, "added", newSnap)

	// --- Step 5: Compute SemanticDiff ---

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	// Verify edges removed.
	if len(result.EdgesRemoved) != 1 {
		t.Fatalf("EdgesRemoved: expected 1, got %d", len(result.EdgesRemoved))
	}
	removedEdge := result.EdgesRemoved[0]
	if removedEdge.SourceName != "pkg/handlers.LoginHandler" {
		t.Errorf("removed edge source: got %q, want %q", removedEdge.SourceName, "pkg/handlers.LoginHandler")
	}
	if removedEdge.TargetName != "pkg/auth.ValidateToken" {
		t.Errorf("removed edge target: got %q, want %q", removedEdge.TargetName, "pkg/auth.ValidateToken")
	}
	if removedEdge.EdgeType != "calls" {
		t.Errorf("removed edge type: got %q, want %q", removedEdge.EdgeType, "calls")
	}

	// Verify edges added.
	if len(result.EdgesAdded) != 1 {
		t.Fatalf("EdgesAdded: expected 1, got %d", len(result.EdgesAdded))
	}
	addedEdge := result.EdgesAdded[0]
	if addedEdge.SourceName != "pkg/handlers.LoginHandler" {
		t.Errorf("added edge source: got %q, want %q", addedEdge.SourceName, "pkg/handlers.LoginHandler")
	}
	if addedEdge.TargetName != "pkg/auth.VerifyJWT" {
		t.Errorf("added edge target: got %q, want %q", addedEdge.TargetName, "pkg/auth.VerifyJWT")
	}

	// Verify modified nodes: LoginHandler should be detected as modified because
	// its outgoing edges changed, but it was not itself added or removed.
	if len(result.NodesModified) != 1 {
		t.Fatalf("NodesModified: expected 1, got %d", len(result.NodesModified))
	}
	mod := result.NodesModified[0]
	if mod.QualifiedName != "pkg/handlers.LoginHandler" {
		t.Errorf("modified node: got %q, want %q", mod.QualifiedName, "pkg/handlers.LoginHandler")
	}
	if len(mod.EdgesAdded) != 1 {
		t.Errorf("modified node edges added: got %d, want 1", len(mod.EdgesAdded))
	}
	if len(mod.EdgesRemoved) != 1 {
		t.Errorf("modified node edges removed: got %d, want 1", len(mod.EdgesRemoved))
	}

	// Verify summary counts.
	if result.Summary.EdgesAdded != 1 {
		t.Errorf("summary EdgesAdded: got %d, want 1", result.Summary.EdgesAdded)
	}
	if result.Summary.EdgesRemoved != 1 {
		t.Errorf("summary EdgesRemoved: got %d, want 1", result.Summary.EdgesRemoved)
	}
	if result.Summary.NodesModified != 1 {
		t.Errorf("summary NodesModified: got %d, want 1", result.Summary.NodesModified)
	}

	// --- Step 6: Compute PRImpact ---

	impact, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// LoginHandler should appear as a modified symbol in the impact analysis.
	if len(impact.ChangedSymbols) < 1 {
		t.Fatalf("ChangedSymbols: expected at least 1, got %d", len(impact.ChangedSymbols))
	}

	foundModified := false
	for _, sym := range impact.ChangedSymbols {
		if sym.Symbol.QualifiedName == "pkg/handlers.LoginHandler" && sym.ChangeType == "modified" {
			foundModified = true
			break
		}
	}
	if !foundModified {
		t.Errorf("expected pkg/handlers.LoginHandler as modified symbol in impact")
	}

	// Verify affected edges are present.
	if len(impact.AffectedEdges) != 2 {
		t.Errorf("AffectedEdges: got %d, want 2", len(impact.AffectedEdges))
	}

	// Verify risk level is computed (low for small change).
	if impact.Summary.RiskLevel == "" {
		t.Error("expected non-empty risk level in impact summary")
	}
}

// TestIntegrationSemanticDiffWithCallerChain tests the blast radius computation
// through PRImpact when a removed node has transitive callers.
func TestIntegrationSemanticDiffWithCallerChain(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	repo := putRepo(t, s, "https://example.com/chain")
	file := putFile(t, s, repo, "pkg/chain.go")

	// Build a call chain: A -> B -> C. We will simulate C being removed.
	nodeA := putNode(t, s, file, "pkg.FuncA", "function")
	nodeB := putNode(t, s, file, "pkg.FuncB", "function")
	nodeC := putNode(t, s, file, "pkg.FuncC", "function")

	edgeAB := putEdge(t, s, nodeA, nodeB, "calls")
	edgeBC := putEdge(t, s, nodeB, nodeC, "calls")

	oldSnap := types.NewHash([]byte("chain-old"))
	newSnap := types.NewHash([]byte("chain-new"))

	// Record both edges added in old snapshot (baseline).
	recordEdgeEvent(t, s, edgeAB, "added", oldSnap)
	recordEdgeEvent(t, s, edgeBC, "added", oldSnap)

	// In the new snapshot, edge B->C is removed (C was deleted).
	recordEdgeEvent(t, s, edgeBC, "removed", newSnap)

	result, err := SemanticDiff(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("SemanticDiff: %v", err)
	}

	// B should be marked as modified (its outgoing edge was removed).
	if len(result.NodesModified) != 1 {
		t.Fatalf("NodesModified: expected 1, got %d", len(result.NodesModified))
	}
	if result.NodesModified[0].QualifiedName != "pkg.FuncB" {
		t.Errorf("modified node: got %q, want %q", result.NodesModified[0].QualifiedName, "pkg.FuncB")
	}

	// Verify PRImpact for the modified node.
	impact, err := PRImpact(ctx, s, oldSnap, newSnap)
	if err != nil {
		t.Fatalf("PRImpact: %v", err)
	}

	// FuncB should appear as modified with callers (FuncA calls it).
	foundB := false
	for _, sym := range impact.ChangedSymbols {
		if sym.Symbol.QualifiedName == "pkg.FuncB" && sym.ChangeType == "modified" {
			foundB = true
			// FuncA is a caller of FuncB, so it should appear in the callers list.
			if sym.CallerCount < 1 {
				t.Errorf("expected at least 1 caller for FuncB, got %d", sym.CallerCount)
			}
			break
		}
	}
	if !foundB {
		t.Errorf("expected pkg.FuncB as modified symbol in impact, got %+v", impact.ChangedSymbols)
	}
}
