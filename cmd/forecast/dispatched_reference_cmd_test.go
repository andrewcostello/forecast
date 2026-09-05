package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewcostello/forecast/internal/dispatched"
	"github.com/spf13/cobra"
)

func TestDispatchedReferenceBuildCommandIsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"dispatched-reference", "build"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd != dispatchedReferenceBuildCmd {
		t.Fatalf("found %q, want dispatched-reference build", cmd.CommandPath())
	}
	for _, name := range []string{
		"runs-dir", "out", "features-repo", "tasks", "min-observations",
		"max-history-commits", "fail-on-empty-required", "fail-on-uncovered-required", "timeout",
	} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing --%s", name)
		}
	}
}

// The command is the operator entrypoint: every flag must reach the builder,
// the artifact must land where --out says, and the coverage report must print.
// Driving RunE rather than buildDispatchedReference is the point — a RunE that
// dropped a flag would still pass a test that called the helper directly.
func TestDispatchedReferenceBuildRunEWiresFlagsToTheBuilder(t *testing.T) {
	fixture := newCommandFixture(t)
	out := filepath.Join(t.TempDir(), "nested", "reference.json")
	stdout := runBuildCommand(t, map[string]string{
		"runs-dir": fixture.runs, "out": out, "features-repo": fixture.repo,
		"tasks": fixture.target, "min-observations": "1",
	}, nil)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var artifact dispatched.Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact.Observations) != 1 || artifact.Observations[0].Model != "stamp" {
		t.Fatalf("artifact observations = %+v", artifact.Observations)
	}
	if artifact.Coverage.MinObservations != 1 {
		t.Fatalf("--min-observations did not reach the builder: %+v", artifact.Coverage)
	}
	if artifact.Coverage.TargetTasks != fixture.target {
		t.Fatalf("--tasks did not reach the builder: %q", artifact.Coverage.TargetTasks)
	}
	if artifact.Coverage.HistoryCommits != 1 {
		t.Fatalf("--features-repo did not reach the builder: %+v", artifact.Coverage)
	}
	for _, want := range []string{
		"Dispatched reference-class coverage",
		"bodies/stamp",
		"required seals/never-seen: target_rows=1 n=0 n_done=0 empty=true covered=false",
		"Rows recovered vs journal starts: 1/1",
		"Reference class written to " + out,
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("report omits %q:\n%s", want, stdout)
		}
	}
}

// A report nobody can gate on is a report. --fail-on-empty-required lets CI
// and the later prediction units refuse rather than forecast into a hole.
func TestDispatchedReferenceBuildFailsOnEmptyRequiredCell(t *testing.T) {
	fixture := newCommandFixture(t)
	out := filepath.Join(t.TempDir(), "reference.json")
	var err error
	runBuildCommand(t, map[string]string{
		"runs-dir": fixture.runs, "out": out, "features-repo": fixture.repo,
		"tasks": fixture.target, "fail-on-empty-required": "true",
	}, &err)
	if err == nil {
		t.Fatal("empty required cell did not fail the command")
	}
	if !strings.Contains(err.Error(), "never-seen") {
		t.Fatalf("error does not name the empty cell: %v", err)
	}
	// The artifact is still written: the report is the deliverable, and the
	// non-zero exit is a gate on top of it.
	if _, statErr := os.Stat(out); statErr != nil {
		t.Fatalf("artifact missing after a gated failure: %v", statErr)
	}
}

func TestDispatchedReferenceBuildWrapsOutputError(t *testing.T) {
	fixture := newCommandFixture(t)
	unwritable := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(unwritable, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(unwritable, "reference.json")
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})

	err := buildDispatchedReference(context.Background(), cmd,
		dispatched.BuildOptions{RunsDir: fixture.runs, FeaturesRepo: fixture.repo}, out, false, false)
	if !errors.Is(err, dispatched.ErrReferenceOutput) {
		t.Fatalf("error = %v, want ErrReferenceOutput", err)
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Fatal("a partial artifact was left behind")
	}
}

func TestDispatchedReferenceBuildRequiresAFeaturesRepo(t *testing.T) {
	var err error
	runBuildCommand(t, map[string]string{
		"runs-dir": t.TempDir(), "out": filepath.Join(t.TempDir(), "o.json"), "features-repo": "",
	}, &err)
	if err == nil || !strings.Contains(err.Error(), "--features-repo is required") {
		t.Fatalf("error = %v, want a --features-repo requirement", err)
	}
}

// runBuildCommand sets flags on the real command and drives its RunE, then
// restores every flag it touched. If wantErr is nil the call must succeed.
func runBuildCommand(t *testing.T, flags map[string]string, wantErr *error) string {
	t.Helper()
	cmd := newDispatchedReferenceBuildCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetContext(context.Background())
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	err := cmd.RunE(cmd, nil)
	if wantErr != nil {
		*wantErr = err
	} else if err != nil {
		t.Fatal(err)
	}
	return stdout.String()
}

type commandFixture struct {
	repo   string
	runs   string
	target string
}

// newCommandFixture builds the smallest tree the command can be pointed at:
// one journal run, one live tasks YAML in a git repository, and a target list
// naming one covered cell and one that has never been observed.
func newCommandFixture(t *testing.T) commandFixture {
	t.Helper()
	root := t.TempDir()
	fixture := commandFixture{
		repo:   filepath.Join(root, "repo"),
		runs:   filepath.Join(root, "runs"),
		target: filepath.Join(root, "target.yaml"),
	}
	yamlPath := filepath.Join(fixture.repo, "features", "study", "tasks.yaml")
	mustMkdirAll(t, filepath.Dir(yamlPath))
	mustMkdirAll(t, filepath.Join(fixture.runs, "run"))
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "T"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", fixture.repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile(t, yamlPath, "tasks:\n"+
		"  - key: ROW\n    role: bodies\n    model: authored\n    status: Done\n"+
		"    started_at: '2026-01-01T00:00:00Z'\n    completed_at: '2026-01-01T01:00:00Z'\n"+
		"    dispatcher_run_id: run\n")
	writeFile(t, fixture.target, "tasks:\n"+
		"  - key: T1\n    role: bodies\n    model: stamp\n"+
		"  - key: T2\n    role: seals\n    model: never-seen\n")
	writeFile(t, filepath.Join(fixture.runs, "run", "journal.jsonl"), strings.Join([]string{
		`{"event_type":"task_started","task_key":"ROW","timestamp":"2026-01-01T00:00:00Z","payload":{"model":"authored"}}`,
		`{"event_type":"task_spawn_finished","task_key":"ROW","timestamp":"2026-01-01T00:30:00Z","payload":{"spawn_kind":"implementer","model":"stamp","cost_usd":1.5}}`,
		`{"event_type":"task_done","task_key":"ROW","timestamp":"2026-01-01T01:00:00Z","payload":null}`,
	}, "\n")+"\n")
	for _, args := range [][]string{{"add", "features"}, {"commit", "-q", "-m", "fixture"}} {
		if out, err := exec.Command("git", append([]string{"-C", fixture.repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	return fixture
}

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestCoverageGateRejectsMalformedTargets(t *testing.T) {
	for _, data := range []string{"foo: bar\n", "tasks:\n - key: A\n   role: wrong\n   model: stamp\n", "tasks:\n - key: A\n   role: bodies\n"} {
		fixture := newCommandFixture(t)
		writeFile(t, fixture.target, data)
		var err error
		runBuildCommand(t, map[string]string{"runs-dir": fixture.runs, "features-repo": fixture.repo, "out": filepath.Join(t.TempDir(), "o.json"), "tasks": fixture.target, "fail-on-empty-required": "true"}, &err)
		if !errors.Is(err, dispatched.ErrYAMLSource) {
			t.Fatalf("malformed target passed: %v", err)
		}
	}
}

func TestCoverageGateRejectsInsufficientCompletedObservations(t *testing.T) {
	for _, status := range []string{"task_done", "task_blocked"} {
		t.Run(status, func(t *testing.T) {
			fixture := newCommandFixture(t)
			writeFile(t, fixture.target, "tasks:\n - key: TARGET\n   role: bodies\n   model: stamp\n")
			journal := filepath.Join(fixture.runs, "run", "journal.jsonl")
			data, err := os.ReadFile(journal)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, journal, strings.ReplaceAll(string(data), "task_done", status))
			var gotErr error
			runBuildCommand(t, map[string]string{"runs-dir": fixture.runs, "features-repo": fixture.repo, "out": filepath.Join(t.TempDir(), "o.json"), "tasks": fixture.target, "fail-on-uncovered-required": "true"}, &gotErr)
			if gotErr == nil || !strings.Contains(gotErr.Error(), "bodies/stamp") {
				t.Fatalf("insufficient completed evidence passed: %v", gotErr)
			}
		})
	}
}

func TestBuildCommandAcceptsRepeatedSourceRepositories(t *testing.T) {
	a, b := newCommandFixture(t), newCommandFixture(t)
	cmd := newDispatchedReferenceBuildCmd()
	out := filepath.Join(t.TempDir(), "union.json")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--runs-dir", a.runs, "--features-repo", a.repo, "--features-repo", b.repo, "--out", out})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var artifact dispatched.Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if len(artifact.Coverage.Repositories) != 2 || len(artifact.Observations) != 1 {
		t.Fatalf("repeated flags or dedupe failed: %+v", artifact)
	}
}
