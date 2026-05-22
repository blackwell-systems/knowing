package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestNotes_PutAndGet(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	objHash := types.NewHash([]byte("test-object"))
	now := time.Now().Unix()

	note := types.Note{
		ObjectHash: objHash,
		Key:        "community_id",
		Value:      "42",
		UpdatedAt:  now,
	}

	if err := s.PutNote(ctx, note); err != nil {
		t.Fatalf("PutNote: %v", err)
	}

	got, err := s.GetNote(ctx, objHash, "community_id")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got == nil {
		t.Fatal("GetNote returned nil")
	}
	if got.ObjectHash != objHash {
		t.Errorf("ObjectHash = %s, want %s", got.ObjectHash, objHash)
	}
	if got.Key != "community_id" {
		t.Errorf("Key = %q, want %q", got.Key, "community_id")
	}
	if got.Value != "42" {
		t.Errorf("Value = %q, want %q", got.Value, "42")
	}
	if got.UpdatedAt != now {
		t.Errorf("UpdatedAt = %d, want %d", got.UpdatedAt, now)
	}
}

func TestNotes_GetNotFound(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	got, err := s.GetNote(ctx, types.NewHash([]byte("nonexistent")), "key")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for nonexistent note, got %+v", got)
	}
}

func TestNotes_Upsert(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	objHash := types.NewHash([]byte("test-object"))

	// Insert initial value.
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: objHash,
		Key:        "score",
		Value:      "0.5",
		UpdatedAt:  100,
	}); err != nil {
		t.Fatalf("PutNote initial: %v", err)
	}

	// Upsert with new value.
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: objHash,
		Key:        "score",
		Value:      "0.9",
		UpdatedAt:  200,
	}); err != nil {
		t.Fatalf("PutNote upsert: %v", err)
	}

	got, err := s.GetNote(ctx, objHash, "score")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if got.Value != "0.9" {
		t.Errorf("Value after upsert = %q, want %q", got.Value, "0.9")
	}
	if got.UpdatedAt != 200 {
		t.Errorf("UpdatedAt after upsert = %d, want 200", got.UpdatedAt)
	}
}

func TestNotes_GetAllForObject(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	objHash := types.NewHash([]byte("multi-note-object"))

	for _, kv := range []struct{ k, v string }{
		{"community_id", "7"},
		{"quality_score", "0.95"},
		{"annotation", `{"tag":"important"}`},
	} {
		if err := s.PutNote(ctx, types.Note{
			ObjectHash: objHash,
			Key:        kv.k,
			Value:      kv.v,
			UpdatedAt:  time.Now().Unix(),
		}); err != nil {
			t.Fatalf("PutNote(%s): %v", kv.k, err)
		}
	}

	notes, err := s.GetNotes(ctx, objHash)
	if err != nil {
		t.Fatalf("GetNotes: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("GetNotes returned %d notes, want 3", len(notes))
	}

	// Results should be ordered by key.
	if notes[0].Key != "annotation" {
		t.Errorf("notes[0].Key = %q, want %q", notes[0].Key, "annotation")
	}
	if notes[1].Key != "community_id" {
		t.Errorf("notes[1].Key = %q, want %q", notes[1].Key, "community_id")
	}
	if notes[2].Key != "quality_score" {
		t.Errorf("notes[2].Key = %q, want %q", notes[2].Key, "quality_score")
	}
}

func TestNotes_GetByKey(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Attach same key to multiple objects.
	for i, obj := range []string{"node-a", "node-b", "node-c"} {
		if err := s.PutNote(ctx, types.Note{
			ObjectHash: types.NewHash([]byte(obj)),
			Key:        "community_id",
			Value:      string(rune('1' + i)),
			UpdatedAt:  time.Now().Unix(),
		}); err != nil {
			t.Fatalf("PutNote(%s): %v", obj, err)
		}
	}

	notes, err := s.GetNotesByKey(ctx, "community_id")
	if err != nil {
		t.Fatalf("GetNotesByKey: %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("GetNotesByKey returned %d notes, want 3", len(notes))
	}
}

func TestNotes_DeleteSingle(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	objHash := types.NewHash([]byte("delete-test"))

	if err := s.PutNote(ctx, types.Note{
		ObjectHash: objHash,
		Key:        "keep",
		Value:      "yes",
		UpdatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: objHash,
		Key:        "remove",
		Value:      "bye",
		UpdatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteNote(ctx, objHash, "remove"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}

	// "remove" should be gone.
	got, err := s.GetNote(ctx, objHash, "remove")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("deleted note still returned by GetNote")
	}

	// "keep" should survive.
	got, err = s.GetNote(ctx, objHash, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("surviving note not found after sibling delete")
	}
}

func TestNotes_DeleteAllForObject(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	objHash := types.NewHash([]byte("nuke-test"))

	for _, k := range []string{"a", "b", "c"} {
		if err := s.PutNote(ctx, types.Note{
			ObjectHash: objHash,
			Key:        k,
			Value:      "v",
			UpdatedAt:  time.Now().Unix(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.DeleteNotesByObject(ctx, objHash); err != nil {
		t.Fatalf("DeleteNotesByObject: %v", err)
	}

	notes, err := s.GetNotes(ctx, objHash)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes after DeleteNotesByObject, got %d", len(notes))
	}
}

func TestNotes_IsolationBetweenObjects(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	obj1 := types.NewHash([]byte("object-1"))
	obj2 := types.NewHash([]byte("object-2"))

	if err := s.PutNote(ctx, types.Note{
		ObjectHash: obj1, Key: "shared_key", Value: "val1", UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: obj2, Key: "shared_key", Value: "val2", UpdatedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}

	// Each object should see its own value.
	n1, _ := s.GetNote(ctx, obj1, "shared_key")
	n2, _ := s.GetNote(ctx, obj2, "shared_key")
	if n1.Value != "val1" {
		t.Errorf("obj1 value = %q, want %q", n1.Value, "val1")
	}
	if n2.Value != "val2" {
		t.Errorf("obj2 value = %q, want %q", n2.Value, "val2")
	}

	// Deleting obj1's notes should not affect obj2.
	if err := s.DeleteNotesByObject(ctx, obj1); err != nil {
		t.Fatal(err)
	}
	n2after, _ := s.GetNote(ctx, obj2, "shared_key")
	if n2after == nil {
		t.Error("obj2 note deleted when obj1 notes were purged")
	}
}

func TestCommunitiesForNodes_Empty(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	result, err := s.CommunitiesForNodes(ctx, nil)
	if err != nil {
		t.Fatalf("CommunitiesForNodes(nil): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil map for empty input, got %v", result)
	}

	result, err = s.CommunitiesForNodes(ctx, []types.Hash{})
	if err != nil {
		t.Fatalf("CommunitiesForNodes([]): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil map for empty slice, got %v", result)
	}
}

func TestCommunitiesForNodes_Found(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// Insert 3 community_id notes.
	hashes := make([]types.Hash, 3)
	for i := range hashes {
		hashes[i] = types.NewHash([]byte(fmt.Sprintf("comm-node-%d", i)))
		if err := s.PutNote(ctx, types.Note{
			ObjectHash: hashes[i],
			Key:        "community_id",
			Value:      fmt.Sprintf("%d", (i+1)*10),
			UpdatedAt:  time.Now().Unix(),
		}); err != nil {
			t.Fatalf("PutNote(%d): %v", i, err)
		}
	}

	// Query with those 3 plus an unknown hash.
	unknown := types.NewHash([]byte("unknown-node"))
	query := append(hashes, unknown)

	result, err := s.CommunitiesForNodes(ctx, query)
	if err != nil {
		t.Fatalf("CommunitiesForNodes: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(result), result)
	}

	for i, h := range hashes {
		want := (i + 1) * 10
		got, ok := result[h]
		if !ok {
			t.Errorf("hash %d not found in result", i)
			continue
		}
		if got != want {
			t.Errorf("hash %d: community = %d, want %d", i, got, want)
		}
	}

	if _, ok := result[unknown]; ok {
		t.Error("unknown hash should not be in result")
	}
}

func TestCommunitiesForNodes_LargeBatch(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	const count = 150
	hashes := make([]types.Hash, count)
	for i := range hashes {
		hashes[i] = types.NewHash([]byte(fmt.Sprintf("large-batch-node-%d", i)))
		if err := s.PutNote(ctx, types.Note{
			ObjectHash: hashes[i],
			Key:        "community_id",
			Value:      fmt.Sprintf("%d", i),
			UpdatedAt:  time.Now().Unix(),
		}); err != nil {
			t.Fatalf("PutNote(%d): %v", i, err)
		}
	}

	result, err := s.CommunitiesForNodes(ctx, hashes)
	if err != nil {
		t.Fatalf("CommunitiesForNodes: %v", err)
	}

	if len(result) != count {
		t.Fatalf("expected %d entries, got %d", count, len(result))
	}

	for i, h := range hashes {
		got, ok := result[h]
		if !ok {
			t.Errorf("hash %d not found in result", i)
			continue
		}
		if got != i {
			t.Errorf("hash %d: community = %d, want %d", i, got, i)
		}
	}
}

func TestCommunitiesForNodes_CorruptValue(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	goodHash := types.NewHash([]byte("good-node"))
	badHash := types.NewHash([]byte("corrupt-node"))

	// Insert a valid community_id note.
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: goodHash,
		Key:        "community_id",
		Value:      "42",
		UpdatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	// Insert a corrupt (non-integer) community_id note.
	if err := s.PutNote(ctx, types.Note{
		ObjectHash: badHash,
		Key:        "community_id",
		Value:      "notanumber",
		UpdatedAt:  time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := s.CommunitiesForNodes(ctx, []types.Hash{goodHash, badHash})
	if err != nil {
		t.Fatalf("CommunitiesForNodes: %v", err)
	}

	// Good hash should be present with correct value.
	if got, ok := result[goodHash]; !ok || got != 42 {
		t.Errorf("goodHash: got (%d, %v), want (42, true)", got, ok)
	}

	// Bad hash should be absent (skipped due to corrupt value).
	if _, ok := result[badHash]; ok {
		t.Error("corrupt value hash should not be in result")
	}
}
