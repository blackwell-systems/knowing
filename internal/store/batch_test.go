package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

func TestMaxChunkSize(t *testing.T) {
	tests := []struct {
		paramsPerItem int
		want          int
	}{
		{10, 99},
		{9, 111},
		{4, 249},
		{1, 999},
		{999, 1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("params=%d", tt.paramsPerItem), func(t *testing.T) {
			got := MaxChunkSize(tt.paramsPerItem)
			if got != tt.want {
				t.Errorf("MaxChunkSize(%d) = %d, want %d", tt.paramsPerItem, got, tt.want)
			}
		})
	}
}

func TestChunkedExec_EmptySlice(t *testing.T) {
	// ChunkedExec with an empty slice should be a no-op and return nil,
	// without ever calling buildSQL or touching the transaction.
	err := ChunkedExec(context.Background(), nil, []int{}, 10, func(chunk []int) (string, []interface{}) {
		t.Fatal("buildSQL should not be called for empty slice")
		return "", nil
	})
	if err != nil {
		t.Fatalf("ChunkedExec with empty slice returned error: %v", err)
	}
}

func TestChunkedExec_Chunking(t *testing.T) {
	// Use an in-memory SQLite database to test real chunking behavior.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE test_items (id INTEGER PRIMARY KEY, value TEXT)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// Insert 250 items with 4 params each. MaxChunkSize(4) = 249, so this
	// should require exactly 2 chunks (249 + 1).
	items := make([]int, 250)
	for i := range items {
		items[i] = i
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	chunksExecuted := 0
	err = ChunkedExec(context.Background(), tx, items, 4, func(chunk []int) (string, []interface{}) {
		chunksExecuted++
		var b strings.Builder
		b.WriteString("INSERT INTO test_items (id, value) VALUES ")
		args := make([]interface{}, 0, len(chunk)*2)
		for j, item := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?, ?)")
			args = append(args, item, fmt.Sprintf("value-%d", item))
		}
		return b.String(), args
	})
	if err != nil {
		t.Fatalf("ChunkedExec returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if chunksExecuted != 2 {
		t.Errorf("expected 2 chunks, got %d", chunksExecuted)
	}

	// Verify all items were inserted.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM test_items").Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 250 {
		t.Errorf("expected 250 rows, got %d", count)
	}
}

func TestChunkedExec_ExactBoundary(t *testing.T) {
	// When item count is exactly the chunk size, only 1 chunk should be executed.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE boundary_test (id INTEGER PRIMARY KEY)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	// MaxChunkSize(10) = 99, so insert exactly 99 items.
	items := make([]int, 99)
	for i := range items {
		items[i] = i
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	chunksExecuted := 0
	err = ChunkedExec(context.Background(), tx, items, 10, func(chunk []int) (string, []interface{}) {
		chunksExecuted++
		var b strings.Builder
		b.WriteString("INSERT INTO boundary_test (id) VALUES ")
		args := make([]interface{}, 0, len(chunk))
		for j, item := range chunk {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?)")
			args = append(args, item)
		}
		return b.String(), args
	})
	if err != nil {
		t.Fatalf("ChunkedExec returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if chunksExecuted != 1 {
		t.Errorf("expected 1 chunk for exact boundary, got %d", chunksExecuted)
	}

	// Verify all items were inserted.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM boundary_test").Scan(&count)
	if err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 99 {
		t.Errorf("expected 99 rows, got %d", count)
	}
}

func TestChunkedExec_ErrorPropagation(t *testing.T) {
	// Verify that SQL errors from a chunk are properly propagated.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck

	items := []int{1, 2, 3}
	err = ChunkedExec(context.Background(), tx, items, 1, func(chunk []int) (string, []interface{}) {
		// This SQL references a non-existent table, which will cause an error.
		return "INSERT INTO nonexistent_table (id) VALUES (?)", []interface{}{chunk[0]}
	})
	if err == nil {
		t.Fatal("expected error from ChunkedExec, got nil")
	}
	if !strings.Contains(err.Error(), "batch chunk 0") {
		t.Errorf("error should contain chunk identifier, got: %v", err)
	}
}
