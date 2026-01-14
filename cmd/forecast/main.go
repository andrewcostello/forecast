package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bitbucket.org/supermoneygames/forecast/internal/config"
	"bitbucket.org/supermoneygames/forecast/internal/jira"
	"bitbucket.org/supermoneygames/forecast/internal/montecarlo"
	"bitbucket.org/supermoneygames/forecast/internal/referenceclass"
	"bitbucket.org/supermoneygames/forecast/internal/report"
	"bitbucket.org/supermoneygames/forecast/internal/storage"
	yamlparser "bitbucket.org/supermoneygames/forecast/internal/yaml"
	"bitbucket.org/supermoneygames/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var cfgFile string

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "forecast",
	Short: "Probabilistic forecasting tool for software projects",
	Long: `Forecast uses Reference Class Forecasting, Earned Value Analysis,
and Monte Carlo simulation to provide probabilistic completion forecasts.

Integrates with JIRA to track cycle times and uses historical data
to improve predictions over time.`,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./.forecast/config.yaml)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(reportCmd)
	rootCmd.AddCommand(referenceClassCmd)
	rootCmd.AddCommand(jiraCmd)
	rootCmd.AddCommand(dashboardCmd)
}

func initConfig() {
	if err := config.Load(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize forecasting for a project",
	Long: `Creates a .forecast directory with configuration files.
Sets up project-specific settings for JIRA integration and item tracking.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit()
	},
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync data from JIRA or YAML files",
	Long: `Pulls latest issue data from JIRA including:
  - Issue status
  - Cycle times (Created → Done)
  - Item types and sizes
  - Assignees

Use --project to sync a specific project, or sync all configured projects.

YAML File Sync (--file):
  Bidirectional sync between a YAML task file and JIRA:
  - Creates JIRA tickets for tasks without jira_key
  - Fetches status for tasks with jira_key
  - Writes jira_key back to YAML after creation
  - Saves items to local storage for forecasting

Example:
  forecast sync --file docs/tasks/WALLET_TASKS.yaml
  forecast sync --file tasks.yaml --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		file, _ := cmd.Flags().GetString("file")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if file != "" {
			return runSyncFromFile(file, dryRun)
		}
		return runSync(project)
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Monte Carlo simulation",
	Long: `Executes Monte Carlo simulation to forecast project completion.
Uses actual cycle time data to predict completion dates with confidence levels.

Use --project to run forecast for a specific project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		confidence, _ := cmd.Flags().GetIntSlice("confidence")
		iterations, _ := cmd.Flags().GetInt("iterations")
		project, _ := cmd.Flags().GetString("project")
		return runMonteCarlo(confidence, iterations, project)
	},
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate forecast report",
	Long: `Generates a detailed report with:
  - Earned Value Analysis metrics
  - Monte Carlo forecast results
  - Throughput trends
  - Reference class comparisons

Use --project to generate report for a specific project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reportType, _ := cmd.Flags().GetString("type")
		project, _ := cmd.Flags().GetString("project")
		return runReport(reportType, project)
	},
}

var referenceClassCmd = &cobra.Command{
	Use:   "reference-class",
	Short: "Manage reference class database",
	Long: `Commands for managing historical project data:
  list - Show available reference classes
  add  - Add completed project to reference database`,
}

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show summary of all tracked projects",
	Long: `Displays a dashboard summary of all configured projects/epics.
Shows completion status, forecast dates, and key metrics for each project.

Use --project to get detailed view of a specific project.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		return runDashboard(project)
	},
}

func init() {
	syncCmd.Flags().StringP("project", "p", "", "Sync specific project (by key or epic), or all if omitted")
	syncCmd.Flags().StringP("file", "f", "", "Sync tasks from a YAML file (bidirectional)")
	syncCmd.Flags().Bool("dry-run", false, "Preview what would be created without making changes")

	runCmd.Flags().IntSlice("confidence", []int{50, 70, 85, 95}, "Confidence levels for forecast")
	runCmd.Flags().Int("iterations", 10000, "Number of Monte Carlo iterations")
	runCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")

	reportCmd.Flags().String("type", "eva", "Report type: eva, montecarlo, full")
	reportCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")

	dashboardCmd.Flags().StringP("project", "p", "", "Show detailed view for specific project")

	referenceClassCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List available reference classes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listReferenceClasses()
		},
	})

	referenceClassCmd.AddCommand(&cobra.Command{
		Use:   "add",
		Short: "Add current project to reference database",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addReferenceClass()
		},
	})
}

// Command implementations (stubs for now)
func runInit() error {
	fmt.Println("Initializing forecast project...")
	return config.InitProject()
}

func runSync(projectFilter string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)
	store := storage.New(".forecast")

	// Get projects to sync
	projects := cfg.GetAllProjects()

	// If no projects configured, use legacy single-epic mode
	if len(projects) == 0 {
		fmt.Println("Syncing from JIRA (legacy mode)...")
		items, err := jiraClient.FetchIssues(cfg)
		if err != nil {
			return fmt.Errorf("failed to fetch issues: %w", err)
		}
		fmt.Printf("Fetched %d items from JIRA\n", len(items))
		if err := store.Save(items); err != nil {
			return fmt.Errorf("failed to save items: %w", err)
		}
		fmt.Println("✓ Sync complete")
		return nil
	}

	// Filter to specific project if requested
	if projectFilter != "" {
		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
		}
		projects = []config.ProjectConfig{*proj}
	}

	// Sync each project
	totalItems := 0
	for _, proj := range projects {
		fmt.Printf("Syncing %s (%s)...\n", proj.Name, proj.Epic)

		// Build JQL for this project's epic
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, err := jiraClient.SearchJQL(jql)
		if err != nil {
			// Try parent field for next-gen projects
			jql = fmt.Sprintf(`project = %s AND parent = %s`, cfg.JIRA.ProjectKey, proj.Epic)
			issues, err = jiraClient.SearchJQL(jql)
			if err != nil {
				fmt.Printf("  Warning: failed to fetch %s: %v\n", proj.Epic, err)
				continue
			}
		}

		// Convert JIRA issues to forecast items with cycle time
		items := jiraClient.ConvertIssuesToItems(issues, cfg)

		// Save to project-specific file
		projectKey := proj.Key
		if projectKey == "" {
			projectKey = proj.Epic
		}
		if err := store.SaveProject(items, projectKey); err != nil {
			return fmt.Errorf("failed to save items for %s: %w", proj.Name, err)
		}

		fmt.Printf("  Fetched %d items\n", len(items))
		totalItems += len(items)
	}

	fmt.Printf("\n✓ Sync complete: %d total items across %d projects\n", totalItems, len(projects))
	return nil
}

func runSyncFromFile(filePath string, dryRun bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	// Parse YAML file
	tf, err := yamlparser.ParseTaskFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to parse task file: %w", err)
	}

	fmt.Printf("Loaded %d tasks from %s\n", len(tf.Tasks), filePath)

	// Get JIRA instance config (from yaml jira_instance field or default)
	jiraCfg := cfg.GetJIRAInstance(tf.JiraInstance)
	if tf.JiraInstance != "" {
		fmt.Printf("Using JIRA instance: %s (%s)\n", tf.JiraInstance, jiraCfg.URL)
	}

	// Use project key from JIRA config if YAML project matches, otherwise use YAML project
	projectKey := tf.Project
	if jiraCfg.ProjectKey != "" {
		projectKey = jiraCfg.ProjectKey
	}

	fmt.Printf("Project: %s, Epic: %s\n\n", projectKey, tf.Epic)

	// Create JIRA client with the selected instance
	jiraClient := jira.NewClient(jiraCfg)

	var created, updated, skipped int
	var modified bool

	for i := range tf.Tasks {
		task := &tf.Tasks[i]

		if task.JiraKey != "" {
			// Task already has JIRA key - fetch status
			fmt.Printf("  [%s] %s → fetching status from %s...\n", task.Key, truncate(task.Summary, 40), task.JiraKey)

			if !dryRun {
				issue, err := jiraClient.GetIssue(task.JiraKey)
				if err != nil {
					fmt.Printf("    Warning: failed to fetch %s: %v\n", task.JiraKey, err)
					skipped++
					continue
				}

				if task.Status != issue.Fields.Status.Name {
					task.Status = issue.Fields.Status.Name
					modified = true
					fmt.Printf("    Status: %s\n", task.Status)
				}
			}
			updated++
		} else {
			// No JIRA key - create new ticket
			fmt.Printf("  [%s] %s → creating in JIRA...\n", task.Key, truncate(task.Summary, 40))

			if !dryRun {
				req := jira.CreateIssueRequest{
					Summary:     fmt.Sprintf("%s: %s", task.Key, task.Summary),
					IssueType:   task.Type,
					Description: task.Description,
					Priority:    "Medium",
					Labels:      task.Labels,
					Epic:        tf.Epic,
					Project:     projectKey,
				}

				// Default to Story if type not specified
				if req.IssueType == "" {
					req.IssueType = "Story"
				}

				result, err := jiraClient.CreateIssue(req)
				if err != nil {
					fmt.Printf("    Error: %v\n", err)
					skipped++
					continue
				}

				task.JiraKey = result.Key
				task.Status = "To Do"
				modified = true
				fmt.Printf("    Created: %s\n", result.Key)
			} else {
				fmt.Printf("    Would create: %s: %s\n", task.Key, task.Summary)
			}
			created++
		}
	}

	// Save updated YAML file
	if modified && !dryRun {
		if err := tf.Save(); err != nil {
			return fmt.Errorf("failed to save updated YAML: %w", err)
		}
		fmt.Printf("\n✓ Updated %s with JIRA keys\n", filePath)
	}

	// Also save to local storage for forecasting
	store := storage.New(".forecast")

	// Convert tasks to forecast items
	items := make([]forecast.Item, 0, len(tf.Tasks))
	for _, task := range tf.Tasks {
		item := forecast.Item{
			ID:          task.Key,
			JiraKey:     task.JiraKey,
			Description: task.Summary,
			Status:      task.Status,
			Type:        task.Type,
		}

		// Parse size from labels
		for _, label := range task.Labels {
			if strings.HasPrefix(label, "size:") {
				item.Size = strings.TrimPrefix(label, "size:")
				break
			}
		}

		items = append(items, item)
	}

	// Use projectKey for storage (already resolved above, fallback to filename if empty)
	storageKey := projectKey
	if storageKey == "" {
		storageKey = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	}

	if !dryRun {
		if err := store.SaveProject(items, storageKey); err != nil {
			fmt.Printf("Warning: failed to save to local storage: %v\n", err)
		} else {
			fmt.Printf("✓ Saved %d items to .forecast/data-%s.json\n", len(items), storageKey)
		}
	}

	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("Summary: %d created, %d updated, %d skipped\n", created, updated, skipped)

	if dryRun {
		fmt.Println("\n(dry-run mode - no changes were made)")
	}

	return nil
}

func runMonteCarlo(confidence []int, iterations int, projectFilter string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	store := storage.New(".forecast")

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
			fmt.Println("Available projects:")
			for _, p := range projects {
				fmt.Printf("  - %s (%s)\n", p.Key, p.Name)
			}
			return fmt.Errorf("specify a project with --project flag")
		}

		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
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

	fmt.Printf("Running Monte Carlo simulation (%d iterations)...\n", iterations)
	if projectName != "" {
		fmt.Printf("Project: %s\n", projectName)
	}

	// Load items
	var items []forecast.Item
	var err error
	if projectKey != "" {
		items, err = store.LoadProject(projectKey)
	} else {
		items, err = store.Load()
	}
	if err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("no items found - run 'forecast sync' first")
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
		} else {
			remaining++
		}
	}

	if cycleTimeCount == 0 {
		return fmt.Errorf("no completed items with cycle time data - cannot forecast")
	}

	// Create and run Monte Carlo simulator
	simulator := montecarlo.NewSimulator(items, teamCapacity)
	result := simulator.Run(iterations)

	// Calculate additional metrics
	avgCycleTime := montecarlo.CalculateAvgCycleTime(items)
	throughput := montecarlo.CalculateThroughput(items, 14) // Last 14 days

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\nRemaining: %d items (%.1f%%)\n", remaining, float64(remaining)/float64(len(items))*100)
	fmt.Printf("Completed: %d items\n", completed)
	fmt.Printf("Avg Cycle Time: %.1f hours\n", avgCycleTime)
	fmt.Printf("Throughput (14d): %.2f items/day\n", throughput)

	// Display forecast results
	fmt.Printf("\nForecast (days to complete remaining work):\n")
	now := time.Now()
	for _, p := range confidence {
		days := result.Percentiles[p]
		targetDate := now.AddDate(0, 0, int(days))
		fmt.Printf("  %d%% confidence: %.0f days (%s)\n", p, days, targetDate.Format("Jan 2, 2006"))
	}

	return nil
}

func runReport(reportType string, projectFilter string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	store := storage.New(".forecast")

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
			fmt.Println("Available projects:")
			for _, p := range projects {
				fmt.Printf("  - %s (%s)\n", p.Key, p.Name)
			}
			return fmt.Errorf("specify a project with --project flag")
		}

		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
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
	var err error
	if projectKey != "" {
		items, err = store.LoadProject(projectKey)
	} else {
		items, err = store.Load()
	}
	if err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("no items found - run 'forecast sync' first")
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

	gen := report.NewGenerator(items, teamCapacity, projectName, startDate)

	switch reportType {
	case "eva":
		return gen.GenerateEVA(os.Stdout)
	case "montecarlo", "mc":
		return gen.GenerateMonteCarlo(os.Stdout, 10000)
	case "full":
		return gen.GenerateFull(os.Stdout, 10000)
	default:
		return fmt.Errorf("unknown report type: %s (valid: eva, montecarlo, full)", reportType)
	}
}

func listReferenceClasses() error {
	db, err := referenceclass.NewDatabase()
	if err != nil {
		return fmt.Errorf("failed to open reference database: %w", err)
	}
	defer db.Close()

	summaries, err := db.ListReferenceClasses()
	if err != nil {
		return fmt.Errorf("failed to list reference classes: %w", err)
	}

	if len(summaries) == 0 {
		fmt.Println("No reference classes found.")
		fmt.Println("\nAdd completed projects using: forecast reference-class add")
		return nil
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Available Reference Classes")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\n%-25s %8s %8s %10s %10s\n", "Type", "Projects", "Items", "Avg Hours", "Std Dev")
	fmt.Println("─────────────────────────────────────────────────────────")

	for _, s := range summaries {
		fmt.Printf("%-25s %8d %8d %10.1f %10.1f\n",
			s.Type, s.ProjectCount, s.ItemCount, s.AvgHours, s.StdDev)
	}

	return nil
}

func addReferenceClass() error {
	fmt.Println("Adding project to reference database...")

	// Load current project items
	store := storage.New(".forecast")
	items, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("no items found - run 'forecast sync' first")
	}

	// Count completed items
	var completedCount int
	for _, item := range items {
		if item.Status == "Done" && item.CycleTime > 0 {
			completedCount++
		}
	}

	if completedCount == 0 {
		return fmt.Errorf("no completed items with cycle time data to add")
	}

	// Get project info from config
	cfg := config.Get()
	projectName := "Unknown Project"
	projectType := "General"
	teamSize := 1

	if cfg != nil {
		if cfg.ProjectName != "" {
			projectName = cfg.ProjectName
		}
		if cfg.ProjectType != "" {
			projectType = cfg.ProjectType
		}
		if cfg.TeamSize > 0 {
			teamSize = cfg.TeamSize
		}
	}

	// Create reference class from items
	rc := referenceclass.CreateFromItems(projectName, projectType, teamSize, items)

	// Open database and add
	db, err := referenceclass.NewDatabase()
	if err != nil {
		return fmt.Errorf("failed to open reference database: %w", err)
	}
	defer db.Close()

	if err := db.AddProject(rc); err != nil {
		return fmt.Errorf("failed to add reference class: %w", err)
	}

	fmt.Printf("\n✓ Added '%s' to reference database\n", projectName)
	fmt.Printf("  Type: %s\n", projectType)
	fmt.Printf("  Team Size: %d\n", teamSize)
	fmt.Printf("  Items: %d completed work items\n", len(rc.Items))

	return nil
}

func runDashboard(projectFilter string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		fmt.Println("No projects configured.")
		fmt.Println("\nAdd projects to your .forecast/config.yaml:")
		fmt.Println(`
projects:
  - name: "Monorepo Migration"
    key: "monorepo"
    epic: "SMG-1688"
  - name: "Feature X"
    key: "feature-x"
    epic: "SMG-1700"`)
		return nil
	}

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)

	// If filtering to specific project, show detailed view
	if projectFilter != "" {
		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
		}
		return showProjectDetail(jiraClient, cfg, proj)
	}

	// Show dashboard summary of all projects
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  PROJECT DASHBOARD")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\n%-20s %-12s %8s %8s %8s %12s %12s\n",
		"Project", "Epic", "Total", "Done", "Progress", "70% Conf", "95% Conf")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

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

	fmt.Println("\n────────────────────────────────────────────────────────────────────────────────")
	fmt.Println("Use 'forecast dashboard --project <key>' for detailed view")
	fmt.Println()

	return nil
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
		stats.Total++
		switch item.Status {
		case "Done":
			stats.Done++
			// Add cycle time if available
			if item.CycleTime > 0 {
				totalCycleTime += item.CycleTime
				cycleTimeCount++
			}
		case "In Progress", "In Development":
			stats.InProgress++
		default:
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

func showProjectDetail(client *jira.Client, cfg *config.Config, proj *config.ProjectConfig) error {
	stats, err := getProjectStats(client, cfg, proj)
	if err != nil {
		return err
	}

	progress := float64(0)
	if stats.Total > 0 {
		progress = float64(stats.Done) / float64(stats.Total) * 100
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("  %s\n", proj.Name)
	fmt.Printf("  Epic: %s\n", proj.Epic)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n## Status")
	fmt.Printf("  Total Items:    %d\n", stats.Total)
	fmt.Printf("  Completed:      %d (%.1f%%)\n", stats.Done, progress)
	fmt.Printf("  In Progress:    %d\n", stats.InProgress)
	fmt.Printf("  Remaining:      %d\n", stats.Remaining)

	if stats.AvgCycle > 0 {
		fmt.Printf("\n## Metrics")
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
	jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s ORDER BY updated DESC`, cfg.JIRA.ProjectKey, proj.Epic)
	issues, err := client.SearchJQL(jql)
	if err != nil {
		jql = fmt.Sprintf(`project = %s AND parent = %s ORDER BY updated DESC`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, _ = client.SearchJQL(jql)
	}

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

func truncate(s string, length int) string {
	if len(s) <= length {
		return s
	}
	return s[:length-2] + ".."
}

// JIRA management commands
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA ticket management",
	Long: `Commands for managing JIRA tickets directly via the REST API:
  create        - Create a new ticket
  update        - Update an existing ticket
  get           - Get ticket details
  transition    - Move ticket to a new status
  search        - Search tickets using JQL
  missing-times - List done tickets missing cycle time data
  types         - List available issue types
  priorities    - List available priorities
  projects      - List available JIRA projects`,
}

var jiraCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new JIRA ticket",
	Long: `Create a new JIRA ticket with the specified details.

Examples:
  forecast jira create --summary "Fix login bug" --type Bug --priority High
  forecast jira create --summary "Add dark mode" --type Story --labels ui,feature
  forecast jira create --summary "Security audit" --type Task --assignee dev@company.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		summary, _ := cmd.Flags().GetString("summary")
		issueType, _ := cmd.Flags().GetString("type")
		description, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetString("priority")
		labelsStr, _ := cmd.Flags().GetString("labels")
		assignee, _ := cmd.Flags().GetString("assignee")
		epic, _ := cmd.Flags().GetString("epic")

		var labels []string
		if labelsStr != "" {
			for _, l := range splitAndTrim(labelsStr, ",") {
				if l != "" {
					labels = append(labels, l)
				}
			}
		}

		return runJiraCreate(summary, issueType, description, priority, labels, assignee, epic)
	},
}

var jiraUpdateCmd = &cobra.Command{
	Use:   "update [issue-key]",
	Short: "Update an existing JIRA ticket",
	Long: `Update an existing JIRA ticket with new values.

Examples:
  forecast jira update SMG-1234 --priority Highest
  forecast jira update SMG-1234 --labels security,urgent
  forecast jira update SMG-1234 --assignee dev@company.com`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		summary, _ := cmd.Flags().GetString("summary")
		description, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetString("priority")
		labelsStr, _ := cmd.Flags().GetString("labels")
		assignee, _ := cmd.Flags().GetString("assignee")
		epic, _ := cmd.Flags().GetString("epic")

		var labels []string
		if labelsStr != "" {
			for _, l := range splitAndTrim(labelsStr, ",") {
				labels = append(labels, l)
			}
		}

		return runJiraUpdate(issueKey, summary, description, priority, labels, assignee, epic)
	},
}

var jiraGetCmd = &cobra.Command{
	Use:   "get [issue-key]",
	Short: "Get JIRA ticket details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraGet(args[0])
	},
}

var jiraTransitionCmd = &cobra.Command{
	Use:   "transition [issue-key]",
	Short: "Transition a ticket to a new status",
	Long: `Move a JIRA ticket to a new status.

Examples:
  forecast jira transition SMG-1234 --to "In Development"
  forecast jira transition SMG-1234 --to Done --comment "Completed the work"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		issueKey := args[0]
		to, _ := cmd.Flags().GetString("to")
		comment, _ := cmd.Flags().GetString("comment")
		return runJiraTransition(issueKey, to, comment)
	},
}

var jiraSearchCmd = &cobra.Command{
	Use:   "search [jql]",
	Short: "Search for tickets using JQL",
	Long: `Search for JIRA tickets using JQL query.

Examples:
  forecast jira search "project=SMG AND status='To Do'"
  forecast jira search "project=SMG AND priority=Highest" --limit 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		return runJiraSearch(args[0], limit)
	},
}

var jiraTransitionsCmd = &cobra.Command{
	Use:   "transitions [issue-key]",
	Short: "List available transitions for a ticket",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraTransitions(args[0])
	},
}

var jiraTypesCmd = &cobra.Command{
	Use:   "types",
	Short: "List available issue types",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraTypes()
	},
}

var jiraPrioritiesCmd = &cobra.Command{
	Use:   "priorities",
	Short: "List available priorities",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraPriorities()
	},
}

var jiraProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List available JIRA projects",
	Long: `List all JIRA projects accessible with the current credentials.
Useful for debugging configuration issues or finding project keys.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraProjects()
	},
}

var jiraFieldsCmd = &cobra.Command{
	Use:   "fields [issue-key]",
	Short: "List custom fields and their values for an issue",
	Long: `Shows all custom fields for a JIRA issue to help identify field IDs.
Useful for finding the story_points_field or cycle_time_field IDs.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runJiraFields(args[0])
	},
}

var jiraMissingTimesCmd = &cobra.Command{
	Use:   "missing-times",
	Short: "List done tickets missing cycle time data",
	Long: `List all tickets that are marked as Done but don't have cycle time data.
These tickets need manual time entry via JIRA's Time Spent field or a custom field.

Use --project to filter to a specific project/epic.
Use --fix to automatically log time based on story points (requires story_points_field in config).
Use --default-hours to specify hours for tickets without story points.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		fix, _ := cmd.Flags().GetBool("fix")
		hoursPerPoint, _ := cmd.Flags().GetFloat64("hours-per-point")
		defaultHours, _ := cmd.Flags().GetFloat64("default-hours")
		return runJiraMissingTimes(project, fix, hoursPerPoint, defaultHours)
	},
}

func init() {
	// Create command flags
	jiraCreateCmd.Flags().StringP("summary", "s", "", "Issue summary (required)")
	jiraCreateCmd.Flags().StringP("type", "t", "Task", "Issue type (Bug, Story, Task, Epic)")
	jiraCreateCmd.Flags().StringP("description", "d", "", "Issue description")
	jiraCreateCmd.Flags().StringP("priority", "p", "Medium", "Priority (Highest, High, Medium, Low, Lowest)")
	jiraCreateCmd.Flags().StringP("labels", "l", "", "Comma-separated labels")
	jiraCreateCmd.Flags().StringP("assignee", "a", "", "Assignee email")
	jiraCreateCmd.Flags().StringP("epic", "e", "", "Parent epic key")
	jiraCreateCmd.MarkFlagRequired("summary")

	// Update command flags
	jiraUpdateCmd.Flags().StringP("summary", "s", "", "New summary")
	jiraUpdateCmd.Flags().StringP("description", "d", "", "New description")
	jiraUpdateCmd.Flags().StringP("priority", "p", "", "New priority")
	jiraUpdateCmd.Flags().StringP("labels", "l", "", "New labels (comma-separated)")
	jiraUpdateCmd.Flags().StringP("assignee", "a", "", "New assignee email")
	jiraUpdateCmd.Flags().StringP("epic", "e", "", "Parent epic key")

	// Transition command flags
	jiraTransitionCmd.Flags().String("to", "", "Target status (required)")
	jiraTransitionCmd.Flags().StringP("comment", "c", "", "Comment to add with transition")
	jiraTransitionCmd.MarkFlagRequired("to")

	// Search command flags
	jiraSearchCmd.Flags().Int("limit", 20, "Maximum results to show")

	// Missing times command flags
	jiraMissingTimesCmd.Flags().StringP("project", "p", "", "Filter to specific project (by key or epic)")
	jiraMissingTimesCmd.Flags().Bool("fix", false, "Automatically log time based on story points")
	jiraMissingTimesCmd.Flags().Float64("hours-per-point", 4.0, "Hours per story point for --fix")
	jiraMissingTimesCmd.Flags().Float64("default-hours", 8.0, "Default hours for tickets without story points")

	// Add subcommands to jira command
	jiraCmd.AddCommand(jiraCreateCmd)
	jiraCmd.AddCommand(jiraUpdateCmd)
	jiraCmd.AddCommand(jiraGetCmd)
	jiraCmd.AddCommand(jiraTransitionCmd)
	jiraCmd.AddCommand(jiraSearchCmd)
	jiraCmd.AddCommand(jiraTransitionsCmd)
	jiraCmd.AddCommand(jiraTypesCmd)
	jiraCmd.AddCommand(jiraPrioritiesCmd)
	jiraCmd.AddCommand(jiraProjectsCmd)
	jiraCmd.AddCommand(jiraFieldsCmd)
	jiraCmd.AddCommand(jiraMissingTimesCmd)
}

// Helper function to create JIRA client from config
func getJiraClient() (*jira.Client, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, fmt.Errorf("failed to load config - run 'forecast init' first")
	}
	return jira.NewClient(&cfg.JIRA), nil
}

// Helper to split and trim strings
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func runJiraCreate(summary, issueType, description, priority string, labels []string, assignee, epic string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	cfg := config.Get()
	projectKey := cfg.JIRA.ProjectKey

	req := jira.CreateIssueRequest{
		Summary:     summary,
		IssueType:   issueType,
		Description: description,
		Priority:    priority,
		Labels:      labels,
		Assignee:    assignee,
		Epic:        epic,
		Project:     projectKey,
	}

	result, err := client.CreateIssue(req)
	if err != nil {
		return fmt.Errorf("failed to create issue: %w", err)
	}

	fmt.Printf("Created: %s\n", result.Key)
	fmt.Printf("URL: %s/browse/%s\n", cfg.JIRA.URL, result.Key)
	return nil
}

func runJiraUpdate(issueKey, summary, description, priority string, labels []string, assignee, epic string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	req := jira.UpdateIssueRequest{}

	if summary != "" {
		req.Summary = &summary
	}
	if description != "" {
		req.Description = &description
	}
	if priority != "" {
		req.Priority = &priority
	}
	if len(labels) > 0 {
		req.Labels = labels
	}
	if assignee != "" {
		req.Assignee = &assignee
	}
	if epic != "" {
		req.Epic = &epic
	}

	if err := client.UpdateIssue(issueKey, req); err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}

	fmt.Printf("Updated: %s\n", issueKey)
	return nil
}

func runJiraGet(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	fmt.Printf("\n%s: %s\n", issue.Key, issue.Fields.Summary)
	fmt.Printf("  Status:   %s\n", issue.Fields.Status.Name)
	fmt.Printf("  Type:     %s\n", issue.Fields.IssueType.Name)
	if issue.Fields.Assignee != nil {
		fmt.Printf("  Assignee: %s\n", issue.Fields.Assignee.DisplayName)
	} else {
		fmt.Printf("  Assignee: Unassigned\n")
	}
	if len(issue.Fields.Labels) > 0 {
		fmt.Printf("  Labels:   %v\n", issue.Fields.Labels)
	}
	fmt.Printf("  Created:  %s\n", issue.Fields.Created[:10])
	fmt.Printf("  Updated:  %s\n", issue.Fields.Updated[:10])

	return nil
}

func runJiraTransition(issueKey, to, comment string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	if err := client.TransitionIssue(issueKey, to, comment); err != nil {
		return fmt.Errorf("failed to transition issue: %w", err)
	}

	fmt.Printf("Transitioned %s to '%s'\n", issueKey, to)
	return nil
}

func runJiraSearch(jql string, limit int) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	issues, err := client.SearchJQL(jql)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	fmt.Printf("Found %d issues\n\n", len(issues))

	displayed := 0
	for _, issue := range issues {
		if displayed >= limit {
			break
		}

		status := issue.Fields.Status.Name
		summary := issue.Fields.Summary
		if len(summary) > 50 {
			summary = summary[:50] + "..."
		}

		fmt.Printf("%-12s | %-20s | %s\n", issue.Key, status, summary)
		displayed++
	}

	if len(issues) > limit {
		fmt.Printf("\n... and %d more (use --limit to see more)\n", len(issues)-limit)
	}

	return nil
}

func runJiraTransitions(issueKey string) error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	transitions, err := client.GetTransitions(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	fmt.Printf("Available transitions for %s:\n\n", issueKey)
	for _, t := range transitions {
		fmt.Printf("  %-25s -> %s\n", t.Name, t.To.Name)
	}

	return nil
}

func runJiraTypes() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	cfg := config.Get()
	types, err := client.GetIssueTypes(cfg.JIRA.ProjectKey)
	if err != nil {
		return fmt.Errorf("failed to get issue types: %w", err)
	}

	fmt.Println("Available issue types:")
	for name, id := range types {
		fmt.Printf("  %-20s (ID: %s)\n", name, id)
	}

	return nil
}

func runJiraPriorities() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	priorities, err := client.GetPriorities()
	if err != nil {
		return fmt.Errorf("failed to get priorities: %w", err)
	}

	fmt.Println("Available priorities:")
	for name, id := range priorities {
		fmt.Printf("  %-15s (ID: %s)\n", name, id)
	}

	return nil
}

func runJiraProjects() error {
	client, err := getJiraClient()
	if err != nil {
		return err
	}

	projects, err := client.GetProjects()
	if err != nil {
		return fmt.Errorf("failed to get projects: %w", err)
	}

	fmt.Println("Available JIRA projects:")
	fmt.Printf("  %-12s %-40s %s\n", "Key", "Name", "ID")
	fmt.Println("  " + strings.Repeat("-", 60))
	for _, p := range projects {
		name := p.Name
		if len(name) > 40 {
			name = name[:37] + "..."
		}
		fmt.Printf("  %-12s %-40s %s\n", p.Key, name, p.ID)
	}

	cfg := config.Get()
	fmt.Printf("\nConfigured project_key: %s\n", cfg.JIRA.ProjectKey)

	return nil
}

func runJiraFields(issueKey string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	client := jira.NewClient(&cfg.JIRA)

	// Get field definitions to show names
	fieldDefs, err := client.GetFields()
	if err != nil {
		fmt.Printf("Warning: couldn't get field definitions: %v\n", err)
	}
	fieldNames := make(map[string]string)
	for _, f := range fieldDefs {
		fieldNames[f.ID] = f.Name
	}

	// Fetch the issue with all fields
	jql := fmt.Sprintf(`key = %s`, issueKey)
	issues, err := client.SearchWithAllFields(jql)
	if err != nil {
		return fmt.Errorf("failed to fetch issue: %w", err)
	}

	if len(issues) == 0 {
		return fmt.Errorf("issue %s not found", issueKey)
	}

	issue := issues[0]

	fmt.Printf("\nCustom fields for %s:\n", issueKey)
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")
	fmt.Printf("%-25s %-25s %-10s %s\n", "Field ID", "Name", "Type", "Value")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

	// Look for story points and other interesting fields
	for fieldID, value := range issue.RawFields {
		if strings.HasPrefix(fieldID, "customfield_") && value != nil {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 25 {
				valueStr = valueStr[:22] + "..."
			}

			name := fieldNames[fieldID]
			if len(name) > 25 {
				name = name[:22] + "..."
			}

			// Detect type
			typeStr := "unknown"
			switch value.(type) {
			case float64:
				typeStr = "number"
			case string:
				typeStr = "string"
			case map[string]interface{}:
				typeStr = "object"
			case []interface{}:
				typeStr = "array"
			}

			fmt.Printf("%-25s %-25s %-10s %s\n", fieldID, name, typeStr, valueStr)
		}
	}

	// Show potential story points fields
	fmt.Println("\nPotential story points fields:")
	for _, f := range fieldDefs {
		nameLower := strings.ToLower(f.Name)
		if strings.Contains(nameLower, "story") || strings.Contains(nameLower, "point") || strings.Contains(nameLower, "estimate") {
			fmt.Printf("  %-25s %s\n", f.ID, f.Name)
		}
	}

	fmt.Println("\nTo use a field for story points, add to .forecast/config.yaml:")
	fmt.Println("  jira:")
	fmt.Println("    story_points_field: customfield_XXXXX")

	return nil
}

func runJiraMissingTimes(projectFilter string, fix bool, hoursPerPoint float64, defaultHours float64) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	// Warn if story points field not configured but fix is requested
	if fix && cfg.JIRA.StoryPointsField == "" {
		fmt.Printf("Note: story_points_field not configured, using default of %.0f hours per ticket\n", defaultHours)
	}

	client := jira.NewClient(&cfg.JIRA)

	// Determine which projects to check
	var projects []config.ProjectConfig
	if projectFilter != "" {
		proj := cfg.GetProject(projectFilter)
		if proj == nil {
			return fmt.Errorf("project '%s' not found in config", projectFilter)
		}
		projects = []config.ProjectConfig{*proj}
	} else {
		projects = cfg.GetAllProjects()
	}

	if len(projects) == 0 {
		fmt.Println("No projects configured.")
		return nil
	}

	// Include custom fields
	var extraFields []string
	if cfg.JIRA.CycleTimeField != "" {
		extraFields = append(extraFields, cfg.JIRA.CycleTimeField)
	}
	if cfg.JIRA.StoryPointsField != "" {
		extraFields = append(extraFields, cfg.JIRA.StoryPointsField)
	}

	if fix {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  LOGGING TIME FOR MISSING TICKETS")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("\nUsing %v hours per story point\n", hoursPerPoint)
	} else {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("  TICKETS MISSING CYCLE TIME DATA")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("\nThese tickets are Done but have no cycle time from workflow transitions or time tracking.")
		fmt.Println("Please log time in JIRA or update the custom cycle time field.")
	}

	totalMissing := 0
	totalFixed := 0

	for _, proj := range projects {
		// Build JQL for done tickets in this epic
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s AND status = Done`, cfg.JIRA.ProjectKey, proj.Epic)

		issues, err := client.SearchJQL(jql)
		if err != nil {
			// Try parent field for next-gen projects
			jql = fmt.Sprintf(`project = %s AND parent = %s AND status = Done`, cfg.JIRA.ProjectKey, proj.Epic)
			issues, err = client.SearchJQL(jql)
			if err != nil {
				fmt.Printf("Error fetching %s: %v\n", proj.Name, err)
				continue
			}
		}

		// Build maps for lookup
		issueMap := make(map[string]*jira.Changelog)
		rawFieldsMap := make(map[string]map[string]interface{})
		for i := range issues {
			issueMap[issues[i].Key] = issues[i].Changelog
			rawFieldsMap[issues[i].Key] = issues[i].RawFields
		}

		// Convert to items to calculate cycle times
		items := client.ConvertIssuesToItems(issues, cfg)

		// Filter for items missing cycle time
		type missingItem struct {
			key        string
			summary    string
			closedBy   string
			rawFields  map[string]interface{}
		}
		var missing []missingItem
		for _, item := range items {
			if item.CycleTime == 0 {
				closedBy := ""
				if changelog, ok := issueMap[item.JiraKey]; ok {
					closedBy = client.ExtractClosedBy(changelog)
				}
				missing = append(missing, missingItem{
					key:       item.JiraKey,
					summary:   item.Description,
					closedBy:  closedBy,
					rawFields: rawFieldsMap[item.JiraKey],
				})
			}
		}

		if len(missing) > 0 {
			fmt.Printf("\n## %s (%d missing)\n", proj.Name, len(missing))
			fmt.Println("────────────────────────────────────────────────────────────────────────────────")
			for _, m := range missing {
				summary := m.summary
				if len(summary) > 40 {
					summary = summary[:37] + "..."
				}
				closedByStr := m.closedBy
				if closedByStr == "" {
					closedByStr = "(unknown)"
				}

				if fix {
					// Get story points and log time
					storyPoints := jira.GetStoryPoints(m.rawFields, cfg.JIRA.StoryPointsField)
					var hours float64
					var comment string
					if storyPoints > 0 {
						hours = storyPoints * hoursPerPoint
						comment = fmt.Sprintf("Retroactive time log: %.0f story points × %.0f hours = %.0f hours", storyPoints, hoursPerPoint, hours)
					} else {
						// Use default hours for tickets without story points
						hours = defaultHours
						comment = fmt.Sprintf("Retroactive time log: default %.0f hours (no story points)", hours)
					}

					seconds := int(hours * 3600)
					err := client.LogWork(m.key, seconds, comment)
					if err != nil {
						fmt.Printf("  %-12s %-42s [%s] ✗ Error: %v\n", m.key, summary, closedByStr, err)
					} else {
						if storyPoints > 0 {
							fmt.Printf("  %-12s %-42s [%s] ✓ Logged %.0fh (%.0f pts)\n", m.key, summary, closedByStr, hours, storyPoints)
						} else {
							fmt.Printf("  %-12s %-42s [%s] ✓ Logged %.0fh (default)\n", m.key, summary, closedByStr, hours)
						}
						totalFixed++
					}
				} else {
					fmt.Printf("  %-12s %-52s [%s]\n", m.key, summary, closedByStr)
					fmt.Printf("               %s/browse/%s\n", cfg.JIRA.URL, m.key)
				}
			}
			totalMissing += len(missing)
		}
	}

	fmt.Println()
	if totalMissing == 0 {
		fmt.Println("✓ All done tickets have cycle time data!")
	} else if fix {
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("Summary: %d tickets processed\n", totalMissing)
		fmt.Printf("  ✓ Fixed:   %d\n", totalFixed)
		fmt.Printf("  ✗ Errors:  %d\n", totalMissing-totalFixed)
		if totalFixed > 0 {
			fmt.Println("\nTime has been logged. Run 'forecast dashboard' to see updated forecasts.")
		}
	} else {
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Printf("Total: %d tickets need time data\n\n", totalMissing)
		fmt.Println("To add time in JIRA:")
		fmt.Println("  1. Open the ticket")
		fmt.Println("  2. Click 'Log work' or update the Time Spent field")
		fmt.Println("  3. Or set a custom cycle time field if configured")
		fmt.Println("\nOr use --fix to automatically log time based on story points:")
		fmt.Printf("  forecast jira missing-times --fix --default-hours %.0f\n", defaultHours)
	}

	return nil
}
