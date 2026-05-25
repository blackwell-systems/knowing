// Package docextract provides language-agnostic docstring extraction from tree-sitter AST nodes.
package docextract

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// FromPrecedingComments extracts documentation from comment nodes immediately
// preceding a declaration. Works for languages where doc comments are siblings
// of the declaration node (Go, Rust, Java, C#, TypeScript).
//
// Handles comment markers for all supported languages:
//   - Go: // and /* */
//   - Rust: /// and //! and /* */
//   - Java/TypeScript: /** */ (JSDoc/Javadoc)
//   - C#: /// (XML doc comments)
//
// Returns up to maxLen characters of stripped comment text.
func FromPrecedingComments(node *sitter.Node, content []byte, maxLen int) string {
	if node == nil {
		return ""
	}
	if maxLen <= 0 {
		maxLen = 500
	}

	var commentLines []string
	prev := node.PrevSibling()
	for prev != nil {
		nodeType := prev.Type()
		if nodeType == "comment" || nodeType == "line_comment" || nodeType == "block_comment" || nodeType == "documentation_comment" {
			text := prev.Content(content)
			text = stripCommentMarkers(text)
			if text != "" {
				commentLines = append([]string{text}, commentLines...)
			}
			prev = prev.PrevSibling()
		} else {
			break
		}
	}

	if len(commentLines) == 0 {
		return ""
	}

	doc := strings.Join(commentLines, " ")
	if len(doc) > maxLen {
		doc = doc[:maxLen]
	}
	return doc
}

// FromBodyFirstString extracts a Python-style docstring (first string literal
// in the function/class body). Already implemented in the treesitter extractor;
// this is the shared version for reuse.
func FromBodyFirstString(body *sitter.Node, content []byte, maxLen int) string {
	if body == nil || body.ChildCount() == 0 {
		return ""
	}
	if maxLen <= 0 {
		maxLen = 500
	}

	first := body.Child(0)
	if first == nil {
		return ""
	}
	if first.Type() != "expression_statement" {
		return ""
	}
	if first.ChildCount() == 0 {
		return ""
	}
	strNode := first.Child(0)
	if strNode == nil {
		return ""
	}
	if strNode.Type() != "string" && strNode.Type() != "concatenated_string" {
		return ""
	}
	raw := strNode.Content(content)
	doc := stripStringQuotes(raw)
	doc = strings.TrimSpace(doc)
	if len(doc) > maxLen {
		doc = doc[:maxLen]
	}
	return doc
}

// stripCommentMarkers removes common comment prefixes and suffixes.
func stripCommentMarkers(text string) string {
	// Multi-line block comments: /** ... */ or /* ... */
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")

	// Line comments with doc markers
	text = strings.TrimPrefix(text, "///")
	text = strings.TrimPrefix(text, "//!")
	text = strings.TrimPrefix(text, "//")

	// Strip leading * on each line (Javadoc/JSDoc style)
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, " ")
}

// stripStringQuotes removes Python string quote wrappers.
func stripStringQuotes(raw string) string {
	doc := strings.TrimPrefix(raw, `"""`)
	doc = strings.TrimSuffix(doc, `"""`)
	doc = strings.TrimPrefix(doc, `'''`)
	doc = strings.TrimSuffix(doc, `'''`)
	doc = strings.TrimPrefix(doc, `"`)
	doc = strings.TrimSuffix(doc, `"`)
	doc = strings.TrimPrefix(doc, `'`)
	doc = strings.TrimSuffix(doc, `'`)
	return doc
}
