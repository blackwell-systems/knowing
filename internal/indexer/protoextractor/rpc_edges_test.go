package protoextractor

import (
	"context"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"

	"github.com/blackwell-systems/knowing/internal/types"
)

func parseGoSource(t *testing.T, src []byte) *sitter.Node {
	t.Helper()
	parser := getGoParser()
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		t.Fatalf("failed to parse Go source: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree.RootNode()
}

func TestExtractRPCEdges_ImplementsServer(t *testing.T) {
	src := []byte(`package server

import (
	pb "github.com/example/api/proto/user"
)

type UserServer struct {
	pb.UnimplementedUserServiceServer
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"pb": "github.com/example/api/proto/user",
	}

	nodes, edges := ExtractRPCEdges(root, opts, "github.com/example/app/internal/server", imports)

	// Should find 1 implements_rpc edge
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "implements_rpc" {
		t.Errorf("edge type = %q, want implements_rpc", edge.EdgeType)
	}
	if edge.Confidence != 0.9 {
		t.Errorf("confidence = %f, want 0.9", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("provenance = %q, want ast_inferred", edge.Provenance)
	}

	// Verify the source hash matches the struct
	expectedStructHash := types.ComputeNodeHash("github.com/example/app", "github.com/example/app/internal/server", types.EmptyHash, "UserServer", "type")
	if edge.SourceHash != expectedStructHash {
		t.Errorf("source hash mismatch")
	}

	// Verify the target hash matches the proto service
	expectedServiceHash := types.NewHash([]byte("github.com/example/api/proto/user" + "://" + "UserService" + "/service"))
	if edge.TargetHash != expectedServiceHash {
		t.Errorf("target hash mismatch")
	}

	// Should also emit a node for the struct
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	if nodes[0].Kind != "type" {
		t.Errorf("node kind = %q, want type", nodes[0].Kind)
	}
}

func TestExtractRPCEdges_ClientCreation(t *testing.T) {
	src := []byte(`package client

import (
	pb "github.com/example/api/proto/user"
	"google.golang.org/grpc"
)

func NewUserClient(conn *grpc.ClientConn) pb.UserServiceClient {
	return pb.NewUserServiceClient(conn)
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"pb":   "github.com/example/api/proto/user",
		"grpc": "google.golang.org/grpc",
	}

	_, edges := ExtractRPCEdges(root, opts, "github.com/example/app/internal/client", imports)

	// Should find 1 consumes_rpc edge
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "consumes_rpc" {
		t.Errorf("edge type = %q, want consumes_rpc", edge.EdgeType)
	}
	if edge.Confidence != 0.8 {
		t.Errorf("confidence = %f, want 0.8", edge.Confidence)
	}
	if edge.Provenance != "ast_inferred" {
		t.Errorf("provenance = %q, want ast_inferred", edge.Provenance)
	}

	// Verify the target hash matches the proto service
	expectedServiceHash := types.NewHash([]byte("github.com/example/api/proto/user" + "://" + "UserService" + "/service"))
	if edge.TargetHash != expectedServiceHash {
		t.Errorf("target hash mismatch")
	}
}

func TestExtractRPCEdges_NoGRPC(t *testing.T) {
	src := []byte(`package main

import "fmt"

type Config struct {
	Host string
	Port int
}

func main() {
	fmt.Println("hello")
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"fmt": "fmt",
	}

	nodes, edges := ExtractRPCEdges(root, opts, "github.com/example/app", imports)

	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

func TestExtractRPCEdges_MultipleServices(t *testing.T) {
	src := []byte(`package server

import (
	userpb "github.com/example/api/proto/user"
	orderpb "github.com/example/api/proto/order"
	"google.golang.org/grpc"
)

type UserServer struct {
	userpb.UnimplementedUserServiceServer
}

type OrderServer struct {
	orderpb.UnimplementedOrderServiceServer
}

func CreateClients(conn *grpc.ClientConn) {
	_ = userpb.NewUserServiceClient(conn)
	_ = orderpb.NewOrderServiceClient(conn)
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"userpb":  "github.com/example/api/proto/user",
		"orderpb": "github.com/example/api/proto/order",
		"grpc":    "google.golang.org/grpc",
	}

	nodes, edges := ExtractRPCEdges(root, opts, "github.com/example/app/internal/server", imports)

	// Should have 2 implements_rpc + 2 consumes_rpc = 4 edges
	implEdges := 0
	consumeEdges := 0
	for _, e := range edges {
		switch e.EdgeType {
		case "implements_rpc":
			implEdges++
		case "consumes_rpc":
			consumeEdges++
		}
	}

	if implEdges != 2 {
		t.Errorf("expected 2 implements_rpc edges, got %d", implEdges)
	}
	if consumeEdges != 2 {
		t.Errorf("expected 2 consumes_rpc edges, got %d", consumeEdges)
	}

	// Should have 2 struct nodes
	if len(nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(nodes))
	}
}

func TestExtractRPCEdges_CustomAlias(t *testing.T) {
	src := []byte(`package server

import (
	userpb "github.com/example/api/proto/user"
)

type MyUserServer struct {
	userpb.UnimplementedUserServiceServer
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"userpb": "github.com/example/api/proto/user",
	}

	_, edges := ExtractRPCEdges(root, opts, "github.com/example/app/internal/server", imports)

	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	edge := edges[0]
	if edge.EdgeType != "implements_rpc" {
		t.Errorf("edge type = %q, want implements_rpc", edge.EdgeType)
	}

	// Verify the correct proto package path was resolved via custom alias
	expectedServiceHash := types.NewHash([]byte("github.com/example/api/proto/user" + "://" + "UserService" + "/service"))
	if edge.TargetHash != expectedServiceHash {
		t.Errorf("target hash mismatch: custom alias not resolved correctly")
	}
}

func TestExtractRPCEdges_NonProtoFile(t *testing.T) {
	// A Go file that has struct embedding but NOT gRPC patterns
	src := []byte(`package models

import (
	"database/sql"
)

type BaseModel struct {
	sql.NullString
}

type UserModel struct {
	BaseModel
	Name string
}

func NewUserModel() *UserModel {
	return &UserModel{}
}
`)
	root := parseGoSource(t, src)
	opts := types.ExtractOptions{
		RepoURL:  "github.com/example/app",
		FileHash: types.NewHash([]byte("test-file")),
		Content:  src,
	}
	imports := map[string]string{
		"sql": "database/sql",
	}

	nodes, edges := ExtractRPCEdges(root, opts, "github.com/example/app/internal/models", imports)

	// Should find no edges because patterns don't match Unimplemented*Server or New*Client
	if len(nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(nodes))
	}
	if len(edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(edges))
	}
}

// Ensure the golang import is used (prevents lint error).
var _ = golang.GetLanguage
