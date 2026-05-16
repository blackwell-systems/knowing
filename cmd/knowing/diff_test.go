package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TestCmdDiff_NoArgs verifies that cmdDiff returns an error when no snapshot
// hashes are provided.
func TestCmdDiff_NoArgs(t *testing.T) {
	err := cmdDiff([]string{})
	if err == nil {
		t.Fatal("expected error for diff without arguments")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("expected 'usage' in error, got %q", err.Error())
	}
}

// TestCmdDiff_InvalidHash verifies that cmdDiff returns an error for a
// malformed hex hash.
func TestCmdDiff_InvalidHash(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Too short (not 64 hex chars).
	err := cmdDiff([]string{"-db", dbPath, "abcd", "1234"})
	if err == nil {
		t.Fatal("expected error for short hash")
	}
	if !strings.Contains(err.Error(), "invalid old snapshot hash") {
		t.Errorf("expected 'invalid old snapshot hash' in error, got %q", err.Error())
	}

	// Invalid hex characters.
	badHex := strings.Repeat("zz", 32)
	goodHex := strings.Repeat("aa", 32)
	err = cmdDiff([]string{"-db", dbPath, badHex, goodHex})
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
	if !strings.Contains(err.Error(), "invalid old snapshot hash") {
		t.Errorf("expected 'invalid old snapshot hash' in error, got %q", err.Error())
	}

	// Valid old hash, invalid new hash.
	err = cmdDiff([]string{"-db", dbPath, goodHex, badHex})
	if err == nil {
		t.Fatal("expected error for invalid new hash")
	}
	if !strings.Contains(err.Error(), "invalid new snapshot hash") {
		t.Errorf("expected 'invalid new snapshot hash' in error, got %q", err.Error())
	}
}

// TestCmdDiff_EmptyDB creates a temp DB with two empty snapshots and verifies
// that cmdDiff produces valid output for both text and JSON formats.
func TestCmdDiff_EmptyDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create store and insert two empty snapshots.
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	oldHash := types.NewHash([]byte("old-snapshot-content"))
	newHash := types.NewHash([]byte("new-snapshot-content"))

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: oldHash,
		CommitHash:   "aaa111",
		Timestamp:    1000,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = st.CreateSnapshot(ctx, types.Snapshot{
		SnapshotHash: newHash,
		CommitHash:   "bbb222",
		Timestamp:    2000,
	})
	if err != nil {
		t.Fatal(err)
	}

	st.Close()

	oldHex := hex.EncodeToString(oldHash[:])
	newHex := hex.EncodeToString(newHash[:])

	// Test JSON format.
	t.Run("json", func(t *testing.T) {
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w

		err = cmdDiff([]string{"-db", dbPath, "-format", "json", oldHex, newHex})

		w.Close()
		os.Stdout = old

		if err != nil {
			t.Fatalf("cmdDiff json: %v", err)
		}

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err != nil {
			t.Fatal(err)
		}

		// Verify valid JSON.
		var result map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("invalid JSON output: %v\noutput: %s", err, buf.String())
		}
	})

	// Test text format.
	t.Run("text", func(t *testing.T) {
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w

		err = cmdDiff([]string{"-db", dbPath, "-format", "text", oldHex, newHex})

		w.Close()
		os.Stdout = old

		if err != nil {
			t.Fatalf("cmdDiff text: %v", err)
		}

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err != nil {
			t.Fatal(err)
		}

		output := buf.String()
		if !strings.Contains(output, "Semantic Diff:") {
			t.Errorf("expected 'Semantic Diff:' header in text output, got %q", output)
		}
		if !strings.Contains(output, "Summary:") {
			t.Errorf("expected 'Summary:' in text output, got %q", output)
		}
	})
}

// TestCmdDiff_UnsupportedFormat verifies that an unsupported format returns an error.
func TestCmdDiff_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create store and insert two snapshots so we get past hash parsing.
	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	oldHash := types.NewHash([]byte("fmt-old"))
	newHash := types.NewHash([]byte("fmt-new"))

	_ = st.CreateSnapshot(ctx, types.Snapshot{SnapshotHash: oldHash, CommitHash: "c1", Timestamp: 1000})
	_ = st.CreateSnapshot(ctx, types.Snapshot{SnapshotHash: newHash, CommitHash: "c2", Timestamp: 2000})
	st.Close()

	oldHex := hex.EncodeToString(oldHash[:])
	newHex := hex.EncodeToString(newHash[:])

	err = cmdDiff([]string{"-db", dbPath, "-format", "yaml", oldHex, newHex})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("expected 'unsupported format' in error, got %q", err.Error())
	}
}
