package forecast

import (
	"testing"
	"time"
)

func TestItemCreation(t *testing.T) {
	now := time.Now()
	item := Item{
		ID:          "TEST-123",
		Type:        "Component",
		Description: "Test item",
		Size:        "M",
		Status:      "To Do",
		Created:     now,
		JiraKey:     "TEST-123",
	}

	if item.ID != "TEST-123" {
		t.Errorf("expected ID TEST-123, got %s", item.ID)
	}
	if item.Type != "Component" {
		t.Errorf("expected Type Component, got %s", item.Type)
	}
	if item.Size != "M" {
		t.Errorf("expected Size M, got %s", item.Size)
	}
	if item.Status != "To Do" {
		t.Errorf("expected Status To Do, got %s", item.Status)
	}
}

func TestItemStatusTransitions(t *testing.T) {
	now := time.Now()
	item := Item{
		ID:      "TEST-123",
		Status:  "To Do",
		Created: now,
	}

	// Transition to In Progress
	inProgressTime := now.Add(24 * time.Hour)
	item.Status = "In Progress"
	item.InProgress = &inProgressTime

	if item.InProgress == nil {
		t.Error("expected InProgress time to be set")
	}

	// Transition to Done
	doneTime := now.Add(48 * time.Hour)
	item.Status = "Done"
	item.Done = &doneTime
	item.CycleTime = doneTime.Sub(inProgressTime).Hours()

	if item.Done == nil {
		t.Error("expected Done time to be set")
	}
	if item.CycleTime != 24.0 {
		t.Errorf("expected CycleTime 24.0, got %f", item.CycleTime)
	}
}

func TestDistribution(t *testing.T) {
	dist := Distribution{
		Mean:   4.0,
		StdDev: 1.5,
	}

	if dist.Mean != 4.0 {
		t.Errorf("expected Mean 4.0, got %f", dist.Mean)
	}
	if dist.StdDev != 1.5 {
		t.Errorf("expected StdDev 1.5, got %f", dist.StdDev)
	}
}

func TestEVAMetrics(t *testing.T) {
	metrics := EVAMetrics{
		Week: 3,
		PV:   30,
		EV:   25,
		AC:   100.0,
		SV:   -5,
		CV:   -75.0,
		SPI:  0.833,
		CPI:  0.25,
	}

	if metrics.Week != 3 {
		t.Errorf("expected Week 3, got %d", metrics.Week)
	}
	if metrics.SV != -5 {
		t.Errorf("expected SV -5, got %d", metrics.SV)
	}
	if metrics.SPI != 0.833 {
		t.Errorf("expected SPI 0.833, got %f", metrics.SPI)
	}
}

func TestForecastResult(t *testing.T) {
	result := ForecastResult{
		Remaining:        10,
		Completed:        20,
		ThroughputPerDay: 2.5,
		AvgCycleTime:     4.0,
		Percentiles: map[int]float64{
			50: 5.0,
			70: 7.0,
			85: 10.0,
			95: 15.0,
		},
	}

	if result.Remaining != 10 {
		t.Errorf("expected Remaining 10, got %d", result.Remaining)
	}
	if result.Percentiles[50] != 5.0 {
		t.Errorf("expected 50th percentile 5.0, got %f", result.Percentiles[50])
	}
	if result.Percentiles[95] != 15.0 {
		t.Errorf("expected 95th percentile 15.0, got %f", result.Percentiles[95])
	}
}

func TestReferenceClass(t *testing.T) {
	now := time.Now()
	rc := ReferenceClass{
		ID:          1,
		Name:        "Test Project",
		Type:        "Web Application",
		CompletedAt: now,
		TeamSize:    5,
		Items: []ReferenceClassItem{
			{ProjectID: 1, ItemType: "Component", Size: "M", CycleTimeHrs: 4.0},
			{ProjectID: 1, ItemType: "Component", Size: "L", CycleTimeHrs: 8.0},
		},
	}

	if rc.Name != "Test Project" {
		t.Errorf("expected Name 'Test Project', got %s", rc.Name)
	}
	if rc.TeamSize != 5 {
		t.Errorf("expected TeamSize 5, got %d", rc.TeamSize)
	}
	if len(rc.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(rc.Items))
	}
}

func TestReferenceClassItem(t *testing.T) {
	item := ReferenceClassItem{
		ProjectID:    1,
		ItemType:     "Migration",
		Size:         "XL",
		CycleTimeHrs: 16.0,
	}

	if item.ItemType != "Migration" {
		t.Errorf("expected ItemType 'Migration', got %s", item.ItemType)
	}
	if item.Size != "XL" {
		t.Errorf("expected Size 'XL', got %s", item.Size)
	}
	if item.CycleTimeHrs != 16.0 {
		t.Errorf("expected CycleTimeHrs 16.0, got %f", item.CycleTimeHrs)
	}
}
