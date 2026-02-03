package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	appcontext "github.com/andrewcostello/forecast/internal/context"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/internal/terminal"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [project]",
	Short: "Show quick project status summary",
	Long: `Display a one-line status summary for a project.

If no project is specified, uses the current context (set with 'forecast use').
If no context is set and only one project exists, uses that project.

Examples:
  forecast status              # Uses current context
  forecast status myproject    # Specific project
  forecast status --watch      # Continuous updates
  forecast status --all        # Show all projects`,
	RunE: func(cmd *cobra.Command, args []string) error {
		watch, _ := cmd.Flags().GetBool("watch")
		all, _ := cmd.Flags().GetBool("all")
		jsonOutput, _ := cmd.Flags().GetBool("json")

		var projectKey string
		if len(args) > 0 {
			projectKey = args[0]
		}

		if watch {
			return runStatusWatch(projectKey, jsonOutput)
		}
		if all {
			return runStatusAll(jsonOutput)
		}
		return runStatus(projectKey, jsonOutput)
	},
}

func init() {
	statusCmd.Flags().Bool("watch", false, "Continuously update status")
	statusCmd.Flags().Bool("all", false, "Show status for all projects")
	statusCmd.Flags().Bool("json", false, "Output as JSON")

	rootCmd.AddCommand(statusCmd)
}

// StatusInfo holds status information for display
type StatusInfo struct {
	ProjectKey   string  `json:"projectKey"`
	ProjectName  string  `json:"projectName"`
	Total        int     `json:"total"`
	Done         int     `json:"done"`
	InProgress   int     `json:"inProgress"`
	Remaining    int     `json:"remaining"`
	Progress     float64 `json:"progress"`
	ForecastDays float64 `json:"forecastDays,omitempty"`
	ForecastDate string  `json:"forecastDate,omitempty"`
	AvgCycle     float64 `json:"avgCycleHours,omitempty"`
	Health       string  `json:"health"` // good, warning, critical
}

func runStatus(projectKey string, jsonOutput bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no config found. Run 'forecast init' to set up")
	}

	// Resolve project
	projectKey = resolveProjectKey(cfg, projectKey)
	if projectKey == "" {
		return fmt.Errorf("no project specified. Use 'forecast status <project>' or 'forecast use <project>' to set context")
	}

	info, err := getProjectStatus(cfg, projectKey)
	if err != nil {
		return err
	}

	if jsonOutput {
		return printStatusJSON(info)
	}

	printStatusLine(info)
	return nil
}

func runStatusAll(jsonOutput bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no config found. Run 'forecast init' to set up")
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		return fmt.Errorf("no projects configured")
	}

	var allInfo []StatusInfo
	for _, proj := range projects {
		key := proj.Key
		if key == "" {
			key = proj.Epic
		}
		info, err := getProjectStatus(cfg, key)
		if err != nil {
			// Skip projects with errors in --all mode
			continue
		}
		allInfo = append(allInfo, info)
	}

	if jsonOutput {
		return printStatusJSONArray(allInfo)
	}

	for _, info := range allInfo {
		printStatusLine(info)
	}
	return nil
}

func runStatusWatch(projectKey string, jsonOutput bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no config found. Run 'forecast init' to set up")
	}

	projectKey = resolveProjectKey(cfg, projectKey)
	if projectKey == "" {
		return fmt.Errorf("no project specified")
	}

	fmt.Println("Watching project status (Ctrl+C to stop)...")
	fmt.Println()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Initial display
	info, err := getProjectStatus(cfg, projectKey)
	if err != nil {
		return err
	}
	if jsonOutput {
		printStatusJSON(info)
	} else {
		printStatusLine(info)
	}

	for range ticker.C {
		// Clear line and reprint
		if !jsonOutput {
			fmt.Print("\033[1A\033[K") // Move up and clear line
		}

		info, err := getProjectStatus(cfg, projectKey)
		if err != nil {
			terminal.PrintError("Error: %v", err)
			continue
		}

		if jsonOutput {
			printStatusJSON(info)
		} else {
			printStatusLine(info)
		}
	}

	return nil
}

func resolveProjectKey(cfg *config.Config, projectKey string) string {
	// If key provided, use it
	if projectKey != "" {
		proj := cfg.GetProject(projectKey)
		if proj != nil {
			if proj.Key != "" {
				return proj.Key
			}
			return proj.Epic
		}
		return projectKey
	}

	// Try context
	contextKey := appcontext.GetProject()
	if contextKey != "" {
		return contextKey
	}

	// If only one project, use it
	projects := cfg.GetAllProjects()
	if len(projects) == 1 {
		if projects[0].Key != "" {
			return projects[0].Key
		}
		return projects[0].Epic
	}

	return ""
}

func getProjectStatus(cfg *config.Config, projectKey string) (StatusInfo, error) {
	info := StatusInfo{
		ProjectKey: projectKey,
	}

	proj := cfg.GetProject(projectKey)
	if proj != nil {
		info.ProjectName = proj.Name
	} else {
		info.ProjectName = projectKey
	}

	// Try to load from local storage first (faster)
	store := storage.New(".forecast")
	items, err := store.LoadProject(projectKey)

	if err != nil || len(items) == 0 {
		// Fall back to JIRA live query
		if proj == nil {
			return info, fmt.Errorf("project '%s' not found", projectKey)
		}

		jiraClient := jira.NewClient(&cfg.JIRA)
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, err := jiraClient.SearchJQL(jql)
		if err != nil {
			// Try parent field
			jql = fmt.Sprintf(`project = %s AND parent = %s`, cfg.JIRA.ProjectKey, proj.Epic)
			issues, err = jiraClient.SearchJQL(jql)
			if err != nil {
				return info, fmt.Errorf("failed to fetch data: %w", err)
			}
		}
		items = jiraClient.ConvertIssuesToItems(issues, cfg)
	}

	// Calculate stats
	var totalCycleTime float64
	var cycleTimeCount int

	for _, item := range items {
		switch item.Status {
		case "Done":
			info.Total++
			info.Done++
			if item.CycleTime > 0 {
				totalCycleTime += item.CycleTime
				cycleTimeCount++
			}
		case "Canceled":
			continue
		case "In Progress", "In Development":
			info.Total++
			info.InProgress++
		default:
			info.Total++
			info.Remaining++
		}
	}

	if info.Total > 0 {
		info.Progress = float64(info.Done) / float64(info.Total) * 100
	}

	if cycleTimeCount > 0 {
		info.AvgCycle = totalCycleTime / float64(cycleTimeCount)
	}

	// Calculate forecast if we have data
	remaining := info.InProgress + info.Remaining
	if cycleTimeCount > 0 && remaining > 0 {
		teamCapacity := cfg.TeamCapacity
		if proj != nil && proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
		if teamCapacity <= 0 {
			teamCapacity = 8.0
		}

		simulator := montecarlo.NewSimulator(items, teamCapacity)
		result := simulator.Run(5000) // Use fewer iterations for speed

		info.ForecastDays = result.Percentiles[85]
		targetDate := time.Now().AddDate(0, 0, int(info.ForecastDays))
		info.ForecastDate = targetDate.Format("Jan 2")
	}

	// Determine health
	info.Health = determineHealth(info)

	return info, nil
}

func determineHealth(info StatusInfo) string {
	// If done, it's good
	if info.Remaining == 0 && info.InProgress == 0 {
		return "good"
	}

	// If high progress, good
	if info.Progress >= 75 {
		return "good"
	}

	// If low progress and far out, warning
	if info.Progress >= 25 {
		return "warning"
	}

	// If very low progress, critical
	return "critical"
}

func printStatusLine(info StatusInfo) {
	// Color codes
	var colorCode string
	switch info.Health {
	case "good":
		colorCode = "\033[32m" // Green
	case "warning":
		colorCode = "\033[33m" // Yellow
	case "critical":
		colorCode = "\033[31m" // Red
	default:
		colorCode = ""
	}
	resetCode := "\033[0m"

	// Build status line
	remaining := info.InProgress + info.Remaining
	line := fmt.Sprintf("%s%s%s: %.0f%% done, %d remaining",
		colorCode, info.ProjectName, resetCode,
		info.Progress, remaining)

	if info.ForecastDate != "" {
		line += fmt.Sprintf(", ~%.0f days (%s @85%%)", info.ForecastDays, info.ForecastDate)
	}

	fmt.Println(line)
}

func printStatusJSON(info StatusInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printStatusJSONArray(infos []StatusInfo) error {
	data, err := json.MarshalIndent(infos, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

