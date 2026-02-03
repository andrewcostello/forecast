package main

import (
	"fmt"
	"os"

	"github.com/andrewcostello/forecast/internal/config"
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
	rootCmd.AddCommand(dashboardCmd)
}

func initConfig() {
	if err := config.Load(cfgFile); err != nil {
		// Only warn if it's not a "not found" error - that's expected for init
		if _, ok := err.(config.ConfigNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
}
