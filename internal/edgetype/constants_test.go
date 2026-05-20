package edgetype

import "testing"

func TestRWRWeight_KnownTypes(t *testing.T) {
	tests := []struct {
		edgeType string
		want     float64
	}{
		{Calls, 1.0},
		{Implements, 0.8},
		{Overrides, 0.8},
		{HandlesRoute, 0.7},
		{Extends, 0.7},
		{Imports, 0.5},
		{DependsOn, 0.5},
		{Tests, 0.6},
		{References, 0.4},
		{Throws, 0.4},
		{Decorates, 0.3},
		{Documents, 0.2},
		{ConsumesEndpoint, 0.5},
		{ImplementsRPC, 0.8},
		{ConsumesRPC, 0.6},
		{GatedByFlag, 0.3},
		{DeployedBy, 0.4},
		{TestedBy, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.edgeType, func(t *testing.T) {
			got := RWRWeight(tt.edgeType)
			if got != tt.want {
				t.Errorf("RWRWeight(%q) = %v, want %v", tt.edgeType, got, tt.want)
			}
		})
	}
}

func TestRWRWeight_UnknownType(t *testing.T) {
	got := RWRWeight("some_unknown_type")
	if got != 0.3 {
		t.Errorf("RWRWeight(unknown) = %v, want 0.3", got)
	}

	got = RWRWeight("")
	if got != 0.3 {
		t.Errorf("RWRWeight(\"\") = %v, want 0.3", got)
	}
}

func TestRWRWeight_OwnershipZero(t *testing.T) {
	if w := RWRWeight(OwnedBy); w != 0.0 {
		t.Errorf("RWRWeight(%q) = %v, want 0.0", OwnedBy, w)
	}
	if w := RWRWeight(AuthoredBy); w != 0.0 {
		t.Errorf("RWRWeight(%q) = %v, want 0.0", AuthoredBy, w)
	}
}

func TestRWRWeight_TestsEdge(t *testing.T) {
	got := RWRWeight(Tests)
	if got != 0.6 {
		t.Errorf("RWRWeight(%q) = %v, want 0.6", Tests, got)
	}
}

func TestConstants_NoEmpty(t *testing.T) {
	constants := []string{
		Calls,
		Imports,
		Implements,
		References,
		HandlesRoute,
		DependsOn,
		Deploys,
		Exposes,
		Configures,
		Publishes,
		Subscribes,
		ConnectsTo,
		Extends,
		Overrides,
		Decorates,
		Throws,
		OwnedBy,
		AuthoredBy,
		Tests,
		RuntimeCalls,
		RuntimeRPC,
		RuntimeProduces,
		RuntimeConsumes,
		Documents,
		ConsumesEndpoint,
		ImplementsRPC,
		ConsumesRPC,
		GatedByFlag,
		DeployedBy,
		TestedBy,
	}

	for _, c := range constants {
		if c == "" {
			t.Error("found empty string constant")
		}
	}
}
