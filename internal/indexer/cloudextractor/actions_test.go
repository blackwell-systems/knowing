package cloudextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func newActionsOpts(filePath string, content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "https://github.com/example/repo",
		FilePath: filePath,
		FileHash: types.NewHash([]byte("test-file-hash")),
		Content:  []byte(content),
	}
}

func TestActionsExtractor_CanHandle(t *testing.T) {
	e := &actionsExtractor{}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"yml workflow", ".github/workflows/ci.yml", true},
		{"yaml workflow", ".github/workflows/deploy.yaml", true},
		{"nested workflow", "repo/.github/workflows/test.yml", true},
		{"actions dir not workflows", ".github/actions/test.yml", false},
		{"plain ci file", "ci.yml", false},
		{"non-yaml file", ".github/workflows/README.md", false},
		{"yml without workflows", "workflows/ci.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.canHandle(tt.path, nil)
			if got != tt.want {
				t.Errorf("canHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestActionsExtractor_ExtractWorkflow(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: CI Pipeline
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hello
`
	opts := newActionsOpts(".github/workflows/ci.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// Should have a workflow node and a job node.
	var workflowNode *types.Node
	for i := range result.Nodes {
		if result.Nodes[i].Kind == "workflow" {
			workflowNode = &result.Nodes[i]
			break
		}
	}
	if workflowNode == nil {
		t.Fatal("expected a workflow node")
	}
	expectedQN := "https://github.com/example/repo://.github/workflows/ci.yml.workflow.CI Pipeline"
	if workflowNode.QualifiedName != expectedQN {
		t.Errorf("workflow QualifiedName = %q, want %q", workflowNode.QualifiedName, expectedQN)
	}
}

func TestActionsExtractor_ExtractJobs(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: CI
on: push
jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - run: echo lint
  test:
    runs-on: ubuntu-latest
    steps:
      - run: echo test
  deploy:
    runs-on: ubuntu-latest
    steps:
      - run: echo deploy
`
	opts := newActionsOpts(".github/workflows/ci.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	jobCount := 0
	for _, n := range result.Nodes {
		if n.Kind == "job" {
			jobCount++
		}
	}
	if jobCount != 3 {
		t.Errorf("expected 3 job nodes, got %d", jobCount)
	}
}

func TestActionsExtractor_ExtractJobNeeds(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo build
  test:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - run: echo test
  deploy:
    needs:
      - build
      - test
    runs-on: ubuntu-latest
    steps:
      - run: echo deploy
`
	opts := newActionsOpts(".github/workflows/ci.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// Count depends_on edges: test->build (1), deploy->build (1), deploy->test (1) = 3
	dependsOnCount := 0
	for _, edge := range result.Edges {
		if edge.EdgeType == "depends_on" {
			dependsOnCount++
		}
	}
	if dependsOnCount != 3 {
		t.Errorf("expected 3 depends_on edges, got %d", dependsOnCount)
	}
}

func TestActionsExtractor_ExtractStepActions(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: go build ./...
`
	opts := newActionsOpts(".github/workflows/ci.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// Should have action nodes for checkout and setup-go.
	actionCount := 0
	for _, n := range result.Nodes {
		if n.Kind == "action" {
			actionCount++
		}
	}
	if actionCount != 2 {
		t.Errorf("expected 2 action nodes, got %d", actionCount)
	}

	// Should have 2 references edges from job to each action.
	refCount := 0
	for _, edge := range result.Edges {
		if edge.EdgeType == "references" {
			refCount++
		}
	}
	if refCount != 2 {
		t.Errorf("expected 2 references edges, got %d", refCount)
	}
}

func TestActionsExtractor_DeduplicateActions(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go build
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go test
`
	opts := newActionsOpts(".github/workflows/ci.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// Only one action node for actions/checkout@v4, even though two jobs use it.
	actionCount := 0
	for _, n := range result.Nodes {
		if n.Kind == "action" {
			actionCount++
		}
	}
	if actionCount != 1 {
		t.Errorf("expected 1 deduplicated action node, got %d", actionCount)
	}

	// But two references edges (one from each job).
	refCount := 0
	for _, edge := range result.Edges {
		if edge.EdgeType == "references" {
			refCount++
		}
	}
	if refCount != 2 {
		t.Errorf("expected 2 references edges, got %d", refCount)
	}
}

func TestActionsExtractor_EmptyJobs(t *testing.T) {
	e := &actionsExtractor{}
	content := `
name: Empty Workflow
on: push
`
	opts := newActionsOpts(".github/workflows/empty.yml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// Should have only the workflow node and no job nodes.
	if len(result.Nodes) != 1 {
		t.Errorf("expected 1 node (workflow only), got %d", len(result.Nodes))
	}
	if result.Nodes[0].Kind != "workflow" {
		t.Errorf("expected workflow node, got %q", result.Nodes[0].Kind)
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}
