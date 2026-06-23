package main

import (
	"encoding/json"
	"fmt"
	"time"

	appcontext "github.com/andrewcostello/forecast/internal/context"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/history"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Monte Carlo simulation",
	Long: `Executes Monte Carlo simulation to forecast project completion.
Uses actual cycle time data to predict completion dates with confidence levels.

Use --project to run forecast for a specific project.
Use --all to run forecasts for all configured projects.
If no project specified, uses current context (set with 'forecast use').`,
	RunE: func(cmd *cobra.Command, args []string) error {
		confidence, _ := cmd.Flags().GetIntSlice("confidence")
		iterations, _ := cmd.Flags().GetInt("iterations")
		project, _ := cmd.Flags().GetString("project")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		quiet, _ := cmd.Flags().GetBool("quiet")
		all, _ := cmd.Flags().GetBool("all")

		if all {
			return runMonteCarloAll(confidence, iterations, jsonOutput, quiet)
		}
		return runMonteCarlo(confidence, iterations, project, jsonOutput, quiet)
	},
}

func init() {
	runCmd.Flags().IntSlice("confidence", []int{50, 70, 85, 95}, "Confidence levels for forecast")
	runCmd.Flags().Int("iterations", 10000, "Number of Monte Carlo iterations")
	runCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")
	runCmd.Flags().Bool("json", false, "Output results as JSON")
	runCmd.Flags().Bool("quiet", false, "Suppress output (exit code only)")
	runCmd.Flags().Bool("all", false, "Run forecast for all configured projects")
}

// RunResult holds the forecast result for JSON output
type RunResult struct {
	ProjectKey  string               `json:"projectKey"`
	ProjectName string               `json:"projectName"`
	Total       int                  `json:"total"`
	Completed   int                  `json:"completed"`
	Remaining   int                  `json:"remaining"`
	AvgCycle    float64              `json:"avgCycleHours"`
	Throughput  float64              `json:"throughputPerDay"`
	Percentiles map[int]PercentileResult `json:"percentiles"`
	GeneratedAt time.Time            `json:"generatedAt"`
}

// PercentileResult holds a single percentile forecast
type PercentileResult struct {
	Days       float64   `json:"days"`
	TargetDate time.Time `json:"targetDate"`
}

func runMonteCarlo(confidence []int, iterations int, projectFilter string, jsonOutput, quiet bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	store := storage.New(".forecast")

	// Resolve project (use context if not specified)
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

	// Determine which project to forecast
	var projectKey string
	var projectName string
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

	if !jsonOutput && !quiet {
		fmt.Printf("Running Monte Carlo simulation (%d iterations)...\n", iterations)
		if projectName != "" {
			fmt.Printf("Project: %s\n", projectName)
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
		return apperrors.NoDataError(projectKey, cfg.Source.Type)
	}

	if len(items) == 0 {
		return apperrors.NoDataError(projectKey, cfg.Source.Type)
	}

	// Count completed items with cycle time data
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

	if cycleTimeCount == 0 {
		return apperrors.NoCycleTimeError(cfg.Source.Type)
	}

	// Create and run Monte Carlo simulator
	simulator := montecarlo.NewSimulator(items, teamCapacity)
	result := simulator.Run(iterations)

	// Calculate additional metrics
	avgCycleTime := montecarlo.CalculateAvgCycleTime(items)
	throughput := montecarlo.CalculateThroughput(items, 14)

	now := time.Now()

	// Save to history (silently, don't fail on error)
	histStore := history.New(".forecast")
	histEntry := history.Entry{
		Timestamp:   now,
		ProjectKey:  projectKey,
		Total:       completed + remaining,
		Completed:   completed,
		Remaining:   remaining,
		AvgCycle:    avgCycleTime,
		Throughput:  throughput,
		Percentiles: make(map[int]float64),
	}
	for _, p := range confidence {
		histEntry.Percentiles[p] = result.Percentiles[p]
	}
	_ = histStore.Save(histEntry) // Ignore errors for history

	if jsonOutput {
		output := RunResult{
			ProjectKey:  projectKey,
			ProjectName: projectName,
			Total:       completed + remaining,
			Completed:   completed,
			Remaining:   remaining,
			AvgCycle:    avgCycleTime,
			Throughput:  throughput,
			Percentiles: make(map[int]PercentileResult),
			GeneratedAt: now,
		}

		for _, p := range confidence {
			days := result.Percentiles[p]
			output.Percentiles[p] = PercentileResult{
				Days:       days,
				TargetDate: now.AddDate(0, 0, int(days)),
			}
		}

		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if quiet {
		return nil
	}

	// Human-readable output
	total := completed + remaining
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\nRemaining: %d items (%.1f%%)\n", remaining, float64(remaining)/float64(total)*100)
	fmt.Printf("Completed: %d items\n", completed)
	fmt.Printf("Avg Cycle Time: %.1f hours\n", avgCycleTime)
	fmt.Printf("Throughput (14d): %.2f items/day\n", throughput)

	fmt.Printf("\nForecast (days to complete remaining work):\n")
	for _, p := range confidence {
		days := result.Percentiles[p]
		targetDate := now.AddDate(0, 0, int(days))
		fmt.Printf("  %d%% confidence: %.0f days (%s)\n", p, days, targetDate.Format("Jan 2, 2006"))
	}

	return nil
}

// AllProjectsResult holds results for all projects (JSON output)
type AllProjectsResult struct {
	Projects    []RunResult `json:"projects"`
	GeneratedAt time.Time   `json:"generatedAt"`
}

func runMonteCarloAll(confidence []int, iterations int, jsonOutput, quiet bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		return apperrors.InvalidArgumentError(
			"No projects configured",
			"Add projects to your .forecast/config.yaml",
		)
	}

	store := storage.New(".forecast")
	histStore := history.New(".forecast")
	now := time.Now()

	var results []RunResult
	var errors []string

	if !jsonOutput && !quiet {
		fmt.Printf("Running Monte Carlo simulation for %d projects (%d iterations each)...\n\n", len(projects), iterations)
	}

	for _, proj := range projects {
		projectKey := proj.Key
		if projectKey == "" {
			projectKey = proj.Epic
		}

		teamCapacity := cfg.TeamCapacity
		if proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
		if teamCapacity <= 0 {
			teamCapacity = 8.0
		}

		// Load items
		items, err := store.LoadProject(projectKey)
		if err != nil || len(items) == 0 {
			errors = append(errors, fmt.Sprintf("%s: no data", projectKey))
			continue
		}

		// Count items
		var completed, remaining, cycleTimeCount int
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

		if cycleTimeCount == 0 {
			errors = append(errors, fmt.Sprintf("%s: no cycle time data", projectKey))
			continue
		}

		// Run simulation
		simulator := montecarlo.NewSimulator(items, teamCapacity)
		result := simulator.Run(iterations)

		avgCycleTime := montecarlo.CalculateAvgCycleTime(items)
		throughput := montecarlo.CalculateThroughput(items, 14)

		// Save to history
		histEntry := history.Entry{
			Timestamp:   now,
			ProjectKey:  projectKey,
			Total:       completed + remaining,
			Completed:   completed,
			Remaining:   remaining,
			AvgCycle:    avgCycleTime,
			Throughput:  throughput,
			Percentiles: make(map[int]float64),
		}
		for _, p := range confidence {
			histEntry.Percentiles[p] = result.Percentiles[p]
		}
		_ = histStore.Save(histEntry)

		// Build result
		runResult := RunResult{
			ProjectKey:  projectKey,
			ProjectName: proj.Name,
			Total:       completed + remaining,
			Completed:   completed,
			Remaining:   remaining,
			AvgCycle:    avgCycleTime,
			Throughput:  throughput,
			Percentiles: make(map[int]PercentileResult),
			GeneratedAt: now,
		}

		for _, p := range confidence {
			days := result.Percentiles[p]
			runResult.Percentiles[p] = PercentileResult{
				Days:       days,
				TargetDate: now.AddDate(0, 0, int(days)),
			}
		}

		results = append(results, runResult)
	}

	if jsonOutput {
		output := AllProjectsResult{
			Projects:    results,
			GeneratedAt: now,
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	if quiet {
		return nil
	}

	// Human-readable table output
	fmt.Println("================================================================================")
	fmt.Println("  FORECAST RESULTS - ALL PROJECTS")
	fmt.Println("================================================================================")
	fmt.Printf("\n%-18s %6s %6s %7s %10s %10s %12s\n",
		"Project", "Done", "Left", "Prog", "70% Days", "85% Days", "85% Date")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, r := range results {
		progress := float64(r.Completed) / float64(r.Total) * 100
		days70 := r.Percentiles[70].Days
		days85 := r.Percentiles[85].Days
		date85 := r.Percentiles[85].TargetDate.Format("Jan 2")

		fmt.Printf("%-18s %6d %6d %6.0f%% %9.0f d %9.0f d %12s\n",
			truncate(r.ProjectName, 18),
			r.Completed,
			r.Remaining,
			progress,
			days70,
			days85,
			date85,
		)
	}

	if len(errors) > 0 {
		fmt.Println("\n--------------------------------------------------------------------------------")
		fmt.Println("Skipped (insufficient data):")
		for _, e := range errors {
			fmt.Printf("  - %s\n", e)
		}
	}

	fmt.Println()
	return nil
}
