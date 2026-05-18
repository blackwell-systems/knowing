package graphqlextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestGraphQLExtractor_CanHandle(t *testing.T) {
	e := NewGraphQLExtractor()
	tests := []struct {
		path string
		want bool
	}{
		{"schema.graphql", true},
		{"schema.gql", true},
		{"src/api/types.graphql", true},
		// Negative cases
		{"main.go", false},
		{"schema.json", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := e.CanHandle(tt.path); got != tt.want {
				t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGraphQLExtractor_ExtractSchema(t *testing.T) {
	e := NewGraphQLExtractor()
	content := []byte(`
type User {
  id: ID!
  name: String!
  posts: [Post!]!
}

type Post {
  id: ID!
  title: String!
  author: User!
}

interface Node {
  id: ID!
}

type Comment implements Node {
  id: ID!
  body: String!
  author: User!
}

enum Role {
  ADMIN
  USER
  GUEST
}

input CreateUserInput {
  name: String!
  email: String!
  role: Role
}

type Query {
  user(id: ID!): User
  posts: [Post!]!
}

type Mutation {
  createUser(input: CreateUserInput!): User
}
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/api",
		FilePath: "schema.graphql",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// Types: User, Post, Node (interface), Comment, Role (enum),
	// CreateUserInput (input), Query, Mutation
	// Operations: user, posts (from Query), createUser (from Mutation)
	nodesByKind := make(map[string][]string)
	for _, n := range result.Nodes {
		nodesByKind[n.Kind] = append(nodesByKind[n.Kind], n.QualifiedName)
	}

	if len(nodesByKind["type"]) < 6 {
		t.Errorf("expected at least 6 type nodes, got %d: %v", len(nodesByKind["type"]), nodesByKind["type"])
	}
	if len(nodesByKind["interface"]) < 1 {
		t.Errorf("expected at least 1 interface node, got %d", len(nodesByKind["interface"]))
	}
	if len(nodesByKind["function"]) < 3 {
		t.Errorf("expected at least 3 function nodes (operations), got %d: %v", len(nodesByKind["function"]), nodesByKind["function"])
	}

	// Check edges: implements, references.
	edgesByType := make(map[string]int)
	for _, e := range result.Edges {
		edgesByType[e.EdgeType]++
	}

	if edgesByType["implements"] < 1 {
		t.Errorf("expected at least 1 implements edge, got %d", edgesByType["implements"])
	}
	if edgesByType["references"] < 1 {
		t.Errorf("expected at least 1 references edge, got %d", edgesByType["references"])
	}
}

func TestGraphQLExtractor_ExtractSimple(t *testing.T) {
	e := NewGraphQLExtractor()
	content := []byte(`
scalar DateTime

type Event {
  id: ID!
  name: String!
  date: DateTime
}
`)

	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/org/api",
		FilePath: "types.gql",
		FileHash: types.NewHash([]byte("test")),
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	// DateTime scalar + Event type = 2 type nodes
	if len(result.Nodes) < 2 {
		t.Errorf("got %d nodes, want at least 2", len(result.Nodes))
	}

	// Event references DateTime.
	if len(result.Edges) < 1 {
		t.Errorf("got %d edges, want at least 1 (Event -> DateTime reference)", len(result.Edges))
	}
}
