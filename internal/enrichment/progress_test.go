package enrichment

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadProgress_NoFile(t *testing.T) {
	dir := t.TempDir()
	p, err := LoadProgress(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil progress")
	}
	if len(p.Modules) != 0 {
		t.Fatalf("expected empty modules map, got %d entries", len(p.Modules))
	}
	if p.StartedAt.IsZero() {
		t.Fatal("expected StartedAt to be set")
	}
	if time.Since(p.StartedAt) > 5*time.Second {
		t.Fatal("StartedAt should be recent")
	}
}

func TestLoadProgress_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	knowingDir := filepath.Join(dir, ".knowing")
	if err := os.MkdirAll(knowingDir, 0755); err != nil {
		t.Fatal(err)
	}

	started := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2025, 1, 1, 12, 5, 0, 0, time.UTC)
	original := &EnrichProgress{
		Modules: map[string]ModuleStatus{
			"./cmd/app": {
				Completed: true,
				UpdatedAt: updated,
			},
			"./pkg/lib": {
				Completed: false,
				Error:     "gopls timeout",
				UpdatedAt: updated,
			},
		},
		StartedAt: started,
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowingDir, progressFileName), data, 0644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadProgress(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !p.StartedAt.Equal(started) {
		t.Fatalf("StartedAt mismatch: got %v, want %v", p.StartedAt, started)
	}
	if len(p.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(p.Modules))
	}
	if !p.Modules["./cmd/app"].Completed {
		t.Fatal("expected ./cmd/app to be completed")
	}
	if p.Modules["./pkg/lib"].Error != "gopls timeout" {
		t.Fatalf("expected error 'gopls timeout', got %q", p.Modules["./pkg/lib"].Error)
	}
}

func TestLoadProgress_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	knowingDir := filepath.Join(dir, ".knowing")
	if err := os.MkdirAll(knowingDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(knowingDir, progressFileName), []byte("not json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProgress(dir)
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

func TestSaveProgress_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	knowingDir := filepath.Join(dir, ".knowing")

	// Ensure .knowing does not exist
	if _, err := os.Stat(knowingDir); !os.IsNotExist(err) {
		t.Fatal("expected .knowing to not exist initially")
	}

	p := &EnrichProgress{
		Modules:   make(map[string]ModuleStatus),
		StartedAt: time.Now(),
	}

	if err := SaveProgress(dir, p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(knowingDir)
	if err != nil {
		t.Fatalf("expected .knowing directory to exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("expected .knowing to be a directory")
	}
}

func TestSaveProgress_AtomicWrite(t *testing.T) {
	dir := t.TempDir()

	p := &EnrichProgress{
		Modules: map[string]ModuleStatus{
			"./internal/foo": {
				Completed: true,
				UpdatedAt: time.Now(),
			},
		},
		StartedAt: time.Now(),
	}

	if err := SaveProgress(dir, p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read back and verify it's valid JSON
	data, err := os.ReadFile(filepath.Join(dir, ".knowing", progressFileName))
	if err != nil {
		t.Fatalf("unexpected error reading file: %v", err)
	}

	var loaded EnrichProgress
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("file content is not valid JSON: %v", err)
	}
	if !loaded.Modules["./internal/foo"].Completed {
		t.Fatal("expected module to be completed after reload")
	}

	// Verify .tmp file does not remain
	tmpPath := filepath.Join(dir, ".knowing", progressFileName+".tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatal("expected .tmp file to not exist after successful save")
	}
}

func TestMarkModule_Success(t *testing.T) {
	p := &EnrichProgress{
		Modules:   make(map[string]ModuleStatus),
		StartedAt: time.Now(),
	}

	p.MarkModule("./cmd/app", nil)

	s, ok := p.Modules["./cmd/app"]
	if !ok {
		t.Fatal("expected module to exist in map")
	}
	if !s.Completed {
		t.Fatal("expected Completed=true")
	}
	if s.Error != "" {
		t.Fatalf("expected empty error, got %q", s.Error)
	}
	if s.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestMarkModule_Error(t *testing.T) {
	p := &EnrichProgress{
		Modules:   make(map[string]ModuleStatus),
		StartedAt: time.Now(),
	}

	p.MarkModule("./pkg/broken", errors.New("connection refused"))

	s, ok := p.Modules["./pkg/broken"]
	if !ok {
		t.Fatal("expected module to exist in map")
	}
	if s.Completed {
		t.Fatal("expected Completed=false")
	}
	if s.Error != "connection refused" {
		t.Fatalf("expected error 'connection refused', got %q", s.Error)
	}
	if s.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestIsComplete(t *testing.T) {
	p := &EnrichProgress{
		Modules: map[string]ModuleStatus{
			"./completed": {Completed: true, UpdatedAt: time.Now()},
			"./errored":   {Completed: false, Error: "failed", UpdatedAt: time.Now()},
		},
		StartedAt: time.Now(),
	}

	if !p.IsComplete("./completed") {
		t.Fatal("expected IsComplete=true for completed module")
	}
	if p.IsComplete("./errored") {
		t.Fatal("expected IsComplete=false for errored module")
	}
	if p.IsComplete("./missing") {
		t.Fatal("expected IsComplete=false for missing module")
	}
}

func TestReset(t *testing.T) {
	p := &EnrichProgress{
		Modules: map[string]ModuleStatus{
			"./a": {Completed: true, UpdatedAt: time.Now()},
			"./b": {Completed: true, UpdatedAt: time.Now()},
		},
		StartedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	p.Reset()

	if len(p.Modules) != 0 {
		t.Fatalf("expected empty modules after reset, got %d", len(p.Modules))
	}
	if p.StartedAt.Year() == 2024 {
		t.Fatal("expected StartedAt to be updated after reset")
	}
	if time.Since(p.StartedAt) > 5*time.Second {
		t.Fatal("expected StartedAt to be recent after reset")
	}
}
