package store

import (
	"context"
	"database/sql"
	"fmt"
)

// MaxChunkSize returns the maximum number of items per SQL batch
// to stay within SQLite's 999-variable limit.
func MaxChunkSize(paramsPerItem int) int {
	return 999 / paramsPerItem
}

// ChunkedExec executes a batch SQL operation in chunks within the provided
// transaction. Each chunk is passed to buildSQL which returns the SQL string
// and the flattened argument slice.
func ChunkedExec[T any](
	ctx context.Context,
	tx *sql.Tx,
	items []T,
	paramsPerItem int,
	buildSQL func(chunk []T) (string, []interface{}),
) error {
	if len(items) == 0 {
		return nil
	}
	chunkSize := MaxChunkSize(paramsPerItem)
	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		query, args := buildSQL(items[i:end])
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("batch chunk %d: %w", i/chunkSize, err)
		}
	}
	return nil
}
