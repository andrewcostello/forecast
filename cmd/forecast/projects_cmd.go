package main

import (
	"encoding/json"
	"fmt"
	"time"

	appcontext "github.com/andrewcostello/forecast/internal/context"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/pkg/forecast"
	"github.com/spf13/cobra"
)

var projectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List all configured projects",
	Long: `Displays all configured projects with their current status.

Shows:
  - Project key and name
  - JIRA Epic link
  - Current context indicator (*)
  - Item counts and progress
  - Last sync time

Use --detail for additional information.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		detail, _ := cmd.Flags().GetBool("detail")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		return runProjects(detail, jsonOutput)
	},
}

func init() {
	projectsCmd.Flags().Bool("detail", false, "Show detailed information")
	projectsCmd.Flags().Bool("json", false, "Output as JSON")

	rootCmd.AddCommand(projectsCmd)
}

// ProjectInfo holds project information for display
type ProjectInfo struct {
	Key        string    `json:"key"`
	Name       string    `json:"name"`
	Epic       string    `json:"epic"`
	IsCurrent  bool      `json:"isCurrent"`
	IsPrevious bool      `json:"isPrevious"`
	Total      int       `json:"total,omitempty"`
	Done       int       `json:"done,omitempty"`
	Remaining  int       `json:"remaining,omitempty"`
	Progress   float64   `json:"progress,omitempty"`
	LastSync   string    `json:"lastSync,omitempty"`
	Capacity   float64   `json:"capacity,omitempty"`
}

// ProjectsResult holds all project info for JSON output
type ProjectsResult struct {
	Projects       []ProjectInfo `json:"projects"`
	CurrentProject string        `json:"currentProject,omitempty"`
	TotalProjects  int           `json:"totalProjects"`
}

func runProjects(detail, jsonOutput bool) error {
	cfg, err := requireConfig()
	if err != nil {
		return err
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		if jsonOutput {
			result := ProjectsResult{Projects: []ProjectInfo{}, TotalProjects: 0}
			data, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Println("No projects configured.")
		fmt.Println("\nAdd projects to your .forecast/config.yaml:")
		fmt.Println(`
projects:
  - name: "My Project"
    key: "myproject"
    epic: "PROJ-123"`)
		return nil
	}

	currentProject := appcontext.GetProject()
	previousProject := appcontext.GetPreviousProject()
	store := storage.New(".forecast")

	result := ProjectsResult{
		Projects:       make([]ProjectInfo, 0, len(projects)),
		CurrentProject: currentProject,
		TotalProjects:  len(projects),
	}

	for _, proj := range projects {
		key := proj.Key
		if key == "" {
			key = proj.Epic
		}

		info := ProjectInfo{
			Key:        key,
			Name:       proj.Name,
			Epic:       proj.Epic,
			IsCurrent:  key == currentProject,
			IsPrevious: key == previousProject,
			Capacity:   proj.Capacity,
		}

		// Load local data if available
		items, err := store.LoadProject(key)
		if err == nil && len(items) > 0 {
			info.Total, info.Done, info.Remaining = countItems(items)
			if info.Total > 0 {
				info.Progress = float64(info.Done) / float64(info.Total) * 100
			}
		}

		// Get last sync time from context
		lastSync := appcontext.GetLastSync()
		if !lastSync.IsZero() {
			info.LastSync = formatRelativeTime(lastSync)
		}

		result.Projects = append(result.Projects, info)
	}

	// JSON output
	if jsonOutput {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Println("\nConfigured Projects:")
	fmt.Println("====================")

	if detail {
		// Detailed view
		for _, info := range result.Projects {
			marker := "  "
			if info.IsCurrent {
				marker = "* "
			} else if info.IsPrevious {
				marker = "- "
			}

			fmt.Printf("\n%s%s\n", marker, info.Name)
			fmt.Printf("    Key:      %s\n", info.Key)
			fmt.Printf("    Epic:     %s\n", info.Epic)
			if info.Total > 0 {
				fmt.Printf("    Progress: %d/%d (%.1f%%)\n", info.Done, info.Total, info.Progress)
			}
			if info.Capacity > 0 {
				fmt.Printf("    Capacity: %.1f hrs/day\n", info.Capacity)
			}
			if info.LastSync != "" {
				fmt.Printf("    Synced:   %s\n", info.LastSync)
			}
		}
	} else {
		// Compact view
		fmt.Println()
		for _, info := range result.Projects {
			marker := "  "
			if info.IsCurrent {
				marker = "* "
			} else if info.IsPrevious {
				marker = "- "
			}

			progressStr := ""
			if info.Total > 0 {
				progressStr = fmt.Sprintf(" [%d/%d %.0f%%]", info.Done, info.Total, info.Progress)
			}

			syncStr := ""
			if info.LastSync != "" {
				syncStr = fmt.Sprintf(" (synced %s)", info.LastSync)
			}

			fmt.Printf("%s%-15s %s%s%s\n", marker, info.Key, info.Name, progressStr, syncStr)
		}
	}

	fmt.Println()
	fmt.Println("Legend: * = current, - = previous")
	fmt.Println("Use 'forecast use <key>' to switch projects")
	fmt.Println("Use 'forecast use -' to toggle to previous project")
	fmt.Println()

	return nil
}

func countItems(items []forecast.Item) (total, done, remaining int) {
	for _, item := range items {
		switch item.Status {
		case "Done":
			total++
			done++
		case "Canceled":
			// Skip
		default:
			total++
			remaining++
		}
	}
	return
}

func formatRelativeTime(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "just now"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
