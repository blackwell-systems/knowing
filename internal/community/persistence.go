package community

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// NoteKey is the notes table key used for community assignments.
const NoteKey = "community_id"

// batchNoteStore is an optional interface for stores that support batch note writes.
type batchNoteStore interface {
	BatchPutNotes(ctx context.Context, notes []types.Note) error
}

// SaveAssignments persists community assignments to the notes table.
// Each node's community ID is stored as a note with key "community_id".
// Uses BatchPutNotes when available for ~100x speedup on large graphs.
func SaveAssignments(ctx context.Context, store types.GraphStore, assignments map[types.Hash]int) error {
	now := time.Now().Unix()

	notes := make([]types.Note, 0, len(assignments))
	for hash, cid := range assignments {
		notes = append(notes, types.Note{
			ObjectHash: hash,
			Key:        NoteKey,
			Value:      strconv.Itoa(cid),
			UpdatedAt:  now,
		})
	}

	// Use batch insert if the store supports it.
	if bs, ok := store.(batchNoteStore); ok {
		return bs.BatchPutNotes(ctx, notes)
	}

	// Fallback: individual inserts.
	for _, n := range notes {
		if err := store.PutNote(ctx, n); err != nil {
			return fmt.Errorf("saving community assignment for %s: %w", n.ObjectHash, err)
		}
	}
	return nil
}

// LoadAssignments retrieves previously persisted community assignments from
// the notes table. Returns nil (not an error) if no assignments are stored.
func LoadAssignments(ctx context.Context, store types.GraphStore) (map[types.Hash]int, error) {
	notes, err := store.GetNotesByKey(ctx, NoteKey)
	if err != nil {
		return nil, fmt.Errorf("loading community assignments: %w", err)
	}
	if len(notes) == 0 {
		return nil, nil
	}
	assignments := make(map[types.Hash]int, len(notes))
	for _, n := range notes {
		cid, err := strconv.Atoi(n.Value)
		if err != nil {
			continue // skip corrupt entries
		}
		assignments[n.ObjectHash] = cid
	}
	return assignments, nil
}
