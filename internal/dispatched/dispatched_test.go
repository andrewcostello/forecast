package dispatched

import (
	"errors"
	"math"
	"reflect"
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

func withCost(c float64) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.CostUSD = c
	return o
}

func withDevElapsed(d time.Duration) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.DevElapsed = d
	return o
}

func withReviewElapsed(d time.Duration) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.ReviewElapsed = d
	return o
}

func withInputTokens(n int64) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.InputTokens = n
	return o
}

func withOutputTokens(n int64) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.OutputTokens = n
	return o
}

func withStart(at time.Time) Observation {
	o := obs("A", seenCell, OutcomeDone, 0)
	o.StartedAt = at
	return o
}

var gitReading = Provenance{RunID: "run-1", Revision: Revision{Source: SourceGit, Commit: "abc"}}

func TestAddDedupesByKeyAndStart(t *testing.T) {
	tab := NewTable()
	first := obs("A", seenCell, OutcomeUnfinished, time.Minute)
	again := first
	again.Outcome = OutcomeDone
	again.StartedAt = first.StartedAt.In(time.FixedZone("PDT", -7*3600))
	again.Provenance = gitReading
	for _, o := range []Observation{first, again} {
		if err := tab.Add(o); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := tab.Count(seenCell)
	if n != (Count{Done: 1}) {
		t.Fatalf("Count = %+v, want the two readings merged into one done row", n)
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

// permutations returns every ordering of rows.
func permutations(rows []Observation) [][]Observation {
	if len(rows) <= 1 {
		return [][]Observation{append([]Observation{}, rows...)}
	}
	var out [][]Observation
	for i, head := range rows {
		rest := append(append([]Observation{}, rows[:i]...), rows[i+1:]...)
		for _, tail := range permutations(rest) {
			out = append(out, append([]Observation{head}, tail...))
		}
	}
	return out
}

func TestAddJoinsReadingsInAnyOrder(t *testing.T) {
	reading := func(outcome Outcome, elapsed time.Duration, rounds int, cost float64, p Provenance) Observation {
		o := obs("A", seenCell, outcome, elapsed)
		o.Rounds, o.CostUSD, o.Provenance = rounds, cost, p
		return o
	}
	live := func(run string) Provenance { return Provenance{RunID: run, Revision: Revision{Source: SourceLive}} }
	git := func(commit, run string) Provenance {
		return Provenance{RunID: run, Revision: Revision{Source: SourceGit, Commit: commit}}
	}

	unfinished := reading(OutcomeUnfinished, time.Minute, 0, 0.5, live("run-1"))
	unfinishedLate := reading(OutcomeUnfinished, 5*time.Hour, 2, 1.0, git("abc", "run-1"))
	done := reading(OutcomeDone, 3*time.Hour, 1, 2.0, git("zzz", "run-2"))
	doneShorter := reading(OutcomeDone, 2*time.Hour, 4, 0.25, live("run-0"))
	doneShorter.StartedAt = doneShorter.StartedAt.In(time.FixedZone("PDT", -7*3600))
	blocked := reading(OutcomeBlocked, time.Hour, 1, 0.75, git("abc", "run-1"))

	cases := []struct {
		name string
		set  []Observation
		want Observation
		err  error
	}{
		{
			name: "join of four readings",
			set:  []Observation{unfinished, unfinishedLate, done, doneShorter},
			want: reading(OutcomeDone, 3*time.Hour, 4, 2.0, live("run-0")),
		},
		{
			name: "unfinished lower bound never outranks a completion",
			set:  []Observation{unfinished, unfinishedLate, done},
			want: reading(OutcomeDone, 3*time.Hour, 2, 2.0, live("run-1")),
		},
		{
			name: "blocked supersedes unfinished and keeps accumulators",
			set:  []Observation{unfinished, unfinishedLate, blocked},
			want: reading(OutcomeBlocked, time.Hour, 2, 1.0, live("run-1")),
		},
		{
			name: "same outcome keeps greater lower bound",
			set:  []Observation{unfinished, unfinishedLate, reading(OutcomeUnfinished, time.Hour, 1, 0, git("abc", "run-0"))},
			want: reading(OutcomeUnfinished, 5*time.Hour, 2, 1.0, live("run-1")),
		},
		{
			name: "two terminal outcomes conflict",
			set:  []Observation{unfinished, done, blocked},
			err:  ErrStampConflict,
		},
	}
	for _, tc := range cases {
		for _, order := range permutations(tc.set) {
			tab := NewTable()
			var conflict error
			for _, o := range order {
				// A rejected reading changes nothing: snapshot the stored
				// rows and require them byte-identical afterwards, so a
				// partial join before the conflict is detected cannot pass.
				before := tab.Observations(seenCell)
				err := tab.Add(o)
				if err != nil {
					if !errors.Is(err, ErrStampConflict) {
						t.Fatalf("%s: %v", tc.name, err)
					}
					conflict = err
					if after := tab.Observations(seenCell); !reflect.DeepEqual(before, after) {
						t.Fatalf("%s: rejected reading mutated the table:\n before %+v\n after  %+v", tc.name, before, after)
					}
				}
			}
			got := tab.Observations(seenCell)
			if tc.err != nil {
				if !errors.Is(conflict, tc.err) {
					t.Errorf("%s: order %v produced no %v", tc.name, outcomes(order), tc.err)
				}
				if len(got) != 1 {
					t.Errorf("%s: order %v stored %d rows, want 1", tc.name, outcomes(order), len(got))
				}
				continue
			}
			if conflict != nil {
				t.Fatalf("%s: order %v: %v", tc.name, outcomes(order), conflict)
			}
			if len(got) != 1 || got[0] != tc.want {
				t.Errorf("%s: order %v stored %+v, want %+v", tc.name, outcomes(order), got, tc.want)
			}
		}
	}
}

func TestAddJoinsExtractedMetricsWithoutDoubleCountingRevisions(t *testing.T) {
	first := obs("A", seenCell, OutcomeDone, time.Hour)
	first.DevElapsed = 10 * time.Minute
	first.ReviewElapsed = 20 * time.Minute
	first.InputTokens = 100
	first.OutputTokens = 10
	second := first
	second.Provenance = gitReading
	second.DevElapsed = 30 * time.Minute
	second.ReviewElapsed = 5 * time.Minute
	second.InputTokens = 50
	second.OutputTokens = 20

	table := NewTable()
	if err := table.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := table.Add(second); err != nil {
		t.Fatal(err)
	}
	got := table.Observations(seenCell)[0]
	if got.DevElapsed != 30*time.Minute || got.ReviewElapsed != 20*time.Minute || got.InputTokens != 100 || got.OutputTokens != 20 {
		t.Fatalf("merged metrics = %+v", got)
	}
}

func outcomes(rows []Observation) []Outcome {
	out := make([]Outcome, len(rows))
	for i, o := range rows {
		out[i] = o.Outcome
	}
	return out
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
		{"zero start", withStart(time.Time{}), ErrUnattributable},
		{"bad role", obs("A", Cell{Role: "reviewer", Model: "opus"}, OutcomeDone, 0), ErrUnattributable},
		{"no model", obs("A", Cell{Role: RoleBodies}, OutcomeDone, 0), ErrUnattributable},
		{"zero outcome", obs("A", seenCell, 0, 0), ErrInvalidOutcome},
		{"out of range outcome", obs("A", seenCell, OutcomeUnfinished+1, 0), ErrInvalidOutcome},
		{"negative elapsed", obs("A", seenCell, OutcomeDone, -time.Second), ErrNegativeValue},
		{"negative development elapsed", withDevElapsed(-time.Second), ErrNegativeValue},
		{"negative review elapsed", withReviewElapsed(-time.Second), ErrNegativeValue},
		{"negative rounds", withRounds(-1), ErrNegativeValue},
		{"negative input tokens", withInputTokens(-1), ErrNegativeValue},
		{"negative output tokens", withOutputTokens(-1), ErrNegativeValue},
		{"negative cost", withCost(-0.01), ErrNegativeValue},
		{"NaN cost", withCost(math.NaN()), ErrNegativeValue},
		{"positive infinite cost", withCost(math.Inf(1)), ErrNegativeValue},
		{"negative infinite cost", withCost(math.Inf(-1)), ErrNegativeValue},
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

// A present cell with n=0 means "asked, nothing observed", which is what the
// refuse-to-predict ruling reads. A typo'd role or an empty model must not be
// able to look like one: Add rejects such a row by the same rule, so a
// declared one would be an empty bucket no observation could ever fill.
func TestDeclareRefusesACellNoRowCouldJoin(t *testing.T) {
	cases := []Cell{
		{Role: "reviewer", Model: "opus"},
		{Role: "", Model: "opus"},
		{Role: RoleBodies, Model: ""},
	}
	for _, cell := range cases {
		tab := NewTable()
		if err := tab.Declare(cell); !errors.Is(err, ErrUnattributable) {
			t.Errorf("Declare(%+v) = %v, want ErrUnattributable", cell, err)
		}
		if _, present := tab.Count(cell); present {
			t.Errorf("Declare(%+v) made an unjoinable cell present", cell)
		}
		if cells := NewTable(cell).Cells(); len(cells) != 0 {
			t.Errorf("NewTable(%+v) declared %v", cell, cells)
		}
	}
	tab := NewTable()
	if err := tab.Declare(emptyCell); err != nil {
		t.Fatalf("Declare(%+v) = %v", emptyCell, err)
	}
	if _, present := tab.Count(emptyCell); !present {
		t.Fatalf("Declare(%+v) did not make the cell present", emptyCell)
	}
}
