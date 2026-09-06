package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/andrewcostello/forecast/internal/dispatched"
	"github.com/spf13/cobra"
)

var dispatchedReferenceCmd = &cobra.Command{
	Use:   "dispatched-reference",
	Short: "Build and inspect dispatched-agent reference classes",
}

var dispatchedReferenceBuildCmd = newDispatchedReferenceBuildCmd()

func newDispatchedReferenceBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build the union reference class and report its coverage",
		Long: "Build the union reference class from dispatcher journals and every recoverable\n" +
			"revision of the tasks YAML, write it to --out, and report what it covers.\n" +
			"No prediction is made: a (role, model) cell with no observations is reported\n" +
			"with n=0, never filled with a default or a pooled distribution.\n\n" +
			"git must be on PATH and --features-repo must name a git repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Cobra otherwise prints command syntax for source/coverage data
			// failures, obscuring the diagnostic report the command just wrote.
			cmd.SilenceUsage = true
			runsDir, _ := cmd.Flags().GetString("runs-dir")
			out, _ := cmd.Flags().GetString("out")
			featuresRepo, _ := cmd.Flags().GetStringSlice("features-repo")
			taskRoots, _ := cmd.Flags().GetStringSlice("task-root")
			tasks, _ := cmd.Flags().GetString("tasks")
			minObservations, _ := cmd.Flags().GetInt("min-observations")
			maxCommits, _ := cmd.Flags().GetInt("max-history-commits")
			timeout, _ := cmd.Flags().GetDuration("timeout")
			if timeout <= 0 {
				return fmt.Errorf("--timeout must be positive")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			failOnEmpty, _ := cmd.Flags().GetBool("fail-on-empty-required")
			failOnUncovered, _ := cmd.Flags().GetBool("fail-on-uncovered-required")
			if (failOnEmpty || failOnUncovered) && tasks == "" {
				return fmt.Errorf("--tasks is required for a coverage gate")
			}
			if len(featuresRepo) == 0 {
				return fmt.Errorf("--features-repo is required (no home directory to default from)")
			}
			cutoff := time.Now().Round(0).UTC()
			sources := make([]dispatched.SourceSpec, 0, 1+2*len(featuresRepo))
			sources = append(sources, dispatched.SourceSpec{ID: "journals", Kind: dispatched.SourceKindJournals, Repository: runsDir})
			for i, repo := range featuresRepo {
				prefix := fmt.Sprintf("features-%03d", i+1)
				sources = append(sources,
					dispatched.SourceSpec{ID: prefix + "-live", Kind: dispatched.SourceKindLiveYAML, Repository: repo, Roots: append([]string{}, taskRoots...)},
					dispatched.SourceSpec{ID: prefix + "-history", Kind: dispatched.SourceKindGitHistory, Repository: repo, Roots: append([]string{}, taskRoots...)},
				)
			}
			return buildDispatchedReference(ctx, cmd, dispatched.BuildOptions{
				Sources: sources, Selection: dispatched.Selection{Cutoff: cutoff},
				Bounds: dispatched.ReadBounds{MaxCommits: maxCommits}, TargetTasks: tasks,
				MinObservations: minObservations,
			}, out, failOnEmpty, failOnUncovered)
		},
	}
	cmd.SilenceUsage = true

	flags := cmd.Flags()
	flags.Duration("timeout", 5*time.Minute, "maximum duration of reference extraction")
	flags.String("runs-dir", "", "directory containing dispatcher run directories")
	flags.String("out", "", "reference-class JSON output path")
	flags.StringSlice("features-repo", nil, "explicit git repositories containing live and historical features YAML (repeat for each repository)")
	flags.StringSlice("task-root", []string{"features"}, "repository-relative task YAML directory selected from each features repository (repeatable)")
	flags.String("tasks", "", "target tasks YAML whose required cells should be measured")
	flags.Int("min-observations", dispatched.DefaultMinObservations, "completed observations a required cell needs before it counts as covered")
	flags.Int("max-history-commits", 0, "cap on commits walked for historical YAML (0 = built-in default)")
	flags.Bool("fail-on-uncovered-required", false, "exit non-zero when any required cell has fewer than --min-observations completed observations")
	flags.Bool("fail-on-empty-required", false, "exit non-zero when a required (role, model) cell has no observations")
	_ = cmd.MarkFlagRequired("runs-dir")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func init() {
	dispatchedReferenceCmd.AddCommand(dispatchedReferenceBuildCmd)
	rootCmd.AddCommand(dispatchedReferenceCmd)
}

func buildDispatchedReference(ctx context.Context, cmd *cobra.Command, opts dispatched.BuildOptions, out string, failOnEmptyRequired, failOnUncovered bool) error {
	result, buildErr := dispatched.Build(ctx, opts)
	if result == nil {
		return fmt.Errorf("build dispatched reference: %w", buildErr)
	}
	if err := dispatched.WriteArtifact(out, result.Artifact); err != nil {
		return fmt.Errorf("build dispatched reference: %w", err)
	}
	dispatched.WriteCoverage(cmd.OutOrStdout(), result.Artifact)
	fmt.Fprintf(cmd.OutOrStdout(), "Reference class written to %s\n", out)
	if (failOnEmptyRequired || failOnUncovered) && result.Artifact.SourceManifest != nil {
		if err := result.Artifact.SourceManifest.ValidateComplete(); err != nil {
			gateDetail := ""
			if failOnEmptyRequired && len(result.Artifact.Coverage.EmptyRequiredCells) > 0 {
				gateDetail = fmt.Sprintf("; empty required cells: %v", result.Artifact.Coverage.EmptyRequiredCells)
			}
			if failOnUncovered && len(result.Artifact.Coverage.UncoveredRequiredCells) > 0 {
				gateDetail += fmt.Sprintf("; required cells below %d completed observations: %v",
					result.Artifact.Coverage.MinObservations, result.Artifact.Coverage.UncoveredRequiredCells)
			}
			return errors.Join(
				fmt.Errorf("%w: coverage gate requires complete sources%s", dispatched.ErrNotEligible, gateDetail),
				fmt.Errorf("%w: %v", dispatched.ErrSourceIncomplete, err),
				buildErr,
			)
		}
	}
	if buildErr != nil {
		return fmt.Errorf("build dispatched reference: %w", buildErr)
	}
	if empty := result.Artifact.Coverage.EmptyRequiredCells; failOnEmptyRequired && len(empty) > 0 {
		return fmt.Errorf("%w: %d required (role, model) cells have no observations: %v", dispatched.ErrNotEligible, len(empty), empty)
	}
	if failOnUncovered && len(result.Artifact.Coverage.UncoveredRequiredCells) > 0 {
		return fmt.Errorf("%w: required cells below %d completed observations: %v", dispatched.ErrNotEligible, result.Artifact.Coverage.MinObservations, result.Artifact.Coverage.UncoveredRequiredCells)
	}
	return nil
}
