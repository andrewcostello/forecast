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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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
	return fmt.Errorf("%w: ReadBounds.Validate", ErrNotImplemented)
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
	return fmt.Errorf("%w: SourceSpec.Validate", ErrNotImplemented)
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
	return fmt.Errorf("%w: Selection.Validate", ErrNotImplemented)
}

// UnmatchedHoldouts wraps ErrInvalidSelection when any held-out run ID names
// no run in discoveredRunIDs, the run IDs of every journal found across all
// journal sources BEFORE exclusion. A holdout that matches nothing is a
// misspelling, and silently ignoring it would put the run it meant to hold
// out into the corpus the artifact is then used to predict. ReadSources
// calls this once discovery is complete. This is a named stub for FC-SOURCES.
func (s Selection) UnmatchedHoldouts(discoveredRunIDs []string) error {
	return fmt.Errorf("%w: Selection.UnmatchedHoldouts", ErrNotImplemented)
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
	return fmt.Errorf("%w: SourceManifest.ValidateComplete", ErrNotImplemented)
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
// ErrSourceCancelled (also ctx.Err()), and ErrJournalSource for a journal read.
// F3 tables and "Entry-point contracts" in notes/FC-SCAFFOLD.md are authoritative.
// FC-SOURCES body; all amended Git children must use runSourceGit, never the
// inherited-env legacy helpers below.
func ReadSources(ctx context.Context, specs []SourceSpec, selection Selection, bounds ReadBounds) (*SourceManifest, *SourceReadings, error) {
	return nil, nil, fmt.Errorf("%w: ReadSources(%d sources)", ErrNotImplemented, len(specs))
}

// sourceBudget is one source's shared byte/process budget. FC-SOURCES may add
// private implementation state here. ReadSources shares it across Git, journal
// and live-file reads; it is never reset per child command.
type sourceBudget struct{ bounds ReadBounds }

// newSourceBudget validates/resolves bounds before I/O. FC-SOURCES body.
func newSourceBudget(bounds ReadBounds) (*sourceBudget, error) {
	return nil, fmt.Errorf("%w: newSourceBudget", ErrNotImplemented)
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
	return nil, fmt.Errorf("%w: runSourceGit", ErrNotImplemented)
}

// sourceGitCommand constructs an amended read-only Git command with an explicit
// isolated environment. No inherited Git location/config overrides or helpers;
// system/global config disabled, repository pinned. See F3-GIT-ENV-STRIPPED and
// "Entry-point contracts". It does not spawn; runSourceGit applies process/byte
// bounds. Called only by runSourceGit. FC-SOURCES body; legacy helpers remain
// on their original environment and cannot be reused by the amended reader.
func sourceGitCommand(ctx context.Context, repo string, args ...string) (*exec.Cmd, error) {
	return nil, fmt.Errorf("%w: sourceGitCommand", ErrNotImplemented)
}

// ValidateReadingRevision requires exactly "live" or "git:" plus a full lowercase
// 40- or 64-hex object ID. No bare/abbreviated SHA or live:<mtime>. Mtime belongs
// only in RecordedAt. ReadSources resolves refs before formatting; this pure
// validator never resolves/fetches. Invalid input wraps ErrUnparseableRevision.
// It narrows the existing legacy ParseRevision grammar without changing it.
func ValidateReadingRevision(revision string) error {
	return fmt.Errorf("%w: ValidateReadingRevision", ErrNotImplemented)
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
	return nil, fmt.Errorf("%w: parseReadings", ErrNotImplemented)
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
