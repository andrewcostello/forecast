package dispatched

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// HandFinishedLimit is edge case 9: no field marks a row that a human
// finished after it blocked, so its wall clock cannot be separated from agent
// throughput. Stated in the report rather than modelled around.
const HandFinishedLimit = "Hand-finished rows have no identifying field; their wall clock cannot be distinguished from agent throughput."

// DefaultMinObservations is the number of completed observations a required
// cell needs before the report calls it covered. One completed row is a
// sample, not a distribution, and the refuse-to-predict ruling is worth
// nothing if a single row silently licenses a forecast.
const DefaultMinObservations = 2

// BuildOptions identifies the journal tree, YAML history repository and,
// optionally, the task list whose coverage should be measured.
type BuildOptions struct {
	RunsDir       string
	FeaturesRepo  string // Compatibility for callers supplying one repository.
	FeaturesRepos []string
	TargetTasks   string
	Now           time.Time
	// MinObservations is the completed-observation threshold for calling a
	// required cell covered. Zero means DefaultMinObservations.
	MinObservations int
	// MaxHistoryCommits bounds the git history walk. Zero means
	// defaultMaxHistoryCommits.
	MaxHistoryCommits int

	// Amended contract inputs (F3/F6). Sources are the
	// explicit source specifications that replace RunsDir/FeaturesRepos;
	// Selection freezes holdout run IDs, cutoff and allow-empty; Bounds are
	// the byte/commit/process caps. The legacy shape (nil Sources, zero cutoff,
	// nil holdouts, false AllowEmpty and zero Bounds) runs schema 3 unchanged.
	Sources   []SourceSpec
	Selection Selection
	Bounds    ReadBounds
}

// amended reports whether the options use the F3/F6 inputs.
func (o BuildOptions) amended() bool {
	return o.Sources != nil || !o.Selection.Cutoff.IsZero() || o.Selection.HoldoutRunIDs != nil ||
		o.Selection.AllowEmpty || o.Bounds != (ReadBounds{})
}

// BuildResult holds both the queryable table and its portable artifact.
type BuildResult struct {
	Table    *Table
	Artifact Artifact
}

type Artifact struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedAt   time.Time              `json:"generated_at"`
	Observations  []ReferenceObservation `json:"observations"`
	Cells         []CellSummary          `json:"cells"`
	Coverage      Coverage               `json:"coverage"`
	// Conflicts is the schema-3 compatibility field. In schema 4,
	// Evidence.Conflicts is the authoritative conflict audit.
	Conflicts []Conflict `json:"conflicts"`
	Limits    []string   `json:"limits"`
	// SourceManifest (F3) names every source the artifact rests on and its
	// completeness. Nil from the baseline path; FC-1 populates it and bumps
	// SchemaVersion when it does.
	SourceManifest *SourceManifest   `json:"source_manifest,omitempty"`
	Evidence       *ArtifactEvidence `json:"evidence,omitempty"`
}

// AmendedEvidenceSchemaVersion is emitted only once FC-1 implements the amended build.
// Legacy version 3 retains its fields. Version 4 requires SourceManifest and Evidence;
// consumers must reject missing payloads or unsupported versions, not default
// their absence to zero. Legacy Observations/Cells are compatibility projections
// only; amended sampling uses Evidence.Observations exclusively.
const AmendedEvidenceSchemaVersion = 4

// BaselineSchemaVersion is the current legacy writer version. Only the amended
// FC-1 Build may emit AmendedEvidenceSchemaVersion after filling both required payloads.
const BaselineSchemaVersion = 3

// ArtifactEvidence is the complete serializable join audit and joint sample.
// A nil payload means unavailable (legacy version 3); all counters inside a version 4
// payload are present, including zero. Durations on Attempt use nanoseconds.
// FC-1 initializes every list in version 4 evidence (including nested lists) to
// [] rather than null. Canonical JSON-value equality includes that distinction.
// Attempt outcome and terminal conflict outcome are done/blocked/unfinished text.
// Compatibility projections map amended EvidenceNone="" to legacy "none",
// leaving "yaml" and "journal" unchanged; amended sampling never reads projections.
// Reading revisions use stable strings; Cell retains legacy Role/Model JSON keys.
// Limits must disclose recorded_task_spawns cost scope, excluded cache tokens,
// and that unrecorded reviewer/operator spend is not total-process cost.
type ArtifactEvidence struct {
	RowsWithYAMLOnlyTerminal int                `json:"rows_with_yaml_only_terminal"`
	StartsAfterCutoff        int                `json:"starts_after_cutoff"`
	Observations             []RecoveredAttempt `json:"observations"`
	Examined                 []Examined         `json:"examined"`
	Dispositions             []DispositionCount `json:"dispositions"`
	Conflicts                []AttemptConflict  `json:"conflicts"`
	Ambiguous                []AmbiguousAttempt `json:"ambiguous"`
	UniqueRows               int                `json:"unique_rows"`
	Attempts                 int                `json:"attempts"`
	Recovered                int                `json:"recovered"`
	LostAttempts             []AttemptID        `json:"lost_attempts"`
	ExcludedJournals         []JournalIdentity  `json:"excluded_journals"`
}

// EligibilityCell is computed from joint evidence, never legacy coverage counts.
type EligibilityCell struct {
	Role         Role   `json:"role"`
	Model        string `json:"model"`
	Completed    int    `json:"completed"`
	MinCompleted int    `json:"min_completed"`
	Eligible     bool   `json:"eligible"`
}

// Eligibility is the F4 prediction gate result. Eligible is true only when
// the manifest is COMPLETE, the target has at least one row, every required
// cell is valid, and every required cell holds at least MinCompleted
// completed samples. Reasons lists every failed condition; MinCompleted is
// reported as a threshold, not as proof of calibration.
type Eligibility struct {
	Cells        []EligibilityCell `json:"cells"`
	Eligible     bool              `json:"eligible"`
	MinCompleted int               `json:"min_completed"`
	Reasons      []string          `json:"reasons,omitempty"`
}

// TargetRow preserves each target identity and cell before coverage aggregation.
// Callers pass original rows, including invalid ones, so validation cannot hide
// blank/duplicate keys or roles/models that aggregation would discard.
type TargetRow struct {
	Key   string `json:"key"`
	Role  Role   `json:"role"`
	Model string `json:"model"`
}

// PredictionEligibility requires SchemaVersion == AmendedEvidenceSchemaVersion exactly,
// nonnil Evidence, a valid complete SourceManifest, a nonempty valid target
// argument and sufficient completed
// samples in every required cell. minCompleted<=0 uses DefaultMinObservations;
// Eligibility.MinCompleted records the effective positive threshold.
// Invalid schema/payload or incomplete sources yield Eligible=false and a
// reason; when refuse is true their error wraps BOTH ErrNotEligible and
// ErrSourceIncomplete. A thin cell wraps ErrNotEligible when refusing. Zero-row
// targets always wrap ErrEmptyTarget; malformed targets wrap ErrInvalidTarget.
// With refuse=false, ordinary insufficiency is a diagnostic result, not an
// error.
//
// Validate target rows first: empty -> ErrEmptyTarget, then declaration-order
// invalid/duplicate keys, invalid roles or blank models -> ErrInvalidTarget.
// Compute completed counts from valid Evidence.Observations, not legacy Cells/Coverage.
// A sampled run listed in manifest holdouts, a mismatched manifest/attempt
// cutoff, malformed joint records or cell/model contradictions make the evidence
// payload invalid; refuse with ErrSourceIncomplete and ErrNotEligible as above.
func PredictionEligibility(artifact Artifact, target []TargetRow, minCompleted int, refuse bool) (Eligibility, error) {
	if minCompleted <= 0 {
		minCompleted = DefaultMinObservations
	}
	out := Eligibility{Cells: []EligibilityCell{}, MinCompleted: minCompleted, Reasons: []string{}}
	if len(target) == 0 {
		return out, fmt.Errorf("%w: prediction gate requires at least one target row", ErrEmptyTarget)
	}
	required := make(map[Cell]bool)
	seenKeys := make(map[string]bool, len(target))
	for i, row := range target {
		if strings.TrimSpace(row.Key) == "" || row.Key != strings.TrimSpace(row.Key) {
			return out, fmt.Errorf("%w: target row %d has a blank or padded key", ErrInvalidTarget, i+1)
		}
		if seenKeys[row.Key] {
			return out, fmt.Errorf("%w: target repeats key %q", ErrInvalidTarget, row.Key)
		}
		seenKeys[row.Key] = true
		if !row.Role.Valid() {
			return out, fmt.Errorf("%w: target row %q has role %q", ErrInvalidTarget, row.Key, row.Role)
		}
		if strings.TrimSpace(row.Model) == "" {
			return out, fmt.Errorf("%w: target row %q has no model", ErrInvalidTarget, row.Key)
		}
		required[Cell{Role: row.Role, Model: row.Model}] = true
	}

	invalid := false
	if artifact.SchemaVersion != AmendedEvidenceSchemaVersion {
		invalid = true
		out.Reasons = append(out.Reasons, fmt.Sprintf("schema version %d is not supported evidence schema %d", artifact.SchemaVersion, AmendedEvidenceSchemaVersion))
	}
	if artifact.Evidence == nil {
		invalid = true
		out.Reasons = append(out.Reasons, "structured evidence payload is missing")
	}
	if artifact.SourceManifest == nil {
		invalid = true
		out.Reasons = append(out.Reasons, "source manifest is missing")
	} else if err := artifact.SourceManifest.ValidateComplete(); err != nil {
		invalid = true
		out.Reasons = append(out.Reasons, err.Error())
	}
	completed := make(map[Cell]int)
	if artifact.Evidence != nil && artifact.SourceManifest != nil {
		if reasons := validateArtifactEvidence(artifact.Evidence, artifact.SourceManifest); len(reasons) != 0 {
			invalid = true
			out.Reasons = append(out.Reasons, reasons...)
		}
		seenAttempts := make(map[AttemptID]bool)
		holdouts := make(map[string]bool, len(artifact.SourceManifest.HoldoutRunIDs))
		for _, runID := range artifact.SourceManifest.HoldoutRunIDs {
			holdouts[runID] = true
		}
		for i, recovered := range artifact.Evidence.Observations {
			attempt := canonicalAttempt(recovered.Attempt)
			switch {
			case attempt.ID.RunID == "" || attempt.ID.Key == "" || attempt.ID.StartedAt.IsZero():
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %d has an incomplete attempt identity", i))
			case seenAttempts[attempt.ID]:
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %d repeats attempt %s/%s", i, attempt.ID.RunID, attempt.ID.Key))
			default:
				seenAttempts[attempt.ID] = true
			}
			if holdouts[attempt.ID.RunID] {
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %s/%s belongs to held-out run", attempt.ID.RunID, attempt.ID.Key))
			}
			if !attempt.Cutoff.Equal(artifact.SourceManifest.Cutoff) {
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %s/%s cutoff differs from manifest", attempt.ID.RunID, attempt.ID.Key))
			}
			if !attempt.Outcome.Valid() || attempt.Elapsed < 0 || validateAttemptWall(attempt) != nil {
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %s/%s has an invalid joint attempt", attempt.ID.RunID, attempt.ID.Key))
			}
			if !recovered.Cell.Role.Valid() || strings.TrimSpace(recovered.Cell.Model) == "" ||
				!attempt.Model.Known || recovered.Cell.Model != attempt.Model.Value {
				invalid = true
				out.Reasons = append(out.Reasons, fmt.Sprintf("observation %s/%s cell contradicts its stamped model", attempt.ID.RunID, attempt.ID.Key))
				continue
			}
			if attempt.Outcome == OutcomeDone {
				completed[recovered.Cell]++
			}
		}
	}

	cells := make([]Cell, 0, len(required))
	for cell := range required {
		cells = append(cells, cell)
	}
	sortCells(cells)
	thin := false
	for _, cell := range cells {
		count := completed[cell]
		eligible := count >= minCompleted
		out.Cells = append(out.Cells, EligibilityCell{Role: cell.Role, Model: cell.Model, Completed: count, MinCompleted: minCompleted, Eligible: eligible})
		if !eligible {
			thin = true
			out.Reasons = append(out.Reasons, fmt.Sprintf("%s/%s has %d completed observations; requires %d", cell.Role, cell.Model, count, minCompleted))
		}
	}
	out.Eligible = !invalid && !thin
	if !refuse || out.Eligible {
		return out, nil
	}
	if invalid {
		return out, errors.Join(
			fmt.Errorf("%w: artifact evidence is incomplete or invalid", ErrNotEligible),
			fmt.Errorf("%w: artifact evidence is incomplete or invalid", ErrSourceIncomplete),
		)
	}
	return out, fmt.Errorf("%w: required cells are below the completed-observation threshold", ErrNotEligible)
}

func validateArtifactEvidence(evidence *ArtifactEvidence, manifest *SourceManifest) []string {
	var reasons []string
	add := func(format string, args ...any) {
		reasons = append(reasons, fmt.Sprintf(format, args...))
	}
	if evidence.Recovered != len(evidence.Observations) {
		add("structured evidence recovered count %d does not match %d observations", evidence.Recovered, len(evidence.Observations))
	}
	if evidence.Attempts != evidence.Recovered+len(evidence.LostAttempts) {
		add("structured evidence attempt count %d does not match %d recovered plus lost identities", evidence.Attempts, evidence.Recovered+len(evidence.LostAttempts))
	}
	if evidence.StartsAfterCutoff < 0 {
		add("structured evidence has a negative starts-after-cutoff count")
	}

	selectedSources := make(map[string]SourceReport, len(manifest.Sources))
	for _, source := range manifest.Sources {
		selectedSources[source.ID] = source
	}
	holdouts := make(map[string]bool, len(manifest.HoldoutRunIDs))
	for _, runID := range manifest.HoldoutRunIDs {
		holdouts[runID] = true
	}

	type auditKey struct {
		Attempt AttemptID
		Reading ReadingRef
	}
	canonicalAuditKey := func(id AttemptID, ref ReadingRef) auditKey {
		id = canonicalAttemptID(id)
		ref.RecordedAt = canonicalTime(ref.RecordedAt)
		return auditKey{Attempt: id, Reading: ref}
	}
	seenAttempts := make(map[AttemptID]bool, len(evidence.Observations)+len(evidence.LostAttempts))
	recoveredReadings := make(map[AttemptID]map[ReadingRef]bool, len(evidence.Observations))
	expectedAudit := make(map[auditKey]int)
	auditEnvelopes := make(map[auditKey][]Examined)
	recoveredExamined := make(map[AttemptID]int, len(evidence.Observations))
	rows := make(map[runTask]bool)
	yamlOnly := 0
	for i, recovered := range evidence.Observations {
		id := canonicalAttemptID(recovered.Attempt.ID)
		if seenAttempts[id] {
			add("structured observation %d repeats attempt %s/%s", i, id.RunID, id.Key)
		}
		seenAttempts[id] = true
		rows[runTask{RunID: id.RunID, Key: id.Key}] = true
		recoveredReadings[id] = make(map[ReadingRef]bool, len(recovered.Readings))
		for _, ref := range recovered.Readings {
			ref.RecordedAt = canonicalTime(ref.RecordedAt)
			recoveredReadings[id][ref] = true
			expectedAudit[canonicalAuditKey(id, ref)]++
		}
		if err := validateRecoveredEvidence(recovered, manifest, selectedSources); err != nil {
			add("structured observation %d is invalid: %v", i, err)
		}
		if recovered.Attempt.Evidence.Terminal.Source == EvidenceYAML {
			yamlOnly++
		}
	}
	for i, id := range evidence.LostAttempts {
		id = canonicalAttemptID(id)
		if id.RunID == "" || id.Key == "" || id.StartedAt.IsZero() {
			add("lost attempt %d has an incomplete identity", i)
			continue
		}
		if seenAttempts[id] {
			add("lost attempt %d duplicates a recovered or lost identity", i)
		}
		seenAttempts[id] = true
		rows[runTask{RunID: id.RunID, Key: id.Key}] = true
	}
	if evidence.UniqueRows != len(rows) {
		add("structured evidence unique-row count %d does not match %d represented rows", evidence.UniqueRows, len(rows))
	}
	if evidence.RowsWithYAMLOnlyTerminal != yamlOnly {
		add("structured evidence YAML-only terminal count %d does not match %d observations", evidence.RowsWithYAMLOnlyTerminal, yamlOnly)
	}

	actualDispositions := make(map[Disposition]int)
	for i, examined := range evidence.Examined {
		if !examined.Disposition.Valid() {
			add("examined reading %d has invalid disposition %q", i, examined.Disposition)
			continue
		}
		actualDispositions[examined.Disposition]++
		if examined.Disposition == DispositionRecovered || examined.Disposition == DispositionDuplicateReading {
			id := canonicalAttemptID(examined.Attempt)
			identityID, identityOK := artifactReadingAttemptID(examined.Identity)
			if !identityOK || identityID != id {
				add("examined recovered reading %d identity does not agree with its attempt", i)
			} else if holdouts[identityID.RunID] {
				add("examined recovered reading %d claims a held-out identity", i)
			}
			if examined.CompletedAt.Known && (examined.CompletedAt.Value.IsZero() || examined.CompletedAt.Value.After(manifest.Cutoff)) {
				add("examined recovered reading %d has completion proof after the extraction cutoff", i)
			}
			examined.Reading.RecordedAt = canonicalTime(examined.Reading.RecordedAt)
			refs, ok := recoveredReadings[id]
			if !ok || !refs[examined.Reading] {
				add("examined recovered reading %d does not belong to its structured observation", i)
			} else {
				key := canonicalAuditKey(id, examined.Reading)
				auditEnvelopes[key] = append(auditEnvelopes[key], examined)
			}
			if examined.Disposition == DispositionRecovered {
				recoveredExamined[id]++
			}
		}
	}
	declared := Dispositions()
	if len(evidence.Examined) == 0 {
		if len(evidence.Dispositions) != 0 && len(evidence.Dispositions) != len(declared) {
			add("empty examined audit has %d disposition counters; want zero or %d", len(evidence.Dispositions), len(declared))
		} else if len(evidence.Dispositions) != 0 {
			for i, count := range evidence.Dispositions {
				if count.Disposition != declared[i] || count.Count != 0 {
					add("empty examined audit has inconsistent disposition counts")
					break
				}
			}
		}
	} else if len(evidence.Dispositions) != len(declared) {
		add("structured evidence has %d disposition counters; want %d", len(evidence.Dispositions), len(declared))
	} else {
		for i, disposition := range declared {
			count := evidence.Dispositions[i]
			if count.Disposition != disposition || count.Count != actualDispositions[disposition] {
				add("disposition counter %d does not describe examined readings", i)
			}
		}
	}
	if actualDispositions[DispositionRecovered] != len(evidence.Observations) {
		add("recovered disposition count %d does not match %d observations", actualDispositions[DispositionRecovered], len(evidence.Observations))
	}
	for id := range recoveredReadings {
		if recoveredExamined[id] != 1 {
			add("structured observation %s/%s has %d recovered audit envelopes; want one", id.RunID, id.Key, recoveredExamined[id])
		}
	}
	for key, want := range expectedAudit {
		if got := len(auditEnvelopes[key]); got != want {
			add("structured observation %s/%s reading has %d audit envelopes; want %d", key.Attempt.RunID, key.Attempt.Key, got, want)
		}
	}
	for i, recovered := range evidence.Observations {
		attempt := canonicalAttempt(recovered.Attempt)
		if attempt.Evidence.Terminal.Source != EvidenceYAML {
			continue
		}
		key := canonicalAuditKey(attempt.ID, attempt.Evidence.Terminal.Reading)
		matched := false
		for _, examined := range auditEnvelopes[key] {
			if examined.CompletedAt.Known && examined.CompletedAt.Value.Equal(attempt.TerminalAt) {
				matched = true
				break
			}
		}
		if !matched {
			add("structured observation %d YAML terminal lacks matching known completion audit", i)
		}
	}

	if manifest.State == SourceComplete {
		if len(evidence.Conflicts) != 0 || actualDispositions[DispositionConflictingEvidence] != 0 {
			add("complete sources contain conflicting evidence")
		}
		if len(evidence.Ambiguous) != 0 || actualDispositions[DispositionAmbiguousStart] != 0 {
			add("complete sources contain ambiguous attempts")
		}
		if actualDispositions[DispositionMalformed] != 0 || actualDispositions[DispositionUnrecoverable] != 0 {
			add("complete sources contain malformed or unrecoverable evidence")
		}
	}
	return canonicalStrings(reasons)
}

func validateRecoveredEvidence(recovered RecoveredAttempt, manifest *SourceManifest, selectedSources map[string]SourceReport) error {
	attempt := canonicalAttempt(recovered.Attempt)
	if attempt.ID.RunID == "" || attempt.ID.Key == "" || attempt.ID.StartedAt.IsZero() {
		return fmt.Errorf("attempt identity is incomplete")
	}
	if !validArtifactJournalIdentity(attempt.Start.Journal, selectedSources) || attempt.Start.Journal.RunID != attempt.ID.RunID || attempt.Start.Type != EventTaskStarted ||
		!attempt.Start.At.Equal(attempt.ID.StartedAt) {
		return fmt.Errorf("start citation disagrees with attempt identity")
	}
	if !validArtifactAttemptJournals(attempt, selectedSources) {
		return fmt.Errorf("attempt contains a journal citation outside its selected run source")
	}
	if attempt.Evidence.Start.Source != EvidenceJournal || attempt.Evidence.Start.Event != attempt.Start {
		return fmt.Errorf("required start provenance is missing or inconsistent")
	}
	if !attempt.Cutoff.Equal(manifest.Cutoff) {
		return fmt.Errorf("attempt cutoff differs from source manifest")
	}
	if !attempt.Outcome.Valid() || attempt.Elapsed < 0 {
		return fmt.Errorf("attempt outcome or elapsed value is invalid")
	}
	if attempt.CostUSD.Known && (attempt.CostUSD.Value < 0 || math.IsNaN(attempt.CostUSD.Value) || math.IsInf(attempt.CostUSD.Value, 0)) {
		return fmt.Errorf("%w: known attempt cost is not finite and nonnegative", ErrNegativeValue)
	}
	if (attempt.InputTokens.Known && attempt.InputTokens.Value < 0) || (attempt.OutputTokens.Known && attempt.OutputTokens.Value < 0) {
		return fmt.Errorf("%w: known attempt token total is negative", ErrNegativeValue)
	}
	if attempt.Corrections < 0 || attempt.Cascades < 0 || attempt.Reviews < 0 || attempt.Verifications < 0 {
		return fmt.Errorf("%w: attempt contains a negative recorded count", ErrNegativeValue)
	}
	if attempt.Outcome.terminal() && attempt.TerminalAt.After(manifest.Cutoff) {
		return fmt.Errorf("terminal attempt ends after the extraction cutoff")
	}
	if attempt.Outcome == OutcomeUnfinished && !attempt.ID.StartedAt.Add(attempt.Elapsed).Equal(manifest.Cutoff) {
		return fmt.Errorf("unfinished attempt elapsed does not reach the extraction cutoff")
	}
	if err := validateAttemptWall(attempt); err != nil {
		return err
	}
	if _, err := SummarizeWall(attempt.Wall); err != nil {
		return err
	}
	if !recovered.Cell.Role.Valid() || strings.TrimSpace(recovered.Cell.Model) == "" ||
		!attempt.Model.Known || strings.TrimSpace(attempt.Model.Value) == "" || recovered.Cell.Model != attempt.Model.Value {
		return fmt.Errorf("cell does not match a valid stamped model and role")
	}
	refs := make(map[ReadingRef]bool, len(recovered.Readings))
	for _, ref := range recovered.Readings {
		ref.RecordedAt = canonicalTime(ref.RecordedAt)
		if !validRecoveredReadingRef(ref, manifest.Cutoff, selectedSources) {
			return fmt.Errorf("recovered reading citation is malformed")
		}
		refs[ref] = true
	}
	if len(recovered.Readings) == 0 {
		return fmt.Errorf("recovered attempt has no reading citations")
	}
	if attempt.Evidence.Role.Source != EvidenceYAML || !refs[attempt.Evidence.Role.Reading] {
		return fmt.Errorf("required role provenance does not cite a recovered reading")
	}
	if !validAttemptJournalEvidence(attempt.Evidence.Model, attempt, EventTaskSpawnFinished) {
		return fmt.Errorf("required model provenance does not cite this attempt's journal")
	}

	terminal, elapsed := attempt.Evidence.Terminal, attempt.Evidence.Elapsed
	switch terminal.Source {
	case EvidenceJournal:
		wantType := EventTaskDone
		if attempt.Outcome == OutcomeBlocked {
			wantType = EventTaskBlocked
		}
		if !attempt.Outcome.terminal() || !validAttemptJournalEvidence(terminal, attempt, wantType) ||
			elapsed.Source != EvidenceJournal || elapsed.Event != terminal.Event {
			return fmt.Errorf("journal terminal/elapsed provenance is missing or inconsistent")
		}
		if !attempt.TerminalAt.Equal(terminal.Event.At) || attempt.Elapsed != attempt.TerminalAt.Sub(attempt.ID.StartedAt) {
			return fmt.Errorf("journal terminal value disagrees with elapsed")
		}
	case EvidenceYAML:
		if !attempt.Outcome.terminal() || !refs[terminal.Reading] || elapsed.Source != EvidenceYAML ||
			elapsed.Reading != terminal.Reading {
			return fmt.Errorf("YAML terminal and elapsed must cite the same recovered reading")
		}
		if !attempt.TerminalAt.Equal(attempt.ID.StartedAt.Add(attempt.Elapsed)) {
			return fmt.Errorf("YAML terminal value disagrees with elapsed")
		}
	case EvidenceNone:
		if attempt.Outcome != OutcomeUnfinished || elapsed.Source != EvidenceNone {
			return fmt.Errorf("terminal/elapsed provenance is absent for a terminal attempt")
		}
	default:
		return fmt.Errorf("terminal provenance source %q is invalid", terminal.Source)
	}
	return nil
}

func artifactReadingAttemptID(identity ReadingIdentity) (AttemptID, bool) {
	if !identity.RunID.Known || !identity.Key.Known || !identity.StartedAt.Known ||
		strings.TrimSpace(identity.RunID.Value) == "" || identity.RunID.Value != strings.TrimSpace(identity.RunID.Value) ||
		strings.TrimSpace(identity.Key.Value) == "" || identity.Key.Value != strings.TrimSpace(identity.Key.Value) ||
		identity.StartedAt.Value.IsZero() {
		return AttemptID{}, false
	}
	return NewAttemptID(identity.RunID.Value, identity.Key.Value, identity.StartedAt.Value), true
}

func validRecoveredReadingRef(ref ReadingRef, cutoff time.Time, selectedSources map[string]SourceReport) bool {
	source, ok := selectedSources[ref.SourceID]
	if !ok || ref.Row <= 0 || ref.Repository != source.Repository ||
		ref.Path == "" || !portableRelativePath(ref.Path) ||
		ref.RecordedAt.IsZero() || ref.RecordedAt.After(cutoff) || ValidateReadingRevision(ref.Revision) != nil {
		return false
	}
	switch source.Kind {
	case SourceKindLiveYAML:
		if ref.Revision != "live" {
			return false
		}
	case SourceKindGitHistory:
		if !strings.HasPrefix(ref.Revision, "git:") {
			return false
		}
	default:
		return false
	}
	for _, root := range source.Roots {
		if portablePathWithin(ref.Path, root) {
			return true
		}
	}
	return false
}

func validAttemptJournalEvidence(evidence FieldEvidence, attempt Attempt, eventType string) bool {
	return evidence.Source == EvidenceJournal && evidence.Event.Journal == attempt.Start.Journal &&
		evidence.Event.Type == eventType && !evidence.Event.At.IsZero() &&
		!evidence.Event.At.Before(attempt.ID.StartedAt) && !evidence.Event.At.After(attempt.Cutoff)
}

func validArtifactJournalIdentity(journal JournalIdentity, selectedSources map[string]SourceReport) bool {
	source, ok := selectedSources[journal.SourceID]
	return ok && source.Kind == SourceKindJournals &&
		journal.RunID != "" && journal.RunID == strings.TrimSpace(journal.RunID) &&
		journal.Producer == ProducerDispatcherV0_1_0 && portableRelativePath(journal.Path) &&
		journal.Path == path.Join(journal.RunID, "journal.jsonl")
}

func validArtifactAttemptJournals(attempt Attempt, selectedSources map[string]SourceReport) bool {
	valid := func(ref EventRef) bool {
		if ref == (EventRef{}) {
			return true
		}
		return ref.Journal == attempt.Start.Journal && validArtifactJournalIdentity(ref.Journal, selectedSources)
	}
	for _, ref := range []EventRef{
		attempt.Start,
		attempt.Evidence.Role.Event,
		attempt.Evidence.Model.Event,
		attempt.Evidence.Start.Event,
		attempt.Evidence.Terminal.Event,
		attempt.Evidence.Elapsed.Event,
		attempt.Evidence.Wall.Event,
		attempt.Evidence.Corrections.Event,
		attempt.Evidence.Cascades.Event,
		attempt.Evidence.Reviews.Event,
		attempt.Evidence.Verifications.Event,
		attempt.Evidence.InputTokens.Event,
		attempt.Evidence.OutputTokens.Event,
		attempt.Evidence.Cost.Event,
	} {
		if !valid(ref) {
			return false
		}
	}
	lists := [][]EventRef{
		attempt.CascadeEvents,
		attempt.CorrectionEvents,
		attempt.ReviewEvents,
		attempt.VerificationEvents,
		attempt.CostEvents,
		attempt.InputTokenEvents,
		attempt.OutputTokenEvents,
	}
	for _, interval := range attempt.Wall.Intervals {
		lists = append(lists, interval.Evidence)
	}
	for _, refs := range lists {
		for _, ref := range refs {
			if !valid(ref) {
				return false
			}
		}
	}
	return true
}

func portableRelativePath(value string) bool {
	return value != "" && !path.IsAbs(value) && !strings.Contains(value, `\`) &&
		path.Clean(value) == value && value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func portablePathWithin(value, root string) bool {
	if !portableRelativePath(value) {
		return false
	}
	cleanRoot := path.Clean(root)
	if cleanRoot == "." {
		return true
	}
	if !portableRelativePath(cleanRoot) {
		return false
	}
	return value == cleanRoot || strings.HasPrefix(value, cleanRoot+"/")
}

// ReferenceObservation is one stored row as the artifact serialises it.
// CostUSD is null when no spawn payload recorded a cost, so a downstream mean
// can exclude the row instead of averaging in a zero nobody measured.
type ReferenceObservation struct {
	Key                string    `json:"key"`
	Role               Role      `json:"role"`
	Model              string    `json:"model"`
	Outcome            string    `json:"outcome"`
	TerminalEvidence   string    `json:"terminal_evidence"`
	Censored           bool      `json:"censored"`
	StartedAt          time.Time `json:"started_at"`
	ElapsedSeconds     float64   `json:"elapsed_seconds"`
	DevelopmentSeconds float64   `json:"development_seconds"`
	ReviewSeconds      float64   `json:"review_seconds"`
	Rounds             int       `json:"rounds"`
	Cascades           int       `json:"cascades"`
	InputTokens        int64     `json:"input_tokens"`
	OutputTokens       int64     `json:"output_tokens"`
	CostUSD            *float64  `json:"cost_usd"`
	DispatcherRunID    string    `json:"dispatcher_run_id"`
	SourceRevision     string    `json:"source_revision"`
	SourceRepository   string    `json:"source_repository"`
	SourcePath         string    `json:"source_path"`
}

type NumericSummary struct {
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

// CellSummary is the per-(role, model) tally. Duration summarises COMPLETED
// rows only: a censored row contributes to N, NBlocked and NCensored and to
// the rounds summary, and never to Duration.
type CellSummary struct {
	Role      Role           `json:"role"`
	Model     string         `json:"model"`
	N         int            `json:"n"`
	NDone     int            `json:"n_done"`
	NBlocked  int            `json:"n_blocked"`
	NCensored int            `json:"n_censored"`
	Duration  NumericSummary `json:"duration_seconds"`
	Rounds    NumericSummary `json:"rounds"`
}

type RequiredCell struct {
	Role       Role   `json:"role"`
	Model      string `json:"model"`
	TargetRows int    `json:"target_rows"`
	N          int    `json:"n"`
	NDone      int    `json:"n_done"`
	Empty      bool   `json:"empty"`
	Covered    bool   `json:"covered"`
}

type RepositoryCoverage struct {
	Repository         string `json:"repository"`
	LiveReadings       int    `json:"live_readings"`
	HistoricalReadings int    `json:"historical_readings"`
	MatchedAttempts    int    `json:"matched_attempts"`
}

type Coverage struct {
	Repositories                    []RepositoryCoverage `json:"repositories"`
	YAMLRowsMissingJoinKeys         int                  `json:"yaml_rows_missing_join_keys"`
	SnapshotsWithoutMatchingAttempt int                  `json:"snapshots_without_matching_attempt"`
	AttemptsWithoutMatchingYAML     int                  `json:"attempts_without_matching_yaml"`
	RecoveredAttempts               int                  `json:"recovered_attempts"`
	AttemptRecoveryShortfall        int                  `json:"attempt_recovery_shortfall"`
	TargetTasks                     string               `json:"target_tasks,omitempty"`
	MinObservations                 int                  `json:"min_observations"`
	RequiredCells                   []RequiredCell       `json:"required_cells"`
	EmptyRequiredCells              []Cell               `json:"empty_required_cells"`
	UncoveredRequiredCells          []Cell               `json:"uncovered_required_cells"`
	TargetRows                      int                  `json:"target_rows"`
	TargetRowsWithCell              int                  `json:"target_rows_with_cell"`
	TargetRowsCovered               int                  `json:"target_rows_covered"`
	TargetCoveredShare              *float64             `json:"target_covered_share"`

	// JournalStartedRows counts DISTINCT (run, task key) pairs that started,
	// which is the denominator a recovered row can be compared against.
	// JournalStartAttempts counts task_started events, so the two differ by
	// JournalRestarts.
	JournalStartedRows   int `json:"journal_started_rows"`
	JournalStartAttempts int `json:"journal_start_attempts"`
	JournalRestarts      int `json:"journal_restarts"`
	// RecoveredObservations counts stored rows, which is one per recovered
	// ATTEMPT; RecoveredRows counts the distinct (run, task key) pairs behind
	// them, so it is comparable with JournalStartedRows. A run that restarts
	// a task can yield two observations from one started row.
	RecoveredObservations int `json:"recovered_observations"`
	RecoveredRows         int `json:"recovered_rows"`
	RecoveryShortfall     int `json:"recovery_shortfall"`

	JournalRunTasksWithoutYAML       int `json:"journal_run_tasks_without_yaml"`
	JournalLinesUnparsed             int `json:"journal_lines_unparsed"`
	JournalEventsWithBadTimestamp    int `json:"journal_events_with_bad_timestamp"`
	TaskStartsWithoutModel           int `json:"task_starts_without_model"`
	JournalAttemptsWithoutStamp      int `json:"journal_attempts_without_stamped_model"`
	UnattributableJoinedRows         int `json:"unattributable_joined_rows"`
	UnrecoverableJoinedRows          int `json:"unrecoverable_joined_rows"`
	StampConflictRows                int `json:"stamp_conflict_rows"`
	AuthoredStampMismatches          int `json:"authored_stamp_mismatches"`
	RowsWithCascade                  int `json:"rows_with_cascade"`
	RowsWithoutRecordedCost          int `json:"rows_without_recorded_cost"`
	RowsWithYAMLOnlyTerminalEvidence int `json:"rows_with_yaml_only_terminal_evidence"`

	LiveYAMLReadings       int  `json:"live_yaml_readings"`
	HistoricalYAMLReadings int  `json:"historical_yaml_readings"`
	HistoryCommits         int  `json:"history_commits"`
	HistoryBlobs           int  `json:"history_blobs"`
	HistoryTruncated       bool `json:"history_truncated"`
	UnparseableYAMLDocs    int  `json:"unparseable_yaml_documents"`
	MalformedYAMLRows      int  `json:"malformed_yaml_rows"`
}

type Conflict struct {
	Key       string    `json:"key"`
	StartedAt time.Time `json:"started_at"`
	Cells     []Cell    `json:"cells"`
	Reason    string    `json:"reason"`
}

// The amended FC-1 Build is frozen in notes/FC-SCAFFOLD.md "Entry-point
// contracts" (Build / serialization) and F4-MIXED-OPTIONS.

// Build constructs the union reference class. The stamped journal model is
// the only model used for attribution; the authored YAML model is retained
// only to count disagreements.
func Build(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.MinObservations <= 0 {
		opts.MinObservations = DefaultMinObservations
	}
	if opts.amended() {
		if opts.RunsDir != "" || opts.FeaturesRepo != "" || opts.FeaturesRepos != nil || opts.MaxHistoryCommits != 0 {
			return nil, fmt.Errorf("%w: amended sources cannot be combined with legacy source or history options", ErrInvalidSourceSpec)
		}
		return buildAmended(ctx, opts)
	}
	journals, err := readJournals(ctx, opts.RunsDir)
	if err != nil {
		return nil, err
	}
	coverage := Coverage{
		TargetTasks: opts.TargetTasks, MinObservations: opts.MinObservations,
		JournalLinesUnparsed: journals.LinesUnparsed, JournalEventsWithBadTimestamp: journals.BadTimestamps,
	}
	repos := append([]string{}, opts.FeaturesRepos...)
	if opts.FeaturesRepo != "" {
		repos = append(repos, opts.FeaturesRepo)
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("%w: at least one features repository is required", ErrYAMLSource)
	}
	var snapshots []taskSnapshot
	seenRepos := make(map[string]bool)
	for _, repo := range repos {
		if strings.TrimSpace(repo) == "" {
			return nil, fmt.Errorf("%w: empty features repository", ErrYAMLSource)
		}
		repo, err = filepath.Abs(repo)
		if err != nil {
			return nil, fmt.Errorf("%w: repository path: %v", ErrYAMLSource, err)
		}
		repo, err = filepath.EvalSymlinks(repo)
		if err != nil {
			return nil, fmt.Errorf("%w: repository path: %v", ErrYAMLSource, err)
		}
		if seenRepos[repo] {
			continue
		}
		seenRepos[repo] = true
		live, err := readLiveSnapshots(ctx, repo)
		if err != nil {
			return nil, err
		}
		historical, err := readGitSnapshots(ctx, repo, opts.MaxHistoryCommits)
		if err != nil {
			return nil, err
		}
		coverage.Repositories = append(coverage.Repositories, RepositoryCoverage{Repository: repo, LiveReadings: len(live.Snapshots), HistoricalReadings: len(historical.Snapshots)})
		coverage.LiveYAMLReadings += len(live.Snapshots)
		coverage.HistoricalYAMLReadings += len(historical.Snapshots)
		coverage.HistoryCommits += historical.Commits
		coverage.HistoryBlobs += historical.Blobs
		coverage.HistoryTruncated = coverage.HistoryTruncated || historical.Truncated
		coverage.UnparseableYAMLDocs += live.UnparseableDocuments + historical.UnparseableDocuments
		coverage.MalformedYAMLRows += live.MalformedRows + historical.MalformedRows
		coverage.YAMLRowsMissingJoinKeys += live.MissingJoinKeys + historical.MissingJoinKeys
		snapshots = append(snapshots, live.Snapshots...)
		snapshots = append(snapshots, historical.Snapshots...)
	}
	sort.Slice(coverage.Repositories, func(i, j int) bool { return coverage.Repositories[i].Repository < coverage.Repositories[j].Repository })
	var target []yamlTask
	if opts.TargetTasks != "" {
		target, err = readTargetTasks(opts.TargetTasks)
		if err != nil {
			return nil, err
		}
	}
	required := make(map[Cell]int)
	coverage.TargetRows = len(target)
	for _, task := range target {
		cell := Cell{Role: task.Role, Model: task.Model}

		required[cell]++
		coverage.TargetRowsWithCell++
	}
	declared := make([]Cell, 0, len(required))
	for cell := range required {
		declared = append(declared, cell)
	}
	table := NewTable(declared...)

	for _, row := range journals.Rows {
		starts := row.Starts()
		if starts > 0 {
			coverage.JournalStartedRows++
		}
		coverage.JournalStartAttempts += starts
		coverage.JournalRestarts += row.Restarts()
		coverage.TaskStartsWithoutModel += row.StartsWithoutModel()
		coverage.JournalAttemptsWithoutStamp += row.AttemptsWithoutStampedModel()
	}

	readings := make(map[identity][]Observation)
	readingAttempts := make(map[identity]map[*JournalFacts]runTask)
	matchedAttempts := make(map[*JournalFacts]bool)
	repoMatches := make(map[string]map[*JournalFacts]bool)
	joinedRunTasks := make(map[runTask]bool)
	unattributable := make(map[identity]bool)
	unrecoverable := make(map[identity]bool)
	mismatches := make(map[identity]bool)
	for _, snapshot := range snapshots {
		key := runTask{RunID: snapshot.DispatcherRunID, Key: snapshot.Key}
		row, ok := journals.Rows[key]
		if !ok {
			continue
		}
		joinedRunTasks[key] = true
		facts := row.Match(snapshot.StartedAt)
		if facts == nil {
			coverage.SnapshotsWithoutMatchingAttempt++
			continue
		}
		matchedAttempts[facts] = true
		if repoMatches[snapshot.Repository] == nil {
			repoMatches[snapshot.Repository] = make(map[*JournalFacts]bool)
		}
		repoMatches[snapshot.Repository][facts] = true
		id := identity{Key: snapshot.Key, StartedAt: snapshot.StartedAt.UTC()}
		observation, err := observationFrom(snapshot, facts, opts.Now)
		if err != nil {
			// Only the two documented degraded classes are tolerated, and only
			// as a classification; anything else is a defect in this package
			// and must not be swallowed because some other revision happened to
			// parse.
			switch {
			case errors.Is(err, ErrUnattributable):
				if len(readings[id]) == 0 && !unrecoverable[id] {
					unattributable[id] = true
				}
			case isUnrecoverableObservationError(err):
				if len(readings[id]) == 0 {
					unrecoverable[id] = true
					delete(unattributable, id)
				}
			default:
				return nil, err
			}
			continue
		}
		delete(unattributable, id)
		delete(unrecoverable, id)
		if snapshot.AuthoredModel != "" && snapshot.AuthoredModel != facts.Model {
			mismatches[id] = true
		}
		readings[id] = append(readings[id], observation)
		if readingAttempts[id] == nil {
			readingAttempts[id] = make(map[*JournalFacts]runTask)
		}
		readingAttempts[id][facts] = key
	}
	for i := range coverage.Repositories {
		coverage.Repositories[i].MatchedAttempts = len(repoMatches[coverage.Repositories[i].Repository])
	}
	for _, row := range journals.Rows {
		for _, facts := range row.Attempts {
			if facts.Started > 0 && !matchedAttempts[facts] {
				coverage.AttemptsWithoutMatchingYAML++
			}
		}
	}
	coverage.UnattributableJoinedRows = len(unattributable)
	coverage.UnrecoverableJoinedRows = len(unrecoverable)
	coverage.AuthoredStampMismatches = len(mismatches)
	for key, row := range journals.Rows {
		if row.Starts() > 0 && !joinedRunTasks[key] {
			coverage.JournalRunTasksWithoutYAML++
		}
	}

	ids := make([]identity, 0, len(readings))
	for id := range readings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].StartedAt.Equal(ids[j].StartedAt) {
			return ids[i].Key < ids[j].Key
		}
		return ids[i].StartedAt.Before(ids[j].StartedAt)
	})
	var conflicts []Conflict
	recoveredRuns := make(map[runTask]bool)
	recoveredAttempts := make(map[*JournalFacts]bool)
	for _, id := range ids {
		rows := readings[id]
		// Merge every reading of the identity BEFORE touching the table, so a
		// conflict discovered on the last reading cannot leave the first ones
		// stored in an artifact that also reports them as excluded.
		joined, err := joinReadings(rows)
		if err != nil {
			conflicts = append(conflicts, Conflict{
				Key: id.Key, StartedAt: id.StartedAt, Cells: distinctCells(rows), Reason: err.Error(),
			})
			continue
		}
		if err := table.Add(joined); err != nil {
			return nil, err
		}
		coverage.RecoveredObservations++
		for facts, key := range readingAttempts[id] {
			recoveredRuns[key] = true
			recoveredAttempts[facts] = true
		}
	}
	coverage.RecoveredRows = len(recoveredRuns)
	coverage.RecoveredAttempts = len(recoveredAttempts)
	coverage.AttemptRecoveryShortfall = coverage.JournalStartAttempts - coverage.RecoveredAttempts
	coverage.StampConflictRows = len(conflicts)
	coverage.RecoveryShortfall = max(coverage.JournalStartedRows-coverage.RecoveredRows, 0)

	observations := flattenObservations(table)
	for _, row := range observations {
		if row.Cascades > 0 {
			coverage.RowsWithCascade++
		}
		if row.CostUSD == nil {
			coverage.RowsWithoutRecordedCost++
		}
		if row.TerminalEvidence == terminalEvidenceYAML {
			coverage.RowsWithYAMLOnlyTerminalEvidence++
		}
	}

	cells := summarizeCells(table)
	coverage.finishTarget(required, cells, opts.MinObservations)
	artifact := Artifact{
		SchemaVersion: BaselineSchemaVersion,
		GeneratedAt:   opts.Now.UTC(),
		Observations:  observations,
		Cells:         cells,
		Coverage:      coverage,
		Conflicts:     conflicts,
		Limits:        []string{HandFinishedLimit},
	}
	return &BuildResult{Table: table, Artifact: artifact}, nil
}

func buildAmended(ctx context.Context, opts BuildOptions) (*BuildResult, error) {
	cutoff := opts.Selection.Cutoff
	if cutoff.IsZero() {
		cutoff = opts.Now
	}
	cutoff = canonicalTime(cutoff)
	selection := opts.Selection
	selection.Cutoff = cutoff

	target, targetErr := readAmendedTarget(opts.TargetTasks)
	if targetErr != nil {
		return nil, targetErr
	}
	required := make(map[Cell]int)
	for _, row := range target {
		required[Cell{Role: row.Role, Model: row.Model}]++
	}

	manifest, sources, sourceErr := ReadSources(ctx, opts.Sources, selection, opts.Bounds)
	if manifest == nil {
		manifest = &SourceManifest{Reasons: []string{}, Sources: []SourceReport{}, Cutoff: cutoff,
			HoldoutRunIDs: append([]string{}, selection.HoldoutRunIDs...), AllowEmpty: selection.AllowEmpty, State: SourcePartial}
	}
	manifest.Cutoff = cutoff
	if sourceErr != nil {
		addManifestReason(manifest, aggregateDiagnosticReason("source", sourceErr))
		manifest.State = SourcePartial
	}
	join := emptyEvidenceJoin(selection)
	var sets []AttemptSet
	var reduceErrors []error
	if sources != nil {
		sets = make([]AttemptSet, 0, len(sources.Journals))
		for _, parsed := range sources.Journals {
			if err := ctx.Err(); err != nil {
				reduceErrors = append(reduceErrors, fmt.Errorf("%w: reduce phase: %w", ErrSourceCancelled, err))
				break
			}
			set, err := ReduceAttempts(parsed, cutoff)
			sets = append(sets, set)
			if err != nil {
				reduceErrors = append(reduceErrors, err)
			}
			if len(set.Conflicts) > 0 && !errors.Is(err, ErrEvidenceConflict) {
				reduceErrors = append(reduceErrors, fmt.Errorf("%w: journal %s contains %d terminal conflicts", ErrEvidenceConflict, parsed.Journal.Path, len(set.Conflicts)))
			}
		}
	}
	reduceErr := errors.Join(reduceErrors...)
	if reduceErr != nil {
		addManifestReason(manifest, aggregateDiagnosticReason("reduce", reduceErr))
		manifest.State = SourcePartial
	}

	var joinErr, cancelErr error
	if err := ctx.Err(); err != nil {
		cancelErr = fmt.Errorf("%w: join phase was not entered: %w", ErrSourceCancelled, err)
		addManifestReason(manifest, aggregateDiagnosticReason("join", cancelErr))
		manifest.State = SourcePartial
	} else if sources != nil {
		universe := make([]JournalIdentity, 0, len(sources.Journals)+len(sources.ExcludedJournals))
		for _, parsed := range sources.Journals {
			universe = append(universe, parsed.Journal)
		}
		universe = append(universe, sources.ExcludedJournals...)
		join, joinErr = JoinEvidence(sets, sources.Readings, selection, universe)
		if joinErr != nil {
			addManifestReason(manifest, aggregateDiagnosticReason("join", joinErr))
			manifest.State = SourcePartial
		}
		if dispositionTotal(join, DispositionMalformed) > 0 || dispositionTotal(join, DispositionUnrecoverable) > 0 {
			manifest.State = SourcePartial
			addManifestReason(manifest, "join: in-sample malformed or unrecoverable evidence")
		}
	}
	manifest.Reasons = canonicalStrings(manifest.Reasons)

	cells := summarizeRecoveredCells(join.Observations, required)
	coverage := amendedCoverage(opts, *manifest, join, target, cells)
	evidence := artifactEvidence(join)
	projected, projectionErr := projectRecoveredObservations(join.Observations)
	if projectionErr != nil {
		addManifestReason(manifest, aggregateDiagnosticReason("projection", projectionErr))
		manifest.Reasons = canonicalStrings(manifest.Reasons)
		manifest.State = SourcePartial
	}
	artifact := Artifact{
		SchemaVersion: AmendedEvidenceSchemaVersion,
		GeneratedAt:   cutoff,
		Observations:  projected,
		Cells:         cells,
		Coverage:      coverage,
		Conflicts:     []Conflict{},
		Limits: []string{
			HandFinishedLimit,
			"Cost covers recorded_task_spawns only; cache tokens and unrecorded reviewer/operator spend are excluded.",
			"Schema-4 rounds records correction events, not review invocation count.",
			"Legacy phase and unknown-token projections cannot express availability; structured evidence is authoritative.",
			"Legacy Coverage journal_lines_unparsed and journal_events_with_bad_timestamp are uncomputed separately; SourceManifest source malformed counts are authoritative for source quality.",
			"Legacy Coverage journal_attempts_without_stamped_model and journal_run_tasks_without_yaml are uncomputed; Evidence dispositions and lost_attempts are authoritative. journal_restarts, task_starts_without_model, and authored_stamp_mismatches remain unavailable in schema 4.",
			"Legacy Conflicts is a compatibility field; Evidence.conflicts is authoritative in schema 4.",
		},
		SourceManifest: manifest,
		Evidence:       evidence,
	}
	result := &BuildResult{Table: nil, Artifact: artifact}
	return result, errors.Join(sourceErr, reduceErr, cancelErr, joinErr, projectionErr)
}

func readAmendedTarget(path string) ([]TargetRow, error) {
	if path == "" {
		return []TargetRow{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("%w: read target %s: %v", ErrInvalidTarget, path, err),
			fmt.Errorf("%w: read target %s: %v", ErrYAMLSource, path, err),
		)
	}
	doc, err := decodeTaskDocument(data)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("%w: parse target %s: %v", ErrInvalidTarget, path, err),
			fmt.Errorf("%w: parse target %s: %v", ErrYAMLSource, path, err),
		)
	}
	if len(doc.Tasks) == 0 {
		return nil, fmt.Errorf("%w: target %s contains no task rows", ErrEmptyTarget, path)
	}
	out := make([]TargetRow, 0, len(doc.Tasks))
	seen := make(map[string]bool, len(doc.Tasks))
	for i, task := range doc.Tasks {
		row := TargetRow{Key: task.Key, Role: task.Role, Model: task.Model}
		if strings.TrimSpace(row.Key) == "" || row.Key != strings.TrimSpace(row.Key) ||
			!row.Role.Valid() || strings.TrimSpace(row.Model) == "" {
			detail := fmt.Sprintf("target %s row %d requires an exact key, valid role and nonblank model", path, i+1)
			return nil, errors.Join(fmt.Errorf("%w: %s", ErrInvalidTarget, detail), fmt.Errorf("%w: %s", ErrYAMLSource, detail))
		}
		if seen[row.Key] {
			detail := fmt.Sprintf("target %s repeats key %q", path, row.Key)
			return nil, errors.Join(fmt.Errorf("%w: %s", ErrInvalidTarget, detail), fmt.Errorf("%w: %s", ErrYAMLSource, detail))
		}
		seen[row.Key] = true
		out = append(out, row)
	}
	return out, nil
}

func aggregateDiagnosticReason(stage string, err error) string {
	name := "error"
	for _, candidate := range []struct {
		err  error
		name string
	}{
		{ErrMeasurementOverflow, "ErrMeasurementOverflow"},
		{ErrReversedInterval, "ErrReversedInterval"},
		{ErrEvidenceConflict, "ErrEvidenceConflict"},
		{ErrNonCanonicalEvidence, "ErrNonCanonicalEvidence"},
		{ErrInvalidSelection, "ErrInvalidSelection"},
		{ErrInvalidSourceSpec, "ErrInvalidSourceSpec"},
		{ErrInvalidOutcome, "ErrInvalidOutcome"},
		{ErrSourceCancelled, "ErrSourceCancelled"},
		{ErrSourceMissing, "ErrSourceMissing"},
		{ErrSourceEmpty, "ErrSourceEmpty"},
		{ErrJournalSource, "ErrJournalSource"},
		{ErrGitHistory, "ErrGitHistory"},
		{ErrBoundExceeded, "ErrBoundExceeded"},
	} {
		if errors.Is(err, candidate.err) {
			name = candidate.name
			break
		}
	}
	return fmt.Sprintf("%s: %s: %s", stage, name, err.Error())
}

func dispositionTotal(join EvidenceJoin, disposition Disposition) int {
	for _, count := range join.Dispositions {
		if count.Disposition == disposition {
			return count.Count
		}
	}
	return 0
}

func artifactEvidence(join EvidenceJoin) *ArtifactEvidence {
	dispositions := join.Dispositions
	if len(join.Examined) == 0 {
		dispositions = []DispositionCount{}
	}
	return &ArtifactEvidence{
		RowsWithYAMLOnlyTerminal: join.RowsWithYAMLOnlyTerminal,
		StartsAfterCutoff:        join.StartsAfterCutoff,
		Observations:             nonnilRecovered(join.Observations),
		Examined:                 nonnilExamined(join.Examined),
		Dispositions:             nonnilDispositions(dispositions),
		Conflicts:                nonnilAttemptConflicts(join.Conflicts),
		Ambiguous:                nonnilAmbiguous(join.Ambiguous),
		UniqueRows:               join.UniqueRows,
		Attempts:                 join.Attempts,
		Recovered:                join.Recovered,
		LostAttempts:             nonnilAttemptIDs(join.LostAttempts),
		ExcludedJournals:         nonnilJournalIdentities(join.ExcludedJournals),
	}
}

func nonnilRecovered(value []RecoveredAttempt) []RecoveredAttempt {
	if value == nil {
		return []RecoveredAttempt{}
	}
	return value
}
func nonnilExamined(value []Examined) []Examined {
	if value == nil {
		return []Examined{}
	}
	return value
}
func nonnilDispositions(value []DispositionCount) []DispositionCount {
	if value == nil {
		return []DispositionCount{}
	}
	return value
}
func nonnilAttemptConflicts(value []AttemptConflict) []AttemptConflict {
	if value == nil {
		return []AttemptConflict{}
	}
	return value
}
func nonnilAmbiguous(value []AmbiguousAttempt) []AmbiguousAttempt {
	if value == nil {
		return []AmbiguousAttempt{}
	}
	return value
}
func nonnilAttemptIDs(value []AttemptID) []AttemptID {
	if value == nil {
		return []AttemptID{}
	}
	return value
}
func nonnilJournalIdentities(value []JournalIdentity) []JournalIdentity {
	if value == nil {
		return []JournalIdentity{}
	}
	return value
}

func summarizeRecoveredCells(observations []RecoveredAttempt, required map[Cell]int) []CellSummary {
	byCell := make(map[Cell][]Attempt)
	for _, observation := range observations {
		byCell[observation.Cell] = append(byCell[observation.Cell], observation.Attempt)
	}
	for cell := range required {
		if _, ok := byCell[cell]; !ok {
			byCell[cell] = []Attempt{}
		}
	}
	cells := make([]Cell, 0, len(byCell))
	for cell := range byCell {
		cells = append(cells, cell)
	}
	sortCells(cells)
	out := make([]CellSummary, 0, len(cells))
	for _, cell := range cells {
		attempts := byCell[cell]
		var durations, rounds []float64
		var done, blocked int
		for _, attempt := range attempts {
			if attempt.Outcome == OutcomeDone {
				done++
				durations = append(durations, attempt.Elapsed.Seconds())
			}
			if attempt.Outcome == OutcomeBlocked {
				blocked++
			}
			rounds = append(rounds, float64(attempt.Corrections))
		}
		out = append(out, CellSummary{Role: cell.Role, Model: cell.Model, N: len(attempts), NDone: done,
			NBlocked: blocked, NCensored: len(attempts) - done, Duration: summarize(durations), Rounds: summarize(rounds)})
	}
	return out
}

func projectRecoveredObservations(observations []RecoveredAttempt) ([]ReferenceObservation, error) {
	out := make([]ReferenceObservation, 0, len(observations))
	for _, recovered := range observations {
		attempt := recovered.Attempt
		wall, err := SummarizeWall(attempt.Wall)
		if err != nil {
			return []ReferenceObservation{}, fmt.Errorf("project attempt %s/%s wall: %w", attempt.ID.RunID, attempt.ID.Key, err)
		}
		var cost *float64
		if attempt.CostUSD.Known {
			value := attempt.CostUSD.Value
			cost = &value
		}
		var revision, repository, path string
		if len(recovered.Readings) > 0 {
			revision = recovered.Readings[0].Revision
			repository = recovered.Readings[0].Repository
			path = recovered.Readings[0].Path
		}
		terminal := terminalEvidenceNone
		if attempt.Evidence.Terminal.Source != EvidenceNone {
			terminal = string(attempt.Evidence.Terminal.Source)
		}
		out = append(out, ReferenceObservation{
			Key: attempt.ID.Key, Role: recovered.Cell.Role, Model: recovered.Cell.Model,
			Outcome: attempt.Outcome.String(), TerminalEvidence: terminal, Censored: attempt.Outcome != OutcomeDone,
			StartedAt: attempt.ID.StartedAt, ElapsedSeconds: attempt.Elapsed.Seconds(),
			DevelopmentSeconds: wall.Development.Seconds(), ReviewSeconds: wall.PanelReview.Seconds(),
			Rounds: attempt.Corrections, Cascades: attempt.Cascades,
			InputTokens: attempt.InputTokens.Value, OutputTokens: attempt.OutputTokens.Value, CostUSD: cost,
			DispatcherRunID: attempt.ID.RunID, SourceRevision: revision, SourceRepository: repository, SourcePath: path,
		})
	}
	return out, nil
}

func amendedCoverage(opts BuildOptions, manifest SourceManifest, join EvidenceJoin, target []TargetRow, cells []CellSummary) Coverage {
	c := Coverage{
		Repositories: []RepositoryCoverage{}, TargetTasks: opts.TargetTasks, MinObservations: opts.MinObservations,
		RequiredCells: []RequiredCell{}, EmptyRequiredCells: []Cell{}, UncoveredRequiredCells: []Cell{},
		TargetRows: len(target), TargetRowsWithCell: len(target), JournalStartedRows: join.UniqueRows,
		JournalStartAttempts: join.Attempts, RecoveredObservations: join.Recovered, RecoveredAttempts: join.Recovered,
		AttemptRecoveryShortfall: max(join.Attempts-join.Recovered, 0), AttemptsWithoutMatchingYAML: len(join.LostAttempts),
		SnapshotsWithoutMatchingAttempt: dispositionTotal(join, DispositionNoMatchingRun) + dispositionTotal(join, DispositionNoMatchingStart) + dispositionTotal(join, DispositionAmbiguousStart),
		YAMLRowsMissingJoinKeys:         dispositionTotal(join, DispositionMissingJoinKeys),
		UnrecoverableJoinedRows:         dispositionTotal(join, DispositionUnrecoverable),
		UnattributableJoinedRows:        dispositionTotal(join, DispositionAbsentStamp),
		StampConflictRows:               len(join.Conflicts), RowsWithYAMLOnlyTerminalEvidence: join.RowsWithYAMLOnlyTerminal,
		MalformedYAMLRows: dispositionTotal(join, DispositionMalformed),
	}
	recoveredRows := make(map[runTask]bool)
	matchedByRepo := make(map[string]map[AttemptID]bool)
	for _, observation := range join.Observations {
		recoveredRows[runTask{RunID: observation.Attempt.ID.RunID, Key: observation.Attempt.ID.Key}] = true
		for _, reading := range observation.Readings {
			if matchedByRepo[reading.Repository] == nil {
				matchedByRepo[reading.Repository] = make(map[AttemptID]bool)
			}
			matchedByRepo[reading.Repository][observation.Attempt.ID] = true
		}
		if observation.Attempt.Cascades > 0 {
			c.RowsWithCascade++
		}
		if !observation.Attempt.CostUSD.Known {
			c.RowsWithoutRecordedCost++
		}
	}
	c.RecoveredRows = len(recoveredRows)
	c.RecoveryShortfall = max(c.JournalStartedRows-c.RecoveredRows, 0)
	repositories := make(map[string]*RepositoryCoverage)
	for _, report := range manifest.Sources {
		switch report.Kind {
		case SourceKindLiveYAML:
			c.LiveYAMLReadings += report.Counts.Records
		case SourceKindGitHistory:
			c.HistoricalYAMLReadings += report.Counts.Records
			c.HistoryCommits += report.Counts.Commits
			c.HistoryBlobs += report.Counts.Blobs
			c.HistoryTruncated = c.HistoryTruncated || report.Counts.BoundsExceeded > 0
		}
		if report.Kind == SourceKindLiveYAML || report.Kind == SourceKindGitHistory {
			coverage := repositories[report.Repository]
			if coverage == nil {
				coverage = &RepositoryCoverage{Repository: report.Repository}
				repositories[report.Repository] = coverage
			}
			coverage.LiveReadings += boolInt(report.Kind == SourceKindLiveYAML) * report.Counts.Records
			coverage.HistoricalReadings += boolInt(report.Kind == SourceKindGitHistory) * report.Counts.Records
			coverage.MatchedAttempts = len(matchedByRepo[report.Repository])
		}
		c.UnparseableYAMLDocs += report.Counts.Malformed
	}
	for _, coverage := range repositories {
		c.Repositories = append(c.Repositories, *coverage)
	}
	sort.Slice(c.Repositories, func(i, j int) bool { return c.Repositories[i].Repository < c.Repositories[j].Repository })
	required := make(map[Cell]int)
	for _, row := range target {
		required[Cell{Role: row.Role, Model: row.Model}]++
	}
	c.finishTarget(required, cells, opts.MinObservations)
	return c
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func distinctCells(rows []Observation) []Cell {
	set := make(map[Cell]bool)
	for _, row := range rows {
		set[row.Cell] = true
	}
	out := make([]Cell, 0, len(set))
	for cell := range set {
		out = append(out, cell)
	}
	sortCells(out)
	return out
}

func sortCells(cells []Cell) {
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Role != cells[j].Role {
			return cells[i].Role < cells[j].Role
		}
		return cells[i].Model < cells[j].Model
	})
}

// summarizeCells builds the per-cell tally. Duration is fed ONLY by
// Observation.Duration, which refuses to return a value for a censored row —
// the single place the right-censoring ruling is enforced for statistics.
func summarizeCells(table *Table) []CellSummary {
	cells := table.Cells()
	out := make([]CellSummary, 0, len(cells))
	for _, cell := range cells {
		observations := table.Observations(cell)
		var durations, rounds []float64
		var done, blocked int
		for _, observation := range observations {
			if duration, ok := observation.Duration(); ok {
				done++
				durations = append(durations, duration.Seconds())
			}
			if observation.Outcome == OutcomeBlocked {
				blocked++
			}
			rounds = append(rounds, float64(observation.Rounds))
		}
		out = append(out, CellSummary{
			Role: cell.Role, Model: cell.Model, N: len(observations),
			NDone: done, NBlocked: blocked, NCensored: len(observations) - done,
			Duration: summarize(durations), Rounds: summarize(rounds),
		})
	}
	return out
}

func summarize(values []float64) NumericSummary {
	if len(values) == 0 {
		return NumericSummary{}
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	var sum float64
	for _, value := range sorted {
		sum += value
	}
	middle := len(sorted) / 2
	median := sorted[middle]
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	}
	return NumericSummary{N: len(sorted), Min: sorted[0], Mean: sum / float64(len(sorted)), Median: median, Max: sorted[len(sorted)-1]}
}

func flattenObservations(table *Table) []ReferenceObservation {
	var out []ReferenceObservation
	for _, cell := range table.Cells() {
		observations := table.Observations(cell)
		sort.Slice(observations, func(i, j int) bool {
			if observations[i].StartedAt.Equal(observations[j].StartedAt) {
				return observations[i].Key < observations[j].Key
			}
			return observations[i].StartedAt.Before(observations[j].StartedAt)
		})
		for _, observation := range observations {
			var cost *float64
			if observation.CostKnown {
				value := observation.CostUSD
				cost = &value
			}
			evidence := observation.TerminalEvidence
			if evidence == "" {
				evidence = terminalEvidenceNone
			}
			out = append(out, ReferenceObservation{
				Key: observation.Key, Role: cell.Role, Model: cell.Model,
				Outcome: observation.Outcome.String(), TerminalEvidence: evidence,
				Censored:  observation.Censored(),
				StartedAt: observation.StartedAt, ElapsedSeconds: observation.Elapsed.Seconds(),
				DevelopmentSeconds: observation.DevElapsed.Seconds(), ReviewSeconds: observation.ReviewElapsed.Seconds(),
				Rounds: observation.Rounds, Cascades: observation.Cascades,
				InputTokens: observation.InputTokens, OutputTokens: observation.OutputTokens,
				CostUSD: cost, DispatcherRunID: observation.Provenance.RunID,
				SourceRevision:   observation.Provenance.Revision.String(),
				SourceRepository: observation.Provenance.Repository, SourcePath: observation.Provenance.Path,
			})
		}
	}
	return out
}

// finishTarget reports which required cells the class can and cannot speak
// for. A cell is covered only when it holds at least minObservations
// COMPLETED rows: a cell of censored rows knows a lower bound and no
// duration, and a cell of one row is a sample.
func (c *Coverage) finishTarget(required map[Cell]int, cells []CellSummary, minObservations int) {
	byCell := make(map[Cell]CellSummary, len(cells))
	for _, summary := range cells {
		byCell[Cell{Role: summary.Role, Model: summary.Model}] = summary
	}
	ordered := make([]Cell, 0, len(required))
	for cell := range required {
		ordered = append(ordered, cell)
	}
	sortCells(ordered)
	for _, cell := range ordered {
		summary := byCell[cell]
		item := RequiredCell{
			Role: cell.Role, Model: cell.Model, TargetRows: required[cell],
			N: summary.N, NDone: summary.NDone,
			Empty:   summary.N == 0,
			Covered: summary.NDone >= minObservations,
		}
		c.RequiredCells = append(c.RequiredCells, item)
		if item.Empty {
			c.EmptyRequiredCells = append(c.EmptyRequiredCells, cell)
		}
		if !item.Covered {
			c.UncoveredRequiredCells = append(c.UncoveredRequiredCells, cell)
		} else {
			c.TargetRowsCovered += item.TargetRows
		}
	}
	if c.TargetRows > 0 {
		share := float64(c.TargetRowsCovered) / float64(c.TargetRows)
		c.TargetCoveredShare = &share
	}
}

// WriteArtifact atomically replaces path with indented JSON.
func WriteArtifact(path string, artifact Artifact) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: create parent for %s: %v", ErrReferenceOutput, path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dispatched-reference-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		return fmt.Errorf("%w: encode %s: %v", ErrReferenceOutput, path, err)
	}
	// CreateTemp makes the file 0600; a shared analysis artifact is read by
	// other accounts and CI steps.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("%w: chmod temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: close temporary output for %s: %v", ErrReferenceOutput, path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("%w: replace %s: %v", ErrReferenceOutput, path, err)
	}
	ok = true
	return nil
}

// WriteCoverage writes the human-readable coverage report delivered by FC-1.
// Every shortfall is a printed number: a reader must be able to see what the
// class does not cover without opening the JSON.
func WriteCoverage(w io.Writer, artifact Artifact) {
	fmt.Fprintln(w, "Dispatched reference-class coverage")
	fmt.Fprintln(w, "role/model\tn\tn_done\tn_blocked\tn_censored\tduration mean/median/min/max\trounds mean/min/max")
	for _, cell := range artifact.Cells {
		fmt.Fprintf(w, "%s/%s\t%d\t%d\t%d\t%d\t%s\t%s\n",
			cell.Role, cell.Model, cell.N, cell.NDone, cell.NBlocked, cell.NCensored,
			formatDurationSummary(cell.Duration), formatRoundsSummary(cell.Rounds))
	}
	c := artifact.Coverage
	for _, repo := range c.Repositories {
		fmt.Fprintf(w, "Source repository %s: live=%d historical=%d matched_attempts=%d\n", repo.Repository, repo.LiveReadings, repo.HistoricalReadings, repo.MatchedAttempts)
	}
	if c.TargetTasks != "" {
		fmt.Fprintf(w, "\nTarget tasks: %s (a cell is covered at n_done >= %d)\n", c.TargetTasks, c.MinObservations)
		for _, cell := range c.RequiredCells {
			fmt.Fprintf(w, "  required %s/%s: target_rows=%d n=%d n_done=%d empty=%t covered=%t\n", cell.Role, cell.Model, cell.TargetRows, cell.N, cell.NDone, cell.Empty, cell.Covered)
		}
		fmt.Fprintf(w, "Required cells empty: %d; required cells not covered: %d\n", len(c.EmptyRequiredCells), len(c.UncoveredRequiredCells))
		// A target row with no role or no model cannot be asked about at all,
		// which is a wider hole than an empty cell and belongs in the same
		// paragraph rather than only in the JSON.
		fmt.Fprintf(w, "Target rows naming a (role, model) cell: %d/%d\n", c.TargetRowsWithCell, c.TargetRows)
		if c.TargetCoveredShare != nil {
			fmt.Fprintf(w, "Target rows in a covered cell: %d/%d (%.1f%%)\n", c.TargetRowsCovered, c.TargetRows, *c.TargetCoveredShare*100)
		}
	}
	if artifact.SchemaVersion == AmendedEvidenceSchemaVersion && artifact.SourceManifest != nil && artifact.Evidence != nil {
		writeAmendedCoverage(w, artifact)
		return
	}
	fmt.Fprintf(w, "Rows recovered vs journal starts: %d/%d; recovery shortfall=%d\n", c.RecoveredRows, c.JournalStartedRows, c.RecoveryShortfall)
	fmt.Fprintf(w, "Attempts recovered vs journal starts: %d/%d; attempt shortfall=%d; attempts without matching YAML=%d\n", c.RecoveredAttempts, c.JournalStartAttempts, c.AttemptRecoveryShortfall, c.AttemptsWithoutMatchingYAML)
	fmt.Fprintf(w, "YAML snapshots without an exact unambiguous attempt: %d; rows missing join keys: %d\n", c.SnapshotsWithoutMatchingAttempt, c.YAMLRowsMissingJoinKeys)
	fmt.Fprintf(w, "Observations stored (one per recovered attempt): %d\n", c.RecoveredObservations)
	fmt.Fprintf(w, "task_started events: %d across %d rows (%d restarts)\n", c.JournalStartAttempts, c.JournalStartedRows, c.JournalRestarts)
	fmt.Fprintf(w, "task_started events carrying no planned model: %d\n", c.TaskStartsWithoutModel)
	fmt.Fprintf(w, "Attempts no implementing spawn stamped a model on: %d\n", c.JournalAttemptsWithoutStamp)
	fmt.Fprintf(w, "Joined rows still unattributable: %d\n", c.UnattributableJoinedRows)
	fmt.Fprintf(w, "Joined rows unrecoverable after validation: %d\n", c.UnrecoverableJoinedRows)
	fmt.Fprintf(w, "Journal run/task keys with no recoverable YAML: %d\n", c.JournalRunTasksWithoutYAML)
	fmt.Fprintf(w, "Journal lines unparsed: %d; events with an unreadable timestamp: %d\n", c.JournalLinesUnparsed, c.JournalEventsWithBadTimestamp)
	fmt.Fprintf(w, "Authored/stamped model disagreements (attributed to stamp): %d\n", c.AuthoredStampMismatches)
	fmt.Fprintf(w, "Recovered rows that cascaded to another agent: %d\n", c.RowsWithCascade)
	fmt.Fprintf(w, "Recovered rows with no recorded cost: %d\n", c.RowsWithoutRecordedCost)
	fmt.Fprintf(w, "Recovered rows whose terminal evidence is YAML only: %d\n", c.RowsWithYAMLOnlyTerminalEvidence)
	fmt.Fprintf(w, "Stamp conflicts excluded: %d\n", c.StampConflictRows)
	fmt.Fprintf(w, "YAML readings: live=%d historical=%d over %d commits / %d blobs (truncated=%t)\n",
		c.LiveYAMLReadings, c.HistoricalYAMLReadings, c.HistoryCommits, c.HistoryBlobs, c.HistoryTruncated)
	fmt.Fprintf(w, "YAML documents that would not parse: %d; rows with an unreadable timestamp: %d\n", c.UnparseableYAMLDocs, c.MalformedYAMLRows)
	for _, limit := range artifact.Limits {
		fmt.Fprintf(w, "Limit: %s\n", limit)
	}
}

func writeAmendedCoverage(w io.Writer, artifact Artifact) {
	manifest := artifact.SourceManifest
	evidence := artifact.Evidence
	fmt.Fprintf(w, "Source manifest: state=%s cutoff=%s allow_empty=%t holdouts=%d\n",
		manifest.State, manifest.Cutoff.Format(time.RFC3339Nano), manifest.AllowEmpty, len(manifest.HoldoutRunIDs))
	for _, source := range manifest.Sources {
		resolved := source.ResolvedRef
		if resolved == "" && len(source.ResolvedRefs) > 0 {
			resolved = fmt.Sprintf("%d recorded refs", len(source.ResolvedRefs))
		}
		fmt.Fprintf(w, "Source %s: kind=%s repository=%s roots=%v requested_ref=%q resolved=%q state=%s records=%d journals=%d malformed=%d unreadable=%d bounds=%d\n",
			source.ID, source.Kind, source.Repository, source.Roots, source.RequestedRef, resolved, source.State,
			source.Counts.Records, source.Counts.Journals, source.Counts.Malformed, source.Counts.Unreadable, source.Counts.BoundsExceeded)
		for _, reason := range source.Reasons {
			fmt.Fprintf(w, "  Source reason: %s\n", reason)
		}
	}
	for _, reason := range manifest.Reasons {
		fmt.Fprintf(w, "Manifest reason: %s\n", reason)
	}
	fmt.Fprintf(w, "Rows recovered vs journal starts: %d/%d; recovery shortfall=%d\n",
		artifact.Coverage.RecoveredRows, evidence.UniqueRows, artifact.Coverage.RecoveryShortfall)
	fmt.Fprintf(w, "Attempts recovered vs counted attempts: %d/%d; not-recovered attempts=%d\n",
		evidence.Recovered, evidence.Attempts, len(evidence.LostAttempts))
	fmt.Fprintf(w, "YAML readings examined: %d; excluded journals retained: %d; starts after cutoff: %d\n",
		len(evidence.Examined), len(evidence.ExcludedJournals), evidence.StartsAfterCutoff)
	for _, count := range evidence.Dispositions {
		fmt.Fprintf(w, "Disposition %s: %d\n", count.Disposition, count.Count)
	}
	fmt.Fprintf(w, "Conflicting attempts: %d; ambiguous attempts: %d; YAML-only terminals: %d\n",
		len(evidence.Conflicts), len(evidence.Ambiguous), evidence.RowsWithYAMLOnlyTerminal)
	for _, limit := range artifact.Limits {
		fmt.Fprintf(w, "Limit: %s\n", limit)
	}
}

func formatDurationSummary(s NumericSummary) string {
	if s.N == 0 {
		return "n=0"
	}
	return fmt.Sprintf("%.0fs/%.0fs/%.0fs/%.0fs", s.Mean, s.Median, s.Min, s.Max)
}

func formatRoundsSummary(s NumericSummary) string {
	if s.N == 0 {
		return "n=0"
	}
	return fmt.Sprintf("%.2f/%.0f/%.0f", s.Mean, s.Min, s.Max)
}
