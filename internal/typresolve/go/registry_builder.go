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
func BuildRegistry(defs []typresolve.ResolverDef, fileContents map[string][]byte, fileTrees map[string]*sitter.Node) *typresolve.Registry {
	reg := typresolve.NewRegistry()

	for _, def := range defs {
		switch def.Kind {
		case "function":
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				MinParams:     -1,
			})

		case "method":
			receiver := extractReceiverType(def.QualifiedName)
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				ReceiverType:  receiver,
				MinParams:     -1,
			})

		case "type":
			reg.AddType(typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				IsInterface:   false,
			})

		case "interface":
			reg.AddType(typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				IsInterface:   true,
			})
		}
	}

	return reg
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
