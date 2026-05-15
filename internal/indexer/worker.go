package indexer

import (
	"context"
	"runtime"
	"sync"

	"github.com/blackwell-systems/knowing/internal/types"
)

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
// drain and return context errors for remaining items.
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

	results := make([]extractResult, len(work))

	type indexedWork struct {
		index int
		item  extractWork
	}

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
