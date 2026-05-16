// Package k8sextractor provides a YAML-based extractor for Kubernetes manifest
// files. It parses .yaml/.yml files in known Kubernetes directories and produces
// nodes and edges representing Kubernetes resources and their relationships.
package k8sextractor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
	"gopkg.in/yaml.v3"
)

const (
	provenance = "ast_inferred"
	confidence = 0.7
)

// k8sDirs are directory path segments that indicate Kubernetes manifests.
var k8sDirs = []string{"kubernetes/", "k8s/", "manifests/", "deploy/", "helm/"}

// K8sExtractor extracts nodes and edges from Kubernetes YAML manifests.
type K8sExtractor struct{}

// NewK8sExtractor creates a new K8sExtractor instance.
func NewK8sExtractor() *K8sExtractor {
	return &K8sExtractor{}
}

// Name returns the identifier for this extractor.
func (e *K8sExtractor) Name() string {
	return "k8s-yaml"
}

// CanHandle returns true if the file is a YAML file in a known Kubernetes directory.
func (e *K8sExtractor) CanHandle(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	normalized := filepath.ToSlash(path) + "/"
	for _, dir := range k8sDirs {
		if strings.Contains(normalized, "/"+dir) || strings.HasPrefix(normalized, dir) {
			return true
		}
	}
	return false
}

// Extract parses Kubernetes YAML manifests and produces nodes and edges.
func (e *K8sExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	result := &types.ExtractResult{}

	docs, err := splitYAMLDocuments(opts.Content)
	if err != nil {
		return result, nil
	}

	// First pass: collect all resources for cross-referencing
	var resources []k8sResource
	for _, doc := range docs {
		res, parseErr := parseResource(doc)
		if parseErr != nil || res.Kind == "" {
			continue
		}
		resources = append(resources, res)
	}

	// Create nodes for each resource
	nodeMap := make(map[string]types.Hash) // key: "kind/namespace/name" -> nodeHash
	for _, res := range resources {
		kind := normalizeKind(res.Kind)
		qn := buildQualifiedName(opts.RepoURL, opts.FilePath, kind, res.Namespace, res.Name)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, fmt.Sprintf("%s/%s", res.Namespace, res.Name), kind)

		node := types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          kind,
			Line:          res.Line,
		}
		result.Nodes = append(result.Nodes, node)
		nodeMap[fmt.Sprintf("%s/%s/%s", kind, res.Namespace, res.Name)] = nodeHash
	}

	// Second pass: create edges from relationships
	for _, res := range resources {
		kind := normalizeKind(res.Kind)
		resKey := fmt.Sprintf("%s/%s/%s", kind, res.Namespace, res.Name)
		resHash := nodeMap[resKey]

		switch kind {
		case "service":
			// Service selector -> Deployment: "deploys" edge
			if res.Selector != nil {
				for _, target := range resources {
					targetKind := normalizeKind(target.Kind)
					if isWorkload(targetKind) && labelsMatchSelector(target.Labels, res.Selector) {
						targetKey := fmt.Sprintf("%s/%s/%s", targetKind, target.Namespace, target.Name)
						if targetHash, ok := nodeMap[targetKey]; ok {
							edge := makeEdge(resHash, targetHash, "deploys")
							result.Edges = append(result.Edges, edge)
						}
					}
				}
			}

		case "ingress":
			// Ingress -> Service: "exposes" edge
			for _, svcName := range res.BackendServices {
				svcKey := fmt.Sprintf("service/%s/%s", res.Namespace, svcName)
				if svcHash, ok := nodeMap[svcKey]; ok {
					edge := makeEdge(resHash, svcHash, "exposes")
					result.Edges = append(result.Edges, edge)
				}
			}

		case "deployment", "statefulset", "daemonset", "job", "cronjob":
			// ConfigMap/Secret -> Deployment: "configures" edge
			for _, cmName := range res.ConfigMapRefs {
				cmKey := fmt.Sprintf("configmap/%s/%s", res.Namespace, cmName)
				if cmHash, ok := nodeMap[cmKey]; ok {
					edge := makeEdge(cmHash, resHash, "configures")
					result.Edges = append(result.Edges, edge)
				}
			}
		}
		_ = resHash // used above
	}

	return result, nil
}

// k8sResource holds parsed metadata from a Kubernetes manifest document.
type k8sResource struct {
	Kind            string
	Name            string
	Namespace       string
	Labels          map[string]string
	Selector        map[string]string
	BackendServices []string
	ConfigMapRefs   []string
	Line            int
}

// splitYAMLDocuments splits multi-document YAML into individual documents.
func splitYAMLDocuments(content []byte) ([][]byte, error) {
	var docs [][]byte
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed documents
			continue
		}
		// Re-encode the node to get its bytes
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if encErr := enc.Encode(&node); encErr != nil {
			continue
		}
		enc.Close()
		docs = append(docs, buf.Bytes())
	}
	return docs, nil
}

// parseResource parses a single YAML document into a k8sResource.
func parseResource(doc []byte) (k8sResource, error) {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(doc, &raw); err != nil {
		return k8sResource{}, err
	}

	res := k8sResource{
		Kind: getString(raw, "kind"),
		Line: 1,
	}

	// Parse metadata
	if meta, ok := raw["metadata"].(map[string]interface{}); ok {
		res.Name = getString(meta, "name")
		res.Namespace = getString(meta, "namespace")
		if res.Namespace == "" {
			res.Namespace = "default"
		}
		if labels, ok := meta["labels"].(map[string]interface{}); ok {
			res.Labels = toStringMap(labels)
		}
	}

	// Parse spec for selectors (Service)
	if spec, ok := raw["spec"].(map[string]interface{}); ok {
		if sel, ok := spec["selector"].(map[string]interface{}); ok {
			// Service uses spec.selector directly as labels
			if res.Kind == "Service" {
				res.Selector = toStringMap(sel)
			} else if matchLabels, ok := sel["matchLabels"].(map[string]interface{}); ok {
				// Deployments use spec.selector.matchLabels
				res.Selector = toStringMap(matchLabels)
			}
		}

		// Parse Ingress rules for backend services
		if strings.EqualFold(res.Kind, "Ingress") {
			res.BackendServices = extractIngressBackends(spec)
		}
	}

	// Parse pod template for ConfigMap/Secret references
	kind := normalizeKind(res.Kind)
	if isWorkload(kind) {
		res.ConfigMapRefs = extractConfigMapRefs(raw)
	}

	return res, nil
}

// extractIngressBackends finds service names referenced in Ingress rules.
func extractIngressBackends(spec map[string]interface{}) []string {
	var services []string
	seen := make(map[string]bool)

	// Check default backend
	if backend, ok := spec["defaultBackend"].(map[string]interface{}); ok {
		if svc, ok := backend["service"].(map[string]interface{}); ok {
			if name := getString(svc, "name"); name != "" && !seen[name] {
				services = append(services, name)
				seen[name] = true
			}
		}
		// Legacy format
		if name := getString(backend, "serviceName"); name != "" && !seen[name] {
			services = append(services, name)
			seen[name] = true
		}
	}

	// Check rules
	if rules, ok := spec["rules"].([]interface{}); ok {
		for _, rule := range rules {
			ruleMap, ok := rule.(map[string]interface{})
			if !ok {
				continue
			}
			if http, ok := ruleMap["http"].(map[string]interface{}); ok {
				if paths, ok := http["paths"].([]interface{}); ok {
					for _, p := range paths {
						pathMap, ok := p.(map[string]interface{})
						if !ok {
							continue
						}
						if backend, ok := pathMap["backend"].(map[string]interface{}); ok {
							// v1 networking API
							if svc, ok := backend["service"].(map[string]interface{}); ok {
								if name := getString(svc, "name"); name != "" && !seen[name] {
									services = append(services, name)
									seen[name] = true
								}
							}
							// Legacy format
							if name := getString(backend, "serviceName"); name != "" && !seen[name] {
								services = append(services, name)
								seen[name] = true
							}
						}
					}
				}
			}
		}
	}

	return services
}

// extractConfigMapRefs finds ConfigMap and Secret references in pod templates.
func extractConfigMapRefs(raw map[string]interface{}) []string {
	var refs []string
	seen := make(map[string]bool)

	// Navigate to pod spec: spec.template.spec for workloads
	spec, ok := raw["spec"].(map[string]interface{})
	if !ok {
		return nil
	}
	template, ok := spec["template"].(map[string]interface{})
	if !ok {
		return nil
	}
	podSpec, ok := template["spec"].(map[string]interface{})
	if !ok {
		return nil
	}

	// Check volumes for configMap references
	if volumes, ok := podSpec["volumes"].([]interface{}); ok {
		for _, v := range volumes {
			vol, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			if cm, ok := vol["configMap"].(map[string]interface{}); ok {
				if name := getString(cm, "name"); name != "" && !seen[name] {
					refs = append(refs, name)
					seen[name] = true
				}
			}
			if secret, ok := vol["secret"].(map[string]interface{}); ok {
				if name := getString(secret, "secretName"); name != "" && !seen[name] {
					refs = append(refs, name)
					seen[name] = true
				}
			}
		}
	}

	// Check containers for envFrom
	if containers, ok := podSpec["containers"].([]interface{}); ok {
		for _, c := range containers {
			container, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if envFromList, ok := container["envFrom"].([]interface{}); ok {
				for _, ef := range envFromList {
					envFrom, ok := ef.(map[string]interface{})
					if !ok {
						continue
					}
					if cmRef, ok := envFrom["configMapRef"].(map[string]interface{}); ok {
						if name := getString(cmRef, "name"); name != "" && !seen[name] {
							refs = append(refs, name)
							seen[name] = true
						}
					}
					if secretRef, ok := envFrom["secretRef"].(map[string]interface{}); ok {
						if name := getString(secretRef, "name"); name != "" && !seen[name] {
							refs = append(refs, name)
							seen[name] = true
						}
					}
				}
			}
		}
	}

	return refs
}

// normalizeKind maps Kubernetes resource kinds to node kinds.
func normalizeKind(kind string) string {
	switch strings.ToLower(kind) {
	case "deployment":
		return "deployment"
	case "statefulset":
		return "statefulset"
	case "daemonset":
		return "daemonset"
	case "job":
		return "job"
	case "cronjob":
		return "cronjob"
	case "service":
		return "service"
	case "configmap":
		return "configmap"
	case "secret":
		return "configmap" // secrets produce "configmap" kind nodes per spec
	case "ingress":
		return "ingress"
	default:
		return strings.ToLower(kind)
	}
}

// isWorkload returns true if the kind is a workload that can reference ConfigMaps.
func isWorkload(kind string) bool {
	switch kind {
	case "deployment", "statefulset", "daemonset", "job", "cronjob":
		return true
	}
	return false
}

// buildQualifiedName constructs the qualified name for a K8s resource node.
// Format: {repoURL}://{filePath}.{kind}.{namespace}/{name}
func buildQualifiedName(repoURL, filePath, kind, namespace, name string) string {
	return fmt.Sprintf("%s://%s.%s.%s/%s", repoURL, filePath, kind, namespace, name)
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

// labelsMatchSelector checks if all selector key-value pairs exist in labels.
func labelsMatchSelector(labels, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
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

// toStringMap converts a map[string]interface{} to map[string]string.
func toStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}
