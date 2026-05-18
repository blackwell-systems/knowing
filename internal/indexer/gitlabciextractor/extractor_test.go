package gitlabciextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestGitLabCIExtractor_CanHandle(t *testing.T) {
	e := NewGitLabCIExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{".gitlab-ci.yml", true},
		{"ci/.gitlab-ci.yml", true},
		// Negative cases
		{".github/workflows/ci.yml", false},
		{"gitlab-ci.yml", false},
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

func TestGitLabCIExtractor_Extract(t *testing.T) {
	e := NewGitLabCIExtractor()
	content := []byte(`stages:
  - build
  - test
  - deploy

.base_template:
  image: golang:1.21
  before_script:
    - go mod download

build_app:
  stage: build
  image: golang:1.21
  extends: .base_template
  script:
    - go build ./...

unit_tests:
  stage: test
  needs: [build_app]
  script:
    - go test ./...

deploy_staging:
  stage: deploy
  needs:
    - job: build_app
    - job: unit_tests
  image: alpine:3.18
  script:
    - ./deploy.sh
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: ".gitlab-ci.yml",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Stages: build, test, deploy = 3 action nodes
	// Jobs: .base_template, build_app, unit_tests, deploy_staging = 4 job nodes
	// Images: golang:1.21, alpine:3.18 = 2 image nodes (golang appears in two jobs but same hash)
	nodesByKind := make(map[string]int)
	for _, n := range result.Nodes {
		nodesByKind[n.Kind]++
	}
	if nodesByKind["action"] < 3 {
		t.Errorf("expected at least 3 action nodes (stages), got %d", nodesByKind["action"])
	}
	if nodesByKind["job"] < 4 {
		t.Errorf("expected at least 4 job nodes, got %d", nodesByKind["job"])
	}

	// Edges: build_app->build (stage), build_app->.base_template (extends),
	// unit_tests->test (stage), unit_tests->build_app (needs),
	// deploy_staging->deploy (stage), deploy_staging->build_app (needs),
	// deploy_staging->unit_tests (needs), build_app->golang:1.21 (image),
	// deploy_staging->alpine:3.18 (image)
	edgesByType := make(map[string]int)
	for _, e := range result.Edges {
		edgesByType[e.EdgeType]++
	}

	if edgesByType["depends_on"] < 5 {
		t.Errorf("expected at least 5 depends_on edges, got %d", edgesByType["depends_on"])
	}
	if edgesByType["extends"] < 1 {
		t.Errorf("expected at least 1 extends edge, got %d", edgesByType["extends"])
	}
}

func TestGitLabCIExtractor_ExtractSimple(t *testing.T) {
	e := NewGitLabCIExtractor()
	content := []byte(`test:
  script:
    - echo "hello"

build:
  script:
    - echo "building"
  needs: [test]
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: ".gitlab-ci.yml",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// 2 job nodes
	if len(result.Nodes) < 2 {
		t.Errorf("got %d nodes, want at least 2", len(result.Nodes))
	}

	// build->test (needs) = 1 depends_on edge
	if len(result.Edges) < 1 {
		t.Errorf("got %d edges, want at least 1", len(result.Edges))
	}
}
