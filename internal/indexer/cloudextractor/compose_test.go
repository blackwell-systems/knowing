package cloudextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func newComposeOpts(filename string, content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "github.com/test/repo",
		FilePath: filename,
		FileHash: types.Hash{1, 2, 3},
		Content:  []byte(content),
	}
}

func TestComposeExtractor_CanHandle(t *testing.T) {
	e := &composeExtractor{}

	tests := []struct {
		name    string
		path    string
		content string
		want    bool
	}{
		{
			name:    "docker-compose.yml",
			path:    "docker-compose.yml",
			content: "services:\n  web:\n    image: nginx\n",
			want:    true,
		},
		{
			name:    "docker-compose.yaml",
			path:    "docker-compose.yaml",
			content: "services:\n  web:\n    image: nginx\n",
			want:    true,
		},
		{
			name:    "compose.yaml with services and version",
			path:    "compose.yaml",
			content: "version: '3'\nservices:\n  web:\n    image: nginx\n",
			want:    true,
		},
		{
			name:    "compose.yml with services and name",
			path:    "compose.yml",
			content: "name: myproject\nservices:\n  web:\n    image: nginx\n",
			want:    true,
		},
		{
			name:    "plain YAML without services",
			path:    "config.yaml",
			content: "foo: bar\nbaz: 1\n",
			want:    false,
		},
		{
			name:    "services key without version or name",
			path:    "app.yaml",
			content: "services:\n  web:\n    image: nginx\n",
			want:    false,
		},
		{
			name:    "non-YAML file",
			path:    "Dockerfile",
			content: "FROM nginx\n",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.canHandle(tt.path, []byte(tt.content))
			if got != tt.want {
				t.Errorf("canHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestComposeExtractor_ExtractServices(t *testing.T) {
	e := &composeExtractor{}
	yaml := `
services:
  web:
    image: nginx
  api:
    image: node
  db:
    image: postgres
`
	opts := newComposeOpts("docker-compose.yml", yaml)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result.Nodes))
	}

	// Verify all are service nodes
	names := make(map[string]bool)
	for _, n := range result.Nodes {
		if n.Kind != "service" {
			t.Errorf("expected kind=service, got %q", n.Kind)
		}
		names[n.QualifiedName] = true
	}

	for _, svc := range []string{"web", "api", "db"} {
		qn := buildQN(opts.RepoURL, opts.FilePath, "service", svc)
		if !names[qn] {
			t.Errorf("missing service node %q", svc)
		}
	}
}

func TestComposeExtractor_ExtractDependsOn(t *testing.T) {
	e := &composeExtractor{}

	t.Run("list form", func(t *testing.T) {
		yaml := `
services:
  web:
    image: nginx
    depends_on:
      - api
      - db
  api:
    image: node
  db:
    image: postgres
`
		opts := newComposeOpts("docker-compose.yml", yaml)
		result, err := e.extract(context.Background(), opts)
		if err != nil {
			t.Fatalf("extract error: %v", err)
		}

		dependsOnEdges := composeFilterEdges(result.Edges, "depends_on")
		if len(dependsOnEdges) != 2 {
			t.Fatalf("expected 2 depends_on edges, got %d", len(dependsOnEdges))
		}
	})

	t.Run("map form", func(t *testing.T) {
		yaml := `
services:
  web:
    image: nginx
    depends_on:
      api:
        condition: service_healthy
      db:
        condition: service_started
  api:
    image: node
  db:
    image: postgres
`
		opts := newComposeOpts("docker-compose.yml", yaml)
		result, err := e.extract(context.Background(), opts)
		if err != nil {
			t.Fatalf("extract error: %v", err)
		}

		dependsOnEdges := composeFilterEdges(result.Edges, "depends_on")
		if len(dependsOnEdges) != 2 {
			t.Fatalf("expected 2 depends_on edges, got %d", len(dependsOnEdges))
		}
	})
}

func TestComposeExtractor_ExtractPorts(t *testing.T) {
	e := &composeExtractor{}
	yaml := `
services:
  web:
    image: nginx
    ports:
      - "8080:80"
      - "8443:443"
  api:
    image: node
    ports:
      - "3000:3000"
`
	opts := newComposeOpts("docker-compose.yml", yaml)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	// 2 services + 3 port nodes = 5 total
	portNodes := filterNodes(result.Nodes, "port")
	if len(portNodes) != 3 {
		t.Fatalf("expected 3 port nodes, got %d", len(portNodes))
	}

	exposesEdges := composeFilterEdges(result.Edges, "exposes")
	if len(exposesEdges) != 3 {
		t.Fatalf("expected 3 exposes edges, got %d", len(exposesEdges))
	}
}

func TestComposeExtractor_ExtractNetworks(t *testing.T) {
	e := &composeExtractor{}
	yaml := `
services:
  web:
    image: nginx
    networks:
      - frontend
      - backend
  api:
    image: node
    networks:
      - backend
`
	opts := newComposeOpts("docker-compose.yml", yaml)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	networkNodes := filterNodes(result.Nodes, "network")
	if len(networkNodes) != 2 {
		t.Fatalf("expected 2 network nodes (deduplicated), got %d", len(networkNodes))
	}

	connectsToEdges := composeFilterEdges(result.Edges, "connects_to")
	if len(connectsToEdges) != 3 {
		t.Fatalf("expected 3 connects_to edges, got %d", len(connectsToEdges))
	}
}

func TestComposeExtractor_ExtractLinks(t *testing.T) {
	e := &composeExtractor{}
	yaml := `
services:
  web:
    image: nginx
    links:
      - api
      - "db:database"
  api:
    image: node
  db:
    image: postgres
`
	opts := newComposeOpts("docker-compose.yml", yaml)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	connectsToEdges := composeFilterEdges(result.Edges, "connects_to")
	if len(connectsToEdges) != 2 {
		t.Fatalf("expected 2 connects_to edges, got %d", len(connectsToEdges))
	}
}

func TestComposeExtractor_EmptyServices(t *testing.T) {
	e := &composeExtractor{}
	yaml := `
services:
`
	opts := newComposeOpts("docker-compose.yml", yaml)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

// filterEdges returns edges matching the given type.
func composeFilterEdges(edges []types.Edge, edgeType string) []types.Edge {
	var filtered []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

// filterNodes returns nodes matching the given kind.
func filterNodes(nodes []types.Node, kind string) []types.Node {
	var filtered []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			filtered = append(filtered, n)
		}
	}
	return filtered
}
