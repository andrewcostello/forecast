package dispatched

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

// TestFCJournalContract is the reserved FC-JOURNAL group. Cases stay red
// against ParseEvents/ReduceAttempts/SummarizeWall/Attempt JSON until that
// body lands. No t.Skip and no implementation edits.
func TestFCJournalContract(t *testing.T) {
	t.Run("F1-ID-UTC-OFFSET", testF1IDUTCOffset)
	t.Run("F1-ID-AMBIGUOUS-START", testF1IDAmbiguousStart)
	t.Run("F1-JOURNAL-PRODUCER-RESOLVED", testF1JournalProducerResolved)
	t.Run("F1-EV-TOKENS-CITED-SEPARATELY", testF1EVTokensCitedSeparately)
	t.Run("F1-EV-PROVENANCE-KEPT", testF1EVProvenanceKept)
	t.Run("F1-EV-UNKNOWN-STAYS-UNKNOWN", testF1EVUnknownStaysUnknown)
	t.Run("F1-EV-MODEL-CONFLICT", testF1EVModelConflict)
	t.Run("F1-EV-TERMINAL-CONFLICT", testF1EVTerminalConflict)
	t.Run("F1-MODEL-CLOSING-STAMP", testF1ModelClosingStamp)
	t.Run("F1-MODEL-ABSENT-STAMP", testF1ModelAbsentStamp)
	t.Run("F1-HASH-PROVENANCE", testF1HashProvenance)
	t.Run("F1-CONFLICT-PORTABLE", testF1ConflictPortable)
	t.Run("F1-ROW-EQUALITY-BY-CONTENT", testF1RowEqualityByContent)
	t.Run("F2-ELAPSED-TERMINAL", testF2ElapsedTerminal)
	t.Run("F2-ELAPSED-CUTOFF", testF2ElapsedCutoff)
	t.Run("F2-ELAPSED-BLOCKED-CENSORED", testF2ElapsedBlockedCensored)
	t.Run("F2-ELAPSED-SURVIVES-UNKNOWN-PHASES", testF2ElapsedSurvivesUnknownPhases)
	t.Run("F2-PHASES-DISJOINT", testF2PhasesDisjoint)
	t.Run("F2-PHASES-PANEL-WALL", testF2PhasesPanelWall)
	t.Run("F2-PHASES-ITERATE-AFTER-SPAWN", testF2PhasesIterateAfterSpawn)
	t.Run("F2-PHASES-INFERRED-LABELED", testF2PhasesInferredLabeled)
	t.Run("F2-ROUNDS-VS-REVIEWS", testF2RoundsVsReviews)
	t.Run("F2-COST-NULL-VS-ZERO", testF2CostNullVsZero)
	t.Run("F2-COST-NO-DOUBLE-SUM", testF2CostNoDoubleSum)
	t.Run("F2-MEASURE-NONFINITE", testF2MeasureNonfinite)
	t.Run("F2-MEASURE-REVERSED", testF2MeasureReversed)
	t.Run("F2-WALL-ABSENT-START", testF2WallAbsentStart)
	t.Run("F2-UNCLASSIFIED-RESIDUAL-ONLY", testF2UnclassifiedResidualOnly)
	t.Run("F2-CANONICAL-ORDER", testF2CanonicalOrder)
	t.Run("F2-PARTIAL-SUM-UNKNOWN", testF2PartialSumUnknown)
	t.Run("F2-SPAWN-WIRE", testF2SpawnWire)
	t.Run("F2-CORRECTIVE-BOUNDARY", testF2CorrectiveBoundary)
	t.Run("F2-PRODUCER-SHAPES", testF2ProducerShapes)
	t.Run("F2-CORRECTION-KINDS", testF2CorrectionKinds)
	t.Run("F2-VERIFICATIONS", testF2Verifications)
	t.Run("F2-EVENT-IDENTITY", testF2EventIdentity)
	t.Run("F2-ALL-SPAWN-COST", testF2AllSpawnCost)
	t.Run("F2-ARITHMETIC-ERRORS", testF2ArithmeticErrors)
	t.Run("F2-LATER-START", testF2LaterStart)
	t.Run("F2-INITIAL-SPAWN", testF2InitialSpawn)
	t.Run("F2-ALL-KINDS", testF2AllKinds)
	t.Run("F2-CITATION-MEMBERSHIP", testF2CitationMembership)
	t.Run("F2-SUMMARY-AVAILABILITY", testF2SummaryAvailability)
	t.Run("F4-OUTCOME-WIRE", testF4OutcomeWire)
	t.Run("F2-DESIGN-SPAWN", testF2DesignSpawn)
	t.Run("F2-PRODUCER-DECLARATIONS", testF2ProducerDeclarations)
	t.Run("F2-LINE-ENVELOPE", testF2LineEnvelope)
	t.Run("F2-PARSER-TOTAL-CAP", testF2ParserTotalCap)
	t.Run("F2-WALL-PARENT-CONSISTENCY", testF2WallParentConsistency)
	t.Run("F3-CANCEL-PERSIST-PARSE", testF2ParseCancellationRetainsEvents)
	t.Run("F3-REDUCE-ZERO-CUTOFF", testF2ZeroCutoffRejected)
}

func testF1IDUTCOffset(t *testing.T) {
	set := reduceFixture(t, "run-offset", "synthetic-utc-offset.jsonl")
	got := oneAttempt(t, set)
	want := mustTime(t, "2026-01-01T00:00:00Z")
	if got.ID.RunID != "run-offset" || got.ID.Key != "F1-OFFSET" {
		t.Fatalf("id = %+v", got.ID)
	}
	requireInstant(t, got.ID.StartedAt, want, "AttemptID.StartedAt")
	requireInstant(t, got.Wall.StartedAt, want, "Wall.StartedAt")
}

func testF1IDAmbiguousStart(t *testing.T) {
	set := reduceFixture(t, "run-ambig", "synthetic-ambiguous-start.jsonl")
	if len(set.Attempts) != 0 {
		t.Fatalf("chose an attempt from an ambiguous identity: %+v", set.Attempts)
	}
	if len(set.Ambiguous) != 1 || set.Ambiguous[0].Starts != 2 {
		t.Fatalf("ambiguous = %+v, want one identity with Starts=2", set.Ambiguous)
	}
	if set.Ambiguous[0].ID != NewAttemptID("run-ambig", "F1-AMBIG", mustTime(t, "2026-01-01T00:00:00Z")) {
		t.Fatalf("ambiguous id = %+v", set.Ambiguous[0].ID)
	}
}

func testF1JournalProducerResolved(t *testing.T) {
	with := parseFixture(t, "run-prod", "synthetic-producer-only.jsonl")
	if with.Journal.Producer != ProducerDispatcherV0_1_0 || with.Diagnostics.MissingProducer || len(with.Events) != 0 {
		t.Fatalf("producer-only journal = %+v", with)
	}
	missing := parseFixture(t, "run-noprod", "synthetic-missing-producer.jsonl")
	if missing.Journal.Producer != "" || !missing.Diagnostics.MissingProducer {
		t.Fatalf("missing producer = %+v", missing)
	}
}

func testF1EVTokensCitedSeparately(t *testing.T) {
	set := reduceLines(t, "run-tok",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "T", "2026-01-01T00:00:00Z", map[string]any{"model": "planned"}),
		journalLine(2, EventTaskSpawnFinished, "T", "2026-01-01T00:01:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 60000, "output_tokens": 4,
		}),
		journalLine(3, EventTaskDone, "T", "2026-01-01T00:01:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	if len(got.OutputTokenEvents) == 0 {
		t.Fatal("output token event missing")
	}
	if len(got.InputTokenEvents) != 0 {
		t.Fatalf("input token events = %+v, want none", got.InputTokenEvents)
	}
	requireUnknown(t, got.InputTokens, "InputTokens")
	requireKnown(t, got.OutputTokens, int64(4), "OutputTokens")
	if got.Evidence.InputTokens.Source == got.Evidence.OutputTokens.Source && got.Evidence.OutputTokens.Source != EvidenceNone {
		t.Fatal("input and output token evidence collapsed together")
	}
}

func testF1EVProvenanceKept(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-prov", "synthetic-disjoint-phases.jsonl"))
	if got.Evidence.Start.Source != EvidenceJournal || got.Evidence.Start.Event.Type != EventTaskStarted {
		t.Fatalf("start evidence = %+v", got.Evidence.Start)
	}
	if got.Evidence.Terminal.Source != EvidenceJournal || got.Evidence.Elapsed.Source != EvidenceJournal {
		t.Fatalf("terminal/elapsed evidence = %+v / %+v", got.Evidence.Terminal, got.Evidence.Elapsed)
	}
	if got.Evidence.Model.Source != EvidenceJournal {
		t.Fatalf("model evidence = %+v", got.Evidence.Model)
	}
	if got.Corrections > 0 && (got.Evidence.Corrections.Source != EvidenceJournal || len(got.CorrectionEvents) == 0) {
		t.Fatalf("corrections evidence missing: %+v events=%d", got.Evidence.Corrections, len(got.CorrectionEvents))
	}
	if got.CostUSD.Known && (len(got.CostEvents) == 0 || got.Evidence.Cost.Event != got.CostEvents[0]) {
		t.Fatalf("cost citation is not the least CostEvents entry: evidence=%+v events=%+v", got.Evidence.Cost, got.CostEvents)
	}
}

func testF1EVUnknownStaysUnknown(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-open", "synthetic-unfinished.jsonl"))
	if got.Outcome != OutcomeUnfinished {
		t.Fatalf("outcome = %s, want unfinished", got.Outcome)
	}
	if got.Evidence.Terminal.Source != EvidenceNone {
		t.Fatalf("terminal source = %q, want none", got.Evidence.Terminal.Source)
	}
	censored, err := got.Censored()
	if err != nil || !censored {
		t.Fatalf("Censored = %t, %v, want true", censored, err)
	}
	if got.Elapsed != contractCutoff().Sub(got.ID.StartedAt) {
		t.Fatalf("elapsed = %s, want cutoff-start", got.Elapsed)
	}
}

func testF1EVModelConflict(t *testing.T) {
	// Two implementing spawns, different models, no cascade marker: last
	// physical implementing event wins. Not a conflict.
	set := reduceLines(t, "run-model",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "M", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "M", "2026-01-01T00:05:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "first", "duration_ms": 300000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(3, EventTaskSpawnFinished, "M", "2026-01-01T00:10:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "second", "duration_ms": 300000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(4, EventTaskDone, "M", "2026-01-01T00:10:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	requireKnown(t, got.Model, "second", "Model")
	if got.Cascades != 0 {
		t.Fatalf("cascades = %d, want 0 (no agent_fallback)", got.Cascades)
	}
}

func testF1EVTerminalConflict(t *testing.T) {
	set := reduceFixture(t, "run-term", "synthetic-terminal-conflict.jsonl")
	if len(set.Attempts) != 0 {
		t.Fatalf("recovered a conflicting terminal: %+v", set.Attempts)
	}
	if len(set.Conflicts) != 1 || set.Conflicts[0].Field != "terminal" {
		t.Fatalf("conflicts = %+v, want Field=terminal", set.Conflicts)
	}
	if set.Conflicts[0].Code != EvidenceConflictCode {
		t.Fatalf("code = %q", set.Conflicts[0].Code)
	}
	if !errors.Is(set.Conflicts[0].Err, ErrEvidenceConflict) && set.Conflicts[0].Err != nil {
		t.Fatalf("conflict err = %v, want ErrEvidenceConflict", set.Conflicts[0].Err)
	}
}

func testF1ModelClosingStamp(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-cascade", "synthetic-cascade-closing.jsonl"))
	requireKnown(t, got.Model, "sol", "Model")
	if got.Cascades != 1 {
		t.Fatalf("cascades = %d, want 1", got.Cascades)
	}
	if got.Model.Value == "opus" {
		t.Fatal("closing stamp pooled with the pre-cascade implementer")
	}
}

func testF1ModelAbsentStamp(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-nostamp", "synthetic-absent-stamp.jsonl"))
	requireUnknown(t, got.Model, "Model")
	if got.Evidence.Model.Source != EvidenceNone {
		t.Fatalf("model evidence = %+v, want none", got.Evidence.Model)
	}
}

func testF1HashProvenance(t *testing.T) {
	parsed := parseFixture(t, "run-hash", "recorded-panel-iterate.jsonl")
	if len(parsed.Events) == 0 {
		t.Fatal("no events")
	}
	ref := parsed.Events[0].Ref
	if ref.Type != EventTaskStarted {
		t.Fatalf("first task event type = %q, want %s", ref.Type, EventTaskStarted)
	}
	if ref.Hash != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("first task event hash = %q", ref.Hash)
	}
	if ref.PrevHash != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("first task event prev_hash = %q", ref.PrevHash)
	}
	found := false
	for _, ev := range parsed.Events {
		if ev.Ref.Type == EventTaskSpawnFinished && ev.Payload.SpawnKind == SpawnKindPanelIterate {
			if ev.Ref.Hash != "4444444444444444444444444444444444444444444444444444444444444444" {
				t.Fatalf("panel-iterate spawn hash = %q", ev.Ref.Hash)
			}
			if ev.Ref.PrevHash != "3333333333333333333333333333333333333333333333333333333333333333" {
				t.Fatalf("panel-iterate spawn prev_hash = %q", ev.Ref.PrevHash)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("panel-iterate spawn missing")
	}
}

func testF1ConflictPortable(t *testing.T) {
	set := reduceFixture(t, "run-term", "synthetic-terminal-conflict.jsonl")
	if len(set.Conflicts) == 0 {
		t.Fatal("no portable conflict")
	}
	c := set.Conflicts[0]
	if c.AValue.Kind != "terminal" || c.BValue.Kind != "terminal" {
		t.Fatalf("kinds = %q / %q", c.AValue.Kind, c.BValue.Kind)
	}
	if len(c.AValue.Value) == 0 || len(c.BValue.Value) == 0 {
		t.Fatal("candidate JSON missing")
	}
	var payload map[string]any
	if err := json.Unmarshal(c.AValue.Value, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["outcome"]; !ok {
		t.Fatalf("terminal candidate missing outcome: %s", c.AValue.Value)
	}
}

func testF1RowEqualityByContent(t *testing.T) {
	base := parseFixture(t, "run-eq", "synthetic-disjoint-phases.jsonl")
	rev := base
	if len(rev.Events) > 1 {
		rev.Events = append([]Event{}, base.Events...)
		rev.Events[1], rev.Events[2] = rev.Events[2], rev.Events[1]
	}
	a, err := ReduceAttempts(base, contractCutoff())
	if err != nil {
		t.Fatal(err)
	}
	b, err := ReduceAttempts(rev, contractCutoff())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encodeJSON(t, a.Attempts), encodeJSON(t, b.Attempts)) {
		t.Fatalf("permutation changed canonical attempts:\n%s\n%s", encodeJSON(t, a.Attempts), encodeJSON(t, b.Attempts))
	}
}

func testF2ElapsedTerminal(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-done", "synthetic-utc-offset.jsonl"))
	if got.Outcome != OutcomeDone {
		t.Fatalf("outcome = %s", got.Outcome)
	}
	if got.Elapsed != 10*time.Minute {
		t.Fatalf("elapsed = %s, want 10m", got.Elapsed)
	}
	censored, err := got.Censored()
	if err != nil || censored {
		t.Fatalf("Censored = %t %v, want false", censored, err)
	}
}

func testF2ElapsedCutoff(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-open", "synthetic-unfinished.jsonl"))
	want := contractCutoff().Sub(mustTime(t, "2026-01-01T00:00:00Z"))
	if got.Elapsed != want {
		t.Fatalf("elapsed = %s, want %s", got.Elapsed, want)
	}
	censored, err := got.Censored()
	if err != nil || !censored {
		t.Fatalf("unfinished must be censored: %t %v", censored, err)
	}
}

func testF2ElapsedBlockedCensored(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-block", "synthetic-blocked.jsonl"))
	if got.Outcome != OutcomeBlocked {
		t.Fatalf("outcome = %s", got.Outcome)
	}
	if got.Elapsed != 8*time.Minute {
		t.Fatalf("elapsed = %s, want 8m lower bound", got.Elapsed)
	}
	censored, err := got.Censored()
	if err != nil || !censored {
		t.Fatalf("blocked must be censored: %t %v", censored, err)
	}
}

func testF2ElapsedSurvivesUnknownPhases(t *testing.T) {
	set := reduceLines(t, "run-nophase",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "P", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "P", "2026-01-01T00:10:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(3, EventTaskDone, "P", "2026-01-01T00:10:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	if got.Elapsed != 10*time.Minute {
		t.Fatalf("elapsed lost when duration_ms missing: %s", got.Elapsed)
	}
	if got.Wall.Complete {
		t.Fatal("wall marked complete without phase bounds")
	}
	if len(got.Wall.Intervals) != 0 {
		t.Fatalf("fabricated intervals: %+v", got.Wall.Intervals)
	}
	sum, err := SummarizeWall(got.Wall)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Complete || sum.Unclassified != got.Elapsed {
		t.Fatalf("summary = %+v, want unclassified=elapsed complete=false", sum)
	}
}

func testF2PhasesDisjoint(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-disj", "synthetic-disjoint-phases.jsonl"))
	requireDisjointIntervals(t, got.Wall)
	if classifiedSum(got.Wall) > got.Elapsed {
		t.Fatalf("classified %s exceeds elapsed %s", classifiedSum(got.Wall), got.Elapsed)
	}
	sum, err := SummarizeWall(got.Wall)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Development == 0 || sum.PanelReview == 0 {
		t.Fatalf("expected development and panel_review: %+v", sum)
	}
	if sum.Unclassified < 0 || sum.Development+sum.PanelReview+sum.Verifier+sum.Unclassified != got.Elapsed {
		t.Fatalf("residual arithmetic: %+v elapsed=%s", sum, got.Elapsed)
	}
}

func testF2PhasesPanelWall(t *testing.T) {
	// Three reviewer seats are not in the journal; one panel_started to
	// panel_verdict is one interval. Path-classification gate must not open one.
	set := reduceLines(t, "run-panel",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "R", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "R", "2026-01-01T00:05:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 300000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(3, EventPanelStarted, "R", "2026-01-01T00:05:00Z", map[string]any{"forced_by": ForcedByPathClassification}),
		journalLine(4, EventPanelStarted, "R", "2026-01-01T00:06:00Z", map[string]any{"iteration": 0, "iterations_remaining": 1}),
		journalLine(5, EventPanelVerdict, "R", "2026-01-01T00:09:00Z", map[string]any{}),
		journalLine(6, EventTaskDone, "R", "2026-01-01T00:10:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	if got.Reviews != 1 {
		t.Fatalf("reviews = %d, want 1 invocation (gate is not a review)", got.Reviews)
	}
	var panel []Interval
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhasePanelReview {
			panel = append(panel, iv)
		}
	}
	if len(panel) != 1 {
		t.Fatalf("panel intervals = %+v, want one wall span", panel)
	}
	if !panel[0].Start.Equal(mustTime(t, "2026-01-01T00:06:00Z")) || !panel[0].End.Equal(mustTime(t, "2026-01-01T00:09:00Z")) {
		t.Fatalf("panel interval = %+v, want invocation-shaped [06:00, 09:00)", panel[0])
	}
}

func testF2PhasesIterateAfterSpawn(t *testing.T) {
	parsed := parseFixture(t, "recorded-convergence-2026-09-02", "recorded-panel-iterate.jsonl")
	if parsed.Journal.Producer != ProducerDispatcherV0_1_0 {
		t.Fatalf("producer = %q", parsed.Journal.Producer)
	}
	var spawnAt, iterateAt time.Time
	var spawnIdx, iterateIdx int
	for i, ev := range parsed.Events {
		if ev.Ref.Type == EventTaskSpawnFinished && ev.Payload.SpawnKind == SpawnKindPanelIterate {
			spawnAt, spawnIdx = ev.Ref.At, i
		}
		if ev.Ref.Type == EventPanelIterate {
			iterateAt, iterateIdx = ev.Ref.At, i
		}
	}
	if spawnAt.IsZero() || iterateAt.IsZero() {
		t.Fatal("recorded fixture missing spawn or panel_iterate")
	}
	if iterateIdx <= spawnIdx {
		t.Fatalf("panel_iterate at index %d precedes spawn finish at %d (production order is spawn THEN iterate)", iterateIdx, spawnIdx)
	}

	set, err := ReduceAttempts(parsed, contractCutoff())
	if err != nil {
		t.Fatal(err)
	}
	got := oneAttempt(t, set)
	requireDisjointIntervals(t, got.Wall)
	finish := mustTime(t, "2026-09-02T09:43:17-07:00")
	dur := 234068 * time.Millisecond
	var found bool
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhaseDevelopment && iv.End.Equal(finish) {
			found = true
			if !iv.Start.Equal(finish.Add(-dur)) {
				t.Fatalf("iterate development start = %s, want finish-duration %s", iv.Start, finish.Add(-dur))
			}
			if iv.Inferred {
				t.Fatal("recorded duration_ms must not be labeled inferred")
			}
			for _, ref := range iv.Evidence {
				if ref.Type == EventPanelIterate {
					t.Fatal("panel_iterate used as a start boundary")
				}
			}
		}
	}
	if !found {
		t.Fatalf("no development interval ending at spawn finish; intervals=%+v", got.Wall.Intervals)
	}
}

func testF2PhasesInferredLabeled(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	wall := WallBreakdown{
		StartedAt: start,
		Elapsed:   time.Hour,
		Complete:  false,
		Intervals: []Interval{{
			Phase:    PhaseDevelopment,
			Start:    start,
			End:      start.Add(10 * time.Minute),
			Inferred: true,
			Evidence: []EventRef{{Type: EventTaskSpawnFinished, At: start.Add(10 * time.Minute)}},
		}},
	}
	sum, err := SummarizeWall(wall)
	if err != nil {
		t.Fatal(err)
	}
	if !wall.Intervals[0].Inferred {
		t.Fatal("SummarizeWall cleared Inferred")
	}
	if sum.Development != 10*time.Minute {
		t.Fatalf("summary development = %s", sum.Development)
	}
	got := oneAttempt(t, reduceFixture(t, "run-disj", "synthetic-disjoint-phases.jsonl"))
	for _, iv := range got.Wall.Intervals {
		if iv.Inferred {
			t.Fatalf("0.1.0 reducer emitted Inferred=true: %+v", iv)
		}
	}
}

func testF2RoundsVsReviews(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-disj", "synthetic-disjoint-phases.jsonl"))
	// first invocation + one after iterate = 2 reviews; one panel_iterate = 1 correction
	if got.Reviews != 2 {
		t.Fatalf("reviews = %d, want 2 invocations", got.Reviews)
	}
	if got.Corrections != 1 {
		t.Fatalf("corrections = %d, want 1 panel_iterate (not a review count)", got.Corrections)
	}
}

func testF2CostNullVsZero(t *testing.T) {
	nullCost := oneAttempt(t, reduceFixture(t, "run-null", "synthetic-null-cost.jsonl"))
	requireUnknown(t, nullCost.CostUSD, "null cost")
	zero := oneAttempt(t, reduceFixture(t, "run-zero", "synthetic-zero-cost.jsonl"))
	requireKnown(t, zero.CostUSD, 0.0, "zero cost")
}

func testF2CostNoDoubleSum(t *testing.T) {
	payload := map[string]any{"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 60000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1.5}
	set := reduceLines(t, "run-dup",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "D", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "D", "2026-01-01T00:01:00Z", payload),
		`{"seq":2,"hash":"x","prev_hash":"y","event_type":"task_spawn_finished","task_key":"D","timestamp":"2026-01-01T00:01:00Z","payload":{"spawn_kind":"implementer","model":"stamp","duration_ms":60000,"input_tokens":1,"output_tokens":1,"cost_usd":1.5}}`,
		journalLine(3, EventTaskDone, "D", "2026-01-01T00:01:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	requireKnown(t, got.CostUSD, 1.5, "cost summed once")
	if len(got.CostEvents) != 1 {
		t.Fatalf("CostEvents = %d, want 1 retransmission", len(got.CostEvents))
	}
}

func testF2MeasureNonfinite(t *testing.T) {
	body := strings.Join([]string{
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "N", "2026-01-01T00:00:00Z", map[string]any{}),
		`{"seq":2,"event_type":"task_spawn_finished","task_key":"N","timestamp":"2026-01-01T00:01:00Z","payload":{"spawn_kind":"implementer","model":"stamp","cost_usd":-1,"duration_ms":1}}`,
		`{"seq":3,"event_type":"task_spawn_finished","task_key":"N","timestamp":"2026-01-01T00:02:00Z","payload":{"spawn_kind":"implementer","model":"stamp","duration_ms":-5}}`,
		`{"seq":4,"event_type":"task_spawn_finished","task_key":"N","timestamp":"2026-01-01T00:03:00Z","payload":{"spawn_kind":"implementer","model":"stamp","input_tokens":"nope"}}`,
		journalLine(5, EventTaskDone, "N", "2026-01-01T00:04:00Z", map[string]any{}),
	}, "\n") + "\n"
	parsed, err := ParseEvents(context.Background(), journalID("run-bad", "bad.jsonl"), strings.NewReader(body), JournalBounds{})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Diagnostics.LinesUnparsed < 3 {
		t.Fatalf("LinesUnparsed = %d, want invalid quantities counted", parsed.Diagnostics.LinesUnparsed)
	}
}

func testF2MeasureReversed(t *testing.T) {
	set, err := ReduceAttempts(parseLines(t, "run-rev",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "R", "2026-01-01T00:10:00Z", map[string]any{}),
		journalLine(2, EventTaskDone, "R", "2026-01-01T00:00:00Z", map[string]any{}),
	), contractCutoff())
	if !errors.Is(err, ErrReversedInterval) {
		t.Fatalf("error = %v, want ErrReversedInterval", err)
	}
	_ = set
}

func testF2WallAbsentStart(t *testing.T) {
	_, err := SummarizeWall(WallBreakdown{
		Elapsed: time.Hour,
		Intervals: []Interval{{
			Phase: PhaseDevelopment,
			Start: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
			End:   time.Date(2099, 1, 1, 1, 0, 0, 0, time.UTC),
		}},
	})
	requireSentinel(t, err, ErrUnattributable)
}

func testF2UnclassifiedResidualOnly(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	_, err := SummarizeWall(WallBreakdown{
		StartedAt: start,
		Elapsed:   time.Hour,
		Intervals: []Interval{{Phase: PhaseUnclassified, Start: start, End: start.Add(time.Minute)}},
	})
	requireSentinel(t, err, ErrInvalidPhase)
}

func testF2CanonicalOrder(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	later := start.Add(20 * time.Minute)
	early := start.Add(time.Minute)
	_, err := SummarizeWall(WallBreakdown{
		StartedAt: start,
		Elapsed:   time.Hour,
		Intervals: []Interval{
			{Phase: PhaseDevelopment, Start: later, End: later.Add(time.Minute)},
			{Phase: PhaseDevelopment, Start: early, End: early.Add(time.Minute)},
		},
	})
	requireSentinel(t, err, ErrNonCanonicalEvidence)
}

func testF2PartialSumUnknown(t *testing.T) {
	set := reduceLines(t, "run-part",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "S", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "S", "2026-01-01T00:01:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 60000, "cost_usd": 1.0, "input_tokens": 2, "output_tokens": 3,
		}),
		journalLine(3, EventTaskSpawnFinished, "S", "2026-01-01T00:02:00Z", map[string]any{
			"spawn_kind": SpawnKindPanelIterate, "model": "stamp", "duration_ms": 60000,
		}),
		journalLine(4, EventPanelIterate, "S", "2026-01-01T00:02:00Z", map[string]any{"iteration": 1, "iterations_remaining": 0}),
		journalLine(5, EventTaskDone, "S", "2026-01-01T00:02:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	requireUnknown(t, got.CostUSD, "partial cost")
	if len(got.CostEvents) == 0 {
		t.Fatal("available cost citations discarded with the unknown total")
	}
}

func testF2SpawnWire(t *testing.T) {
	parsed := parseLines(t, "run-wire",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "W", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "W", "2026-01-01T00:01:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 1250, "cost_usd": 0, "input_tokens": 0,
		}),
	)
	if len(parsed.Events) < 2 {
		t.Fatalf("events = %d", len(parsed.Events))
	}
	p := parsed.Events[1].Payload
	if p.DurationMillis == nil || *p.DurationMillis != 1250 {
		t.Fatalf("duration_ms = %v", p.DurationMillis)
	}
	if p.CostUSD == nil || *p.CostUSD != 0 {
		t.Fatalf("measured zero cost lost: %v", p.CostUSD)
	}
	set, err := ReduceAttempts(parsed, contractCutoff())
	if err != nil {
		t.Fatal(err)
	}
	got := oneAttempt(t, set)
	var found bool
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhaseDevelopment && iv.End.Sub(iv.Start) == 1250*time.Millisecond {
			found = true
		}
	}
	if !found && got.Wall.Complete {
		t.Fatalf("duration_ms 1250 did not become 1.25s span: %+v", got.Wall.Intervals)
	}
}

func testF2CorrectiveBoundary(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-over", "synthetic-overlap-withheld.jsonl"))
	if got.Elapsed != 30*time.Minute {
		t.Fatalf("elapsed = %s, must survive withheld phases", got.Elapsed)
	}
	if got.Wall.Complete {
		t.Fatal("overlapping candidate component must make Complete=false")
	}
	// panel [10m,20m) and iterate duration 20m ending at 25m => [5m,25m) overlap.
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhasePanelReview || (iv.Phase == PhaseDevelopment && iv.End.Equal(mustTime(t, "2026-01-01T00:25:00Z"))) {
			t.Fatalf("overlapping component retained: %+v", iv)
		}
	}
}

func testF2ProducerShapes(t *testing.T) {
	parsed := parseLines(t, "run-shape",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "S", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventPanelStarted, "S", "2026-01-01T00:01:00Z", map[string]any{"reason": "unknown-shape"}),
		journalLine(3, EventTaskDone, "S", "2026-01-01T00:02:00Z", map[string]any{}),
	)
	if parsed.Diagnostics.LinesUnparsed < 1 {
		t.Fatalf("unknown panel_started shape not counted LinesUnparsed: %+v", parsed.Diagnostics)
	}
}

func testF2CorrectionKinds(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-kinds", "synthetic-all-kinds.jsonl"))
	// panel_iterate + verification_iterate + test-fix-retry + commit-retry + push-retry + summary-recovery
	if got.Corrections != 6 {
		t.Fatalf("corrections = %d, want 6 counted kinds", got.Corrections)
	}
	if len(got.CorrectionEvents) != got.Corrections {
		t.Fatalf("CorrectionEvents = %d, corrections = %d", len(got.CorrectionEvents), got.Corrections)
	}
}

func testF2Verifications(t *testing.T) {
	set := reduceLines(t, "run-ver",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "V", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "V", "2026-01-01T00:01:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 60000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(3, EventVerificationStarted, "V", "2026-01-01T00:01:00Z", map[string]any{"iteration": 0}),
		journalLine(4, EventVerificationMechanical, "V", "2026-01-01T00:01:00Z", map[string]any{}),
		journalLine(5, EventVerificationSkipped, "V", "2026-01-01T00:01:30Z", map[string]any{}),
		journalLine(6, EventTaskDone, "V", "2026-01-01T00:02:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	if got.Verifications != 1 {
		t.Fatalf("verifications = %d, want 1 started (skipped/mechanical do not count)", got.Verifications)
	}
	if got.Wall.Complete {
		t.Fatal("missing verification_verdict must leave Complete=false")
	}
}

func testF2EventIdentity(t *testing.T) {
	payload := map[string]any{"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 1000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1}
	parsed := parseLines(t, "run-id",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "I", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "I", "2026-01-01T00:01:00Z", payload),
		`{"seq":2,"hash":"dup","prev_hash":"x","event_type":"task_spawn_finished","task_key":"I","timestamp":"2026-01-01T00:01:00Z","payload":{"spawn_kind":"implementer","model":"stamp","duration_ms":1000,"input_tokens":1,"output_tokens":1,"cost_usd":1}}`,
		`{"seq":3,"hash":"c1","event_type":"task_spawn_finished","task_key":"I","timestamp":"2026-01-01T00:01:00Z","payload":{"spawn_kind":"implementer","model":"other","duration_ms":1000}}`,
		`{"seq":3,"hash":"c2","event_type":"task_spawn_finished","task_key":"I","timestamp":"2026-01-01T00:01:00Z","payload":{"spawn_kind":"implementer","model":"other2","duration_ms":1000}}`,
		journalLine(4, EventTaskDone, "I", "2026-01-01T00:02:00Z", map[string]any{}),
	)
	if parsed.Diagnostics.LinesUnparsed < 2 {
		t.Fatalf("colliding seq=3 lines should all be unparsed: %+v", parsed.Diagnostics)
	}
	spawns := 0
	for _, ev := range parsed.Events {
		if ev.Ref.Type == EventTaskSpawnFinished {
			spawns++
		}
	}
	if spawns != 1 {
		t.Fatalf("spawn events = %d, want 1 retained retransmission", spawns)
	}
}

func testF2AllSpawnCost(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-kinds", "synthetic-all-kinds.jsonl"))
	// 0.1+0.2+0.05+0.01+0.01+0.01+0.3+0.4+0.02 = 1.10
	requireKnown(t, got.CostUSD, 1.10, "all spawn kinds")
	if got.CostScope != CostScopeRecordedSpawns {
		t.Fatalf("cost scope = %q", got.CostScope)
	}
	if got.InputTokens.Known && got.InputTokens.Value != 15 {
		// uncached only: 1+2+1+1+1+1+3+4+1 = 15; cache tokens are absent here
		t.Fatalf("input tokens = %v, want uncached 15", got.InputTokens)
	}
}

func testF2ArithmeticErrors(t *testing.T) {
	huge := int64(math.MaxInt64 / int64(time.Millisecond))
	parsed := parseLines(t, "run-ovf",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "O", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "O", "2026-01-01T00:01:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": huge, "cost_usd": 1, "input_tokens": 1, "output_tokens": 1,
		}),
	)
	if parsed.Diagnostics.LinesUnparsed == 0 {
		_, err := ReduceAttempts(parsed, contractCutoff())
		if err != nil && !errors.Is(err, ErrMeasurementOverflow) && !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("overflow path = %v", err)
		}
		if err == nil {
			t.Fatal("overflow duration produced a finite attempt")
		}
		if errors.Is(err, ErrNotImplemented) {
			t.Fatal(err)
		}
	}
}

func testF2LaterStart(t *testing.T) {
	set := reduceFixture(t, "run-late", "synthetic-later-start.jsonl")
	if len(set.Attempts) != 0 || len(set.Ambiguous) != 0 {
		t.Fatalf("post-cutoff start created an attempt: %+v", set)
	}
	if set.StartsAfterCutoff != 1 {
		t.Fatalf("StartsAfterCutoff = %d, want 1", set.StartsAfterCutoff)
	}
}

func testF2InitialSpawn(t *testing.T) {
	set := reduceLines(t, "run-init",
		journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}),
		journalLine(1, EventTaskStarted, "I", "2026-01-01T00:00:00Z", map[string]any{}),
		journalLine(2, EventTaskSpawnFinished, "I", "2026-01-01T00:10:00Z", map[string]any{
			"spawn_kind": SpawnKindImplementer, "model": "stamp", "duration_ms": 300000, "input_tokens": 1, "output_tokens": 1, "cost_usd": 1,
		}),
		journalLine(3, EventTaskDone, "I", "2026-01-01T00:12:00Z", map[string]any{}),
	)
	got := oneAttempt(t, set)
	finish := mustTime(t, "2026-01-01T00:10:00Z")
	var found bool
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhaseDevelopment {
			found = true
			if !iv.Start.Equal(finish.Add(-5*time.Minute)) || !iv.End.Equal(finish) {
				t.Fatalf("development = [%s, %s), want [T1-D, T1)", iv.Start, iv.End)
			}
			if iv.Inferred {
				t.Fatal("recorded duration labeled inferred")
			}
		}
	}
	if !found {
		t.Fatal("missing development span")
	}
	sum, err := SummarizeWall(got.Wall)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Unclassified < 5*time.Minute {
		t.Fatalf("setup/queue residual missing: %+v", sum)
	}
}

func testF2AllKinds(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-kinds", "synthetic-all-kinds.jsonl"))
	requireKnown(t, got.Model, "sol", "closing implementing stamp")
	if got.Model.Value == "designer" || got.Model.Value == "verifier-model" {
		t.Fatalf("design/verifier stamped the implementing model: %q", got.Model.Value)
	}
	if !got.CostUSD.Known {
		t.Fatal("design cost must still be in the recorded-spawn total")
	}
}

func testF2CitationMembership(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-cascade", "synthetic-cascade-closing.jsonl"))
	if got.Cascades != 1 || len(got.CascadeEvents) != 1 {
		t.Fatalf("cascades=%d events=%d", got.Cascades, len(got.CascadeEvents))
	}
	if got.Evidence.Cascades.Event != got.CascadeEvents[0] {
		t.Fatal("Evidence.Cascades does not cite CascadeEvents[0]")
	}
	if got.CostUSD.Known && (len(got.CostEvents) == 0 || got.Evidence.Cost.Event != got.CostEvents[0]) {
		t.Fatal("Evidence.Cost does not cite CostEvents[0]")
	}
}

func testF2SummaryAvailability(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	complete := WallBreakdown{
		StartedAt: start, Elapsed: 10 * time.Minute, Complete: true,
		Intervals: []Interval{{Phase: PhaseDevelopment, Start: start, End: start.Add(10 * time.Minute)}},
	}
	incomplete := complete
	incomplete.Complete = false
	a, err := SummarizeWall(complete)
	if err != nil {
		t.Fatal(err)
	}
	b, err := SummarizeWall(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	if a.Development != b.Development {
		t.Fatalf("numeric parts diverged: %+v vs %+v", a, b)
	}
	if a.Complete == b.Complete {
		t.Fatal("Complete flag did not distinguish unavailable phases")
	}
}

func testF4OutcomeWire(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-done", "synthetic-utc-offset.jsonl"))
	data, err := got.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"outcome":"done"`)) {
		t.Fatalf("wire outcome missing: %s", data)
	}
	var round Attempt
	if err := round.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if round.Outcome != OutcomeDone {
		t.Fatalf("round-trip outcome = %s", round.Outcome)
	}
	if err := (&Attempt{}).UnmarshalJSON([]byte(`{"id":{"run_id":"r","key":"k","started_at":"2026-01-01T00:00:00Z"}}`)); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("missing outcome = %v, want ErrInvalidOutcome", err)
	}
	bad := Attempt{Outcome: Outcome(99), ID: NewAttemptID("r", "k", mustTime(t, "2026-01-01T00:00:00Z"))}
	if _, err := bad.MarshalJSON(); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("invalid marshal = %v", err)
	}
	if _, err := bad.Censored(); !errors.Is(err, ErrInvalidOutcome) {
		t.Fatalf("invalid Censored = %v", err)
	}
}

func testF2DesignSpawn(t *testing.T) {
	got := oneAttempt(t, reduceFixture(t, "run-kinds", "synthetic-all-kinds.jsonl"))
	if got.Model.Value == "designer" {
		t.Fatal("design spawn stamped implementing model")
	}
	for _, iv := range got.Wall.Intervals {
		if iv.Phase == PhaseDevelopment {
			for _, ref := range iv.Evidence {
				if ref.Type == EventTaskSpawnFinished && strings.Contains(strings.ToLower(ref.Journal.Path), "design") {
					t.Fatal("design time classified as development")
				}
			}
		}
	}
	if got.Wall.Complete {
		t.Fatal("design residual must leave the breakdown incomplete")
	}
}

func testF2ProducerDeclarations(t *testing.T) {
	conflict := parseLines(t, "run-pc",
		`{"seq":0,"event_type":"run_started","timestamp":"2026-01-01T00:00:00Z","payload":{"dispatcher_version":"0.1.0"}}`,
		`{"seq":1,"event_type":"run_started","timestamp":"2026-01-01T00:00:01Z","payload":{"dispatcher_version":"9.9.9"}}`,
	)
	if !conflict.Diagnostics.ProducerConflict || conflict.Journal.Producer != "" {
		t.Fatalf("conflicting producers = %+v", conflict)
	}
	compat := parseLines(t, "run-ok",
		`{"seq":0,"event_type":"run_started","timestamp":"2026-01-01T00:00:00Z","payload":{"dispatcher_version":"0.1.0"}}`,
		`{"seq":1,"event_type":"run_started","timestamp":"2026-01-01T00:00:01Z","payload":{"dispatcher_version":"0.1.0"}}`,
	)
	if compat.Diagnostics.ProducerConflict || compat.Journal.Producer != ProducerDispatcherV0_1_0 {
		t.Fatalf("compatible repeats = %+v", compat)
	}
}

func testF2LineEnvelope(t *testing.T) {
	parsed := parseLines(t, "run-seq",
		`{"event_type":"run_started","timestamp":"2026-01-01T00:00:00Z","payload":{"dispatcher_version":"0.1.0"}}`,
		`{"seq":0,"hash":"aa","prev_hash":"bb","event_type":"task_started","task_key":"L","timestamp":"2026-01-01T00:00:00Z","payload":{}}`,
		`{"seq":null,"event_type":"task_done","task_key":"L","timestamp":"2026-01-01T00:01:00Z","payload":{}}`,
	)
	var started, done Event
	for _, ev := range parsed.Events {
		switch ev.Ref.Type {
		case EventTaskStarted:
			started = ev
		case EventTaskDone:
			done = ev
		}
	}
	if !started.Ref.HasSeq || started.Ref.Seq != 0 {
		t.Fatalf("explicit zero seq: HasSeq=%t Seq=%d", started.Ref.HasSeq, started.Ref.Seq)
	}
	if started.Ref.Hash != "aa" || started.Ref.PrevHash != "bb" {
		t.Fatalf("hash envelope dropped: %+v", started.Ref)
	}
	if done.Ref.HasSeq {
		t.Fatalf("null seq treated as present: %+v", done.Ref)
	}
}

func testF2ParserTotalCap(t *testing.T) {
	var b strings.Builder
	b.WriteString(journalLine(0, EventRunStarted, "", "2026-01-01T00:00:00Z", map[string]any{"dispatcher_version": ProducerDispatcherV0_1_0}) + "\n")
	for i := 0; i < 50; i++ {
		b.WriteString(journalLine(i+1, EventTaskStarted, "C", "2026-01-01T00:00:00Z", map[string]any{"model": "planned"}) + "\n")
	}
	parsed, err := ParseEvents(context.Background(), journalID("run-cap", "cap.jsonl"), strings.NewReader(b.String()), JournalBounds{MaxTotalBytes: 200})
	if err != nil && !errors.Is(err, ErrBoundExceeded) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("ParseEvents = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if !parsed.Diagnostics.TotalBoundExceeded {
		t.Fatalf("TotalBoundExceeded not set: %+v", parsed.Diagnostics)
	}
	if parsed.Diagnostics.Events == 0 && len(parsed.Events) == 0 && parsed.Diagnostics.Bytes == 0 {
		t.Fatal("cap discarded all retained diagnostics")
	}
}

func testF2WallParentConsistency(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	a := journalAttempt("run", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	a.Wall.StartedAt = start.Add(time.Minute)
	data, err := a.MarshalJSON()
	if err == nil {
		var round Attempt
		err = round.UnmarshalJSON(data)
	}
	if err != nil && !errors.Is(err, ErrEvidenceConflict) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("mismatch path = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) || err == nil {
		// Reducer must also refuse a ParsedJournal that would produce a mismatch;
		// the JSON seam is the portable half of the same rule.
		if err == nil {
			t.Fatal("wall/parent mismatch serialized")
		}
		t.Fatal(err)
	}
}

func testF2ParseCancellationRetainsEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelAfterRead{reader: strings.NewReader(strings.Repeat(
		journalLine(1, EventTaskStarted, "C", "2026-01-01T00:00:00Z", map[string]any{"model": "planned"})+"\n", 10000)), cancel: cancel}
	parsed, err := ParseEvents(ctx, journalID("run-c", "c.jsonl"), reader, JournalBounds{})
	if !errors.Is(err, ErrSourceCancelled) && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("cancel = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if !errors.Is(err, ErrSourceCancelled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must wrap both sentinels: %v", err)
	}
	if parsed.Diagnostics.Lines == 0 && len(parsed.Events) == 0 && parsed.Diagnostics.Bytes == 0 {
		t.Fatal("cancellation dropped retained diagnostics")
	}
}

func testF2ZeroCutoffRejected(t *testing.T) {
	parsed := parseFixture(t, "run-z", "synthetic-producer-only.jsonl")
	_, err := ReduceAttempts(parsed, time.Time{})
	requireSentinel(t, err, ErrInvalidSelection)
}
