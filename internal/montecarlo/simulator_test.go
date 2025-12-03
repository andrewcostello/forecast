package montecarlo

import (
	"testing"
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

func TestNewSimulator(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "3", Status: "To Do", Size: "M"},
		{ID: "4", Status: "To Do", Size: "L"},
	}

	sim := NewSimulator(items, 8.0)

	if sim == nil {
		t.Fatal("expected simulator to be created")
	}
	if sim.TeamCapacity != 8.0 {
		t.Errorf("expected team capacity 8.0, got %f", sim.TeamCapacity)
	}
	if sim.Remaining["M"] != 1 {
		t.Errorf("expected 1 remaining M item, got %d", sim.Remaining["M"])
	}
	if sim.Remaining["L"] != 1 {
		t.Errorf("expected 1 remaining L item, got %d", sim.Remaining["L"])
	}
}

func TestSimulatorCalculatesDistributions(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 6.0},
		{ID: "3", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "4", Status: "To Do", Size: "M"},
	}

	sim := NewSimulator(items, 8.0)

	dist, ok := sim.CycleTimes["M"]
	if !ok {
		t.Fatal("expected distribution for size M")
	}

	// Mean should be (4+6+5)/3 = 5.0
	expectedMean := 5.0
	if dist.Mean != expectedMean {
		t.Errorf("expected mean %f, got %f", expectedMean, dist.Mean)
	}

	// StdDev should be non-zero
	if dist.StdDev <= 0 {
		t.Errorf("expected positive stddev, got %f", dist.StdDev)
	}
}

func TestSimulatorRun(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "3", Status: "To Do", Size: "M"},
		{ID: "4", Status: "To Do", Size: "M"},
	}

	sim := NewSimulator(items, 8.0)
	result := sim.Run(1000)

	if result == nil {
		t.Fatal("expected result from simulation")
	}
	if result.Remaining != 2 {
		t.Errorf("expected 2 remaining items, got %d", result.Remaining)
	}
	if len(result.Percentiles) != 4 {
		t.Errorf("expected 4 percentiles, got %d", len(result.Percentiles))
	}
}

func TestSimulatorPercentiles(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "3", Status: "To Do", Size: "M"},
	}

	sim := NewSimulator(items, 8.0)
	result := sim.Run(10000)

	// Percentiles should be in ascending order
	if result.Percentiles[50] > result.Percentiles[70] {
		t.Error("50th percentile should be <= 70th percentile")
	}
	if result.Percentiles[70] > result.Percentiles[85] {
		t.Error("70th percentile should be <= 85th percentile")
	}
	if result.Percentiles[85] > result.Percentiles[95] {
		t.Error("85th percentile should be <= 95th percentile")
	}
}

func TestSimulatorWithNoRemainingItems(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
	}

	sim := NewSimulator(items, 8.0)
	result := sim.Run(100)

	if result.Remaining != 0 {
		t.Errorf("expected 0 remaining items, got %d", result.Remaining)
	}

	// All percentiles should be 0 or very small
	for p, days := range result.Percentiles {
		if days != 0 {
			t.Errorf("expected 0 days for %dth percentile with no remaining work, got %f", p, days)
		}
	}
}

func TestSimulatorFallbackDistribution(t *testing.T) {
	// Items with no historical data for the size
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "To Do", Size: "XL"}, // No XL completed items
	}

	sim := NewSimulator(items, 8.0)

	// Should use fallback for XL
	_, hasXL := sim.CycleTimes["XL"]
	if hasXL {
		t.Error("should not have distribution for XL (no completed items)")
	}

	// Simulation should still work with fallback
	result := sim.Run(100)
	if result.Remaining != 1 {
		t.Errorf("expected 1 remaining item, got %d", result.Remaining)
	}
}

func TestCalculateThroughput(t *testing.T) {
	now := time.Now()
	items := []forecast.Item{
		{ID: "1", Status: "Done", Done: timePtr(now.AddDate(0, 0, -1))},  // Yesterday
		{ID: "2", Status: "Done", Done: timePtr(now.AddDate(0, 0, -2))},  // 2 days ago
		{ID: "3", Status: "Done", Done: timePtr(now.AddDate(0, 0, -7))},  // 7 days ago
		{ID: "4", Status: "Done", Done: timePtr(now.AddDate(0, 0, -20))}, // 20 days ago (outside window)
		{ID: "5", Status: "To Do"},
	}

	// In last 14 days, 3 items completed
	throughput := CalculateThroughput(items, 14)
	expected := 3.0 / 14.0

	if throughput != expected {
		t.Errorf("expected throughput %f, got %f", expected, throughput)
	}
}

func TestCalculateThroughputNoCompletedItems(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "To Do"},
		{ID: "2", Status: "In Progress"},
	}

	throughput := CalculateThroughput(items, 14)
	if throughput != 0 {
		t.Errorf("expected throughput 0, got %f", throughput)
	}
}

func TestCalculateAvgCycleTime(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
		{ID: "2", Status: "Done", CycleTime: 6.0},
		{ID: "3", Status: "Done", CycleTime: 5.0},
		{ID: "4", Status: "To Do", CycleTime: 0},
	}

	avg := CalculateAvgCycleTime(items)
	expected := (4.0 + 6.0 + 5.0) / 3.0

	if avg != expected {
		t.Errorf("expected avg cycle time %f, got %f", expected, avg)
	}
}

func TestCalculateAvgCycleTimeNoData(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 0}, // No cycle time data
		{ID: "2", Status: "To Do"},
	}

	avg := CalculateAvgCycleTime(items)
	if avg != 0 {
		t.Errorf("expected avg cycle time 0, got %f", avg)
	}
}

func TestSimulatorDeterminism(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "3", Status: "To Do", Size: "M"},
	}

	// Run multiple simulations - results should be similar but not identical
	sim1 := NewSimulator(items, 8.0)
	result1 := sim1.Run(10000)

	sim2 := NewSimulator(items, 8.0)
	result2 := sim2.Run(10000)

	// Results should be in the same ballpark (within 50% of each other)
	diff := result1.Percentiles[50] - result2.Percentiles[50]
	if diff < 0 {
		diff = -diff
	}

	avg := (result1.Percentiles[50] + result2.Percentiles[50]) / 2
	if avg > 0 && diff/avg > 0.5 {
		t.Errorf("simulation results too different: %f vs %f", result1.Percentiles[50], result2.Percentiles[50])
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
