package typresolve

// Scope is a linked-list scope chain for variable-to-type bindings during
// AST walk. Each scope level holds its own bindings map and a pointer to
// the enclosing (parent) scope. Scope is NOT thread-safe; it is used
// within a single ResolveFile call (one goroutine per file).
//
// This is the Go equivalent of CBMScope from codebase-memory's
// scope.c, simplified to use map[string]*Type per level instead of
// arena-allocated linked chunks.
type Scope struct {
	bindings map[string]*Type
	parent   *Scope
}

// NewScope creates a new scope level. Pass nil for the root scope.
func NewScope(parent *Scope) *Scope {
	return &Scope{
		bindings: make(map[string]*Type),
		parent:   parent,
	}
}

// Bind binds a variable name to a type in the current scope. If the name
// already exists in the current scope, it is overwritten.
func (s *Scope) Bind(name string, typ *Type) {
	s.bindings[name] = typ
}

// Lookup walks up the scope chain looking for name. Returns the first
// match found. Returns nil if the name is not bound in any enclosing scope.
func (s *Scope) Lookup(name string) *Type {
	for cur := s; cur != nil; cur = cur.parent {
		if t, ok := cur.bindings[name]; ok {
			return t
		}
	}
	return nil
}

// Parent returns the parent scope, or nil for the root scope.
func (s *Scope) Parent() *Scope {
	return s.parent
}
