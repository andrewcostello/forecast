package dispatched

// sources.go: filesystem and Git reads of explicit sources.
//
// Ownership: FC-SOURCES implements the frozen seam in this file
// (ReadSources) and may replace the baseline readers. The baseline readers
// are the FC-1 code moved verbatim from extract.go; Build still runs them
// until FC-1 switches Build to ReadSources, so the artifact does not change
// under the move. Source discovery and Git reads live here; legacy YAML decoding stays in extract.go, while
// the amended per-row parser is owned here by FC-SOURCES.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Frozen contract (F3, with the F6 selection inputs).
// ---------------------------------------------------------------------------

// Read bounds. The commit, line, blob and total-byte caps are applied BEFORE
// the bounded quantity is collected, so a cap stops a read rather than
// trimming its result; a read that hits one of them is PARTIAL, never
// COMPLETE, and is counted in SourceCounts.BoundsExceeded. MaxProcesses is
// different in kind: it is a serializer, not a cap on the data. It limits
// how many git children are in flight per source, a read that would exceed
// it waits for a slot rather than spawning, and it never stops a read, never
// sets BoundsExceeded, never wraps ErrBoundExceeded and never changes counts,
// order or SourceState.
const (
	// defaultMaxHistoryCommits bounds the history walk. The walk is linear in
	// commits and, after blob de-duplication, in distinct file contents; the
	// cap exists so an unexpectedly large repository fails loudly and
	// countably rather than running until it is killed.
	defaultMaxHistoryCommits = 5000
	// DefaultMaxCommits is the amended history bound; the legacy default above stays fixed.
	DefaultMaxCommits = 5000
	// DefaultMaxLineBytes is the longest journal line that is decoded; the
	// baseline scanner used this value.
	DefaultMaxLineBytes = 16 * 1024 * 1024
	// DefaultMaxBlobBytes is the largest single YAML blob that is read.
	DefaultMaxBlobBytes = 16 * 1024 * 1024
	// DefaultMaxTotalBytes caps input bytes read from one source across
	// journals, blobs and files, including metadata retained for enumeration.
	DefaultMaxTotalBytes = 512 * 1024 * 1024
	// DefaultMaxProcesses is the serializer width: the largest number of git
	// children in flight per source at one instant. Excess reads wait.
	DefaultMaxProcesses = 2
)

// ReadBounds are the explicit, repeatable caps for one extraction. Zero
// means the default above. A negative value is ErrInvalidSourceSpec.
// MaxCommits, MaxLineBytes, MaxBlobBytes and MaxTotalBytes stop a read and
// mark the source PARTIAL; MaxProcesses only serialises git children (see
// the block comment above) and never affects the manifest.
type ReadBounds struct {
	MaxCommits    int   `json:"max_commits"`
	MaxLineBytes  int   `json:"max_line_bytes"`
	MaxBlobBytes  int64 `json:"max_blob_bytes"`
	MaxTotalBytes int64 `json:"max_total_bytes"`
	MaxProcesses  int   `json:"max_processes"`
}

// Validate wraps ErrInvalidSourceSpec for any negative bound when FC-SOURCES
// fills this named stub. Zero fields mean their documented defaults.
func (b ReadBounds) Validate() error {
	if b.MaxCommits < 0 || b.MaxLineBytes < 0 || b.MaxBlobBytes < 0 ||
		b.MaxTotalBytes < 0 || b.MaxProcesses < 0 {
		return fmt.Errorf("%w: read bounds must not be negative", ErrInvalidSourceSpec)
	}
	return nil
}

// SourceKind is what a source contributes.
type SourceKind string

const (
	// SourceKindJournals: a directory whose direct children are dispatcher
	// run directories holding journal.jsonl.
	SourceKindJournals SourceKind = "journals"
	// SourceKindLiveYAML: task YAML files under explicit roots of a working
	// tree, as checked out.
	SourceKindLiveYAML SourceKind = "live_yaml"
	// SourceKindGitHistory: every task YAML blob reachable from the requested
	// ref (default: all refs) under explicit roots, including superseded
	// merge parents and deleted or renamed files.
	SourceKindGitHistory SourceKind = "git_history"
)

// Valid reports whether k is declared.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceKindJournals, SourceKindLiveYAML, SourceKindGitHistory:
		return true
	}
	return false
}

// SourceSpec is one explicit, repeatable source. There is no personal-home
// default anywhere: every repository and root is supplied by the caller.
//
// Repository is the working tree (live) or git directory (history) path.
// Roots are relative paths under it that are scanned; nothing outside them
// is read, which is how paths such as dispatcher/ are supported without
// scanning unrelated files. Ref is the requested revision for history
// ("" means --all). For journals, Repository is the runs directory and Roots
// is empty.
type SourceSpec struct {
	ID         string
	Kind       SourceKind
	Repository string
	Roots      []string
	Ref        string
}

// Validate wraps ErrInvalidSourceSpec for a blank ID or repository, an
// undeclared kind, missing roots on a YAML source, a blank/absolute/escaping
// root, roots on a journal source, or nonempty Ref on non-history kinds. FC-SOURCES fills this named stub.
func (s SourceSpec) Validate() error {
	if strings.TrimSpace(s.ID) == "" || s.ID != strings.TrimSpace(s.ID) {
		return fmt.Errorf("%w: source ID %q is blank or padded", ErrInvalidSourceSpec, s.ID)
	}
	if strings.TrimSpace(s.Repository) == "" {
		return fmt.Errorf("%w: source %q has no repository", ErrInvalidSourceSpec, s.ID)
	}
	if !s.Kind.Valid() {
		return fmt.Errorf("%w: source %q has kind %q", ErrInvalidSourceSpec, s.ID, s.Kind)
	}
	if s.Kind != SourceKindGitHistory && s.Ref != "" {
		return fmt.Errorf("%w: source %q kind %q cannot select ref %q", ErrInvalidSourceSpec, s.ID, s.Kind, s.Ref)
	}
	if s.Kind == SourceKindJournals {
		if len(s.Roots) != 0 {
			return fmt.Errorf("%w: journal source %q cannot have roots", ErrInvalidSourceSpec, s.ID)
		}
		return nil
	}
	if len(s.Roots) == 0 {
		return fmt.Errorf("%w: YAML source %q has no roots", ErrInvalidSourceSpec, s.ID)
	}
	seen := make(map[string]struct{}, len(s.Roots))
	for _, root := range s.Roots {
		if strings.TrimSpace(root) == "" || filepath.IsAbs(root) {
			return fmt.Errorf("%w: source %q has invalid root %q", ErrInvalidSourceSpec, s.ID, root)
		}
		clean := filepath.Clean(root)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: source %q root %q escapes its repository", ErrInvalidSourceSpec, s.ID, root)
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("%w: source %q repeats root %q", ErrInvalidSourceSpec, s.ID, root)
		}
		seen[clean] = struct{}{}
	}
	return nil
}

// SourceState is the completeness of one source or of the whole manifest.
// A failed discovery is an error, not a state.
type SourceState string

const (
	// SourceComplete: every requested record was enumerated and read within
	// bounds, history is not shallow, nothing was cancelled.
	SourceComplete SourceState = "COMPLETE"
	// SourcePartial: enumeration/read is incomplete because at least one of: a bound was
	// hit, history is shallow/grafted/replaced, a record was unreadable or
	// malformed, or the read was cancelled. Valid records are retained.
	SourcePartial SourceState = "PARTIAL"
	// SourceEmpty: aggregate discovery successfully found zero journal files
	// in AllowEmpty diagnostic mode. Source reports themselves are COMPLETE/PARTIAL;
	// zero-record sources can be COMPLETE. An EMPTY manifest is never eligible.
	SourceEmpty SourceState = "EMPTY"
)

// SourceCounts are the per-source tallies the manifest stores.
// Journals counts discovered journal files BEFORE holdout exclusion, not parsed
// or in-sample journals. JournalsExcludedByHoldout counts the held-out files;
// ExcludedByHoldout counts YAML envelopes only. All-held-out discovery can be
// COMPLETE; eligibility then fails for thin cells, not incomplete sources.
type SourceCounts struct {
	JournalsExcludedByHoldout int   `json:"journals_excluded_by_holdout"`
	Files                     int   `json:"files"`
	Journals                  int   `json:"journals"`
	Commits                   int   `json:"commits"`
	Blobs                     int   `json:"blobs"`
	Bytes                     int64 `json:"bytes"`
	Records                   int   `json:"records"`
	NonTaskDocuments          int   `json:"non_task_documents"`
	MalformedExcluded         int   `json:"malformed_excluded"`
	UnreadableExcluded        int   `json:"unreadable_excluded"`
	Malformed                 int   `json:"malformed"`
	Unreadable                int   `json:"unreadable"`
	ExcludedByHoldout         int   `json:"excluded_by_holdout"`
	ExcludedAfterCutoff       int   `json:"excluded_after_cutoff"`
	// BoundsExceeded counts the commit, line, blob and total-byte caps that
	// stopped a read of this source. MaxProcesses never contributes to it.
	BoundsExceeded int `json:"bounds_exceeded"`
}

type ResolvedRef struct {
	Name   string `json:"name"`
	Commit string `json:"commit"`
}

// SourceReport is one source's identity, what was asked for, what was
// actually resolved and how the read ended. For explicit Ref, ResolvedRef is
// its commit and ResolvedRefs contains that single name/commit pair. For --all,
// ResolvedRef is empty and ResolvedRefs is the sorted complete ref-tip list.
type SourceReport struct {
	Grafted           bool          `json:"grafted"`
	Replaced          bool          `json:"replaced"`
	ProducerUncertain bool          `json:"producer_uncertain"`
	ID                string        `json:"id"`
	Kind              SourceKind    `json:"kind"`
	Repository        string        `json:"repository"`
	Roots             []string      `json:"roots"`
	RequestedRef      string        `json:"requested_ref,omitempty"`
	ResolvedRef       string        `json:"resolved_ref,omitempty"`
	ResolvedRefs      []ResolvedRef `json:"resolved_refs"`
	State             SourceState   `json:"state"`
	Reasons           []string      `json:"reasons"`
	Shallow           bool          `json:"shallow"`
	Cancelled         bool          `json:"cancelled"`
	Counts            SourceCounts  `json:"counts"`
}

// Selection freezes, before extraction, which evidence is excluded: runs
// held out for validation and the extraction cutoff. Evidence from a
// held-out run is excluded at the source boundary for BOTH live and
// historical joins; evidence after Cutoff is excluded and counted.
//
// AllowEmpty permits a zero-journal diagnostic manifest instead of ErrSourceEmpty.
// A successfully enumerated YAML source may have zero task rows without error. An EMPTY or
// PARTIAL manifest is never prediction-eligible.
type Selection struct {
	Cutoff        time.Time
	HoldoutRunIDs []string
	AllowEmpty    bool
}

// Validate requires a nonzero Cutoff and wraps ErrInvalidSelection for a held-out run ID that is blank,
// that differs from its whitespace-trimmed form, or that is listed twice.
// Rejecting an untrimmed ID (rather than trimming it) keeps the stored list
// identical to the compared one, so a holdout can never pass validation and
// then fail to match the run it names. FC-SOURCES fills this named stub.
func (s Selection) Validate() error {
	if s.Cutoff.IsZero() {
		return fmt.Errorf("%w: selection cutoff is zero", ErrInvalidSelection)
	}
	seen := make(map[string]struct{}, len(s.HoldoutRunIDs))
	for _, runID := range s.HoldoutRunIDs {
		if strings.TrimSpace(runID) == "" || runID != strings.TrimSpace(runID) {
			return fmt.Errorf("%w: held-out run ID %q is blank or padded", ErrInvalidSelection, runID)
		}
		if _, ok := seen[runID]; ok {
			return fmt.Errorf("%w: held-out run ID %q is repeated", ErrInvalidSelection, runID)
		}
		seen[runID] = struct{}{}
	}
	return nil
}

// UnmatchedHoldouts wraps ErrInvalidSelection when any held-out run ID names
// no run in discoveredRunIDs, the run IDs of every journal found across all
// journal sources BEFORE exclusion. A holdout that matches nothing is a
// misspelling, and silently ignoring it would put the run it meant to hold
// out into the corpus the artifact is then used to predict. ReadSources
// calls this once discovery is complete. This is a named stub for FC-SOURCES.
func (s Selection) UnmatchedHoldouts(discoveredRunIDs []string) error {
	if err := s.Validate(); err != nil {
		return err
	}
	discovered := make(map[string]struct{}, len(discoveredRunIDs))
	for _, runID := range discoveredRunIDs {
		if runID != "" {
			discovered[runID] = struct{}{}
		}
	}
	for _, runID := range s.HoldoutRunIDs {
		if _, ok := discovered[runID]; !ok {
			return fmt.Errorf("%w: held-out run %q was not discovered", ErrInvalidSelection, runID)
		}
	}
	return nil
}

// SourceManifest is the record of every source supplied to one extraction.
// No claim extends beyond it.
type SourceManifest struct {
	// Reasons is the canonical sorted, duplicate-free aggregate diagnostic list.
	// FC-1 adds "reduce: <sentinel-name>: <detail>" or "join: <sentinel-name>: <detail>"
	// and PARTIAL state for cross-source failures, without falsifying source reports.
	Reasons       []string       `json:"reasons"`
	Sources       []SourceReport `json:"sources"`
	Cutoff        time.Time      `json:"cutoff"`
	HoldoutRunIDs []string       `json:"holdout_run_ids"`
	AllowEmpty    bool           `json:"allow_empty"`
	// Bounds stores effective positive values, never unresolved zero defaults.
	Bounds ReadBounds  `json:"bounds"`
	State  SourceState `json:"state"`
}

// ValidateComplete is nil-safe and wraps ErrSourceIncomplete, plus
// ErrShallowHistory/ErrBoundExceeded when those facts apply. Excluded quality
// counters are diagnostic only. The authoritative predicate is in the handoff's
// "Entry-point contracts" and F3-COMPLETENESS-CAUSES. FC-SOURCES body.
func (m *SourceManifest) ValidateComplete() error {
	var incomplete, shallow, bounded bool
	if m == nil {
		return fmt.Errorf("%w: manifest is nil", ErrSourceIncomplete)
	}
	if m.State != SourceComplete || len(m.Sources) == 0 || m.Cutoff.IsZero() ||
		m.Bounds.MaxCommits <= 0 || m.Bounds.MaxLineBytes <= 0 ||
		m.Bounds.MaxBlobBytes <= 0 || m.Bounds.MaxTotalBytes <= 0 || m.Bounds.MaxProcesses <= 0 {
		incomplete = true
	}
	if err := (Selection{Cutoff: m.Cutoff, HoldoutRunIDs: m.HoldoutRunIDs, AllowEmpty: m.AllowEmpty}).Validate(); err != nil {
		incomplete = true
	}
	seen := make(map[string]struct{}, len(m.Sources))
	journalSources, journals := 0, 0
	if len(m.Reasons) != 0 {
		incomplete = true
	}
	for _, source := range m.Sources {
		if source.ID == "" || !source.Kind.Valid() || source.Repository == "" || source.State != SourceComplete {
			incomplete = true
		}
		if len(source.Reasons) != 0 {
			incomplete = true
		}
		if err := (SourceSpec{ID: source.ID, Kind: source.Kind, Repository: source.Repository, Roots: source.Roots, Ref: source.RequestedRef}).Validate(); err != nil {
			incomplete = true
		}
		if source.Kind == SourceKindGitHistory {
			if !validResolvedRefs(source.ResolvedRefs, source.RequestedRef == "") {
				incomplete = true
			}
			if source.RequestedRef == "" && (source.ResolvedRef != "" || len(source.ResolvedRefs) == 0) {
				incomplete = true
			}
			if source.RequestedRef != "" && (!validObjectID(source.ResolvedRef) || len(source.ResolvedRefs) != 1 || source.ResolvedRefs[0].Name != source.RequestedRef || source.ResolvedRefs[0].Commit != source.ResolvedRef) {
				incomplete = true
			}
		} else if source.ResolvedRef != "" || len(source.ResolvedRefs) != 0 {
			incomplete = true
		}
		if _, ok := seen[source.ID]; ok {
			incomplete = true
		}
		seen[source.ID] = struct{}{}
		if source.Kind == SourceKindJournals {
			journalSources++
			journals += source.Counts.Journals
		}
		if source.Shallow || source.Grafted || source.Replaced {
			incomplete, shallow = true, true
		}
		if source.Counts.BoundsExceeded > 0 {
			incomplete, bounded = true, true
		}
		if source.ProducerUncertain || source.Cancelled || source.Counts.Malformed > 0 || source.Counts.Unreadable > 0 || invalidSourceCounts(source.Counts) {
			incomplete = true
		}
	}
	if journalSources == 0 || journals == 0 {
		incomplete = true
	}
	if !incomplete {
		return nil
	}
	errList := []error{fmt.Errorf("%w: manifest does not prove complete selected sources", ErrSourceIncomplete)}
	if shallow {
		errList = append(errList, ErrShallowHistory)
	}
	if bounded {
		errList = append(errList, ErrBoundExceeded)
	}
	return errors.Join(errList...)
}

func validResolvedRefs(refs []ResolvedRef, implicit bool) bool {
	seen := make(map[string]struct{}, len(refs))
	for i, ref := range refs {
		if ref.Name == "" || ref.Name != strings.TrimSpace(ref.Name) || !validObjectID(ref.Commit) {
			return false
		}
		if implicit && ref.Name != "HEAD" && !validFullRefName(ref.Name) {
			return false
		}
		if _, duplicate := seen[ref.Name]; duplicate {
			return false
		}
		seen[ref.Name] = struct{}{}
		if i > 0 {
			previous := refs[i-1]
			if previous.Name > ref.Name || previous.Name == ref.Name && previous.Commit >= ref.Commit {
				return false
			}
		}
	}
	return true
}

// validFullRefName implements the canonical restrictions enforced by
// git-check-ref-format for implicit snapshots. HEAD is handled separately as
// the one authorized pseudoref; every other implicit name must live in refs/.
func validFullRefName(name string) bool {
	if !strings.HasPrefix(name, "refs/") || strings.HasSuffix(name, "/") ||
		strings.HasSuffix(name, ".") || strings.Contains(name, "//") ||
		strings.Contains(name, "..") || strings.Contains(name, "@{") ||
		strings.ContainsAny(name, " ~^:?*[\\") {
		return false
	}
	for _, char := range name {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	for _, component := range strings.Split(name, "/") {
		if component == "" || strings.HasPrefix(component, ".") || strings.HasSuffix(component, ".lock") {
			return false
		}
	}
	return true
}

// ReadingRef identifies one reading of a tasks YAML: the source, the
// repository-relative path, revision, and 1-based task row (zero for document
// envelopes). RecordedAt is the UTC Git committer instant for a historical
// revision or observed file mtime for live input. It is required by amended
// selection; it is recorded metadata, not a guarantee against forged clocks.
// Canonical order is SourceID, Repository, Path, Revision (bytewise strings),
// Row (numeric), then RecordedAt (UTC instant without monotonic state). Identical
// refs compare equal; indistinguishable duplicate envelopes yield one recovered
// and remaining duplicate dispositions, sorted by disposition to avoid input-order
// dependence. This order governs recovery selection, reading lists and YAML ties.
type ReadingRef struct {
	Row        int       `json:"row"`
	RecordedAt time.Time `json:"recorded_at"`
	SourceID   string    `json:"source_id"`
	Repository string    `json:"repository"`
	Path       string    `json:"path"`
	Revision   string    `json:"revision"`
}

// SourceReadings retains every YAML envelope, including non-task documents
// and excluded rows. ReadSources marks Excluded before joining and counts it;
// excluded envelopes carry identity/citation/selection evidence but no predictive
// Snapshot. CompletedAt is retained solely for cutoff validation, never sampling.
// Journals contains only in-sample ParsedJournal values. Held-out identities
// occur only in ExcludedJournals, never even as an empty ParsedJournal in Journals. ReduceAttempts ignores events after cutoff.
// JoinEvidence rechecks selection, emits one audit disposition per envelope,
// and never contributes excluded rows to observations.
type SourceReadings struct {
	Journals         []ParsedJournal
	ExcludedJournals []JournalIdentity
	Readings         []Reading
}

// ReadSources reads explicit specs at Selection.Cutoff with resolved bounds.
// Retained manifest/readings remain meaningful on error. Named errors are
// ErrInvalidSourceSpec, ErrInvalidSelection, ErrSourceMissing, ErrSourceEmpty,
// ErrSourceCancelled (also ctx.Err()), ErrDuplicateJournalRun, ErrJournalSource,
// ErrGitHistory and ErrBoundExceeded as specified by the authoritative F3 rows.
// F3 tables and "Entry-point contracts" in notes/FC-SCAFFOLD.md are authoritative.
// FC-SOURCES body; all amended Git children must use runSourceGit, never the
// inherited-env legacy helpers below.
func ReadSources(ctx context.Context, specs []SourceSpec, selection Selection, bounds ReadBounds) (*SourceManifest, *SourceReadings, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resolved, boundsErr := resolveReadBounds(bounds)
	holdouts := append([]string{}, selection.HoldoutRunIDs...)
	sort.Strings(holdouts)
	manifest := &SourceManifest{
		Reasons:       []string{},
		Sources:       []SourceReport{},
		Cutoff:        selection.Cutoff.Round(0).UTC(),
		HoldoutRunIDs: holdouts,
		AllowEmpty:    selection.AllowEmpty,
		Bounds:        resolved,
		State:         SourcePartial,
	}
	readings := &SourceReadings{Journals: []ParsedJournal{}, ExcludedJournals: []JournalIdentity{}, Readings: []Reading{}}
	if boundsErr != nil {
		return manifest, readings, boundsErr
	}
	if err := selection.Validate(); err != nil {
		return manifest, readings, err
	}
	if len(specs) == 0 {
		return manifest, readings, fmt.Errorf("%w: at least one explicit source is required", ErrInvalidSourceSpec)
	}

	ordered := append([]SourceSpec(nil), specs...)
	seenIDs := make(map[string]struct{}, len(ordered))
	hasJournals := false
	for _, spec := range ordered {
		if err := spec.Validate(); err != nil {
			return manifest, readings, err
		}
		if _, ok := seenIDs[spec.ID]; ok {
			return manifest, readings, fmt.Errorf("%w: duplicate source ID %q", ErrInvalidSourceSpec, spec.ID)
		}
		seenIDs[spec.ID] = struct{}{}
		hasJournals = hasJournals || spec.Kind == SourceKindJournals
	}
	if !hasJournals {
		return manifest, readings, fmt.Errorf("%w: at least one journal source is required", ErrInvalidSourceSpec)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	for _, spec := range ordered {
		roots := append([]string(nil), spec.Roots...)
		sort.Strings(roots)
		manifest.Sources = append(manifest.Sources, SourceReport{
			ID: spec.ID, Kind: spec.Kind, Repository: spec.Repository, Roots: roots,
			RequestedRef: spec.Ref, ResolvedRefs: []ResolvedRef{}, State: SourcePartial,
			Reasons: []string{},
		})
	}
	if err := preflightSources(ordered); err != nil {
		addManifestReason(manifest, err.Error())
		finalizeManifest(manifest)
		return manifest, readings, err
	}
	manifest.State = SourceComplete

	discoveredRuns := make([]string, 0)
	seenRuns := make(map[string]JournalIdentity)
	var duplicateErr error
	for i, spec := range ordered {
		report := &manifest.Sources[i]
		if err := ctx.Err(); err != nil {
			report.Cancelled = true
			report.State = SourcePartial
			addSourceReason(report, "source read cancelled")
			finalizeManifest(manifest)
			return manifest, readings, fmt.Errorf("%w: source %q: %w", ErrSourceCancelled, spec.ID, err)
		}
		budget, err := newSourceBudget(resolved)
		if err != nil {
			addSourceReason(report, err.Error())
			finalizeManifest(manifest)
			return manifest, readings, err
		}
		if err := budget.setFileRoot(spec.Repository); err != nil {
			addSourceReason(report, "cannot open confined source root")
			report.Counts.Unreadable++
			finalizeManifest(manifest)
			return manifest, readings, fmt.Errorf("%w: source %q repository %s: %v", ErrSourceMissing, spec.ID, spec.Repository, err)
		}

		var sourceErr error
		switch spec.Kind {
		case SourceKindJournals:
			sourceErr = readJournalSource(ctx, spec, selection, budget, report, readings, &discoveredRuns, seenRuns, &duplicateErr)
		case SourceKindLiveYAML:
			sourceErr = readLiveSource(ctx, spec, selection, budget, report, readings)
		case SourceKindGitHistory:
			sourceErr = readHistorySource(ctx, spec, selection, budget, report, readings)
		}
		report.Counts.Bytes = budget.bytesRead()
		budget.closeFileRoot()
		if sourceErr != nil {
			if errors.Is(sourceErr, ErrSourceCancelled) || errors.Is(sourceErr, context.Canceled) || errors.Is(sourceErr, context.DeadlineExceeded) {
				report.Cancelled = true
				addSourceReason(report, "source read cancelled")
			}
			report.State = SourcePartial
			addSourceReason(report, sourceErr.Error())
			finalizeManifest(manifest)
			return manifest, readings, sourceErr
		}
		if report.State == "" || report.State == SourcePartial && len(report.Reasons) == 0 {
			report.State = SourceComplete
		}
		canonicalizeSourceReport(report)
	}

	if duplicateErr != nil {
		addManifestReason(manifest, duplicateErr.Error())
		finalizeManifest(manifest)
		return manifest, readings, duplicateErr
	}
	if err := selection.UnmatchedHoldouts(discoveredRuns); err != nil {
		addManifestReason(manifest, err.Error())
		finalizeManifest(manifest)
		return manifest, readings, err
	}
	journalCount := 0
	for _, source := range manifest.Sources {
		if source.Kind == SourceKindJournals {
			journalCount += source.Counts.Journals
		}
	}
	if journalCount == 0 {
		if !selection.AllowEmpty {
			err := fmt.Errorf("%w: no journal.jsonl files in requested sources", ErrSourceEmpty)
			addManifestReason(manifest, err.Error())
			finalizeManifest(manifest)
			return manifest, readings, err
		}
		finalizeManifest(manifest)
		manifest.State = SourceEmpty
	} else {
		finalizeManifest(manifest)
	}
	sortSourceReadings(readings)
	return manifest, readings, nil
}

// sourceBudget is one source's shared byte/process budget. FC-SOURCES may add
// private implementation state here. ReadSources shares it across Git, journal
// and live-file reads; it is never reset per child command.
type sourceBudget struct {
	bounds ReadBounds
	slots  chan struct{}

	mu            sync.Mutex
	bytes         int64
	reservedBytes int64
	totalExceeded bool
	changed       chan struct{}
	fileRoot      *os.Root
	fileRootDir   *os.File
	fileRootPath  string
}

// newSourceBudget validates/resolves bounds before I/O. FC-SOURCES body.
func newSourceBudget(bounds ReadBounds) (*sourceBudget, error) {
	resolved, err := resolveReadBounds(bounds)
	if err != nil {
		return nil, err
	}
	return &sourceBudget{
		bounds:  resolved,
		slots:   make(chan struct{}, resolved.MaxProcesses),
		changed: make(chan struct{}),
	}, nil
}

// SourceGitRequest distinguishes a single raw blob from metadata. Blob requests
// are exactly cat-file blob <full-object-id>; metadata permits only the fixed
// read-only traversal/ref-inspection commands listed in the handoff. No batch,
// filtered output, arbitrary subcommand, helper, network or shell execution.
type SourceGitRequest struct {
	Args []string
	Blob bool
}

// runSourceGit is the ONLY amended Git execution entry point. It installs the
// isolated command environment, shares sourceBudget slots/total bytes, streams
// stdout, caps each Blob request before buffering, and bounds stderr. Returned
// Read/Close surface exit/cancel/bound errors; Close terminates/reaps and releases
// slots on every path. EOF cannot hide a nonzero child exit. See F3-GIT-RUNNER.
func runSourceGit(ctx context.Context, repo string, budget *sourceBudget, request SourceGitRequest) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if budget == nil {
		return nil, fmt.Errorf("%w: runSourceGit has nil budget", ErrInvalidSourceSpec)
	}
	if err := validateSourceGitRequest(request); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: git source: %w", ErrSourceCancelled, err)
	}
	select {
	case budget.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: waiting for Git process slot: %w", ErrSourceCancelled, ctx.Err())
	}
	internalCtx, cancel := context.WithCancel(ctx)
	cmd, err := sourceGitCommand(internalCtx, repo, request.Args...)
	if err != nil {
		cancel()
		<-budget.slots
		return nil, err
	}
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		cancel()
		<-budget.slots
		return nil, fmt.Errorf("%w: open Git stdout: %v", ErrGitHistory, err)
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		cancel()
		<-budget.slots
		return nil, fmt.Errorf("%w: open Git stderr: %v", ErrGitHistory, err)
	}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		cancel()
		<-budget.slots
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: start git %s in %s: %w", ErrSourceCancelled, strings.Join(request.Args, " "), repo, ctx.Err())
		}
		return nil, fmt.Errorf("%w: start git %s in %s: %v", ErrGitHistory, strings.Join(request.Args, " "), repo, err)
	}
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	reader := &sourceGitReadCloser{
		parentCtx: ctx, ioCtx: internalCtx, cancel: cancel, cmd: cmd,
		stdout: stdout, stderrPipe: stderr, budget: budget,
		repo: repo, args: append([]string(nil), request.Args...),
		stderrDone: make(chan struct{}), processDone: make(chan struct{}),
	}
	if request.Blob {
		reader.localLimit = budget.bounds.MaxBlobBytes
	}
	go reader.drainStderr(stderr)
	go reader.waitForProcess()
	return reader, nil
}

// openSourceFile is the mandatory amended live/journal reader. It opens a local
// file and bounds physical reads by the shared source budget BEFORE retaining
// bytes. ReadSources installs an os.Root descriptor so the open is confined even
// if a parent is replaced concurrently, and final-component symlinks are refused.
// A live YAML file also uses MaxBlobBytes; journals use line/total bounds through
// ParseEvents. Cancellation and byte overflow retain diagnostics and wrap
// ErrSourceCancelled plus ctx.Err(), or ErrBoundExceeded. Close releases the file.
// FC-SOURCES body; nil budget is ErrInvalidSourceSpec. No os.ReadFile fallback.
func openSourceFile(ctx context.Context, path string, budget *sourceBudget, journal bool) (io.ReadCloser, error) {
	return openSourceFileFromParent(ctx, path, budget, journal, nil)
}

// openSourceFileFromParent optionally binds the final parent directory to the
// identity observed during discovery. Every path component is still opened
// relative to the preceding held directory descriptor by openConfined.
func openSourceFileFromParent(ctx context.Context, path string, budget *sourceBudget, journal bool, expectedParent os.FileInfo) (io.ReadCloser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if budget == nil {
		return nil, fmt.Errorf("%w: openSourceFile has nil budget", ErrInvalidSourceSpec)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", ErrSourceCancelled, path, err)
	}
	file, err := budget.openConfined(path, expectedParent)
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", ErrSourceMissing, path, err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err == nil {
			err = fmt.Errorf("not a regular file")
		}
		return nil, fmt.Errorf("%w: open %s: %v", ErrSourceMissing, path, err)
	}
	// The atomic initial open is nonblocking so a regular-file-to-FIFO
	// substitution cannot hang open. Once descriptor metadata proves this is a
	// regular evidence file, restore ordinary blocking read semantics.
	if err := clearSourceFileNonblock(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%w: open %s: clear nonblocking mode: %v", ErrSourceMissing, path, err)
	}
	localLimit := int64(0)
	if !journal {
		localLimit = budget.bounds.MaxBlobBytes
	}
	return &boundedSourceReadCloser{
		ctx: ctx, source: file, file: file, initialInfo: info,
		budget: budget, localLimit: localLimit,
	}, nil
}

// sourceGitCommand constructs an amended read-only Git command with an explicit
// isolated environment: discard all inherited GIT_* except GIT_EXEC_PATH, then
// install the fixed affirmative settings in the handoff. No inherited helpers;
// system/global config disabled, repository pinned. See F3-GIT-ENV-STRIPPED and
// "Entry-point contracts". It does not spawn; runSourceGit applies process/byte
// bounds. Called only by runSourceGit. FC-SOURCES body; legacy helpers remain
// on their original environment and cannot be reused by the amended reader.
func sourceGitCommand(ctx context.Context, repo string, args ...string) (*exec.Cmd, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(repo) == "" {
		return nil, fmt.Errorf("%w: Git repository is blank", ErrInvalidSourceSpec)
	}
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("%w: find git: %v", ErrGitHistory, err)
	}
	argv := []string{
		"--no-pager",
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "protocol.allow=never",
		"-c", "protocol.file.allow=never",
		"-C", repo,
	}
	argv = append(argv, args...)
	cmd := exec.CommandContext(ctx, gitPath, argv...)
	cmd.WaitDelay = time.Second
	execPath := os.Getenv("GIT_EXEC_PATH")
	env := make([]string, 0, len(os.Environ())+12)
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if ok && strings.HasPrefix(key, "GIT_") {
			continue
		}
		env = append(env, item)
	}
	if execPath != "" {
		env = append(env, "GIT_EXEC_PATH="+execPath)
	}
	env = append(env,
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_NO_LAZY_FETCH=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_ALLOW_PROTOCOL=",
		"GIT_SSH_COMMAND=/bin/false",
		"GIT_ASKPASS=/bin/false",
		"GIT_PROXY_COMMAND=/bin/false",
	)
	cmd.Env = env
	return cmd, nil
}

// ValidateReadingRevision requires exactly "live" or "git:" plus a full lowercase
// 40- or 64-hex object ID. No bare/abbreviated SHA or live:<mtime>. Mtime belongs
// only in RecordedAt. ReadSources resolves refs before formatting; this pure
// validator never resolves/fetches. Invalid input wraps ErrUnparseableRevision.
// It narrows the existing legacy ParseRevision grammar without changing it.
func ValidateReadingRevision(revision string) error {
	if revision == "live" {
		return nil
	}
	sha, ok := strings.CutPrefix(revision, "git:")
	if !ok || !validObjectID(sha) {
		return fmt.Errorf("%w: %q", ErrUnparseableRevision, revision)
	}
	return nil
}

// parseReadings copies the source citation and sets Ref.Row per envelope. It
// decodes each YAML tasks-sequence row independently, preserving
// valid siblings of type-invalid rows. Non-task documents yield one
// DocumentNotTasks envelope, Row=0, Err=nil. Invalid document syntax or an
// explicitly malformed tasks value yields DocumentMalformed, Row=0, Err set.
// Valid empty tasks lists yield no row envelopes. Every task row has Row>=1,
// raw join-key presence, and its own parse error. Exclusion is not applied here.
// FC-SOURCES implements it; the top-level error is reserved for an unimplemented
// parser (this scaffold), not an ordinary malformed row/document.
func parseReadings(data []byte, ref ReadingRef) ([]Reading, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return []Reading{{Kind: DocumentMalformed, Ref: ref, Err: fmt.Errorf("%w: decode YAML document: %v", ErrYAMLSource, err)}}, nil
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return []Reading{{Kind: DocumentNotTasks, Ref: ref}}, nil
	}
	tasks, found := yamlMappingValue(root.Content[0], "tasks")
	if !found {
		return []Reading{{Kind: DocumentNotTasks, Ref: ref}}, nil
	}
	if tasks.Kind != yaml.SequenceNode {
		return []Reading{{Kind: DocumentMalformed, Ref: ref, Err: fmt.Errorf("%w: tasks must be a sequence", ErrYAMLSource)}}, nil
	}
	out := make([]Reading, 0, len(tasks.Content))
	for i, row := range tasks.Content {
		rowRef := ref
		rowRef.Row = i + 1
		reading := Reading{Kind: DocumentTaskRow, Ref: rowRef}
		if row.Kind != yaml.MappingNode {
			reading.Err = fmt.Errorf("%w: tasks row %d is not a mapping", ErrYAMLSource, i+1)
			out = append(out, reading)
			continue
		}
		var rowErrors []error
		decodeIdentityField(row, "key", &reading.Present.Key, func(value string) {
			reading.Identity.Key = Known(value)
		}, &rowErrors)
		decodeIdentityField(row, "dispatcher_run_id", &reading.Present.RunID, func(value string) {
			reading.Identity.RunID = Known(value)
		}, &rowErrors)
		decodeTimeField(row, "started_at", &reading.Present.StartedAt, func(value time.Time) {
			reading.Identity.StartedAt = Known(value)
		}, &rowErrors)
		var completedPresent bool
		decodeTimeField(row, "completed_at", &completedPresent, func(value time.Time) {
			reading.CompletedAt = Known(value)
		}, &rowErrors)
		decodePredictiveFields(row, &reading, &rowErrors)
		reading.Err = errors.Join(rowErrors...)
		out = append(out, reading)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// FC-SOURCES implementation details.
// ---------------------------------------------------------------------------

func resolveReadBounds(bounds ReadBounds) (ReadBounds, error) {
	if err := bounds.Validate(); err != nil {
		return ReadBounds{}, err
	}
	if bounds.MaxCommits == 0 {
		bounds.MaxCommits = DefaultMaxCommits
	}
	if bounds.MaxLineBytes == 0 {
		bounds.MaxLineBytes = DefaultMaxLineBytes
	}
	if bounds.MaxBlobBytes == 0 {
		bounds.MaxBlobBytes = DefaultMaxBlobBytes
	}
	if bounds.MaxTotalBytes == 0 {
		bounds.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if bounds.MaxProcesses == 0 {
		bounds.MaxProcesses = DefaultMaxProcesses
	}
	return bounds, nil
}

func invalidSourceCounts(counts SourceCounts) bool {
	return counts.JournalsExcludedByHoldout < 0 || counts.Files < 0 || counts.Journals < 0 ||
		counts.Commits < 0 || counts.Blobs < 0 || counts.Bytes < 0 || counts.Records < 0 ||
		counts.NonTaskDocuments < 0 || counts.MalformedExcluded < 0 || counts.UnreadableExcluded < 0 ||
		counts.Malformed < 0 || counts.Unreadable < 0 || counts.ExcludedByHoldout < 0 ||
		counts.ExcludedAfterCutoff < 0 || counts.BoundsExceeded < 0 ||
		counts.JournalsExcludedByHoldout > counts.Journals
}

func preflightSources(specs []SourceSpec) error {
	for _, spec := range specs {
		info, err := os.Stat(spec.Repository)
		if err != nil || !info.IsDir() {
			if err == nil {
				err = fmt.Errorf("not a directory")
			}
			return fmt.Errorf("%w: source %q repository %s: %v", ErrSourceMissing, spec.ID, spec.Repository, err)
		}
		if spec.Kind != SourceKindLiveYAML {
			continue
		}
		for _, root := range spec.Roots {
			path := filepath.Join(spec.Repository, filepath.Clean(root))
			rootInfo, err := os.Lstat(path)
			if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
				if err == nil {
					err = fmt.Errorf("not a real directory")
				}
				return fmt.Errorf("%w: source %q root %s: %v", ErrSourceMissing, spec.ID, root, err)
			}
		}
	}
	return nil
}

func (b *sourceBudget) setFileRoot(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(abs)
	if err != nil {
		return err
	}
	// Derive the starting directory descriptor from the already-held Root. No
	// later component walk reopens the root by its ambient filesystem path.
	rootDir, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return err
	}
	rootInfo, err := rootDir.Stat()
	if err != nil || !rootInfo.IsDir() {
		_ = rootDir.Close()
		_ = root.Close()
		if err == nil {
			err = fmt.Errorf("confined source root is not a directory")
		}
		return err
	}
	b.mu.Lock()
	b.fileRoot = root
	b.fileRootDir = rootDir
	b.fileRootPath = filepath.Clean(abs)
	b.mu.Unlock()
	return nil
}

func (b *sourceBudget) closeFileRoot() {
	b.mu.Lock()
	root := b.fileRoot
	rootDir := b.fileRootDir
	b.fileRoot = nil
	b.fileRootDir = nil
	b.fileRootPath = ""
	b.mu.Unlock()
	if rootDir != nil {
		_ = rootDir.Close()
	}
	if root != nil {
		_ = root.Close()
	}
}

func (b *sourceBudget) openConfined(path string, expectedParent os.FileInfo) (*os.File, error) {
	b.mu.Lock()
	rootDir, rootPath := b.fileRootDir, b.fileRootPath
	b.mu.Unlock()
	if rootDir == nil {
		return nil, fmt.Errorf("confined source root is not open")
	}
	rel := filepath.Clean(path)
	if filepath.IsAbs(path) {
		var err error
		rel, err = filepath.Rel(rootPath, path)
		if err != nil {
			return nil, err
		}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return nil, fmt.Errorf("path escapes confined source root")
	}
	parentName, baseName := filepath.Dir(rel), filepath.Base(rel)
	if baseName == "." || baseName == ".." || baseName == "" {
		return nil, fmt.Errorf("source path has no final file name")
	}
	parent := rootDir
	parentOwned := false
	if parentName != "." {
		for _, component := range strings.Split(parentName, string(filepath.Separator)) {
			if component == "" || component == "." || component == ".." {
				if parentOwned {
					_ = parent.Close()
				}
				return nil, fmt.Errorf("invalid source directory component %q", component)
			}
			next, err := openSourceDirNoFollow(parent, component)
			if parentOwned {
				_ = parent.Close()
			}
			if err != nil {
				return nil, err
			}
			parent = next
			parentOwned = true
		}
	}
	if parentOwned {
		defer parent.Close()
	}
	parentInfo, err := parent.Stat()
	if err != nil {
		return nil, err
	}
	if !parentInfo.IsDir() {
		return nil, fmt.Errorf("source parent is not a directory")
	}
	if expectedParent != nil && !os.SameFile(expectedParent, parentInfo) {
		return nil, fmt.Errorf("source parent changed after discovery")
	}
	return openSourceFileNoFollow(parent, baseName)
}

func (b *sourceBudget) confinedFS() (fs.FS, error) {
	b.mu.Lock()
	root := b.fileRoot
	b.mu.Unlock()
	if root == nil {
		return nil, fmt.Errorf("confined source root is not open")
	}
	return root.FS(), nil
}

func (b *sourceBudget) readDirConfined(path string) ([]fs.DirEntry, error) {
	rootFS, err := b.confinedFS()
	if err != nil {
		return nil, err
	}
	return fs.ReadDir(rootFS, filepath.ToSlash(filepath.Clean(path)))
}

func (b *sourceBudget) signalChangeLocked() {
	close(b.changed)
	b.changed = make(chan struct{})
}

// reserveRead atomically reserves bytes before a physical read. The shared
// allowance includes one source-wide probe byte so exact-bound EOF can be
// distinguished from overflow. Callers wait for an in-flight reservation to
// be returned rather than mistaking temporary contention for a bound hit.
func (b *sourceBudget) reserveRead(ctx context.Context, want int) (int, error) {
	if want <= 0 {
		return 0, nil
	}
	probeLimit := b.bounds.MaxTotalBytes
	if probeLimit < int64(^uint64(0)>>1) {
		probeLimit++
	}
	for {
		b.mu.Lock()
		if b.totalExceeded {
			b.mu.Unlock()
			return 0, ErrBoundExceeded
		}
		available := probeLimit - b.bytes - b.reservedBytes
		if available > 0 {
			reserved := int64(want)
			if reserved > available {
				reserved = available
			}
			b.reservedBytes += reserved
			b.mu.Unlock()
			return int(reserved), nil
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("%w: %w", ErrSourceCancelled, ctx.Err())
		case <-changed:
		}
	}
}

func (b *sourceBudget) accountReservation(reserved, n int) (retained int, exceeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if int64(reserved) > b.reservedBytes {
		reserved = int(b.reservedBytes)
	}
	b.reservedBytes -= int64(reserved)
	before := b.bytes
	b.bytes += int64(n)
	allowed := b.bounds.MaxTotalBytes - before
	if allowed < 0 {
		allowed = 0
	}
	retained = n
	if int64(retained) > allowed {
		retained = int(allowed)
		b.totalExceeded = true
	}
	b.signalChangeLocked()
	return retained, b.totalExceeded
}

func (b *sourceBudget) bytesRead() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.bytes
}

func (b *sourceBudget) hitTotalBound() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalExceeded
}

type boundedSourceReadCloser struct {
	ctx         context.Context
	source      io.ReadCloser
	file        *os.File
	initialInfo os.FileInfo
	budget      *sourceBudget
	localLimit  int64
	localRead   int64
	closed      bool
	mu          sync.Mutex
}

func (r *boundedSourceReadCloser) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	n, err := readBoundedSource(r.ctx, r.source, r.budget, r.localLimit, &r.localRead, p, nil)
	if errors.Is(err, io.EOF) {
		if _, stableErr := r.sourceFileInfoLocked(); stableErr != nil {
			return n, stableErr
		}
	}
	return n, err
}

func (r *boundedSourceReadCloser) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	return r.source.Close()
}

func (r *boundedSourceReadCloser) sourceFileInfo() (os.FileInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, os.ErrClosed
	}
	return r.sourceFileInfoLocked()
}

func (r *boundedSourceReadCloser) sourceFileInfoLocked() (os.FileInfo, error) {
	current, err := r.file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(r.initialInfo, current) || r.initialInfo.Mode() != current.Mode() ||
		r.initialInfo.Size() != current.Size() || !r.initialInfo.ModTime().Equal(current.ModTime()) {
		return nil, fmt.Errorf("%w: source file changed while it was read", ErrSourceMissing)
	}
	return r.initialInfo, nil
}

var errSourcePipeIncomplete = errors.New("Git pipe remained open without readable data after child exit")

func waitSourceReadable(ctx context.Context, source io.Reader, budget *sourceBudget, processDone <-chan struct{}) error {
	// Local evidence descriptors have already been fstat-validated as regular.
	// Only Git pipes need readiness polling and post-exit inherited-pipe checks.
	if processDone == nil {
		return nil
	}
	file, ok := source.(*os.File)
	if !ok {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		return nil
	}
	var idleSince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: %w", ErrSourceCancelled, err)
		}
		if budget.hitTotalBound() {
			return ErrBoundExceeded
		}
		ready, err := sourceFileReadReady(file)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		if processDone != nil {
			select {
			case <-processDone:
				if idleSince.IsZero() {
					idleSince = time.Now()
				} else if time.Since(idleSince) >= time.Second {
					return errSourcePipeIncomplete
				}
			default:
			}
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("%w: %w", ErrSourceCancelled, ctx.Err())
		case <-timer.C:
		}
	}
}

func readBoundedSource(ctx context.Context, source io.Reader, budget *sourceBudget, localLimit int64, localRead *int64, p []byte, processDone <-chan struct{}) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("%w: %w", ErrSourceCancelled, err)
	}
	if len(p) == 0 {
		return 0, nil
	}
	want := len(p)
	if localLimit > 0 {
		remaining := localLimit - *localRead
		if remaining < 0 {
			return 0, ErrBoundExceeded
		}
		if remaining < int64(want) {
			want = int(remaining + 1)
		}
	}
	if err := waitSourceReadable(ctx, source, budget, processDone); err != nil {
		return 0, err
	}
	reserved, reserveErr := budget.reserveRead(ctx, want)
	if reserveErr != nil {
		return 0, reserveErr
	}
	n, err := source.Read(p[:reserved])
	retained, totalExceeded := budget.accountReservation(reserved, n)
	localAllowed := n
	if localLimit > 0 {
		remaining := localLimit - *localRead
		if remaining < 0 {
			remaining = 0
		}
		if int64(localAllowed) > remaining {
			localAllowed = int(remaining)
		}
	}
	if retained > localAllowed {
		retained = localAllowed
	}
	*localRead += int64(n)
	localExceeded := localLimit > 0 && *localRead > localLimit
	if totalExceeded || localExceeded {
		return retained, ErrBoundExceeded
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return retained, fmt.Errorf("%w: %w", ErrSourceCancelled, ctxErr)
		}
		return retained, err
	}
	return retained, nil
}

type sourceGitReadCloser struct {
	parentCtx  context.Context
	ioCtx      context.Context
	cancel     context.CancelFunc
	cmd        *exec.Cmd
	stdout     *os.File
	stderrPipe *os.File
	budget     *sourceBudget
	repo       string
	args       []string
	localLimit int64
	localRead  int64

	stderr      bytes.Buffer
	stderrDone  chan struct{}
	processDone chan struct{}
	processErr  error
	stderrErr   error
	finishOnce  sync.Once
	finishErr   error
	readMu      sync.Mutex
	stateMu     sync.Mutex
	readEnded   bool
	closed      bool
}

func (r *sourceGitReadCloser) Read(p []byte) (int, error) {
	r.readMu.Lock()
	defer r.readMu.Unlock()
	r.stateMu.Lock()
	closed := r.closed
	r.stateMu.Unlock()
	if closed {
		return 0, os.ErrClosed
	}
	n, err := readBoundedSource(r.ioCtx, r.stdout, r.budget, r.localLimit, &r.localRead, p, r.processDone)
	if err == nil {
		return n, nil
	}
	finishErr := r.finish(err, !errors.Is(err, io.EOF))
	r.stateMu.Lock()
	r.readEnded = true
	r.stateMu.Unlock()
	return n, finishErr
}

func (r *sourceGitReadCloser) Close() error {
	r.stateMu.Lock()
	if r.closed {
		r.stateMu.Unlock()
		return nil
	}
	r.closed = true
	readEnded := r.readEnded
	r.stateMu.Unlock()
	if !readEnded {
		r.cancel()
		if r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
		}
	}
	r.readMu.Lock()
	defer r.readMu.Unlock()
	err := r.finish(io.ErrClosedPipe, true)
	if readEnded || err == nil || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return nil
	}
	return err
}

func (r *sourceGitReadCloser) drainStderr(stderr io.Reader) {
	defer close(r.stderrDone)
	buffer := make([]byte, 4096)
	var localRead int64
	for {
		n, err := readBoundedSource(r.ioCtx, stderr, r.budget, 0, &localRead, buffer, r.processDone)
		if n > 0 && r.stderr.Len() < 64*1024 {
			keep := n
			if keep > 64*1024-r.stderr.Len() {
				keep = 64*1024 - r.stderr.Len()
			}
			_, _ = r.stderr.Write(buffer[:keep])
		}
		if errors.Is(err, ErrBoundExceeded) {
			r.stderrErr = err
			r.cancel()
			return
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				r.stderrErr = err
			}
			return
		}
	}
}

// Wait begins immediately after Start so process reaping never depends on a
// caller reaching Read or Close. Pipe reads distinguish buffered data from a
// post-exit idle descriptor; healthy buffered output is never timer-discarded.
func (r *sourceGitReadCloser) waitForProcess() {
	r.processErr = r.cmd.Wait()
	close(r.processDone)
}

func (r *sourceGitReadCloser) finish(cause error, kill bool) error {
	r.finishOnce.Do(func() {
		if kill {
			r.cancel()
			if r.cmd.Process != nil {
				_ = r.cmd.Process.Kill()
			}
		}
		<-r.processDone
		<-r.stderrDone
		r.cancel()
		_ = r.stdout.Close()
		_ = r.stderrPipe.Close()
		<-r.budget.slots
		switch {
		case r.budget.hitTotalBound() || errors.Is(cause, ErrBoundExceeded):
			r.finishErr = fmt.Errorf("%w: git %s in %s", ErrBoundExceeded, strings.Join(r.args, " "), r.repo)
		case r.parentCtx.Err() != nil:
			r.finishErr = fmt.Errorf("%w: git %s in %s: %w", ErrSourceCancelled, strings.Join(r.args, " "), r.repo, r.parentCtx.Err())
		case r.processErr != nil:
			detail := strings.TrimSpace(r.stderr.String())
			if detail != "" {
				r.finishErr = fmt.Errorf("%w: git %s in %s: %w: %s", ErrGitHistory, strings.Join(r.args, " "), r.repo, r.processErr, detail)
			} else {
				r.finishErr = fmt.Errorf("%w: git %s in %s: %w", ErrGitHistory, strings.Join(r.args, " "), r.repo, r.processErr)
			}
		case r.stderrErr != nil && !(errors.Is(cause, io.ErrClosedPipe) && errors.Is(r.stderrErr, ErrSourceCancelled)):
			r.finishErr = fmt.Errorf("%w: git %s in %s ended with incomplete stderr: %w", ErrGitHistory, strings.Join(r.args, " "), r.repo, r.stderrErr)
		case errors.Is(cause, io.EOF):
			r.finishErr = io.EOF
		case errors.Is(cause, io.ErrClosedPipe):
			r.finishErr = nil
		default:
			detail := strings.TrimSpace(r.stderr.String())
			if detail != "" {
				r.finishErr = fmt.Errorf("%w: git %s in %s: %v: %s", ErrGitHistory, strings.Join(r.args, " "), r.repo, cause, detail)
			} else {
				r.finishErr = fmt.Errorf("%w: git %s in %s: %v", ErrGitHistory, strings.Join(r.args, " "), r.repo, cause)
			}
		}
	})
	return r.finishErr
}

const (
	sourceRefFormat           = "%(refname)%00%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)"
	sourceHistoryTipBatchSize = 128
)

func validateSourceGitRequest(request SourceGitRequest) error {
	if len(request.Args) == 0 {
		return fmt.Errorf("%w: empty Git request", ErrInvalidSourceSpec)
	}
	for _, arg := range request.Args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			return fmt.Errorf("%w: invalid Git argument", ErrInvalidSourceSpec)
		}
	}
	if request.Blob {
		if len(request.Args) != 3 || request.Args[0] != "cat-file" || request.Args[1] != "blob" || !validObjectID(request.Args[2]) {
			return fmt.Errorf("%w: blob request must be cat-file blob <full-object-id>", ErrInvalidSourceSpec)
		}
		return nil
	}
	valid := false
	switch request.Args[0] {
	case "rev-parse":
		valid = validSourceRevParse(request.Args)
	case "for-each-ref":
		valid = validSourceForEachRef(request.Args)
	case "rev-list":
		valid = validSourceRevList(request.Args)
	case "ls-tree":
		valid = validSourceLSTree(request.Args)
	case "show":
		// The accepted legacy form has no empty trailing operand. Keep it
		// spelled explicitly rather than opening the full `show` option space.
		valid = len(request.Args) == 5 && request.Args[1] == "-s" && request.Args[2] == "--format=%cI" &&
			validObjectID(request.Args[3]) && request.Args[4] == "--"
	case "diff-tree":
		valid = len(request.Args) == 5 && request.Args[1] == "--no-ext-diff" &&
			request.Args[2] == "--no-textconv" && request.Args[3] == "-r" && validObjectID(request.Args[4])
	case "symbolic-ref":
		valid = len(request.Args) == 3 && request.Args[1] == "--quiet" && request.Args[2] == "HEAD"
	case "show-ref":
		valid = validSourceShowRef(request.Args)
	}
	if !valid {
		return fmt.Errorf("%w: Git metadata request %q is not an accepted read-only form", ErrInvalidSourceSpec, strings.Join(request.Args, " "))
	}
	return nil
}

func validSourceRevParse(args []string) bool {
	if len(args) == 2 {
		switch args[1] {
		case "HEAD", "--is-shallow-repository", "--absolute-git-dir", "--git-common-dir":
			return true
		}
	}
	if len(args) == 5 && args[1] == "--verify" && args[2] == "--quiet" && args[3] == "--end-of-options" {
		return args[4] != ""
	}
	return len(args) == 4 && args[1] == "--verify" && args[2] == "--end-of-options" && args[3] != ""
}

func validSourceForEachRef(args []string) bool {
	if len(args) < 2 || len(args) > 3 {
		return false
	}
	if args[1] != "--format=%(refname)" && args[1] != "--format="+sourceRefFormat {
		return false
	}
	return len(args) == 2 || args[2] == "refs/replace"
}

func validSourceRevList(args []string) bool {
	if len(args) < 2 {
		return false
	}
	i := 1
	topo, parents := false, false
	for i < len(args) {
		switch args[i] {
		case "--topo-order":
			if topo {
				return false
			}
			topo = true
			i++
		case "--parents":
			if parents {
				return false
			}
			parents = true
			i++
		default:
			goto optionsDone
		}
	}

optionsDone:
	if i < len(args) && strings.HasPrefix(args[i], "--max-count=") {
		value := strings.TrimPrefix(args[i], "--max-count=")
		count, err := strconv.Atoi(value)
		if err != nil || count <= 0 {
			return false
		}
		i++
	}
	if i < len(args) && args[i] == "--format=%cI" {
		i++
	}
	if i >= len(args) {
		return false
	}
	if args[i] == "--all" {
		return i+1 == len(args)
	}
	if len(args)-i > sourceHistoryTipBatchSize {
		return false
	}
	for ; i < len(args); i++ {
		if !validObjectID(args[i]) {
			return false
		}
	}
	return true
}

func validSourceLSTree(args []string) bool {
	if len(args) < 7 || args[1] != "-r" || args[2] != "-z" || args[3] != "--full-tree" ||
		!validObjectID(args[4]) || args[5] != "--" {
		return false
	}
	for _, root := range args[6:] {
		clean := filepath.Clean(root)
		if root == "" || filepath.IsAbs(root) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

func validSourceShowRef(args []string) bool {
	return len(args) == 4 && args[1] == "--verify" && args[2] == "--quiet" && validFullRefName(args[3])
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func yamlMappingValue(mapping *yaml.Node, name string) (*yaml.Node, bool) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == name {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

func strictYAMLString(node *yaml.Node) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", fmt.Errorf("expected a string scalar")
	}
	return node.Value, nil
}

func decodeIdentityField(row *yaml.Node, name string, present *bool, set func(string), errs *[]error) {
	node, ok := yamlMappingValue(row, name)
	if !ok {
		return
	}
	if node.Kind != yaml.ScalarNode {
		*errs = append(*errs, fmt.Errorf("%s: expected a string scalar", name))
		return
	}
	*present = node.Kind == yaml.ScalarNode && node.Value != ""
	if !*present {
		return
	}
	value, err := strictYAMLString(node)
	if err == nil && (strings.TrimSpace(value) == "" || value != strings.TrimSpace(value)) {
		err = fmt.Errorf("value is blank or padded")
	}
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", name, err))
		return
	}
	set(value)
}

func decodeTimeField(row *yaml.Node, name string, present *bool, set func(time.Time), errs *[]error) {
	node, ok := yamlMappingValue(row, name)
	if !ok {
		return
	}
	if node.Kind != yaml.ScalarNode {
		*errs = append(*errs, fmt.Errorf("%s: expected a string scalar", name))
		return
	}
	*present = node.Kind == yaml.ScalarNode && node.Value != ""
	if !*present {
		return
	}
	value, err := strictYAMLString(node)
	if err == nil {
		var parsed time.Time
		parsed, err = time.Parse(time.RFC3339Nano, value)
		if err == nil {
			set(parsed.Round(0).UTC())
		}
	}
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", name, err))
	}
}

func decodePredictiveFields(row *yaml.Node, reading *Reading, errs *[]error) {
	for _, field := range []struct {
		name string
		set  func(string)
	}{
		{name: "role", set: func(value string) { reading.Snapshot.Role = Role(value) }},
		{name: "model", set: func(value string) { reading.Snapshot.AuthoredModel = value }},
		{name: "status", set: func(value string) { reading.Snapshot.Status = value }},
	} {
		node, ok := yamlMappingValue(row, field.name)
		if !ok || node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
			continue
		}
		value, err := strictYAMLString(node)
		if err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %w", field.name, err))
			continue
		}
		field.set(value)
	}
	if node, ok := yamlMappingValue(row, "iteration_count"); ok && !(node.Kind == yaml.ScalarNode && node.Tag == "!!null") {
		if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
			*errs = append(*errs, fmt.Errorf("iteration_count: expected an integer scalar"))
		} else {
			value, err := strconv.Atoi(node.Value)
			if err != nil {
				*errs = append(*errs, fmt.Errorf("iteration_count: %w", err))
			} else {
				reading.Snapshot.IterationCount = value
			}
		}
	}
}

func readJournalSource(ctx context.Context, spec SourceSpec, selection Selection, budget *sourceBudget, report *SourceReport, readings *SourceReadings, discoveredRuns *[]string, seenRuns map[string]JournalIdentity, duplicateErr *error) error {
	entries, err := budget.readDirConfined(".")
	if err != nil {
		return fmt.Errorf("%w: list journal source %q: %v", ErrSourceMissing, spec.ID, err)
	}
	holdouts := stringSet(selection.HoldoutRunIDs)
	for _, entry := range entries {
		// Direct-child symlinks are never traversed and are always a named
		// incomplete-discovery disposition, including convenience aliases.
		if entry.Type()&os.ModeSymlink != 0 {
			report.State = SourcePartial
			addSourceReason(report, "journal-runs entry is a symbolic link and was not traversed: "+entry.Name())
			continue
		}
		if !entry.IsDir() {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: journal source %q: %w", ErrSourceCancelled, spec.ID, err)
		}
		runID := entry.Name()
		runInfo, infoErr := entry.Info()
		if infoErr != nil || !runInfo.IsDir() || runInfo.Mode()&os.ModeSymlink != 0 {
			if infoErr == nil {
				infoErr = fmt.Errorf("not a real directory")
			}
			return fmt.Errorf("%w: inspect journal run %s: %w", ErrSourceMissing, runID, infoErr)
		}
		relPath := filepath.Join(runID, "journal.jsonl")
		reader, openErr := openSourceFileFromParent(ctx, relPath, budget, true, runInfo)
		if openErr != nil {
			if errors.Is(openErr, fs.ErrNotExist) {
				continue
			}
			if _, heldout := holdouts[runID]; heldout {
				report.Counts.UnreadableExcluded++
				continue
			}
			report.Counts.Unreadable++
			return fmt.Errorf("journal source %q repository %q run %q: %w", spec.ID, spec.Repository, runID, openErr)
		}
		report.Counts.Files++
		report.Counts.Journals++
		*discoveredRuns = append(*discoveredRuns, runID)
		identity := JournalIdentity{RunID: runID, SourceID: spec.ID, Path: filepath.ToSlash(filepath.Join(runID, "journal.jsonl"))}
		first, isDuplicate := seenRuns[runID]
		if isDuplicate {
			err := fmt.Errorf("%w: run %q appears in %s and %s", ErrDuplicateJournalRun, runID, first.SourceID, spec.ID)
			*duplicateErr = errors.Join(*duplicateErr, err)
			addSourceReason(report, err.Error())
			report.State = SourcePartial
		} else {
			seenRuns[runID] = identity
		}
		parsed, parseErr := ParseEvents(ctx, identity, reader, JournalBounds{MaxLineBytes: budget.bounds.MaxLineBytes, MaxTotalBytes: budget.bounds.MaxTotalBytes})
		_ = reader.Close()
		identity = parsed.Journal
		malformed := parsed.Diagnostics.LinesUnparsed + parsed.Diagnostics.BadTimestamps
		boundsHit := parsed.Diagnostics.LinesOverBound
		if parsed.Diagnostics.TotalBoundExceeded {
			boundsHit++
		}
		if budget.hitTotalBound() && !parsed.Diagnostics.TotalBoundExceeded {
			boundsHit++
		}
		_, heldout := holdouts[runID]
		if heldout {
			report.Counts.JournalsExcludedByHoldout++
			report.Counts.MalformedExcluded += malformed
			readings.ExcludedJournals = append(readings.ExcludedJournals, identity)
		} else {
			report.Counts.Malformed += malformed
			report.Counts.Records += len(parsed.Events)
			if parsed.Diagnostics.MissingProducer || parsed.Diagnostics.ProducerConflict || parsed.Journal.Producer != ProducerDispatcherV0_1_0 {
				report.ProducerUncertain = true
				addSourceReason(report, "journal producer is missing, conflicting, or unsupported")
			}
			if !isDuplicate {
				readings.Journals = append(readings.Journals, parsed)
			}
		}
		if malformed > 0 && !heldout {
			report.State = SourcePartial
			addSourceReason(report, "journal contains malformed records")
		}
		if boundsHit > 0 {
			report.Counts.BoundsExceeded += boundsHit
			report.State = SourcePartial
			addSourceReason(report, "journal byte or line bound exceeded")
		}
		if parseErr != nil {
			if errors.Is(parseErr, ErrSourceCancelled) || ctx.Err() != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("%w: journal source %q: %w", ErrSourceCancelled, spec.ID, ctx.Err())
				}
				return parseErr
			}
			if heldout {
				report.Counts.UnreadableExcluded++
				continue
			}
			if boundsHit == 0 {
				report.Counts.Unreadable++
				return parseErr
			}
		}
		if budget.hitTotalBound() {
			break
		}
	}
	if report.State == SourcePartial && len(report.Reasons) == 0 {
		report.State = SourceComplete
	}
	return nil
}

func readLiveSource(ctx context.Context, spec SourceSpec, selection Selection, budget *sourceBudget, report *SourceReport, readings *SourceReadings) error {
	paths := make([]string, 0)
	seen := make(map[string]struct{})
	rootFS, err := budget.confinedFS()
	if err != nil {
		return fmt.Errorf("%w: live source %q has no confined root: %v", ErrSourceMissing, spec.ID, err)
	}
	for _, root := range report.Roots {
		rootPath := filepath.ToSlash(filepath.Clean(root))
		err := fs.WalkDir(rootFS, rootPath, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symbolic link %s is outside the readable source model", path)
			}
			if entry.IsDir() || !isTaskYAML(entry.Name()) {
				return nil
			}
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("%w: live source %q: %w", ErrSourceCancelled, spec.ID, ctx.Err())
			}
			return fmt.Errorf("%w: scan source %q root %s: %v", ErrSourceMissing, spec.ID, root, err)
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: live source %q: %w", ErrSourceCancelled, spec.ID, err)
		}
		report.Counts.Files++
		reader, err := openSourceFile(ctx, path, budget, false)
		if err != nil {
			report.Counts.Unreadable++
			return err
		}
		data, readErr := io.ReadAll(reader)
		if readErr != nil {
			_ = reader.Close()
			if errors.Is(readErr, ErrBoundExceeded) {
				report.Counts.BoundsExceeded++
				report.State = SourcePartial
				addSourceReason(report, "live YAML byte bound exceeded: "+filepath.ToSlash(path))
				if budget.hitTotalBound() {
					break
				}
				continue
			}
			if errors.Is(readErr, ErrSourceCancelled) || ctx.Err() != nil {
				return fmt.Errorf("%w: live source %q: %w", ErrSourceCancelled, spec.ID, ctx.Err())
			}
			report.Counts.Unreadable++
			return fmt.Errorf("%w: read live YAML %s: %v", ErrSourceMissing, path, readErr)
		}
		metadata, ok := reader.(*boundedSourceReadCloser)
		if !ok {
			_ = reader.Close()
			return fmt.Errorf("%w: live YAML reader did not retain descriptor metadata", ErrSourceMissing)
		}
		info, infoErr := metadata.sourceFileInfo()
		_ = reader.Close()
		if infoErr != nil {
			report.Counts.Unreadable++
			return fmt.Errorf("%w: verify live YAML %s: %v", ErrSourceMissing, path, infoErr)
		}
		ref := ReadingRef{SourceID: spec.ID, Repository: spec.Repository, Path: filepath.ToSlash(path), Revision: "live", RecordedAt: info.ModTime().Round(0).UTC()}
		parsed, _ := parseReadings(data, ref)
		ingestReadings(parsed, selection, report, readings)
	}
	return nil
}

func readHistorySource(ctx context.Context, spec SourceSpec, selection Selection, budget *sourceBudget, report *SourceReport, readings *SourceReadings) error {
	shallowRaw, err := readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: []string{"rev-parse", "--is-shallow-repository"}})
	if err != nil {
		if errors.Is(err, ErrBoundExceeded) {
			markSourceBound(report, "Git metadata total-byte bound exceeded")
			return nil
		}
		return classifyHistoryReadError(spec.ID, err)
	}
	report.Shallow = strings.TrimSpace(string(shallowRaw)) == "true"
	commonDirRaw, err := readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: []string{"rev-parse", "--git-common-dir"}})
	if err != nil {
		if errors.Is(err, ErrBoundExceeded) {
			markSourceBound(report, "Git metadata total-byte bound exceeded")
			return nil
		}
		return classifyHistoryReadError(spec.ID, err)
	}
	commonDir := strings.TrimSpace(string(commonDirRaw))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(spec.Repository, commonDir)
	}
	graftsPath := filepath.Join(commonDir, "info", "grafts")
	if info, statErr := os.Stat(graftsPath); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%w: source %q graft metadata %s is not a regular file", ErrGitHistory, spec.ID, graftsPath)
		}
		report.Grafted = info.Size() > 0
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return fmt.Errorf("%w: source %q cannot inspect graft metadata %s: %w", ErrGitHistory, spec.ID, graftsPath, statErr)
	}

	var traversal []string
	if spec.Ref != "" {
		replaceRaw, replaceErr := readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: []string{"for-each-ref", "--format=%(refname)", "refs/replace"}})
		if replaceErr != nil {
			if errors.Is(replaceErr, ErrBoundExceeded) {
				markSourceBound(report, "Git metadata total-byte bound exceeded")
				return nil
			}
			return classifyHistoryReadError(spec.ID, replaceErr)
		}
		report.Replaced = strings.TrimSpace(string(replaceRaw)) != ""
		resolvedRaw, resolveErr := readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: []string{"rev-parse", "--verify", "--end-of-options", spec.Ref + "^{commit}"}})
		if resolveErr != nil {
			if errors.Is(resolveErr, ErrBoundExceeded) {
				markSourceBound(report, "Git metadata total-byte bound exceeded")
				return nil
			}
			return classifyHistoryReadError(spec.ID, resolveErr)
		}
		resolved := strings.TrimSpace(string(resolvedRaw))
		if !validObjectID(resolved) {
			return fmt.Errorf("%w: source %q ref %q did not resolve to a full commit", ErrGitHistory, spec.ID, spec.Ref)
		}
		report.ResolvedRef = resolved
		report.ResolvedRefs = []ResolvedRef{{Name: spec.Ref, Commit: resolved}}
		traversal = []string{resolved}
	} else {
		resolvedRefs, replaced, unbornHEAD, refsErr := readAllResolvedRefs(ctx, spec.Repository, budget)
		if refsErr != nil {
			if errors.Is(refsErr, ErrBoundExceeded) {
				markSourceBound(report, "Git metadata total-byte bound exceeded")
				return nil
			}
			return classifyHistoryReadError(spec.ID, refsErr)
		}
		report.ResolvedRefs = resolvedRefs
		report.Replaced = replaced
		if unbornHEAD {
			report.State = SourcePartial
			addSourceReason(report, "Git HEAD is an unborn branch")
		}
		for _, ref := range resolvedRefs {
			traversal = append(traversal, ref.Commit)
		}
	}
	if report.Shallow {
		addSourceReason(report, "Git history is shallow")
	}
	if report.Grafted {
		addSourceReason(report, "Git history has grafts")
	}
	if report.Replaced {
		addSourceReason(report, "Git history has replacement refs")
	}
	if report.Shallow || report.Grafted || report.Replaced {
		report.State = SourcePartial
	}
	if len(traversal) == 0 {
		report.State = SourcePartial
		addSourceReason(report, "Git history has no resolved commit refs")
		return nil
	}

	commits, capped, err := readHistoryCommits(ctx, spec.Repository, traversal, budget, budget.bounds.MaxCommits)
	report.Counts.Commits = len(commits)
	if err != nil {
		if errors.Is(err, ErrBoundExceeded) {
			markSourceBound(report, "Git metadata total-byte bound exceeded")
			return nil
		}
		return classifyHistoryReadError(spec.ID, err)
	}
	if capped {
		markSourceBound(report, "Git commit bound exceeded")
	}

	rootSeen := make(map[string]bool, len(report.Roots))
	type cachedBlob struct {
		data []byte
		err  error
	}
	cache := make(map[string]cachedBlob)
	stop := false
	for _, historyCommit := range commits {
		if stop {
			break
		}
		commit := historyCommit.objectID
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%w: history source %q: %w", ErrSourceCancelled, spec.ID, err)
		}
		args := []string{"ls-tree", "-r", "-z", "--full-tree", commit, "--"}
		args = append(args, report.Roots...)
		treeRaw, treeErr := readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: args})
		if treeErr != nil {
			if errors.Is(treeErr, ErrBoundExceeded) {
				markSourceBound(report, "Git metadata total-byte bound exceeded")
				break
			}
			return classifyHistoryReadError(spec.ID, treeErr)
		}
		seenPaths := make(map[string]struct{})
		for _, raw := range bytes.Split(treeRaw, []byte{0}) {
			if len(raw) == 0 {
				continue
			}
			header, pathBytes, ok := bytes.Cut(raw, []byte{'\t'})
			fields := strings.Fields(string(header))
			if !ok || len(fields) != 3 {
				return fmt.Errorf("%w: source %q received malformed ls-tree metadata", ErrGitHistory, spec.ID)
			}
			path := filepath.ToSlash(string(pathBytes))
			for _, root := range report.Roots {
				cleanRoot := filepath.ToSlash(filepath.Clean(root))
				if cleanRoot == "." || path == cleanRoot || strings.HasPrefix(path, cleanRoot+"/") {
					rootSeen[root] = true
				}
			}
			if !isTaskYAML(path) {
				continue
			}
			if _, duplicate := seenPaths[path]; duplicate {
				continue
			}
			seenPaths[path] = struct{}{}
			report.Counts.Files++
			if fields[1] != "blob" || !strings.HasPrefix(fields[0], "100") || !validObjectID(fields[2]) {
				report.Counts.Unreadable++
				report.State = SourcePartial
				addSourceReason(report, "historical YAML entry is not a regular blob: "+path)
				continue
			}
			blob, cached := cache[fields[2]]
			newBlob := !cached
			if !cached {
				report.Counts.Blobs++
				blob.data, blob.err = readSourceGitAll(ctx, spec.Repository, budget, SourceGitRequest{Args: []string{"cat-file", "blob", fields[2]}, Blob: true})
				cache[fields[2]] = blob
			}
			if blob.err != nil {
				if errors.Is(blob.err, ErrBoundExceeded) {
					if newBlob {
						markSourceBound(report, "Git blob byte bound exceeded: "+path)
					}
					if budget.hitTotalBound() {
						stop = true
						break
					}
					continue
				}
				return classifyHistoryReadError(spec.ID, blob.err)
			}
			ref := ReadingRef{
				SourceID: spec.ID, Repository: spec.Repository, Path: path,
				Revision: "git:" + commit, RecordedAt: historyCommit.recordedAt,
			}
			parsed, _ := parseReadings(blob.data, ref)
			ingestReadings(parsed, selection, report, readings)
		}
	}
	for _, root := range report.Roots {
		if !rootSeen[root] && len(commits) > 0 && !capped && !budget.hitTotalBound() {
			return fmt.Errorf("%w: history source %q root %s was not found in reachable history", ErrSourceMissing, spec.ID, root)
		}
	}
	return nil
}

func readSourceGitAll(ctx context.Context, repo string, budget *sourceBudget, request SourceGitRequest) ([]byte, error) {
	reader, err := runSourceGit(ctx, repo, budget, request)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(reader)
	_ = reader.Close()
	return data, readErr
}

func readAllResolvedRefs(ctx context.Context, repo string, budget *sourceBudget) ([]ResolvedRef, bool, bool, error) {
	raw, err := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
		Args: []string{"for-each-ref", "--format=" + sourceRefFormat},
	})
	if err != nil {
		return nil, false, false, err
	}
	refs := make([]ResolvedRef, 0)
	replaced := false
	for _, record := range bytes.Split(bytes.TrimSpace(raw), []byte{'\n'}) {
		if len(record) == 0 {
			continue
		}
		fields := bytes.Split(record, []byte{0})
		if len(fields) != 5 {
			return refs, replaced, false, fmt.Errorf("%w: malformed for-each-ref metadata", ErrGitHistory)
		}
		name := string(fields[0])
		objectType, objectID := string(fields[1]), string(fields[2])
		peeledType, peeledID := string(fields[3]), string(fields[4])
		if !validFullRefName(name) {
			return refs, replaced, false, fmt.Errorf("%w: invalid ref name %q", ErrGitHistory, name)
		}
		if !validObjectID(objectID) {
			return refs, replaced, false, fmt.Errorf("%w: ref %q has invalid captured object ID", ErrGitHistory, name)
		}
		replaced = replaced || strings.HasPrefix(name, "refs/replace/")
		commit := ""
		switch {
		case objectType == "commit":
			commit = objectID
		case peeledType == "commit" && validObjectID(peeledID):
			commit = peeledID
		default:
			resolvedRaw, resolveErr := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
				Args: []string{"rev-parse", "--verify", "--end-of-options", objectID + "^{commit}"},
			})
			if resolveErr != nil {
				if errors.Is(resolveErr, ErrBoundExceeded) || errors.Is(resolveErr, ErrSourceCancelled) ||
					errors.Is(resolveErr, context.Canceled) || errors.Is(resolveErr, context.DeadlineExceeded) {
					return refs, replaced, false, resolveErr
				}
				return refs, replaced, false, fmt.Errorf("%w: ref %q captured as %s is not a supported commit ref: %w", ErrGitHistory, name, objectID, resolveErr)
			}
			commit = strings.TrimSpace(string(resolvedRaw))
		}
		if !validObjectID(commit) {
			return refs, replaced, false, fmt.Errorf("%w: ref %q did not resolve to a full commit", ErrGitHistory, name)
		}
		refs = append(refs, ResolvedRef{Name: name, Commit: commit})
	}
	head, resolved, unborn, headErr := readCapturedHEAD(ctx, repo, budget)
	if headErr != nil {
		return refs, replaced, false, headErr
	}
	if resolved {
		refs = append(refs, head)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Name != refs[j].Name {
			return refs[i].Name < refs[j].Name
		}
		return refs[i].Commit < refs[j].Commit
	})
	return refs, replaced, unborn, nil
}

func readCapturedHEAD(ctx context.Context, repo string, budget *sourceBudget) (ResolvedRef, bool, bool, error) {
	raw, err := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
		Args: []string{"rev-parse", "--verify", "--quiet", "--end-of-options", "HEAD"},
	})
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			targetRaw, targetErr := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
				Args: []string{"symbolic-ref", "--quiet", "HEAD"},
			})
			if targetErr != nil {
				if errors.Is(targetErr, ErrBoundExceeded) || errors.Is(targetErr, ErrSourceCancelled) ||
					errors.Is(targetErr, context.Canceled) || errors.Is(targetErr, context.DeadlineExceeded) {
					return ResolvedRef{}, false, false, targetErr
				}
				return ResolvedRef{}, false, false, fmt.Errorf("%w: unresolved HEAD is not a readable symbolic ref: %w", ErrGitHistory, targetErr)
			}
			target := strings.TrimSuffix(string(targetRaw), "\n")
			target = strings.TrimSuffix(target, "\r")
			if !validFullRefName(target) {
				return ResolvedRef{}, false, false, fmt.Errorf("%w: HEAD has invalid symbolic target %q", ErrGitHistory, target)
			}
			_, verifyErr := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
				Args: []string{"show-ref", "--verify", "--quiet", target},
			})
			if verifyErr == nil {
				return ResolvedRef{}, false, false, fmt.Errorf("%w: HEAD target %q exists but HEAD did not resolve", ErrGitHistory, target)
			}
			var verifyExitErr *exec.ExitError
			if errors.As(verifyErr, &verifyExitErr) && verifyExitErr.ExitCode() == 1 {
				return ResolvedRef{}, false, true, nil
			}
			if errors.Is(verifyErr, ErrBoundExceeded) || errors.Is(verifyErr, ErrSourceCancelled) ||
				errors.Is(verifyErr, context.Canceled) || errors.Is(verifyErr, context.DeadlineExceeded) {
				return ResolvedRef{}, false, false, verifyErr
			}
			return ResolvedRef{}, false, false, fmt.Errorf("%w: cannot verify unresolved HEAD target %q: %w", ErrGitHistory, target, verifyErr)
		}
		return ResolvedRef{}, false, false, err
	}
	objectID := strings.TrimSpace(string(raw))
	if !validObjectID(objectID) {
		return ResolvedRef{}, false, false, fmt.Errorf("%w: HEAD did not resolve to a full captured object ID", ErrGitHistory)
	}
	peeledRaw, err := readSourceGitAll(ctx, repo, budget, SourceGitRequest{
		Args: []string{"rev-parse", "--verify", "--end-of-options", objectID + "^{commit}"},
	})
	if err != nil {
		if errors.Is(err, ErrBoundExceeded) || errors.Is(err, ErrSourceCancelled) ||
			errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ResolvedRef{}, false, false, err
		}
		return ResolvedRef{}, false, false, fmt.Errorf("%w: HEAD captured as %s is not a supported commit: %w", ErrGitHistory, objectID, err)
	}
	commit := strings.TrimSpace(string(peeledRaw))
	if !validObjectID(commit) {
		return ResolvedRef{}, false, false, fmt.Errorf("%w: HEAD captured as %s did not peel to a full commit", ErrGitHistory, objectID)
	}
	return ResolvedRef{Name: "HEAD", Commit: commit}, true, false, nil
}

type sourceHistoryCommit struct {
	objectID   string
	recordedAt time.Time
}

func readHistoryCommits(ctx context.Context, repo string, traversal []string, budget *sourceBudget, limit int) ([]sourceHistoryCommit, bool, error) {
	tipSet := make(map[string]struct{}, len(traversal))
	for _, tip := range traversal {
		if !validObjectID(tip) {
			return nil, false, fmt.Errorf("%w: invalid captured traversal tip %q", ErrGitHistory, tip)
		}
		tipSet[tip] = struct{}{}
	}
	tips := make([]string, 0, len(tipSet))
	for tip := range tipSet {
		tips = append(tips, tip)
	}
	sort.Strings(tips)
	capacity := limit
	if capacity > 1024 {
		capacity = 1024
	}
	commits := make([]sourceHistoryCommit, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	capped := false
	for start := 0; start < len(tips); start += sourceHistoryTipBatchSize {
		end := start + sourceHistoryTipBatchSize
		if end > len(tips) {
			end = len(tips)
		}
		batch, batchCapped, err := readHistoryCommitBatch(ctx, repo, tips[start:end], budget, limit)
		if err != nil {
			return commits, capped, err
		}
		capped = capped || batchCapped
		for _, commit := range batch {
			if _, duplicate := seen[commit.objectID]; duplicate {
				continue
			}
			if len(commits) == limit {
				capped = true
				return commits, capped, nil
			}
			seen[commit.objectID] = struct{}{}
			commits = append(commits, commit)
		}
	}
	return commits, capped, nil
}

func readHistoryCommitBatch(ctx context.Context, repo string, traversal []string, budget *sourceBudget, limit int) ([]sourceHistoryCommit, bool, error) {
	providerLimit := limit
	if limit < int(^uint(0)>>1) {
		providerLimit++
	}
	args := []string{"rev-list", "--max-count=" + strconv.Itoa(providerLimit), "--format=%cI"}
	args = append(args, traversal...)
	reader, err := runSourceGit(ctx, repo, budget, SourceGitRequest{Args: args})
	if err != nil {
		return nil, false, err
	}
	buffered := bufio.NewReaderSize(reader, 32*1024)
	capacity := limit
	if capacity > 1024 {
		capacity = 1024
	}
	commits := make([]sourceHistoryCommit, 0, capacity)
	pendingCommit := ""
	for {
		line, readErr := buffered.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			_ = reader.Close()
			return commits, false, readErr
		}
		value := strings.TrimSpace(line)
		if value != "" {
			if pendingCommit == "" {
				commit, ok := strings.CutPrefix(value, "commit ")
				if !ok || !validObjectID(commit) {
					_ = reader.Close()
					return commits, false, fmt.Errorf("%w: rev-list returned invalid commit header %q", ErrGitHistory, value)
				}
				pendingCommit = commit
			} else {
				recordedAt, parseErr := time.Parse(time.RFC3339Nano, value)
				if parseErr != nil {
					_ = reader.Close()
					return commits, false, fmt.Errorf("%w: commit %s has invalid committer time: %v", ErrGitHistory, pendingCommit, parseErr)
				}
				commits = append(commits, sourceHistoryCommit{objectID: pendingCommit, recordedAt: recordedAt.Round(0).UTC()})
				pendingCommit = ""
			}
		}
		if errors.Is(readErr, io.EOF) {
			_ = reader.Close()
			if pendingCommit != "" {
				return commits, false, fmt.Errorf("%w: rev-list omitted committer time for %s", ErrGitHistory, pendingCommit)
			}
			break
		}
	}
	capped := len(commits) > limit
	return commits, capped, nil
}

func classifyHistoryReadError(sourceID string, err error) error {
	if errors.Is(err, ErrSourceCancelled) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, ErrBoundExceeded) {
		return err
	}
	if errors.Is(err, ErrGitHistory) {
		return err
	}
	return fmt.Errorf("%w: history source %q: %w", ErrGitHistory, sourceID, err)
}

func ingestReadings(parsed []Reading, selection Selection, report *SourceReport, readings *SourceReadings) {
	holdouts := stringSet(selection.HoldoutRunIDs)
	for _, reading := range parsed {
		if reading.Kind == DocumentNotTasks {
			report.Counts.NonTaskDocuments++
			readings.Readings = append(readings.Readings, reading)
			continue
		}
		if reading.Kind == DocumentTaskRow {
			report.Counts.Records++
		}
		if reading.Identity.RunID.Known {
			if _, ok := holdouts[reading.Identity.RunID.Value]; ok {
				reading.Excluded = DispositionHeldOut
			}
		}
		if reading.Excluded == "" && readingAfterCutoff(reading, selection.Cutoff) {
			reading.Excluded = DispositionAfterCutoff
		}
		if reading.Excluded == "" && reading.Ref.RecordedAt.IsZero() {
			reading.Err = errors.Join(reading.Err, fmt.Errorf("%w: reading has no recorded revision time", ErrYAMLSource))
		}
		if reading.Excluded != "" {
			if reading.Excluded == DispositionHeldOut {
				report.Counts.ExcludedByHoldout++
			} else {
				report.Counts.ExcludedAfterCutoff++
			}
			if reading.Err != nil {
				report.Counts.MalformedExcluded++
			}
			reading.Snapshot = ReadingSnapshot{}
		} else if reading.Err != nil {
			report.Counts.Malformed++
			report.State = SourcePartial
			addSourceReason(report, "YAML contains malformed records")
		}
		readings.Readings = append(readings.Readings, reading)
	}
}

func readingAfterCutoff(reading Reading, cutoff time.Time) bool {
	cutoff = cutoff.Round(0).UTC()
	return !reading.Ref.RecordedAt.IsZero() && reading.Ref.RecordedAt.After(cutoff) ||
		reading.Identity.StartedAt.Known && reading.Identity.StartedAt.Value.After(cutoff) ||
		reading.CompletedAt.Known && reading.CompletedAt.Value.After(cutoff)
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func markSourceBound(report *SourceReport, reason string) {
	report.Counts.BoundsExceeded++
	report.State = SourcePartial
	addSourceReason(report, reason)
}

func addSourceReason(report *SourceReport, reason string) {
	if reason != "" {
		report.Reasons = append(report.Reasons, reason)
	}
}

func addManifestReason(manifest *SourceManifest, reason string) {
	if reason != "" {
		manifest.Reasons = append(manifest.Reasons, reason)
	}
}

func canonicalStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func canonicalizeSourceReport(report *SourceReport) {
	if report.Roots == nil {
		report.Roots = []string{}
	}
	if report.ResolvedRefs == nil {
		report.ResolvedRefs = []ResolvedRef{}
	}
	report.Reasons = canonicalStrings(report.Reasons)
	if report.State == "" {
		report.State = SourceComplete
	}
}

func finalizeManifest(manifest *SourceManifest) {
	partial := false
	for i := range manifest.Sources {
		canonicalizeSourceReport(&manifest.Sources[i])
		if manifest.Sources[i].State != SourceComplete {
			partial = true
		}
		for _, reason := range manifest.Sources[i].Reasons {
			manifest.Reasons = append(manifest.Reasons, manifest.Sources[i].ID+": "+reason)
		}
	}
	manifest.Reasons = canonicalStrings(manifest.Reasons)
	if partial || manifest.State == SourcePartial {
		manifest.State = SourcePartial
	} else {
		manifest.State = SourceComplete
	}
}

func sortSourceReadings(readings *SourceReadings) {
	sort.Slice(readings.Journals, func(i, j int) bool {
		return journalIdentityLess(readings.Journals[i].Journal, readings.Journals[j].Journal)
	})
	sort.Slice(readings.ExcludedJournals, func(i, j int) bool {
		return journalIdentityLess(readings.ExcludedJournals[i], readings.ExcludedJournals[j])
	})
	sort.SliceStable(readings.Readings, func(i, j int) bool { return readingRefLess(readings.Readings[i].Ref, readings.Readings[j].Ref) })
}

func journalIdentityLess(a, b JournalIdentity) bool {
	if a.RunID != b.RunID {
		return a.RunID < b.RunID
	}
	if a.SourceID != b.SourceID {
		return a.SourceID < b.SourceID
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Producer < b.Producer
}

func readingRefLess(a, b ReadingRef) bool {
	if a.SourceID != b.SourceID {
		return a.SourceID < b.SourceID
	}
	if a.Repository != b.Repository {
		return a.Repository < b.Repository
	}
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	if a.Revision != b.Revision {
		return a.Revision < b.Revision
	}
	if a.Row != b.Row {
		return a.Row < b.Row
	}
	return a.RecordedAt.UTC().Before(b.RecordedAt.UTC())
}

// ---------------------------------------------------------------------------
// FC-1 baseline readers, moved from extract.go unchanged. Build still calls
// these; FC-SOURCES replaces them behind ReadSources.
// ---------------------------------------------------------------------------

// readJournals reads direct child run directories of runsDir. A line that is
// not JSON, and an event whose timestamp is not RFC 3339, are counted and
// skipped; a journal that cannot be opened or scanned is an error.
//
// Superseded: zero journals is not an error here (FC-1 panel Grok-1); F3
// makes it ErrSourceEmpty in ReadSources.
func readJournals(ctx context.Context, runsDir string) (*journalSources, error) {
	info, err := os.Stat(runsDir)
	if err != nil {
		return nil, fmt.Errorf("%w: stat %s: %v", ErrJournalSource, runsDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrJournalSource, runsDir)
	}
	pattern := filepath.Join(runsDir, "*", "journal.jsonl")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: glob %s: %v", ErrJournalSource, pattern, err)
	}
	sort.Strings(paths)
	sources := &journalSources{Rows: make(map[runTask]*JournalRow)}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("%w: reading %s: %v", ErrJournalSource, path, err)
		}
		if err := readJournal(ctx, path, filepath.Base(filepath.Dir(path)), sources); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

func readJournal(ctx context.Context, path, runID string, out *journalSources) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("%w: open %s: %v", ErrJournalSource, path, err)
	}
	defer f.Close()

	return scanJournal(ctx, f, path, runID, out)
}

func readLiveSnapshots(ctx context.Context, featuresRepo string) (yamlSources, error) {
	featuresDir := filepath.Join(featuresRepo, "features")
	var paths []string
	err := filepath.WalkDir(featuresDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if !entry.IsDir() && isTaskYAML(entry.Name()) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return yamlSources{}, fmt.Errorf("%w: walk %s: %v", ErrYAMLSource, featuresDir, err)
	}
	sort.Strings(paths)
	var out yamlSources
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return yamlSources{}, fmt.Errorf("%w: read %s: %v", ErrYAMLSource, path, err)
		}
		source := parseSnapshots(data, Revision{Source: SourceLive})
		relative, _ := filepath.Rel(featuresRepo, path)
		for i := range source.Snapshots {
			source.Snapshots[i].Repository = featuresRepo
			source.Snapshots[i].Path = filepath.ToSlash(relative)
		}
		out.absorb(source)
	}
	return out, nil
}

// gitSources adds history-walk shape to what the blobs yielded.
type gitSources struct {
	yamlSources
	Commits   int
	Blobs     int
	Truncated bool
}

// blobRef is one distinct file content, attributed to the first commit in
// walk order that holds it.
type blobRef struct {
	oid    string
	path   string
	commit string
}

// readGitSnapshots reads changed YAML blobs in a bounded commit walk. Git
// enumerates at most maxCommits+1 commits; the extra commit detects truncation.
// One diff-tree process lists changes, and one cat-file process reads blobs.
//
// Superseded: rev-list without --full-history drops superseded merge parents
// (FC-1 panel Claude-2), shallow clones look complete (Grok-3), the parent
// git environment is inherited (Grok-4) and blobs are buffered whole
// (Claude-5/Grok-5). F3 corrects all four in ReadSources.
func readGitSnapshots(ctx context.Context, featuresRepo string, maxCommits int) (gitSources, error) {
	if maxCommits <= 0 {
		maxCommits = defaultMaxHistoryCommits
	}
	// Avoid overflow for the lookahead used to detect truncation.
	limit := maxCommits
	if limit < int(^uint(0)>>1) {
		limit++
	}
	commits, err := gitLines(ctx, featuresRepo, "rev-list", "--max-count="+strconv.Itoa(limit), "--all", "--", "features")
	if err != nil {
		return gitSources{}, err
	}
	out := gitSources{}
	if len(commits) > maxCommits {
		commits, out.Truncated = commits[:maxCommits], true
	}
	out.Commits = len(commits)
	if len(commits) == 0 {
		return out, nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", featuresRepo, "diff-tree", "--stdin", "--root", "-r", "-m", "--raw", "-z", "--no-abbrev", "--no-renames", "--", "features")
	cmd.Stdin = strings.NewReader(strings.Join(commits, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return gitSources{}, fmt.Errorf("%w: git diff-tree in %s: %v: %s", ErrGitHistory, featuresRepo, err, strings.TrimSpace(stderr.String()))
	}
	records := strings.Split(stdout.String(), "\x00")
	seen := make(map[string]blobRef)
	var order []string
	commit := ""
	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}
		if !strings.HasPrefix(record, ":") {
			commit = record
			continue
		}
		fields := strings.Fields(record)
		if len(fields) != 5 || i+1 >= len(records) || commit == "" {
			return gitSources{}, fmt.Errorf("%w: malformed git diff-tree record %q", ErrGitHistory, record)
		}
		i++
		path, oid := records[i], fields[3]
		if fields[4] == "D" || !strings.HasPrefix(fields[1], "100") || !isTaskYAML(path) {
			continue
		}
		if _, ok := seen[oid]; ok {
			continue
		}
		seen[oid] = blobRef{oid: oid, path: path, commit: commit}
		order = append(order, oid)
	}
	out.Blobs = len(order)
	if len(order) == 0 {
		return out, nil
	}
	blobs, err := gitCatFileBatch(ctx, featuresRepo, order)
	if err != nil {
		return gitSources{}, err
	}
	for _, oid := range order {
		data, ok := blobs[oid]
		if !ok {
			return gitSources{}, fmt.Errorf("%w: git cat-file did not return blob %s (%s)", ErrGitHistory, oid, seen[oid].path)
		}
		source := parseSnapshots(data, Revision{Source: SourceGit, Commit: seen[oid].commit})
		for i := range source.Snapshots {
			source.Snapshots[i].Repository = featuresRepo
			source.Snapshots[i].Path = seen[oid].path
		}
		out.absorb(source)
	}
	return out, nil
}

// gitCatFileBatch reads every requested blob through one child process.
func gitCatFileBatch(ctx context.Context, repo string, oids []string) (map[string][]byte, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repo, "cat-file", "--batch")
	cmd.Stdin = strings.NewReader(strings.Join(oids, "\n") + "\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: git cat-file --batch in %s: %v: %s", ErrGitHistory, repo, err, strings.TrimSpace(stderr.String()))
	}
	out := make(map[string][]byte, len(oids))
	rest := stdout.Bytes()
	for len(rest) > 0 {
		newline := bytes.IndexByte(rest, '\n')
		if newline < 0 {
			return nil, fmt.Errorf("%w: git cat-file --batch produced a truncated header", ErrGitHistory)
		}
		header := string(rest[:newline])
		rest = rest[newline+1:]
		fields := strings.Fields(header)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%w: git cat-file --batch header %q", ErrGitHistory, header)
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || size < 0 || size > len(rest) {
			return nil, fmt.Errorf("%w: git cat-file --batch header %q", ErrGitHistory, header)
		}
		out[fields[0]] = append([]byte(nil), rest[:size]...)
		rest = rest[size:]
		if len(rest) > 0 && rest[0] == '\n' {
			rest = rest[1:]
		}
	}
	return out, nil
}

// gitLines runs git and splits its output. Records are NUL-separated when -z
// is passed and newline-separated otherwise. stderr is carried into the error
// because "exit status 128" alone names neither the repository nor the cause.
func gitLines(ctx context.Context, repo string, args ...string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: git %s in %s: %v: %s",
			ErrGitHistory, strings.Join(args, " "), repo, err, strings.TrimSpace(stderr.String()))
	}
	separator := "\n"
	for _, arg := range args {
		if arg == "-z" {
			separator = "\x00"
		}
	}
	var lines []string
	for _, line := range strings.Split(stdout.String(), separator) {
		if line = strings.Trim(line, "\n"); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// readTargetTasks reads and validates the target task list. A row without a
// key, valid role and nonblank model, or a repeated key, wraps ErrYAMLSource
// only, preserving baseline behavior. The amended FC-1 path uses ErrInvalidTarget.
func readTargetTasks(path string) ([]yamlTask, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read target %s: %v", ErrYAMLSource, path, err)
	}
	doc, err := decodeTaskDocument(data)
	if err != nil {
		return nil, fmt.Errorf("%w: parse target %s: %v", ErrYAMLSource, path, err)
	}
	seen := make(map[string]bool)
	for i, task := range doc.Tasks {
		if strings.TrimSpace(task.Key) == "" || !task.Role.Valid() || strings.TrimSpace(task.Model) == "" {
			return nil, fmt.Errorf("%w: target %s row %d (%q) requires a key, valid role and model", ErrYAMLSource, path, i+1, task.Key)
		}
		if seen[task.Key] {
			return nil, fmt.Errorf("%w: target %s repeats key %q", ErrYAMLSource, path, task.Key)
		}
		seen[task.Key] = true
	}
	return doc.Tasks, nil
}
