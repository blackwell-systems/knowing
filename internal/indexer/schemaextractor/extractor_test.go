package schemaextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestSchemaExtractor_Name(t *testing.T) {
	e := NewSchemaExtractor()
	if got := e.Name(); got != "openapi-schema" {
		t.Errorf("Name() = %q, want %q", got, "openapi-schema")
	}
}

func TestSchemaExtractor_CanHandle(t *testing.T) {
	e := NewSchemaExtractor()

	positives := []string{"api.yaml", "spec.json", "openapi.yml", "path/to/schema.JSON", "foo.YAML"}
	for _, p := range positives {
		if !e.CanHandle(p) {
			t.Errorf("CanHandle(%q) = false, want true", p)
		}
	}

	negatives := []string{"main.go", "app.ts", "readme.md", "config.toml"}
	for _, p := range negatives {
		if e.CanHandle(p) {
			t.Errorf("CanHandle(%q) = true, want false", p)
		}
	}
}

func TestSchemaExtractor_OpenAPI3Paths(t *testing.T) {
	spec := []byte(`
openapi: "3.0.3"
info:
  title: Test API
  version: "1.0"
paths:
  /users:
    get:
      summary: List users
    post:
      summary: Create user
  /users/{id}:
    get:
      summary: Get user
    delete:
      summary: Delete user
`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "api/openapi.yaml",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have 4 route nodes: GET /users, POST /users, GET /users/{id}, DELETE /users/{id}
	routeNodes := filterNodesByKind(result.Nodes, "route")
	if len(routeNodes) != 4 {
		t.Errorf("got %d route nodes, want 4", len(routeNodes))
		for _, n := range routeNodes {
			t.Logf("  node: %s", n.QualifiedName)
		}
	}

	// Verify qualified name format
	found := false
	for _, n := range routeNodes {
		if n.QualifiedName == "github.com/example/repo://api/openapi.yaml.route.GET /users/{id}" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected route node with QualifiedName containing 'GET /users/{id}'")
	}
}

func TestSchemaExtractor_OpenAPI3Schemas(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
paths: {}
components:
  schemas:
    User:
      type: object
      properties:
        name:
          type: string
    Error:
      type: object
      properties:
        message:
          type: string
`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "api/openapi.yaml",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	typeNodes := filterNodesByKind(result.Nodes, "type")
	if len(typeNodes) != 2 {
		t.Errorf("got %d type nodes, want 2", len(typeNodes))
	}

	names := make(map[string]bool)
	for _, n := range typeNodes {
		names[n.QualifiedName] = true
	}
	expected := "github.com/example/repo://api/openapi.yaml.schema.User"
	if !names[expected] {
		t.Errorf("missing expected schema node: %s", expected)
	}
}

func TestSchemaExtractor_OpenAPI3References(t *testing.T) {
	spec := []byte(`
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
paths:
  /users:
    post:
      requestBody:
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/User"
      responses:
        "200":
          description: Success
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/User"
components:
  schemas:
    User:
      type: object
      properties:
        name:
          type: string
`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "api/openapi.yaml",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have at least one "references" edge from route to schema
	refEdges := filterEdgesByType(result.Edges, "references")
	if len(refEdges) == 0 {
		t.Error("expected at least one 'references' edge, got 0")
	}

	// Verify edge properties
	for _, edge := range refEdges {
		if edge.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q", edge.Provenance, "ast_inferred")
		}
		if edge.Confidence != 0.7 {
			t.Errorf("edge confidence = %v, want 0.7", edge.Confidence)
		}
	}
}

func TestSchemaExtractor_Swagger2(t *testing.T) {
	spec := []byte(`
swagger: "2.0"
info:
  title: Test API
  version: "1.0"
paths:
  /items:
    get:
      responses:
        "200":
          schema:
            $ref: "#/definitions/Item"
definitions:
  Item:
    type: object
    properties:
      id:
        type: integer
      related:
        $ref: "#/definitions/Category"
  Category:
    type: object
    properties:
      name:
        type: string
`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "api/swagger.yaml",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have type nodes for Item and Category
	typeNodes := filterNodesByKind(result.Nodes, "type")
	if len(typeNodes) != 2 {
		t.Errorf("got %d type nodes, want 2", len(typeNodes))
	}

	// Should have route node for GET /items
	routeNodes := filterNodesByKind(result.Nodes, "route")
	if len(routeNodes) != 1 {
		t.Errorf("got %d route nodes, want 1", len(routeNodes))
	}

	// Should have reference edges
	refEdges := filterEdgesByType(result.Edges, "references")
	if len(refEdges) == 0 {
		t.Error("expected reference edges for $ref usage")
	}
}

func TestSchemaExtractor_JSONSchema(t *testing.T) {
	spec := []byte(`{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Address",
  "type": "object",
  "properties": {
    "street": { "type": "string" },
    "city": { "$ref": "#/definitions/City" }
  },
  "definitions": {
    "City": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      }
    }
  }
}`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "schemas/address.json",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have type nodes for "Address" (root) and "City" (definition)
	typeNodes := filterNodesByKind(result.Nodes, "type")
	if len(typeNodes) != 2 {
		t.Errorf("got %d type nodes, want 2", len(typeNodes))
		for _, n := range typeNodes {
			t.Logf("  node: %s", n.QualifiedName)
		}
	}

	// Should have a "references" edge from Address to City
	refEdges := filterEdgesByType(result.Edges, "references")
	if len(refEdges) == 0 {
		t.Error("expected 'references' edge from root schema to City definition")
	}
}

func TestSchemaExtractor_NonOpenAPIYAML(t *testing.T) {
	content := []byte(`
name: my-app
version: 1.0.0
dependencies:
  - foo
  - bar
`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "config.yaml",
		Content:  content,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("got %d nodes for non-OpenAPI YAML, want 0", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("got %d edges for non-OpenAPI YAML, want 0", len(result.Edges))
	}
}

func TestSchemaExtractor_JSONFile(t *testing.T) {
	spec := []byte(`{
  "openapi": "3.0.1",
  "info": { "title": "JSON API", "version": "1.0" },
  "paths": {
    "/health": {
      "get": {
        "summary": "Health check"
      }
    }
  },
  "components": {
    "schemas": {
      "Status": {
        "type": "object",
        "properties": {
          "ok": { "type": "boolean" }
        }
      }
    }
  }
}`)

	e := NewSchemaExtractor()
	result, err := e.Extract(context.Background(), types.ExtractOptions{
		RepoURL:  "github.com/example/repo",
		FilePath: "api/openapi.json",
		Content:  spec,
	})
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	routeNodes := filterNodesByKind(result.Nodes, "route")
	if len(routeNodes) != 1 {
		t.Errorf("got %d route nodes, want 1", len(routeNodes))
	}

	typeNodes := filterNodesByKind(result.Nodes, "type")
	if len(typeNodes) != 1 {
		t.Errorf("got %d type nodes, want 1", len(typeNodes))
	}
}

// --- Test helpers ---

func filterNodesByKind(nodes []types.Node, kind string) []types.Node {
	var filtered []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			filtered = append(filtered, n)
		}
	}
	return filtered
}

func filterEdgesByType(edges []types.Edge, edgeType string) []types.Edge {
	var filtered []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			filtered = append(filtered, e)
		}
	}
	return filtered
}
