package dispatched

// evidence.go: joining journal attempts with tasks-YAML readings.
//
// Ownership: FC-1 implements the frozen seam in this file (JoinEvidence)
// and may replace the baseline join. The baseline section is the FC-1 code
// moved verbatim from build.go; Build still runs it until FC-1 switches to
// JoinEvidence, so the artifact does not change under the move.

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Frozen contract (F1/F3 reconciliation).
// ---------------------------------------------------------------------------

// Disposition is the fate of one examined YAML snapshot. Every snapshot
// gets exactly one; the counts are reported per disposition so no lost
// attempt hides behind a recovered sibling.
type Disposition string

const (
	// DispositionRecovered: the snapshot matched one unambiguous attempt and
	// contributed a reading to it.
	DispositionRecovered       Disposition = "recovered"
	DispositionNotTaskDocument Disposition = "not_task_document"
	// DispositionDuplicateReading: a further reading of an attempt that
	// already has a recovered envelope, even across different revisions. Once all
	// compatible evidence has been reconciled, the least canonical ReadingRef is
	// Recovered; every other envelope for that AttemptID is DuplicateReading.
	// All citations remain; conflicting attempts get no Recovered envelope.
	DispositionDuplicateReading Disposition = "duplicate_reading"
	// DispositionMissingJoinKeys: no key, run ID or started_at in the raw
	// row (Reading.Present incomplete) while the row otherwise parsed.
	DispositionMissingJoinKeys Disposition = "missing_join_keys"
	// DispositionMalformed: a timestamp or field that would not parse, or a
	// document that would not decode (Reading.Err != nil). Distinct from
	// DispositionMissingJoinKeys: a malformed started_at was present.
	DispositionMalformed Disposition = "malformed"
	// DispositionNoMatchingRun: no journal has that (run ID, key).
	DispositionNoMatchingRun Disposition = "no_matching_run_key"
	// DispositionNoMatchingStart: the run/key exists but no task_started has
	// that exact instant. Stale or hand-edited timestamps land here; there
	// is no nearest-start match.
	DispositionNoMatchingStart Disposition = "no_matching_start"
	// DispositionAmbiguousStart: more than one task_started shares the
	// instant (ErrAmbiguousAttempt).
	DispositionAmbiguousStart Disposition = "ambiguous_start"
	// DispositionAbsentStamp: the attempt matched but no implementing spawn
	// stamped a model, so it has no cell (ErrUnattributable).
	DispositionAbsentStamp Disposition = "absent_stamp"
	// DispositionConflictingEvidence: the attempt's readings disagree at
	// equal authority (ErrEvidenceConflict).
	DispositionConflictingEvidence Disposition = "conflicting_evidence"
	// DispositionUnrecoverable: the joined row failed Validate for a reason
	// other than attribution (negative value, invalid outcome, revision).
	DispositionUnrecoverable Disposition = "unrecoverable"
	// DispositionHeldOut: the run is in Selection.HoldoutRunIDs.
	DispositionHeldOut Disposition = "held_out"
	// DispositionAfterCutoff: RecordedAt OR YAML started_at OR completed_at
	// exceeds Selection.Cutoff. HeldOut wins if both apply. Live mtime and Git
	// committer time supply the same RecordedAt predicate. Such envelopes are
	// identity-only, with the marker retained after predictive fields are erased.
	DispositionAfterCutoff Disposition = "after_cutoff"
)

// Dispositions lists every declared value in report order.
func Dispositions() []Disposition {
	return []Disposition{
		DispositionNotTaskDocument, DispositionRecovered, DispositionDuplicateReading, DispositionMissingJoinKeys,
		DispositionMalformed, DispositionNoMatchingRun, DispositionNoMatchingStart,
		DispositionAmbiguousStart, DispositionAbsentStamp, DispositionConflictingEvidence,
		DispositionUnrecoverable, DispositionHeldOut, DispositionAfterCutoff,
	}
}

// Valid reports whether d is declared, without allocating a slice per row.
func (d Disposition) Valid() bool {
	switch d {
	case DispositionNotTaskDocument, DispositionRecovered, DispositionDuplicateReading,
		DispositionMissingJoinKeys, DispositionMalformed, DispositionNoMatchingRun,
		DispositionNoMatchingStart, DispositionAmbiguousStart, DispositionAbsentStamp,
		DispositionConflictingEvidence, DispositionUnrecoverable, DispositionHeldOut,
		DispositionAfterCutoff:
		return true
	}
	return false
}

// Examined is one reading's disposition with the identity it was matched
// to (zero when no attempt could be named) and a reason a human can act on.
// Reading.Row identifies the row, so two rows of one file are two
// Examined entries.
type Examined struct {
	Reading     ReadingRef  `json:"reading"`
	Attempt     AttemptID   `json:"attempt"`
	Disposition Disposition `json:"disposition"`
	Reason      string      `json:"reason"`
}

// DispositionCount is one row of the per-disposition tally.
type DispositionCount struct {
	Disposition Disposition `json:"disposition"`
	Count       int         `json:"count"`
}

// RecoveredAttempt is the amended joint sample and portable record. Cell.Model
// equals Attempt.Model.Value (which must be known); Cell.Role comes from matching
// YAML evidence, cited by Attempt.Evidence.Role. Differing valid YAML roles at
// equal authority are ErrEvidenceConflict, not separate samples. Readings lists
// every contributing tasks-YAML citation. No
// legacy Table.Add/merge operation is used to reconcile this record.
type RecoveredAttempt struct {
	Attempt  Attempt      `json:"attempt"`
	Cell     Cell         `json:"cell"`
	Readings []ReadingRef `json:"readings"`
}

// EvidenceJoin is the result of joining attempts with readings. Unique rows
// (distinct run/key) and attempts (distinct AttemptID) are reported
// separately; LostAttempts are started attempts with no recovered reading,
// listed individually so a recovered sibling cannot hide them.
type EvidenceJoin struct {
	StartsAfterCutoff int
	Observations      []RecoveredAttempt
	Examined          []Examined
	Dispositions      []DispositionCount
	Conflicts         []AttemptConflict
	UniqueRows        int
	Attempts          int
	Recovered         int
	LostAttempts      []AttemptID
	Ambiguous         []AmbiguousAttempt
	HeldOutRuns       []string
	CutoffApplied     time.Time
}

// JoinEvidence joins attempts with readings under F1/F3: readings bind to
// an attempt only on an exact AttemptID; the journal terminal and
// implementer stamp outrank YAML; a YAML-only terminal is labeled; unknown
// stays unknown; within-attempt conflicts of equal authority are excluded
// as ErrEvidenceConflict; no row is manufactured from independent maxima
// of incompatible readings; every Reading receives exactly one Disposition
// (non-task document → DispositionNotTaskDocument; exclusion marker or selection
// match → excluded disposition; Err → DispositionMalformed; missing join keys →
// DispositionMissingJoinKeys,
// then the match outcome); held-out runs and post-cutoff attempts are
// excluded from contribution before joining; every reconciled field carries its
// FieldEvidence in its RecoveredAttempt.Attempt; the result is identical under any permutation of attempts
// and readings.
//
// FC-1 body. Parameters are named so the body can use them; the scaffold
// returns ErrNotImplemented and reads none of them.
// Values and citations are selected atomically, with the terminal outcome,
// terminal time and elapsed treated as one unit. Canonicalize all instants to
// UTC without monotonic components. Event citation ties use WallBreakdown's
// canonical EventRef order; YAML ties compare ReadingRef source ID, repository,
// path, revision string, row, RecordedAt. A tie never chooses incompatible values.
// Full event lists and verification counts remain in RecoveredAttempt.
// Selection.Validate is mandatory. All Attempt.Cutoff values must equal the
// nonzero Selection.Cutoff or return ErrInvalidSelection; unfinished elapsed is
// always measured to that instant. Recheck Ref.RecordedAt and YAML start/terminal
// against cutoff. Missing recorded time is malformed (including direct inputs
// without Reading.Err), never unrecoverable. HeldOut wins over cutoff, then
// malformed, then missing keys. Honor source AfterCutoff markers after erased
// fields; rechecking may add exclusions but never remove a source exclusion.
// A HeldOut marker inconsistent with Selection wraps ErrInvalidSelection. A YAML-only
// terminal can contribute only from a revision and terminal at/before cutoff.
// Invalid roles, outcomes, negative/non-finite measurements, overflow or invalid
// citations are unrecoverable; no invalid joint record enters Observations.
// Arithmetic overflow wraps ErrMeasurementOverflow; invalid canonical evidence
// supplied to validation wraps ErrNonCanonicalEvidence. Build retains diagnostic
// data but marks the artifact PARTIAL on these reconciliation errors.
func JoinEvidence(attempts []AttemptSet, readings []Reading, selection Selection) (EvidenceJoin, error) {
	return EvidenceJoin{}, fmt.Errorf("%w: JoinEvidence(%d journals, %d readings)", ErrNotImplemented, len(attempts), len(readings))
}

// ---------------------------------------------------------------------------
// FC-1 baseline join, moved from build.go unchanged.
// ---------------------------------------------------------------------------

// joinReadings folds every reading of one identity into a single row using
// the same rules Table.Add applies, and reports the conflict rather than
// picking a winner. It is total: the fold is over a commutative operation, so
// the order readings arrive in does not change the result.
//
// Superseded: the identity it folds over omits the run ID (FC-1 panel
// Codex-1) and the TerminalEvidence tie is order-dependent (Claude-7).
func joinReadings(rows []Observation) (Observation, error) {
	firstCell := rows[0]
	var terminal *Observation
	for i := range rows {
		row := &rows[i]
		if row.Cell != firstCell.Cell {
			_, err := merge(firstCell, *row)
			return Observation{}, err
		}
		if row.Outcome.terminal() {
			if terminal != nil && terminal.Outcome != row.Outcome {
				_, err := merge(*terminal, *row)
				return Observation{}, err
			}
			terminal = row
		}
	}
	joined := rows[0]
	for _, row := range rows[1:] {
		next, err := merge(joined, row)
		if err != nil {
			return Observation{}, err
		}
		joined = next
	}
	return joined, nil
}

// Terminal evidence values. A journal terminal event is the dispatcher's own
// record; a YAML status is a mutable file a human may have edited, and edge
// case 9 says a hand-finished row is indistinguishable from an agent one.
// Legacy none is the string "none"; amended EvidenceNone is deliberately "".
// The other two source strings agree. Do not rewrite baseline output to match.
const (
	terminalEvidenceJournal = "journal"
	terminalEvidenceYAML    = "yaml"
	terminalEvidenceNone    = "none"
)

// observationFrom joins one YAML snapshot to the journal attempt it names.
//
// The terminal event, when the journal has one, decides both the outcome and
// the instant elapsed time is measured to. A YAML completed_at is used ONLY
// when the YAML status is itself terminal: a row still marked in progress
// carrying a stale completed_at is censored, and its lower bound runs to now,
// not back to a timestamp from a previous attempt.
func observationFrom(snapshot taskSnapshot, facts *JournalFacts, now time.Time) (Observation, error) {
	if !snapshot.Role.Valid() || facts.Model == "" {
		return Observation{}, fmt.Errorf("%w: row %s has role %q and stamped model %q", ErrUnattributable, snapshot.Key, snapshot.Role, facts.Model)
	}
	outcome, end, evidence := OutcomeUnfinished, now, terminalEvidenceNone
	if facts.TerminalOutcome.Valid() {
		outcome, end, evidence = facts.TerminalOutcome, facts.TerminalAt, terminalEvidenceJournal
	} else if yamlOutcome, ok := terminalStatus(snapshot.Status); ok && !snapshot.CompletedAt.IsZero() {
		outcome, end, evidence = yamlOutcome, snapshot.CompletedAt, terminalEvidenceYAML
	}
	if end.Before(snapshot.StartedAt) {
		return Observation{}, fmt.Errorf("build observation: %w: row %s ends at %s before it starts at %s", ErrNegativeValue, snapshot.Key, end.Format(time.RFC3339Nano), snapshot.StartedAt.Format(time.RFC3339Nano))
	}
	observation := Observation{
		Key:              snapshot.Key,
		Cell:             Cell{Role: snapshot.Role, Model: facts.Model},
		Outcome:          outcome,
		TerminalEvidence: evidence,
		StartedAt:        snapshot.StartedAt,
		Elapsed:          end.Sub(snapshot.StartedAt),
		DevElapsed:       facts.DevElapsed,
		ReviewElapsed:    facts.ReviewElapsed,
		Rounds:           max(snapshot.IterationCount, facts.Rounds),
		Cascades:         facts.Fallbacks,
		InputTokens:      facts.InputTokens,
		OutputTokens:     facts.OutputTokens,
		CostUSD:          facts.CostUSD,
		CostKnown:        facts.CostKnown,
		Provenance:       Provenance{RunID: snapshot.DispatcherRunID, Revision: snapshot.Revision, Repository: snapshot.Repository, Path: snapshot.Path},
	}
	if err := observation.Validate(); err != nil {
		return Observation{}, fmt.Errorf("build observation: %w", err)
	}
	return observation, nil
}

func terminalStatus(status string) (Outcome, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "done":
		return OutcomeDone, true
	case "blocked":
		return OutcomeBlocked, true
	}
	return 0, false
}

func isUnrecoverableObservationError(err error) bool {
	return errors.Is(err, ErrNegativeValue) ||
		errors.Is(err, ErrInvalidOutcome) ||
		errors.Is(err, ErrUnparseableRevision)
}
