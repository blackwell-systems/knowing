package cppextractor

import (
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractSymbols walks the root AST node and extracts all symbol nodes and
// structural edges (contains, member_of, extends). Call edges are extracted
// separately via extractCallEdges.
func extractSymbols(root *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		n, ed := extractTopLevel(child, opts, basePath, compileEntry)
		nodes = append(nodes, n...)
		edges = append(edges, ed...)
	}

	return nodes, edges
}

// extractTopLevel dispatches extraction based on tree-sitter node type.
func extractTopLevel(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	if node == nil {
		return nil, nil
	}

	switch node.Type() {
	case "function_definition":
		return extractFunction(node, opts, basePath, "", compileEntry)
	case "class_specifier":
		return extractClass(node, opts, basePath, compileEntry)
	case "struct_specifier":
		return extractStruct(node, opts, basePath, compileEntry)
	case "namespace_definition":
		return extractNamespace(node, opts, basePath, compileEntry)
	case "enum_specifier":
		return extractEnum(node, opts, basePath)
	case "type_definition":
		return extractTypedef(node, opts, basePath)
	case "using_declaration", "alias_declaration":
		return extractUsing(node, opts, basePath)
	case "preproc_def":
		return extractMacro(node, opts, basePath)
	case "declaration": // forward declarations, global variable declarations, or function declarations without body
		return extractDeclaration(node, opts, basePath, "", compileEntry)
	case "linkage_specification": // extern "C" { ... }
		return extractLinkageSpec(node, opts, basePath, compileEntry)
	case "template_declaration":
		return extractTemplateDecl(node, opts, basePath, "", compileEntry)
	case "template_instantiation":
		return nil, nil // explicit instantiations: no new node, handled via edges
	}
	return nil, nil
}

// extractFunction extracts a function_definition node.
// When classContext is non-empty, the function is treated as a method.
func extractFunction(node *sitter.Node, opts types.ExtractOptions, basePath, classContext string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	// In tree-sitter C++, function_definition has a "declarator" field
	// containing a function_declarator, which in turn has the name.
	declarator := node.ChildByFieldName("declarator")
	if declarator == nil {
		return nil, nil
	}
	name := getDeclaratorName(declarator, opts.Content)
	if name == "" {
		return nil, nil
	}
	// Get line from the function_declarator's name, not the top-level node.
	line := int(declarator.StartPoint().Row) + 1

	// Determine kind: method if inside a class context, function otherwise.
	kind := types.KindFunction
	if classContext != "" {
		kind = types.KindMethod
	}

	qname := makeFuncQName(opts, basePath, classContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)

	sig := buildFunctionSignature(node, opts.Content, classContext, name)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	// Extract call edges from the function body.
	var edges []types.Edge
	body := node.ChildByFieldName("body")
	callEdges := extractCallEdges(body, opts, basePath, nodeHash, compileEntry)
	edges = append(edges, callEdges...)

	// Emit containment edges if inside a class.
	if classContext != "" {
		classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, classContext, types.KindType)
		edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)
	}

	return []types.Node{n}, edges
}

// extractClass extracts a class_specifier node and recurses into its body
// for methods, fields, and nested types.
func extractClass(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("class %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	// Extract inheritance edges.
	inheritEdges := extractInheritanceEdges(node, opts, basePath, nodeHash)
	edges = append(edges, inheritEdges...)

	// Recurse into class body.
	body := node.ChildByFieldName("body")
	if body == nil {
		// tree-sitter C++ uses "field_declaration_list" for class bodies.
		body = findChildByType(node, "field_declaration_list")
	}
	if body != nil {
		childNodes, childEdges := extractClassBody(body, opts, basePath, name, compileEntry)
		nodes = append(nodes, childNodes...)
		edges = append(edges, childEdges...)
	}

	return nodes, edges
}

// extractStruct extracts a struct_specifier node (treated identically to class in C++).
func extractStruct(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("struct %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	// Extract inheritance edges (structs can inherit in C++).
	inheritEdges := extractInheritanceEdges(node, opts, basePath, nodeHash)
	edges = append(edges, inheritEdges...)

	// Recurse into struct body.
	body := node.ChildByFieldName("body")
	if body != nil {
		childNodes, childEdges := extractClassBody(body, opts, basePath, name, compileEntry)
		nodes = append(nodes, childNodes...)
		edges = append(edges, childEdges...)
	}

	return nodes, edges
}

// extractClassBody walks a class/struct body node and extracts methods,
// fields, nested types, and their containment edges.
func extractClassBody(body *sitter.Node, opts types.ExtractOptions, basePath, className string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)

	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		switch child.Type() {
		case "function_definition":
			// Method definition with body.
			n, ed := extractFunction(child, opts, basePath, className, compileEntry)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)

		case "declaration":
			// Could be a method declaration (without body) or a field.
			fieldNodes, fieldEdges := extractDeclaration(child, opts, basePath, className, compileEntry)
			nodes = append(nodes, fieldNodes...)
			edges = append(edges, fieldEdges...)

		case "field_declaration":
			// In tree-sitter C++, methods without a body appear as
			// field_declaration with a function_declarator inside.
			// Nested classes also appear inside field_declaration.
			if hasFunctionDeclarator(child) {
				methodNodes, methodEdges := extractDeclaration(child, opts, basePath, className, compileEntry)
				nodes = append(nodes, methodNodes...)
				edges = append(edges, methodEdges...)
			} else if nestedClass := findChildByType(child, "class_specifier"); nestedClass != nil {
				nestedNodes, nestedEdges := extractClass(nestedClass, opts, basePath, compileEntry)
				nodes = append(nodes, nestedNodes...)
				edges = append(edges, nestedEdges...)
				for _, nn := range nestedNodes {
					edges = append(edges, extractContainmentEdges(classHash, []types.Node{nn}, edgetype.Contains)...)
				}
			} else if nestedStruct := findChildByType(child, "struct_specifier"); nestedStruct != nil {
				nestedNodes, nestedEdges := extractStruct(nestedStruct, opts, basePath, compileEntry)
				nodes = append(nodes, nestedNodes...)
				edges = append(edges, nestedEdges...)
				for _, nn := range nestedNodes {
					edges = append(edges, extractContainmentEdges(classHash, []types.Node{nn}, edgetype.Contains)...)
				}
			} else {
				fieldNodes, fieldEdges := extractField(child, opts, basePath, className)
				nodes = append(nodes, fieldNodes...)
				edges = append(edges, fieldEdges...)
			}

		case "class_specifier", "struct_specifier":
			// Nested class/struct.
			var nestedNodes []types.Node
			var nestedEdges []types.Edge
			if child.Type() == "class_specifier" {
				nestedNodes, nestedEdges = extractClass(child, opts, basePath, compileEntry)
			} else {
				nestedNodes, nestedEdges = extractStruct(child, opts, basePath, compileEntry)
			}
			nodes = append(nodes, nestedNodes...)
			edges = append(edges, nestedEdges...)
			// Containment edge for nested type.
			for _, nn := range nestedNodes {
				edges = append(edges, extractContainmentEdges(classHash, []types.Node{nn}, edgetype.Contains)...)
			}

		case "enum_specifier":
			n, ed := extractEnum(child, opts, basePath)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)
			for _, nn := range n {
				edges = append(edges, extractContainmentEdges(classHash, []types.Node{nn}, edgetype.Contains)...)
			}

		case "type_definition":
			n, ed := extractTypedef(child, opts, basePath)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)

		case "using_declaration", "alias_declaration":
			n, ed := extractUsing(child, opts, basePath)
			nodes = append(nodes, n...)
			edges = append(edges, ed...)

		case "access_specifier":
			// public:, private:, protected: — skip, no node.

		case "template_declaration":
			// Template method or nested template class inside class body.
			tmplNodes, tmplEdges := extractClassBodyTemplate(child, opts, basePath, className, compileEntry)
			nodes = append(nodes, tmplNodes...)
			edges = append(edges, tmplEdges...)
		}
	}

	return nodes, edges
}

// extractClassBodyTemplate handles a template_declaration inside a class body.
// It delegates to extractTemplateDecl with the class context and creates
// containment edges for the resulting nodes.
func extractClassBodyTemplate(node *sitter.Node, opts types.ExtractOptions, basePath, className string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)

	childNodes, childEdges := extractTemplateDecl(node, opts, basePath, className, compileEntry)

	// Add containment edges for extracted nodes.
	for _, cn := range childNodes {
		childEdges = append(childEdges, extractContainmentEdges(classHash, []types.Node{cn}, edgetype.Contains)...)
	}

	return childNodes, childEdges
}

// extractDeclaration handles a generic declaration node.
// It may be a field declaration, method declaration (without body), or
// top-level function declaration (without body).
func extractDeclaration(node *sitter.Node, opts types.ExtractOptions, basePath, className string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	// Check if this declaration has a function_declarator (function/method declaration).
	decl := findChildByType(node, "function_declarator")
	if decl != nil {
		// Function/method declaration without body — extract as a node.
		nameNode := decl.ChildByFieldName("declarator")
		if nameNode == nil {
			return nil, nil
		}
		// Unwrap pointer_declarator if present.
		if nameNode.Type() == "pointer_declarator" {
			inner := nameNode.ChildByFieldName("declarator")
			if inner != nil {
				nameNode = inner
			} else if nameNode.ChildCount() > 0 {
				nameNode = nameNode.Child(int(nameNode.ChildCount() - 1))
			}
		}
		name := getDeclaratorName(nameNode, opts.Content)
		if name == "" {
			return nil, nil
		}
		line := int(nameNode.StartPoint().Row) + 1

		// Determine kind: method if inside a class, function otherwise.
		kind := types.KindFunction
		sig := fmt.Sprintf("%s()", name)
		if className != "" {
			kind = types.KindMethod
			sig = fmt.Sprintf("%s::%s()", className, name)
		}

		qname := makeFuncQName(opts, basePath, className, name)
		nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)

		n := types.Node{
			NodeHash:      nodeHash,
			FileHash:      opts.FileHash,
			QualifiedName: qname,
			Kind:          kind,
			Line:          line,
			Signature:     sig,
			Doc:           extractDoc(node, opts.Content),
		}

		var edges []types.Edge
		// Only emit containment edges if inside a class.
		if className != "" {
			classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)
			edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)
		}

		return []types.Node{n}, edges
	}

	// Otherwise treat as a field declaration.
	return extractFieldFromDecl(node, opts, basePath, className)
}

// extractField extracts a field_declaration node inside a class/struct.
func extractField(node *sitter.Node, opts types.ExtractOptions, basePath, className string) ([]types.Node, []types.Edge) {
	// The field name is in the declarator, which may be nested (pointer, array, etc.).
	decl := node.ChildByFieldName("declarator")
	if decl == nil {
		// Try first named child that looks like a declarator.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.IsNamed() && child.Type() != "type_identifier" &&
				child.Type() != "primitive_type" && child.Type() != "struct_specifier" &&
				child.Type() != "class_specifier" && child.Type() != "enum_specifier" {
				decl = child
				break
			}
		}
	}
	if decl == nil {
		return nil, nil
	}

	fieldName := getDeclaratorName(decl, opts.Content)
	if fieldName == "" {
		return nil, nil
	}

	line := int(decl.StartPoint().Row) + 1
	qname := makeFieldQName(opts, basePath, className, fieldName)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, fieldName, types.KindField)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          types.KindField,
		Line:          line,
		Signature:     fmt.Sprintf("%s::%s", className, fieldName),
	}

	var edges []types.Edge
	classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)
	edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)

	return []types.Node{n}, edges
}

// extractFieldFromDecl extracts a field from a declaration node (used when
// the declaration doesn't have a function_declarator).
func extractFieldFromDecl(node *sitter.Node, opts types.ExtractOptions, basePath, className string) ([]types.Node, []types.Edge) {
	// Walk children to find the declarator (identifier or field_identifier).
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_identifier" || child.Type() == "identifier" {
			fieldName := child.Content(opts.Content)
			line := int(child.StartPoint().Row) + 1
			qname := makeFieldQName(opts, basePath, className, fieldName)
			nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, fieldName, types.KindField)

			n := types.Node{
				NodeHash:      nodeHash,
				FileHash:      opts.FileHash,
				QualifiedName: qname,
				Kind:          types.KindField,
				Line:          line,
				Signature:     fmt.Sprintf("%s::%s", className, fieldName),
			}

			var edges []types.Edge
			classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)
			edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)

			return []types.Node{n}, edges
		}
	}
	return nil, nil
}

// extractNamespace extracts a namespace_definition node and recurses into its body.
func extractNamespace(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("namespace %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	// Recurse into namespace body.
	body := node.ChildByFieldName("body")
	if body == nil {
		// tree-sitter C++ uses "declaration_list" for namespace bodies.
		body = findChildByType(node, "declaration_list")
	}
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			child := body.Child(i)
			childNodes, childEdges := extractTopLevel(child, opts, basePath, compileEntry)
			// For methods and functions inside namespace, containment edges go to namespace.
			for _, cn := range childNodes {
				if cn.Kind == types.KindMethod || cn.Kind == types.KindFunction ||
					cn.Kind == types.KindType || cn.Kind == types.KindField {
					edges = append(edges, extractContainmentEdges(nodeHash, []types.Node{cn}, edgetype.Contains)...)
				}
			}
			nodes = append(nodes, childNodes...)
			edges = append(edges, childEdges...)
		}
	}

	return nodes, edges
}

// extractEnum extracts an enum_specifier node and its enumerator values.
func extractEnum(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("enum %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	// Extract enumerator values as const nodes.
	body := findChildByType(node, "enumerator_list")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			child := body.Child(i)
			if child.Type() == "enumerator" {
				enumeratorNodes, enumeratorEdges := extractEnumerator(child, opts, basePath, name, nodeHash)
				nodes = append(nodes, enumeratorNodes...)
				edges = append(edges, enumeratorEdges...)
			}
		}
	}

	return nodes, edges
}

// extractEnumerator extracts a single enumerator value from an enum.
func extractEnumerator(node *sitter.Node, opts types.ExtractOptions, basePath, enumName string, enumHash types.Hash) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	qname := makeEnumValueQName(opts, basePath, enumName, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindConst)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          types.KindConst,
		Line:          line,
		Signature:     fmt.Sprintf("%s::%s", enumName, name),
	}

	edges := extractContainmentEdges(enumHash, []types.Node{n}, edgetype.Contains)

	return []types.Node{n}, edges
}

// extractTypedef extracts a type_definition node.
func extractTypedef(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	// In tree-sitter C++, type_definition has a type_descriptor containing
	// a type_identifier for the new name. The name is the last type_identifier.
	var name string
	var nameNode *sitter.Node

	// Walk children to find the type_identifier that is the new name.
	// In "typedef int MyInt;", MyInt is the last type_identifier.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			name = child.Content(opts.Content)
			nameNode = child
		}
	}

	if name == "" || nameNode == nil {
		return nil, nil
	}

	line := int(nameNode.StartPoint().Row) + 1
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("typedef %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	return []types.Node{n}, nil
}

// extractUsing extracts a using_declaration node.
func extractUsing(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	// using_declaration has a "name" field.
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     fmt.Sprintf("using %s", name),
		Doc:           extractDoc(node, opts.Content),
	}

	return []types.Node{n}, nil
}

// extractMacro extracts a preproc_def node (#define).
func extractMacro(node *sitter.Node, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	// Extract the macro value (everything after the name).
	value := extractMacroValue(node, opts.Content, name)

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindConst)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeMacroQName(opts, basePath, name),
		Kind:          types.KindConst,
		Line:          line,
		Signature:     fmt.Sprintf("#define %s%s", name, value),
		Doc:           extractDoc(node, opts.Content),
	}

	return []types.Node{n}, nil
}

// extractMacroValue extracts the value portion of a #define directive.
func extractMacroValue(node *sitter.Node, content []byte, name string) string {
	fullText := node.Content(content)
	// Find the name in the full text and take everything after it.
	idx := strings.Index(fullText, name)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(fullText[idx+len(name):])
	if rest == "" {
		return ""
	}
	return " " + rest
}

// extractLinkageSpec handles extern "C" { ... } blocks by recursing into children.
func extractLinkageSpec(node *sitter.Node, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	var nodes []types.Node
	var edges []types.Edge

	// extern "C" has a declaration_list child containing the declarations.
	declList := findChildByType(node, "declaration_list")
	if declList != nil {
		for i := 0; i < int(declList.ChildCount()); i++ {
			child := declList.Child(i)
			childNodes, childEdges := extractTopLevel(child, opts, basePath, compileEntry)
			nodes = append(nodes, childNodes...)
			edges = append(edges, childEdges...)
		}
	}

	return nodes, edges
}

// buildFunctionSignature builds a human-readable signature string for a function.
func buildFunctionSignature(node *sitter.Node, content []byte, classContext, funcName string) string {
	// Get return type from the "type" field.
	returnType := ""
	typeNode := node.ChildByFieldName("type")
	if typeNode != nil {
		returnType = typeNode.Content(content)
		// Clean up common patterns.
		returnType = strings.TrimSpace(returnType)
	}

	// Get parameters.
	params := ""
	paramNode := node.ChildByFieldName("parameters")
	if paramNode != nil {
		params = paramNode.Content(content)
	}

	if classContext != "" {
		if returnType != "" {
			return fmt.Sprintf("%s %s::%s%s", returnType, classContext, funcName, params)
		}
		return fmt.Sprintf("%s::%s%s", classContext, funcName, params)
	}
	if returnType != "" {
		return fmt.Sprintf("%s %s%s", returnType, funcName, params)
	}
	return fmt.Sprintf("%s%s", funcName, params)
}

// getDeclaratorName extracts the name from a declarator node, unwrapping
// pointer_declarator, array_declarator, and other wrappers.
func getDeclaratorName(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	switch node.Type() {
	case "identifier", "field_identifier":
		return node.Content(content)
	case "pointer_declarator":
		// *foo -> foo
		inner := node.ChildByFieldName("declarator")
		if inner != nil {
			return getDeclaratorName(inner, content)
		}
		if node.ChildCount() > 0 {
			return getDeclaratorName(node.Child(int(node.ChildCount()-1)), content)
		}
	case "reference_declarator":
		// &foo -> foo
		inner := node.ChildByFieldName("declarator")
		if inner != nil {
			return getDeclaratorName(inner, content)
		}
		if node.ChildCount() > 0 {
			return getDeclaratorName(node.Child(int(node.ChildCount()-1)), content)
		}
	case "array_declarator":
		// foo[] -> foo
		inner := node.ChildByFieldName("declarator")
		if inner != nil {
			return getDeclaratorName(inner, content)
		}
	case "function_declarator":
		// foo(...) -> foo
		inner := node.ChildByFieldName("declarator")
		if inner != nil {
			return getDeclaratorName(inner, content)
		}
	case "parameter_declaration":
		// For parameter types — not used for names.
		return ""
	}

	// Fallback: walk children looking for an identifier.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "field_identifier" {
			return child.Content(content)
		}
		name := getDeclaratorName(child, content)
		if name != "" {
			return name
		}
	}

	return ""
}

// hasFunctionDeclarator checks if a node contains a function_declarator child.
func hasFunctionDeclarator(node *sitter.Node) bool {
	return findChildByType(node, "function_declarator") != nil
}

// extractTemplateDecl extracts a template_declaration node by unwrapping to the
// inner declaration and augmenting the signature with template parameters.
func extractTemplateDecl(node *sitter.Node, opts types.ExtractOptions, basePath, classContext string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	info := extractTemplateHeader(node, opts.Content)
	inner := findInnerDeclaration(node)
	if inner == nil {
		return nil, nil
	}

	switch inner.Type() {
	case "class_specifier":
		return extractClassWithTemplate(inner, node, info, opts, basePath, compileEntry)
	case "struct_specifier":
		return extractStructWithTemplate(inner, node, info, opts, basePath, compileEntry)
	case "function_definition":
		return extractFunctionWithTemplate(inner, node, info, opts, basePath, classContext, compileEntry)
	case "declaration":
		// Method declaration without body (inside class) or forward declaration.
		return extractDeclarationWithTemplate(inner, info, opts, basePath, classContext)
	case "alias_declaration":
		return extractUsingWithTemplate(inner, info, opts, basePath)
	case "concept_definition":
		return extractConcept(inner, info, opts, basePath)
	}

	return nil, nil
}

// extractClassWithTemplate extracts a class_specifier that was wrapped in a
// template_declaration. Primary templates use type-erased QN. Specializations
// get a variant QN with template args appended (e.g. "Vec.<bool>").
func extractClassWithTemplate(node *sitter.Node, tmplDecl *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	primaryName := extractPrimaryName(nameNode, opts.Content)
	if primaryName == "" {
		return nil, nil
	}
	line := int(nameNode.StartPoint().Row) + 1

	// Specializations get a distinct QN with template args suffix.
	qnName := primaryName
	if info.IsSpecialized {
		args := extractSpecializationArgs(nameNode, opts.Content)
		if args != "" {
			qnName = primaryName + "." + args
		}
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, qnName, types.KindType)

	innerSig := fmt.Sprintf("class %s", primaryName)
	sig := buildTemplateSignature(info, innerSig)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, qnName),
		Kind:          types.KindType,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	// Extract inheritance edges.
	inheritEdges := extractInheritanceEdges(node, opts, basePath, nodeHash)
	edges = append(edges, inheritEdges...)

	// Extract template edges (specializes, requires).
	tmplEdges := extractTemplateEdges(tmplDecl, info, nodeHash, opts, basePath)
	edges = append(edges, tmplEdges...)

	// Recurse into class body.
	body := node.ChildByFieldName("body")
	if body == nil {
		body = findChildByType(node, "field_declaration_list")
	}
	if body != nil {
		childNodes, childEdges := extractClassBody(body, opts, basePath, primaryName, compileEntry)
		nodes = append(nodes, childNodes...)
		edges = append(edges, childEdges...)
	}

	return nodes, edges
}

// extractStructWithTemplate extracts a struct_specifier wrapped in a template_declaration.
func extractStructWithTemplate(node *sitter.Node, tmplDecl *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	primaryName := extractPrimaryName(nameNode, opts.Content)
	if primaryName == "" {
		return nil, nil
	}
	line := int(nameNode.StartPoint().Row) + 1

	qnName := primaryName
	if info.IsSpecialized {
		args := extractSpecializationArgs(nameNode, opts.Content)
		if args != "" {
			qnName = primaryName + "." + args
		}
	}

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, qnName, types.KindType)

	innerSig := fmt.Sprintf("struct %s", primaryName)
	sig := buildTemplateSignature(info, innerSig)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, qnName),
		Kind:          types.KindType,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	var nodes []types.Node
	var edges []types.Edge
	nodes = append(nodes, n)

	inheritEdges := extractInheritanceEdges(node, opts, basePath, nodeHash)
	edges = append(edges, inheritEdges...)

	tmplEdges := extractTemplateEdges(tmplDecl, info, nodeHash, opts, basePath)
	edges = append(edges, tmplEdges...)

	body := node.ChildByFieldName("body")
	if body != nil {
		childNodes, childEdges := extractClassBody(body, opts, basePath, primaryName, compileEntry)
		nodes = append(nodes, childNodes...)
		edges = append(edges, childEdges...)
	}

	return nodes, edges
}

// extractDeclarationWithTemplate extracts a declaration (without body) that was
// wrapped in a template_declaration. Handles template method declarations.
func extractDeclarationWithTemplate(node *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath, className string) ([]types.Node, []types.Edge) {
	decl := findChildByType(node, "function_declarator")
	if decl == nil {
		return nil, nil
	}
	nameNode := decl.ChildByFieldName("declarator")
	if nameNode == nil {
		return nil, nil
	}
	if nameNode.Type() == "pointer_declarator" {
		inner := nameNode.ChildByFieldName("declarator")
		if inner != nil {
			nameNode = inner
		}
	}
	name := getDeclaratorName(nameNode, opts.Content)
	if name == "" {
		return nil, nil
	}
	line := int(nameNode.StartPoint().Row) + 1

	kind := types.KindFunction
	if className != "" {
		kind = types.KindMethod
	}

	// Build signature with template parameters.
	innerSig := fmt.Sprintf("%s()", name)
	if className != "" {
		innerSig = fmt.Sprintf("%s::%s()", className, name)
	}
	sig := buildTemplateSignature(info, innerSig)

	qname := makeFuncQName(opts, basePath, className, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	var edges []types.Edge
	if className != "" {
		classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, className, types.KindType)
		edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)
	}

	return []types.Node{n}, edges
}

// extractFunctionWithTemplate extracts a function_definition wrapped in a
// template_declaration.
func extractFunctionWithTemplate(node *sitter.Node, tmplDecl *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath, classContext string, compileEntry *CompileEntry) ([]types.Node, []types.Edge) {
	decl := node.ChildByFieldName("declarator")
	if decl == nil {
		return nil, nil
	}
	name := getDeclaratorName(decl, opts.Content)
	if name == "" {
		return nil, nil
	}
	line := int(decl.StartPoint().Row) + 1

	kind := types.KindFunction
	if classContext != "" {
		kind = types.KindMethod
	}

	qname := makeFuncQName(opts, basePath, classContext, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, kind)

	// Build signature with template parameters.
	innerSig := buildFunctionSignature(node, opts.Content, classContext, name)
	sig := buildTemplateSignature(info, innerSig)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qname,
		Kind:          kind,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	var edges []types.Edge
	body := node.ChildByFieldName("body")
	callEdges := extractCallEdges(body, opts, basePath, nodeHash, compileEntry)
	edges = append(edges, callEdges...)

	// Requires edges from the template's requires clause.
	tmplEdges := extractTemplateEdges(tmplDecl, info, nodeHash, opts, basePath)
	edges = append(edges, tmplEdges...)

	if classContext != "" {
		classHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, classContext, types.KindType)
		edges = append(edges, extractContainmentEdges(classHash, []types.Node{n}, edgetype.Contains)...)
	}

	return []types.Node{n}, edges
}

// extractUsingWithTemplate extracts an alias_declaration wrapped in a
// template_declaration (template alias).
func extractUsingWithTemplate(node *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	innerSig := fmt.Sprintf("using %s", name)
	sig := buildTemplateSignature(info, innerSig)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	return []types.Node{n}, nil
}

// extractConcept extracts a concept_definition node as a type node.
func extractConcept(node *sitter.Node, info TemplateInfo, opts types.ExtractOptions, basePath string) ([]types.Node, []types.Edge) {
	// concept_definition has an "identifier" field for the concept name.
	nameNode := findChildByType(node, "identifier")
	if nameNode == nil {
		return nil, nil
	}
	name := nameNode.Content(opts.Content)
	line := int(nameNode.StartPoint().Row) + 1

	nodeHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, name, types.KindType)

	innerSig := fmt.Sprintf("concept %s", name)
	sig := buildTemplateSignature(info, innerSig)

	n := types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: makeTypeQName(opts, basePath, name),
		Kind:          types.KindType,
		Line:          line,
		Signature:     sig,
		Doc:           extractDoc(node, opts.Content),
	}

	return []types.Node{n}, nil
}

// extractTemplateInstantiationEdges creates instantiates edges from explicit
// template_instantiation AST nodes (e.g. "template class Vec<int>;").
func extractTemplateInstantiationEdges(node *sitter.Node, opts types.ExtractOptions, basePath string, tmplTable *TemplateTable) []types.Edge {
	if node == nil || node.Type() != "template_instantiation" {
		return nil
	}

	var edges []types.Edge

	// Walk children looking for template_type (class instantiation)
	// or template_function (function instantiation).
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "class_specifier":
			// template class Vec<int>;
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				baseName := extractPrimaryName(nameNode, opts.Content)
				if baseName != "" {
					if hash, ok := tmplTable.Lookup(baseName); ok {
						// Edge from file-level to template. We'll use a synthetic
						// file hash as the source since explicit instantiations
						// don't have an enclosing function.
						fileHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash,
							"", types.KindFile)
						edgeHash := types.ComputeEdgeHash(fileHash, hash, edgetype.Instantiates, "ast_inferred")
						edges = append(edges, types.Edge{
							EdgeHash:   edgeHash,
							SourceHash: fileHash,
							TargetHash: hash,
							EdgeType:   edgetype.Instantiates,
							Confidence: 0.7,
							Provenance: "ast_inferred",
						})
					}
				}
			}

		case "function_declarator":
			// template void sort<int>(int*, int*);
			declName := findChildByType(child, "template_function")
			if declName != nil {
				nameNode := declName.ChildByFieldName("name")
				if nameNode != nil {
					baseName := nameNode.Content(opts.Content)
					if idx := strings.Index(baseName, "<"); idx > 0 {
						baseName = baseName[:idx]
					}
					if baseName != "" {
						if hash, ok := tmplTable.Lookup(baseName); ok {
							fileHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash,
								"", types.KindFile)
							edgeHash := types.ComputeEdgeHash(fileHash, hash, edgetype.Instantiates, "ast_inferred")
							edges = append(edges, types.Edge{
								EdgeHash:   edgeHash,
								SourceHash: fileHash,
								TargetHash: hash,
								EdgeType:   edgetype.Instantiates,
								Confidence: 0.7,
								Provenance: "ast_inferred",
							})
						}
					}
				}
			}
		}
	}

	return edges
}
