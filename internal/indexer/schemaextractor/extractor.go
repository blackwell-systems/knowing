package schemaextractor

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	provenance = "ast_inferred"
	confidence = 0.7
)

// SchemaExtractor extracts nodes and edges from OpenAPI and JSON Schema files.
type SchemaExtractor struct{}

// NewSchemaExtractor creates a new SchemaExtractor instance.
func NewSchemaExtractor() *SchemaExtractor {
	return &SchemaExtractor{}
}

// Name returns the identifier for this extractor.
func (e *SchemaExtractor) Name() string {
	return "openapi-schema"
}

// CanHandle returns true for .yaml, .yml, and .json file extensions.
func (e *SchemaExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml" || ext == ".json"
}

// Extract parses OpenAPI/Swagger/JSON Schema files and produces nodes and edges.
// If the file content does not contain recognized markers, it returns an empty
// result without error (the established pattern for non-matching content).
func (e *SchemaExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	data, err := unmarshalContent(opts.FilePath, opts.Content)
	if err != nil {
		// Malformed content: return empty result, not an error.
		return result, nil
	}

	switch {
	case isOpenAPI3(data):
		extractOpenAPI3(data, opts, result)
	case isSwagger2(data):
		extractSwagger2(data, opts, result)
	case isJSONSchema(data):
		extractJSONSchema(data, opts, result)
	default:
		// No recognized markers; return empty result.
		return result, nil
	}

	return result, nil
}

// unmarshalContent parses file content into a generic map. It uses the file
// extension to decide between YAML and JSON parsing.
func unmarshalContent(filePath string, content []byte) (map[string]interface{}, error) {
	var data map[string]interface{}
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		if err := json.Unmarshal(content, &data); err != nil {
			return nil, err
		}
	default:
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// isOpenAPI3 returns true if data has an "openapi" field starting with "3".
func isOpenAPI3(data map[string]interface{}) bool {
	v, ok := data["openapi"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && strings.HasPrefix(s, "3")
}

// isSwagger2 returns true if data has a "swagger" field equal to "2.0".
func isSwagger2(data map[string]interface{}) bool {
	v, ok := data["swagger"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && s == "2.0"
}

// isJSONSchema returns true if data has a "$schema" field containing "json-schema.org".
func isJSONSchema(data map[string]interface{}) bool {
	v, ok := data["$schema"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && strings.Contains(s, "json-schema.org")
}

// extractOpenAPI3 handles OpenAPI 3.x spec extraction.
func extractOpenAPI3(data map[string]interface{}, opts types.ExtractOptions, result *types.ExtractResult) {
	nodeMap := make(map[string]types.Hash) // schema name -> node hash

	// Extract components.schemas
	if components, ok := data["components"].(map[string]interface{}); ok {
		if schemas, ok := components["schemas"].(map[string]interface{}); ok {
			extractSchemas(schemas, opts, result, nodeMap)
		}
	}

	// Extract paths
	if paths, ok := data["paths"].(map[string]interface{}); ok {
		extractPaths(paths, opts, result, nodeMap, "#/components/schemas/")
	}
}

// extractSwagger2 handles Swagger 2.x spec extraction.
func extractSwagger2(data map[string]interface{}, opts types.ExtractOptions, result *types.ExtractResult) {
	nodeMap := make(map[string]types.Hash)

	// Extract definitions (Swagger 2.x equivalent of components.schemas)
	if definitions, ok := data["definitions"].(map[string]interface{}); ok {
		extractSchemas(definitions, opts, result, nodeMap)
	}

	// Extract paths
	if paths, ok := data["paths"].(map[string]interface{}); ok {
		extractPaths(paths, opts, result, nodeMap, "#/definitions/")
	}
}

// extractJSONSchema handles standalone JSON Schema extraction.
func extractJSONSchema(data map[string]interface{}, opts types.ExtractOptions, result *types.ExtractResult) {
	nodeMap := make(map[string]types.Hash)

	// Root schema node
	title := getString(data, "title")
	if title == "" {
		// Derive name from filename without extension
		base := filepath.Base(opts.FilePath)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}

	qn := buildQualifiedName(opts.RepoURL, opts.FilePath, "schema", title)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, title, types.KindType)

	result.Nodes = append(result.Nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          types.KindType,
		Line:          1,
	})
	nodeMap[title] = nodeHash

	// Extract definitions
	if definitions, ok := data["definitions"].(map[string]interface{}); ok {
		for name, def := range definitions {
			defQN := buildQualifiedName(opts.RepoURL, opts.FilePath, "schema", name)
			defHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, types.KindType)

			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      defHash,
				FileHash:      opts.FileHash,
				QualifiedName: defQN,
				Kind:          types.KindType,
				Line:          1,
			})
			nodeMap[name] = defHash

			// Scan definition properties for $ref
			if defMap, ok := def.(map[string]interface{}); ok {
				collectPropertyRefs(defMap, defHash, nodeMap, result, "#/definitions/")
			}
		}
	}

	// Scan root properties for $ref
	collectPropertyRefs(data, nodeHash, nodeMap, result, "#/definitions/")
}

// extractSchemas creates type nodes from a schema map (components.schemas or definitions).
func extractSchemas(schemas map[string]interface{}, opts types.ExtractOptions, result *types.ExtractResult, nodeMap map[string]types.Hash) {
	for name, schema := range schemas {
		qn := buildQualifiedName(opts.RepoURL, opts.FilePath, "schema", name)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, types.KindType)

		result.Nodes = append(result.Nodes, types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindType,
			Line:          1,
		})
		nodeMap[name] = nodeHash

		// Scan schema properties for $ref (schema-to-schema references)
		if schemaMap, ok := schema.(map[string]interface{}); ok {
			collectPropertyRefs(schemaMap, nodeHash, nodeMap, result, "")
		}
	}
}

// extractPaths creates route nodes for each method on each path.
func extractPaths(paths map[string]interface{}, opts types.ExtractOptions, result *types.ExtractResult, nodeMap map[string]types.Hash, refPrefix string) {
	methods := []string{"get", "post", "put", "delete", "patch", "head", "options", "trace"}

	for path, pathItem := range paths {
		pathMap, ok := pathItem.(map[string]interface{})
		if !ok {
			continue
		}

		for _, method := range methods {
			operation, ok := pathMap[method].(map[string]interface{})
			if !ok {
				continue
			}

			upperMethod := strings.ToUpper(method)
			routeIdent := fmt.Sprintf("%s %s", upperMethod, path)
			qn := buildQualifiedName(opts.RepoURL, opts.FilePath, "route", routeIdent)
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, routeIdent, "route")

			result.Nodes = append(result.Nodes, types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qn,
				Kind:          "route",
				Line:          1,
			})

			// Collect $ref from operation (requestBody, responses, parameters)
			refs := collectOperationRefs(operation)
			for _, ref := range refs {
				schemaName := parseRef(ref)
				if schemaName == "" {
					continue
				}
				if targetHash, ok := nodeMap[schemaName]; ok {
					result.Edges = append(result.Edges, makeEdge(nodeHash, targetHash, "references"))
				}
			}
		}
	}
}

// collectOperationRefs walks an operation object recursively to find all $ref values.
func collectOperationRefs(obj interface{}) []string {
	var refs []string
	switch v := obj.(type) {
	case map[string]interface{}:
		if ref, ok := v["$ref"].(string); ok {
			refs = append(refs, ref)
		}
		for _, val := range v {
			refs = append(refs, collectOperationRefs(val)...)
		}
	case []interface{}:
		for _, item := range v {
			refs = append(refs, collectOperationRefs(item)...)
		}
	}
	return refs
}

// collectPropertyRefs scans a schema object's properties for $ref and creates edges.
func collectPropertyRefs(schema map[string]interface{}, sourceHash types.Hash, nodeMap map[string]types.Hash, result *types.ExtractResult, refPrefix string) {
	refs := collectOperationRefs(schema)
	for _, ref := range refs {
		schemaName := parseRef(ref)
		if schemaName == "" {
			continue
		}
		if targetHash, ok := nodeMap[schemaName]; ok {
			result.Edges = append(result.Edges, makeEdge(sourceHash, targetHash, "references"))
		}
	}
}

// parseRef extracts the schema name from a $ref value.
// Examples: "#/components/schemas/User" -> "User", "#/definitions/Foo" -> "Foo"
func parseRef(ref string) string {
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ""
}

// buildQualifiedName constructs the qualified name for a schema extractor node.
// Format: {repoURL}://{filePath}.{kind}.{name}
func buildQualifiedName(repoURL, filePath, kind, name string) string {
	return fmt.Sprintf("%s://%s.%s.%s", repoURL, filePath, kind, name)
}

// makeEdge creates an edge with the standard provenance and confidence.
func makeEdge(sourceHash, targetHash types.Hash, edgeType string) types.Edge {
	return types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, edgeType, provenance),
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgeType,
		Confidence: confidence,
		Provenance: provenance,
	}
}

// getString safely extracts a string value from a map.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
