package tsresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers functions, methods (with receiver type extracted from QN),
// classes, interfaces, and type aliases. TS methods have QN format:
// "repoURL://module/path.ClassName.MethodName".
func BuildRegistry(defs []typresolve.ResolverDef) *typresolve.Registry {
	reg := typresolve.NewRegistry()

	for _, def := range defs {
		switch def.Kind {
		case "function":
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: def.QualifiedName,
				ShortName:     lastSegment(def.QualifiedName),
				MinParams:     -1,
			})

		case "method":
			// TS methods have QN format: "prefix.ClassName.MethodName"
			// ReceiverType = everything up to the last "." before method name.
			qn := def.QualifiedName
			lastDot := strings.LastIndex(qn, ".")
			if lastDot < 0 {
				// Malformed; register as function.
				reg.AddFunc(typresolve.RegisteredFunc{
					QualifiedName: qn,
					ShortName:     qn,
					MinParams:     -1,
				})
				continue
			}
			receiverType := qn[:lastDot]
			methodName := qn[lastDot+1:]
			reg.AddFunc(typresolve.RegisteredFunc{
				QualifiedName: qn,
				ShortName:     methodName,
				ReceiverType:  receiverType,
				MinParams:     -1,
			})

		case "type", "class":
			reg.AddType(typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     lastSegment(def.QualifiedName),
			})

		case "interface":
			reg.AddType(typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     lastSegment(def.QualifiedName),
				IsInterface:   true,
			})
		}
	}

	return reg
}

// lastSegment returns the part of a qualified name after the last ".".
func lastSegment(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}
