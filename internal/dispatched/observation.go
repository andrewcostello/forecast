// Package dispatched is the contract between the units that observe
// dispatched agent rows, the reference class built from them, and the
// scheduler that forecasts a dependency graph of such rows.
//
// It states rules. Counts, coverage and which cells are empty are
// measurements: they live in the assignment doc and in tests.
package dispatched

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Role is the dispatcher role authored on a row. The declared constants are
// the only valid values; Validate rejects anything else.
type Role string

const (
	RoleScaffold   Role = "scaffold"
	RoleSeals      Role = "seals"
	RoleBodies     Role = "bodies"
	RoleAdjudicate Role = "adjudicate"
)

// Valid reports whether r is one of the declared constants.
func (r Role) Valid() bool {
	switch r {
	case RoleScaffold, RoleSeals, RoleBodies, RoleAdjudicate:
		return true
	}
	return false
}

// Outcome is how an observed row ended. Zero is deliberately not a declared
// value, so an unset Outcome fails Validate rather than passing as a result.
type Outcome int

const (
	// OutcomeDone: the dispatcher recorded task_done. The only outcome whose
	// elapsed time is a completed duration.
	OutcomeDone Outcome = iota + 1
	// OutcomeBlocked: the dispatcher recorded task_blocked.
	OutcomeBlocked
	// OutcomeUnfinished: a start with no terminal event (in progress, or a
	// run that died).
	OutcomeUnfinished
)

// Valid reports whether o is one of the declared constants.
func (o Outcome) Valid() bool {
	return o >= OutcomeDone && o <= OutcomeUnfinished
}

// terminal reports whether o records how the row ended rather than that it
// was still running when read.
func (o Outcome) terminal() bool { return o == OutcomeDone || o == OutcomeBlocked }

func (o Outcome) String() string {
	switch o {
	case OutcomeDone:
		return "done"
	case OutcomeBlocked:
		return "blocked"
	case OutcomeUnfinished:
		return "unfinished"
	}
	return fmt.Sprintf("Outcome(%d)", int(o))
}

// Cell is the bucket key. Model is the model the dispatcher STAMPED on the
// row after any cascade, never the one authored in the tasks YAML. A row
// whose stamp is unknown has no Cell.
type Cell struct {
	Role  Role
	Model string
}

func (c Cell) String() string { return string(c.Role) + "/" + c.Model }

// Source names where a row's fields were read from.
type Source string

const (
	// SourceLive: the tasks YAML as checked out.
	SourceLive Source = "live"
	// SourceGit: the tasks YAML at a specific commit.
	SourceGit Source = "git"
)

// Revision identifies one reading of a tasks YAML. Commit is required for
// SourceGit and must be empty for SourceLive; Valid checks this and
// Observation.Validate applies it.
type Revision struct {
	Source Source
	Commit string
}

// Valid reports whether r obeys the Source/Commit rule.
func (r Revision) Valid() bool {
	switch r.Source {
	case SourceLive:
		return r.Commit == ""
	case SourceGit:
		return r.Commit != ""
	}
	return false
}

// ParseRevision reads the form produced by Revision.String: "live" or
// "git:<commit>". Any other input wraps ErrUnparseableRevision.
func ParseRevision(s string) (Revision, error) {
	if s == string(SourceLive) {
		return Revision{Source: SourceLive}, nil
	}
	if commit, ok := strings.CutPrefix(s, string(SourceGit)+":"); ok && commit != "" {
		return Revision{Source: SourceGit, Commit: commit}, nil
	}
	return Revision{}, fmt.Errorf("%w: %q", ErrUnparseableRevision, s)
}

func (r Revision) String() string {
	if r.Source == SourceGit {
		return string(SourceGit) + ":" + r.Commit
	}
	return string(r.Source)
}

// Provenance says which dispatcher run produced the row and which reading
// of the tasks YAML supplied its role and timestamps.
type Provenance struct {
	RunID    string
	Revision Revision
}

// Observation is one dispatched row as the reference class stores it.
//
// Elapsed is wall clock from start to the terminal event, or to the moment
// of extraction when there is none. It is a completed duration only when
// Outcome is OutcomeDone; otherwise it is a lower bound and must never enter
// a duration statistic. Use Duration rather than reading Elapsed directly.
type Observation struct {
	Key        string
	Cell       Cell
	Outcome    Outcome
	StartedAt  time.Time
	Elapsed    time.Duration
	Rounds     int
	CostUSD    float64
	Provenance Provenance
}

// Censored reports whether Elapsed is a lower bound rather than a duration.
func (o Observation) Censored() bool { return o.Outcome != OutcomeDone }

// Duration returns the completed duration, or ok=false for a censored row.
func (o Observation) Duration() (d time.Duration, ok bool) {
	if o.Censored() {
		return 0, false
	}
	return o.Elapsed, true
}

// Validate reports the first rule the row breaks. A missing key or start
// time, an invalid role or an empty model wrap ErrUnattributable; an
// undeclared outcome wraps ErrInvalidOutcome; a negative Elapsed, Rounds or
// CostUSD, or a NaN CostUSD, wraps ErrNegativeValue; a revision that fails
// Revision.Valid wraps ErrUnparseableRevision.
func (o Observation) Validate() error {
	switch {
	case o.Key == "":
		return fmt.Errorf("%w: empty key", ErrUnattributable)
	case o.StartedAt.IsZero():
		return fmt.Errorf("%w: row %s has no start time", ErrUnattributable, o.Key)
	case !o.Cell.Role.Valid():
		return fmt.Errorf("%w: row %s has role %q", ErrUnattributable, o.Key, o.Cell.Role)
	case o.Cell.Model == "":
		return fmt.Errorf("%w: row %s has no stamped model", ErrUnattributable, o.Key)
	case !o.Outcome.Valid():
		return fmt.Errorf("%w: row %s has %s", ErrInvalidOutcome, o.Key, o.Outcome)
	case o.Elapsed < 0:
		return fmt.Errorf("%w: row %s has elapsed %v", ErrNegativeValue, o.Key, o.Elapsed)
	case o.Rounds < 0:
		return fmt.Errorf("%w: row %s has rounds %d", ErrNegativeValue, o.Key, o.Rounds)
	case o.CostUSD < 0 || math.IsNaN(o.CostUSD):
		return fmt.Errorf("%w: row %s has cost %v", ErrNegativeValue, o.Key, o.CostUSD)
	case !o.Provenance.Revision.Valid():
		return fmt.Errorf("%w: row %s has revision %+v", ErrUnparseableRevision, o.Key, o.Provenance.Revision)
	}
	return nil
}
