package main

import (
	"fmt"

	"github.com/andrewcostello/forecast/internal/config"
	appcontext "github.com/andrewcostello/forecast/internal/context"
	"github.com/andrewcostello/forecast/internal/terminal"
	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:   "use [project]",
	Short: "Set current project context",
	Long: `Set the current project for subsequent commands.

Once set, commands like 'forecast status', 'forecast run', and 'forecast sync'
will use this project by default without needing the --project flag.

Special arguments:
  -       Toggle to previous project (like 'cd -')
  (none)  Interactive project picker

Use 'forecast use --clear' to remove the current context.

Examples:
  forecast use myproject      # Set current project
  forecast use                # Interactive picker
  forecast use -              # Toggle to previous project
  forecast use --clear        # Clear context`,
	RunE: func(cmd *cobra.Command, args []string) error {
		clear, _ := cmd.Flags().GetBool("clear")
		show, _ := cmd.Flags().GetBool("show")

		if clear {
			return clearContext()
		}

		if show {
			return showContext()
		}

		if len(args) == 0 {
			return interactiveProjectPicker()
		}

		// Handle "-" for toggle
		if args[0] == "-" {
			return toggleContext()
		}

		return setContext(args[0])
	},
}

func init() {
	useCmd.Flags().Bool("clear", false, "Clear current project context")
	useCmd.Flags().Bool("show", false, "Show current context without interactive picker")

	rootCmd.AddCommand(useCmd)
}

func setContext(projectKey string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no config found. Run 'forecast init' first")
	}

	// Validate project exists
	proj := cfg.GetProject(projectKey)
	if proj == nil {
		// Show available projects
		projects := cfg.GetAllProjects()
		if len(projects) > 0 {
			fmt.Println("Available projects:")
			for _, p := range projects {
				fmt.Printf("  - %s (%s)\n", p.Key, p.Name)
			}
		}
		return fmt.Errorf("project '%s' not found in config", projectKey)
	}

	// Use the key if available, otherwise epic
	key := proj.Key
	if key == "" {
		key = proj.Epic
	}

	if err := appcontext.SetProject(key); err != nil {
		return fmt.Errorf("failed to save context: %w", err)
	}

	terminal.PrintSuccess("Now using project: %s (%s)", proj.Name, key)
	fmt.Println("\nCommands will now default to this project:")
	fmt.Println("  forecast status")
	fmt.Println("  forecast run")
	fmt.Println("  forecast sync")
	fmt.Println("  forecast report")

	return nil
}

func showContext() error {
	current := appcontext.GetProject()

	if current == "" {
		fmt.Println("No project context set.")
		fmt.Println("\nSet a project with: forecast use <project>")

		cfg := config.Get()
		if cfg != nil {
			projects := cfg.GetAllProjects()
			if len(projects) > 0 {
				fmt.Println("\nAvailable projects:")
				for _, p := range projects {
					fmt.Printf("  - %s (%s)\n", p.Key, p.Name)
				}
			}
		}
		return nil
	}

	cfg := config.Get()
	if cfg != nil {
		proj := cfg.GetProject(current)
		if proj != nil {
			fmt.Printf("Current project: %s (%s)\n", proj.Name, current)
		} else {
			fmt.Printf("Current project: %s\n", current)
		}
	} else {
		fmt.Printf("Current project: %s\n", current)
	}

	lastSync := appcontext.GetLastSync()
	if !lastSync.IsZero() {
		fmt.Printf("Last synced: %s\n", lastSync.Format("Jan 2, 2006 3:04 PM"))
	}

	return nil
}

func clearContext() error {
	if err := appcontext.Clear(); err != nil {
		// Ignore error if file doesn't exist
		return nil
	}
	terminal.PrintSuccess("Project context cleared")
	return nil
}

func toggleContext() error {
	previous := appcontext.GetPreviousProject()
	if previous == "" {
		return fmt.Errorf("no previous project to toggle to")
	}

	newProject, err := appcontext.ToggleProject()
	if err != nil {
		return fmt.Errorf("failed to toggle project: %w", err)
	}

	cfg := config.Get()
	if cfg != nil {
		proj := cfg.GetProject(newProject)
		if proj != nil {
			terminal.PrintSuccess("Switched to: %s (%s)", proj.Name, newProject)
			return nil
		}
	}

	terminal.PrintSuccess("Switched to: %s", newProject)
	return nil
}

func interactiveProjectPicker() error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("no config found. Run 'forecast init' first")
	}

	projects := cfg.GetAllProjects()
	if len(projects) == 0 {
		return fmt.Errorf("no projects configured")
	}

	// If only one project, just use it
	if len(projects) == 1 {
		key := projects[0].Key
		if key == "" {
			key = projects[0].Epic
		}
		return setContext(key)
	}

	currentProject := appcontext.GetProject()

	// Build options list
	options := make([]string, 0, len(projects))
	for _, proj := range projects {
		key := proj.Key
		if key == "" {
			key = proj.Epic
		}

		marker := "  "
		if key == currentProject {
			marker = "* "
		}

		options = append(options, fmt.Sprintf("%s%s - %s", marker, key, proj.Name))
	}

	fmt.Println("\nSelect a project:")
	fmt.Println()

	prompter := terminal.NewPrompter()
	defaultIdx := 0
	for i, proj := range projects {
		key := proj.Key
		if key == "" {
			key = proj.Epic
		}
		if key == currentProject {
			defaultIdx = i
		}
	}

	idx, _, err := prompter.PromptSelect("Project", options, defaultIdx)
	if err != nil {
		return fmt.Errorf("selection cancelled")
	}

	// Get the selected project key
	selectedProj := projects[idx]
	key := selectedProj.Key
	if key == "" {
		key = selectedProj.Epic
	}

	return setContext(key)
}
