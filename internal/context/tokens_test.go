package context

import (
	"strings"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestEstimateTokens_Empty(t *testing.T) {
	got := EstimateTokens("")
	if got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokens_Short(t *testing.T) {
	// "hello" is 5 chars; 5/4 = 1
	got := EstimateTokens("hello")
	if got != 1 {
		t.Errorf("EstimateTokens(\"hello\") = %d, want 1", got)
	}
}

func TestEstimateTokens_Long(t *testing.T) {
	// 400 chars / 4 = 100
	text := strings.Repeat("x", 400)
	got := EstimateTokens(text)
	if got != 100 {
		t.Errorf("EstimateTokens(400 chars) = %d, want 100", got)
	}
}

func TestEstimateTokens_CodeSnippet(t *testing.T) {
	snippet := "func (s *SQLiteStore) PutNode(ctx context.Context, n types.Node) error"
	got := EstimateTokens(snippet)
	want := len(snippet) / 4
	if got != want {
		t.Errorf("EstimateTokens(snippet) = %d, want %d", got, want)
	}
}

func TestEstimateNodeTokens_Basic(t *testing.T) {
	node := types.Node{
		QualifiedName: "github.com/org/repo://pkg.MyFunc",
		Kind:          "function",
		Signature:     "func MyFunc(x int) error",
	}
	// Should include QualifiedName + " " + Kind + " " + Signature
	combined := node.QualifiedName + " " + node.Kind + " " + node.Signature
	want := len(combined) / 4
	got := EstimateNodeTokens(node)
	if got != want {
		t.Errorf("EstimateNodeTokens = %d, want %d", got, want)
	}
}
