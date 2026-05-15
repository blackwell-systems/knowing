package indexer

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeWork(n int, extractFn func(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error)) []extractWork {
	work := make([]extractWork, n)
	for i := range n {
		opts := types.ExtractOptions{
			FilePath: fmt.Sprintf("file%d.go", i),
			FileHash: types.NewHash([]byte(fmt.Sprintf("hash%d", i))),
			RepoHash: types.NewHash([]byte("repo")),
			Content:  []byte(fmt.Sprintf("content%d", i)),
		}
		work[i] = extractWork{
			opts:    opts,
			extract: extractFn,
		}
	}
	return work
}

func TestParallelExtract_Basic(t *testing.T) {
	extractFn := func(_ context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
		return &types.ExtractResult{
			Nodes: []types.Node{{QualifiedName: opts.FilePath}},
		}, nil
	}
	work := makeWork(10, extractFn)

	results := parallelExtract(context.Background(), work, 4)

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	for i, r := range results {
		if r.err != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.err)
		}
		if r.result == nil {
			t.Errorf("result[%d] has nil result", i)
		}
	}
}

func TestParallelExtract_PreservesOrder(t *testing.T) {
	extractFn := func(_ context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
		return &types.ExtractResult{
			Nodes: []types.Node{{QualifiedName: opts.FilePath}},
		}, nil
	}
	work := makeWork(10, extractFn)

	results := parallelExtract(context.Background(), work, 4)

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("result[%d] unexpected error: %v", i, r.err)
		}
		expected := fmt.Sprintf("file%d.go", i)
		if r.result.Nodes[0].QualifiedName != expected {
			t.Errorf("result[%d]: expected QualifiedName %q, got %q", i, expected, r.result.Nodes[0].QualifiedName)
		}
		if r.file == nil {
			t.Fatalf("result[%d] has nil file", i)
		}
		if r.file.Path != expected {
			t.Errorf("result[%d]: expected file path %q, got %q", i, expected, r.file.Path)
		}
	}
}

func TestParallelExtract_PropagatesErrors(t *testing.T) {
	extractFn := func(_ context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
		if opts.FilePath == "file5.go" {
			return nil, fmt.Errorf("extraction failed for %s", opts.FilePath)
		}
		return &types.ExtractResult{}, nil
	}
	work := makeWork(10, extractFn)

	results := parallelExtract(context.Background(), work, 4)

	if results[5].err == nil {
		t.Fatal("expected error for work item 5")
	}
	if results[5].err.Error() != "extraction failed for file5.go" {
		t.Errorf("unexpected error message: %v", results[5].err)
	}

	// All other results should succeed.
	for i, r := range results {
		if i == 5 {
			continue
		}
		if r.err != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.err)
		}
	}
}

func TestParallelExtract_RespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var started atomic.Int32
	extractFn := func(ctx context.Context, _ types.ExtractOptions) (*types.ExtractResult, error) {
		count := started.Add(1)
		if count >= 3 {
			cancel()
			// Give other workers time to see the cancellation.
			time.Sleep(10 * time.Millisecond)
		}
		// Simulate some work.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Millisecond):
			return &types.ExtractResult{}, nil
		}
	}
	work := makeWork(20, extractFn)

	results := parallelExtract(ctx, work, 2)

	if len(results) != 20 {
		t.Fatalf("expected 20 results, got %d", len(results))
	}

	// At least some results should have context.Canceled errors.
	canceledCount := 0
	for _, r := range results {
		if r.err == context.Canceled {
			canceledCount++
		}
	}
	if canceledCount == 0 {
		t.Error("expected at least one context.Canceled error")
	}
}

func TestParallelExtract_SingleWorker(t *testing.T) {
	var order []int
	extractFn := func(_ context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
		// With 1 worker, items should be processed sequentially.
		for i := range 10 {
			if opts.FilePath == fmt.Sprintf("file%d.go", i) {
				order = append(order, i)
				break
			}
		}
		return &types.ExtractResult{
			Nodes: []types.Node{{QualifiedName: opts.FilePath}},
		}, nil
	}
	work := makeWork(10, extractFn)

	results := parallelExtract(context.Background(), work, 1)

	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}

	// All results should be correct.
	for i, r := range results {
		if r.err != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.err)
		}
		expected := fmt.Sprintf("file%d.go", i)
		if r.result.Nodes[0].QualifiedName != expected {
			t.Errorf("result[%d]: expected %q, got %q", i, expected, r.result.Nodes[0].QualifiedName)
		}
	}

	// With single worker, order should be sequential.
	if len(order) != 10 {
		t.Fatalf("expected 10 ordered items, got %d", len(order))
	}
	for i, v := range order {
		if v != i {
			t.Errorf("order[%d] = %d, expected %d", i, v, i)
		}
	}
}
