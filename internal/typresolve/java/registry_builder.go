package javaresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// BuildRegistry builds a typresolve.Registry from ResolverDef entries.
// Registers functions, methods (with receiver type extracted from QN),
// classes, interfaces, and enums.
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

		case "type", "class":
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

		case "enum":
			reg.AddType(typresolve.RegisteredType{
				QualifiedName: def.QualifiedName,
				ShortName:     shortName(def.QualifiedName),
				IsInterface:   false,
				EmbeddedTypes: []string{"java.lang.Enum"},
			})
			// Register synthetic enum methods: values(), valueOf(), name(), ordinal().
			registerEnumSynthetics(reg, def.QualifiedName)
		}
	}

	return reg
}

// shortName extracts the last segment after "." from a qualified name.
// For "com.example.service.UserService.processData", returns "processData".
func shortName(qn string) string {
	if idx := strings.LastIndex(qn, "."); idx >= 0 {
		return qn[idx+1:]
	}
	return qn
}

// extractReceiverType extracts the receiver type QN from a method's
// qualified name. Java method QNs have format:
// "com.example.service.UserService.processData"
// Returns everything before the last "." (i.e. "com.example.service.UserService").
func extractReceiverType(methodQN string) string {
	if idx := strings.LastIndex(methodQN, "."); idx >= 0 {
		return methodQN[:idx]
	}
	return ""
}

// registerEnumSynthetics registers the compiler-generated methods for an enum type:
// values() returns an array of the enum type, valueOf(String) returns the enum type,
// name() returns String, ordinal() returns int.
func registerEnumSynthetics(reg *typresolve.Registry, enumQN string) {
	// values() - returns EnumType[]
	valuesQN := enumQN + ".values"
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: valuesQN,
		ShortName:     "values",
		ReceiverType:  enumQN,
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Named(enumQN))}),
		MinParams:     0,
	})

	// valueOf(String) - returns EnumType
	valueOfQN := enumQN + ".valueOf"
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: valueOfQN,
		ShortName:     "valueOf",
		ReceiverType:  enumQN,
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named(enumQN)}),
		MinParams:     1,
	})

	// name() - returns String
	nameQN := enumQN + ".name"
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: nameQN,
		ShortName:     "name",
		ReceiverType:  enumQN,
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("String")}),
		MinParams:     0,
	})

	// ordinal() - returns int
	ordinalQN := enumQN + ".ordinal"
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: ordinalQN,
		ShortName:     "ordinal",
		ReceiverType:  enumQN,
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
		MinParams:     0,
	})
}
