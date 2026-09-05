package dispatched

// sources.go: filesystem and Git reads of explicit sources.
//
// Ownership: FC-SOURCES implements the frozen seam in this file
// (ReadSources) and may replace the baseline readers. The baseline readers
// are the FC-1 code moved verbatim from extract.go; Build still runs them
// until FC-1 switches Build to ReadSources, so the artifact does not change
// under the move. Everything in this file that touches the filesystem or
// runs git lives here; YAML decoding stays in extract.go.

import (
	"bytes"
	"context"
	"fmt"
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
	// DefaultMaxLineBytes is the longest journal line that is decoded; the
	// baseline scanner used this value.
	DefaultMaxLineBytes = 16 * 1024 * 1024
	// DefaultMaxBlobBytes is the largest single YAML blob that is read.
	DefaultMaxBlobBytes = 16 * 1024 * 1024
	// DefaultMaxTotalBytes caps the bytes of YAML content read from one
	// source across all blobs and files.
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

// WithDefaults fills zero fields.
func (b ReadBounds) WithDefaults() ReadBounds {
	if b.MaxCommits == 0 {
		b.MaxCommits = defaultMaxHistoryCommits
	}
	if b.MaxLineBytes == 0 {
		b.MaxLineBytes = DefaultMaxLineBytes
	}
	if b.MaxBlobBytes == 0 {
		b.MaxBlobBytes = DefaultMaxBlobBytes
	}
	if b.MaxTotalBytes == 0 {
		b.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if b.MaxProcesses == 0 {
		b.MaxProcesses = DefaultMaxProcesses
	}
	return b
}

// Validate wraps ErrInvalidSourceSpec for any negative bound.
func (b ReadBounds) Validate() error {
	if b.MaxCommits < 0 || b.MaxLineBytes < 0 || b.MaxBlobBytes < 0 || b.MaxTotalBytes < 0 || b.MaxProcesses < 0 {
		return fmt.Errorf("%w: read bounds must not be negative: %+v", ErrInvalidSourceSpec, b)
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
// undeclared kind, a blank or absolute root, or roots on a journal source.
func (s SourceSpec) Validate() error {
	switch {
	case strings.TrimSpace(s.ID) == "":
		return fmt.Errorf("%w: source has no ID", ErrInvalidSourceSpec)
	case !s.Kind.Valid():
		return fmt.Errorf("%w: source %s has kind %q", ErrInvalidSourceSpec, s.ID, s.Kind)
	case strings.TrimSpace(s.Repository) == "":
		return fmt.Errorf("%w: source %s has no repository", ErrInvalidSourceSpec, s.ID)
	case s.Kind == SourceKindJournals && len(s.Roots) > 0:
		return fmt.Errorf("%w: journal source %s does not take roots", ErrInvalidSourceSpec, s.ID)
	case s.Kind != SourceKindJournals && len(s.Roots) == 0:
		return fmt.Errorf("%w: source %s names no roots", ErrInvalidSourceSpec, s.ID)
	}
	for _, root := range s.Roots {
		clean := filepath.ToSlash(filepath.Clean(root))
		if strings.TrimSpace(root) == "" || filepath.IsAbs(root) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
			return fmt.Errorf("%w: source %s root %q must be a relative path inside the repository", ErrInvalidSourceSpec, s.ID, root)
		}
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
	// SourcePartial: something was read, and at least one of: a bound was
	// hit, history is shallow/grafted/replaced, a record was unreadable or
	// malformed, or the read was cancelled. Valid records are retained.
	SourcePartial SourceState = "PARTIAL"
	// SourceEmpty: the source was read successfully and yielded zero
	// records. Reachable only under Selection.AllowEmpty; otherwise
	// ErrSourceEmpty. Never prediction-eligible.
	SourceEmpty SourceState = "EMPTY"
)

// SourceCounts are the per-source tallies the manifest stores.
type SourceCounts struct {
	Files         int   `json:"files"`
	Journals      int   `json:"journals"`
	Commits       int   `json:"commits"`
	Blobs         int   `json:"blobs"`
	Bytes         int64 `json:"bytes"`
	Records       int   `json:"records"`
	Malformed     int   `json:"malformed"`
	Unreadable    int   `json:"unreadable"`
	ExcludedByRun int   `json:"excluded_by_holdout"`
	AfterCutoff   int   `json:"excluded_after_cutoff"`
	// BoundsExceeded counts the commit, line, blob and total-byte caps that
	// stopped a read of this source. MaxProcesses never contributes to it.
	BoundsExceeded int `json:"bounds_exceeded"`
}

// SourceReport is one source's identity, what was asked for, what was
// actually resolved and how the read ended.
type SourceReport struct {
	ID           string       `json:"id"`
	Kind         SourceKind   `json:"kind"`
	Repository   string       `json:"repository"`
	Roots        []string     `json:"roots,omitempty"`
	RequestedRef string       `json:"requested_ref,omitempty"`
	ResolvedRef  string       `json:"resolved_ref,omitempty"`
	State        SourceState  `json:"state"`
	Reasons      []string     `json:"reasons,omitempty"`
	Shallow      bool         `json:"shallow"`
	Cancelled    bool         `json:"cancelled"`
	Counts       SourceCounts `json:"counts"`
}

// Selection freezes, before extraction, which evidence is excluded: runs
// held out for validation and the extraction cutoff. Evidence from a
// held-out run is excluded at the source boundary for BOTH live and
// historical joins; evidence after Cutoff is excluded and counted.
//
// AllowEmpty is the explicit diagnostic mode: zero journals or zero readings
// then yield an EMPTY manifest instead of ErrSourceEmpty. An EMPTY or
// PARTIAL manifest is never prediction-eligible.
type Selection struct {
	Cutoff        time.Time
	HoldoutRunIDs []string
	AllowEmpty    bool
}

// Validate wraps ErrInvalidSelection for a held-out run ID that is blank,
// that differs from its whitespace-trimmed form, or that is listed twice.
// Rejecting an untrimmed ID (rather than trimming it) keeps the stored list
// identical to the compared one, so a holdout can never pass validation and
// then fail to match the run it names.
func (s Selection) Validate() error {
	seen := make(map[string]bool, len(s.HoldoutRunIDs))
	for _, id := range s.HoldoutRunIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return fmt.Errorf("%w: blank held-out run ID", ErrInvalidSelection)
		}
		if trimmed != id {
			return fmt.Errorf("%w: held-out run ID %q has surrounding whitespace", ErrInvalidSelection, id)
		}
		if seen[id] {
			return fmt.Errorf("%w: held-out run %q listed twice", ErrInvalidSelection, id)
		}
		seen[id] = true
	}
	return nil
}

// HeldOut reports whether runID is excluded by the selection. Both sides are
// compared in whitespace-trimmed form so a caller that skipped Validate
// still cannot leak a padded holdout into the corpus.
func (s Selection) HeldOut(runID string) bool {
	want := strings.TrimSpace(runID)
	if want == "" {
		return false
	}
	for _, id := range s.HoldoutRunIDs {
		if strings.TrimSpace(id) == want {
			return true
		}
	}
	return false
}

// UnmatchedHoldouts wraps ErrInvalidSelection when any held-out run ID names
// no run in discoveredRunIDs, the run IDs of every journal found across all
// journal sources BEFORE exclusion. A holdout that matches nothing is a
// misspelling, and silently ignoring it would put the run it meant to hold
// out into the corpus the artifact is then used to predict. ReadSources
// calls this once discovery is complete (F3-HOLDOUT-UNMATCHED); it is a pure
// check so seals can exercise it directly.
func (s Selection) UnmatchedHoldouts(discoveredRunIDs []string) error {
	discovered := make(map[string]bool, len(discoveredRunIDs))
	for _, id := range discoveredRunIDs {
		discovered[strings.TrimSpace(id)] = true
	}
	var missing []string
	for _, id := range s.HoldoutRunIDs {
		if !discovered[strings.TrimSpace(id)] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: held-out run IDs match no discovered run: %q", ErrInvalidSelection, missing)
	}
	return nil
}

// SourceManifest is the record of every source supplied to one extraction.
// No claim extends beyond it.
type SourceManifest struct {
	Sources       []SourceReport `json:"sources"`
	Cutoff        time.Time      `json:"cutoff"`
	HoldoutRunIDs []string       `json:"holdout_run_ids,omitempty"`
	AllowEmpty    bool           `json:"allow_empty"`
	Bounds        ReadBounds     `json:"bounds"`
	State         SourceState    `json:"state"`
}

// Complete reports whether every source is COMPLETE. Only a complete
// manifest can pass a prediction gate.
func (m SourceManifest) Complete() bool {
	if len(m.Sources) == 0 {
		return false
	}
	for _, s := range m.Sources {
		if s.State != SourceComplete {
			return false
		}
	}
	return true
}

// ReadingRef identifies one reading of a tasks YAML: the source, the
// repository-relative path, and the revision it was read at.
type ReadingRef struct {
	SourceID   string
	Repository string
	Path       string
	Revision   Revision
}

// Less is the strict total order used for deterministic evidence ties:
// live before git, then repository, path, commit, source ID.
func (r ReadingRef) Less(other ReadingRef) bool {
	switch {
	case r.Revision.Source != other.Revision.Source:
		return r.Revision.Source == SourceLive
	case r.Repository != other.Repository:
		return r.Repository < other.Repository
	case r.Path != other.Path:
		return r.Path < other.Path
	case r.Revision.Commit != other.Revision.Commit:
		return r.Revision.Commit < other.Revision.Commit
	}
	return r.SourceID < other.SourceID
}

func (r ReadingRef) String() string {
	return r.SourceID + ":" + r.Repository + "/" + r.Path + "@" + r.Revision.String()
}

// SourceReadings is everything ReadSources recovered: every parsed journal
// (with its resolved identity, events and diagnostics), ordered by
// JournalIdentity, and every discovered tasks-YAML row as a Reading envelope,
// ordered by ReadingRef then row. Readings that failed to parse are present
// with Err set so the join can give each one its Disposition; selection
// exclusions (holdout, cutoff) have already been applied and counted per
// source and those readings are NOT present here.
type SourceReadings struct {
	Journals []ParsedJournal
	Readings []Reading
}

// gitEnvironmentOverrides are the variables that redirect git away from the
// repository named on the command line. Every git child spawned against a
// selected repository runs with them stripped (FC-1 panel Grok-4).
var gitEnvironmentOverrides = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_INDEX_FILE", "GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
}

// gitEnvironment returns parent without the location overrides. Pure; the
// baseline readers below do not yet apply it (FC-SOURCES wires it).
func gitEnvironment(parent []string) []string {
	out := make([]string, 0, len(parent))
	for _, kv := range parent {
		name, _, _ := strings.Cut(kv, "=")
		stripped := false
		for _, override := range gitEnvironmentOverrides {
			if name == override {
				stripped = true
				break
			}
		}
		if !stripped {
			out = append(out, kv)
		}
	}
	return out
}

// ReadSources reads every spec under bounds and selection and returns the
// manifest and readings. Rules: a missing or unreadable requested source is
// ErrSourceMissing; zero journals or zero readings without AllowEmpty is
// ErrSourceEmpty; commit enumeration is bounded before collection; blobs
// are streamed and capped by bytes; shallow, grafted or replaced history is
// PARTIAL (or ErrShallowHistory when the caller demands complete history);
// cancellation is ErrSourceCancelled with the partial manifest returned
// beside it; held-out runs and post-cutoff records are excluded at this
// boundary and counted; a held-out run ID that matches no discovered journal
// is ErrInvalidSelection via Selection.UnmatchedHoldouts; MaxProcesses only
// serialises git children and never marks a source PARTIAL; git runs with
// gitEnvironment; history enumeration uses --full-history over the roots and
// includes deleted and renamed paths.
//
// FC-SOURCES body. Parameters are named so the body can use them; the
// scaffold returns ErrNotImplemented and reads none of them.
func ReadSources(ctx context.Context, specs []SourceSpec, selection Selection, bounds ReadBounds) (*SourceManifest, *SourceReadings, error) {
	return nil, nil, fmt.Errorf("%w: ReadSources(%d sources)", ErrNotImplemented, len(specs))
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
// (baseline) and ErrInvalidTarget (F4) so both classifications hold.
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
			return nil, fmt.Errorf("%w: %w: target %s row %d (%q) requires a key, valid role and model", ErrYAMLSource, ErrInvalidTarget, path, i+1, task.Key)
		}
		if seen[task.Key] {
			return nil, fmt.Errorf("%w: %w: target %s repeats key %q", ErrYAMLSource, ErrInvalidTarget, path, task.Key)
		}
		seen[task.Key] = true
	}
	return doc.Tasks, nil
}
