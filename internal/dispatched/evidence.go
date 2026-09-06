package dispatched

// evidence.go: joining journal attempts with tasks-YAML readings.
//
// JoinEvidence is the schema-4 reconciliation path. The legacy helpers at the
// end of this file remain only for schema-3 Build callers.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
	// DispositionNoMatchingRun: no journal attempt has this run AND key. Missing
	// run and existing-run/missing-key cases share this bucket; unmatched YAML
	// never creates a LostAttempt or enlarges the journal denominator.
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
	// other than attribution (negative normalized measurement or invalid outcome).
	// Missing/invalid ReadingRef revision or RecordedAt is Malformed, not this case.
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
	Identity    ReadingIdentity     `json:"identity"`
	CompletedAt Measured[time.Time] `json:"completed_at"`
	Reading     ReadingRef          `json:"reading"`
	Attempt     AttemptID           `json:"attempt"`
	Disposition Disposition         `json:"disposition"`
	Reason      string              `json:"reason"`
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
	RowsWithYAMLOnlyTerminal int
	ExcludedJournals         []JournalIdentity
	StartsAfterCutoff        int
	Observations             []RecoveredAttempt
	Examined                 []Examined
	Dispositions             []DispositionCount
	Conflicts                []AttemptConflict
	UniqueRows               int
	Attempts                 int
	Recovered                int
	LostAttempts             []AttemptID
	Ambiguous                []AmbiguousAttempt
	HeldOutRuns              []string
	CutoffApplied            time.Time
}

// JoinEvidence reconciles attempts/readings under a validated Selection. Journals
// is the full discovered identity set INCLUDING excluded journals; every supplied
// AttemptSet must belong to it. Any supplied held-out AttemptSet/Attempt is
// refused with ErrInvalidSelection, before it can contribute observations or
// LostAttempts; run identities must also agree between each set, attempt and start. Unmatched holdouts and inconsistent markers wrap
// ErrInvalidSelection before reconciliation. Duplicate universe run IDs (even
// byte-identical replicas) wrap ErrDuplicateJournalRun; no replica merging. Full exclusions, deterministic audit
// and error rules are authoritative in notes/FC-SCAFFOLD.md "Entry-point contracts"
// and F1/F3 rows. FC-1 body; ReadSources supplies the universe via Journals plus
// ExcludedJournals, so valid held-out runs remain checkable after event removal.
func JoinEvidence(attempts []AttemptSet, readings []Reading, selection Selection, journals []JournalIdentity) (EvidenceJoin, error) {
	out := emptyEvidenceJoin(selection)
	if err := selection.Validate(); err != nil {
		return out, err
	}
	selection.Cutoff = canonicalTime(selection.Cutoff)
	out.CutoffApplied = selection.Cutoff

	holdouts := make(map[string]bool, len(selection.HoldoutRunIDs))
	for _, runID := range selection.HoldoutRunIDs {
		holdouts[runID] = true
	}
	universe := make(map[string]JournalIdentity, len(journals))
	discovered := make([]string, 0, len(journals))
	for _, journal := range journals {
		if strings.TrimSpace(journal.RunID) == "" || strings.TrimSpace(journal.SourceID) == "" || strings.TrimSpace(journal.Path) == "" {
			return emptyEvidenceJoin(selection), fmt.Errorf("%w: journal universe contains an incomplete identity", ErrInvalidSelection)
		}
		if previous, ok := universe[journal.RunID]; ok {
			return emptyEvidenceJoin(selection), fmt.Errorf("%w: run %q occurs in %s and %s", ErrDuplicateJournalRun, journal.RunID, previous.Path, journal.Path)
		}
		universe[journal.RunID] = journal
		discovered = append(discovered, journal.RunID)
		if holdouts[journal.RunID] {
			out.ExcludedJournals = append(out.ExcludedJournals, journal)
		}
	}
	if err := selection.UnmatchedHoldouts(discovered); err != nil {
		return emptyEvidenceJoin(selection), err
	}
	sort.Slice(out.ExcludedJournals, func(i, j int) bool {
		return journalIdentityLess(out.ExcludedJournals[i], out.ExcludedJournals[j])
	})

	byID := make(map[AttemptID]Attempt)
	ambiguous := make(map[AttemptID]AmbiguousAttempt)
	conflicts := make(map[AttemptID][]AttemptConflict)
	counted := make(map[AttemptID]bool)
	rows := make(map[runTask]bool)
	runKeys := make(map[runTask]bool)
	seenSets := make(map[JournalIdentity]bool, len(attempts))
	categories := make(map[AttemptID]string)
	for _, set := range attempts {
		journal, ok := universe[set.Journal.RunID]
		if !ok || journal != set.Journal || holdouts[set.Journal.RunID] {
			return emptyEvidenceJoin(selection), fmt.Errorf("%w: attempt set journal %q is outside the selected universe", ErrInvalidSelection, set.Journal.RunID)
		}
		if seenSets[set.Journal] {
			return emptyEvidenceJoin(selection), fmt.Errorf("%w: repeated attempt set for journal %q", ErrInvalidSelection, set.Journal.RunID)
		}
		seenSets[set.Journal] = true
		for _, attempt := range set.Attempts {
			attempt = canonicalAttempt(attempt)
			if err := validateJoinedAttempt(attempt, set.Journal, selection); err != nil {
				return emptyEvidenceJoin(selection), err
			}
			if _, duplicate := categories[attempt.ID]; duplicate {
				return emptyEvidenceJoin(selection), fmt.Errorf("%w: duplicate normalized attempt %s/%s", ErrEvidenceConflict, attempt.ID.RunID, attempt.ID.Key)
			}
			categories[attempt.ID] = "attempt"
			byID[attempt.ID] = attempt
			counted[attempt.ID] = true
			key := runTask{RunID: attempt.ID.RunID, Key: attempt.ID.Key}
			rows[key] = true
			runKeys[key] = true
		}
		for _, item := range set.Ambiguous {
			item.ID = canonicalAttemptID(item.ID)
			if err := validateAttemptIDForSet(item.ID, set.Journal, selection); err != nil {
				return emptyEvidenceJoin(selection), err
			}
			if _, duplicate := categories[item.ID]; duplicate {
				return emptyEvidenceJoin(selection), fmt.Errorf("%w: duplicate normalized ambiguous attempt %s/%s", ErrEvidenceConflict, item.ID.RunID, item.ID.Key)
			}
			categories[item.ID] = "ambiguous"
			item.Refs = append([]EventRef{}, item.Refs...)
			for i := range item.Refs {
				item.Refs[i] = canonicalEventRef(item.Refs[i], item.Refs[i].Journal)
			}
			sort.Slice(item.Refs, func(i, j int) bool { return eventRefTotalLess(item.Refs[i], item.Refs[j]) })
			ambiguous[item.ID] = item
			counted[item.ID] = true
			key := runTask{RunID: item.ID.RunID, Key: item.ID.Key}
			rows[key] = true
			runKeys[key] = true
		}
		for _, conflict := range set.Conflicts {
			conflict.ID = canonicalAttemptID(conflict.ID)
			if err := validateAttemptIDForSet(conflict.ID, set.Journal, selection); err != nil {
				return emptyEvidenceJoin(selection), err
			}
			if category := categories[conflict.ID]; category != "" && category != "conflict" {
				return emptyEvidenceJoin(selection), fmt.Errorf("%w: attempt %s/%s occurs in %s and conflict categories", ErrEvidenceConflict, conflict.ID.RunID, conflict.ID.Key, category)
			}
			for _, previous := range conflicts[conflict.ID] {
				if sameAttemptConflict(previous, conflict) {
					return emptyEvidenceJoin(selection), fmt.Errorf("%w: duplicate conflict fact for attempt %s/%s", ErrEvidenceConflict, conflict.ID.RunID, conflict.ID.Key)
				}
			}
			categories[conflict.ID] = "conflict"
			conflicts[conflict.ID] = append(conflicts[conflict.ID], conflict)
			counted[conflict.ID] = true
			key := runTask{RunID: conflict.ID.RunID, Key: conflict.ID.Key}
			rows[key] = true
			runKeys[key] = true
		}
		out.StartsAfterCutoff += set.StartsAfterCutoff
	}
	out.Attempts = len(counted)
	out.UniqueRows = len(rows)
	for _, item := range ambiguous {
		out.Ambiguous = append(out.Ambiguous, item)
	}
	sort.Slice(out.Ambiguous, func(i, j int) bool { return attemptIDLess(out.Ambiguous[i].ID, out.Ambiguous[j].ID) })
	for _, list := range conflicts {
		out.Conflicts = append(out.Conflicts, list...)
	}
	sort.Slice(out.Conflicts, func(i, j int) bool { return evidenceConflictLess(out.Conflicts[i], out.Conflicts[j]) })

	type matchedReading struct {
		examined int
		reading  Reading
	}
	grouped := make(map[AttemptID][]matchedReading)
	for i := range readings {
		reading := canonicalReading(readings[i])
		examined := Examined{Identity: reading.Identity, CompletedAt: reading.CompletedAt, Reading: reading.Ref}
		disposition, id, reason, err := classifyReading(reading, selection, holdouts, byID, ambiguous, conflicts, runKeys)
		if err != nil {
			return emptyEvidenceJoin(selection), err
		}
		examined.Attempt = id
		examined.Disposition = disposition
		examined.Reason = reason
		out.Examined = append(out.Examined, examined)
		if disposition == "" {
			grouped[id] = append(grouped[id], matchedReading{examined: len(out.Examined) - 1, reading: reading})
		}
	}

	ids := make([]AttemptID, 0, len(grouped))
	for id := range grouped {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return attemptIDLess(ids[i], ids[j]) })
	var joinErrors []error
	if len(out.Ambiguous) > 0 {
		joinErrors = append(joinErrors, fmt.Errorf("%w: %d ambiguous attempt identities", ErrAmbiguousAttempt, len(out.Ambiguous)))
	}
	if len(out.Conflicts) > 0 {
		joinErrors = append(joinErrors, fmt.Errorf("%w: %d journal evidence conflicts", ErrEvidenceConflict, len(out.Conflicts)))
	}
	for _, id := range ids {
		matched := grouped[id]
		sort.SliceStable(matched, func(i, j int) bool {
			return readingLess(matched[i].reading, matched[j].reading)
		})
		attempt := byID[id]
		exact := make([]Reading, 0, len(matched))
		for _, item := range matched {
			exact = append(exact, item.reading)
		}
		recovered, conflict, err := reconcileAttempt(attempt, exact)
		if err != nil {
			if conflict != nil {
				out.Conflicts = append(out.Conflicts, *conflict)
			}
			for _, item := range matched {
				index := item.examined
				out.Examined[index].Disposition = DispositionConflictingEvidence
				out.Examined[index].Reason = err.Error()
			}
			joinErrors = append(joinErrors, err)
			continue
		}
		if !recovered.Attempt.Model.Known || strings.TrimSpace(recovered.Attempt.Model.Value) == "" {
			for _, item := range matched {
				index := item.examined
				out.Examined[index].Disposition = DispositionAbsentStamp
				out.Examined[index].Reason = "attempt has no recorded implementing-model stamp"
			}
			continue
		}
		if !recovered.Cell.Role.Valid() || recovered.Cell.Model == "" {
			reason := "reconciled attempt has no valid role/model cell"
			if recovered.Cell.Role == "" {
				reason = "attempt has no cited role in compatible YAML readings"
			}
			for _, item := range matched {
				index := item.examined
				out.Examined[index].Disposition = DispositionUnrecoverable
				out.Examined[index].Reason = reason
			}
			continue
		}
		out.Observations = append(out.Observations, recovered)
		if recovered.Attempt.Evidence.Terminal.Source == EvidenceYAML {
			out.RowsWithYAMLOnlyTerminal++
		}
		for position, item := range matched {
			index := item.examined
			if position == 0 {
				out.Examined[index].Disposition = DispositionRecovered
				out.Examined[index].Reason = "least canonical compatible reading"
			} else {
				out.Examined[index].Disposition = DispositionDuplicateReading
				out.Examined[index].Reason = "additional compatible reading of recovered attempt"
			}
		}
	}

	sort.Slice(out.Observations, func(i, j int) bool {
		return attemptIDLess(out.Observations[i].Attempt.ID, out.Observations[j].Attempt.ID)
	})
	sort.Slice(out.Conflicts, func(i, j int) bool { return evidenceConflictLess(out.Conflicts[i], out.Conflicts[j]) })
	recoveredIDs := make(map[AttemptID]bool, len(out.Observations))
	for _, observation := range out.Observations {
		recoveredIDs[observation.Attempt.ID] = true
	}
	for id := range counted {
		if !recoveredIDs[id] {
			out.LostAttempts = append(out.LostAttempts, id)
		}
	}
	sort.Slice(out.LostAttempts, func(i, j int) bool { return attemptIDLess(out.LostAttempts[i], out.LostAttempts[j]) })
	out.Recovered = len(out.Observations)
	sort.SliceStable(out.Examined, func(i, j int) bool { return examinedLess(out.Examined[i], out.Examined[j]) })
	counts := make(map[Disposition]int)
	for _, examined := range out.Examined {
		counts[examined.Disposition]++
	}
	for _, disposition := range Dispositions() {
		out.Dispositions = append(out.Dispositions, DispositionCount{Disposition: disposition, Count: counts[disposition]})
	}
	return out, errors.Join(joinErrors...)
}

func sameAttemptConflict(a, b AttemptConflict) bool {
	return a.Code == b.Code && a.ID == b.ID && a.Field == b.Field &&
		a.AValue.Kind == b.AValue.Kind && bytes.Equal(a.AValue.Value, b.AValue.Value) &&
		a.BValue.Kind == b.BValue.Kind && bytes.Equal(a.BValue.Value, b.BValue.Value) &&
		a.A == b.A && a.B == b.B && a.Reason == b.Reason
}

// evidenceConflictLess extends the Journal-owned primary order into a total
// order for every serialized conflict field. Distinct conflict facts for one
// identity stay paired and retained; Err is intentionally absent because it is
// not part of the portable representation.
func evidenceConflictLess(a, b AttemptConflict) bool {
	if attemptConflictLess(a, b) {
		return true
	}
	if attemptConflictLess(b, a) {
		return false
	}
	if fieldEvidenceTotalLess(a.A, b.A) {
		return true
	}
	if fieldEvidenceTotalLess(b.A, a.A) {
		return false
	}
	if conflictValueLess(a.AValue, b.AValue) {
		return true
	}
	if conflictValueLess(b.AValue, a.AValue) {
		return false
	}
	if fieldEvidenceTotalLess(a.B, b.B) {
		return true
	}
	if fieldEvidenceTotalLess(b.B, a.B) {
		return false
	}
	if conflictValueLess(a.BValue, b.BValue) {
		return true
	}
	if conflictValueLess(b.BValue, a.BValue) {
		return false
	}
	if a.Code != b.Code {
		return a.Code < b.Code
	}
	return a.Reason < b.Reason
}

func fieldEvidenceTotalLess(a, b FieldEvidence) bool {
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if eventRefTotalLess(a.Event, b.Event) {
		return true
	}
	if eventRefTotalLess(b.Event, a.Event) {
		return false
	}
	return readingRefTotalLess(a.Reading, b.Reading)
}

func eventRefTotalLess(a, b EventRef) bool {
	if eventRefLess(a, b) {
		return true
	}
	if eventRefLess(b, a) {
		return false
	}
	return a.At.Format(time.RFC3339Nano) < b.At.Format(time.RFC3339Nano)
}

func readingRefTotalLess(a, b ReadingRef) bool {
	if readingRefLess(a, b) {
		return true
	}
	if readingRefLess(b, a) {
		return false
	}
	return a.RecordedAt.Format(time.RFC3339Nano) < b.RecordedAt.Format(time.RFC3339Nano)
}

func conflictValueLess(a, b ConflictValue) bool {
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return bytes.Compare(a.Value, b.Value) < 0
}

func emptyEvidenceJoin(selection Selection) EvidenceJoin {
	return EvidenceJoin{
		Observations: []RecoveredAttempt{}, Examined: []Examined{}, Dispositions: []DispositionCount{},
		Conflicts: []AttemptConflict{}, LostAttempts: []AttemptID{}, Ambiguous: []AmbiguousAttempt{},
		ExcludedJournals: []JournalIdentity{}, HeldOutRuns: append([]string{}, selection.HoldoutRunIDs...),
		CutoffApplied: canonicalTime(selection.Cutoff),
	}
}

func canonicalAttemptID(id AttemptID) AttemptID {
	id.StartedAt = canonicalTime(id.StartedAt)
	return id
}

func validateAttemptIDForSet(id AttemptID, journal JournalIdentity, selection Selection) error {
	if id.RunID == "" || id.Key == "" || id.StartedAt.IsZero() || id.RunID != journal.RunID {
		return fmt.Errorf("%w: attempt identity disagrees with journal %q", ErrInvalidSelection, journal.RunID)
	}
	if !id.StartedAt.Equal(canonicalTime(id.StartedAt)) || id.StartedAt.After(selection.Cutoff) {
		return fmt.Errorf("%w: attempt %s/%s has an invalid selected start", ErrInvalidSelection, id.RunID, id.Key)
	}
	return nil
}

func validateJoinedAttempt(attempt Attempt, journal JournalIdentity, selection Selection) error {
	if err := validateAttemptIDForSet(attempt.ID, journal, selection); err != nil {
		return err
	}
	if attempt.Start.Journal != journal || attempt.Start.Type != EventTaskStarted || !attempt.Start.At.Equal(attempt.ID.StartedAt) {
		return fmt.Errorf("%w: attempt %s/%s start citation disagrees with its journal identity", ErrInvalidSelection, attempt.ID.RunID, attempt.ID.Key)
	}
	if !attempt.Cutoff.Equal(selection.Cutoff) {
		return fmt.Errorf("%w: attempt %s/%s cutoff differs from selection", ErrInvalidSelection, attempt.ID.RunID, attempt.ID.Key)
	}
	if !attempt.Outcome.Valid() {
		return fmt.Errorf("%w: attempt %s/%s", ErrInvalidOutcome, attempt.ID.RunID, attempt.ID.Key)
	}
	if attempt.Elapsed < 0 {
		return fmt.Errorf("%w: attempt %s/%s has negative elapsed", ErrNegativeValue, attempt.ID.RunID, attempt.ID.Key)
	}
	if err := validateAttemptWall(attempt); err != nil {
		return err
	}
	if _, err := SummarizeWall(attempt.Wall); err != nil {
		return err
	}
	return nil
}

func canonicalReading(reading Reading) Reading {
	reading.Ref.RecordedAt = canonicalTime(reading.Ref.RecordedAt)
	// Measured values are serialized even when Known is false. Normalize their
	// time representation as part of the private portable ordering while
	// preserving the caller's envelope and its availability bit.
	reading.Identity.StartedAt.Value = canonicalTime(reading.Identity.StartedAt.Value)
	reading.CompletedAt.Value = canonicalTime(reading.CompletedAt.Value)
	return reading
}

func classifyReading(reading Reading, selection Selection, holdouts map[string]bool, attempts map[AttemptID]Attempt, ambiguous map[AttemptID]AmbiguousAttempt, conflicts map[AttemptID][]AttemptConflict, runKeys map[runTask]bool) (Disposition, AttemptID, string, error) {
	if reading.Kind == DocumentNotTasks {
		return DispositionNotTaskDocument, AttemptID{}, "YAML document has no tasks sequence", nil
	}
	run, runKnown := reading.Identity.RunID.Get()
	key, keyKnown := reading.Identity.Key.Get()
	start, startKnown := reading.Identity.StartedAt.Get()
	if reading.Excluded != "" && reading.Excluded != DispositionHeldOut && reading.Excluded != DispositionAfterCutoff {
		return "", AttemptID{}, "", fmt.Errorf("%w: reading has unsupported exclusion %q", ErrInvalidSelection, reading.Excluded)
	}
	if runKnown && holdouts[run] {
		if reading.Excluded == DispositionAfterCutoff {
			return "", AttemptID{}, "", fmt.Errorf("%w: held-out reading is marked after-cutoff", ErrInvalidSelection)
		}
		return DispositionHeldOut, AttemptID{}, "reading belongs to a predeclared held-out run", nil
	}
	after := (!reading.Ref.RecordedAt.IsZero() && reading.Ref.RecordedAt.After(selection.Cutoff)) ||
		(startKnown && start.After(selection.Cutoff)) ||
		(reading.CompletedAt.Known && reading.CompletedAt.Value.After(selection.Cutoff))
	if reading.Excluded == DispositionHeldOut {
		return "", AttemptID{}, "", fmt.Errorf("%w: held-out marker lacks matching independently decoded run", ErrInvalidSelection)
	}
	if after {
		return DispositionAfterCutoff, AttemptID{}, "reading contains evidence after the extraction cutoff", nil
	}
	if reading.Excluded == DispositionAfterCutoff {
		return "", AttemptID{}, "", fmt.Errorf("%w: after-cutoff marker has no retained cutoff proof", ErrInvalidSelection)
	}
	invalidIdentity := runKnown && (strings.TrimSpace(run) == "" || run != strings.TrimSpace(run)) ||
		keyKnown && (strings.TrimSpace(key) == "" || key != strings.TrimSpace(key)) ||
		startKnown && start.IsZero() || reading.CompletedAt.Known && reading.CompletedAt.Value.IsZero()
	invalidRef := strings.TrimSpace(reading.Ref.SourceID) == "" || strings.TrimSpace(reading.Ref.Repository) == "" ||
		strings.TrimSpace(reading.Ref.Path) == "" || reading.Ref.RecordedAt.IsZero() ||
		(reading.Kind == DocumentTaskRow && reading.Ref.Row <= 0) || ValidateReadingRevision(reading.Ref.Revision) != nil
	invalidRole := reading.Snapshot.Role != "" && !reading.Snapshot.Role.Valid()
	if reading.Kind == DocumentMalformed || reading.Err != nil || invalidIdentity || invalidRef || invalidRole {
		return DispositionMalformed, AttemptID{}, "reading or citation is malformed", nil
	}
	if !reading.Present.Complete() || !runKnown || !keyKnown || !startKnown {
		return DispositionMissingJoinKeys, AttemptID{}, "reading lacks a complete run/key/start identity", nil
	}
	id := NewAttemptID(run, key, start)
	if _, ok := ambiguous[id]; ok {
		return DispositionAmbiguousStart, id, "multiple journal starts share this exact attempt identity", nil
	}
	if _, ok := conflicts[id]; ok {
		return DispositionConflictingEvidence, id, "journal attempt contains conflicting evidence", nil
	}
	if _, ok := attempts[id]; ok {
		return "", id, "", nil
	}
	if runKeys[runTask{RunID: run, Key: key}] {
		return DispositionNoMatchingStart, id, "run/key exists but no journal start has this exact instant", nil
	}
	return DispositionNoMatchingRun, AttemptID{}, "no journal attempt has this run and task key", nil
}

func readingLess(a, b Reading) bool {
	if readingRefLess(a.Ref, b.Ref) {
		return true
	}
	if readingRefLess(b.Ref, a.Ref) {
		return false
	}
	if readingIdentityLess(a.Identity, b.Identity) {
		return true
	}
	if readingIdentityLess(b.Identity, a.Identity) {
		return false
	}
	if measuredTimeLess(a.CompletedAt, b.CompletedAt) {
		return true
	}
	if measuredTimeLess(b.CompletedAt, a.CompletedAt) {
		return false
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	if a.Excluded != b.Excluded {
		return a.Excluded < b.Excluded
	}
	if a.Present.Key != b.Present.Key {
		return !a.Present.Key
	}
	if a.Present.RunID != b.Present.RunID {
		return !a.Present.RunID
	}
	if a.Present.StartedAt != b.Present.StartedAt {
		return !a.Present.StartedAt
	}
	if a.Snapshot.Role != b.Snapshot.Role {
		return a.Snapshot.Role < b.Snapshot.Role
	}
	if a.Snapshot.AuthoredModel != b.Snapshot.AuthoredModel {
		return a.Snapshot.AuthoredModel < b.Snapshot.AuthoredModel
	}
	if a.Snapshot.Status != b.Snapshot.Status {
		return a.Snapshot.Status < b.Snapshot.Status
	}
	if a.Snapshot.IterationCount != b.Snapshot.IterationCount {
		return a.Snapshot.IterationCount < b.Snapshot.IterationCount
	}
	if (a.Err == nil) != (b.Err == nil) {
		return a.Err == nil
	}
	if a.Err != nil && a.Err.Error() != b.Err.Error() {
		return a.Err.Error() < b.Err.Error()
	}
	return false
}

func reconcileAttempt(attempt Attempt, readings []Reading) (RecoveredAttempt, *AttemptConflict, error) {
	canonical := append([]Reading{}, readings...)
	sort.SliceStable(canonical, func(i, j int) bool { return readingLess(canonical[i], canonical[j]) })
	refs := make([]ReadingRef, 0, len(canonical))
	var role Role
	var roleEvidence FieldEvidence
	var yamlOutcome Outcome
	var yamlTerminal time.Time
	var yamlEvidence FieldEvidence
	for _, reading := range canonical {
		refs = append(refs, reading.Ref)
		candidateRole := reading.Snapshot.Role
		candidateEvidence := FieldEvidence{Source: EvidenceYAML, Reading: reading.Ref}
		if candidateRole == "" {
			// Absent/null role is unknown, not equal-authority disagreement.
		} else if role == "" {
			role, roleEvidence = candidateRole, candidateEvidence
		} else if role != candidateRole {
			conflict := evidenceConflict(attempt.ID, "role", string(role), roleEvidence, string(candidateRole), candidateEvidence)
			return RecoveredAttempt{}, &conflict, conflict.Err
		}
		if outcome, terminal := terminalStatus(reading.Snapshot.Status); terminal && reading.CompletedAt.Known {
			if yamlOutcome == 0 {
				yamlOutcome, yamlTerminal, yamlEvidence = outcome, reading.CompletedAt.Value, candidateEvidence
			} else if yamlOutcome != outcome || !yamlTerminal.Equal(reading.CompletedAt.Value) {
				a := map[string]any{"outcome": yamlOutcome.String(), "terminal_at": yamlTerminal, "elapsed_ns": yamlTerminal.Sub(attempt.ID.StartedAt).Nanoseconds()}
				b := map[string]any{"outcome": outcome.String(), "terminal_at": reading.CompletedAt.Value, "elapsed_ns": reading.CompletedAt.Value.Sub(attempt.ID.StartedAt).Nanoseconds()}
				conflict := evidenceConflict(attempt.ID, "terminal", a, yamlEvidence, b, candidateEvidence)
				return RecoveredAttempt{}, &conflict, conflict.Err
			}
		}
	}
	attempt = canonicalAttempt(attempt)
	attempt.Evidence.Role = roleEvidence
	if attempt.Evidence.Terminal.Source == EvidenceNone && yamlOutcome.Valid() && yamlOutcome != OutcomeUnfinished {
		if yamlTerminal.Before(attempt.ID.StartedAt) {
			return RecoveredAttempt{}, nil, fmt.Errorf("%w: YAML terminal precedes attempt start", ErrReversedInterval)
		}
		attempt.Outcome = yamlOutcome
		attempt.TerminalAt = yamlTerminal
		attempt.Elapsed = yamlTerminal.Sub(attempt.ID.StartedAt)
		attempt.Wall.Elapsed = attempt.Elapsed
		retained := attempt.Wall.Intervals[:0]
		withheld := false
		for _, interval := range attempt.Wall.Intervals {
			if interval.Start.Before(attempt.ID.StartedAt) || interval.End.After(yamlTerminal) {
				withheld = true
				continue
			}
			retained = append(retained, interval)
		}
		attempt.Wall.Intervals = retained
		if withheld {
			attempt.Wall.Complete = false
		}
		attempt.Evidence.Terminal = yamlEvidence
		attempt.Evidence.Elapsed = yamlEvidence
	}
	if err := validateAttemptWall(attempt); err != nil {
		return RecoveredAttempt{}, nil, err
	}
	return RecoveredAttempt{Attempt: attempt, Cell: Cell{Role: role, Model: attempt.Model.Value}, Readings: refs}, nil, nil
}

func evidenceConflict(id AttemptID, field string, aValue any, a FieldEvidence, bValue any, b FieldEvidence) AttemptConflict {
	aRaw, _ := json.Marshal(aValue)
	bRaw, _ := json.Marshal(bValue)
	if bytes.Compare(aRaw, bRaw) > 0 {
		aRaw, bRaw, a, b = bRaw, aRaw, b, a
	}
	reason := fmt.Sprintf("%s evidence conflicts within %s/%s", field, id.RunID, id.Key)
	err := fmt.Errorf("%w: %s", ErrEvidenceConflict, reason)
	return AttemptConflict{Code: EvidenceConflictCode, ID: id, Field: field,
		AValue: ConflictValue{Kind: field, Value: aRaw}, BValue: ConflictValue{Kind: field, Value: bRaw},
		A: a, B: b, Err: err, Reason: reason}
}

func examinedLess(a, b Examined) bool {
	if readingRefLess(a.Reading, b.Reading) {
		return true
	}
	if readingRefLess(b.Reading, a.Reading) {
		return false
	}
	if readingIdentityLess(a.Identity, b.Identity) {
		return true
	}
	if readingIdentityLess(b.Identity, a.Identity) {
		return false
	}
	if measuredTimeLess(a.CompletedAt, b.CompletedAt) {
		return true
	}
	if measuredTimeLess(b.CompletedAt, a.CompletedAt) {
		return false
	}
	if attemptIDLess(a.Attempt, b.Attempt) {
		return true
	}
	if attemptIDLess(b.Attempt, a.Attempt) {
		return false
	}
	if a.Disposition != b.Disposition {
		return a.Disposition < b.Disposition
	}
	return a.Reason < b.Reason
}

func readingIdentityLess(a, b ReadingIdentity) bool {
	if measuredStringLess(a.RunID, b.RunID) {
		return true
	}
	if measuredStringLess(b.RunID, a.RunID) {
		return false
	}
	if measuredStringLess(a.Key, b.Key) {
		return true
	}
	if measuredStringLess(b.Key, a.Key) {
		return false
	}
	return measuredTimeLess(a.StartedAt, b.StartedAt)
}

func measuredStringLess(a, b Measured[string]) bool {
	if a.Known != b.Known {
		return !a.Known
	}
	return a.Value < b.Value
}

func measuredTimeLess(a, b Measured[time.Time]) bool {
	if a.Known != b.Known {
		return !a.Known
	}
	aTime, bTime := canonicalTime(a.Value), canonicalTime(b.Value)
	if !aTime.Equal(bTime) {
		return aTime.Before(bTime)
	}
	return false
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
