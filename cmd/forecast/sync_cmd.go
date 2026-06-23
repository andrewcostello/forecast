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

// projectKeyFromEpic extracts the JIRA project key from an epic key
// (e.g. "FSG-8348" → "FSG"). Falls back to fallback when epic is malformed.
func projectKeyFromEpic(epic, fallback string) string {
	for i := 0; i < len(epic); i++ {
		if epic[i] == '-' {
			if i == 0 {
				return fallback
			}
			return epic[:i]
		}
	}
	return fallback
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

	// Authoritative-yaml source: tasks.yaml is the system of record, no JIRA.
	if strings.EqualFold(cfg.Source.Type, "yaml") {
		return runSyncFromAuthoritativeYAML(cfg, jsonOutput)
	}

	// Resolve project from context if not specified
	if projectFilter == "" {
		projectFilter = appcontext.GetProject()
	}

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
		jiraClient := jira.NewClient(&cfg.JIRA)
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

	// Sync each project against its own JIRA instance.
	totalItems := 0
	for _, proj := range projects {
		inst := cfg.GetJIRAInstanceForProject(&proj)
		jiraClient := jira.NewClient(inst)

		if !jsonOutput {
			label := proj.Epic
			if proj.JIRAInstance != "" {
				label = fmt.Sprintf("%s @ %s", proj.Epic, proj.JIRAInstance)
			}
			fmt.Printf("Syncing %s (%s)...\n", proj.Name, label)
		}

		projResult := ProjectSyncResult{
			Key:  proj.Key,
			Name: proj.Name,
			Epic: proj.Epic,
		}

		// JQL is anchored on the project key the epic actually belongs to,
		// which lives on the resolved instance — not on cfg.JIRA.
		projectKeyForJQL := projectKeyFromEpic(proj.Epic, inst.ProjectKey)
		jql := fmt.Sprintf(`project = %s AND "Epic Link" = %s`, projectKeyForJQL, proj.Epic)
		issues, err := jiraClient.SearchJQL(jql)
		if err != nil {
			// Try parent field for next-gen projects
			jql = fmt.Sprintf(`project = %s AND parent = %s`, projectKeyForJQL, proj.Epic)
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

		// Convert JIRA issues to forecast items with cycle time, using the
		// project's instance config (story-points field, done statuses, etc.).
		instCfg := *cfg
		instCfg.JIRA = *inst
		items := jiraClient.ConvertIssuesToItems(issues, &instCfg)

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

// runSyncFromAuthoritativeYAML loads forecast items from yaml files. yaml is
// the system of record: status and timestamps come from yaml, cycle time is
// computed locally, and no JIRA call is made. Two layouts are supported:
//
//   - Single-file (cfg.Source.Path): one tasks file → .forecast/data.json
//   - Multi-project (cfg.Source.Projects): each entry → .forecast/data-{key}.json
//
// Projects wins if both are set.
func runSyncFromAuthoritativeYAML(cfg *config.Config, jsonOutput bool) error {
	if len(cfg.Source.Projects) > 0 {
		return runSyncFromMultiYAML(cfg, jsonOutput)
	}
	if cfg.Source.Path == "" {
		return fmt.Errorf("source.type=yaml requires source.path (single file) or source.projects (multi-file)")
	}
	path := config.ResolvePath(cfg.Source.Path)

	items, breakdown, err := loadYAMLItems(path)
	if err != nil {
		return err
	}

	store := storage.New(".forecast")
	if err := store.Save(items); err != nil {
		return fmt.Errorf("save items: %w", err)
	}

	if jsonOutput {
		result := SyncResult{
			Projects: []ProjectSyncResult{{
				ItemCount: len(items),
			}},
			TotalItems: len(items),
			SyncedAt:   time.Now(),
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("Loaded %d items from authoritative yaml: %s\n", len(items), path)
		printBreakdown(breakdown)
	}

	appcontext.SetLastSync(time.Now())
	return nil
}

// runSyncFromMultiYAML loads each yaml file in cfg.Source.Projects into its
// own data-{key}.json so the existing multi-project run/report flows can
// forecast each independently.
func runSyncFromMultiYAML(cfg *config.Config, jsonOutput bool) error {
	store := storage.New(".forecast")
	now := time.Now()
	result := SyncResult{SyncedAt: now}

	for _, proj := range cfg.Source.Projects {
		if proj.Key == "" {
			return fmt.Errorf("source.projects: key is required for every entry")
		}
		if proj.Path == "" {
			return fmt.Errorf("source.projects[%s]: path is required", proj.Key)
		}
		path := config.ResolvePath(proj.Path)

		items, breakdown, err := loadYAMLItems(path)
		if err != nil {
			return fmt.Errorf("project %s: %w", proj.Key, err)
		}

		if err := store.SaveProject(items, proj.Key); err != nil {
			return fmt.Errorf("save %s: %w", proj.Key, err)
		}

		name := proj.Name
		if name == "" {
			name = proj.Key
		}
		result.Projects = append(result.Projects, ProjectSyncResult{
			Key: proj.Key, Name: name, ItemCount: len(items),
		})
		result.TotalItems += len(items)

		if !jsonOutput {
			fmt.Printf("Loaded %d items from %s → data-%s.json\n", len(items), path, proj.Key)
			printBreakdown(breakdown)
		}
	}

	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("\nSync complete: %d items across %d projects\n", result.TotalItems, len(result.Projects))
	}

	appcontext.SetLastSync(now)
	return nil
}

type statusBreakdown struct {
	Done, InProgress, ToDo, Blocked, Other int
}

func loadYAMLItems(path string) ([]forecast.Item, statusBreakdown, error) {
	tf, err := yamlparser.ParseTaskFile(path)
	if err != nil {
		return nil, statusBreakdown{}, apperrors.WrapWithSuggestion(err,
			fmt.Sprintf("Failed to parse authoritative yaml: %s", path),
			"Check that the file exists and is valid YAML",
		)
	}
	items, err := tf.ToItems()
	if err != nil {
		return nil, statusBreakdown{}, fmt.Errorf("convert yaml tasks to forecast items: %w", err)
	}
	var b statusBreakdown
	for _, it := range items {
		switch it.Status {
		case "Done":
			b.Done++
		case "In Progress":
			b.InProgress++
		case "To Do":
			b.ToDo++
		case "Blocked":
			b.Blocked++
		default:
			b.Other++
		}
	}
	return items, b, nil
}

func printBreakdown(b statusBreakdown) {
	fmt.Printf("  Done: %d  In Progress: %d  To Do: %d  Blocked: %d  Other: %d\n",
		b.Done, b.InProgress, b.ToDo, b.Blocked, b.Other)
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
