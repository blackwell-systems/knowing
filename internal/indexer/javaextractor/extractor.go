// Package javaextractor provides a tree-sitter based extractor for Java files.
// It implements types.Extractor and produces declaration nodes and syntactic
// call/import edges without full semantic analysis.
//
// This extractor handles classes, interfaces, enums, methods, constructors,
// imports, method invocations, object creation expressions, and Spring
// framework route annotations. Edges have provenance "ast_inferred" and
// confidence 0.7 since tree-sitter parsing does not provide full type
// resolution for Java.
package javaextractor

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/blackwell-systems/knowing/internal/types"
)

// JavaExtractor implements types.Extractor for Java files using tree-sitter
// AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type JavaExtractor struct{}

// NewJavaExtractor creates a new JavaExtractor.
func NewJavaExtractor() *JavaExtractor {
	return &JavaExtractor{}
}

// Name returns the extractor name.
func (e *JavaExtractor) Name() string {
	return "treesitter-java"
}

// CanHandle returns true for .java files, excluding files in build/ and
// target/ directories (Maven/Gradle output).
func (e *JavaExtractor) CanHandle(path string) bool {
	if !strings.HasSuffix(path, ".java") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, p := range parts {
		if p == "build" || p == "target" {
			return false
		}
	}
	return true
}

// springAnnotations maps Spring route annotation names to their HTTP method.
var springAnnotations = map[string]string{
	"RequestMapping": "ANY",
	"GetMapping":     "GET",
	"PostMapping":    "POST",
	"PutMapping":     "PUT",
	"DeleteMapping":  "DELETE",
	"PatchMapping":   "PATCH",
}

// Extract parses the Java file with tree-sitter and produces nodes for
// declarations and edges for calls and imports.
func (e *JavaExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(java.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	// Extract Java package declaration from AST for proper qualified names.
	pkgPath := extractJavaPackage(root, opts)

	// Build Java import map for cross-file call resolution.
	javaImports := buildJavaImportMap(root, opts)

	var nodes []types.Node
	var edges []types.Edge

	// Walk top-level children. Java files typically contain:
	// program -> [package_declaration, import_declaration*, class_declaration*]
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "class_declaration":
			cn, ce := extractClassDeclWithImports(child, opts, pkgPath, "", javaImports)
			nodes = append(nodes, cn...)
			edges = append(edges, ce...)

		case "interface_declaration":
			in, ie := extractInterfaceDecl(child, opts, pkgPath)
			nodes = append(nodes, in...)
			edges = append(edges, ie...)

		case "enum_declaration":
			en := extractEnumDecl(child, opts, pkgPath)
			nodes = append(nodes, en...)

		case "import_declaration":
			ie := extractImportEdge(child, opts, pkgPath)
			if ie != nil {
				edges = append(edges, *ie)
			}
		}
	}

	// Sort nodes by QualifiedName then Kind.
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].QualifiedName != nodes[j].QualifiedName {
			return nodes[i].QualifiedName < nodes[j].QualifiedName
		}
		return nodes[i].Kind < nodes[j].Kind
	})

	// Sort edges by SourceHash, TargetHash, EdgeType.
	sort.Slice(edges, func(i, j int) bool {
		si, sj := edges[i], edges[j]
		if si.SourceHash != sj.SourceHash {
			return si.SourceHash.String() < sj.SourceHash.String()
		}
		if si.TargetHash != sj.TargetHash {
			return si.TargetHash.String() < sj.TargetHash.String()
		}
		return si.EdgeType < sj.EdgeType
	})

	// Deduplicate edges by EdgeHash.
	edges = deduplicateEdges(edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// extractJavaPackage extracts the package declaration from the AST root.
// Returns the Java package path (e.g., "org.springframework.samples.petclinic.owner").
// Falls back to file-path-based package if no declaration found.
func extractJavaPackage(root *sitter.Node, opts types.ExtractOptions) string {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "package_declaration" {
			// package_declaration has a child that is the scoped_identifier or identifier.
			for j := 0; j < int(child.ChildCount()); j++ {
				pkgNode := child.Child(j)
				if pkgNode.Type() == "scoped_identifier" || pkgNode.Type() == "identifier" {
					return pkgNode.Content(opts.Content)
				}
			}
		}
	}
	// Fallback: use directory path.
	dir := filepath.Dir(opts.FilePath)
	if dir == "." {
		return opts.RepoURL
	}
	return opts.RepoURL + "/" + filepath.ToSlash(dir)
}

// buildJavaImportMap extracts all import declarations from a Java AST root and
// builds a map from simple class/type names to their fully-qualified package path.
// Handles: import com.pkg.Class (maps "Class" -> "com.pkg"),
// import static com.pkg.Class.method (maps "Class" -> "com.pkg").
// Wildcard imports (import com.pkg.*) are skipped (cannot resolve individual names).
func buildJavaImportMap(root *sitter.Node, opts types.ExtractOptions) map[string]string {
	imports := make(map[string]string)

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child == nil || child.Type() != "import_declaration" {
			continue
		}

		text := strings.TrimSpace(child.Content(opts.Content))
		text = strings.TrimPrefix(text, "import ")
		isStatic := strings.HasPrefix(text, "static ")
		text = strings.TrimPrefix(text, "static ")
		text = strings.TrimSuffix(text, ";")
		text = strings.TrimSpace(text)

		if text == "" {
			continue
		}

		// Skip wildcard imports (import com.example.model.*).
		if strings.HasSuffix(text, ".*") {
			continue
		}

		lastDot := strings.LastIndex(text, ".")
		if lastDot < 0 {
			continue
		}

		if isStatic {
			// For static imports like "com.example.util.MathHelper.calculate",
			// the last segment is the method name; we want the class name
			// (second-to-last segment).
			packagePath := text[:lastDot]
			secondLastDot := strings.LastIndex(packagePath, ".")
			if secondLastDot < 0 {
				// Single-segment package with a class name: e.g., "MathHelper.calculate"
				// className = packagePath, classPackage = ""
				imports[packagePath] = ""
				continue
			}
			className := packagePath[secondLastDot+1:]
			classPackage := packagePath[:secondLastDot]
			imports[className] = classPackage
		} else {
			// Regular import: "com.example.service.UserService"
			simpleName := text[lastDot+1:]
			packagePath := text[:lastDot]
			imports[simpleName] = packagePath
		}
	}

	return imports
}

// extractMethodInvocationWithImports creates a call edge for a method_invocation node,
// using the import map to resolve cross-file targets. When the method_invocation has
// an object field whose name matches an imported class (uppercase first letter heuristic),
// resolves the target hash against the import's qualified path with provenance
// "ast_resolved" and confidence 0.85.
func extractMethodInvocationWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, javaImports map[string]string) *types.Edge {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	methodName := nameNode.Content(opts.Content)

	objectNode := node.ChildByFieldName("object")
	targetName := methodName
	targetBasePath := pkgPath
	edgeProvenance := "ast_inferred"
	edgeConfidence := 0.7

	if objectNode != nil {
		objectName := objectNode.Content(opts.Content)
		targetName = objectName + "." + methodName

		// Class reference heuristic: first character is uppercase.
		if len(objectName) > 0 && unicode.IsUpper(rune(objectName[0])) {
			if importPath, ok := javaImports[objectName]; ok {
				targetBasePath = importPath
				edgeProvenance = "ast_resolved"
				edgeConfidence = 0.85
			}
		}
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, targetBasePath, types.EmptyHash, targetName, "method")
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", edgeProvenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   edgeConfidence,
		Provenance:   edgeProvenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractCallEdgesWithImports recursively walks a node tree looking for method
// invocations and object creation expressions, producing call edges with
// import-aware resolution.
func extractCallEdgesWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, javaImports map[string]string) []types.Edge {
	if node == nil {
		return nil
	}
	var edges []types.Edge
	walkForCallsWithImports(node, opts, pkgPath, sourceHash, javaImports, &edges)
	return edges
}

// walkForCallsWithImports recursively visits nodes looking for call expressions,
// using the import map to resolve cross-file targets.
func walkForCallsWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, javaImports map[string]string, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "method_invocation":
		edge := extractMethodInvocationWithImports(node, opts, pkgPath, sourceHash, javaImports)
		if edge != nil {
			*edges = append(*edges, *edge)
		}

	case "object_creation_expression":
		edge := extractObjectCreation(node, opts, pkgPath, sourceHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCallsWithImports(node.Child(i), opts, pkgPath, sourceHash, javaImports, edges)
	}
}

// extractClassDecl extracts a class declaration and its members (methods,
// constructors). parentClass is non-empty for nested classes.
// Delegates to extractClassDeclWithImports with a nil import map.
func extractClassDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath, parentClass string) ([]types.Node, []types.Edge) {
	return extractClassDeclWithImports(node, opts, pkgPath, parentClass, nil)
}

// extractClassDeclWithImports extracts a class declaration and its members,
// passing the Java import map through for cross-file call resolution.
func extractClassDeclWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath, parentClass string, javaImports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	className := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedClass := className
	if parentClass != "" {
		qualifiedClass = parentClass + "." + className
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qualifiedClass, "type")
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, qualifiedClass),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("class %s", className),
	})

	// Emit extends edge if there is a superclass field.
	extractJavaSuperclass(node, opts, pkgPath, nodeHash, &edges)

	// Emit implements edges if there is a super_interfaces field.
	extractJavaInterfaces(node, opts, pkgPath, nodeHash, &edges)

	// Emit decorates edges for annotations on the class.
	extractJavaAnnotationEdges(node, opts, pkgPath, nodeHash, &edges)

	// Extract Spring route annotations on the class itself (e.g., @RequestMapping on class).
	classRoutePrefix := extractAnnotationRoute(node, opts.Content)

	// Walk the class body for methods, constructors, nested classes.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			member := body.Child(i)
			switch member.Type() {
			case "method_declaration":
				mn, me := extractMethodDeclWithImports(member, opts, pkgPath, qualifiedClass, classRoutePrefix, javaImports)
				nodes = append(nodes, mn...)
				edges = append(edges, me...)

			case "constructor_declaration":
				cn, ce := extractConstructorDeclWithImports(member, opts, pkgPath, qualifiedClass, javaImports)
				nodes = append(nodes, cn...)
				edges = append(edges, ce...)

			case "class_declaration":
				// Nested class.
				nn, ne := extractClassDeclWithImports(member, opts, pkgPath, qualifiedClass, javaImports)
				nodes = append(nodes, nn...)
				edges = append(edges, ne...)
			}
		}
	}

	return nodes, edges
}

// extractInterfaceDecl extracts an interface declaration and its method signatures.
func extractInterfaceDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, "interface")
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name),
		Kind:          "interface",
		Line:          line,
		Signature:     fmt.Sprintf("interface %s", name),
	})

	// Walk body for method signatures.
	body := node.ChildByFieldName("body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			member := body.Child(i)
			if member.Type() == "method_declaration" {
				mn, me := extractMethodDecl(member, opts, pkgPath, name, "")
				nodes = append(nodes, mn...)
				edges = append(edges, me...)
			}
		}
	}

	return nodes, edges
}

// extractEnumDecl extracts an enum declaration.
func extractEnumDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath string) []types.Node {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, name, "type")
	return []types.Node{{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, name),
		Kind:          "type",
		Line:          line,
		Signature:     fmt.Sprintf("enum %s", name),
	}}
}

// extractMethodDecl extracts a method declaration inside a class or interface.
// It also detects Spring route annotations on the method and produces
// route_handler nodes and handles_route edges.
func extractMethodDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath, className, classRoutePrefix string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	methodName := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedName := className + "." + methodName
	kind := "method"

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qualifiedName, kind)
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, qualifiedName),
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("%s.%s()", className, methodName),
	})

	// Emit overrides edge if @Override annotation is present.
	extractJavaOverrideEdge(node, opts, pkgPath, className, methodName, nodeHash, &edges)

	// Emit decorates edges for annotations on this method.
	extractJavaAnnotationEdges(node, opts, pkgPath, nodeHash, &edges)

	// Check for Spring route annotations on this method.
	routeAnnotation, httpMethod := extractSpringAnnotation(node, opts.Content)
	if routeAnnotation != "" {
		routePath := classRoutePrefix + routeAnnotation
		routePattern := httpMethod + " " + routePath
		routeNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, routePattern, "route_handler")
		nodes = append(nodes, types.Node{
			NodeHash:      routeNodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, routePattern),
			Kind:          "route_handler",
			Signature:     routePattern,
		})

		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(routeNodeHash, nodeHash, "handles_route", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: routeNodeHash,
			TargetHash: nodeHash,
			EdgeType:   "handles_route",
			Confidence: 0.7,
			Provenance: provenance,
		})
	}

	// Walk the method body for call edges.
	body := node.ChildByFieldName("body")
	if body != nil {
		callEdges := extractCallEdges(body, opts, pkgPath, nodeHash)
		edges = append(edges, callEdges...)
	}

	return nodes, edges
}

// extractConstructorDecl extracts a constructor declaration.
func extractConstructorDecl(node *sitter.Node, opts types.ExtractOptions, pkgPath, className string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedName := className + ".<init>"
	kind := "method"

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qualifiedName, kind)
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, qualifiedName),
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("%s()", className),
	})

	// Walk the constructor body for call edges.
	body := node.ChildByFieldName("body")
	if body != nil {
		callEdges := extractCallEdges(body, opts, pkgPath, nodeHash)
		edges = append(edges, callEdges...)
	}

	return nodes, edges
}

// extractMethodDeclWithImports extracts a method declaration with import-aware
// call edge resolution.
func extractMethodDeclWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath, className, classRoutePrefix string, javaImports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	methodName := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedName := className + "." + methodName
	kind := "method"

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qualifiedName, kind)
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, qualifiedName),
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("%s.%s()", className, methodName),
	})

	// Emit overrides edge if @Override annotation is present.
	extractJavaOverrideEdge(node, opts, pkgPath, className, methodName, nodeHash, &edges)

	// Emit decorates edges for annotations on this method.
	extractJavaAnnotationEdges(node, opts, pkgPath, nodeHash, &edges)

	// Check for Spring route annotations on this method.
	routeAnnotation, httpMethod := extractSpringAnnotation(node, opts.Content)
	if routeAnnotation != "" {
		routePath := classRoutePrefix + routeAnnotation
		routePattern := httpMethod + " " + routePath
		routeNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, routePattern, "route_handler")
		nodes = append(nodes, types.Node{
			NodeHash:      routeNodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, routePattern),
			Kind:          "route_handler",
			Signature:     routePattern,
		})

		provenance := "ast_inferred"
		edgeHash := types.ComputeEdgeHash(routeNodeHash, nodeHash, "handles_route", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: routeNodeHash,
			TargetHash: nodeHash,
			EdgeType:   "handles_route",
			Confidence: 0.7,
			Provenance: provenance,
		})
	}

	// Walk the method body for call edges with import resolution.
	body := node.ChildByFieldName("body")
	if body != nil {
		callEdges := extractCallEdgesWithImports(body, opts, pkgPath, nodeHash, javaImports)
		edges = append(edges, callEdges...)
	}

	return nodes, edges
}

// extractConstructorDeclWithImports extracts a constructor declaration with
// import-aware call edge resolution.
func extractConstructorDeclWithImports(node *sitter.Node, opts types.ExtractOptions, pkgPath, className string, javaImports map[string]string) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nodes, edges
	}
	line := int(nameNode.StartPoint().Row) + 1

	qualifiedName := className + ".<init>"
	kind := "method"

	nodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, qualifiedName, kind)
	nodes = append(nodes, types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: fmt.Sprintf("%s://%s.%s", opts.RepoURL, pkgPath, qualifiedName),
		Kind:          kind,
		Line:          line,
		Signature:     fmt.Sprintf("%s()", className),
	})

	// Walk the constructor body for call edges with import resolution.
	body := node.ChildByFieldName("body")
	if body != nil {
		callEdges := extractCallEdgesWithImports(body, opts, pkgPath, nodeHash, javaImports)
		edges = append(edges, callEdges...)
	}

	return nodes, edges
}

// extractImportEdge extracts an import declaration and produces an import edge.
func extractImportEdge(node *sitter.Node, opts types.ExtractOptions, pkgPath string) *types.Edge {
	// In Java tree-sitter grammar, import_declaration contains the full
	// scoped identifier as child nodes. We get the text content directly.
	importText := strings.TrimSpace(node.Content(opts.Content))
	importText = strings.TrimPrefix(importText, "import ")
	importText = strings.TrimPrefix(importText, "static ")
	importText = strings.TrimSuffix(importText, ";")
	importText = strings.TrimSpace(importText)

	if importText == "" {
		return nil
	}

	fileNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, filepath.Base(opts.FilePath), "file")

	// The import target is hashed as a package node.
	importHash := types.ComputeNodeHash(opts.RepoURL, importText, types.EmptyHash, importText, "package")

	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(fileNodeHash, importHash, "imports", provenance)
	return &types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: fileNodeHash,
		TargetHash: importHash,
		EdgeType:   "imports",
		Confidence: 0.7,
		Provenance: provenance,
	}
}

// extractCallEdges recursively walks a node tree looking for method invocations
// and object creation expressions, producing call edges.
func extractCallEdges(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) []types.Edge {
	if node == nil {
		return nil
	}
	var edges []types.Edge
	walkForCalls(node, opts, pkgPath, sourceHash, &edges)
	return edges
}

// walkForCalls recursively visits nodes looking for call expressions.
func walkForCalls(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "method_invocation":
		edge := extractMethodInvocation(node, opts, pkgPath, sourceHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}

	case "object_creation_expression":
		edge := extractObjectCreation(node, opts, pkgPath, sourceHash)
		if edge != nil {
			*edges = append(*edges, *edge)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForCalls(node.Child(i), opts, pkgPath, sourceHash, edges)
	}
}

// extractJavaThrowsClause extracts throws edges from the "throws" clause in a
// Java method signature. Tree-sitter represents the thrown types as children of
// a "throws" node within the method_declaration.
func extractJavaThrowsClause(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) []types.Edge {
	var edges []types.Edge
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "throws" {
			// The throws clause contains type_identifier children for each exception type.
			for j := 0; j < int(child.ChildCount()); j++ {
				typeNode := child.Child(j)
				if typeNode.Type() == "type_identifier" || typeNode.Type() == "scoped_type_identifier" {
					exceptionType := typeNode.Content(opts.Content)
					if exceptionType != "" {
						targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, exceptionType, "error")
						provenance := "ast_inferred"
						edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
						edges = append(edges, types.Edge{
							EdgeHash:   edgeHash,
							SourceHash: sourceHash,
							TargetHash: targetHash,
							EdgeType:   "throws",
							Confidence: 0.7,
							Provenance: provenance,
						})
					}
				}
			}
		}
	}
	return edges
}

// extractJavaThrowStatements walks a method body looking for throw_statement
// nodes and emits throws edges for each thrown exception type.
func extractJavaThrowStatements(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) []types.Edge {
	if node == nil {
		return nil
	}
	var edges []types.Edge
	walkForThrowStatements(node, opts, pkgPath, sourceHash, &edges)
	return edges
}

// walkForThrowStatements recursively walks nodes looking for throw_statement nodes.
func walkForThrowStatements(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash, edges *[]types.Edge) {
	if node == nil {
		return
	}
	if node.Type() == "throw_statement" {
		// Look for "new ExceptionType(...)" within the throw statement.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "object_creation_expression" {
				typeNode := child.ChildByFieldName("type")
				if typeNode != nil {
					exceptionType := typeNode.Content(opts.Content)
					if exceptionType != "" {
						targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, exceptionType, "error")
						provenance := "ast_inferred"
						edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "throws", provenance)
						*edges = append(*edges, types.Edge{
							EdgeHash:   edgeHash,
							SourceHash: sourceHash,
							TargetHash: targetHash,
							EdgeType:   "throws",
							Confidence: 0.7,
							Provenance: provenance,
						})
					}
				}
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkForThrowStatements(node.Child(i), opts, pkgPath, sourceHash, edges)
	}
}

// extractMethodInvocation creates a call edge for a method_invocation node.
// Handles both object.method() and simple method() calls.
func extractMethodInvocation(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) *types.Edge {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	methodName := nameNode.Content(opts.Content)

	objectNode := node.ChildByFieldName("object")
	targetName := methodName
	if objectNode != nil {
		objectName := objectNode.Content(opts.Content)
		targetName = objectName + "." + methodName
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, targetName, "method")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   provenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractObjectCreation creates a call edge for an object_creation_expression (new Foo()).
func extractObjectCreation(node *sitter.Node, opts types.ExtractOptions, pkgPath string, sourceHash types.Hash) *types.Edge {
	typeNode := node.ChildByFieldName("type")
	if typeNode == nil {
		return nil
	}
	typeName := typeNode.Content(opts.Content)

	// Object creation calls the constructor.
	targetName := typeName + ".<init>"
	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, targetName, "method")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "calls", provenance)

	return &types.Edge{
		EdgeHash:     edgeHash,
		SourceHash:   sourceHash,
		TargetHash:   targetHash,
		EdgeType:     "calls",
		Confidence:   0.7,
		Provenance:   provenance,
		CallSiteLine: int(node.StartPoint().Row) + 1,
		CallSiteCol:  int(node.StartPoint().Column),
		CallSiteFile: opts.FilePath,
	}
}

// extractAnnotationRoute extracts a route path from a class-level
// @RequestMapping annotation. Returns empty string if not found.
func extractAnnotationRoute(node *sitter.Node, content []byte) string {
	// Walk the modifiers looking for annotations.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				if mod.Type() == "annotation" || mod.Type() == "marker_annotation" {
					annName := extractAnnotationName(mod, content)
					if annName == "RequestMapping" {
						return extractAnnotationValue(mod, content)
					}
				}
			}
		}
	}
	return ""
}

// extractSpringAnnotation checks a method node for Spring route annotations
// and returns the route path and HTTP method. Returns ("", "") if no Spring
// annotation is found.
func extractSpringAnnotation(node *sitter.Node, content []byte) (string, string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				if mod.Type() == "annotation" || mod.Type() == "marker_annotation" {
					annName := extractAnnotationName(mod, content)
					if httpMethod, ok := springAnnotations[annName]; ok {
						routePath := extractAnnotationValue(mod, content)
						if routePath == "" {
							// Annotations like @PostMapping without a value
							// default to empty path.
							routePath = ""
						}
						return routePath, httpMethod
					}
				}
			}
		}
	}
	return "", ""
}

// extractAnnotationName extracts the simple name from an annotation node.
// For @GetMapping("..."), returns "GetMapping".
func extractAnnotationName(node *sitter.Node, content []byte) string {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		// For marker_annotation, the name might be a direct child.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "identifier" || child.Type() == "scoped_identifier" {
				text := child.Content(content)
				// For scoped identifiers like "org.springframework...GetMapping",
				// take the last segment.
				parts := strings.Split(text, ".")
				return parts[len(parts)-1]
			}
		}
		return ""
	}
	text := nameNode.Content(content)
	parts := strings.Split(text, ".")
	return parts[len(parts)-1]
}

// extractAnnotationValue extracts the first string literal argument from an
// annotation. For @GetMapping("/{id}"), returns "/{id}".
func extractAnnotationValue(node *sitter.Node, content []byte) string {
	args := node.ChildByFieldName("arguments")
	if args == nil {
		// Check for a direct value child (some annotation forms).
		return findFirstStringLiteral(node, content)
	}
	return findFirstStringLiteral(args, content)
}

// findFirstStringLiteral recursively finds the first string literal in a node tree.
func findFirstStringLiteral(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}
	if node.Type() == "string_literal" {
		val := node.Content(content)
		return strings.Trim(val, `"`)
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		if result := findFirstStringLiteral(node.Child(i), content); result != "" {
			return result
		}
	}
	return ""
}

// extractJavaSuperclass checks a class_declaration for a superclass field
// and emits an "extends" edge from the class to the superclass.
func extractJavaSuperclass(classNode *sitter.Node, opts types.ExtractOptions, pkgPath string, classHash types.Hash, edges *[]types.Edge) {
	superNode := classNode.ChildByFieldName("superclass")
	if superNode == nil {
		return
	}
	// The superclass field may wrap the type name. Extract it.
	superName := superNode.Content(opts.Content)
	// Strip "extends " prefix if tree-sitter includes the keyword.
	superName = strings.TrimPrefix(superName, "extends ")
	superName = strings.TrimSpace(superName)
	if superName == "" {
		return
	}

	targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, superName, "type")
	provenance := "ast_inferred"
	edgeHash := types.ComputeEdgeHash(classHash, targetHash, "extends", provenance)
	*edges = append(*edges, types.Edge{
		EdgeHash:   edgeHash,
		SourceHash: classHash,
		TargetHash: targetHash,
		EdgeType:   "extends",
		Confidence: 0.7,
		Provenance: provenance,
	})
}

// extractJavaInterfaces checks a class_declaration for a super_interfaces field
// and emits "implements" edges from the class to each interface.
func extractJavaInterfaces(classNode *sitter.Node, opts types.ExtractOptions, pkgPath string, classHash types.Hash, edges *[]types.Edge) {
	ifacesNode := classNode.ChildByFieldName("interfaces")
	if ifacesNode == nil {
		return
	}
	// The interfaces node (super_interfaces) contains type_list with type identifiers.
	for i := 0; i < int(ifacesNode.ChildCount()); i++ {
		child := ifacesNode.Child(i)
		if child.Type() == "type_list" {
			for j := 0; j < int(child.ChildCount()); j++ {
				typeRef := child.Child(j)
				if typeRef.Type() == "type_identifier" || typeRef.Type() == "generic_type" || typeRef.Type() == "scoped_type_identifier" {
					ifaceName := typeRef.Content(opts.Content)
					if idx := strings.Index(ifaceName, "<"); idx > 0 {
						ifaceName = ifaceName[:idx]
					}
					targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, ifaceName, "interface")
					provenance := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(classHash, targetHash, "implements", provenance)
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: classHash,
						TargetHash: targetHash,
						EdgeType:   "implements",
						Confidence: 0.7,
						Provenance: provenance,
					})
				}
			}
		}
		// If the child is directly a type identifier (no type_list wrapper).
		if child.Type() == "type_identifier" || child.Type() == "generic_type" || child.Type() == "scoped_type_identifier" {
			ifaceName := child.Content(opts.Content)
			if idx := strings.Index(ifaceName, "<"); idx > 0 {
				ifaceName = ifaceName[:idx]
			}
			targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, ifaceName, "interface")
			provenance := "ast_inferred"
			edgeHash := types.ComputeEdgeHash(classHash, targetHash, "implements", provenance)
			*edges = append(*edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: classHash,
				TargetHash: targetHash,
				EdgeType:   "implements",
				Confidence: 0.7,
				Provenance: provenance,
			})
		}
	}
}

// extractJavaOverrideEdge checks if a method has @Override annotation and
// emits an "overrides" edge.
func extractJavaOverrideEdge(methodNode *sitter.Node, opts types.ExtractOptions, pkgPath, className, methodName string, methodHash types.Hash, edges *[]types.Edge) {
	for i := 0; i < int(methodNode.ChildCount()); i++ {
		child := methodNode.Child(i)
		if child.Type() == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				if mod.Type() == "marker_annotation" || mod.Type() == "annotation" {
					annName := extractAnnotationName(mod, opts.Content)
					if annName == "Override" {
						targetName := "super." + methodName
						targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, targetName, "method")
						provenance := "ast_inferred"
						edgeHash := types.ComputeEdgeHash(methodHash, targetHash, "overrides", provenance)
						*edges = append(*edges, types.Edge{
							EdgeHash:   edgeHash,
							SourceHash: methodHash,
							TargetHash: targetHash,
							EdgeType:   "overrides",
							Confidence: 0.7,
							Provenance: provenance,
						})
						return
					}
				}
			}
		}
	}
}

// extractJavaAnnotationEdges emits "decorates" edges for annotations found
// on a class or method declaration.
func extractJavaAnnotationEdges(node *sitter.Node, opts types.ExtractOptions, pkgPath string, declHash types.Hash, edges *[]types.Edge) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				if mod.Type() == "annotation" || mod.Type() == "marker_annotation" {
					annName := extractAnnotationName(mod, opts.Content)
					if annName == "" || annName == "Override" {
						// Skip @Override (handled as overrides edge).
						continue
					}
					targetHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, annName, "function")
					provenance := "ast_inferred"
					edgeHash := types.ComputeEdgeHash(targetHash, declHash, "decorates", provenance)
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: targetHash,
						TargetHash: declHash,
						EdgeType:   "decorates",
						Confidence: 0.7,
						Provenance: provenance,
					})
				}
			}
		}
	}
}

// deduplicateEdges removes duplicate edges based on EdgeHash.
func deduplicateEdges(edges []types.Edge) []types.Edge {
	if len(edges) <= 1 {
		return edges
	}
	seen := make(map[types.Hash]struct{}, len(edges))
	result := make([]types.Edge, 0, len(edges))
	for _, e := range edges {
		if _, exists := seen[e.EdgeHash]; !exists {
			seen[e.EdgeHash] = struct{}{}
			result = append(result, e)
		}
	}
	return result
}
