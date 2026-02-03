package main

import (
	"fmt"
	"strings"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/spf13/cobra"
)

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
			key       string
			summary   string
			closedBy  string
			rawFields map[string]interface{}
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
