package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrewcostello/forecast/internal/config"
	appcontext "github.com/andrewcostello/forecast/internal/context"
	apperrors "github.com/andrewcostello/forecast/internal/errors"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/andrewcostello/forecast/internal/storage"
	yamlparser "github.com/andrewcostello/forecast/internal/yaml"
	"github.com/andrewcostello/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync data from JIRA or YAML files",
	Long: `Pulls latest issue data from JIRA including:
  - Issue status
  - Cycle times (Created → Done)
  - Item types and sizes
  - Assignees

Use --project to sync a specific project, or sync all configured projects.
If no project specified, uses current context (set with 'forecast use').

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
		jsonOutput, _ := cmd.Flags().GetBool("json")

		if file != "" {
			return runSyncFromFile(file, dryRun, jsonOutput)
		}
		return runSync(project, jsonOutput)
	},
}

func init() {
	syncCmd.Flags().StringP("project", "p", "", "Sync specific project (by key or epic), or all if omitted")
	syncCmd.Flags().StringP("file", "f", "", "Sync tasks from a YAML file (bidirectional)")
	syncCmd.Flags().Bool("dry-run", false, "Preview what would be created without making changes")
	syncCmd.Flags().Bool("json", false, "Output results as JSON")
}

// SyncResult holds sync operation results for JSON output
type SyncResult struct {
	Projects    []ProjectSyncResult `json:"projects"`
	TotalItems  int                 `json:"totalItems"`
	SyncedAt    time.Time           `json:"syncedAt"`
}

// ProjectSyncResult holds results for a single project sync
type ProjectSyncResult struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Epic      string `json:"epic"`
	ItemCount int    `json:"itemCount"`
	Error     string `json:"error,omitempty"`
}

// FileSyncResult holds results for YAML file sync
type FileSyncResult struct {
	FilePath  string    `json:"filePath"`
	Project   string    `json:"project"`
	Epic      string    `json:"epic"`
	Created   int       `json:"created"`
	Updated   int       `json:"updated"`
	Skipped   int       `json:"skipped"`
	DryRun    bool      `json:"dryRun"`
	SyncedAt  time.Time `json:"syncedAt"`
}

func runSync(projectFilter string, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	// Resolve project from context if not specified
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

	// Create JIRA client
	jiraClient := jira.NewClient(&cfg.JIRA)
	store := storage.New(".forecast")

	// Get projects to sync
	projects := cfg.GetAllProjects()

	// For JSON output, collect results
	result := SyncResult{
		Projects: []ProjectSyncResult{},
		SyncedAt: time.Now(),
	}

	// If no projects configured, use legacy single-epic mode
	if len(projects) == 0 {
		if !jsonOutput {
			fmt.Println("Syncing from JIRA (legacy mode)...")
		}
		items, err := jiraClient.FetchIssues(cfg)
		if err != nil {
			return apperrors.JIRAConnectionError(err)
		}
		if !jsonOutput {
			fmt.Printf("Fetched %d items from JIRA\n", len(items))
		}
		if err := store.Save(items); err != nil {
			return apperrors.FileWriteError(".forecast/data.json", err)
		}
		if jsonOutput {
			result.TotalItems = len(items)
			result.Projects = append(result.Projects, ProjectSyncResult{
				Key:       "default",
				Name:      "Default",
				ItemCount: len(items),
			})
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
		} else {
			fmt.Println("Sync complete")
		}
		appcontext.SetLastSync(time.Now())
		return nil
	}

	// Filter to specific project if requested
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
		projects = []config.ProjectConfig{*proj}
	}

	// Sync each project
	totalItems := 0
	for _, proj := range projects {
		if !jsonOutput {
			fmt.Printf("Syncing %s (%s)...\n", proj.Name, proj.Epic)
		}

		projResult := ProjectSyncResult{
			Key:  proj.Key,
			Name: proj.Name,
			Epic: proj.Epic,
		}

		// Build JQL for this project's epic
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, cfg.JIRA.ProjectKey, proj.Epic)
		issues, err := jiraClient.SearchJQL(jql)
		if err != nil {
			// Try parent field for next-gen projects
			jql = fmt.Sprintf(`project = %s AND parent = %s`, cfg.JIRA.ProjectKey, proj.Epic)
			issues, err = jiraClient.SearchJQL(jql)
			if err != nil {
				if !jsonOutput {
					fmt.Printf("  Warning: failed to fetch %s: %v\n", proj.Epic, err)
				}
				projResult.Error = err.Error()
				result.Projects = append(result.Projects, projResult)
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
			return apperrors.FileWriteError(fmt.Sprintf(".forecast/data-%s.json", projectKey), err)
		}

		projResult.ItemCount = len(items)
		result.Projects = append(result.Projects, projResult)

		if !jsonOutput {
			fmt.Printf("  Fetched %d items\n", len(items))
		}
		totalItems += len(items)
	}

	result.TotalItems = totalItems

	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("\nSync complete: %d total items across %d projects\n", totalItems, len(projects))
	}

	// Record last sync time
	appcontext.SetLastSync(time.Now())

	return nil
}

func runSyncFromFile(filePath string, dryRun bool, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	// Parse YAML file
	tf, err := yamlparser.ParseTaskFile(filePath)
	if err != nil {
		return apperrors.WrapWithSuggestion(err,
			fmt.Sprintf("Failed to parse task file: %s", filePath),
			"Check that the file exists and is valid YAML",
		)
	}

	if !jsonOutput {
		fmt.Printf("Loaded %d tasks from %s\n", len(tf.Tasks), filePath)
	}

	// Get JIRA instance config (from yaml jira_instance field or default)
	jiraCfg := cfg.GetJIRAInstance(tf.JiraInstance)
	if tf.JiraInstance != "" && !jsonOutput {
		fmt.Printf("Using JIRA instance: %s (%s)\n", tf.JiraInstance, jiraCfg.URL)
	}

	// Use project key from JIRA config if YAML project matches, otherwise use YAML project
	projectKey := tf.Project
	if jiraCfg.ProjectKey != "" {
		projectKey = jiraCfg.ProjectKey
	}

	if !jsonOutput {
		fmt.Printf("Project: %s, Epic: %s\n\n", projectKey, tf.Epic)
	}

	// Create JIRA client with the selected instance
	jiraClient := jira.NewClient(jiraCfg)

	var created, updated, skipped int
	var modified bool

	for i := range tf.Tasks {
		task := &tf.Tasks[i]

		if task.JiraKey != "" {
			// Task already has JIRA key - fetch status
			if !jsonOutput {
				fmt.Printf("  [%s] %s -> fetching status from %s...\n", task.Key, truncate(task.Summary, 40), task.JiraKey)
			}

			if !dryRun {
				issue, err := jiraClient.GetIssue(task.JiraKey)
				if err != nil {
					if !jsonOutput {
						fmt.Printf("    Warning: failed to fetch %s: %v\n", task.JiraKey, err)
					}
					skipped++
					continue
				}

				if task.Status != issue.Fields.Status.Name {
					task.Status = issue.Fields.Status.Name
					modified = true
					if !jsonOutput {
						fmt.Printf("    Status: %s\n", task.Status)
					}
				}
			}
			updated++
		} else {
			// No JIRA key - create new ticket
			if !jsonOutput {
				fmt.Printf("  [%s] %s -> creating in JIRA...\n", task.Key, truncate(task.Summary, 40))
			}

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
					if !jsonOutput {
						fmt.Printf("    Error: %v\n", err)
					}
					skipped++
					continue
				}

				task.JiraKey = result.Key
				task.Status = "To Do"
				modified = true
				if !jsonOutput {
					fmt.Printf("    Created: %s\n", result.Key)
				}
			} else if !jsonOutput {
				fmt.Printf("    Would create: %s: %s\n", task.Key, task.Summary)
			}
			created++
		}
	}

	// Save updated YAML file
	if modified && !dryRun {
		if err := tf.Save(); err != nil {
			return apperrors.FileWriteError(filePath, err)
		}
		if !jsonOutput {
			fmt.Printf("\nUpdated %s with JIRA keys\n", filePath)
		}
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
			if !jsonOutput {
				fmt.Printf("Warning: failed to save to local storage: %v\n", err)
			}
		} else if !jsonOutput {
			fmt.Printf("Saved %d items to .forecast/data-%s.json\n", len(items), storageKey)
		}
	}

	// Output results
	if jsonOutput {
		result := FileSyncResult{
			FilePath: filePath,
			Project:  projectKey,
			Epic:     tf.Epic,
			Created:  created,
			Updated:  updated,
			Skipped:  skipped,
			DryRun:   dryRun,
			SyncedAt: time.Now(),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("\n-----------------------------------------------------------\n")
		fmt.Printf("Summary: %d created, %d updated, %d skipped\n", created, updated, skipped)

		if dryRun {
			fmt.Println("\n(dry-run mode - no changes were made)")
		}
	}

	return nil
}
