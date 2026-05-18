package packagejsonextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestPackageJSONExtractor_CanHandle(t *testing.T) {
	e := NewPackageJSONExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{"package.json", true},
		{"src/package.json", true},
		{"packages/core/package.json", true},
		// Negative cases
		{"package-lock.json", false},
		{"composer.json", false},
		{"main.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPackageJSONExtractor_Extract(t *testing.T) {
	e := NewPackageJSONExtractor()
	content := []byte(`{
  "name": "@org/my-app",
  "version": "1.0.0",
  "scripts": {
    "build": "tsc",
    "test": "jest",
    "start": "node dist/index.js"
  },
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "typescript": "^5.0.0",
    "jest": "^29.0.0"
  }
}`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/app",
		FilePath: "package.json",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Nodes: @org/my-app (type) + build, test, start (actions) + express, lodash,
	// typescript, jest (types for deps) = 8 nodes
	nodesByKind := make(map[string]int)
	for _, n := range result.Nodes {
		nodesByKind[n.Kind]++
	}

	if nodesByKind["type"] < 5 {
		t.Errorf("expected at least 5 type nodes (package + 4 deps), got %d", nodesByKind["type"])
	}
	if nodesByKind["action"] < 3 {
		t.Errorf("expected at least 3 action nodes (scripts), got %d", nodesByKind["action"])
	}

	// 4 depends_on edges (express, lodash, typescript, jest)
	if len(result.Edges) != 4 {
		t.Errorf("got %d edges, want 4", len(result.Edges))
	}
	for _, edge := range result.Edges {
		if edge.EdgeType != "depends_on" {
			t.Errorf("edge type = %q, want depends_on", edge.EdgeType)
		}
	}
}

func TestPackageJSONExtractor_ExtractMinimal(t *testing.T) {
	e := NewPackageJSONExtractor()
	content := []byte(`{
  "name": "simple-lib",
  "version": "0.1.0"
}`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/lib",
		FilePath: "package.json",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Just the package name node.
	if len(result.Nodes) != 1 {
		t.Errorf("got %d nodes, want 1", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("got %d edges, want 0", len(result.Edges))
	}
}
