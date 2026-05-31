package rustresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers structs, enums, traits (as IsInterface=true), functions, and
// methods (with ReceiverType from impl block). Processes "implements" edges
// to populate EmbeddedTypes for trait-based method dispatch.
func BuildRegistry(defs []typresolve.ResolverDef) *typresolve.Registry {
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

	// Second pass: process "implements" relationships to populate EmbeddedTypes
	// for trait-based method dispatch.
	for _, def := range defs {
		if def.Kind != "implements" {
			continue
		}
		// For "implements" defs, QualifiedName is the implementing type
		// and Signature contains the trait QN being implemented.
		implTypeQN := def.QualifiedName
		traitQN := def.Signature
		if traitQN == "" {
			continue
		}

		t := reg.LookupType(implTypeQN)
		if t == nil {
			continue
		}

		// Add the trait to the type's EmbeddedTypes for method dispatch.
		t.EmbeddedTypes = append(t.EmbeddedTypes, traitQN)
	}

	return reg
}

// shortName extracts the last segment after "." from a qualified name.
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
