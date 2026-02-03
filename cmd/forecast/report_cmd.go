package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	appcontext "github.com/andrewcostello/forecast/internal/context"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/report"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate forecast report",
	Long: `Generates a detailed report with:
  - Earned Value Analysis metrics
  - Monte Carlo forecast results
  - Throughput trends
  - Reference class comparisons

Use --project to generate report for a specific project.
If no project specified, uses current context (set with 'forecast use').`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reportType, _ := cmd.Flags().GetString("type")
		project, _ := cmd.Flags().GetString("project")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		return runReport(reportType, project, jsonOutput)
	},
}

func init() {
	reportCmd.Flags().String("type", "eva", "Report type: eva, montecarlo, full")
	reportCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")
	reportCmd.Flags().Bool("json", false, "Output results as JSON")
}

// ReportResult holds the report data for JSON output
type ReportResult struct {
	ProjectKey  string      `json:"projectKey"`
	ProjectName string      `json:"projectName"`
	ReportType  string      `json:"reportType"`
	GeneratedAt time.Time   `json:"generatedAt"`
	EVA         *EVAMetrics `json:"eva,omitempty"`
	Forecast    *ForecastMetrics `json:"forecast,omitempty"`
}

// EVAMetrics holds Earned Value Analysis data
type EVAMetrics struct {
	Total         int     `json:"total"`
	Completed     int     `json:"completed"`
	Remaining     int     `json:"remaining"`
	PercentDone   float64 `json:"percentDone"`
	AvgCycleHours float64 `json:"avgCycleHours"`
	ThroughputDay float64 `json:"throughputPerDay"`
}

// ForecastMetrics holds Monte Carlo forecast data
type ForecastMetrics struct {
	Iterations  int                  `json:"iterations"`
	Remaining   int                  `json:"remaining"`
	Percentiles map[int]PercentileResult `json:"percentiles"`
}

func runReport(reportType string, projectFilter string, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	store := storage.New(".forecast")

	// Resolve project (use context if not specified)
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

	// Determine which project to report on
	var projectKey string
	projectName := cfg.ProjectName
	if projectName == "" {
		projectName = "Project"
	}
	teamCapacity := cfg.TeamCapacity
	if teamCapacity <= 0 {
		teamCapacity = 8.0
	}

	projects := cfg.GetAllProjects()
	if len(projects) > 0 {
		// Multi-project mode
		if projectFilter == "" {
			// If only one project, use it
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
		projectName = proj.Name
		if proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
	}

	// Load items
	var items []forecast.Item
	if projectKey != "" {
		items, err = store.LoadProject(projectKey)
	} else {
		items, err = store.Load()
	}
	if err != nil {
		return apperrors.NoDataError(projectKey)
	}

	if len(items) == 0 {
		return apperrors.NoDataError(projectKey)
	}

	// Estimate project start from earliest item
	var startDate time.Time
	for _, item := range items {
		if !item.Created.IsZero() {
			if startDate.IsZero() || item.Created.Before(startDate) {
				startDate = item.Created
			}
		}
	}

	// JSON output mode
	if jsonOutput {
		return generateJSONReport(items, projectKey, projectName, teamCapacity, reportType)
	}

	// Human-readable output
	gen := report.NewGenerator(items, teamCapacity, projectName, startDate)

	switch reportType {
	case "eva":
		return gen.GenerateEVA(os.Stdout)
	case "montecarlo", "mc":
		return gen.GenerateMonteCarlo(os.Stdout, 10000)
	case "full":
		return gen.GenerateFull(os.Stdout, 10000)
	default:
		return apperrors.InvalidArgumentError(
			fmt.Sprintf("unknown report type: %s", reportType),
			"Valid types: eva, montecarlo, full",
		)
	}
}

func generateJSONReport(items []forecast.Item, projectKey, projectName string, teamCapacity float64, reportType string) error {
	result := ReportResult{
		ProjectKey:  projectKey,
		ProjectName: projectName,
		ReportType:  reportType,
		GeneratedAt: time.Now(),
	}

	// Count items
	var completed, remaining int
	var cycleTimeCount int

	for _, item := range items {
		if item.Status == "Done" {
			completed++
			if item.CycleTime > 0 {
				cycleTimeCount++
			}
		} else if item.Status != "Canceled" {
			remaining++
		}
	}

	// EVA metrics (always included)
	avgCycleTime := montecarlo.CalculateAvgCycleTime(items)
	throughput := montecarlo.CalculateThroughput(items, 14)

	result.EVA = &EVAMetrics{
		Total:         completed + remaining,
		Completed:     completed,
		Remaining:     remaining,
		PercentDone:   float64(completed) / float64(completed+remaining) * 100,
		AvgCycleHours: avgCycleTime,
		ThroughputDay: throughput,
	}

	// Monte Carlo forecast (if requested and we have cycle time data)
	if (reportType == "montecarlo" || reportType == "mc" || reportType == "full") && cycleTimeCount > 0 {
		simulator := montecarlo.NewSimulator(items, teamCapacity)
		mcResult := simulator.Run(10000)

		result.Forecast = &ForecastMetrics{
			Iterations:  10000,
			Remaining:   remaining,
			Percentiles: make(map[int]PercentileResult),
		}

		now := time.Now()
		for _, p := range []int{50, 70, 85, 95} {
			days := mcResult.Percentiles[p]
			result.Forecast.Percentiles[p] = PercentileResult{
				Days:       days,
				TargetDate: now.AddDate(0, 0, int(days)),
			}
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return nil
}
