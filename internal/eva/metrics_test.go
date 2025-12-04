package eva

import (
	"testing"
	"time"

	"bitbucket.org/supermoneygames/forecast/pkg/forecast"
)

func TestNewCalculator(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -30) // 30 days ago
	calc := NewCalculator(100, 10, startDate)

	if calc == nil {
		t.Fatal("expected calculator to be created")
	}
	if len(calc.PlannedSchedule) != 10 {
		t.Errorf("expected 10 weeks in schedule, got %d", len(calc.PlannedSchedule))
	}
	if calc.StartDate != startDate {
		t.Errorf("expected start date to match")
	}
}

func TestCalculatorLinearSchedule(t *testing.T) {
	startDate := time.Now()
	calc := NewCalculator(100, 10, startDate)

	// Linear distribution should have 10 items per week
	expectedPerWeek := 10
	for week := 1; week <= 10; week++ {
		expected := expectedPerWeek * week
		if calc.PlannedSchedule[week] != expected {
			t.Errorf("week %d: expected %d planned items, got %d", week, expected, calc.PlannedSchedule[week])
		}
	}
}

func TestCalculateWithNoItems(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -7) // 1 week ago
	calc := NewCalculator(50, 5, startDate)

	items := []forecast.Item{}
	metrics := calc.Calculate(items)

	if metrics.EV != 0 {
		t.Errorf("expected EV 0 for empty items, got %d", metrics.EV)
	}
	if metrics.AC != 0 {
		t.Errorf("expected AC 0 for empty items, got %f", metrics.AC)
	}
}

func TestCalculateWithCompletedItems(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -14) // 2 weeks ago
	calc := NewCalculator(50, 5, startDate)

	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
		{ID: "2", Status: "Done", CycleTime: 6.0},
		{ID: "3", Status: "Done", CycleTime: 5.0},
		{ID: "4", Status: "In Progress", CycleTime: 0},
		{ID: "5", Status: "To Do", CycleTime: 0},
	}

	metrics := calc.Calculate(items)

	if metrics.EV != 3 {
		t.Errorf("expected EV 3 (completed items), got %d", metrics.EV)
	}
	if metrics.AC != 15.0 {
		t.Errorf("expected AC 15.0 (sum of cycle times), got %f", metrics.AC)
	}
}

func TestSchedulePerformanceIndex(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -7) // 1 week ago
	calc := NewCalculator(10, 10, startDate)

	// After 1 week, PV should be 1 item
	// If we completed 2 items, SPI = 2/1 = 2.0
	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
		{ID: "2", Status: "Done", CycleTime: 4.0},
	}

	metrics := calc.Calculate(items)

	if metrics.EV != 2 {
		t.Errorf("expected EV 2, got %d", metrics.EV)
	}
	// SPI = EV/PV
	if metrics.PV > 0 && metrics.SPI != float64(metrics.EV)/float64(metrics.PV) {
		t.Errorf("SPI calculation mismatch")
	}
}

func TestCostPerformanceIndex(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -7)
	calc := NewCalculator(10, 10, startDate)

	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 8.0},
		{ID: "2", Status: "Done", CycleTime: 8.0},
	}

	metrics := calc.Calculate(items)

	// CPI = EV/AC = 2/16 = 0.125
	expectedCPI := float64(metrics.EV) / metrics.AC
	if metrics.CPI != expectedCPI {
		t.Errorf("expected CPI %f, got %f", expectedCPI, metrics.CPI)
	}
}

func TestScheduleVariance(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -7)
	calc := NewCalculator(10, 10, startDate)

	items := []forecast.Item{
		{ID: "1", Status: "Done", CycleTime: 4.0},
	}

	metrics := calc.Calculate(items)

	// SV = EV - PV
	expectedSV := metrics.EV - metrics.PV
	if metrics.SV != expectedSV {
		t.Errorf("expected SV %d, got %d", expectedSV, metrics.SV)
	}
}

func TestCalculateEarnedSchedule(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -14)
	calc := NewCalculator(100, 10, startDate)

	// If EV = 10 (week 1's planned value)
	es := calc.CalculateEarnedSchedule(10)
	if es != 1.0 {
		t.Errorf("expected ES 1.0 for EV=10, got %f", es)
	}

	// If EV = 20 (week 2's planned value)
	es = calc.CalculateEarnedSchedule(20)
	if es != 2.0 {
		t.Errorf("expected ES 2.0 for EV=20, got %f", es)
	}
}

func TestForecastCompletion(t *testing.T) {
	startDate := time.Now().AddDate(0, 0, -7)
	calc := NewCalculator(100, 10, startDate)

	// With SPI = 1.0, forecast should equal planned duration
	metrics := &forecast.EVAMetrics{SPI: 1.0}
	forecast := calc.ForecastCompletion(metrics, 100)
	if forecast != 10 {
		t.Errorf("expected forecast 10 weeks with SPI=1.0, got %d", forecast)
	}

	// With SPI = 0.5, forecast should be double
	metrics.SPI = 0.5
	forecast = calc.ForecastCompletion(metrics, 100)
	if forecast != 20 {
		t.Errorf("expected forecast 20 weeks with SPI=0.5, got %d", forecast)
	}

	// With SPI = 2.0, forecast should be half
	metrics.SPI = 2.0
	forecast = calc.ForecastCompletion(metrics, 100)
	if forecast != 5 {
		t.Errorf("expected forecast 5 weeks with SPI=2.0, got %d", forecast)
	}
}

func TestForecastCompletionWithZeroSPI(t *testing.T) {
	startDate := time.Now()
	calc := NewCalculator(100, 10, startDate)

	metrics := &forecast.EVAMetrics{SPI: 0}
	forecast := calc.ForecastCompletion(metrics, 100)

	if forecast != 0 {
		t.Errorf("expected forecast 0 with SPI=0, got %d", forecast)
	}
}
