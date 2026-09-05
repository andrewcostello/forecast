package dispatched

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestFCEvidenceContract is the reserved FC-1 group.
func TestFCEvidenceContract(t *testing.T) {
	t.Run("F1-ID-DISTINCT-RUNS", testF1IDDistinctRuns)
	t.Run("F1-ID-SAME-RUN-REVISIONS", testF1IDSameRunRevisions)
	t.Run("F1-ID-NEAREST-NOT-MATCHED", testF1IDNearestNotMatched)
	t.Run("F1-ID-UTC-OFFSET-YAML", testF1IDUTCOffsetYAML)
	t.Run("F1-EV-MERGE-PERMUTATION", testF1EVMergePermutation)
	t.Run("F1-EV-NO-MANUFACTURED-ROW", testF1EVNoManufacturedRow)
	t.Run("F1-EV-JOURNAL-OVER-YAML", testF1EVJournalOverYAML)
	t.Run("F1-EV-YAML-ONLY-TERMINAL", testF1EVYAMLOnlyTerminal)
	t.Run("F1-EV-PERMUTATION", testF1EVPermutation)
	t.Run("F1-MODEL-NO-ALIAS-POOL", testF1ModelNoAliasPool)
	t.Run("F1-ROLE-CITATION", testF1RoleCitation)
	t.Run("F1-READING-TOTAL-ORDER", testF1ReadingTotalOrder)
	t.Run("F1-ATTEMPT-RUN-CONSISTENCY", testF1AttemptRunConsistency)
	t.Run("F3-SRC-READ-OK-ZERO-MATCH", testF3SrcReadOKZeroMatch)
	t.Run("F3-DISPOSITION-EVERY-SNAPSHOT", testF3DispositionEverySnapshot)
	t.Run("F3-DISPOSITION-NO-RUN", testF3DispositionNoRun)
	t.Run("F3-DISPOSITION-MISSING-JOIN-KEYS", testF3DispositionMissingJoinKeys)
	t.Run("F3-ROWS-VS-ATTEMPTS", testF3RowsVsAttempts)
	t.Run("F3-LOST-NOT-HIDDEN", testF3LostNotHidden)
	t.Run("F3-MANIFEST-CUTOFF-STORED", testF3ManifestCutoffStored)
	t.Run("F3-DIRECT-HOLDOUT", testF3DirectHoldout)
	t.Run("F3-DIRECT-HOLDOUT-ATTEMPTS", testF3DirectHoldoutAttempts)
	t.Run("F3-EXCLUDED-JOURNAL-AUDIT", testF3ExcludedJournalAudit)
	t.Run("F3-HOLDOUT-EXCLUDED-JOIN", testF3HoldoutExcludedJoin)
	t.Run("F2-YAML-TERMINAL-WALL", testF2YAMLTerminalWall)
	t.Run("F4-TARGET-MALFORMED", testF4TargetMalformed)
	t.Run("F4-TARGET-ZERO-ROWS", testF4TargetZeroRows)
	t.Run("F4-ELIGIBLE-THRESHOLD", testF4EligibleThreshold)
	t.Run("F4-NOT-ELIGIBLE-THIN", testF4NotEligibleThin)
	t.Run("F4-NOT-ELIGIBLE-PARTIAL", testF4NotEligiblePartial)
	t.Run("F4-HAND-FINISHED-LIMIT", testF4HandFinishedLimit)
	t.Run("F4-BUILD-AMENDED-OPTIONS", testF4BuildAmendedOptions)
	t.Run("F4-THRESHOLD-NONPOSITIVE", testF4ThresholdNonpositive)
	t.Run("F4-SCHEMA-ROUNDTRIP", testF4SchemaRoundtrip)
	t.Run("F4-TARGET-INPUT", testF4TargetInput)
	t.Run("F4-VERSION-EXACT", testF4VersionExact)
	t.Run("F4-MIXED-OPTIONS", testF4MixedOptions)
	t.Run("F4-CANONICAL-LISTS", testF4CanonicalLists)
	t.Run("F4-ONE-ARTIFACT-INSTANT", testF4OneArtifactInstant)
	t.Run("F4-AGGREGATE-REASON", testF4AggregateReason)
	t.Run("F4-PROJECTION-MAPPING", testF4ProjectionMapping)
	t.Run("F4-ARTIFACT-HOLDOUT", testF4ArtifactHoldout)
	t.Run("F4-STRUCTURED-THIN-CELL", testF4StructuredThinCell)
	t.Run("F4-CELL-EMPTY-N0", testF4CellEmptyN0)
}

func twoRunUniverse(t *testing.T) (sets []AttemptSet, readings []Reading, journals []JournalIdentity) {
	t.Helper()
	start := mustTime(t, "2026-01-01T00:00:00Z")
	a := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	b := journalAttempt("run-b", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	sa := attemptSetOf("run-a", a)
	sb := attemptSetOf("run-b", b)
	ra := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	rb := syntheticReading("run-b", "K", start, 1, "features/study/tasks.yaml")
	rb.Ref.Path = "features/other/tasks.yaml"
	return []AttemptSet{sa, sb}, []Reading{ra, rb}, []JournalIdentity{sa.Journal, sb.Journal}
}

func testF1IDDistinctRuns(t *testing.T) {
	sets, readings, journals := twoRunUniverse(t)
	got := mustJoin(t, sets, readings, defaultSelection(), journals)
	if got.Attempts != 2 || got.UniqueRows != 2 || got.Recovered != 2 {
		t.Fatalf("distinct runs merged: attempts=%d rows=%d recovered=%d", got.Attempts, got.UniqueRows, got.Recovered)
	}
	if len(got.Observations) != 2 {
		t.Fatalf("observations = %d, want 2", len(got.Observations))
	}
}

func testF1IDSameRunRevisions(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	r1 := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	r1.Ref.Revision = "git:" + strings.Repeat("aa", 20)
	r1.Ref.RecordedAt = start
	r2 := r1
	r2.Ref.Revision = "git:" + strings.Repeat("bb", 20)
	r2.Ref.RecordedAt = start.Add(time.Minute)
	r3 := r1
	r3.Ref.Revision = "live"
	r3.Ref.RecordedAt = start.Add(2 * time.Minute)
	got := mustJoin(t, []AttemptSet{set}, []Reading{r3, r2, r1}, defaultSelection(), identityUniverse(set))
	if got.Recovered != 1 || len(got.Observations) != 1 {
		t.Fatalf("revisions created extra samples: %+v", got)
	}
	requireDisposition(t, got, DispositionRecovered, 1)
	requireDisposition(t, got, DispositionDuplicateReading, 2)
}

func testF1IDNearestNotMatched(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	stale := syntheticReading("run-a", "K", start.Add(time.Second), 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{stale}, defaultSelection(), identityUniverse(set))
	requireDisposition(t, got, DispositionNoMatchingStart, 1)
	if len(got.LostAttempts) != 1 {
		t.Fatalf("lost attempts = %+v", got.LostAttempts)
	}
	if got.Recovered != 0 {
		t.Fatal("nearest-start match recovered a row")
	}
}

func testF1IDUTCOffsetYAML(t *testing.T) {
	startUTC := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-offset", "F1-OFFSET", startUTC, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-offset", att)
	yamlStart := mustTime(t, "2025-12-31T16:00:00-08:00")
	reading := syntheticReading("run-offset", "F1-OFFSET", yamlStart, 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{reading}, defaultSelection(), identityUniverse(set))
	if got.Recovered != 1 {
		t.Fatalf("offset-equivalent start not joined: recovered=%d dispositions=%+v", got.Recovered, got.Dispositions)
	}
}

func testF1EVMergePermutation(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	att.TerminalAt = start.Add(10 * time.Minute)
	set := attemptSetOf("run-a", att)
	yaml := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	yaml.CompletedAt = Known(start.Add(12 * time.Minute))
	yaml.Snapshot.Status = "Done"
	var encoded [][]byte
	for _, order := range [][]Reading{{yaml}, {yaml}} {
		got := mustJoin(t, []AttemptSet{set}, order, defaultSelection(), identityUniverse(set))
		if got.Recovered != 1 {
			t.Fatal("merge failed")
		}
		obs := got.Observations[0]
		if obs.Attempt.Elapsed != 10*time.Minute {
			t.Fatalf("elapsed = %s, want journal 10m not YAML 12m", obs.Attempt.Elapsed)
		}
		if obs.Attempt.Evidence.Terminal.Source != EvidenceJournal {
			t.Fatalf("terminal source = %q, want journal (atomic with elapsed)", obs.Attempt.Evidence.Terminal.Source)
		}
		encoded = append(encoded, encodeJSON(t, obs.Attempt.Evidence.Terminal))
	}
	if !bytes.Equal(encoded[0], encoded[1]) {
		t.Fatal("terminal selection was permutation-dependent")
	}
}

func testF1EVNoManufacturedRow(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	att.CostUSD = Known(1.0)
	att.CostEvents = []EventRef{att.Start}
	att.Evidence.Cost = FieldEvidence{Source: EvidenceJournal, Event: att.Start}
	set := attemptSetOf("run-a", att)
	x := syntheticReading("run-a", "K", start, 1, "features/a.yaml")
	y := syntheticReading("run-a", "K", start, 1, "features/b.yaml")
	y.Ref.Revision = "git:" + strings.Repeat("cc", 20)
	y.CompletedAt = Known(start.Add(12 * time.Minute))
	got := mustJoin(t, []AttemptSet{set}, []Reading{x, y}, defaultSelection(), identityUniverse(set))
	obs := got.Observations[0]
	if obs.Attempt.Elapsed != 10*time.Minute {
		t.Fatalf("elapsed took an independent max: %s", obs.Attempt.Elapsed)
	}
	requireKnown(t, obs.Attempt.CostUSD, 1.0, "cost")
	if obs.Attempt.Evidence.Elapsed.Reading.Path != "" && obs.Attempt.Evidence.Cost.Reading.Path != "" &&
		obs.Attempt.Evidence.Elapsed.Reading.Path != obs.Attempt.Evidence.Cost.Reading.Path &&
		obs.Attempt.Evidence.Elapsed.Source == EvidenceYAML {
		t.Fatal("elapsed and cost attributed to independent YAML maxima")
	}
}

func testF1EVJournalOverYAML(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	yaml := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	yaml.Snapshot.Status = "Blocked"
	yaml.CompletedAt = Known(start.Add(12 * time.Minute))
	got := mustJoin(t, []AttemptSet{set}, []Reading{yaml}, defaultSelection(), identityUniverse(set))
	obs := got.Observations[0]
	if obs.Attempt.Outcome != OutcomeDone {
		t.Fatalf("YAML outcome overrode journal: %s", obs.Attempt.Outcome)
	}
	if obs.Attempt.Evidence.Terminal.Source != EvidenceJournal {
		t.Fatalf("source = %q", obs.Attempt.Evidence.Terminal.Source)
	}
}

func testF1EVYAMLOnlyTerminal(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, contractCutoff().Sub(start), "stamp", OutcomeUnfinished)
	att.Evidence.Terminal = FieldEvidence{}
	att.TerminalAt = time.Time{}
	set := attemptSetOf("run-a", att)
	yaml := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	yaml.Snapshot.Status = "Done"
	yaml.CompletedAt = Known(start.Add(10 * time.Minute))
	got := mustJoin(t, []AttemptSet{set}, []Reading{yaml}, defaultSelection(), identityUniverse(set))
	if got.RowsWithYAMLOnlyTerminal != 1 {
		t.Fatalf("RowsWithYAMLOnlyTerminal = %d", got.RowsWithYAMLOnlyTerminal)
	}
	if got.Observations[0].Attempt.Evidence.Terminal.Source != EvidenceYAML {
		t.Fatalf("source = %q", got.Observations[0].Attempt.Evidence.Terminal.Source)
	}
}

func testF1EVPermutation(t *testing.T) {
	sets, readings, journals := twoRunUniverse(t)
	var first []byte
	for i, order := range [][]AttemptSet{{sets[0], sets[1]}, {sets[1], sets[0]}} {
		got := mustJoin(t, order, readings, defaultSelection(), journals)
		enc := encodeJSON(t, got.Observations)
		if i == 0 {
			first = enc
		} else if !bytes.Equal(first, enc) {
			t.Fatal("JoinEvidence output depended on attempt-set order")
		}
	}
}

func testF1ModelNoAliasPool(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	a := journalAttempt("run-a", "A", start, 10*time.Minute, "claude-opus-5", OutcomeDone)
	b := journalAttempt("run-a", "B", start, 10*time.Minute, "opus-5", OutcomeDone)
	set := attemptSetOf("run-a", a, b)
	ra := syntheticReading("run-a", "A", start, 1, "features/study/tasks.yaml")
	rb := syntheticReading("run-a", "B", start, 2, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{ra, rb}, defaultSelection(), identityUniverse(set))
	cells := map[string]bool{}
	for _, obs := range got.Observations {
		cells[obs.Cell.Model] = true
	}
	if !cells["claude-opus-5"] || !cells["opus-5"] {
		t.Fatalf("aliases pooled: %+v", cells)
	}
}

func testF1RoleCitation(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-role", "F1-ROLE", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-role", att)
	a := syntheticReading("run-role", "F1-ROLE", start, 1, "features/study/tasks.yaml")
	a.Snapshot.Role = RoleBodies
	b := a
	b.Ref.Row = 2
	b.Snapshot.Role = RoleSeals
	got, err := joinContract(t, []AttemptSet{set}, []Reading{a, b}, defaultSelection(), identityUniverse(set))
	if err != nil && !errors.Is(err, ErrEvidenceConflict) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("role conflict = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err == nil && dispositionCount(got, DispositionConflictingEvidence) != 2 && got.Recovered != 0 {
		t.Fatalf("conflicting roles recovered: %+v", got)
	}
}

func testF1ReadingTotalOrder(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	r1 := syntheticReading("run-a", "K", start, 1, "a.yaml")
	r1.Ref.SourceID = "s1"
	r2 := syntheticReading("run-a", "K", start, 2, "b.yaml")
	r2.Ref.SourceID = "s2"
	got1 := mustJoin(t, []AttemptSet{set}, []Reading{r2, r1}, defaultSelection(), identityUniverse(set))
	got2 := mustJoin(t, []AttemptSet{set}, []Reading{r1, r2}, defaultSelection(), identityUniverse(set))
	if !bytes.Equal(encodeJSON(t, got1.Examined), encodeJSON(t, got2.Examined)) {
		t.Fatal("Examined order depended on input order")
	}
}

func testF1AttemptRunConsistency(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-b", att)
	_, err := joinContract(t, []AttemptSet{set}, nil, defaultSelection(), []JournalIdentity{set.Journal})
	requireSentinel(t, err, ErrInvalidSelection)
}

func testF3SrcReadOKZeroMatch(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "OTHER", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	reading := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{reading}, defaultSelection(), identityUniverse(set))
	if got.Recovered != 0 {
		t.Fatal("zero-match produced a sample")
	}
	requireDisposition(t, got, DispositionNoMatchingRun, 1)
}

func testF3DispositionEverySnapshot(t *testing.T) {
	sets, readings, journals := twoRunUniverse(t)
	got := mustJoin(t, sets, readings, defaultSelection(), journals)
	if len(got.Examined) != len(readings) {
		t.Fatalf("examined = %d, readings = %d", len(got.Examined), len(readings))
	}
	sum := 0
	seen := map[Disposition]bool{}
	for _, d := range got.Dispositions {
		sum += d.Count
		seen[d.Disposition] = true
	}
	if sum != len(got.Examined) {
		t.Fatalf("disposition counts %d != examined %d", sum, len(got.Examined))
	}
	for _, want := range Dispositions() {
		if !seen[want] && dispositionCount(got, want) != 0 {
			t.Fatalf("missing declared disposition %s", want)
		}
	}
	if len(got.Dispositions) != len(Dispositions()) {
		t.Fatalf("Dispositions length = %d, want every declared value including zeros (%d)", len(got.Dispositions), len(Dispositions()))
	}
}

func testF3DispositionNoRun(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	reading := syntheticReading("missing", "K", start, 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{reading}, defaultSelection(), identityUniverse(set))
	requireDisposition(t, got, DispositionNoMatchingRun, 1)
}

func testF3DispositionMissingJoinKeys(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	reading := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	reading.Present.StartedAt = false
	reading.Identity.StartedAt = Unknown[time.Time]()
	got := mustJoin(t, []AttemptSet{set}, []Reading{reading}, defaultSelection(), identityUniverse(set))
	requireDisposition(t, got, DispositionMissingJoinKeys, 1)
}

func testF3RowsVsAttempts(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	a := journalAttempt("run-a", "K", start, 10*time.Minute, "first", OutcomeUnfinished)
	b := journalAttempt("run-a", "K", start.Add(time.Hour), 10*time.Minute, "second", OutcomeDone)
	set := attemptSetOf("run-a", a, b)
	r1 := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	r2 := syntheticReading("run-a", "K", start.Add(time.Hour), 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{r1, r2}, defaultSelection(), identityUniverse(set))
	if got.UniqueRows != 1 || got.Attempts != 2 {
		t.Fatalf("rows=%d attempts=%d", got.UniqueRows, got.Attempts)
	}
}

func testF3LostNotHidden(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	a := journalAttempt("run-a", "K", start, 10*time.Minute, "first", OutcomeUnfinished)
	b := journalAttempt("run-a", "K", start.Add(time.Hour), 10*time.Minute, "second", OutcomeDone)
	set := attemptSetOf("run-a", a, b)
	r2 := syntheticReading("run-a", "K", start.Add(time.Hour), 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{r2}, defaultSelection(), identityUniverse(set))
	if got.Recovered != 1 || len(got.LostAttempts) != 1 {
		t.Fatalf("recovered=%d lost=%v", got.Recovered, got.LostAttempts)
	}
	if got.LostAttempts[0] != a.ID {
		t.Fatalf("lost = %+v, want the unmatched sibling", got.LostAttempts)
	}
}

func testF3ManifestCutoffStored(t *testing.T) {
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())}, defaultSelection(), ReadBounds{}))
	if err != nil && !errors.Is(err, ErrNotImplemented) && !errors.Is(err, ErrSourceEmpty) && !errors.Is(err, ErrSourceMissing) {
		t.Fatalf("Build = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result == nil || result.Artifact.SourceManifest == nil || result.Artifact.SourceManifest.Cutoff.IsZero() {
		t.Fatal("amended Build omitted SourceManifest.Cutoff")
	}
}

func testF3DirectHoldout(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("run-a", att)
	_, err := joinContract(t, []AttemptSet{set}, nil, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{"nope"},
	}, identityUniverse(set))
	requireSentinel(t, err, ErrInvalidSelection)
}

func testF3DirectHoldoutAttempts(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("held", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("held", att)
	keep := journalAttempt("keep", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	keepSet := attemptSetOf("keep", keep)
	got, err := joinContract(t, []AttemptSet{set, keepSet}, nil, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"},
	}, []JournalIdentity{set.Journal, keepSet.Journal})
	requireSentinel(t, err, ErrInvalidSelection)
	if err == nil && (len(got.Observations) != 0 || len(got.LostAttempts) != 0) {
		t.Fatalf("held-out attempt set contributed: %+v", got)
	}
}

func testF3ExcludedJournalAudit(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	keep := journalAttempt("keep", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	keepSet := attemptSetOf("keep", keep)
	heldID := JournalIdentity{RunID: "held", SourceID: "journals", Path: "held/journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	reading := syntheticReading("keep", "K", start, 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{keepSet}, []Reading{reading}, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"},
	}, []JournalIdentity{keepSet.Journal, heldID})
	found := false
	for _, id := range got.ExcludedJournals {
		if id.RunID == "held" && id.Path == heldID.Path && id.Producer == ProducerDispatcherV0_1_0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("excluded journal identity not audited: %+v", got.ExcludedJournals)
	}
}

func testF3HoldoutExcludedJoin(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("keep", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	set := attemptSetOf("keep", att)
	held := syntheticReading("held", "K", start, 1, "features/study/tasks.yaml")
	held.Excluded = DispositionHeldOut
	held.Snapshot = ReadingSnapshot{}
	keep := syntheticReading("keep", "K", start, 1, "features/study/tasks.yaml")
	heldID := JournalIdentity{RunID: "held", SourceID: "journals", Path: "held/journal.jsonl", Producer: ProducerDispatcherV0_1_0}
	got := mustJoin(t, []AttemptSet{set}, []Reading{held, keep}, Selection{
		Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"},
	}, []JournalIdentity{set.Journal, heldID})
	requireDisposition(t, got, DispositionHeldOut, 1)
	if got.Recovered != 1 {
		t.Fatalf("held-out reading contributed or keep lost: recovered=%d", got.Recovered)
	}
}

func testF2YAMLTerminalWall(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, contractCutoff().Sub(start), "stamp", OutcomeUnfinished)
	att.Wall.Intervals = []Interval{{
		Phase: PhaseDevelopment,
		Start: start,
		End:   start.Add(time.Hour),
	}}
	att.Evidence.Terminal = FieldEvidence{}
	set := attemptSetOf("run-a", att)
	yaml := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	yaml.Snapshot.Status = "Done"
	yaml.CompletedAt = Known(start.Add(10 * time.Minute))
	got := mustJoin(t, []AttemptSet{set}, []Reading{yaml}, defaultSelection(), identityUniverse(set))
	obs := got.Observations[0]
	if obs.Attempt.Elapsed != 10*time.Minute {
		t.Fatalf("elapsed = %s after YAML terminal", obs.Attempt.Elapsed)
	}
	if obs.Attempt.Wall.Elapsed != obs.Attempt.Elapsed {
		t.Fatal("Wall.Elapsed not rebased with terminal")
	}
	for _, iv := range obs.Attempt.Wall.Intervals {
		if iv.End.After(start.Add(10 * time.Minute)) {
			t.Fatalf("interval not withheld after YAML terminal: %+v", iv)
		}
	}
	if len(att.Wall.Intervals) > 0 && obs.Attempt.Wall.Complete {
		t.Fatal("withheld span left Complete=true")
	}
}

func eligibleTargets() []TargetRow {
	return []TargetRow{{Key: "T1", Role: RoleBodies, Model: "stamp"}}
}

func recoveredArtifact(n int) Artifact {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	obs := make([]RecoveredAttempt, 0, n)
	for i := 0; i < n; i++ {
		att := journalAttempt("run-a", "K"+string(rune('A'+i)), start, 10*time.Minute, "stamp", OutcomeDone)
		obs = append(obs, RecoveredAttempt{Attempt: att, Cell: Cell{Role: RoleBodies, Model: "stamp"}, Readings: []ReadingRef{}})
	}
	ev := &ArtifactEvidence{
		Observations:     obs,
		Examined:         []Examined{},
		Dispositions:     []DispositionCount{},
		Conflicts:        []AttemptConflict{},
		Ambiguous:        []AmbiguousAttempt{},
		LostAttempts:     []AttemptID{},
		ExcludedJournals: []JournalIdentity{},
		Recovered:        n,
		Attempts:         n,
		UniqueRows:       n,
	}
	return schema4Artifact(completeManifest(SourceComplete), ev)
}

func testF4TargetMalformed(t *testing.T) {
	art := recoveredArtifact(2)
	_, err := PredictionEligibility(art, []TargetRow{{Key: "T1", Role: Role("nope"), Model: "stamp"}}, 2, true)
	requireSentinel(t, err, ErrInvalidTarget)
}

func testF4TargetZeroRows(t *testing.T) {
	_, err := PredictionEligibility(recoveredArtifact(2), nil, 2, true)
	requireSentinel(t, err, ErrEmptyTarget)
	_, err = PredictionEligibility(recoveredArtifact(2), []TargetRow{}, 2, false)
	requireSentinel(t, err, ErrEmptyTarget)
}

func testF4EligibleThreshold(t *testing.T) {
	got, err := PredictionEligibility(recoveredArtifact(2), eligibleTargets(), 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.MinCompleted != 2 {
		t.Fatalf("eligibility = %+v", got)
	}
}

func testF4NotEligibleThin(t *testing.T) {
	got, err := PredictionEligibility(recoveredArtifact(1), eligibleTargets(), 2, true)
	requireSentinel(t, err, ErrNotEligible)
	requireNoSentinel(t, err, ErrSourceIncomplete)
	if got.Eligible {
		t.Fatal("thin cell marked eligible")
	}
}

func testF4NotEligiblePartial(t *testing.T) {
	art := recoveredArtifact(4)
	art.SourceManifest = completeManifest(SourcePartial)
	got, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("partial refuse = %v, want both sentinels", err)
	}
	if got.Eligible {
		t.Fatal("partial sources eligible")
	}
	_, err = PredictionEligibility(art, eligibleTargets(), 2, false)
	if err != nil && errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	art.SourceManifest = nil
	_, err = PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("nil manifest refuse = %v", err)
	}
}

func testF4HandFinishedLimit(t *testing.T) {
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())}, defaultSelection(), ReadBounds{}))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("no artifact")
	}
	found := false
	for _, limit := range result.Artifact.Limits {
		if limit == HandFinishedLimit {
			found = true
		}
	}
	if !found {
		t.Fatalf("amended Build omitted HandFinishedLimit: %v", result.Artifact.Limits)
	}
	var buf bytes.Buffer
	WriteCoverage(&buf, result.Artifact)
	if !strings.Contains(buf.String(), "Hand-finished rows have no identifying field") {
		t.Fatalf("report omitted limit:\n%s", buf.String())
	}
}

func testF4BuildAmendedOptions(t *testing.T) {
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())},
		Selection{Cutoff: contractCutoff(), HoldoutRunIDs: []string{"held"}},
		ReadBounds{},
	))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err == nil && result != nil && result.Artifact.SchemaVersion == BaselineSchemaVersion {
		t.Fatal("amended Sources/holdout/cutoff silently ran the legacy builder")
	}
	if result != nil && result.Artifact.SchemaVersion == AmendedEvidenceSchemaVersion && result.Artifact.SourceManifest == nil {
		t.Fatal("schema 4 artifact missing SourceManifest")
	}
}

func testF4ThresholdNonpositive(t *testing.T) {
	got, err := PredictionEligibility(recoveredArtifact(2), eligibleTargets(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.MinCompleted != DefaultMinObservations {
		t.Fatalf("MinCompleted = %d, want default %d", got.MinCompleted, DefaultMinObservations)
	}
}

func testF4SchemaRoundtrip(t *testing.T) {
	att := oneAttempt(t, reduceFixture(t, "run-disj", "synthetic-disjoint-phases.jsonl"))
	data, err := att.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var round Attempt
	if err := round.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if round.Reviews != att.Reviews || round.Corrections != att.Corrections {
		t.Fatalf("counts dropped: %+v vs %+v", round, att)
	}
	art := schema4Artifact(completeManifest(SourceComplete), &ArtifactEvidence{
		Observations:     []RecoveredAttempt{{Attempt: att, Cell: Cell{Role: RoleBodies, Model: att.Model.Value}, Readings: []ReadingRef{}}},
		Examined:         []Examined{},
		Dispositions:     []DispositionCount{},
		Conflicts:        []AttemptConflict{},
		Ambiguous:        []AmbiguousAttempt{},
		LostAttempts:     []AttemptID{},
		ExcludedJournals: []JournalIdentity{},
	})
	raw, err := json.Marshal(art)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"observations":null`)) {
		t.Fatalf("null observations: %s", raw)
	}
}

func testF4TargetInput(t *testing.T) {
	art := recoveredArtifact(2)
	// Aggregate looks fine; original rows duplicate a key.
	_, err := PredictionEligibility(art, []TargetRow{
		{Key: "T1", Role: RoleBodies, Model: "stamp"},
		{Key: "T1", Role: RoleBodies, Model: "stamp"},
	}, 2, true)
	requireSentinel(t, err, ErrInvalidTarget)
}

func testF4VersionExact(t *testing.T) {
	art := recoveredArtifact(2)
	art.SchemaVersion = BaselineSchemaVersion
	_, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("schema 3 = %v", err)
	}
	art = recoveredArtifact(2)
	art.SchemaVersion = 5
	_, err = PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) {
		t.Fatalf("schema 5 = %v", err)
	}
	art = recoveredArtifact(2)
	art.Evidence = nil
	_, err = PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("missing evidence = %v", err)
	}
}

func testF4MixedOptions(t *testing.T) {
	_, err := Build(context.Background(), BuildOptions{
		RunsDir: t.TempDir(), FeaturesRepo: t.TempDir(),
		Sources:   []SourceSpec{journalSpec("j", t.TempDir())},
		Selection: defaultSelection(),
	})
	if !errors.Is(err, ErrInvalidSourceSpec) && !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("mixed options = %v", err)
	}
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
}

func testF4CanonicalLists(t *testing.T) {
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())},
		Selection{Cutoff: contractCutoff(), AllowEmpty: true},
		ReadBounds{},
	))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result == nil || result.Artifact.Evidence == nil {
		t.Fatal("version 4 evidence payload missing")
	}
	raw, mErr := json.Marshal(result.Artifact.Evidence)
	if mErr != nil {
		t.Fatal(mErr)
	}
	for _, key := range []string{"observations", "examined", "lost_attempts", "excluded_journals", "conflicts", "ambiguous"} {
		if strings.Contains(string(raw), `"`+key+`":null`) {
			t.Fatalf("%s serialized as null: %s", key, raw)
		}
	}
}

func testF4OneArtifactInstant(t *testing.T) {
	cutoff := mustTime(t, "2026-02-01T00:00:00Z")
	now := mustTime(t, "2026-03-01T00:00:00Z")
	result, err := Build(context.Background(), BuildOptions{
		Sources:   []SourceSpec{journalSpec("j", t.TempDir())},
		Selection: Selection{Cutoff: cutoff},
		Now:       now,
	})
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.Artifact.GeneratedAt.Equal(cutoff) || result.Artifact.SourceManifest == nil || !result.Artifact.SourceManifest.Cutoff.Equal(cutoff) {
		t.Fatalf("GeneratedAt=%s cutoff=%v, want explicit cutoff %s", result.Artifact.GeneratedAt, result.Artifact.SourceManifest, cutoff)
	}
}

func testF4AggregateReason(t *testing.T) {
	// Two journal sources with the same run ID: join/reduce error with retained PARTIAL reasons.
	a := filepath.Join(t.TempDir(), "a")
	b := filepath.Join(t.TempDir(), "b")
	// directories only; ReadSources may fail; we assert Build reasons if it returns an artifact
	result, err := Build(context.Background(), amendedBuildOpts([]SourceSpec{
		journalSpec("j1", a), journalSpec("j2", b),
	}, defaultSelection(), ReadBounds{}))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if result != nil && result.Artifact.SourceManifest != nil {
		raw, _ := json.Marshal(result.Artifact.SourceManifest)
		if result.Artifact.SourceManifest.State == SourcePartial && strings.Contains(string(raw), `"reasons":null`) {
			t.Fatalf("null reasons: %s", raw)
		}
	}
}

func testF4ProjectionMapping(t *testing.T) {
	start := mustTime(t, "2026-01-01T00:00:00Z")
	att := journalAttempt("run-a", "K", start, 10*time.Minute, "stamp", OutcomeDone)
	att.Corrections = 2
	att.Reviews = 3
	set := attemptSetOf("run-a", att)
	reading := syntheticReading("run-a", "K", start, 1, "features/study/tasks.yaml")
	got := mustJoin(t, []AttemptSet{set}, []Reading{reading}, defaultSelection(), identityUniverse(set))
	obs := got.Observations[0]
	if obs.Attempt.Corrections != 2 {
		t.Fatalf("corrections = %d", obs.Attempt.Corrections)
	}
	result, err := Build(context.Background(), amendedBuildOpts(
		[]SourceSpec{journalSpec("j", t.TempDir())}, defaultSelection(), ReadBounds{}))
	if errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	_ = obs
	if result != nil {
		for _, row := range result.Artifact.Observations {
			if row.TerminalEvidence == "none" && row.Outcome == "done" {
				t.Fatal("legacy none projected from a done row without mapping EvidenceNone")
			}
		}
	}
}

func testF4ArtifactHoldout(t *testing.T) {
	art := recoveredArtifact(2)
	art.SourceManifest.HoldoutRunIDs = []string{"run-a"}
	_, err := PredictionEligibility(art, eligibleTargets(), 2, true)
	if !errors.Is(err, ErrNotEligible) || !errors.Is(err, ErrSourceIncomplete) {
		t.Fatalf("held-out observations in artifact = %v", err)
	}
}

func testF4StructuredThinCell(t *testing.T) {
	got, err := PredictionEligibility(recoveredArtifact(1), eligibleTargets(), 2, false)
	if err != nil && errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cells) != 1 || got.Cells[0].Completed != 1 || got.Cells[0].MinCompleted != 2 || got.Cells[0].Eligible {
		t.Fatalf("cells = %+v", got.Cells)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "bodies") {
		t.Fatalf("reasons = %v", got.Reasons)
	}
}

func testF4CellEmptyN0(t *testing.T) {
	got, err := PredictionEligibility(recoveredArtifact(2), []TargetRow{
		{Key: "T1", Role: RoleBodies, Model: "stamp"},
		{Key: "T2", Role: RoleSeals, Model: "never-seen"},
	}, 2, false)
	if err != nil && errors.Is(err, ErrNotImplemented) {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range got.Cells {
		if c.Role == RoleSeals && c.Model == "never-seen" {
			found = true
			if c.Completed != 0 || c.Eligible {
				t.Fatalf("empty cell = %+v", c)
			}
		}
	}
	if !found {
		t.Fatalf("empty required cell omitted: %+v", got.Cells)
	}
}
