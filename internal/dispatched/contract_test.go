package dispatched

// contract_test.go: unit tests for the scaffold-implemented amended surfaces
// (Selection, merge over the amended fields, Reading envelopes, producer
// constant). Each test would fail under a defect the FC-SCAFFOLD review
// panel named. Acceptance seals over the behavior table remain FC-SEALS'.

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSelectionRejectsPaddedHoldoutAndMatchesTrimmed(t *testing.T) {
	padded := Selection{HoldoutRunIDs: []string{" R1\n"}}
	if err := padded.Validate(); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("padded holdout Validate = %v, want ErrInvalidSelection", err)
	}
	// Even without Validate the padded holdout must still exclude its run.
	if !padded.HeldOut("R1") {
		t.Fatal("padded holdout did not exclude run R1")
	}
	exact := Selection{HoldoutRunIDs: []string{"R1"}}
	if err := exact.Validate(); err != nil {
		t.Fatal(err)
	}
	if !exact.HeldOut("R1") || exact.HeldOut("R2") || exact.HeldOut("") {
		t.Fatal("HeldOut must match exactly the listed run")
	}
	dup := Selection{HoldoutRunIDs: []string{"R1", "R1"}}
	if err := dup.Validate(); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("duplicate holdout Validate = %v", err)
	}
}

func TestUnmatchedHoldoutIsInvalidSelection(t *testing.T) {
	s := Selection{HoldoutRunIDs: []string{"R1", "R-misspelt"}}
	err := s.UnmatchedHoldouts([]string{"R1", "R2"})
	if !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("unmatched holdout = %v, want ErrInvalidSelection", err)
	}
	if err := s.UnmatchedHoldouts([]string{"R1", "R-misspelt", "R2"}); err != nil {
		t.Fatalf("every holdout matched but got %v", err)
	}
	if err := (Selection{}).UnmatchedHoldouts(nil); err != nil {
		t.Fatalf("no holdouts must never be unmatched: %v", err)
	}
}

func TestMergeJoinsEvidenceOrderIndependently(t *testing.T) {
	journal := JournalIdentity{RunID: "run-1", SourceID: "runs", Path: "run-1/journal.jsonl"}
	a := obs("A", seenCell, OutcomeDone, time.Hour)
	a.TerminalEvidence = terminalEvidenceYAML
	a.Evidence.Terminal = FieldEvidence{Source: EvidenceYAML, Reading: ReadingRef{Path: "features/x.yaml", Revision: Revision{Source: SourceLive}}}
	b := a
	b.Provenance = gitReading
	b.TerminalEvidence = terminalEvidenceJournal
	b.Evidence.Terminal = FieldEvidence{Source: EvidenceJournal, Event: EventRef{Journal: journal, Seq: 9, Type: EventTaskDone}}

	ab, err := merge(a, b)
	if err != nil {
		t.Fatal(err)
	}
	ba, err := merge(b, a)
	if err != nil {
		t.Fatal(err)
	}
	if !ab.Equal(ba) {
		t.Fatalf("merge is order-dependent:\n ab %+v\n ba %+v", ab, ba)
	}
	if ab.Evidence.Terminal.Source != EvidenceJournal {
		t.Fatalf("journal evidence must outrank yaml, got %v", ab.Evidence.Terminal.Source)
	}
	if ab.TerminalEvidence != terminalEvidenceJournal {
		t.Fatalf("TerminalEvidence %q disagrees with Evidence.Terminal", ab.TerminalEvidence)
	}
}

func TestMergeWallIsContentJoinWithNilIdentity(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	wall := func(complete bool) *WallBreakdown {
		return &WallBreakdown{StartedAt: start, Elapsed: time.Hour, Complete: complete, Intervals: []Interval{
			{Phase: PhaseDevelopment, Start: start, End: start.Add(30 * time.Minute)},
		}}
	}
	base := obs("A", seenCell, OutcomeDone, time.Hour)
	withWall := base
	withWall.Wall = wall(true)
	same := base
	same.Wall = wall(true) // distinct pointer, equal content

	for _, pair := range [][2]Observation{{base, withWall}, {withWall, base}, {withWall, same}} {
		got, err := merge(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		if got.Wall == nil || !got.Wall.Equal(*wall(true)) {
			t.Fatalf("merge(%v, %v) lost the breakdown: %+v", pair[0].Wall != nil, pair[1].Wall != nil, got.Wall)
		}
		if !got.Equal(withWall) {
			t.Fatal("Equal must compare Wall by content, not pointer")
		}
	}
	if withWall == same {
		t.Fatal("== on rows with distinct Wall pointers must not be the equality the seals rely on")
	}

	other := base
	other.Wall = wall(false)
	for _, pair := range [][2]Observation{{withWall, other}, {other, withWall}} {
		_, err := merge(pair[0], pair[1])
		if !errors.Is(err, ErrStampConflict) || !errors.Is(err, ErrEvidenceConflict) {
			t.Fatalf("two different breakdowns merged without refusal: %v", err)
		}
	}
}

func TestProducerConstantIsTheWireValue(t *testing.T) {
	if ProducerDispatcherV0_1_0 != "0.1.0" {
		t.Fatalf("ProducerDispatcherV0_1_0 = %q, want the raw dispatcher_version", ProducerDispatcherV0_1_0)
	}
}

func TestParseReadingsKeepsMalformedAndMissingRowsApart(t *testing.T) {
	ref := ReadingRef{SourceID: "live", Repository: "/repo", Path: "features/t.yaml", Revision: Revision{Source: SourceLive}}
	doc := []byte(`tasks:
  - key: A
    role: scaffold
    dispatcher_run_id: run-1
    started_at: 2026-09-01T00:00:00Z
  - key: B
    role: scaffold
    dispatcher_run_id: run-1
  - key: C
    role: scaffold
    dispatcher_run_id: run-1
    started_at: not-a-time
`)
	readings := parseReadings(doc, ref)
	if len(readings) != 3 {
		t.Fatalf("got %d readings, want one per row", len(readings))
	}
	ok, missing, malformed := readings[0], readings[1], readings[2]
	if ok.Err != nil || !ok.Present.Complete() || ok.Row != 1 || ok.Snapshot.StartedAt.IsZero() || ok.Ref != ref {
		t.Fatalf("valid row = %+v", ok)
	}
	if missing.Err != nil || missing.Present.Complete() || missing.Present.StartedAt {
		t.Fatalf("missing started_at must be present=false without an error: %+v", missing)
	}
	if malformed.Err == nil || !malformed.Present.StartedAt || malformed.Row != 3 {
		t.Fatalf("malformed started_at must be present=true with an error: %+v", malformed)
	}
	if bad := parseReadings([]byte("- not: a mapping"), ref); len(bad) != 1 || bad[0].Err == nil || bad[0].Row != 0 {
		t.Fatalf("undecodable document = %+v", bad)
	}
}

func TestHolesReturnNotImplementedWithNamedInputs(t *testing.T) {
	parsed, err := ParseEvents(context.Background(), JournalIdentity{Path: "j"}, nil, ReadBounds{})
	if !errors.Is(err, ErrNotImplemented) || parsed.Journal.Path != "j" {
		t.Fatalf("ParseEvents = %+v, %v", parsed, err)
	}
	set, err := ReduceAttempts(parsed, time.Time{})
	if !errors.Is(err, ErrNotImplemented) || set.Journal.Path != "j" {
		t.Fatalf("ReduceAttempts = %+v, %v", set, err)
	}
	if _, err := JoinEvidence(nil, nil, Selection{}, time.Time{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("JoinEvidence = %v", err)
	}
}
