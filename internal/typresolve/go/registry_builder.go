package goresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// fileContents and fileTrees provide optional parsed AST data for richer
// type extraction (Tier 2). When nil, only Tier 1 resolution is available:
// the registry knows which functions/types exist and their qualified names,
// but has no struct fields, embeddings, or full signatures.
//
// Gap 5: Parses Signature field from ResolverDef into full function types.
// Gap 7: Extracts struct field types from AST when fileTrees are available.
// Gap 8: Detects type aliases and sets AliasOf on RegisteredType.
// Gap 9: Extracts interface method names and sets MethodNames/IsInterface.
func BuildRegistry(defs []typresolve.ResolverDef, fileContents map[string][]byte, fileTrees map[string]*sitter.Node) *typresolve.Registry {
	reg := typresolve.NewRegistry()

	// Reset the package-level type QN list for interface satisfaction.
	registeredTypeQNs = make([]string, 0, len(defs))

	for _, def := range defs {
		switch def.Kind {
		case "function":
			rf := typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				MinParams:     -1,
			}
			// Gap 5: Parse signature text into a proper Type.
			if def.Signature != "" {
				rf.Signature = parseGoSignatureText(def.Signature, packageFromQN(def.QualifiedName))
			}
			reg.AddFunc(rf)

		case "method":
			receiver := extractReceiverType(def.QualifiedName)
			rf := typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				ReceiverType:  receiver,
				MinParams:     -1,
			}
			// Gap 5: Parse signature text into a proper Type.
			if def.Signature != "" {
				rf.Signature = parseGoSignatureText(def.Signature, packageFromQN(def.QualifiedName))
			}
			reg.AddFunc(rf)

		case "type":
			rt := typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				IsInterface:   false,
			}
			// Gap 8: Detect type aliases from signature field.
			// Format: "= TargetType" indicates an alias.
			if strings.HasPrefix(def.Signature, "= ") {
				target := strings.TrimPrefix(def.Signature, "= ")
				target = strings.TrimSpace(target)
				if target != "" {
					pkg := packageFromQN(def.QualifiedName)
					if !isBuiltinType(target) && !strings.Contains(target, ".") {
						rt.AliasOf = pkg + "." + target
					} else {
						rt.AliasOf = target
					}
				}
			}
			reg.AddType(rt)
			registeredTypeQNs = append(registeredTypeQNs, def.QualifiedName)

		case "interface":
			rt := typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				IsInterface:   true,
			}
			// Gap 9: Extract interface method names from signature.
			// Format: "interface{MethodA;MethodB;MethodC}" or pipe-separated "MethodA|MethodB"
			if def.Signature != "" {
				rt.MethodNames = parseInterfaceMethodNames(def.Signature)
			}
			reg.AddType(rt)
			registeredTypeQNs = append(registeredTypeQNs, def.QualifiedName)
		}
	}

	// Gap 7: If AST data is available, scan for struct field types,
	// embedded types, interface methods, and type aliases.
	if fileTrees != nil && fileContents != nil {
		enrichRegistryFromAST(reg, fileContents, fileTrees)
	}

	return reg
}

// parseGoSignatureText parses a Go function signature string into a
// typresolve.Type with KindFunc. Handles common signature formats:
//   - "func(context.Context) (*Server, error)"
//   - "func() string"
//   - "func(int, string) (bool, error)"
//
// Gap 5: This is the single most impactful fix. Without it, all call
// chains break because the registry has no return type information.
func parseGoSignatureText(sig string, pkgQN string) *typresolve.Type {
	sig = strings.TrimSpace(sig)

	// Strip leading "func" keyword.
	if strings.HasPrefix(sig, "func") {
		sig = sig[4:]
	}

	// Find the parameter list.
	params, rest := splitParenthesized(sig)
	_ = params // We don't parse params into Param structs yet (would need names).

	// Parse return types from `rest`.
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return typresolve.Func(nil, nil)
	}

	var returns []*typresolve.Type

	// Multi-return in parentheses: (Type1, Type2, ...)
	if strings.HasPrefix(rest, "(") {
		retText, _ := splitParenthesized(rest)
		retParts := splitTopLevelComma(retText)
		for _, part := range retParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			returns = append(returns, parseTypeText(part, pkgQN))
		}
	} else {
		// Single return type.
		returns = append(returns, parseTypeText(rest, pkgQN))
	}

	return typresolve.Func(nil, returns)
}

// parseTypeText parses a type text string into a typresolve.Type.
// Handles: *T, []T, map[K]V, chan T, builtins, qualified types.
func parseTypeText(text string, pkgQN string) *typresolve.Type {
	text = strings.TrimSpace(text)
	if text == "" {
		return typresolve.Unknown()
	}

	// Pointer: *T
	if strings.HasPrefix(text, "*") {
		inner := parseTypeText(text[1:], pkgQN)
		return typresolve.Pointer(inner)
	}

	// Slice: []T
	if strings.HasPrefix(text, "[]") {
		inner := parseTypeText(text[2:], pkgQN)
		return typresolve.Slice(inner)
	}

	// Map: map[K]V
	if strings.HasPrefix(text, "map[") {
		// Find matching bracket for key.
		depth := 0
		keyEnd := -1
		for i := 3; i < len(text); i++ {
			if text[i] == '[' {
				depth++
			} else if text[i] == ']' {
				depth--
				if depth == 0 {
					keyEnd = i
					break
				}
			}
		}
		if keyEnd > 4 {
			keyText := text[4:keyEnd]
			valText := text[keyEnd+1:]
			return typresolve.Map(parseTypeText(keyText, pkgQN), parseTypeText(valText, pkgQN))
		}
		return typresolve.Unknown()
	}

	// Channel: chan T, <-chan T, chan<- T
	if strings.HasPrefix(text, "<-chan ") {
		inner := parseTypeText(text[7:], pkgQN)
		return typresolve.Channel(inner, typresolve.ChanRecv)
	}
	if strings.HasPrefix(text, "chan<- ") {
		inner := parseTypeText(text[6:], pkgQN)
		return typresolve.Channel(inner, typresolve.ChanSend)
	}
	if strings.HasPrefix(text, "chan ") {
		inner := parseTypeText(text[5:], pkgQN)
		return typresolve.Channel(inner, typresolve.ChanBidi)
	}

	// Function type: func(...)...
	if strings.HasPrefix(text, "func") {
		return parseGoSignatureText(text, pkgQN)
	}

	// Interface: interface{} or interface{...}
	if text == "interface{}" || strings.HasPrefix(text, "interface{") {
		return &typresolve.Type{Kind: typresolve.KindInterface}
	}

	// Struct: struct{...}
	if text == "struct{}" || strings.HasPrefix(text, "struct{") {
		return &typresolve.Type{Kind: typresolve.KindStruct}
	}

	// Builtin type.
	if isBuiltinType(text) {
		return typresolve.Builtin(text)
	}

	// Qualified type: pkg.Type
	if strings.Contains(text, ".") {
		return typresolve.Named(text)
	}

	// Unqualified named type: qualify with current package.
	if pkgQN != "" {
		return typresolve.Named(pkgQN + "." + text)
	}
	return typresolve.Named(text)
}

// splitParenthesized extracts the content of the first balanced
// parenthesized group and returns (content, rest).
// Input: "(a, b) c" -> ("a, b", " c")
func splitParenthesized(s string) (string, string) {
	if len(s) == 0 || s[0] != '(' {
		return "", s
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return s[1:i], s[i+1:]
			}
		}
	}
	// Unbalanced; return best effort.
	return s[1:], ""
}

// splitTopLevelComma splits a string by commas, respecting nested
// brackets and parentheses.
func splitTopLevelComma(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseInterfaceMethodNames extracts method names from an interface
// signature. Supports formats:
//   - "interface{MethodA;MethodB;MethodC}"
//   - "MethodA|MethodB|MethodC" (pipe-separated)
//   - "MethodA(args) returns; MethodB(args) returns" (full signatures)
func parseInterfaceMethodNames(sig string) []string {
	sig = strings.TrimSpace(sig)

	// Strip "interface{" prefix and "}" suffix.
	if strings.HasPrefix(sig, "interface{") {
		sig = strings.TrimPrefix(sig, "interface{")
		sig = strings.TrimSuffix(sig, "}")
	}

	if sig == "" {
		return nil
	}

	// Try pipe-separated first.
	if strings.Contains(sig, "|") {
		parts := strings.Split(sig, "|")
		var names []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			// Extract just the method name (before any parenthesis).
			if idx := strings.Index(p, "("); idx > 0 {
				p = p[:idx]
			}
			p = strings.TrimSpace(p)
			if p != "" {
				names = append(names, p)
			}
		}
		return names
	}

	// Try semicolon-separated.
	parts := strings.Split(sig, ";")
	var names []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Extract just the method name (before any parenthesis).
		if idx := strings.Index(p, "("); idx > 0 {
			p = p[:idx]
		}
		p = strings.TrimSpace(p)
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// enrichRegistryFromAST scans parsed ASTs for struct definitions,
// type aliases, and interface method names, enriching the registry
// with field types, embedded types, alias targets, and method names.
//
// Gap 7: Struct field type registration from AST.
// Gap 8: Type alias detection from AST.
// Gap 9: Interface method names from AST.
func enrichRegistryFromAST(reg *typresolve.Registry, fileContents map[string][]byte, fileTrees map[string]*sitter.Node) {
	for filePath, root := range fileTrees {
		content := fileContents[filePath]
		if content == nil || root == nil {
			continue
		}

		// Determine package QN from file path.
		pkgQN := inferPkgQNFromPath(filePath)
		imports := BuildImportMap(root, content)

		// Walk top-level type declarations.
		for i := 0; i < int(root.NamedChildCount()); i++ {
			child := root.NamedChild(i)
			if child == nil || child.Type() != "type_declaration" {
				continue
			}

			for j := 0; j < int(child.NamedChildCount()); j++ {
				spec := child.NamedChild(j)
				if spec == nil {
					continue
				}

				switch spec.Type() {
				case "type_spec":
					enrichTypeSpec(reg, spec, content, pkgQN, imports)
				case "type_alias":
					enrichTypeAlias(reg, spec, content, pkgQN)
				}
			}
		}
	}
}

// enrichTypeSpec processes a type_spec AST node, extracting struct
// fields, embedded types, and interface methods.
func enrichTypeSpec(reg *typresolve.Registry, spec *sitter.Node, content []byte, pkgQN string, imports map[string]string) {
	nameNode := spec.ChildByFieldName("name")
	typeNode := spec.ChildByFieldName("type")
	if nameNode == nil || typeNode == nil {
		return
	}

	typeName := nameNode.Content(content)
	typeQN := pkgQN + "." + typeName

	switch typeNode.Type() {
	case "struct_type":
		enrichStructType(reg, typeQN, typeNode, content, pkgQN, imports)
	case "interface_type":
		enrichInterfaceType(reg, typeQN, typeNode, content)
	}
}

// enrichStructType extracts fields and embedded types from a struct
// type AST node and updates the registered type.
func enrichStructType(reg *typresolve.Registry, typeQN string, typeNode *sitter.Node, content []byte, pkgQN string, imports map[string]string) {
	rt := reg.LookupType(typeQN)
	if rt == nil {
		return
	}

	// Find field_declaration_list (the body of the struct).
	var fieldList *sitter.Node
	for i := 0; i < int(typeNode.NamedChildCount()); i++ {
		child := typeNode.NamedChild(i)
		if child != nil && child.Type() == "field_declaration_list" {
			fieldList = child
			break
		}
	}
	if fieldList == nil {
		return
	}

	var fields []typresolve.Field
	var embeddedTypes []string

	for i := 0; i < int(fieldList.NamedChildCount()); i++ {
		fieldDecl := fieldList.NamedChild(i)
		if fieldDecl == nil || fieldDecl.Type() != "field_declaration" {
			continue
		}

		nameN := fieldDecl.ChildByFieldName("name")
		typeN := fieldDecl.ChildByFieldName("type")

		if nameN == nil && typeN != nil {
			// Embedded field: no name, just type.
			embedText := extractTypeName(typeN, content)
			if embedText != "" {
				if !strings.Contains(embedText, ".") && !isBuiltinType(embedText) {
					embedText = pkgQN + "." + embedText
				}
				embeddedTypes = append(embeddedTypes, embedText)
			}
		} else if nameN != nil && typeN != nil {
			// Named field.
			fieldName := nameN.Content(content)
			fieldType := ParseTypeNode(typeN, content, pkgQN, imports)
			if fieldName != "" && fieldType != nil {
				fields = append(fields, typresolve.Field{Name: fieldName, Type: fieldType})
			}
		}
	}

	if len(fields) > 0 {
		rt.Fields = fields
	}
	if len(embeddedTypes) > 0 {
		rt.EmbeddedTypes = embeddedTypes
	}
}

// enrichInterfaceType extracts method names from an interface type
// AST node and updates the registered type.
func enrichInterfaceType(reg *typresolve.Registry, typeQN string, typeNode *sitter.Node, content []byte) {
	rt := reg.LookupType(typeQN)
	if rt == nil {
		return
	}

	var methodNames []string
	for i := 0; i < int(typeNode.NamedChildCount()); i++ {
		child := typeNode.NamedChild(i)
		if child == nil {
			continue
		}
		// method_spec or method_elem nodes contain the method name.
		if child.Type() == "method_spec" || child.Type() == "method_elem" {
			mNameNode := child.ChildByFieldName("name")
			if mNameNode != nil {
				mName := mNameNode.Content(content)
				if mName != "" {
					methodNames = append(methodNames, mName)
				}
			}
		}
	}

	if len(methodNames) > 0 {
		rt.MethodNames = methodNames
		rt.IsInterface = true
	}
}

// enrichTypeAlias processes a type_alias AST node and sets the AliasOf
// field on the registered type.
func enrichTypeAlias(reg *typresolve.Registry, spec *sitter.Node, content []byte, pkgQN string) {
	nameNode := spec.ChildByFieldName("name")
	typeNode := spec.ChildByFieldName("type")
	if nameNode == nil || typeNode == nil {
		return
	}

	aliasName := nameNode.Content(content)
	aliasQN := pkgQN + "." + aliasName

	rt := reg.LookupType(aliasQN)
	if rt == nil {
		// Auto-register the alias type.
		reg.AddType(typresolve.RegisteredType{
			QualifiedName: aliasQN,
			ShortName:     aliasName,
		})
		rt = reg.LookupType(aliasQN)
		if rt == nil {
			return
		}
	}

	// Extract target type name.
	targetName := extractTypeName(typeNode, content)
	if targetName != "" {
		if !strings.Contains(targetName, ".") && !isBuiltinType(targetName) {
			rt.AliasOf = pkgQN + "." + targetName
		} else {
			rt.AliasOf = targetName
		}
	}
}

// shortName extracts the last segment after "." from a qualified name.
// For "github.com/org/repo://pkg.Func", returns "Func".
// For "pkg.ReceiverType.MethodName", returns "MethodName".
func shortName(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

// extractReceiverType extracts the receiver type QN from a method's
// qualified name. Format: "repoURL://pkg.ReceiverType.MethodName"
// Returns "repoURL://pkg.ReceiverType".
func extractReceiverType(methodQN string) string {
	if idx := strings.LastIndex(methodQN, "."); idx >= 0 {
		return methodQN[:idx]
	}
	return ""
}

// packageFromQN extracts the package path from a qualified name.
// "pkg.Func" -> "pkg", "pkg.Type.Method" -> "pkg"
// Uses the same logic as splitQualifiedName but only returns the package.
func packageFromQN(qn string) string {
	// Find the first dot followed by an uppercase letter (symbol boundary).
	// For simple cases like "fmt.Println", the package is "fmt".
	// For "net/http.Handler.ServeHTTP", the package is "net/http".
	lastDot := -1
	secondLastDot := -1
	for i := len(qn) - 1; i >= 0; i-- {
		if qn[i] == '.' {
			if lastDot == -1 {
				lastDot = i
			} else if secondLastDot == -1 {
				secondLastDot = i
				break
			}
		}
	}

	if secondLastDot >= 0 {
		return qn[:secondLastDot]
	}
	if lastDot >= 0 {
		return qn[:lastDot]
	}
	return qn
}
