package cloudextractor

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeDeployTestOpts() types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "https://github.com/example/repo",
		FilePath: ".github/workflows/deploy.yml",
		FileHash: types.NewHash([]byte("test-file-hash")),
	}
}

func makeTestJobDefs(jobs map[string][]stepDef) map[string]jobDef {
	defs := make(map[string]jobDef)
	for id, steps := range jobs {
		defs[id] = jobDef{
			Name:  id,
			Hash:  types.ComputeNodeHash("https://github.com/example/repo", ".github/workflows/deploy.yml", types.EmptyHash, id, "job"),
			Steps: steps,
		}
	}
	return defs
}

func TestExtractDeployedByEdges_DockerPush(t *testing.T) {
	opts := makeDeployTestOpts()
	workflowHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "deploy", "workflow")

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"deploy": {
			{Run: "docker push myimage:latest"},
		},
	})

	nodes, edges := extractDeployedByEdges(opts, workflowHash, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 deployment target node, got %d", len(nodes))
	}
	if nodes[0].Kind != "deployment_target" {
		t.Errorf("expected kind 'deployment_target', got %q", nodes[0].Kind)
	}

	if len(edges) != 1 {
		t.Fatalf("expected 1 deployed_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "deployed_by" {
		t.Errorf("expected edge type 'deployed_by', got %q", edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.9 {
		t.Errorf("expected confidence 0.9, got %f", edges[0].Confidence)
	}
	if edges[0].TargetHash != workflowHash {
		t.Error("deployed_by edge target should be the workflow hash")
	}
}

func TestExtractDeployedByEdges_KubectlApply(t *testing.T) {
	opts := makeDeployTestOpts()
	workflowHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "deploy", "workflow")

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"deploy": {
			{Run: "kubectl apply -f deploy.yaml"},
		},
	})

	nodes, edges := extractDeployedByEdges(opts, workflowHash, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 deployment target node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 deployed_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "deployed_by" {
		t.Errorf("expected edge type 'deployed_by', got %q", edges[0].EdgeType)
	}

	// The target should be "deploy.yaml" (extracted from the -f flag).
	expectedTarget := "deploy.yaml"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "deployment_target")
	if nodes[0].NodeHash != expectedHash {
		t.Error("deployment target node hash does not match expected for 'deploy.yaml'")
	}
}

func TestExtractDeployedByEdges_DeployAction(t *testing.T) {
	opts := makeDeployTestOpts()
	workflowHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "deploy", "workflow")

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"deploy": {
			{
				Uses: "aws-actions/amazon-ecs-deploy-task-definition@v1",
				With: map[string]string{
					"service":         "my-service",
					"task-definition": "task-def.json",
				},
			},
		},
	})

	nodes, edges := extractDeployedByEdges(opts, workflowHash, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 deployment target node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 deployed_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "deployed_by" {
		t.Errorf("expected edge type 'deployed_by', got %q", edges[0].EdgeType)
	}

	// The target should be "my-service" (from with.service).
	expectedTarget := "my-service"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "deployment_target")
	if nodes[0].NodeHash != expectedHash {
		t.Error("deployment target node hash does not match expected for 'my-service'")
	}
}

func TestExtractDeployedByEdges_NoDeployment(t *testing.T) {
	opts := makeDeployTestOpts()
	workflowHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "ci", "workflow")

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"build": {
			{Run: "go build ./..."},
		},
		"lint": {
			{Run: "golangci-lint run"},
		},
	})

	nodes, edges := extractDeployedByEdges(opts, workflowHash, jobDefs)

	if len(nodes) != 0 {
		t.Errorf("expected 0 deployment target nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 deployed_by edges, got %d", len(edges))
	}
}

func TestExtractDeployedByEdges_HelmUpgrade(t *testing.T) {
	opts := makeDeployTestOpts()
	workflowHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "deploy", "workflow")

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"deploy": {
			{Run: "helm upgrade my-release ./chart"},
		},
	})

	nodes, edges := extractDeployedByEdges(opts, workflowHash, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 deployment target node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 deployed_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "deployed_by" {
		t.Errorf("expected edge type 'deployed_by', got %q", edges[0].EdgeType)
	}

	// The target should be "my-release" (helm upgrade release_name).
	expectedTarget := "my-release"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "deployment_target")
	if nodes[0].NodeHash != expectedHash {
		t.Error("deployment target node hash does not match expected for 'my-release'")
	}
}

func TestExtractTestedByEdges_GoTest(t *testing.T) {
	opts := makeDeployTestOpts()

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"test": {
			{Run: "go test ./internal/store/..."},
		},
	})

	nodes, edges := extractTestedByEdges(opts, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 test target node, got %d", len(nodes))
	}
	if nodes[0].Kind != "test_target" {
		t.Errorf("expected kind 'test_target', got %q", nodes[0].Kind)
	}

	if len(edges) != 1 {
		t.Fatalf("expected 1 tested_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "tested_by" {
		t.Errorf("expected edge type 'tested_by', got %q", edges[0].EdgeType)
	}
	if edges[0].Confidence != 0.8 {
		t.Errorf("expected confidence 0.8, got %f", edges[0].Confidence)
	}

	// The test target should be "internal/store" (normalized from ./internal/store/...).
	expectedTarget := "internal/store"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "test_target")
	if nodes[0].NodeHash != expectedHash {
		t.Errorf("test target node hash does not match expected for %q", expectedTarget)
	}
}

func TestExtractTestedByEdges_NpmTest(t *testing.T) {
	opts := makeDeployTestOpts()

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"test": {
			{Run: "npm test"},
		},
	})

	nodes, edges := extractTestedByEdges(opts, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 test target node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 tested_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "tested_by" {
		t.Errorf("expected edge type 'tested_by', got %q", edges[0].EdgeType)
	}

	// The test target should be "npm:test".
	expectedTarget := "npm:test"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "test_target")
	if nodes[0].NodeHash != expectedHash {
		t.Errorf("test target node hash does not match expected for %q", expectedTarget)
	}
}

func TestExtractTestedByEdges_MultiplePackages(t *testing.T) {
	opts := makeDeployTestOpts()

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"test": {
			{Run: "go test ./pkg/auth/ ./pkg/user/"},
		},
	})

	nodes, edges := extractTestedByEdges(opts, jobDefs)

	if len(nodes) != 2 {
		t.Fatalf("expected 2 test target nodes, got %d", len(nodes))
	}
	if len(edges) != 2 {
		t.Fatalf("expected 2 tested_by edges, got %d", len(edges))
	}

	// Both edges should be tested_by.
	for _, edge := range edges {
		if edge.EdgeType != "tested_by" {
			t.Errorf("expected edge type 'tested_by', got %q", edge.EdgeType)
		}
	}
}

func TestExtractTestedByEdges_NoTests(t *testing.T) {
	opts := makeDeployTestOpts()

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"build": {
			{Run: "go build ./..."},
		},
		"lint": {
			{Uses: "golangci/golangci-lint-action@v3"},
		},
	})

	nodes, edges := extractTestedByEdges(opts, jobDefs)

	if len(nodes) != 0 {
		t.Errorf("expected 0 test target nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 tested_by edges, got %d", len(edges))
	}
}

func TestExtractTestedByEdges_Pytest(t *testing.T) {
	opts := makeDeployTestOpts()

	jobDefs := makeTestJobDefs(map[string][]stepDef{
		"test": {
			{Run: "pytest tests/"},
		},
	})

	nodes, edges := extractTestedByEdges(opts, jobDefs)

	if len(nodes) != 1 {
		t.Fatalf("expected 1 test target node, got %d", len(nodes))
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 tested_by edge, got %d", len(edges))
	}
	if edges[0].EdgeType != "tested_by" {
		t.Errorf("expected edge type 'tested_by', got %q", edges[0].EdgeType)
	}

	// The test target should be "pytest".
	expectedTarget := "pytest"
	expectedHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, expectedTarget, "test_target")
	if nodes[0].NodeHash != expectedHash {
		t.Errorf("test target node hash does not match expected for %q", expectedTarget)
	}
}
