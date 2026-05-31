package rubyresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers classes, modules, methods (instance and singleton), and
// constants. Processes include/extend/prepend edges to populate
// EmbeddedTypes for MRO-based attribute lookup.
func BuildRegistry(defs []typresolve.ResolverDef) *typresolve.Registry {
	reg := typresolve.NewRegistry()

	for _, def := range defs {
		switch def.Kind {
		case "function":
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     rubyShortName(def.QualifiedName),
				MinParams:     -1,
			})

		case "method":
			receiver := rubyExtractReceiverType(def.QualifiedName)
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     rubyShortName(def.QualifiedName),
				ReceiverType:  receiver,
				MinParams:     -1,
			})

		case "type":
			isIface := false
			// Parse signature to determine class vs module.
			if strings.HasPrefix(def.Signature, "module ") {
				// Modules can act like interfaces (mixins).
				isIface = true
			}
			// Open class merging: if the type already exists, preserve its
			// accumulated state (EmbeddedTypes, methods) rather than overwriting.
			if existing := reg.LookupType(def.QualifiedName); existing != nil {
				// Merge: upgrade to interface if any definition is a module.
				if isIface {
					existing.IsInterface = true
				}
			} else {
				reg.AddType(typresolve.RegisteredType{
					QualifiedName: def.QualifiedName,
					ShortName:     rubyShortName(def.QualifiedName),
					IsInterface:   isIface,
				})
			}
		}
	}

	// Second pass: process implements/extends relationships.
	for _, def := range defs {
		switch def.Kind {
		case "implements":
			// include/extend/prepend: source includes target module.
			// def.QualifiedName is the source type, def.Signature is the target module.
			srcType := reg.LookupType(def.QualifiedName)
			if srcType != nil && def.Signature != "" {
				srcType.EmbeddedTypes = append(srcType.EmbeddedTypes, def.Signature)
			}

		case "extends":
			// Class inheritance: source extends target class.
			// def.QualifiedName is the subclass, def.Signature is the superclass.
			srcType := reg.LookupType(def.QualifiedName)
			if srcType != nil && def.Signature != "" {
				// Superclass goes first in EmbeddedTypes (MRO: superclass after mixins).
				srcType.EmbeddedTypes = append(srcType.EmbeddedTypes, def.Signature)
			}
		}
	}

	return reg
}

// rubyShortName extracts the last segment after "." from a qualified name.
// For Ruby QN format "repoURL://filePath.ClassName.methodName", returns "methodName".
func rubyShortName(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

// rubyExtractReceiverType extracts the receiver type QN from a method's
// qualified name. Ruby format: "repoURL://filePath.ClassName.methodName"
// Returns "repoURL://filePath.ClassName".
func rubyExtractReceiverType(methodQN string) string {
	if idx := strings.LastIndex(methodQN, "."); idx >= 0 {
		return methodQN[:idx]
	}
	return ""
}
