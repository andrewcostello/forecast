package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/referenceclass"
	"github.com/andrewcostello/forecast/internal/report"
	"github.com/andrewcostello/forecast/internal/storage"
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
	Short: "Sync data from JIRA",
	Long: `Pulls latest issue data from JIRA including:
  - Issue status
  - Cycle times (Created → Done)
  - Item types and sizes
  - Assignees`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync()
	},
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run Monte Carlo simulation",
	Long: `Executes Monte Carlo simulation to forecast project completion.
Uses actual cycle time data to predict completion dates with confidence levels.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		confidence, _ := cmd.Flags().GetIntSlice("confidence")
		iterations, _ := cmd.Flags().GetInt("iterations")
		return runMonteCarlo(confidence, iterations)
	},
}

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate forecast report",
	Long: `Generates a detailed report with:
  - Earned Value Analysis metrics
  - Monte Carlo forecast results
  - Throughput trends
  - Reference class comparisons`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reportType, _ := cmd.Flags().GetString("type")
		return runReport(reportType)
	},
}

var referenceClassCmd = &cobra.Command{
	Use:   "reference-class",
	Short: "Manage reference class database",
	Long: `Commands for managing historical project data:
  list - Show available reference classes
  add  - Add completed project to reference database`,
}

func init() {
	runCmd.Flags().IntSlice("confidence", []int{50, 70, 85, 95}, "Confidence levels for forecast")
	runCmd.Flags().Int("iterations", 10000, "Number of Monte Carlo iterations")

	reportCmd.Flags().String("type", "eva", "Report type: eva, montecarlo, full")

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

func runSync() error {
	fmt.Println("Syncing from JIRA...")

	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config")
	}

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)

	// Fetch issues
	items, err := jiraClient.FetchIssues(cfg)
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}

	fmt.Printf("Fetched %d items from JIRA\n", len(items))

	// Save to storage
	store := storage.New(".forecast")
	if err := store.Save(items); err != nil {
		return fmt.Errorf("failed to save items: %w", err)
	}

	fmt.Println("✓ Sync complete")
	return nil
}

func runMonteCarlo(confidence []int, iterations int) error {
	fmt.Printf("Running Monte Carlo simulation (%d iterations)...\n", iterations)

	// Load items
	store := storage.New(".forecast")
	items, err := store.Load()
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

	// Get team capacity from config, default to 8 hours/day
	teamCapacity := 8.0
	cfg := config.Get()
	if cfg != nil && cfg.TeamCapacity > 0 {
		teamCapacity = cfg.TeamCapacity
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

func runReport(reportType string) error {
	// Load items
	store := storage.New(".forecast")
	items, err := store.Load()
	if err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("no items found - run 'forecast sync' first")
	}

	// Get config for project settings
	cfg := config.Get()
	projectName := "Project"
	teamCapacity := 8.0
	var startDate time.Time

	if cfg != nil {
		if cfg.ProjectName != "" {
			projectName = cfg.ProjectName
		}
		if cfg.TeamCapacity > 0 {
			teamCapacity = cfg.TeamCapacity
		}
	}

	// Estimate project start from earliest item
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

// JIRA management commands
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA ticket management",
	Long: `Commands for managing JIRA tickets directly via the REST API:
  create     - Create a new ticket
  update     - Update an existing ticket
  get        - Get ticket details
  transition - Move ticket to a new status
  search     - Search tickets using JQL
  types      - List available issue types
  priorities - List available priorities`,
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

		var labels []string
		if labelsStr != "" {
			for _, l := range splitAndTrim(labelsStr, ",") {
				labels = append(labels, l)
			}
		}

		return runJiraUpdate(issueKey, summary, description, priority, labels, assignee)
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

	// Transition command flags
	jiraTransitionCmd.Flags().String("to", "", "Target status (required)")
	jiraTransitionCmd.Flags().StringP("comment", "c", "", "Comment to add with transition")
	jiraTransitionCmd.MarkFlagRequired("to")

	// Search command flags
	jiraSearchCmd.Flags().Int("limit", 20, "Maximum results to show")

	// Add subcommands to jira command
	jiraCmd.AddCommand(jiraCreateCmd)
	jiraCmd.AddCommand(jiraUpdateCmd)
	jiraCmd.AddCommand(jiraGetCmd)
	jiraCmd.AddCommand(jiraTransitionCmd)
	jiraCmd.AddCommand(jiraSearchCmd)
	jiraCmd.AddCommand(jiraTransitionsCmd)
	jiraCmd.AddCommand(jiraTypesCmd)
	jiraCmd.AddCommand(jiraPrioritiesCmd)
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

func runJiraUpdate(issueKey, summary, description, priority string, labels []string, assignee string) error {
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

	fmt.Println("Available issue types:\n")
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

	fmt.Println("Available priorities:\n")
	for name, id := range priorities {
		fmt.Printf("  %-15s (ID: %s)\n", name, id)
	}

	return nil
}
