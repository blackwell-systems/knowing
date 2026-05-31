package pyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a Python statement node to bind variables
// into the current scope. Handles assignment, for_statement, with_statement,
// and try_statement (except clause binding). This is the Python port of
// py_process_statement from the C reference implementation.
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "assignment":
		processAssignment(ctx, node)
	case "for_statement":
		processForStatement(ctx, node)
	case "with_statement":
		processWithStatement(ctx, node)
	case "try_statement":
		processTryStatement(ctx, node)
	}
}

// processAssignment handles simple assignment (x = expr) and annotated
// assignment (x: T = expr).
func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	typeAnn := node.ChildByFieldName("type")

	var rhsType *typresolve.Type

	// If has annotation, annotation wins over RHS type.
	if typeAnn != nil {
		annText := nodeText(typeAnn, ctx.Content)
		rhsType = ParseAnnotation(annText, ctx.ModuleQN)
	}

	// If no annotation or annotation returned Unknown, try RHS.
	if rhsType == nil || rhsType.Kind == typresolve.KindUnknown {
		if right != nil {
			rhsType = EvalExprType(ctx, right)
		}
	}

	if rhsType == nil {
		rhsType = typresolve.Unknown()
	}

	if left == nil {
		return
	}

	switch left.Type() {
	case "identifier":
		name := nodeText(left, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, rhsType)
		}

	case "attribute":
		// self.x = expr or cls.x = expr: skip instance field registration
		// for now (requires mutable registry; defer to wave 2).

	case "pattern_list", "tuple_pattern", "expression_list":
		// Tuple unpacking.
		bindTupleTargets(ctx, left, rhsType)
	}
}

// processForStatement handles for-loop variable binding.
// e.g. for x in [1, 2]: ...
func processForStatement(ctx *ResolveContext, node *sitter.Node) {
	left := node.ChildByFieldName("left")
	right := node.ChildByFieldName("right")
	if left == nil || right == nil {
		return
	}

	// Evaluate RHS (iterable) type.
	iterType := EvalExprType(ctx, right)

	// Get element type from iterable.
	elemType := IterableElementType(iterType)

	// Bind LHS target.
	switch left.Type() {
	case "identifier":
		name := nodeText(left, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, elemType)
		}
	case "pattern_list", "tuple_pattern":
		// Destructure tuple element types.
		bindTupleTargets(ctx, left, elemType)
	}
}

// processWithStatement handles with-statement variable binding.
// e.g. with open("f") as fh: ...
func processWithStatement(ctx *ResolveContext, node *sitter.Node) {
	// Walk children looking for with_clause or with_item nodes.
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}

		switch child.Type() {
		case "with_clause":
			// Walk with_clause children for with_item nodes.
			for j := 0; j < int(child.NamedChildCount()); j++ {
				item := child.NamedChild(int(j))
				if item != nil && item.Type() == "with_item" {
					processWithItem(ctx, item)
				}
			}
		case "with_item":
			processWithItem(ctx, child)
		case "as_pattern":
			processAsPattern(ctx, child)
		}
	}
}

// processWithItem handles a single with_item: value [as alias].
func processWithItem(ctx *ResolveContext, node *sitter.Node) {
	valueNode := node.ChildByFieldName("value")
	if valueNode == nil {
		// Try first named child as value.
		if node.NamedChildCount() > 0 {
			valueNode = node.NamedChild(0)
		}
	}

	if valueNode == nil {
		return
	}

	// Check for as_pattern wrapping.
	if valueNode.Type() == "as_pattern" {
		processAsPattern(ctx, valueNode)
		return
	}

	// Evaluate value type.
	valType := EvalExprType(ctx, valueNode)

	// Look up __enter__ method on the type to get the bound type.
	boundType := resolveEnterType(ctx, valType)

	// Find alias.
	aliasNode := node.ChildByFieldName("alias")
	if aliasNode == nil {
		// Check for second named child as alias.
		if node.NamedChildCount() > 1 {
			aliasNode = node.NamedChild(int(node.NamedChildCount()) - 1)
		}
	}

	if aliasNode != nil && aliasNode.Type() == "identifier" {
		name := nodeText(aliasNode, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, boundType)
		}
	}
}

// processAsPattern handles as_pattern nodes in with statements.
func processAsPattern(ctx *ResolveContext, node *sitter.Node) {
	// as_pattern has the value expression and an alias.
	var valueNode, aliasNode *sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		if child.Type() == "as_pattern_target" || (aliasNode == nil && i == int(node.NamedChildCount())-1 && child.Type() == "identifier") {
			aliasNode = child
		} else if valueNode == nil {
			valueNode = child
		}
	}

	if valueNode == nil {
		return
	}

	valType := EvalExprType(ctx, valueNode)
	boundType := resolveEnterType(ctx, valType)

	if aliasNode != nil {
		// as_pattern_target may wrap an identifier.
		target := aliasNode
		if target.Type() == "as_pattern_target" && target.NamedChildCount() > 0 {
			target = target.NamedChild(0)
		}
		if target.Type() == "identifier" {
			name := nodeText(target, ctx.Content)
			if name != "_" {
				ctx.Scope.Bind(name, boundType)
			}
		}
	}
}

// resolveEnterType attempts to find the __enter__ method on the given type
// and return its return type. Falls back to the value type itself.
func resolveEnterType(ctx *ResolveContext, valType *typresolve.Type) *typresolve.Type {
	if valType == nil || valType.Kind != typresolve.KindNamed {
		return valType
	}

	// Look up __enter__ method.
	if m := LookupAttribute(ctx.Registry, valType.Name, "__enter__"); m != nil {
		return extractFuncReturnType(m)
	}

	// Fallback: return value type.
	return valType
}

// processTryStatement handles try/except clause variable binding.
// e.g. except ValueError as e: ...
func processTryStatement(ctx *ResolveContext, node *sitter.Node) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil || child.Type() != "except_clause" {
			continue
		}
		processExceptClause(ctx, child)
	}
}

// processExceptClause handles a single except clause.
// e.g. except ValueError as e: bind e to Named("ValueError").
func processExceptClause(ctx *ResolveContext, node *sitter.Node) {
	// Find the exception type and alias.
	// In tree-sitter-python: except_clause has unnamed children.
	// Pattern: "except" type "as" name ":"
	var typeNode, nameNode *sitter.Node

	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		switch child.Type() {
		case "identifier":
			if typeNode == nil {
				typeNode = child
			} else {
				nameNode = child
			}
		case "attribute":
			typeNode = child
		case "as_pattern":
			// Handle "except E as e" parsed as as_pattern.
			processExceptAsPattern(ctx, child)
			return
		case "block":
			// Skip the body block.
		default:
			if typeNode == nil {
				typeNode = child
			}
		}
	}

	if typeNode != nil && nameNode != nil {
		typeName := nodeText(typeNode, ctx.Content)
		excType := ParseAnnotation(typeName, ctx.ModuleQN)
		name := nodeText(nameNode, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, excType)
		}
	}
}

// processExceptAsPattern handles the case where tree-sitter parses
// "except E as e" with an as_pattern node.
func processExceptAsPattern(ctx *ResolveContext, node *sitter.Node) {
	var typeNode, nameNode *sitter.Node
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(int(i))
		if child == nil {
			continue
		}
		if child.Type() == "as_pattern_target" {
			nameNode = child
		} else if typeNode == nil {
			typeNode = child
		}
	}

	if typeNode == nil || nameNode == nil {
		return
	}

	typeName := nodeText(typeNode, ctx.Content)
	excType := ParseAnnotation(typeName, ctx.ModuleQN)

	// as_pattern_target may wrap an identifier.
	target := nameNode
	if target.Type() == "as_pattern_target" && target.NamedChildCount() > 0 {
		target = target.NamedChild(0)
	}
	if target.Type() == "identifier" {
		name := nodeText(target, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, excType)
		}
	}
}

// bindTupleTargets binds tuple unpacking targets to their respective types.
// If RHS is a Tuple, bind each LHS variable to the corresponding element.
// Otherwise bind all to Unknown.
func bindTupleTargets(ctx *ResolveContext, left *sitter.Node, rhsType *typresolve.Type) {
	count := int(left.NamedChildCount())
	for i := 0; i < count; i++ {
		child := left.NamedChild(int(i))
		if child == nil || child.Type() != "identifier" {
			continue
		}
		name := nodeText(child, ctx.Content)
		if name == "_" {
			continue
		}

		if rhsType != nil && rhsType.Kind == typresolve.KindTuple && i < len(rhsType.Elements) {
			ctx.Scope.Bind(name, rhsType.Elements[i])
		} else {
			ctx.Scope.Bind(name, typresolve.Unknown())
		}
	}
}
