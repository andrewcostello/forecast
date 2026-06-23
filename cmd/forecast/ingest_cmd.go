package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/andrewcostello/forecast/internal/ingest"
	"github.com/andrewcostello/forecast/internal/jira"
	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Project a claude-dispatcher run's outcomes onto JIRA",
	Long: `Reads a claude-dispatcher run's canonical artifacts (tasks.yaml + the
run's journal.jsonl) and projects them onto JIRA — keeping the dispatcher itself
pure.

For each task mapped to a JIRA issue (jira_key, set by 'forecast sync'), it
posts an outcome comment (status, PR, cost, haiku summary) and attaches the
agent transcript. When the run included a whole-feature review and --epic-key is
given, it posts the final review (consensus + dispositions) on the epic.

Dry-run by default: it prints the planned JIRA writes and changes nothing.
Pass --apply to actually write.

Examples:
  forecast ingest --tasks docs/tasks/WALLET.yaml --run-dir docs/runs/2026-...
  forecast ingest -f tasks.yaml --run-dir docs/runs/R --epic-key SMG-100 --apply`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tasks := mustGetString(cmd, "tasks")
		runDir, _ := cmd.Flags().GetString("run-dir")
		epicKey, _ := cmd.Flags().GetString("epic-key")
		apply, _ := cmd.Flags().GetBool("apply")

		journal := ""
		if runDir != "" {
			journal = filepath.Join(runDir, "journal.jsonl")
		}

		ra, err := ingest.LoadRun(tasks, journal)
		if err != nil {
			return err
		}
		actions := ingest.Plan(ra, epicKey)

		mode := "DRY RUN"
		if apply {
			mode = "APPLY"
		}
		printBanner(fmt.Sprintf("forecast ingest — %s", mode))
		if len(actions) == 0 {
			fmt.Println("No JIRA actions planned. (Are tasks mapped to jira_key? " +
				"Run 'forecast sync --file' first.)")
			return nil
		}
		for _, a := range actions {
			fmt.Printf("  • %s\n", a.Label)
		}

		if !apply {
			fmt.Printf("\nDry run — %d action(s) planned. Re-run with --apply to write to JIRA.\n",
				len(actions))
			return nil
		}

		done, errs := ingest.Apply(actions, func(key string) (ingest.Writer, error) {
			c, _, err := getJiraClientForKey(key)
			if err != nil {
				return nil, err
			}
			return jiraWriter{c}, nil
		})
		fmt.Printf("\nApplied %d/%d action(s).\n", done, len(actions))
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  ! %v\n", e)
		}
		if len(errs) > 0 {
			return fmt.Errorf("%d action(s) failed", len(errs))
		}
		return nil
	},
}

func init() {
	ingestCmd.Flags().StringP("tasks", "f", "", "Path to the dispatcher tasks.yaml (required)")
	ingestCmd.Flags().String("run-dir", "", "Path to the run dir (docs/runs/<id>) for journal.jsonl (final review)")
	ingestCmd.Flags().String("epic-key", "", "JIRA key to post the feature-review summary on")
	ingestCmd.Flags().Bool("apply", false, "Actually write to JIRA (default: dry-run preview)")
	_ = ingestCmd.MarkFlagRequired("tasks")
}

// jiraWriter adapts *jira.Client to ingest.Writer (drops the *Attachment return).
type jiraWriter struct{ c *jira.Client }

func (w jiraWriter) AddComment(issueKey, comment string) error {
	return w.c.AddComment(issueKey, comment)
}

func (w jiraWriter) UploadAttachment(issueKey, filePath string) error {
	_, err := w.c.UploadAttachment(issueKey, filePath)
	return err
}
