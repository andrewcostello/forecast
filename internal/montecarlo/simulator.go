package montecarlo

import (
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/andrewcostello/forecast/pkg/forecast"
	"gonum.org/v1/gonum/stat"
)

// Simulator runs Monte Carlo simulations
type Simulator struct {
	Remaining     map[string]int                   // size → count
	CycleTimes    map[string]forecast.Distribution // size → distribution
	TeamCapacity  float64                          // hours per day
	Rand          *rand.Rand
}

// NewSimulator creates a new Monte Carlo simulator
func NewSimulator(items []forecast.Item, teamCapacity float64) *Simulator {
	s := &Simulator{
		Remaining:    make(map[string]int),
		CycleTimes:   make(map[string]forecast.Distribution),
		TeamCapacity: teamCapacity,
		Rand:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Count remaining items by size (exclude Done and Canceled)
	for _, item := range items {
		if item.Status != "Done" && item.Status != "Canceled" {
			s.Remaining[item.Size]++
		}
	}

	// Calculate cycle time distributions from completed items
	s.calculateDistributions(items)

	return s
}

func (s *Simulator) calculateDistributions(items []forecast.Item) {
	cycleTimes := make(map[string][]float64)

	for _, item := range items {
		if item.Status == "Done" && item.CycleTime > 0 {
			cycleTimes[item.Size] = append(cycleTimes[item.Size], item.CycleTime)
		}
	}

	for size, times := range cycleTimes {
		if len(times) == 0 {
			continue
		}

		mean := stat.Mean(times, nil)
		stddev := stat.StdDev(times, nil)

		s.CycleTimes[size] = forecast.Distribution{
			Mean:   mean,
			StdDev: stddev,
		}
	}
}

// Run executes Monte Carlo simulation
func (s *Simulator) Run(iterations int) *forecast.ForecastResult {
	results := make([]float64, iterations)

	for i := 0; i < iterations; i++ {
		results[i] = s.simulateOnce()
	}

	sort.Float64s(results)

	return &forecast.ForecastResult{
		Remaining:        s.totalRemaining(),
		Percentiles: map[int]float64{
			50: s.percentile(results, 50),
			70: s.percentile(results, 70),
			85: s.percentile(results, 85),
			95: s.percentile(results, 95),
		},
	}
}

func (s *Simulator) simulateOnce() float64 {
	totalHours := 0.0

	for size, count := range s.Remaining {
		dist, ok := s.CycleTimes[size]
		if !ok {
			// No historical data for this size, use default
			dist = s.estimateDistribution(size)
		}

		for i := 0; i < count; i++ {
			// Sample from normal distribution
			hours := s.Rand.NormFloat64()*dist.StdDev + dist.Mean

			// Ensure positive (minimum 0.5 hours)
			if hours < 0.5 {
				hours = 0.5
			}

			totalHours += hours
		}
	}

	// Convert hours to days
	days := totalHours / s.TeamCapacity
	return days
}

func (s *Simulator) estimateDistribution(size string) forecast.Distribution {
	// Fallback estimates if no historical data
	estimates := map[string]forecast.Distribution{
		"S":  {Mean: 1.5, StdDev: 0.5},
		"M":  {Mean: 4.0, StdDev: 1.5},
		"L":  {Mean: 8.0, StdDev: 2.5},
		"XL": {Mean: 16.0, StdDev: 5.0},
	}

	if dist, ok := estimates[size]; ok {
		return dist
	}

	return forecast.Distribution{Mean: 4.0, StdDev: 2.0}
}

func (s *Simulator) percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}

	index := int(math.Ceil(float64(len(sorted)) * float64(p) / 100.0))
	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func (s *Simulator) totalRemaining() int {
	total := 0
	for _, count := range s.Remaining {
		total += count
	}
	return total
}

// CalculateThroughput calculates items per day from recent history
func CalculateThroughput(items []forecast.Item, days int) float64 {
	cutoff := time.Now().AddDate(0, 0, -days)

	completed := 0
	for _, item := range items {
		if item.Done != nil && item.Done.After(cutoff) {
			completed++
		}
	}

	return float64(completed) / float64(days)
}

// CalculateAvgCycleTime calculates average cycle time from completed items
func CalculateAvgCycleTime(items []forecast.Item) float64 {
	var total float64
	count := 0

	for _, item := range items {
		if item.Status == "Done" && item.CycleTime > 0 {
			total += item.CycleTime
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return total / float64(count)
}
