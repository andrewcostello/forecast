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

// Producer wire semantics are frozen in the handoff's F2-PRODUCER-SHAPES,
// F2-CORRECTION-KINDS and F2-VERIFICATIONS rows. Verified against dispatcher
// orchestrator.py at df771516b905355995d03313b470b06e1aea4e06 and recorded
// dispatcher_version 0.1.0 journals. The version string alone does not identify
// a source revision; fixtures record both their origin and the shape used.
// panel_started with iteration AND iterations_remaining is a review invocation;
// forced_by=path_classification is a gate record, not a review. Corrective
// panel_iterate/verification_iterate markers follow their spawn finish.
// Planned task_started models never supply an implementing stamp.
const (
	// ProducerDispatcherV0_1_0 is the exact wire value of
	// run_started.payload.dispatcher_version for the producer whose sequences
	// the fixtures freeze. JournalIdentity.Producer carries the raw value as
	// read, with no prefix or normalisation, so a real journal compares equal
	// to this constant.
	ProducerDispatcherV0_1_0 = "0.1.0"

	EventRunStarted             = "run_started"
	EventTaskStarted            = "task_started"
	EventTaskSpawnFinished      = "task_spawn_finished"
	EventPanelStarted           = "panel_started"
	EventPanelVerdict           = "panel_verdict"
	EventPanelIterate           = "panel_iterate"
	EventVerificationStarted    = "verification_started"
	EventVerificationVerdict    = "verification_verdict"
	EventVerificationIterate    = "verification_iterate"
	EventVerificationSkipped    = "verification_skipped"
	EventVerificationMechanical = "verification_mechanical"
	EventAgentFallback          = "agent_fallback"
	EventTaskDone               = "task_done"
	EventTaskBlocked            = "task_blocked"
	SpawnKindImplementer        = "implementer"
	SpawnKindPanelIterate       = "panel-iterate"
	SpawnKindVerifier           = "verifier"
	SpawnKindVerifierIterate    = "verifier-iterate"
	SpawnKindTestFixRetry       = "test-fix-retry"
	SpawnKindCommitRetry        = "commit-retry"
	SpawnKindPushRetry          = "push-retry"
	SpawnKindSummaryRecovery    = "summary-recovery"
	ForcedByPathClassification  = "path_classification"
)

// JournalIdentity names one journal file: the run it belongs to, the source
// it was read from and its path there. Producer is the raw
// run_started.payload.dispatcher_version as read from the journal, empty
// when the journal has none; ParseEvents fills it and returns the resolved
// identity in ParsedJournal.Journal, and stamps that identity on every
// EventRef it emits.
type JournalIdentity struct {
	RunID    string `json:"run_id"`
	SourceID string `json:"source_id"`
	Path     string `json:"path"`
	Producer string `json:"producer"`
}

// EventRef retains the physical citation and optional producer identity.
// HasSeq distinguishes an absent sequence (Seq=0) from an explicit zero. Inside
// one JournalIdentity, equal present Seq plus equivalent task/type/instant/payload
// is a retransmission: ParseEvents retains the least physical line once. Different
// payloads under one sequence are malformed: discard every colliding line and
// count each in LinesUnparsed. Without Seq, distinct physical lines are distinct
// events even when their contents agree; never guess that identical spawns are
// retransmissions. Hash fields preserve producer provenance; this contract does
// not claim cryptographic chain verification or detection of an omitted tail.
type EventRef struct {
	HasSeq   bool            `json:"has_seq"`
	Hash     string          `json:"hash"`
	PrevHash string          `json:"prev_hash"`
	Journal  JournalIdentity `json:"journal"`
	Seq      int             `json:"seq"`
	Line     int             `json:"line"`
	Type     string          `json:"type"`
	At       time.Time       `json:"at"`
}

// EventPayload mirrors scalar producer payloads. Nil means absent or JSON null;
// a pointer to zero is measured zero. ParseEvents validates quantities and
// ReduceAttempts lifts these wire values into Attempt's Measured fields.
// DurationMillis is duration_ms on the wire; conversion to nanoseconds is checked.
type EventPayload struct {
	Model               string   `json:"model"`
	SpawnKind           string   `json:"spawn_kind"`
	Iteration           *int     `json:"iteration"`
	InputTokens         *int64   `json:"input_tokens"`
	OutputTokens        *int64   `json:"output_tokens"`
	CostUSD             *float64 `json:"cost_usd"`
	DurationMillis      *int64   `json:"duration_ms"`
	FromAgent           string   `json:"from_agent"`
	ToAgent             string   `json:"to_agent"`
	Reason              string   `json:"reason"`
	Status              string   `json:"status"`
	IterationsRemaining *int     `json:"iterations_remaining"`
	ForcedBy            string   `json:"forced_by"`
}

// Event is one parsed journal line with a task key. Lines without a task
// key (run_started, preflight, heartbeat) are not Events; ParseEvents keeps
// run_started only to fill JournalIdentity.Producer.
type Event struct {
	Ref     EventRef     `json:"ref"`
	TaskKey string       `json:"task_key"`
	Payload EventPayload `json:"payload"`
}

// ParsedJournal is what ParseEvents yields for one journal: the identity
// with Producer resolved from run_started (so a journal holding run_started
// and no task events still exposes its producer), the task events in file
// order, and the diagnostics. It is the unit ReduceAttempts consumes.
type ParsedJournal struct {
	Journal     JournalIdentity    `json:"journal"`
	Events      []Event            `json:"events"`
	Diagnostics JournalDiagnostics `json:"diagnostics"`
}

// JournalDiagnostics counts what a journal could not yield. Every count is
// reported; none is a reason to drop the rest of the journal.
type JournalDiagnostics struct {
	Lines           int  `json:"lines"`
	Events          int  `json:"events"`
	LinesUnparsed   int  `json:"lines_unparsed"`
	BadTimestamps   int  `json:"bad_timestamps"`
	LinesOverBound  int  `json:"lines_over_bound"`
	MissingProducer bool `json:"missing_producer"`
}

// CostScopeRecordedSpawns labels quantities available in recorded task spawns.
const CostScopeRecordedSpawns = "recorded_task_spawns"

// Attempt is the normalized joint record resampled by F4; fields stay paired.
type Attempt struct {
	ID    AttemptID `json:"id"`
	Start EventRef  `json:"start"`

	// Model is the final recorded implementing stamp: the model on the last
	// implementing task_spawn_finished (implementer, panel-iterate, verifier-iterate,
	// test-fix-retry, commit-retry, push-retry or summary-recovery). Verifier/unknown kinds do not stamp a model. Unknown when no such
	// spawn carried a model (DispositionAbsentStamp); never the planned
	// model from task_started nor the authored YAML model. Its evidence is
	// Evidence.Model; there is no second copy.
	Model Measured[string] `json:"model"`
	// Cascades counts agent_fallback events; a cascaded attempt is disclosed,
	// and Model describes the closing model only. Evidence.Cascades cites
	// the least agent_fallback event, or EvidenceNone when Cascades is 0.
	Cascades      int        `json:"cascades"`
	CascadeEvents []EventRef `json:"cascade_events"`

	// Outcome is OutcomeDone/OutcomeBlocked on a terminal event, else
	// OutcomeUnfinished with Elapsed measured to Cutoff. Blocked and
	// unfinished elapsed are right-censored lower bounds. The terminal
	// event is Evidence.Terminal; there is no second copy.
	Outcome    Outcome       `json:"outcome"`
	TerminalAt time.Time     `json:"terminal_at"`
	Cutoff     time.Time     `json:"cutoff"`
	Elapsed    time.Duration `json:"elapsed_ns"`
	Wall       WallBreakdown `json:"wall"`
	// Corrections counts panel_iterate + verification_iterate markers plus
	// test-fix-retry/commit-retry/push-retry/summary-recovery finishes, each once. Reviews counts only
	// invocation-shaped panel_started; Verifications counts verification_started
	// only (not skipped, mechanical, iterate or verdict). Each evidence field cites
	// the least canonical counted event, or EvidenceNone for zero; full lists follow.
	Corrections        int        `json:"corrections"`
	Reviews            int        `json:"reviews"`
	Verifications      int        `json:"verifications"`
	CorrectionEvents   []EventRef `json:"correction_events"`
	ReviewEvents       []EventRef `json:"review_events"`
	VerificationEvents []EventRef `json:"verification_events"`

	// Cost/tokens sum ALL task_spawn_finished kinds once under EventRef identity,
	// including verifier and retries. Any missing contribution makes that total
	// Unknown; zero spawns also means Unknown. Complete measured zeros are Known(0).
	// InputTokens means the producer's uncached input_tokens only; cache token
	// quantities are not included. CostScope must equal CostScopeRecordedSpawns;
	// separate panel-seat/operator spend absent from journals is outside this
	// measurement. Consumers must label that scope, never call it total process cost.
	// Every counted-event list (including CascadeEvents) is COMPLETE, including
	// the least event named by FieldEvidence, in canonical order. Zero lists are [].
	// Available per-quantity event lists remain even when the total is unknown.
	CostScope string `json:"cost_scope"`

	InputTokens       Measured[int64]   `json:"input_tokens"`
	OutputTokens      Measured[int64]   `json:"output_tokens"`
	CostUSD           Measured[float64] `json:"cost_usd"`
	CostEvents        []EventRef        `json:"cost_events"`
	InputTokenEvents  []EventRef        `json:"input_token_events"`
	OutputTokenEvents []EventRef        `json:"output_token_events"`

	// Evidence is the single per-field provenance record of the attempt.
	// Invariants: Evidence.Model.Source is EvidenceJournal iff Model.Known;
	// Evidence.Terminal.Source is EvidenceJournal iff Outcome is terminal
	// from a journal event (EvidenceNone when censored to Cutoff);
	// Evidence.Start is the task_started event; Evidence.Elapsed equals
	// Evidence.Terminal; summed fields cite the first available canonical ref.
	// JoinEvidence reconciles this record with YAML citations atomically.
	Evidence ObservationEvidence `json:"evidence"`
}

// Censored rejects invalid outcomes with ErrInvalidOutcome, otherwise reports
// whether elapsed is a lower bound. FC-JOURNAL body, F4-OUTCOME-WIRE.
func (a Attempt) Censored() (bool, error) {
	return false, fmt.Errorf("%w: Attempt.Censored", ErrNotImplemented)
}

// MarshalJSON emits textual outcome done/blocked/unfinished in schema 4; invalid
// outcomes wrap ErrInvalidOutcome. All other fields retain their declared wire
// tags and Measured representation. FC-JOURNAL body; baseline Outcome is unchanged.
func (a Attempt) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("%w: Attempt.MarshalJSON", ErrNotImplemented)
}

// UnmarshalJSON rejects missing/unknown/non-text outcome with ErrInvalidOutcome;
// successful decoding never converts absent outcome to a plausible censored row.
// FC-JOURNAL body; see F4-OUTCOME-WIRE and F4-SCHEMA-ROUNDTRIP.
func (a *Attempt) UnmarshalJSON(data []byte) error {
	return fmt.Errorf("%w: Attempt.UnmarshalJSON", ErrNotImplemented)
}

// AttemptSet is what ReduceAttempts yields for one journal: the unambiguous
// attempts, the identities that were excluded as ambiguous (each counted
// once per identity, with the number of task_started events that shared
// it), and conflicts found inside one attempt.
type AttemptSet struct {
	// StartsAfterCutoff counts omitted task_started events; they create no attempt
	// and never enter Attempts/LostAttempts or a negative elapsed calculation.
	StartsAfterCutoff int                `json:"starts_after_cutoff"`
	Journal           JournalIdentity    `json:"journal"`
	Attempts          []Attempt          `json:"attempts"`
	Ambiguous         []AmbiguousAttempt `json:"ambiguous"`
	Conflicts         []AttemptConflict  `json:"conflicts"`
	Diagnostics       JournalDiagnostics `json:"diagnostics"`
	// LeadingEvents counts events seen before any task_started for a key;
	// they belong to no attempt and are never folded into the next one.
	LeadingEvents int `json:"leading_events"`
}

// AmbiguousAttempt is an identity that two or more task_started events
// shared. Starts is that number; Refs cites them.
type AmbiguousAttempt struct {
	ID     AttemptID  `json:"id"`
	Starts int        `json:"starts"`
	Refs   []EventRef `json:"refs"`
}

// ConflictValue is a tagged portable candidate. Value is canonical JSON:
// model/role -> string; terminal -> {outcome,terminal_at,elapsed_ns}; tokens ->
// integer; cost -> finite number; wall -> WallBreakdown; event -> Event.
// Kind is one of model, role, terminal, input_tokens, output_tokens, cost, wall,
// event. No candidate is omitted; A/AValue and B/BValue are selected atomically.
type ConflictValue struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

const EvidenceConflictCode = "evidence_conflict"

// AttemptConflict retains both incompatible values and their complete citations.
// Field names the conflicting measurement, role, model or terminal selection unit.
type AttemptConflict struct {
	Code   string        `json:"code"` // always EvidenceConflictCode
	AValue ConflictValue `json:"a_value"`
	BValue ConflictValue `json:"b_value"`
	ID     AttemptID     `json:"id"`
	Field  string        `json:"field"`
	A      FieldEvidence `json:"a"`
	B      FieldEvidence `json:"b"`
	Err    error         `json:"-"`
	Reason string        `json:"reason"`
}

// ParseEvents decodes one journal stream into a ParsedJournal, events in
// file order, bounded by bounds.MaxLineBytes (zero uses DefaultMaxLineBytes;
// negative returns ErrInvalidSourceSpec). It applies that line-bound rule
// itself, without depending on the later FC-SOURCES validation body.
// It applies EventRef retransmission/sequence-collision rules before reduction.
// Unknown panel_started shapes (neither invocation nor path-classification gate)
// are malformed LinesUnparsed, rather than guessed review starts. The returned Journal is the
// input identity with Producer filled from run_started (Diagnostics.
// MissingProducer when there is none), and every Event.Ref.Journal is that
// resolved identity. It never guesses: an undecodable line, an unparseable
// timestamp and an over-bound line are counted in the diagnostics and
// skipped; a read failure or cancellation is an error wrapping
// ErrJournalSource or ErrSourceCancelled (also ctx.Err()), with all parsed
// events/diagnostics retained. Numeric decode/type failures count LinesUnparsed;
// negative/non-finite quantities and duration-ms conversion overflow do too.
// See F2-SPAWN-WIRE and F2-CORRECTIVE-BOUNDARY in the handoff table.
//
// FC-JOURNAL body. Parameters are named so the body can use them; the
// scaffold returns ErrNotImplemented and reads none of them.
func ParseEvents(ctx context.Context, journal JournalIdentity, reader io.Reader, bounds ReadBounds) (ParsedJournal, error) {
	return ParsedJournal{Journal: journal}, fmt.Errorf("%w: ParseEvents(%s)", ErrNotImplemented, journal.Path)
}

// ReduceAttempts reduces one journal at a required nonzero UTC cutoff. It returns
// retained valid attempts/diagnostics alongside ErrMeasurementOverflow or
// ErrReversedInterval, and ErrInvalidSelection for an invalid cutoff.
// Authoritative behavior: notes/FC-SCAFFOLD.md, F1/F2 tables and the "Entry-point
// contracts" section. FC-JOURNAL owns this and the Attempt JSON/Censored seams.
func ReduceAttempts(parsed ParsedJournal, cutoff time.Time) (AttemptSet, error) {
	return AttemptSet{Journal: parsed.Journal}, fmt.Errorf("%w: ReduceAttempts(%s)", ErrNotImplemented, parsed.Journal.Path)
}

// SummarizeWall validates nonzero start, nonnegative elapsed, canonical
// interval/citation order, valid classified phases, containment and disjointness.
// It uses checked duration sums; zero-length intervals occupy no time.
// ErrInvalidPhase names a bad phase (including an explicit unclassified span),
// ErrUnattributable a missing start, ErrReversedInterval a reversed span, and
// ErrOverlappingIntervals an overlap/outside span. Negative elapsed wraps
// ErrNegativeValue; noncanonical order wraps ErrNonCanonicalEvidence; arithmetic
// overflow wraps ErrMeasurementOverflow (checked, never saturating). Complete is
// copied from WallBreakdown so missing phases remain distinguishable. Unclassified is elapsed
// minus classified time. No duration is returned on error.
// FC-JOURNAL implements this after TestFCJournalContract is authored.
func SummarizeWall(w WallBreakdown) (WallSummary, error) {
	return WallSummary{}, fmt.Errorf("%w: SummarizeWall", ErrNotImplemented)
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
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
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
