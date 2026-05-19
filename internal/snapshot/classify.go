package snapshot

// ChangeClass categorizes what kind of change occurred between two snapshots.
type ChangeClass string

const (
	// Behavioral means call edges changed: functions call different things.
	// Agents should re-query for affected packages.
	Behavioral ChangeClass = "behavioral"

	// Structural means import/implements/extends edges changed: dependency
	// structure shifted but call behavior may be the same.
	Structural ChangeClass = "structural"

	// RuntimeDrift means runtime-observed edges changed: production traffic
	// patterns diverged from static analysis.
	RuntimeDrift ChangeClass = "runtime_drift"

	// MetadataOnly means only non-behavioral edges changed (references,
	// handles_route, configures, etc.). Safe to skip re-query in most cases.
	MetadataOnly ChangeClass = "metadata_only"

	// NoChange means the snapshots are identical.
	NoChange ChangeClass = "no_change"
)

// behavioralEdgeTypes are edge types that indicate behavioral changes.
var behavioralEdgeTypes = map[string]bool{
	"calls": true, "throws": true, "publishes": true,
	"subscribes": true, "connects_to": true,
}

// structuralEdgeTypes are edge types that indicate structural changes.
var structuralEdgeTypes = map[string]bool{
	"imports": true, "implements": true, "extends": true,
	"overrides": true, "depends_on": true, "decorates": true,
}

// runtimeEdgeTypes are edge types that indicate runtime observation changes.
var runtimeEdgeTypes = map[string]bool{
	"runtime_calls": true, "runtime_rpc": true,
	"runtime_produces": true, "runtime_consumes": true,
}

// ChangeClassification is the result of ClassifyChanges.
type ChangeClassification struct {
	// Class is the highest-severity change class detected.
	Class ChangeClass

	// BehavioralCount is the number of changed edge types classified as behavioral.
	BehavioralCount int
	// StructuralCount is the number of changed edge types classified as structural.
	StructuralCount int
	// RuntimeDriftCount is the number of changed edge types classified as runtime drift.
	RuntimeDriftCount int
	// MetadataCount is the number of changed edge types not in any other category.
	MetadataCount int

	// ChangedPackages from the underlying diff.
	ChangedPackages []string
	// ChangedEdgeTypes from the underlying diff (raw "package:edgeType" keys).
	ChangedEdgeTypes []string
}

// ClassifyChanges categorizes the changes between two hierarchical trees
// based on which edge-type roots changed. Returns the highest-severity
// classification: Behavioral > RuntimeDrift > Structural > MetadataOnly > NoChange.
func ClassifyChanges(diff *HierarchicalDiff) ChangeClassification {
	if diff == nil || !diff.RootChanged {
		return ChangeClassification{Class: NoChange}
	}

	c := ChangeClassification{
		ChangedPackages:  diff.ChangedPackages,
		ChangedEdgeTypes: diff.ChangedEdgeTypes,
	}

	for _, key := range diff.ChangedEdgeTypes {
		// Keys are "package:edgeType" format.
		edgeType := key
		if idx := lastColon(key); idx >= 0 {
			edgeType = key[idx+1:]
		}

		switch {
		case behavioralEdgeTypes[edgeType]:
			c.BehavioralCount++
		case runtimeEdgeTypes[edgeType]:
			c.RuntimeDriftCount++
		case structuralEdgeTypes[edgeType]:
			c.StructuralCount++
		default:
			c.MetadataCount++
		}
	}

	// Also count added/removed packages as structural.
	c.StructuralCount += len(diff.AddedPackages) + len(diff.RemovedPackages)

	// Highest severity wins.
	switch {
	case c.BehavioralCount > 0:
		c.Class = Behavioral
	case c.RuntimeDriftCount > 0:
		c.Class = RuntimeDrift
	case c.StructuralCount > 0:
		c.Class = Structural
	case c.MetadataCount > 0:
		c.Class = MetadataOnly
	default:
		c.Class = NoChange
	}

	return c
}

func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}
