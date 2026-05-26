package store

import (
	"bytes"
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestBatchPutAndGetEmbeddings_RoundTrip(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	h1 := types.NewHash([]byte("node-alpha"))
	h2 := types.NewHash([]byte("node-beta"))
	v1 := []byte{0x01, 0x02, 0x03, 0x04}
	v2 := []byte{0x05, 0x06, 0x07, 0x08}

	if err := s.BatchPutEmbeddings(ctx, "test-model", []types.Hash{h1, h2}, [][]byte{v1, v2}); err != nil {
		t.Fatalf("BatchPutEmbeddings: %v", err)
	}

	got, err := s.GetEmbeddings(ctx, "test-model", []types.Hash{h1, h2})
	if err != nil {
		t.Fatalf("GetEmbeddings: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}
	if !bytes.Equal(got[h1], v1) {
		t.Errorf("h1: got %v, want %v", got[h1], v1)
	}
	if !bytes.Equal(got[h2], v2) {
		t.Errorf("h2: got %v, want %v", got[h2], v2)
	}
}

func TestGetEmbeddings_MultipleModels(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	h := types.NewHash([]byte("shared-node"))
	vA := []byte{0xAA, 0xBB}
	vB := []byte{0xCC, 0xDD}

	if err := s.BatchPutEmbeddings(ctx, "model-a", []types.Hash{h}, [][]byte{vA}); err != nil {
		t.Fatalf("put model-a: %v", err)
	}
	if err := s.BatchPutEmbeddings(ctx, "model-b", []types.Hash{h}, [][]byte{vB}); err != nil {
		t.Fatalf("put model-b: %v", err)
	}

	gotA, err := s.GetEmbeddings(ctx, "model-a", []types.Hash{h})
	if err != nil {
		t.Fatalf("get model-a: %v", err)
	}
	gotB, err := s.GetEmbeddings(ctx, "model-b", []types.Hash{h})
	if err != nil {
		t.Fatalf("get model-b: %v", err)
	}

	if !bytes.Equal(gotA[h], vA) {
		t.Errorf("model-a: got %v, want %v", gotA[h], vA)
	}
	if !bytes.Equal(gotB[h], vB) {
		t.Errorf("model-b: got %v, want %v", gotB[h], vB)
	}
}

func TestBatchPutEmbeddings_Upsert(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	h := types.NewHash([]byte("upsert-node"))
	original := []byte{0x01, 0x02}
	replacement := []byte{0xFF, 0xFE, 0xFD}

	if err := s.BatchPutEmbeddings(ctx, "m", []types.Hash{h}, [][]byte{original}); err != nil {
		t.Fatalf("first put: %v", err)
	}
	if err := s.BatchPutEmbeddings(ctx, "m", []types.Hash{h}, [][]byte{replacement}); err != nil {
		t.Fatalf("upsert put: %v", err)
	}

	got, err := s.GetEmbeddings(ctx, "m", []types.Hash{h})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Equal(got[h], replacement) {
		t.Errorf("upsert failed: got %v, want %v", got[h], replacement)
	}
}

func TestGetEmbeddings_MissingHashes(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	missing := types.NewHash([]byte("does-not-exist"))

	got, err := s.GetEmbeddings(ctx, "any-model", []types.Hash{missing})
	if err != nil {
		t.Fatalf("GetEmbeddings: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

func TestEmbeddings_EmptyInputs(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	// BatchPutEmbeddings with nil slices
	if err := s.BatchPutEmbeddings(ctx, "m", nil, nil); err != nil {
		t.Errorf("BatchPutEmbeddings(nil): %v", err)
	}

	// BatchPutEmbeddings with empty slices
	if err := s.BatchPutEmbeddings(ctx, "m", []types.Hash{}, [][]byte{}); err != nil {
		t.Errorf("BatchPutEmbeddings(empty): %v", err)
	}

	// GetEmbeddings with nil
	got, err := s.GetEmbeddings(ctx, "m", nil)
	if err != nil {
		t.Errorf("GetEmbeddings(nil): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map for nil input, got %v", got)
	}

	// GetEmbeddings with empty slice
	got, err = s.GetEmbeddings(ctx, "m", []types.Hash{})
	if err != nil {
		t.Errorf("GetEmbeddings(empty): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map for empty input, got %v", got)
	}
}

func TestGetEmbeddings_LargeBatchChunking(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	const count = 250
	hashes := make([]types.Hash, count)
	vectors := make([][]byte, count)
	for i := 0; i < count; i++ {
		hashes[i] = types.NewHash([]byte{byte(i), byte(i >> 8), byte(i >> 16)})
		vectors[i] = []byte{byte(i), byte(i + 1)}
	}

	if err := s.BatchPutEmbeddings(ctx, "chunked", hashes, vectors); err != nil {
		t.Fatalf("BatchPutEmbeddings: %v", err)
	}

	got, err := s.GetEmbeddings(ctx, "chunked", hashes)
	if err != nil {
		t.Fatalf("GetEmbeddings: %v", err)
	}
	if len(got) != count {
		t.Fatalf("expected %d results, got %d", count, len(got))
	}
	for i, h := range hashes {
		if !bytes.Equal(got[h], vectors[i]) {
			t.Errorf("mismatch at index %d", i)
		}
	}
}

func TestGetEmbeddings_MixedHitsAndMisses(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	present := types.NewHash([]byte("present"))
	absent := types.NewHash([]byte("absent"))
	vec := []byte{0xDE, 0xAD}

	if err := s.BatchPutEmbeddings(ctx, "m", []types.Hash{present}, [][]byte{vec}); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := s.GetEmbeddings(ctx, "m", []types.Hash{present, absent})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if !bytes.Equal(got[present], vec) {
		t.Errorf("present hash: got %v, want %v", got[present], vec)
	}
	if _, ok := got[absent]; ok {
		t.Errorf("absent hash should not be in results")
	}
}
