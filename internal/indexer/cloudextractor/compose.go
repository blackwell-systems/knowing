package cloudextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

// composeExtractor handles Docker Compose YAML files.
type composeExtractor struct{}

func (e *composeExtractor) name() string { return "docker-compose" }

// canHandle returns true if the file is a Docker Compose file.
func (e *composeExtractor) canHandle(path string, content []byte) bool {
	base := filepath.Base(path)
	baseLower := strings.ToLower(base)

	// Direct filename match
	if baseLower == "docker-compose.yml" || baseLower == "docker-compose.yaml" {
		return true
	}

	// Must be a YAML file for content-based detection
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}

	// Content-based: must have top-level "services" key plus "version" or "name"
	var top map[string]interface{}
	if err := yaml.Unmarshal(content, &top); err != nil {
		return false
	}
	_, hasServices := top["services"]
	if !hasServices {
		return false
	}
	_, hasVersion := top["version"]
	_, hasName := top["name"]
	return hasVersion || hasName
}

// extract parses a Docker Compose file and produces nodes and edges.
func (e *composeExtractor) extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	var top map[string]interface{}
	if err := yaml.Unmarshal(opts.Content, &top); err != nil {
		return result, nil
	}

	servicesRaw, ok := top["services"]
	if !ok {
		return result, nil
	}
	services, ok := servicesRaw.(map[string]interface{})
	if !ok {
		return result, nil
	}

	// First pass: create service nodes and build a hash map for edge resolution
	serviceHashes := make(map[string]types.Hash)
	for svcName := range services {
		qn := buildQN(opts.RepoURL, opts.FilePath, "service", svcName)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, svcName, "service")
		node := types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          "service",
			Line:          1,
		}
		result.Nodes = append(result.Nodes, node)
		serviceHashes[svcName] = nodeHash
	}

	// Track synthetic nodes to avoid duplicates
	portHashes := make(map[string]types.Hash)
	networkHashes := make(map[string]types.Hash)

	// Second pass: create edges from service definitions
	for svcName, svcRaw := range services {
		svcDef, ok := svcRaw.(map[string]interface{})
		if !ok {
			continue
		}
		svcHash := serviceHashes[svcName]

		// depends_on
		e.extractDependsOn(svcDef, svcHash, serviceHashes, result)

		// ports
		e.extractPorts(svcDef, opts, svcHash, portHashes, result)

		// links
		e.extractLinks(svcDef, svcHash, serviceHashes, result)

		// networks
		e.extractNetworks(svcDef, opts, svcHash, networkHashes, result)
	}

	return result, nil
}

// extractDependsOn handles both list and map forms of depends_on.
func (e *composeExtractor) extractDependsOn(svcDef map[string]interface{}, svcHash types.Hash, serviceHashes map[string]types.Hash, result *types.ExtractResult) {
	depsRaw, ok := svcDef["depends_on"]
	if !ok {
		return
	}

	var depNames []string
	switch deps := depsRaw.(type) {
	case []interface{}:
		for _, d := range deps {
			if name, ok := d.(string); ok {
				depNames = append(depNames, name)
			}
		}
	case map[string]interface{}:
		for name := range deps {
			depNames = append(depNames, name)
		}
	}

	for _, depName := range depNames {
		if targetHash, ok := serviceHashes[depName]; ok {
			edge := makeEdge(svcHash, targetHash, "depends_on")
			result.Edges = append(result.Edges, edge)
		}
	}
}

// extractPorts parses port mappings and creates port nodes + exposes edges.
func (e *composeExtractor) extractPorts(svcDef map[string]interface{}, opts types.ExtractOptions, svcHash types.Hash, portHashes map[string]types.Hash, result *types.ExtractResult) {
	portsRaw, ok := svcDef["ports"]
	if !ok {
		return
	}
	ports, ok := portsRaw.([]interface{})
	if !ok {
		return
	}

	for _, p := range ports {
		var portStr string
		switch v := p.(type) {
		case string:
			portStr = v
		case int:
			portStr = fmt.Sprintf("%d", v)
		}
		if portStr == "" {
			continue
		}

		// Extract host port from mapping like "8080:80" or just "8080"
		hostPort := portStr
		if idx := strings.Index(portStr, ":"); idx >= 0 {
			hostPort = portStr[:idx]
		}

		// Create or reuse port node
		if _, exists := portHashes[hostPort]; !exists {
			qn := buildQN(opts.RepoURL, opts.FilePath, "port", hostPort)
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, hostPort, "port")
			node := types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qn,
				Kind:          "port",
				Line:          1,
			}
			result.Nodes = append(result.Nodes, node)
			portHashes[hostPort] = nodeHash
		}

		edge := makeEdge(svcHash, portHashes[hostPort], "exposes")
		result.Edges = append(result.Edges, edge)
	}
}

// extractLinks parses links and creates connects_to edges to referenced services.
func (e *composeExtractor) extractLinks(svcDef map[string]interface{}, svcHash types.Hash, serviceHashes map[string]types.Hash, result *types.ExtractResult) {
	linksRaw, ok := svcDef["links"]
	if !ok {
		return
	}
	links, ok := linksRaw.([]interface{})
	if !ok {
		return
	}

	for _, l := range links {
		linkStr, ok := l.(string)
		if !ok {
			continue
		}
		// Parse "service:alias" format
		svcName := linkStr
		if idx := strings.Index(linkStr, ":"); idx >= 0 {
			svcName = linkStr[:idx]
		}
		if targetHash, ok := serviceHashes[svcName]; ok {
			edge := makeEdge(svcHash, targetHash, "connects_to")
			result.Edges = append(result.Edges, edge)
		}
	}
}

// extractNetworks creates network nodes and connects_to edges.
func (e *composeExtractor) extractNetworks(svcDef map[string]interface{}, opts types.ExtractOptions, svcHash types.Hash, networkHashes map[string]types.Hash, result *types.ExtractResult) {
	networksRaw, ok := svcDef["networks"]
	if !ok {
		return
	}

	var networkNames []string
	switch nets := networksRaw.(type) {
	case []interface{}:
		for _, n := range nets {
			if name, ok := n.(string); ok {
				networkNames = append(networkNames, name)
			}
		}
	case map[string]interface{}:
		for name := range nets {
			networkNames = append(networkNames, name)
		}
	}

	for _, netName := range networkNames {
		// Create or reuse network node
		if _, exists := networkHashes[netName]; !exists {
			qn := buildQN(opts.RepoURL, opts.FilePath, "network", netName)
			nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, netName, "network")
			node := types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qn,
				Kind:          "network",
				Line:          1,
			}
			result.Nodes = append(result.Nodes, node)
			networkHashes[netName] = nodeHash
		}

		edge := makeEdge(svcHash, networkHashes[netName], "connects_to")
		result.Edges = append(result.Edges, edge)
	}
}

