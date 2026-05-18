package dockerfileextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestDockerfileExtractor_CanHandle(t *testing.T) {
	e := NewDockerfileExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{"Dockerfile", true},
		{"Dockerfile.dev", true},
		{"Dockerfile.production", true},
		{"build.dockerfile", true},
		{"app.Dockerfile", true},
		{"src/Dockerfile", true},
		// Negative cases
		{"main.go", false},
		{"docker-compose.yaml", false},
		{"README.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDockerfileExtractor_ExtractMultiStage(t *testing.T) {
	e := NewDockerfileExtractor()
	content := []byte(`FROM golang:1.21 AS builder
ARG VERSION=dev
ENV APP_NAME=myapp
RUN go build -o /app
EXPOSE 8080

FROM alpine:3.18 AS runner
COPY --from=builder /app /app
EXPOSE 9090
ENV LOG_LEVEL=info
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "Dockerfile",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Expect nodes: file, builder stage, golang:1.21 image, VERSION var, APP_NAME var,
	// 8080 port, runner stage, alpine:3.18 image, 9090 port, LOG_LEVEL var
	if len(result.Nodes) < 10 {
		t.Errorf("got %d nodes, want at least 10", len(result.Nodes))
	}

	// Check edges: builder->golang:1.21, runner->alpine:3.18, runner->builder (COPY --from)
	if len(result.Edges) < 3 {
		t.Errorf("got %d edges, want at least 3", len(result.Edges))
	}

	// Verify node kinds exist.
	kinds := make(map[string]int)
	for _, n := range result.Nodes {
		kinds[n.Kind]++
	}
	if kinds["image"] < 2 {
		t.Errorf("expected at least 2 image nodes, got %d", kinds["image"])
	}
	if kinds["port"] < 2 {
		t.Errorf("expected at least 2 port nodes, got %d", kinds["port"])
	}
	if kinds["var"] < 3 {
		t.Errorf("expected at least 3 var nodes, got %d", kinds["var"])
	}
}

func TestDockerfileExtractor_ExtractSimple(t *testing.T) {
	e := NewDockerfileExtractor()
	content := []byte(`FROM node:18
WORKDIR /app
COPY . .
RUN npm install
EXPOSE 3000
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/app",
		FilePath: "Dockerfile",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// file node + stage node + image node + port node = at least 4
	if len(result.Nodes) < 4 {
		t.Errorf("got %d nodes, want at least 4", len(result.Nodes))
	}

	// At least depends_on from stage to image.
	if len(result.Edges) < 1 {
		t.Errorf("got %d edges, want at least 1", len(result.Edges))
	}
}
