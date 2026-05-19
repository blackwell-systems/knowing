// Package moduleb processes entities from module-a.
// It imports module-a's helpers and types, creating cross-repo call edges.
package moduleb

import (
	"fmt"
	"strings"

	modulea "github.com/blackwell-systems/cross-repo-test/module-a"
)

// Processor transforms entities using module-a's utilities.
type Processor struct {
	registry *modulea.Registry
	prefix   string
}

// NewProcessor creates a Processor with the given prefix.
func NewProcessor(prefix string) *Processor {
	return &Processor{
		registry: modulea.NewRegistry(),
		prefix:   modulea.Normalize(prefix),
	}
}

// Process creates an entity, registers it, and returns its hash.
// This creates cross-repo edges: calls modulea.NewEntity, modulea.Hash,
// and modulea.Registry.Register.
func (p *Processor) Process(name, kind string) (string, error) {
	if err := modulea.ValidateNonEmpty(name, kind); err != nil {
		return "", fmt.Errorf("process: %w", err)
	}

	fullName := p.prefix + "." + modulea.Normalize(name)
	entity := modulea.NewEntity(fullName, kind)

	if err := p.registry.Register(entity); err != nil {
		return "", fmt.Errorf("register: %w", err)
	}

	return entity.Hash, nil
}

// Resolve looks up an entity by name using the registry.
func (p *Processor) Resolve(name string) (modulea.Entity, bool) {
	return p.registry.Lookup(name)
}

// BatchProcess processes multiple items and returns their hashes.
func (p *Processor) BatchProcess(items []string, kind string) ([]string, error) {
	hashes := make([]string, 0, len(items))
	for _, item := range items {
		h, err := p.Process(item, kind)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, h)
	}
	return modulea.Deduplicate(hashes), nil
}

// FormatReport generates a report of all registered entities.
func (p *Processor) FormatReport() string {
	names := p.registry.Names()
	var b strings.Builder
	fmt.Fprintf(&b, "Processor report (prefix=%s, count=%d)\n", p.prefix, len(names))
	for _, name := range names {
		if e, ok := p.registry.Lookup(name); ok {
			fmt.Fprintf(&b, "  %s [%s] hash=%s\n", e.QualifiedName(), e.Kind, e.Hash[:8])
		}
	}
	return b.String()
}
