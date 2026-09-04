package dispatched

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildUnionsRepositoriesAndRetainsSource(t *testing.T) {
	a, b := newBuildFixture(t), newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	end := "2026-01-01T01:00:00Z"
	a.commitYAML(tasksYAML(taskYAML("A", "bodies", "pin", "Done", start, end, "run")))
	b.commitYAML(tasksYAML(taskYAML("B", "adjudicate", "pin", "Done", start, end, "run")))
	b.writeLive("tasks: []\n") // B is recoverable only from the second repo's history.
	writeJournal(t, a.runs, "run",
		event("task_started", "A", start, nil), spawnEvent("A", end, "implementer", "stamp", 1, 2, 3), event("task_done", "A", end, nil),
		event("task_started", "B", start, nil), spawnEvent("B", end, "implementer", "stamp", 4, 5, 6), event("task_done", "B", end, nil))
	result, err := Build(context.Background(), BuildOptions{RunsDir: a.runs, FeaturesRepos: []string{a.repo, b.repo, a.repo}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifact.Observations) != 2 || len(result.Artifact.Coverage.Repositories) != 2 {
		t.Fatalf("union = %+v", result.Artifact)
	}
	for _, row := range result.Artifact.Observations {
		want := a.repo
		if row.Key == "B" {
			want = b.repo
		}
		if row.SourceRepository != want || row.SourcePath != "features/study/tasks.yaml" {
			t.Fatalf("source = %+v", row)
		}
	}
	for _, repo := range result.Artifact.Coverage.Repositories {
		if repo.MatchedAttempts != 1 {
			t.Fatalf("repository coverage = %+v", repo)
		}
	}
}

func TestHistoricalTimestampCannotClaimAnotherAttempt(t *testing.T) {
	f := newBuildFixture(t)
	start, end := "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z"
	f.commitYAML(tasksYAML(taskYAML("A", "bodies", "pin", "Done", "2026-01-01T00:00:01Z", end, "run")))
	f.writeLive(tasksYAML(taskYAML("A", "bodies", "pin", "Done", "2025-12-31T17:00:00-07:00", end, "run")))
	writeJournal(t, f.runs, "run", event("task_started", "A", start, nil), spawnEvent("A", end, "implementer", "stamp", 10, 20, 3), event("task_done", "A", end, nil),
		event("task_started", "A", "2026-01-02T00:00:00Z", nil))
	result := f.build(t)
	c := result.Artifact.Coverage
	if len(result.Artifact.Observations) != 1 || result.Artifact.Observations[0].InputTokens != 10 {
		t.Fatalf("attempt was duplicated: %+v", result.Artifact.Observations)
	}
	if c.SnapshotsWithoutMatchingAttempt != 1 || c.AttemptsWithoutMatchingYAML != 1 || c.AttemptRecoveryShortfall != 1 || c.RecoveredAttempts != 1 {
		t.Fatalf("shortfall = %+v", c)
	}
}

func TestAmbiguousStartCannotSupplyEvidence(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row := JournalRow{Attempts: []*JournalFacts{{StartedAt: start}, {StartedAt: start}}}
	if row.Match(start) != nil {
		t.Fatal("ambiguous start matched")
	}
	if (&JournalRow{Attempts: []*JournalFacts{{Model: "stamp"}}}).Match(start) != nil {
		t.Fatal("unstated start matched")
	}
}

func TestTaskDocumentShapeAndTargetCellsAreValidated(t *testing.T) {
	for name, data := range map[string]string{
		"mapping": "foo: bar\n", "root sequence": "- item\n", "null tasks": "tasks: null\n", "mapping tasks": "tasks: {}\n", "scalar tasks": "tasks: nope\n",
	} {
		t.Run(name, func(t *testing.T) {
			sources := parseSnapshots([]byte(data), Revision{Source: SourceLive})
			if sources.UnparseableDocuments != 1 {
				t.Fatalf("bad shape not counted: %+v", sources)
			}
			path := filepath.Join(t.TempDir(), "tasks.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readTargetTasks(path); !errors.Is(err, ErrYAMLSource) {
				t.Fatalf("bad target accepted: %v", err)
			}
		})
	}
	for name, data := range map[string]string{
		"missing model": "tasks:\n - key: A\n   role: bodies\n", "invalid role": "tasks:\n - key: A\n   role: typo\n   model: stamp\n", "blank model": "tasks:\n - key: A\n   role: bodies\n   model: ' '\n", "duplicate key": "tasks:\n - {key: A, role: bodies, model: stamp}\n - {key: A, role: bodies, model: stamp}\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tasks.yaml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := readTargetTasks(path); !errors.Is(err, ErrYAMLSource) {
				t.Fatalf("invalid cell accepted: %v", err)
			}
		})
	}
	sources := parseSnapshots([]byte("tasks:\n - key: A\n"), Revision{Source: SourceLive})
	if sources.MissingJoinKeys != 1 {
		t.Fatalf("missing keys not counted: %+v", sources)
	}
}

func TestHistoryUsesBoundedBatchedGitProcesses(t *testing.T) {
	f := newBuildFixture(t)
	f.commitYAML(tasksYAML(taskYAML("A", "bodies", "stamp", "Done", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", "run")))
	for i := 0; i < 12; i++ {
		f.writeSibling("other.txt", strings.Repeat("x", i+1))
		runGit(t, f.repo, "add", "features")
		runGit(t, f.repo, "commit", "-q", "-m", "unrelated")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "calls")
	// Observe process count without changing Git's behavior.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$FC_GIT_LOG\"\nexec \"$FC_REAL_GIT\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FC_GIT_LOG", log)
	t.Setenv("FC_REAL_GIT", git)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	sources, err := readGitSnapshots(context.Background(), f.repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if len(bytes.Split(bytes.TrimSpace(calls), []byte("\n"))) != 3 || sources.Blobs != 1 || sources.Commits != 13 {
		t.Fatalf("history scaled by commits: %+v; calls %s", sources, calls)
	}
	if err := os.WriteFile(log, nil, 0600); err != nil {
		t.Fatal(err)
	}
	capped, err := readGitSnapshots(context.Background(), f.repo, 2)
	if err != nil {
		t.Fatal(err)
	}
	calls, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if capped.Commits != 2 || !capped.Truncated || !strings.Contains(string(calls), "--max-count=3") {
		t.Fatalf("unbounded enumeration: %+v; calls %s", capped, calls)
	}
}

func TestHistoryRecoversDeletedAndRenamedTaskFiles(t *testing.T) {
	f := newBuildFixture(t)
	f.commitYAML(tasksYAML(taskYAML("A", "bodies", "stamp", "Done", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", "run")))
	runGit(t, f.repo, "mv", "features/study/tasks.yaml", "features/study/renamed tasks.yaml")
	runGit(t, f.repo, "commit", "-q", "-m", "rename")
	runGit(t, f.repo, "rm", "features/study/renamed tasks.yaml")
	runGit(t, f.repo, "commit", "-q", "-m", "remove")
	sources, err := readGitSnapshots(context.Background(), f.repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources.Snapshots) != 1 || sources.Snapshots[0].Key != "A" {
		t.Fatalf("history lost deleted file: %+v", sources)
	}
}

func TestConflictUsesOriginalTerminalReadings(t *testing.T) {
	unfinished := obs("A", seenCell, OutcomeUnfinished, time.Hour)
	unfinished.Provenance.RunID = "unrelated"
	done := unfinished
	done.Outcome = OutcomeDone
	done.Provenance = gitReading
	done.Provenance.RunID = "completed"
	blocked := done
	blocked.Outcome = OutcomeBlocked
	blocked.Provenance.RunID = "blocked"
	_, err := joinReadings([]Observation{unfinished, done, blocked})
	if !errors.Is(err, ErrStampConflict) || !strings.Contains(err.Error(), "completed") || !strings.Contains(err.Error(), "blocked") || strings.Contains(err.Error(), "unrelated") {
		t.Fatalf("wrong conflict evidence: %v", err)
	}
}

func TestNegativeCascadesRejected(t *testing.T) {
	row := obs("A", seenCell, OutcomeDone, time.Hour)
	row.Cascades = -1
	if !errors.Is(row.Validate(), ErrNegativeValue) {
		t.Fatal("negative cascades accepted")
	}
}

type cancelAfterRead struct {
	reader *strings.Reader
	cancel context.CancelFunc
	reads  int
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.reads++
	if r.reads == 2 {
		r.cancel()
	}
	return n, err
}

func TestCancellationDuringJournalScanStopsExtraction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelAfterRead{reader: strings.NewReader(strings.Repeat("not json\n", 100000)), cancel: cancel}
	sources := &journalSources{Rows: make(map[runTask]*JournalRow)}
	err := scanJournal(ctx, reader, "fixture", "run", sources)
	if !errors.Is(err, context.Canceled) || sources.LinesUnparsed == 0 || sources.LinesUnparsed >= 100000 {
		t.Fatalf("in-flight cancellation ignored: %v (%d lines)", err, sources.LinesUnparsed)
	}
}
