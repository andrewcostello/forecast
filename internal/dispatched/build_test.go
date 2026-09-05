package dispatched

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
	"time"
)

func TestJournalFactsPortsDevelopmentReviewRoundsAndTokens(t *testing.T) {
	runs := t.TempDir()
	run := "run-1"
	writeJournal(t, runs, run,
		event("task_started", "A", "2026-01-01T00:00:00Z", map[string]any{"model": "planned"}),
		event("task_spawn_finished", "A", "2026-01-01T00:10:00Z", map[string]any{"spawn_kind": "implementer", "model": "actual", "input_tokens": 10, "output_tokens": 4, "cost_usd": 1.25}),
		event("panel_started", "A", "2026-01-01T00:10:00Z", nil),
		event("panel_verdict", "A", "2026-01-01T00:15:00Z", nil),
		event("agent_fallback", "A", "2026-01-01T00:15:00Z", map[string]any{"from_agent": "claude", "to_agent": "codex"}),
		event("panel_iterate", "A", "2026-01-01T00:15:00Z", nil),
		event("task_spawn_finished", "A", "2026-01-01T00:25:00Z", map[string]any{"spawn_kind": "panel-iterate", "model": "actual", "input_tokens": 20, "output_tokens": 8, "cost_usd": 2.5}),
		event("task_spawn_finished", "A", "2026-01-01T00:26:00Z", map[string]any{"spawn_kind": "verifier", "model": "verifier-model", "input_tokens": 99, "output_tokens": 99, "cost_usd": 99}),
		event("task_done", "A", "2026-01-01T00:30:00Z", nil),
	)

	sources, err := readJournals(context.Background(), runs)
	if err != nil {
		t.Fatal(err)
	}
	row := sources.Rows[runTask{RunID: run, Key: "A"}]
	if len(row.Attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(row.Attempts))
	}
	got := row.Attempts[0]
	if got.Model != "actual" || got.DevElapsed != 20*time.Minute || got.ReviewElapsed != 5*time.Minute || got.Rounds != 1 || got.Fallbacks != 1 {
		t.Fatalf("facts = %+v", got)
	}
	// The verifier spawn is not an implementing spawn; the reference ignores
	// it for model and for token totals alike.
	if got.InputTokens != 30 || got.OutputTokens != 12 || got.CostUSD != 3.75 || !got.CostKnown {
		t.Fatalf("totals = input %d output %d cost %.2f known %t", got.InputTokens, got.OutputTokens, got.CostUSD, got.CostKnown)
	}
	if got.TerminalOutcome != OutcomeDone {
		t.Fatalf("terminal = %s", got.TerminalOutcome)
	}
}

// Edge case 3. The model on task_started is the model the dispatcher PLANNED,
// stamped before any cascade. Using it attributes a cascaded row to a model
// that never ran, which is exactly what the assignment forbids.
func TestPlannedModelOnTaskStartedNeverAttributesARow(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(taskYAML("CASCADED", "bodies", "planned-model", "Done", start, "2026-01-01T01:00:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "CASCADED", start, map[string]any{"model": "planned-model"}),
		event("agent_fallback", "CASCADED", "2026-01-01T00:05:00Z", map[string]any{"from_agent": "claude", "to_agent": "codex"}),
		// The cascade ran, but the dispatcher stamped no model on the spawn.
		event("task_spawn_finished", "CASCADED", "2026-01-01T00:50:00Z", map[string]any{"spawn_kind": "implementer"}),
		event("task_done", "CASCADED", "2026-01-01T01:00:00Z", nil),
	)

	result := fixture.build(t)
	if len(result.Artifact.Observations) != 0 {
		t.Fatalf("row was attributed to the pre-cascade planned model: %+v", result.Artifact.Observations)
	}
	c := result.Artifact.Coverage
	if c.JournalAttemptsWithoutStamp != 1 || c.UnattributableJoinedRows != 1 || c.RecoveryShortfall != 1 {
		t.Fatalf("unstamped attempt was not named in the report: %+v", c)
	}
	for _, cell := range result.Table.Cells() {
		if cell.Model == "planned-model" {
			t.Fatalf("planned model became a cell: %v", cell)
		}
	}
}

// Edge case 3, the other half: when a cascade DOES stamp a model, that stamp
// is the cell and the cascade itself is surfaced.
func TestCascadeUsesStampedModelAndIsCounted(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(taskYAML("CASCADED", "bodies", "authored-pin", "Done", start, "2026-01-01T01:00:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "CASCADED", start, map[string]any{"model": "authored-pin"}),
		event("agent_fallback", "CASCADED", "2026-01-01T00:05:00Z", map[string]any{"to_agent": "codex"}),
		spawnEvent("CASCADED", "2026-01-01T00:50:00Z", "implementer", "stamped-model", 1, 1, 0.5),
		event("task_done", "CASCADED", "2026-01-01T01:00:00Z", nil),
	)

	result := fixture.build(t)
	if len(result.Artifact.Observations) != 1 {
		t.Fatalf("observations = %+v", result.Artifact.Observations)
	}
	row := result.Artifact.Observations[0]
	if row.Model != "stamped-model" {
		t.Fatalf("model = %q, want the stamp", row.Model)
	}
	if row.Cascades != 1 || result.Artifact.Coverage.RowsWithCascade != 1 {
		t.Fatalf("cascade not surfaced: row=%+v coverage=%+v", row, result.Artifact.Coverage)
	}
	if result.Artifact.Coverage.AuthoredStampMismatches != 1 {
		t.Fatalf("authored/stamp disagreement not counted: %+v", result.Artifact.Coverage)
	}
	if row.TerminalEvidence != terminalEvidenceJournal {
		t.Fatalf("terminal evidence = %q, want journal", row.TerminalEvidence)
	}
}

// Edge case 1. started_at with no completed_at is not a duration.
func TestStartedWithoutCompletedAtIsCensored(t *testing.T) {
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	row, err := observationFrom(taskSnapshot{
		Key: "A", Role: RoleBodies, AuthoredModel: "model", Status: "In Progress",
		StartedAt: started, DispatcherRunID: "run", Revision: Revision{Source: SourceLive},
	}, &JournalFacts{Model: "model"}, started.Add(9*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if row.Outcome != OutcomeUnfinished || !row.Censored() || row.Elapsed != 9*time.Hour {
		t.Fatalf("unfinished row = %+v", row)
	}
	if row.TerminalEvidence != terminalEvidenceNone {
		t.Fatalf("terminal evidence = %q, want none", row.TerminalEvidence)
	}
	if _, ok := row.Duration(); ok {
		t.Fatal("started_at without completed_at yielded a duration")
	}
}

// Edge case 1, the trap underneath it: a row still marked in progress can
// carry a completed_at left behind by an earlier attempt. Its lower bound
// runs to now; ending it at the stale stamp understates unfinished work.
func TestNonTerminalStatusIgnoresStaleCompletedAt(t *testing.T) {
	started := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	now := started.Add(9 * time.Hour)
	row, err := observationFrom(taskSnapshot{
		Key: "A", Role: RoleBodies, Status: "In Progress",
		StartedAt: started, CompletedAt: started.Add(time.Minute),
		DispatcherRunID: "run", Revision: Revision{Source: SourceLive},
	}, &JournalFacts{Model: "model"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if row.Outcome != OutcomeUnfinished || !row.Censored() {
		t.Fatalf("row = %+v, want a censored unfinished row", row)
	}
	if row.Elapsed != 9*time.Hour {
		t.Fatalf("elapsed = %v, want the lower bound to run to now, not to the stale completed_at", row.Elapsed)
	}
}

func TestObservationRejectsTerminalBeforeStart(t *testing.T) {
	started := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	_, err := observationFrom(taskSnapshot{
		Key: "STALE", Role: RoleBodies, Status: "Done",
		StartedAt: started, CompletedAt: started.Add(-time.Hour), DispatcherRunID: "run",
		Revision: Revision{Source: SourceGit, Commit: "abc123"},
	}, &JournalFacts{Model: "model"}, started.Add(time.Hour))
	if !errors.Is(err, ErrNegativeValue) {
		t.Fatalf("observationFrom error = %v, want ErrNegativeValue", err)
	}
}

func TestStaleCompletedAtIsUnrecoverableWithoutAbortingBuild(t *testing.T) {
	fixture := newBuildFixture(t)
	staleStart := "2026-01-02T00:00:00Z"
	validStart := "2026-01-03T00:00:00Z"
	fixture.commitYAML(tasksYAML(
		taskYAML("STALE", "bodies", "authored", "Done", staleStart, "2026-01-01T00:00:00Z", "run-stale"),
		taskYAML("VALID", "bodies", "authored", "Done", validStart, "2026-01-03T01:00:00Z", "run-valid"),
	))
	writeJournal(t, fixture.runs, "run-stale",
		event("task_started", "STALE", staleStart, map[string]any{"model": "planned"}),
		spawnEvent("STALE", staleStart, "implementer", "model", 0, 0, 0))
	writeJournal(t, fixture.runs, "run-valid",
		event("task_started", "VALID", validStart, map[string]any{"model": "planned"}),
		spawnEvent("VALID", validStart, "implementer", "model", 0, 0, 0),
		event("task_done", "VALID", "2026-01-03T01:00:00Z", nil))

	result := fixture.build(t)
	coverage := result.Artifact.Coverage
	if coverage.JournalStartedRows != 2 || coverage.RecoveredRows != 1 || coverage.RecoveryShortfall != 1 || coverage.UnrecoverableJoinedRows != 1 {
		t.Fatalf("coverage = %+v", coverage)
	}
	if len(result.Artifact.Observations) != 1 || result.Artifact.Observations[0].Key != "VALID" {
		t.Fatalf("observations = %+v", result.Artifact.Observations)
	}

	out := filepath.Join(fixture.root, "reference.json")
	if err := WriteArtifact(out, result.Artifact); err != nil {
		t.Fatalf("write artifact after unrecoverable row: %v", err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Fatalf("artifact mode = %v, want 0644", perm)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var artifact Artifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatal(err)
	}
	if artifact.Coverage.UnrecoverableJoinedRows != 1 || artifact.Coverage.RecoveryShortfall != 1 {
		t.Fatalf("written coverage = %+v", artifact.Coverage)
	}

	var report bytes.Buffer
	WriteCoverage(&report, result.Artifact)
	if !strings.Contains(report.String(), "recovery shortfall=1") || !strings.Contains(report.String(), "Joined rows unrecoverable after validation: 1") {
		t.Fatalf("coverage report did not name the shortfall and reason:\n%s", report.String())
	}
}

func TestValidRevisionRecoversRowDespiteStaleHistoricalReading(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-02T00:00:00Z"
	fixture.commitYAML(tasksYAML(
		taskYAML("ROW", "bodies", "authored", "Done", start, "2026-01-01T00:00:00Z", "run"),
	))
	fixture.writeLive(tasksYAML(
		taskYAML("ROW", "bodies", "authored", "Done", start, "2026-01-02T01:00:00Z", "run"),
	))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "ROW", start, map[string]any{"model": "planned"}),
		spawnEvent("ROW", start, "implementer", "model", 0, 0, 0))

	result := fixture.build(t)
	coverage := result.Artifact.Coverage
	if coverage.RecoveredRows != 1 || coverage.RecoveryShortfall != 0 || coverage.UnrecoverableJoinedRows != 0 {
		t.Fatalf("coverage = %+v", coverage)
	}
	if len(result.Artifact.Observations) != 1 {
		t.Fatalf("observations = %+v", result.Artifact.Observations)
	}
	// The journal has no terminal event, so the outcome rests on a YAML
	// status. That must be visible in the artifact.
	if got := result.Artifact.Observations[0].TerminalEvidence; got != terminalEvidenceYAML {
		t.Fatalf("terminal evidence = %q, want yaml", got)
	}
	if result.Artifact.Coverage.RowsWithYAMLOnlyTerminalEvidence != 1 {
		t.Fatalf("yaml-only terminal evidence not counted: %+v", result.Artifact.Coverage)
	}
}

// Edge case 2 and the ruling on record: a censored elapsed time is a lower
// bound. This test fails if a blocked row's eight hours reaches a mean.
func TestBlockedElapsedNeverReachesDurationMean(t *testing.T) {
	cell := Cell{Role: RoleBodies, Model: "model"}
	table := NewTable(cell)
	if err := table.Add(obs("done", cell, OutcomeDone, 2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	blocked := obs("blocked", cell, OutcomeBlocked, 8*time.Hour)
	blocked.Rounds = 8
	if err := table.Add(blocked); err != nil {
		t.Fatal(err)
	}
	unfinished := obs("unfinished", cell, OutcomeUnfinished, 12*time.Hour)
	if err := table.Add(unfinished); err != nil {
		t.Fatal(err)
	}
	got := summarizeCells(table)[0]
	if got.N != 3 || got.NDone != 1 || got.NBlocked != 1 || got.NCensored != 2 {
		t.Fatalf("counts = %+v", got)
	}
	if got.Duration.N != 1 || got.Duration.Mean != (2*time.Hour).Seconds() {
		t.Fatalf("duration = %+v; censored 8h and 12h must not enter the mean", got.Duration)
	}
	if got.Duration.Max != (2 * time.Hour).Seconds() {
		t.Fatalf("duration max = %v; a censored row reached a duration statistic", got.Duration.Max)
	}
	if got.Rounds.N != 3 || got.Rounds.Max != 8 {
		t.Fatalf("rounds = %+v; censored work must remain reported", got.Rounds)
	}
}

// Edge case 4 has a second reading the assignment does not spell out: one run
// may restart a task. Both task_started events are the same (run, key), but
// they are different attempts with different stamps and different totals.
func TestRestartWithinOneRunIsPartitionedIntoAttempts(t *testing.T) {
	fixture := newBuildFixture(t)
	first := "2026-01-01T00:00:00Z"
	second := "2026-01-01T01:00:00Z"
	fixture.commitYAML(tasksYAML(taskYAML("R", "bodies", "pin", "In Progress", first, "", "run")))
	fixture.writeLive(tasksYAML(taskYAML("R", "bodies", "pin", "Done", second, "2026-01-01T01:30:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "R", first, map[string]any{"model": "planned"}),
		spawnEvent("R", "2026-01-01T00:10:00Z", "implementer", "first-model", 10, 5, 1),
		event("task_started", "R", second, map[string]any{"model": "planned"}),
		spawnEvent("R", "2026-01-01T01:10:00Z", "implementer", "second-model", 20, 6, 2),
		event("task_done", "R", "2026-01-01T01:30:00Z", nil),
	)

	result := fixture.build(t)
	c := result.Artifact.Coverage
	if c.JournalStartedRows != 1 || c.JournalStartAttempts != 2 || c.JournalRestarts != 1 {
		t.Fatalf("restart accounting = %+v", c)
	}
	if c.RecoveredRows != 1 || c.RecoveredObservations != 2 || c.RecoveryShortfall != 0 {
		t.Fatalf("restart recovery = %+v; one started row yielded two attempts", c)
	}
	if len(result.Artifact.Observations) != 2 {
		t.Fatalf("observations = %+v", result.Artifact.Observations)
	}
	byModel := map[string]ReferenceObservation{}
	for _, row := range result.Artifact.Observations {
		byModel[row.Model] = row
	}
	firstAttempt, ok := byModel["first-model"]
	if !ok {
		t.Fatalf("first attempt lost its own stamp: %+v", result.Artifact.Observations)
	}
	if firstAttempt.Outcome != "unfinished" || !firstAttempt.Censored {
		t.Fatalf("first attempt = %+v; the terminal event belongs to the second", firstAttempt)
	}
	if firstAttempt.InputTokens != 10 {
		t.Fatalf("first attempt tokens = %d; attempts must not pool", firstAttempt.InputTokens)
	}
	secondAttempt := byModel["second-model"]
	if secondAttempt.Outcome != "done" || secondAttempt.InputTokens != 20 {
		t.Fatalf("second attempt = %+v", secondAttempt)
	}
}

// Edge case 5. A cell nobody has observed is present with n=0, and is not
// substituted, pooled or omitted.
func TestUnionIncludesLiveOnlyAndHistoryOnlyRowsAndEmptyTargetCell(t *testing.T) {
	fixture := newBuildFixture(t)
	oldStart := "2026-01-01T00:00:00Z"
	newStart := "2026-01-02T00:00:00Z"
	fixture.commitYAML(tasksYAML(taskYAML("OLD", "scaffold", "old-authored", "Done", oldStart, "2026-01-01T01:00:00Z", "run-old")))
	fixture.writeLive(tasksYAML(taskYAML("NEW", "bodies", "new-authored", "Done", newStart, "2026-01-02T02:00:00Z", "run-new")))
	writeJournal(t, fixture.runs, "run-old",
		event("task_started", "OLD", oldStart, map[string]any{"model": "planned"}),
		spawnEvent("OLD", oldStart, "implementer", "old-model", 0, 0, 0),
		event("task_done", "OLD", "2026-01-01T01:00:00Z", nil))
	writeJournal(t, fixture.runs, "run-new",
		event("task_started", "NEW", newStart, map[string]any{"model": "planned"}),
		spawnEvent("NEW", newStart, "implementer", "new-model", 0, 0, 0),
		event("task_done", "NEW", "2026-01-02T02:00:00Z", nil))
	fixture.writeTarget(tasksYAML(
		taskYAML("T1", "scaffold", "old-model", "To Do", "", "", ""),
		taskYAML("T2", "bodies", "new-model", "To Do", "", "", ""),
		taskYAML("T3", "seals", "never-seen", "To Do", "", "", ""),
	))

	result := fixture.build(t)
	if result.Artifact.Coverage.RecoveredRows != 2 || len(result.Artifact.Observations) != 2 {
		t.Fatalf("recovered=%d observations=%d", result.Artifact.Coverage.RecoveredRows, len(result.Artifact.Observations))
	}
	if result.Artifact.Coverage.LiveYAMLReadings != 1 || result.Artifact.Coverage.HistoricalYAMLReadings != 1 {
		t.Fatalf("source readings = live %d history %d", result.Artifact.Coverage.LiveYAMLReadings, result.Artifact.Coverage.HistoricalYAMLReadings)
	}
	coverage := result.Artifact.Coverage
	if len(coverage.EmptyRequiredCells) != 1 || coverage.EmptyRequiredCells[0] != (Cell{Role: RoleSeals, Model: "never-seen"}) {
		t.Fatalf("empty required cells = %v", coverage.EmptyRequiredCells)
	}
	if n, present := result.Table.Count(Cell{Role: RoleSeals, Model: "never-seen"}); !present || n.N() != 0 {
		t.Fatalf("empty target cell present=%t count=%+v", present, n)
	}
	// One completed row is below the default threshold, so nothing is
	// covered: the report refuses rather than forecasting from a sample.
	if coverage.TargetCoveredShare == nil || *coverage.TargetCoveredShare != 0 {
		t.Fatalf("target covered share = %v, want 0 at min_observations=%d", coverage.TargetCoveredShare, coverage.MinObservations)
	}
	if len(coverage.UncoveredRequiredCells) != 3 {
		t.Fatalf("uncovered required cells = %v", coverage.UncoveredRequiredCells)
	}
	var report bytes.Buffer
	WriteCoverage(&report, result.Artifact)
	for _, want := range []string{
		"required seals/never-seen: target_rows=1 n=0 n_done=0 empty=true covered=false",
		"Required cells empty: 1; required cells not covered: 3",
		"Target rows naming a (role, model) cell: 3/3",
		"Target rows in a covered cell: 0/3 (0.0%)",
	} {
		if !strings.Contains(report.String(), want) {
			t.Fatalf("report omits %q:\n%s", want, report.String())
		}
	}
	// A cell with one row IS covered once the operator lowers the bar, and
	// the empty cell still is not.
	relaxed := fixture.buildWith(t, BuildOptions{MinObservations: 1})
	rc := relaxed.Artifact.Coverage
	if rc.TargetCoveredShare == nil || *rc.TargetCoveredShare != 2.0/3.0 {
		t.Fatalf("relaxed covered share = %v", rc.TargetCoveredShare)
	}
	if len(rc.EmptyRequiredCells) != 1 {
		t.Fatalf("relaxing the threshold filled an empty cell: %v", rc.EmptyRequiredCells)
	}
}

// Edge case 4. Two revisions giving different stamps for one (key,
// started_at) is a re-run reusing a key: surfaced, never last-write-wins.
func TestDuplicateRevisionsDedupeAndConflictingStampsAreSurfaced(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.commitYAML(tasksYAML(taskYAML("RERUN", "seals", "pin-one", "Done", start, "2026-01-01T01:00:00Z", "run-one")))
	fixture.commitYAML(tasksYAML(taskYAML("RERUN", "seals", "pin-two", "Done", start, "2026-01-01T02:00:00Z", "run-two")))
	writeJournal(t, fixture.runs, "run-one",
		event("task_started", "RERUN", start, map[string]any{"model": "planned"}),
		spawnEvent("RERUN", start, "implementer", "stamp-one", 0, 0, 0),
		event("task_done", "RERUN", "2026-01-01T01:00:00Z", nil))
	writeJournal(t, fixture.runs, "run-two",
		event("task_started", "RERUN", start, map[string]any{"model": "planned"}),
		spawnEvent("RERUN", start, "implementer", "stamp-two", 0, 0, 0),
		event("task_done", "RERUN", "2026-01-01T02:00:00Z", nil))

	result := fixture.build(t)
	if result.Artifact.Coverage.StampConflictRows != 1 || len(result.Artifact.Conflicts) != 1 {
		t.Fatalf("conflicts = %+v", result.Artifact.Conflicts)
	}
	if result.Artifact.Coverage.RecoveredRows != 0 || len(result.Artifact.Observations) != 0 {
		t.Fatalf("conflicting rerun was retained: %+v", result.Artifact.Observations)
	}
	// The operator acting on a re-run needs to know which run and revision
	// each disagreeing stamp came from.
	reason := result.Artifact.Conflicts[0].Reason
	for _, want := range []string{"run-one", "run-two", "stamp-one", "stamp-two"} {
		if !strings.Contains(reason, want) {
			t.Fatalf("conflict reason %q omits %q", reason, want)
		}
	}
}

// Edge case 8. Timestamps carry UTC offsets; equal instants are one row.
func TestSameRowAcrossRevisionsAndOffsetsDedupes(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.commitYAML(tasksYAML(taskYAML("A", "scaffold", "pin", "Done", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", "run")))
	fixture.commitYAML(tasksYAML(taskYAML("A", "scaffold", "pin", "Done", "2025-12-31T17:00:00-07:00", "2025-12-31T18:00:00-07:00", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "A", "2025-12-31T17:00:00-07:00", map[string]any{"model": "planned"}),
		spawnEvent("A", "2025-12-31T17:00:00-07:00", "implementer", "stamp", 0, 0, 0),
		event("task_done", "A", "2025-12-31T18:00:00-07:00", nil))

	result := fixture.build(t)
	if result.Artifact.Coverage.RecoveredRows != 1 || len(result.Artifact.Observations) != 1 {
		t.Fatalf("equivalent UTC instants did not dedupe: coverage=%+v rows=%+v", result.Artifact.Coverage, result.Artifact.Observations)
	}
	if got := result.Artifact.Observations[0].ElapsedSeconds; got != time.Hour.Seconds() {
		t.Fatalf("elapsed = %v, want one hour compared as instants", got)
	}
}

// Edge cases 6 and 7: a start with no model, and a start with no recoverable
// YAML. Both are counted; neither is guessed at or dropped.
func TestMissingModelAndMissingYAMLAreNamedInRecoveryShortfall(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.commitYAML(tasksYAML(taskYAML("NO-MODEL", "bodies", "do-not-guess", "Done", start, "2026-01-01T01:00:00Z", "run")))
	fixture.writeLive(tasksYAML(taskYAML("NO-MODEL", "bodies", "do-not-guess", "Done", start, "2026-01-01T01:00:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "NO-MODEL", start, map[string]any{"model": nil}),
		event("task_done", "NO-MODEL", "2026-01-01T01:00:00Z", nil),
		event("task_started", "NO-YAML", "2026-01-02T00:00:00Z", map[string]any{"model": "planned"}),
		spawnEvent("NO-YAML", "2026-01-02T00:00:00Z", "implementer", "stamp", 0, 0, 0),
		event("task_done", "NO-YAML", "2026-01-02T01:00:00Z", nil),
	)

	result := fixture.build(t)
	c := result.Artifact.Coverage
	if c.JournalStartedRows != 2 || c.TaskStartsWithoutModel != 1 || c.JournalRunTasksWithoutYAML != 1 || c.RecoveredRows != 0 || c.RecoveryShortfall != 2 {
		t.Fatalf("coverage = %+v", c)
	}
	if len(result.Artifact.Observations) != 0 {
		t.Fatal("authored model was guessed for an unstamped row")
	}
	var report bytes.Buffer
	WriteCoverage(&report, result.Artifact)
	for _, want := range []string{
		"task_started events carrying no planned model: 1",
		"Journal run/task keys with no recoverable YAML: 1",
		"Rows recovered vs journal starts: 0/2; recovery shortfall=2",
	} {
		if !strings.Contains(report.String(), want) {
			t.Fatalf("report omits %q:\n%s", want, report.String())
		}
	}
}

// Corrupt input reduces what can be recovered, so it must not silently reduce
// the denominator too: the shortfall is only honest if unreadable lines are
// named.
func TestUnreadableJournalLinesAreCountedNotDropped(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(taskYAML("OK", "bodies", "pin", "Done", start, "2026-01-01T01:00:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		`{"event_type":"task_started","task_key":"TRUNC"`,
		event("task_started", "BAD-TIME", "not-a-timestamp", nil),
		event("task_started", "OK", start, map[string]any{"model": "planned"}),
		spawnEvent("OK", start, "implementer", "stamp", 0, 0, 0),
		event("task_done", "OK", "2026-01-01T01:00:00Z", nil),
	)

	result := fixture.build(t)
	c := result.Artifact.Coverage
	if c.JournalLinesUnparsed != 1 || c.JournalEventsWithBadTimestamp != 1 {
		t.Fatalf("unreadable journal input not counted: %+v", c)
	}
	if c.RecoveredRows != 1 || len(result.Artifact.Observations) != 1 {
		t.Fatalf("a corrupt line stopped the recoverable row: %+v", result.Artifact.Observations)
	}
	var report bytes.Buffer
	WriteCoverage(&report, result.Artifact)
	if !strings.Contains(report.String(), "Journal lines unparsed: 1; events with an unreadable timestamp: 1") {
		t.Fatalf("report omits unreadable journal input:\n%s", report.String())
	}
}

// features/ holds YAML that is not a task list. One such file, or one corrupt
// historical revision, must not abort an otherwise recoverable union.
func TestUnparseableYAMLIsCountedNotFatal(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(taskYAML("OK", "bodies", "pin", "Done", start, "2026-01-01T01:00:00Z", "run")))
	fixture.writeSibling("notes.yaml", "tasks: [: this is not yaml\n")
	fixture.writeSibling("config.yaml", tasksYAML(taskYAML("BAD-TIME", "bodies", "pin", "Done", "not-a-timestamp", "", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "OK", start, map[string]any{"model": "planned"}),
		spawnEvent("OK", start, "implementer", "stamp", 0, 0, 0),
		event("task_done", "OK", "2026-01-01T01:00:00Z", nil))

	result := fixture.build(t)
	c := result.Artifact.Coverage
	if c.UnparseableYAMLDocs != 1 || c.MalformedYAMLRows != 1 {
		t.Fatalf("unreadable YAML not counted: %+v", c)
	}
	if c.RecoveredRows != 1 {
		t.Fatalf("one unreadable YAML poisoned the union: %+v", c)
	}
}

// A cost nobody recorded is not a cost of zero.
func TestUnrecordedCostSerializesAsNull(t *testing.T) {
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(
		taskYAML("PRICED", "bodies", "pin", "Done", start, "2026-01-01T01:00:00Z", "run"),
		taskYAML("UNPRICED", "seals", "pin", "Done", start, "2026-01-01T01:00:00Z", "run"),
	))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "PRICED", start, map[string]any{"model": "planned"}),
		spawnEvent("PRICED", start, "implementer", "stamp", 1, 1, 4.5),
		event("task_done", "PRICED", "2026-01-01T01:00:00Z", nil),
		event("task_started", "UNPRICED", start, map[string]any{"model": "planned"}),
		event("task_spawn_finished", "UNPRICED", start, map[string]any{"spawn_kind": "implementer", "model": "stamp"}),
		event("task_done", "UNPRICED", "2026-01-01T01:00:00Z", nil),
	)

	result := fixture.build(t)
	byKey := map[string]ReferenceObservation{}
	for _, row := range result.Artifact.Observations {
		byKey[row.Key] = row
	}
	if got := byKey["PRICED"].CostUSD; got == nil || *got != 4.5 {
		t.Fatalf("priced cost = %v", got)
	}
	if byKey["UNPRICED"].CostUSD != nil {
		t.Fatalf("unmeasured cost serialized as %v, want null", *byKey["UNPRICED"].CostUSD)
	}
	if result.Artifact.Coverage.RowsWithoutRecordedCost != 1 {
		t.Fatalf("unmeasured cost not counted: %+v", result.Artifact.Coverage)
	}
	data, err := json.Marshal(result.Artifact.Observations)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"cost_usd":null`) {
		t.Fatalf("artifact JSON has no null cost: %s", data)
	}
}

// Edge case 9. The limit is stated, not modelled around. Codex-3: the
// formatter fixture is not enough; Build itself must populate Limits.
func TestCoverageStatesHandFinishedLimit(t *testing.T) {
	artifact := Artifact{Limits: []string{HandFinishedLimit}}
	var out bytes.Buffer
	WriteCoverage(&out, artifact)
	if !strings.Contains(out.String(), "Hand-finished rows have no identifying field") {
		t.Fatalf("coverage report omitted hand-finished limit:\n%s", out.String())
	}
	fixture := newBuildFixture(t)
	start := "2026-01-01T00:00:00Z"
	fixture.writeLive(tasksYAML(taskYAML("A", "bodies", "pin", "Done", start, "2026-01-01T00:10:00Z", "run")))
	writeJournal(t, fixture.runs, "run",
		event("task_started", "A", start, map[string]any{"model": "planned"}),
		spawnEvent("A", "2026-01-01T00:10:00Z", "implementer", "stamp", 1, 1, 1),
		event("task_done", "A", "2026-01-01T00:10:00Z", nil),
	)
	result := fixture.build(t)
	found := false
	for _, limit := range result.Artifact.Limits {
		if limit == HandFinishedLimit {
			found = true
		}
	}
	if !found {
		t.Fatalf("Build omitted HandFinishedLimit: %v", result.Artifact.Limits)
	}
	out.Reset()
	WriteCoverage(&out, result.Artifact)
	if !strings.Contains(out.String(), "Hand-finished rows have no identifying field") {
		t.Fatalf("Build report omitted hand-finished limit:\n%s", out.String())
	}
}

func TestSourceErrorsWrapSentinels(t *testing.T) {
	ctx := context.Background()
	_, err := readJournals(ctx, filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrJournalSource) {
		t.Fatalf("readJournals error = %v", err)
	}
	_, err = readLiveSnapshots(ctx, filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, ErrYAMLSource) {
		t.Fatalf("readLiveSnapshots error = %v", err)
	}
	_, err = readGitSnapshots(ctx, filepath.Join(t.TempDir(), "not-a-repo"), 0)
	if !errors.Is(err, ErrGitHistory) {
		t.Fatalf("readGitSnapshots error = %v", err)
	}
	if !strings.Contains(err.Error(), "not-a-repo") {
		t.Fatalf("git error names neither command nor repository: %v", err)
	}
	if err := WriteArtifact(filepath.Join("/dev/null", "out.json"), Artifact{}); !errors.Is(err, ErrReferenceOutput) {
		t.Fatalf("WriteArtifact error = %v", err)
	}
}

func TestCancelledContextStopsTheBuild(t *testing.T) {
	fixture := newBuildFixture(t)
	fixture.commitYAML(tasksYAML(taskYAML("A", "bodies", "pin", "Done", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", "run")))
	writeJournal(t, fixture.runs, "run", event("task_started", "A", "2026-01-01T00:00:00Z", nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Build(ctx, BuildOptions{RunsDir: fixture.runs, FeaturesRepo: fixture.repo}); err == nil {
		t.Fatal("Build ignored a cancelled context")
	}
}

// The history walk reads each distinct blob once, through one child process,
// so cost is linear in file CONTENTS rather than commits x files.
func TestHistoryWalkReadsEachDistinctBlobOnce(t *testing.T) {
	fixture := newBuildFixture(t)
	yaml := tasksYAML(taskYAML("A", "bodies", "pin", "Done", "2026-01-01T00:00:00Z", "2026-01-01T01:00:00Z", "run"))
	fixture.commitYAML(yaml)
	for i := 0; i < 4; i++ {
		fixture.writeSibling("filler.txt", strings.Repeat("x", i+1))
		runGit(t, fixture.repo, "add", "features")
		runGit(t, fixture.repo, "commit", "-q", "-m", "unrelated")
	}
	sources, err := readGitSnapshots(context.Background(), fixture.repo, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sources.Commits != 5 {
		t.Fatalf("commits = %d, want 5", sources.Commits)
	}
	if sources.Blobs != 1 || len(sources.Snapshots) != 1 {
		t.Fatalf("blobs=%d snapshots=%d; an unchanged file was reread per commit", sources.Blobs, len(sources.Snapshots))
	}

	capped, err := readGitSnapshots(context.Background(), fixture.repo, 2)
	if err != nil {
		t.Fatal(err)
	}
	if capped.Commits != 2 || !capped.Truncated {
		t.Fatalf("cap not honoured: commits=%d truncated=%t", capped.Commits, capped.Truncated)
	}
}

type buildFixture struct {
	t        *testing.T
	root     string
	repo     string
	runs     string
	yamlPath string
	target   string
}

func newBuildFixture(t *testing.T) *buildFixture {
	t.Helper()
	root := t.TempDir()
	fixture := &buildFixture{t: t, root: root, repo: filepath.Join(root, "repo"), runs: filepath.Join(root, "runs")}
	fixture.yamlPath = filepath.Join(fixture.repo, "features", "study", "tasks.yaml")
	fixture.target = filepath.Join(root, "target.yaml")
	mustMkdir(t, filepath.Dir(fixture.yamlPath))
	mustMkdir(t, fixture.runs)
	runGit(t, fixture.repo, "init", "-q")
	runGit(t, fixture.repo, "config", "user.email", "test@example.com")
	runGit(t, fixture.repo, "config", "user.name", "Test")
	return fixture
}

func (f *buildFixture) writeLive(data string) {
	f.t.Helper()
	if err := os.WriteFile(f.yamlPath, []byte(data), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *buildFixture) writeSibling(name, data string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(filepath.Dir(f.yamlPath), name), []byte(data), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *buildFixture) commitYAML(data string) {
	f.t.Helper()
	f.writeLive(data)
	runGit(f.t, f.repo, "add", "features")
	runGit(f.t, f.repo, "commit", "-q", "-m", "fixture")
}

func (f *buildFixture) writeTarget(data string) {
	f.t.Helper()
	if err := os.WriteFile(f.target, []byte(data), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *buildFixture) build(t *testing.T) *BuildResult {
	t.Helper()
	return f.buildWith(t, BuildOptions{})
}

func (f *buildFixture) buildWith(t *testing.T, opts BuildOptions) *BuildResult {
	t.Helper()
	opts.RunsDir, opts.FeaturesRepo = f.runs, f.repo
	if _, err := os.Stat(f.target); err == nil {
		opts.TargetTasks = f.target
	}
	if opts.Now.IsZero() {
		opts.Now = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	}
	result, err := Build(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func taskYAML(key, role, model, status, started, completed, run string) string {
	var fields strings.Builder
	fmtField := func(name, value string) {
		if value != "" {
			fields.WriteString("    " + name + ": '" + value + "'\n")
		}
	}
	fields.WriteString("  - key: " + key + "\n")
	fmtField("role", role)
	fmtField("model", model)
	fmtField("status", status)
	fmtField("started_at", started)
	fmtField("completed_at", completed)
	fmtField("dispatcher_run_id", run)
	return fields.String()
}

func tasksYAML(tasks ...string) string { return "tasks:\n" + strings.Join(tasks, "") }

func event(kind, key, at string, payload map[string]any) string {
	data, err := json.Marshal(map[string]any{"event_type": kind, "task_key": key, "timestamp": at, "payload": payload})
	if err != nil {
		panic(err)
	}
	return string(data)
}

// spawnEvent is the only journal event that may stamp a model on a row.
func spawnEvent(key, at, kind, model string, in, out int64, cost float64) string {
	return event("task_spawn_finished", key, at, map[string]any{
		"spawn_kind": kind, "model": model,
		"input_tokens": in, "output_tokens": out, "cost_usd": cost,
	})
}

func writeJournal(t *testing.T, runs, run string, events ...string) {
	t.Helper()
	dir := filepath.Join(runs, run)
	mustMkdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte(strings.Join(events, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
