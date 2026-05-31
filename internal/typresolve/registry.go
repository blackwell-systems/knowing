package typresolve

// RegisteredFunc represents a function or method registered in the type registry.
// For methods, ReceiverType holds the qualified name of the receiver type.
type RegisteredFunc struct {
	QualifiedName string
	ReceiverType  string   // empty for free functions
	ShortName     string
	Signature     *Type    // KindFunc
	TypeParams    []string // generic type param names
	MinParams     int      // -1 = unknown
}

// RegisteredType represents a type registered in the type registry.
// MethodNames and MethodQNs are parallel arrays: MethodNames[i] is the short
// name and MethodQNs[i] is the qualified name of the same method.
type RegisteredType struct {
	QualifiedName string
	ShortName     string
	Fields        []Field
	MethodNames   []string // parallel with MethodQNs
	MethodQNs     []string // parallel with MethodNames
	EmbeddedTypes []string
	AliasOf       string   // empty if not alias
	TypeParams    []string
	IsInterface   bool
}

// Registry provides hash-indexed lookup tables for functions and types by
// qualified name. It supports fallback chaining for two-level lookup (e.g.
// local workspace registry falling back to a stdlib registry).
//
// Registry is NOT thread-safe for writes. Build it single-threaded during
// InitWorkspace, then use it read-only for concurrent ResolveFile calls.
type Registry struct {
	funcs    map[string]*RegisteredFunc // qualified name -> func
	types    map[string]*RegisteredType // qualified name -> type
	methods  map[string]*RegisteredFunc // "receiver.method" -> func
	fallback *Registry                 // chained parent registry
}

// NewRegistry creates an empty registry with initialized maps.
func NewRegistry() *Registry {
	return &Registry{
		funcs:   make(map[string]*RegisteredFunc),
		types:   make(map[string]*RegisteredType),
		methods: make(map[string]*RegisteredFunc),
	}
}

// SetFallback sets the fallback (parent) registry for chained lookups.
// When a lookup misses in this registry, it falls through to the fallback.
func (r *Registry) SetFallback(parent *Registry) {
	r.fallback = parent
}

// AddFunc registers a function by its qualified name. If the function has a
// ReceiverType, it is also indexed under "ReceiverType.ShortName" in the
// methods map for method lookup.
func (r *Registry) AddFunc(f RegisteredFunc) {
	r.funcs[f.QualifiedName] = &f
	if f.ReceiverType != "" {
		r.methods[f.ReceiverType+"."+f.ShortName] = &f
	}
}

// AddType registers a type by its qualified name.
func (r *Registry) AddType(t RegisteredType) {
	r.types[t.QualifiedName] = &t
}

// LookupFunc looks up a function by qualified name. Returns nil if not found
// in this registry or any fallback.
func (r *Registry) LookupFunc(qualifiedName string) *RegisteredFunc {
	if f, ok := r.funcs[qualifiedName]; ok {
		return f
	}
	if r.fallback != nil {
		return r.fallback.LookupFunc(qualifiedName)
	}
	return nil
}

// LookupType looks up a type by qualified name. Returns nil if not found
// in this registry or any fallback.
func (r *Registry) LookupType(qualifiedName string) *RegisteredType {
	if t, ok := r.types[qualifiedName]; ok {
		return t
	}
	if r.fallback != nil {
		return r.fallback.LookupType(qualifiedName)
	}
	return nil
}

// LookupMethod looks up a method by receiver qualified name and method name.
// The key is "receiverQN.methodName". Returns nil if not found.
func (r *Registry) LookupMethod(receiverQN, methodName string) *RegisteredFunc {
	key := receiverQN + "." + methodName
	if f, ok := r.methods[key]; ok {
		return f
	}
	if r.fallback != nil {
		return r.fallback.LookupMethod(receiverQN, methodName)
	}
	return nil
}

// LookupSymbol looks up a function by constructing "packageQN.name" and
// searching the funcs map. Returns nil if not found.
func (r *Registry) LookupSymbol(packageQN, name string) *RegisteredFunc {
	key := packageQN + "." + name
	if f, ok := r.funcs[key]; ok {
		return f
	}
	if r.fallback != nil {
		return r.fallback.LookupSymbol(packageQN, name)
	}
	return nil
}

// ResolveAlias follows the AliasOf chain on types up to 16 levels deep.
// Returns the concrete (non-alias) type at the end of the chain, or nil
// if the type is not found or the chain exceeds 16 levels (cycle detection).
func (r *Registry) ResolveAlias(typeQN string) *RegisteredType {
	const maxDepth = 16
	for i := 0; i < maxDepth; i++ {
		t := r.LookupType(typeQN)
		if t == nil {
			return nil
		}
		if t.AliasOf == "" {
			return t
		}
		typeQN = t.AliasOf
	}
	// Chain too deep (likely a cycle).
	return nil
}

// IterFuncsByShortName calls fn for every registered function whose ShortName
// matches shortName. Searches this registry and all fallbacks. Used as a
// last-resort resolution strategy when qualified-name lookup fails.
func (r *Registry) IterFuncsByShortName(shortName string, fn func(*RegisteredFunc) bool) {
	for _, f := range r.funcs {
		if f.ShortName == shortName {
			if !fn(f) {
				return
			}
		}
	}
	if r.fallback != nil {
		r.fallback.IterFuncsByShortName(shortName, fn)
	}
}

// FuncCount returns the number of functions registered locally (not including
// fallback).
func (r *Registry) FuncCount() int {
	return len(r.funcs)
}

// TypeCount returns the number of types registered locally (not including
// fallback).
func (r *Registry) TypeCount() int {
	return len(r.types)
}
