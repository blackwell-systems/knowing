package cloudextractor

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
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
	workflowName := getString(raw, "name")
	if workflowName == "" {
		workflowName = filepath.Base(filePath)
	}

	// Create the workflow node.
	workflowQN := buildQN(repoURL, filePath, "workflow", workflowName)
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
		jobQN := buildQN(repoURL, filePath, "job", jobID)
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
				edge := makeEdge(jobHash, depHash, "depends_on")
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
					usesVal := getString(step, "uses")
					if usesVal == "" {
						continue
					}

					// Create action node (deduplicated).
					actionHash, exists := actionNodes[usesVal]
					if !exists {
						actionQN := buildQN(repoURL, filePath, "action", usesVal)
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
					edge := makeEdge(jobHash, actionHash, "references")
					result.Edges = append(result.Edges, edge)
				}
			}
		}
	}

	// Extract deployed_by and tested_by edges.
	jDefs := make(map[string]jobDef)
	for jobID, jobVal := range jobsMap {
		jd, ok := jobVal.(map[string]interface{})
		if !ok {
			continue
		}
		var steps []stepDef
		if stepsVal, ok := jd["steps"]; ok {
			if stepsList, ok := stepsVal.([]interface{}); ok {
				for _, s := range stepsList {
					if sm, ok := s.(map[string]interface{}); ok {
						sd := stepDef{}
						if r, ok := sm["run"].(string); ok {
							sd.Run = r
						}
						if u, ok := sm["uses"].(string); ok {
							sd.Uses = u
						}
						if n, ok := sm["name"].(string); ok {
							sd.Name = n
						}
						if withVal, ok := sm["with"].(map[string]interface{}); ok {
							sd.With = make(map[string]string)
							for k, v := range withVal {
								if vs, ok := v.(string); ok {
									sd.With[k] = vs
								}
							}
						}
						steps = append(steps, sd)
					}
				}
			}
		}
		name, _ := jd["name"].(string)
		jDefs[jobID] = jobDef{Name: name, Hash: jobHashes[jobID], Steps: steps}
	}
	deployNodes, deployEdges := extractDeployedByEdges(opts, workflowHash, jDefs)
	result.Nodes = append(result.Nodes, deployNodes...)
	result.Edges = append(result.Edges, deployEdges...)
	testNodes, testEdges := extractTestedByEdges(opts, jDefs)
	result.Nodes = append(result.Nodes, testNodes...)
	result.Edges = append(result.Edges, testEdges...)

	return result, nil
}

