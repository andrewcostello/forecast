package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/diagram"
	"github.com/andrewcostello/forecast/internal/montecarlo"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/spf13/cobra"
)

var diagramCmd = &cobra.Command{
	Use:   "diagram",
	Short: "Generate project visualization diagrams",
	Long: `Generate D2 diagrams for project visualizations.

Diagram Types:
  burndown  - Burndown chart showing items remaining over time
  flow      - Status flow diagram (To Do → In Progress → Done)
  timeline  - Forecast timeline with confidence intervals

Output Formats:
  d2   - D2 diagram source code
  svg  - Rendered SVG image (requires d2 CLI installed)

Examples:
  forecast diagram --project myproject --type burndown
  forecast diagram --project myproject --type flow --output flow.svg
  forecast diagram --project myproject --type timeline --format d2`,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		diagramType, _ := cmd.Flags().GetString("type")
		output, _ := cmd.Flags().GetString("output")
		format, _ := cmd.Flags().GetString("format")

		return runDiagram(project, diagramType, output, format)
	},
}

func init() {
	diagramCmd.Flags().StringP("project", "p", "", "Project to visualize (required)")
	diagramCmd.Flags().StringP("type", "t", "burndown", "Diagram type: burndown, flow, timeline")
	diagramCmd.Flags().StringP("output", "o", "", "Output file (optional, prints to stdout if not specified)")
	diagramCmd.Flags().StringP("format", "f", "svg", "Output format: d2, svg")

	diagramCmd.MarkFlagRequired("project")

	rootCmd.AddCommand(diagramCmd)
}

func runDiagram(projectFilter, diagramType, outputFile, format string) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	// Find the project
	proj := cfg.GetProject(projectFilter)
	if proj == nil {
		// List available projects
		projects := cfg.GetAllProjects()
		if len(projects) > 0 {
			fmt.Println("Available projects:")
			for _, p := range projects {
				fmt.Printf("  - %s (%s)\n", p.Key, p.Name)
			}
		}
		return fmt.Errorf("project '%s' not found in config", projectFilter)
	}

	projectKey := proj.Key
	if projectKey == "" {
		projectKey = proj.Epic
	}

	// Load items
	store := storage.New(".forecast")
	items, err := store.LoadProject(projectKey)
	if err != nil {
		return fmt.Errorf("failed to load items: %w", err)
	}

	if len(items) == 0 {
		return fmt.Errorf("no items found - run 'forecast sync' first")
	}

	// Create generator
	gen := diagram.NewGenerator(items, proj.Name, proj.Key)

	// If timeline, add percentiles from forecast
	if diagramType == "timeline" {
		teamCapacity := cfg.TeamCapacity
		if proj.Capacity > 0 {
			teamCapacity = proj.Capacity
		}
		if teamCapacity <= 0 {
			teamCapacity = 8.0
		}

		// Check if we have cycle time data
		hasCycleTime := false
		for _, item := range items {
			if item.Status == "Done" && item.CycleTime > 0 {
				hasCycleTime = true
				break
			}
		}

		if hasCycleTime {
			simulator := montecarlo.NewSimulator(items, teamCapacity)
			result := simulator.Run(10000)
			gen.SetPercentiles(result.Percentiles)
		}
	}

	// Determine diagram type
	var dt diagram.DiagramType
	switch strings.ToLower(diagramType) {
	case "burndown":
		dt = diagram.TypeBurndown
	case "flow":
		dt = diagram.TypeFlow
	case "timeline":
		dt = diagram.TypeTimeline
	default:
		return fmt.Errorf("unknown diagram type: %s (valid: burndown, flow, timeline)", diagramType)
	}

	// Generate output
	var output []byte
	if strings.ToLower(format) == "d2" {
		// Output D2 source code
		output = []byte(gen.Generate(dt))
	} else {
		// Render to SVG
		renderer := diagram.NewRenderer()
		var contentType string
		output, contentType, err = gen.RenderWithFallback(dt, renderer)
		if err != nil {
			return fmt.Errorf("failed to render diagram: %w", err)
		}
		_ = contentType // Used for HTTP responses
	}

	// Write output
	if outputFile != "" {
		if err := os.WriteFile(outputFile, output, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Printf("Diagram written to %s\n", outputFile)
	} else {
		fmt.Print(string(output))
	}

	return nil
}
