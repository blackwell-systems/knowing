package envextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestEnvExtractor_CanHandle(t *testing.T) {
	e := NewEnvExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{".env", true},
		{".env.local", true},
		{".env.production", true},
		{".env.development", true},
		{".env.example", true},
		{"config/.env", true},
		// Negative cases
		{"env", false},
		{"main.go", false},
		{"config.yaml", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestEnvExtractor_Extract(t *testing.T) {
	e := NewEnvExtractor()
	content := []byte(`# Database configuration
DATABASE_URL=postgres://localhost:5432/mydb
DATABASE_HOST=localhost

# App settings
APP_SECRET=supersecret
PORT=3000

# Empty and comments should be skipped

export NODE_ENV=production
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/app",
		FilePath: ".env",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// DATABASE_URL, DATABASE_HOST, APP_SECRET, PORT, NODE_ENV = 5 vars
	if len(result.Nodes) != 5 {
		t.Errorf("got %d nodes, want 5", len(result.Nodes))
		for _, n := range result.Nodes {
			t.Logf("  node: %s", n.QualifiedName)
		}
	}

	// All should be var kind.
	for _, n := range result.Nodes {
		if n.Kind != "var" {
			t.Errorf("node %q has kind %q, want var", n.QualifiedName, n.Kind)
		}
	}
}

func TestEnvExtractor_ExtractEmpty(t *testing.T) {
	e := NewEnvExtractor()
	content := []byte(`# Only comments
# No actual vars
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/app",
		FilePath: ".env.example",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("got %d nodes, want 0 for comments-only file", len(result.Nodes))
	}
}
