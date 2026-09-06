package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewcostello/forecast/internal/dispatched"
)

// TestFCReferenceCLIContract is the reserved FC-1 CLI group. New-capability
// cases stay red until FC-1 maps explicit sources, refuses empty/partial
// corpora at the prediction gate, and silences Cobra usage on data errors.
func TestFCReferenceCLIContract(t *testing.T) {
	t.Run("F3-SRC-EXPLICIT-ONLY", testCLIMissingExplicitRepos)
	t.Run("F4-TARGET-ZERO-ROWS", testCLIEmptyTarget)
	t.Run("F4-TARGET-MALFORMED", testCLIMalformedTarget)
	t.Run("F4-MISSING-FLAGS", testCLIMissingFlags)
	t.Run("F3-SRC-ZERO-JOURNALS", testCLIEmptyCorpus)
	t.Run("F4-NOT-ELIGIBLE-PARTIAL", testCLIPartialCorpusRefusal)
	t.Run("F4-DATA-ERROR-NO-USAGE", testCLIDataErrorNoUsage)
	t.Run("F4-HAND-FINISHED-LIMIT", testCLIHandFinishedLimit)
	t.Run("F4-GATE-REQUIRES-TASKS", testCLIGateRequiresTasks)
	t.Run("F4-GATE-BYPASS-EMPTY-TARGET", testCLIGateBypassEmptyTarget)
	t.Run("F4-UNKNOWN-FLAG-USAGE", testCLIUnknownFlagUsage)
	t.Run("F4-TIMEOUT-USAGE", testCLITimeoutUsage)
}

func cliTestdata(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"testdata", "dispatched"}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type cliResult struct {
	stdout string
	stderr string
	err    error
}

func executeReferenceBuild(t *testing.T, args []string) cliResult {
	t.Helper()
	cmd := newDispatchedReferenceBuildCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	cmd.SilenceUsage = false
	cmd.SilenceErrors = false
	err := cmd.Execute()
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func combinedOutput(r cliResult) string { return r.stdout + r.stderr }

func hasUsage(r cliResult) bool {
	out := combinedOutput(r)
	return strings.Contains(out, "Usage:") || strings.Contains(out, "usage:")
}

func testCLIMissingExplicitRepos(t *testing.T) {
	fixture := newCommandFixture(t)
	home := t.TempDir()
	decoy := filepath.Join(home, "Project", "claude-workflow")
	decoyFixture := newCommandFixture(t)
	if err := os.MkdirAll(filepath.Dir(decoy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(decoyFixture.repo, decoy); err != nil {
		t.Fatal(err)
	}
	// This distinctive row is valid and would be scanned only by the forbidden
	// HOME default. The matching ROW row already in this repository also makes
	// legacy defaulting capable of completing against fixture.runs.
	writeFile(t, filepath.Join(decoy, "features", "study", "home-decoy.yaml"), `tasks:
  - key: FC-SEALS-HOME-DECOY
    role: bodies
    model: home-decoy
    status: Done
    started_at: '2026-01-01T00:00:00Z'
    completed_at: '2026-01-01T00:01:00Z'
    dispatcher_run_id: decoy-only
`)
	runCommandFixtureGit(t, decoy, "add", "features")
	runCommandFixtureGit(t, decoy, "commit", "-q", "-m", "distinctive home decoy")
	t.Setenv("HOME", home)
	out := filepath.Join(t.TempDir(), "o.json")
	r := executeReferenceBuild(t, []string{
		"--runs-dir", fixture.runs, "--out", out,
	})
	if r.err == nil {
		t.Fatal("missing explicit repo succeeded via home default")
	}
	if !strings.Contains(r.err.Error(), "--features-repo") && !errors.Is(r.err, dispatched.ErrInvalidSourceSpec) {
		t.Fatalf("missing explicit repo failed after source processing: %v", r.err)
	}
	if strings.Contains(r.err.Error(), decoy) || strings.Contains(combinedOutput(r), decoy) {
		t.Fatalf("CLI used HOME default %s: %v\n%s", decoy, r.err, combinedOutput(r))
	}
	if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing explicit source wrote an artifact before refusal: %v", err)
	}
	if hasUsage(r) {
		t.Fatalf("data error printed usage:\n%s", combinedOutput(r))
	}
}

func testCLIEmptyTarget(t *testing.T) {
	fixture := newCommandFixture(t)
	target := filepath.Join(t.TempDir(), "empty.yaml")
	writeFile(t, target, cliTestdata(t, "targets", "empty.yaml"))
	r := executeReferenceBuild(t, []string{
		"--runs-dir", fixture.runs, "--features-repo", fixture.repo,
		"--out", filepath.Join(t.TempDir(), "o.json"), "--tasks", target,
		"--fail-on-empty-required", "true",
	})
	if r.err == nil || !errors.Is(r.err, dispatched.ErrEmptyTarget) {
		t.Fatalf("empty target = %v, want ErrEmptyTarget", r.err)
	}
	if hasUsage(r) {
		t.Fatalf("empty target printed usage:\n%s", combinedOutput(r))
	}
}

func testCLIMalformedTarget(t *testing.T) {
	fixture := newCommandFixture(t)
	for _, name := range []string{"malformed.yaml", "blank-model.yaml", "duplicate-key.yaml"} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), name)
			writeFile(t, target, cliTestdata(t, "targets", name))
			r := executeReferenceBuild(t, []string{
				"--runs-dir", fixture.runs, "--features-repo", fixture.repo,
				"--out", filepath.Join(t.TempDir(), "o.json"), "--tasks", target,
			})
			if r.err == nil || !errors.Is(r.err, dispatched.ErrInvalidTarget) {
				t.Fatalf("malformed target %s = %v, want ErrInvalidTarget (amended path)", name, r.err)
			}
			if hasUsage(r) {
				t.Fatalf("malformed target printed usage:\n%s", combinedOutput(r))
			}
		})
	}
}

func testCLIMissingFlags(t *testing.T) {
	r := executeReferenceBuild(t, nil)
	if r.err == nil {
		t.Fatal("missing required flags succeeded")
	}
	r2 := executeReferenceBuild(t, []string{"--runs-dir", t.TempDir()})
	if r2.err == nil {
		t.Fatal("missing --out succeeded")
	}
}

func testCLIEmptyCorpus(t *testing.T) {
	fixture := newCommandFixture(t)
	emptyRuns := t.TempDir()
	r := executeReferenceBuild(t, []string{
		"--runs-dir", emptyRuns, "--features-repo", fixture.repo,
		"--out", filepath.Join(t.TempDir(), "o.json"),
	})
	if r.err == nil || !errors.Is(r.err, dispatched.ErrSourceEmpty) {
		t.Fatalf("empty corpus = %v, want ErrSourceEmpty", r.err)
	}
	if hasUsage(r) {
		t.Fatalf("empty corpus printed usage:\n%s", combinedOutput(r))
	}
}

func testCLIPartialCorpusRefusal(t *testing.T) {
	fixture := newCommandFixture(t)
	writeFile(t, filepath.Join(fixture.repo, "features", "study", "broken.yaml"), "tasks:\n - key: [")
	target := filepath.Join(t.TempDir(), "valid.yaml")
	writeFile(t, target, cliTestdata(t, "targets", "valid.yaml"))
	r := executeReferenceBuild(t, []string{
		"--runs-dir", fixture.runs, "--features-repo", fixture.repo,
		"--out", filepath.Join(t.TempDir(), "o.json"),
		"--tasks", target, "--fail-on-uncovered-required", "true",
	})
	if r.err == nil {
		t.Fatal("partial corpus licensed a coverage/prediction gate")
	}
	if !errors.Is(r.err, dispatched.ErrNotEligible) && !errors.Is(r.err, dispatched.ErrSourceIncomplete) {
		t.Fatalf("partial refusal = %v, want eligibility/incomplete sentinels", r.err)
	}
	if hasUsage(r) {
		t.Fatalf("partial corpus printed usage:\n%s", combinedOutput(r))
	}
}

func testCLIDataErrorNoUsage(t *testing.T) {
	r := executeReferenceBuild(t, []string{
		"--runs-dir", filepath.Join(t.TempDir(), "missing"),
		"--features-repo", filepath.Join(t.TempDir(), "missing-repo"),
		"--out", filepath.Join(t.TempDir(), "o.json"),
	})
	if r.err == nil {
		t.Fatal("missing sources succeeded")
	}
	if hasUsage(r) {
		t.Fatalf("source data error printed usage:\n%s", combinedOutput(r))
	}
}

func testCLIHandFinishedLimit(t *testing.T) {
	fixture := newCommandFixture(t)
	out := filepath.Join(t.TempDir(), "o.json")
	r := executeReferenceBuild(t, []string{
		"--runs-dir", fixture.runs, "--features-repo", fixture.repo, "--out", out,
	})
	if r.err != nil && errors.Is(r.err, dispatched.ErrNotImplemented) {
		t.Fatal(r.err)
	}
	if r.err != nil {
		// Amended CLI may still write a diagnostic artifact then fail a gate.
		if _, statErr := os.Stat(out); statErr != nil && !strings.Contains(r.stdout, "Hand-finished") {
			t.Fatalf("CLI = %v\n%s", r.err, combinedOutput(r))
		}
	}
	if !strings.Contains(r.stdout, "Hand-finished rows have no identifying field") {
		t.Fatalf("CLI report omitted hand-finished limit:\n%s", r.stdout)
	}
}
