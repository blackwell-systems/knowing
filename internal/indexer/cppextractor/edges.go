package cppextractor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractCallEdges walks a function/method body looking for call_expression
// nodes and creates call edges. Returns edges with call-site position info.
func extractCallEdges(body *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, compileEntry *CompileEntry) []types.Edge {
	if body == nil {
		return nil
	}

	var edges []types.Edge
	// Build a local symbol table from the file for same-file resolution.
	localSymbols := buildLocalSymbolTable(opts, basePath)

	walkForCalls(body, opts, basePath, sourceHash, localSymbols, &edges)
	return edges
}

// walkForCalls recursively walks AST nodes looking for call_expression nodes.
func walkForCalls(node *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, localSymbols map[string]types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		edge := resolveCallEdge(node, opts, basePath, sourceHash, localSymbols)
		if edge != nil {
			*edges = append(*edges, *edge)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, basePath, sourceHash, localSymbols, edges)
	}
}

// resolveCallEdge creates a call edge from a call_expression node.
func resolveCallEdge(callNode *sitter.Node, opts types.ExtractOptions, basePath string, sourceHash types.Hash, localSymbols map[string]types.Hash) *types.Edge {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil {
		return nil
	}

	targetName := extractCallTargetName(funcNode, opts.Content)
	if targetName == "" {
		return nil
	}

	// Strip template parameters for identity: foo<int> -> foo
	if idx := strings.Index(targetName, "<"); idx > 0 {
		targetName = targetName[:idx]
	}

	// Try same-file resolution first.
	provenance := "ast_inferred"
	confidence := 0.7
	targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, targetName, types.KindFunction)

	if hash, ok := localSymbols[targetName]; ok {
		targetHash = hash
		provenance = "ast_resolved"
		confidence = 0.85
	}

	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgetype.Calls, provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     edgetype.Calls,
		Confidence:   confidence,
		Provenance:   provenance,
		CallSiteLine: int(callNode.StartPoint().Row) + 1,
		CallSiteCol:  int(callNode.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractCallTargetName extracts the target function name from a call expression's
// function node. Handles: foo(), obj.method(), ns::func(), Class::method().
func extractCallTargetName(funcNode *sitter.Node, content []byte) string {
	switch funcNode.Type() {
	case "identifier":
		return funcNode.Content(content)
	case "field_expression":
		// obj.method -> method
		prop := funcNode.ChildByFieldName("field")
		if prop != nil {
			return prop.Content(content)
		}
	case "qualified_identifier":
		// ns::func -> func (strip namespace prefix for lookup)
		text := funcNode.Content(content)
		if idx := strings.LastIndex(text, "::"); idx >= 0 {
			return text[idx+2:]
		}
		return text
	case "template_function":
		// func<int>(...) -> func
		nameNode := funcNode.ChildByFieldName("name")
		if nameNode != nil {
			name := nameNode.Content(content)
			if idx := strings.Index(name, "<"); idx > 0 {
				return name[:idx]
			}
			return name
		}
	}

	// Fallback: walk children looking for an identifier.
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		child := funcNode.Child(i)
		if child.Type() == "identifier" || child.Type() == "field_identifier" {
			return child.Content(content)
		}
	}

	return ""
}

// extractInheritanceEdges extracts extends edges from a class/struct's
// base class list.
func extractInheritanceEdges(classNode *sitter.Node, opts types.ExtractOptions, basePath string, classHash types.Hash) []types.Edge {
	var edges []types.Edge

	// Look for base_class_clause child (tree-sitter C++ uses this name).
	for i := 0; i < int(classNode.ChildCount()); i++ {
		child := classNode.Child(i)
		if child.Type() == "base_class_clause" {
			for j := 0; j < int(child.ChildCount()); j++ {
				baseClass := child.Child(j)
				// The base class name is in a type_identifier or template_type.
				var baseName string
				switch baseClass.Type() {
				case "type_identifier":
					baseName = baseClass.Content(opts.Content)
				case "template_type":
					// Extract the base name from template_type (e.g. "Base<T>" -> "Base").
					typeId := findChildByType(baseClass, "type_identifier")
					if typeId != nil {
						baseName = typeId.Content(opts.Content)
					}
				}
				if baseName != "" {
					// Strip template parameters.
					if idx := strings.Index(baseName, "<"); idx > 0 {
						baseName = baseName[:idx]
					}
					// Strip namespace prefix for simple lookup.
					if idx := strings.LastIndex(baseName, "::"); idx >= 0 {
						baseName = baseName[idx+2:]
					}

					targetHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, baseName, types.KindType)
					prov := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(classHash, targetHash, edgetype.Extends, prov)
					edges = append(edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: classHash,
						TargetHash: targetHash,
						EdgeType:   edgetype.Extends,
						Confidence: 0.7,
						Provenance: prov,
					})
				}
			}
		}
	}

	return edges
}

// extractContainmentEdges creates contains + member_of edge pairs for
// structural containment (class -> method, class -> field, etc.).
func extractContainmentEdges(containerHash types.Hash, children []types.Node, edgeType string) []types.Edge {
	var edges []types.Edge

	for _, child := range children {
		// contains edge: container -> child
		containsHash := types.ComputeEdgeHash(containerHash, child.NodeHash, edgeType, "ast_resolved")
		edges = append(edges, types.Edge{
			EdgeHash:   containsHash,
			SourceHash: containerHash,
			TargetHash: child.NodeHash,
			EdgeType:   edgeType,
			Confidence: 0.85,
			Provenance: "ast_resolved",
		})

		// member_of edge: child -> container
		memberOfHash := types.ComputeEdgeHash(child.NodeHash, containerHash, edgetype.MemberOf, "ast_resolved")
		edges = append(edges, types.Edge{
			EdgeHash:   memberOfHash,
			SourceHash: child.NodeHash,
			TargetHash: containerHash,
			EdgeType:   edgetype.MemberOf,
			Confidence: 0.85,
			Provenance: "ast_resolved",
		})
	}

	return edges
}

// extractIncludeEdges extracts #include directives from the AST and creates
// includes edges. For resolved includes (when compileDB is available), the
// edge points to the included file's module node with higher confidence.
func extractIncludeEdges(root *sitter.Node, opts types.ExtractOptions, sourceHash types.Hash, basePath string, compileEntry *CompileEntry) []types.Edge {
	var edges []types.Edge

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "preproc_include" {
			edge := extractSingleInclude(child, opts, basePath, compileEntry)
			if edge != nil {
				edges = append(edges, *edge)
			}
		}
	}

	return edges
}

// extractSingleInclude processes a single preproc_include node.
func extractSingleInclude(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) *types.Edge {
	// The path is in the "path" field.
	pathNode := node.ChildByFieldName("path")
	if pathNode == nil {
		return nil
	}

	includePath := pathNode.Content(opts.Content)
	// Strip quotes or angle brackets.
	includePath = strings.Trim(includePath, `"<>'`)
	if includePath == "" {
		return nil
	}

	// Determine if this is a system include (angle brackets) or local (quotes).
	isSystem := strings.HasPrefix(node.Content(opts.Content), "#include <")

	// Try to resolve the include to an actual file.
	resolvedPath, confidence, provenance := resolveInclude(includePath, opts, compileEntry, isSystem)

	// Compute target hash. For resolved includes, use the resolved path as module.
	targetModule := includePath
	if resolvedPath != "" {
		targetModule = computeIncludeModulePath(resolvedPath, opts.ModuleRoot)
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, targetModule, types.EmptyHash, filepath.Base(includePath), types.KindFile)

	// Compute file-level hash for the source (the file containing the #include).
	srcHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, filepath.Base(opts.FilePath), types.KindFile)

	edgeHash := types.ComputeEdgeHash(srcHash, targetHash, edgetype.Includes, provenance)

	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: srcHash,
		TargetHash: targetHash,
		EdgeType:   edgetype.Includes,
		Confidence: confidence,
		Provenance: provenance,
	}
}

// resolveInclude tries to resolve an include path to an actual file.
// Returns (resolvedPath, confidence, provenance).
func resolveInclude(includePath string, opts types.ExtractOptions, compileEntry *CompileEntry, isSystem bool) (string, float64, string) {
	// Without compile commands, we can't resolve.
	if compileEntry == nil {
		return "", 0.7, "ast_inferred"
	}

	// For quoted includes, first try relative to the current file's directory.
	if !isSystem {
		dir := filepath.Dir(opts.FilePath)
		relativeCandidate := filepath.Join(dir, includePath)
		// Check if the file exists relative to the module root.
		absCandidate := filepath.Join(opts.ModuleRoot, relativeCandidate)
		if fileExists(absCandidate) {
			return relativeCandidate, 0.85, "ast_resolved"
		}
	}

	// Search include paths.
	for _, incPath := range compileEntry.IncludePaths {
		candidate := filepath.Join(incPath, includePath)
		if fileExists(candidate) {
			// Make path relative to module root if possible.
			rel, err := filepath.Rel(opts.ModuleRoot, candidate)
			if err == nil {
				return rel, 0.85, "ast_resolved"
			}
			return candidate, 0.85, "ast_resolved"
		}
	}

	// Unresolved — still emit the edge with lower confidence.
	return "", 0.7, "ast_inferred"
}

// computeIncludeModulePath computes the module path for an included file.
func computeIncludeModulePath(resolvedPath, moduleRoot string) string {
	// Strip extension to get module path.
	base := strings.TrimSuffix(resolvedPath, filepath.Ext(resolvedPath))
	return filepath.ToSlash(base)
}

// fileExists checks if a file exists at the given path.
func fileExists(path string) bool {
	info, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	_, err = os.Stat(info)
	return err == nil
}

// buildLocalSymbolTable builds a map from symbol names to their hashes for
// all nodes in the current file. Used for same-file call resolution.
func buildLocalSymbolTable(opts types.ExtractOptions, basePath string) map[string]types.Hash {
	symbols := make(map[string]types.Hash)

	// We don't have access to the already-extracted nodes here, so we
	// build a simple name -> hash mapping from the qualified name prefix.
	// The actual resolution happens in resolveCallEdge which checks this map.
	// For now, this is a placeholder that enables same-file resolution when
	// the caller populates it.

	return symbols
}

// BuildLocalSymbolTable creates a local symbol table from a list of extracted
// nodes. This is called by the extractor after symbol extraction to enable
// same-file call resolution.
func BuildLocalSymbolTable(nodes []types.Node) map[string]types.Hash {
	symbols := make(map[string]types.Hash, len(nodes))
	for _, n := range nodes {
		// Extract the simple name from the qualified name.
		name := extractSimpleName(n.QualifiedName)
		if name != "" {
			symbols[name] = n.NodeHash
		}
	}
	return symbols
}

// extractSimpleName extracts the last component of a qualified name.
// "repo://root/src/main.cpp.foo" -> "foo"
// "repo://root/src/main.cpp.Foo.bar" -> "bar"
func extractSimpleName(qname string) string {
	// Split by "." and take the last component.
	parts := strings.Split(qname, ".")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// FormatCallEdge is a helper for debugging — formats a call edge as a string.
func FormatCallEdge(e types.Edge) string {
	return fmt.Sprintf("calls %s -> %s (confidence=%.2f, provenance=%s)",
		e.SourceHash.String()[:8], e.TargetHash.String()[:8],
		e.Confidence, e.Provenance)
}
