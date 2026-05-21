package metrics

import (
	"math"
	"testing"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
)

func TestCompareSystems_SignificantDifference(t *testing.T) {
	// System A consistently better than B
	results := []benchtype.MetricResult{
		{System: "A", TaskID: "t1", PrecisionAt10: 0.8},
		{System: "A", TaskID: "t2", PrecisionAt10: 0.7},
		{System: "A", TaskID: "t3", PrecisionAt10: 0.9},
		{System: "A", TaskID: "t4", PrecisionAt10: 0.6},
		{System: "A", TaskID: "t5", PrecisionAt10: 0.8},
		{System: "A", TaskID: "t6", PrecisionAt10: 0.7},
		{System: "A", TaskID: "t7", PrecisionAt10: 0.9},
		{System: "A", TaskID: "t8", PrecisionAt10: 0.8},
		{System: "A", TaskID: "t9", PrecisionAt10: 0.7},
		{System: "A", TaskID: "t10", PrecisionAt10: 0.8},
		{System: "B", TaskID: "t1", PrecisionAt10: 0.3},
		{System: "B", TaskID: "t2", PrecisionAt10: 0.2},
		{System: "B", TaskID: "t3", PrecisionAt10: 0.4},
		{System: "B", TaskID: "t4", PrecisionAt10: 0.1},
		{System: "B", TaskID: "t5", PrecisionAt10: 0.3},
		{System: "B", TaskID: "t6", PrecisionAt10: 0.2},
		{System: "B", TaskID: "t7", PrecisionAt10: 0.4},
		{System: "B", TaskID: "t8", PrecisionAt10: 0.3},
		{System: "B", TaskID: "t9", PrecisionAt10: 0.2},
		{System: "B", TaskID: "t10", PrecisionAt10: 0.3},
	}

	comp := CompareSystems(results, "A", "B", "precision_at_10")

	if !comp.Significant {
		t.Errorf("Expected significant difference, got p=%.4f", comp.WilcoxonP)
	}
	if comp.Difference <= 0 {
		t.Errorf("Expected positive difference (A > B), got %.4f", comp.Difference)
	}
	if comp.CohensD <= 0.5 {
		t.Errorf("Expected large effect size, got d=%.4f", comp.CohensD)
	}
	if comp.TaskCount != 10 {
		t.Errorf("Expected 10 paired tasks, got %d", comp.TaskCount)
	}
	t.Logf("p=%.4f, d=%.2f, CI=[%.3f, %.3f], diff=%.3f",
		comp.WilcoxonP, comp.CohensD, comp.CI95Lower, comp.CI95Upper, comp.Difference)
}

func TestCompareSystems_NoSignificance(t *testing.T) {
	// Two systems with similar performance
	results := []benchtype.MetricResult{
		{System: "A", TaskID: "t1", PrecisionAt10: 0.5},
		{System: "A", TaskID: "t2", PrecisionAt10: 0.6},
		{System: "A", TaskID: "t3", PrecisionAt10: 0.4},
		{System: "A", TaskID: "t4", PrecisionAt10: 0.5},
		{System: "A", TaskID: "t5", PrecisionAt10: 0.6},
		{System: "B", TaskID: "t1", PrecisionAt10: 0.5},
		{System: "B", TaskID: "t2", PrecisionAt10: 0.5},
		{System: "B", TaskID: "t3", PrecisionAt10: 0.5},
		{System: "B", TaskID: "t4", PrecisionAt10: 0.6},
		{System: "B", TaskID: "t5", PrecisionAt10: 0.5},
	}

	comp := CompareSystems(results, "A", "B", "precision_at_10")

	if comp.Significant {
		t.Errorf("Expected no significance, got p=%.4f", comp.WilcoxonP)
	}
	if math.Abs(comp.CohensD) > 0.8 {
		t.Errorf("Expected small effect size, got d=%.4f", comp.CohensD)
	}
}

func TestBootstrapCI_Deterministic(t *testing.T) {
	data := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}

	lower1, upper1 := bootstrapCI(data, 10000, 0.05)
	lower2, upper2 := bootstrapCI(data, 10000, 0.05)

	// Should be deterministic (seeded RNG)
	if lower1 != lower2 || upper1 != upper2 {
		t.Errorf("Bootstrap CI not deterministic: [%.4f,%.4f] vs [%.4f,%.4f]",
			lower1, upper1, lower2, upper2)
	}

	// CI should contain the true mean (0.55)
	if lower1 > 0.55 || upper1 < 0.55 {
		t.Errorf("CI [%.4f, %.4f] does not contain true mean 0.55", lower1, upper1)
	}
}
