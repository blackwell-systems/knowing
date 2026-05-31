package pyresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// pythonBuiltinScalars maps Python builtin scalar type names to true.
var pythonBuiltinScalars = map[string]bool{
	"int":       true,
	"str":       true,
	"bool":      true,
	"float":     true,
	"bytes":     true,
	"None":      true,
	"complex":   true,
	"bytearray": true,
	"object":    true,
}

// typingNames is the set of names from the typing module that should NOT
// be qualified with the module prefix (they are well-known typing constructs,
// not user-defined types).
var typingNames = map[string]bool{
	"Any":            true,
	"Self":           true,
	"Iterator":       true,
	"Iterable":       true,
	"Generator":      true,
	"Coroutine":      true,
	"AsyncIterator":  true,
	"AsyncIterable":  true,
	"AsyncGenerator": true,
	"Awaitable":      true,
	"Sequence":       true,
	"Mapping":        true,
	"MutableMapping": true,
	"MutableSequence": true,
	"AbstractSet":    true,
	"MutableSet":     true,
	"Reversible":     true,
	"SupportsInt":    true,
	"SupportsFloat":  true,
	"SupportsComplex": true,
	"SupportsBytes":  true,
	"SupportsAbs":    true,
	"SupportsRound":  true,
	"Hashable":       true,
	"Sized":          true,
	"Container":      true,
	"Collection":     true,
	"ByteString":     true,
	"Pattern":        true,
	"Match":          true,
	"IO":             true,
	"TextIO":         true,
	"BinaryIO":       true,
	"NoReturn":       true,
	"Never":          true,
	"TypeVar":        true,
	"ParamSpec":      true,
	"TypeVarTuple":   true,
	"Protocol":       true,
	"TypeGuard":      true,
	"TypeAlias":      true,
	"Unpack":         true,
	"Concatenate":    true,
	"LiteralString":  true,
}

// typeWrappers is the set of typing wrapper names that should be unwrapped
// to their inner type (e.g., ClassVar[int] -> int).
var typeWrappers = map[string]bool{
	"ClassVar":    true,
	"Final":       true,
	"InitVar":     true,
	"ReadOnly":    true,
	"Required":    true,
	"NotRequired": true,
	"Annotated":   true,
	"Mapped":      true,
}

// ParseAnnotation parses a Python type annotation text into a typresolve.Type.
// It handles Optional[T], Union[A, B], X | Y (PEP 604), generic containers
// (list[T] -> Slice, dict[K, V] -> Map), forward reference quotes,
// Callable[[Args], Return], typing wrappers (ClassVar, Final, Annotated),
// and builtin scalar names. moduleQN is used to qualify bare unqualified
// names that are not builtins or typing constructs.
func ParseAnnotation(ann string, moduleQN string) *typresolve.Type {
	ann = strings.TrimSpace(ann)
	if ann == "" {
		return nil
	}

	// Handle quoted forward references: "Foo" or 'Foo'
	if len(ann) >= 2 && ((ann[0] == '"' && ann[len(ann)-1] == '"') ||
		(ann[0] == '\'' && ann[len(ann)-1] == '\'')) {
		inner := ann[1 : len(ann)-1]
		return ParseAnnotation(inner, moduleQN)
	}

	// Handle PEP 604 pipe syntax: X | Y
	// Only split at top-level pipes (not inside brackets).
	if pipeMembers := splitPipe(ann); len(pipeMembers) > 1 {
		return parseUnionMembers(pipeMembers, moduleQN)
	}

	// Handle subscript forms: Name[...]
	if baseName, args, ok := parseSubscript(ann); ok {
		return parseSubscriptAnnotation(baseName, args, moduleQN)
	}

	// Builtin scalars
	if pythonBuiltinScalars[ann] {
		return typresolve.Builtin(ann)
	}

	// Bare unqualified name
	return qualifyName(ann, moduleQN)
}

// splitPipe splits an annotation string on top-level pipe characters,
// respecting bracket nesting. Returns a single-element slice if no
// top-level pipe is found.
func splitPipe(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '|':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

// parseUnionMembers handles Union-like semantics for pipe-separated or
// Union[...] members. If exactly 2 members and one is None, returns
// Optional of the other. Otherwise returns the first non-None member
// as best-effort.
func parseUnionMembers(members []string, moduleQN string) *typresolve.Type {
	// Check for Optional pattern: exactly 2 members, one is None
	if len(members) == 2 {
		if members[0] == "None" {
			return typresolve.Optional(ParseAnnotation(members[1], moduleQN))
		}
		if members[1] == "None" {
			return typresolve.Optional(ParseAnnotation(members[0], moduleQN))
		}
	}

	// Return first non-None member as best-effort
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m != "None" {
			return ParseAnnotation(m, moduleQN)
		}
	}

	// All None (degenerate case)
	return typresolve.Builtin("None")
}

// parseSubscript extracts the base name and argument string from a
// subscript form like "list[int]". Returns (base, argsStr, true) or
// ("", "", false) if not a subscript.
func parseSubscript(ann string) (string, string, bool) {
	idx := strings.IndexByte(ann, '[')
	if idx < 0 {
		return "", "", false
	}
	// Verify matching close bracket at end
	if ann[len(ann)-1] != ']' {
		return "", "", false
	}
	base := strings.TrimSpace(ann[:idx])
	args := ann[idx+1 : len(ann)-1]
	return base, args, true
}

// parseSubscriptAnnotation handles all subscript forms:
// list[T], dict[K,V], set[T], tuple[A,B], Optional[T], Union[A,B],
// Callable[[Args], Return], Type[T], type[T], ClassVar[T], etc.
func parseSubscriptAnnotation(baseName, argsStr, moduleQN string) *typresolve.Type {
	// Strip typing. prefix if present
	plain := baseName
	if strings.HasPrefix(plain, "typing.") {
		plain = plain[len("typing."):]
	}

	switch plain {
	case "list", "List":
		inner := ParseAnnotation(argsStr, moduleQN)
		return typresolve.Slice(inner)

	case "dict", "Dict":
		parts := splitSubscriptArgs(argsStr)
		if len(parts) >= 2 {
			k := ParseAnnotation(parts[0], moduleQN)
			v := ParseAnnotation(parts[1], moduleQN)
			return typresolve.Map(k, v)
		}
		return typresolve.Named("builtins.dict")

	case "set", "Set", "frozenset", "FrozenSet":
		return typresolve.Named("builtins." + strings.ToLower(plain))

	case "tuple", "Tuple":
		parts := splitSubscriptArgs(argsStr)
		var elems []*typresolve.Type
		for _, p := range parts {
			elems = append(elems, ParseAnnotation(p, moduleQN))
		}
		return typresolve.Tuple(elems)

	case "Optional":
		inner := ParseAnnotation(argsStr, moduleQN)
		return typresolve.Optional(inner)

	case "Union":
		parts := splitSubscriptArgs(argsStr)
		return parseUnionMembers(parts, moduleQN)

	case "Callable":
		return parseCallable(argsStr, moduleQN)

	case "Type", "type":
		// Type[T] -> return T itself (class object treated as class type)
		inner := ParseAnnotation(argsStr, moduleQN)
		return inner

	default:
		// Check for type wrappers (ClassVar, Final, Annotated, etc.)
		if typeWrappers[plain] {
			// For Annotated, only the first arg is the type
			if plain == "Annotated" {
				parts := splitSubscriptArgs(argsStr)
				if len(parts) > 0 {
					return ParseAnnotation(parts[0], moduleQN)
				}
			}
			return ParseAnnotation(argsStr, moduleQN)
		}

		// Generic user type: MyClass[T] -> Named
		return qualifyName(baseName, moduleQN)
	}
}

// parseCallable parses a Callable[[Args], Return] annotation.
// The argsStr is expected to be "[int, str], bool" (the content between
// the outer Callable[...]).
func parseCallable(argsStr string, moduleQN string) *typresolve.Type {
	// Find the inner bracket list for params
	argsStr = strings.TrimSpace(argsStr)
	if len(argsStr) == 0 {
		return typresolve.Func(nil, nil)
	}

	// Look for the inner [...] for parameter types
	if argsStr[0] == '[' {
		// Find matching close bracket
		depth := 0
		closeIdx := -1
		for i := 0; i < len(argsStr); i++ {
			switch argsStr[i] {
			case '[':
				depth++
			case ']':
				depth--
				if depth == 0 {
					closeIdx = i
					goto found
				}
			}
		}
	found:
		if closeIdx >= 0 {
			paramStr := argsStr[1:closeIdx]
			rest := strings.TrimSpace(argsStr[closeIdx+1:])

			// Parse parameters
			var params []typresolve.Param
			if paramStr != "" {
				paramParts := splitSubscriptArgs(paramStr)
				for i, pp := range paramParts {
					pt := ParseAnnotation(pp, moduleQN)
					params = append(params, typresolve.Param{
						Name: "", // Python Callable doesn't name params
						Type: pt,
					})
					_ = i
				}
			}

			// Parse return type (after the comma following the close bracket)
			var returns []*typresolve.Type
			if rest != "" {
				rest = strings.TrimPrefix(rest, ",")
				rest = strings.TrimSpace(rest)
				if rest != "" {
					rt := ParseAnnotation(rest, moduleQN)
					returns = append(returns, rt)
				}
			}

			return typresolve.Func(params, returns)
		}
	}

	// Fallback: can't parse, return generic Callable
	return typresolve.Named("typing.Callable")
}

// qualifyName qualifies a bare name with the module QN if appropriate.
// Typing-module names (Self, Any, Iterator, etc.) are returned as Named
// without module prefix. Dotted names are returned as-is.
func qualifyName(name string, moduleQN string) *typresolve.Type {
	if name == "" {
		return nil
	}

	// Typing names are not qualified to the user's module
	if typingNames[name] {
		return typresolve.Named(name)
	}

	// Already has a dot (qualified)
	if strings.Contains(name, ".") {
		return typresolve.Named(name)
	}

	// Qualify with module
	if moduleQN != "" {
		return typresolve.Named(moduleQN + "." + name)
	}

	return typresolve.Named(name)
}

// splitSubscriptArgs splits a comma-separated string at depth 0,
// ignoring commas inside [], (), {}. Trims whitespace from each part.
func splitSubscriptArgs(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(', '{':
			depth++
		case ']', ')', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	part := strings.TrimSpace(s[start:])
	if part != "" {
		parts = append(parts, part)
	}
	return parts
}
