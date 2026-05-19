package modulea

// Entity is a named object with a hash identity.
type Entity struct {
	Name string
	Hash string
	Kind string
}

// NewEntity creates an Entity with a computed hash.
func NewEntity(name, kind string) Entity {
	return Entity{
		Name: name,
		Hash: Hash(name + ":" + kind),
		Kind: kind,
	}
}

// QualifiedName returns the entity's qualified identifier.
func (e Entity) QualifiedName() string {
	return FormatID(e.Kind, e.Name)
}

// Registry holds a set of entities keyed by name.
type Registry struct {
	entries map[string]Entity
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]Entity)}
}

// Register adds an entity. Returns error if name is empty.
func (r *Registry) Register(e Entity) error {
	if err := ValidateNonEmpty(e.Name); err != nil {
		return err
	}
	r.entries[e.Name] = e
	return nil
}

// Lookup finds an entity by name. Returns false if not found.
func (r *Registry) Lookup(name string) (Entity, bool) {
	e, ok := r.entries[Normalize(name)]
	return e, ok
}

// Names returns all registered entity names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.entries))
	for k := range r.entries {
		names = append(names, k)
	}
	return Deduplicate(names)
}
