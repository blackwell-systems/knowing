package gotsextractor

import (
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ExtractTestsEdges analyzes a test file's function bodies and produces
// 'tests' edges from each Test*/Benchmark* function to production functions it calls.
// Parameters:
//
//	root: tree-sitter root node of the file
//	opts: standard ExtractOptions for the test file
//	pkgPath: resolved Go package path
//	imports: import alias map from buildImportMap
//
// Returns: slice of Edge with EdgeType="tests", Provenance="ast_inferred", Confidence=0.7
func ExtractTestsEdges(root *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) []types.Edge {
	// Only process test files.
	if !strings.HasSuffix(opts.FilePath, "_test.go") {
		return nil
	}

	var edges []types.Edge
	seen := make(map[types.Hash]struct{})

	// Walk top-level declarations looking for test/benchmark functions.
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "function_declaration" {
			continue
		}

		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		funcName := nameNode.Content(opts.Content)

		// Only process Test* and Benchmark* functions.
		if !strings.HasPrefix(funcName, "Test") && !strings.HasPrefix(funcName, "Benchmark") {
			continue
		}

		// Compute the test function's node hash (same as extractFuncDecl).
		testNodeHash := types.ComputeNodeHash(opts.RepoURL, pkgPath, types.EmptyHash, funcName, "function")

		// Walk the function body for call expressions.
		body := child.ChildByFieldName("body")
		if body == nil {
			continue
		}

		walkForTestTargets(body, opts, pkgPath, testNodeHash, imports, &edges, seen)
	}

	return edges
}

// walkForTestTargets recursively walks nodes in a test function body looking
// for call_expression nodes targeting production (non-test) functions.
func walkForTestTargets(node *sitter.Node, opts types.ExtractOptions, pkgPath string, testNodeHash types.Hash, imports map[string]string, edges *[]types.Edge, seen map[types.Hash]struct{}) {
	if node == nil {
		return
	}

	if node.Type() == "call_expression" {
		funcNode := node.ChildByFieldName("function")
		if funcNode != nil {
			targetName, targetPkg, targetKind := resolveTestCallTarget(funcNode, opts, pkgPath, imports)
			if targetName != "" && isProductionTarget(targetName) {
				targetRepoURL := inferRepoURL(opts, targetPkg, pkgPath)
				targetHash := types.ComputeNodeHash(targetRepoURL, targetPkg, types.EmptyHash, targetName, targetKind)

				provenance := "ast_inferred"
				edgeHash := types.ComputeEdgeHash(testNodeHash, targetHash, "tests", provenance)

				// Deduplicate by EdgeHash.
				if _, exists := seen[edgeHash]; !exists {
					seen[edgeHash] = struct{}{}
					*edges = append(*edges, types.Edge{
						EdgeHash:   edgeHash,
						SourceHash: testNodeHash,
						TargetHash: targetHash,
						EdgeType:   "tests",
						Confidence: 0.7,
						Provenance: provenance,
					})
				}
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		walkForTestTargets(node.Child(i), opts, pkgPath, testNodeHash, imports, edges, seen)
	}
}

// resolveTestCallTarget resolves a call expression's function node to a target
// name, package, and kind. This mirrors the logic in resolveCallEdge but returns
// the components rather than constructing an edge.
func resolveTestCallTarget(funcNode *sitter.Node, opts types.ExtractOptions, pkgPath string, imports map[string]string) (name, pkg, kind string) {
	content := opts.Content

	switch funcNode.Type() {
	case "identifier":
		// Local function call.
		return funcNode.Content(content), pkgPath, "function"

	case "selector_expression":
		// Qualified call: pkg.Func or receiver.Method
		operandNode := funcNode.ChildByFieldName("operand")
		fieldNode := funcNode.ChildByFieldName("field")
		if operandNode == nil || fieldNode == nil {
			return "", "", ""
		}
		targetName := fieldNode.Content(content)

		switch operandNode.Type() {
		case "identifier":
			operandName := operandNode.Content(content)
			if importPath, ok := imports[operandName]; ok {
				// Package-qualified call (cross-package).
				return targetName, importPath, "function"
			}
			// Method call on a local variable.
			return targetName, pkgPath, "method"

		case "selector_expression":
			// Chained selector: obj.field.Method()
			return targetName, pkgPath, "method"

		default:
			return "", "", ""
		}

	default:
		return "", "", ""
	}
}

// isProductionTarget returns true if the call target name does NOT indicate
// a test helper function. A target is considered "production" if its name
// does not start with "Test" or "Benchmark".
func isProductionTarget(name string) bool {
	if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") {
		return false
	}
	return true
}
