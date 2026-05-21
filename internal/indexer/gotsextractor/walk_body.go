package gotsextractor

import (
	sitter "github.com/smacker/go-tree-sitter"

	"github.com/blackwell-systems/knowing/internal/types"
)

// bodyWalkResult accumulates results from a single-pass walk over a function body.
type bodyWalkResult struct {
	Edges []types.Edge
	Nodes []types.Node
}

// walkBodyOnce performs a single recursive walk over a function/method body,
// dispatching to all pattern detectors at each node. This replaces 5 separate
// recursive walks (calls, throws, routes, feature flags, endpoints) with one.
//
// Performance impact: for a function with N AST nodes, this visits N nodes once
// instead of 5*N. On a 26K-line file with thousands of call expressions, this
// is a 5x reduction in tree-sitter node visits.
func walkBodyOnce(body *sitter.Node, opts types.ExtractOptions, pkgPath string, funcNodeHash types.Hash, imports map[string]string, extNodes map[types.Hash]types.Node) bodyWalkResult {
	if body == nil {
		return bodyWalkResult{}
	}

	var result bodyWalkResult
	flagSeen := make(map[types.Hash]struct{})
	epSeen := make(map[types.Hash]struct{})

	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}

		nodeType := node.Type()

		switch nodeType {
		case "call_expression":
			// 1. Call edges (the primary extractor)
			funcNode := node.ChildByFieldName("function")
			if funcNode != nil {
				edge := resolveCallEdge(funcNode, opts, pkgPath, funcNodeHash, imports, extNodes)
				if edge != nil {
					posNode := callSitePositionNode(funcNode)
					edge.CallSiteLine = int(posNode.StartPoint().Row) + 1
					edge.CallSiteCol = int(posNode.StartPoint().Column)
					edge.CallSiteFile = opts.FilePath
					result.Edges = append(result.Edges, *edge)
				}

				// 2. Route symbol detection (checks if call is a route registration)
				fnText := funcNode.Content(opts.Content)
				if route := tryExtractRoute(funcNode, node, opts, pkgPath, imports); route != nil {
					rn, re := routeSymbolsToNodesAndEdges([]routeSymbol{*route}, opts, pkgPath)
					result.Nodes = append(result.Nodes, rn...)
					result.Edges = append(result.Edges, re...)
				}

				// 3. Feature flag detection
				if fNodes, fEdges := tryExtractFeatureFlag(node, funcNode, fnText, opts, pkgPath, funcNodeHash, flagSeen); len(fEdges) > 0 {
					result.Nodes = append(result.Nodes, fNodes...)
					result.Edges = append(result.Edges, fEdges...)
				}

				// 4. HTTP endpoint consumption detection
				if eNodes, eEdges := tryExtractGoEndpoint(node, funcNode, fnText, opts, pkgPath, funcNodeHash, imports, epSeen); len(eEdges) > 0 {
					result.Nodes = append(result.Nodes, eNodes...)
					result.Edges = append(result.Edges, eEdges...)
				}

				// 5. Throws detection (panic calls)
				if fnText == "panic" {
					if throwEdge := makeThrowEdge(opts, pkgPath, funcNodeHash, "panic"); throwEdge != nil {
						result.Edges = append(result.Edges, *throwEdge)
					}
				}
			}

		case "return_statement":
			// Throws detection: error returns (fmt.Errorf, errors.New, etc.)
			throwEdges := checkReturnForThrows(node, opts, pkgPath, funcNodeHash, imports)
			result.Edges = append(result.Edges, throwEdges...)
		}

		// Recurse into children.
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}

	walk(body)
	return result
}
