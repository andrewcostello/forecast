package main

import (
	"encoding/json"
	"fmt"
	"time"

	appcontext "github.com/andrewcostello/forecast/internal/context"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/history"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View forecast history and trends",
	Long: `Shows historical forecast data for a project.
Tracks how forecasts have changed over time as work progresses.

Use --days to limit the time range shown.
Use --clear to remove all history for a project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		days, _ := cmd.Flags().GetInt("days")
		clear, _ := cmd.Flags().GetBool("clear")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		return runHistory(project, days, clear, jsonOutput)
	},
}

func init() {
	historyCmd.Flags().StringP("project", "p", "", "Project to show history for")
	historyCmd.Flags().Int("days", 30, "Number of days of history to show")
	historyCmd.Flags().Bool("clear", false, "Clear all history for the project")
	historyCmd.Flags().Bool("json", false, "Output results as JSON")

	rootCmd.AddCommand(historyCmd)
}

// HistoryResult holds history data for JSON output
type HistoryResult struct {
	ProjectKey string                `json:"projectKey"`
	Days       int                   `json:"days"`
	Entries    []HistoryEntryResult  `json:"entries"`
	Trend      *TrendAnalysis        `json:"trend,omitempty"`
}

// HistoryEntryResult holds a single history entry for JSON output
type HistoryEntryResult struct {
	Timestamp   time.Time       `json:"timestamp"`
	Total       int             `json:"total"`
	Completed   int             `json:"completed"`
	Remaining   int             `json:"remaining"`
	Progress    float64         `json:"progressPercent"`
	AvgCycle    float64         `json:"avgCycleHours"`
	Throughput  float64         `json:"throughputPerDay"`
	Forecast70  float64         `json:"forecast70Days,omitempty"`
	Forecast95  float64         `json:"forecast95Days,omitempty"`
}

// TrendAnalysis holds trend data
type TrendAnalysis struct {
	ProgressChange   float64 `json:"progressChangePercent"`
	ThroughputChange float64 `json:"throughputChangePercent"`
	ForecastChange   float64 `json:"forecast70ChangePercent"`
	Direction        string  `json:"direction"` // "improving", "stable", "declining"
}

func runHistory(projectFilter string, days int, clear, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	// Resolve project (use context if not specified)
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

	// Resolve project key
	var projectKey string
	projects := cfg.GetAllProjects()
	if len(projects) > 0 {
		if projectFilter == "" {
			if len(projects) == 1 {
				projectFilter = projects[0].Key
				if projectFilter == "" {
					projectFilter = projects[0].Epic
				}
			} else {
				available := make([]string, 0, len(projects))
				for _, p := range projects {
					if p.Key != "" {
						available = append(available, p.Key)
					}
				}
				return apperrors.NoProjectError(available)
			}
		}

		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			available := make([]string, 0, len(projects))
			for _, p := range projects {
				if p.Key != "" {
					available = append(available, p.Key)
				}
			}
			return apperrors.ProjectNotFoundError(projectFilter, available)
		}
		projectKey = proj.Key
		if projectKey == "" {
			projectKey = proj.Epic
		}
	}

	store := history.New(".forecast")

	// Handle clear command
	if clear {
		if err := store.Clear(projectKey); err != nil {
			return fmt.Errorf("failed to clear history: %w", err)
		}
		if !jsonOutput {
			fmt.Printf("Cleared history for %s\n", projectKey)
		}
		return nil
	}

	// Load history
	entries, err := store.GetTrend(projectKey, days)
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	if len(entries) == 0 {
		if jsonOutput {
			result := HistoryResult{
				ProjectKey: projectKey,
				Days:       days,
				Entries:    []HistoryEntryResult{},
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		return apperrors.InvalidArgumentError(
			fmt.Sprintf("No history found for project '%s'", projectKey),
			"Run 'forecast run' to generate forecasts and build history",
		)
	}

	// Build result
	result := HistoryResult{
		ProjectKey: projectKey,
		Days:       days,
		Entries:    make([]HistoryEntryResult, 0, len(entries)),
	}

	for _, e := range entries {
		entry := HistoryEntryResult{
			Timestamp:  e.Timestamp,
			Total:      e.Total,
			Completed:  e.Completed,
			Remaining:  e.Remaining,
			AvgCycle:   e.AvgCycle,
			Throughput: e.Throughput,
		}
		if e.Total > 0 {
			entry.Progress = float64(e.Completed) / float64(e.Total) * 100
		}
		if v, ok := e.Percentiles[70]; ok {
			entry.Forecast70 = v
		}
		if v, ok := e.Percentiles[95]; ok {
			entry.Forecast95 = v
		}
		result.Entries = append(result.Entries, entry)
	}

	// Calculate trend if we have enough data
	if len(entries) >= 2 {
		first := entries[0]
		last := entries[len(entries)-1]

		trend := &TrendAnalysis{}

		// Progress change
		firstProgress := float64(0)
		lastProgress := float64(0)
		if first.Total > 0 {
			firstProgress = float64(first.Completed) / float64(first.Total) * 100
		}
		if last.Total > 0 {
			lastProgress = float64(last.Completed) / float64(last.Total) * 100
		}
		trend.ProgressChange = lastProgress - firstProgress

		// Throughput change
		if first.Throughput > 0 {
			trend.ThroughputChange = ((last.Throughput - first.Throughput) / first.Throughput) * 100
		}

		// Forecast change (negative is better - fewer days remaining)
		firstF70 := first.Percentiles[70]
		lastF70 := last.Percentiles[70]
		if firstF70 > 0 {
			trend.ForecastChange = ((lastF70 - firstF70) / firstF70) * 100
		}

		// Determine direction
		if trend.ProgressChange > 5 && trend.ForecastChange < -5 {
			trend.Direction = "improving"
		} else if trend.ProgressChange < -5 || trend.ForecastChange > 10 {
			trend.Direction = "declining"
		} else {
			trend.Direction = "stable"
		}

		result.Trend = trend
	}

	// JSON output
	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Printf("\n=== Forecast History: %s (last %d days) ===\n\n", projectKey, days)

	fmt.Printf("%-20s %8s %8s %8s %10s %10s\n",
		"Date", "Done", "Total", "Progress", "70% Fcst", "Throughput")
	fmt.Println("------------------------------------------------------------------------")

	for _, e := range result.Entries {
		fmt.Printf("%-20s %8d %8d %7.1f%% %9.0f d %8.2f/d\n",
			e.Timestamp.Format("Jan 02 15:04"),
			e.Completed,
			e.Total,
			e.Progress,
			e.Forecast70,
			e.Throughput,
		)
	}

	// Show trend analysis
	if result.Trend != nil {
		fmt.Println("\n--- Trend Analysis ---")

		progressDir := "+"
		if result.Trend.ProgressChange < 0 {
			progressDir = ""
		}
		fmt.Printf("  Progress:   %s%.1f%% (%s)\n", progressDir, result.Trend.ProgressChange, result.Trend.Direction)

		throughputDir := "+"
		if result.Trend.ThroughputChange < 0 {
			throughputDir = ""
		}
		fmt.Printf("  Throughput: %s%.1f%%\n", throughputDir, result.Trend.ThroughputChange)

		forecastDir := "+"
		if result.Trend.ForecastChange < 0 {
			forecastDir = ""
		}
		fmt.Printf("  Forecast:   %s%.1f%% (negative is better)\n", forecastDir, result.Trend.ForecastChange)
	}

	fmt.Println()
	return nil
}
