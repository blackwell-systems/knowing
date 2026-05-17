package cloudextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

const testRepoURL = "github.com/example/myservice"
const testFilePath = "serverless.yml"

func makeTestOpts(content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  testRepoURL,
		FilePath: testFilePath,
		Content:  []byte(content),
	}
}

// --- canHandle tests ---

func TestServerlessExtractor_CanHandle(t *testing.T) {
	e := &serverlessExtractor{}

	validYML := []byte("service: myapp\nprovider:\n  name: aws\n")
	validYAML := []byte("service: myapp\nprovider:\n  name: aws\n")
	noProvider := []byte("service: myapp\nfunctions: {}\n")
	noService := []byte("provider:\n  name: aws\n")
	randomYAML := []byte("apiVersion: apps/v1\nkind: Deployment\n")

	tests := []struct {
		name   string
		path   string
		content []byte
		want   bool
	}{
		{"serverless.yml with service+provider", "serverless.yml", validYML, true},
		{"serverless.yaml with service+provider", "serverless.yaml", validYAML, true},
		{"nested path serverless.yml", "services/api/serverless.yml", validYML, true},
		{"missing provider", "serverless.yml", noProvider, false},
		{"missing service", "serverless.yml", noService, false},
		{"non-serverless yaml", "deployment.yaml", randomYAML, false},
		{"non-yaml file", "serverless.json", validYML, false},
		{"plain yaml without service or provider", "serverless.yml", randomYAML, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.canHandle(tt.path, tt.content)
			if got != tt.want {
				t.Errorf("canHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// --- extract tests ---

func TestServerlessExtractor_ExtractFunctions(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  hello:
    handler: handler.hello
  goodbye:
    handler: handler.goodbye
  process:
    handler: handler.process
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	funcNodes := filterNodesByKind(result.Nodes, "function")
	if len(funcNodes) != 3 {
		t.Fatalf("expected 3 function nodes, got %d", len(funcNodes))
	}

	names := make(map[string]bool)
	for _, n := range funcNodes {
		names[n.QualifiedName] = true
	}

	for _, fn := range []string{"hello", "goodbye", "process"} {
		expected := buildQN(testRepoURL, testFilePath, "function", fn)
		if !names[expected] {
			t.Errorf("missing function node with QN %q", expected)
		}
	}
}

func TestServerlessExtractor_ExtractHttpRoutes(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  getUsers:
    handler: handler.getUsers
    events:
      - http:
          path: /users
          method: get
  createUser:
    handler: handler.createUser
    events:
      - http:
          path: /users
          method: post
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	routeNodes := filterNodesByKind(result.Nodes, "route")
	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route nodes, got %d", len(routeNodes))
	}

	routeEdges := filterEdgesByType(result.Edges, "handles_route")
	if len(routeEdges) != 2 {
		t.Fatalf("expected 2 handles_route edges, got %d", len(routeEdges))
	}

	// Verify route names are uppercase method
	routeNames := make(map[string]bool)
	for _, n := range routeNodes {
		routeNames[n.QualifiedName] = true
	}
	expectGET := buildQN(testRepoURL, testFilePath, "route", "GET /users")
	expectPOST := buildQN(testRepoURL, testFilePath, "route", "POST /users")
	if !routeNames[expectGET] {
		t.Errorf("missing route node %q", expectGET)
	}
	if !routeNames[expectPOST] {
		t.Errorf("missing route node %q", expectPOST)
	}
}

func TestServerlessExtractor_ExtractHttpApiRoutes(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  listItems:
    handler: handler.listItems
    events:
      - httpApi:
          path: /items
          method: get
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	routeNodes := filterNodesByKind(result.Nodes, "route")
	if len(routeNodes) != 1 {
		t.Fatalf("expected 1 route node, got %d", len(routeNodes))
	}

	expected := buildQN(testRepoURL, testFilePath, "route", "GET /items")
	if routeNodes[0].QualifiedName != expected {
		t.Errorf("route QN = %q, want %q", routeNodes[0].QualifiedName, expected)
	}

	routeEdges := filterEdgesByType(result.Edges, "handles_route")
	if len(routeEdges) != 1 {
		t.Fatalf("expected 1 handles_route edge, got %d", len(routeEdges))
	}
}

func TestServerlessExtractor_ExtractSQSSubscription(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  processQueue:
    handler: handler.processQueue
    events:
      - sqs:
          arn: arn:aws:sqs:us-east-1:123456789:my-queue
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	srcNodes := filterNodesByKind(result.Nodes, "event_source")
	if len(srcNodes) != 1 {
		t.Fatalf("expected 1 event_source node, got %d", len(srcNodes))
	}

	expected := buildQN(testRepoURL, testFilePath, "event_source", "sqs:my-queue")
	if srcNodes[0].QualifiedName != expected {
		t.Errorf("event_source QN = %q, want %q", srcNodes[0].QualifiedName, expected)
	}

	subEdges := filterEdgesByType(result.Edges, "subscribes")
	if len(subEdges) != 1 {
		t.Fatalf("expected 1 subscribes edge, got %d", len(subEdges))
	}
}

func TestServerlessExtractor_ExtractSNSSubscription(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  handleNotification:
    handler: handler.handleNotification
    events:
      - sns:
          arn: arn:aws:sns:us-east-1:123456789:my-topic
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	srcNodes := filterNodesByKind(result.Nodes, "event_source")
	if len(srcNodes) != 1 {
		t.Fatalf("expected 1 event_source node, got %d", len(srcNodes))
	}

	expected := buildQN(testRepoURL, testFilePath, "event_source", "sns:my-topic")
	if srcNodes[0].QualifiedName != expected {
		t.Errorf("event_source QN = %q, want %q", srcNodes[0].QualifiedName, expected)
	}

	subEdges := filterEdgesByType(result.Edges, "subscribes")
	if len(subEdges) != 1 {
		t.Fatalf("expected 1 subscribes edge, got %d", len(subEdges))
	}
}

func TestServerlessExtractor_ExtractSchedule(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  cron:
    handler: handler.cron
    events:
      - schedule: rate(1 hour)
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	srcNodes := filterNodesByKind(result.Nodes, "event_source")
	if len(srcNodes) != 1 {
		t.Fatalf("expected 1 event_source node, got %d", len(srcNodes))
	}

	expected := buildQN(testRepoURL, testFilePath, "event_source", "schedule:rate(1 hour)")
	if srcNodes[0].QualifiedName != expected {
		t.Errorf("event_source QN = %q, want %q", srcNodes[0].QualifiedName, expected)
	}

	subEdges := filterEdgesByType(result.Edges, "subscribes")
	if len(subEdges) != 1 {
		t.Fatalf("expected 1 subscribes edge, got %d", len(subEdges))
	}
}

func TestServerlessExtractor_ExtractS3Event(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions:
  processUpload:
    handler: handler.processUpload
    events:
      - s3:
          bucket: my-uploads-bucket
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	srcNodes := filterNodesByKind(result.Nodes, "event_source")
	if len(srcNodes) != 1 {
		t.Fatalf("expected 1 event_source node, got %d", len(srcNodes))
	}

	expected := buildQN(testRepoURL, testFilePath, "event_source", "s3:my-uploads-bucket")
	if srcNodes[0].QualifiedName != expected {
		t.Errorf("event_source QN = %q, want %q", srcNodes[0].QualifiedName, expected)
	}

	subEdges := filterEdgesByType(result.Edges, "subscribes")
	if len(subEdges) != 1 {
		t.Fatalf("expected 1 subscribes edge, got %d", len(subEdges))
	}
}

func TestServerlessExtractor_EmptyFunctions(t *testing.T) {
	e := &serverlessExtractor{}

	content := `
service: myapp
provider:
  name: aws
functions: {}
`
	opts := makeTestOpts(content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("extract returned error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

// --- helpers ---

func filterNodesByKind(nodes []types.Node, kind string) []types.Node {
	var out []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func filterEdgesByType(edges []types.Edge, edgeType string) []types.Edge {
	var out []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			out = append(out, e)
		}
	}
	return out
}
