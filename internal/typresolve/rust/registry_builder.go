package rustresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers structs, enums, traits (as IsInterface=true), functions, and
// methods (with ReceiverType from impl block). Processes "implements" edges
// to populate EmbeddedTypes for trait-based method dispatch. Also registers
// stdlib types/methods as a fallback and processes derive macro annotations.
func BuildRegistry(defs []typresolve.ResolverDef) *typresolve.Registry {
	// Create stdlib registry as fallback.
	stdlibReg := typresolve.NewRegistry()
	RegisterStdlib(stdlibReg)

	reg := typresolve.NewRegistry()
	reg.SetFallback(stdlibReg)

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

	// Third pass: process "derives" entries to register synthetic trait implementations.
	for _, def := range defs {
		if def.Kind != "derives" {
			continue
		}
		// For "derives" defs, QualifiedName is the type and Signature is the
		// comma-separated list of derive macro names.
		typeQN := def.QualifiedName
		t := reg.LookupType(typeQN)
		if t == nil {
			continue
		}
		derives := strings.Split(def.Signature, ",")
		for _, d := range derives {
			d = strings.TrimSpace(d)
			traitQN := KnownDeriveTraitQN(d)
			if traitQN != "" {
				// Add trait to EmbeddedTypes if not already present.
				found := false
				for _, existing := range t.EmbeddedTypes {
					if existing == traitQN {
						found = true
						break
					}
				}
				if !found {
					t.EmbeddedTypes = append(t.EmbeddedTypes, traitQN)
				}
			}
		}
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
