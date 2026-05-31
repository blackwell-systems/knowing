package rubyresolve

import (
	"github.com/blackwell-systems/knowing/internal/typresolve"
	sitter "github.com/smacker/go-tree-sitter"
)

// ProcessStatement processes a Ruby statement node to bind variables
// into the current scope. Handles assignment (=), operator assignment,
// multiple assignment, for-in, and rescue clauses with exception
// variable binding.
func ProcessStatement(ctx *ResolveContext, node *sitter.Node) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "assignment":
		processAssignment(ctx, node)
	case "operator_assignment":
		processOperatorAssignment(ctx, node)
	case "left_assignment_list", "right_assignment_list":
		processMultiAssignment(ctx, node)
	case "for":
		processForStatement(ctx, node)
	case "rescue":
		processRescue(ctx, node)
	case "block_parameters", "method_parameters":
		processParameters(ctx, node)
	}
}

// processAssignment handles simple assignment: x = expr, @name = expr, @@name = expr.
func processAssignment(ctx *ResolveContext, node *sitter.Node) {
	// Ruby tree-sitter assignment: NamedChild(0) = left, NamedChild(1) = right
	if node.NamedChildCount() < 2 {
		return
	}

	left := node.NamedChild(0)
	right := node.NamedChild(int(node.NamedChildCount()) - 1)

	rhsType := EvalExprType(ctx, right)
	if rhsType == nil {
		rhsType = typresolve.Unknown()
	}

	bindTarget(ctx, left, rhsType)
}

// processOperatorAssignment handles +=, -=, ||=, &&=, etc.
func processOperatorAssignment(ctx *ResolveContext, node *sitter.Node) {
	if node.NamedChildCount() < 2 {
		return
	}

	left := node.NamedChild(0)
	right := node.NamedChild(int(node.NamedChildCount()) - 1)

	leftName := nodeText(left, ctx.Content)

	// Get operator text from the non-named child.
	var op string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if !child.IsNamed() {
			op = nodeText(child, ctx.Content)
			break
		}
	}

	// For ||=, the type could be the existing type or the right side.
	if op == "||=" {
		existing := ctx.Scope.Lookup(leftName)
		if existing != nil && !existing.IsUnknown() {
			// Keep existing type (||= only assigns if nil/false).
			return
		}
		rhsType := EvalExprType(ctx, right)
		bindTarget(ctx, left, rhsType)
		return
	}

	// For numeric operators (+=, -=, *=, /=, %=, **=), keep existing type.
	existing := ctx.Scope.Lookup(leftName)
	if existing != nil && !existing.IsUnknown() {
		// Type doesn't change for numeric operator assignment.
		return
	}

	// No existing type; evaluate RHS.
	rhsType := EvalExprType(ctx, right)
	bindTarget(ctx, left, rhsType)
}

// processMultiAssignment handles multiple assignment: a, b, c = 1, "two", :three
func processMultiAssignment(ctx *ResolveContext, node *sitter.Node) {
	// The parent assignment node contains left_assignment_list and the right side.
	// We're called on the assignment node itself when it has multi-targets.
	// Actually, the brief says we handle left_assignment_list / right_assignment_list nodes.
	// In Ruby tree-sitter, multiple assignment is parsed as an "assignment" with
	// left_assignment_list on the left. Let's handle this from the parent.

	// Get parent (assignment node).
	parent := node.Parent()
	if parent == nil || parent.Type() != "assignment" {
		return
	}

	// Left is a left_assignment_list, right could be right_assignment_list or single expr.
	leftList := parent.NamedChild(0)
	if leftList == nil {
		return
	}

	// Find the right side (last named child of parent).
	rightNode := parent.NamedChild(int(parent.NamedChildCount()) - 1)
	if rightNode == nil {
		return
	}

	leftCount := int(leftList.NamedChildCount())

	if rightNode.Type() == "right_assignment_list" {
		// Pair up left and right children.
		rightCount := int(rightNode.NamedChildCount())
		for i := 0; i < leftCount; i++ {
			lChild := leftList.NamedChild(i)
			var rhsType *typresolve.Type
			if i < rightCount {
				rhsType = EvalExprType(ctx, rightNode.NamedChild(i))
			} else {
				rhsType = typresolve.Named("Ruby::NilClass")
			}
			bindTarget(ctx, lChild, rhsType)
		}
	} else {
		// Single expression on right side; if Array type, bind each to Unknown.
		rhsType := EvalExprType(ctx, rightNode)
		for i := 0; i < leftCount; i++ {
			lChild := leftList.NamedChild(i)
			if rhsType.Kind == typresolve.KindNamed && rhsType.Name == "Ruby::Array" {
				bindTarget(ctx, lChild, typresolve.Unknown())
			} else {
				bindTarget(ctx, lChild, typresolve.Unknown())
			}
		}
	}
}

// processForStatement handles for-loop variable binding.
func processForStatement(ctx *ResolveContext, node *sitter.Node) {
	pattern := node.ChildByFieldName("pattern")
	if pattern == nil {
		// Try first named child as pattern.
		if node.NamedChildCount() > 0 {
			pattern = node.NamedChild(0)
		}
	}

	if pattern != nil {
		// Bind loop variable to Unknown (element type not inferrable).
		bindTarget(ctx, pattern, typresolve.Unknown())
	}
}

// processRescue handles rescue clause exception variable binding.
// e.g., rescue TypeError => e
func processRescue(ctx *ResolveContext, node *sitter.Node) {
	// Look for the exception variable (after =>).
	varNode := node.ChildByFieldName("variable")

	if varNode == nil {
		// Walk children looking for an identifier after "=>" operator.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "exception_variable" {
				// The exception_variable node wraps the identifier.
				if child.NamedChildCount() > 0 {
					varNode = child.NamedChild(0)
				}
				break
			}
		}
	}

	if varNode == nil {
		return
	}

	// Try to get the exception class.
	exceptionsNode := node.ChildByFieldName("exceptions")
	if exceptionsNode == nil {
		// Look for the exceptions child.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(i)
			if child.Type() == "exceptions" || child.Type() == "constant" {
				exceptionsNode = child
				break
			}
		}
	}

	var exType *typresolve.Type
	if exceptionsNode != nil && exceptionsNode.Type() == "constant" {
		constName := nodeText(exceptionsNode, ctx.Content)
		resolved := ResolveConstant(constName, ctx.Nesting)
		exType = typresolve.Named(resolved)
	} else if exceptionsNode != nil && exceptionsNode.NamedChildCount() > 0 {
		// Multiple exception types; use the first one.
		first := exceptionsNode.NamedChild(0)
		if first.Type() == "constant" {
			constName := nodeText(first, ctx.Content)
			resolved := ResolveConstant(constName, ctx.Nesting)
			exType = typresolve.Named(resolved)
		}
	}

	if exType == nil {
		exType = typresolve.Named("Ruby::StandardError")
	}

	varName := nodeText(varNode, ctx.Content)
	ctx.Scope.Bind(varName, exType)
}

// processParameters handles block_parameters and method_parameters by
// binding each parameter name to Unknown.
func processParameters(ctx *ResolveContext, node *sitter.Node) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child.Type() == "identifier" {
			name := nodeText(child, ctx.Content)
			ctx.Scope.Bind(name, typresolve.Unknown())
		}
	}
}

// bindTarget binds a variable target node to a type in the current scope.
func bindTarget(ctx *ResolveContext, target *sitter.Node, typ *typresolve.Type) {
	if target == nil {
		return
	}

	switch target.Type() {
	case "identifier":
		name := nodeText(target, ctx.Content)
		if name != "_" {
			ctx.Scope.Bind(name, typ)
		}
	case "instance_variable":
		name := nodeText(target, ctx.Content)
		ctx.Scope.Bind(name, typ)
	case "class_variable":
		name := nodeText(target, ctx.Content)
		ctx.Scope.Bind(name, typ)
	}
}
