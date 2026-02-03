package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

func TestNewGenerator(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
	}
	now := time.Now()

	gen := NewGenerator(items, 8.0, "Test Project", now)

	if gen == nil {
		t.Fatal("expected generator to be created")
	}
	if gen.ProjectName != "Test Project" {
		t.Errorf("expected project name 'Test Project', got %s", gen.ProjectName)
	}
	if gen.TeamCapacity != 8.0 {
		t.Errorf("expected team capacity 8.0, got %f", gen.TeamCapacity)
	}
	if len(gen.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(gen.Items))
	}
}

func TestCalculateStats(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
		{ID: "2", Status: "Done", CycleTime: 6.0},
		{ID: "3", Status: "In Progress"},
		{ID: "4", Status: "To Do"},
		{ID: "5", Status: "To Do"},
	}

	gen := NewGenerator(items, 8.0, "Test", time.Now())
	stats := gen.calculateStats()

	if stats.Total != 5 {
		t.Errorf("expected total 5, got %d", stats.Total)
	}
	if stats.Completed != 2 {
		t.Errorf("expected completed 2, got %d", stats.Completed)
	}
	if stats.InProgress != 1 {
		t.Errorf("expected in progress 1, got %d", stats.InProgress)
	}
	if stats.ToDo != 2 {
		t.Errorf("expected to do 2, got %d", stats.ToDo)
	}
	if stats.CycleTimeCount != 2 {
		t.Errorf("expected cycle time count 2, got %d", stats.CycleTimeCount)
	}

	expectedAvg := 5.0 // (4+6)/2
	if stats.AvgCycleTime != expectedAvg {
		t.Errorf("expected avg cycle time %f, got %f", expectedAvg, stats.AvgCycleTime)
	}

	expectedPercent := 40.0 // 2/5 * 100
	if stats.PercentComplete != expectedPercent {
		t.Errorf("expected percent complete %f, got %f", expectedPercent, stats.PercentComplete)
	}
}

func TestCalculateSizeStats(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "S", CycleTime: 2.0},
		{ID: "2", Status: "Done", Size: "S", CycleTime: 3.0},
		{ID: "3", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "4", Status: "Done", Size: "L", CycleTime: 10.0},
		{ID: "5", Status: "To Do", Size: "M"}, // Not counted
	}

	gen := NewGenerator(items, 8.0, "Test", time.Now())
	sizeStats := gen.calculateSizeStats()

	if sizeStats["S"].Count != 2 {
		t.Errorf("expected S count 2, got %d", sizeStats["S"].Count)
	}
	if sizeStats["S"].AvgHours != 2.5 {
		t.Errorf("expected S avg 2.5, got %f", sizeStats["S"].AvgHours)
	}
	if sizeStats["M"].Count != 1 {
		t.Errorf("expected M count 1, got %d", sizeStats["M"].Count)
	}
	if sizeStats["L"].Count != 1 {
		t.Errorf("expected L count 1, got %d", sizeStats["L"].Count)
	}
}

func TestCalculateTypeStats(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Type: "Component", Status: "Done"},
		{ID: "2", Type: "Component", Status: "Done"},
		{ID: "3", Type: "Component", Status: "To Do"},
		{ID: "4", Type: "Fix", Status: "Done"},
		{ID: "5", Type: "Fix", Status: "In Progress"},
	}

	gen := NewGenerator(items, 8.0, "Test", time.Now())
	typeStats := gen.calculateTypeStats()

	if typeStats["Component"].Total != 3 {
		t.Errorf("expected Component total 3, got %d", typeStats["Component"].Total)
	}
	if typeStats["Component"].Done != 2 {
		t.Errorf("expected Component done 2, got %d", typeStats["Component"].Done)
	}
	if typeStats["Component"].Remaining != 1 {
		t.Errorf("expected Component remaining 1, got %d", typeStats["Component"].Remaining)
	}
	if typeStats["Fix"].Total != 2 {
		t.Errorf("expected Fix total 2, got %d", typeStats["Fix"].Total)
	}
}

func TestGenerateEVA(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
		{ID: "2", Status: "Done", CycleTime: 5.0},
		{ID: "3", Status: "To Do"},
	}

	startDate := time.Now().AddDate(0, 0, -7)
	gen := NewGenerator(items, 8.0, "EVA Test Project", startDate)

	var buf bytes.Buffer
	err := gen.GenerateEVA(&buf)
	if err != nil {
		t.Fatalf("failed to generate EVA report: %v", err)
	}

	output := buf.String()

	// Check for expected content
	if !strings.Contains(output, "Earned Value Analysis Report") {
		t.Error("expected 'Earned Value Analysis Report' header")
	}
	if !strings.Contains(output, "EVA Test Project") {
		t.Error("expected project name in output")
	}
	if !strings.Contains(output, "Total Items") {
		t.Error("expected 'Total Items' in output")
	}
	if !strings.Contains(output, "Completed") {
		t.Error("expected 'Completed' in output")
	}
}

func TestGenerateMonteCarlo(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "Done", Size: "M", CycleTime: 4.0},
		{ID: "2", Status: "Done", Size: "M", CycleTime: 5.0},
		{ID: "3", Status: "To Do", Size: "M"},
	}

	gen := NewGenerator(items, 8.0, "MC Test Project", time.Now())

	var buf bytes.Buffer
	err := gen.GenerateMonteCarlo(&buf, 1000)
	if err != nil {
		t.Fatalf("failed to generate Monte Carlo report: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Monte Carlo Forecast Report") {
		t.Error("expected 'Monte Carlo Forecast Report' header")
	}
	if !strings.Contains(output, "MC Test Project") {
		t.Error("expected project name in output")
	}
	if !strings.Contains(output, "Probabilistic Forecast") {
		t.Error("expected 'Probabilistic Forecast' section")
	}
	if !strings.Contains(output, "50%") {
		t.Error("expected 50% confidence level")
	}
	if !strings.Contains(output, "95%") {
		t.Error("expected 95% confidence level")
	}
}

func TestGenerateMonteCarloNoData(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Status: "To Do"},
		{ID: "2", Status: "In Progress"},
	}

	gen := NewGenerator(items, 8.0, "Test", time.Now())

	var buf bytes.Buffer
	err := gen.GenerateMonteCarlo(&buf, 1000)

	if err == nil {
		t.Error("expected error when no cycle time data available")
	}
}

func TestGenerateFull(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Type: "Component", Size: "M", Status: "Done", CycleTime: 4.0},
		{ID: "2", Type: "Component", Size: "L", Status: "Done", CycleTime: 8.0},
		{ID: "3", Type: "Fix", Size: "S", Status: "Done", CycleTime: 2.0},
		{ID: "4", Type: "Component", Size: "M", Status: "To Do"},
	}

	gen := NewGenerator(items, 8.0, "Full Report Project", time.Now())

	var buf bytes.Buffer
	err := gen.GenerateFull(&buf, 1000)
	if err != nil {
		t.Fatalf("failed to generate full report: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Project Forecast Report") {
		t.Error("expected 'Project Forecast Report' header")
	}
	if !strings.Contains(output, "Project Overview") {
		t.Error("expected 'Project Overview' section")
	}
	if !strings.Contains(output, "Cycle Time Analysis") {
		t.Error("expected 'Cycle Time Analysis' section")
	}
	if !strings.Contains(output, "Monte Carlo Forecast") {
		t.Error("expected 'Monte Carlo Forecast' section")
	}
	if !strings.Contains(output, "Item Breakdown by Type") {
		t.Error("expected 'Item Breakdown by Type' section")
	}
}

func TestInterpretSV(t *testing.T) {
	tests := []struct {
		sv       int
		expected string
	}{
		{5, "Ahead of schedule"},
		{-5, "Behind schedule"},
		{0, "On schedule"},
	}

	for _, tt := range tests {
		result := interpretSV(tt.sv)
		if result != tt.expected {
			t.Errorf("interpretSV(%d) = %s, expected %s", tt.sv, result, tt.expected)
		}
	}
}

func TestInterpretSPI(t *testing.T) {
	tests := []struct {
		spi      float64
		expected string
	}{
		{1.2, "On or ahead of schedule"},
		{1.0, "On or ahead of schedule"},
		{0.95, "Slightly behind schedule"},
		{0.85, "Behind schedule - attention needed"},
		{0.5, "Significantly behind schedule"},
	}

	for _, tt := range tests {
		result := interpretSPI(tt.spi)
		if result != tt.expected {
			t.Errorf("interpretSPI(%f) = %s, expected %s", tt.spi, result, tt.expected)
		}
	}
}

func TestFormatTable(t *testing.T) {
	headers := []string{"Name", "Value"}
	rows := [][]string{
		{"Item 1", "100"},
		{"Item 2", "200"},
	}

	result := FormatTable(headers, rows)

	if !strings.Contains(result, "Name") {
		t.Error("expected 'Name' header in table")
	}
	if !strings.Contains(result, "Item 1") {
		t.Error("expected 'Item 1' in table")
	}
	if !strings.Contains(result, "200") {
		t.Error("expected '200' in table")
	}
}

func TestFormatTableEmpty(t *testing.T) {
	result := FormatTable([]string{}, [][]string{})
	if result != "" {
		t.Error("expected empty string for empty table")
	}
}

func TestUntypedItems(t *testing.T) {
	items := []forecast.Item{
		{ID: "1", Type: "", Status: "Done"},
		{ID: "2", Type: "", Status: "To Do"},
	}

	gen := NewGenerator(items, 8.0, "Test", time.Now())
	typeStats := gen.calculateTypeStats()

	if _, ok := typeStats["Untyped"]; !ok {
		t.Error("expected 'Untyped' category for items without type")
	}
	if typeStats["Untyped"].Total != 2 {
		t.Errorf("expected 2 untyped items, got %d", typeStats["Untyped"].Total)
	}
}
