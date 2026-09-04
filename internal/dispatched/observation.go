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
//
// Adding a value means updating every site that decodes the set together:
// Valid, terminal, String, rank (referenceclass.go) and Table.Count's switch.
// Valid is a range check, so a new constant appended below Unfinished passes
// it silently while the others still treat it as unknown.
type Outcome int

const (
	// OutcomeDone: the row finished. The only outcome whose elapsed time is a
	// completed duration. The evidence may be a journal task_done or a
	// terminal status in the tasks YAML; Observation.TerminalEvidence says
	// which, because a YAML status is a mutable file a human may have edited.
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
	Role  Role   `json:"role"`
	Model string `json:"model"`
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

// Provenance says which dispatcher run produced the row and names ONE
// reading of the tasks YAML it was recovered from.
//
// When several readings are joined into one row, Provenance is the least of
// them under provenanceLess (live before git, then commit, then run) so the
// join is order-independent. It therefore identifies a reading the row was
// seen in, NOT the reading that supplied any particular other field: Outcome,
// Elapsed, Rounds and CostUSD are each taken from whichever reading ranked
// highest for that field. Per-field attribution would need a set of
// contributing provenances and is deliberately not carried.
type Provenance struct {
	RunID      string
	Revision   Revision
	Repository string
	Path       string
}

// Observation is one dispatched row as the reference class stores it.
//
// Elapsed is wall clock from start to the terminal event, or to the moment
// of extraction when there is none. It is a completed duration only when
// Outcome is OutcomeDone; otherwise it is a lower bound and must never enter
// a duration statistic. Use Duration rather than reading Elapsed directly.
type Observation struct {
	Key     string
	Cell    Cell
	Outcome Outcome
	// TerminalEvidence records what the Outcome rests on: a journal terminal
	// event, a tasks-YAML status, or nothing. A consumer that will not average
	// a hand-editable timestamp needs to be able to tell them apart.
	TerminalEvidence string
	StartedAt        time.Time
	Elapsed          time.Duration
	DevElapsed       time.Duration
	ReviewElapsed    time.Duration
	Rounds           int
	// Cascades is the number of agent_fallback events on the row: the signal
	// that the authored model is stale for it.
	Cascades     int
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	// CostKnown separates a measured zero from an unmeasured one. A row with
	// CostKnown false has no cost, and CostUSD must not be read as one.
	CostKnown  bool
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
// undeclared outcome wraps ErrInvalidOutcome; a negative duration, count or
// token total, or a cost that is not finite, wraps ErrNegativeValue; a
// revision that fails Revision.Valid wraps ErrUnparseableRevision.
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
	case o.DevElapsed < 0:
		return fmt.Errorf("%w: row %s has development elapsed %v", ErrNegativeValue, o.Key, o.DevElapsed)
	case o.ReviewElapsed < 0:
		return fmt.Errorf("%w: row %s has review elapsed %v", ErrNegativeValue, o.Key, o.ReviewElapsed)
	case o.Rounds < 0:
		return fmt.Errorf("%w: row %s has rounds %d", ErrNegativeValue, o.Key, o.Rounds)
	case o.Cascades < 0:
		return fmt.Errorf("%w: row %s has cascades %d", ErrNegativeValue, o.Key, o.Cascades)
	case o.InputTokens < 0:
		return fmt.Errorf("%w: row %s has input tokens %d", ErrNegativeValue, o.Key, o.InputTokens)
	case o.OutputTokens < 0:
		return fmt.Errorf("%w: row %s has output tokens %d", ErrNegativeValue, o.Key, o.OutputTokens)
	case o.CostUSD < 0 || math.IsNaN(o.CostUSD) || math.IsInf(o.CostUSD, 0):
		// -Inf is already caught by < 0; the explicit form documents that a
		// cost must be finite. merge combines cost with max, so one +Inf
		// reading would poison the row and every aggregate over it.
		return fmt.Errorf("%w: row %s has cost %v", ErrNegativeValue, o.Key, o.CostUSD)
	case !o.Provenance.Revision.Valid():
		return fmt.Errorf("%w: row %s has revision %+v", ErrUnparseableRevision, o.Key, o.Provenance.Revision)
	}
	return nil
}
