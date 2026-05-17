package mcp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
)

func testServerWithSQLStore(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	sqlStore, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { sqlStore.Close() })

	s := &Server{
		store:    sqlStore,
		sqlStore: sqlStore,
	}
	return s
}

func makeFeedbackRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "feedback",
			Arguments: args,
		},
	}
}

func TestFeedback_Record(t *testing.T) {
	s := testServerWithSQLStore(t)
	ctx := context.Background()

	// Valid hash (64 hex chars).
	hashHex := hex.EncodeToString(make([]byte, 32))

	result, err := s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "record",
		"symbol_hash": hashHex,
		"session_id":  "test-session",
		"useful":      true,
	}))
	if err != nil {
		t.Fatalf("handleFeedback error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}
}

func TestFeedback_Query(t *testing.T) {
	s := testServerWithSQLStore(t)
	ctx := context.Background()

	hashHex := hex.EncodeToString(make([]byte, 32))

	// Record some data first.
	_, _ = s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "record",
		"symbol_hash": hashHex,
		"session_id":  "s1",
		"useful":      true,
	}))
	_, _ = s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "record",
		"symbol_hash": hashHex,
		"session_id":  "s2",
		"useful":      false,
	}))

	result, err := s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "query",
		"symbol_hash": hashHex,
	}))
	if err != nil {
		t.Fatalf("handleFeedback error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	// Parse the JSON response.
	textContent := result.Content[0].(mcp.TextContent)
	var stats store.FeedbackStats
	if err := json.Unmarshal([]byte(textContent.Text), &stats); err != nil {
		t.Fatalf("unmarshal stats: %v", err)
	}
	if stats.UsefulCount != 1 {
		t.Errorf("UsefulCount: got %d, want 1", stats.UsefulCount)
	}
	if stats.NotUsefulCount != 1 {
		t.Errorf("NotUsefulCount: got %d, want 1", stats.NotUsefulCount)
	}
	if stats.Score != 0.5 {
		t.Errorf("Score: got %f, want 0.5", stats.Score)
	}
}

func TestFeedback_MissingArgs(t *testing.T) {
	s := testServerWithSQLStore(t)
	ctx := context.Background()

	tests := []struct {
		name string
		args map[string]any
	}{
		{"missing action", map[string]any{"symbol_hash": hex.EncodeToString(make([]byte, 32))}},
		{"missing symbol_hash", map[string]any{"action": "record"}},
		{"record missing session_id", map[string]any{
			"action":      "record",
			"symbol_hash": hex.EncodeToString(make([]byte, 32)),
			"useful":      true,
		}},
		{"record missing useful", map[string]any{
			"action":      "record",
			"symbol_hash": hex.EncodeToString(make([]byte, 32)),
			"session_id":  "s1",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.handleFeedback(ctx, makeFeedbackRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if !result.IsError {
				t.Error("expected tool error for missing args")
			}
		})
	}
}

func TestFeedback_InvalidHash(t *testing.T) {
	s := testServerWithSQLStore(t)
	ctx := context.Background()

	result, err := s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "query",
		"symbol_hash": "not-a-valid-hex",
	}))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for invalid hash")
	}
}

func TestFeedback_NilSQLStore(t *testing.T) {
	s := &Server{
		store:    nil,
		sqlStore: nil,
	}
	ctx := context.Background()

	result, err := s.handleFeedback(ctx, makeFeedbackRequest(map[string]any{
		"action":      "query",
		"symbol_hash": hex.EncodeToString(make([]byte, 32)),
	}))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for nil sqlStore")
	}
}
