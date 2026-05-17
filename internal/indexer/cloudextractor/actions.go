package cloudextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	actionsProvenance = "ast_inferred"
	actionsConfidence = 0.7
)

// actionsExtractor extracts nodes and edges from GitHub Actions workflow files.
// It detects workflow files under .github/workflows/ and parses them to produce
// workflow, job, and action nodes with their relationships.
type actionsExtractor struct{}

func (e *actionsExtractor) name() string { return "github-actions" }

// canHandle returns true if the file is a YAML file under .github/workflows/.
func (e *actionsExtractor) canHandle(path string, _ []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yml" && ext != ".yaml" {
		return false
	}
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, ".github/workflows/")
}

// extract parses a GitHub Actions workflow file and produces nodes and edges.
func (e *actionsExtractor) extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &raw); err != nil {
		return result, nil
	}

	repoURL := opts.RepoURL
	filePath := opts.FilePath

	// Extract workflow name, falling back to the base filename.
	workflowName := actionsGetString(raw, "name")
	if workflowName == "" {
		workflowName = filepath.Base(filePath)
	}

	// Create the workflow node.
	workflowQN := actionsBuildQN(repoURL, filePath, "workflow", workflowName)
	workflowHash := types.ComputeNodeHash(repoURL, filePath, types.EmptyHash, workflowName, "workflow")
	workflowNode := types.Node{
		NodeHash:      workflowHash,
		FileHash:      opts.FileHash,
		QualifiedName: workflowQN,
		Kind:          "workflow",
	}
	result.Nodes = append(result.Nodes, workflowNode)

	// Parse jobs.
	jobsRaw, ok := raw["jobs"]
	if !ok {
		return result, nil
	}
	jobsMap, ok := jobsRaw.(map[string]interface{})
	if !ok {
		return result, nil
	}

	// Track job node hashes for resolving "needs" edges.
	jobHashes := make(map[string]types.Hash)

	// Track action nodes to deduplicate across jobs.
	actionNodes := make(map[string]types.Hash) // uses value -> nodeHash

	for jobID, jobVal := range jobsMap {
		jobDef, ok := jobVal.(map[string]interface{})
		if !ok {
			continue
		}

		// Create the job node.
		jobQN := actionsBuildQN(repoURL, filePath, "job", jobID)
		jobHash := types.ComputeNodeHash(repoURL, filePath, types.EmptyHash, jobID, "job")
		jobNode := types.Node{
			NodeHash:      jobHash,
			FileHash:      opts.FileHash,
			QualifiedName: jobQN,
			Kind:          "job",
		}
		result.Nodes = append(result.Nodes, jobNode)
		jobHashes[jobID] = jobHash

		// Parse "needs" dependencies.
		if needsVal, ok := jobDef["needs"]; ok {
			var needsList []string
			switch v := needsVal.(type) {
			case string:
				needsList = []string{v}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						needsList = append(needsList, s)
					}
				}
			}
			for _, dep := range needsList {
				depHash := types.ComputeNodeHash(repoURL, filePath, types.EmptyHash, dep, "job")
				edge := actionsMakeEdge(jobHash, depHash, "depends_on")
				result.Edges = append(result.Edges, edge)
			}
		}

		// Parse steps for "uses" references.
		if stepsVal, ok := jobDef["steps"]; ok {
			if steps, ok := stepsVal.([]interface{}); ok {
				for _, stepVal := range steps {
					step, ok := stepVal.(map[string]interface{})
					if !ok {
						continue
					}
					usesVal := actionsGetString(step, "uses")
					if usesVal == "" {
						continue
					}

					// Create action node (deduplicated).
					actionHash, exists := actionNodes[usesVal]
					if !exists {
						actionQN := actionsBuildQN(repoURL, filePath, "action", usesVal)
						actionHash = types.ComputeNodeHash(repoURL, filePath, types.EmptyHash, usesVal, "action")
						actionNode := types.Node{
							NodeHash:      actionHash,
							FileHash:      opts.FileHash,
							QualifiedName: actionQN,
							Kind:          "action",
						}
						result.Nodes = append(result.Nodes, actionNode)
						actionNodes[usesVal] = actionHash
					}

					// Create references edge from job to action.
					edge := actionsMakeEdge(jobHash, actionHash, "references")
					result.Edges = append(result.Edges, edge)
				}
			}
		}
	}

	return result, nil
}

// actionsGetString safely extracts a string value from a map.
func actionsGetString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// actionsToStringMap converts a map[string]interface{} to map[string]string.
func actionsToStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

// actionsMakeEdge creates an edge with standard actions provenance and confidence.
func actionsMakeEdge(sourceHash, targetHash types.Hash, edgeType string) types.Edge {
	return types.Edge{
		EdgeHash:   types.ComputeEdgeHash(sourceHash, targetHash, edgeType, actionsProvenance),
		SourceHash: sourceHash,
		TargetHash: targetHash,
		EdgeType:   edgeType,
		Confidence: actionsConfidence,
		Provenance: actionsProvenance,
	}
}

// actionsBuildQN constructs the qualified name for a GitHub Actions node.
// Format: {repoURL}://{filePath}.{kind}.{name}
func actionsBuildQN(repoURL, filePath, kind, name string) string {
	return fmt.Sprintf("%s://%s.%s.%s", repoURL, filePath, kind, name)
}
