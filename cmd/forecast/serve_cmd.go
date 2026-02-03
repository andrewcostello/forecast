package main

import (
	"fmt"

	"github.com/andrewcostello/forecast/internal/config"
	"github.com/andrewcostello/forecast/internal/storage"
	"github.com/andrewcostello/forecast/internal/web"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start web server with dashboard",
	Long: `Starts an HTTP server that serves a web dashboard and REST API.

The dashboard provides:
  - Overview of all tracked projects
  - Progress visualization
  - Monte Carlo forecasts
  - D2 diagrams (burndown, flow, timeline)

API Endpoints:
  GET /api/health             - Health check
  GET /api/projects           - List all projects with stats
  GET /api/projects/{key}     - Project details
  GET /api/projects/{key}/forecast - Monte Carlo forecast
  GET /api/projects/{key}/report   - Full report data
  GET /api/projects/{key}/diagram  - D2 diagram (SVG)

Examples:
  forecast serve
  forecast serve --port 3000
  forecast serve --dev --port 8080`,
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		project, _ := cmd.Flags().GetString("project")
		devMode, _ := cmd.Flags().GetBool("dev")

		return runServe(port, project, devMode)
	},
}

func init() {
	serveCmd.Flags().StringP("port", "p", "8080", "Port to listen on")
	serveCmd.Flags().String("project", "", "Serve specific project only (not implemented)")
	serveCmd.Flags().Bool("dev", false, "Development mode (serve static files from disk)")

	rootCmd.AddCommand(serveCmd)
}

func runServe(port, project string, devMode bool) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("failed to load config - run 'forecast init' first")
	}

	store := storage.New(".forecast")

	srv := web.NewServer(cfg, store, devMode)

	fmt.Printf("Starting forecast server on http://localhost:%s\n", port)
	fmt.Println("Press Ctrl+C to stop")

	return srv.Start(port)
}
