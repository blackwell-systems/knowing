package store

import (
	"context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestDeleteRepoData_RemovesAllAssociatedData(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Create two repos: one to delete and one to keep.
	repoA := makeRepo(t, s, "https://github.com/org/repo-a")
	repoB := makeRepo(t, s, "https://github.com/org/repo-b")

	// Create files for both repos.
	fileA1 := makeFile(t, s, repoA, "pkg/foo.go")
	fileA2 := makeFile(t, s, repoA, "pkg/bar.go")
	fileB1 := makeFile(t, s, repoB, "cmd/main.go")

	// Create nodes for both repos.
	nodeA1 := makeNode(t, s, fileA1, "repo-a://pkg/foo.go.Func1", "function")
	nodeA2 := makeNode(t, s, fileA2, "repo-a://pkg/bar.go.Func2", "function")
	nodeB1 := makeNode(t, s, fileB1, "repo-b://cmd/main.go.Main", "function")

	// Create edges: one within repo A, one cross-repo (A->B), one within repo B.
	_ = makeEdge(t, s, nodeA1, nodeA2, "calls")
	_ = makeEdge(t, s, nodeA1, nodeB1, "calls") // cross-repo: should be deleted (source in A)
	edgeB := makeEdge(t, s, nodeB1, nodeB1, "calls")

	// Create a snapshot for repo A.
	snapA := types.Snapshot{
		SnapshotHash: types.NewHash([]byte("snap-a")),
		ParentHash:   types.EmptyHash,
		RepoHash:     repoA.RepoHash,
		CommitHash:   "abc123",
		Timestamp:    time.Now().Unix(),
		NodeCount:    2,
		EdgeCount:    1,
	}
	if err := s.CreateSnapshot(ctx, snapA); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// Create a snapshot for repo B (should be preserved).
	snapB := types.Snapshot{
		SnapshotHash: types.NewHash([]byte("snap-b")),
		ParentHash:   types.EmptyHash,
		RepoHash:     repoB.RepoHash,
		CommitHash:   "def456",
		Timestamp:    time.Now().Unix(),
		NodeCount:    1,
		EdgeCount:    1,
	}
	if err := s.CreateSnapshot(ctx, snapB); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}

	// Record feedback for node in repo A.
	if err := s.RecordFeedback(ctx, nodeA1.NodeHash, "session1", true, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}

	// Record feedback for node in repo B (should be preserved).
	if err := s.RecordFeedback(ctx, nodeB1.NodeHash, "session2", false, types.EmptyHash, types.EmptyHash); err != nil {
		t.Fatalf("RecordFeedback: %v", err)
	}

	// Create graph_notes for node in repo A.
	noteA := types.Note{
		ObjectHash: nodeA1.NodeHash,
		Key:        "community_id",
		Value:      "5",
		UpdatedAt:  time.Now().Unix(),
	}
	if err := s.PutNote(ctx, noteA); err != nil {
		t.Fatalf("PutNote: %v", err)
	}

	// Create graph_notes for node in repo B (should be preserved).
	noteB := types.Note{
		ObjectHash: nodeB1.NodeHash,
		Key:        "community_id",
		Value:      "7",
		UpdatedAt:  time.Now().Unix(),
	}
	if err := s.PutNote(ctx, noteB); err != nil {
		t.Fatalf("PutNote: %v", err)
	}

	// ---- Delete repo A data ----
	result, err := s.DeleteRepoData(ctx, repoA.RepoHash)
	if err != nil {
		t.Fatalf("DeleteRepoData: %v", err)
	}

	// Verify counts in result.
	if result.Nodes != 2 {
		t.Errorf("result.Nodes = %d, want 2", result.Nodes)
	}
	if result.Files != 2 {
		t.Errorf("result.Files = %d, want 2", result.Files)
	}
	if result.Snapshots != 1 {
		t.Errorf("result.Snapshots = %d, want 1", result.Snapshots)
	}
	if result.Feedback != 1 {
		t.Errorf("result.Feedback = %d, want 1", result.Feedback)
	}
	if result.Notes != 1 {
		t.Errorf("result.Notes = %d, want 1", result.Notes)
	}
	// Edges: 2 deleted (A1->A2, A1->B1 because source is in A)
	if result.Edges != 2 {
		t.Errorf("result.Edges = %d, want 2", result.Edges)
	}

	// ---- Verify repo A data is gone ----

	// Nodes for repo A should be gone.
	gotA1, err := s.GetNode(ctx, nodeA1.NodeHash)
	if err != nil {
		t.Fatalf("GetNode A1: %v", err)
	}
	if gotA1 != nil {
		t.Error("nodeA1 should be deleted but was found")
	}

	gotA2, err := s.GetNode(ctx, nodeA2.NodeHash)
	if err != nil {
		t.Fatalf("GetNode A2: %v", err)
	}
	if gotA2 != nil {
		t.Error("nodeA2 should be deleted but was found")
	}

	// Files for repo A should be gone.
	filesA, err := s.FilesByRepo(ctx, repoA.RepoHash)
	if err != nil {
		t.Fatalf("FilesByRepo A: %v", err)
	}
	if len(filesA) != 0 {
		t.Errorf("FilesByRepo A returned %d files, want 0", len(filesA))
	}

	// Snapshot for repo A should be gone.
	gotSnapA, err := s.LatestSnapshot(ctx, repoA.RepoHash)
	if err != nil {
		t.Fatalf("LatestSnapshot A: %v", err)
	}
	if gotSnapA != nil {
		t.Error("snapshot for repo A should be deleted")
	}

	// Feedback for nodeA1 should be gone.
	fbA, err := s.QueryFeedback(ctx, nodeA1.NodeHash)
	if err != nil {
		t.Fatalf("QueryFeedback A: %v", err)
	}
	if fbA.UsefulCount != 0 || fbA.NotUsefulCount != 0 {
		t.Errorf("feedback for nodeA1 should be deleted, got useful=%d notUseful=%d",
			fbA.UsefulCount, fbA.NotUsefulCount)
	}

	// Notes for nodeA1 should be gone.
	notesA, err := s.GetNotes(ctx, nodeA1.NodeHash)
	if err != nil {
		t.Fatalf("GetNotes A: %v", err)
	}
	if len(notesA) != 0 {
		t.Errorf("notes for nodeA1 should be deleted, got %d", len(notesA))
	}

	// ---- Verify repo B data is untouched ----

	// Node B1 should still exist.
	gotB1, err := s.GetNode(ctx, nodeB1.NodeHash)
	if err != nil {
		t.Fatalf("GetNode B1: %v", err)
	}
	if gotB1 == nil {
		t.Fatal("nodeB1 should still exist")
	}

	// Edge B->B should still exist.
	gotEdgeB, err := s.GetEdge(ctx, edgeB.EdgeHash)
	if err != nil {
		t.Fatalf("GetEdge B: %v", err)
	}
	if gotEdgeB == nil {
		t.Error("edgeB should still exist")
	}

	// Files for repo B should still exist.
	filesB, err := s.FilesByRepo(ctx, repoB.RepoHash)
	if err != nil {
		t.Fatalf("FilesByRepo B: %v", err)
	}
	if len(filesB) != 1 {
		t.Errorf("FilesByRepo B returned %d files, want 1", len(filesB))
	}

	// Snapshot for repo B should still exist.
	gotSnapB, err := s.LatestSnapshot(ctx, repoB.RepoHash)
	if err != nil {
		t.Fatalf("LatestSnapshot B: %v", err)
	}
	if gotSnapB == nil {
		t.Error("snapshot for repo B should still exist")
	}

	// Feedback for nodeB1 should still exist.
	fbB, err := s.QueryFeedback(ctx, nodeB1.NodeHash)
	if err != nil {
		t.Fatalf("QueryFeedback B: %v", err)
	}
	if fbB.NotUsefulCount != 1 {
		t.Errorf("feedback for nodeB1 should be preserved, got notUseful=%d", fbB.NotUsefulCount)
	}

	// Notes for nodeB1 should still exist.
	notesB, err := s.GetNotes(ctx, nodeB1.NodeHash)
	if err != nil {
		t.Fatalf("GetNotes B: %v", err)
	}
	if len(notesB) != 1 {
		t.Errorf("notes for nodeB1 should be preserved, got %d", len(notesB))
	}

	// ---- Verify repos table is NOT touched ----
	gotRepoA, err := s.GetRepo(ctx, repoA.RepoHash)
	if err != nil {
		t.Fatalf("GetRepo A: %v", err)
	}
	if gotRepoA == nil {
		t.Error("repos entry for repo A should NOT be deleted by DeleteRepoData")
	}
}

func TestDeleteRepoData_EmptyRepo(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Create repo with no files/nodes.
	repo := makeRepo(t, s, "https://github.com/org/empty")

	result, err := s.DeleteRepoData(ctx, repo.RepoHash)
	if err != nil {
		t.Fatalf("DeleteRepoData: %v", err)
	}

	if result.Nodes != 0 || result.Edges != 0 || result.Files != 0 ||
		result.Snapshots != 0 || result.Feedback != 0 || result.Notes != 0 {
		t.Errorf("expected all zeros for empty repo, got %+v", result)
	}
}

func TestDeleteRepoData_NonexistentRepo(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Try deleting a repo that does not exist (no error, just zero counts).
	fakeHash := types.NewHash([]byte("nonexistent-repo"))
	result, err := s.DeleteRepoData(ctx, fakeHash)
	if err != nil {
		t.Fatalf("DeleteRepoData: %v", err)
	}

	if result.Nodes != 0 || result.Edges != 0 || result.Files != 0 ||
		result.Snapshots != 0 || result.Feedback != 0 || result.Notes != 0 {
		t.Errorf("expected all zeros for nonexistent repo, got %+v", result)
	}
}
