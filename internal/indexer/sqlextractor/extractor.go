// Package sqlextractor provides a tree-sitter based extractor for SQL files.
// It implements types.Extractor and produces nodes for CREATE TABLE, CREATE VIEW,
// CREATE FUNCTION, and CREATE PROCEDURE statements, plus edges for table references,
// procedure/function calls, and view-to-table dependencies.
//
// Provenance is "ast_inferred" and confidence is 0.7 since tree-sitter SQL
// parsing does not provide full semantic analysis.
package sqlextractor

import (
	"context"
	"fmt"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/sql"

	"github.com/blackwell-systems/knowing/internal/types"
)

const (
	provenance = "ast_inferred"
	confidence = 0.7
)

// SQLExtractor implements types.Extractor for SQL files using tree-sitter
// AST parsing.
// Thread-safe: each Extract call creates its own parser (required for
// concurrent use; tree-sitter parsers are not goroutine-safe).
type SQLExtractor struct{}

// NewSQLExtractor creates a new SQLExtractor.
func NewSQLExtractor() *SQLExtractor {
	return &SQLExtractor{}
}

// Name returns the extractor name.
func (e *SQLExtractor) Name() string {
	return "treesitter-sql"
}

// CanHandle returns true for files ending in .sql.
func (e *SQLExtractor) CanHandle(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".sql")
}

// Extract parses the SQL file with tree-sitter and produces nodes for
// DDL statements and edges for references between objects.
func (e *SQLExtractor) Extract(ctx context.Context, opts types.ExtractOptions) (*types.ExtractResult, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(sql.GetLanguage())
	tree, err := parser.ParseCtx(ctx, nil, opts.Content)
	if err != nil {
		return nil, fmt.Errorf("tree-sitter parse: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()

	var nodes []types.Node
	var edges []types.Edge

	// Walk all top-level statements.
	walkStatements(root, opts, &nodes, &edges)

	// Deduplicate edges.
	edges = deduplicateEdges(edges)

	return &types.ExtractResult{
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// walkStatements recursively walks the AST looking for CREATE statements
// and DML statements that produce edges.
func walkStatements(node *sitter.Node, opts types.ExtractOptions, nodes *[]types.Node, edges *[]types.Edge) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	switch {
	case isCreateTableStatement(nodeType):
		n := extractCreateTable(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
		}
		return

	case isCreateViewStatement(nodeType):
		n, viewEdges := extractCreateView(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
			*edges = append(*edges, viewEdges...)
		}
		return

	case isCreateFunctionStatement(nodeType):
		n, fnEdges := extractCreateFunction(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
			*edges = append(*edges, fnEdges...)
		}
		return

	case isCreateProcedureStatement(nodeType):
		n, procEdges := extractCreateProcedure(node, opts)
		if n != nil {
			*nodes = append(*nodes, *n)
			*edges = append(*edges, procEdges...)
		}
		return
	}

	// Process children, looking for ERROR nodes that contain CREATE FUNCTION/PROCEDURE.
	// The body (block node) may be a sibling of the ERROR node.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()

		if childType == "ERROR" {
			text := child.Content(opts.Content)
			upper := strings.ToUpper(text)

			// Look for a sibling block node that follows this ERROR (procedure/function body).
			var bodyNode *sitter.Node
			if i+1 < int(node.ChildCount()) {
				sibling := node.Child(i + 1)
				if sibling.Type() == "block" {
					bodyNode = sibling
				}
			}

			if strings.Contains(upper, "CREATE FUNCTION") || strings.Contains(upper, "CREATE OR REPLACE FUNCTION") {
				n, fnEdges := extractCreateFunctionFromError(child, bodyNode, opts)
				if n != nil {
					*nodes = append(*nodes, *n)
					*edges = append(*edges, fnEdges...)
				}
			} else if strings.Contains(upper, "CREATE PROCEDURE") || strings.Contains(upper, "CREATE OR REPLACE PROCEDURE") {
				n, procEdges := extractCreateProcedureFromError(child, bodyNode, opts)
				if n != nil {
					*nodes = append(*nodes, *n)
					*edges = append(*edges, procEdges...)
				}
			} else {
				// Recurse into unrecognized ERROR nodes.
				walkStatements(child, opts, nodes, edges)
			}
		} else {
			walkStatements(child, opts, nodes, edges)
		}
	}
}

func isCreateTableStatement(nodeType string) bool {
	return nodeType == "create_table_statement" || nodeType == "create_table"
}

func isCreateViewStatement(nodeType string) bool {
	return nodeType == "create_view_statement" || nodeType == "create_view"
}

func isCreateFunctionStatement(nodeType string) bool {
	return nodeType == "create_function_statement" || nodeType == "create_function"
}

func isCreateProcedureStatement(nodeType string) bool {
	return nodeType == "create_procedure_statement" || nodeType == "create_procedure"
}

// extractCreateTable extracts a CREATE TABLE statement as a "table" node.
func extractCreateTable(node *sitter.Node, opts types.ExtractOptions) *types.Node {
	name := findObjectName(node, opts.Content, "CREATE", "TABLE")
	if name == "" {
		return nil
	}
	line := int(node.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.table.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "table")

	return &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "table",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE TABLE %s", name),
	}
}

// extractCreateView extracts a CREATE VIEW statement as a "view" node
// and produces depends_on edges to tables referenced in the SELECT body.
func extractCreateView(node *sitter.Node, opts types.ExtractOptions) (*types.Node, []types.Edge) {
	name := findObjectName(node, opts.Content, "CREATE", "VIEW")
	if name == "" {
		return nil, nil
	}
	line := int(node.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.view.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "view")

	viewNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "view",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE VIEW %s", name),
	}

	// Find table references in the view body (SELECT statement).
	tableRefs := findTableReferences(node, opts.Content)
	var edges []types.Edge
	for _, ref := range tableRefs {
		if strings.EqualFold(ref, name) {
			continue // skip self-references
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref, "table")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "depends_on", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "depends_on",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return viewNode, edges
}

// extractCreateFunctionFromError extracts a CREATE FUNCTION from an ERROR node,
// with an optional body (block) node that may be a sibling.
func extractCreateFunctionFromError(errorNode *sitter.Node, bodyNode *sitter.Node, opts types.ExtractOptions) (*types.Node, []types.Edge) {
	name := findObjectName(errorNode, opts.Content, "CREATE", "FUNCTION")
	if name == "" {
		return nil, nil
	}
	line := int(errorNode.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.function.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "function")

	fnNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "function",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE FUNCTION %s", name),
	}

	var edges []types.Edge

	// Search for table refs in both the ERROR node and the body node.
	tableRefs := findTableReferences(errorNode, opts.Content)
	if bodyNode != nil {
		bodyRefs := findTableReferences(bodyNode, opts.Content)
		tableRefs = append(tableRefs, bodyRefs...)
	}
	tableRefs = deduplicateStrings(tableRefs)

	for _, ref := range tableRefs {
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref, "table")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "references", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	// Function calls.
	calls := findFunctionCalls(errorNode, opts.Content)
	if bodyNode != nil {
		bodyCalls := findFunctionCalls(bodyNode, opts.Content)
		calls = append(calls, bodyCalls...)
	}
	calls = deduplicateStrings(calls)

	for _, call := range calls {
		if strings.EqualFold(call, name) {
			continue
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, call, "function")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return fnNode, edges
}

// extractCreateProcedureFromError extracts a CREATE PROCEDURE from an ERROR node,
// with an optional body (block) node that may be a sibling.
func extractCreateProcedureFromError(errorNode *sitter.Node, bodyNode *sitter.Node, opts types.ExtractOptions) (*types.Node, []types.Edge) {
	name := findObjectName(errorNode, opts.Content, "CREATE", "PROCEDURE")
	if name == "" {
		return nil, nil
	}
	line := int(errorNode.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.procedure.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "procedure")

	procNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "procedure",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE PROCEDURE %s", name),
	}

	var edges []types.Edge

	// Search for table refs in both the ERROR node and the body node.
	tableRefs := findTableReferences(errorNode, opts.Content)
	if bodyNode != nil {
		bodyRefs := findTableReferences(bodyNode, opts.Content)
		tableRefs = append(tableRefs, bodyRefs...)
	}
	tableRefs = deduplicateStrings(tableRefs)

	for _, ref := range tableRefs {
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref, "table")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "references", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	// Function/procedure calls.
	calls := findFunctionCalls(errorNode, opts.Content)
	if bodyNode != nil {
		bodyCalls := findFunctionCalls(bodyNode, opts.Content)
		calls = append(calls, bodyCalls...)
	}
	calls = deduplicateStrings(calls)

	for _, call := range calls {
		if strings.EqualFold(call, name) {
			continue
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, call, "function")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return procNode, edges
}

// extractCreateFunction extracts a CREATE FUNCTION statement as a "function" node
// and produces edges for table references and function calls within the body.
func extractCreateFunction(node *sitter.Node, opts types.ExtractOptions) (*types.Node, []types.Edge) {
	name := findObjectName(node, opts.Content, "CREATE", "FUNCTION")
	if name == "" {
		return nil, nil
	}
	line := int(node.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.function.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "function")

	fnNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "function",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE FUNCTION %s", name),
	}

	var edges []types.Edge

	// Table references produce "references" edges.
	tableRefs := findTableReferences(node, opts.Content)
	for _, ref := range tableRefs {
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref, "table")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "references", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	// Function/procedure calls produce "calls" edges.
	calls := findFunctionCalls(node, opts.Content)
	for _, call := range calls {
		if strings.EqualFold(call, name) {
			continue // skip self-calls
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, call, "function")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return fnNode, edges
}

// extractCreateProcedure extracts a CREATE PROCEDURE statement as a "procedure" node
// and produces edges for table references and calls within the body.
func extractCreateProcedure(node *sitter.Node, opts types.ExtractOptions) (*types.Node, []types.Edge) {
	name := findObjectName(node, opts.Content, "CREATE", "PROCEDURE")
	if name == "" {
		return nil, nil
	}
	line := int(node.StartPoint().Row) + 1
	qn := fmt.Sprintf("%s://%s.procedure.%s", opts.RepoURL, opts.FilePath, name)
	nodeHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, name, "procedure")

	procNode := &types.Node{
		NodeHash:      nodeHash,
		FileHash:      opts.FileHash,
		QualifiedName: qn,
		Kind:          "procedure",
		Line:          line,
		Signature:     fmt.Sprintf("CREATE PROCEDURE %s", name),
	}

	var edges []types.Edge

	// Table references produce "references" edges.
	tableRefs := findTableReferences(node, opts.Content)
	for _, ref := range tableRefs {
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, ref, "table")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "references", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	// Function/procedure calls produce "calls" edges.
	calls := findFunctionCalls(node, opts.Content)
	for _, call := range calls {
		if strings.EqualFold(call, name) {
			continue // skip self-calls
		}
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.FilePath, types.EmptyHash, call, "function")
		edgeHash := types.ComputeEdgeHash(nodeHash, targetHash, "calls", provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: nodeHash,
			TargetHash: targetHash,
			EdgeType:   "calls",
			Confidence: confidence,
			Provenance: provenance,
		})
	}

	return procNode, edges
}

// findObjectName extracts the object name from a CREATE statement node.
// It looks for the identifier following the keyword sequence (e.g., CREATE TABLE name).
func findObjectName(node *sitter.Node, content []byte, keywords ...string) string {
	// Strategy: walk children looking for identifier nodes that follow the keywords.
	// The tree-sitter SQL grammar varies, so we use a text-based fallback.
	text := node.Content(content)
	return extractNameFromCreateStatement(text, keywords...)
}

// extractNameFromCreateStatement parses the object name from a CREATE statement text.
// Handles: CREATE [OR REPLACE] [TEMP|TEMPORARY] TABLE|VIEW|FUNCTION|PROCEDURE [IF NOT EXISTS] name
func extractNameFromCreateStatement(text string, keywords ...string) string {
	// Normalize whitespace and tokenize.
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}

	// Find the position after the last keyword.
	lastKeywordIdx := -1
	for i, field := range fields {
		upper := strings.ToUpper(field)
		for _, kw := range keywords {
			if upper == kw {
				lastKeywordIdx = i
			}
		}
		// Skip OR REPLACE, TEMP, TEMPORARY.
		if upper == "OR" || upper == "REPLACE" || upper == "TEMP" || upper == "TEMPORARY" {
			continue
		}
	}

	if lastKeywordIdx < 0 {
		return ""
	}

	// Skip past IF NOT EXISTS.
	idx := lastKeywordIdx + 1
	if idx < len(fields) && strings.ToUpper(fields[idx]) == "IF" {
		// Skip "IF NOT EXISTS"
		idx += 3
	}

	if idx >= len(fields) {
		return ""
	}

	name := fields[idx]
	// Clean up: remove parentheses, semicolons, backticks, quotes, brackets.
	// If name contains '(', take only the part before it (e.g., "func_name()" -> "func_name").
	if parenIdx := strings.IndexByte(name, '('); parenIdx >= 0 {
		name = name[:parenIdx]
	}
	name = strings.TrimRight(name, ";")
	name = strings.Trim(name, "`\"[]")
	// Handle schema-qualified names (schema.name): take only the object name.
	if parts := strings.Split(name, "."); len(parts) > 1 {
		name = parts[len(parts)-1]
	}
	return name
}

// findTableReferences recursively walks an AST node looking for table/relation
// references in FROM, JOIN, INSERT INTO, UPDATE, and DELETE FROM clauses.
func findTableReferences(node *sitter.Node, content []byte) []string {
	var refs []string
	seen := make(map[string]bool)
	walkForTableRefs(node, content, &refs, seen)
	return refs
}

// walkForTableRefs recursively finds table references in query nodes.
func walkForTableRefs(node *sitter.Node, content []byte, refs *[]string, seen map[string]bool) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	// Look for nodes that typically contain table references.
	switch nodeType {
	case "from_clause", "from":
		collectTableNamesFromClause(node, content, refs, seen)
	case "join_clause", "join":
		collectTableNamesFromClause(node, content, refs, seen)
	case "insert_statement", "insert":
		collectInsertTarget(node, content, refs, seen)
	case "update_statement", "update":
		collectUpdateTarget(node, content, refs, seen)
	case "delete_statement", "delete":
		collectDeleteTarget(node, content, refs, seen)
	case "relation", "table_reference", "object_reference":
		name := extractIdentifierFromNode(node, content)
		if name != "" && !seen[name] {
			seen[name] = true
			*refs = append(*refs, name)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForTableRefs(node.Child(i), content, refs, seen)
	}
}

// collectTableNamesFromClause extracts table names from FROM/JOIN clauses.
func collectTableNamesFromClause(node *sitter.Node, content []byte, refs *[]string, seen map[string]bool) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		childType := child.Type()
		if childType == "relation" || childType == "table_reference" ||
			childType == "object_reference" || childType == "identifier" ||
			childType == "dotted_name" {
			name := extractIdentifierFromNode(child, content)
			if name != "" && !isKeyword(name) && !seen[name] {
				seen[name] = true
				*refs = append(*refs, name)
			}
		}
	}
}

// collectInsertTarget extracts the target table from an INSERT statement.
func collectInsertTarget(node *sitter.Node, content []byte, refs *[]string, seen map[string]bool) {
	// Look for the table name after "INTO" keyword.
	text := node.Content(content)
	name := extractTableAfterKeyword(text, "INTO")
	if name != "" && !seen[name] {
		seen[name] = true
		*refs = append(*refs, name)
	}
}

// collectUpdateTarget extracts the target table from an UPDATE statement.
func collectUpdateTarget(node *sitter.Node, content []byte, refs *[]string, seen map[string]bool) {
	text := node.Content(content)
	name := extractTableAfterKeyword(text, "UPDATE")
	if name != "" && !seen[name] {
		seen[name] = true
		*refs = append(*refs, name)
	}
}

// collectDeleteTarget extracts the target table from a DELETE statement.
func collectDeleteTarget(node *sitter.Node, content []byte, refs *[]string, seen map[string]bool) {
	text := node.Content(content)
	name := extractTableAfterKeyword(text, "FROM")
	if name != "" && !seen[name] {
		seen[name] = true
		*refs = append(*refs, name)
	}
}

// extractTableAfterKeyword finds the first identifier after a given keyword in SQL text.
func extractTableAfterKeyword(text, keyword string) string {
	fields := strings.Fields(text)
	for i, f := range fields {
		if strings.ToUpper(f) == keyword && i+1 < len(fields) {
			name := fields[i+1]
			name = strings.TrimRight(name, "(;,")
			name = strings.Trim(name, "`\"[]")
			if isKeyword(name) {
				continue
			}
			if parts := strings.Split(name, "."); len(parts) > 1 {
				name = parts[len(parts)-1]
			}
			return name
		}
	}
	return ""
}

// extractIdentifierFromNode gets the text content of an identifier-like node.
func extractIdentifierFromNode(node *sitter.Node, content []byte) string {
	text := node.Content(content)
	text = strings.TrimSpace(text)
	// Take just the first token (in case there's an alias).
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return ""
	}
	name := parts[0]
	name = strings.TrimRight(name, "(;,")
	name = strings.Trim(name, "`\"[]")
	if dotParts := strings.Split(name, "."); len(dotParts) > 1 {
		name = dotParts[len(dotParts)-1]
	}
	return name
}

// findFunctionCalls recursively walks an AST node looking for function/procedure
// invocations (CALL, EXEC, or function_call nodes).
func findFunctionCalls(node *sitter.Node, content []byte) []string {
	var calls []string
	seen := make(map[string]bool)
	walkForFunctionCalls(node, content, &calls, seen)
	return calls
}

// walkForFunctionCalls recursively finds function calls.
func walkForFunctionCalls(node *sitter.Node, content []byte, calls *[]string, seen map[string]bool) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	switch nodeType {
	case "function_call", "invocation":
		name := extractFunctionCallName(node, content)
		if name != "" && !isBuiltinFunction(name) && !seen[name] {
			seen[name] = true
			*calls = append(*calls, name)
		}
	case "call_statement", "execute_statement":
		name := extractCallStatementName(node, content)
		if name != "" && !isBuiltinFunction(name) && !seen[name] {
			seen[name] = true
			*calls = append(*calls, name)
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForFunctionCalls(node.Child(i), content, calls, seen)
	}
}

// extractFunctionCallName extracts the function name from a function_call node.
func extractFunctionCallName(node *sitter.Node, content []byte) string {
	// Try the "name" field first.
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Content(content)
	}
	// Fallback: first identifier child.
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "object_reference" || child.Type() == "dotted_name" {
			name := child.Content(content)
			name = strings.Trim(name, "`\"[]")
			return name
		}
	}
	return ""
}

// extractCallStatementName extracts the procedure name from CALL/EXEC statements.
func extractCallStatementName(node *sitter.Node, content []byte) string {
	text := node.Content(content)
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return ""
	}
	// Skip CALL or EXEC/EXECUTE keyword.
	upper := strings.ToUpper(fields[0])
	if upper == "CALL" || upper == "EXEC" || upper == "EXECUTE" {
		name := fields[1]
		name = strings.TrimRight(name, "(;,")
		name = strings.Trim(name, "`\"[]")
		return name
	}
	return ""
}

// isKeyword returns true if the string is a SQL keyword (not a table name).
func isKeyword(s string) bool {
	upper := strings.ToUpper(s)
	keywords := map[string]bool{
		"SELECT": true, "FROM": true, "WHERE": true, "AND": true, "OR": true,
		"JOIN": true, "INNER": true, "LEFT": true, "RIGHT": true, "OUTER": true,
		"ON": true, "AS": true, "IN": true, "NOT": true, "NULL": true,
		"INSERT": true, "INTO": true, "VALUES": true, "UPDATE": true, "SET": true,
		"DELETE": true, "CREATE": true, "TABLE": true, "VIEW": true,
		"DROP": true, "ALTER": true, "INDEX": true, "GROUP": true, "BY": true,
		"ORDER": true, "HAVING": true, "LIMIT": true, "OFFSET": true,
		"UNION": true, "ALL": true, "DISTINCT": true, "EXISTS": true,
		"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
		"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "TRANSACTION": true,
		"IF": true, "REPLACE": true, "TEMPORARY": true, "TEMP": true,
		"FUNCTION": true, "PROCEDURE": true, "RETURNS": true, "RETURN": true,
		"DECLARE": true, "CURSOR": true, "FETCH": true, "CROSS": true,
		"FULL": true, "NATURAL": true, "USING": true, "BETWEEN": true,
	}
	return keywords[upper]
}

// isBuiltinFunction returns true if the function name is a SQL built-in.
func isBuiltinFunction(name string) bool {
	upper := strings.ToUpper(name)
	builtins := map[string]bool{
		"COUNT": true, "SUM": true, "AVG": true, "MIN": true, "MAX": true,
		"COALESCE": true, "NULLIF": true, "CAST": true, "CONVERT": true,
		"TRIM": true, "UPPER": true, "LOWER": true, "LENGTH": true, "LEN": true,
		"SUBSTRING": true, "SUBSTR": true, "REPLACE": true, "CONCAT": true,
		"NOW": true, "GETDATE": true, "CURRENT_TIMESTAMP": true,
		"DATEADD": true, "DATEDIFF": true, "YEAR": true, "MONTH": true, "DAY": true,
		"ISNULL": true, "IFNULL": true, "NVL": true, "IIF": true,
		"ROW_NUMBER": true, "RANK": true, "DENSE_RANK": true,
		"LAG": true, "LEAD": true, "FIRST_VALUE": true, "LAST_VALUE": true,
		"ROUND": true, "FLOOR": true, "CEILING": true, "ABS": true,
		"LEFT": true, "RIGHT": true, "CHARINDEX": true, "PATINDEX": true,
		"STRING_AGG": true, "GROUP_CONCAT": true, "LISTAGG": true,
	}
	return builtins[upper]
}

// deduplicateStrings removes duplicate strings preserving order.
func deduplicateStrings(items []string) []string {
	if len(items) <= 1 {
		return items
	}
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
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
