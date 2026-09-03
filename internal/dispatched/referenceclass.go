package dispatched

import (
	"fmt"
	"sort"
	"time"
)

// Count is the observation tally for one cell, split by outcome. N is the
// total. All zero is a legitimate value: it means the cell is known and
// nothing has been observed in it.
type Count struct {
	Done       int
	Blocked    int
	Unfinished int
}

// N is the total number of observations in the cell.
func (c Count) N() int { return c.Done + c.Blocked + c.Unfinished }

// ReferenceClass answers questions about observed cells.
//
// A cell is either present or absent. Present with N()==0 means the class
// was asked about the cell and has no observations; absent means nobody
// asked. The two must be distinguishable: Count returns present=false only
// for the absent case, never for an empty one.
type ReferenceClass interface {
	// Count reports the tally for cell and whether the cell is present.
	Count(cell Cell) (n Count, present bool)

	// Cells lists every present cell, including empty ones, in a stable order.
	Cells() []Cell

	// Observations returns the rows in cell. Nil for an absent cell; an
	// empty non-nil slice for a present empty one.
	Observations(cell Cell) []Observation
}

// Table is an in-memory ReferenceClass. Declaring a cell makes it present
// with no observations; adding a row makes its cell present.
//
// A Table is not safe for concurrent use: Add both reads and writes its maps.
// Serialise calls or guard it externally.
type Table struct {
	cells map[Cell][]Observation
	seen  map[identity]position
}

// identity is what makes two readings of a row the same row. StartedAt is
// held as a UTC instant so offsets do not split one row in two.
type identity struct {
	Key       string
	StartedAt time.Time
}

func identityOf(o Observation) identity {
	return identity{Key: o.Key, StartedAt: o.StartedAt.UTC()}
}

// position locates a stored row. Rows are never removed, so an index into
// the cell's slice stays valid.
type position struct {
	cell Cell
	idx  int
}

// NewTable returns a Table with the given cells declared and empty. A cell
// Declare rejects is skipped, so NewTable never produces a present cell that
// no row could ever join.
func NewTable(declared ...Cell) *Table {
	t := &Table{
		cells: make(map[Cell][]Observation, len(declared)),
		seen:  make(map[identity]position),
	}
	for _, c := range declared {
		_ = t.Declare(c)
	}
	return t
}

// Declare makes cell present with no observations. It does not alter an
// already-present cell.
//
// A cell with an undeclared role or an empty model wraps ErrUnattributable
// and is NOT made present: Add rejects rows for such a cell by the same rule,
// so declaring one would show a typo as an empty coverage bucket that looks
// exactly like a real one — the distinction the refuse-to-predict ruling
// rests on.
func (t *Table) Declare(cell Cell) error {
	switch {
	case !cell.Role.Valid():
		return fmt.Errorf("%w: cell has role %q", ErrUnattributable, cell.Role)
	case cell.Model == "":
		return fmt.Errorf("%w: cell %s has no model", ErrUnattributable, cell.Role)
	}
	if _, ok := t.cells[cell]; !ok {
		t.cells[cell] = []Observation{}
	}
	return nil
}

// Add stores o after Validate, wrapping any error it returns. A row is
// identified by (Key, StartedAt) and stored with StartedAt in UTC. A second
// reading of a stored identity is joined into it field by field, so any set
// of readings stores one row whatever order they arrive in:
//
//   - Outcome and Elapsed join as one pair: a terminal outcome outranks
//     OutcomeUnfinished and brings its own Elapsed; equal rank keeps the
//     greater Elapsed. Two different terminal outcomes wrap ErrStampConflict.
//   - Rounds, Cascades, DevElapsed, ReviewElapsed, the token totals and
//     CostUSD each keep the greater value; CostKnown is the disjunction.
//   - Provenance keeps the least under: SourceLive before SourceGit, then
//     Commit, then RunID.
//   - A different Cell wraps ErrStampConflict: one row has one stamp.
//
// A rejected reading changes nothing.
func (t *Table) Add(o Observation) error {
	if err := o.Validate(); err != nil {
		return fmt.Errorf("add observation: %w", err)
	}
	o.StartedAt = o.StartedAt.UTC()
	id := identityOf(o)
	if at, ok := t.seen[id]; ok {
		stored := &t.cells[at.cell][at.idx]
		merged, err := merge(*stored, o)
		if err != nil {
			return fmt.Errorf("add observation: %w", err)
		}
		*stored = merged
		return nil
	}
	t.seen[id] = position{cell: o.Cell, idx: len(t.cells[o.Cell])}
	t.cells[o.Cell] = append(t.cells[o.Cell], o)
	return nil
}

// rank orders outcomes for merging. The two terminal outcomes tie so that
// they conflict rather than one silently winning.
func (o Outcome) rank() int {
	if o.terminal() {
		return 1
	}
	return 0
}

// merge joins two readings of one row. Each field is combined by a
// commutative, associative operation on a totally ordered value, so a fold
// over any permutation of readings yields the same row.
func merge(a, b Observation) (Observation, error) {
	when := a.StartedAt.Format(time.RFC3339)
	if a.Cell != b.Cell {
		return Observation{}, fmt.Errorf("%w: row %s at %s is %s from %s and %s from %s",
			ErrStampConflict, a.Key, when, a.Cell, describe(a.Provenance), b.Cell, describe(b.Provenance))
	}
	if a.Outcome != b.Outcome && a.Outcome.terminal() && b.Outcome.terminal() {
		return Observation{}, fmt.Errorf("%w: row %s at %s is %s from %s and %s from %s",
			ErrStampConflict, a.Key, when, a.Outcome, describe(a.Provenance), b.Outcome, describe(b.Provenance))
	}
	out := a
	if ra, rb := a.Outcome.rank(), b.Outcome.rank(); rb > ra || (rb == ra && b.Elapsed > a.Elapsed) {
		out.Outcome, out.Elapsed, out.TerminalEvidence = b.Outcome, b.Elapsed, b.TerminalEvidence
	}
	out.Rounds = max(a.Rounds, b.Rounds)
	out.Cascades = max(a.Cascades, b.Cascades)
	out.DevElapsed = max(a.DevElapsed, b.DevElapsed)
	out.ReviewElapsed = max(a.ReviewElapsed, b.ReviewElapsed)
	out.InputTokens = max(a.InputTokens, b.InputTokens)
	out.OutputTokens = max(a.OutputTokens, b.OutputTokens)
	out.CostUSD = max(a.CostUSD, b.CostUSD)
	out.CostKnown = a.CostKnown || b.CostKnown
	if provenanceLess(b.Provenance, a.Provenance) {
		out.Provenance = b.Provenance
	}
	return out, nil
}

// describe names a provenance in an error a human has to act on: which run
// and which reading of the tasks YAML the disagreeing value came from.
func describe(p Provenance) string {
	return "run " + p.RunID + " at " + p.Revision.String()
}

// provenanceLess is a strict total order over Provenance alone: SourceLive
// before SourceGit, then Commit, then RunID.
func provenanceLess(a, b Provenance) bool {
	switch {
	case a.Revision.Source != b.Revision.Source:
		return a.Revision.Source == SourceLive
	case a.Revision.Commit != b.Revision.Commit:
		return a.Revision.Commit < b.Revision.Commit
	}
	return a.RunID < b.RunID
}

func (t *Table) Count(cell Cell) (Count, bool) {
	obs, ok := t.cells[cell]
	if !ok {
		return Count{}, false
	}
	// Every row lands in exactly one bucket, so Count.N() always equals
	// len(Observations(cell)) — including for an Outcome added later that
	// none of the switches above knows about.
	var n Count
	for _, o := range obs {
		switch {
		case o.Outcome == OutcomeDone:
			n.Done++
		case o.Outcome == OutcomeBlocked:
			n.Blocked++
		default:
			n.Unfinished++
		}
	}
	return n, true
}

func (t *Table) Cells() []Cell {
	out := make([]Cell, 0, len(t.cells))
	for c := range t.cells {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].Model < out[j].Model
	})
	return out
}

func (t *Table) Observations(cell Cell) []Observation {
	obs, ok := t.cells[cell]
	if !ok {
		return nil
	}
	return append([]Observation{}, obs...)
}

var _ ReferenceClass = (*Table)(nil)
