package main

import (
	"fmt"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/referenceclass"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/spf13/cobra"
)

var referenceClassCmd = &cobra.Command{
	Use:   "reference-class",
	Short: "Manage reference class database",
	Long: `Commands for managing historical project data:
  list - Show available reference classes
  add  - Add completed project to reference database`,
}

func init() {
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
