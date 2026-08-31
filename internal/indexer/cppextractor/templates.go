package cppextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// TemplateInfo holds the parsed template header information from a template_declaration node.
type TemplateInfo struct {
	Params       string // raw text from template_parameter_list, e.g. "typename T, int N"
	IsSpecialized bool  // true when the inner name is a template_type (specialization)
	Requires     string // raw text from requires_clause, e.g. "Sortable<T>"
}

// extractTemplateHeader extracts template parameters and requires clause from
// a template_declaration node. Returns empty TemplateInfo if node is not a
// template_declaration.
func extractTemplateHeader(node *sitter.Node, content []byte) TemplateInfo {
	if node == nil || node.Type() != "template_declaration" {
		return TemplateInfo{}
	}

	info := TemplateInfo{}

	// Extract template_parameter_list text (without the outer angle brackets, just the params).
	paramList := findChildByType(node, "template_parameter_list")
	if paramList != nil {
		raw := paramList.Content(content)
		// Strip outer angle brackets: "<typename T>" -> "typename T"
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "<") && strings.HasSuffix(raw, ">") {
			raw = raw[1 : len(raw)-1]
		}
		info.Params = strings.TrimSpace(raw)
	}

	// Extract requires_clause if present.
	reqClause := findChildByType(node, "requires_clause")
	if reqClause != nil {
		raw := reqClause.Content(content)
		// Strip leading "requires " keyword.
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(strings.ToLower(raw), "requires") {
			raw = strings.TrimSpace(raw[len("requires"):])
		}
		info.Requires = raw
	}

	// Detect specialization: check if the inner declaration's name is a template_type.
	inner := findInnerDeclaration(node)
	if inner != nil {
		info.IsSpecialized = isSpecialization(inner)
	}

	return info
}

// findInnerDeclaration finds the actual declaration wrapped by template_declaration.
// Skips requires_clause and template_parameter_list children.
func findInnerDeclaration(node *sitter.Node) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "template_parameter_list", "template", "requires_clause":
			continue
		default:
			return child
		}
	}
	return nil
}

// isSpecialization returns true if the declaration's name node is a template_type,
// indicating this is a full or partial specialization.
func isSpecialization(decl *sitter.Node) bool {
	if decl == nil {
		return false
	}

	switch decl.Type() {
	case "class_specifier", "struct_specifier":
		// Check if the "name" child is template_type (specialization) vs type_identifier (primary).
		nameNode := decl.ChildByFieldName("name")
		if nameNode != nil && nameNode.Type() == "template_type" {
			return true
		}
	case "alias_declaration":
		// Template alias: template<typename T> using Alias = vector<T>
		// The name is type_identifier, not template_type — not a specialization.
	}
	return false
}

// buildTemplateSignature prepends template parameters and requires clause to
// an existing signature string.
func buildTemplateSignature(info TemplateInfo, innerSig string) string {
	if info.Params == "" && info.Requires == "" {
		return innerSig
	}

	var sb strings.Builder
	if info.Params != "" {
		sb.WriteString("template<")
		sb.WriteString(info.Params)
		sb.WriteString("> ")
	}
	if info.Requires != "" {
		sb.WriteString("requires ")
		sb.WriteString(info.Requires)
		sb.WriteString(" ")
	}
	sb.WriteString(innerSig)
	return sb.String()
}

// extractPrimaryName extracts the base template name (without arguments) from
// a declaration's name node.
// For template_type (specialization): "Vec<bool>" -> "Vec"
// For type_identifier (primary): "Vec" -> "Vec"
func extractPrimaryName(nameNode *sitter.Node, content []byte) string {
	if nameNode == nil {
		return ""
	}

	if nameNode.Type() == "template_type" {
		// template_type has a type_identifier child with the base name.
		typeId := findChildByType(nameNode, "type_identifier")
		if typeId != nil {
			return typeId.Content(content)
		}
	}

	if nameNode.Type() == "type_identifier" {
		return nameNode.Content(content)
	}

	return nameNode.Content(content)
}

// extractPrimaryNameFromDecl extracts the base template name from a declaration
// node's name field (handles both class_specifier and struct_specifier).
func extractPrimaryNameFromDecl(decl *sitter.Node, content []byte) string {
	if decl == nil {
		return ""
	}
	nameNode := decl.ChildByFieldName("name")
	return extractPrimaryName(nameNode, content)
}

// extractSpecializationArgs extracts the template argument list from a
// specialization's template_type node.
// e.g. "Vec<bool>" -> "<bool>", "Pair<T, T>" -> "<T, T>"
// Returns empty string if the name is not a template_type.
func extractSpecializationArgs(nameNode *sitter.Node, content []byte) string {
	if nameNode == nil || nameNode.Type() != "template_type" {
		return ""
	}

	argList := findChildByType(nameNode, "template_argument_list")
	if argList != nil {
		return argList.Content(content)
	}
	return ""
}

// extractTemplateEdges creates specializes and requires edges from a
// template_declaration node. The sourceHash is the hash of the specialization
// or constrained template being extracted. tmplDecl is the outer
// template_declaration node (used to find the inner declaration for
// specialization detection).
func extractTemplateEdges(
	tmplDecl *sitter.Node,
	info TemplateInfo,
	sourceHash types.Hash,
	opts types.ExtractOptions,
	basePath string,
) []types.Edge {
	var edges []types.Edge

	if tmplDecl == nil || tmplDecl.Type() != "template_declaration" {
		return edges
	}

	inner := findInnerDeclaration(tmplDecl)
	if inner == nil {
		return edges
	}

	// Specialization edges: specialization -> primary template.
	if info.IsSpecialized {
		primaryName := ""
		switch inner.Type() {
		case "class_specifier", "struct_specifier":
			nameNode := inner.ChildByFieldName("name")
			primaryName = extractPrimaryName(nameNode, opts.Content)
		}

		if primaryName != "" {
			primaryHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, primaryName, types.KindType)
			edgeHash := types.ComputeEdgeHash(sourceHash, primaryHash, edgetype.Specializes, "ast_inferred")
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: primaryHash,
				EdgeType:   edgetype.Specializes,
				Confidence: 0.7,
				Provenance: "ast_inferred",
			})
		}
	}

	// Requires edges: constrained template -> concept.
	if info.Requires != "" {
		conceptName := extractConceptName(info.Requires)
		if conceptName != "" {
			conceptHash := types.ComputeNodeHash(opts.RepoURL, basePath, types.EmptyHash, conceptName, types.KindType)
			edgeHash := types.ComputeEdgeHash(sourceHash, conceptHash, edgetype.Requires, "ast_inferred")
			edges = append(edges, types.Edge{
				EdgeHash:   edgeHash,
				SourceHash: sourceHash,
				TargetHash: conceptHash,
				EdgeType:   edgetype.Requires,
				Confidence: 0.7,
				Provenance: "ast_inferred",
			})
		}
	}

	return edges
}

// extractConceptName extracts the simple concept name from a requires clause.
// "Sortable<T>" -> "Sortable"
// "std::is_integral_v<T>" -> "is_integral_v"
// "std::same_as<T, int>" -> "same_as"
func extractConceptName(requires string) string {
	// Strip template arguments: "Sortable<T>" -> "Sortable"
	if idx := strings.Index(requires, "<"); idx > 0 {
		requires = requires[:idx]
	}
	// Strip namespace prefix: "std::is_integral_v" -> "is_integral_v"
	if idx := strings.LastIndex(requires, "::"); idx >= 0 {
		requires = requires[idx+2:]
	}
	return strings.TrimSpace(requires)
}

// TemplateTable tracks template declarations for same-file instantiation resolution.
// Maps base template name to its node hash.
type TemplateTable struct {
	templates map[string]types.Hash // "Vec" -> hash of Vec node
}

// NewTemplateTable creates an empty TemplateTable.
func NewTemplateTable() *TemplateTable {
	return &TemplateTable{
		templates: make(map[string]types.Hash),
	}
}

// Register adds a template to the table.
func (t *TemplateTable) Register(name string, hash types.Hash) {
	if t != nil {
		t.templates[name] = hash
	}
}

// Lookup finds a template by base name.
func (t *TemplateTable) Lookup(name string) (types.Hash, bool) {
	if t == nil {
		return types.EmptyHash, false
	}
	h, ok := t.templates[name]
	return h, ok
}

// walkForInstantiations walks an AST node looking for template_type nodes
// in declarations and creates instantiates edges. The enclosingFn hash is
// the hash of the function/method containing the usage.
func walkForInstantiations(
	node *sitter.Node,
	opts types.ExtractOptions,
	basePath string,
	enclosingFnHash types.Hash,
	tmplTable *TemplateTable,
) []types.Edge {
	if node == nil || tmplTable == nil {
		return nil
	}

	var edges []types.Edge
	walkForTemplateUsage(node, opts, basePath, enclosingFnHash, tmplTable, &edges)
	return edges
}

// walkForTemplateUsage recursively looks for template_type in declaration type
// specifiers and function call targets.
func walkForTemplateUsage(
	node *sitter.Node,
	opts types.ExtractOptions,
	basePath string,
	enclosingFnHash types.Hash,
	tmplTable *TemplateTable,
	edges *[]types.Edge,
) {
	if node == nil {
		return
	}

	// Match template_type in variable/parameter declarations.
	if node.Type() == "declaration" {
		edgesForDecl := checkDeclForTemplateUsage(node, opts, basePath, enclosingFnHash, tmplTable)
		*edges = append(*edges, edgesForDecl...)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForTemplateUsage(node.Child(i), opts, basePath, enclosingFnHash, tmplTable, edges)
	}
}

// checkDeclForTemplateUsage checks a declaration for template_type usage and
// creates instantiates edges.
func checkDeclForTemplateUsage(
	decl *sitter.Node,
	opts types.ExtractOptions,
	basePath string,
	enclosingFnHash types.Hash,
	tmplTable *TemplateTable,
) []types.Edge {
	var edges []types.Edge

	// Walk children looking for template_type.
	for i := 0; i < int(decl.ChildCount()); i++ {
		child := decl.Child(i)
		if child.Type() == "template_type" {
			// Extract base type name from the template_type.
			baseName := extractBaseNameFromTemplateType(child, opts.Content)
			if baseName != "" {
				if hash, ok := tmplTable.Lookup(baseName); ok {
					edgeHash := types.ComputeEdgeHash(enclosingFnHash, hash, edgetype.Instantiates, "ast_resolved")
					edges = append(edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: enclosingFnHash,
						TargetHash: hash,
						EdgeType:   edgetype.Instantiates,
						Confidence: 0.85,
						Provenance: "ast_resolved",
					})
				}
			}
		}
	}

	return edges
}

// extractBaseNameFromTemplateType extracts the base type name from a template_type node.
// e.g. "Vec<int>" -> "Vec", "std::vector<std::string>" -> "vector" (via nested type_identifier)
func extractBaseNameFromTemplateType(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	// template_type -> type_identifier child has the base name.
	typeId := findChildByType(node, "type_identifier")
	if typeId != nil {
		return typeId.Content(content)
	}

	return ""
}

// extractConceptFromTemplateType extracts the concept name from a template_type
// that appears in a requires clause.
// e.g. "Sortable<T>" -> "Sortable", "std::is_integral_v<T>" -> "is_integral_v"
func extractConceptFromTemplateType(node *sitter.Node, content []byte) string {
	if node == nil {
		return ""
	}

	text := node.Content(content)
	return extractConceptName(text)
}
