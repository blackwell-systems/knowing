package cloudextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeOpts(filePath string, content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:  "github.com/test/repo",
		RepoHash: types.NewHash([]byte("github.com/test/repo")),
		FilePath: filePath,
		FileHash: types.NewHash([]byte("filehash")),
		Content:  []byte(content),
	}
}

func TestCfnExtractor_CanHandle(t *testing.T) {
	e := &cfnExtractor{}

	tests := []struct {
		name    string
		path    string
		content string
		want    bool
	}{
		{
			name: "CFN template with AWSTemplateFormatVersion",
			path: "template.yaml",
			content: `AWSTemplateFormatVersion: "2010-09-09"
Resources:
  MyBucket:
    Type: AWS::S3::Bucket
`,
			want: true,
		},
		{
			name: "SAM template with Transform",
			path: "template.yml",
			content: `Transform: AWS::Serverless-2016-10-31
Resources:
  MyFunc:
    Type: AWS::Serverless::Function
`,
			want: true,
		},
		{
			name: "plain YAML without CFN markers",
			path: "config.yaml",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
`,
			want: false,
		},
		{
			name: "non-YAML file",
			path: "readme.md",
			content: `# Hello`,
			want: false,
		},
		{
			name: "JSON file with yaml-like content",
			path: "data.json",
			content: `{"AWSTemplateFormatVersion": "2010-09-09"}`,
			want: false,
		},
		{
			name: "YAML without any CFN markers",
			path: "docker-compose.yml",
			content: `version: "3"
services:
  web:
    image: nginx
`,
			want: false,
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

func TestCfnExtractor_ExtractResources(t *testing.T) {
	e := &cfnExtractor{}
	content := `AWSTemplateFormatVersion: "2010-09-09"
Resources:
  MyFunction:
    Type: AWS::Lambda::Function
    Properties:
      Runtime: python3.9
  MyTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: my-table
  MyBucket:
    Type: AWS::S3::Bucket
`
	opts := makeOpts("infra/template.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(result.Nodes))
	}

	// Check that all nodes have kind "resource"
	for _, n := range result.Nodes {
		if n.Kind != "resource" {
			t.Errorf("expected kind 'resource', got %q for %s", n.Kind, n.QualifiedName)
		}
	}

	// Check qualified names contain the logical IDs
	qnSet := make(map[string]bool)
	for _, n := range result.Nodes {
		qnSet[n.QualifiedName] = true
	}
	for _, id := range []string{"MyFunction", "MyTable", "MyBucket"} {
		expected := "github.com/test/repo://infra/template.yaml.resource." + id
		if !qnSet[expected] {
			t.Errorf("missing expected QualifiedName %q", expected)
		}
	}
}

func TestCfnExtractor_ExtractRefEdges(t *testing.T) {
	e := &cfnExtractor{}
	content := `AWSTemplateFormatVersion: "2010-09-09"
Resources:
  MyFunction:
    Type: AWS::Lambda::Function
    Properties:
      Environment:
        Variables:
          TABLE_NAME:
            Ref: MyTable
  MyTable:
    Type: AWS::DynamoDB::Table
`
	opts := makeOpts("template.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}

	// Should have a "references" edge from MyFunction to MyTable
	refEdges := filterEdges(result.Edges, "references")
	if len(refEdges) != 1 {
		t.Fatalf("expected 1 references edge, got %d", len(refEdges))
	}

	funcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "MyFunction", "resource")
	tableHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "MyTable", "resource")

	if refEdges[0].SourceHash != funcHash {
		t.Error("references edge source should be MyFunction")
	}
	if refEdges[0].TargetHash != tableHash {
		t.Error("references edge target should be MyTable")
	}
}

func TestCfnExtractor_ExtractGetAttEdges(t *testing.T) {
	e := &cfnExtractor{}
	content := `AWSTemplateFormatVersion: "2010-09-09"
Resources:
  MyFunction:
    Type: AWS::Lambda::Function
    Properties:
      Role:
        Fn::GetAtt:
          - MyRole
          - Arn
  MyRole:
    Type: AWS::IAM::Role
`
	opts := makeOpts("template.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	refEdges := filterEdges(result.Edges, "references")
	if len(refEdges) != 1 {
		t.Fatalf("expected 1 references edge from Fn::GetAtt, got %d", len(refEdges))
	}

	funcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "MyFunction", "resource")
	roleHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "MyRole", "resource")

	if refEdges[0].SourceHash != funcHash {
		t.Error("GetAtt edge source should be MyFunction")
	}
	if refEdges[0].TargetHash != roleHash {
		t.Error("GetAtt edge target should be MyRole")
	}
}

func TestCfnExtractor_ExtractDependsOn(t *testing.T) {
	e := &cfnExtractor{}
	content := `AWSTemplateFormatVersion: "2010-09-09"
Resources:
  MyFunction:
    Type: AWS::Lambda::Function
    DependsOn:
      - MyTable
      - MyBucket
  MyTable:
    Type: AWS::DynamoDB::Table
  MyBucket:
    Type: AWS::S3::Bucket
`
	opts := makeOpts("template.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	depEdges := filterEdges(result.Edges, "depends_on")
	if len(depEdges) != 2 {
		t.Fatalf("expected 2 depends_on edges, got %d", len(depEdges))
	}

	funcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, "MyFunction", "resource")
	for _, edge := range depEdges {
		if edge.SourceHash != funcHash {
			t.Error("depends_on edge source should be MyFunction")
		}
	}
}

func TestCfnExtractor_ExtractSAMEvents(t *testing.T) {
	e := &cfnExtractor{}
	content := `Transform: AWS::Serverless-2016-10-31
Resources:
  MyApiFunc:
    Type: AWS::Serverless::Function
    Properties:
      Runtime: python3.9
      Events:
        GetItems:
          Type: Api
          Properties:
            Path: /items
            Method: get
        ProcessQueue:
          Type: SQS
          Properties:
            Queue: !GetAtt MyQueue.Arn
        DailyJob:
          Type: Schedule
          Properties:
            Schedule: rate(1 day)
  MyQueue:
    Type: AWS::SQS::Queue
`
	opts := makeOpts("sam-template.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: MyApiFunc, MyQueue (resources) + route node + 2 event_source nodes = 5
	if len(result.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(result.Nodes))
	}

	// Check for route node
	var routeNodes []types.Node
	var eventSourceNodes []types.Node
	for _, n := range result.Nodes {
		switch n.Kind {
		case "route":
			routeNodes = append(routeNodes, n)
		case "event_source":
			eventSourceNodes = append(eventSourceNodes, n)
		}
	}

	if len(routeNodes) != 1 {
		t.Fatalf("expected 1 route node, got %d", len(routeNodes))
	}
	if routeNodes[0].QualifiedName != "github.com/test/repo://sam-template.yaml.route.GET /items" {
		t.Errorf("unexpected route QN: %s", routeNodes[0].QualifiedName)
	}

	if len(eventSourceNodes) != 2 {
		t.Fatalf("expected 2 event_source nodes, got %d", len(eventSourceNodes))
	}

	// Check edges: 1 handles_route + 2 subscribes
	routeEdges := filterEdges(result.Edges, "handles_route")
	if len(routeEdges) != 1 {
		t.Fatalf("expected 1 handles_route edge, got %d", len(routeEdges))
	}

	subEdges := filterEdges(result.Edges, "subscribes")
	if len(subEdges) != 2 {
		t.Fatalf("expected 2 subscribes edges, got %d", len(subEdges))
	}
}

func TestCfnExtractor_EmptyResources(t *testing.T) {
	e := &cfnExtractor{}
	content := `AWSTemplateFormatVersion: "2010-09-09"
Description: Empty template
`
	opts := makeOpts("empty.yaml", content)
	result, err := e.extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(result.Edges))
	}
}

// filterEdges returns edges matching the given type.
func filterEdges(edges []types.Edge, edgeType string) []types.Edge {
	var out []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			out = append(out, e)
		}
	}
	return out
}
