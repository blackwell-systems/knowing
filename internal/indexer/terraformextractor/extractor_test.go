package terraformextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestTerraformExtractor_Name(t *testing.T) {
	e := NewTerraformExtractor()
	if got := e.Name(); got != "treesitter-terraform" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-terraform")
	}
}

func TestTerraformExtractor_CanHandle(t *testing.T) {
	e := NewTerraformExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"main.tf", true},
		{"modules/vpc/main.tf", true},
		{"infra/prod/variables.tf", true},
		{"main.tfvars", false},
		{"readme.md", false},
		{".terraform/modules/vpc/main.tf", false},
		{"path/to/.terraform/providers/aws.tf", false},
		{"main.tf.bak", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestTerraformExtractor_ExtractResources(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}

resource "aws_security_group" "allow_http" {
  name = "allow_http"
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "main.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(result.Nodes))
	}

	// Check that both resources were extracted.
	foundWeb := false
	foundSG := false
	for _, n := range result.Nodes {
		if n.Kind == "resource" && n.QualifiedName == "github.com/org/infra://main.tf.resource.aws_instance.web" {
			foundWeb = true
			if n.Line == 0 {
				t.Error("expected non-zero line for aws_instance.web")
			}
		}
		if n.Kind == "resource" && n.QualifiedName == "github.com/org/infra://main.tf.resource.aws_security_group.allow_http" {
			foundSG = true
		}
	}
	if !foundWeb {
		t.Error("did not find resource node for aws_instance.web")
	}
	if !foundSG {
		t.Error("did not find resource node for aws_security_group.allow_http")
	}
}

func TestTerraformExtractor_ExtractDataSources(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"]
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "data.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}

	found := false
	for _, n := range result.Nodes {
		if n.Kind == "data" && n.QualifiedName == "github.com/org/infra://data.tf.data.aws_ami.ubuntu" {
			found = true
		}
	}
	if !found {
		t.Error("did not find data node for aws_ami.ubuntu")
	}
}

func TestTerraformExtractor_ExtractModules(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
module "vpc" {
  source = "./modules/vpc"
  cidr   = "10.0.0.0/16"
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "main.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) == 0 {
		t.Fatal("expected at least 1 node")
	}

	foundModule := false
	for _, n := range result.Nodes {
		if n.Kind == "module" && n.QualifiedName == "github.com/org/infra://main.tf.module.vpc" {
			foundModule = true
		}
	}
	if !foundModule {
		t.Error("did not find module node for vpc")
	}
}

func TestTerraformExtractor_ExtractVariablesAndOutputs(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

output "instance_ip" {
  value = aws_instance.web.public_ip
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "variables.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(result.Nodes))
	}

	foundVar := false
	foundOutput := false
	for _, n := range result.Nodes {
		if n.Kind == "variable" && n.QualifiedName == "github.com/org/infra://variables.tf.variable.region" {
			foundVar = true
		}
		if n.Kind == "output" && n.QualifiedName == "github.com/org/infra://variables.tf.output.instance_ip" {
			foundOutput = true
		}
	}
	if !foundVar {
		t.Error("did not find variable node for region")
	}
	if !foundOutput {
		t.Error("did not find output node for instance_ip")
	}
}

func TestTerraformExtractor_ExtractDependsOnEdges(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
  subnet_id     = "${aws_subnet.main.id}"
}

resource "aws_subnet" "main" {
  vpc_id     = "${aws_vpc.main.id}"
  cidr_block = "10.0.1.0/24"
}

resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "main.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) < 3 {
		t.Fatalf("expected at least 3 nodes, got %d", len(result.Nodes))
	}

	// Should have depends_on edges for the interpolation references.
	foundDependsOn := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "depends_on" {
			foundDependsOn = true
			break
		}
	}
	if !foundDependsOn {
		t.Error("expected at least one depends_on edge from interpolation references")
	}
}

func TestTerraformExtractor_ExtractModuleCallEdges(t *testing.T) {
	e := NewTerraformExtractor()

	content := []byte(`
module "network" {
  source = "terraform-aws-modules/vpc/aws"
  version = "3.0.0"
}
`)

	opts := types.ExtractOptions{
		RepoURL:  "github.com/org/infra",
		FilePath: "main.tf",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  content,
	}

	result, err := e.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have a "calls" edge from the module to its source.
	foundCalls := false
	for _, edge := range result.Edges {
		if edge.EdgeType == "calls" {
			foundCalls = true
			break
		}
	}
	if !foundCalls {
		t.Error("expected a calls edge from module to source")
	}
}
