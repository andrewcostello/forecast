package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	appcontext "github.com/andrewcostello/forecast/internal/context"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show summary of all tracked projects",
	Long: `Displays a dashboard summary of all configured projects/epics.
Shows completion status, forecast dates, and key metrics for each project.

Use --project to get detailed view of a specific project.
If no project specified and context is set, shows that project's detailed view.

Use --watch for continuous updates (refreshes every 60 seconds).
Use --local to use cached data only (faster, no JIRA calls).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		watch, _ := cmd.Flags().GetBool("watch")
		local, _ := cmd.Flags().GetBool("local")

		if watch {
			return runDashboardWatch(project, jsonOutput, local)
		}
		return runDashboard(project, jsonOutput)
	},
}

func init() {
	dashboardCmd.Flags().StringP("project", "p", "", "Show detailed view for specific project")
	dashboardCmd.Flags().Bool("json", false, "Output results as JSON")
	dashboardCmd.Flags().Bool("watch", false, "Continuously update dashboard")
	dashboardCmd.Flags().Bool("local", false, "Use cached data only (no JIRA calls)")
}

// DashboardResult holds dashboard data for JSON output
type DashboardResult struct {
	Projects    []DashboardProject `json:"projects"`
	GeneratedAt time.Time          `json:"generatedAt"`
}

// DashboardProject holds project data for dashboard
type DashboardProject struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Epic        string  `json:"epic"`
	Total       int     `json:"total"`
	Done        int     `json:"done"`
	InProgress  int     `json:"inProgress"`
	Remaining   int     `json:"remaining"`
	Progress    float64 `json:"progressPercent"`
	AvgCycle    float64 `json:"avgCycleHours,omitempty"`
	Forecast70  string  `json:"forecast70,omitempty"`
	Forecast95  string  `json:"forecast95,omitempty"`
	Error       string  `json:"error,omitempty"`
}

type projectStats struct {
	Total      int
	Done       int
	InProgress int
	Remaining  int
	AvgCycle   float64
	Forecast70 string
	Forecast95 string
}

func runDashboard(projectFilter string, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	// Resolve project from context if not specified
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		if jsonOutput {
			result := DashboardResult{
				Projects:    []DashboardProject{},
				GeneratedAt: time.Now(),
			}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		return apperrors.InvalidArgumentError(
			"No projects configured",
			"Add projects to your .forecast/config.yaml:\n\n"+
				"projects:\n"+
				"  - name: \"My Project\"\n"+
				"    key: \"myproject\"\n"+
				"    epic: \"PROJ-123\"",
		)
	}

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)

	// If filtering to specific project, show detailed view
	if projectFilter != "" {
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
		return showProjectDetail(jiraClient, cfg, proj, jsonOutput)
	}

	// JSON output for all projects
	if jsonOutput {
		result := DashboardResult{
			Projects:    []DashboardProject{},
			GeneratedAt: time.Now(),
		}

		for _, proj := range projects {
			dp := DashboardProject{
				Key:  proj.Key,
				Name: proj.Name,
				Epic: proj.Epic,
			}

			stats, err := getProjectStats(jiraClient, cfg, &proj)
			if err != nil {
				dp.Error = err.Error()
			} else {
				dp.Total = stats.Total
				dp.Done = stats.Done
				dp.InProgress = stats.InProgress
				dp.Remaining = stats.Remaining
				if stats.Total > 0 {
					dp.Progress = float64(stats.Done) / float64(stats.Total) * 100
				}
				dp.AvgCycle = stats.AvgCycle
				dp.Forecast70 = stats.Forecast70
				dp.Forecast95 = stats.Forecast95
			}

			result.Projects = append(result.Projects, dp)
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Show dashboard summary of all projects
	fmt.Println("\n================================================================================")
	fmt.Println("  PROJECT DASHBOARD")
	fmt.Println("================================================================================")
	fmt.Printf("\n%-20s %-12s %8s %8s %8s %12s %12s\n",
		"Project", "Epic", "Total", "Done", "Progress", "70% Conf", "95% Conf")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, proj := range projects {
		stats, err := getProjectStats(jiraClient, cfg, &proj)
		if err != nil {
			fmt.Printf("%-20s %-12s %s\n", truncate(proj.Name, 20), proj.Epic, fmt.Sprintf("Error: %v", err))
			continue
		}

		progress := float64(0)
		if stats.Total > 0 {
			progress = float64(stats.Done) / float64(stats.Total) * 100
		}

		forecast70 := "-"
		forecast95 := "-"
		if stats.Forecast70 != "" {
			forecast70 = stats.Forecast70
		}
		if stats.Forecast95 != "" {
			forecast95 = stats.Forecast95
		}

		fmt.Printf("%-20s %-12s %8d %8d %7.1f%% %12s %12s\n",
			truncate(proj.Name, 20),
			proj.Epic,
			stats.Total,
			stats.Done,
			progress,
			forecast70,
			forecast95)
	}

	fmt.Println("\n--------------------------------------------------------------------------------")
	fmt.Println("Use 'forecast dashboard --project <key>' for detailed view")
	fmt.Println()

	return nil
}

func getProjectStats(client *jira.Client, cfg *config.Config, proj *config.ProjectConfig) (*projectStats, error) {
	// Build JQL for this project's epic
	jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, cfg.JIRA.ProjectKey, proj.Epic)

	issues, err := client.SearchJQL(jql)
	if err != nil {
		// Try parent field for next-gen projects
		jql = fmt.Sprintf(`project = %s AND parent = %s`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, err = client.SearchJQL(jql)
		if err != nil {
			return nil, err
		}
	}

	stats := &projectStats{}
	var totalCycleTime float64
	var cycleTimeCount int

	// Convert issues to forecast items to get cycle time data
	items := client.ConvertIssuesToItems(issues, cfg)

	for _, item := range items {
		switch item.Status {
		case "Done":
			stats.Total++
			stats.Done++
			// Add cycle time if available
			if item.CycleTime > 0 {
				totalCycleTime += item.CycleTime
				cycleTimeCount++
			}
		case "Canceled":
			// Exclude canceled items from all counts
			continue
		case "In Progress", "In Development":
			stats.Total++
			stats.InProgress++
		default:
			stats.Total++
			stats.Remaining++
		}
	}

	// Calculate average cycle time from completed items with changelog
	if cycleTimeCount > 0 {
		stats.AvgCycle = totalCycleTime / float64(cycleTimeCount)
	}

	// Generate forecast if we have data
	if stats.Done > 0 && stats.Remaining > 0 && stats.AvgCycle > 0 {
		// Simple forecast based on average cycle time
		capacity := cfg.TeamCapacity
		if proj.Capacity > 0 {
			capacity = proj.Capacity
		}
		if capacity <= 0 {
			capacity = 8
		}

		daysPerItem := stats.AvgCycle / capacity
		days70 := int(float64(stats.Remaining) * daysPerItem * 1.3)  // 30% buffer for 70%
		days95 := int(float64(stats.Remaining) * daysPerItem * 1.8)  // 80% buffer for 95%

		now := time.Now()
		stats.Forecast70 = now.AddDate(0, 0, days70).Format("Jan 2")
		stats.Forecast95 = now.AddDate(0, 0, days95).Format("Jan 2")
	}

	return stats, nil
}

// ProjectDetailResult holds detailed project data for JSON output
type ProjectDetailResult struct {
	Key          string              `json:"key"`
	Name         string              `json:"name"`
	Epic         string              `json:"epic"`
	Total        int                 `json:"total"`
	Done         int                 `json:"done"`
	InProgress   int                 `json:"inProgress"`
	Remaining    int                 `json:"remaining"`
	Progress     float64             `json:"progressPercent"`
	AvgCycle     float64             `json:"avgCycleHours,omitempty"`
	Forecast70   string              `json:"forecast70,omitempty"`
	Forecast95   string              `json:"forecast95,omitempty"`
	RecentItems  []RecentItemResult  `json:"recentItems,omitempty"`
	GeneratedAt  time.Time           `json:"generatedAt"`
}

// RecentItemResult holds recent item data for JSON output
type RecentItemResult struct {
	Key     string `json:"key"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

func showProjectDetail(client *jira.Client, cfg *config.Config, proj *config.ProjectConfig, jsonOutput bool) error {
	stats, err := getProjectStats(client, cfg, proj)
	if err != nil {
		return err
	}

	progress := float64(0)
	if stats.Total > 0 {
		progress = float64(stats.Done) / float64(stats.Total) * 100
	}

	// Fetch recent items
	jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s ORDER BY updated DESC`, cfg.JIRA.ProjectKey, proj.Epic)
	issues, err := client.SearchJQL(jql)
	if err != nil {
		jql = fmt.Sprintf(`project = %s AND parent = %s ORDER BY updated DESC`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, _ = client.SearchJQL(jql)
	}

	// JSON output
	if jsonOutput {
		result := ProjectDetailResult{
			Key:         proj.Key,
			Name:        proj.Name,
			Epic:        proj.Epic,
			Total:       stats.Total,
			Done:        stats.Done,
			InProgress:  stats.InProgress,
			Remaining:   stats.Remaining,
			Progress:    progress,
			AvgCycle:    stats.AvgCycle,
			Forecast70:  stats.Forecast70,
			Forecast95:  stats.Forecast95,
			RecentItems: []RecentItemResult{},
			GeneratedAt: time.Now(),
		}

		count := 0
		for _, issue := range issues {
			if count >= 5 {
				break
			}
			result.RecentItems = append(result.RecentItems, RecentItemResult{
				Key:     issue.Key,
				Status:  issue.Fields.Status.Name,
				Summary: issue.Fields.Summary,
			})
			count++
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Println("\n===========================================================")
	fmt.Printf("  %s\n", proj.Name)
	fmt.Printf("  Epic: %s\n", proj.Epic)
	fmt.Println("===========================================================")

	fmt.Println("\n## Status")
	fmt.Printf("  Total Items:    %d\n", stats.Total)
	fmt.Printf("  Completed:      %d (%.1f%%)\n", stats.Done, progress)
	fmt.Printf("  In Progress:    %d\n", stats.InProgress)
	fmt.Printf("  Remaining:      %d\n", stats.Remaining)

	if stats.AvgCycle > 0 {
		fmt.Println("\n## Metrics")
		fmt.Printf("  Avg Cycle Time: %.1f hours\n", stats.AvgCycle)
	}

	if stats.Forecast70 != "" {
		fmt.Println("\n## Forecast")
		fmt.Printf("  70%% confidence: %s\n", stats.Forecast70)
		fmt.Printf("  95%% confidence: %s\n", stats.Forecast95)
	} else {
		fmt.Println("\n## Forecast")
		fmt.Println("  Insufficient data - need completed items with cycle times")
	}

	// Show recent activity
	fmt.Println("\n## Recent Items")

	count := 0
	for _, issue := range issues {
		if count >= 5 {
			break
		}
		summary := issue.Fields.Summary
		if len(summary) > 40 {
			summary = summary[:40] + "..."
		}
		fmt.Printf("  %-12s %-15s %s\n", issue.Key, issue.Fields.Status.Name, summary)
		count++
	}

	fmt.Println()
	return nil
}

// runDashboardWatch continuously updates the dashboard
func runDashboardWatch(projectFilter string, jsonOutput, local bool) error {
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

	fmt.Println("Dashboard (Ctrl+C to stop, updates every 60s)...")
	fmt.Println()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial display
	if err := displayDashboardOnce(cfg, projects, projectFilter, jsonOutput, local); err != nil {
		return err
	}

	for range ticker.C {
		// Clear screen for fresh display
		if !jsonOutput {
			fmt.Print("\033[H\033[2J") // Clear screen
			fmt.Println("Dashboard (Ctrl+C to stop, updates every 60s)...")
			fmt.Println()
		}

		if err := displayDashboardOnce(cfg, projects, projectFilter, jsonOutput, local); err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
	}

	return nil
}

// displayDashboardOnce renders a single dashboard frame
func displayDashboardOnce(cfg *config.Config, projects []config.ProjectConfig, projectFilter string, jsonOutput, local bool) error {
	// If filtering to specific project, show detailed view
	if projectFilter != "" || appcontext.GetProject() != "" {
		filter := projectFilter
		if filter == "" {
			filter = appcontext.GetProject()
		}
		proj := cfg.GetProject(filter)
		if proj == nil {
			return apperrors.ProjectNotFoundError(filter, nil)
		}
		jiraClient := jira.NewClient(&cfg.JIRA)
		return showProjectDetail(jiraClient, cfg, proj, jsonOutput)
	}

	// JSON output for all projects
	if jsonOutput {
		result := DashboardResult{
			Projects:    []DashboardProject{},
			GeneratedAt: time.Now(),
		}

		jiraClient := jira.NewClient(&cfg.JIRA)
		for _, proj := range projects {
			dp := DashboardProject{
				Key:  proj.Key,
				Name: proj.Name,
				Epic: proj.Epic,
			}

			stats, err := getProjectStats(jiraClient, cfg, &proj)
			if err != nil {
				dp.Error = err.Error()
			} else {
				dp.Total = stats.Total
				dp.Done = stats.Done
				dp.InProgress = stats.InProgress
				dp.Remaining = stats.Remaining
				if stats.Total > 0 {
					dp.Progress = float64(stats.Done) / float64(stats.Total) * 100
				}
				dp.AvgCycle = stats.AvgCycle
				dp.Forecast70 = stats.Forecast70
				dp.Forecast95 = stats.Forecast95
			}

			result.Projects = append(result.Projects, dp)
		}

		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable dashboard
	fmt.Println("================================================================================")
	fmt.Println("  PROJECT DASHBOARD")
	fmt.Printf("  Updated: %s\n", time.Now().Format("15:04:05"))
	fmt.Println("================================================================================")
	fmt.Printf("\n%-20s %-12s %8s %8s %8s %12s %12s\n",
		"Project", "Epic", "Total", "Done", "Progress", "70% Conf", "95% Conf")
	fmt.Println("--------------------------------------------------------------------------------")

	jiraClient := jira.NewClient(&cfg.JIRA)
	for _, proj := range projects {
		stats, err := getProjectStats(jiraClient, cfg, &proj)
		if err != nil {
			fmt.Printf("%-20s %-12s %s\n", truncate(proj.Name, 20), proj.Epic, fmt.Sprintf("Error: %v", err))
			continue
		}

		progress := float64(0)
		if stats.Total > 0 {
			progress = float64(stats.Done) / float64(stats.Total) * 100
		}

		forecast70 := "-"
		forecast95 := "-"
		if stats.Forecast70 != "" {
			forecast70 = stats.Forecast70
		}
		if stats.Forecast95 != "" {
			forecast95 = stats.Forecast95
		}

		fmt.Printf("%-20s %-12s %8d %8d %7.1f%% %12s %12s\n",
			truncate(proj.Name, 20),
			proj.Epic,
			stats.Total,
			stats.Done,
			progress,
			forecast70,
			forecast95)
	}

	fmt.Println("\n--------------------------------------------------------------------------------")
	fmt.Println("Use 'forecast dashboard --project <key>' for detailed view")
	fmt.Println()

	return nil
}
