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

// NewTable returns a Table with the given cells declared and empty.
func NewTable(declared ...Cell) *Table {
	t := &Table{
		cells: make(map[Cell][]Observation, len(declared)),
		seen:  make(map[identity]position),
	}
	for _, c := range declared {
		t.Declare(c)
	}
	return t
}

// Declare makes cell present. It does not alter an already-present cell.
func (t *Table) Declare(cell Cell) {
	if _, ok := t.cells[cell]; !ok {
		t.cells[cell] = []Observation{}
	}
}

// Add stores o after Validate, wrapping any error it returns. A row is
// identified by (Key, StartedAt); a second reading of a stored identity is
// merged into it by precedence, not by arrival order, so the stored row is
// the same whichever reading arrives first:
//
//   - A terminal outcome supersedes OutcomeUnfinished and brings its own
//     Elapsed, Rounds and Provenance with it. Two different terminal
//     outcomes wrap ErrStampConflict.
//   - Between readings with the same outcome, Elapsed and Rounds keep the
//     greater value and the remaining fields come from the reading with the
//     greater Elapsed.
//   - A different Cell wraps ErrStampConflict: one row has one stamp.
//
// A rejected reading changes nothing.
func (t *Table) Add(o Observation) error {
	if err := o.Validate(); err != nil {
		return fmt.Errorf("add observation: %w", err)
	}
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

// merge reconciles two readings of one row. It is commutative.
func merge(a, b Observation) (Observation, error) {
	when := a.StartedAt.UTC().Format(time.RFC3339)
	if a.Cell != b.Cell {
		return Observation{}, fmt.Errorf("%w: row %s at %s is %s and %s",
			ErrStampConflict, a.Key, when, a.Cell, b.Cell)
	}
	switch {
	case a.Outcome == b.Outcome:
	case a.Outcome.terminal() && b.Outcome.terminal():
		return Observation{}, fmt.Errorf("%w: row %s at %s is %s and %s",
			ErrStampConflict, a.Key, when, a.Outcome, b.Outcome)
	case b.Outcome.terminal():
		return b, nil
	default:
		return a, nil
	}
	if precedes(a, b) {
		a, b = b, a
	}
	if b.Rounds > a.Rounds {
		a.Rounds = b.Rounds
	}
	return a, nil
}

// precedes is a strict total order over readings of one row with one
// outcome; the later reading in this order is the one whose fields win.
func precedes(a, b Observation) bool {
	switch {
	case a.Elapsed != b.Elapsed:
		return a.Elapsed < b.Elapsed
	case a.Rounds != b.Rounds:
		return a.Rounds < b.Rounds
	case a.CostUSD != b.CostUSD:
		return a.CostUSD < b.CostUSD
	case a.Provenance.Revision != b.Provenance.Revision:
		return a.Provenance.Revision.String() < b.Provenance.Revision.String()
	}
	return a.Provenance.RunID < b.Provenance.RunID
}

func (t *Table) Count(cell Cell) (Count, bool) {
	obs, ok := t.cells[cell]
	if !ok {
		return Count{}, false
	}
	var n Count
	for _, o := range obs {
		switch o.Outcome {
		case OutcomeDone:
			n.Done++
		case OutcomeBlocked:
			n.Blocked++
		case OutcomeUnfinished:
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
