package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/andrewcostello/forecast/internal/terminal"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize forecasting for a project",
	Long: `Creates a .forecast directory with configuration files.

Interactive setup will guide you through:
  - JIRA connection settings
  - Project configuration
  - Auto-discovery of custom fields

Use --non-interactive to create a template config file instead.
Use --source=yaml to scaffold a yaml-authoritative project (no JIRA).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		source, _ := cmd.Flags().GetString("source")
		if strings.EqualFold(source, "yaml") {
			return runInitYAML()
		}
		if nonInteractive {
			return runInitNonInteractive()
		}
		return runInitInteractive()
	},
}

func init() {
	initCmd.Flags().Bool("non-interactive", false, "Create template config without prompts")
	initCmd.Flags().String("source", "jira", "Item source: jira | yaml")
}

func runInitInteractive() error {
	terminal.PrintHeader("Forecast Setup")

	// Check if config already exists
	configPath := ".forecast/config.yaml"
	if _, err := os.Stat(configPath); err == nil {
		terminal.PrintWarning("Config already exists at %s", configPath)
		p := terminal.NewPrompter()
		overwrite, err := p.PromptConfirm("Overwrite existing config?", false)
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Println("Aborted.")
			return nil
		}
	}

	p := terminal.NewPrompter()

	// JIRA Configuration
	terminal.PrintHeader("JIRA Connection")

	jiraURL, err := p.Prompt("JIRA URL (e.g., https://company.atlassian.net)", "")
	if err != nil {
		return err
	}
	if jiraURL == "" {
		return fmt.Errorf("JIRA URL is required")
	}
	// Normalize URL
	jiraURL = strings.TrimSuffix(jiraURL, "/")
	if !strings.HasPrefix(jiraURL, "http") {
		jiraURL = "https://" + jiraURL
	}

	email, err := p.PromptRequired("JIRA email")
	if err != nil {
		return err
	}

	fmt.Println("\nYou can create an API token at:")
	fmt.Printf("  %s/secure/ViewProfile.jspa (API tokens section)\n", jiraURL)
	fmt.Println("Or: https://id.atlassian.com/manage-profile/security/api-tokens")
	fmt.Println()

	apiToken, err := p.PromptSecret("JIRA API token")
	if err != nil {
		return err
	}
	if apiToken == "" {
		return fmt.Errorf("API token is required")
	}

	// Test connection
	terminal.PrintInfo("Testing JIRA connection...")

	testConfig := &config.JIRAConfig{
		URL:      jiraURL,
		Email:    email,
		APIToken: apiToken,
	}
	client := jira.NewClient(testConfig)

	projects, err := client.GetProjects()
	if err != nil {
		terminal.PrintError("Failed to connect to JIRA: %v", err)
		return fmt.Errorf("connection failed - check URL, email, and API token")
	}
	terminal.PrintSuccess("Connected to JIRA! Found %d accessible projects.", len(projects))

	// Select project
	terminal.PrintHeader("Project Selection")

	projectOptions := make([]string, 0, len(projects))
	projectKeys := make([]string, 0, len(projects))
	for _, proj := range projects {
		projectOptions = append(projectOptions, fmt.Sprintf("%s - %s", proj.Key, proj.Name))
		projectKeys = append(projectKeys, proj.Key)
	}

	_, selectedProject, err := p.PromptSelect("Select your JIRA project", projectOptions, 0)
	if err != nil {
		return err
	}

	// Extract project key from selection
	projectKey := strings.Split(selectedProject, " - ")[0]

	// Update test config with project key
	testConfig.ProjectKey = projectKey

	// Search for custom fields
	terminal.PrintHeader("Custom Fields Discovery")
	terminal.PrintInfo("Searching for story points and cycle time fields...")

	fields, err := client.GetFields()
	storyPointsField := ""
	cycleTimeField := ""

	if err == nil {
		// Find potential story points fields
		var storyPointOptions []string
		var storyPointIDs []string
		for _, f := range fields {
			nameLower := strings.ToLower(f.Name)
			if strings.Contains(nameLower, "story") && strings.Contains(nameLower, "point") {
				storyPointOptions = append(storyPointOptions, fmt.Sprintf("%s (%s)", f.Name, f.ID))
				storyPointIDs = append(storyPointIDs, f.ID)
			}
		}

		if len(storyPointOptions) > 0 {
			storyPointOptions = append(storyPointOptions, "None / Skip")
			idx, _, err := p.PromptSelect("Story points field", storyPointOptions, 0)
			if err == nil && idx < len(storyPointIDs) {
				storyPointsField = storyPointIDs[idx]
				terminal.PrintSuccess("Using %s for story points", storyPointsField)
			}
		} else {
			terminal.PrintInfo("No story points field found (can be configured later)")
		}

		// Find potential cycle time fields
		var cycleTimeOptions []string
		var cycleTimeIDs []string
		for _, f := range fields {
			nameLower := strings.ToLower(f.Name)
			if strings.Contains(nameLower, "cycle") || strings.Contains(nameLower, "lead time") {
				cycleTimeOptions = append(cycleTimeOptions, fmt.Sprintf("%s (%s)", f.Name, f.ID))
				cycleTimeIDs = append(cycleTimeIDs, f.ID)
			}
		}

		if len(cycleTimeOptions) > 0 {
			cycleTimeOptions = append(cycleTimeOptions, "None / Skip (use workflow transitions)")
			idx, _, err := p.PromptSelect("Cycle time field (optional)", cycleTimeOptions, len(cycleTimeOptions)-1)
			if err == nil && idx < len(cycleTimeIDs) {
				cycleTimeField = cycleTimeIDs[idx]
				terminal.PrintSuccess("Using %s for cycle time", cycleTimeField)
			}
		}
	}

	// Project details
	terminal.PrintHeader("Project Details")

	projectName, err := p.Prompt("Project display name", projectKey)
	if err != nil {
		return err
	}

	teamCapacity, err := p.Prompt("Team capacity (hours/day)", "8")
	if err != nil {
		return err
	}

	// Epic selection (optional)
	terminal.PrintHeader("Epic Configuration (Optional)")
	fmt.Println("You can filter issues by an epic. Leave blank to include all project issues.")

	epic, err := p.Prompt("Epic key (e.g., PROJ-123)", "")
	if err != nil {
		return err
	}

	// Create directory and config
	terminal.PrintHeader("Creating Configuration")

	if err := os.MkdirAll(".forecast", 0755); err != nil {
		return fmt.Errorf("failed to create .forecast directory: %w", err)
	}

	// Build config content
	var sb strings.Builder
	sb.WriteString("# Forecast Configuration\n")
	sb.WriteString("# Generated by 'forecast init'\n\n")
	sb.WriteString(fmt.Sprintf("project_name: \"%s\"\n", projectName))
	sb.WriteString(fmt.Sprintf("team_capacity: %s\n\n", teamCapacity))
	sb.WriteString("jira:\n")
	sb.WriteString(fmt.Sprintf("  url: %s\n", jiraURL))
	sb.WriteString(fmt.Sprintf("  email: %s\n", email))
	sb.WriteString("  api_token: ${JIRA_API_TOKEN}  # Set via environment variable for security\n")
	sb.WriteString(fmt.Sprintf("  project_key: %s\n", projectKey))
	if epic != "" {
		sb.WriteString(fmt.Sprintf("  epic: %s\n", epic))
	}
	if storyPointsField != "" {
		sb.WriteString(fmt.Sprintf("  story_points_field: %s\n", storyPointsField))
	}
	if cycleTimeField != "" {
		sb.WriteString(fmt.Sprintf("  cycle_time_field: %s\n", cycleTimeField))
	}

	// Add projects section if epic is specified
	if epic != "" {
		sb.WriteString("\n# Tracked projects/epics\n")
		sb.WriteString("projects:\n")
		sb.WriteString(fmt.Sprintf("  - name: \"%s\"\n", projectName))
		sb.WriteString(fmt.Sprintf("    key: \"%s\"\n", strings.ToLower(projectKey)))
		sb.WriteString(fmt.Sprintf("    epic: \"%s\"\n", epic))
	}

	// Write config
	if err := os.WriteFile(configPath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	terminal.PrintSuccess("Created %s", configPath)

	// Save API token instruction
	fmt.Println("\n--- Next Steps ---")
	fmt.Println("1. Set your JIRA API token as an environment variable:")
	fmt.Printf("   export JIRA_API_TOKEN='%s'\n", maskToken(apiToken))
	fmt.Println()
	fmt.Println("2. Sync data from JIRA:")
	fmt.Println("   forecast sync")
	fmt.Println()
	fmt.Println("3. Run a forecast:")
	fmt.Println("   forecast run")
	fmt.Println()

	// Offer to save token
	saveToken, err := p.PromptConfirm("Save API token to config file? (less secure but convenient)", false)
	if err != nil {
		return nil // Non-fatal
	}
	if saveToken {
		// Re-read and update config
		content, err := os.ReadFile(configPath)
		if err == nil {
			updated := strings.Replace(
				string(content),
				"api_token: ${JIRA_API_TOKEN}  # Set via environment variable for security",
				fmt.Sprintf("api_token: %s", apiToken),
				1,
			)
			os.WriteFile(configPath, []byte(updated), 0644)
			terminal.PrintSuccess("API token saved to config")
		}
	}

	return nil
}

func runInitNonInteractive() error {
	fmt.Println("Creating template configuration...")

	forecastDir := ".forecast"
	if err := os.MkdirAll(forecastDir, 0755); err != nil {
		return fmt.Errorf("failed to create .forecast directory: %w", err)
	}

	configPath := filepath.Join(forecastDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %s (use --force to overwrite)", configPath)
	}

	defaultConfig := `# Forecast Configuration
# See https://github.com/andrewcostello/forecast for documentation

# Project settings
project_name: "My Project"
team_capacity: 8  # Hours per day

jira:
  url: https://yourcompany.atlassian.net
  email: your.email@company.com
  api_token: ${JIRA_API_TOKEN}  # Set via environment variable
  project_key: PROJ
  # epic: PROJ-123  # Optional: filter to specific epic
  # story_points_field: customfield_10004  # Optional: custom field for story points
  # cycle_time_field: customfield_10001    # Optional: custom field for cycle time

# Multiple projects/epics can be tracked:
# projects:
#   - name: "Feature Alpha"
#     key: "alpha"
#     epic: "PROJ-100"
#   - name: "Feature Beta"
#     key: "beta"
#     epic: "PROJ-200"
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	terminal.PrintSuccess("Created template config at %s", configPath)
	fmt.Println("\nNext steps:")
	fmt.Println("1. Edit .forecast/config.yaml with your JIRA details")
	fmt.Println("2. Set JIRA_API_TOKEN environment variable")
	fmt.Println("3. Run 'forecast sync' to pull data from JIRA")

	return nil
}

// runInitYAML scaffolds a yaml-authoritative project: a minimal
// .forecast/config.yaml pointing at ../tasks.yaml, plus a starter tasks.yaml
// with one example task. No JIRA prompts; no network. Intentionally simple
// so the user can edit-and-go.
func runInitYAML() error {
	configPath := filepath.Join(".forecast", "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("config already exists at %s", configPath)
	}
	if err := os.MkdirAll(".forecast", 0755); err != nil {
		return fmt.Errorf("create .forecast: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	projectName := filepath.Base(wd)
	today := time.Now().Format("2006-01-02")
	target := time.Now().AddDate(0, 6, 0).Format("2006-01-02")

	cfg := fmt.Sprintf(`# Forecast configuration — yaml-authoritative mode.
# Tasks live in ../tasks.yaml; forecast reads them directly, no JIRA round-trip.

project_name: %q
team_capacity: 8

source:
  type: yaml
  path: ../tasks.yaml

reference_class:
  name: %q
  type: ""   # categorize for the reference-class database (e.g. "Go CLI", "React refactor")
`, projectName, projectName)

	if err := os.WriteFile(configPath, []byte(cfg), 0644); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}
	terminal.PrintSuccess("Created %s", configPath)

	tasksPath := "tasks.yaml"
	if _, err := os.Stat(tasksPath); err == nil {
		terminal.PrintInfo("Skipped %s (already exists)", tasksPath)
	} else {
		starter := fmt.Sprintf(`# %s tasks — authoritative source for forecast.
#
# Conventions:
#   status: todo | in_progress | done | blocked
#   size:   S | M | L | XL
#   dates:  YYYY-MM-DD
#
# When a task starts, set status: in_progress and started_at.
# When a task finishes, set status: done and completed_at — forecast computes
# cycle time from started_at → completed_at.

project: %s
reference_class: %q

milestones:
  - name: v1.0
    target: %s
    scope: "Initial release"

tasks:
  - key: TASK-1
    summary: "Example task — replace with real work"
    type: component
    size: M
    status: todo
    milestone: v1.0
    created_at: %s
`, projectName, projectName, projectName, target, today)

		if err := os.WriteFile(tasksPath, []byte(starter), 0644); err != nil {
			return fmt.Errorf("write %s: %w", tasksPath, err)
		}
		terminal.PrintSuccess("Created %s (starter)", tasksPath)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit tasks.yaml — add your real tasks and milestones")
	fmt.Println("  2. forecast sync          # load items from tasks.yaml")
	fmt.Println("  3. forecast run           # once you have done tasks with cycle time")
	return nil
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
