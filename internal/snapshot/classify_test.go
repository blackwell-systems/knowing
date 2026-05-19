package snapshot

import (
	"testing"
)

func TestClassifyChanges_NoChange(t *testing.T) {
	diff := &HierarchicalDiff{RootChanged: false}
	c := ClassifyChanges(diff)
	if c.Class != NoChange {
		t.Errorf("expected NoChange, got %s", c.Class)
	}
}

func TestClassifyChanges_Nil(t *testing.T) {
	c := ClassifyChanges(nil)
	if c.Class != NoChange {
		t.Errorf("expected NoChange for nil diff, got %s", c.Class)
	}
}

func TestClassifyChanges_Behavioral(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:      true,
		ChangedPackages:  []string{"internal/mcp"},
		ChangedEdgeTypes: []string{"internal/mcp:calls", "internal/mcp:imports"},
	}
	c := ClassifyChanges(diff)
	if c.Class != Behavioral {
		t.Errorf("expected Behavioral, got %s", c.Class)
	}
	if c.BehavioralCount != 1 {
		t.Errorf("expected 1 behavioral, got %d", c.BehavioralCount)
	}
	if c.StructuralCount != 1 {
		t.Errorf("expected 1 structural (imports), got %d", c.StructuralCount)
	}
}

func TestClassifyChanges_StructuralOnly(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:      true,
		ChangedPackages:  []string{"internal/store"},
		ChangedEdgeTypes: []string{"internal/store:imports", "internal/store:implements"},
	}
	c := ClassifyChanges(diff)
	if c.Class != Structural {
		t.Errorf("expected Structural, got %s", c.Class)
	}
	if c.BehavioralCount != 0 {
		t.Errorf("expected 0 behavioral, got %d", c.BehavioralCount)
	}
	if c.StructuralCount != 2 {
		t.Errorf("expected 2 structural, got %d", c.StructuralCount)
	}
}

func TestClassifyChanges_RuntimeDrift(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:      true,
		ChangedPackages:  []string{"internal/trace"},
		ChangedEdgeTypes: []string{"internal/trace:runtime_calls"},
	}
	c := ClassifyChanges(diff)
	if c.Class != RuntimeDrift {
		t.Errorf("expected RuntimeDrift, got %s", c.Class)
	}
	if c.RuntimeDriftCount != 1 {
		t.Errorf("expected 1 runtime drift, got %d", c.RuntimeDriftCount)
	}
}

func TestClassifyChanges_MetadataOnly(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:      true,
		ChangedPackages:  []string{"internal/mcp"},
		ChangedEdgeTypes: []string{"internal/mcp:references", "internal/mcp:handles_route"},
	}
	c := ClassifyChanges(diff)
	if c.Class != MetadataOnly {
		t.Errorf("expected MetadataOnly, got %s", c.Class)
	}
	if c.MetadataCount != 2 {
		t.Errorf("expected 2 metadata, got %d", c.MetadataCount)
	}
}

func TestClassifyChanges_BehavioralTrumpsStructural(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:      true,
		ChangedPackages:  []string{"internal/context"},
		ChangedEdgeTypes: []string{"internal/context:imports", "internal/context:calls", "internal/context:references"},
	}
	c := ClassifyChanges(diff)
	if c.Class != Behavioral {
		t.Errorf("expected Behavioral (trumps structural), got %s", c.Class)
	}
	if c.BehavioralCount != 1 {
		t.Errorf("expected 1 behavioral, got %d", c.BehavioralCount)
	}
	if c.StructuralCount != 1 {
		t.Errorf("expected 1 structural, got %d", c.StructuralCount)
	}
	if c.MetadataCount != 1 {
		t.Errorf("expected 1 metadata, got %d", c.MetadataCount)
	}
}

func TestClassifyChanges_AddedPackagesCountAsStructural(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:   true,
		AddedPackages: []string{"internal/newpkg"},
	}
	c := ClassifyChanges(diff)
	if c.Class != Structural {
		t.Errorf("expected Structural for added package, got %s", c.Class)
	}
	if c.StructuralCount != 1 {
		t.Errorf("expected 1 structural (added package), got %d", c.StructuralCount)
	}
}

func TestClassifyChanges_RemovedPackagesCountAsStructural(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged:     true,
		RemovedPackages: []string{"internal/oldpkg"},
	}
	c := ClassifyChanges(diff)
	if c.Class != Structural {
		t.Errorf("expected Structural for removed package, got %s", c.Class)
	}
}

func TestClassifyChanges_AllEdgeTypes(t *testing.T) {
	diff := &HierarchicalDiff{
		RootChanged: true,
		ChangedEdgeTypes: []string{
			"pkg:calls",          // behavioral
			"pkg:throws",         // behavioral
			"pkg:publishes",      // behavioral
			"pkg:imports",        // structural
			"pkg:implements",     // structural
			"pkg:runtime_calls",  // runtime
			"pkg:runtime_rpc",    // runtime
			"pkg:references",     // metadata
			"pkg:handles_route",  // metadata
		},
	}
	c := ClassifyChanges(diff)
	if c.Class != Behavioral {
		t.Errorf("expected Behavioral, got %s", c.Class)
	}
	if c.BehavioralCount != 3 {
		t.Errorf("expected 3 behavioral, got %d", c.BehavioralCount)
	}
	if c.StructuralCount != 2 {
		t.Errorf("expected 2 structural, got %d", c.StructuralCount)
	}
	if c.RuntimeDriftCount != 2 {
		t.Errorf("expected 2 runtime, got %d", c.RuntimeDriftCount)
	}
	if c.MetadataCount != 2 {
		t.Errorf("expected 2 metadata, got %d", c.MetadataCount)
	}
}
