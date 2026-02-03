package main

import (
	"encoding/json"
	"fmt"
	"time"

	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/history"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var compareCmd = &cobra.Command{
	Use:   "compare <project1> <project2> [project3]",
	Short: "Compare projects side by side",
	Long: `Compares 2-3 projects showing progress, forecasts, and key metrics.

Useful for:
  - Comparing project health across teams
  - Identifying which projects need attention
  - Portfolio-level planning decisions

Examples:
  forecast compare proj1 proj2
  forecast compare wallet engine platform`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOutput, _ := cmd.Flags().GetBool("json")
		return runCompare(args, jsonOutput)
	},
}

func init() {
	compareCmd.Flags().Bool("json", false, "Output as JSON")

	rootCmd.AddCommand(compareCmd)
}

// CompareResult holds comparison data for JSON output
type CompareResult struct {
	Projects    []ProjectComparison `json:"projects"`
	Summary     ComparisonSummary   `json:"summary"`
	GeneratedAt time.Time           `json:"generatedAt"`
}

// ProjectComparison holds data for a single project in comparison
type ProjectComparison struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Total       int     `json:"total"`
	Done        int     `json:"done"`
	Remaining   int     `json:"remaining"`
	Progress    float64 `json:"progressPercent"`
	AvgCycle    float64 `json:"avgCycleHours"`
	Throughput  float64 `json:"throughputPerDay"`
	Forecast70  float64 `json:"forecast70Days"`
	Forecast85  float64 `json:"forecast85Days"`
	Forecast95  float64 `json:"forecast95Days"`
	TargetDate  string  `json:"targetDate85"`
	Health      string  `json:"health"` // "ahead", "on-track", "at-risk", "behind"
	TrendDays   float64 `json:"trendDays,omitempty"` // Change in 85% forecast over last 7 days
}

// ComparisonSummary holds summary insights
type ComparisonSummary struct {
	FastestProject   string `json:"fastestProject"`
	MostComplete     string `json:"mostComplete"`
	HighestRisk      string `json:"highestRisk,omitempty"`
	TotalRemaining   int    `json:"totalRemaining"`
	AverageProgress  float64 `json:"averageProgress"`
}

func runCompare(projectKeys []string, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	store := storage.New(".forecast")
	histStore := history.New(".forecast")
	now := time.Now()

	var comparisons []ProjectComparison

	for _, key := range projectKeys {
		proj := cfg.GetProject(key)
		if proj == nil {
			return apperrors.ProjectNotFoundError(key, nil)
		}

		projectKey := proj.Key
		if projectKey == "" {
			projectKey = proj.Epic
		}

		// Load items
		items, err := store.LoadProject(projectKey)
		if err != nil || len(items) == 0 {
			return apperrors.NoDataError(projectKey)
		}

		// Calculate metrics
		comp := calculateProjectComparison(items, proj, cfg.TeamCapacity)
		comp.Key = projectKey
		comp.Name = proj.Name

		// Get trend from history
		trend, _ := histStore.GetTrend(projectKey, 7)
		if len(trend) >= 2 {
			first := trend[0].Percentiles[85]
			last := trend[len(trend)-1].Percentiles[85]
			comp.TrendDays = last - first // Negative is good (forecast improving)
		}

		comparisons = append(comparisons, comp)
	}

	// Calculate summary
	summary := calculateSummary(comparisons)

	if jsonOutput {
		result := CompareResult{
			Projects:    comparisons,
			Summary:     summary,
			GeneratedAt: now,
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	printComparisonTable(comparisons, summary)
	return nil
}

func calculateProjectComparison(items []forecast.Item, proj interface{ }, defaultCapacity float64) ProjectComparison {
	var comp ProjectComparison

	// Count items
	var cycleTimeCount int
	for _, item := range items {
		switch item.Status {
		case "Done":
			comp.Total++
			comp.Done++
			if item.CycleTime > 0 {
				cycleTimeCount++
			}
		case "Canceled":
			// Skip
		default:
			comp.Total++
			comp.Remaining++
		}
	}

	if comp.Total > 0 {
		comp.Progress = float64(comp.Done) / float64(comp.Total) * 100
	}

	// Calculate metrics
	comp.AvgCycle = montecarlo.CalculateAvgCycleTime(items)
	comp.Throughput = montecarlo.CalculateThroughput(items, 14)

	// Run Monte Carlo if we have cycle time data
	if cycleTimeCount > 0 {
		capacity := defaultCapacity
		if capacity <= 0 {
			capacity = 8.0
		}

		simulator := montecarlo.NewSimulator(items, capacity)
		result := simulator.Run(5000) // Use fewer iterations for speed

		comp.Forecast70 = result.Percentiles[70]
		comp.Forecast85 = result.Percentiles[85]
		comp.Forecast95 = result.Percentiles[95]
		comp.TargetDate = time.Now().AddDate(0, 0, int(comp.Forecast85)).Format("Jan 2")
	}

	// Determine health
	comp.Health = determineProjectHealth(comp)

	return comp
}

func determineProjectHealth(comp ProjectComparison) string {
	// If nearly done, ahead
	if comp.Progress >= 90 {
		return "ahead"
	}

	// If good progress and reasonable forecast
	if comp.Progress >= 50 && comp.Forecast85 <= 60 {
		return "on-track"
	}

	// If low progress or long forecast
	if comp.Progress < 25 || comp.Forecast85 > 90 {
		return "behind"
	}

	return "at-risk"
}

func calculateSummary(comparisons []ProjectComparison) ComparisonSummary {
	var summary ComparisonSummary

	if len(comparisons) == 0 {
		return summary
	}

	// Find fastest (lowest 85% forecast)
	minForecast := comparisons[0].Forecast85
	summary.FastestProject = comparisons[0].Key

	// Find most complete
	maxProgress := comparisons[0].Progress
	summary.MostComplete = comparisons[0].Key

	// Find highest risk
	var highestRisk string
	for _, c := range comparisons {
		summary.TotalRemaining += c.Remaining
		summary.AverageProgress += c.Progress

		if c.Forecast85 < minForecast && c.Forecast85 > 0 {
			minForecast = c.Forecast85
			summary.FastestProject = c.Key
		}

		if c.Progress > maxProgress {
			maxProgress = c.Progress
			summary.MostComplete = c.Key
		}

		if c.Health == "behind" || c.Health == "at-risk" {
			if highestRisk == "" || c.Forecast85 > comparisons[0].Forecast85 {
				highestRisk = c.Key
			}
		}
	}

	summary.AverageProgress /= float64(len(comparisons))
	summary.HighestRisk = highestRisk

	return summary
}

func printComparisonTable(comparisons []ProjectComparison, summary ComparisonSummary) {
	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("  PROJECT COMPARISON")
	fmt.Println("================================================================================")

	// Header row
	fmt.Printf("\n%-20s", "Metric")
	for _, c := range comparisons {
		fmt.Printf(" %15s", truncate(c.Name, 15))
	}
	fmt.Println()

	fmt.Printf("%-20s", "--------------------")
	for range comparisons {
		fmt.Printf(" %15s", "---------------")
	}
	fmt.Println()

	// Progress
	fmt.Printf("%-20s", "Progress")
	for _, c := range comparisons {
		fmt.Printf(" %14.0f%%", c.Progress)
	}
	fmt.Println()

	// Done / Total
	fmt.Printf("%-20s", "Items (done/total)")
	for _, c := range comparisons {
		fmt.Printf(" %15s", fmt.Sprintf("%d/%d", c.Done, c.Total))
	}
	fmt.Println()

	// Remaining
	fmt.Printf("%-20s", "Remaining")
	for _, c := range comparisons {
		fmt.Printf(" %15d", c.Remaining)
	}
	fmt.Println()

	// Avg Cycle Time
	fmt.Printf("%-20s", "Avg Cycle (hrs)")
	for _, c := range comparisons {
		fmt.Printf(" %15.1f", c.AvgCycle)
	}
	fmt.Println()

	// Throughput
	fmt.Printf("%-20s", "Throughput (/day)")
	for _, c := range comparisons {
		fmt.Printf(" %15.2f", c.Throughput)
	}
	fmt.Println()

	// Forecasts
	fmt.Printf("\n%-20s", "--- Forecasts ---")
	for range comparisons {
		fmt.Printf(" %15s", "")
	}
	fmt.Println()

	fmt.Printf("%-20s", "70% confidence")
	for _, c := range comparisons {
		if c.Forecast70 > 0 {
			fmt.Printf(" %13.0f d", c.Forecast70)
		} else {
			fmt.Printf(" %15s", "-")
		}
	}
	fmt.Println()

	fmt.Printf("%-20s", "85% confidence")
	for _, c := range comparisons {
		if c.Forecast85 > 0 {
			fmt.Printf(" %13.0f d", c.Forecast85)
		} else {
			fmt.Printf(" %15s", "-")
		}
	}
	fmt.Println()

	fmt.Printf("%-20s", "95% confidence")
	for _, c := range comparisons {
		if c.Forecast95 > 0 {
			fmt.Printf(" %13.0f d", c.Forecast95)
		} else {
			fmt.Printf(" %15s", "-")
		}
	}
	fmt.Println()

	fmt.Printf("%-20s", "Target Date (85%)")
	for _, c := range comparisons {
		if c.TargetDate != "" {
			fmt.Printf(" %15s", c.TargetDate)
		} else {
			fmt.Printf(" %15s", "-")
		}
	}
	fmt.Println()

	// Health
	fmt.Printf("\n%-20s", "Health")
	for _, c := range comparisons {
		healthStr := c.Health
		// Add color codes
		switch c.Health {
		case "ahead":
			healthStr = "\033[32m" + healthStr + "\033[0m" // Green
		case "on-track":
			healthStr = "\033[34m" + healthStr + "\033[0m" // Blue
		case "at-risk":
			healthStr = "\033[33m" + healthStr + "\033[0m" // Yellow
		case "behind":
			healthStr = "\033[31m" + healthStr + "\033[0m" // Red
		}
		fmt.Printf(" %15s", healthStr)
	}
	fmt.Println()

	// Trend
	fmt.Printf("%-20s", "7-day trend")
	for _, c := range comparisons {
		if c.TrendDays != 0 {
			trendStr := fmt.Sprintf("%+.0fd", c.TrendDays)
			if c.TrendDays < 0 {
				trendStr = "\033[32m" + trendStr + "\033[0m" // Green (improving)
			} else if c.TrendDays > 0 {
				trendStr = "\033[31m" + trendStr + "\033[0m" // Red (slipping)
			}
			fmt.Printf(" %15s", trendStr)
		} else {
			fmt.Printf(" %15s", "-")
		}
	}
	fmt.Println()

	// Summary
	fmt.Println()
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Summary:")
	fmt.Printf("  Fastest to complete: %s\n", summary.FastestProject)
	fmt.Printf("  Most progress: %s (%.0f%%)\n", summary.MostComplete, summary.AverageProgress)
	if summary.HighestRisk != "" {
		fmt.Printf("  Needs attention: %s\n", summary.HighestRisk)
	}
	fmt.Printf("  Total remaining: %d items\n", summary.TotalRemaining)
	fmt.Println()
}
