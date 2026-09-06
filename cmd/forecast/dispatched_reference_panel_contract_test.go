package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrewcostello/forecast/internal/dispatched"
	"github.com/spf13/cobra"
)

func testCLIGateRequiresTasks(t *testing.T) {
	fixture := newCommandFixture(t)
	for _, flag := range []string{"--fail-on-empty-required", "--fail-on-uncovered-required"} {
		t.Run(flag, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "o.json")
			r := executeReferenceBuild(t, []string{
				"--runs-dir", fixture.runs, "--features-repo", fixture.repo,
				"--out", out, flag, "true",
			})
			if r.err == nil {
				t.Fatalf("missing --tasks under %s succeeded", flag)
			}
			if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("RunE missing --tasks wrote an artifact: %v", err)
			}
		})
	}
}

func testCLIGateBypassEmptyTarget(t *testing.T) {
	fixture := newCommandFixture(t)
	cutoff := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	opts := dispatched.BuildOptions{
		Sources: []dispatched.SourceSpec{
			{ID: "journals", Kind: dispatched.SourceKindJournals, Repository: fixture.runs},
			{ID: "features-001-live", Kind: dispatched.SourceKindLiveYAML, Repository: fixture.repo, Roots: []string{"features"}},
			{ID: "features-001-history", Kind: dispatched.SourceKindGitHistory, Repository: fixture.repo, Roots: []string{"features"}},
		},
		Selection: dispatched.Selection{Cutoff: cutoff},
	}
	for _, tc := range []struct {
		name          string
		failEmpty     bool
		failUncovered bool
	}{
		{name: "fail-on-empty-required", failEmpty: true},
		{name: "fail-on-uncovered-required", failUncovered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "o.json")
			cmd := &cobra.Command{}
			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			err := buildDispatchedReference(context.Background(), cmd, opts, out, tc.failEmpty, tc.failUncovered)
			if _, statErr := os.Stat(out); statErr != nil {
				t.Fatalf("bypass did not write diagnostics: %v (err=%v)", statErr, err)
			}
			if !strings.Contains(stdout.String(), "Dispatched reference-class coverage") {
				t.Fatalf("bypass omitted coverage report:\n%s", stdout.String())
			}
			if err == nil {
				t.Fatal("omitted TargetTasks licensed a coverage gate")
			}
			if !errors.Is(err, dispatched.ErrEmptyTarget) || !errors.Is(err, dispatched.ErrNotEligible) {
				t.Fatalf("bypass = %v, want ErrEmptyTarget and ErrNotEligible", err)
			}
		})
	}
}
