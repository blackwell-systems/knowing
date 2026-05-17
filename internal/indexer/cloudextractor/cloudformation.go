package cloudextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

// cfnExtractor extracts nodes and edges from AWS CloudFormation and SAM templates.
type cfnExtractor struct{}

// name returns the identifier for this sub-extractor.
func (e *cfnExtractor) name() string { return "cloudformation" }

// canHandle returns true if the file is a YAML file containing CloudFormation
// or SAM template markers.
func (e *cfnExtractor) canHandle(path string, content []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}

	var top map[string]interface{}
	if err := yaml.Unmarshal(content, &top); err != nil {
		return false
	}

	// Check for AWSTemplateFormatVersion
	if _, ok := top["AWSTemplateFormatVersion"]; ok {
		return true
	}

	// Check for SAM Transform
	if t, ok := top["Transform"]; ok {
		if s, ok := t.(string); ok && strings.HasPrefix(s, "AWS::Serverless") {
			return true
		}
	}

	return false
}

// extract parses an AWS CloudFormation/SAM template and produces nodes and edges.
func (e *cfnExtractor) extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var template map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &template); err != nil {
		return result, nil
	}

	resourcesRaw, ok := template["Resources"]
	if !ok {
		return result, nil
	}
	resources, ok := resourcesRaw.(map[string]interface{})
	if !ok {
		return result, nil
	}

	// First pass: create nodes for all resources and build a lookup map.
	nodeMap := make(map[string]types.Hash) // logicalID -> nodeHash
	for logicalID, resDef := range resources {
		resMap, ok := resDef.(map[string]interface{})
		if !ok {
			continue
		}

		_ = getString(resMap, "Type") // used for SAM event detection below

		qn := buildQN(opts.RepoURL, opts.FilePath, "resource", logicalID)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, logicalID, "resource")

		node := types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          "resource",
			Line:          1,
		}
		result.Nodes = append(result.Nodes, node)
		nodeMap[logicalID] = nodeHash
	}

	// Second pass: create edges from references and SAM events.
	for logicalID, resDef := range resources {
		resMap, ok := resDef.(map[string]interface{})
		if !ok {
			continue
		}

		sourceHash := nodeMap[logicalID]
		resType := getString(resMap, "Type")

		// DependsOn edges
		if dep, ok := resMap["DependsOn"]; ok {
			switch v := dep.(type) {
			case string:
				if targetHash, found := nodeMap[v]; found {
					result.Edges = append(result.Edges, makeEdge(sourceHash, targetHash, "depends_on"))
				}
			case []interface{}:
				for _, d := range v {
					if s, ok := d.(string); ok {
						if targetHash, found := nodeMap[s]; found {
							result.Edges = append(result.Edges, makeEdge(sourceHash, targetHash, "depends_on"))
						}
					}
				}
			}
		}

		// Scan Properties for Ref and Fn::GetAtt
		if props, ok := resMap["Properties"].(map[string]interface{}); ok {
			cfnScanRefs(props, sourceHash, nodeMap, result)

			// SAM event handling for serverless functions
			if strings.HasPrefix(resType, "AWS::Serverless::Function") {
				cfnExtractSAMEvents(props, logicalID, sourceHash, opts, result)
			}
		}
	}

	return result, nil
}

// cfnScanRefs recursively scans a value for Ref and Fn::GetAtt references,
// creating "references" edges in the result.
func cfnScanRefs(val interface{}, sourceHash types.Hash, nodeMap map[string]types.Hash, result *types.ExtractResult) {
	switch v := val.(type) {
	case map[string]interface{}:
		// Check for Ref
		if ref, ok := v["Ref"]; ok {
			if s, ok := ref.(string); ok {
				if targetHash, found := nodeMap[s]; found {
					result.Edges = append(result.Edges, makeEdge(sourceHash, targetHash, "references"))
				}
			}
			return
		}
		// Check for Fn::GetAtt
		if getAtt, ok := v["Fn::GetAtt"]; ok {
			if list, ok := getAtt.([]interface{}); ok && len(list) >= 1 {
				if s, ok := list[0].(string); ok {
					if targetHash, found := nodeMap[s]; found {
						result.Edges = append(result.Edges, makeEdge(sourceHash, targetHash, "references"))
					}
				}
			}
			return
		}
		// Recurse into map values
		for _, child := range v {
			cfnScanRefs(child, sourceHash, nodeMap, result)
		}
	case []interface{}:
		for _, item := range v {
			cfnScanRefs(item, sourceHash, nodeMap, result)
		}
	}
}

// cfnExtractSAMEvents processes SAM function Events to create route and event_source nodes.
func cfnExtractSAMEvents(props map[string]interface{}, logicalID string, sourceHash types.Hash, opts types.ExtractOptions, result *types.ExtractResult) {
	eventsRaw, ok := props["Events"]
	if !ok {
		return
	}
	events, ok := eventsRaw.(map[string]interface{})
	if !ok {
		return
	}

	for eventName, eventDef := range events {
		evMap, ok := eventDef.(map[string]interface{})
		if !ok {
			continue
		}

		eventType := getString(evMap, "Type")
		eventProps, _ := evMap["Properties"].(map[string]interface{})

		switch eventType {
		case "Api", "HttpApi":
			path := ""
			method := ""
			if eventProps != nil {
				path = getString(eventProps, "Path")
				method = strings.ToUpper(getString(eventProps, "Method"))
			}
			routeName := fmt.Sprintf("%s %s", method, path)
			routeQN := buildQN(opts.RepoURL, opts.FilePath, "route", routeName)
			routeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, routeName, "route")

			routeNode := types.Node{
				NodeHash:      routeHash,
				FileHash:      opts.FileHash,
				QualifiedName: routeQN,
				Kind:          "route",
				Line:          1,
			}
			result.Nodes = append(result.Nodes, routeNode)
			result.Edges = append(result.Edges, makeEdge(sourceHash, routeHash, "handles_route"))

		case "S3", "SQS", "SNS", "Schedule":
			srcName := fmt.Sprintf("%s-%s-%s", logicalID, eventName, eventType)
			srcQN := buildQN(opts.RepoURL, opts.FilePath, "event_source", srcName)
			srcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, srcName, "event_source")

			srcNode := types.Node{
				NodeHash:      srcHash,
				FileHash:      opts.FileHash,
				QualifiedName: srcQN,
				Kind:          "event_source",
				Line:          1,
			}
			result.Nodes = append(result.Nodes, srcNode)
			result.Edges = append(result.Edges, makeEdge(sourceHash, srcHash, "subscribes"))
		}
	}
}

