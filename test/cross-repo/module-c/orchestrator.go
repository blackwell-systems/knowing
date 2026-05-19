// Package modulec orchestrates module-a and module-b.
// It imports both, creating cross-repo edges to A (directly) and B (which
// itself calls A). This tests the full three-module dependency chain.
package modulec

import (
	"fmt"

	modulea "github.com/blackwell-systems/cross-repo-test/module-a"
	moduleb "github.com/blackwell-systems/cross-repo-test/module-b"
)

// Orchestrator coordinates processing across modules.
type Orchestrator struct {
	processor *moduleb.Processor
	auditLog  []string
}

// NewOrchestrator creates an Orchestrator with the given namespace.
func NewOrchestrator(namespace string) *Orchestrator {
	return &Orchestrator{
		processor: moduleb.NewProcessor(namespace),
	}
}

// Ingest processes a batch of items through module-b's processor (which
// calls module-a internally). Then directly calls module-a's Hash for
// the audit log. This creates edges to both A and B.
func (o *Orchestrator) Ingest(items []string) error {
	hashes, err := o.processor.BatchProcess(items, "symbol")
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}

	// Direct call to module-a: creates a C -> A edge (not through B).
	for _, h := range hashes {
		entry := modulea.FormatID("audit", h)
		o.auditLog = append(o.auditLog, entry)
	}

	return nil
}

// Verify checks that all ingested items can be resolved through module-b.
// Calls module-b's Resolve, which calls module-a's Lookup internally.
func (o *Orchestrator) Verify(names []string) (int, int) {
	found, missing := 0, 0
	for _, name := range names {
		if _, ok := o.processor.Resolve(name); ok {
			found++
		} else {
			missing++
		}
	}
	return found, missing
}

// AuditReport returns the orchestrator's audit log and module-b's report.
func (o *Orchestrator) AuditReport() string {
	report := o.processor.FormatReport()
	report += fmt.Sprintf("Audit log entries: %d\n", len(o.auditLog))
	return report
}

// DirectHash calls module-a's Hash directly (C -> A edge, not through B).
// This is deliberately separate from the B -> A path to test that both
// cross-repo paths are detected.
func (o *Orchestrator) DirectHash(input string) string {
	normalized := modulea.Normalize(input)
	return modulea.Hash(normalized)
}

// CheckContainment uses module-a's Contains directly (another C -> A edge).
func (o *Orchestrator) CheckContainment(haystack []string, needle string) bool {
	return modulea.Contains(haystack, needle)
}

// SplitAndProcess splits a qualified name using module-a, then processes
// the parts through module-b. Tests the A <- C -> B triangle.
func (o *Orchestrator) SplitAndProcess(qualifiedName string) (string, error) {
	pkg, name := modulea.SplitQualified(qualifiedName)
	if pkg == "" {
		return "", fmt.Errorf("invalid qualified name: %s", qualifiedName)
	}
	return o.processor.Process(name, pkg)
}
