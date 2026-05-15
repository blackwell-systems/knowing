package indexer

import (
	"context"
	"runtime"
	"sync"

	"github.com/blackwell-systems/knowing/internal/types"
)

// worker.go implements a fan-out/fan-in worker pool for parallel file
// extraction. The pattern:
//
//  1. Fan-out: all work items are pre-loaded into a buffered channel.
//  2. N worker goroutines read from the channel and execute extractions.
//  3. Results are written directly into a pre-allocated slice at the
//     original work item's index, preserving submission order.
//  4. Fan-in: WaitGroup.Wait blocks until all workers complete.
//
// This avoids a separate collector goroutine and preserves ordering
// without sorting. The results slice is safe for concurrent writes because
// each worker writes to a distinct index.

// extractWork represents a single file extraction work item.
type extractWork struct {
	opts    types.ExtractOptions
	extract func(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error)
}

// extractResult holds the output of a single file extraction.
type extractResult struct {
	result *types.ExtractResult
	file   *types.File
	err    error
}

// parallelExtract runs extraction work items across numWorkers goroutines.
// Returns all results in submission order. If ctx is cancelled, workers
// drain remaining items and record context errors for each.
func parallelExtract(ctx context.Context, work []extractWork, numWorkers int) []extractResult {
	if numWorkers <= 0 {
		numWorkers = runtime.GOMAXPROCS(0)
	}
	if numWorkers > len(work) {
		numWorkers = len(work)
	}
	if len(work) == 0 {
		return nil
	}

	// Pre-allocate the results slice so each worker can write to its own
	// index without synchronization (distinct indices, no data races).
	results := make([]extractResult, len(work))

	type indexedWork struct {
		index int
		item  extractWork
	}

	// Pre-load all work items into a buffered channel. Workers pull from
	// this channel until it is drained and closed.
	ch := make(chan indexedWork, len(work))
	for i, w := range work {
		ch <- indexedWork{index: i, item: w}
	}
	close(ch)

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for range numWorkers {
		go func() {
			defer wg.Done()
			for iw := range ch {
				if err := ctx.Err(); err != nil {
					results[iw.index] = extractResult{err: err}
					continue
				}
				res, err := iw.item.extract(ctx, iw.item.opts)
				if err != nil {
					results[iw.index] = extractResult{err: err}
				} else {
					contentHash := types.NewHash(iw.item.opts.Content)
					file := &types.File{
						FileHash:    iw.item.opts.FileHash,
						RepoHash:    iw.item.opts.RepoHash,
						Path:        iw.item.opts.FilePath,
						ContentHash: contentHash,
					}
					results[iw.index] = extractResult{result: res, file: file}
				}
			}
		}()
	}

	wg.Wait()
	return results
}
