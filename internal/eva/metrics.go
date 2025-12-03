package eva

import (
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
)

// Calculator calculates Earned Value Analysis metrics
type Calculator struct {
	PlannedSchedule map[int]int // week → planned items complete
	StartDate       time.Time
}

// NewCalculator creates a new EVA calculator
func NewCalculator(totalItems int, durationWeeks int, startDate time.Time) *Calculator {
	schedule := make(map[int]int)

	// Linear distribution of work across weeks
	itemsPerWeek := float64(totalItems) / float64(durationWeeks)
	cumulative := 0.0

	for week := 1; week <= durationWeeks; week++ {
		cumulative += itemsPerWeek
		schedule[week] = int(cumulative)
	}

	return &Calculator{
		PlannedSchedule: schedule,
		StartDate:       startDate,
	}
}

// Calculate computes EVA metrics for current state
func (c *Calculator) Calculate(items []forecast.Item) *forecast.EVAMetrics {
	currentWeek := c.getCurrentWeek()

	pv := c.PlannedSchedule[currentWeek]
	ev := c.countCompleted(items)
	ac := c.calculateActualCost(items)

	sv := ev - pv
	cv := float64(ev) - ac

	spi := 0.0
	if pv > 0 {
		spi = float64(ev) / float64(pv)
	}

	cpi := 0.0
	if ac > 0 {
		cpi = float64(ev) / ac
	}

	return &forecast.EVAMetrics{
		Week: currentWeek,
		PV:   pv,
		EV:   ev,
		AC:   ac,
		SV:   sv,
		CV:   cv,
		SPI:  spi,
		CPI:  cpi,
	}
}

func (c *Calculator) getCurrentWeek() int {
	duration := time.Since(c.StartDate)
	weeks := int(duration.Hours() / (24 * 7))
	return weeks + 1
}

func (c *Calculator) countCompleted(items []forecast.Item) int {
	count := 0
	for _, item := range items {
		if item.Status == "Done" {
			count++
		}
	}
	return count
}

func (c *Calculator) calculateActualCost(items []forecast.Item) float64 {
	total := 0.0
	for _, item := range items {
		if item.CycleTime > 0 {
			total += item.CycleTime
		}
	}
	return total
}

// CalculateEarnedSchedule calculates Earned Schedule (ES)
func (c *Calculator) CalculateEarnedSchedule(ev int) float64 {
	// Find the week where PV = current EV
	for week, pv := range c.PlannedSchedule {
		if pv >= ev {
			if week == 1 {
				return float64(week)
			}

			// Interpolate between weeks
			prevWeek := week - 1
			prevPV := c.PlannedSchedule[prevWeek]

			fraction := float64(ev-prevPV) / float64(pv-prevPV)
			return float64(prevWeek) + fraction
		}
	}

	// EV exceeds all planned values
	maxWeek := 0
	for week := range c.PlannedSchedule {
		if week > maxWeek {
			maxWeek = week
		}
	}
	return float64(maxWeek)
}

// ForecastCompletion forecasts completion week based on current performance
func (c *Calculator) ForecastCompletion(metrics *forecast.EVAMetrics, totalItems int) int {
	if metrics.SPI == 0 {
		return 0
	}

	maxWeek := 0
	for week := range c.PlannedSchedule {
		if week > maxWeek {
			maxWeek = week
		}
	}

	plannedDuration := float64(maxWeek)
	forecastDuration := plannedDuration / metrics.SPI

	return int(forecastDuration)
}
