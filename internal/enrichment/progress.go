package enrichment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// progressFileName is the name of the progress tracking file within .knowing/.
const progressFileName = "enrich-progress.json"

// EnrichProgress tracks per-module enrichment completion.
type EnrichProgress struct {
	Modules   map[string]ModuleStatus `json:"modules"`
	StartedAt time.Time               `json:"started_at"`
}

// ModuleStatus tracks the enrichment state of a single module.
type ModuleStatus struct {
	Completed bool      `json:"completed"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoadProgress reads progress from .knowing/enrich-progress.json.
// Returns a fresh EnrichProgress if the file does not exist.
func LoadProgress(workspaceRoot string) (*EnrichProgress, error) {
	path := filepath.Join(workspaceRoot, ".knowing", progressFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &EnrichProgress{
				Modules:   make(map[string]ModuleStatus),
				StartedAt: time.Now(),
			}, nil
		}
		return nil, err
	}

	var p EnrichProgress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	if p.Modules == nil {
		p.Modules = make(map[string]ModuleStatus)
	}
	return &p, nil
}

// SaveProgress writes progress to .knowing/enrich-progress.json.
// Creates the .knowing directory if it does not exist. The write is atomic:
// data is written to a temporary file first, then renamed into place.
func SaveProgress(workspaceRoot string, p *EnrichProgress) error {
	dir := filepath.Join(workspaceRoot, ".knowing")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}

	target := filepath.Join(dir, progressFileName)
	tmp := target + ".tmp"

	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmp, target)
}

// MarkModule records the result of enriching a module. If err is nil,
// the module is marked as completed. Otherwise, the error message is stored.
func (p *EnrichProgress) MarkModule(modulePath string, err error) {
	status := ModuleStatus{
		UpdatedAt: time.Now(),
	}
	if err == nil {
		status.Completed = true
	} else {
		status.Error = err.Error()
	}
	p.Modules[modulePath] = status
}

// IsComplete returns true if the module has been successfully enriched.
func (p *EnrichProgress) IsComplete(modulePath string) bool {
	s, ok := p.Modules[modulePath]
	return ok && s.Completed
}

// Reset clears all progress (used when a fresh run is requested).
func (p *EnrichProgress) Reset() {
	p.Modules = make(map[string]ModuleStatus)
	p.StartedAt = time.Now()
}
