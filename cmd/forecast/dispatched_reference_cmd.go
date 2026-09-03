package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrewcostello/forecast/internal/dispatched"
	"github.com/spf13/cobra"
)

var dispatchedReferenceCmd = &cobra.Command{
	Use:   "dispatched-reference",
	Short: "Build and inspect dispatched-agent reference classes",
}

var dispatchedReferenceBuildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the union reference class and report its coverage",
	Long: "Build the union reference class from dispatcher journals and every recoverable\n" +
		"revision of the tasks YAML, write it to --out, and report what it covers.\n" +
		"No prediction is made: a (role, model) cell with no observations is reported\n" +
		"with n=0, never filled with a default or a pooled distribution.\n\n" +
		"git must be on PATH and --features-repo must name a git repository.",
	RunE: func(cmd *cobra.Command, args []string) error {
		runsDir, _ := cmd.Flags().GetString("runs-dir")
		out, _ := cmd.Flags().GetString("out")
		featuresRepo, _ := cmd.Flags().GetString("features-repo")
		tasks, _ := cmd.Flags().GetString("tasks")
		minObservations, _ := cmd.Flags().GetInt("min-observations")
		maxCommits, _ := cmd.Flags().GetInt("max-history-commits")
		failOnEmpty, _ := cmd.Flags().GetBool("fail-on-empty-required")
		if featuresRepo == "" {
			return fmt.Errorf("--features-repo is required (no home directory to default from)")
		}
		return buildDispatchedReference(cmd.Context(), cmd, dispatched.BuildOptions{
			RunsDir: runsDir, FeaturesRepo: featuresRepo, TargetTasks: tasks,
			MinObservations: minObservations, MaxHistoryCommits: maxCommits,
		}, out, failOnEmpty)
	},
}

func init() {
	// An unavailable home directory must not silently become the relative
	// path "Project/claude-workflow", which would walk whatever features/
	// tree happens to sit under the working directory.
	defaultFeaturesRepo := ""
	if home, err := os.UserHomeDir(); err == nil {
		defaultFeaturesRepo = filepath.Join(home, "Project", "claude-workflow")
	}
	flags := dispatchedReferenceBuildCmd.Flags()
	flags.String("runs-dir", "", "directory containing dispatcher run directories")
	flags.String("out", "", "reference-class JSON output path")
	flags.String("features-repo", defaultFeaturesRepo, "git repository containing live and historical features YAML")
	flags.String("tasks", "", "target tasks YAML whose required cells should be measured")
	flags.Int("min-observations", dispatched.DefaultMinObservations, "completed observations a required cell needs before it counts as covered")
	flags.Int("max-history-commits", 0, "cap on commits walked for historical YAML (0 = built-in default)")
	flags.Bool("fail-on-empty-required", false, "exit non-zero when a required (role, model) cell has no observations")
	_ = dispatchedReferenceBuildCmd.MarkFlagRequired("runs-dir")
	_ = dispatchedReferenceBuildCmd.MarkFlagRequired("out")
	dispatchedReferenceCmd.AddCommand(dispatchedReferenceBuildCmd)
	rootCmd.AddCommand(dispatchedReferenceCmd)
}

func buildDispatchedReference(ctx context.Context, cmd *cobra.Command, opts dispatched.BuildOptions, out string, failOnEmptyRequired bool) error {
	result, err := dispatched.Build(ctx, opts)
	if err != nil {
		return fmt.Errorf("build dispatched reference: %w", err)
	}
	if err := dispatched.WriteArtifact(out, result.Artifact); err != nil {
		return fmt.Errorf("build dispatched reference: %w", err)
	}
	dispatched.WriteCoverage(cmd.OutOrStdout(), result.Artifact)
	fmt.Fprintf(cmd.OutOrStdout(), "Reference class written to %s\n", out)
	if empty := result.Artifact.Coverage.EmptyRequiredCells; failOnEmptyRequired && len(empty) > 0 {
		return fmt.Errorf("%d required (role, model) cells have no observations: %v", len(empty), empty)
	}
	return nil
}
