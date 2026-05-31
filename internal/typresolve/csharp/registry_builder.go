package csresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers classes, interfaces, structs, enums, methods (with receiver type),
// constructors, and properties (as methods). Handles partial class merging by
// accumulating methods and fields from multiple definitions of the same type QN.
func BuildRegistry(defs []typresolve.ResolverDef) *typresolve.Registry {
	reg := typresolve.NewRegistry()

	// Track seen types for partial class merging.
	seenTypes := make(map[string]bool)

	for _, def := range defs {
		switch def.Kind {
		case "function":
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     csShortName(def.QualifiedName),
				MinParams:     -1,
			})

		case "method":
			receiver := csExtractReceiverType(def.QualifiedName)
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     csShortName(def.QualifiedName),
				ReceiverType:  receiver,
				MinParams:     -1,
			})

		case "constructor":
			// Register constructors as methods on the type.
			// Constructor QN is typically "Namespace.ClassName..ctor" or "Namespace.ClassName.ClassName"
			receiver := csExtractReceiverType(def.QualifiedName)
			shortName := csShortName(def.QualifiedName)
			if shortName == ".ctor" {
				// Normalize to class short name for lookup.
				shortName = csShortName(receiver)
			}
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName,
				ReceiverType:  receiver,
				MinParams:     -1,
			})

		case "property":
			// Register properties as methods (get/set accessors).
			receiver := csExtractReceiverType(def.QualifiedName)
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     csShortName(def.QualifiedName),
				ReceiverType:  receiver,
				MinParams:     0,
			})

		case "type", "class", "struct", "enum":
			if !seenTypes[def.QualifiedName] {
				reg.AddType(typresolve.RegisteredType{
					QualifiedName: def.QualifiedName,
					ShortName:     csShortName(def.QualifiedName),
					IsInterface:   false,
				})
				seenTypes[def.QualifiedName] = true
			}
			// Partial class merging: if the type already exists, the registry
			// accumulates methods registered with this QN as receiver.

		case "interface":
			if !seenTypes[def.QualifiedName] {
				reg.AddType(typresolve.RegisteredType{
					QualifiedName: def.QualifiedName,
					ShortName:     csShortName(def.QualifiedName),
					IsInterface:   true,
				})
				seenTypes[def.QualifiedName] = true
			}
		}
	}

	return reg
}

// csShortName extracts the last segment after "." from a qualified name.
// For "MyApp.MyService.DoWork", returns "DoWork".
func csShortName(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

// csExtractReceiverType extracts the receiver type QN from a method's
// qualified name. Format: "Namespace.ClassName.MethodName"
// Returns "Namespace.ClassName".
func csExtractReceiverType(methodQN string) string {
	if idx := strings.LastIndex(methodQN, "."); idx >= 0 {
		return methodQN[:idx]
	}
	return ""
}
