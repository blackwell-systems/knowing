package rustresolve

import (
	"strings"

	"github.com/blackwell-systems/knowing/internal/typresolve"
)

// LookupMethod looks up a method on a named type, following impl block resolution
// and trait implementations (via EmbeddedTypes). Searches up to 8 levels deep.
func LookupMethod(reg *typresolve.Registry, typeQN string, methodName string) *typresolve.RegisteredFunc {
	return lookupMethodDepth(reg, typeQN, methodName, 0)
}

func lookupMethodDepth(reg *typresolve.Registry, typeQN string, methodName string, depth int) *typresolve.RegisteredFunc {
	if depth >= 8 {
		return nil
	}

	// Direct method lookup
	if f := reg.LookupMethod(typeQN, methodName); f != nil {
		return f
	}

	// Get type info
	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// Follow alias
	if t.AliasOf != "" {
		return lookupMethodDepth(reg, t.AliasOf, methodName, depth+1)
	}

	// Check trait impls (EmbeddedTypes stores trait QNs for Rust)
	for _, traitQN := range t.EmbeddedTypes {
		if f := lookupMethodDepth(reg, traitQN, methodName, depth+1); f != nil {
			return f
		}
	}

	return nil
}

// LookupField looks up a struct field on a named type. Returns the field's type
// if found, nil otherwise.
func LookupField(reg *typresolve.Registry, typeQN string, fieldName string) *typresolve.Type {
	return lookupFieldDepth(reg, typeQN, fieldName, 0)
}

func lookupFieldDepth(reg *typresolve.Registry, typeQN string, fieldName string, depth int) *typresolve.Type {
	if depth >= 8 {
		return nil
	}

	t := reg.LookupType(typeQN)
	if t == nil {
		return nil
	}

	// Follow alias
	if t.AliasOf != "" {
		return lookupFieldDepth(reg, t.AliasOf, fieldName, depth+1)
	}

	// Search fields
	for _, f := range t.Fields {
		if f.Name == fieldName {
			return f.Type
		}
	}

	return nil
}

// DerefToBase strips ownership/reference wrappers for method resolution.
// Applies iteratively (max 4 levels) to handle nested wrappers like &Box<T>.
func DerefToBase(t *typresolve.Type) *typresolve.Type {
	if t == nil {
		return nil
	}

	for i := 0; i < 4; i++ {
		switch t.Kind {
		case typresolve.KindReference:
			if t.Elem != nil {
				t = t.Elem
				continue
			}
			return t
		case typresolve.KindPointer:
			if t.Elem != nil {
				t = t.Elem
				continue
			}
			return t
		case typresolve.KindNamed:
			// Auto-deref for Box, Arc, Rc
			if (strings.HasPrefix(t.Name, "std::Box") ||
				strings.HasPrefix(t.Name, "std::Arc") ||
				strings.HasPrefix(t.Name, "std::Rc")) && t.Elem != nil {
				t = t.Elem
				continue
			}
			return t
		default:
			return t
		}
	}

	return t
}
