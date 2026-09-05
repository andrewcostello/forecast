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
	"sort"
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

	// Amended contract fields (F1/F2). Wall is the classified breakdown of
	// Elapsed, nil while unknown; when set it supersedes DevElapsed and
	// ReviewElapsed, which the baseline reducer filled from the superseded
	// faithful-copy timing rule and which FC-JOURNAL replaces. Evidence is
	// per-field provenance beside the single baseline Provenance. Validate
	// does not inspect either; Wall.Validate is applied by the reducer that
	// produces it.
	Wall     *WallBreakdown
	Evidence ObservationEvidence
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

// ---------------------------------------------------------------------------
// F1–F4 amendment (docs/plans/forecasting-contracts.md). Everything below is
// frozen for the FC-SEALS / FC-JOURNAL / FC-SOURCES / FC-1 rows. The baseline
// types above are unchanged so legacy call shapes keep compiling; the notes
// file names which baseline fields are superseded.
// ---------------------------------------------------------------------------

// AttemptID is the identity of one attempt at one task: the dispatcher run,
// the task key and the task_started instant, normalised to UTC.
//
// Different runs are different attempts even when key and instant coincide.
// Readings that share the whole triple are readings of ONE attempt, never
// additional samples. Two journal task_started events with an identical
// triple are ambiguous (ErrAmbiguousAttempt), never resolved by proximity.
// This supersedes the baseline identity (Key, StartedAt) in referenceclass.go.
type AttemptID struct {
	RunID     string
	Key       string
	StartedAt time.Time
}

// NewAttemptID builds an AttemptID with startedAt normalised to UTC, so two
// offsets of one instant are one identity.
func NewAttemptID(runID, key string, startedAt time.Time) AttemptID {
	return AttemptID{RunID: runID, Key: key, StartedAt: startedAt.UTC()}
}

// Normalize returns the identity with StartedAt in UTC.
func (id AttemptID) Normalize() AttemptID {
	return NewAttemptID(id.RunID, id.Key, id.StartedAt)
}

// Valid reports whether every component is present. An identity missing any
// part cannot join evidence (DispositionMissingJoinKeys).
func (id AttemptID) Valid() bool {
	return id.RunID != "" && id.Key != "" && !id.StartedAt.IsZero()
}

// Equal compares identities by instant, not by offset.
func (id AttemptID) Equal(other AttemptID) bool {
	return id.RunID == other.RunID && id.Key == other.Key && id.StartedAt.Equal(other.StartedAt)
}

// Less is the stable order for reports and deterministic tie-breaks: start
// instant, then key, then run ID.
func (id AttemptID) Less(other AttemptID) bool {
	switch {
	case !id.StartedAt.Equal(other.StartedAt):
		return id.StartedAt.Before(other.StartedAt)
	case id.Key != other.Key:
		return id.Key < other.Key
	}
	return id.RunID < other.RunID
}

func (id AttemptID) String() string {
	return id.RunID + "/" + id.Key + "@" + id.StartedAt.UTC().Format(time.RFC3339Nano)
}

// AttemptID returns the amended identity of a stored row. Provenance.RunID is
// the run component; the baseline row never carried it in its identity, which
// is the defect (FC-1 panel Codex-1) the amendment corrects.
func (o Observation) AttemptID() AttemptID {
	return NewAttemptID(o.Provenance.RunID, o.Key, o.StartedAt)
}

// Measured is a quantity that may be unknown. Known false means nobody
// measured it: it is not zero, must not enter a mean, and reading Value
// without checking Known is a defect. A measured zero has Known true.
//
// Money and tokens are separate Measured values from wall time; nothing
// converts between them.
type Measured[T any] struct {
	Value T
	Known bool
}

// Known returns a known measurement.
func Known[T any](value T) Measured[T] { return Measured[T]{Value: value, Known: true} }

// Unknown returns an unknown measurement.
func Unknown[T any]() Measured[T] { return Measured[T]{} }

// Get returns the value and whether it is known.
func (m Measured[T]) Get() (T, bool) { return m.Value, m.Known }

// Must returns the value or wraps ErrUnknownMeasurement; name says what the
// measurement was for the error message.
func (m Measured[T]) Must(name string) (T, error) {
	if !m.Known {
		var zero T
		return zero, fmt.Errorf("%w: %s", ErrUnknownMeasurement, name)
	}
	return m.Value, nil
}

// Phase classifies a wall-clock interval inside an attempt. The four values
// are distinct concepts and never pooled: development is implementer or
// corrective work; panel review is panel wall time (one interval per review,
// never the sum of simultaneous reviewer seats); verifier is verification
// wall time; unclassified is the residual an additive breakdown could not
// attribute. Unclassified is never reported as development.
type Phase string

const (
	PhaseDevelopment  Phase = "development"
	PhasePanelReview  Phase = "panel_review"
	PhaseVerifier     Phase = "verifier"
	PhaseUnclassified Phase = "unclassified"
)

// Valid reports whether p is a declared phase.
func (p Phase) Valid() bool {
	switch p {
	case PhaseDevelopment, PhasePanelReview, PhaseVerifier, PhaseUnclassified:
		return true
	}
	return false
}

// Interval is one classified half-open wall-clock span [Start, End) inside
// an attempt. Inferred is true when the boundaries were not both recorded
// events (an explicit boundary or duration in the record makes it false);
// an inferred interval whose attribution is ambiguous is not emitted at all.
// Evidence names the events that bound it.
type Interval struct {
	Phase    Phase
	Start    time.Time
	End      time.Time
	Inferred bool
	Evidence []EventRef
}

// Duration is End−Start. Validate rejects a reversed interval before it can
// be summed.
func (iv Interval) Duration() time.Duration { return iv.End.Sub(iv.Start) }

// Validate wraps ErrInvalidOutcome for an undeclared phase and
// ErrReversedInterval when End precedes Start.
func (iv Interval) Validate() error {
	if !iv.Phase.Valid() {
		return fmt.Errorf("%w: interval phase %q", ErrInvalidOutcome, iv.Phase)
	}
	if iv.End.Before(iv.Start) {
		return fmt.Errorf("%w: %s interval %s to %s", ErrReversedInterval, iv.Phase,
			iv.Start.UTC().Format(time.RFC3339Nano), iv.End.UTC().Format(time.RFC3339Nano))
	}
	return nil
}

// WallBreakdown is the additive decomposition of one attempt's elapsed wall
// time into disjoint classified intervals contained in [StartedAt,
// StartedAt+Elapsed]. Complete is false when phase data was missing or
// ambiguous: the breakdown is then incomplete, not zero and not a fabricated
// precise split. Elapsed is always usable on its own (F2: known elapsed
// survives an unknown finer breakdown).
type WallBreakdown struct {
	StartedAt time.Time
	Elapsed   time.Duration
	Intervals []Interval
	Complete  bool
}

// Classified sums the intervals by phase. Unclassified residual is reported
// by Unclassified, never stored as an interval of PhaseDevelopment.
func (w WallBreakdown) Classified() map[Phase]time.Duration {
	out := make(map[Phase]time.Duration, 4)
	for _, iv := range w.Intervals {
		out[iv.Phase] += iv.Duration()
	}
	return out
}

// Unclassified is Elapsed minus the sum of classified intervals. Validate
// guarantees it is not negative.
func (w WallBreakdown) Unclassified() time.Duration {
	sum := time.Duration(0)
	for _, iv := range w.Intervals {
		sum += iv.Duration()
	}
	return w.Elapsed - sum
}

// Validate wraps ErrNegativeValue for a negative Elapsed, the interval's own
// error for a reversed or unphased interval, and ErrOverlappingIntervals
// when intervals overlap each other, leave the attempt, or sum past Elapsed.
// Intervals are checked in Start order regardless of slice order.
func (w WallBreakdown) Validate() error {
	if w.Elapsed < 0 {
		return fmt.Errorf("%w: elapsed %v", ErrNegativeValue, w.Elapsed)
	}
	ordered := make([]Interval, len(w.Intervals))
	copy(ordered, w.Intervals)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
	attemptEnd := w.StartedAt.Add(w.Elapsed)
	sum := time.Duration(0)
	for i, iv := range ordered {
		if err := iv.Validate(); err != nil {
			return err
		}
		if !w.StartedAt.IsZero() && (iv.Start.Before(w.StartedAt) || iv.End.After(attemptEnd)) {
			return fmt.Errorf("%w: %s interval %s to %s leaves the attempt %s to %s", ErrOverlappingIntervals, iv.Phase,
				iv.Start.UTC().Format(time.RFC3339Nano), iv.End.UTC().Format(time.RFC3339Nano),
				w.StartedAt.UTC().Format(time.RFC3339Nano), attemptEnd.UTC().Format(time.RFC3339Nano))
		}
		if i > 0 && iv.Start.Before(ordered[i-1].End) {
			return fmt.Errorf("%w: %s interval starting %s overlaps %s interval ending %s", ErrOverlappingIntervals,
				iv.Phase, iv.Start.UTC().Format(time.RFC3339Nano), ordered[i-1].Phase, ordered[i-1].End.UTC().Format(time.RFC3339Nano))
		}
		sum += iv.Duration()
	}
	if sum > w.Elapsed {
		return fmt.Errorf("%w: classified %v exceeds elapsed %v", ErrOverlappingIntervals, sum, w.Elapsed)
	}
	return nil
}

// EvidenceSource is where a field's value was read from, in precedence
// order: a journal event outranks a tasks-YAML field, which outranks nothing.
// The authored YAML model is never evidence for the implementing model.
type EvidenceSource int

const (
	EvidenceNone EvidenceSource = iota
	EvidenceYAML
	EvidenceJournal
)

func (s EvidenceSource) String() string {
	switch s {
	case EvidenceJournal:
		return "journal"
	case EvidenceYAML:
		return "yaml"
	case EvidenceNone:
		return "none"
	}
	return fmt.Sprintf("EvidenceSource(%d)", int(s))
}

// FieldEvidence records what one field of an observation rests on: the
// source class, the journal event (when Source is EvidenceJournal) and the
// reading (when Source is EvidenceYAML). Unknown stays EvidenceNone.
type FieldEvidence struct {
	Source  EvidenceSource
	Event   EventRef
	Reading ReadingRef
}

// ObservationEvidence is the per-field provenance the amended row carries
// beside the baseline single Provenance. Model is the implementing stamp
// (journal spawn payload) or none; Terminal is the terminal outcome's
// evidence. It is comparable, so Observation stays comparable; the events
// summed for cost and tokens are listed on Attempt, not here.
type ObservationEvidence struct {
	Model    FieldEvidence
	Terminal FieldEvidence
}
