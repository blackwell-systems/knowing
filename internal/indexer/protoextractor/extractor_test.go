package protoextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

const testProto = `syntax = "proto3";

package myapp.users;

import "google/protobuf/timestamp.proto";
import "common/types.proto";

message User {
  string id = 1;
  string name = 2;
  string email = 3;
  Address address = 4;
  repeated Role roles = 5;
}

message Address {
  string street = 1;
  string city = 2;
  string country = 3;
}

message Role {
  string name = 1;
  repeated Permission permissions = 2;
}

message Permission {
  string resource = 1;
  string action = 2;
}

enum UserStatus {
  UNKNOWN = 0;
  ACTIVE = 1;
  SUSPENDED = 2;
}

message CreateUserRequest {
  string name = 1;
  string email = 2;
}

message CreateUserResponse {
  User user = 1;
}

message GetUserRequest {
  string id = 1;
}

message GetUserResponse {
  User user = 1;
}

message ListUsersRequest {
  int32 page_size = 1;
  string page_token = 2;
}

message ListUsersResponse {
  repeated User users = 1;
  string next_page_token = 2;
}

service UserService {
  rpc CreateUser(CreateUserRequest) returns (CreateUserResponse);
  rpc GetUser(GetUserRequest) returns (GetUserResponse);
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);
}
`

func TestCanHandle(t *testing.T) {
	ext := NewProtoExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"api/users.proto", true},
		{"proto/service.proto", true},
		{"vendor/google/api.proto", false},
		{".git/objects/pack.proto", false},
		{"main.go", false},
		{"schema.sql", false},
	}

	for _, tt := range tests {
		if got := ext.CanHandle(tt.path); got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestExtract_Messages(t *testing.T) {
	ext := NewProtoExtractor()
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  []byte(testProto),
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Count nodes by kind.
	kindCount := make(map[string]int)
	for _, n := range result.Nodes {
		kindCount[n.Kind]++
	}

	// Should have messages: User, Address, Role, Permission, UserStatus (enum),
	// CreateUserRequest, CreateUserResponse, GetUserRequest, GetUserResponse,
	// ListUsersRequest, ListUsersResponse = 11 types
	// Plus service: UserService = 1 service
	// Plus RPCs: CreateUser, GetUser, ListUsers = 3 functions
	if kindCount["type"] < 5 {
		t.Errorf("Expected at least 5 type nodes, got %d", kindCount["type"])
	}
	if kindCount["service"] != 1 {
		t.Errorf("Expected 1 service node, got %d", kindCount["service"])
	}
	if kindCount["function"] != 3 {
		t.Errorf("Expected 3 function nodes (RPCs), got %d", kindCount["function"])
	}

	t.Logf("Nodes: %d total (%d types, %d services, %d functions)",
		len(result.Nodes), kindCount["type"], kindCount["service"], kindCount["function"])
	t.Logf("Edges: %d total", len(result.Edges))

	// Verify some specific nodes exist.
	nodeNames := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeNames[n.QualifiedName] = true
		t.Logf("  Node: %s (%s) line=%d sig=%q", n.QualifiedName, n.Kind, n.Line, n.Signature)
	}

	expectedNodes := []string{
		"github.com/example/app://myapp.users.UserService",
	}
	for _, expected := range expectedNodes {
		if !nodeNames[expected] {
			t.Errorf("Missing expected node: %s", expected)
		}
	}
}

func TestExtract_Edges(t *testing.T) {
	ext := NewProtoExtractor()
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  []byte(testProto),
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Count edges by type.
	edgeTypeCount := make(map[string]int)
	for _, e := range result.Edges {
		edgeTypeCount[e.EdgeType]++
	}

	t.Logf("Edge types: %v", edgeTypeCount)

	// Should have imports edges for the two import statements.
	if edgeTypeCount["imports"] != 2 {
		t.Errorf("Expected 2 imports edges, got %d", edgeTypeCount["imports"])
	}

	// Should have calls edges (service -> rpc).
	if edgeTypeCount["calls"] != 3 {
		t.Errorf("Expected 3 calls edges (service -> rpcs), got %d", edgeTypeCount["calls"])
	}

	// Should have references edges (rpc -> request/response types, message -> field types).
	if edgeTypeCount["references"] < 3 {
		t.Errorf("Expected at least 3 references edges, got %d", edgeTypeCount["references"])
	}

	// All edges should have correct provenance and confidence.
	for _, e := range result.Edges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("Edge provenance = %q, want ast_inferred", e.Provenance)
		}
		if e.Confidence != 0.7 {
			t.Errorf("Edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestExtract_EmptyFile(t *testing.T) {
	ext := NewProtoExtractor()
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("empty")),
		Content:  []byte("syntax = \"proto3\";\n"),
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("Expected 0 nodes for empty proto, got %d", len(result.Nodes))
	}
}

func TestExtract_SimpleService(t *testing.T) {
	ext := NewProtoExtractor()
	proto := `syntax = "proto3";
package health;

message HealthCheckRequest {
  string service = 1;
}

message HealthCheckResponse {
  enum ServingStatus {
    UNKNOWN = 0;
    SERVING = 1;
    NOT_SERVING = 2;
  }
  ServingStatus status = 1;
}

service Health {
  rpc Check(HealthCheckRequest) returns (HealthCheckResponse);
}
`
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/svc",
		FileHash: types.NewHash([]byte("health")),
		Content:  []byte(proto),
	}

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Should have: 2 messages + 1 service + 1 rpc = 4 nodes minimum.
	if len(result.Nodes) < 4 {
		t.Errorf("Expected at least 4 nodes, got %d", len(result.Nodes))
		for _, n := range result.Nodes {
			t.Logf("  %s (%s)", n.QualifiedName, n.Kind)
		}
	}
}
