package makefileextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestMakefileExtractor_CanHandle(t *testing.T) {
	e := NewMakefileExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{"Makefile", true},
		{"makefile", true},
		{"build.mk", true},
		{"scripts/common.mk", true},
		// Negative cases
		{"main.go", false},
		{"Makefile.bak.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestMakefileExtractor_ExtractTargets(t *testing.T) {
	e := NewMakefileExtractor()
	content := []byte(`.PHONY: build test clean

build: deps
	go build ./...

test: build
	go test ./...

deps:
	go mod download

clean:
	rm -rf bin/

include common.mk
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "Makefile",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Targets: build, test, deps, clean + Makefile (file node for include) + common.mk
	if len(result.Nodes) < 5 {
		t.Errorf("got %d nodes, want at least 5", len(result.Nodes))
		for _, n := range result.Nodes {
			t.Logf("  node: %s (kind=%s)", n.QualifiedName, n.Kind)
		}
	}

	// Edges: build->deps, test->build, file->common.mk (imports)
	edgesByType := make(map[string]int)
	for _, e := range result.Edges {
		edgesByType[e.EdgeType]++
	}

	if edgesByType["depends_on"] < 2 {
		t.Errorf("expected at least 2 depends_on edges, got %d", edgesByType["depends_on"])
	}
	if edgesByType["imports"] < 1 {
		t.Errorf("expected at least 1 imports edge, got %d", edgesByType["imports"])
	}
}

func TestMakefileExtractor_ExtractSimple(t *testing.T) {
	e := NewMakefileExtractor()
	content := []byte(`all: build test

build:
	echo "building"

test:
	echo "testing"
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "Makefile",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// all, build, test = 3 targets
	if len(result.Nodes) < 3 {
		t.Errorf("got %d nodes, want at least 3", len(result.Nodes))
	}

	// all->build, all->test = 2 depends_on edges
	if len(result.Edges) < 2 {
		t.Errorf("got %d edges, want at least 2", len(result.Edges))
	}
}
