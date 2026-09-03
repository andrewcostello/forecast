package dispatched

import (
	"errors"
	"testing"
	"time"
)

var (
	seenCell   = Cell{Role: RoleScaffold, Model: "opus"}
	emptyCell  = Cell{Role: RoleBodies, Model: "codex"}
	absentCell = Cell{Role: RoleAdjudicate, Model: "sonnet"}
)

func obs(key string, cell Cell, outcome Outcome, elapsed time.Duration) Observation {
	return Observation{
		Key:       key,
		Cell:      cell,
		Outcome:   outcome,
		StartedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		Elapsed:   elapsed,
		Provenance: Provenance{
			RunID:    "run-1",
			Revision: Revision{Source: SourceLive},
		},
	}
}

func withRevision(r Revision) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.Provenance.Revision = r
	return o
}

func withRounds(n int) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.Rounds = n
	return o
}

func TestAddDedupesByKeyAndStart(t *testing.T) {
	tab := NewTable()
	first := obs("A", seenCell, OutcomeUnfinished, time.Minute)
	again := first
	again.Outcome = OutcomeDone
	again.StartedAt = first.StartedAt.In(time.FixedZone("PDT", -7*3600))
	again.Provenance.Revision = Revision{Source: SourceGit, Commit: "abc"}
	for _, o := range []Observation{first, again} {
		if err := tab.Add(o); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := tab.Count(seenCell)
	if n != (Count{Unfinished: 1}) {
		t.Fatalf("Count = %+v, want first reading counted once", n)
	}

	rerun := first
	rerun.StartedAt = first.StartedAt.Add(time.Hour)
	if err := tab.Add(rerun); err != nil {
		t.Fatal(err)
	}
	if n, _ = tab.Count(seenCell); n.N() != 2 {
		t.Fatalf("same key at a different start counted %d, want 2", n.N())
	}

	conflict := first
	conflict.Cell = emptyCell
	err := tab.Add(conflict)
	if !errors.Is(err, ErrStampConflict) {
		t.Fatalf("Add(conflicting stamp) = %v, want ErrStampConflict", err)
	}
	if _, present := tab.Count(emptyCell); present {
		t.Fatal("rejected row made its cell present")
	}
}

func TestEmptyCellIsPresentAndDistinctFromAbsent(t *testing.T) {
	tab := NewTable(emptyCell)
	if err := tab.Add(obs("A", seenCell, OutcomeDone, time.Hour)); err != nil {
		t.Fatal(err)
	}

	n, present := tab.Count(emptyCell)
	if !present {
		t.Fatal("declared cell reported absent")
	}
	if n.N() != 0 {
		t.Fatalf("declared cell has n=%d, want 0", n.N())
	}
	if got := tab.Observations(emptyCell); got == nil || len(got) != 0 {
		t.Fatalf("empty present cell must return non-nil empty slice, got %#v", got)
	}

	n, present = tab.Count(absentCell)
	if present {
		t.Fatal("undeclared cell reported present")
	}
	if n.N() != 0 {
		t.Fatalf("absent cell has n=%d, want 0", n.N())
	}
	if got := tab.Observations(absentCell); got != nil {
		t.Fatalf("absent cell must return nil, got %#v", got)
	}

	n, present = tab.Count(seenCell)
	if !present || n.Done != 1 || n.N() != 1 {
		t.Fatalf("seen cell: present=%v count=%+v", present, n)
	}

	cells := tab.Cells()
	if len(cells) != 2 || cells[0] != emptyCell || cells[1] != seenCell {
		t.Fatalf("Cells() = %v, want [%v %v]", cells, emptyCell, seenCell)
	}
}

func TestCountSplitsByOutcome(t *testing.T) {
	tab := NewTable()
	for i, o := range []Outcome{OutcomeDone, OutcomeBlocked, OutcomeBlocked, OutcomeUnfinished} {
		if err := tab.Add(obs(string(rune('A'+i)), seenCell, o, time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := tab.Count(seenCell)
	want := Count{Done: 1, Blocked: 2, Unfinished: 1}
	if n != want {
		t.Fatalf("Count = %+v, want %+v", n, want)
	}
}

func TestCensoredRowsHaveNoDuration(t *testing.T) {
	for _, o := range []Outcome{OutcomeBlocked, OutcomeUnfinished} {
		row := obs("A", seenCell, o, 8*time.Hour)
		if !row.Censored() {
			t.Errorf("%s row not censored", o)
		}
		if d, ok := row.Duration(); ok || d != 0 {
			t.Errorf("%s row yielded duration %v ok=%v", o, d, ok)
		}
	}
	done := obs("A", seenCell, OutcomeDone, time.Hour)
	if d, ok := done.Duration(); !ok || d != time.Hour {
		t.Errorf("done row yielded %v ok=%v", d, ok)
	}
}

func TestValidateWrapsSentinels(t *testing.T) {
	cases := []struct {
		name string
		row  Observation
		want error
	}{
		{"no key", obs("", seenCell, OutcomeDone, 0), ErrUnattributable},
		{"bad role", obs("A", Cell{Role: "reviewer", Model: "opus"}, OutcomeDone, 0), ErrUnattributable},
		{"no model", obs("A", Cell{Role: RoleBodies}, OutcomeDone, 0), ErrUnattributable},
		{"zero outcome", obs("A", seenCell, 0, 0), ErrInvalidOutcome},
		{"out of range outcome", obs("A", seenCell, OutcomeUnfinished+1, 0), ErrInvalidOutcome},
		{"negative elapsed", obs("A", seenCell, OutcomeDone, -time.Second), ErrNegativeValue},
		{"negative rounds", withRounds(-1), ErrNegativeValue},
		{"zero revision", withRevision(Revision{}), ErrUnparseableRevision},
		{"live with commit", withRevision(Revision{Source: SourceLive, Commit: "abc"}), ErrUnparseableRevision},
		{"git without commit", withRevision(Revision{Source: SourceGit}), ErrUnparseableRevision},
		{"unknown source", withRevision(Revision{Source: "svn", Commit: "1"}), ErrUnparseableRevision},
	}
	for _, tc := range cases {
		err := tc.row.Validate()
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: Validate() = %v, want errors.Is %v", tc.name, err, tc.want)
		}
		if err := NewTable().Add(tc.row); !errors.Is(err, tc.want) {
			t.Errorf("%s: Add() = %v, want errors.Is %v", tc.name, err, tc.want)
		}
	}
	if err := obs("A", seenCell, OutcomeDone, 0).Validate(); err != nil {
		t.Errorf("valid row rejected: %v", err)
	}
}

func TestParseRevision(t *testing.T) {
	for _, in := range []string{"live", "git:abc123"} {
		r, err := ParseRevision(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if r.String() != in {
			t.Errorf("%q round-tripped to %q", in, r.String())
		}
	}
	for _, in := range []string{"", "git:", "git", "svn:1", "LIVE"} {
		if _, err := ParseRevision(in); !errors.Is(err, ErrUnparseableRevision) {
			t.Errorf("%q: err = %v, want ErrUnparseableRevision", in, err)
		}
	}
}
