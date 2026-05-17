package cloudextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

// serverlessExtractor handles Serverless Framework configuration files
// (serverless.yml / serverless.yaml). It produces function nodes, route nodes,
// event_source nodes, and edges connecting functions to their event triggers.
type serverlessExtractor struct{}

func (e *serverlessExtractor) name() string { return "serverless" }

// canHandle returns true if the file is named serverless.yml or serverless.yaml
// and contains both top-level "service" and "provider" keys.
func (e *serverlessExtractor) canHandle(path string, content []byte) bool {
	base := filepath.Base(path)
	if base != "serverless.yml" && base != "serverless.yaml" {
		return false
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return false
	}

	_, hasService := raw["service"]
	_, hasProvider := raw["provider"]
	return hasService && hasProvider
}

// extract parses a Serverless Framework config and produces nodes and edges
// for functions, HTTP routes, and event subscriptions.
func (e *serverlessExtractor) extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &raw); err != nil {
		return result, nil
	}

	functionsRaw, ok := raw["functions"].(map[string]interface{})
	if !ok {
		return result, nil
	}

	for funcName, funcDef := range functionsRaw {
		funcMap, ok := funcDef.(map[string]interface{})
		if !ok {
			continue
		}

		// Create function node
		funcQN := buildQN(opts.RepoURL, opts.FilePath, "function", funcName)
		funcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, funcName, "function")
		funcNode := types.Node{
			NodeHash:      funcHash,
			FileHash:      opts.FileHash,
			QualifiedName: funcQN,
			Kind:          "function",
		}
		result.Nodes = append(result.Nodes, funcNode)

		// Process events
		events, ok := funcMap["events"].([]interface{})
		if !ok {
			continue
		}

		for _, evt := range events {
			evtMap, ok := evt.(map[string]interface{})
			if !ok {
				continue
			}

			for eventType, eventVal := range evtMap {
				switch eventType {
				case "http", "httpApi":
					e.extractHTTPEvent(opts, result, funcHash, eventVal)
				case "sqs":
					e.extractARNEvent(opts, result, funcHash, "sqs", eventVal)
				case "sns":
					e.extractARNEvent(opts, result, funcHash, "sns", eventVal)
				case "schedule":
					e.extractScheduleEvent(opts, result, funcHash, eventVal)
				case "s3":
					e.extractS3Event(opts, result, funcHash, eventVal)
				}
			}
		}
	}

	return result, nil
}

// extractHTTPEvent creates a route node and a handles_route edge from the function.
func (e *serverlessExtractor) extractHTTPEvent(opts types.ExtractOptions, result *types.ExtractResult, funcHash types.Hash, eventVal interface{}) {
	evtMap, ok := eventVal.(map[string]interface{})
	if !ok {
		return
	}

	path := getString(evtMap, "path")
	method := strings.ToUpper(getString(evtMap, "method"))
	if path == "" || method == "" {
		return
	}

	routeName := fmt.Sprintf("%s %s", method, path)
	routeQN := buildQN(opts.RepoURL, opts.FilePath, "route", routeName)
	routeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, routeName, "route")

	routeNode := types.Node{
		NodeHash:      routeHash,
		FileHash:      opts.FileHash,
		QualifiedName: routeQN,
		Kind:          "route",
	}
	result.Nodes = append(result.Nodes, routeNode)
	result.Edges = append(result.Edges, makeEdge(funcHash, routeHash, "handles_route"))
}

// extractARNEvent handles sqs and sns events, creating an event_source node
// and a subscribes edge from the function.
func (e *serverlessExtractor) extractARNEvent(opts types.ExtractOptions, result *types.ExtractResult, funcHash types.Hash, eventType string, eventVal interface{}) {
	var sourceName string

	switch v := eventVal.(type) {
	case string:
		sourceName = fmt.Sprintf("%s:%s", eventType, arnName(v))
	case map[string]interface{}:
		arn := getString(v, "arn")
		if arn != "" {
			sourceName = fmt.Sprintf("%s:%s", eventType, arnName(arn))
		} else {
			// Fallback: use the topicArn or queueArn field
			for _, key := range []string{"topicArn", "queueArn"} {
				if a := getString(v, key); a != "" {
					sourceName = fmt.Sprintf("%s:%s", eventType, arnName(a))
					break
				}
			}
		}
	}

	if sourceName == "" {
		sourceName = fmt.Sprintf("%s:unknown", eventType)
	}

	srcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, sourceName, "event_source")
	srcNode := types.Node{
		NodeHash:      srcHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "event_source", sourceName),
		Kind:          "event_source",
	}
	result.Nodes = append(result.Nodes, srcNode)
	result.Edges = append(result.Edges, makeEdge(funcHash, srcHash, "subscribes"))
}

// extractScheduleEvent creates an event_source node for a schedule expression
// and a subscribes edge from the function.
func (e *serverlessExtractor) extractScheduleEvent(opts types.ExtractOptions, result *types.ExtractResult, funcHash types.Hash, eventVal interface{}) {
	var expr string
	switch v := eventVal.(type) {
	case string:
		expr = v
	case map[string]interface{}:
		expr = getString(v, "rate")
		if expr == "" {
			expr = getString(v, "cron")
		}
	}

	if expr == "" {
		return
	}

	sourceName := fmt.Sprintf("schedule:%s", expr)
	srcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, sourceName, "event_source")
	srcNode := types.Node{
		NodeHash:      srcHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "event_source", sourceName),
		Kind:          "event_source",
	}
	result.Nodes = append(result.Nodes, srcNode)
	result.Edges = append(result.Edges, makeEdge(funcHash, srcHash, "subscribes"))
}

// extractS3Event creates an event_source node for an S3 bucket trigger
// and a subscribes edge from the function.
func (e *serverlessExtractor) extractS3Event(opts types.ExtractOptions, result *types.ExtractResult, funcHash types.Hash, eventVal interface{}) {
	var bucket string
	switch v := eventVal.(type) {
	case string:
		bucket = v
	case map[string]interface{}:
		bucket = getString(v, "bucket")
	}

	if bucket == "" {
		return
	}

	sourceName := fmt.Sprintf("s3:%s", bucket)
	srcHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, sourceName, "event_source")
	srcNode := types.Node{
		NodeHash:      srcHash,
		FileHash:      opts.FileHash,
		QualifiedName: buildQN(opts.RepoURL, opts.FilePath, "event_source", sourceName),
		Kind:          "event_source",
	}
	result.Nodes = append(result.Nodes, srcNode)
	result.Edges = append(result.Edges, makeEdge(funcHash, srcHash, "subscribes"))
}

// arnName extracts the resource name from an ARN string.
// For example, "arn:aws:sqs:us-east-1:123456:my-queue" returns "my-queue".
func arnName(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return arn
}

