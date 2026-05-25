package treesitter

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/types"
)

// extractProcessExecEdges walks a Python function/method body and creates
// executes_process edges for each subprocess/os.system call it detects.
//
// Patterns matched:
//   - subprocess.run(["cmd", ...]) or subprocess.run("cmd")
//   - subprocess.Popen(["cmd", ...]) or subprocess.Popen("cmd")
//   - subprocess.call(["cmd", ...]) or subprocess.call("cmd")
//   - os.system("cmd")
//
// For list arguments, the first element is extracted as the command.
// For non-literal arguments, "process://dynamic" is used.
func extractProcessExecEdges(funcNode *sitter.Node, opts types.ExtractOptions, classContext string, funcHash types.Hash) ([]types.Node, []types.Edge) {
	body := funcNode.ChildByFieldName("body")
	if body == nil {
		return nil, nil
	}

	// Collect unique process commands.
	commands := make(map[string]bool)
	walkForProcessExec(body, opts.Content, commands)

	if len(commands) == 0 {
		return nil, nil
	}

	var nodes []types.Node
	var edges []types.Edge
	provenance := "ast_inferred"

	for cmd := range commands {
		qn := "process://" + cmd
		targetHash := types.ComputeNodeHash(opts.RepoURL, opts.ModuleRoot, types.EmptyHash, qn, types.KindProcess)

		nodes = append(nodes, types.Node{
			NodeHash:      targetHash,
			FileHash:      opts.FileHash,
			QualifiedName: qn,
			Kind:          types.KindProcess,
		})

		edgeHash := types.ComputeEdgeHash(funcHash, targetHash, edgetype.ExecutesProcess, provenance)
		edges = append(edges, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: funcHash,
			TargetHash: targetHash,
			EdgeType:   edgetype.ExecutesProcess,
			Confidence: 0.9,
			Provenance: provenance,
		})
	}

	return nodes, edges
}

// walkForProcessExec recursively walks a Python AST subtree collecting process
// command names from subprocess.run/Popen/call and os.system calls.
func walkForProcessExec(node *sitter.Node, content []byte, commands map[string]bool) {
	if node == nil {
		return
	}

	if node.Type() == "call" {
		if cmd := extractProcessCommand(node, content); cmd != "" {
			commands[cmd] = true
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForProcessExec(node.Child(i), content, commands)
	}
}

// extractProcessCommand checks if a call node is a subprocess or os.system
// invocation and returns the command name, or "" if it doesn't match.
func extractProcessCommand(callNode *sitter.Node, content []byte) string {
	funcNode := callNode.ChildByFieldName("function")
	if funcNode == nil || funcNode.Type() != "attribute" {
		return ""
	}

	objectNode := funcNode.ChildByFieldName("object")
	attrNode := funcNode.ChildByFieldName("attribute")
	if objectNode == nil || attrNode == nil {
		return ""
	}

	objName := ""
	if objectNode.Type() == "identifier" {
		objName = objectNode.Content(content)
	}
	attrName := attrNode.Content(content)

	// Pattern 1: os.system("cmd")
	if objName == "os" && attrName == "system" {
		return extractCommandFromArgs(callNode, content)
	}

	// Pattern 2: subprocess.run/Popen/call(...)
	if objName == "subprocess" {
		switch attrName {
		case "run", "Popen", "call":
			return extractCommandFromArgs(callNode, content)
		}
	}

	return ""
}

// extractCommandFromArgs extracts the command name from call arguments.
// Handles both string arguments ("cmd") and list arguments (["cmd", ...]).
// Returns "dynamic" for non-literal arguments.
func extractCommandFromArgs(callNode *sitter.Node, content []byte) string {
	args := callNode.ChildByFieldName("arguments")
	if args == nil {
		return "dynamic"
	}

	// Find the first non-punctuation argument.
	for i := 0; i < int(args.ChildCount()); i++ {
		child := args.Child(i)
		if child == nil {
			continue
		}
		// Skip punctuation.
		if child.Type() == "(" || child.Type() == ")" || child.Type() == "," {
			continue
		}

		switch child.Type() {
		case "string":
			// Direct string argument: os.system("cmd string") or subprocess.run("cmd")
			s := extractStringLiteral(child, content)
			if s == "" {
				return "dynamic"
			}
			// For os.system, the whole string is the command. Extract just the first word.
			return firstWord(s)

		case "list":
			// List argument: subprocess.run(["cmd", "arg1", ...])
			return extractFirstListElement(child, content)

		default:
			// Variable or expression: dynamic.
			return "dynamic"
		}
	}

	return "dynamic"
}

// extractFirstListElement extracts the first string element from a list node.
// Returns "dynamic" if the first element is not a string literal.
func extractFirstListElement(listNode *sitter.Node, content []byte) string {
	for i := 0; i < int(listNode.ChildCount()); i++ {
		child := listNode.Child(i)
		if child == nil {
			continue
		}
		// Skip brackets and commas.
		if child.Type() == "[" || child.Type() == "]" || child.Type() == "," {
			continue
		}
		if child.Type() == "string" {
			s := extractStringLiteral(child, content)
			if s == "" {
				return "dynamic"
			}
			return s
		}
		// First element is not a string literal.
		return "dynamic"
	}
	return "dynamic"
}

// firstWord returns the first space-separated word from a string.
func firstWord(s string) string {
	for i, c := range s {
		if c == ' ' || c == '\t' {
			return s[:i]
		}
	}
	return s
}
