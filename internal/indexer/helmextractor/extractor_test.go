package helmextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestHelmExtractor_CanHandle(t *testing.T) {
	e := NewHelmExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{"Chart.yaml", true},
		{"chart.yaml", true},
		{"values.yaml", true},
		{"charts/myapp/Chart.yaml", true},
		{"charts/myapp/templates/deployment.yaml", true},
		{"templates/service.yml", true},
		// Negative cases
		{"main.go", false},
		{"config.yaml", false},
		{"src/app.yaml", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestHelmExtractor_ExtractChart(t *testing.T) {
	e := NewHelmExtractor()
	content := []byte(`apiVersion: v2
name: myapp
version: 1.2.3
dependencies:
  - name: redis
    version: "17.0.0"
    repository: "https://charts.bitnami.com/bitnami"
  - name: postgresql
    version: "12.1.0"
    repository: "https://charts.bitnami.com/bitnami"
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/chart",
		FilePath: "Chart.yaml",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// myapp + redis + postgresql = 3 type nodes
	if len(result.Nodes) != 3 {
		t.Errorf("got %d nodes, want 3", len(result.Nodes))
		for _, n := range result.Nodes {
			t.Logf("  node: %s", n.QualifiedName)
		}
	}

	// myapp->redis, myapp->postgresql = 2 depends_on edges
	if len(result.Edges) != 2 {
		t.Errorf("got %d edges, want 2", len(result.Edges))
	}
}

func TestHelmExtractor_ExtractValues(t *testing.T) {
	e := NewHelmExtractor()
	content := []byte(`replicaCount: 3
image:
  repository: myapp
  tag: latest
service:
  type: ClusterIP
  port: 80
ingress:
  enabled: false
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/chart",
		FilePath: "values.yaml",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Top-level keys: replicaCount, image, service, ingress = 4 var nodes
	if len(result.Nodes) != 4 {
		t.Errorf("got %d nodes, want 4", len(result.Nodes))
		for _, n := range result.Nodes {
			t.Logf("  node: %s", n.QualifiedName)
		}
	}

	for _, n := range result.Nodes {
		if n.Kind != "var" {
			t.Errorf("node %q has kind %q, want var", n.QualifiedName, n.Kind)
		}
	}
}
