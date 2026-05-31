package tsresolve

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// tsStdlibNames is the set of TypeScript/JavaScript standard library names
// that should NOT be module-qualified. These stay bare for registry matching.
var tsStdlibNames = map[string]bool{
	"Array": true, "Promise": true, "Map": true, "Set": true,
	"WeakMap": true, "WeakSet": true, "Object": true, "String": true,
	"Number": true, "Boolean": true, "BigInt": true, "Symbol": true,
	"Function": true, "Date": true, "RegExp": true, "Error": true,
	"Math": true, "JSON": true, "console": true,
	"Element": true, "HTMLElement": true, "Document": true,
	"Window": true, "Node": true, "EventTarget": true, "Event": true,
	"Response": true, "Request": true, "Headers": true, "URL": true,
	"URLSearchParams": true, "FormData": true, "Blob": true, "File": true,
	"NodeList": true, "HTMLCollection": true,
	"Iterable": true, "Iterator": true, "Generator": true,
	"AsyncIterable": true, "AsyncIterator": true, "AsyncGenerator": true,
	"ReadonlyArray": true, "Partial": true, "Required": true,
	"Readonly": true, "Pick": true, "Omit": true, "Record": true,
	"Exclude": true, "Extract": true, "NonNullable": true,
	"Parameters": true, "ReturnType": true, "Awaited": true,
	"InstanceType": true, "ConstructorParameters": true,
	"Uppercase": true, "Lowercase": true, "Capitalize": true,
	"Uncapitalize": true, "ThisType": true,
}

// tsBuiltinPrimitives is the set of TypeScript builtin primitive type names.
var tsBuiltinPrimitives = map[string]bool{
	"string": true, "number": true, "boolean": true, "bigint": true,
	"any": true, "unknown": true, "void": true, "never": true,
	"null": true, "undefined": true, "object": true, "symbol": true,
}

// IsBuiltinType, ResolveBuiltinType, BuiltinWrapperClass, LiteralType,
// and UnwrapPromise are defined in builtins.go.

// ParseTypeText parses a TypeScript type annotation text string into a
// typresolve.Type. This is the TS equivalent of Go's ParseTypeNode and
// Python's ParseAnnotation. Handles builtin primitives, generic
// instantiation, array shorthand, union, intersection, tuple, function
// types, and qualified names.
func ParseTypeText(text string, moduleQN string) *typresolve.Type {
	// Step 1: Trim whitespace and leading ":"
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, ":")
	text = strings.TrimSpace(text)
	text = strings.TrimRight(text, ";,")
	text = strings.TrimSpace(text)

	if text == "" {
		return typresolve.Unknown()
	}

	// Step 2: Function type "(params) => returnType"
	if strings.HasPrefix(text, "(") {
		if t := parseFunctionType(text, moduleQN); t != nil {
			return t
		}
	}

	// Step 3: Object literal type "{...}"
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		return typresolve.Unknown()
	}

	// Step 4: Tuple "[T, U]"
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		inner := text[1 : len(text)-1]
		parts := splitAtDepthZero(inner, ',')
		var elems []*typresolve.Type
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				elems = append(elems, ParseTypeText(p, moduleQN))
			}
		}
		return typresolve.Tuple(elems)
	}

	// Step 5: Union "A | B" (top-level | at depth 0)
	if parts := splitAtDepthZero(text, '|'); len(parts) > 1 {
		return parseUnion(parts, moduleQN)
	}

	// Step 6: Intersection "A & B"
	if parts := splitAtDepthZero(text, '&'); len(parts) > 1 {
		// Best-effort: return first member.
		return ParseTypeText(strings.TrimSpace(parts[0]), moduleQN)
	}

	// Step 7: Builtin primitives
	if tsBuiltinPrimitives[text] {
		return typresolve.Builtin(text)
	}

	// Step 8: Array shorthand "T[]"
	if strings.HasSuffix(text, "[]") {
		inner := strings.TrimSuffix(text, "[]")
		return typresolve.Slice(ParseTypeText(inner, moduleQN))
	}

	// Step 9: Generic instantiation "Foo<A, B>"
	if idx := findGenericOpen(text); idx >= 0 && strings.HasSuffix(text, ">") {
		return parseGenericType(text, idx, moduleQN)
	}

	// Step 10: Qualified name "a.b.c"
	if strings.Contains(text, ".") {
		return typresolve.Named(text)
	}

	// Step 11: Bare identifier
	return parseBareIdentifier(text, moduleQN)
}

// parseFunctionType parses "(params) => returnType" into a Func type.
func parseFunctionType(text string, moduleQN string) *typresolve.Type {
	// Find balanced closing paren.
	depth := 0
	closeIdx := -1
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeIdx = i
				goto found
			}
		}
	}
found:
	if closeIdx < 0 {
		return nil
	}

	// Look for " => " after the closing paren.
	rest := text[closeIdx+1:]
	arrowIdx := strings.Index(rest, " => ")
	if arrowIdx < 0 {
		return nil
	}

	paramsText := text[1:closeIdx]
	returnText := rest[arrowIdx+4:]

	// Parse parameters.
	var params []typresolve.Param
	if strings.TrimSpace(paramsText) != "" {
		paramParts := splitAtDepthZero(paramsText, ',')
		for _, p := range paramParts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			// Remove optional "?" marker.
			p = strings.TrimSuffix(p, "?")
			// Split "name: type" or "name?: type"
			if colonIdx := findTopLevelColon(p); colonIdx >= 0 {
				name := strings.TrimSpace(p[:colonIdx])
				name = strings.TrimSuffix(name, "?")
				typText := strings.TrimSpace(p[colonIdx+1:])
				params = append(params, typresolve.Param{
					Name: name,
					Type: ParseTypeText(typText, moduleQN),
				})
			} else {
				// Bare type or just a name.
				params = append(params, typresolve.Param{
					Name: "",
					Type: ParseTypeText(p, moduleQN),
				})
			}
		}
	}

	var returns []*typresolve.Type
	returnText = strings.TrimSpace(returnText)
	if returnText != "" && returnText != "void" {
		returns = append(returns, ParseTypeText(returnText, moduleQN))
	}

	return typresolve.Func(params, returns)
}

// parseUnion handles union type "A | B".
func parseUnion(parts []string, moduleQN string) *typresolve.Type {
	// Check for nullable pattern: exactly 2 members, one is null/undefined.
	if len(parts) == 2 {
		a := strings.TrimSpace(parts[0])
		b := strings.TrimSpace(parts[1])
		if a == "null" || a == "undefined" {
			return typresolve.Optional(ParseTypeText(b, moduleQN))
		}
		if b == "null" || b == "undefined" {
			return typresolve.Optional(ParseTypeText(a, moduleQN))
		}
	}
	// Best-effort: return first non-null/undefined member.
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "null" && p != "undefined" {
			return ParseTypeText(p, moduleQN)
		}
	}
	return typresolve.Unknown()
}

// parseGenericType handles "Foo<A, B>" generic instantiation.
func parseGenericType(text string, openIdx int, moduleQN string) *typresolve.Type {
	baseName := text[:openIdx]
	argsText := text[openIdx+1 : len(text)-1]
	args := splitAtDepthZero(argsText, ',')

	var typeArgs []*typresolve.Type
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a != "" {
			typeArgs = append(typeArgs, ParseTypeText(a, moduleQN))
		}
	}

	// Special cases for well-known generics.
	switch baseName {
	case "Array", "ReadonlyArray":
		if len(typeArgs) > 0 {
			return typresolve.Slice(typeArgs[0])
		}
		return typresolve.Slice(typresolve.Unknown())

	case "Map":
		k := typresolve.Unknown()
		v := typresolve.Unknown()
		if len(typeArgs) > 0 {
			k = typeArgs[0]
		}
		if len(typeArgs) > 1 {
			v = typeArgs[1]
		}
		return typresolve.Map(k, v)

	case "Record":
		k := typresolve.Unknown()
		v := typresolve.Unknown()
		if len(typeArgs) > 0 {
			k = typeArgs[0]
		}
		if len(typeArgs) > 1 {
			v = typeArgs[1]
		}
		return typresolve.Map(k, v)

	case "Set", "WeakSet":
		t := typresolve.Named(baseName)
		if len(typeArgs) > 0 {
			t.TypeParams = toTypeParams(typeArgs)
		}
		return t

	case "Promise":
		t := typresolve.Named("Promise")
		if len(typeArgs) > 0 {
			t.TypeParams = toTypeParams(typeArgs)
		}
		return t

	case "Partial", "Required", "Readonly", "NonNullable":
		// Unwrap utility types to the inner type for method dispatch.
		if len(typeArgs) > 0 {
			return typeArgs[0]
		}
		return typresolve.Unknown()

	case "ReturnType":
		return typresolve.Unknown()
	}

	// Project-local or other generic type.
	qualifiedBase := qualifyName(baseName, moduleQN)
	t := typresolve.Named(qualifiedBase)
	if len(typeArgs) > 0 {
		t.TypeParams = toTypeParams(typeArgs)
	}
	return t
}

// parseBareIdentifier handles a bare identifier with optional module
// qualification.
func parseBareIdentifier(text string, moduleQN string) *typresolve.Type {
	qualified := qualifyName(text, moduleQN)
	return typresolve.Named(qualified)
}

// qualifyName qualifies a bare name with the module QN if it is not a
// stdlib name and starts with an uppercase letter.
func qualifyName(name string, moduleQN string) string {
	if tsStdlibNames[name] {
		return name
	}
	if moduleQN != "" && len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return moduleQN + "." + name
	}
	return name
}

// toTypeParams converts a slice of types into TypeParam entries.
func toTypeParams(types []*typresolve.Type) []typresolve.TypeParam {
	params := make([]typresolve.TypeParam, len(types))
	for i, t := range types {
		params[i] = typresolve.TypeParam{
			Name:       t.Name,
			Constraint: t,
		}
	}
	return params
}

// findGenericOpen finds the index of the '<' that opens a generic parameter
// list at the end of a type name. Returns -1 if not found.
func findGenericOpen(text string) int {
	if !strings.HasSuffix(text, ">") {
		return -1
	}
	depth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case '>':
			depth++
		case '<':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitAtDepthZero splits text at the given separator character, but only
// at depth 0 (ignoring separators inside <>, [], ()). Trims whitespace
// from each part.
func splitAtDepthZero(text string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '<', '[', '(':
			depth++
		case '>', ']', ')':
			depth--
		default:
			if text[i] == sep && depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(text[start:]))
	return parts
}

// findTopLevelColon finds the index of the first ':' at depth 0,
// ignoring colons inside <>, [], (). Returns -1 if not found.
func findTopLevelColon(text string) int {
	depth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '<', '[', '(':
			depth++
		case '>', ']', ')':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ParseTypeNode parses a tree-sitter TypeScript type AST node into a
// typresolve.Type. Handles type_annotation, type_identifier, generic_type,
// union_type, intersection_type, array_type, tuple_type,
// parenthesized_type, function_type, and predefined_type nodes.
// Falls back to ParseTypeText for complex cases.
func ParseTypeNode(node *sitter.Node, content []byte, moduleQN string, imports map[string]ImportInfo) *typresolve.Type {
	if node == nil {
		return nil
	}

	switch node.Type() {
	case "type_annotation":
		// Extract the type child (skip the leading ":")
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				return ParseTypeNode(child, content, moduleQN, imports)
			}
		}
		// Fallback: extract text.
		return ParseTypeText(node.Content(content), moduleQN)

	case "type_identifier", "predefined_type":
		text := node.Content(content)
		return ParseTypeText(text, moduleQN)

	case "generic_type":
		// Extract base type and type arguments.
		var baseName string
		var typeArgNodes []*sitter.Node
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child == nil {
				continue
			}
			switch child.Type() {
			case "type_identifier", "nested_type_identifier":
				baseName = child.Content(content)
			case "type_arguments":
				for j := 0; j < int(child.NamedChildCount()); j++ {
					arg := child.NamedChild(j)
					if arg != nil {
						typeArgNodes = append(typeArgNodes, arg)
					}
				}
			}
		}
		if baseName != "" {
			// Build the full text and delegate to ParseTypeText.
			var argTexts []string
			for _, a := range typeArgNodes {
				argTexts = append(argTexts, a.Content(content))
			}
			if len(argTexts) > 0 {
				full := baseName + "<" + strings.Join(argTexts, ", ") + ">"
				return ParseTypeText(full, moduleQN)
			}
			return ParseTypeText(baseName, moduleQN)
		}
		return ParseTypeText(node.Content(content), moduleQN)

	case "union_type":
		var parts []string
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				parts = append(parts, child.Content(content))
			}
		}
		if len(parts) > 1 {
			return parseUnion(parts, moduleQN)
		}
		return ParseTypeText(node.Content(content), moduleQN)

	case "intersection_type":
		// Best-effort: return first member.
		if node.NamedChildCount() > 0 {
			return ParseTypeNode(node.NamedChild(0), content, moduleQN, imports)
		}
		return ParseTypeText(node.Content(content), moduleQN)

	case "array_type":
		if node.NamedChildCount() > 0 {
			elem := ParseTypeNode(node.NamedChild(0), content, moduleQN, imports)
			if elem != nil {
				return typresolve.Slice(elem)
			}
		}
		return ParseTypeText(node.Content(content), moduleQN)

	case "tuple_type":
		var elems []*typresolve.Type
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child != nil {
				elems = append(elems, ParseTypeNode(child, content, moduleQN, imports))
			}
		}
		return typresolve.Tuple(elems)

	case "parenthesized_type":
		if node.NamedChildCount() > 0 {
			return ParseTypeNode(node.NamedChild(0), content, moduleQN, imports)
		}
		return nil

	case "function_type":
		// Extract parameters and return type.
		return ParseTypeText(node.Content(content), moduleQN)

	default:
		// Fallback: extract full text and parse.
		return ParseTypeText(node.Content(content), moduleQN)
	}
}
