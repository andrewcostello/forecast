package dispatched

// journal.go: dispatcher journal parsing and per-attempt state.
//
// Ownership: FC-JOURNAL implements the frozen seams in this file
// (ParseEvents, ReduceAttempts). The baseline section is the FC-1 reducer
// moved verbatim from extract.go; Build still runs it, and it stays the
// behavior of record until FC-1 switches Build to ReduceAttempts. The
// baseline's timing rule (a faithful copy of model-matrix journal_facts) is
// explicitly superseded by F2; see the notes file.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ---------------------------------------------------------------------------
// Frozen contract (F1/F2).
// ---------------------------------------------------------------------------

// Producer semantics of the measured dispatcher journal, frozen so fixtures
// and the reducer agree on what an event means. Recorded from real journals
// on 2026-09-04 (dispatcher_version 0.1.0); a fixture states the producer
// revision it was captured from.
//
// Per corrective round the producer emits, in this order:
//
//	task_spawn_finished{spawn_kind: panel-iterate}   corrective spawn returned
//	panel_iterate{iteration: n}                       emitted AFTER the spawn returns
//	panel_started{iteration: n}
//	panel_verdict
//
// The first review of an attempt is panel_started/panel_verdict with no
// preceding panel_iterate: a first review is not a correction round.
// Verification is verification_started/verification_verdict around a
// task_spawn_finished{spawn_kind: verifier}. Terminal events are task_done
// and task_blocked. agent_fallback records a cascade. task_started carries
// the PLANNED model, never the stamp.
const (
	// ProducerDispatcherV0_1_0 names the journal producer whose sequences the
	// fixtures freeze. Read from run_started.payload.dispatcher_version.
	ProducerDispatcherV0_1_0 = "dispatcher 0.1.0"

	EventRunStarted           = "run_started"
	EventTaskStarted          = "task_started"
	EventTaskSpawnFinished    = "task_spawn_finished"
	EventPanelStarted         = "panel_started"
	EventPanelVerdict         = "panel_verdict"
	EventPanelIterate         = "panel_iterate"
	EventVerificationStarted  = "verification_started"
	EventVerificationVerdict  = "verification_verdict"
	EventVerificationIterate  = "verification_iterate"
	EventAgentFallback        = "agent_fallback"
	EventTaskDone             = "task_done"
	EventTaskBlocked          = "task_blocked"
	SpawnKindImplementer      = "implementer"
	SpawnKindPanelIterate     = "panel-iterate"
	SpawnKindVerifier         = "verifier"
	SpawnKindReviewer         = "reviewer"
	journalScannerInitialSize = 64 * 1024
)

// JournalIdentity names one journal file: the run it belongs to, the source
// it was read from and its path there. Producer is the dispatcher version
// recorded in run_started, empty when the journal has none.
type JournalIdentity struct {
	RunID    string
	SourceID string
	Path     string
	Producer string
}

// EventRef is the position of one event in one journal: the producer's seq
// when present, else the 1-based line. It is the provenance unit for every
// field of an attempt; conflicts and sums cite it.
type EventRef struct {
	Journal JournalIdentity
	Seq     int
	Line    int
	Type    string
	At      time.Time
}

// EventPayload is the subset of a journal payload the reducer reads. Every
// number that can be absent in the producer is Measured: duration_ms and
// cost_usd are null in real spawn records.
type EventPayload struct {
	Model        string
	SpawnKind    string
	Iteration    Measured[int]
	InputTokens  Measured[int64]
	OutputTokens Measured[int64]
	CostUSD      Measured[float64]
	Duration     Measured[time.Duration]
	FromAgent    string
	ToAgent      string
	Reason       string
	Status       string
}

// Event is one parsed journal line with a task key. Lines without a task
// key (run_started, preflight, heartbeat) are not Events; ParseEvents keeps
// run_started only to fill JournalIdentity.Producer.
type Event struct {
	Ref     EventRef
	TaskKey string
	Payload EventPayload
}

// JournalDiagnostics counts what a journal could not yield. Every count is
// reported; none is a reason to drop the rest of the journal.
type JournalDiagnostics struct {
	Lines           int
	Events          int
	LinesUnparsed   int
	BadTimestamps   int
	LinesOverBound  int
	MissingProducer bool
}

// Attempt is the normalised record of one attempt (F1/F2). It is the joint
// record the sampler (F4) resamples, so its fields stay paired.
type Attempt struct {
	ID    AttemptID
	Start EventRef

	// Model is the final recorded implementing stamp: the model on the last
	// implementer or panel-iterate task_spawn_finished. Unknown when no such
	// spawn carried a model (DispositionAbsentStamp); never the planned
	// model from task_started nor the authored YAML model.
	Model         Measured[string]
	ModelEvidence FieldEvidence
	// Cascades counts agent_fallback events; a cascaded attempt is disclosed,
	// and Model describes the closing model only.
	Cascades int

	// Outcome is OutcomeDone/OutcomeBlocked on a terminal event, else
	// OutcomeUnfinished with Elapsed measured to Cutoff. Blocked and
	// unfinished elapsed are right-censored lower bounds.
	Outcome    Outcome
	Terminal   FieldEvidence
	TerminalAt time.Time
	Cutoff     time.Time
	Elapsed    time.Duration
	Wall       WallBreakdown
	// Corrections is the recorded correction-round count (panel_iterate
	// events). Reviews is the review invocation count (panel_started). They
	// are different numbers; a first review is not a correction.
	Corrections   int
	Reviews       int
	Verifications int

	// Tokens and Cost are summed once over implementing spawn payloads;
	// CostEvents and TokenEvents list exactly the events summed, so no
	// event is summed twice. Absent cost is Unknown, measured zero is
	// Known(0).
	InputTokens  Measured[int64]
	OutputTokens Measured[int64]
	CostUSD      Measured[float64]
	CostEvents   []EventRef
	TokenEvents  []EventRef
	Evidence     ObservationEvidence
}

// Censored reports whether Elapsed is a lower bound.
func (a Attempt) Censored() bool { return a.Outcome != OutcomeDone }

// AttemptSet is what ReduceAttempts yields for one journal: the unambiguous
// attempts, the identities that were excluded as ambiguous (each counted
// once per identity, with the number of task_started events that shared
// it), and conflicts found inside one attempt.
type AttemptSet struct {
	Journal     JournalIdentity
	Attempts    []Attempt
	Ambiguous   []AmbiguousAttempt
	Conflicts   []AttemptConflict
	Diagnostics JournalDiagnostics
	// LeadingEvents counts events seen before any task_started for a key;
	// they belong to no attempt and are never folded into the next one.
	LeadingEvents int
}

// AmbiguousAttempt is an identity that two or more task_started events
// shared. Starts is that number; Refs cites them.
type AmbiguousAttempt struct {
	ID     AttemptID
	Starts int
	Refs   []EventRef
}

// AttemptConflict is a within-attempt disagreement between two readings of
// equal authority (F1). Field is "model" or "terminal".
type AttemptConflict struct {
	ID    AttemptID
	Field string
	A, B  FieldEvidence
	Err   error
}

// ParseEvents decodes one journal stream into Events, in file order, bounded
// by bounds.MaxLineBytes. It never guesses: an undecodable line, an
// unparseable timestamp and an over-bound line are counted in the
// diagnostics and skipped; a read failure or cancellation is an error
// wrapping ErrJournalSource or ErrSourceCancelled.
//
// FC-JOURNAL body. Parameters are named so the body can use them; the
// scaffold returns ErrNotImplemented and reads none of them.
func ParseEvents(ctx context.Context, journal JournalIdentity, reader io.Reader, bounds ReadBounds) ([]Event, JournalDiagnostics, error) {
	return nil, JournalDiagnostics{}, fmt.Errorf("%w: ParseEvents(%s)", ErrNotImplemented, journal.Path)
}

// ReduceAttempts folds a journal's events into attempts under the F1/F2
// rules: one Attempt per unambiguous AttemptID; identical triples excluded
// as ambiguous; elapsed to the terminal event or to cutoff; a validated
// disjoint WallBreakdown with Complete false when phase data is missing;
// corrections separate from reviews; cost and tokens summed once with their
// EventRefs; implementing stamp with cascade count. The result is
// deterministic under permutations of events that share a timestamp.
//
// FC-JOURNAL body. Parameters are named so the body can use them; the
// scaffold returns ErrNotImplemented and reads none of them.
func ReduceAttempts(journal JournalIdentity, events []Event, cutoff time.Time) (AttemptSet, error) {
	return AttemptSet{}, fmt.Errorf("%w: ReduceAttempts(%s)", ErrNotImplemented, journal.Path)
}

// ---------------------------------------------------------------------------
// FC-1 baseline reducer, moved from extract.go unchanged. Superseded by
// ReduceAttempts once FC-1 switches Build over; kept so the artifact does not
// change under the move.
// ---------------------------------------------------------------------------

type runTask struct {
	RunID string
	Key   string
}

// JournalFacts is the Go port of model-matrix/report.py::journal_facts, for a
// single attempt at one task.
//
// Model is taken ONLY from payloads whose spawn_kind is implementer or
// panel-iterate, exactly as the reference does. The model on task_started is
// the model the dispatcher PLANNED, recorded before any cascade; using it
// would attribute a cascaded row to a model that never ran. An attempt with
// no such payload has no Model and cannot be placed in a cell.
//
// Superseded: DevElapsed and ReviewElapsed overlap in production event order
// (FC-1 panel Claude-1); F2 replaces them with Attempt.Wall.
type JournalFacts struct {
	// StartedAt is the timestamp of the task_started that opened this
	// attempt; zero for events seen before any task_started.
	StartedAt          time.Time
	Model              string
	Started            int
	StartsWithoutModel int
	Rounds             int
	InputTokens        int64
	OutputTokens       int64
	// CostUSD is summed from spawn payloads. CostKnown distinguishes "the
	// spawns cost nothing that was recorded" from "no spawn recorded a cost";
	// the reference implementation reads cost from the tasks YAML instead and
	// so has no equivalent.
	CostUSD         float64
	CostKnown       bool
	DevElapsed      time.Duration
	ReviewElapsed   time.Duration
	TerminalOutcome Outcome
	TerminalAt      time.Time
	Fallbacks       int

	reviewOpen time.Time
	devOpen    time.Time
}

// JournalRow holds every attempt at one (run, task key). A run that restarts
// a task emits a second task_started; folding both into one set of facts
// double-counts its tokens and lets two YAML snapshots with different
// started_at values each claim the same terminal event.
type JournalRow struct {
	Attempts []*JournalFacts
}

// Starts is the number of task_started events seen for the row.
func (r *JournalRow) Starts() int {
	n := 0
	for _, f := range r.Attempts {
		n += f.Started
	}
	return n
}

// Restarts is the number of task_started events beyond the first.
func (r *JournalRow) Restarts() int { return max(r.Starts()-1, 0) }

// StartsWithoutModel is the number of task_started events carrying no model.
func (r *JournalRow) StartsWithoutModel() int {
	n := 0
	for _, f := range r.Attempts {
		n += f.StartsWithoutModel
	}
	return n
}

// AttemptsWithoutStampedModel counts started attempts that no implementing
// spawn stamped a model on. Such an attempt is unattributable by rule.
func (r *JournalRow) AttemptsWithoutStampedModel() int {
	n := 0
	for _, f := range r.Attempts {
		if f.Started > 0 && f.Model == "" {
			n++
		}
	}
	return n
}

// Match binds a snapshot only to an attempt with the same start instant.
// Multiple starts at that instant are ambiguous and cannot supply evidence.
func (r *JournalRow) Match(startedAt time.Time) *JournalFacts {
	var match *JournalFacts
	for _, f := range r.Attempts {
		if !f.StartedAt.IsZero() && f.StartedAt.Equal(startedAt) {
			if match != nil {
				return nil
			}
			match = f
		}
	}
	return match
}

// observe routes one event to the attempt it belongs to. A task_started opens
// a new attempt; events seen before any task_started are kept in a leading
// attempt with no start instant rather than discarded.
func (r *JournalRow) observe(eventType string, at time.Time, p journalPayload) {
	if eventType == "task_started" {
		r.Attempts = append(r.Attempts, &JournalFacts{StartedAt: at})
	} else if len(r.Attempts) == 0 {
		r.Attempts = append(r.Attempts, &JournalFacts{})
	}
	r.Attempts[len(r.Attempts)-1].observe(eventType, at, p)
}

type journalEvent struct {
	EventType string          `json:"event_type"`
	TaskKey   string          `json:"task_key"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type journalPayload struct {
	Model        string   `json:"model"`
	SpawnKind    string   `json:"spawn_kind"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

// implementing reports whether a payload describes the spawn that actually
// did the work, which is the only payload the reference trusts for a model.
func (p journalPayload) implementing() bool {
	return p.SpawnKind == "implementer" || p.SpawnKind == "panel-iterate"
}

// journalSources is what readJournals recovered, with the shortfalls it hit.
// Unreadable input is counted rather than dropped: recovered-versus-started
// is only an honest denominator if the lines nobody could read are named.
type journalSources struct {
	Rows          map[runTask]*JournalRow
	LinesUnparsed int
	BadTimestamps int
}

func scanJournal(ctx context.Context, reader io.Reader, path, runID string, out *journalSources) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, journalScannerInitialSize), DefaultMaxLineBytes)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: scan %s: %w", ErrJournalSource, path, err)
		}
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var event journalEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			out.LinesUnparsed++
			continue
		}
		if event.TaskKey == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil {
			out.BadTimestamps++
			continue
		}
		var payload journalPayload
		if len(event.Payload) != 0 && string(event.Payload) != "null" {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				out.LinesUnparsed++
				continue
			}
		}

		key := runTask{RunID: runID, Key: event.TaskKey}
		row := out.Rows[key]
		if row == nil {
			row = &JournalRow{}
			out.Rows[key] = row
		}
		row.observe(event.EventType, at, payload)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: scan %s: %v", ErrJournalSource, path, err)
	}
	return nil
}

// observe deliberately mirrors journal_facts' state machine, event for event.
// Superseded by F2 (see ReduceAttempts); retained for the baseline artifact.
func (f *JournalFacts) observe(eventType string, at time.Time, p journalPayload) {
	switch eventType {
	case "panel_started":
		f.reviewOpen = at
	case "panel_verdict":
		if !f.reviewOpen.IsZero() {
			f.ReviewElapsed += at.Sub(f.reviewOpen)
			f.reviewOpen = time.Time{}
		}
	}

	if eventType == "task_started" || eventType == "panel_iterate" {
		f.devOpen = at
	} else if eventType == "task_spawn_finished" && p.implementing() && !f.devOpen.IsZero() {
		f.DevElapsed += at.Sub(f.devOpen)
		f.devOpen = time.Time{}
	}

	switch eventType {
	case "task_started":
		f.Started++
		// Counted, never used to attribute: this model is the plan, not the
		// stamp. See the JournalFacts godoc.
		if p.Model == "" {
			f.StartsWithoutModel++
		}
	case "panel_iterate":
		f.Rounds++
	case "agent_fallback":
		f.Fallbacks++
	case "task_done":
		f.TerminalOutcome, f.TerminalAt = OutcomeDone, at
	case "task_blocked":
		f.TerminalOutcome, f.TerminalAt = OutcomeBlocked, at
	}

	if p.implementing() {
		if p.Model != "" {
			f.Model = p.Model
		}
		f.InputTokens += p.InputTokens
		f.OutputTokens += p.OutputTokens
		if p.CostUSD != nil {
			f.CostUSD += *p.CostUSD
			f.CostKnown = true
		}
	}
}
