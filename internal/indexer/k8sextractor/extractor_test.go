package k8sextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestK8sExtractor_Name(t *testing.T) {
	e := NewK8sExtractor()
	if got := e.Name(); got != "k8s-yaml" {
		t.Errorf("Name() = %q, want %q", got, "k8s-yaml")
	}
}

func TestK8sExtractor_CanHandle(t *testing.T) {
	e := NewK8sExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"k8s/deployment.yaml", true},
		{"kubernetes/service.yml", true},
		{"manifests/app.yaml", true},
		{"deploy/ingress.yaml", true},
		{"helm/templates/configmap.yaml", true},
		{"infra/k8s/pod.yml", true},
		{"src/kubernetes/deploy.yaml", true},
		// Negative cases
		{"config.yaml", false},
		{"src/config.yml", false},
		{"main.go", false},
		{"docs/readme.md", false},
		{"terraform/main.tf", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestK8sExtractor_ExtractDeployment(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: production
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/deployment.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result.Nodes))
	}
	node := result.Nodes[0]
	if node.Kind != "deployment" {
		t.Errorf("node.Kind = %q, want %q", node.Kind, "deployment")
	}
	wantQN := "github.com/org/repo://k8s/deployment.yaml.deployment.production/nginx"
	if node.QualifiedName != wantQN {
		t.Errorf("node.QualifiedName = %q, want %q", node.QualifiedName, wantQN)
	}
}

func TestK8sExtractor_ExtractService(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: default
spec:
  selector:
    app: nginx
  ports:
  - port: 80
    targetPort: 8080
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/service.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result.Nodes))
	}
	if result.Nodes[0].Kind != "service" {
		t.Errorf("Kind = %q, want %q", result.Nodes[0].Kind, "service")
	}
}

func TestK8sExtractor_ExtractConfigMap(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  DATABASE_URL: postgres://localhost:5432/app
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/configmap.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result.Nodes))
	}
	if result.Nodes[0].Kind != "configmap" {
		t.Errorf("Kind = %q, want %q", result.Nodes[0].Kind, "configmap")
	}
}

func TestK8sExtractor_ExtractIngress(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: app-ingress
  namespace: default
spec:
  rules:
  - host: app.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: my-service
            port:
              number: 80
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/ingress.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(result.Nodes))
	}
	if result.Nodes[0].Kind != "ingress" {
		t.Errorf("Kind = %q, want %q", result.Nodes[0].Kind, "ingress")
	}
}

func TestK8sExtractor_ExtractDeploysEdge(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
  labels:
    app: nginx
spec:
  selector:
    matchLabels:
      app: nginx
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.21
---
apiVersion: v1
kind: Service
metadata:
  name: nginx-svc
  namespace: default
spec:
  selector:
    app: nginx
  ports:
  - port: 80
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/app.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(result.Nodes))
	}
	// Should have a "deploys" edge from service to deployment
	var deploysEdge *types.Edge
	for i := range result.Edges {
		if result.Edges[i].EdgeType == "deploys" {
			deploysEdge = &result.Edges[i]
			break
		}
	}
	if deploysEdge == nil {
		t.Fatal("expected a 'deploys' edge, got none")
	}
	if deploysEdge.Confidence != 0.7 {
		t.Errorf("edge confidence = %v, want 0.7", deploysEdge.Confidence)
	}
	if deploysEdge.Provenance != "ast_inferred" {
		t.Errorf("edge provenance = %q, want %q", deploysEdge.Provenance, "ast_inferred")
	}
}

func TestK8sExtractor_ExtractExposesEdge(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: v1
kind: Service
metadata:
  name: web-svc
  namespace: default
spec:
  selector:
    app: web
  ports:
  - port: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web-ingress
  namespace: default
spec:
  rules:
  - host: web.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: web-svc
            port:
              number: 80
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/ingress.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(result.Nodes))
	}
	var exposesEdge *types.Edge
	for i := range result.Edges {
		if result.Edges[i].EdgeType == "exposes" {
			exposesEdge = &result.Edges[i]
			break
		}
	}
	if exposesEdge == nil {
		t.Fatal("expected an 'exposes' edge, got none")
	}
}

func TestK8sExtractor_ExtractConfiguresEdge(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  KEY: value
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app
  namespace: default
  labels:
    app: myapp
spec:
  selector:
    matchLabels:
      app: myapp
  template:
    spec:
      containers:
      - name: app
        image: app:latest
        envFrom:
        - configMapRef:
            name: app-config
      volumes:
      - name: config-vol
        configMap:
          name: app-config
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/app.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(result.Nodes))
	}
	var configuresEdge *types.Edge
	for i := range result.Edges {
		if result.Edges[i].EdgeType == "configures" {
			configuresEdge = &result.Edges[i]
			break
		}
	}
	if configuresEdge == nil {
		t.Fatal("expected a 'configures' edge, got none")
	}
}

func TestK8sExtractor_MultiDocument(t *testing.T) {
	e := NewK8sExtractor()
	content := []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: config1
  namespace: default
data:
  key: value
---
apiVersion: v1
kind: Service
metadata:
  name: svc1
  namespace: default
spec:
  ports:
  - port: 80
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deploy1
  namespace: default
  labels:
    app: test
spec:
  selector:
    matchLabels:
      app: test
  template:
    spec:
      containers:
      - name: app
        image: app:latest
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/repo",
		FilePath: "k8s/multi.yaml",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(result.Nodes) != 3 {
		t.Fatalf("got %d nodes, want 3", len(result.Nodes))
	}

	kinds := make(map[string]bool)
	for _, n := range result.Nodes {
		kinds[n.Kind] = true
	}
	for _, expected := range []string{"configmap", "service", "deployment"} {
		if !kinds[expected] {
			t.Errorf("missing node kind %q", expected)
		}
	}
}
