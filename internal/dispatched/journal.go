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
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strings"
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
	SpawnKindDesign             = "design"
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
	DispatcherVersion   *string  `json:"dispatcher_version"`
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

// JournalLine is the producer envelope, distinct from portable EventRef tags.
// HasSeq is derived from Seq!=nil; explicit zero is present, absent/null is not.
// Negative/noninteger/overflowing seq is malformed. Timestamp is RFC3339Nano.
type JournalLine struct {
	Seq       *int            `json:"seq"`
	Hash      string          `json:"hash"`
	PrevHash  string          `json:"prev_hash"`
	EventType string          `json:"event_type"`
	TaskKey   string          `json:"task_key"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// Event is a parsed task-keyed event. Run metadata stays in ParsedJournal identity/diagnostics.
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
	ProducerConflict   bool     `json:"producer_conflict"`
	ProducerVersions   []string `json:"producer_versions"`
	Bytes              int64    `json:"bytes"`
	TotalBoundExceeded bool     `json:"total_bound_exceeded"`
	Lines              int      `json:"lines"`
	Events             int      `json:"events"`
	LinesUnparsed      int      `json:"lines_unparsed"`
	BadTimestamps      int      `json:"bad_timestamps"`
	LinesOverBound     int      `json:"lines_over_bound"`
	MissingProducer    bool     `json:"missing_producer"`
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
	// Wall.StartedAt must equal ID.StartedAt as an instant and Wall.Elapsed must
	// equal Elapsed. ReduceAttempts, Attempt JSON methods and JoinEvidence refuse
	// input mismatches with ErrEvidenceConflict. JoinEvidence may atomically rebase
	// both values when adopting a YAML terminal, under F2-YAML-TERMINAL-WALL; this
	// never repairs a preexisting mismatch or takes independent maxima.
	Wall WallBreakdown `json:"wall"`
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
	if !a.Outcome.Valid() {
		return false, fmt.Errorf("%w: Attempt.Censored has %s", ErrInvalidOutcome, a.Outcome)
	}
	return a.Outcome != OutcomeDone, nil
}

// MarshalJSON emits textual outcome done/blocked/unfinished in schema 4; invalid
// outcomes wrap ErrInvalidOutcome. All other fields retain their declared wire
// tags and Measured representation. FC-JOURNAL body; baseline Outcome is unchanged.
func (a Attempt) MarshalJSON() ([]byte, error) {
	if !a.Outcome.Valid() {
		return nil, fmt.Errorf("%w: Attempt.MarshalJSON has %s", ErrInvalidOutcome, a.Outcome)
	}
	a = canonicalAttempt(a)
	if err := validateAttemptWall(a); err != nil {
		return nil, err
	}
	type alias Attempt
	return json.Marshal(struct {
		alias
		Outcome string `json:"outcome"`
	}{alias: alias(a), Outcome: a.Outcome.String()})
}

// UnmarshalJSON rejects missing/unknown/non-text outcome with ErrInvalidOutcome;
// successful decoding never converts absent outcome to a plausible censored row.
// FC-JOURNAL body; see F4-OUTCOME-WIRE and F4-SCHEMA-ROUNDTRIP.
func (a *Attempt) UnmarshalJSON(data []byte) error {
	if a == nil {
		return fmt.Errorf("%w: Attempt.UnmarshalJSON on nil receiver", ErrUnattributable)
	}
	type alias Attempt
	var wire struct {
		alias
		Outcome json.RawMessage `json:"outcome"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var outcomeText string
	if len(wire.Outcome) == 0 || json.Unmarshal(wire.Outcome, &outcomeText) != nil {
		return fmt.Errorf("%w: Attempt.UnmarshalJSON outcome must be text", ErrInvalidOutcome)
	}
	var outcome Outcome
	switch outcomeText {
	case "done":
		outcome = OutcomeDone
	case "blocked":
		outcome = OutcomeBlocked
	case "unfinished":
		outcome = OutcomeUnfinished
	default:
		return fmt.Errorf("%w: Attempt.UnmarshalJSON has outcome %q", ErrInvalidOutcome, outcomeText)
	}
	decoded := canonicalAttempt(Attempt(wire.alias))
	decoded.Outcome = outcome
	if err := validateAttemptWall(decoded); err != nil {
		return err
	}
	*a = decoded
	return nil
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
	// Diagnostics contains parser diagnostics plus reducer-added facts. Build
	// uses only the additions to augment already-counted source diagnostics.
	// Never sum this total and ParsedJournal.Diagnostics for the same journal.
	Diagnostics JournalDiagnostics `json:"diagnostics"`
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

// JournalBounds is the standalone parser's complete byte-bound input. Zero
// uses the shared defaults; negative fields wrap ErrInvalidSourceSpec.
type JournalBounds struct {
	MaxLineBytes  int   `json:"max_line_bytes"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
}

// ParseEvents decodes one journal stream into a ParsedJournal, events in
// file order, bounded by bounds.MaxLineBytes (zero uses DefaultMaxLineBytes;
// negative returns ErrInvalidSourceSpec). It applies that line-bound rule
// itself, without depending on the later FC-SOURCES validation body. It also
// enforces MaxTotalBytes, retaining data and setting TotalBoundExceeded at a cap.
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
// FC-JOURNAL body. Parameters retain the frozen scaffold names.
func ParseEvents(ctx context.Context, journal JournalIdentity, reader io.Reader, bounds JournalBounds) (ParsedJournal, error) {
	result := ParsedJournal{
		Journal: journal,
		Events:  []Event{},
		Diagnostics: JournalDiagnostics{
			ProducerVersions: []string{},
		},
	}
	// Producer is evidence from the stream, never a trusted caller assertion.
	result.Journal.Producer = ""
	if bounds.MaxLineBytes < 0 || bounds.MaxTotalBytes < 0 {
		return result, fmt.Errorf("%w: ParseEvents has negative bounds", ErrInvalidSourceSpec)
	}
	if bounds.MaxLineBytes == 0 {
		bounds.MaxLineBytes = DefaultMaxLineBytes
	}
	if bounds.MaxTotalBytes == 0 {
		bounds.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if reader == nil {
		result.Diagnostics.MissingProducer = true
		return result, fmt.Errorf("%w: ParseEvents(%s) has nil reader", ErrJournalSource, journal.Path)
	}
	if err := ctx.Err(); err != nil {
		result.Diagnostics.MissingProducer = true
		return result, fmt.Errorf("%w: ParseEvents(%s): %w", ErrSourceCancelled, journal.Path, err)
	}

	capped := &journalCappedReader{reader: reader, limit: bounds.MaxTotalBytes}
	buffered := bufio.NewReaderSize(capped, 64*1024)
	candidates := make([]journalCandidate, 0)
	lineNumber := 0
	var line []byte
	var lineBytes int64
	overLineBound := false
	inLine := false
	var readErr error

	for {
		fragment, prefix, err := buffered.ReadLine()
		hasLine := len(fragment) != 0 || prefix || err == nil
		if hasLine {
			if !inLine {
				inLine = true
				lineNumber++
				result.Diagnostics.Lines++
			}
			if int64(len(fragment)) > math.MaxInt64-lineBytes {
				overLineBound = true
				lineBytes = math.MaxInt64
			} else {
				lineBytes += int64(len(fragment))
				if lineBytes > int64(bounds.MaxLineBytes) {
					overLineBound = true
				}
			}
			if !overLineBound {
				line = append(line, fragment...)
			}
		}

		if err != nil {
			if errors.Is(err, errJournalTotalBound) {
				result.Diagnostics.TotalBoundExceeded = true
				if inLine && overLineBound {
					result.Diagnostics.LinesOverBound++
				}
				break
			}
			if errors.Is(err, io.EOF) {
				break
			}
			readErr = err
			break
		}
		if prefix {
			if err := ctx.Err(); err != nil {
				readErr = fmt.Errorf("%w: ParseEvents(%s): %w", ErrSourceCancelled, journal.Path, err)
				break
			}
			continue
		}

		if overLineBound {
			result.Diagnostics.LinesOverBound++
		} else if len(bytes.TrimSpace(line)) != 0 {
			candidate, ok, badTimestamp := decodeJournalCandidate(line, lineNumber)
			switch {
			case badTimestamp:
				result.Diagnostics.BadTimestamps++
			case !ok:
				result.Diagnostics.LinesUnparsed++
			default:
				candidates = append(candidates, candidate)
			}
		}
		line = line[:0]
		lineBytes = 0
		overLineBound = false
		inLine = false

		if err := ctx.Err(); err != nil {
			readErr = fmt.Errorf("%w: ParseEvents(%s): %w", ErrSourceCancelled, journal.Path, err)
			break
		}
	}
	result.Diagnostics.Bytes = capped.read
	retained := resolveJournalCandidates(candidates, &result.Diagnostics)
	versions := make(map[string]struct{})
	for _, candidate := range retained {
		if candidate.event.Ref.Type == EventRunStarted {
			if candidate.event.Payload.DispatcherVersion != nil {
				versions[*candidate.event.Payload.DispatcherVersion] = struct{}{}
			}
			continue
		}
		if candidate.event.TaskKey == "" {
			continue
		}
		result.Events = append(result.Events, candidate.event)
	}
	for version := range versions {
		result.Diagnostics.ProducerVersions = append(result.Diagnostics.ProducerVersions, version)
	}
	sort.Strings(result.Diagnostics.ProducerVersions)
	result.Diagnostics.MissingProducer = len(result.Diagnostics.ProducerVersions) == 0
	result.Diagnostics.ProducerConflict = len(result.Diagnostics.ProducerVersions) > 1
	if len(result.Diagnostics.ProducerVersions) == 1 {
		result.Journal.Producer = result.Diagnostics.ProducerVersions[0]
	}
	for i := range result.Events {
		result.Events[i].Ref = canonicalEventRef(result.Events[i].Ref, result.Journal)
	}
	result.Diagnostics.Events = len(result.Events)

	if readErr != nil {
		if err := ctx.Err(); err != nil && !errors.Is(readErr, ErrSourceCancelled) {
			return result, fmt.Errorf("%w: ParseEvents(%s): %w", ErrSourceCancelled, journal.Path, err)
		}
		if errors.Is(readErr, ErrSourceCancelled) {
			return result, readErr
		}
		if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
			return result, fmt.Errorf("%w: ParseEvents(%s): %w", ErrSourceCancelled, journal.Path, readErr)
		}
		return result, fmt.Errorf("%w: ParseEvents(%s): %v", ErrJournalSource, journal.Path, readErr)
	}
	return result, nil
}

// ReduceAttempts reduces one journal at a required nonzero UTC cutoff. It returns
// retained valid attempts/diagnostics alongside ErrMeasurementOverflow or
// ErrReversedInterval, and ErrInvalidSelection for an invalid cutoff.
// Authoritative behavior: notes/FC-SCAFFOLD.md, F1/F2 tables and the "Entry-point
// contracts" section. FC-JOURNAL owns this and the Attempt JSON/Censored seams.
func ReduceAttempts(parsed ParsedJournal, cutoff time.Time) (AttemptSet, error) {
	set := AttemptSet{
		Journal:     parsed.Journal,
		Attempts:    []Attempt{},
		Ambiguous:   []AmbiguousAttempt{},
		Conflicts:   []AttemptConflict{},
		Diagnostics: canonicalJournalDiagnostics(parsed.Diagnostics),
	}
	if cutoff.IsZero() {
		return set, fmt.Errorf("%w: ReduceAttempts requires a cutoff", ErrInvalidSelection)
	}
	cutoff = canonicalTime(cutoff)
	if set.Diagnostics.ProducerConflict {
		return set, fmt.Errorf("%w: ReduceAttempts(%s) has conflicting producers", ErrEvidenceConflict, parsed.Journal.Path)
	}
	if parsed.Journal.Producer == "" {
		return set, fmt.Errorf("%w: ReduceAttempts(%s) has no producer", ErrJournalSource, parsed.Journal.Path)
	}
	if parsed.Journal.Producer != ProducerDispatcherV0_1_0 {
		return set, fmt.Errorf("%w: ReduceAttempts(%s) has unsupported producer %q", ErrJournalSource, parsed.Journal.Path, parsed.Journal.Producer)
	}

	events := validateReducerEvents(parsed.Events, parsed.Journal, &set.Diagnostics)
	sort.Slice(events, func(i, j int) bool { return physicalEventLess(events[i], events[j]) })
	type activeAttempt struct {
		attempt  *journalAttemptBuilder
		started  bool
		excluded bool
	}
	active := make(map[string]activeAttempt)
	builders := make([]*journalAttemptBuilder, 0)
	for _, event := range events {
		if event.Ref.Type == EventTaskStarted {
			if event.Ref.At.After(cutoff) {
				set.StartsAfterCutoff++
				active[event.TaskKey] = activeAttempt{started: true, excluded: true}
				continue
			}
			builder := &journalAttemptBuilder{
				id:     NewAttemptID(parsed.Journal.RunID, event.TaskKey, event.Ref.At),
				start:  event.Ref,
				events: []Event{},
			}
			builders = append(builders, builder)
			active[event.TaskKey] = activeAttempt{attempt: builder, started: true}
			continue
		}
		if event.Ref.At.After(cutoff) {
			continue
		}
		owner, ok := active[event.TaskKey]
		if !ok || !owner.started {
			set.LeadingEvents++
			continue
		}
		if owner.excluded || owner.attempt == nil {
			continue
		}
		owner.attempt.events = append(owner.attempt.events, event)
	}

	byID := make(map[AttemptID][]*journalAttemptBuilder)
	for _, builder := range builders {
		byID[builder.id] = append(byID[builder.id], builder)
	}
	var reduceErrors []error
	for id, sameID := range byID {
		if len(sameID) > 1 {
			refs := make([]EventRef, 0, len(sameID))
			for _, builder := range sameID {
				refs = append(refs, builder.start)
			}
			sortEventRefs(refs)
			set.Ambiguous = append(set.Ambiguous, AmbiguousAttempt{ID: id, Starts: len(sameID), Refs: refs})
			continue
		}
		attempt, conflict, err := reduceJournalAttempt(sameID[0], cutoff)
		if conflict != nil {
			set.Conflicts = append(set.Conflicts, *conflict)
			continue
		}
		if err != nil {
			reduceErrors = append(reduceErrors, err)
			if !errors.Is(err, ErrMeasurementOverflow) || !attempt.Outcome.Valid() || errors.Is(err, ErrReversedInterval) {
				continue
			}
		}
		if wallErr := validateAttemptWall(attempt); wallErr != nil {
			reduceErrors = append(reduceErrors, wallErr)
			continue
		}
		set.Attempts = append(set.Attempts, attempt)
	}
	sort.Slice(set.Attempts, func(i, j int) bool { return attemptIDLess(set.Attempts[i].ID, set.Attempts[j].ID) })
	sort.Slice(set.Ambiguous, func(i, j int) bool { return attemptIDLess(set.Ambiguous[i].ID, set.Ambiguous[j].ID) })
	sort.Slice(set.Conflicts, func(i, j int) bool { return attemptConflictLess(set.Conflicts[i], set.Conflicts[j]) })
	return set, errors.Join(reduceErrors...)
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
// FC-JOURNAL body; authoritative behavior is in the handoff F2 rows.
func SummarizeWall(w WallBreakdown) (WallSummary, error) {
	if w.StartedAt.IsZero() {
		return WallSummary{}, fmt.Errorf("%w: SummarizeWall has no start", ErrUnattributable)
	}
	if w.Elapsed < 0 {
		return WallSummary{}, fmt.Errorf("%w: SummarizeWall elapsed %s", ErrNegativeValue, w.Elapsed)
	}
	end := w.StartedAt.Add(w.Elapsed)
	var previous *Interval
	summary := WallSummary{Complete: w.Complete}
	var classified time.Duration
	for i := range w.Intervals {
		interval := w.Intervals[i]
		switch interval.Phase {
		case PhaseDevelopment, PhasePanelReview, PhaseVerifier:
		default:
			return WallSummary{}, fmt.Errorf("%w: SummarizeWall phase %q", ErrInvalidPhase, interval.Phase)
		}
		if interval.End.Before(interval.Start) {
			return WallSummary{}, fmt.Errorf("%w: SummarizeWall interval %d", ErrReversedInterval, i)
		}
		if interval.Start.Before(w.StartedAt) || interval.End.After(end) {
			return WallSummary{}, fmt.Errorf("%w: SummarizeWall interval %d outside attempt", ErrOverlappingIntervals, i)
		}
		for j := 1; j < len(interval.Evidence); j++ {
			if eventRefLess(interval.Evidence[j], interval.Evidence[j-1]) {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall interval %d citations", ErrNonCanonicalEvidence, i)
			}
		}
		if previous != nil {
			if intervalLess(interval, *previous) {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall interval %d", ErrNonCanonicalEvidence, i)
			}
			if intervalsOverlapNonempty(*previous, interval) {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall intervals %d and %d", ErrOverlappingIntervals, i-1, i)
			}
		}
		duration := interval.End.Sub(interval.Start)
		if duration > time.Duration(math.MaxInt64)-classified {
			return WallSummary{}, fmt.Errorf("%w: SummarizeWall classified duration", ErrMeasurementOverflow)
		}
		classified += duration
		switch interval.Phase {
		case PhaseDevelopment:
			if duration > time.Duration(math.MaxInt64)-summary.Development {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall development", ErrMeasurementOverflow)
			}
			summary.Development += duration
		case PhasePanelReview:
			if duration > time.Duration(math.MaxInt64)-summary.PanelReview {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall panel review", ErrMeasurementOverflow)
			}
			summary.PanelReview += duration
		case PhaseVerifier:
			if duration > time.Duration(math.MaxInt64)-summary.Verifier {
				return WallSummary{}, fmt.Errorf("%w: SummarizeWall verifier", ErrMeasurementOverflow)
			}
			summary.Verifier += duration
		}
		previous = &w.Intervals[i]
	}
	if classified > w.Elapsed {
		return WallSummary{}, fmt.Errorf("%w: SummarizeWall classified duration exceeds elapsed", ErrOverlappingIntervals)
	}
	summary.Unclassified = w.Elapsed - classified
	return summary, nil
}

var errJournalTotalBound = errors.New("journal total-byte bound reached")

// journalCappedReader exposes at most limit bytes and performs one extra-byte
// probe to distinguish an exact-bound EOF from truncation. The probe is counted
// but never returned to the parser.
type journalCappedReader struct {
	reader   io.Reader
	limit    int64
	read     int64
	exceeded bool
}

func (r *journalCappedReader) Read(p []byte) (int, error) {
	if r.exceeded {
		return 0, errJournalTotalBound
	}
	if r.read < r.limit {
		remaining := r.limit - r.read
		if int64(len(p)) > remaining {
			p = p[:int(remaining)]
		}
		n, err := r.reader.Read(p)
		r.read += int64(n)
		return n, err
	}
	var probe [1]byte
	n, err := r.reader.Read(probe[:])
	if n > 0 {
		r.read++
		r.exceeded = true
		return 0, errJournalTotalBound
	}
	return 0, err
}

type journalCandidate struct {
	event Event
}

func decodeJournalCandidate(line []byte, lineNumber int) (journalCandidate, bool, bool) {
	var wire JournalLine
	if err := json.Unmarshal(line, &wire); err != nil {
		return journalCandidate{}, false, false
	}
	if wire.Seq != nil && *wire.Seq < 0 {
		return journalCandidate{}, false, false
	}
	at, err := time.Parse(time.RFC3339Nano, wire.Timestamp)
	if err != nil {
		return journalCandidate{}, false, true
	}
	payload := EventPayload{}
	trimmedPayload := bytes.TrimSpace(wire.Payload)
	if len(trimmedPayload) != 0 && !bytes.Equal(trimmedPayload, []byte("null")) {
		if err := json.Unmarshal(trimmedPayload, &payload); err != nil {
			return journalCandidate{}, false, false
		}
	}
	if !validEventPayload(wire.EventType, payload) {
		return journalCandidate{}, false, false
	}
	if wire.EventType == "" {
		return journalCandidate{}, false, false
	}
	ref := EventRef{
		HasSeq:   wire.Seq != nil,
		Hash:     wire.Hash,
		PrevHash: wire.PrevHash,
		Line:     lineNumber,
		Type:     wire.EventType,
		At:       canonicalTime(at),
	}
	if wire.Seq != nil {
		ref.Seq = *wire.Seq
	}
	return journalCandidate{event: Event{Ref: ref, TaskKey: wire.TaskKey, Payload: payload}}, true, false
}

func validEventPayload(eventType string, payload EventPayload) bool {
	if payload.Iteration != nil && *payload.Iteration < 0 {
		return false
	}
	if payload.IterationsRemaining != nil && *payload.IterationsRemaining < 0 {
		return false
	}
	if payload.InputTokens != nil && *payload.InputTokens < 0 {
		return false
	}
	if payload.OutputTokens != nil && *payload.OutputTokens < 0 {
		return false
	}
	if payload.CostUSD != nil && (*payload.CostUSD < 0 || math.IsNaN(*payload.CostUSD) || math.IsInf(*payload.CostUSD, 0)) {
		return false
	}
	if payload.DurationMillis != nil {
		if *payload.DurationMillis < 0 || *payload.DurationMillis > math.MaxInt64/int64(time.Millisecond) {
			return false
		}
	}
	if eventType == EventRunStarted && payload.DispatcherVersion != nil {
		version := *payload.DispatcherVersion
		if version == "" || strings.TrimSpace(version) != version {
			return false
		}
	}
	if eventType == EventPanelStarted {
		invocation := payload.ForcedBy == "" && payload.Iteration != nil && payload.IterationsRemaining != nil
		gate := payload.ForcedBy == ForcedByPathClassification && payload.Iteration == nil && payload.IterationsRemaining == nil
		if !invocation && !gate {
			return false
		}
	}
	return true
}

func resolveJournalCandidates(candidates []journalCandidate, diagnostics *JournalDiagnostics) []journalCandidate {
	groups := make(map[int][]journalCandidate)
	retained := make([]journalCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.event.Ref.HasSeq {
			retained = append(retained, candidate)
			continue
		}
		groups[candidate.event.Ref.Seq] = append(groups[candidate.event.Ref.Seq], candidate)
	}
	for _, group := range groups {
		compatible := true
		for i := 1; i < len(group); i++ {
			if !equivalentJournalEvents(group[0].event, group[i].event) {
				compatible = false
				break
			}
		}
		if !compatible {
			diagnostics.LinesUnparsed += len(group)
			continue
		}
		best := group[0]
		for _, candidate := range group[1:] {
			if physicalEventLess(candidate.event, best.event) {
				best = candidate
			}
		}
		retained = append(retained, best)
	}
	sort.Slice(retained, func(i, j int) bool { return physicalEventLess(retained[i].event, retained[j].event) })
	return retained
}

func equivalentJournalEvents(a, b Event) bool {
	return a.TaskKey == b.TaskKey && a.Ref.Type == b.Ref.Type && a.Ref.At.Equal(b.Ref.At) && reflect.DeepEqual(a.Payload, b.Payload)
}

func validateReducerEvents(input []Event, journal JournalIdentity, diagnostics *JournalDiagnostics) []Event {
	candidates := make([]journalCandidate, 0, len(input))
	for _, event := range input {
		if event.TaskKey == "" || event.Ref.Type == "" || event.Ref.At.IsZero() {
			if event.Ref.At.IsZero() {
				diagnostics.BadTimestamps++
			} else {
				diagnostics.LinesUnparsed++
			}
			continue
		}
		if event.Ref.HasSeq && event.Ref.Seq < 0 || !validEventPayload(event.Ref.Type, event.Payload) {
			diagnostics.LinesUnparsed++
			continue
		}
		event.Ref = canonicalEventRef(event.Ref, journal)
		candidates = append(candidates, journalCandidate{event: event})
	}
	retainedCandidates := resolveJournalCandidates(candidates, diagnostics)
	retained := make([]Event, 0, len(retainedCandidates))
	for _, candidate := range retainedCandidates {
		retained = append(retained, candidate.event)
	}
	diagnostics.Events = len(retained)
	return retained
}

type journalAttemptBuilder struct {
	id     AttemptID
	start  EventRef
	events []Event
}

type intervalCandidate struct {
	interval Interval
	invalid  bool
}

func reduceJournalAttempt(builder *journalAttemptBuilder, cutoff time.Time) (Attempt, *AttemptConflict, error) {
	attempt := Attempt{
		ID:                 builder.id,
		Start:              builder.start,
		Model:              Unknown[string](),
		CascadeEvents:      []EventRef{},
		Outcome:            OutcomeUnfinished,
		Cutoff:             cutoff,
		CorrectionEvents:   []EventRef{},
		ReviewEvents:       []EventRef{},
		VerificationEvents: []EventRef{},
		CostScope:          CostScopeRecordedSpawns,
		InputTokens:        Unknown[int64](),
		OutputTokens:       Unknown[int64](),
		CostUSD:            Unknown[float64](),
		CostEvents:         []EventRef{},
		InputTokenEvents:   []EventRef{},
		OutputTokenEvents:  []EventRef{},
		Evidence: ObservationEvidence{
			Start: FieldEvidence{Source: EvidenceJournal, Event: builder.start},
		},
	}

	terminals := make([]Event, 0, 1)
	for _, event := range builder.events {
		if event.Ref.Type == EventTaskDone || event.Ref.Type == EventTaskBlocked {
			terminals = append(terminals, event)
		}
	}
	if len(terminals) != 0 {
		firstOutcome := terminalOutcome(terminals[0].Ref.Type)
		firstAt := terminals[0].Ref.At
		for _, event := range terminals[1:] {
			if terminalOutcome(event.Ref.Type) != firstOutcome || !event.Ref.At.Equal(firstAt) {
				conflict := terminalConflict(builder.id, terminals[0], event, builder.id.StartedAt)
				return Attempt{}, &conflict, nil
			}
		}
		sort.Slice(terminals, func(i, j int) bool { return eventRefLess(terminals[i].Ref, terminals[j].Ref) })
		attempt.Outcome = firstOutcome
		attempt.TerminalAt = canonicalTime(firstAt)
		attempt.Evidence.Terminal = journalEvidence(terminals[0].Ref)
		attempt.Evidence.Elapsed = journalEvidence(terminals[0].Ref)
	}

	end := cutoff
	if attempt.Outcome.terminal() {
		end = attempt.TerminalAt
	}
	if end.Before(builder.id.StartedAt) {
		return Attempt{}, nil, fmt.Errorf("%w: attempt %s/%s", ErrReversedInterval, builder.id.RunID, builder.id.Key)
	}
	attempt.Elapsed = end.Sub(builder.id.StartedAt)
	if !builder.id.StartedAt.Add(attempt.Elapsed).Equal(end) {
		return Attempt{}, nil, fmt.Errorf("%w: attempt %s/%s elapsed", ErrMeasurementOverflow, builder.id.RunID, builder.id.Key)
	}

	intervals := make([]intervalCandidate, 0)
	wallComplete := true
	sawPhaseEvidence := false
	panelOpens := make([]Event, 0, 1)
	verificationOpens := make([]Event, 0, 1)
	verifierSpawns := 0
	verifierIntervals := 0
	panelIterateSpawns := 0
	panelIterateMarkers := 0
	verifierIterateSpawns := 0
	verifierIterateMarkers := 0
	spawnCount := 0
	allInputKnown, allOutputKnown, allCostKnown := true, true, true
	var inputTotal, outputTotal int64
	var costTotal float64
	var aggregateErrors []error

	for _, event := range builder.events {
		switch event.Ref.Type {
		case EventTaskSpawnFinished:
			spawnCount++
			if event.Payload.InputTokens == nil {
				allInputKnown = false
			} else {
				attempt.InputTokenEvents = append(attempt.InputTokenEvents, event.Ref)
				if next, ok := checkedAddInt64(inputTotal, *event.Payload.InputTokens); ok {
					inputTotal = next
				} else {
					allInputKnown = false
					aggregateErrors = append(aggregateErrors, fmt.Errorf("%w: attempt %s/%s input tokens", ErrMeasurementOverflow, builder.id.RunID, builder.id.Key))
				}
			}
			if event.Payload.OutputTokens == nil {
				allOutputKnown = false
			} else {
				attempt.OutputTokenEvents = append(attempt.OutputTokenEvents, event.Ref)
				if next, ok := checkedAddInt64(outputTotal, *event.Payload.OutputTokens); ok {
					outputTotal = next
				} else {
					allOutputKnown = false
					aggregateErrors = append(aggregateErrors, fmt.Errorf("%w: attempt %s/%s output tokens", ErrMeasurementOverflow, builder.id.RunID, builder.id.Key))
				}
			}
			if event.Payload.CostUSD == nil {
				allCostKnown = false
			} else {
				attempt.CostEvents = append(attempt.CostEvents, event.Ref)
				next := costTotal + *event.Payload.CostUSD
				if math.IsInf(next, 0) || math.IsNaN(next) {
					allCostKnown = false
					aggregateErrors = append(aggregateErrors, fmt.Errorf("%w: attempt %s/%s cost", ErrMeasurementOverflow, builder.id.RunID, builder.id.Key))
				} else {
					costTotal = next
				}
			}

			if implementingSpawnKind(event.Payload.SpawnKind) && event.Payload.Model != "" {
				attempt.Model = Known(event.Payload.Model)
				attempt.Evidence.Model = journalEvidence(event.Ref)
			}
			switch event.Payload.SpawnKind {
			case SpawnKindImplementer, SpawnKindPanelIterate, SpawnKindVerifierIterate,
				SpawnKindTestFixRetry, SpawnKindCommitRetry, SpawnKindPushRetry, SpawnKindSummaryRecovery:
				sawPhaseEvidence = true
				if event.Payload.DurationMillis == nil {
					wallComplete = false
				} else {
					duration := time.Duration(*event.Payload.DurationMillis) * time.Millisecond
					intervals = append(intervals, intervalCandidate{interval: newInterval(PhaseDevelopment, event.Ref.At.Add(-duration), event.Ref.At, event.Ref)})
				}
			case SpawnKindDesign:
				// Design is accounted spend but has no implementing phase bucket.
				wallComplete = false
			case SpawnKindVerifier:
				// Verifier wall is bounded by verification_started/verdict below.
				verifierSpawns++
			case "":
				wallComplete = false
			default:
				wallComplete = false
			}
			if correctionSpawnKind(event.Payload.SpawnKind) {
				attempt.CorrectionEvents = append(attempt.CorrectionEvents, event.Ref)
			}
			if event.Payload.SpawnKind == SpawnKindPanelIterate {
				panelIterateSpawns++
			}
			if event.Payload.SpawnKind == SpawnKindVerifierIterate {
				verifierIterateSpawns++
			}

		case EventPanelStarted:
			// Parser validation guarantees that retained starts are either a gate
			// record or an invocation. Gates deliberately open no interval.
			if panelInvocation(event.Payload) {
				sawPhaseEvidence = true
				attempt.ReviewEvents = append(attempt.ReviewEvents, event.Ref)
				panelOpens = append(panelOpens, event)
			}
		case EventPanelVerdict:
			sawPhaseEvidence = true
			if len(panelOpens) == 1 {
				if event.Ref.At.Before(panelOpens[0].Ref.At) {
					aggregateErrors = append(aggregateErrors, fmt.Errorf("%w: attempt %s/%s panel interval", ErrReversedInterval, builder.id.RunID, builder.id.Key))
					wallComplete = false
				} else {
					intervals = append(intervals, intervalCandidate{interval: newInterval(PhasePanelReview, panelOpens[0].Ref.At, event.Ref.At, panelOpens[0].Ref, event.Ref)})
				}
			} else {
				wallComplete = false
			}
			panelOpens = panelOpens[:0]
		case EventVerificationStarted:
			sawPhaseEvidence = true
			attempt.VerificationEvents = append(attempt.VerificationEvents, event.Ref)
			verificationOpens = append(verificationOpens, event)
		case EventVerificationVerdict:
			sawPhaseEvidence = true
			if len(verificationOpens) == 1 {
				if event.Ref.At.Before(verificationOpens[0].Ref.At) {
					aggregateErrors = append(aggregateErrors, fmt.Errorf("%w: attempt %s/%s verifier interval", ErrReversedInterval, builder.id.RunID, builder.id.Key))
					wallComplete = false
				} else {
					intervals = append(intervals, intervalCandidate{interval: newInterval(PhaseVerifier, verificationOpens[0].Ref.At, event.Ref.At, verificationOpens[0].Ref, event.Ref)})
					verifierIntervals++
				}
			} else {
				wallComplete = false
			}
			verificationOpens = verificationOpens[:0]
		case EventPanelIterate:
			attempt.CorrectionEvents = append(attempt.CorrectionEvents, event.Ref)
			panelIterateMarkers++
		case EventVerificationIterate:
			attempt.CorrectionEvents = append(attempt.CorrectionEvents, event.Ref)
			verifierIterateMarkers++
		case EventAgentFallback:
			attempt.CascadeEvents = append(attempt.CascadeEvents, event.Ref)
		}
	}
	if len(panelOpens) != 0 || len(verificationOpens) != 0 || !sawPhaseEvidence ||
		verifierSpawns != verifierIntervals || panelIterateMarkers > panelIterateSpawns || verifierIterateMarkers > verifierIterateSpawns {
		wallComplete = false
	}

	sortEventRefs(attempt.CascadeEvents)
	sortEventRefs(attempt.CorrectionEvents)
	sortEventRefs(attempt.ReviewEvents)
	sortEventRefs(attempt.VerificationEvents)
	sortEventRefs(attempt.CostEvents)
	sortEventRefs(attempt.InputTokenEvents)
	sortEventRefs(attempt.OutputTokenEvents)
	attempt.Cascades = len(attempt.CascadeEvents)
	attempt.Corrections = len(attempt.CorrectionEvents)
	attempt.Reviews = len(attempt.ReviewEvents)
	attempt.Verifications = len(attempt.VerificationEvents)
	if attempt.Cascades != 0 {
		attempt.Evidence.Cascades = journalEvidence(attempt.CascadeEvents[0])
	}
	if attempt.Corrections != 0 {
		attempt.Evidence.Corrections = journalEvidence(attempt.CorrectionEvents[0])
	}
	if attempt.Reviews != 0 {
		attempt.Evidence.Reviews = journalEvidence(attempt.ReviewEvents[0])
	}
	if attempt.Verifications != 0 {
		attempt.Evidence.Verifications = journalEvidence(attempt.VerificationEvents[0])
	}
	if spawnCount != 0 && allInputKnown {
		attempt.InputTokens = Known(inputTotal)
		attempt.Evidence.InputTokens = journalEvidence(attempt.InputTokenEvents[0])
	}
	if spawnCount != 0 && allOutputKnown {
		attempt.OutputTokens = Known(outputTotal)
		attempt.Evidence.OutputTokens = journalEvidence(attempt.OutputTokenEvents[0])
	}
	if spawnCount != 0 && allCostKnown {
		attempt.CostUSD = Known(costTotal)
		attempt.Evidence.Cost = journalEvidence(attempt.CostEvents[0])
	}

	attempt.Wall = buildWall(builder.id.StartedAt, attempt.Elapsed, intervals, wallComplete)
	if len(attempt.Wall.Intervals) != 0 {
		refs := make([]EventRef, 0)
		for _, interval := range attempt.Wall.Intervals {
			refs = append(refs, interval.Evidence...)
		}
		sortEventRefs(refs)
		if len(refs) != 0 {
			attempt.Evidence.Wall = journalEvidence(refs[0])
		}
	}
	if _, err := SummarizeWall(attempt.Wall); err != nil {
		aggregateErrors = append(aggregateErrors, err)
	}
	return canonicalAttempt(attempt), nil, errors.Join(aggregateErrors...)
}

func buildWall(start time.Time, elapsed time.Duration, candidates []intervalCandidate, complete bool) WallBreakdown {
	end := start.Add(elapsed)
	for i := range candidates {
		interval := candidates[i].interval
		if interval.End.Before(interval.Start) || interval.Start.Before(start) || interval.End.After(end) {
			candidates[i].invalid = true
			complete = false
		}
	}
	for i := range candidates {
		for j := i + 1; j < len(candidates); j++ {
			if intervalsOverlapNonempty(candidates[i].interval, candidates[j].interval) {
				candidates[i].invalid = true
				candidates[j].invalid = true
				complete = false
			}
		}
	}
	intervals := make([]Interval, 0, len(candidates))
	for _, candidate := range candidates {
		if !candidate.invalid {
			intervals = append(intervals, canonicalInterval(candidate.interval))
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervalLess(intervals[i], intervals[j]) })
	return WallBreakdown{StartedAt: canonicalTime(start), Elapsed: elapsed, Intervals: intervals, Complete: complete}
}

func newInterval(phase Phase, start, end time.Time, evidence ...EventRef) Interval {
	sortEventRefs(evidence)
	return Interval{Phase: phase, Start: canonicalTime(start), End: canonicalTime(end), Evidence: evidence}
}

func terminalOutcome(eventType string) Outcome {
	if eventType == EventTaskBlocked {
		return OutcomeBlocked
	}
	return OutcomeDone
}

func terminalConflict(id AttemptID, a, b Event, start time.Time) AttemptConflict {
	if eventRefLess(b.Ref, a.Ref) {
		a, b = b, a
	}
	value := func(event Event) ConflictValue {
		payload := struct {
			Outcome    string        `json:"outcome"`
			TerminalAt time.Time     `json:"terminal_at"`
			Elapsed    time.Duration `json:"elapsed_ns"`
		}{
			Outcome:    terminalOutcome(event.Ref.Type).String(),
			TerminalAt: canonicalTime(event.Ref.At),
			Elapsed:    event.Ref.At.Sub(start),
		}
		encoded, _ := json.Marshal(payload)
		return ConflictValue{Kind: "terminal", Value: encoded}
	}
	return AttemptConflict{
		Code:   EvidenceConflictCode,
		AValue: value(a),
		BValue: value(b),
		ID:     id,
		Field:  "terminal",
		A:      journalEvidence(a.Ref),
		B:      journalEvidence(b.Ref),
		Err:    fmt.Errorf("%w: attempt %s/%s terminal", ErrEvidenceConflict, id.RunID, id.Key),
		Reason: "journal terminal events disagree",
	}
}

func implementingSpawnKind(kind string) bool {
	switch kind {
	case SpawnKindImplementer, SpawnKindPanelIterate, SpawnKindVerifierIterate,
		SpawnKindTestFixRetry, SpawnKindCommitRetry, SpawnKindPushRetry, SpawnKindSummaryRecovery:
		return true
	}
	return false
}

func correctionSpawnKind(kind string) bool {
	switch kind {
	case SpawnKindTestFixRetry, SpawnKindCommitRetry, SpawnKindPushRetry, SpawnKindSummaryRecovery:
		return true
	}
	return false
}

func panelInvocation(payload EventPayload) bool {
	return payload.ForcedBy == "" && payload.Iteration != nil && payload.IterationsRemaining != nil
}

func checkedAddInt64(a, b int64) (int64, bool) {
	if b < 0 || a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}

func validateAttemptWall(a Attempt) error {
	if !a.Wall.StartedAt.Equal(a.ID.StartedAt) || a.Wall.Elapsed != a.Elapsed {
		return fmt.Errorf("%w: attempt %s/%s wall disagrees with parent", ErrEvidenceConflict, a.ID.RunID, a.ID.Key)
	}
	return nil
}

func canonicalAttempt(a Attempt) Attempt {
	a.ID.StartedAt = canonicalTime(a.ID.StartedAt)
	a.Start = canonicalEventRef(a.Start, a.Start.Journal)
	a.TerminalAt = canonicalTime(a.TerminalAt)
	a.Cutoff = canonicalTime(a.Cutoff)
	a.Wall.StartedAt = canonicalTime(a.Wall.StartedAt)
	if a.Wall.Intervals == nil {
		a.Wall.Intervals = []Interval{}
	} else {
		a.Wall.Intervals = append([]Interval{}, a.Wall.Intervals...)
	}
	for i := range a.Wall.Intervals {
		a.Wall.Intervals[i] = canonicalInterval(a.Wall.Intervals[i])
	}
	canonicalizeRefList := func(refs *[]EventRef) {
		if *refs == nil {
			*refs = []EventRef{}
		} else {
			*refs = append([]EventRef{}, (*refs)...)
		}
		for i := range *refs {
			(*refs)[i] = canonicalEventRef((*refs)[i], (*refs)[i].Journal)
		}
	}
	canonicalizeRefList(&a.CascadeEvents)
	canonicalizeRefList(&a.CorrectionEvents)
	canonicalizeRefList(&a.ReviewEvents)
	canonicalizeRefList(&a.VerificationEvents)
	canonicalizeRefList(&a.CostEvents)
	canonicalizeRefList(&a.InputTokenEvents)
	canonicalizeRefList(&a.OutputTokenEvents)
	a.Evidence = canonicalObservationEvidence(a.Evidence)
	return a
}

func canonicalObservationEvidence(e ObservationEvidence) ObservationEvidence {
	fields := []*FieldEvidence{
		&e.Role, &e.Model, &e.Start, &e.Terminal, &e.Elapsed, &e.Wall,
		&e.Corrections, &e.Cascades, &e.Reviews, &e.Verifications,
		&e.InputTokens, &e.OutputTokens, &e.Cost,
	}
	for _, field := range fields {
		field.Event = canonicalEventRef(field.Event, field.Event.Journal)
		field.Reading.RecordedAt = canonicalTime(field.Reading.RecordedAt)
	}
	return e
}

func canonicalInterval(interval Interval) Interval {
	interval.Start = canonicalTime(interval.Start)
	interval.End = canonicalTime(interval.End)
	if interval.Evidence == nil {
		interval.Evidence = []EventRef{}
	} else {
		interval.Evidence = append([]EventRef{}, interval.Evidence...)
	}
	for i := range interval.Evidence {
		interval.Evidence[i] = canonicalEventRef(interval.Evidence[i], interval.Evidence[i].Journal)
	}
	return interval
}

func canonicalEventRef(ref EventRef, journal JournalIdentity) EventRef {
	ref.Journal = journal
	ref.At = canonicalTime(ref.At)
	if !ref.HasSeq {
		ref.Seq = 0
	}
	return ref
}

func canonicalTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return value.Round(0).UTC()
}

func canonicalJournalDiagnostics(d JournalDiagnostics) JournalDiagnostics {
	if d.ProducerVersions == nil {
		d.ProducerVersions = []string{}
	} else {
		d.ProducerVersions = append([]string{}, d.ProducerVersions...)
		sort.Strings(d.ProducerVersions)
		unique := d.ProducerVersions[:0]
		for _, version := range d.ProducerVersions {
			if len(unique) == 0 || unique[len(unique)-1] != version {
				unique = append(unique, version)
			}
		}
		d.ProducerVersions = unique
	}
	d.ProducerConflict = d.ProducerConflict || len(d.ProducerVersions) > 1
	return d
}

func journalEvidence(ref EventRef) FieldEvidence {
	return FieldEvidence{Source: EvidenceJournal, Event: ref}
}

func sortEventRefs(refs []EventRef) {
	sort.Slice(refs, func(i, j int) bool { return eventRefLess(refs[i], refs[j]) })
}

func eventRefLess(a, b EventRef) bool {
	if c := strings.Compare(a.Journal.RunID, b.Journal.RunID); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a.Journal.SourceID, b.Journal.SourceID); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a.Journal.Path, b.Journal.Path); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a.Journal.Producer, b.Journal.Producer); c != 0 {
		return c < 0
	}
	if a.HasSeq != b.HasSeq {
		return !a.HasSeq
	}
	if a.Seq != b.Seq {
		return a.Seq < b.Seq
	}
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	if c := strings.Compare(a.Type, b.Type); c != 0 {
		return c < 0
	}
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	if c := strings.Compare(a.Hash, b.Hash); c != 0 {
		return c < 0
	}
	return a.PrevHash < b.PrevHash
}

func physicalEventLess(a, b Event) bool {
	if a.Ref.Line != b.Ref.Line {
		return a.Ref.Line < b.Ref.Line
	}
	if a.Ref.HasSeq != b.Ref.HasSeq {
		return !a.Ref.HasSeq
	}
	if a.Ref.Seq != b.Ref.Seq {
		return a.Ref.Seq < b.Ref.Seq
	}
	if c := strings.Compare(a.TaskKey, b.TaskKey); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a.Ref.Type, b.Ref.Type); c != 0 {
		return c < 0
	}
	if !a.Ref.At.Equal(b.Ref.At) {
		return a.Ref.At.Before(b.Ref.At)
	}
	encodedA, _ := json.Marshal(a.Payload)
	encodedB, _ := json.Marshal(b.Payload)
	if c := bytes.Compare(encodedA, encodedB); c != 0 {
		return c < 0
	}
	return eventRefLess(a.Ref, b.Ref)
}

func intervalLess(a, b Interval) bool {
	if !a.Start.Equal(b.Start) {
		return a.Start.Before(b.Start)
	}
	if !a.End.Equal(b.End) {
		return a.End.Before(b.End)
	}
	if c := strings.Compare(string(a.Phase), string(b.Phase)); c != 0 {
		return c < 0
	}
	if a.Inferred != b.Inferred {
		return !a.Inferred
	}
	for i := 0; i < len(a.Evidence) && i < len(b.Evidence); i++ {
		if eventRefLess(a.Evidence[i], b.Evidence[i]) {
			return true
		}
		if eventRefLess(b.Evidence[i], a.Evidence[i]) {
			return false
		}
	}
	return len(a.Evidence) < len(b.Evidence)
}

func intervalsOverlapNonempty(a, b Interval) bool {
	if a.Start.Equal(a.End) || b.Start.Equal(b.End) {
		return false
	}
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

func attemptIDLess(a, b AttemptID) bool {
	if c := strings.Compare(a.RunID, b.RunID); c != 0 {
		return c < 0
	}
	if c := strings.Compare(a.Key, b.Key); c != 0 {
		return c < 0
	}
	return a.StartedAt.Before(b.StartedAt)
}

func attemptConflictLess(a, b AttemptConflict) bool {
	if attemptIDLess(a.ID, b.ID) {
		return true
	}
	if attemptIDLess(b.ID, a.ID) {
		return false
	}
	if a.Field != b.Field {
		return a.Field < b.Field
	}
	if eventRefLess(a.A.Event, b.A.Event) {
		return true
	}
	if eventRefLess(b.A.Event, a.A.Event) {
		return false
	}
	if a.AValue.Kind != b.AValue.Kind {
		return a.AValue.Kind < b.AValue.Kind
	}
	return bytes.Compare(a.AValue.Value, b.AValue.Value) < 0
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
